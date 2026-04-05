// mod_learn.go — Controller learn mode, monitoring, and auto-setup tools.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ===========================================================================
// I/O types
// ===========================================================================

// ── Learn Start ──

type LearnStartInput struct {
	DeviceName string `json:"device_name,omitempty" jsonschema:"description=Controller or MIDI device name. If empty, uses first detected device."`
	EventPath  string `json:"event_path,omitempty" jsonschema:"description=Event device path (e.g. /dev/input/event5). Alternative to device_name."`
	DurationSec int   `json:"duration_sec,omitempty" jsonschema:"description=How long to listen for events (default 5, max 15),minimum=1,maximum=15"`
	Purpose    string `json:"purpose,omitempty" jsonschema:"description=What this control will be mapped to (e.g. 'master volume'). Helps with suggestions."`
}

type CapturedControl struct {
	Type        string   `json:"type"`         // "button", "axis", "encoder", "midi_cc", "midi_note"
	Source      string   `json:"source"`       // Canonical ID: "BTN_SOUTH", "ABS_X", "midi:cc:1"
	Code        string   `json:"code"`         // Raw code name from evtest
	Values      []string `json:"values"`       // Observed values
	MinValue    string   `json:"min_value"`
	MaxValue    string   `json:"max_value"`
	Continuous  bool     `json:"continuous"`    // true if multiple distinct values seen
	EventCount  int      `json:"event_count"`
}

type LearnSuggestion struct {
	OutputType  string `json:"output_type"`  // "key", "command", "osc", "movement"
	Description string `json:"description"`
	Example     string `json:"example"`      // Example TOML mapping
}

type LearnStartOutput struct {
	DeviceName   string            `json:"device_name"`
	EventPath    string            `json:"event_path,omitempty"`
	Duration     int               `json:"duration_sec"`
	TotalEvents  int               `json:"total_events"`
	Controls     []CapturedControl `json:"controls"`
	Suggestions  []LearnSuggestion `json:"suggestions,omitempty"`
	Status       string            `json:"status"` // "captured", "no_events", "timeout"
}

// ── Monitor ──

type MonitorInput struct {
	DeviceName  string `json:"device_name,omitempty" jsonschema:"description=Controller name to monitor"`
	EventPath   string `json:"event_path,omitempty" jsonschema:"description=Event device path"`
	DurationSec int    `json:"duration_sec,omitempty" jsonschema:"description=Duration to monitor (default 3, max 10),minimum=1,maximum=10"`
}

type MonitorEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Code      string `json:"code"`
	Value     string `json:"value"`
}

type MonitorOutput struct {
	DeviceName string         `json:"device_name"`
	EventPath  string         `json:"event_path"`
	Duration   int            `json:"duration_sec"`
	EventCount int            `json:"event_count"`
	Events     []MonitorEvent `json:"events"`
	Status     string         `json:"status"`
}

// ── Auto Setup ──

type AutoSetupInput struct {
	Template string `json:"template,omitempty" jsonschema:"description=Base template for gamepad profiles,enum=desktop,enum=claude-code,enum=gaming,enum=media"`
	Execute  bool   `json:"execute,omitempty" jsonschema:"description=Write profiles and restart makima (default: dry-run)"`
}

type DeviceSetupResult struct {
	Name    string `json:"name"`
	Type    string `json:"type"`    // "gamepad", "midi"
	Brand   string `json:"brand,omitempty"`
	Action  string `json:"action"`  // "generated", "exists", "skipped"
	Profile string `json:"profile,omitempty"`
}

type AutoSetupOutput struct {
	DryRun  bool                `json:"dry_run"`
	Devices []DeviceSetupResult `json:"devices"`
	Restart string              `json:"restart,omitempty"`
}

// ── List Templates ──

type ListTemplatesInput struct{}

type TemplateInfo struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // "gamepad", "midi"
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type ListTemplatesOutput struct {
	Templates []TemplateInfo `json:"templates"`
}

// ===========================================================================
// Module
// ===========================================================================

// LearnModule provides controller learn mode, monitoring, and auto-setup tools.
type LearnModule struct{}

func (m *LearnModule) Name() string        { return "learn" }
func (m *LearnModule) Description() string {
	return "Controller learn mode, live monitoring, templates, and auto-setup workflows"
}

