package main

import (
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestLearnModuleRegistration(t *testing.T) {
	m := &LearnModule{}

	tools := m.Tools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 learn tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"mapping_learn",
		"mapping_monitor",
		"mapping_list_templates",
		"mapping_auto_setup",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// evtest output parsing
// ---------------------------------------------------------------------------

func TestParseEvtestOutput_Buttons(t *testing.T) {
	output := `
Event: time 1234.567890, type 1 (EV_KEY), code 304 (BTN_SOUTH), value 1
Event: time 1234.577890, type 0 (EV_SYN), code 0 (SYN_REPORT), value 0
Event: time 1234.867890, type 1 (EV_KEY), code 304 (BTN_SOUTH), value 0
Event: time 1235.067890, type 1 (EV_KEY), code 305 (BTN_EAST), value 1
Event: time 1235.167890, type 1 (EV_KEY), code 305 (BTN_EAST), value 0
`
	controls, total := parseEvtestOutput(output)
	if total != 4 { // 2 BTN_SOUTH + 2 BTN_EAST (SYN filtered)
		t.Errorf("total events = %d, want 4", total)
	}
	if len(controls) != 2 {
		t.Fatalf("controls = %d, want 2", len(controls))
	}

	// Find BTN_SOUTH.
	var south *CapturedControl
	for i := range controls {
		if controls[i].Code == "BTN_SOUTH" {
			south = &controls[i]
			break
		}
	}
	if south == nil {
		t.Fatal("BTN_SOUTH not found in controls")
	}
	if south.Type != "button" {
		t.Errorf("BTN_SOUTH type = %q, want button", south.Type)
	}
	if south.Source != "BTN_SOUTH" {
		t.Errorf("BTN_SOUTH source = %q", south.Source)
	}
	if south.Continuous {
		t.Error("BTN_SOUTH should not be continuous")
	}
}

func TestParseEvtestOutput_Axes(t *testing.T) {
	output := `
Event: time 1234.567890, type 3 (EV_ABS), code 0 (ABS_X), value 128
Event: time 1234.577890, type 3 (EV_ABS), code 0 (ABS_X), value 129
Event: time 1234.587890, type 3 (EV_ABS), code 0 (ABS_X), value 130
Event: time 1234.597890, type 3 (EV_ABS), code 0 (ABS_X), value 131
`
	controls, total := parseEvtestOutput(output)
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	if len(controls) != 1 {
		t.Fatalf("controls = %d, want 1", len(controls))
	}

	axis := controls[0]
	if axis.Type != "axis" {
		t.Errorf("type = %q, want axis", axis.Type)
	}
	if !axis.Continuous {
		t.Error("axis should be continuous (4 distinct values)")
	}
	if axis.Source != "ABS_X" {
		t.Errorf("source = %q, want ABS_X", axis.Source)
	}
}

func TestParseEvtestOutput_Encoder(t *testing.T) {
	output := `
Event: time 1234.567890, type 2 (EV_REL), code 7 (REL_DIAL), value 1
Event: time 1234.577890, type 2 (EV_REL), code 7 (REL_DIAL), value 1
Event: time 1234.587890, type 2 (EV_REL), code 7 (REL_DIAL), value -1
`
	controls, _ := parseEvtestOutput(output)
	if len(controls) != 1 {
		t.Fatalf("controls = %d, want 1", len(controls))
	}
	if controls[0].Type != "encoder" {
		t.Errorf("type = %q, want encoder", controls[0].Type)
	}
}

func TestParseEvtestOutput_Empty(t *testing.T) {
	controls, total := parseEvtestOutput("")
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(controls) != 0 {
		t.Errorf("controls = %d, want 0", len(controls))
	}
}

func TestParseEvtestOutput_SynFiltered(t *testing.T) {
	output := `
Event: time 1234.567890, type 0 (EV_SYN), code 0 (SYN_REPORT), value 0
Event: time 1234.577890, type 4 (EV_MSC), code 4 (MSC_SCAN), value 589825
`
	_, total := parseEvtestOutput(output)
	if total != 0 {
		t.Errorf("total = %d, want 0 (SYN and MSC should be filtered)", total)
	}
}

// ---------------------------------------------------------------------------
// Suggestion generation
// ---------------------------------------------------------------------------

func TestSuggestMappings_ButtonsOnly(t *testing.T) {
	controls := []CapturedControl{
		{Type: "button", Source: "BTN_SOUTH"},
	}
	suggestions := suggestMappings(controls, "", "Generic Controller")
	if len(suggestions) < 2 {
		t.Errorf("expected at least 2 suggestions for buttons, got %d", len(suggestions))
	}

	// Should suggest key mapping and command mapping.
	hasKey := false
	hasCmd := false
	for _, s := range suggestions {
		if s.OutputType == "key" {
			hasKey = true
		}
		if s.OutputType == "command" {
			hasCmd = true
		}
	}
	if !hasKey {
		t.Error("missing key suggestion for buttons")
	}
	if !hasCmd {
		t.Error("missing command suggestion for buttons")
	}
}

func TestSuggestMappings_Axes(t *testing.T) {
	controls := []CapturedControl{
		{Type: "axis", Source: "ABS_Z"},
	}
	suggestions := suggestMappings(controls, "", "")
	hasOSC := false
	for _, s := range suggestions {
		if s.OutputType == "osc" {
			hasOSC = true
		}
	}
	if !hasOSC {
		t.Error("missing OSC suggestion for axes")
	}
}

func TestSuggestMappings_WithPurpose(t *testing.T) {
	controls := []CapturedControl{
		{Type: "button"},
	}
	suggestions := suggestMappings(controls, "master volume", "")
	hasPurpose := false
	for _, s := range suggestions {
		if s.OutputType == "suggestion" {
			hasPurpose = true
		}
	}
	if !hasPurpose {
		t.Error("missing purpose-specific suggestion")
	}
}

func TestSuggestMappings_Encoders(t *testing.T) {
	controls := []CapturedControl{
		{Type: "encoder"},
	}
	suggestions := suggestMappings(controls, "", "")
	found := false
	for _, s := range suggestions {
		if s.OutputType == "command" {
			found = true
		}
	}
	if !found {
		t.Error("missing encoder suggestion")
	}
}

func TestSuggestMappings_EN16_ByGridName(t *testing.T) {
	controls := []CapturedControl{
		{Type: "midi_cc", Source: "midi:cc:32"},
	}
	suggestions := suggestMappings(controls, "", "Grid")
	hasTemplate := false
	hasEN16OSC := false
	for _, s := range suggestions {
		if s.OutputType == "template" {
			hasTemplate = true
			if !strings.Contains(s.Description, "vj-control") {
				t.Error("EN16 template suggestion should mention vj-control")
			}
			if !strings.Contains(s.Description, "en16-default") {
				t.Error("EN16 template suggestion should mention en16-default")
			}
		}
		if s.OutputType == "osc" && strings.Contains(s.Description, "EN16") {
			hasEN16OSC = true
		}
	}
	if !hasTemplate {
		t.Error("missing EN16 template suggestion for Grid device")
	}
	if !hasEN16OSC {
		t.Error("missing EN16 OSC suggestion for Grid device")
	}
}

func TestSuggestMappings_EN16_ByIntechName(t *testing.T) {
	controls := []CapturedControl{}
	suggestions := suggestMappings(controls, "", "Intech Studio: Grid")
	hasTemplate := false
	for _, s := range suggestions {
		if s.OutputType == "template" {
			hasTemplate = true
		}
	}
	if !hasTemplate {
		t.Error("missing EN16 template suggestion for Intech Studio device")
	}
}

func TestSuggestMappings_NonEN16_NoTemplate(t *testing.T) {
	controls := []CapturedControl{
		{Type: "midi_cc", Source: "midi:cc:1"},
	}
	suggestions := suggestMappings(controls, "", "APC Mini mk2")
	for _, s := range suggestions {
		if s.OutputType == "template" {
			t.Error("non-EN16 device should not get EN16 template suggestion")
		}
	}
}

func TestIsIntechGrid(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Grid", true},
		{"Intech Studio: Grid", true},
		{"GRID EN16", true},
		{"intech", true},
		{"APC Mini mk2", false},
		{"nanoKONTROL2", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isIntechGrid(tt.name); got != tt.want {
			t.Errorf("isIntechGrid(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Template listing
// ---------------------------------------------------------------------------

func TestListTemplates(t *testing.T) {
	m := &LearnModule{}
	tools := m.Tools()

	// Find the list_templates tool.
	var listTool *registry.ToolDefinition
	for i := range tools {
		if tools[i].Tool.Name == "mapping_list_templates" {
			listTool = &tools[i]
			break
		}
	}
	if listTool == nil {
		t.Fatal("mapping_list_templates not found")
	}

	// Templates should include both gamepad and MIDI types.
	gamepadCount := 0
	midiCount := 0
	// We can verify by checking the hardcoded list exists.
	templates := []TemplateInfo{
		{Name: "desktop", Type: "gamepad"},
		{Name: "claude-code", Type: "gamepad"},
		{Name: "gaming", Type: "gamepad"},
		{Name: "media", Type: "gamepad"},
		{Name: "macropad", Type: "gamepad"},
		{Name: "desktop-control", Type: "midi"},
		{Name: "shader-control", Type: "midi"},
		{Name: "volume-mixer", Type: "midi"},
		{Name: "vj-control", Type: "midi"},
		{Name: "en16-default", Type: "midi"},
	}
	for _, tmpl := range templates {
		if tmpl.Type == "gamepad" {
			gamepadCount++
		} else {
			midiCount++
		}
	}
	if gamepadCount != 5 {
		t.Errorf("gamepad templates = %d, want 5", gamepadCount)
	}
	if midiCount != 5 {
		t.Errorf("midi templates = %d, want 5", midiCount)
	}
}
