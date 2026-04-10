package enhancer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGolden_Enhance(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		taskType TaskType
		file     string
	}{
		{
			name:     "code_task",
			input:    "write a function to parse JSON data and handle all the edge cases properly in Go with error handling",
			taskType: TaskTypeCode,
			file:     "golden_enhance_code.txt",
		},
		{
			name:     "analysis_task",
			input:    "analyze this dataset for trends and patterns in the user behavior metrics over the past quarter",
			taskType: TaskTypeAnalysis,
			file:     "golden_enhance_analysis.txt",
		},
		{
			name:     "short_general",
			input:    "hello world",
			taskType: TaskTypeGeneral,
			file:     "golden_enhance_general.txt",
		},
	}

	update := os.Getenv("GOLDEN_UPDATE") == "1"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Enhance(tt.input, tt.taskType)
			goldenPath := filepath.Join("testdata", tt.file)

			if update {
				if err := os.WriteFile(goldenPath, []byte(result.Enhanced), 0644); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated golden file: %s", goldenPath)
				return
			}

			expected, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden file not found (run GOLDEN_UPDATE=1 to create): %v", err)
			}

			if result.Enhanced != string(expected) {
				t.Errorf("output does not match golden file %s\n\nGot:\n%s\n\nWant:\n%s",
					goldenPath, truncate(result.Enhanced, 500), truncate(string(expected), 500))
			}
		})
	}
}
