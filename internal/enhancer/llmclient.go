package enhancer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// LLMClient calls the Claude Messages API to improve prompts using a meta-prompt.
type LLMClient struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

// NewLLMClient always returns nil while hosted LLM improvement is disabled.
func NewLLMClient(cfg LLMConfig) *LLMClient {
	_ = cfg
	return nil
}

// ImproveOptions configures the LLM improvement request.
type ImproveOptions struct {
	ThinkingEnabled bool
	TaskType        TaskType
	Feedback        string // optional targeted hints
}

// ImproveResult holds the LLM-improved prompt and metadata.
type ImproveResult struct {
	Enhanced     string   `json:"enhanced"`
	TaskType     string   `json:"task_type"`
	Improvements []string `json:"improvements"`
}

// messagesRequest is the Claude Messages API request body.
type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []message `json:"messages"`
}

// message is a single message in the Messages API conversation.
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// messagesResponse is the relevant portion of the Claude Messages API response.
type messagesResponse struct {
	Content []contentBlock `json:"content"`
	Error   *apiError      `json:"error,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type apiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Improve sends the prompt to Claude with the meta-prompt and returns the improved version.
func (c *LLMClient) Improve(ctx context.Context, prompt string, opts ImproveOptions) (*ImproveResult, error) {
	systemPrompt := MetaPrompt
	if opts.ThinkingEnabled {
		systemPrompt = MetaPromptWithThinking
	}

	userContent := prompt
	if opts.Feedback != "" {
		userContent += "\n\n[Additional guidance: " + opts.Feedback + "]"
	}

	reqBody := messagesRequest{
		Model:     c.Model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages: []message{
			{Role: "user", Content: userContent},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp messagesResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("api error: %s: %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	// Extract text from content blocks
	var enhanced strings.Builder
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			enhanced.WriteString(block.Text)
		}
	}

	result := &ImproveResult{
		Enhanced:     strings.TrimSpace(enhanced.String()),
		TaskType:     string(opts.TaskType),
		Improvements: []string{"LLM-powered improvement via Claude Messages API"},
	}

	return result, nil
}
