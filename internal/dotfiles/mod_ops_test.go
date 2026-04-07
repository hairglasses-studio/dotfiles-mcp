package dotfiles

import (
	"os"
	"testing"
	"time"
)

func TestOpsParseCompileErrors(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   int
	}{
		{"empty", "", 0},
		{"single error", "./main.go:10:5: undefined: foo", 1},
		{"multiple errors", "./main.go:10:5: undefined: foo\n./main.go:20:3: cannot use x", 2},
		{"non-error lines", "# github.com/example/pkg\n./main.go:10:5: undefined: foo\nok done", 1},
		{"no column", "this is not a compile error", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := opsParseCompileErrors(tt.stderr)
			if len(got) != tt.want {
				t.Errorf("parseCompileErrors(%q) = %d errors, want %d", tt.stderr, len(got), tt.want)
			}
		})
	}
}

func TestOpsParseCompileErrorFields(t *testing.T) {
	errors := opsParseCompileErrors("./pkg/handler.go:42:10: undefined: SomeFunc")
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}
	e := errors[0]
	if e.File != "./pkg/handler.go" {
		t.Errorf("File = %q, want %q", e.File, "./pkg/handler.go")
	}
	if e.Line != 42 {
		t.Errorf("Line = %d, want 42", e.Line)
	}
	if e.Column != 10 {
		t.Errorf("Column = %d, want 10", e.Column)
	}
	if e.Message != "undefined: SomeFunc" {
		t.Errorf("Message = %q, want %q", e.Message, "undefined: SomeFunc")
	}
}

