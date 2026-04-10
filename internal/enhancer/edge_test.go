package enhancer

import (
	"strings"
	"testing"
	"time"
)

func TestEdge_EmptyInputs(t *testing.T) {
	t.Run("Enhance_empty", func(t *testing.T) {
		result := Enhance("", TaskTypeGeneral)
		if result.TaskType != TaskTypeGeneral {
			t.Errorf("expected general task type, got %q", result.TaskType)
		}
	})

	t.Run("Analyze_empty", func(t *testing.T) {
		result := Analyze("")
		if result.Score < 1 {
			t.Errorf("score should be at least 1, got %d", result.Score)
		}
	})

	t.Run("Lint_empty", func(t *testing.T) {
		results := Lint("")
		// Should not panic, may or may not have results
		_ = results
	})

	t.Run("Classify_empty", func(t *testing.T) {
		tt := Classify("")
		if tt != TaskTypeGeneral {
			t.Errorf("empty should classify as general, got %q", tt)
		}
	})

	t.Run("DetectAndWrapExamples_empty", func(t *testing.T) {
		result, imps := DetectAndWrapExamples("")
		if result != "" {
			t.Error("expected empty result")
		}
		if len(imps) > 0 {
			t.Error("expected no improvements")
		}
	})

	t.Run("FillTemplate_nilVars", func(t *testing.T) {
		tmpl := GetTemplate("troubleshoot")
		result := FillTemplate(tmpl, nil)
		if result == "" {
			t.Error("should not return empty")
		}
	})
}

func TestEdge_Unicode(t *testing.T) {
	t.Run("CJK_Enhance", func(t *testing.T) {
		input := "请编写一个函数来排序用户列表，处理所有边界情况并返回结果。这个函数需要高效且可靠。"
		result := Enhance(input, TaskTypeCode)
		if result.Enhanced == "" {
			t.Error("should produce non-empty enhancement for CJK text")
		}
		if result.EstimatedTokens == 0 {
			t.Error("should estimate tokens for CJK text")
		}
	})

	t.Run("Emoji_Enhance", func(t *testing.T) {
		input := "🚀 Create a deployment pipeline that handles 📦 packaging and 🧪 testing across all services"
		result := Enhance(input, TaskTypeWorkflow)
		if result.Enhanced == "" {
			t.Error("should produce non-empty enhancement for emoji text")
		}
	})

	t.Run("RTL_Enhance", func(t *testing.T) {
		input := "اكتب دالة لفرز المستخدمين حسب الاسم مع معالجة الأخطاء بشكل صحيح"
		result := Enhance(input, TaskTypeCode)
		if result.Enhanced == "" {
			t.Error("should produce non-empty enhancement for RTL text")
		}
	})

	t.Run("EstimateTokens_CJK", func(t *testing.T) {
		text := strings.Repeat("你好世界", 100)
		tokens := EstimateTokens(text)
		if tokens == 0 {
			t.Error("should estimate tokens for CJK text")
		}
	})

	t.Run("EstimateTokens_Emoji", func(t *testing.T) {
		text := strings.Repeat("🎵🎶🎼", 50)
		tokens := EstimateTokens(text)
		if tokens == 0 {
			t.Error("should estimate tokens for emoji text")
		}
	})

	t.Run("EstimateTokens_Mixed", func(t *testing.T) {
		text := "Hello 你好 مرحبا 🌍"
		tokens := EstimateTokens(text)
		if tokens == 0 {
			t.Error("should estimate tokens for mixed scripts")
		}
	})
}

func TestEdge_LargeInputs(t *testing.T) {
	large := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 2500) // ~100K chars

	t.Run("Enhance_large", func(t *testing.T) {
		start := time.Now()
		result := Enhance(large, TaskTypeGeneral)
		elapsed := time.Since(start)
		if elapsed > time.Second {
			t.Errorf("Enhance took %v for 100K input, want < 1s", elapsed)
		}
		if result.Enhanced == "" {
			t.Error("should produce output for large input")
		}
	})

	t.Run("Lint_large", func(t *testing.T) {
		start := time.Now()
		results := Lint(large)
		elapsed := time.Since(start)
		if elapsed > time.Second {
			t.Errorf("Lint took %v for 100K input, want < 1s", elapsed)
		}
		_ = results
	})
}

func TestEdge_ZeroValueConfig(t *testing.T) {
	t.Run("empty_config", func(t *testing.T) {
		cfg := Config{}
		result := EnhanceWithConfig("write a function to sort users by name with error handling and edge cases", "", cfg)
		if result.Enhanced == "" {
			t.Error("should produce output with zero-value config")
		}
	})

	t.Run("nil_slices", func(t *testing.T) {
		cfg := Config{
			DisabledStages: nil,
			Rules:          nil,
			BlockPatterns:  nil,
		}
		result := EnhanceWithConfig("write a function to sort users by name with error handling and edge cases", "", cfg)
		if result.Enhanced == "" {
			t.Error("should produce output with nil slice config")
		}
	})
}
