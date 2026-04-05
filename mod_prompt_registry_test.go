package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestPromptRegistryModuleRegistration(t *testing.T) {
	m := &PromptRegistryModule{}

	if m.Name() != "prompt_registry" {
		t.Fatalf("expected name prompt_registry, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) != 8 {
		t.Fatalf("expected 8 tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"prompt_capture",
		"prompt_search",
		"prompt_get",
		"prompt_tag",
		"prompt_score",
		"prompt_improve",
		"prompt_stats",
		"prompt_export",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestPromptRegistryDeferredInDefaultProfile(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "default")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg)

	for _, name := range []string{
		"prompt_capture",
		"prompt_search",
		"prompt_score",
	} {
		if !reg.IsDeferred(name) {
			t.Errorf("expected %s to be deferred in default profile", name)
		}
	}
}

func TestComputePromptHash(t *testing.T) {
	hash1 := computePromptHash("test prompt")
	hash2 := computePromptHash("test prompt")
	hash3 := computePromptHash("different prompt")

	if hash1 != hash2 {
		t.Error("same input should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different input should produce different hash")
	}
	if len(hash1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(hash1))
	}
}

func TestPromptIndex(t *testing.T) {
	// Use a temp dir for test data
	tmpDir := t.TempDir()
	origBase := promptsBaseDir
	promptsBaseDir = func() string { return tmpDir }
	defer func() { promptsBaseDir = origBase }()

	// Create the index file
	indexPath := filepath.Join(tmpDir, ".prompt-index.jsonl")
	os.WriteFile(indexPath, []byte{}, 0644)

	// Reset global index for test isolation
	idx := &PromptIndex{records: make(map[string]*PromptRecord)}

	rec := &PromptRecord{
		Hash:      "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		ShortHash: "abcdef123456",
		Repo:      "test-repo",
		Timestamp: "2026-04-05T10:00:00Z",
		WordCount: 10,
		TaskType:  "code",
		Score:     75,
		Grade:     "C",
		Tags:      []string{"go", "mcp"},
		Status:    "unsorted",
		Prompt:    "Write a Go function that handles MCP tool registration",
	}

	// Test add
	if err := idx.add(rec); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	// Test get by full hash
	got := idx.get(rec.Hash)
	if got == nil {
		t.Fatal("get by full hash returned nil")
	}
	if got.Repo != "test-repo" {
		t.Errorf("expected repo test-repo, got %s", got.Repo)
	}

	// Test get by short hash
	got = idx.get("abcdef123456")
	if got == nil {
		t.Fatal("get by short hash returned nil")
	}

	// Test search
	results := idx.search("MCP tool", "", "", nil, 0, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	}

	// Test search with repo filter
	results = idx.search("", "other-repo", "", nil, 0, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for wrong repo, got %d", len(results))
	}

	// Test search with tag filter
	results = idx.search("", "", "", []string{"go"}, 0, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for tag 'go', got %d", len(results))
	}

	// Test search with min score filter
	results = idx.search("", "", "", nil, 80, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for min_score 80, got %d", len(results))
	}

	// Test stats
	stats := idx.stats("")
	if stats.TotalPrompts != 1 {
		t.Errorf("expected 1 total prompt, got %d", stats.TotalPrompts)
	}
	if stats.ByRepo["test-repo"] != 1 {
		t.Error("expected 1 prompt in test-repo")
	}
}

func TestWriteAndReadPromptFile(t *testing.T) {
	tmpDir := t.TempDir()

	rec := &PromptRecord{
		Hash:      "deadbeef1234567890deadbeef1234567890deadbeef1234567890deadbeef12",
		ShortHash: "deadbeef1234",
		Repo:      "test",
		Timestamp: "2026-04-05T12:00:00Z",
		WordCount: 8,
		TaskType:  "analysis",
		Score:     60,
		Grade:     "D",
		Tags:      []string{"test", "analysis"},
		Status:    "unsorted",
		Prompt:    "Analyze the performance of the database queries",
	}

	path := filepath.Join(tmpDir, "test.md")
	if err := writePromptFile(path, rec); err != nil {
		t.Fatalf("writePromptFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Read back the prompt text
	text, err := readPromptFile(path)
	if err != nil {
		t.Fatalf("readPromptFile failed: %v", err)
	}
	if text != rec.Prompt {
		t.Errorf("expected prompt %q, got %q", rec.Prompt, text)
	}
}

func TestScorePrompt(t *testing.T) {
	// A well-structured prompt should score > 0
	prompt := `You are an expert Go developer. Review the following code for thread safety issues.

<context>
The module uses sync.RWMutex for concurrent access to a shared map.
</context>

<instructions>
1. Check all map access paths for proper lock/unlock
2. Verify no goroutine reads without RLock
3. Check for deadlock potential
</instructions>

<output_format>
Return findings as a markdown table with columns: Location, Issue, Severity, Fix.
</output_format>`

	score, grade, taskType, report := scorePrompt(prompt)
	if score <= 0 {
		t.Errorf("expected positive score for structured prompt, got %d", score)
	}
	if grade == "" {
		t.Error("expected non-empty grade")
	}
	if taskType == "" {
		t.Error("expected non-empty task type")
	}
	if report == nil {
		t.Error("expected non-nil score report")
	}
	if len(report.Dimensions) != 10 {
		t.Errorf("expected 10 dimensions, got %d", len(report.Dimensions))
	}
}
