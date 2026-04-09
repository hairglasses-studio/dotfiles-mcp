package mapping

import (
	"sync"
	"testing"
)

// helper: build a RuleIndex from a flat slice of MappingRules (no app overrides).
func indexFromRules(rules []MappingRule) *RuleIndex {
	return BuildRuleIndex(&MappingProfile{Mappings: rules})
}

// helper: build a RuleIndex that also has app overrides.
func indexWithOverrides(rules []MappingRule, overrides []AppOverride) *RuleIndex {
	return BuildRuleIndex(&MappingProfile{
		Mappings:     rules,
		AppOverrides: overrides,
	})
}

// ---------------------------------------------------------------------------
// 1. Basic match
// ---------------------------------------------------------------------------

func TestResolve_BasicMatch(t *testing.T) {
	idx := indexFromRules([]MappingRule{
		{Input: "BTN_SOUTH", Output: OutputAction{Type: OutputCommand, Exec: []string{"echo", "test"}}},
	})
	state := NewEngineState()

	r := idx.Resolve("BTN_SOUTH", state, "")
	if r == nil {
		t.Fatal("expected a matching rule, got nil")
	}
	if r.Input != "BTN_SOUTH" {
		t.Errorf("matched rule Input = %q, want BTN_SOUTH", r.Input)
	}
	if r.Output.Type != OutputCommand {
		t.Errorf("matched rule output type = %q, want %q", r.Output.Type, OutputCommand)
	}
}

// ---------------------------------------------------------------------------
// 2. No match
// ---------------------------------------------------------------------------

