package enhancer

import (
	"regexp"
	"strings"
)

// conversationalPattern matches short conversational prompts that should not be enhanced.
var conversationalPattern = regexp.MustCompile(
	`(?i)^(y|n|yes|no|ok|k|sure|thanks|done|continue|go ahead|looks good|lgtm|ship it|do it|next|proceed|approve|reject|cancel|stop|undo|revert|nah)$`,
)

// alreadyStructuredPattern matches prompts that already have XML structure tags.
var alreadyStructuredPattern = regexp.MustCompile(`(?i)<(instructions|role|system|prompt)[\s>]`)

// filePathOnlyPattern matches prompts that are just a file path or glob.
var filePathOnlyPattern = regexp.MustCompile(`^[./~][\w/.*\- ]+$`)

// ShouldEnhance returns true if the prompt is worth running through the enhancement pipeline.
// Short conversational replies, already-structured prompts, and bare file paths are skipped.
func ShouldEnhance(prompt string, cfg Config) bool {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return false
	}

	minWords := cfg.Hook.MinWordCount
	if minWords <= 0 {
		minWords = 5
	}

	// Word count gate
	words := strings.Fields(trimmed)
	if len(words) < minWords {
		return false
	}

	// Conversational allowlist
	if conversationalPattern.MatchString(trimmed) {
		return false
	}

	// Already-structured gate
	if alreadyStructuredPattern.MatchString(trimmed) {
		return false
	}

	// File-path-only gate
	if filePathOnlyPattern.MatchString(trimmed) {
		return false
	}

	// Custom skip patterns from config
	for _, pat := range cfg.Hook.SkipPatterns {
		if re, err := regexp.Compile(pat); err == nil {
			if re.MatchString(trimmed) {
				return false
			}
		}
	}

	return true
}
