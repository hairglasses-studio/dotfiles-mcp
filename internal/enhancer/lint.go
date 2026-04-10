package enhancer

import (
	"fmt"
	"regexp"
	"strings"
)

// LintResult represents a single lint finding
type LintResult struct {
	Line        int    `json:"line"`
	Category    string `json:"category"`
	Severity    string `json:"severity"` // "error", "warn", "info"
	Original    string `json:"original"`
	Suggestion  string `json:"suggestion"`
	AutoFixable bool   `json:"auto_fixable"`
}

// directivePattern detects imperative/directive sentences
var directivePattern = regexp.MustCompile(
	`(?mi)^(always|never|do\s+not|don't|must|should|ensure|make\s+sure|be\s+sure)\s+.+[\.\n]`,
)

// motivationMarkers indicate the directive explains WHY
var motivationMarkers = regexp.MustCompile(
	`(?i)\b(because|since|so\s+that|in\s+order\s+to|to\s+(ensure|prevent|avoid|enable|allow|improve|reduce|maintain)|this\s+(helps|ensures|prevents|allows|enables|improves)|otherwise|the\s+reason|as\s+this|which\s+(helps|ensures|prevents))\b`,
)

// Lint runs all lint checks on a prompt and returns findings.
// This is deeper than Analyze — it returns per-line actionable findings.
func Lint(text string) []LintResult {
	var results []LintResult
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check for unmotivated directives
		if r := checkUnmotivatedRule(i+1, trimmed); r != nil {
			results = append(results, *r)
		}

		// Check for negative framing (that isn't in our reframing table)
		if r := checkNegativeFraming(i+1, trimmed); r != nil {
			results = append(results, *r)
		}

		// Check for aggressive emphasis
		if r := checkAggressiveEmphasis(i+1, trimmed); r != nil {
			results = append(results, *r)
		}

		// Check for vague quantifiers
		if r := checkVagueQuantifiers(i+1, trimmed); r != nil {
			results = append(results, *r)
		}

		// Check for overtrigger phrases
		if r := checkOvertriggerPhrase(i+1, trimmed); r != nil {
			results = append(results, *r)
		}

		// Check for injection vulnerabilities
		if r := checkInjectionVulnerability(i+1, trimmed); r != nil {
			results = append(results, *r)
		}

		// Check for thinking mode redundancy
		if r := checkThinkingMode(i+1, trimmed); r != nil {
			results = append(results, *r)
		}
	}

	// Whole-prompt checks
	results = append(results, checkOverSpecification(text)...)
	results = append(results, checkDecomposition(text)...)
	results = append(results, checkExampleQuality(text)...)
	results = append(results, checkCompactionReadiness(text)...)

	return results
}

func checkUnmotivatedRule(lineNum int, line string) *LintResult {
	if !directivePattern.MatchString(line) {
		return nil
	}
	if motivationMarkers.MatchString(line) {
		return nil // has motivation
	}
	// Skip very short lines (probably not standalone rules)
	if len(strings.Fields(line)) < 4 {
		return nil
	}

	return &LintResult{
		Line:        lineNum,
		Category:    "unmotivated-rule",
		Severity:    "info",
		Original:    line,
		Suggestion:  "Add a 'because...' clause — motivated instructions improve compliance. Per Anthropic: Claude generalizes better from understanding the purpose.",
		AutoFixable: false,
	}
}

func checkNegativeFraming(lineNum int, line string) *LintResult {
	if !negativePattern.MatchString(line) {
		return nil
	}
	// Skip safety-critical negatives
	if safetyNegativePattern.MatchString(line) {
		return nil
	}
	// Skip if it's already in our reframing table (handled by the enhancer)
	lower := strings.ToLower(line)
	for neg := range negativeReframings {
		if strings.Contains(lower, neg) {
			return nil
		}
	}

	return &LintResult{
		Line:       lineNum,
		Category:   "negative-framing",
		Severity:   "warn",
		Original:   line,
		Suggestion: "Reframe as a positive instruction — tell Claude what to do, not what to avoid. Per Anthropic: negative framing can cause reverse psychology with Claude 4.x.",
		AutoFixable: false,
	}
}

