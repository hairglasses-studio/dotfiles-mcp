package dotfiles

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Module registration
// ---------------------------------------------------------------------------

func TestOSSModuleRegistration(t *testing.T) {
	m := &OSSModule{}
	if m.Name() != "oss" {
		t.Fatalf("expected name oss, got %s", m.Name())
	}
	tools := m.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 OSS tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, td := range tools {
		names[td.Tool.Name] = true
	}
	for _, want := range []string{"dotfiles_oss_score", "dotfiles_oss_check"} {
		if !names[want] {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// letterGrade
// ---------------------------------------------------------------------------

func TestLetterGrade(t *testing.T) {
	tests := []struct {
		score int
		max   int
		want  string
	}{
		{95, 100, "A"},
		{90, 100, "A"},
		{85, 100, "B"},
		{80, 100, "B"},
		{75, 100, "C"},
		{70, 100, "C"},
		{65, 100, "D"},
		{60, 100, "D"},
		{50, 100, "F"},
		{0, 100, "F"},
		{0, 0, "F"},   // edge case: zero max
		{10, 10, "A"}, // 100%
	}
	for _, tc := range tests {
		got := letterGrade(tc.score, tc.max)
		if got != tc.want {
			t.Errorf("letterGrade(%d, %d) = %q, want %q", tc.score, tc.max, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// boolInt
// ---------------------------------------------------------------------------

func TestBoolInt(t *testing.T) {
	if got := boolInt(true, 5); got != 5 {
		t.Errorf("boolInt(true, 5) = %d, want 5", got)
	}
	if got := boolInt(false, 5); got != 0 {
		t.Errorf("boolInt(false, 5) = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// ternary
// ---------------------------------------------------------------------------

func TestTernary(t *testing.T) {
	if got := ternary(true, "yes", "no"); got != "yes" {
		t.Errorf("ternary(true) = %q, want yes", got)
	}
	if got := ternary(false, "yes", "no"); got != "no" {
		t.Errorf("ternary(false) = %q, want no", got)
	}
}

// ---------------------------------------------------------------------------
// extractGoVersion / extractModulePath
// ---------------------------------------------------------------------------

func TestExtractGoVersion(t *testing.T) {
	tests := []struct {
		gomod string
		want  string
	}{
		{"module example.com/foo\n\ngo 1.21\n", "1.21"},
		{"module foo\ngo 1.26.1\n", "1.26.1"},
		{"module foo\n", "unknown"},
		{"", "unknown"},
	}
	for _, tc := range tests {
		got := extractGoVersion(tc.gomod)
		if got != tc.want {
			t.Errorf("extractGoVersion(%q) = %q, want %q", tc.gomod, got, tc.want)
		}
	}
}

func TestExtractModulePath(t *testing.T) {
	tests := []struct {
		gomod string
		want  string
	}{
		{"module github.com/foo/bar\n\ngo 1.21\n", "github.com/foo/bar"},
		{"module example.com/x\n", "example.com/x"},
		{"go 1.21\n", "unknown"},
		{"", "unknown"},
	}
	for _, tc := range tests {
		got := extractModulePath(tc.gomod)
		if got != tc.want {
			t.Errorf("extractModulePath(%q) = %q, want %q", tc.gomod, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// fileExists / fileExistsMulti
// ---------------------------------------------------------------------------

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.txt")
	os.WriteFile(existing, []byte("hello"), 0644)

	if !fileExists(existing) {
		t.Error("expected fileExists=true for existing file")
	}
	if fileExists(filepath.Join(dir, "nope.txt")) {
		t.Error("expected fileExists=false for missing file")
	}
}

func TestFileExistsMulti(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "LICENSE.md"), []byte("MIT"), 0644)

	if !fileExistsMulti(dir, []string{"LICENSE", "LICENSE.md", "LICENSE.txt"}) {
		t.Error("expected true when LICENSE.md exists")
	}
	if fileExistsMulti(dir, []string{"NOPE", "ALSO_NOPE"}) {
		t.Error("expected false when no match exists")
	}
}

// ---------------------------------------------------------------------------
// fileCheck / fileCheckMulti
// ---------------------------------------------------------------------------

func TestFileCheck(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0644)

	r := fileCheck(dir, "README.md", 5, "Add a README")
	if !r.Passed || r.Points != 5 {
		t.Errorf("expected passed=true points=5, got passed=%v points=%d", r.Passed, r.Points)
	}

	r = fileCheck(dir, "MISSING.md", 3, "Add it")
	if r.Passed || r.Points != 0 {
		t.Errorf("expected passed=false points=0, got passed=%v points=%d", r.Passed, r.Points)
	}
	if r.Suggestion == "" {
		t.Error("expected suggestion for missing file")
	}
}

func TestFileCheckMulti(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "COPYING"), []byte("license"), 0644)

	r := fileCheckMulti(dir, []string{"LICENSE", "LICENSE.md", "COPYING"}, "LICENSE", 5, "Add a license")
	if !r.Passed || r.Points != 5 {
		t.Errorf("expected passed=true for COPYING, got passed=%v points=%d", r.Passed, r.Points)
	}
}

// ---------------------------------------------------------------------------
// contentCheck
// ---------------------------------------------------------------------------

func TestContentCheck(t *testing.T) {
	text := "# My Project\n\n## Installation\n\ngo install github.com/foo/bar\n"

	r := contentCheck("install", text, 3, func(s string) bool {
		return len(s) > 10
	}, "Found", "Add install instructions")
	if !r.Passed || r.Points != 3 {
		t.Errorf("expected passed=true, got passed=%v points=%d", r.Passed, r.Points)
	}

	r = contentCheck("missing", text, 2, func(s string) bool {
		return false
	}, "Found", "Fix it")
	if r.Passed || r.Points != 0 {
		t.Errorf("expected passed=false, got passed=%v", r.Passed)
	}
	if r.Suggestion == "" {
		t.Error("expected suggestion when check fails")
	}
}

// ---------------------------------------------------------------------------
// countFiles
// ---------------------------------------------------------------------------

func TestCountFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(dir, "b_test.go"), []byte("package b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.go"), []byte("package c"), 0644)

	if got := countFiles(dir, "*_test.go"); got != 2 {
		t.Errorf("countFiles(*_test.go) = %d, want 2", got)
	}
	if got := countFiles(dir, "*.go"); got != 3 {
		t.Errorf("countFiles(*.go) = %d, want 3", got)
	}
	if got := countFiles(dir, "*.rs"); got != 0 {
		t.Errorf("countFiles(*.rs) = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// grepFiles
// ---------------------------------------------------------------------------

func TestGrepFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"context\"\n"), 0644)

	if !grepFiles(dir, `import "context"`) {
		t.Error("expected grepFiles to find context import")
	}
	if grepFiles(dir, `zzz_nonexistent_pattern`) {
		t.Error("expected grepFiles to return false for missing pattern")
	}
}

// ---------------------------------------------------------------------------
// scanFilesForPattern
// ---------------------------------------------------------------------------

func TestScanFilesForPattern(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "secret.go")
	os.WriteFile(goFile, []byte("var apiKey = \"sk-abc123\"\n"), 0644)

	// A _test.go file with same content should be skipped.
	os.WriteFile(filepath.Join(dir, "secret_test.go"), []byte("var apiKey = \"sk-abc123\"\n"), 0644)

	// Non-Go file should be skipped too.
	os.WriteFile(filepath.Join(dir, "data.bin"), []byte("var apiKey = \"sk-abc123\"\n"), 0644)

	// Pattern matching in .go file (not _test.go).
	re := regexp.MustCompile(`sk-[a-zA-Z0-9]{6}`)
	match := scanFilesForPattern(dir, re)
	if match == "" {
		t.Error("expected to find pattern in secret.go")
	}
	if match != goFile {
		t.Errorf("expected match in %s, got %s", goFile, match)
	}
}

// ---------------------------------------------------------------------------
// checkCommunity — integration with temp repo
// ---------------------------------------------------------------------------

func TestCheckCommunity(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644)
	os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("MIT"), 0644)

	results := checkCommunity(dir)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	// README and LICENSE should pass.
	readmeOK := false
	licenseOK := false
	for _, r := range results {
		if r.Name == "readme_md" && r.Passed {
			readmeOK = true
		}
		if r.Name == "license" && r.Passed {
			licenseOK = true
		}
	}
	if !readmeOK {
		t.Error("expected README.md check to pass")
	}
	if !licenseOK {
		t.Error("expected LICENSE check to pass")
	}
}

// ---------------------------------------------------------------------------
// checkReadme — integration with temp repo
// ---------------------------------------------------------------------------

func TestCheckReadme_WithContent(t *testing.T) {
	dir := t.TempDir()
	content := `# My Project

A comprehensive tool for managing dotfiles across Linux systems.

![Build](https://img.shields.io/github/actions/workflow/status/user/repo/ci.yml)

## Installation

` + "```bash\ngo install github.com/user/repo@latest\n```\n\n" +
		"## Usage\n\n```bash\nrepo serve\n```\n\n" +
		"## API\n\n| Tool | Description |\n|------|-------------|\n| foo | does stuff |\n\n" +
		"## License\n\nMIT License\n"

	os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0644)

	results := checkReadme(dir)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	passedCount := 0
	for _, r := range results {
		if r.Passed {
			passedCount++
		}
	}
	// With badges, install, usage, description, API, and license section, all 6 should pass.
	if passedCount != 6 {
		t.Errorf("expected 6 passing checks, got %d", passedCount)
		for _, r := range results {
			t.Logf("  %s: passed=%v detail=%s", r.Name, r.Passed, r.Detail)
		}
	}
}

func TestCheckReadme_NoReadme(t *testing.T) {
	dir := t.TempDir()
	results := checkReadme(dir)
	if len(results) != 1 {
		t.Fatalf("expected 1 fallback result, got %d", len(results))
	}
	if results[0].Passed {
		t.Error("expected check to fail when README.md is missing")
	}
}

// ---------------------------------------------------------------------------
// checkGoMod — integration with temp repo
// ---------------------------------------------------------------------------

func TestCheckGoMod_WithGoMod(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/test/repo\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "go.sum"), []byte(""), 0644)

	results := checkGoMod(dir)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	gomodFound := false
	for _, r := range results {
		if r.Name == "gomod_exists" && r.Passed {
			gomodFound = true
		}
	}
	if !gomodFound {
		t.Error("expected gomod_exists to pass")
	}
}

