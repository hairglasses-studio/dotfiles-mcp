package enhancer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestLLMServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := messagesResponse{
			Content: []contentBlock{{Type: "text", Text: response}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func newTestEngine(serverURL string) *HybridEngine {
	return &HybridEngine{
		Client: &LLMClient{
			APIKey:     "test-key",
			Model:      "claude-sonnet-4-6",
			BaseURL:    serverURL,
			HTTPClient: &http.Client{Timeout: 5 * time.Second},
		},
		CB:    NewCircuitBreaker(),
		Cache: NewPromptCache(),
		Cfg:   LLMConfig{Enabled: true, Timeout: 5 * time.Second},
	}
}

func TestEnhanceHybrid_LocalMode(t *testing.T) {
	result := EnhanceHybrid(context.Background(), "fix the bug in the authentication module", "", Config{}, nil, ModeLocal)
	if result.Source != "local" {
		t.Errorf("expected source 'local', got %q", result.Source)
	}
	if result.Enhanced == "" {
		t.Error("expected non-empty enhanced prompt")
	}
}

func TestEnhanceHybrid_NilEngineFallsBackToLocal(t *testing.T) {
	result := EnhanceHybrid(context.Background(), "fix the bug in the authentication module", "", Config{}, nil, ModeAuto)
	if result.Source != "local" {
		t.Errorf("expected source 'local' when engine is nil, got %q", result.Source)
	}
}

func TestEnhanceHybrid_AutoModeLLMSuccess(t *testing.T) {
	server := newTestLLMServer("You are an expert. Improved prompt here.")
	defer server.Close()

	engine := newTestEngine(server.URL)
	result := EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, ModeAuto)

	if result.Source != "llm" {
		t.Errorf("expected source 'llm', got %q", result.Source)
	}
	if result.Enhanced != "You are an expert. Improved prompt here." {
		t.Errorf("unexpected enhanced: %q", result.Enhanced)
	}
}

func TestEnhanceHybrid_LLMModeLLMSuccess(t *testing.T) {
	server := newTestLLMServer("LLM improved prompt")
	defer server.Close()

	engine := newTestEngine(server.URL)
	result := EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, ModeLLM)

	if result.Source != "llm" {
		t.Errorf("expected source 'llm', got %q", result.Source)
	}
	if result.Enhanced != "LLM improved prompt" {
		t.Errorf("unexpected enhanced: %q", result.Enhanced)
	}
}

func TestEnhanceHybrid_CacheHit(t *testing.T) {
	server := newTestLLMServer("first call result")
	defer server.Close()

	engine := newTestEngine(server.URL)

	// First call — populates cache
	result1 := EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, ModeAuto)
	if result1.Source != "llm" {
		t.Fatalf("expected first call source 'llm', got %q", result1.Source)
	}

	// Second call — should hit cache
	result2 := EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, ModeAuto)
	if result2.Source != "llm_cached" {
		t.Errorf("expected source 'llm_cached', got %q", result2.Source)
	}
	if result2.Enhanced != result1.Enhanced {
		t.Errorf("cached result doesn't match: %q vs %q", result2.Enhanced, result1.Enhanced)
	}
}

func TestEnhanceHybrid_CircuitBreakerOpenAutoFallback(t *testing.T) {
	engine := &HybridEngine{
		Client: &LLMClient{
			APIKey:     "test-key",
			Model:      "claude-sonnet-4-6",
			BaseURL:    "http://localhost:1", // will fail
			HTTPClient: &http.Client{Timeout: 100 * time.Millisecond},
		},
		CB:    NewCircuitBreaker(),
		Cache: NewPromptCache(),
		Cfg:   LLMConfig{Enabled: true, Timeout: 100 * time.Millisecond},
	}

	// Trip the circuit breaker
	engine.CB.RecordFailure()
	engine.CB.RecordFailure()
	engine.CB.RecordFailure()

	result := EnhanceHybrid(context.Background(), "fix the bug in the authentication module", "", Config{}, engine, ModeAuto)
	if result.Source != "local_fallback" {
		t.Errorf("expected source 'local_fallback', got %q", result.Source)
	}
	assertContains(t, result.Improvements[len(result.Improvements)-1], "circuit breaker")
}

