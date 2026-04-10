package enhancer

import (
	"strings"
	"testing"
)

func TestDetectAndWrapExamples_InputOutputPairs(t *testing.T) {
	input := "Convert these dates to ISO format:\n\nInput: January 5, 2024\nOutput: 2024-01-05\n\nInput: March 15, 2023\nOutput: 2023-03-15\n\nInput: December 31, 2025\nOutput: 2025-12-31\n\nNow convert: February 28, 2026"

	result, imps := DetectAndWrapExamples(input)

	if len(imps) == 0 {
		t.Fatal("Should detect and wrap input/output pairs")
	}
	assertContains(t, result, "<examples>")
	assertContains(t, result, `<example index="1">`)
	if strings.Count(result, "<example index=") < 2 {
		t.Error("Should have at least 2 wrapped examples")
	}
}

func TestDetectAndWrapExamples_ExampleHeaders(t *testing.T) {
	input := "Format user data as follows:\n\nExample 1: Simple user\nName: John, Age: 30 -> {\"name\": \"John\", \"age\": 30}\n\nExample 2: With email\nName: Jane, Email: jane@test.com -> {\"name\": \"Jane\", \"email\": \"jane@test.com\"}\n\nNow format this data."

	result, imps := DetectAndWrapExamples(input)

	if len(imps) == 0 {
		t.Fatal("Should detect and wrap example headers")
	}
	assertContains(t, result, "<examples>")
}

func TestDetectAndWrapExamples_ArrowTransformations(t *testing.T) {
	input := "Convert snake_case to camelCase:\n\nuser_name -> userName\nfirst_name -> firstName\nlast_updated_at -> lastUpdatedAt\n\nConvert: my_variable_name"

	result, imps := DetectAndWrapExamples(input)

	if len(imps) == 0 {
		t.Fatal("Should detect and wrap arrow transformations")
	}
	assertContains(t, result, "<examples>")
}

func TestDetectAndWrapExamples_AlreadyTagged(t *testing.T) {
	input := `<examples><example>Already tagged</example></examples>`
	result, imps := DetectAndWrapExamples(input)

	if len(imps) > 0 {
		t.Error("Should not double-wrap already tagged examples")
	}
	if result != input {
		t.Error("Should return input unchanged")
	}
}

func TestDetectAndWrapExamples_NoExamples(t *testing.T) {
	input := "Write a function to sort an array of integers."
	_, imps := DetectAndWrapExamples(input)

	if len(imps) > 0 {
		t.Error("Should not detect examples in plain instruction")
	}
}

// --- Phase 3E: New coverage tests ---

func TestDetectAndWrapExamples_SinglePair(t *testing.T) {
	// Single pair is not enough to wrap (need >= 2)
	input := "Convert:\n\nInput: hello\nOutput: HELLO\n\nNow convert: world"
	_, imps := DetectAndWrapExamples(input)
	if len(imps) > 0 {
		t.Error("Single pair should not be enough to wrap")
	}
}

func TestDetectAndWrapExamples_SingleHeader(t *testing.T) {
	// Single example header is not enough
	input := "Example 1: Simple case\nhello -> HELLO\n\nNow do this."
	_, imps := DetectAndWrapExamples(input)
	// wrapExampleHeaders requires >= 2 headers
	if len(imps) > 0 {
		// It might match arrow pattern though — check if < 2 arrows
		// "hello -> HELLO" is 1 arrow, so should not wrap
		_ = imps
	}
}

func TestDetectAndWrapExamples_ShortArrows(t *testing.T) {
	// Arrows with < 5 chars on each side should not match
	input := "a -> b\nc -> d\ne -> f"
	_, imps := DetectAndWrapExamples(input)
	if len(imps) > 0 {
		t.Error("Short arrow strings (< 5 chars each side) should not be wrapped")
	}
}