func TestCheckGoMod_NoGoMod(t *testing.T) {
	dir := t.TempDir()
	results := checkGoMod(dir)
	if len(results) != 1 {
		t.Fatalf("expected 1 fallback result, got %d", len(results))
	}
	if results[0].Passed {
		t.Error("expected check to fail when go.mod is missing")
	}
}

// ---------------------------------------------------------------------------
// checkCICD
// ---------------------------------------------------------------------------

func TestCheckCICD(t *testing.T) {
	dir := t.TempDir()
	ghDir := filepath.Join(dir, ".github", "workflows")
	os.MkdirAll(ghDir, 0755)
	os.WriteFile(filepath.Join(ghDir, "ci.yml"), []byte("name: CI\non: push\njobs:\n  test:\n    runs-on: ubuntu-latest"), 0644)

	results := checkCICD(dir)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}
	ciFound := false
	for _, r := range results {
		if r.Name == "has_workflows" && r.Passed {
			ciFound = true
		}
	}
	if !ciFound {
		t.Error("expected has_workflows to pass when workflow file exists")
	}
}

// ---------------------------------------------------------------------------
// checkSecurity
// ---------------------------------------------------------------------------

func TestCheckSecurity(t *testing.T) {
	dir := t.TempDir()
	// Create a .go file without hardcoded secrets
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	results := checkSecurity(dir)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}
}

