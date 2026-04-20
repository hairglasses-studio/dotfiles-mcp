package enhancer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLLMClient_Improve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected api key 'test-key', got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version 2023-06-01, got %q", r.Header.Get("anthropic-version"))
		}

		// Verify request body
		var req messagesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "claude-sonnet-4-6" {
			t.Errorf("expected model claude-sonnet-4-6, got %s", req.Model)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Error("expected single user message")
		}

		// Return mock response
		resp := messagesResponse{
			Content: []contentBlock{
				{Type: "text", Text: "You are an expert software engineer.\n\nImproved prompt here."},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &LLMClient{
		APIKey:     "test-key",
		Model:      "claude-sonnet-4-6",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	result, err := client.Improve(context.Background(), "fix this bug", ImproveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Enhanced == "" {
		t.Error("expected non-empty enhanced prompt")
	}
	if result.Enhanced != "You are an expert software engineer.\n\nImproved prompt here." {
		t.Errorf("unexpected enhanced: %q", result.Enhanced)
	}
}

func TestLLMClient_ImproveWithThinking(t *testing.T) {
	var receivedSystem string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req messagesRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedSystem = req.System

		resp := messagesResponse{
			Content: []contentBlock{{Type: "text", Text: "improved"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &LLMClient{
		APIKey:     "test-key",
		Model:      "claude-sonnet-4-6",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := client.Improve(context.Background(), "test", ImproveOptions{ThinkingEnabled: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedSystem == MetaPrompt {
		t.Error("expected thinking meta-prompt, got standard")
	}
	if receivedSystem != MetaPromptWithThinking {
		t.Error("expected MetaPromptWithThinking system prompt")
	}
}

func TestLLMClient_ImproveWithFeedback(t *testing.T) {
	var receivedContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req messagesRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 {
			receivedContent = req.Messages[0].Content
		}

		resp := messagesResponse{
			Content: []contentBlock{{Type: "text", Text: "improved"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &LLMClient{
		APIKey:     "test-key",
		Model:      "claude-sonnet-4-6",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := client.Improve(context.Background(), "fix the bug", ImproveOptions{
		Feedback: "focus on error handling",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, receivedContent, "fix the bug")
	assertContains(t, receivedContent, "focus on error handling")
}

func TestLLMClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"rate limited"}}`))
	}))
	defer server.Close()

	client := &LLMClient{
		APIKey:     "test-key",
		Model:      "claude-sonnet-4-6",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := client.Improve(context.Background(), "test", ImproveOptions{})
	if err == nil {
		t.Error("expected error for 429 response")
	}
	assertContains(t, err.Error(), "429")
}

func TestLLMClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // simulate slow response
		resp := messagesResponse{
			Content: []contentBlock{{Type: "text", Text: "too late"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &LLMClient{
		APIKey:     "test-key",
		Model:      "claude-sonnet-4-6",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Improve(ctx, "test", ImproveOptions{})
	if err == nil {
		t.Error("expected error from context cancellation")
	}
}

func TestNewLLMClient_NoAPIKey(t *testing.T) {
	client := NewLLMClient(LLMConfig{
		APIKeyEnv: "NONEXISTENT_KEY_FOR_TESTING_12345",
	})
	if client != nil {
		t.Error("expected nil client while LLM-backed improvement is disabled")
	}
}

func TestNewLLMClient_DisabledEvenWithConfiguredAnthropicKeyAndDefaultModel(t *testing.T) {
	client := NewLLMClient(LLMConfig{
		APIKeyEnv: "DISABLED_LLM_KEY",
	})
	if client != nil {
		t.Fatal("expected nil client while hosted LLM improvement is disabled")
	}
}

func TestNewLLMClient_DisabledEvenWhenDefaultingAnthropicBaseURL(t *testing.T) {
	client := NewLLMClient(LLMConfig{
		APIKeyEnv: "DISABLED_LLM_KEY",
	})
	if client != nil {
		t.Fatal("expected nil client while hosted LLM improvement is disabled")
	}
}
