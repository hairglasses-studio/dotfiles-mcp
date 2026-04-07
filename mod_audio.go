// mod_audio.go — PipeWire/WirePlumber audio control tools via wpctl/pactl
package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// audioCheckTool checks if a CLI tool is available on PATH.
func audioCheckTool(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found on PATH — is PipeWire/WirePlumber running? (pacman -S wireplumber pipewire-pulse)", name)
	}
	return nil
}

// audioRunCmd executes a command with context and returns combined output.
func audioRunCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

// audioSinkArg returns the wpctl sink identifier, defaulting to @DEFAULT_AUDIO_SINK@.
func audioSinkArg(sink string) string {
	if sink == "" {
		return "@DEFAULT_AUDIO_SINK@"
	}
	return sink
}

// audioSourceArg returns the wpctl source identifier, defaulting to @DEFAULT_AUDIO_SOURCE@.
func audioSourceArg(source string) string {
	if source == "" {
		return "@DEFAULT_AUDIO_SOURCE@"
	}
	return source
}

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------

// AudioStatusInput defines the input for the audio_status tool.
type AudioStatusInput struct{}

// AudioVolumeInput defines the input for the audio_volume tool.
type AudioVolumeInput struct {
	Volume string `json:"volume" jsonschema:"required,description=Volume level: absolute (e.g. '50' for 50%% or '0.5' for 50%%) or relative (e.g. '+5' or '-10' for percentage adjustment)"`
	Sink   string `json:"sink,omitempty" jsonschema:"description=Sink ID or name. Defaults to @DEFAULT_AUDIO_SINK@"`
}

// AudioMuteInput defines the input for the audio_mute tool.
type AudioMuteInput struct {
	Action string `json:"action" jsonschema:"required,description=Mute action: toggle/on/off,enum=toggle,enum=on,enum=off"`
	Sink   string `json:"sink,omitempty" jsonschema:"description=Sink ID or name. Defaults to @DEFAULT_AUDIO_SINK@"`
}

// AudioDeviceSwitchInput defines the input for the audio_device_switch tool.
type AudioDeviceSwitchInput struct {
	Device string `json:"device" jsonschema:"required,description=Device name or numeric PipeWire node ID to set as default output"`
}

// AudioDevicesInput defines the input for the audio_devices tool.
type AudioDevicesInput struct{}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------

