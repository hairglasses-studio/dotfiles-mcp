package enhancer

import (
	"testing"
)

func TestGradeForScore(t *testing.T) {
	tests := []struct {
		name  string
		score int
		want  string
	}{
		{"A_high", 95, "A"},
		{"A_boundary", 90, "A"},
		{"B_high", 89, "B"},
		{"B_low", 80, "B"},
		{"C_high", 79, "C"},
		{"C_low", 65, "C"},
		{"D_high", 64, "D"},
		{"D_low", 50, "D"},
		{"F_high", 49, "F"},
		{"F_zero", 0, "F"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gradeForScore(tt.score)
			if got != tt.want {
				t.Errorf("gradeForScore(%d) = %q, want %q", tt.score, got, tt.want)
			}
		})
	}
}

func TestScore_PerfectPrompt(t *testing.T) {
	prompt := `<role>You are an expert Go developer with 10 years of experience.</role>

<context>
We are building a user management API in Go. The codebase uses the standard library
net/http package with chi router. This is because we want minimal dependencies.
</context>

<instructions>
Review the following function for error handling issues.
Focus on nil pointer dereferences and unchecked errors because these cause runtime panics.
Return exactly 5 issues, each in one sentence, sorted by severity.
</instructions>

<examples>
<example index="1">
Input: func getUser(id string) *User { return db.Find(id) }
Output: Missing nil check on db.Find return — will panic if user not found.
</example>
<example index="2">
Input: data, _ := json.Marshal(user)
Output: Ignoring json.Marshal error — will silently produce empty data on failure.
</example>
<example index="3">
Input: f, err := os.Open(path); defer f.Close()
Output: Defer before error check — will panic on nil file handle if Open fails.
</example>
</examples>

<output_format>
Return a numbered list of exactly 5 issues. Each issue should include:
1. The problematic code pattern
2. The risk (in 10 words or fewer)
3. The fix
</output_format>

<constraints>
- Only report real issues supported by the code, because false positives waste review time
- Distinguish severity levels (critical, warning, info) to help prioritize fixes
</constraints>`

	ar := Analyze(prompt)
	report := ar.ScoreReport

	if report == nil {
		t.Fatal("ScoreReport should not be nil")
	}
	if report.Overall < 80 {
		t.Errorf("Well-structured prompt overall = %d, want >= 80", report.Overall)
	}
	if report.Grade != "A" && report.Grade != "B" {
		t.Errorf("Grade = %q, want A or B", report.Grade)
	}
	if len(report.Dimensions) != 10 {
		t.Errorf("Dimensions count = %d, want 10", len(report.Dimensions))
	}
}

func TestScore_BadPrompt(t *testing.T) {
	ar := Analyze("fix this")
	report := ar.ScoreReport

	if report == nil {
		t.Fatal("ScoreReport should not be nil")
	}
	if report.Overall > 50 {
		t.Errorf("Bad prompt overall = %d, want <= 50", report.Overall)
	}
	if report.Grade != "D" && report.Grade != "F" {
		t.Errorf("Grade = %q, want D or F", report.Grade)
	}
}

func TestScore_DecentPrompt(t *testing.T) {
	prompt := "You are an expert software engineer. Review this Go function for error handling. Focus on nil checks and unchecked returns. Format output as a numbered list."
	ar := Analyze(prompt)
	report := ar.ScoreReport

	if report == nil {
		t.Fatal("ScoreReport should not be nil")
	}
	// Decent but not great — should be C range
	if report.Overall < 40 || report.Overall > 85 {
		t.Errorf("Decent prompt overall = %d, want 40-85", report.Overall)
	}
}

func TestScore_LegacyDerivation(t *testing.T) {
	// A prompt scoring ~75 overall should give legacy 7-8
	prompt := `<role>You are an expert Go developer.</role>
<instructions>Review this function for error handling issues.
Focus on nil pointer dereferences and unchecked errors.</instructions>
<context>The function processes user-uploaded files.</context>
<output_format>List issues by severity with line numbers.</output_format>
<examples><example>Good: if err != nil { return fmt.Errorf("upload: %%w", err) }</example></examples>`
	ar := Analyze(prompt)

	if ar.Score < 1 || ar.Score > 10 {
		t.Errorf("Legacy score = %d, want 1-10", ar.Score)
	}
	// The report should be populated
	if ar.ScoreReport == nil {
		t.Fatal("ScoreReport should be populated")
	}
	// Legacy score should roughly be overall/10
	expected := ar.ScoreReport.Overall / 10
	if expected < 1 {
		expected = 1
	}
	if ar.Score != expected {
		t.Errorf("Legacy score %d != overall/10 (%d)", ar.Score, expected)
	}
}

