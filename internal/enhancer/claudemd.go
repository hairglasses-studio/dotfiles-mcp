package enhancer

import (
	"fmt"
	"os"
	"strings"
)

// ClaudeMDResult represents a finding from CLAUDE.md health check
type ClaudeMDResult struct {
	Category   string `json:"category"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// CheckClaudeMD performs a health check on a CLAUDE.md file.
// It checks for common issues: excessive length, inline code, style guide content,
// overtrigger language, and missing section headers.
func CheckClaudeMD(path string) ([]ClaudeMDResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	text := string(data)
	lines := strings.Split(text, "\n")
	var results []ClaudeMDResult

	// Check 1: Excessive length (>200 lines)
	if len(lines) > 200 {
		results = append(results, ClaudeMDResult{
			Category:   "excessive-length",
			Severity:   "warn",
			Message:    fmt.Sprintf("CLAUDE.md is %d lines (recommended: <200)", len(lines)),
			Suggestion: "Long CLAUDE.md files consume context window on every conversation. Move detailed guides to separate docs and reference them.",
		})
	}

	// Check 2: Inline code snippets (full function bodies, not just backtick references)
	codeBlockCount := strings.Count(text, "```")
	if codeBlockCount/2 > 3 {
		results = append(results, ClaudeMDResult{
			Category:   "inline-code",
			Severity:   "info",
			Message:    fmt.Sprintf("Contains %d code blocks", codeBlockCount/2),
			Suggestion: "CLAUDE.md with many code blocks wastes context. Claude can read source files directly — reference file paths instead of inlining code.",
		})
	}

	// Check 3: Style guide content (formatting rules that belong in linter config)
	lower := strings.ToLower(text)
	styleIndicators := []string{
		"indent with", "use tabs", "use spaces", "line length",
		"naming convention", "camelcase", "snake_case", "pascal case",
		"import order", "sort imports", "blank line",
	}
	styleCount := 0
	for _, indicator := range styleIndicators {
		if strings.Contains(lower, indicator) {
			styleCount++
		}
	}
	if styleCount >= 3 {
		results = append(results, ClaudeMDResult{
			Category:   "style-guide-content",
			Severity:   "info",
			Message:    fmt.Sprintf("Contains %d style/formatting directives", styleCount),
			Suggestion: "Style rules belong in linter config (eslint, gofmt, black), not CLAUDE.md. Claude follows the linter automatically — these directives waste context.",
		})
	}

	// Check 4: Overtrigger language
	overtriggerCount := 0
	for _, line := range lines {
		if overtriggerLintPattern.MatchString(line) {
			overtriggerCount++
		}
	}
	if overtriggerCount > 0 {
		results = append(results, ClaudeMDResult{
			Category:   "overtrigger-language",
			Severity:   "warn",
			Message:    fmt.Sprintf("Contains %d overtrigger phrase(s) (e.g., 'CRITICAL: You MUST')", overtriggerCount),
			Suggestion: "Claude 4.x overtriggers on aggressive language in CLAUDE.md. Replace 'CRITICAL: You MUST always' with calm directives like 'Use X when...'.",
		})
	}

	// Check 5: Missing section headers
	hasHeaders := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			hasHeaders = true
			break
		}
	}
	if !hasHeaders && len(lines) > 20 {
		results = append(results, ClaudeMDResult{
			Category:   "missing-headers",
			Severity:   "info",
			Message:    "No markdown headers found",
			Suggestion: "Add section headers (## Code Standards, ## Key Patterns, etc.) to help Claude navigate and selectively attend to relevant sections.",
		})
	}

	// Check 6: Aggressive ALL-CAPS emphasis
	capsCount := 0
	for _, line := range lines {
		matches := aggressiveCapsPattern.FindAllString(line, -1)
		for _, m := range matches {
			if !acronymWhitelist[m] {
				capsCount++
			}
		}
	}
	if capsCount > 3 {
		results = append(results, ClaudeMDResult{
			Category:   "aggressive-caps",
			Severity:   "info",
			Message:    fmt.Sprintf("Contains %d ALL-CAPS emphasis words", capsCount),
			Suggestion: "Excessive ALL-CAPS in CLAUDE.md causes Claude 4.x to overtrigger. Use normal case — it's equally effective.",
		})
	}

	return results, nil
}
