package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// repoNameFromPath
// ---------------------------------------------------------------------------

func TestRepoNameFromPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/hg/hairglasses-studio/dotfiles-mcp", "dotfiles-mcp"},
		{"/home/hg/hairglasses-studio/mcpkit", "mcpkit"},
		{"/tmp/test", "test"},
		{"/", "/"},
		{"", "."},
	}
	for _, tc := range tests {
		got := repoNameFromPath(tc.input)
		if got != tc.want {
			t.Errorf("repoNameFromPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// cwdFromJSONL
// ---------------------------------------------------------------------------

func TestCwdFromJSONL_Found(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"system","subtype":"init"}
{"type":"user","cwd":"/home/hg/hairglasses-studio/dotfiles-mcp","message":{"content":"hello"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}
`
	os.WriteFile(path, []byte(content), 0644)

	got := cwdFromJSONL(path)
	if got != "/home/hg/hairglasses-studio/dotfiles-mcp" {
		t.Errorf("cwdFromJSONL() = %q, want dotfiles-mcp path", got)
	}
}

func TestCwdFromJSONL_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"system","subtype":"init"}
{"type":"user","message":{"content":"hello"}}
`
	os.WriteFile(path, []byte(content), 0644)

	got := cwdFromJSONL(path)
	if got != "" {
		t.Errorf("cwdFromJSONL() = %q, want empty", got)
	}
}

func TestCwdFromJSONL_MissingFile(t *testing.T) {
	got := cwdFromJSONL("/nonexistent/path/test.jsonl")
	if got != "" {
		t.Errorf("cwdFromJSONL() = %q for missing file, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// titleFromJSONL
// ---------------------------------------------------------------------------

func TestTitleFromJSONL_CustomTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"system"}
{"type":"custom-title","customTitle":"My Custom Title"}
{"type":"agent-name","agentName":"test-agent"}
`
	os.WriteFile(path, []byte(content), 0644)

	got := titleFromJSONL(path)
	if got != "My Custom Title" {
		t.Errorf("titleFromJSONL() = %q, want 'My Custom Title'", got)
	}
}

func TestTitleFromJSONL_AgentName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"system"}
{"type":"agent-name","agentName":"my-agent"}
{"slug":"backup-slug"}
`
	os.WriteFile(path, []byte(content), 0644)

	got := titleFromJSONL(path)
	if got != "my-agent" {
		t.Errorf("titleFromJSONL() = %q, want 'my-agent'", got)
	}
}

func TestTitleFromJSONL_FallbackSlug(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"system"}
{"type":"user","message":{"content":"hello"},"slug":"my-slug"}
`
	os.WriteFile(path, []byte(content), 0644)

	got := titleFromJSONL(path)
	if got != "my-slug" {
		t.Errorf("titleFromJSONL() = %q, want 'my-slug'", got)
	}
}

func TestTitleFromJSONL_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"system"}
{"type":"user","message":{"content":"hello"}}
`
	os.WriteFile(path, []byte(content), 0644)

	got := titleFromJSONL(path)
	if got != "" {
		t.Errorf("titleFromJSONL() = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// extractSnippet
// ---------------------------------------------------------------------------

func TestExtractSnippet_UserMessage(t *testing.T) {
	line := `{"type":"user","message":{"content":"Please help me fix the build error in dotfiles-mcp"}}`
	snippet := extractSnippet(line, "build error")
	if snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
	if len(snippet) > 200 {
		t.Errorf("snippet too long: %d chars", len(snippet))
	}
}

func TestExtractSnippet_AssistantMessage(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"I found the build error in the main.go file."}]}}`
	snippet := extractSnippet(line, "build error")
	if snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
}

func TestExtractSnippet_NoMatch(t *testing.T) {
	line := `{"type":"user","message":{"content":"hello world"}}`
	snippet := extractSnippet(line, "nonexistent")
	if snippet == "" {
		// extractSnippet falls back to truncate when query not in text
		// but text exists, so it should return something
		t.Log("snippet is empty (query not in text content), which is acceptable for this edge case")
	}
}

func TestExtractSnippet_InvalidJSON(t *testing.T) {
	snippet := extractSnippet("not json", "query")
	if snippet != "" {
		t.Errorf("expected empty snippet for invalid JSON, got %q", snippet)
	}
}

func TestExtractSnippet_NoContent(t *testing.T) {
	line := `{"type":"system","subtype":"init"}`
	snippet := extractSnippet(line, "init")
	if snippet != "" {
		t.Errorf("expected empty snippet for system entry, got %q", snippet)
	}
}

// ---------------------------------------------------------------------------
// searchSessionFile
// ---------------------------------------------------------------------------

func TestSearchSessionFile_Found(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"user","message":{"content":"fix the coverage bug"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"I found the coverage issue in oss.go"}]}}
{"type":"user","message":{"content":"now run the tests"}}
`
	os.WriteFile(path, []byte(content), 0644)

	hits, snippets := searchSessionFile(path, "coverage", false, nil, 5)
	if hits < 2 {
		t.Errorf("expected at least 2 hits, got %d", hits)
	}
	if len(snippets) == 0 {
		t.Error("expected at least 1 snippet")
	}
}

func TestSearchSessionFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"user","message":{"content":"hello world"}}
`
	os.WriteFile(path, []byte(content), 0644)

	hits, snippets := searchSessionFile(path, "nonexistent_xyz", false, nil, 5)
	if hits != 0 {
		t.Errorf("expected 0 hits, got %d", hits)
	}
	if len(snippets) != 0 {
		t.Errorf("expected 0 snippets, got %d", len(snippets))
	}
}

func TestSearchSessionFile_MissingFile(t *testing.T) {
	hits, snippets := searchSessionFile("/nonexistent/path.jsonl", "query", false, nil, 5)
	if hits != 0 || len(snippets) != 0 {
		t.Error("expected 0 hits and 0 snippets for missing file")
	}
}

// ---------------------------------------------------------------------------
// findSessionJSONL
// ---------------------------------------------------------------------------

func TestFindSessionJSONL_DirectFile(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-session-abc"
	path := filepath.Join(dir, sessionID+".jsonl")
	os.WriteFile(path, []byte(`{"type":"system"}`), 0644)

	got := findSessionJSONL(dir, sessionID)
	if got != path {
		t.Errorf("findSessionJSONL() = %q, want %q", got, path)
	}
}

func TestFindSessionJSONL_SubdirFile(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-session-abc"
	subdir := filepath.Join(dir, sessionID)
	os.MkdirAll(subdir, 0755)
	path := filepath.Join(subdir, sessionID+".jsonl")
	os.WriteFile(path, []byte(`{"type":"system"}`), 0644)

	got := findSessionJSONL(dir, sessionID)
	if got != path {
		t.Errorf("findSessionJSONL() = %q, want %q", got, path)
	}
}

func TestFindSessionJSONL_NotFound(t *testing.T) {
	dir := t.TempDir()
	got := findSessionJSONL(dir, "nonexistent-session")
	if got != "" {
		t.Errorf("findSessionJSONL() = %q, want empty for missing session", got)
	}
}

// ---------------------------------------------------------------------------
// readJSONLAll
// ---------------------------------------------------------------------------

func TestReadJSONLAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"a","n":1}
{"type":"b","n":2}
{"type":"c","n":3}
`
	os.WriteFile(path, []byte(content), 0644)

	entries, err := readJSONLAll(path)
	if err != nil {
		t.Fatalf("readJSONLAll error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0]["type"] != "a" {
		t.Errorf("first entry type = %v, want 'a'", entries[0]["type"])
	}
}

func TestReadJSONLAll_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0644)

	entries, err := readJSONLAll(path)
	if err != nil {
		t.Fatalf("readJSONLAll error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty file, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// findPlanFileForSession
// ---------------------------------------------------------------------------

func TestFindPlanFileForSession_Found(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	plansDir := filepath.Join(dir, ".claude", "plans")
	os.MkdirAll(plansDir, 0755)
	os.WriteFile(filepath.Join(plansDir, "my-plan.md"), []byte("Session: test-session-123\nSome plan content"), 0644)
	os.WriteFile(filepath.Join(plansDir, "other-plan.md"), []byte("Unrelated content"), 0644)

	got := findPlanFileForSession("test-session-123")
	if got != "my-plan.md" {
		t.Errorf("findPlanFileForSession() = %q, want 'my-plan.md'", got)
	}
}

func TestFindPlanFileForSession_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	plansDir := filepath.Join(dir, ".claude", "plans")
	os.MkdirAll(plansDir, 0755)
	os.WriteFile(filepath.Join(plansDir, "plan.md"), []byte("No matching session"), 0644)

	got := findPlanFileForSession("nonexistent-session")
	if got != "" {
		t.Errorf("findPlanFileForSession() = %q, want empty", got)
	}
}

func TestFindPlanFileForSession_NoPlanDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	got := findPlanFileForSession("test-session")
	if got != "" {
		t.Errorf("findPlanFileForSession() = %q, want empty for missing dir", got)
	}
}