// ---------------------------------------------------------------------------
// runCategory — unknown category
// ---------------------------------------------------------------------------

func TestRunCategory_Unknown(t *testing.T) {
	dir := t.TempDir()
	result := runCategory(dir, "nonexistent_category", true)
	if result.Category != "nonexistent_category" {
		t.Errorf("expected category name in result, got %s", result.Category)
	}
	if len(result.Checks) == 0 {
		t.Error("expected at least one check result for unknown category")
	}
}

// ---------------------------------------------------------------------------
// runFullScore — integration with minimal repo
// ---------------------------------------------------------------------------

func TestRunFullScore_MinimalRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\nA test project.\n"), 0644)
	os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("MIT License"), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "go.sum"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	result := runFullScore(dir, true)
	if result.RepoName == "" {
		t.Error("expected non-empty repo name")
	}
	if result.Grade == "" {
		t.Error("expected non-empty grade")
	}
	if result.MaxScore == 0 {
		t.Error("expected non-zero max score")
	}
	if len(result.Categories) != 8 {
		t.Errorf("expected 8 categories, got %d", len(result.Categories))
	}
	// Total score should be > 0 since README and LICENSE exist.
	if result.TotalScore == 0 {
		t.Error("expected non-zero total score for repo with README and LICENSE")
	}
}

