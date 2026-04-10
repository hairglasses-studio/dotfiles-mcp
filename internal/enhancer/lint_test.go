package enhancer

import (
	"strings"
	"testing"
)

func TestLint_UnmotivatedRule(t *testing.T) {
	text := "Always use structured error responses.\nHandle all edge cases."
	results := Lint(text)
	assertLintCategory(t, results, "unmotivated-rule")

	// Verify suggestion mentions "because"
	for _, r := range results {
		if r.Category == "unmotivated-rule" {
			assertContains(t, r.Suggestion, "because")
			break
		}
	}
}

func TestLint_MotivatedRule(t *testing.T) {
	text := "Always use structured error responses because the AI assistant uses the error type to decide whether to retry."
	results := Lint(text)
	for _, r := range results {
		if r.Category == "unmotivated-rule" && strings.Contains(r.Original, "structured error") {
			t.Error("Should NOT flag motivated rules (contains 'because')")
		}
	}
}

func TestLint_AggressiveEmphasis(t *testing.T) {
	text := "CRITICAL: You MUST follow this rule."
	results := Lint(text)
	assertLintCategory(t, results, "aggressive-emphasis")

	for _, r := range results {
		if r.Category == "aggressive-emphasis" {
			if !r.AutoFixable {
				t.Error("Aggressive emphasis should be auto-fixable")
			}
			break
		}
	}
}

func TestLint_VagueQuantifiers(t *testing.T) {
	text := "Return several items from the list with appropriate formatting."
	results := Lint(text)
	assertLintCategory(t, results, "vague-quantifier")

	for _, r := range results {
		if r.Category == "vague-quantifier" {
			assertContains(t, r.Suggestion, "specific numbers")
			break
		}
	}
}

func TestLint_CleanPrompt(t *testing.T) {
	text := "Return exactly 5 user records as JSON, sorted by creation date."
	results := Lint(text)

	for _, r := range results {
		if r.Severity == "warn" || r.Severity == "error" {
			t.Errorf("Clean prompt should not have warn/error findings, got: %s - %s", r.Category, r.Original)
		}
	}
}

func TestLint_NegativeFraming(t *testing.T) {
	text := "NEVER include personal information in the output headers."
	results := Lint(text)

	found := false
	for _, r := range results {
		if r.Category == "negative-framing" || r.Category == "aggressive-emphasis" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Should detect negative framing or aggressive emphasis")
	}
}

// --- Overtrigger phrase lint (subtests) ---

func TestLint_OvertriggerPhrase(t *testing.T) {
	t.Run("detects_phrase", func(t *testing.T) {
		results := Lint("CRITICAL: You MUST use this tool when processing data.")
		assertLintCategory(t, results, "overtrigger-phrase")
		for _, r := range results {
			if r.Category == "overtrigger-phrase" {
				if !r.AutoFixable {
					t.Error("Overtrigger phrase should be auto-fixable")
				}
				break
			}
		}
	})

	t.Run("clean_prompt", func(t *testing.T) {
		results := Lint("Use this tool when processing data.")
		assertNoLintCategory(t, results, "overtrigger-phrase")
	})
}

// --- Over-specification lint (subtests) ---

func TestLint_OverSpecification(t *testing.T) {
	t.Run("many_steps", func(t *testing.T) {
		text := "Follow these steps:\n1. Read the file\n2. Parse the JSON\n3. Validate the schema\n4. Extract the fields\n5. Transform the data\n6. Write the output\n7. Verify the result"
		results := Lint(text)
		assertLintCategory(t, results, "over-specification")
		for _, r := range results {
			if r.Category == "over-specification" {
				assertContains(t, r.Suggestion, "end-state")
				break
			}
		}
	})

	t.Run("few_steps", func(t *testing.T) {
		results := Lint("1. Read file\n2. Process\n3. Save")
		assertNoLintCategory(t, results, "over-specification")
	})

	t.Run("no_steps", func(t *testing.T) {
		results := Lint("Fix the bug in the sorting function.")
		assertNoLintCategory(t, results, "over-specification")
	})
}

// --- Decomposition suggestion lint (subtests) ---

func TestLint_Decomposition(t *testing.T) {
	t.Run("multi_task", func(t *testing.T) {
		results := Lint("Create the API endpoint, fix the database migration, and deploy the service to production.")
		assertLintCategory(t, results, "decomposition-needed")
	})

	t.Run("single_task", func(t *testing.T) {
		results := Lint("Create a function to sort users.")
		assertNoLintCategory(t, results, "decomposition-needed")
	})

	t.Run("repeated_verb", func(t *testing.T) {
		results := Lint("Create the user model, create the user controller, create the user view.")
		assertNoLintCategory(t, results, "decomposition-needed")
	})
}

// --- Injection vulnerability lint (subtests) ---

func TestLint_InjectionVulnerability(t *testing.T) {
	t.Run("dollar_brace", func(t *testing.T) {
		results := Lint("Process this request: ${user_input}")
		assertLintCategory(t, results, "injection-risk")
		for _, r := range results {
			if r.Category == "injection-risk" {
				if r.Severity != "error" {
					t.Error("Injection risk should be error severity")
				}
				break
			}
		}
	})

	t.Run("double_handlebars", func(t *testing.T) {
		results := Lint("Respond to: {{user_query}}")
		assertLintCategory(t, results, "injection-risk")
	})

	t.Run("safe_var", func(t *testing.T) {
		results := Lint("The system name is {{system_name}}")
		assertNoLintCategory(t, results, "injection-risk")
	})

	t.Run("no_vars", func(t *testing.T) {
		results := Lint("Write a function to sort users.")
		assertNoLintCategory(t, results, "injection-risk")
	})
}

// --- Thinking mode detection lint (subtests) ---

func TestLint_ThinkingMode(t *testing.T) {
	t.Run("detects_phrase", func(t *testing.T) {
		results := Lint("Think step by step about how to solve this problem.")
		assertLintCategory(t, results, "thinking-mode-redundant")
	})

	t.Run("clean_prompt", func(t *testing.T) {
		results := Lint("Solve this problem efficiently.")
		assertNoLintCategory(t, results, "thinking-mode-redundant")
	})
}

// --- Example quality lint (subtests) ---

func TestLint_ExampleQuality(t *testing.T) {
	t.Run("too_few", func(t *testing.T) {
		results := Lint("<example>one</example>")
		assertLintCategory(t, results, "example-quality")
	})

	t.Run("too_many", func(t *testing.T) {
		results := Lint("<example>1</example><example>2</example><example>3</example><example>4</example><example>5</example><example>6</example>")
		found := false
		for _, r := range results {
			if r.Category == "example-quality" && strings.Contains(r.Suggestion, "diminishing") {
				found = true
				break
			}
		}
		if !found {
			t.Error("Should flag too many examples")
		}
	})

	t.Run("good_count", func(t *testing.T) {
		results := Lint("<example>1</example><example>2</example><example>3</example>")
		assertNoLintCategory(t, results, "example-quality")
	})
}

// --- Compaction readiness lint (subtests) ---

func TestLint_CompactionReadiness(t *testing.T) {
	t.Run("long_prompt", func(t *testing.T) {
		text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 6000)
		results := Lint(text)
		assertLintCategory(t, results, "compaction-readiness")
	})

	t.Run("short_prompt", func(t *testing.T) {
		results := Lint("Fix the bug in sorting.")
		assertNoLintCategory(t, results, "compaction-readiness")
	})
}
