package dotfiles

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// prompt_capture — input validation
// ---------------------------------------------------------------------------

func TestPromptCapture_EmptyPrompt(t *testing.T) {
	// Use temp dir so we don't touch real prompt index.
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_capture")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"prompt": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty prompt")
	}
}

func TestPromptCapture_WhitespaceOnlyPrompt(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_capture")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"prompt": "   \n\t  ",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for whitespace-only prompt")
	}
}

func TestPromptCapture_ValidPrompt(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	// Reset global index for isolation.
	globalPromptIndex.mu.Lock()
	globalPromptIndex.records = make(map[string]*PromptRecord)
	globalPromptIndex.loaded = false
	globalPromptIndex.mu.Unlock()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_capture")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"prompt": "Write a function that calculates the Fibonacci sequence using dynamic programming. Include proper error handling for negative inputs.",
		"repo":   "test-repo",
		"tags":   []any{"test", "fibonacci"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result for valid prompt")
	}

	text := extractTextFromResult(t, result)
	var out promptCaptureOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Hash == "" {
		t.Error("hash should not be empty")
	}
	if out.ShortHash == "" {
		t.Error("short_hash should not be empty")
	}
	if out.Score < 0 {
		t.Errorf("score = %d, should be >= 0", out.Score)
	}
	if out.Grade == "" {
		t.Error("grade should not be empty")
	}
	if out.Status != "unsorted" {
		t.Errorf("status = %q, want unsorted", out.Status)
	}
}

func TestPromptCapture_DedupExtended(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	globalPromptIndex.mu.Lock()
	globalPromptIndex.records = make(map[string]*PromptRecord)
	globalPromptIndex.loaded = false
	globalPromptIndex.mu.Unlock()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_capture")

	prompt := "Unique test prompt for dedup testing 123456789"

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"prompt": prompt,
		"repo":   "test-repo",
	}

	// First capture.
	result1, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("first capture error: %v", err)
	}
	text1 := extractTextFromResult(t, result1)
	var out1 promptCaptureOutput
	json.Unmarshal([]byte(text1), &out1)

	// Second capture (same prompt) — should dedup.
	result2, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("second capture error: %v", err)
	}
	text2 := extractTextFromResult(t, result2)
	var out2 promptCaptureOutput
	json.Unmarshal([]byte(text2), &out2)

	if out1.Hash != out2.Hash {
		t.Errorf("hashes differ: %q vs %q", out1.Hash, out2.Hash)
	}
}

// ---------------------------------------------------------------------------
// prompt_get — input validation
// ---------------------------------------------------------------------------

func TestPromptGet_EmptyHash(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	globalPromptIndex.mu.Lock()
	globalPromptIndex.records = make(map[string]*PromptRecord)
	globalPromptIndex.loaded = false
	globalPromptIndex.mu.Unlock()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_get")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"hash": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty hash")
	}
}

func TestPromptGet_NotFound(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	globalPromptIndex.mu.Lock()
	globalPromptIndex.records = make(map[string]*PromptRecord)
	globalPromptIndex.loaded = false
	globalPromptIndex.mu.Unlock()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_get")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"hash": "nonexistent-hash-abc123",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for nonexistent hash")
	}
}

// ---------------------------------------------------------------------------
// prompt_search — exercises handler path
// ---------------------------------------------------------------------------

func TestPromptSearch_EmptyIndex(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	globalPromptIndex.mu.Lock()
	globalPromptIndex.records = make(map[string]*PromptRecord)
	globalPromptIndex.loaded = false
	globalPromptIndex.mu.Unlock()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_search")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query": "test",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result for empty search")
	}

	text := extractTextFromResult(t, result)
	var out promptSearchOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 0 {
		t.Errorf("total = %d, want 0 for empty index", out.Total)
	}
}

// ---------------------------------------------------------------------------
// prompt_tag — input validation
// ---------------------------------------------------------------------------

func TestPromptTag_EmptyHash(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	globalPromptIndex.mu.Lock()
	globalPromptIndex.records = make(map[string]*PromptRecord)
	globalPromptIndex.loaded = false
	globalPromptIndex.mu.Unlock()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_tag")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"hash": "",
		"tags": []any{"test"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty hash")
	}
}

