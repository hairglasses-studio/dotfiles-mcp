package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestDesktopSemanticModuleRegistration(t *testing.T) {
	m := &DesktopSemanticModule{}
	tools := m.Tools()
	if len(tools) != 18 {
		t.Fatalf("expected 18 semantic tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"desktop_snapshot",
		"desktop_list_windows",
		"desktop_target_windows",
		"desktop_find",
		"desktop_find_all",
		"desktop_focus_window",
		"desktop_focus",
		"desktop_read_value",
		"desktop_set_text",
		"desktop_set_value",
		"desktop_form_fields",
		"desktop_fill_form",
		"desktop_click",
		"desktop_act",
		"desktop_wait_for_element",
		"desktop_type",
		"desktop_key",
		"desktop_capabilities",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestDesktopSessionModuleRegistration(t *testing.T) {
	m := &DesktopSessionModule{}
	tools := m.Tools()
	if len(tools) != 28 {
		t.Fatalf("expected 28 session tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"session_list",
		"session_start",
		"session_connect",
		"session_status",
		"session_stop",
		"session_screenshot",
		"session_list_windows",
		"session_focus_window",
		"session_launch_app",
		"session_clipboard_get",
		"session_clipboard_set",
		"session_wayland_info",
		"session_read_log",
		"session_read_app_log",
		"session_accessibility_tree",
		"session_find_ui_element",
		"session_find_ui_elements",
		"session_focus_element",
		"session_read_value",
		"session_set_text",
		"session_set_value",
		"session_form_fields",
		"session_fill_form",
		"session_wait_for_element",
		"session_click_element",
		"session_invoke_action",
		"session_type_text",
		"session_dbus_call",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}
