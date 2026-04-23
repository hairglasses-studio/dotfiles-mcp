package enhancer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// assertLintCategory asserts that at least one LintResult has the given category.
func assertLintCategory(t *testing.T, results []LintResult, category string) {
	t.Helper()
	for _, r := range results {
		if r.Category == category {
			return
		}
	}
	t.Errorf("expected lint category %q, but not found in %d results", category, len(results))
}

// assertNoLintCategory asserts that no LintResult has the given category.
func assertNoLintCategory(t *testing.T, results []LintResult, category string) {
	t.Helper()
	for _, r := range results {
		if r.Category == category {
			t.Errorf("unexpected lint category %q found: %s", category, r.Original)
			return
		}
	}
}

// assertContains asserts that got contains want as a substring.
func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("expected string to contain %q, got:\n%s", want, truncate(got, 200))
	}
}

// assertNotContains asserts that got does NOT contain substr.
func assertNotContains(t *testing.T, got, substr string) {
	t.Helper()
	if strings.Contains(got, substr) {
		t.Errorf("expected string NOT to contain %q, got:\n%s", substr, truncate(got, 200))
	}
}

// assertImprovementMentions asserts that at least one improvement string contains keyword.
func assertImprovementMentions(t *testing.T, improvements []string, keyword string) {
	t.Helper()
	for _, imp := range improvements {
		if strings.Contains(imp, keyword) {
			return
		}
	}
	t.Errorf("expected an improvement mentioning %q, got: %v", keyword, improvements)
}

// assertStageNotRun asserts that the given stage does NOT appear in the stages list.
func assertStageNotRun(t *testing.T, stages []string, stage string) {
	t.Helper()
	for _, s := range stages {
		if s == stage {
			t.Errorf("expected stage %q NOT to be run, but it was", stage)
			return
		}
	}
}

// writeTempCLAUDEMD writes content to a temporary CLAUDE.md file and returns its path.
func writeTempCLAUDEMD(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeTempYAML writes a Config to a temporary .prompt-improver.yaml file
// and returns the directory path (for use with LoadConfig).
func writeTempYAML(t *testing.T, cfg Config) string {
	t.Helper()
	dir := t.TempDir()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".prompt-improver.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// truncate returns s truncated to maxLen with "..." appended if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
