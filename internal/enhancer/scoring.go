package enhancer

import (
	"regexp"
	"strings"
)

// DimensionScore represents a single scoring dimension with its evaluation.
type DimensionScore struct {
	Name        string   `json:"name"`
	Score       int      `json:"score"`       // 0-100
	Weight      float64  `json:"weight"`      // sums to 1.0
	Grade       string   `json:"grade"`       // A/B/C/D/F
	Suggestions []string `json:"suggestions"`
}

// ScoreReport is a multi-dimensional prompt quality evaluation.
type ScoreReport struct {
	Overall    int              `json:"overall_score"` // 0-100 weighted
	Grade      string           `json:"overall_grade"` // A/B/C/D/F
	Dimensions []DimensionScore `json:"dimensions"`
}

// gradeForScore maps a 0-100 score to a letter grade.
func gradeForScore(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 65:
		return "C"
	case score >= 50:
		return "D"
	default:
		return "F"
	}
}

// Score produces a multi-dimensional ScoreReport for a prompt.
// It reuses existing AnalyzeResult booleans and lint findings to avoid duplicating detection.
func Score(text string, taskType TaskType, lints []LintResult, ar *AnalyzeResult) *ScoreReport {
	dims := []DimensionScore{
		scoreClarity(text, taskType, lints, ar),
		scoreSpecificity(text, taskType, lints, ar),
		scoreContextMotivation(text, taskType, lints, ar),
		scoreStructure(text, taskType, lints, ar),
		scoreExamples(text, taskType, lints, ar),
		scoreDocumentPlacement(text, taskType, lints, ar),
		scoreRoleDefinition(text, taskType, lints, ar),
		scoreTaskFocus(text, taskType, lints, ar),
		scoreFormatSpec(text, taskType, lints, ar),
		scoreTone(text, taskType, lints, ar),
	}

	// Compute weighted overall
	var weighted float64
	for _, d := range dims {
		weighted += float64(d.Score) * d.Weight
	}
	overall := int(weighted + 0.5) // round

	// Coherence bonus: reward well-rounded prompts that score ≥60 on multiple dimensions.
	// This helps CLI prompts that lack XML formalism but are otherwise well-written.
	// Only applies when the base score is ≥46 to avoid inflating truly poor prompts.
	aboveThreshold := 0
	for _, d := range dims {
		if d.Score >= 60 {
			aboveThreshold++
		}
	}
	if aboveThreshold >= 3 && overall >= 46 {
		overall += aboveThreshold * 3
	}

	if overall > 100 {
		overall = 100
	}
	if overall < 0 {
		overall = 0
	}

	return &ScoreReport{
		Overall:    overall,
		Grade:      gradeForScore(overall),
		Dimensions: dims,
	}
}

// --- helpers ---

func countLintCategory(lints []LintResult, category string) int {
	n := 0
	for _, l := range lints {
		if l.Category == category {
			n++
		}
	}
	return n
}

func hasLintCategory(lints []LintResult, category string) bool {
	return countLintCategory(lints, category) > 0
}

var numericConstraintPattern = regexp.MustCompile(`\b\d+\s*(items?|bullets?|sentences?|words?|lines?|steps?|paragraphs?|characters?|tokens?|minutes?|seconds?|points?|examples?|max|min|limit|at\s+most|at\s+least)\b`)

var rolePattern = regexp.MustCompile(`(?i)(you\s+are\s+(a|an)\s+|<role>)`)

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- dimension scorers ---

func scoreClarity(text string, _ TaskType, lints []LintResult, ar *AnalyzeResult) DimensionScore {
	score := 50 // baseline
	var suggestions []string

	wc := ar.WordCount

	// Word count contribution
	switch {
	case wc >= 50:
		score += 25
	case wc >= 20:
		score += 15
	case wc >= 10:
		score += 5
	default:
		suggestions = append(suggestions, "Prompt is very short — add detail for consistent results")
	}

	// Vague phrase penalty
	vagueCount := 0
	lower := strings.ToLower(text)
	for pattern := range vagueReplacements {
		if strings.Contains(lower, pattern) {
			vagueCount++
		}
	}
	score -= vagueCount * 5
	if vagueCount > 0 {
		suggestions = append(suggestions, "Replace vague phrases with specific instructions")
	}

	// Vague quantifier lint penalty
	vqCount := countLintCategory(lints, "vague-quantifier")
	score -= vqCount * 5
	if vqCount > 0 {
		suggestions = append(suggestions, "Replace vague quantifiers ('several', 'a few') with specific numbers")
	}

	// Bonus for numeric constraints (shows precision)
	if numericConstraintPattern.MatchString(text) {
		score += 15
	}

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Clarity",
		Score:       score,
		Weight:      0.15,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}