func TestScore_DimensionWeightsSum(t *testing.T) {
	ar := Analyze("test prompt for weight verification purposes with enough words")
	report := ar.ScoreReport
	if report == nil {
		t.Fatal("ScoreReport should not be nil")
	}

	var totalWeight float64
	for _, d := range report.Dimensions {
		totalWeight += d.Weight
	}
	// Should sum to 1.0 (within floating point tolerance)
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("Dimension weights sum to %f, want ~1.0", totalWeight)
	}
}

func TestScore_DimensionNames(t *testing.T) {
	ar := Analyze("write a function to sort users by name")
	report := ar.ScoreReport
	if report == nil {
		t.Fatal("ScoreReport should not be nil")
	}

	expectedNames := []string{
		"Clarity", "Specificity", "Context & Motivation", "Structure",
		"Examples", "Document Placement", "Role Definition", "Task Focus",
		"Format Specification", "Tone",
	}
	if len(report.Dimensions) != len(expectedNames) {
		t.Fatalf("Got %d dimensions, want %d", len(report.Dimensions), len(expectedNames))
	}
	for i, d := range report.Dimensions {
		if d.Name != expectedNames[i] {
			t.Errorf("Dimension[%d].Name = %q, want %q", i, d.Name, expectedNames[i])
		}
	}
}

func TestScore_DimensionScoresInRange(t *testing.T) {
	prompts := []string{
		"fix this",
		"write a function to sort users by name with error handling",
		"CRITICAL: You MUST NEVER expose secrets. DO NOT use bullet points.",
		`<role>Expert</role><instructions>Do the thing</instructions>`,
	}
	for _, p := range prompts {
		ar := Analyze(p)
		for _, d := range ar.ScoreReport.Dimensions {
			if d.Score < 0 || d.Score > 100 {
				t.Errorf("Dimension %q score %d out of range for prompt %q", d.Name, d.Score, p[:20])
			}
			validGrades := map[string]bool{"A": true, "B": true, "C": true, "D": true, "F": true}
			if !validGrades[d.Grade] {
				t.Errorf("Dimension %q has invalid grade %q", d.Name, d.Grade)
			}
		}
	}
}

// --- Per-dimension tests ---

func TestScoreClarity(t *testing.T) {
	t.Run("high_clarity", func(t *testing.T) {
		ar := Analyze("Write exactly 5 unit tests covering edge cases for the parseJSON function. Each test should be under 20 lines.")
		d := findDimension(ar.ScoreReport, "Clarity")
		if d.Score < 65 {
			t.Errorf("High-clarity prompt scored %d, want >= 65", d.Score)
		}
	})
	t.Run("low_clarity", func(t *testing.T) {
		ar := Analyze("make it good")
		d := findDimension(ar.ScoreReport, "Clarity")
		if d.Score > 50 {
			t.Errorf("Low-clarity prompt scored %d, want <= 50", d.Score)
		}
	})
}

func TestScoreStructure(t *testing.T) {
	t.Run("with_xml", func(t *testing.T) {
		ar := Analyze("<role>Expert</role>\n\n<instructions>Do the thing.</instructions>\n\n<constraints>Be careful.</constraints>")
		d := findDimension(ar.ScoreReport, "Structure")
		if d.Score < 65 {
			t.Errorf("Structured prompt scored %d, want >= 65", d.Score)
		}
	})
	t.Run("no_xml", func(t *testing.T) {
		ar := Analyze("just do the thing please")
		d := findDimension(ar.ScoreReport, "Structure")
		if d.Score > 40 {
			t.Errorf("Unstructured prompt scored %d, want <= 40", d.Score)
		}
	})
}

func TestScoreExamples(t *testing.T) {
	t.Run("with_examples", func(t *testing.T) {
		ar := Analyze(`Do the task.
<example index="1">First</example>
<example index="2">Second</example>
<example index="3">Third</example>`)
		d := findDimension(ar.ScoreReport, "Examples")
		if d.Score < 65 {
			t.Errorf("Prompt with 3 examples scored %d, want >= 65", d.Score)
		}
	})
	t.Run("no_examples", func(t *testing.T) {
		ar := Analyze("do the task without any demonstration")
		d := findDimension(ar.ScoreReport, "Examples")
		if d.Score > 40 {
			t.Errorf("Prompt without examples scored %d, want <= 40", d.Score)
		}
	})
}

