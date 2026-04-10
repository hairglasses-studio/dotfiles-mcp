package enhancer

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// CharsPerToken is the approximate number of characters per token for English text.
	// Anthropic uses ~4 chars/token for English, ~3.5 for code.
	CharsPerToken = 4

	// LongContextThreshold is the token count above which context reordering is applied.
	// Per Anthropic: placing long context before the query improves quality by up to 30%.
	LongContextThreshold = 20_000

	// LongContextChars is the character threshold (tokens * chars/token).
	LongContextChars = LongContextThreshold * CharsPerToken // 80,000
)

// EstimateTokens returns an approximate token count for text.
func EstimateTokens(text string) int {
	return utf8.RuneCountInString(text) / CharsPerToken
}

// ReorderLongContext detects prompts where a large context block appears after
// the instruction/query, and moves it before the query. This follows Anthropic's
// recommendation that long context should appear at the top of the prompt.
//
// Returns the reordered text and improvements, or the original text unchanged.
func ReorderLongContext(text string) (string, []string) {
	// Already structured with XML — don't rearrange
	lower := strings.ToLower(text)
	if strings.Contains(lower, "<context>") || strings.Contains(lower, "<documents>") {
		return text, nil
	}

	// Not long enough to benefit from reordering
	if len(text) < LongContextChars/4 { // start checking at ~20K chars
		return text, nil
	}

	// Strategy: Split into paragraphs, find the "query" paragraph (short, has ? or imperative),
	// and find the "context" paragraph (long, mostly data/prose).
	paragraphs := splitParagraphs(text)
	if len(paragraphs) < 2 {
		return text, nil
	}

	// Find the longest paragraph(s) — likely context
	// Find the shortest paragraph near the end — likely the query
	queryIdx := -1
	contextIdx := -1
	maxLen := 0

	for i, p := range paragraphs {
		pLen := len(p)
		if pLen > maxLen {
			maxLen = pLen
			contextIdx = i
		}
	}

	// The query is usually the last short paragraph
	for i := len(paragraphs) - 1; i >= 0; i-- {
		p := strings.TrimSpace(paragraphs[i])
		if len(p) < 500 && (strings.Contains(p, "?") || isImperative(p)) {
			queryIdx = i
			break
		}
	}

	// Only reorder if context comes after query and context is significantly larger
	if queryIdx == -1 || contextIdx == -1 || contextIdx < queryIdx {
		return text, nil
	}
	if len(paragraphs[contextIdx]) < len(paragraphs[queryIdx])*3 {
		return text, nil // context isn't significantly larger
	}

	// Reorder: move context before query
	var reordered []string
	for i, p := range paragraphs {
		if i == contextIdx {
			continue // skip for now, will be placed earlier
		}
		if i == queryIdx {
			// Insert context before query
			reordered = append(reordered, paragraphs[contextIdx])
		}
		reordered = append(reordered, p)
	}

	result := strings.Join(reordered, "\n\n")
	tokens := EstimateTokens(paragraphs[contextIdx])
	return result, []string{fmt.Sprintf("Moved long context block (~%d tokens) before query (Anthropic: up to 30%% quality improvement)", tokens)}
}

// InjectQuoteGrounding adds a "find relevant quotes first" instruction for long-context prompts.
// Per Anthropic: asking Claude to quote relevant sections before answering grounds the response.
func InjectQuoteGrounding(text string, taskType TaskType) (string, []string) {
	// Only for analysis-type tasks with substantial context
	if taskType != TaskTypeAnalysis && taskType != TaskTypeGeneral {
		return text, nil
	}

	// Check if already has grounding instructions
	lower := strings.ToLower(text)
	if strings.Contains(lower, "<quotes>") || strings.Contains(lower, "quote") ||
		strings.Contains(lower, "cite") || strings.Contains(lower, "reference the") {
		return text, nil
	}

	// Only inject if prompt is long enough to benefit
	tokens := EstimateTokens(text)
	if tokens < 5000 {
		return text, nil
	}

	grounding := "\n\nBefore answering, find and quote the specific passages from the context above that are most relevant to this question. Place each quote in <quotes> tags. Then, grounding your answer in these quotes, provide your response."

	return text + grounding, []string{fmt.Sprintf("Injected quote-grounding instruction for long-context prompt (~%d tokens)", tokens)}
}

