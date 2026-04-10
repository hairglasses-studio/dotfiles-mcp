package enhancer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig_IsStageDisabled(t *testing.T) {
	cfg := Config{
		DisabledStages: []string{"tone_downgrade", "preamble_suppression"},
	}

	if !cfg.IsStageDisabled("tone_downgrade") {
		t.Error("Should report tone_downgrade as disabled")
	}
	if !cfg.IsStageDisabled("TONE_DOWNGRADE") {
		t.Error("Should be case-insensitive")
	}
	if cfg.IsStageDisabled("specificity") {
		t.Error("Should not report specificity as disabled")
	}
}

func TestConfig_ApplyRules(t *testing.T) {
	t.Run("match_append", func(t *testing.T) {
		cfg := Config{
			Rules: []Rule{
				{Match: "add tool", Append: "Follow the existing tool registration pattern using init()."},
			},
		}
		text := "Add tool for audio processing in the studio"
		result, imps := cfg.ApplyRules(text)
		if len(imps) == 0 {
			t.Fatal("Should apply matching rule")
		}
		assertContains(t, result, "tool registration pattern")
	})

	t.Run("no_match", func(t *testing.T) {
		cfg := Config{
			Rules: []Rule{
				{Match: "deploy", Append: "Check CI first."},
			},
		}
		_, imps := cfg.ApplyRules("Write a function to sort users")
		if len(imps) > 0 {
			t.Error("Should not apply non-matching rules")
		}
	})

	t.Run("both_prepend_and_append", func(t *testing.T) {
		cfg := Config{
			Rules: []Rule{
				{
					Match:   "fix",
					Prepend: "Include a test that reproduces the bug.",
					Append:  "Verify the fix with existing tests.",
				},
			},
		}
		text := "Fix the sorting bug in the user module"
		result, imps := cfg.ApplyRules(text)
		if len(imps) != 2 {
			t.Errorf("expected 2 improvements (prepend + append), got %d", len(imps))
		}
		assertContains(t, result, "Include a test")
		assertContains(t, result, "Verify the fix")
		// Prepend should be before original text
		prepIdx := strings.Index(result, "Include a test")
		origIdx := strings.Index(result, "Fix the sorting")
		if prepIdx > origIdx {
			t.Error("Prepend should appear before original text")
		}
	})

	t.Run("empty_match_skipped", func(t *testing.T) {
		cfg := Config{
			Rules: []Rule{
				{Match: "", Append: "Should not be applied"},
			},
		}
		text := "Write a function"
		_, imps := cfg.ApplyRules(text)
		if len(imps) > 0 {
			t.Error("Empty match rule should be skipped")
		}
	})

	t.Run("case_insensitive", func(t *testing.T) {
		cfg := Config{
			Rules: []Rule{
				{Match: "ADD TOOL", Append: "Follow pattern."},
			},
		}
		text := "add tool for audio"
		_, imps := cfg.ApplyRules(text)
		if len(imps) == 0 {
			t.Error("Rule matching should be case-insensitive")
		}
	})
}

func TestConfig_LoadConfig_Missing(t *testing.T) {
	cfg := LoadConfig("/nonexistent/path")
	if cfg.Preamble != "" || len(cfg.Rules) > 0 {
		t.Error("Should return zero config for missing file")
	}
}

func TestConfig_LoadConfig_ValidYAML(t *testing.T) {
	cfg := Config{
		Preamble:        "Test project preamble.",
		DefaultTaskType: "code",
		DefaultEffort:   "high",
		DisabledStages:  []string{"tone_downgrade"},
		BlockPatterns:   []string{"drop table"},
		Rules: []Rule{
			{Match: "fix", Prepend: "Add a test.", Append: "Verify fix."},
		},
	}
	dir := writeTempYAML(t, cfg)
	loaded := LoadConfig(dir)

	if loaded.Preamble != "Test project preamble." {
		t.Errorf("Preamble = %q, want 'Test project preamble.'", loaded.Preamble)
	}
	if loaded.DefaultTaskType != "code" {
		t.Errorf("DefaultTaskType = %q, want 'code'", loaded.DefaultTaskType)
	}
	if loaded.DefaultEffort != "high" {
		t.Errorf("DefaultEffort = %q, want 'high'", loaded.DefaultEffort)
	}
	if len(loaded.DisabledStages) != 1 || loaded.DisabledStages[0] != "tone_downgrade" {
		t.Errorf("DisabledStages = %v, want [tone_downgrade]", loaded.DisabledStages)
	}
	if len(loaded.BlockPatterns) != 1 {
		t.Errorf("BlockPatterns = %v, want 1 pattern", loaded.BlockPatterns)
	}
	if len(loaded.Rules) != 1 {
		t.Errorf("Rules = %v, want 1 rule", loaded.Rules)
	}
}

func TestConfig_LoadConfig_AlternatePaths(t *testing.T) {
	t.Run("yml_extension", func(t *testing.T) {
		dir := t.TempDir()
		content := []byte("preamble: yml test\n")
		os.WriteFile(filepath.Join(dir, ".prompt-improver.yml"), content, 0644)
		cfg := LoadConfig(dir)
		if cfg.Preamble != "yml test" {
			t.Errorf("should load .yml file, got preamble %q", cfg.Preamble)
		}
	})

	t.Run("claude_subdir", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
		content := []byte("preamble: claude subdir test\n")
		os.WriteFile(filepath.Join(dir, ".claude", "prompt-improver.yaml"), content, 0644)
		cfg := LoadConfig(dir)
		if cfg.Preamble != "claude subdir test" {
			t.Errorf("should load from .claude/ subdir, got preamble %q", cfg.Preamble)
		}
	})
}

func TestConfig_BlockPatterns(t *testing.T) {
	cfg := Config{
		BlockPatterns: []string{"drop table", "rm -rf"},
	}
	dir := writeTempYAML(t, cfg)
	loaded := LoadConfig(dir)
	if len(loaded.BlockPatterns) != 2 {
		t.Errorf("expected 2 block patterns, got %d", len(loaded.BlockPatterns))
	}
}

func TestEnhanceWithConfig_DisabledStage(t *testing.T) {
	cfg := Config{
		DisabledStages: []string{"structure"},
	}

	result := EnhanceWithConfig("write a function to sort users by name with error handling and edge cases covered", "", cfg)
	assertNotContains(t, result.Enhanced, "<role>")
	assertStageNotRun(t, result.StagesRun, "structure")
}

func TestEnhanceWithConfig_Preamble(t *testing.T) {
	cfg := Config{
		Preamble: "This is the hg-mcp project, a Go MCP server.",
	}

	result := EnhanceWithConfig("write a function to sort users by name with error handling and edge cases", "", cfg)

	if !strings.HasPrefix(result.Enhanced, "This is the hg-mcp project") {
		t.Error("Should prepend preamble to enhanced output")
	}
}