func scoreSpecificity(text string, _ TaskType, lints []LintResult, ar *AnalyzeResult) DimensionScore {
	score := 50
	var suggestions []string

	// Numeric constraints
	numericMatches := numericConstraintPattern.FindAllString(text, -1)
	score += len(numericMatches) * 10
	if len(numericMatches) == 0 {
		suggestions = append(suggestions, "Add numeric constraints — '5 bullets, each under 15 words' beats 'be concise'")
	}

	// Word count as proxy for detail
	if ar.WordCount >= 50 {
		score += 15
	} else if ar.WordCount >= 20 {
		score += 8
	}

	// Vague phrase penalty
	lower := strings.ToLower(text)
	vagueCount := 0
	for pattern := range vagueReplacements {
		if strings.Contains(lower, pattern) {
			vagueCount++
		}
	}
	score -= vagueCount * 10

	// Format specification bonus
	if ar.HasFormat {
		score += 15
	}

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Specificity",
		Score:       score,
		Weight:      0.12,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}

func scoreContextMotivation(text string, _ TaskType, lints []LintResult, ar *AnalyzeResult) DimensionScore {
	score := 30
	var suggestions []string

	if ar.HasContext {
		score += 30
	} else {
		suggestions = append(suggestions, "Add a <context> section with relevant background")
	}

	// Motivation markers
	if motivationMarkers.MatchString(text) {
		score += 25
	}

	// Unmotivated rule penalty
	umCount := countLintCategory(lints, "unmotivated-rule")
	score -= umCount * 8
	if umCount > 0 {
		suggestions = append(suggestions, "Add 'because...' clauses to directives — motivated rules improve compliance")
	}

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Context & Motivation",
		Score:       score,
		Weight:      0.10,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}

func scoreStructure(text string, _ TaskType, lints []LintResult, ar *AnalyzeResult) DimensionScore {
	score := 40
	var suggestions []string

	if ar.HasXML {
		score += 40
	} else {
		suggestions = append(suggestions, "Add XML structure tags — Claude is specifically trained to recognize them")
	}

	// Count distinct XML tags
	lower := strings.ToLower(text)
	xmlTags := []string{"<role>", "<instructions>", "<context>", "<constraints>", "<examples>", "<output_format>", "<verification>"}
	tagCount := 0
	for _, tag := range xmlTags {
		if strings.Contains(lower, tag) {
			tagCount++
		}
	}
	score += tagCount * 7

	// Paragraph separation (shows organized thought)
	paragraphs := strings.Count(text, "\n\n")
	if paragraphs >= 2 {
		score += 10
	}

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Structure",
		Score:       score,
		Weight:      0.15,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}

func scoreExamples(text string, _ TaskType, lints []LintResult, _ *AnalyzeResult) DimensionScore {
	score := 20
	var suggestions []string

	lower := strings.ToLower(text)

	// Count <example> tags
	exampleCount := strings.Count(lower, "<example")
	if exampleCount >= 3 && exampleCount <= 5 {
		score += 60 // ideal range
	} else if exampleCount >= 1 {
		score += 30
	}

	// General example mention without tags
	if exampleCount == 0 && (strings.Contains(lower, "example") || strings.Contains(lower, "e.g.") || strings.Contains(lower, "for instance")) {
		score += 15
	}

	if exampleCount == 0 {
		suggestions = append(suggestions, "Include 3-5 examples in <example> tags — few-shot examples dramatically improve accuracy")
	}

	// Example quality lints
	eqCount := countLintCategory(lints, "example-quality")
	if eqCount > 0 {
		score -= 10
	}

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Examples",
		Score:       score,
		Weight:      0.10,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}

