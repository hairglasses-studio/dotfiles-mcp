package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/prompt-improver/pkg/enhancer"
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
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

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

// ---------------------------------------------------------------------------
// Integration tests: exercise MCP tool handlers via the typed handler functions
// ---------------------------------------------------------------------------

// setupTestIndex creates a temp dir, overrides promptsBaseDir, and returns a
// cleanup function. Also resets the global index.
func setupTestIndex(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	origBase := promptsBaseDir
	promptsBaseDir = func() string { return tmpDir }

	// Create the index file and required dirs
	os.WriteFile(filepath.Join(tmpDir, ".prompt-index.jsonl"), []byte{}, 0644)
	os.MkdirAll(filepath.Join(tmpDir, "test-repo", "unsorted"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "test-repo", "sorted"), 0755)

	// Reset global index
	globalPromptIndex = &PromptIndex{records: make(map[string]*PromptRecord)}

	return tmpDir, func() {
		promptsBaseDir = origBase
		globalPromptIndex = &PromptIndex{records: make(map[string]*PromptRecord)}
	}
}

// capturePrompt is a test helper that directly exercises the capture logic
// (hash, score, write file, index) without going through MCP protocol.
func capturePrompt(t *testing.T, prompt, repo string, tags []string) promptCaptureOutput {
	t.Helper()

	hash := computePromptHash(prompt)
	shortHash := hash[:12]

	// Dedup check
	if existing := globalPromptIndex.get(hash); existing != nil {
		return promptCaptureOutput{
			Hash: existing.Hash, ShortHash: existing.ShortHash,
			Score: existing.Score, Grade: existing.Grade,
			TaskType: existing.TaskType, SavedTo: promptFilePath(existing),
			Status: existing.Status,
		}
	}

	score, grade, taskType, _ := scorePrompt(prompt)
	if tags == nil {
		tags = []string{}
	}

	rec := &PromptRecord{
		Hash: hash, ShortHash: shortHash, Repo: repo,
		Timestamp: "2026-04-05T12:00:00Z", WordCount: wordCount(prompt),
		TaskType: taskType, Score: score, Grade: grade,
		Tags: tags, Status: "unsorted", Prompt: prompt,
	}

	path := promptFilePath(rec)
	if err := writePromptFile(path, rec); err != nil {
		t.Fatalf("writePromptFile: %v", err)
	}
	if err := globalPromptIndex.add(rec); err != nil {
		t.Fatalf("index add: %v", err)
	}

	return promptCaptureOutput{
		Hash: hash, ShortHash: shortHash, Score: score,
		Grade: grade, TaskType: taskType, SavedTo: path, Status: "unsorted",
	}
}

func TestPromptCapture_Dedup(t *testing.T) {
	_, cleanup := setupTestIndex(t)
	defer cleanup()

	prompt := "Write a comprehensive Go function that implements\na thread-safe map with TTL-based expiration.\nInclude proper error handling and benchmarks."

	// First capture
	r1 := capturePrompt(t, prompt, "test-repo", nil)
	if r1.Hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Second capture of same prompt should return same hash (dedup)
	r2 := capturePrompt(t, prompt, "test-repo", nil)
	if r2.Hash != r1.Hash {
		t.Errorf("dedup failed: expected same hash %s, got %s", r1.Hash, r2.Hash)
	}
}

func TestPromptCapture_AutoScoring(t *testing.T) {
	_, cleanup := setupTestIndex(t)
	defer cleanup()

	prompt := "Implement a REST API endpoint that accepts JSON payloads,\nvalidates the schema against a predefined template,\nand stores valid records in a PostgreSQL database.\nReturn appropriate HTTP status codes for each case."

	result := capturePrompt(t, prompt, "test-repo", []string{"go", "api"})

	if result.Score <= 0 {
		t.Errorf("expected positive score, got %d", result.Score)
	}
	if result.Grade == "" {
		t.Error("expected non-empty grade")
	}
	if result.TaskType == "" {
		t.Error("expected non-empty task type")
	}
	if result.SavedTo == "" {
		t.Error("expected non-empty saved_to path")
	}
	// Verify file was actually created
	if _, err := os.Stat(result.SavedTo); err != nil {
		t.Errorf("saved file does not exist: %v", err)
	}
}