// VerifyCacheFriendlyOrder checks whether a prompt's structure is optimized for
// Anthropic's prompt caching (90% cost reduction, 85% latency reduction).
// Caching requires exact prefix matching — static content MUST precede dynamic content.
func VerifyCacheFriendlyOrder(text string) []LintResult {
	var results []LintResult

	// Identify static section positions (role, constraints, examples, output_format)
	staticTags := []string{"<role>", "<constraints>", "<examples>", "<output_format>"}
	// Identify dynamic section positions (instructions with {{...}}, context with runtime data)
	dynamicTags := []string{"<instructions>", "<context>"}

	firstStaticPos := -1
	lastDynamicPos := -1

	lower := strings.ToLower(text)

	for _, tag := range staticTags {
		if idx := strings.Index(lower, tag); idx != -1 {
			if firstStaticPos == -1 || idx < firstStaticPos {
				firstStaticPos = idx
			}
		}
	}

	for _, tag := range dynamicTags {
		if idx := strings.Index(lower, tag); idx != -1 {
			// Check if this section contains template variables (dynamic)
			endTag := strings.Replace(tag, "<", "</", 1)
			endIdx := strings.Index(lower[idx:], endTag)
			section := ""
			if endIdx != -1 {
				section = text[idx : idx+endIdx]
			} else {
				section = text[idx:]
			}

			hasDynamic := strings.Contains(section, "{{") || strings.Contains(section, "${")
			if hasDynamic && idx > lastDynamicPos {
				lastDynamicPos = idx
			}
		}
	}

	// Check if any dynamic section appears before a static section
	if lastDynamicPos != -1 && firstStaticPos != -1 && lastDynamicPos < firstStaticPos {
		lineNum := strings.Count(text[:lastDynamicPos], "\n") + 1
		results = append(results, LintResult{
			Line:        lineNum,
			Category:    "cache-unfriendly-order",
			Severity:    "warn",
			Original:    "(dynamic section before static section)",
			Suggestion:  "Move static sections (<role>, <constraints>, <examples>, <output_format>) BEFORE dynamic sections (<instructions>, <context>) for prompt caching. Caching requires exact prefix matching — static content first gives 90% cost reduction.",
			AutoFixable: false,
		})
	}

	// Check for template variables in early sections
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if (strings.Contains(line, "{{") || strings.Contains(line, "${")) && i < len(lines)/3 {
			// Dynamic content in first third of prompt — bad for caching
			results = append(results, LintResult{
				Line:        i + 1,
				Category:    "cache-unfriendly-variable",
				Severity:    "info",
				Original:    strings.TrimSpace(line),
				Suggestion:  "Template variable in the first third of the prompt breaks cache prefix matching. Move dynamic/variable content toward the end of the prompt.",
				AutoFixable: false,
			})
			break // only report once
		}
	}

	// Check for missing structure altogether (hard to cache unstructured prompts)
	if !strings.Contains(lower, "<") && EstimateTokens(text) > 1000 {
		results = append(results, LintResult{
			Line:        0,
			Category:    "cache-no-structure",
			Severity:    "info",
			Original:    "(no XML structure detected)",
			Suggestion:  "Prompt has no XML structure. Adding XML tags enables prompt caching — place stable content (<role>, <constraints>) at the top as a cacheable prefix.",
			AutoFixable: false,
		})
	}

	return results
}

// splitParagraphs splits text on double newlines
func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var paragraphs []string
	for _, p := range raw {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			paragraphs = append(paragraphs, trimmed)
		}
	}
	return paragraphs
}

// isImperative checks if a paragraph starts with an imperative verb
func isImperative(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	imperativeStarts := []string{
		"analyze ", "review ", "compare ", "evaluate ", "explain ",
		"create ", "write ", "implement ", "build ", "design ",
		"fix ", "debug ", "find ", "identify ", "list ",
		"describe ", "summarize ", "extract ", "determine ",
		"what ", "how ", "why ", "when ", "where ", "which ",
		"please ", "can you ", "could you ",
	}
	for _, prefix := range imperativeStarts {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}