func scoreDocumentPlacement(text string, _ TaskType, lints []LintResult, ar *AnalyzeResult) DimensionScore {
	score := 60 // default OK if not applicable
	var suggestions []string

	tokens := ar.EstimatedTokens

	// For short prompts, placement is less important
	if tokens < 1000 {
		score = 70
	}

	// Cache-unfriendly lints
	cacheUnfriendly := 0
	for _, l := range lints {
		if strings.HasPrefix(l.Category, "cache-") {
			cacheUnfriendly++
		}
	}
	score -= cacheUnfriendly * 15
	if cacheUnfriendly > 0 {
		suggestions = append(suggestions, "Reorder for prompt caching — static content before dynamic content gives 90% cost reduction")
	}

	// Bonus for having XML structure (enables caching)
	if ar.HasXML {
		score += 15
	}

	// Long prompt without structure is bad for placement
	if tokens > 5000 && !ar.HasXML {
		score -= 20
		suggestions = append(suggestions, "Long prompt without XML structure — add tags to enable caching and proper context placement")
	}

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Document Placement",
		Score:       score,
		Weight:      0.08,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}

func scoreRoleDefinition(text string, _ TaskType, _ []LintResult, _ *AnalyzeResult) DimensionScore {
	score := 35
	var suggestions []string

	lower := strings.ToLower(text)

	if strings.Contains(lower, "<role>") {
		score += 50
	}

	if rolePattern.MatchString(text) {
		score += 25
	}

	// Expert/specialist persona
	if strings.Contains(lower, "expert") || strings.Contains(lower, "specialist") ||
		strings.Contains(lower, "senior") || strings.Contains(lower, "experienced") {
		score += 10
	}

	if score <= 35 {
		suggestions = append(suggestions, "Add a role definition — 'You are an expert...' sets Claude's expertise level")
	}

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Role Definition",
		Score:       score,
		Weight:      0.08,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}

func scoreTaskFocus(text string, _ TaskType, lints []LintResult, _ *AnalyzeResult) DimensionScore {
	score := 60
	var suggestions []string

	// Decomposition needed = too many tasks
	if hasLintCategory(lints, "decomposition-needed") {
		score -= 25
		suggestions = append(suggestions, "Prompt contains multiple distinct tasks — consider splitting for better results")
	}

	// Over-specification
	if hasLintCategory(lints, "over-specification") {
		score -= 15
		suggestions = append(suggestions, "Too many numbered steps — describe the desired outcome instead")
	}

	// Imperative verbs count (moderate number is good)
	verbs := imperativeVerbPattern.FindAllString(text, -1)
	switch {
	case len(verbs) == 0:
		score -= 10
		suggestions = append(suggestions, "Add clear action verbs — tell Claude exactly what to do")
	case len(verbs) >= 1 && len(verbs) <= 3:
		score += 20 // focused
	case len(verbs) > 3:
		score += 5 // some, but potentially unfocused
	}

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Task Focus",
		Score:       score,
		Weight:      0.07,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}

func scoreFormatSpec(text string, _ TaskType, _ []LintResult, ar *AnalyzeResult) DimensionScore {
	score := 20
	var suggestions []string

	lower := strings.ToLower(text)

	if ar.HasFormat {
		score += 35
	}

	if strings.Contains(lower, "<output_format>") {
		score += 25
	}

	// Quantified format ("5 bullets", "3 paragraphs")
	if numericConstraintPattern.MatchString(text) {
		score += 15
	}

	if !ar.HasFormat {
		suggestions = append(suggestions, "Specify desired output format — use positive format instructions")
	}

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Format Specification",
		Score:       score,
		Weight:      0.08,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}

func scoreTone(text string, _ TaskType, lints []LintResult, ar *AnalyzeResult) DimensionScore {
	score := 80 // default good tone
	var suggestions []string

	if ar.HasAggressiveCaps {
		score -= 25
		suggestions = append(suggestions, "Downgrade ALL-CAPS emphasis — Claude 4.x overtriggers on aggressive language")
	}

	if ar.HasNegativeFrames {
		score -= 20
		suggestions = append(suggestions, "Reframe negative instructions as positive — Claude 4.x can reverse-psychology on negatives")
	}

	// Overtrigger phrases
	otCount := countLintCategory(lints, "overtrigger-phrase")
	score -= otCount * 15
	if otCount > 0 {
		suggestions = append(suggestions, "Remove aggressive 'CRITICAL: You MUST' prefixes — use calm, direct instructions")
	}

	// Negative framing lints (beyond the bool)
	nfCount := countLintCategory(lints, "negative-framing")
	score -= nfCount * 8

	score = clamp(score, 0, 100)
	return DimensionScore{
		Name:        "Tone",
		Score:       score,
		Weight:      0.07,
		Grade:       gradeForScore(score),
		Suggestions: suggestions,
	}
}