func TestOpsParseGoTestJSON(t *testing.T) {
	input := `{"Action":"run","Package":"pkg","Test":"TestA"}
{"Action":"output","Package":"pkg","Test":"TestA","Output":"--- PASS: TestA\n"}
{"Action":"pass","Package":"pkg","Test":"TestA","Elapsed":0.01}
{"Action":"run","Package":"pkg","Test":"TestB"}
{"Action":"output","Package":"pkg","Test":"TestB","Output":"expected 1, got 2\n"}
{"Action":"fail","Package":"pkg","Test":"TestB","Elapsed":0.02}
{"Action":"skip","Package":"pkg","Test":"TestC"}
`
	passed, failures, skipped := opsParseGoTestJSON(input)
	if len(passed) != 1 {
		t.Errorf("passed = %d, want 1", len(passed))
	}
	if len(failures) != 1 {
		t.Errorf("failures = %d, want 1", len(failures))
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if len(failures) > 0 && failures[0].Test != "TestB" {
		t.Errorf("failure test = %q, want TestB", failures[0].Test)
	}
}

func TestOpsParseGoTestJSON_Malformed(t *testing.T) {
	input := `not json
{"Action":"pass","Package":"pkg","Test":"TestA"}
also not json`
	passed, failures, _ := opsParseGoTestJSON(input)
	if len(passed) != 1 {
		t.Errorf("passed = %d, want 1 (should skip malformed lines)", len(passed))
	}
	if len(failures) != 0 {
		t.Errorf("failures = %d, want 0", len(failures))
	}
}

func TestOpsCategorizeError(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"undefined: SomeFunc", "type_error"},
		{"cannot use x (type int) as string", "type_error"},
		{"not enough arguments in call", "type_error"},
		{"cannot convert int to string", "type_error"},
		{"cannot find package \"foo\"", "missing_dep"},
		{"no required module provides package", "missing_dep"},
		{"missing go.sum entry", "missing_dep"},
		{"import cycle not allowed", "import_cycle"},
		{"context deadline exceeded", "timeout"},
		{"test timed out after 30s", "timeout"},
		{"panic: runtime error", "test_assertion"},
		{"expected 5, got 3", "test_assertion"},
		{"fatal error: concurrent map writes", "fatal_error"},
		{"undefined reference to main", "fatal_error"},
		{"syntax error: unexpected newline", "syntax_error"},
		{"some random compile error", "compile_error"},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			got := opsCategorizeError(tt.msg)
			if got != tt.want {
				t.Errorf("categorizeError(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestIsConventionalCommit(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"feat: add new feature", true},
		{"fix: resolve bug", true},
		{"chore: update deps", true},
		{"docs: update README", true},
		{"refactor(core): simplify logic", true},
		{"test: add unit tests", true},
		{"random commit message", false},
		{"", false},
		{"feat:", false},
		{"feat:missing space", false},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			got := conventionalRe.MatchString(tt.msg)
			if got != tt.want {
				t.Errorf("isConventional(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestOpsDetectLanguage(t *testing.T) {
	// Test with current directory (should be Go)
	lang := opsDetectLanguage("../..")
	if lang != "go" {
		t.Errorf("detectLanguage(\"../..\") = %q, want \"go\"", lang)
	}

	// Test with non-existent directory
	lang = opsDetectLanguage("/tmp/nonexistent-dir-12345")
	if lang != "unknown" {
		t.Errorf("detectLanguage(nonexistent) = %q, want \"unknown\"", lang)
	}
}

func TestOpsDefaultBase(t *testing.T) {
	// Should return "main", "master", or "HEAD" — not empty
	base := opsDefaultBase(".")
	if base == "" {
		t.Error("defaultBase(\".\") returned empty string")
	}
	// In this repo, "main" should exist
	if base != "HEAD" && base != "main" && base != "master" {
		t.Errorf("defaultBase(\".\") = %q, want HEAD/main/master", base)
	}
}

func TestOpsParseNodeTestJSON_Jest(t *testing.T) {
	input := `{
		"numPassedTests": 2,
		"numFailedTests": 1,
		"numPendingTests": 0,
		"testResults": [{
			"testFilePath": "src/app.test.ts",
			"testResults": [
				{"fullName": "App renders", "status": "passed", "duration": 50},
				{"fullName": "App handles error", "status": "failed", "duration": 100},
				{"fullName": "App loading", "status": "pending", "duration": 0}
			],
			"message": "Expected 1 to equal 2"
		}]
	}`
	passed, failures, skipped := opsParseNodeTestJSON(input)
	if len(passed) != 1 {
		t.Errorf("passed = %d, want 1", len(passed))
	}
	if len(failures) != 1 {
		t.Errorf("failures = %d, want 1", len(failures))
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if len(failures) > 0 && failures[0].Test != "App handles error" {
		t.Errorf("failure test = %q, want 'App handles error'", failures[0].Test)
	}
}

func TestOpsParseNodeTestJSON_Vitest(t *testing.T) {
	// Vitest uses assertionResults instead of testResults at the inner level
	input := `{
		"testResults": [{
			"name": "src/utils.test.ts",
			"assertionResults": [
				{"fullName": "adds numbers", "status": "passed", "duration": 10},
				{"fullName": "handles null", "status": "failed", "duration": 20}
			],
			"message": "TypeError: null is not a function"
		}]
	}`
	passed, failures, skipped := opsParseNodeTestJSON(input)
	if len(passed) != 1 {
		t.Errorf("passed = %d, want 1", len(passed))
	}
	if len(failures) != 1 {
		t.Errorf("failures = %d, want 1", len(failures))
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
}

func TestOpsParseNodeTestJSON_Fallback(t *testing.T) {
	// Non-JSON output should use line-based fallback
	input := "PASS src/a.test.ts\nFAIL src/b.test.ts\n✓ it works"
	passed, failures, _ := opsParseNodeTestJSON(input)
	if len(passed) < 1 {
		t.Errorf("passed = %d, want >= 1", len(passed))
	}
	if len(failures) < 1 {
		t.Errorf("failures = %d, want >= 1", len(failures))
	}
}

func TestOpsParsePytestOutput_Verbose(t *testing.T) {
	input := `tests/test_api.py::test_health PASSED
tests/test_api.py::test_create FAILED
E       AssertionError: expected 200 got 500
E       assert response.status_code == 200
tests/test_api.py::test_delete SKIPPED
tests/test_api.py::test_update PASSED`
	passed, failures, skipped := opsParsePytestOutput(input)
	if len(passed) != 2 {
		t.Errorf("passed = %d, want 2", len(passed))
	}
	if len(failures) != 1 {
		t.Errorf("failures = %d, want 1", len(failures))
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	// Failure should capture traceback output
	if len(failures) > 0 && failures[0].Output == "" {
		t.Error("failure output should contain traceback")
	}
}

func TestOpsHasNPMScript(t *testing.T) {
	// Test against current repo (should not have npm scripts)
	if opsHasNPMScript(".", "lint") {
		t.Error("current Go repo should not have npm lint script")
	}
}

// ---------------------------------------------------------------------------
// Session persistence tests
// ---------------------------------------------------------------------------

func TestOpsSessionPersistence(t *testing.T) {
	// Create a temp state dir
	origDir := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origDir)

	// Ensure state dir
	stateDir := opsStateDir()
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	// Create session
	session := &OpsSession{
		ID:          "test-abc",
		Repo:        "/tmp/test-repo",
		Branch:      "main",
		Description: "test session",
		CreatedAt:   time.Now(),
	}
	if err := opsWriteSession(session); err != nil {
		t.Fatalf("write session: %v", err)
	}

	// Read it back
	got, err := opsReadSession("test-abc")
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if got.ID != "test-abc" {
		t.Errorf("ID = %q, want test-abc", got.ID)
	}
	if got.Repo != "/tmp/test-repo" {
		t.Errorf("Repo = %q, want /tmp/test-repo", got.Repo)
	}
	if got.Description != "test session" {
		t.Errorf("Description = %q, want 'test session'", got.Description)
	}
}

func TestOpsIterationPersistence(t *testing.T) {
	tmpHome := t.TempDir()
	origDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origDir)

	stateDir := opsStateDir()
	os.MkdirAll(stateDir, 0o755)

	session := &OpsSession{ID: "iter-test", Repo: "/tmp", Branch: "main", CreatedAt: time.Now()}
	opsWriteSession(session)

	// Append 3 iterations
	for i := 1; i <= 3; i++ {
		opsAppendIteration("iter-test", IterationRecord{
			Number:     i,
			Status:     "all_pass",
			ErrorCount: 3 - i, // decreasing errors
			DurationMs: int64(i * 1000),
		})
	}

	// Read them back
	iters := opsReadIterations("iter-test")
	if len(iters) != 3 {
		t.Fatalf("iterations = %d, want 3", len(iters))
	}
	if iters[0].ErrorCount != 2 {
		t.Errorf("iter 1 errors = %d, want 2", iters[0].ErrorCount)
	}
	if iters[2].ErrorCount != 0 {
		t.Errorf("iter 3 errors = %d, want 0", iters[2].ErrorCount)
	}
}

func TestOpsSessionList(t *testing.T) {
	tmpHome := t.TempDir()
	origDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origDir)

	stateDir := opsStateDir()
	os.MkdirAll(stateDir, 0o755)

	// Create two sessions
	opsWriteSession(&OpsSession{ID: "s1", Repo: "/a", Branch: "main", CreatedAt: time.Now()})
	opsWriteSession(&OpsSession{ID: "s2", Repo: "/b", Branch: "feat/x", CreatedAt: time.Now()})

	ids := opsListSessionIDs()
	if len(ids) != 2 {
		t.Errorf("session count = %d, want 2", len(ids))
	}
}

func TestOpsSessionNotFound(t *testing.T) {
	tmpHome := t.TempDir()
	origDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origDir)

	_, err := opsReadSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

// ---------------------------------------------------------------------------
// ID and helper tests
// ---------------------------------------------------------------------------

func TestOpsID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := opsID()
		if seen[id] {
			t.Errorf("duplicate ID after %d iterations: %s", i, id)
		}
		seen[id] = true
	}
}

func TestOpsResolveRepo_Default(t *testing.T) {
	repo, err := opsResolveRepo("")
	if err != nil {
		t.Fatalf("resolveRepo empty: %v", err)
	}
	if repo == "" {
		t.Error("resolveRepo returned empty string for cwd")
	}
}

func TestOpsCurrentBranch(t *testing.T) {
	branch := opsCurrentBranch(".")
	if branch == "" {
		t.Error("currentBranch returned empty for current repo")
	}
}