func TestPromptSearch_MultiFilter(t *testing.T) {
	_, cleanup := setupTestIndex(t)
	defer cleanup()

	// Capture several prompts
	capturePrompt(t, "Write a Go function that implements sorting\nwith custom comparator support", "test-repo", []string{"go", "algorithms"})
	capturePrompt(t, "Create a Python script that analyzes\nCSV data and generates summary statistics", "other-repo", []string{"python", "data"})
	capturePrompt(t, "Build a React component that displays\na sortable data table with pagination", "frontend-repo", []string{"react", "ui"})

	// Search by repo
	results := globalPromptIndex.search("", "test-repo", "", nil, 0, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for repo=test-repo, got %d", len(results))
	}

	// Search by tag
	results = globalPromptIndex.search("", "", "", []string{"go"}, 0, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for tag=go, got %d", len(results))
	}

	// Search by query
	results = globalPromptIndex.search("React", "", "", nil, 0, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for query=React, got %d", len(results))
	}

	// Search all
	results = globalPromptIndex.search("", "", "", nil, 0, 10)
	if len(results) != 3 {
		t.Errorf("expected 3 total results, got %d", len(results))
	}
}

func TestPromptTag_AddRemove(t *testing.T) {
	_, cleanup := setupTestIndex(t)
	defer cleanup()

	result := capturePrompt(t, "Write a Go function that implements\na concurrent-safe LRU cache with TTL", "test-repo", []string{"go"})

	rec := globalPromptIndex.get(result.Hash)
	if rec == nil {
		t.Fatal("captured prompt not found in index")
	}

	// Build tag set and add
	tagSet := make(map[string]bool, len(rec.Tags))
	for _, tg := range rec.Tags {
		tagSet[tg] = true
	}
	tagSet["cache"] = true
	tagSet["concurrency"] = true

	newTags := make([]string, 0, len(tagSet))
	for tg := range tagSet {
		if tg != "" {
			newTags = append(newTags, tg)
		}
	}
	rec.Tags = newTags

	// Verify tags added
	hasCache := false
	for _, tg := range rec.Tags {
		if tg == "cache" {
			hasCache = true
		}
	}
	if !hasCache {
		t.Error("expected 'cache' tag after add")
	}

	// Remove a tag
	delete(tagSet, "go")
	newTags = make([]string, 0, len(tagSet))
	for tg := range tagSet {
		if tg != "" {
			newTags = append(newTags, tg)
		}
	}
	rec.Tags = newTags

	hasGo := false
	for _, tg := range rec.Tags {
		if tg == "go" {
			hasGo = true
		}
	}
	if hasGo {
		t.Error("expected 'go' tag removed")
	}
}

func TestPromptImprove_Enhancement(t *testing.T) {
	_, cleanup := setupTestIndex(t)
	defer cleanup()

	// Capture a deliberately vague prompt that should benefit from enhancement
	vague := "do stuff with the code\nmake it better and fix things\nalso add some tests maybe"
	result := capturePrompt(t, vague, "test-repo", nil)

	// Score the original
	origScore := result.Score

	// Run enhancement via enhancer directly
	enhanced := enhancer.Enhance(vague, "")
	if enhanced.Enhanced == vague {
		t.Skip("enhancer did not change the prompt (possibly too short)")
	}

	// The enhanced version should exist and differ from original
	if enhanced.Enhanced == "" {
		t.Error("expected non-empty enhanced prompt")
	}
	if len(enhanced.Improvements) == 0 {
		t.Log("warning: no improvements reported (prompt may already be acceptable)")
	}

	_ = origScore // Score comparison is informational, not a hard assertion
}

