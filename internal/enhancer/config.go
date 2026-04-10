package enhancer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds per-project prompt enhancement configuration.
// Loaded from .prompt-improver.yaml in the project directory.
type Config struct {
	// Preamble is always-injected context added before the enhanced prompt
	Preamble string `yaml:"preamble"`

	// Rules are pattern-matched augmentations
	Rules []Rule `yaml:"rules"`

	// BlockPatterns are regexes that cause the prompt to be blocked (exit 2)
	BlockPatterns []string `yaml:"block_patterns"`

	// DisabledStages allows disabling specific pipeline stages
	DisabledStages []string `yaml:"disabled_stages"`

	// DefaultTaskType overrides auto-detection
	DefaultTaskType string `yaml:"default_task_type"`

	// DefaultEffort overrides auto-detection of effort level (low, medium, high)
	DefaultEffort string `yaml:"default_effort"`

	// Hook holds configuration specific to the UserPromptSubmit hook mode.
	Hook HookConfig `yaml:"hook"`

	// LLM holds configuration for LLM-backed prompt improvement
	LLM LLMConfig `yaml:"llm"`
}

// LLMConfig holds settings for the LLM-backed enhancement mode.
type LLMConfig struct {
	// Enabled activates LLM-backed improvement (default false — opt-in)
	Enabled bool `yaml:"enabled"`

	// ThinkingEnabled adds thinking scaffolding to the meta-prompt
	ThinkingEnabled bool `yaml:"thinking_enabled"`

	// Model is the LLM model to use (default depends on base URL / provider path)
	Model string `yaml:"model"`

	// BaseURL is the API base URL (default Anthropic; may also target local Ollama compatibility)
	BaseURL string `yaml:"base_url"`

	// Timeout is the API call timeout (default 15s)
	Timeout time.Duration `yaml:"timeout"`

	// APIKeyEnv is the environment variable holding the API key (default "ANTHROPIC_API_KEY")
	APIKeyEnv string `yaml:"api_key_env"`
}

// HookConfig holds settings for a UserPromptSubmit hook.
type HookConfig struct {
	// SkipScoreThreshold skips enhancement if the prompt already scores >= this (default 75, 0 = always enhance)
	SkipScoreThreshold int `yaml:"skip_score_threshold"`

	// MinWordCount skips prompts shorter than this (default 5)
	MinWordCount int `yaml:"min_word_count"`

	// SkipPatterns are additional regex patterns that cause the hook to skip enhancement
	SkipPatterns []string `yaml:"skip_patterns"`
}

// Rule is a pattern-matched augmentation rule
type Rule struct {
	Match   string `yaml:"match"`   // regex pattern on the prompt
	Prepend string `yaml:"prepend"` // context to add before the prompt
	Append  string `yaml:"append"`  // context to add after the prompt
}

