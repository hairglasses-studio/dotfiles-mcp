package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestDesktopSemanticModuleRegistration(t *testing.T) {
	m := &DesktopSemanticModule{}
	tools := m.Tools()
	if len(tools) != 8 {
		t.Fatalf("expected 8 semantic tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"desktop_snapshot",
		"desktop_target_windows",
		"desktop_find",
		"desktop_click",
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
	if len(tools) != 11 {
		t.Fatalf("expected 11 session tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"session_start",
		"session_connect",
		"session_stop",
		"session_screenshot",
		"session_list_windows",
		"session_focus_window",
		"session_launch_app",
		"session_clipboard_get",
		"session_clipboard_set",
		"session_wayland_info",
		"session_read_app_log",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}