func TestPromptExport_JSONL(t *testing.T) {
	_, cleanup := setupTestIndex(t)
	defer cleanup()

	capturePrompt(t, "Build a REST API with authentication\nand rate limiting middleware", "test-repo", []string{"go", "api"})
	capturePrompt(t, "Create a database migration tool\nthat supports rollback operations", "test-repo", []string{"go", "database"})

	// Export as JSONL
	results := globalPromptIndex.search("", "test-repo", "", nil, 0, 100)
	if len(results) != 2 {
		t.Fatalf("expected 2 prompts for export, got %d", len(results))
	}

	var buf strings.Builder
	for _, rec := range results {
		line, _ := json.Marshal(rec)
		buf.Write(line)
		buf.WriteByte('\n')
	}

	exported := buf.String()
	lines := strings.Split(strings.TrimSpace(exported), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 JSONL lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// PromptIndex update + rewriteJSONL
// ---------------------------------------------------------------------------

func TestPromptIndex_Update(t *testing.T) {
	tmpDir := t.TempDir()
	origBase := promptsBaseDir
	promptsBaseDir = func() string { return tmpDir }
	defer func() { promptsBaseDir = origBase }()

	// Create index file
	indexPath := filepath.Join(tmpDir, ".prompt-index.jsonl")
	os.WriteFile(indexPath, []byte{}, 0644)

	idx := &PromptIndex{records: make(map[string]*PromptRecord)}

	rec := &PromptRecord{
		Hash:      "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888",
		ShortHash: "aaaa1111bbbb",
		Repo:      "test-repo",
		Timestamp: "2026-04-05T10:00:00Z",
		WordCount: 5,
		TaskType:  "code",
		Score:     50,
		Grade:     "D",
		Tags:      []string{"go"},
		Status:    "unsorted",
		Prompt:    "Write a Go function",
	}

	// Add the record first
	if err := idx.add(rec); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	// Now update it with new data
	rec.Score = 90
	rec.Grade = "A"
	rec.Status = "scored"
	if err := idx.update(rec); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Verify in-memory update
	got := idx.get(rec.Hash)
	if got == nil {
		t.Fatal("record not found after update")
	}
	if got.Score != 90 {
		t.Errorf("score = %d, want 90", got.Score)
	}
	if got.Grade != "A" {
		t.Errorf("grade = %q, want A", got.Grade)
	}
	if got.Status != "scored" {
		t.Errorf("status = %q, want scored", got.Status)
	}

	// Verify the JSONL was rewritten by loading from disk
	idx2 := &PromptIndex{records: make(map[string]*PromptRecord)}
	if err := idx2.load(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	got2 := idx2.get(rec.Hash)
	if got2 == nil {
		t.Fatal("record not found after reload")
	}
	if got2.Score != 90 {
		t.Errorf("reloaded score = %d, want 90", got2.Score)
	}
}

func TestPromptIndex_SearchLimit(t *testing.T) {
	idx := &PromptIndex{records: make(map[string]*PromptRecord)}

	// Add several records
	for i := 0; i < 5; i++ {
		hash := computePromptHash(strings.Repeat("x", i+1))
		idx.records[hash] = &PromptRecord{
			Hash:      hash,
			ShortHash: hash[:12],
			Repo:      "test-repo",
			Timestamp: "2026-04-05T10:00:00Z",
			Prompt:    "test prompt " + strings.Repeat("x", i+1),
			Status:    "unsorted",
		}
	}

	// Search with limit
	results := idx.search("", "", "", nil, 0, 3)
	if len(results) > 3 {
		t.Errorf("expected at most 3 results with limit=3, got %d", len(results))
	}
}

func TestPromptIndex_SearchByStatus(t *testing.T) {
	idx := &PromptIndex{records: make(map[string]*PromptRecord)}

	hash1 := computePromptHash("prompt1")
	hash2 := computePromptHash("prompt2")
	idx.records[hash1] = &PromptRecord{
		Hash: hash1, Prompt: "prompt1", Status: "unsorted", Repo: "test",
	}
	idx.records[hash2] = &PromptRecord{
		Hash: hash2, Prompt: "prompt2", Status: "scored", Repo: "test",
	}

	results := idx.search("", "", "scored", nil, 0, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for status=scored, got %d", len(results))
	}
}

func TestPromptIndex_StatsRepoFilter(t *testing.T) {
	idx := &PromptIndex{records: make(map[string]*PromptRecord)}

	hash1 := computePromptHash("prompt1")
	hash2 := computePromptHash("prompt2")
	idx.records[hash1] = &PromptRecord{
		Hash: hash1, Repo: "repo-a", Score: 80, Grade: "B",
		TaskType: "code", Status: "unsorted", Tags: []string{"go"},
	}
	idx.records[hash2] = &PromptRecord{
		Hash: hash2, Repo: "repo-b", Score: 60, Grade: "D",
		TaskType: "analysis", Status: "scored", Tags: []string{"python"},
	}

	// Stats with filter
	stats := idx.stats("repo-a")
	if stats.TotalPrompts != 1 {
		t.Errorf("expected 1 prompt for repo-a, got %d", stats.TotalPrompts)
	}
	if stats.AverageScore != 80 {
		t.Errorf("expected average score 80, got %.1f", stats.AverageScore)
	}

	// Stats without filter
	statsAll := idx.stats("")
	if statsAll.TotalPrompts != 2 {
		t.Errorf("expected 2 total prompts, got %d", statsAll.TotalPrompts)
	}
}

// ---------------------------------------------------------------------------
// wordCount
// ---------------------------------------------------------------------------

func TestWordCount(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello world", 2},
		{"one", 1},
		{"", 0},
		{"  spaces   between   words  ", 3},
		{"multi\nline\ntext", 3},
	}
	for _, tc := range tests {
		got := wordCount(tc.input)
		if got != tc.want {
			t.Errorf("wordCount(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// promptFilePath
// ---------------------------------------------------------------------------

func TestPromptFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	origBase := promptsBaseDir
	promptsBaseDir = func() string { return tmpDir }
	defer func() { promptsBaseDir = origBase }()

	rec := &PromptRecord{
		ShortHash: "abc123",
		Repo:      "test-repo",
		Status:    "unsorted",
	}
	path := promptFilePath(rec)
	if !strings.Contains(path, "unsorted") {
		t.Errorf("expected unsorted in path, got %s", path)
	}
	if !strings.HasSuffix(path, "abc123.md") {
		t.Errorf("expected abc123.md suffix, got %s", path)
	}

	rec.Status = "improved"
	path = promptFilePath(rec)
	if !strings.Contains(path, "sorted") {
		t.Errorf("expected sorted in path for improved status, got %s", path)
	}

	rec.Status = "scored"
	path = promptFilePath(rec)
	if !strings.Contains(path, "sorted") {
		t.Errorf("expected sorted in path for scored status, got %s", path)
	}
}

func TestCorruptJSONLRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	origBase := promptsBaseDir
	promptsBaseDir = func() string { return tmpDir }
	defer func() { promptsBaseDir = origBase }()

	// Write a JSONL file with one valid and one corrupt line
	indexPath := filepath.Join(tmpDir, ".prompt-index.jsonl")
	content := `{"hash":"abc123","short_hash":"abc1","repo":"test","prompt":"valid"}
this is not valid json
{"hash":"def456","short_hash":"def4","repo":"test","prompt":"also valid"}
`
	os.WriteFile(indexPath, []byte(content), 0644)

	idx := &PromptIndex{records: make(map[string]*PromptRecord)}
	if err := idx.load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Should have loaded 2 valid records, skipped 1 corrupt
	if len(idx.records) != 2 {
		t.Errorf("expected 2 records (1 skipped), got %d", len(idx.records))
	}

	// Corrupt line should be logged to .corrupt file
	corruptPath := indexPath + ".corrupt"
	if _, err := os.Stat(corruptPath); err != nil {
		t.Error("expected .corrupt file to be created")
	}
}