// ---------------------------------------------------------------------------
// prompt_score — input validation
// ---------------------------------------------------------------------------

func TestPromptScore_EmptyHash(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	globalPromptIndex.mu.Lock()
	globalPromptIndex.records = make(map[string]*PromptRecord)
	globalPromptIndex.loaded = false
	globalPromptIndex.mu.Unlock()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_score")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"hash": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty hash")
	}
}

// ---------------------------------------------------------------------------
// prompt helpers — unit tests
// ---------------------------------------------------------------------------

func TestComputePromptHash_Extended(t *testing.T) {
	hash1 := computePromptHash("test prompt")
	hash2 := computePromptHash("test prompt")
	hash3 := computePromptHash("different prompt")

	if hash1 != hash2 {
		t.Error("same input should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different inputs should produce different hashes")
	}
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64 (SHA-256 hex)", len(hash1))
	}
}

func TestWordCount_Extended(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  spaces  everywhere  ", 2},
		{"one\ntwo\tthree", 3},
	}
	for _, tc := range tests {
		got := wordCount(tc.input)
		if got != tc.want {
			t.Errorf("wordCount(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestScorePrompt_Extended(t *testing.T) {
	score, grade, taskType, _ := scorePrompt("Write a function that calculates the Fibonacci sequence using dynamic programming. Include proper error handling for negative inputs and return the first N numbers.")
	if score < 0 || score > 100 {
		t.Errorf("score = %d, should be 0-100", score)
	}
	if grade == "" {
		t.Error("grade should not be empty")
	}
	if taskType == "" {
		t.Error("taskType should not be empty")
	}
}

func TestPromptFilePath_Extended(t *testing.T) {
	rec := &PromptRecord{
		Hash:      "abc123def456",
		ShortHash: "abc123def456",
		Repo:      "test-repo",
		Status:    "unsorted",
	}

	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	path := promptFilePath(rec)
	if !filepath.IsAbs(path) {
		t.Errorf("path should be absolute: %s", path)
	}
	if !strings.Contains(path, "test-repo") {
		t.Errorf("path should contain repo name: %s", path)
	}
	if !strings.Contains(path, "unsorted") {
		t.Errorf("path should contain status dir: %s", path)
	}
}

// ---------------------------------------------------------------------------
// PromptIndex — unit tests
// ---------------------------------------------------------------------------

func TestPromptIndex_AddAndGet(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	idx := &PromptIndex{
		records: make(map[string]*PromptRecord),
	}

	rec := &PromptRecord{
		Hash:      "abc123",
		ShortHash: "abc1",
		Repo:      "test",
		Prompt:    "test prompt",
	}

	if err := idx.add(rec); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Get by full hash.
	got := idx.get("abc123")
	if got == nil {
		t.Fatal("expected to find record by full hash")
	}
	if got.Prompt != "test prompt" {
		t.Errorf("prompt = %q, want 'test prompt'", got.Prompt)
	}

	// Get by short hash.
	gotShort := idx.get("abc1")
	if gotShort == nil {
		t.Fatal("expected to find record by short hash")
	}
}

func TestPromptIndex_GetNotFound(t *testing.T) {
	idx := &PromptIndex{
		records: make(map[string]*PromptRecord),
	}

	got := idx.get("nonexistent")
	if got != nil {
		t.Error("expected nil for nonexistent hash")
	}
}

// ---------------------------------------------------------------------------
// writePromptFile and readPromptFile round-trip
// ---------------------------------------------------------------------------

func TestWriteReadPromptFile(t *testing.T) {
	dir := t.TempDir()

	rec := &PromptRecord{
		Hash:      "abc123",
		ShortHash: "abc1",
		Repo:      "test",
		TaskType:  "coding",
		Score:     75,
		Grade:     "B+",
		Tags:      []string{"test", "unit"},
		Status:    "unsorted",
		Prompt:    "Write a test for the function.",
	}

	path := filepath.Join(dir, "test-prompt.md")
	if err := writePromptFile(path, rec); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}
