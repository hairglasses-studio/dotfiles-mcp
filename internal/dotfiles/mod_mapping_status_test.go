package dotfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestMappingStatusModuleRegistration(t *testing.T) {
	m := &MappingStatusModule{}
	tools := m.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"mapping_status",
		"mapping_quick_map",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// appendToSection tests
// ---------------------------------------------------------------------------

func TestAppendToSection_NewSection(t *testing.T) {
	content := `[remap]
BTN_SOUTH = ["KEY_ENTER"]
`
	result := appendToSection(content, "commands", `BTN_TL = ["hyprctl dispatch movefocus l"]`)
	if !strings.Contains(result, "[commands]") {
		t.Error("missing [commands] section")
	}
	if !strings.Contains(result, "BTN_TL") {
		t.Error("missing mapping line")
	}
}

func TestAppendToSection_ExistingSection(t *testing.T) {
	content := `[remap]
BTN_SOUTH = ["KEY_ENTER"]

[commands]
BTN_TL = ["hyprctl dispatch movefocus l"]
`
	result := appendToSection(content, "commands", `BTN_TR = ["hyprctl dispatch movefocus r"]`)
	if !strings.Contains(result, "BTN_TR") {
		t.Error("missing new mapping line")
	}
	// Should still have old mapping.
	if !strings.Contains(result, "BTN_TL") {
		t.Error("lost existing mapping")
	}
}

func TestAppendToSection_EmptyContent(t *testing.T) {
	result := appendToSection("", "remap", `BTN_SOUTH = ["KEY_ENTER"]`)
	if !strings.Contains(result, "[remap]") {
		t.Error("missing section header")
	}
	if !strings.Contains(result, "BTN_SOUTH") {
		t.Error("missing mapping")
	}
}

func TestAppendToSection_SectionAtEnd(t *testing.T) {
	content := `[commands]
BTN_SOUTH = ["echo hello"]`
	result := appendToSection(content, "commands", `BTN_EAST = ["echo bye"]`)
	if !strings.Contains(result, "BTN_EAST") {
		t.Error("missing new line when section is at EOF")
	}
}

// ---------------------------------------------------------------------------
// Quick map input validation
// ---------------------------------------------------------------------------

func TestQuickMap_Validation(t *testing.T) {
	m := &MappingStatusModule{}
	tools := m.Tools()

	var quickMap *registry.ToolDefinition
	for i := range tools {
		if tools[i].Tool.Name == "mapping_quick_map" {
			quickMap = &tools[i]
			break
		}
	}
	if quickMap == nil {
		t.Fatal("mapping_quick_map not found")
	}

	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{
			name: "path traversal in profile",
			args: map[string]any{
				"profile":     "../../etc/evil",
				"input":       "BTN_SOUTH",
				"output_type": "key",
				"keys":        "KEY_ENTER",
			},
			wantErr: "must not contain '..'",
		},
		{
			name: "invalid output_type",
			args: map[string]any{
				"profile":     "Test Controller",
				"input":       "BTN_SOUTH",
				"output_type": "banana",
				"keys":        "KEY_ENTER",
			},
			wantErr: "invalid output_type",
		},
		{
			name: "invalid input name",
			args: map[string]any{
				"profile":     "Test Controller",
				"input":       "NOPE_THING",
				"output_type": "key",
				"keys":        "KEY_ENTER",
			},
			wantErr: "invalid input",
		},
		{
			name: "valid gamepad input accepted",
			args: map[string]any{
				"profile":     "Test Controller",
				"input":       "BTN_SOUTH",
				"output_type": "key",
				"keys":        "KEY_ENTER",
			},
			wantErr: "", // should succeed
		},
	}

	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "makima"), 0755)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := registry.CallToolRequest{}
			req.Params.Arguments = tt.args

			result, err := quickMap.Handler(nil, req)
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if tt.wantErr != "" {
				if !registry.IsResultError(result) {
					t.Fatalf("expected error result containing %q, got success", tt.wantErr)
				}
				text := ""
				if len(result.Content) > 0 {
					text, _ = registry.ExtractTextContent(result.Content[0])
				}
				if !strings.Contains(text, tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, text)
				}
			} else if registry.IsResultError(result) {
				t.Fatalf("unexpected error result: %+v", result)
			}
		})
	}
}

func TestValidInputPattern(t *testing.T) {
	valid := []string{
		"BTN_SOUTH", "BTN_TL", "ABS_X", "ABS_Z",
		"KEY_ENTER", "KEY_LEFTCTRL", "REL_X",
		"midi:cc:1", "midi:note:60", "midi:pb",
	}
	for _, in := range valid {
		if !validInputPattern.MatchString(in) {
			t.Errorf("expected %q to be valid", in)
		}
	}
	invalid := []string{
		"NOPE_THING", "banana", "", "SOUTH", "../etc",
	}
	for _, in := range invalid {
		if validInputPattern.MatchString(in) {
			t.Errorf("expected %q to be invalid", in)
		}
	}
}

// ---------------------------------------------------------------------------
// Quick map integration (filesystem-based)
// ---------------------------------------------------------------------------

func TestQuickMap_CreateNewProfile(t *testing.T) {
	dir := t.TempDir()
	origDir := os.Getenv("DOTFILES_DIR")
	t.Setenv("DOTFILES_DIR", dir)
	defer func() {
		if origDir != "" {
			os.Setenv("DOTFILES_DIR", origDir)
		}
	}()

	// Create makima directory.
	os.MkdirAll(filepath.Join(dir, "makima"), 0755)

	m := &MappingStatusModule{}
	tools := m.Tools()

	// Find quick_map tool.
	var quickMap *registry.ToolDefinition
	for i := range tools {
		if tools[i].Tool.Name == "mapping_quick_map" {
			quickMap = &tools[i]
			break
		}
	}
	if quickMap == nil {
		t.Fatal("mapping_quick_map not found")
	}

	// Test creating a key mapping.
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"profile":     "Test Controller",
		"input":       "BTN_SOUTH",
		"output_type": "key",
		"keys":        "KEY_ENTER",
	}

	result, err := quickMap.Handler(nil, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}

	// Verify file was created.
	path := filepath.Join(dir, "makima", "Test Controller.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[remap]") {
		t.Error("missing [remap] section")
	}
	if !strings.Contains(content, "BTN_SOUTH") {
		t.Error("missing BTN_SOUTH mapping")
	}
	if !strings.Contains(content, "KEY_ENTER") {
		t.Error("missing KEY_ENTER value")
	}
}
