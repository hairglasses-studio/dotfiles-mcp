// Package enhancer provides deterministic prompt enhancement using XML structuring,
// specificity improvements, context reordering, and task-type-aware formatting.
// No external LLM calls — pure Go string manipulation.
//
// Based on Anthropic's official prompt engineering best practices:
// - XML tags for structure (Claude is specifically trained to recognize them)
// - Context placement (long context before query for 20K+ token prompts)
// - Positive framing over negative (Claude 4.x responds better to "do X" than "don't Y")
// - Aggressive language downgrading (MUST/CRITICAL → normal case for Claude 4.x)
// - Few-shot example wrapping in <examples><example> tags
// - Self-check injection for code/math/analysis tasks
// - Preamble suppression for direct output
// - Format enforcement for JSON/YAML/code output requests
// - Over-tagging prevention for short single-purpose prompts
package enhancer

import (
	"fmt"
	"regexp"
	"strings"
)

// EnhanceResult holds the output of the enhancement pipeline
type EnhanceResult struct {
	Original        string   `json:"original"`
	Enhanced        string   `json:"enhanced"`
	TaskType        TaskType `json:"task_type"`
	StagesRun       []string `json:"stages_run"`
	Improvements    []string `json:"improvements"`
	EstimatedTokens int      `json:"estimated_tokens"`
	CostTier        string   `json:"cost_tier"`
	Source          string   `json:"source,omitempty"` // "local", "llm", "llm_cached", "local_fallback", "error"
}

// AnalyzeResult holds prompt quality analysis
type AnalyzeResult struct {
	Score              int          `json:"score"`
	ScoreReport        *ScoreReport `json:"score_report"`
	Suggestions        []string     `json:"suggestions"`
	HasXML             bool         `json:"has_xml_structure"`
	HasExamples        bool         `json:"has_examples"`
	HasContext          bool         `json:"has_context"`
	HasFormat          bool         `json:"has_output_format"`
	HasNegativeFrames  bool         `json:"has_negative_framing"`
	HasAggressiveCaps  bool         `json:"has_aggressive_caps"`
	WordCount          int          `json:"word_count"`
	TaskType           TaskType     `json:"task_type"`
	EstimatedTokens    int          `json:"estimated_tokens"`
	CostTier           string       `json:"cost_tier"`
	RecommendedEffort  string       `json:"recommended_effort"`
}

