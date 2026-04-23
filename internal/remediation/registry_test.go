package remediation

import (
	"testing"
)

func TestLookupKnownCode(t *testing.T) {
	r, ok := Lookup(CodeGoMissingDep)
	if !ok {
		t.Fatal("expected CodeGoMissingDep to be registered")
	}
	if r.Tool != "ops_auto_fix" {
		t.Errorf("CodeGoMissingDep should map to ops_auto_fix, got %q", r.Tool)
	}
	if r.Risk != RiskSafe {
		t.Errorf("CodeGoMissingDep should be RiskSafe, got %q", r.Risk)
	}
	if r.Why == "" {
		t.Error("every remediation should have a Why")
	}
}

func TestLookupUnknownCode(t *testing.T) {
	_, ok := Lookup(ErrorCode("this_does_not_exist"))
	if ok {
		t.Error("lookup of unknown code should return ok=false")
	}
}

func TestLookupReturnsCopy(t *testing.T) {
	// Mutating the returned Args must not affect the registry.
	r1, ok := Lookup(CodeGoMissingDep)
	if !ok {
		t.Fatal("expected CodeGoMissingDep to be registered")
	}
	if r1.Args == nil {
		r1.Args = map[string]any{}
	}
	r1.Args["mutated"] = true

	r2, _ := Lookup(CodeGoMissingDep)
	if _, tainted := r2.Args["mutated"]; tainted {
		t.Error("mutating Lookup result leaked into the registry")
	}
}

func TestListReturnsAllCodes(t *testing.T) {
	catalog := List()
	if len(catalog) == 0 {
		t.Fatal("catalog should not be empty")
	}

	// Spot-check a few expected codes.
	want := map[ErrorCode]bool{
		CodeHyprConfigParse:      true,
		CodeGoMissingDep:         true,
		CodeGoTimeout:            true,
		CodeTickerStale:          true,
		CodeMcpkitVersionDrift:   true,
		CodeHyprReloadInducedDrm: true,
	}
	seen := map[ErrorCode]bool{}
	for _, e := range catalog {
		seen[e.Code] = true
	}
	for code := range want {
		if !seen[code] {
			t.Errorf("catalog is missing expected code %q", code)
		}
	}
}

func TestListIsSorted(t *testing.T) {
	catalog := List()
	for i := 1; i < len(catalog); i++ {
		if catalog[i-1].Code >= catalog[i].Code {
			t.Errorf("catalog not sorted: %q comes before %q", catalog[i-1].Code, catalog[i].Code)
		}
	}
}

func TestAllEntriesHaveRequiredFields(t *testing.T) {
	// Every registered remediation should have Tool and Why set — they
	// are what callers see in the UI and use to dispatch the fix.
	for _, e := range List() {
		if e.Remediation.Tool == "" {
			t.Errorf("%q has empty Tool", e.Code)
		}
		if e.Remediation.Why == "" {
			t.Errorf("%q has empty Why", e.Code)
		}
		switch e.Remediation.Risk {
		case RiskSafe, RiskReload, RiskDestructive:
			// ok
		default:
			t.Errorf("%q has invalid Risk %q", e.Code, e.Remediation.Risk)
		}
	}
}
