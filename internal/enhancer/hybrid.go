package enhancer

import (
	"context"
	"fmt"
	"os"
	"time"
)

// EnhanceMode determines which enhancement strategy to use.
type EnhanceMode string

const (
	ModeLocal EnhanceMode = "local" // deterministic 13-stage pipeline
	ModeLLM   EnhanceMode = "llm"   // Claude Messages API + meta-prompt
	ModeAuto  EnhanceMode = "auto"  // try LLM, fall back to local
)

// ValidMode checks if a string is a valid enhance mode.
func ValidMode(s string) EnhanceMode {
	switch EnhanceMode(s) {
	case ModeLocal, ModeLLM, ModeAuto:
		return EnhanceMode(s)
	default:
		return ""
	}
}

// HybridEngine manages the LLM client, circuit breaker, and cache.
type HybridEngine struct {
	Client  *LLMClient
	CB      *CircuitBreaker
	Cache   *PromptCache
	Cfg     LLMConfig
}

// NewHybridEngine creates a hybrid engine from config.
// Returns nil if LLM is not enabled or no API key is available.
func NewHybridEngine(cfg LLMConfig) *HybridEngine {
	if !cfg.Enabled {
		return nil
	}

	client := NewLLMClient(cfg)
	if client == nil {
		return nil
	}

	return &HybridEngine{
		Client: client,
		CB:     NewCircuitBreaker(),
		Cache:  NewPromptCache(),
		Cfg:    cfg,
	}
}

// EnhanceHybrid runs the enhancement using the specified mode.
// For "auto" mode: tries LLM first, falls back to local pipeline on failure.
func EnhanceHybrid(ctx context.Context, prompt string, taskType TaskType, cfg Config, engine *HybridEngine, mode EnhanceMode) EnhanceResult {
	// Determine effective mode
	if mode == "" {
		mode = ModeAuto
	}

	// Local mode or no engine available
	if mode == ModeLocal || engine == nil {
		result := EnhanceWithConfig(prompt, taskType, cfg)
		result.Source = "local"
		return result
	}

	// LLM or Auto mode — try LLM
	opts := ImproveOptions{
		ThinkingEnabled: engine.Cfg.ThinkingEnabled,
		TaskType:        taskType,
	}

	// Check cache first
	if cached := engine.Cache.Get(prompt, opts); cached != nil {
		return EnhanceResult{
			Original:        prompt,
			Enhanced:        cached.Enhanced,
			TaskType:        TaskType(cached.TaskType),
			StagesRun:       []string{"llm_cached"},
			Improvements:    cached.Improvements,
			EstimatedTokens: EstimateTokens(cached.Enhanced),
			CostTier:        costTierForTokens(EstimateTokens(cached.Enhanced)),
			Source:          "llm_cached",
		}
	}

	// Check circuit breaker
	if !engine.CB.Allow() {
		if mode == ModeLLM {
			// LLM-only mode and circuit is open — return error result
			return EnhanceResult{
				Original:     prompt,
				Enhanced:     prompt,
				TaskType:     taskType,
				StagesRun:    []string{"llm_circuit_open"},
				Improvements: []string{"LLM circuit breaker is open — returning original prompt"},
				Source:       "error",
			}
		}
		// Auto mode — fall back to local
		result := EnhanceWithConfig(prompt, taskType, cfg)
		result.Source = "local_fallback"
		result.Improvements = append(result.Improvements, "LLM circuit breaker open — used local pipeline")
		return result
	}

	// Call LLM
	timeout := engine.Cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	llmCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	llmResult, err := engine.Client.Improve(llmCtx, prompt, opts)
	if err != nil {
		engine.CB.RecordFailure()
		fmt.Fprintf(os.Stderr, "prompt-improver: LLM enhancement failed: %v\n", err)

		if mode == ModeLLM {
			return EnhanceResult{
				Original:     prompt,
				Enhanced:     prompt,
				TaskType:     taskType,
				StagesRun:    []string{"llm_error"},
				Improvements: []string{"LLM enhancement failed: " + err.Error()},
				Source:       "error",
			}
		}
		// Auto mode — fall back to local
		result := EnhanceWithConfig(prompt, taskType, cfg)
		result.Source = "local_fallback"
		result.Improvements = append(result.Improvements, "LLM failed, used local pipeline: "+err.Error())
		return result
	}

	// Success
	engine.CB.RecordSuccess()
	engine.Cache.Put(prompt, opts, llmResult)

	return EnhanceResult{
		Original:        prompt,
		Enhanced:        llmResult.Enhanced,
		TaskType:        taskType,
		StagesRun:       []string{"llm"},
		Improvements:    llmResult.Improvements,
		EstimatedTokens: EstimateTokens(llmResult.Enhanced),
		CostTier:        costTierForTokens(EstimateTokens(llmResult.Enhanced)),
		Source:          "llm",
	}
}
