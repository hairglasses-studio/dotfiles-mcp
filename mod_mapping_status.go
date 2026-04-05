// mod_mapping_status.go — High-level status and quick-map tools for the mapping system.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hairglasses-studio/mapping"
	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ===========================================================================
// I/O types
// ===========================================================================

// ── Status ──

type MappingStatusInput struct{}

type ConnectedDevice struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // "gamepad", "midi"
	Brand      string `json:"brand,omitempty"`
	EventPath  string `json:"event_path,omitempty"`
	HasProfile bool   `json:"has_profile"`
}

type MappingStatusOutput struct {
	Gamepads       []ConnectedDevice         `json:"gamepads"`
	MIDIDevices    []ConnectedDevice         `json:"midi_devices"`
	Profiles       []mapping.MappingProfileSummary `json:"profiles"`
	ProfileCount   int                       `json:"profile_count"`
	TemplateCount  int                       `json:"template_count"`
	MakimaDir      string                    `json:"makima_dir"`
	MidiDir        string                    `json:"midi_dir"`
}

// ── Quick Map ──

type QuickMapInput struct {
	Profile    string `json:"profile" jsonschema:"required,description=Profile name to add the mapping to"`
	Input      string `json:"input" jsonschema:"required,description=Input source (e.g. BTN_SOUTH, ABS_Z, midi:cc:1)"`
	OutputType string `json:"output_type" jsonschema:"required,description=Output type,enum=key,enum=command,enum=osc,enum=movement"`
	Keys       string `json:"keys,omitempty" jsonschema:"description=Key names for key output (comma-separated, e.g. KEY_ENTER or KEY_LEFTCTRL,KEY_Z)"`
	Command    string `json:"command,omitempty" jsonschema:"description=Shell command for command output"`
	OSCAddress string `json:"osc_address,omitempty" jsonschema:"description=OSC address for osc output (e.g. /resolume/layer1/video/opacity)"`
	OSCPort    int    `json:"osc_port,omitempty" jsonschema:"description=OSC port (default 7000)"`
	Movement   string `json:"movement,omitempty" jsonschema:"description=Movement target (CURSOR_UP, SCROLL_DOWN, etc.)"`
	Modifiers  string `json:"modifiers,omitempty" jsonschema:"description=Modifier keys (comma-separated, e.g. BTN_TL,BTN_TR)"`
	AppClass   string `json:"app_class,omitempty" jsonschema:"description=Window class for per-app mapping"`
}

type QuickMapOutput struct {
	Added       string `json:"added"`
	Profile     string `json:"profile"`
	Input       string `json:"input"`
	OutputType  string `json:"output_type"`
	Description string `json:"description"`
	TOMLSnippet string `json:"toml_snippet"`
}

// ===========================================================================
// Module
// ===========================================================================

// MappingStatusModule provides high-level status and quick-map tools.
type MappingStatusModule struct{}

func (m *MappingStatusModule) Name() string        { return "mapping_status" }
func (m *MappingStatusModule) Description() string {
	return "Controller mapping status overview and quick mapping creation"
}