// LoadConfig loads configuration from .prompt-improver.yaml in the given directory.
// Returns a zero Config if the file does not exist.
func LoadConfig(dir string) Config {
	var cfg Config

	paths := []string{
		filepath.Join(dir, ".prompt-improver.yaml"),
		filepath.Join(dir, ".prompt-improver.yml"),
		filepath.Join(dir, ".claude", "prompt-improver.yaml"),
		filepath.Join(dir, ".claude", "prompt-improver.yml"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		return cfg
	}

	return cfg
}

// configFound returns true if the config has any non-zero fields,
// distinguishing "found a config file" from "no config file at all".
func configFound(cfg Config) bool {
	return cfg.Preamble != "" ||
		len(cfg.Rules) > 0 ||
		len(cfg.BlockPatterns) > 0 ||
		len(cfg.DisabledStages) > 0 ||
		cfg.DefaultTaskType != "" ||
		cfg.DefaultEffort != "" ||
		cfg.Hook.SkipScoreThreshold != 0 ||
		cfg.Hook.MinWordCount != 0 ||
		len(cfg.Hook.SkipPatterns) > 0 ||
		cfg.LLM.Enabled ||
		cfg.LLM.Model != "" ||
		cfg.LLM.BaseURL != "" ||
		cfg.LLM.Timeout != 0 ||
		cfg.LLM.APIKeyEnv != "" ||
		cfg.LLM.ThinkingEnabled
}

// LoadConfigWithFallback loads config from the project directory first,
// then falls back to the user's home directory if no project config exists.
func LoadConfigWithFallback(projectDir string) Config {
	cfg := LoadConfig(projectDir)
	if configFound(cfg) {
		return cfg
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	return LoadConfig(home)
}

// ResolveConfig loads config with fallback and applies environment variable overrides.
// PROMPT_IMPROVER_LLM=1 enables LLM mode; PROMPT_IMPROVER_LLM=0 disables it.
// PROMPT_IMPROVER_MODEL overrides the LLM model.
func ResolveConfig(projectDir string) Config {
	cfg := LoadConfigWithFallback(projectDir)

	if v := os.Getenv("PROMPT_IMPROVER_LLM"); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes":
			cfg.LLM.Enabled = true
		case "0", "false", "no":
			cfg.LLM.Enabled = false
		}
	}

	if m := os.Getenv("PROMPT_IMPROVER_MODEL"); m != "" {
		cfg.LLM.Model = m
	}

	if t := os.Getenv("PROMPT_IMPROVER_TIMEOUT"); t != "" {
		if d, err := time.ParseDuration(t); err == nil {
			cfg.LLM.Timeout = d
		}
	}

	return cfg
}

// ValidateConfig checks for potential misconfiguration and returns a list of warnings.
// Callers decide whether to treat warnings as fatal or informational.
func ValidateConfig(cfg Config) []string {
	var warnings []string

	if cfg.LLM.Timeout != 0 && cfg.LLM.Timeout < 5*time.Second {
		warnings = append(warnings, fmt.Sprintf("LLM timeout %v is very short (< 5s) — API calls may time out", cfg.LLM.Timeout))
	}
	if cfg.LLM.Timeout > 120*time.Second {
		warnings = append(warnings, fmt.Sprintf("LLM timeout %v is very long (> 120s) — consider reducing to avoid blocking", cfg.LLM.Timeout))
	}

	if cfg.Hook.SkipScoreThreshold < 0 || cfg.Hook.SkipScoreThreshold > 100 {
		warnings = append(warnings, fmt.Sprintf("skip_score_threshold %d is out of range (0-100)", cfg.Hook.SkipScoreThreshold))
	}

	if cfg.Hook.MinWordCount < 0 {
		warnings = append(warnings, fmt.Sprintf("min_word_count %d is negative", cfg.Hook.MinWordCount))
	}

	if cfg.LLM.Enabled && cfg.LLM.Model == "" {
		warnings = append(warnings, "LLM is enabled but model name is empty — will use default")
	}

	return warnings
}

// IsStageDisabled checks if a pipeline stage is disabled in config
func (c Config) IsStageDisabled(stage string) bool {
	for _, s := range c.DisabledStages {
		if strings.EqualFold(s, stage) {
			return true
		}
	}
	return false
}

// ApplyRules applies matching rules to the prompt, returning modified text
func (c Config) ApplyRules(text string) (string, []string) {
	if len(c.Rules) == 0 {
		return text, nil
	}

	var improvements []string
	for _, rule := range c.Rules {
		if rule.Match == "" {
			continue
		}
		lower := strings.ToLower(text)
		matchLower := strings.ToLower(rule.Match)

		// Simple substring match (not regex for safety)
		if !strings.Contains(lower, matchLower) {
			continue
		}

		if rule.Prepend != "" {
			text = rule.Prepend + "\n\n" + text
			improvements = append(improvements, "Applied config rule: prepended context for '"+rule.Match+"'")
		}
		if rule.Append != "" {
			text = text + "\n\n" + rule.Append
			improvements = append(improvements, "Applied config rule: appended context for '"+rule.Match+"'")
		}
	}

	return text, improvements
}
