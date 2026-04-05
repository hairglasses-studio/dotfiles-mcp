package main

import (
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// InputModule registration
// ---------------------------------------------------------------------------

func TestInputModuleRegistration(t *testing.T) {
	m := &InputModule{}

	if m.Name() != "input" {
		t.Fatalf("expected name input, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) == 0 {
		t.Fatal("expected at least 1 tool")
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"input_status",
		"input_get_logiops_config",
		"input_set_logiops_config",
		"input_list_makima_profiles",
		"input_get_makima_profile",
		"input_set_makima_profile",
		"input_delete_makima_profile",
		"input_restart_services",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// MidiModule registration
// ---------------------------------------------------------------------------

func TestMidiModuleRegistration(t *testing.T) {
	m := &MidiModule{}

	if m.Name() != "midi" {
		t.Fatalf("expected name midi, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 midi tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"midi_list_devices",
		"midi_generate_mapping",
		"midi_get_mapping",
		"midi_set_mapping",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// SolaarModule registration
// ---------------------------------------------------------------------------

func TestSolaarModuleRegistration(t *testing.T) {
	m := &SolaarModule{}

	if m.Name() != "solaar" {
		t.Fatalf("expected name solaar, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 solaar tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"input_get_solaar_settings",
		"input_set_solaar_setting",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// InputSimulateModule registration
// ---------------------------------------------------------------------------

func TestInputSimulateModuleRegistration(t *testing.T) {
	m := &InputSimulateModule{}

	if m.Name() != "input_simulate" {
		t.Fatalf("expected name input_simulate, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) != 6 {
		t.Fatalf("expected 6 input simulate tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"input_type_text",
		"input_key_press",
		"input_mouse_move",
		"input_mouse_click",
		"input_mouse_scroll",
		"input_screenshot_click",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// ScreenModule registration
// ---------------------------------------------------------------------------

func TestScreenModuleRegistration(t *testing.T) {
	m := &ScreenModule{}

	if m.Name() != "screen" {
		t.Fatalf("expected name screen, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) != 5 {
		t.Fatalf("expected 5 screen tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"screen_screenshot",
		"screen_record_start",
		"screen_record_stop",
		"screen_ocr",
		"screen_color_pick",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// MappingDaemonModule registration
// ---------------------------------------------------------------------------

func TestMappingDaemonModuleRegistration(t *testing.T) {
	m := &MappingDaemonModule{}

	if m.Name() != "mapping_daemon" {
		t.Fatalf("expected name mapping_daemon, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 mapping daemon tool, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	if !srv.HasTool("mapping_daemon_control") {
		t.Error("missing tool: mapping_daemon_control")
	}
}

// ---------------------------------------------------------------------------
// mapitallSocketPath
// ---------------------------------------------------------------------------

func TestMapitallSocketPath_EnvOverride(t *testing.T) {
	t.Setenv("MAPITALL_SOCKET", "/custom/path/mapitall.sock")
	got := mapitallSocketPath()
	if got != "/custom/path/mapitall.sock" {
		t.Errorf("mapitallSocketPath() = %q, want /custom/path/mapitall.sock", got)
	}
}

func TestMapitallSocketPath_Default(t *testing.T) {
	t.Setenv("MAPITALL_SOCKET", "")
	got := mapitallSocketPath()
	if got == "" {
		t.Error("expected non-empty default socket path")
	}
	// On Linux with XDG_RUNTIME_DIR, should use that. Otherwise /tmp.
	// Either way it should end in mapitall.sock.
	if !strings.HasSuffix(got, "mapitall.sock") {
		t.Errorf("expected path ending in mapitall.sock, got %q", got)
	}
}