func TestResolve_NoMatch(t *testing.T) {
	idx := indexFromRules([]MappingRule{
		{Input: "BTN_SOUTH", Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_A"}}},
	})
	state := NewEngineState()

	r := idx.Resolve("BTN_NORTH", state, "")
	if r != nil {
		t.Errorf("expected nil for unregistered source, got rule with Input=%q", r.Input)
	}
}

// ---------------------------------------------------------------------------
// 3. App override wins over default
// ---------------------------------------------------------------------------

func TestResolve_AppOverride(t *testing.T) {
	defaultRule := MappingRule{
		Input:  "BTN_SOUTH",
		Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_A"}},
	}
	firefoxRule := MappingRule{
		Input:  "BTN_SOUTH",
		Output: OutputAction{Type: OutputCommand, Exec: []string{"firefox-action"}},
	}

	idx := indexWithOverrides(
		[]MappingRule{defaultRule},
		[]AppOverride{{
			WindowClass: "firefox",
			Mappings:    []MappingRule{firefoxRule},
		}},
	)

	state := NewEngineState()
	state.SetActiveApp("firefox")

	r := idx.Resolve("BTN_SOUTH", state, "")
	if r == nil {
		t.Fatal("expected a matching rule, got nil")
	}
	if r.Output.Type != OutputCommand {
		t.Errorf("expected app-override rule (command), got type=%q", r.Output.Type)
	}

	// Without the active app, the default should win.
	state.SetActiveApp("")
	r = idx.Resolve("BTN_SOUTH", state, "")
	if r == nil {
		t.Fatal("expected default rule, got nil")
	}
	if r.Output.Type != OutputKey {
		t.Errorf("expected default rule (key), got type=%q", r.Output.Type)
	}
}

// ---------------------------------------------------------------------------
// 4. Modifier filtering
// ---------------------------------------------------------------------------

func TestResolve_ModifierFiltering(t *testing.T) {
	idx := indexFromRules([]MappingRule{
		{
			Input:     "BTN_SOUTH",
			Modifiers: []string{"BTN_TL"},
			Output:    OutputAction{Type: OutputCommand, Exec: []string{"with-modifier"}},
		},
	})
	state := NewEngineState()

	// Without modifier -> no match.
	r := idx.Resolve("BTN_SOUTH", state, "")
	if r != nil {
		t.Errorf("expected nil without modifier, got rule with Input=%q", r.Input)
	}

	// With modifier -> match.
	state.SetModifier("BTN_TL", true)
	r = idx.Resolve("BTN_SOUTH", state, "")
	if r == nil {
		t.Fatal("expected a match with BTN_TL active, got nil")
	}
	if r.Output.Exec[0] != "with-modifier" {
		t.Errorf("unexpected exec = %v", r.Output.Exec)
	}

	// Clear modifier -> no match again.
	state.SetModifier("BTN_TL", false)
	r = idx.Resolve("BTN_SOUTH", state, "")
	if r != nil {
		t.Errorf("expected nil after clearing modifier, got rule")
	}
}

// ---------------------------------------------------------------------------
// 5. Layer filtering
// ---------------------------------------------------------------------------

func TestResolve_LayerFiltering(t *testing.T) {
	layer2Rule := MappingRule{
		Input:  "BTN_SOUTH",
		Layer:  2,
		Output: OutputAction{Type: OutputCommand, Exec: []string{"layer2-action"}},
	}
	anyLayerRule := MappingRule{
		Input:  "BTN_WEST",
		Layer:  0, // matches any layer
		Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_B"}},
	}

	idx := indexFromRules([]MappingRule{layer2Rule, anyLayerRule})
	state := NewEngineState()

	// Layer 2 active for dev1 -> layer-2 rule matches.
	state.SetActiveLayer("dev1", 2)
	r := idx.Resolve("BTN_SOUTH", state, "dev1")
	if r == nil {
		t.Fatal("expected layer-2 rule to match when active layer is 2, got nil")
	}
	if r.Output.Exec[0] != "layer2-action" {
		t.Errorf("unexpected exec = %v", r.Output.Exec)
	}

	// Layer 1 active for dev1 -> layer-2 rule does NOT match.
	state.SetActiveLayer("dev1", 1)
	r = idx.Resolve("BTN_SOUTH", state, "dev1")
	if r != nil {
		t.Errorf("expected nil when active layer is 1 for a layer-2 rule, got rule")
	}

	// Layer=0 rule matches regardless of active layer.
	r = idx.Resolve("BTN_WEST", state, "dev1")
	if r == nil {
		t.Fatal("expected layer-0 rule to match on any active layer, got nil")
	}
	if r.Output.Type != OutputKey {
		t.Errorf("expected key output, got %q", r.Output.Type)
	}

	// Layer=0 rule matches even with a different layer active.
	state.SetActiveLayer("dev1", 99)
	r = idx.Resolve("BTN_WEST", state, "dev1")
	if r == nil {
		t.Fatal("expected layer-0 rule to match on layer 99, got nil")
	}
}

// ---------------------------------------------------------------------------
// 6. Priority ordering
// ---------------------------------------------------------------------------

func TestResolve_PriorityOrdering(t *testing.T) {
	lowPriority := MappingRule{
		Input:    "BTN_SOUTH",
		Priority: 1,
		Output:   OutputAction{Type: OutputKey, Keys: []string{"KEY_LOW"}},
	}
	highPriority := MappingRule{
		Input:    "BTN_SOUTH",
		Priority: 10,
		Output:   OutputAction{Type: OutputCommand, Exec: []string{"high-priority"}},
	}

	// Insert in low-then-high order to ensure it doesn't just return the first.
	idx := indexFromRules([]MappingRule{lowPriority, highPriority})
	state := NewEngineState()

	r := idx.Resolve("BTN_SOUTH", state, "")
	if r == nil {
		t.Fatal("expected a match, got nil")
	}
	if r.Priority != 10 {
		t.Errorf("expected priority=10 rule to win, got priority=%d", r.Priority)
	}
	if r.Output.Type != OutputCommand {
		t.Errorf("expected command output from high-priority rule, got %q", r.Output.Type)
	}
}

// ---------------------------------------------------------------------------
// 7. Condition gating
// ---------------------------------------------------------------------------

func TestResolve_ConditionGating(t *testing.T) {
	idx := indexFromRules([]MappingRule{
		{
			Input:     "BTN_SOUTH",
			Condition: &Condition{Variable: "mode", Equals: "combat"},
			Output:    OutputAction{Type: OutputCommand, Exec: []string{"combat-action"}},
		},
	})
	state := NewEngineState()

	// Without the variable set -> no match.
	r := idx.Resolve("BTN_SOUTH", state, "")
	if r != nil {
		t.Errorf("expected nil without condition variable, got rule")
	}

	// Set wrong value -> no match.
	state.SetVariable("mode", "explore")
	r = idx.Resolve("BTN_SOUTH", state, "")
	if r != nil {
		t.Errorf("expected nil with wrong variable value, got rule")
	}

	// Set correct value -> match.
	state.SetVariable("mode", "combat")
	r = idx.Resolve("BTN_SOUTH", state, "")
	if r == nil {
		t.Fatal("expected match with mode=combat, got nil")
	}
	if r.Output.Exec[0] != "combat-action" {
		t.Errorf("unexpected exec = %v", r.Output.Exec)
	}
}

// ---------------------------------------------------------------------------
// 8. BuildRuleIndex with empty profile
// ---------------------------------------------------------------------------

func TestBuildRuleIndex_Empty(t *testing.T) {
	// Nil mappings.
	idx := BuildRuleIndex(&MappingProfile{})
	state := NewEngineState()

	r := idx.Resolve("BTN_SOUTH", state, "")
	if r != nil {
		t.Errorf("expected nil from empty index, got rule with Input=%q", r.Input)
	}

	r = idx.Resolve("ABS_X", state, "dev1")
	if r != nil {
		t.Errorf("expected nil from empty index for ABS_X, got rule")
	}

	// Explicit empty slice.
	idx2 := BuildRuleIndex(&MappingProfile{Mappings: []MappingRule{}})
	r = idx2.Resolve("BTN_SOUTH", state, "")
	if r != nil {
		t.Errorf("expected nil from empty-slice index, got rule")
	}
}

// ---------------------------------------------------------------------------
// 9. Concurrency safety
// ---------------------------------------------------------------------------

func TestEngineState_Concurrency(t *testing.T) {
	state := NewEngineState()
	idx := indexFromRules([]MappingRule{
		{Input: "BTN_SOUTH", Output: OutputAction{Type: OutputKey, Keys: []string{"KEY_A"}}},
	})

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(4)
		go func(n int) {
			defer wg.Done()
			state.SetModifier("BTN_TL", n%2 == 0)
		}(i)
		go func(n int) {
			defer wg.Done()
			state.SetActiveApp("app" + string(rune('A'+n)))
		}(i)
		go func(n int) {
			defer wg.Done()
			state.SetVariable("counter", n)
		}(i)
		go func(n int) {
			defer wg.Done()
			state.SetActiveLayer("dev1", n%3)
		}(i)
	}

	// Also do concurrent reads via Resolve.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = idx.Resolve("BTN_SOUTH", state, "dev1")
		}()
	}

	wg.Wait()

	// If we get here without a data race (under -race), the test passes.
	// Verify state is still coherent by doing basic reads.
	_ = state.GetActiveApp()
	_ = state.GetActiveLayer("dev1")
	_, _ = state.GetVariable("counter")
}