func (m *MappingStatusModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── mapping_status ──
		handler.TypedHandler[MappingStatusInput, MappingStatusOutput](
			"mapping_status",
			"Show complete controller mapping status: connected devices, active profiles, template availability, and directory paths. Call this first to understand the current setup.",
			func(_ context.Context, _ MappingStatusInput) (MappingStatusOutput, error) {
				var result MappingStatusOutput

				// Detect gamepads.
				for _, c := range parseInputDevices() {
					profilePath := filepath.Join(makimaDir(), c.Name+".toml")
					_, hasProfile := os.Stat(profilePath)
					result.Gamepads = append(result.Gamepads, ConnectedDevice{
						Name:       c.Name,
						Type:       "gamepad",
						Brand:      c.Brand,
						EventPath:  c.EventPath,
						HasProfile: hasProfile == nil,
					})
				}

				// Detect MIDI devices.
				for _, d := range detectMidiDevices() {
					result.MIDIDevices = append(result.MIDIDevices, ConnectedDevice{
						Name:       d.Name,
						Type:       "midi",
						EventPath:  d.DevicePath,
						HasProfile: d.HasMapping,
					})
				}

				// List profiles.
				profiles, _ := listMappingProfiles()
				result.Profiles = profiles
				result.ProfileCount = len(profiles)

				// Count templates.
				result.TemplateCount = len(controllerTemplates) + len(midiTemplates)
				result.MakimaDir = makimaDir()
				result.MidiDir = midiDir()

				return result, nil
			},
		),

		// ── mapping_quick_map ──
		handler.TypedHandler[QuickMapInput, QuickMapOutput](
			"mapping_quick_map",
			"Add a single mapping rule to an existing profile without writing full TOML. Specify the input source, output type, and target. Creates the profile if it doesn't exist.",
			func(_ context.Context, input QuickMapInput) (QuickMapOutput, error) {
				if input.Profile == "" || input.Input == "" || input.OutputType == "" {
					return QuickMapOutput{}, fmt.Errorf("[%s] profile, input, and output_type are required", handler.ErrInvalidParam)
				}

				// Build the mapping line based on output type.
				var mappingSection, mappingLine, description string

				switch input.OutputType {
				case "key":
					if input.Keys == "" {
						return QuickMapOutput{}, fmt.Errorf("[%s] keys is required for key output type", handler.ErrInvalidParam)
					}
					keys := strings.Split(input.Keys, ",")
					quotedKeys := make([]string, len(keys))
					for i, k := range keys {
						quotedKeys[i] = fmt.Sprintf("%q", strings.TrimSpace(k))
					}
					inputKey := input.Input
					if input.Modifiers != "" {
						inputKey = input.Modifiers + "-" + input.Input
					}
					mappingSection = "remap"
					mappingLine = fmt.Sprintf("%s = [%s]", inputKey, strings.Join(quotedKeys, ", "))
					description = fmt.Sprintf("%s → %s", input.Input, input.Keys)

				case "command":
					if input.Command == "" {
						return QuickMapOutput{}, fmt.Errorf("[%s] command is required for command output type", handler.ErrInvalidParam)
					}
					inputKey := input.Input
					if input.Modifiers != "" {
						inputKey = input.Modifiers + "-" + input.Input
					}
					mappingSection = "commands"
					mappingLine = fmt.Sprintf("%s = [%q]", inputKey, input.Command)
					description = fmt.Sprintf("%s → %s", input.Input, input.Command)

				case "movement":
					if input.Movement == "" {
						return QuickMapOutput{}, fmt.Errorf("[%s] movement is required for movement output type", handler.ErrInvalidParam)
					}
					inputKey := input.Input
					if input.Modifiers != "" {
						inputKey = input.Modifiers + "-" + input.Input
					}
					mappingSection = "movements"
					mappingLine = fmt.Sprintf("%s = %q", inputKey, input.Movement)
					description = fmt.Sprintf("%s → %s", input.Input, input.Movement)

				case "osc":
					if input.OSCAddress == "" {
						return QuickMapOutput{}, fmt.Errorf("[%s] osc_address is required for osc output type", handler.ErrInvalidParam)
					}
					port := input.OSCPort
					if port == 0 {
						port = 7000
					}
					mappingSection = "commands"
					// OSC via command: use a helper script or direct socat/oscsend
					cmd := fmt.Sprintf("oscsend localhost %d %s f {value}", port, input.OSCAddress)
					inputKey := input.Input
					if input.Modifiers != "" {
						inputKey = input.Modifiers + "-" + input.Input
					}
					mappingLine = fmt.Sprintf("%s = [%q]", inputKey, cmd)
					description = fmt.Sprintf("%s → OSC %s:%d", input.Input, input.OSCAddress, port)

				default:
					return QuickMapOutput{}, fmt.Errorf("[%s] unknown output_type: %s", handler.ErrInvalidParam, input.OutputType)
				}

				// Determine profile name (may include app class).
				profileName := input.Profile
				if input.AppClass != "" {
					profileName = input.Profile + "::" + input.AppClass
				}

				// Read existing profile or create new one.
				path := resolveMappingPath(profileName)
				content := ""
				if data, err := os.ReadFile(path); err == nil {
					content = string(data)
				}

				// Add the mapping to the appropriate section.
				content = appendToSection(content, mappingSection, mappingLine)

				// Validate the result.
				var parsed map[string]any
				if _, err := toml.Decode(content, &parsed); err != nil {
					return QuickMapOutput{}, fmt.Errorf("generated invalid TOML: %w", err)
				}

				// Write the updated profile.
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return QuickMapOutput{}, fmt.Errorf("create directory: %w", err)
				}
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					return QuickMapOutput{}, fmt.Errorf("write profile: %w", err)
				}

				return QuickMapOutput{
					Added:       path,
					Profile:     profileName,
					Input:       input.Input,
					OutputType:  input.OutputType,
					Description: description,
					TOMLSnippet: fmt.Sprintf("[%s]\n%s", mappingSection, mappingLine),
				}, nil
			},
		),
	}
}

// appendToSection adds a mapping line to the specified TOML section,
// creating the section if it doesn't exist.
func appendToSection(content, section, line string) string {
	sectionHeader := "[" + section + "]"

	if strings.Contains(content, sectionHeader) {
		// Find the section and append the line after it.
		lines := strings.Split(content, "\n")
		var result []string
		inSection := false
		inserted := false
		for _, l := range lines {
			result = append(result, l)
			trimmed := strings.TrimSpace(l)
			if trimmed == sectionHeader {
				inSection = true
				continue
			}
			if inSection && !inserted {
				if strings.HasPrefix(trimmed, "[") || trimmed == "" {
					// Insert before the next section or blank line after content.
					result = append(result[:len(result)-1], line, l)
					inserted = true
					inSection = false
				}
			}
		}
		if !inserted {
			// Section was the last one, append at end.
			result = append(result, line)
		}
		return strings.Join(result, "\n")
	}

	// Section doesn't exist, append it.
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if content != "" {
		content += "\n"
	}
	content += sectionHeader + "\n" + line + "\n"
	return content
}