func (m *LearnModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── mapping_learn ──
		handler.TypedHandler[LearnStartInput, LearnStartOutput](
			"mapping_learn",
			"Listen to a controller or MIDI device and capture which controls are moved. Use this to discover available inputs before creating mappings. Returns detected controls with type classification (button, axis, encoder, MIDI CC/note) and mapping suggestions.",
			func(ctx context.Context, input LearnStartInput) (LearnStartOutput, error) {
				duration := input.DurationSec
				if duration <= 0 {
					duration = 5
				}
				if duration > 15 {
					duration = 15
				}

				// Resolve event path.
				eventPath := input.EventPath
				deviceName := input.DeviceName

				if eventPath == "" {
					controllers := parseInputDevices()
					if deviceName != "" {
						for _, c := range controllers {
							if c.Name == deviceName {
								eventPath = c.EventPath
								break
							}
						}
					} else if len(controllers) > 0 {
						eventPath = controllers[0].EventPath
						deviceName = controllers[0].Name
					}
				}

				if eventPath == "" {
					return LearnStartOutput{
						Status: "no_events",
					}, fmt.Errorf("[%s] no controller found. Connect a device and try again", handler.ErrNotFound)
				}

				if deviceName == "" {
					deviceName = filepath.Base(eventPath)
				}

				// Capture events via evtest.
				captureCtx, cancel := context.WithTimeout(ctx, time.Duration(duration)*time.Second)
				defer cancel()

				cmd := exec.CommandContext(captureCtx, "evtest", eventPath)
				var out bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &out
				_ = cmd.Run()

				// Parse captured events.
				controls, totalEvents := parseEvtestOutput(out.String())

				status := "captured"
				if totalEvents == 0 {
					status = "no_events"
				}

				// Generate suggestions based on detected controls.
				suggestions := suggestMappings(controls, input.Purpose)

				return LearnStartOutput{
					DeviceName:  deviceName,
					EventPath:   eventPath,
					Duration:    duration,
					TotalEvents: totalEvents,
					Controls:    controls,
					Suggestions: suggestions,
					Status:      status,
				}, nil
			},
		),

		// ── mapping_monitor ──
		handler.TypedHandler[MonitorInput, MonitorOutput](
			"mapping_monitor",
			"Live-monitor controller events for debugging. Shows raw events with timestamps. Use to verify a controller is working and identify input codes.",
			func(ctx context.Context, input MonitorInput) (MonitorOutput, error) {
				duration := input.DurationSec
				if duration <= 0 {
					duration = 3
				}
				if duration > 10 {
					duration = 10
				}

				eventPath := input.EventPath
				deviceName := input.DeviceName

				if eventPath == "" {
					for _, c := range parseInputDevices() {
						if deviceName == "" || c.Name == deviceName {
							eventPath = c.EventPath
							deviceName = c.Name
							break
						}
					}
				}

				if eventPath == "" {
					return MonitorOutput{Status: "no_device"}, fmt.Errorf("[%s] no controller found", handler.ErrNotFound)
				}

				captureCtx, cancel := context.WithTimeout(ctx, time.Duration(duration)*time.Second)
				defer cancel()

				cmd := exec.CommandContext(captureCtx, "evtest", eventPath)
				var out bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &out
				_ = cmd.Run()

				eventRe := regexp.MustCompile(`time (\d+\.\d+), type \d+ \((\w+)\), code \d+ \((\w+)\), value (\S+)`)
				var events []MonitorEvent
				for _, line := range strings.Split(out.String(), "\n") {
					m := eventRe.FindStringSubmatch(line)
					if m != nil {
						events = append(events, MonitorEvent{
							Timestamp: m[1],
							Type:      m[2],
							Code:      m[3],
							Value:     m[4],
						})
					}
				}

				// Keep last 50 events.
				if len(events) > 50 {
					events = events[len(events)-50:]
				}

				status := "ok"
				if len(events) == 0 {
					status = "no_events"
				}

				return MonitorOutput{
					DeviceName: deviceName,
					EventPath:  eventPath,
					Duration:   duration,
					EventCount: len(events),
					Events:     events,
					Status:     status,
				}, nil
			},
		),

		// ── mapping_list_templates ──
		handler.TypedHandler[ListTemplatesInput, ListTemplatesOutput](
			"mapping_list_templates",
			"List all available mapping templates for gamepad and MIDI controllers.",
			func(_ context.Context, _ ListTemplatesInput) (ListTemplatesOutput, error) {
				templates := []TemplateInfo{
					{Name: "desktop", Type: "gamepad", Description: "Window management, workspace navigation, and desktop rice controls via Hyprland", Tags: []string{"desktop", "hyprland", "tiling"}},
					{Name: "claude-code", Type: "gamepad", Description: "Terminal interaction optimized for Claude Code (Enter, Esc, /, navigation)", Tags: []string{"terminal", "claude-code", "keyboard-emulation"}},
					{Name: "gaming", Type: "gamepad", Description: "Minimal gaming setup — sticks for cursor/scroll, triggers for mouse buttons", Tags: []string{"gaming", "mouse-emulation"}},
					{Name: "media", Type: "gamepad", Description: "Music/media control via playerctl — play/pause, skip, volume, brightness", Tags: []string{"media", "audio", "playerctl"}},
					{Name: "macropad", Type: "gamepad", Description: "Generic USB keypad/macropad with rotary encoders (F13-F24 defaults)", Tags: []string{"macropad", "encoder", "keyboard"}},
					{Name: "desktop-control", Type: "midi", Description: "MIDI faders for volume/brightness, pads for workspace switching", Tags: []string{"desktop", "volume", "workspace"}},
					{Name: "shader-control", Type: "midi", Description: "MIDI controls for Ghostty shader cycling and wallpaper selection", Tags: []string{"shader", "wallpaper", "ghostty"}},
					{Name: "volume-mixer", Type: "midi", Description: "Per-sink audio control via PipeWire — one fader per audio source", Tags: []string{"audio", "pipewire", "mixer"}},
				}
				return ListTemplatesOutput{Templates: templates}, nil
			},
		),

		// ── mapping_auto_setup ──
		handler.TypedHandler[AutoSetupInput, AutoSetupOutput](
			"mapping_auto_setup",
			"Detect all connected controllers and MIDI devices, generate missing profiles from templates, and optionally restart makima. Dry-run by default.",
			func(_ context.Context, input AutoSetupInput) (AutoSetupOutput, error) {
				template := input.Template
				if template == "" {
					template = "desktop"
				}

				var results []DeviceSetupResult

				// Detect gamepads.
				for _, c := range parseInputDevices() {
					profilePath := filepath.Join(makimaDir(), c.Name+".toml")
					if _, err := os.Stat(profilePath); err == nil {
						results = append(results, DeviceSetupResult{
							Name:    c.Name,
							Type:    "gamepad",
							Brand:   c.Brand,
							Action:  "exists",
							Profile: profilePath,
						})
						continue
					}

					if !input.Execute {
						results = append(results, DeviceSetupResult{
							Name:   c.Name,
							Type:   "gamepad",
							Brand:  c.Brand,
							Action: "would_generate",
						})
						continue
					}

					// Generate profile from template.
					tmpl, ok := controllerTemplates[template]
					if !ok {
						results = append(results, DeviceSetupResult{
							Name:   c.Name,
							Type:   "gamepad",
							Brand:  c.Brand,
							Action: "skipped",
						})
						continue
					}

					labels := brandLabels[c.Brand]
					if labels == "" {
						labels = brandLabels["generic"]
					}
					content := fmt.Sprintf(tmpl, c.Name, c.Brand, labels)

					if err := os.MkdirAll(makimaDir(), 0755); err == nil {
						os.WriteFile(profilePath, []byte(content), 0644)
					}

					results = append(results, DeviceSetupResult{
						Name:    c.Name,
						Type:    "gamepad",
						Brand:   c.Brand,
						Action:  "generated",
						Profile: profilePath,
					})
				}

				// Detect MIDI devices.
				for _, d := range detectMidiDevices() {
					mappingPath := filepath.Join(midiDir(), d.Name+".toml")
					if d.HasMapping {
						results = append(results, DeviceSetupResult{
							Name:    d.Name,
							Type:    "midi",
							Action:  "exists",
							Profile: mappingPath,
						})
					} else {
						action := "would_generate"
						if input.Execute {
							action = "generated"
							if tmpl, ok := midiTemplates["desktop-control"]; ok {
								content := fmt.Sprintf(tmpl, d.Name, d.RawPath, d.Name, d.RawPath)
								os.MkdirAll(midiDir(), 0755)
								os.WriteFile(mappingPath, []byte(content), 0644)
							}
						}
						results = append(results, DeviceSetupResult{
							Name:    d.Name,
							Type:    "midi",
							Action:  action,
							Profile: mappingPath,
						})
					}
				}

				out := AutoSetupOutput{
					DryRun:  !input.Execute,
					Devices: results,
				}

				// Restart mapitall if executing and gamepads were generated.
				if input.Execute {
					hasNewGamepad := false
					for _, r := range results {
						if r.Type == "gamepad" && r.Action == "generated" {
							hasNewGamepad = true
							break
						}
					}
					if hasNewGamepad {
						if _, _, err := inputRunCmd("sudo", "systemctl", "restart", "mapitall.service"); err == nil {
							out.Restart = "mapitall restarted"
						} else {
							out.Restart = "mapitall restart failed (may need sudo)"
						}
					}
				}

				return out, nil
			},
		),
	}
}