// ---------------------------------------------------------------------------
// 10. Profile-level layer inheritance
// ---------------------------------------------------------------------------

func TestParseProfile_LayerInheritance(t *testing.T) {
	tomlContent := `
[profile]
schema_version = 1
device = "TestPad"
layer = 2

[[mapping]]
input = "BTN_SOUTH"

[mapping.output]
type = "key"
keys = ["KEY_A"]

[[mapping]]
input = "BTN_NORTH"
layer = 5

[mapping.output]
type = "key"
keys = ["KEY_B"]
`
	p, err := ParseMappingProfile(tomlContent, "test-layer.toml")
	if err != nil {
		t.Fatalf("ParseMappingProfile failed: %v", err)
	}

	if p.Profile == nil || p.Profile.Layer != 2 {
		t.Fatalf("expected profile-level layer=2, got %+v", p.Profile)
	}

	// Rule without explicit layer should inherit the profile-level layer.
	if len(p.Mappings) < 2 {
		t.Fatalf("expected 2 mappings, got %d", len(p.Mappings))
	}

	btnSouth := p.Mappings[0]
	if btnSouth.Input != "BTN_SOUTH" {
		t.Fatalf("expected first mapping to be BTN_SOUTH, got %q", btnSouth.Input)
	}
	if btnSouth.Layer != 2 {
		t.Errorf("BTN_SOUTH layer = %d, want 2 (inherited from profile). "+
			"NOTE: This test is expected to fail until profile.go propagates ProfileMeta.Layer to rules.",
			btnSouth.Layer)
	}

	// Rule with explicit layer should keep its own value.
	btnNorth := p.Mappings[1]
	if btnNorth.Input != "BTN_NORTH" {
		t.Fatalf("expected second mapping to be BTN_NORTH, got %q", btnNorth.Input)
	}
	if btnNorth.Layer != 5 {
		t.Errorf("BTN_NORTH layer = %d, want 5 (explicitly set)", btnNorth.Layer)
	}
}
