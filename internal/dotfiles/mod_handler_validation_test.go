package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// MIDI handler validation tests
// ---------------------------------------------------------------------------

func TestMidiGenerateMapping_MissingDeviceName(t *testing.T) {
	m := &MidiModule{}
	td := findModuleTool(t, m, "midi_generate_mapping")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"device_name": "",
		"template":    "desktop-control",
	}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty device_name")
	}
}

func TestMidiGenerateMapping_MissingTemplate(t *testing.T) {
	m := &MidiModule{}
	td := findModuleTool(t, m, "midi_generate_mapping")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"device_name": "My MIDI",
		"template":    "",
	}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty template")
	}
}

func TestMidiGenerateMapping_InvalidTemplate(t *testing.T) {
	m := &MidiModule{}
	td := findModuleTool(t, m, "midi_generate_mapping")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"device_name": "My MIDI",
		"template":    "nonexistent-template",
	}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for unknown template")
	}
}

func TestMidiGetMapping_MissingName(t *testing.T) {
	m := &MidiModule{}
	td := findModuleTool(t, m, "midi_get_mapping")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": ""}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty name")
	}
}

func TestMidiSetMapping_MissingName(t *testing.T) {
	m := &MidiModule{}
	td := findModuleTool(t, m, "midi_set_mapping")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":    "",
		"content": "[test]\nkey = 1",
	}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty name")
	}
}

// ---------------------------------------------------------------------------
// MIDI generate with temp dir -- actually generates a file
// ---------------------------------------------------------------------------

func TestMidiGenerateMapping_ValidTemplate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &MidiModule{}
	td := findModuleTool(t, m, "midi_generate_mapping")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"device_name": "TestMIDI",
		"template":    "desktop-control",
	}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	// Verify file was created
	path := filepath.Join(dir, "midi", "TestMIDI.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected midi mapping file at %s", path)
	}
}

// ---------------------------------------------------------------------------
// Solaar handler validation tests
// ---------------------------------------------------------------------------

func TestSolaarSetSetting_MissingSetting(t *testing.T) {
	m := &SolaarModule{}
	td := findModuleTool(t, m, "input_set_solaar_setting")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"device":  "MX Master 4",
		"setting": "",
		"value":   "10",
	}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty setting")
	}
}

func TestSolaarSetSetting_MissingValue(t *testing.T) {
	m := &SolaarModule{}
	td := findModuleTool(t, m, "input_set_solaar_setting")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"device":  "MX Master 4",
		"setting": "scroll-ratchet-torque",
		"value":   "",
	}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty value")
	}
}

// ---------------------------------------------------------------------------
// Input simulate handler validation tests
// ---------------------------------------------------------------------------

func TestInputTypeText_EmptyText(t *testing.T) {
	m := &InputSimulateModule{}
	td := findModuleTool(t, m, "input_type_text")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"text": ""}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty text")
	}
}

func TestInputKeyPress_EmptyKeys(t *testing.T) {
	m := &InputSimulateModule{}
	td := findModuleTool(t, m, "input_key_press")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"keys": ""}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty keys")
	}
}

func TestInputMouseScroll_EmptyDirection(t *testing.T) {
	m := &InputSimulateModule{}
	td := findModuleTool(t, m, "input_mouse_scroll")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"direction": ""}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty direction")
	}
}

// ---------------------------------------------------------------------------
// Mapping daemon handler validation tests
// ---------------------------------------------------------------------------

func TestMappingDaemonControl_UnknownAction(t *testing.T) {
	m := &MappingDaemonModule{}
	td := findModuleTool(t, m, "mapping_daemon_control")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"action": "invalid_action"}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for unknown action")
	}
}

func TestMappingDaemonControl_SetVariableMissingName(t *testing.T) {
	m := &MappingDaemonModule{}
	td := findModuleTool(t, m, "mapping_daemon_control")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"action":   "set_variable",
		"variable": "",
	}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for missing variable name")
	}
}

// ---------------------------------------------------------------------------
// OSS handler validation tests
// ---------------------------------------------------------------------------

func TestOSSScore_MissingRepoPath_Handler(t *testing.T) {
	m := &OSSModule{}
	tools := m.Tools()
	var scoreTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_oss_score" {
			scoreTool = td
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"repo_path": ""}
	result, err := scoreTool.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty repo_path")
	}
}

func TestOSSCheck_MissingRepoPath_Handler(t *testing.T) {
	m := &OSSModule{}
	tools := m.Tools()
	var checkTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_oss_check" {
			checkTool = td
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo_path": "",
		"category":  "community",
	}
	result, err := checkTool.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty repo_path")
	}
}

// ---------------------------------------------------------------------------
// Input logiops config handler validation
// ---------------------------------------------------------------------------

func TestInputGetLogiopsConfig_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &InputModule{}
	td := findModuleTool(t, m, "input_get_logiops_config")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error when logiops config file doesn't exist")
	}
}

func TestInputGetLogiopsConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	os.MkdirAll(filepath.Join(dir, "logiops"), 0755)
	os.WriteFile(filepath.Join(dir, "logiops", "logid.cfg"), []byte("devices = []"), 0644)

	m := &InputModule{}
	td := findModuleTool(t, m, "input_get_logiops_config")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}
}

// ---------------------------------------------------------------------------
// Input makima profile list
// ---------------------------------------------------------------------------

func TestInputListMakimaProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	// Create makima dir with some profiles
	mDir := filepath.Join(dir, "makima")
	os.MkdirAll(mDir, 0755)
	os.WriteFile(filepath.Join(mDir, "Xbox Controller.toml"), []byte("[remap]\nBTN_SOUTH = [\"KEY_ENTER\"]\n"), 0644)
	os.WriteFile(filepath.Join(mDir, "PS5 Controller.toml"), []byte("[remap]\nBTN_SOUTH = [\"KEY_SPACE\"]\n"), 0644)

	m := &InputModule{}
	td := findModuleTool(t, m, "input_list_makima_profiles")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result for listing profiles")
	}
}

func TestInputGetMakimaProfile_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "makima"), 0755)

	m := &InputModule{}
	td := findModuleTool(t, m, "input_get_makima_profile")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "nonexistent"}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestInputDeleteMakimaProfile_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "makima"), 0755)

	m := &InputModule{}
	td := findModuleTool(t, m, "input_delete_makima_profile")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "nonexistent"}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for nonexistent profile")
	}
}