// ---------------------------------------------------------------------------
// Tool handler input validation
// ---------------------------------------------------------------------------

func TestOSSScore_MissingRepoPath(t *testing.T) {
	m := &OSSModule{}
	var td registry.ToolDefinition
	for _, tool := range m.Tools() {
		if tool.Tool.Name == "dotfiles_oss_score" {
			td = tool
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"repo_path": ""}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for empty repo_path")
	}
}

func TestOSSScore_InvalidRepoPath(t *testing.T) {
	m := &OSSModule{}
	var td registry.ToolDefinition
	for _, tool := range m.Tools() {
		if tool.Tool.Name == "dotfiles_oss_score" {
			td = tool
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"repo_path": "/nonexistent/path/xyz"}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for invalid repo_path")
	}
}

func TestOSSCheck_MissingRepoPath(t *testing.T) {
	m := &OSSModule{}
	var td registry.ToolDefinition
	for _, tool := range m.Tools() {
		if tool.Tool.Name == "dotfiles_oss_check" {
			td = tool
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"repo_path": "", "category": "community"}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for empty repo_path")
	}
}

// ---------------------------------------------------------------------------
// checkTesting — skip_tests path
// ---------------------------------------------------------------------------

func TestCheckTesting_SkipTests(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\nfunc TestFoo(t *testing.T) {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0644)

	results := checkTesting(dir, true)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	hasTestsOK := false
	covSkipped := false
	for _, r := range results {
		if r.Name == "has_tests" && r.Passed {
			hasTestsOK = true
		}
		if r.Name == "coverage" && !r.Passed && r.Points == 0 {
			covSkipped = true
		}
	}
	if !hasTestsOK {
		t.Error("expected has_tests to pass")
	}
	if !covSkipped {
		t.Error("expected coverage to be skipped")
	}
}

func TestCheckTesting_NoGoMod(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\nfunc TestFoo(t *testing.T) {}\n"), 0644)
	// No go.mod

	results := checkTesting(dir, false)
	// Without go.mod, should skip coverage/race even with skip_tests=false
	covSkipped := false
	for _, r := range results {
		if r.Name == "coverage" && !r.Passed && r.Points == 0 {
			covSkipped = true
		}
	}
	if !covSkipped {
		t.Error("expected coverage to be skipped without go.mod")
	}
}

func TestCheckTesting_NoTestFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	results := checkTesting(dir, true)
	hasTestsFailed := false
	for _, r := range results {
		if r.Name == "has_tests" && !r.Passed {
			hasTestsFailed = true
		}
	}
	if !hasTestsFailed {
		t.Error("expected has_tests to fail when no test files exist")
	}
}