func checkAggressiveEmphasis(lineNum int, line string) *LintResult {
	matches := aggressiveCapsPattern.FindAllString(line, -1)
	if len(matches) == 0 {
		return nil
	}
	// Filter out acronyms
	var real []string
	for _, m := range matches {
		if !acronymWhitelist[m] {
			real = append(real, m)
		}
	}
	if len(real) == 0 {
		return nil
	}

	return &LintResult{
		Line:        lineNum,
		Category:    "aggressive-emphasis",
		Severity:    "info",
		Original:    line,
		Suggestion:  fmt.Sprintf("Downgrade %s to normal case — Claude 4.x overtriggers on aggressive ALL-CAPS language.", strings.Join(real, ", ")),
		AutoFixable: true,
	}
}

var vagueQuantifierPattern = regexp.MustCompile(`(?i)\b(a few|some|several|many|a lot|a bit|enough|various|appropriate|suitable|proper|good|nice|decent)\b`)

func checkVagueQuantifiers(lineNum int, line string) *LintResult {
	matches := vagueQuantifierPattern.FindAllString(line, -1)
	if len(matches) == 0 {
		return nil
	}

	return &LintResult{
		Line:        lineNum,
		Category:    "vague-quantifier",
		Severity:    "info",
		Original:    line,
		Suggestion:  fmt.Sprintf("Replace vague quantifier(s) %q with specific numbers — '3-5 items' is better than 'several items'.", matches),
		AutoFixable: false,
	}
}

// --- P0: Overtrigger phrase detection ---

var overtriggerLintPattern = regexp.MustCompile(
	`(?i)(CRITICAL|IMPORTANT|REQUIRED|WARNING)\s*[:!]\s*(You\s+)?(MUST|ALWAYS|NEVER|SHOULD)\s+`,
)

func checkOvertriggerPhrase(lineNum int, line string) *LintResult {
	if !overtriggerLintPattern.MatchString(line) {
		return nil
	}
	return &LintResult{
		Line:        lineNum,
		Category:    "overtrigger-phrase",
		Severity:    "warn",
		Original:    line,
		Suggestion:  "Remove aggressive prefix (e.g., 'CRITICAL: You MUST') — Claude 4.x overtriggers on these anti-laziness patterns. Use calm, direct instructions instead.",
		AutoFixable: true,
	}
}

// --- P1: Over-specification detection ---

var numberedStepPattern = regexp.MustCompile(`(?m)^\s*\d+[\.\)]\s+`)

func checkOverSpecification(text string) []LintResult {
	steps := numberedStepPattern.FindAllStringIndex(text, -1)
	if len(steps) <= 5 {
		return nil
	}

	return []LintResult{{
		Line:        0,
		Category:    "over-specification",
		Severity:    "info",
		Original:    fmt.Sprintf("(%d numbered steps detected)", len(steps)),
		Suggestion:  fmt.Sprintf("Prompt has %d numbered steps. Spotify found Claude works better with end-state descriptions than step-by-step. Consider describing the desired outcome instead.", len(steps)),
		AutoFixable: false,
	}}
}

// --- P1: Prompt decomposition suggestion ---

var imperativeVerbPattern = regexp.MustCompile(`(?i)\b(create|build|implement|write|fix|debug|refactor|analyze|review|design|test|deploy|configure|set up|migrate|update|delete|remove)\b`)

func checkDecomposition(text string) []LintResult {
	matches := imperativeVerbPattern.FindAllString(text, -1)
	if len(matches) < 3 {
		return nil
	}

	// Deduplicate verbs
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		lower := strings.ToLower(m)
		if !seen[lower] {
			seen[lower] = true
			unique = append(unique, lower)
		}
	}
	if len(unique) < 3 {
		return nil
	}

	return []LintResult{{
		Line:        0,
		Category:    "decomposition-needed",
		Severity:    "info",
		Original:    fmt.Sprintf("(%d distinct imperative verbs: %s)", len(unique), strings.Join(unique, ", ")),
		Suggestion:  "Prompt contains multiple distinct tasks. Consider splitting into separate prompts for better results — multi-task prompts dilute Claude's attention.",
		AutoFixable: false,
	}}
}