// ---------------------------------------------------------------------------
// evtest output parsing for learn mode
// ---------------------------------------------------------------------------

var evtestEventRe = regexp.MustCompile(`type \d+ \((\w+)\), code \d+ \((\w+)\), value (\S+)`)

func parseEvtestOutput(output string) ([]CapturedControl, int) {
	// Collect all events by code.
	type eventData struct {
		evType string
		code   string
		values map[string]bool
		count  int
	}

	byCode := make(map[string]*eventData)
	totalEvents := 0

	for _, line := range strings.Split(output, "\n") {
		m := evtestEventRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		evType, code, value := m[1], m[2], m[3]

		// Skip sync events and misc.
		if evType == "EV_SYN" || evType == "EV_MSC" {
			continue
		}

		totalEvents++
		ed, ok := byCode[code]
		if !ok {
			ed = &eventData{evType: evType, code: code, values: make(map[string]bool)}
			byCode[code] = ed
		}
		ed.values[value] = true
		ed.count++
	}

	// Convert to CapturedControl.
	var controls []CapturedControl
	for _, ed := range byCode {
		var values []string
		for v := range ed.values {
			values = append(values, v)
		}

		cc := CapturedControl{
			Code:       ed.code,
			Values:     values,
			EventCount: ed.count,
			Continuous: len(ed.values) > 2,
		}

		// Classify control type and set source identifier.
		switch ed.evType {
		case "EV_KEY":
			cc.Type = "button"
			cc.Source = ed.code
		case "EV_ABS":
			if cc.Continuous {
				cc.Type = "axis"
			} else {
				cc.Type = "button" // D-pad triggers as ABS with discrete values
			}
			cc.Source = ed.code
		case "EV_REL":
			cc.Type = "encoder"
			cc.Source = ed.code
		default:
			cc.Type = ed.evType
			cc.Source = ed.code
		}

		// Compute min/max.
		if len(values) > 0 {
			cc.MinValue = values[0]
			cc.MaxValue = values[0]
			for _, v := range values {
				if v < cc.MinValue {
					cc.MinValue = v
				}
				if v > cc.MaxValue {
					cc.MaxValue = v
				}
			}
		}

		controls = append(controls, cc)
	}

	return controls, totalEvents
}

