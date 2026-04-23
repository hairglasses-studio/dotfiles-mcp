package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestKeybindsModuleInfo(t *testing.T) {
	m := &KeybindsModule{}
	if m.Name() != "keybinds" {
		t.Fatalf("expected name keybinds, got %s", m.Name())
	}
	if m.Description() == "" {
		t.Fatal("expected non-empty description")
	}
	tools := m.Tools()
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"keybinds_list",
		"keybinds_search",
		"keybinds_free_slots",
		"keybinds_conflicts",
		"keybinds_refresh_ticker",
		"keybind_add",
		"keybind_remove",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}