func TestEnhanceHybrid_CircuitBreakerOpenLLMMode(t *testing.T) {
	engine := &HybridEngine{
		Client: &LLMClient{APIKey: "test-key"},
		CB:     NewCircuitBreaker(),
		Cache:  NewPromptCache(),
		Cfg:    LLMConfig{Enabled: true},
	}

	engine.CB.RecordFailure()
	engine.CB.RecordFailure()
	engine.CB.RecordFailure()

	result := EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, ModeLLM)
	if result.Source != "error" {
		t.Errorf("expected source 'error', got %q", result.Source)
	}
	assertContains(t, result.Improvements[0], "circuit breaker")
}

func TestEnhanceHybrid_LLMFailureAutoFallback(t *testing.T) {
	// Server returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"type":"server_error","message":"internal error"}}`))
	}))
	defer server.Close()

	engine := newTestEngine(server.URL)
	result := EnhanceHybrid(context.Background(), "fix the bug in the authentication module", "", Config{}, engine, ModeAuto)

	if result.Source != "local_fallback" {
		t.Errorf("expected source 'local_fallback', got %q", result.Source)
	}
	assertContains(t, result.Improvements[len(result.Improvements)-1], "LLM failed")
}

func TestEnhanceHybrid_LLMFailureLLMMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"type":"server_error","message":"internal error"}}`))
	}))
	defer server.Close()

	engine := newTestEngine(server.URL)
	result := EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, ModeLLM)

	if result.Source != "error" {
		t.Errorf("expected source 'error', got %q", result.Source)
	}
	if result.Enhanced != "fix the bug" {
		t.Errorf("expected original prompt returned on LLM-only error, got %q", result.Enhanced)
	}
}

func TestEnhanceHybrid_DefaultModeIsAuto(t *testing.T) {
	server := newTestLLMServer("improved")
	defer server.Close()

	engine := newTestEngine(server.URL)
	result := EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, "")

	if result.Source != "llm" {
		t.Errorf("expected source 'llm' for default (auto) mode, got %q", result.Source)
	}
}

func TestEnhanceHybrid_RecordsSuccessOnCircuitBreaker(t *testing.T) {
	server := newTestLLMServer("improved")
	defer server.Close()

	engine := newTestEngine(server.URL)
	// Add some failures (but not enough to open)
	engine.CB.RecordFailure()
	engine.CB.RecordFailure()

	EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, ModeAuto)

	// After success, circuit breaker should be closed with 0 failures
	if engine.CB.State() != "closed" {
		t.Errorf("expected circuit breaker closed after success, got %s", engine.CB.State())
	}
}

func TestEnhanceHybrid_RecordsFailureOnCircuitBreaker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"type":"server_error","message":"fail"}}`))
	}))
	defer server.Close()

	engine := newTestEngine(server.URL)
	EnhanceHybrid(context.Background(), "fix the bug in the authentication module", "", Config{}, engine, ModeAuto)
	EnhanceHybrid(context.Background(), "another prompt for the thing", "", Config{}, engine, ModeAuto)
	EnhanceHybrid(context.Background(), "third prompt to trigger breaker", "", Config{}, engine, ModeAuto)

	if engine.CB.State() != "open" {
		t.Errorf("expected circuit breaker open after 3 failures, got %s", engine.CB.State())
	}
}

func TestValidMode(t *testing.T) {
	tests := []struct {
		input string
		want  EnhanceMode
	}{
		{"local", ModeLocal},
		{"llm", ModeLLM},
		{"auto", ModeAuto},
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := ValidMode(tt.input)
		if got != tt.want {
			t.Errorf("ValidMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
