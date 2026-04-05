package main

import (
	"math"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Module registration
// ---------------------------------------------------------------------------

func TestMappingModuleRegistration(t *testing.T) {
	m := &MappingEngineModule{}

	tools := m.Tools()
	if len(tools) != 8 {
		t.Fatalf("expected 8 mapping tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"mapping_list_profiles",
		"mapping_get_profile",
		"mapping_set_profile",
		"mapping_delete_profile",
		"mapping_validate",
		"mapping_migrate_legacy",
		"mapping_resolve_test",
		"mapping_generate",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Value transform tests
// ---------------------------------------------------------------------------

func TestValueTransform_Linear(t *testing.T) {
	vt := ValueTransform{
		InputRange:  [2]float64{0, 127},
		OutputRange: [2]float64{0, 1.0},
		Curve:       CurveLinear,
	}

	tests := []struct {
		input float64
		want  float64
	}{
		{0, 0},
		{63.5, 0.5},
		{127, 1.0},
	}
	for _, tt := range tests {
		got := vt.Transform(tt.input)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("Transform(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestValueTransform_Exponential(t *testing.T) {
	vt := ValueTransform{
		InputRange:  [2]float64{0, 127},
		OutputRange: [2]float64{0, 1.0},
		Curve:       CurveExponential,
	}

	// Exponential: midpoint should be below 0.5 (curve rises slowly then fast).
	mid := vt.Transform(63.5)
	if mid >= 0.5 {
		t.Errorf("exponential midpoint = %v, expected < 0.5", mid)
	}

	// Endpoints should be exact.
	if got := vt.Transform(0); math.Abs(got) > 0.01 {
		t.Errorf("Transform(0) = %v, want 0", got)
	}
	if got := vt.Transform(127); math.Abs(got-1.0) > 0.01 {
		t.Errorf("Transform(127) = %v, want 1.0", got)
	}
}

func TestValueTransform_Logarithmic(t *testing.T) {
	vt := ValueTransform{
		InputRange:  [2]float64{0, 127},
		OutputRange: [2]float64{0, 1.0},
		Curve:       CurveLogarithmic,
	}

	// Logarithmic: midpoint should be above 0.5 (curve rises fast then slow).
	mid := vt.Transform(63.5)
	if mid <= 0.5 {
		t.Errorf("logarithmic midpoint = %v, expected > 0.5", mid)
	}
}

func TestValueTransform_SCurve(t *testing.T) {
	vt := ValueTransform{
		InputRange:  [2]float64{0, 127},
		OutputRange: [2]float64{0, 1.0},
		Curve:       CurveSCurve,
	}

	// S-curve: midpoint should be approximately 0.5.
	mid := vt.Transform(63.5)
	if math.Abs(mid-0.5) > 0.1 {
		t.Errorf("scurve midpoint = %v, expected ~0.5", mid)
	}
}

func TestValueTransform_Clamping(t *testing.T) {
	vt := ValueTransform{
		InputRange:  [2]float64{0, 127},
		OutputRange: [2]float64{0, 100},
		Curve:       CurveLinear,
	}

	// Below range should clamp to min.
	if got := vt.Transform(-10); got != 0 {
		t.Errorf("Transform(-10) = %v, want 0", got)
	}

	// Above range should clamp to max.
	if got := vt.Transform(200); got != 100 {
		t.Errorf("Transform(200) = %v, want 100", got)
	}
}

func TestValueTransform_EqualRange(t *testing.T) {
	vt := ValueTransform{
		InputRange:  [2]float64{64, 64},
		OutputRange: [2]float64{50, 100},
	}
	// Equal input range should return output min.
	if got := vt.Transform(64); got != 50 {
		t.Errorf("Transform with equal range = %v, want 50", got)
	}
}

// ---------------------------------------------------------------------------
// Condition evaluation
// ---------------------------------------------------------------------------

func TestCondition_Equals(t *testing.T) {
	c := Condition{Variable: "mode", Equals: "recording"}
	vars := map[string]any{"mode": "recording"}
	if !c.Evaluate(vars) {
		t.Error("expected condition to match")
	}
	vars["mode"] = "idle"
	if c.Evaluate(vars) {
		t.Error("expected condition to not match")
	}
}

func TestCondition_NotEqual(t *testing.T) {
	c := Condition{Variable: "mode", NotEqual: "recording"}
	vars := map[string]any{"mode": "idle"}
	if !c.Evaluate(vars) {
		t.Error("expected not_equal condition to match")
	}
}

func TestCondition_Numeric(t *testing.T) {
	c := Condition{Variable: "level", GreaterThan: 50}
	vars := map[string]any{"level": 75.0}
	if !c.Evaluate(vars) {
		t.Error("expected numeric > condition to match")
	}
	vars["level"] = 25.0
	if c.Evaluate(vars) {
		t.Error("expected numeric > condition to fail for low value")
	}
}

func TestCondition_MissingVariable(t *testing.T) {
	c := Condition{Variable: "missing", Equals: "anything"}
	vars := map[string]any{}
	if c.Evaluate(vars) {
		t.Error("expected condition to fail for missing variable")
	}
}

// ---------------------------------------------------------------------------
// Rule resolution
// ---------------------------------------------------------------------------

func TestRuleIndex_BasicResolve(t *testing.T) {
	p := &MappingProfile{
		Mappings: []MappingRule{
			{Input: "BTN_SOUTH", Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_ENTER"}}},
			{Input: "BTN_EAST", Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_ESC"}}},
		},
	}
	idx := BuildRuleIndex(p)
	state := NewEngineState()

	rule := idx.Resolve("BTN_SOUTH", state)
	if rule == nil {
		t.Fatal("expected rule for BTN_SOUTH")
	}
	if rule.Output.Keys[0] != "KEY_ENTER" {
		t.Errorf("got keys=%v, want [KEY_ENTER]", rule.Output.Keys)
	}

	rule = idx.Resolve("BTN_WEST", state)
	if rule != nil {
		t.Error("expected nil for unmapped BTN_WEST")
	}
}

func TestRuleIndex_AppOverride(t *testing.T) {
	p := &MappingProfile{
		Mappings: []MappingRule{
			{Input: "BTN_SOUTH", Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_ENTER"}}, Description: "default"},
		},
		AppOverrides: []AppOverride{
			{
				WindowClass: "firefox",
				Mappings: []MappingRule{
					{Input: "BTN_SOUTH", Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_SPACE"}}, Description: "firefox"},
				},
			},
		},
	}
	idx := BuildRuleIndex(p)

	// Default context.
	state := NewEngineState()
	rule := idx.Resolve("BTN_SOUTH", state)
	if rule == nil || rule.Description != "default" {
		t.Errorf("expected default rule, got %v", rule)
	}

	// Firefox context.
	state.ActiveApp = "firefox"
	rule = idx.Resolve("BTN_SOUTH", state)
	if rule == nil || rule.Description != "firefox" {
		t.Errorf("expected firefox override, got %v", rule)
	}
}

func TestRuleIndex_ModifierMatch(t *testing.T) {
	p := &MappingProfile{
		Mappings: []MappingRule{
			{Input: "BTN_SOUTH", Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_ENTER"}}, Description: "plain"},
			{Input: "BTN_SOUTH", Modifiers: []string{"BTN_TL"}, Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_Z"}}, Description: "with modifier", Priority: 1},
		},
	}
	idx := BuildRuleIndex(p)

	// No modifier held → plain rule.
	state := NewEngineState()
	rule := idx.Resolve("BTN_SOUTH", state)
	if rule == nil || rule.Description != "plain" {
		t.Errorf("expected plain rule, got %v", rule)
	}

	// Modifier held → modified rule (higher priority).
	state.ActiveModifiers["BTN_TL"] = true
	rule = idx.Resolve("BTN_SOUTH", state)
	if rule == nil || rule.Description != "with modifier" {
		t.Errorf("expected modified rule, got %v", rule)
	}
}

func TestRuleIndex_ConditionFilter(t *testing.T) {
	p := &MappingProfile{
		Mappings: []MappingRule{
			{
				Input:     "midi:cc:1",
				Condition: &Condition{Variable: "recording", Equals: true},
				Output:    OutputAction{Type: OutputOSC, Address: "/record/level"},
				Description: "recording mode",
			},
			{
				Input:  "midi:cc:1",
				Output: OutputAction{Type: OutputCommand, Exec: []string{"wpctl", "set-volume"}},
				Description: "default volume",
			},
		},
	}
	idx := BuildRuleIndex(p)

	// No recording variable → default.
	state := NewEngineState()
	rule := idx.Resolve("midi:cc:1", state)
	if rule == nil || rule.Description != "default volume" {
		t.Errorf("expected default volume, got %v", rule)
	}

	// Recording = true → recording mode.
	state.Variables["recording"] = true
	rule = idx.Resolve("midi:cc:1", state)
	if rule == nil || rule.Description != "recording mode" {
		t.Errorf("expected recording mode, got %v", rule)
	}
}

// ---------------------------------------------------------------------------
// Legacy parsing
// ---------------------------------------------------------------------------

func TestParseLegacyInput_Simple(t *testing.T) {
	rule := parseLegacyInput("BTN_SOUTH")
	if rule.Input != "BTN_SOUTH" {
		t.Errorf("got input=%q, want BTN_SOUTH", rule.Input)
	}
	if len(rule.Modifiers) != 0 {
		t.Errorf("got modifiers=%v, want empty", rule.Modifiers)
	}
}

func TestParseLegacyInput_WithModifiers(t *testing.T) {
	rule := parseLegacyInput("KEY_LEFTCTRL-KEY_LEFTSHIFT-BTN_SOUTH")
	if rule.Input != "BTN_SOUTH" {
		t.Errorf("got input=%q, want BTN_SOUTH", rule.Input)
	}
	if len(rule.Modifiers) != 2 {
		t.Fatalf("got %d modifiers, want 2", len(rule.Modifiers))
	}
	if rule.Modifiers[0] != "KEY_LEFTCTRL" || rule.Modifiers[1] != "KEY_LEFTSHIFT" {
		t.Errorf("got modifiers=%v, want [KEY_LEFTCTRL KEY_LEFTSHIFT]", rule.Modifiers)
	}
}

func TestParseMappingProfile_Legacy(t *testing.T) {
	content := `
[remap]
BTN_SOUTH = ["KEY_ENTER"]
BTN_EAST = ["KEY_ESC"]

[commands]
BTN_TL = ["hyprctl dispatch movefocus l"]

[settings]
LSTICK = "cursor"
LSTICK_SENSITIVITY = "6"
`
	p, err := ParseMappingProfile(content, "TestController.toml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if !p.IsLegacyFormat() {
		t.Error("expected legacy format")
	}
	if p.IsUnifiedFormat() {
		t.Error("should not be unified format")
	}
	if len(p.Remap) != 2 {
		t.Errorf("got %d remap entries, want 2", len(p.Remap))
	}
	if len(p.Commands) != 1 {
		t.Errorf("got %d command entries, want 1", len(p.Commands))
	}
}

func TestParseMappingProfile_Unified(t *testing.T) {
	content := `
[profile]
schema_version = 1
type = "mapping"
device = "Test Controller"

[[mapping]]
input = "BTN_SOUTH"
description = "A button"

[mapping.output]
type = "key"
keys = ["KEY_ENTER"]

[[mapping]]
input = "midi:cc:1"
description = "Fader 1"

[mapping.output]
type = "osc"
address = "/volume"
port = 7000

[mapping.value]
input_range = [0, 127]
output_range = [0.0, 1.0]
curve = "logarithmic"
`
	p, err := ParseMappingProfile(content, "test-unified.toml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if !p.IsUnifiedFormat() {
		t.Error("expected unified format")
	}
	if p.Profile.Device != "Test Controller" {
		t.Errorf("device = %q, want Test Controller", p.Profile.Device)
	}
	if len(p.Mappings) != 2 {
		t.Errorf("got %d mappings, want 2", len(p.Mappings))
	}
	if p.Mappings[1].Value == nil {
		t.Fatal("mapping[1].value is nil")
	}
	if p.Mappings[1].Value.Curve != CurveLogarithmic {
		t.Errorf("curve = %q, want logarithmic", p.Mappings[1].Value.Curve)
	}
}

func TestConvertLegacyToUnified(t *testing.T) {
	content := `
[remap]
BTN_SOUTH = ["KEY_ENTER"]

[commands]
BTN_TL = ["hyprctl dispatch movefocus l"]

[settings]
LSTICK = "cursor"
LSTICK_SENSITIVITY = "6"
`
	p, err := ParseMappingProfile(content, "Xbox Controller.toml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	unified := ConvertLegacyToUnified(p)
	if !unified.IsUnifiedFormat() {
		t.Error("expected unified format after conversion")
	}
	if unified.Profile.Device != "Xbox Controller" {
		t.Errorf("device = %q, want 'Xbox Controller'", unified.Profile.Device)
	}
	if len(unified.Mappings) != 2 {
		t.Errorf("got %d mappings, want 2", len(unified.Mappings))
	}
	if unified.Settings == nil || unified.Settings.LStick == nil {
		t.Fatal("settings.lstick is nil")
	}
	if unified.Settings.LStick.Function != "cursor" {
		t.Errorf("lstick.function = %q, want cursor", unified.Settings.LStick.Function)
	}
	if unified.Settings.LStick.Sensitivity != 6 {
		t.Errorf("lstick.sensitivity = %d, want 6", unified.Settings.LStick.Sensitivity)
	}
}

func TestConvertLegacyToUnified_PerApp(t *testing.T) {
	content := `
[remap]
BTN_SOUTH = ["KEY_ENTER"]
`
	p, err := ParseMappingProfile(content, "Xbox Controller::com.mitchellh.ghostty.toml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	unified := ConvertLegacyToUnified(p)
	if unified.Profile.AppClass != "com.mitchellh.ghostty" {
		t.Errorf("app_class = %q, want com.mitchellh.ghostty", unified.Profile.AppClass)
	}
	if len(unified.AppOverrides) != 1 {
		t.Fatalf("got %d app overrides, want 1", len(unified.AppOverrides))
	}
	if unified.AppOverrides[0].WindowClass != "com.mitchellh.ghostty" {
		t.Errorf("window_class = %q", unified.AppOverrides[0].WindowClass)
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func TestValidateProfile_Valid(t *testing.T) {
	p := &MappingProfile{
		Profile: &ProfileMeta{SchemaVersion: 1},
		Mappings: []MappingRule{
			{Input: "BTN_SOUTH", Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_ENTER"}}},
		},
	}
	issues := ValidateProfile(p)
	for _, issue := range issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error: %s: %s", issue.Field, issue.Message)
		}
	}
}

func TestValidateProfile_MissingInput(t *testing.T) {
	p := &MappingProfile{
		Profile: &ProfileMeta{SchemaVersion: 1},
		Mappings: []MappingRule{
			{Output: OutputAction{Type: OutputKey}},
		},
	}
	issues := ValidateProfile(p)
	found := false
	for _, issue := range issues {
		if issue.Severity == "error" && issue.Field == "mapping[0].input" {
			found = true
		}
	}
	if !found {
		t.Error("expected error for missing input")
	}
}

func TestValidateProfile_InvalidCurve(t *testing.T) {
	p := &MappingProfile{
		Profile: &ProfileMeta{SchemaVersion: 1},
		Mappings: []MappingRule{
			{
				Input:  "midi:cc:1",
				Output: OutputAction{Type: OutputOSC},
				Value:  &ValueTransform{Curve: "banana"},
			},
		},
	}
	issues := ValidateProfile(p)
	found := false
	for _, issue := range issues {
		if issue.Severity == "error" && issue.Field == "mapping[0].value.curve" {
			found = true
		}
	}
	if !found {
		t.Error("expected error for invalid curve type")
	}
}

func TestValidateProfile_Legacy(t *testing.T) {
	p := &MappingProfile{
		Remap: map[string][]string{"BTN_SOUTH": {"KEY_ENTER"}},
	}
	issues := ValidateProfile(p)
	found := false
	for _, issue := range issues {
		if issue.Severity == "info" && issue.Field == "format" {
			found = true
		}
	}
	if !found {
		t.Error("expected info about legacy format")
	}
}

// ---------------------------------------------------------------------------
// Profile helpers
// ---------------------------------------------------------------------------

func TestMappingProfile_MappingCount(t *testing.T) {
	// Unified format.
	p := &MappingProfile{
		Profile: &ProfileMeta{SchemaVersion: 1},
		Mappings: []MappingRule{
			{Input: "A"}, {Input: "B"},
		},
		AppOverrides: []AppOverride{
			{WindowClass: "app", Mappings: []MappingRule{{Input: "C"}}},
		},
	}
	if got := p.MappingCount(); got != 3 {
		t.Errorf("unified MappingCount = %d, want 3", got)
	}

	// Legacy format.
	p2 := &MappingProfile{
		Remap:    map[string][]string{"A": {"B"}, "C": {"D"}},
		Commands: map[string][]string{"E": {"cmd"}},
	}
	if got := p2.MappingCount(); got != 3 {
		t.Errorf("legacy MappingCount = %d, want 3", got)
	}
}