// --- P1: Injection vulnerability scanning ---

var injectionPattern = regexp.MustCompile(`(\$\{[^}]*\}|\{\{[^}]*\}\})`)
var untrustedVarNames = regexp.MustCompile(`(?i)(user[_\s]?input|user[_\s]?query|user[_\s]?message|user[_\s]?data|raw[_\s]?input|untrusted|external|request[_\s]?body|form[_\s]?data|query[_\s]?string|params?)`)

func checkInjectionVulnerability(lineNum int, line string) *LintResult {
	matches := injectionPattern.FindAllString(line, -1)
	if len(matches) == 0 {
		return nil
	}

	for _, m := range matches {
		if untrustedVarNames.MatchString(m) {
			return &LintResult{
				Line:        lineNum,
				Category:    "injection-risk",
				Severity:    "error",
				Original:    line,
				Suggestion:  fmt.Sprintf("Variable %s may contain untrusted user input. Wrap in XML tags (e.g., <user_input>%s</user_input>) and add instructions to treat content as data, not instructions.", m, m),
				AutoFixable: false,
			}
		}
	}
	return nil
}

// --- P2: Thinking mode detection ---

var thinkingModePattern = regexp.MustCompile(`(?i)\b(think\s+step\s+by\s+step|let'?s\s+think|chain\s+of\s+thought|reason\s+through\s+this|think\s+carefully)\b`)

func checkThinkingMode(lineNum int, line string) *LintResult {
	if !thinkingModePattern.MatchString(line) {
		return nil
	}
	return &LintResult{
		Line:        lineNum,
		Category:    "thinking-mode-redundant",
		Severity:    "info",
		Original:    line,
		Suggestion:  "Claude 4.x has adaptive extended thinking built-in. 'Think step by step' is redundant and may waste tokens. Remove it or use the effort parameter instead.",
		AutoFixable: false,
	}
}

// --- P2: Example quality scoring ---

var exampleTagPattern = regexp.MustCompile(`(?i)<example[\s>]`)

func checkExampleQuality(text string) []LintResult {
	matches := exampleTagPattern.FindAllStringIndex(text, -1)
	count := len(matches)
	if count == 0 {
		return nil // no examples to score
	}

	if count < 3 {
		return []LintResult{{
			Line:        0,
			Category:    "example-quality",
			Severity:    "info",
			Original:    fmt.Sprintf("(%d example(s) found)", count),
			Suggestion:  fmt.Sprintf("Only %d example(s) found. 3-5 examples is ideal for few-shot prompting — Claude needs enough variety to generalize the pattern.", count),
			AutoFixable: false,
		}}
	}
	if count > 5 {
		return []LintResult{{
			Line:        0,
			Category:    "example-quality",
			Severity:    "info",
			Original:    fmt.Sprintf("(%d examples found)", count),
			Suggestion:  fmt.Sprintf("%d examples found. More than 5 examples has diminishing returns and wastes context window. Consider trimming to the 3-5 most diverse examples.", count),
			AutoFixable: false,
		}}
	}
	return nil
}

// --- P2: Compaction readiness check ---

func checkCompactionReadiness(text string) []LintResult {
	tokens := EstimateTokens(text)
	if tokens < 50_000 {
		return nil
	}

	lower := strings.ToLower(text)
	hasCompactionGuidance := strings.Contains(lower, "compaction") ||
		strings.Contains(lower, "compress") ||
		strings.Contains(lower, "summarize prior") ||
		strings.Contains(lower, "context window")

	if hasCompactionGuidance {
		return nil
	}

	return []LintResult{{
		Line:        0,
		Category:    "compaction-readiness",
		Severity:    "warn",
		Original:    fmt.Sprintf("(~%d tokens estimated)", tokens),
		Suggestion:  "Prompt exceeds 50K tokens. Add compaction guidance (e.g., which sections to prioritize, what can be summarized) so Claude's automatic context management preserves critical information.",
		AutoFixable: false,
	}}
}