func TestScoreTone(t *testing.T) {
	t.Run("clean_tone", func(t *testing.T) {
		ar := Analyze("Please review this code for bugs and suggest improvements.")
		d := findDimension(ar.ScoreReport, "Tone")
		if d.Score < 70 {
			t.Errorf("Clean tone scored %d, want >= 70", d.Score)
		}
	})
	t.Run("aggressive_tone", func(t *testing.T) {
		ar := Analyze("CRITICAL: You MUST NEVER skip tests. DO NOT use mocks. ALWAYS validate inputs.")
		d := findDimension(ar.ScoreReport, "Tone")
		if d.Score > 50 {
			t.Errorf("Aggressive tone scored %d, want <= 50", d.Score)
		}
	})
}

func TestScoreRoleDefinition(t *testing.T) {
	t.Run("with_role", func(t *testing.T) {
		ar := Analyze("<role>You are an expert security auditor.</role>\nReview this code.")
		d := findDimension(ar.ScoreReport, "Role Definition")
		if d.Score < 70 {
			t.Errorf("Prompt with role scored %d, want >= 70", d.Score)
		}
	})
	t.Run("no_role", func(t *testing.T) {
		ar := Analyze("review this code")
		d := findDimension(ar.ScoreReport, "Role Definition")
		if d.Score > 40 {
			t.Errorf("Prompt without role scored %d, want <= 40", d.Score)
		}
	})
}

func TestScoreContextMotivation(t *testing.T) {
	t.Run("motivated", func(t *testing.T) {
		ar := Analyze("<context>Building a payment API</context>\nValidate all inputs because unvalidated input caused a production incident last week.")
		d := findDimension(ar.ScoreReport, "Context & Motivation")
		if d.Score < 70 {
			t.Errorf("Motivated prompt with context scored %d, want >= 70", d.Score)
		}
	})
	t.Run("unmotivated", func(t *testing.T) {
		ar := Analyze("do the thing right")
		d := findDimension(ar.ScoreReport, "Context & Motivation")
		if d.Score > 45 {
			t.Errorf("Unmotivated prompt scored %d, want <= 45", d.Score)
		}
	})
}

// --- Regression: existing TestAnalyze_ScoresPrompts must still pass ---
// (The test in enhancer_test.go covers this — we just verify ScoreReport is populated)

func TestScore_AnalyzeIntegration(t *testing.T) {
	t.Run("bad_prompt_has_report", func(t *testing.T) {
		ar := Analyze("fix this")
		if ar.ScoreReport == nil {
			t.Fatal("ScoreReport should be populated for bad prompt")
		}
		if ar.Score < 1 || ar.Score > 5 {
			t.Errorf("Bad prompt legacy score = %d, want 1-5", ar.Score)
		}
	})
	t.Run("good_prompt_has_report", func(t *testing.T) {
		ar := Analyze(`<role>You are an expert Go developer.</role>
<instructions>Review this function for error handling issues.
Focus on nil pointer dereferences and unchecked errors.</instructions>
<context>The function processes user-uploaded files.</context>
<output_format>List issues by severity with line numbers.</output_format>
<examples><example>Good: if err != nil { return fmt.Errorf("upload: %%w", err) }</example></examples>`)
		if ar.ScoreReport == nil {
			t.Fatal("ScoreReport should be populated for good prompt")
		}
		if ar.Score < 5 {
			t.Errorf("Good prompt legacy score = %d, want >= 5", ar.Score)
		}
	})
}

func TestScoringCalibration(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		minScore int
		maxScore int
	}{
		{"simple CLI prompt", "Write a Go function that parses JSON and returns a struct", 55, 75},
		{"structured system prompt", "<role>You are an expert Go engineer</role>\n<instructions>Review this code for bugs</instructions>\n<constraints>Focus on error handling</constraints>", 75, 100},
		{"trivial question", "what does this do", 30, 50},
		{"medium effort", "Analyze the authentication middleware in this codebase. Look for security vulnerabilities, especially around token validation and session management. Provide a severity rating for each finding.", 60, 85},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Analyze(tt.prompt)
			if result.ScoreReport.Overall < tt.minScore || result.ScoreReport.Overall > tt.maxScore {
				t.Errorf("score %d outside expected range [%d, %d]", result.ScoreReport.Overall, tt.minScore, tt.maxScore)
			}
		})
	}
}

// findDimension returns the DimensionScore with the given name, or a zero value.
func findDimension(report *ScoreReport, name string) DimensionScore {
	if report == nil {
		return DimensionScore{}
	}
	for _, d := range report.Dimensions {
		if d.Name == name {
			return d
		}
	}
	return DimensionScore{}
}
