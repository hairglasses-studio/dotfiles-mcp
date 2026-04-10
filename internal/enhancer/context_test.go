package enhancer

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		text := strings.Repeat("word ", 100) // 500 chars
		tokens := EstimateTokens(text)
		if tokens < 100 || tokens > 150 {
			t.Errorf("Expected ~125 tokens for 500 chars, got %d", tokens)
		}
	})

	t.Run("empty", func(t *testing.T) {
		tokens := EstimateTokens("")
		if tokens != 0 {
			t.Errorf("Expected 0 tokens for empty string, got %d", tokens)
		}
	})

	t.Run("unicode_CJK", func(t *testing.T) {
		text := strings.Repeat("你好世界", 100) // 400 CJK chars = 400 runes
		tokens := EstimateTokens(text)
		if tokens != 100 {
			t.Errorf("Expected 100 tokens for 400 CJK runes, got %d", tokens)
		}
	})

	t.Run("emoji", func(t *testing.T) {
		text := strings.Repeat("🎵", 100) // 100 emoji runes
		tokens := EstimateTokens(text)
		if tokens != 25 {
			t.Errorf("Expected 25 tokens for 100 emoji runes, got %d", tokens)
		}
	})

	t.Run("mixed_scripts", func(t *testing.T) {
		text := "Hello 你好 🌍 مرحبا"
		tokens := EstimateTokens(text)
		if tokens == 0 {
			t.Error("Should produce non-zero tokens for mixed scripts")
		}
	})
}

func TestReorderLongContext_ShortPrompt(t *testing.T) {
	text := "Analyze this data please."
	result, imps := ReorderLongContext(text)

	if len(imps) > 0 {
		t.Error("Should not reorder short prompts")
	}
	if result != text {
		t.Error("Should return unchanged")
	}
}

func TestReorderLongContext_AlreadyStructured(t *testing.T) {
	text := "<context>Long data here</context>\n\nWhat does this mean?"
	result, imps := ReorderLongContext(text)

	if len(imps) > 0 {
		t.Error("Should not reorder already-structured prompts")
	}
	if result != text {
		t.Error("Should return unchanged")
	}
}

func TestReorderLongContext_ActualReorder(t *testing.T) {
	// Build: short query + large context block (query first, context after = wrong order)
	query := "What patterns do you see in this data?"
	context := strings.Repeat("User session data point with timestamp and metrics. ", 1000)
	text := query + "\n\n" + context

	result, imps := ReorderLongContext(text)
	if len(imps) == 0 {
		t.Error("Should reorder when query comes before long context")
	}
	if result == text {
		t.Error("Should produce different text after reordering")
	}
	// After reorder, the long context should appear before the query
	contextIdx := strings.Index(result, "User session data")
	queryIdx := strings.Index(result, "What patterns")
	if contextIdx == -1 || queryIdx == -1 {
		t.Error("Both context and query should be present")
	} else if contextIdx > queryIdx {
		t.Error("After reorder, context should appear before query")
	}
}

func TestInjectQuoteGrounding_ShortPrompt(t *testing.T) {
	text := "Analyze this briefly."
	result, imps := InjectQuoteGrounding(text, TaskTypeAnalysis)

	if len(imps) > 0 {
		t.Error("Should not inject grounding for short prompts")
	}
	if result != text {
		t.Error("Should return unchanged")
	}
}

func TestInjectQuoteGrounding_AlreadyHasQuotes(t *testing.T) {
	text := strings.Repeat("data point. ", 5000) + "\nPlease quote the relevant sections."
	result, imps := InjectQuoteGrounding(text, TaskTypeAnalysis)

	if len(imps) > 0 {
		t.Error("Should not inject grounding when 'quote' already mentioned")
	}
	if result != text {
		t.Error("Should return unchanged")
	}
}

func TestInjectQuoteGrounding_LongAnalysis(t *testing.T) {
	text := strings.Repeat("The system logged an important data point about user behavior. ", 400)
	text += "\n\nAnalyze the patterns in the data above."

	result, imps := InjectQuoteGrounding(text, TaskTypeAnalysis)

	if len(imps) == 0 {
		t.Error("Should inject grounding for long analysis prompts")
	}
	assertContains(t, result, "<quotes>")
}

// --- Cache-friendly order verification (subtests) ---

func TestVerifyCacheFriendlyOrder(t *testing.T) {
	t.Run("dynamic_before_static", func(t *testing.T) {
		text := "<instructions>\nProcess the {{user_input}} data.\n</instructions>\n\n<role>You are an expert analyst.</role>\n\n<constraints>\nBe thorough.\n</constraints>"
		results := VerifyCacheFriendlyOrder(text)
		assertLintCategory(t, results, "cache-unfriendly-order")
	})

	t.Run("correct_order", func(t *testing.T) {
		text := "<role>You are an expert analyst.</role>\n\n<constraints>\nBe thorough.\n</constraints>\n\n<instructions>\nProcess the {{user_input}} data.\n</instructions>"
		results := VerifyCacheFriendlyOrder(text)
		assertNoLintCategory(t, results, "cache-unfriendly-order")
	})

	t.Run("early_variable", func(t *testing.T) {
		text := "Process {{user_input}} now.\n\nSome more static content here that doesn't change.\n\nAnd even more static content that could be cached."
		results := VerifyCacheFriendlyOrder(text)
		assertLintCategory(t, results, "cache-unfriendly-variable")
	})

	t.Run("no_structure_long", func(t *testing.T) {
		text := strings.Repeat("This is a long unstructured prompt without any XML tags. ", 100)
		results := VerifyCacheFriendlyOrder(text)
		assertLintCategory(t, results, "cache-no-structure")
	})

	t.Run("no_structure_short", func(t *testing.T) {
		results := VerifyCacheFriendlyOrder("Fix the bug.")
		assertNoLintCategory(t, results, "cache-no-structure")
	})
}

// --- Phase 3D: New coverage tests ---

func TestSplitParagraphs(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := splitParagraphs("")
		if len(result) != 0 {
			t.Errorf("empty input should return 0 paragraphs, got %d", len(result))
		}
	})

	t.Run("single", func(t *testing.T) {
		result := splitParagraphs("hello world")
		if len(result) != 1 {
			t.Errorf("single paragraph should return 1, got %d", len(result))
		}
	})

	t.Run("multiple", func(t *testing.T) {
		result := splitParagraphs("first\n\nsecond\n\nthird")
		if len(result) != 3 {
			t.Errorf("three paragraphs should return 3, got %d", len(result))
		}
	})

	t.Run("consecutive_blanks", func(t *testing.T) {
		result := splitParagraphs("first\n\n\n\nsecond")
		if len(result) != 2 {
			t.Errorf("consecutive blanks should still give 2, got %d", len(result))
		}
	})
}

func TestIsImperative(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{"analyze", "Analyze this data", true},
		{"review", "Review this code", true},
		{"create", "Create a function", true},
		{"write", "Write unit tests", true},
		{"fix", "Fix the bug", true},
		{"what", "What happened here", true},
		{"how", "How does this work", true},
		{"please", "Please help me", true},
		{"can_you", "Can you explain", true},
		{"plain_noun", "The quick brown fox", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isImperative(tt.text)
			if got != tt.expected {
				t.Errorf("isImperative(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}