// AudioResult wraps a string result so the MCP response is a JSON object.
type AudioResult struct {
	Result string `json:"result"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// AudioModule provides PipeWire/WirePlumber audio control tools.
type AudioModule struct{}

func (m *AudioModule) Name() string        { return "audio" }
func (m *AudioModule) Description() string { return "PipeWire/WirePlumber audio control" }

func (m *AudioModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── audio_status ──────────────────────────────────
		handler.TypedHandler[AudioStatusInput, AudioResult](
			"audio_status",
			"Show current audio status: default sink/source, volume level, mute state. Uses wpctl to query PipeWire.",
			func(ctx context.Context, _ AudioStatusInput) (AudioResult, error) {
				if err := audioCheckTool("wpctl"); err != nil {
					return AudioResult{}, err
				}

				out, err := audioRunCmd(ctx, "wpctl", "status")
				if err != nil {
					return AudioResult{}, fmt.Errorf("failed to get audio status: %w", err)
				}

				// Get default sink volume/mute via wpctl get-volume
				sinkVol, _ := audioRunCmd(ctx, "wpctl", "get-volume", "@DEFAULT_AUDIO_SINK@")
				sourceVol, _ := audioRunCmd(ctx, "wpctl", "get-volume", "@DEFAULT_AUDIO_SOURCE@")

				var sb strings.Builder
				sb.WriteString("=== PipeWire Audio Status ===\n\n")

				// Default sink volume
				if sinkVol != "" {
					sb.WriteString("Default Sink: ")
					sb.WriteString(strings.TrimSpace(sinkVol))
					sb.WriteString("\n")
				}

				// Default source volume
				if sourceVol != "" {
					sb.WriteString("Default Source: ")
					sb.WriteString(strings.TrimSpace(sourceVol))
					sb.WriteString("\n")
				}

				sb.WriteString("\n--- wpctl status ---\n")
				sb.WriteString(strings.TrimSpace(out))
				sb.WriteString("\n")

				return AudioResult{Result: sb.String()}, nil
			},
		),

		// ── audio_volume ──────────────────────────────────
		handler.TypedHandler[AudioVolumeInput, AudioResult](
			"audio_volume",
			"Set audio volume. Accepts absolute percentage (e.g. '50'), float 0-1 (e.g. '0.5'), or relative adjustment (e.g. '+5', '-10'). Uses wpctl to control PipeWire.",
			func(ctx context.Context, input AudioVolumeInput) (AudioResult, error) {
				if err := audioCheckTool("wpctl"); err != nil {
					return AudioResult{}, err
				}

				if input.Volume == "" {
					return AudioResult{}, fmt.Errorf("[%s] volume must not be empty", handler.ErrInvalidParam)
				}

				sink := audioSinkArg(input.Sink)
				vol := strings.TrimSpace(input.Volume)

				var wpctlVol string

				if strings.HasPrefix(vol, "+") || strings.HasPrefix(vol, "-") {
					// Relative volume: "+5" -> "5%+", "-10" -> "10%-"
					numStr := strings.TrimLeft(vol, "+-")
					num, err := strconv.ParseFloat(numStr, 64)
					if err != nil || num < 0 {
						return AudioResult{}, fmt.Errorf("[%s] invalid relative volume %q — use e.g. '+5' or '-10'", handler.ErrInvalidParam, vol)
					}
					if strings.HasPrefix(vol, "+") {
						wpctlVol = fmt.Sprintf("%.0f%%+", num)
					} else {
						wpctlVol = fmt.Sprintf("%.0f%%-", num)
					}
				} else {
					// Absolute volume
					num, err := strconv.ParseFloat(vol, 64)
					if err != nil {
						return AudioResult{}, fmt.Errorf("[%s] invalid volume %q — use integer percentage (e.g. '50'), float 0-1 (e.g. '0.5'), or relative (e.g. '+5', '-10')", handler.ErrInvalidParam, vol)
					}

					if num > 1.0 && num <= 150 {
						// Treat as percentage: "50" -> 0.50
						wpctlVol = fmt.Sprintf("%.2f", num/100.0)
					} else if num >= 0 && num <= 1.0 {
						// Already a float 0-1
						wpctlVol = fmt.Sprintf("%.2f", num)
					} else {
						return AudioResult{}, fmt.Errorf("[%s] volume out of range: %.0f — use 0-150 (percentage) or 0.0-1.0 (float)", handler.ErrInvalidParam, num)
					}
				}

				_, err := audioRunCmd(ctx, "wpctl", "set-volume", sink, wpctlVol)
				if err != nil {
					return AudioResult{}, fmt.Errorf("failed to set volume: %w", err)
				}

				// Read back the new volume for confirmation
				newVol, _ := audioRunCmd(ctx, "wpctl", "get-volume", sink)

				return AudioResult{Result: fmt.Sprintf("Volume set to %s (sink: %s)\nCurrent: %s", vol, sink, strings.TrimSpace(newVol))}, nil
			},
		),

		// ── audio_mute ────────────────────────────────────
		handler.TypedHandler[AudioMuteInput, AudioResult](
			"audio_mute",
			"Toggle or set mute state for an audio sink. Actions: toggle, on (mute), off (unmute). Uses wpctl.",
			func(ctx context.Context, input AudioMuteInput) (AudioResult, error) {
				if err := audioCheckTool("wpctl"); err != nil {
					return AudioResult{}, err
				}

				sink := audioSinkArg(input.Sink)

				var muteArg string
				switch input.Action {
				case "toggle":
					muteArg = "toggle"
				case "on":
					muteArg = "1"
				case "off":
					muteArg = "0"
				default:
					return AudioResult{}, fmt.Errorf("[%s] action must be one of: toggle, on, off", handler.ErrInvalidParam)
				}

				_, err := audioRunCmd(ctx, "wpctl", "set-mute", sink, muteArg)
				if err != nil {
					return AudioResult{}, fmt.Errorf("failed to set mute: %w", err)
				}

				// Read back volume to confirm mute state
				newVol, _ := audioRunCmd(ctx, "wpctl", "get-volume", sink)
				volStr := strings.TrimSpace(newVol)

				muted := strings.Contains(volStr, "[MUTED]")
				stateStr := "unmuted"
				if muted {
					stateStr = "muted"
				}

				return AudioResult{Result: fmt.Sprintf("Sink %s is now %s\nCurrent: %s", sink, stateStr, volStr)}, nil
			},
		),

		// ── audio_device_switch ───────────────────────────
		handler.TypedHandler[AudioDeviceSwitchInput, AudioResult](
			"audio_device_switch",
			"Switch the default audio output device. Provide a PipeWire node ID (numeric) or a device name substring to match. Uses wpctl set-default.",
			func(ctx context.Context, input AudioDeviceSwitchInput) (AudioResult, error) {
				if err := audioCheckTool("wpctl"); err != nil {
					return AudioResult{}, err
				}

				device := strings.TrimSpace(input.Device)
				if device == "" {
					return AudioResult{}, fmt.Errorf("[%s] device must not be empty", handler.ErrInvalidParam)
				}

				// If numeric, use directly as node ID
				if _, err := strconv.Atoi(device); err == nil {
					_, setErr := audioRunCmd(ctx, "wpctl", "set-default", device)
					if setErr != nil {
						return AudioResult{}, fmt.Errorf("failed to set default device to ID %s: %w", device, setErr)
					}
					return AudioResult{Result: fmt.Sprintf("Default audio output set to node ID %s", device)}, nil
				}

				// Non-numeric: search sinks by name using pactl
				if err := audioCheckTool("pactl"); err != nil {
					return AudioResult{}, fmt.Errorf("pactl needed for name-based device lookup: %w", err)
				}

				sinks, err := audioRunCmd(ctx, "pactl", "list", "sinks", "short")
				if err != nil {
					return AudioResult{}, fmt.Errorf("failed to list sinks: %w", err)
				}

				deviceLower := strings.ToLower(device)
				var matchID, matchName string

				for _, line := range strings.Split(strings.TrimSpace(sinks), "\n") {
					fields := strings.Fields(line)
					if len(fields) < 2 {
						continue
					}
					sinkName := fields[1]
					if strings.Contains(strings.ToLower(sinkName), deviceLower) {
						matchID = fields[0]
						matchName = sinkName
						break
					}
				}

				if matchID == "" {
					return AudioResult{}, fmt.Errorf("[%s] no sink matching %q found — use audio_devices to list available devices", handler.ErrInvalidParam, device)
				}

				_, setErr := audioRunCmd(ctx, "wpctl", "set-default", matchID)
				if setErr != nil {
					return AudioResult{}, fmt.Errorf("failed to set default device: %w", setErr)
				}

				return AudioResult{Result: fmt.Sprintf("Default audio output set to %s (ID: %s)", matchName, matchID)}, nil
			},
		),

		// ── audio_devices ─────────────────────────────────
		handler.TypedHandler[AudioDevicesInput, AudioResult](
			"audio_devices",
			"List all audio devices (sinks and sources) with type, name, ID, and state. Uses pactl for structured output.",
			func(ctx context.Context, _ AudioDevicesInput) (AudioResult, error) {
				if err := audioCheckTool("pactl"); err != nil {
					return AudioResult{}, err
				}

				var sb strings.Builder
				sb.WriteString("=== Audio Devices ===\n")

				// List sinks
				sinks, err := audioRunCmd(ctx, "pactl", "list", "sinks", "short")
				if err != nil {
					sb.WriteString("\n[Sinks] Error: ")
					sb.WriteString(err.Error())
					sb.WriteString("\n")
				} else {
					sb.WriteString("\n--- Sinks (Outputs) ---\n")
					for _, line := range strings.Split(strings.TrimSpace(sinks), "\n") {
						if line == "" {
							continue
						}
						fields := strings.Fields(line)
						if len(fields) >= 5 {
							sb.WriteString(fmt.Sprintf("  ID: %-4s  Name: %-50s  Driver: %-20s  State: %s\n",
								fields[0], fields[1], fields[2], fields[len(fields)-1]))
						} else {
							sb.WriteString("  " + line + "\n")
						}
					}
				}

				// List sources
				sources, err := audioRunCmd(ctx, "pactl", "list", "sources", "short")
				if err != nil {
					sb.WriteString("\n[Sources] Error: ")
					sb.WriteString(err.Error())
					sb.WriteString("\n")
				} else {
					sb.WriteString("\n--- Sources (Inputs) ---\n")
					for _, line := range strings.Split(strings.TrimSpace(sources), "\n") {
						if line == "" {
							continue
						}
						fields := strings.Fields(line)
						if len(fields) >= 5 {
							sb.WriteString(fmt.Sprintf("  ID: %-4s  Name: %-50s  Driver: %-20s  State: %s\n",
								fields[0], fields[1], fields[2], fields[len(fields)-1]))
						} else {
							sb.WriteString("  " + line + "\n")
						}
					}
				}

				// Add default sink/source info
				if err := audioCheckTool("wpctl"); err == nil {
					sb.WriteString("\n--- Defaults ---\n")
					sinkVol, _ := audioRunCmd(ctx, "wpctl", "get-volume", "@DEFAULT_AUDIO_SINK@")
					if sinkVol != "" {
						sb.WriteString("  Default Sink:   " + strings.TrimSpace(sinkVol) + "\n")
					}
					sourceVol, _ := audioRunCmd(ctx, "wpctl", "get-volume", "@DEFAULT_AUDIO_SOURCE@")
					if sourceVol != "" {
						sb.WriteString("  Default Source: " + strings.TrimSpace(sourceVol) + "\n")
					}
				}

				return AudioResult{Result: sb.String()}, nil
			},
		),
	}
}