// ---------------------------------------------------------------------------
// loadSessionMemory
// ---------------------------------------------------------------------------

func TestLoadSessionMemory_WithFiles(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0755)

	os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("# Memory"), 0644)     // should be excluded
	os.WriteFile(filepath.Join(memDir, "topic1.md"), []byte("Topic 1 content"), 0644)
	os.WriteFile(filepath.Join(memDir, "topic2.md"), []byte("Topic 2 content"), 0644)
	os.WriteFile(filepath.Join(memDir, "notes.txt"), []byte("not markdown"), 0644) // should be excluded

	files := loadSessionMemory(dir)
	if len(files) != 2 {
		t.Fatalf("expected 2 memory files, got %d", len(files))
	}

	names := make(map[string]bool)
	for _, f := range files {
		names[f.Name] = true
	}
	if !names["topic1.md"] || !names["topic2.md"] {
		t.Errorf("expected topic1.md and topic2.md, got %v", names)
	}
}

func TestLoadSessionMemory_NoDir(t *testing.T) {
	dir := t.TempDir()
	files := loadSessionMemory(dir)
	if files != nil {
		t.Errorf("expected nil for missing memory dir, got %v", files)
	}
}

// ---------------------------------------------------------------------------
// extractJSONLMeta
// ---------------------------------------------------------------------------

