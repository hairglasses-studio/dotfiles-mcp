package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestKittyModuleRegistration(t *testing.T) {
	m := &KittyModule{}
	tools := m.Tools()
	if len(tools) != 23 {
		t.Fatalf("expected 23 kitty tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"kitty_status",
		"kitty_list_tabs",
		"kitty_list_windows",
		"kitty_focus_window",
		"kitty_focus_tab",
		"kitty_get_text",
		"kitty_launch",
		"kitty_load_config",
		"kitty_set_font_size",
		"kitty_set_opacity",
		"kitty_set_theme",
		"kitty_set_layout",
		"kitty_last_used_layout",
		"kitty_set_title",
		"kitty_set_tab_title",
		"kitty_send_text",
		"kitty_send_key",
		"kitty_resize_window",
		"kitty_resize_os_window",
		"kitty_show_image",
		"kitty_close_window",
		"kitty_close_tab",
		"kitty_run_remote",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestKittyLaunchResult(t *testing.T) {
	out := kittyLaunchResult("1234\n")
	if out.WindowID != 1234 {
		t.Fatalf("window_id = %d, want 1234", out.WindowID)
	}
	if out.Raw != "1234" {
		t.Fatalf("raw = %q", out.Raw)
	}

	nonNumeric := kittyLaunchResult("background task started")
	if nonNumeric.WindowID != 0 {
		t.Fatalf("window_id = %d, want 0", nonNumeric.WindowID)
	}
	if nonNumeric.Raw != "background task started" {
		t.Fatalf("raw = %q", nonNumeric.Raw)
	}
}
