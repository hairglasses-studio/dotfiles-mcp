// mod_learn.go — Controller learn mode, monitoring, and auto-setup tools.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
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

				// Try IPC event streaming from mapitall first, fall back to evtest.
				controls, totalEvents := captureEventsIPC(ctx, deviceName, duration)
				if totalEvents < 0 {
					// IPC unavailable, fall back to evtest.
					captureCtx, cancel := context.WithTimeout(ctx, time.Duration(duration)*time.Second)
					defer cancel()

					cmd := exec.CommandContext(captureCtx, "evtest", eventPath)
					var out bytes.Buffer
					cmd.Stdout = &out
					cmd.Stderr = &out
					_ = cmd.Run()

					controls, totalEvents = parseEvtestOutput(out.String())
				}

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

				// Try IPC event streaming from mapitall first, fall back to evtest.
				events := monitorEventsIPC(ctx, deviceName, duration)
				ipcFailed := events == nil

				if ipcFailed {
					captureCtx, cancel := context.WithTimeout(ctx, time.Duration(duration)*time.Second)
					defer cancel()

					cmd := exec.CommandContext(captureCtx, "evtest", eventPath)
					var out bytes.Buffer
					cmd.Stdout = &out
					cmd.Stderr = &out
					_ = cmd.Run()

					eventRe := regexp.MustCompile(`time (\d+\.\d+), type \d+ \((\w+)\), code \d+ \((\w+)\), value (\S+)`)
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

				// Restart mapitall if executing and any profiles were generated.
				if input.Execute {
					hasNewProfile := false
					for _, r := range results {
						if r.Action == "generated" {
							hasNewProfile = true
							break
						}
					}
					if hasNewProfile {
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

// ---------------------------------------------------------------------------
// IPC event streaming helpers (mapitall subscribe_events)
// ---------------------------------------------------------------------------

// ipcDeviceEvent mirrors the DeviceEvent type from mapitall's IPC event bus.
type ipcDeviceEvent struct {
	DeviceID  string  `json:"device_id"`
	Type      string  `json:"type"`
	Source    string  `json:"source"`
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value,omitempty"`
	Pressed   bool    `json:"pressed,omitempty"`
	Code      uint16  `json:"code,omitempty"`
	Channel   uint8   `json:"channel,omitempty"`
	MIDINote  uint8   `json:"midi_note,omitempty"`
	Velocity  uint8   `json:"velocity,omitempty"`
	MIDIValue uint8   `json:"midi_value,omitempty"`
}

// ipcNotification is a JSON-RPC 2.0 notification from the mapitall event stream.
type ipcNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// connectIPCEventStream opens a connection to the mapitall IPC socket,
// sends a subscribe_events request, and returns the connection and decoder.
// Returns nil, nil if mapitall is not running.
func connectIPCEventStream(deviceID string) (net.Conn, *json.Decoder, error) {
	socketPath := mapitallSocketPath()
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, nil, err
	}

	// Build the subscribe_events request with optional device filter.
	params := map[string]string{}
	if deviceID != "" {
		params["device_id"] = deviceID
	}

	req := jsonRPCRequest{JSONRPC: "2.0", Method: "subscribe_events", Params: params, ID: 1}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		conn.Close()
		return nil, nil, err
	}

	dec := json.NewDecoder(conn)

	// Read the initial ack response.
	var resp jsonRPCResponse
	if err := dec.Decode(&resp); err != nil {
		conn.Close()
		return nil, nil, err
	}
	if resp.Error != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return conn, dec, nil
}

// captureEventsIPC subscribes to mapitall events and collects them for the given duration.
// Returns controls and total event count. Returns nil, -1 if IPC is unavailable.
func captureEventsIPC(ctx context.Context, deviceName string, durationSec int) ([]CapturedControl, int) {
	conn, dec, err := connectIPCEventStream(deviceName)
	if err != nil {
		return nil, -1 // signal caller to fall back to evtest
	}
	defer conn.Close()

	deadline := time.Now().Add(time.Duration(durationSec) * time.Second)
	conn.SetReadDeadline(deadline)

	type eventData struct {
		evType string
		code   string
		values map[string]bool
		count  int
	}
	byCode := make(map[string]*eventData)
	totalEvents := 0

	for {
		var notif ipcNotification
		if err := dec.Decode(&notif); err != nil {
			break // timeout or disconnect
		}
		if notif.Method != "event" {
			continue
		}

		var ev ipcDeviceEvent
		if err := json.Unmarshal(notif.Params, &ev); err != nil {
			continue
		}

		// Skip sync-like events.
		if ev.Type == "" || ev.Source == "" {
			continue
		}

		totalEvents++
		ed, ok := byCode[ev.Source]
		if !ok {
			ed = &eventData{evType: ev.Type, code: ev.Source, values: make(map[string]bool)}
			byCode[ev.Source] = ed
		}
		ed.values[fmt.Sprintf("%.6g", ev.Value)] = true
		if ev.Pressed {
			ed.values["1"] = true
		}
		ed.count++

		select {
		case <-ctx.Done():
			break
		default:
		}
	}

	// Convert to CapturedControl (same classification logic as parseEvtestOutput).
	var controls []CapturedControl
	for _, ed := range byCode {
		var values []string
		for v := range ed.values {
			values = append(values, v)
		}

		cc := CapturedControl{
			Code:       ed.code,
			Source:     ed.code,
			Values:     values,
			EventCount: ed.count,
			Continuous: len(ed.values) > 2,
		}

		switch ed.evType {
		case "button", "key":
			cc.Type = "button"
		case "axis":
			if cc.Continuous {
				cc.Type = "axis"
			} else {
				cc.Type = "button"
			}
		case "encoder":
			cc.Type = "encoder"
		case "midi_note":
			cc.Type = "midi_note"
		case "midi_cc":
			cc.Type = "midi_cc"
		default:
			cc.Type = ed.evType
		}

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

// monitorEventsIPC subscribes to mapitall events and returns them as MonitorEvents.
// Returns nil if IPC is unavailable (caller should fall back to evtest).
func monitorEventsIPC(ctx context.Context, deviceName string, durationSec int) []MonitorEvent {
	conn, dec, err := connectIPCEventStream(deviceName)
	if err != nil {
		return nil // signal caller to fall back to evtest
	}
	defer conn.Close()

	deadline := time.Now().Add(time.Duration(durationSec) * time.Second)
	conn.SetReadDeadline(deadline)

	var events []MonitorEvent

	for {
		var notif ipcNotification
		if err := dec.Decode(&notif); err != nil {
			break // timeout or disconnect
		}
		if notif.Method != "event" {
			continue
		}

		var ev ipcDeviceEvent
		if err := json.Unmarshal(notif.Params, &ev); err != nil {
			continue
		}

		if ev.Type == "" || ev.Source == "" {
			continue
		}

		events = append(events, MonitorEvent{
			Timestamp: ev.Timestamp,
			Type:      ev.Type,
			Code:      ev.Source,
			Value:     fmt.Sprintf("%.6g", ev.Value),
		})

		select {
		case <-ctx.Done():
			break
		default:
		}
	}

	// Return empty non-nil slice to distinguish from "IPC unavailable" (nil).
	if events == nil {
		events = []MonitorEvent{}
	}
	return events
}