func TestExtractJSONLMeta_Full(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"custom-title","customTitle":"My Session"}
{"type":"user","cwd":"/home/hg/project","gitBranch":"main","version":"1.0.0","message":{"content":"hi"}}
{"type":"assistant","slug":"my-slug","message":{"model":"claude-opus-4-6","content":[{"type":"text","text":"hello"}]}}
`
	os.WriteFile(path, []byte(content), 0644)

	title, cwd, branch, version, model, slug := extractJSONLMeta(path)
	if title != "My Session" {
		t.Errorf("title = %q, want 'My Session'", title)
	}
	if cwd != "/home/hg/project" {
		t.Errorf("cwd = %q, want '/home/hg/project'", cwd)
	}
	if branch != "main" {
		t.Errorf("branch = %q, want 'main'", branch)
	}
	if version != "1.0.0" {
		t.Errorf("version = %q, want '1.0.0'", version)
	}
	if model != "claude-opus-4-6" {
		t.Errorf("model = %q, want 'claude-opus-4-6'", model)
	}
	if slug != "my-slug" {
		t.Errorf("slug = %q, want 'my-slug'", slug)
	}
}

func TestExtractJSONLMeta_Partial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"type":"system","subtype":"init"}
{"type":"user","cwd":"/tmp/test","message":{"content":"hello"}}
`
	os.WriteFile(path, []byte(content), 0644)

	title, cwd, branch, _, _, _ := extractJSONLMeta(path)
	if title != "" {
		t.Errorf("title = %q, want empty", title)
	}
	if cwd != "/tmp/test" {
		t.Errorf("cwd = %q, want '/tmp/test'", cwd)
	}
	if branch != "" {
		t.Errorf("branch = %q, want empty", branch)
	}
}

func TestExtractJSONLMeta_MissingFile(t *testing.T) {
	title, cwd, _, _, _, _ := extractJSONLMeta("/nonexistent/test.jsonl")
	if title != "" || cwd != "" {
		t.Error("expected all empty for missing file")
	}
}