// suggestMappings generates mapping suggestions based on detected controls.
func suggestMappings(controls []CapturedControl, purpose string) []LearnSuggestion {
	var suggestions []LearnSuggestion

	hasButtons := false
	hasAxes := false
	hasEncoders := false

	for _, c := range controls {
		switch c.Type {
		case "button":
			hasButtons = true
		case "axis":
			hasAxes = true
		case "encoder":
			hasEncoders = true
		}
	}

	if hasButtons {
		suggestions = append(suggestions, LearnSuggestion{
			OutputType:  "key",
			Description: "Map buttons to keyboard shortcuts",
			Example: `[[mapping]]
input = "BTN_SOUTH"
output = { type = "key", keys = ["KEY_ENTER"] }`,
		})
		suggestions = append(suggestions, LearnSuggestion{
			OutputType:  "command",
			Description: "Map buttons to shell commands",
			Example: `[[mapping]]
input = "BTN_SOUTH"
output = { type = "command", exec = ["hyprctl", "dispatch", "killactive"] }`,
		})
	}

	if hasAxes {
		suggestions = append(suggestions, LearnSuggestion{
			OutputType:  "osc",
			Description: "Map axes/faders to OSC parameters (Resolume, TouchDesigner)",
			Example: `[[mapping]]
input = "ABS_Z"
output = { type = "osc", address = "/resolume/layer1/video/opacity", port = 7000 }
[mapping.value]
input_range = [0, 255]
output_range = [0.0, 1.0]
curve = "linear"`,
		})
		suggestions = append(suggestions, LearnSuggestion{
			OutputType:  "command",
			Description: "Map axes/triggers to volume or brightness control",
			Example: `[[mapping]]
input = "ABS_Z"
output = { type = "command", exec = ["wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", "{scaled}%"] }
[mapping.value]
input_range = [0, 255]
output_range = [0, 100]`,
		})
	}

	if hasEncoders {
		suggestions = append(suggestions, LearnSuggestion{
			OutputType:  "command",
			Description: "Map encoders to incremental controls (volume, brightness, scroll)",
			Example: `[[mapping]]
input = "REL_DIAL"
output = { type = "command", exec = ["brightnessctl", "set", "{delta}%"] }
[mapping.value]
mode = "relative"
step = 5`,
		})
	}

	if purpose != "" {
		suggestions = append(suggestions, LearnSuggestion{
			OutputType:  "suggestion",
			Description: fmt.Sprintf("For '%s': use the mapping_set_profile tool to create a custom mapping based on the detected controls above", purpose),
		})
	}

	return suggestions
}
