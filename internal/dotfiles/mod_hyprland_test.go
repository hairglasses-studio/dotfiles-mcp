package dotfiles

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
	if len(tools) != 53 {
		t.Fatalf("expected 53 hyprland tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"hypr_list_windows", "hypr_list_workspaces", "hypr_get_monitors",
		"hypr_screenshot", "hypr_screenshot_monitors", "hypr_screenshot_window",
		"hypr_focus_window", "hypr_switch_workspace", "hypr_reload_config",
		"hypr_click", "hypr_type_text", "hypr_key", "hypr_set_monitor",
		"hypr_move_window", "hypr_resize_window", "hypr_close_window",
		"hypr_toggle_floating", "hypr_minimize_window", "hypr_fullscreen_window",
		"hypr_get_active_window", "hypr_get_active_workspace", "hypr_list_binds",
		"hypr_list_devices", "hypr_list_layers", "hypr_list_layouts",
		"hypr_get_config_errors", "hypr_get_cursor_position", "hypr_get_version",
		"hypr_get_system_info", "hypr_list_workspace_rules", "hypr_get_option",
		"hypr_set_keyword", "hypr_dispatch", "hypr_notify", "hypr_dismiss_notify",
		"hypr_set_cursor", "hypr_set_prop", "hypr_get_prop", "hypr_switch_xkb_layout",
		"hypr_output", "hypr_plugin", "hypr_hyprpaper", "hypr_hyprsunset",
		"hypr_rolling_log", "hypr_capture_events", "hypr_wait_for_event",
		"hypr_monitor_preset_save", "hypr_monitor_preset_restore", "hypr_monitor_preset_list",
		"hypr_layout_save", "hypr_layout_restore", "hypr_layout_list", "desktop_project_open",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}
