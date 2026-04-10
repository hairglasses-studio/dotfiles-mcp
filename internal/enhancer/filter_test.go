package enhancer

import "testing"

func TestShouldEnhance(t *testing.T) {
	defaultCfg := Config{}

	tests := []struct {
		name   string
		prompt string
		cfg    Config
		want   bool
	}{
		// Should skip
		{"empty", "", defaultCfg, false},
		{"whitespace", "   ", defaultCfg, false},
		{"yes", "yes", defaultCfg, false},
		{"ok", "ok", defaultCfg, false},
		{"continue", "continue", defaultCfg, false},
		{"lgtm", "lgtm", defaultCfg, false},
		{"ship_it", "ship it", defaultCfg, false},
		{"go_ahead", "go ahead", defaultCfg, false},
		{"case_insensitive", "LGTM", defaultCfg, false},
		{"case_insensitive_yes", "Yes", defaultCfg, false},
		{"too_short_3_words", "do it now", defaultCfg, false},
		{"too_short_1_word", "help", defaultCfg, false},
		{"file_path", "./src/main.go", defaultCfg, false},
		{"glob_path", "~/projects/*.go", defaultCfg, false},
		{"already_structured", "<instructions>Do the thing</instructions>", defaultCfg, false},
		{"already_role", "<role>You are an expert</role>", defaultCfg, false},

		// Should enhance
		{"normal_prompt", "fix the sorting bug in the user module", defaultCfg, true},
		{"longer_prompt", "write a function that takes a list of users and returns them sorted by name", defaultCfg, true},
		{"five_words", "refactor this code for clarity", defaultCfg, true},
		{"code_request", "implement a binary search function in Go with error handling", defaultCfg, true},

		// Config overrides
		{"custom_min_words", "fix the bug", Config{Hook: HookConfig{MinWordCount: 10}}, false},
		{"custom_skip_pattern", "just deploy this to staging now please", Config{Hook: HookConfig{SkipPatterns: []string{`(?i)deploy`}}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldEnhance(tt.prompt, tt.cfg)
			if got != tt.want {
				t.Errorf("ShouldEnhance(%q) = %v, want %v", tt.prompt, got, tt.want)
			}
		})
	}
}
