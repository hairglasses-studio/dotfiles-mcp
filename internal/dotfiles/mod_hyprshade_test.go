package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestHyprshadeModuleInfo(t *testing.T) {
	m := &HyprshadeModule{}

	if m.Name() != "hyprshade" {
		t.Fatalf("expected name hyprshade, got %s", m.Name())
	}
	if m.Description() == "" {
		t.Fatal("expected non-empty description")
	}

	tools := m.Tools()
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"hyprshade_list",
		"hyprshade_set",
		"hyprshade_toggle",
		"hyprshade_off",
		"hyprshade_status",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}