// ---------------------------------------------------------------------------
// checkRelease — integration
// ---------------------------------------------------------------------------

func TestCheckRelease_MinimalRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/test/repo\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	// Initialize git repo for semver tag check
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		// Not a git repo -- checkRelease won't find tags, which is fine
	}

	results := checkRelease(dir)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	// Should detect public module path
	publicPath := false
	installable := false
	for _, r := range results {
		if r.Name == "public_module_path" && r.Passed {
			publicPath = true
		}
		if r.Name == "installable" && r.Passed {
			installable = true
		}
	}
	if !publicPath {
		t.Error("expected public_module_path to pass for github.com module")
	}
	if !installable {
		t.Error("expected installable to pass with main.go present")
	}
}

func TestCheckRelease_LibraryOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/test/lib\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package lib\nfunc Foo() {}\n"), 0644)

	results := checkRelease(dir)
	installable := false
	for _, r := range results {
		if r.Name == "installable" && !r.Passed {
			installable = true
		}
	}
	if !installable {
		t.Error("expected installable to fail for library-only repo")
	}
}

// ---------------------------------------------------------------------------
// checkMaintenance — integration
// ---------------------------------------------------------------------------

func TestCheckMaintenance_WithTemplates(t *testing.T) {
	dir := t.TempDir()
	// Create issue templates
	os.MkdirAll(filepath.Join(dir, ".github", "ISSUE_TEMPLATE"), 0755)
	os.WriteFile(filepath.Join(dir, ".github", "ISSUE_TEMPLATE", "bug.md"), []byte("bug template"), 0644)
	// Create PR template
	os.WriteFile(filepath.Join(dir, ".github", "pull_request_template.md"), []byte("PR template"), 0644)
	// Create .editorconfig
	os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n"), 0644)

	results := checkMaintenance(dir)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}

	issueTemplateOK := false
	prTemplateOK := false
	editorconfigOK := false
	for _, r := range results {
		if r.Name == "issue_templates" && r.Passed {
			issueTemplateOK = true
		}
		if r.Name == "pr_template" && r.Passed {
			prTemplateOK = true
		}
		if r.Name == "editorconfig" && r.Passed {
			editorconfigOK = true
		}
	}
	if !issueTemplateOK {
		t.Error("expected issue_templates to pass")
	}
	if !prTemplateOK {
		t.Error("expected pr_template to pass")
	}
	if !editorconfigOK {
		t.Error("expected editorconfig to pass")
	}
}

func TestCheckMaintenance_Bare(t *testing.T) {
	dir := t.TempDir()

	results := checkMaintenance(dir)
	for _, r := range results {
		if r.Name == "issue_templates" && r.Passed {
			t.Error("expected issue_templates to fail when not present")
		}
		if r.Name == "pr_template" && r.Passed {
			t.Error("expected pr_template to fail when not present")
		}
		if r.Name == "editorconfig" && r.Passed {
			t.Error("expected editorconfig to fail when not present")
		}
	}
}

// ---------------------------------------------------------------------------
// goVetCheck — integration
// ---------------------------------------------------------------------------

func TestGoVetCheck_ValidRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	results := goVetCheck(dir, 5)
	if len(results) == 0 {
		t.Fatal("expected non-empty results")
	}
	if !results[0].Passed {
		t.Errorf("expected go vet to pass on valid code, detail: %s", results[0].Detail)
	}
	if results[0].Points != 5 {
		t.Errorf("expected 5 points, got %d", results[0].Points)
	}
}

func TestOSSCheck_InvalidRepoPath(t *testing.T) {
	m := &OSSModule{}
	var td registry.ToolDefinition
	for _, tool := range m.Tools() {
		if tool.Tool.Name == "dotfiles_oss_check" {
			td = tool
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"repo_path": "/nonexistent/path/xyz", "category": "community"}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for invalid repo_path")
	}
}
