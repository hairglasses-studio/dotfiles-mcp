package main

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// HyprlandModule Registration
// ---------------------------------------------------------------------------

func TestHyprlandModuleRegistration(t *testing.T) {
	m := &HyprlandModule{}
	tools := m.Tools()
	if len(tools) != 13 {
		t.Fatalf("expected 13 hyprland tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"hypr_list_windows", "hypr_list_workspaces", "hypr_get_monitors",
		"hypr_screenshot", "hypr_screenshot_monitors", "hypr_screenshot_window",
		"hypr_focus_window", "hypr_switch_workspace", "hypr_reload_config",
		"hypr_click", "hypr_type_text", "hypr_key", "hypr_set_monitor",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}
