package main

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
