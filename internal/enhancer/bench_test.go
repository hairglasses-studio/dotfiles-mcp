package enhancer

import (
	"strings"
	"testing"
)

func BenchmarkEnhance(b *testing.B) {
	prompt := "Write a Go function that parses JSON input, validates the schema, and returns a typed struct with comprehensive error handling for malformed input"
	for b.Loop() {
		Enhance(prompt, TaskTypeCode)
	}
}

func BenchmarkEnhance_Short(b *testing.B) {
	for b.Loop() {
		Enhance("fix this bug in the user sorting function", TaskTypeCode)
	}
}

func BenchmarkEnhance_Medium(b *testing.B) {
	input := "CRITICAL: You MUST write a function to parse JSON data and handle all the edge cases properly in Go with error handling. Never use bullet points. Format nicely."
	for b.Loop() {
		Enhance(input, TaskTypeCode)
	}
}

func BenchmarkEnhance_Long(b *testing.B) {
	input := strings.Repeat("The system should handle ", 200) + "all edge cases properly in the codebase."
	for b.Loop() {
		Enhance(input, TaskTypeGeneral)
	}
}

func BenchmarkLint(b *testing.B) {
	input := "CRITICAL: You MUST follow this rule.\nNEVER use markdown.\nThink step by step.\nReturn several items with appropriate formatting."
	for b.Loop() {
		Lint(input)
	}
}

func BenchmarkClassify(b *testing.B) {
	input := "fix this broken function that has a timeout error in the API endpoint"
	for b.Loop() {
		Classify(input)
	}
}

func BenchmarkEstimateTokens(b *testing.B) {
	input := strings.Repeat("word ", 1000)
	for b.Loop() {
		EstimateTokens(input)
	}
}

func BenchmarkAnalyze(b *testing.B) {
	input := "NEVER use markdown. CRITICAL: fix the bug. Create a function to sort users."
	for b.Loop() {
		Analyze(input)
	}
}