// EnhanceWithConfig runs the full enhancement pipeline with optional project config.
func EnhanceWithConfig(raw string, taskType TaskType, cfg Config) EnhanceResult {
	if taskType == "" {
		if cfg.DefaultTaskType != "" {
			taskType = ValidTaskType(cfg.DefaultTaskType)
		}
		if taskType == "" {
			taskType = Classify(raw)
		}
	}

	result := EnhanceResult{
		Original: raw,
		TaskType: taskType,
	}

	text := raw
	var imps []string

	// Stage 0: Apply config rules (prepend/append based on pattern matches)
	if len(cfg.Rules) > 0 {
		text, imps = cfg.ApplyRules(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "config_rules")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 1: Specificity — replace vague phrases with concrete instructions
	if !cfg.IsStageDisabled("specificity") {
		text, imps = improveSpecificity(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "specificity")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 2: Positive reframing — rewrite known negative patterns first
	if !cfg.IsStageDisabled("positive_reframe") {
		text, imps = reframeNegatives(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "positive_reframe")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 3: Tone — downgrade remaining aggressive ALL-CAPS for Claude 4.x
	if !cfg.IsStageDisabled("tone_downgrade") {
		text, imps = downgradeTone(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "tone_downgrade")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 4: Overtrigger rewrite — soften aggressive anti-laziness phrases for Claude 4.x
	if !cfg.IsStageDisabled("overtrigger_rewrite") {
		text, imps = rewriteOvertriggerPhrases(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "overtrigger_rewrite")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 5: Example detection — wrap bare Input/Output pairs in <example> tags
	if !cfg.IsStageDisabled("examples") {
		text, imps = DetectAndWrapExamples(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "example_wrapping")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 6: Structure — wrap in XML tags based on task type
	if !cfg.IsStageDisabled("structure") {
		text, imps = addStructure(text, taskType)
		result.StagesRun = append(result.StagesRun, "structure")
		result.Improvements = append(result.Improvements, imps...)
	}

	// Stage 7: Long-context reordering — move bulk context before query
	if !cfg.IsStageDisabled("context_reorder") {
		text, imps = ReorderLongContext(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "context_reorder")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 8: Format enforcement — detect output format requests
	if !cfg.IsStageDisabled("format_enforcement") {
		text, imps = enforceOutputFormat(text)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "format_enforcement")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 9: Quote grounding — inject "find quotes first" for long-context analysis
	if !cfg.IsStageDisabled("quote_grounding") {
		text, imps = InjectQuoteGrounding(text, taskType)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "quote_grounding")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 10: Self-check — inject verification for code/math/analysis
	if !cfg.IsStageDisabled("self_check") {
		text, imps = injectSelfCheck(text, taskType)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "self_check")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 11: Overengineering guard — prevent unnecessary abstractions (code tasks only)
	if !cfg.IsStageDisabled("overengineering_guard") {
		text, imps = injectOverengineeringGuard(text, taskType)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "overengineering_guard")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Stage 12: Preamble suppression — add direct response instruction
	if !cfg.IsStageDisabled("preamble_suppression") {
		text, imps = suppressPreamble(text, taskType)
		if len(imps) > 0 {
			result.StagesRun = append(result.StagesRun, "preamble_suppression")
			result.Improvements = append(result.Improvements, imps...)
		}
	}

	// Prepend config preamble if set
	if cfg.Preamble != "" {
		text = cfg.Preamble + "\n\n" + text
		result.Improvements = append(result.Improvements, "Prepended project-specific preamble from config")
	}

	result.Enhanced = text

	// Populate token estimate and cost tier
	result.EstimatedTokens = EstimateTokens(text)
	result.CostTier = costTierForTokens(result.EstimatedTokens)

	return result
}

// Enhance runs the full enhancement pipeline on a raw prompt (no config).
func Enhance(raw string, taskType TaskType) EnhanceResult {
	return EnhanceWithConfig(raw, taskType, Config{})
}

// Analyze scores a prompt and returns improvement suggestions
func Analyze(prompt string) AnalyzeResult {
	lower := strings.ToLower(prompt)
	words := strings.Fields(prompt)
	taskType := Classify(prompt)

	result := AnalyzeResult{
		WordCount: len(words),
		TaskType:  taskType,
	}

	// Check for existing structure
	result.HasXML = strings.Contains(prompt, "<") && strings.Contains(prompt, ">") &&
		(strings.Contains(lower, "<instructions") || strings.Contains(lower, "<context") ||
			strings.Contains(lower, "<role") || strings.Contains(lower, "<constraints"))
	result.HasExamples = strings.Contains(lower, "example") || strings.Contains(lower, "<example")
	result.HasContext = strings.Contains(lower, "context") || strings.Contains(lower, "<context")
	result.HasFormat = strings.Contains(lower, "format") || strings.Contains(lower, "<output")
	result.HasNegativeFrames = negativePattern.MatchString(prompt)
	result.HasAggressiveCaps = aggressiveCapsPattern.MatchString(prompt)

	// Count vague phrases for suggestions
	vagueCount := 0
	for pattern := range vagueReplacements {
		if strings.Contains(lower, pattern) {
			vagueCount++
		}
	}

	// Suggestions
	if !result.HasXML {
		result.Suggestions = append(result.Suggestions, "Add XML structure tags (<role>, <instructions>, <constraints>) — Claude is specifically trained to recognize XML as prompt structure")
	}
	if !result.HasExamples {
		result.Suggestions = append(result.Suggestions, "Include 3-5 examples in <examples><example> tags — Claude replicates formatting details from examples")
	}
	if !result.HasContext {
		result.Suggestions = append(result.Suggestions, "Add a <context> section with relevant background — place long context BEFORE the query for best results")
	}
	if !result.HasFormat {
		result.Suggestions = append(result.Suggestions, "Specify desired output format in an <output_format> section — use positive format instructions ('write in prose') not negative ('don't use bullets')")
	}
	if len(words) < 20 {
		result.Suggestions = append(result.Suggestions, "Add more detail — prompts under 20 words produce inconsistent results. Quantify constraints: '5 bullets, each under 15 words' instead of 'be concise'")
	}
	if vagueCount > 0 {
		result.Suggestions = append(result.Suggestions, fmt.Sprintf("Replace %d vague phrases with specific instructions (e.g., 'format nicely' → 'Format using markdown with headers and code blocks')", vagueCount))
	}
	if result.HasNegativeFrames {
		result.Suggestions = append(result.Suggestions, "Reframe negative instructions as positive — 'Write in flowing prose paragraphs' works better than 'NEVER use bullet points'. Claude 4.x can exhibit reverse psychology with heavy negative framing")
	}
	if result.HasAggressiveCaps {
		result.Suggestions = append(result.Suggestions, "Downgrade ALL-CAPS emphasis — Claude 4.x overtriggers on aggressive language like CRITICAL/MUST/IMPORTANT. Normal case is equally effective")
	}

	// Populate token estimate, cost tier, and recommended effort
	result.EstimatedTokens = EstimateTokens(prompt)
	result.CostTier = costTierForTokens(result.EstimatedTokens)
	result.RecommendedEffort = recommendEffort(prompt, taskType)

	// Multi-dimensional scoring
	allLints := Lint(prompt)
	allLints = append(allLints, VerifyCacheFriendlyOrder(prompt)...)
	report := Score(prompt, taskType, allLints, &result)
	result.ScoreReport = report

	// Derive legacy score from overall
	legacyScore := report.Overall / 10
	if legacyScore < 1 {
		legacyScore = 1
	}
	if legacyScore > 10 {
		legacyScore = 10
	}
	result.Score = legacyScore

	// Task-specific suggestions
	switch taskType {
	case TaskTypeCode:
		if !strings.Contains(lower, "language") && !strings.Contains(lower, "go") &&
			!strings.Contains(lower, "python") && !strings.Contains(lower, "typescript") {
			result.Suggestions = append(result.Suggestions, "Specify the programming language")
		}
	case TaskTypeTroubleshooting:
		if !strings.Contains(lower, "error") && !strings.Contains(lower, "symptom") {
			result.Suggestions = append(result.Suggestions, "Include the exact error message or symptoms")
		}
	case TaskTypeAnalysis:
		if !strings.Contains(lower, "criteria") && !strings.Contains(lower, "focus") {
			result.Suggestions = append(result.Suggestions, "Specify evaluation criteria or focus areas")
		}
	}

	return result
}

// WrapWithExamples wraps a prompt and examples into proper XML few-shot format
func WrapWithExamples(prompt string, examples []string) string {
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n<examples>\n")
	for i, ex := range examples {
		fmt.Fprintf(&b, "<example index=\"%d\">\n%s\n</example>\n", i+1, strings.TrimSpace(ex))
	}
	b.WriteString("</examples>")
	return b.String()
}

// --- Stage 1: Specificity ---

var vagueReplacements = map[string]string{
	"format nicely":       "Format using markdown with headers and code blocks",
	"make it good":        "Ensure correctness, clarity, and completeness",
	"make it better":      "Improve clarity, reduce redundancy, and strengthen specificity",
	"clean it up":         "Refactor for readability: consistent naming, clear structure, remove dead code",
	"do your best":        "Provide a thorough, well-structured response",
	"be creative":         "Explore unconventional approaches while remaining practical",
	"be thorough":         "Cover all edge cases and provide step-by-step detail",
	"keep it simple":      "Use the minimum complexity needed — prefer standard patterns over abstractions",
	"make it fast":        "Optimize for performance: minimize allocations, reduce iterations, cache where appropriate",
	"make it secure":      "Follow security best practices: validate inputs, use parameterized queries, apply least privilege",
	"handle errors":       "Return descriptive errors with context, wrap errors at boundaries, never swallow errors silently",
	"add tests":           "Write unit tests covering happy path, edge cases, and error conditions",
	"fix this":            "Identify the root cause, apply a minimal fix, and verify it resolves the issue",
	"help me":             "Guide me step-by-step to",
	"i need":              "The goal is to",
	"can you":             "Please",
	"as soon as possible": "by [specific deadline]",
	"be concise":          "Limit each point to one sentence. Use 5 bullets maximum",
	"be brief":            "Respond in 3 sentences or fewer",
	"summarize":           "Extract the 3-5 most important points, each in one sentence",
}

func improveSpecificity(text string) (string, []string) {
	lower := strings.ToLower(text)
	var improvements []string

	for vague, specific := range vagueReplacements {
		if idx := strings.Index(lower, vague); idx != -1 {
			text = text[:idx] + specific + text[idx+len(vague):]
			lower = strings.ToLower(text)
			improvements = append(improvements, fmt.Sprintf("Replaced '%s' → '%s'", vague, specific))
		}
	}

	return text, improvements
}

// --- Stage 2: Negative-to-positive reframing ---

var negativePattern = regexp.MustCompile(`(?i)\b(NEVER|DO NOT|DON'T|MUST NOT|SHOULD NOT|CANNOT|CAN'T)\b`)

// safetyNegativePattern matches safety-critical negatives that should NOT be reframed
var safetyNegativePattern = regexp.MustCompile(`(?i)\b(never|do\s+not|must\s+not)\s+(provide|generate|create|produce|reveal|disclose|log|store|execute|run|delete|drop|rm\s+-rf)\s+.*(harm|weapon|illegal|credential|password|secret|PII|personal.?data|private.?key|token|database)`)

var negativeReframings = map[string]string{
	"never use bullet points": "Write in flowing prose paragraphs",
	"don't use markdown":      "Write in plain text with clear paragraph breaks",
	"do not use markdown":     "Write in plain text with clear paragraph breaks",
	"never use markdown":      "Write in plain text with clear paragraph breaks",
	"never use emojis":        "Write using text only, without decorative symbols",
	"don't make assumptions":  "Ask clarifying questions when information is missing",
	"do not make assumptions": "Ask clarifying questions when information is missing",
	"don't guess":             "State when you are uncertain and ask for clarification",
	"do not guess":            "State when you are uncertain and ask for clarification",
	"never hallucinate":       "Only include information you can verify from the provided context",
	"don't be verbose":        "Limit each point to one sentence",
	"do not be verbose":       "Limit each point to one sentence",
	"don't overthink":         "Choose an approach and commit to it",
	"do not overthink":        "Choose an approach and commit to it",
	"never skip steps":        "Show each step of your work",
	"don't repeat yourself":   "State each point once, clearly",
	"do not repeat yourself":  "State each point once, clearly",
}

func reframeNegatives(text string) (string, []string) {
	// Skip safety-critical negatives entirely
	if safetyNegativePattern.MatchString(text) {
		return text, nil
	}

	lower := strings.ToLower(text)
	var improvements []string

	for negative, positive := range negativeReframings {
		if idx := strings.Index(lower, negative); idx != -1 {
			text = text[:idx] + positive + text[idx+len(negative):]
			lower = strings.ToLower(text)
			improvements = append(improvements, fmt.Sprintf("Reframed '%s' → '%s' (positive framing)", negative, positive))
		}
	}

	return text, improvements
}

// --- Stage 3: Tone downgrade (Claude 4.x best practice) ---

// aggressiveCapsPattern detects ALL-CAPS emphasis words (not acronyms)
var aggressiveCapsPattern = regexp.MustCompile(`\b(CRITICAL|IMPORTANT|MUST|ALWAYS|NEVER|WARNING|REQUIRED|MANDATORY|ABSOLUTELY|ESSENTIAL)\b`)

// acronymWhitelist contains ALL-CAPS words that are acronyms, not emphasis
var acronymWhitelist = map[string]bool{
	"API": true, "URL": true, "HTTP": true, "HTTPS": true,
	"JSON": true, "XML": true, "HTML": true, "CSS": true,
	"SQL": true, "SSH": true, "TCP": true, "UDP": true,
	"DNS": true, "TLS": true, "SSL": true, "JWT": true,
	"UUID": true, "URI": true, "REST": true, "GRPC": true,
	"MCP": true, "SSE": true, "MIDI": true, "NDI": true,
	"OTEL": true, "PII": true, "YAML": true, "TOML": true,
	"CI": true, "CD": true, "PR": true, "UI": true,
	"OS": true, "IO": true, "ID": true, "OK": true,
	"BPM": true, "OSC": true, "DMX": true, "OBS": true,
	"AWS": true, "GCP": true, "CLI": true, "SDK": true,
	"CSV": true, "PDF": true, "PNG": true, "JPG": true,
	"EOF": true, "NULL": true, "TRUE": true, "FALSE": true,
}

var aggressiveCapsReplacements = map[string]string{
	"CRITICAL":   "critical",
	"IMPORTANT":  "important",
	"MUST":       "must",
	"ALWAYS":     "always",
	"NEVER":      "never",
	"WARNING":    "warning",
	"REQUIRED":   "required",
	"MANDATORY":  "required",
	"ABSOLUTELY": "",
	"ESSENTIAL":  "essential",
}

func downgradeTone(text string) (string, []string) {
	var improvements []string

	matches := aggressiveCapsPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	downgraded := 0
	for _, match := range matches {
		// Skip acronyms
		if acronymWhitelist[match] {
			continue
		}
		replacement, ok := aggressiveCapsReplacements[match]
		if !ok {
			continue
		}
		if replacement == "" {
			text = strings.Replace(text, match+" ", "", 1)
			text = strings.Replace(text, " "+match, "", 1)
		} else {
			text = strings.Replace(text, match, replacement, 1)
		}
		downgraded++
	}

	if downgraded > 0 {
		improvements = append(improvements, fmt.Sprintf("Downgraded %d aggressive ALL-CAPS words to normal case (Claude 4.x best practice — overtriggers on aggressive language)", downgraded))
	}
	return text, improvements
}

// --- Stage 6: XML Structure (with over-tagging prevention) ---

func roleForTaskType(tt TaskType) string {
	switch tt {
	case TaskTypeCode:
		return "You are an expert software engineer."
	case TaskTypeCreative:
		return "You are a creative director with deep technical knowledge."
	case TaskTypeAnalysis:
		return "You are a thorough analytical reviewer."
	case TaskTypeTroubleshooting:
		return "You are a systems diagnostician focused on root cause analysis."
	case TaskTypeWorkflow:
		return "You are a workflow architect focused on reliability and simplicity."
	default:
		return "You are a knowledgeable assistant."
	}
}

func constraintsForTaskType(tt TaskType) string {
	switch tt {
	case TaskTypeCode:
		return `- Write clean, idiomatic code
- Handle errors explicitly — return descriptive errors with context
- Prefer simplicity over cleverness
- Only implement what was requested`
	case TaskTypeCreative:
		return `- Provide specific parameters and values, not vague descriptions
- Balance creative ambition with practical constraints
- Include concrete examples of the aesthetic you're describing`
	case TaskTypeAnalysis:
		return `- Support every claim with evidence from the provided data
- Distinguish facts from inferences, note confidence levels
- Use structured comparisons when evaluating alternatives`
	case TaskTypeTroubleshooting:
		return `- Start with the least disruptive diagnostic checks
- Identify root cause, not surface symptoms
- Propose fixes with rollback steps`
	case TaskTypeWorkflow:
		return `- Each step must have a clear success/failure condition
- Include error handling and rollback for each step
- Run independent steps in parallel where dependencies allow`
	default:
		return `- Be specific and actionable
- Structure output clearly with headers
- Respond directly without preamble`
	}
}

// shouldAddStructure implements over-tagging prevention.
// Anthropic docs: "Tags help most when prompts mix instructions, context, examples, and variable inputs."
// Simple, short, single-purpose prompts should NOT get XML tags — they add noise.
func shouldAddStructure(text string) bool {
	words := strings.Fields(text)
	// Too short — XML tags would be noise
	if len(words) < 15 {
		return false
	}
	// Already has XML tags
	lower := strings.ToLower(text)
	if strings.Contains(lower, "<instructions") || strings.Contains(lower, "<role") {
		return false
	}
	return true
}

func addStructure(text string, taskType TaskType) (string, []string) {
	if !shouldAddStructure(text) {
		return text, []string{"Prompt is short/simple — skipped XML wrapping (over-tagging prevention)"}
	}

	var b strings.Builder
	var improvements []string

	b.WriteString("<role>")
	b.WriteString(roleForTaskType(taskType))
	b.WriteString("</role>\n\n")
	improvements = append(improvements, "Added <role> tag with task-appropriate persona")

	// Detect if there's a code block or long context that should be separated
	if codeBlock, query := extractCodeBlock(text); codeBlock != "" {
		b.WriteString("<context>\n")
		b.WriteString(codeBlock)
		b.WriteString("\n</context>\n\n")
		b.WriteString("<instructions>\n")
		b.WriteString(strings.TrimSpace(query))
		b.WriteString("\n</instructions>\n\n")
		improvements = append(improvements, "Separated code/context block from instructions (long context before query — up to 30% quality improvement per Anthropic)")
	} else {
		b.WriteString("<instructions>\n")
		b.WriteString(strings.TrimSpace(text))
		b.WriteString("\n</instructions>\n\n")
	}
	improvements = append(improvements, "Wrapped prompt in XML structure tags")

	b.WriteString("<constraints>\n")
	b.WriteString(constraintsForTaskType(taskType))
	b.WriteString("\n</constraints>\n")
	improvements = append(improvements, "Added task-type-specific <constraints>")

	return b.String(), improvements
}

// extractCodeBlock detects if the prompt contains a code block (``` delimited) and
// separates it from the surrounding text. Returns (code, rest) or ("", original).
func extractCodeBlock(text string) (string, string) {
	startIdx := strings.Index(text, "```")
	if startIdx == -1 {
		return "", text
	}
	endIdx := strings.Index(text[startIdx+3:], "```")
	if endIdx == -1 {
		return "", text
	}
	endIdx += startIdx + 3 + 3

	codeBlock := text[startIdx:endIdx]
	rest := strings.TrimSpace(text[:startIdx] + text[endIdx:])
	if len(rest) < 10 {
		return "", text
	}
	return codeBlock, rest
}

// --- Stage 4a: Overtrigger phrase rewriting (Claude 4.x) ---

// overtriggerPattern detects aggressive anti-laziness prefixes that cause Claude 4.x to overtrigger.
// Per Anthropic: "CRITICAL: You MUST use this tool" should become "Use this tool when..."
var overtriggerPattern = regexp.MustCompile(
	`(?i)(CRITICAL|IMPORTANT|REQUIRED|WARNING)\s*[:!]\s*(You\s+)?(MUST|ALWAYS|NEVER|SHOULD)\s+`,
)

func rewriteOvertriggerPhrases(text string) (string, []string) {
	matches := overtriggerPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	var improvements []string
	count := 0
	result := overtriggerPattern.ReplaceAllStringFunc(text, func(match string) string {
		count++
		// Extract the action verb that follows the aggressive prefix
		// The pattern captures up to and including "MUST ", "ALWAYS ", etc.
		// We want to strip the prefix and keep just the verb context
		lower := strings.ToLower(match)
		// Find the modal verb position
		for _, modal := range []string{"must ", "always ", "never ", "should "} {
			if idx := strings.Index(lower, modal); idx != -1 {
				verb := strings.TrimSpace(match[idx:])
				// Convert "MUST do" to "do" or "NEVER do" to "avoid doing" / keep "never"
				verbLower := strings.ToLower(verb)
				if strings.HasPrefix(verbLower, "never ") {
					return "Avoid: " + strings.TrimSpace(verb[6:])
				}
				if strings.HasPrefix(verbLower, "must ") {
					return strings.TrimSpace(verb[5:])
				}
				if strings.HasPrefix(verbLower, "always ") {
					return strings.TrimSpace(verb[7:])
				}
				if strings.HasPrefix(verbLower, "should ") {
					return strings.TrimSpace(verb[7:])
				}
			}
		}
		return match
	})

	if count > 0 {
		improvements = append(improvements, fmt.Sprintf("Rewrote %d overtrigger phrase(s) — Claude 4.x overtriggers on aggressive 'CRITICAL: You MUST' prefixes", count))
	}
	return result, improvements
}

// --- Stage 11: Overengineering guard (Claude 4.x, code tasks only) ---

// overengineeringExemptPattern matches prompts that explicitly request new scaffolding
var overengineeringExemptPattern = regexp.MustCompile(`(?i)(create\s+new|scaffold|generate\s+boilerplate|set\s+up\s+a\s+new|initialize\s+a\s+new)`)

func injectOverengineeringGuard(text string, taskType TaskType) (string, []string) {
	if taskType != TaskTypeCode {
		return text, nil
	}

	lower := strings.ToLower(text)
	// Skip if prompt is asking for new creation/scaffolding
	if overengineeringExemptPattern.MatchString(text) {
		return text, nil
	}
	// Skip if guard already present
	if strings.Contains(lower, "only make changes that are directly requested") ||
		strings.Contains(lower, "overengineering") {
		return text, nil
	}

	guard := "\n\nOnly make changes that are directly requested or clearly necessary. Prefer editing existing files to creating new ones. Do not add abstractions, helpers, or defensive code for scenarios that cannot happen."
	return text + guard, []string{"Injected overengineering guard — Claude 4.x tends to over-abstract and create unnecessary files"}
}

// --- Token budget and cost tier ---

func costTierForTokens(tokens int) string {
	switch {
	case tokens < 1_000:
		return "minimal"
	case tokens < 10_000:
		return "small"
	case tokens < 50_000:
		return "medium"
	case tokens < 200_000:
		return "large"
	default:
		return "max-context"
	}
}

// --- Effort level recommendation ---

// complexityKeywords indicate prompts that benefit from high effort
var complexityKeywords = regexp.MustCompile(`(?i)\b(refactor|architecture|across\s+multiple\s+files|redesign|migrate|rewrite|comprehensive|exhaustive|all\s+edge\s+cases|full\s+audit)\b`)

func recommendEffort(prompt string, taskType TaskType) string {
	words := strings.Fields(prompt)
	wordCount := len(words)
	hasComplexity := complexityKeywords.MatchString(prompt)

	// High effort: complex tasks, long prompts, or explicit complexity keywords
	if hasComplexity || wordCount > 200 {
		return "high"
	}

	// Low effort: short general prompts
	if taskType == TaskTypeGeneral && wordCount < 20 {
		return "low"
	}

	// Medium: everything else
	return "medium"
}

// --- Stage 5: Format enforcement ---

// Format detection patterns (from Anthropic docs: positive format instructions work best)
var (
	jsonFormatPattern = regexp.MustCompile(`(?i)\b(json|JSON)\b.*(output|response|format|return|respond)|return\s+(?:as\s+)?json|(as|in)\s+json`)
	yamlFormatPattern = regexp.MustCompile(`(?i)\b(yaml|YAML)\b.*(output|response|format|return)|(as|in)\s+yaml`)
	codeFormatPattern = regexp.MustCompile(`(?i)^(write|create|implement|generate)\s+(a\s+)?(function|class|method|script|program|module)\b`)
	csvFormatPattern  = regexp.MustCompile(`(?i)\b(csv|CSV)\b.*(output|response|format|return)|(as|in)\s+csv`)
)

func enforceOutputFormat(text string) (string, []string) {
	lower := strings.ToLower(text)

	// Already has format specification
	if strings.Contains(lower, "<output_format>") || strings.Contains(lower, "output_format") {
		return text, nil
	}

	var formatBlock string
	var desc string

	switch {
	case jsonFormatPattern.MatchString(text):
		formatBlock = "\n\n<output_format>\nYour entire response must be valid JSON. Do not wrap it in markdown code fences. Do not include any text before or after the JSON. The response must parse with a standard JSON parser.\n</output_format>"
		desc = "Injected JSON format enforcement in <output_format> tags"
	case yamlFormatPattern.MatchString(text):
		formatBlock = "\n\n<output_format>\nYour entire response must be valid YAML. Do not wrap it in markdown code fences. Do not include any text before or after the YAML.\n</output_format>"
		desc = "Injected YAML format enforcement in <output_format> tags"
	case csvFormatPattern.MatchString(text):
		formatBlock = "\n\n<output_format>\nYour entire response must be valid CSV. Include a header row. Do not wrap in code fences or include any text before or after the CSV data.\n</output_format>"
		desc = "Injected CSV format enforcement in <output_format> tags"
	case codeFormatPattern.MatchString(text):
		formatBlock = "\n\n<output_format>\nReturn only the code. Do not include markdown code fences unless explicitly asked. Do not include explanatory text before or after the code unless asked.\n</output_format>"
		desc = "Injected code format enforcement in <output_format> tags"
	default:
		return text, nil
	}

	return text + formatBlock, []string{desc}
}

// --- Stage 10: Self-check injection ---

func injectSelfCheck(text string, taskType TaskType) (string, []string) {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "verify") || strings.Contains(lower, "double-check") ||
		strings.Contains(lower, "self-check") || strings.Contains(lower, "before you finish") {
		return text, nil
	}

	var check string
	switch taskType {
	case TaskTypeCode:
		check = "\n\n<verification>\nBefore finishing, verify:\n- The code compiles/runs without errors\n- Edge cases are handled (empty input, nil, zero values)\n- Error messages are descriptive\n</verification>"
	case TaskTypeAnalysis:
		check = "\n\n<verification>\nBefore finishing, verify:\n- Every claim is supported by specific evidence\n- You have distinguished facts from inferences\n- Alternative interpretations have been considered\n</verification>"
	case TaskTypeTroubleshooting:
		check = "\n\n<verification>\nBefore finishing, verify:\n- The proposed fix addresses the root cause, not a symptom\n- Rollback steps are included\n- No other systems are affected by the fix\n</verification>"
	default:
		return text, nil
	}

	return text + check, []string{"Injected self-verification checklist for " + string(taskType) + " task"}
}

// --- Stage 12: Preamble suppression ---

func suppressPreamble(text string, taskType TaskType) (string, []string) {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "without preamble") || strings.Contains(lower, "respond directly") ||
		strings.Contains(lower, "no preamble") {
		return text, nil
	}

	switch taskType {
	case TaskTypeCode, TaskTypeWorkflow:
		return text + "\n\nRespond directly without preamble. Do not start with phrases like 'Here is...', 'Sure,...', or 'Based on...'.",
			[]string{"Added preamble suppression for direct output"}
	default:
		return text, nil
	}
}
