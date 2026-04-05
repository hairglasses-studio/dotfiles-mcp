package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestClaudeSessionModuleRegistration(t *testing.T) {
	m := &ClaudeSessionModule{}

	if m.Name() != "claude_session" {
		t.Fatalf("expected name claude_session, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) != 8 {
		t.Fatalf("expected 8 tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"claude_crash_detect",
		"claude_session_scan",
		"claude_session_detail",
		"claude_session_logs",
		"claude_session_tag",
		"claude_repo_status",
		"claude_repo_diff",
		"claude_recovery_report",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestClaudeSessionDeferredInDefaultProfile(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "default")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg)

	// All claude_* tools should be deferred in default profile.
	for _, name := range []string{
		"claude_crash_detect",
		"claude_session_scan",
		"claude_repo_status",
	} {
		if !reg.IsDeferred(name) {
			t.Errorf("expected %s to be deferred in default profile", name)
		}
	}
}

func TestEncodeRepoPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/hg/hairglasses-studio", "-home-hg-hairglasses-studio"},
		{"/home/hg/hairglasses-studio/dotfiles", "-home-hg-hairglasses-studio-dotfiles"},
		{"/tmp/test", "-tmp-test"},
	}
	for _, tc := range tests {
		got := encodeRepoPath(tc.input)
		if got != tc.want {
			t.Errorf("encodeRepoPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseDurationString(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		err   bool
	}{
		{"2h", 2 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"1d", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"", 0, false},
		{"invalid", 0, true},
	}
	for _, tc := range tests {
		got, err := parseDurationString(tc.input)
		if tc.err && err == nil {
			t.Errorf("parseDurationString(%q) expected error", tc.input)
		}
		if !tc.err && err != nil {
			t.Errorf("parseDurationString(%q) unexpected error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("parseDurationString(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIsProcessAlive(t *testing.T) {
	// Current process should be alive.
	if !isProcessAlive(os.Getpid()) {
		t.Error("expected current process to be alive")
	}
	// PID 0 and negative should be dead.
	if isProcessAlive(0) {
		t.Error("expected PID 0 to be dead")
	}
	if isProcessAlive(-1) {
		t.Error("expected PID -1 to be dead")
	}
	// Very high PID should be dead.
	if isProcessAlive(999999999) {
		t.Error("expected very high PID to be dead")
	}
}

func TestReadJSONLTail(t *testing.T) {
	// Create a temp JSONL file.
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"a","n":1}`,
		`{"type":"b","n":2}`,
		`{"type":"c","n":3}`,
		`{"type":"d","n":4}`,
		`{"type":"e","n":5}`,
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Read last 3 entries.
	entries, err := readJSONLTail(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0]["type"] != "c" {
		t.Errorf("expected first entry type=c, got %v", entries[0]["type"])
	}
	if entries[2]["type"] != "e" {
		t.Errorf("expected last entry type=e, got %v", entries[2]["type"])
	}

	// Read more than available.
	entries, err = readJSONLTail(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
}

func TestReadJSONLTail_CorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	content := `{"type":"a"}
not json
{"type":"b"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := readJSONLTail(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 valid entries (skipping corrupt), got %d", len(entries))
	}
}

func TestMsToRFC3339(t *testing.T) {
	// Known timestamp: 1775380058118 ms
	result := msToRFC3339(1775380058118)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	// Should parse back.
	if _, err := time.Parse(time.RFC3339, result); err != nil {
		t.Errorf("result is not valid RFC3339: %s", result)
	}
}

func TestMsToRFC3339_Zero(t *testing.T) {
	if got := msToRFC3339(0); got != "" {
		t.Errorf("expected empty for 0, got %q", got)
	}
}

func TestSummarizeLogEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry map[string]any
		want  string
	}{
		{
			name: "user message",
			entry: map[string]any{
				"type":      "user",
				"timestamp": "2026-04-05T02:00:00Z",
				"message":   map[string]any{"content": "hello world"},
			},
			want: "hello world",
		},
		{
			name: "permission-mode",
			entry: map[string]any{
				"type":           "permission-mode",
				"permissionMode": "bypassPermissions",
			},
			want: "[permission-mode: bypassPermissions]",
		},
		{
			name: "system subtype",
			entry: map[string]any{
				"type":    "system",
				"subtype": "local_command",
			},
			want: "[system: local_command]",
		},
		{
			name: "file-history-snapshot",
			entry: map[string]any{
				"type": "file-history-snapshot",
			},
			want: "[file-history-snapshot]",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			le := summarizeLogEntry(tc.entry)
			if le.Summary != tc.want {
				t.Errorf("got summary=%q, want %q", le.Summary, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if got := truncate(short, 10); got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}

	long := "abcdefghij"
	if got := truncate(long, 5); got != "abcde..." {
		t.Errorf("expected %q, got %q", "abcde...", got)
	}

	multiline := "line1\nline2\nline3"
	got := truncate(multiline, 100)
	if got != "line1 line2 line3" {
		t.Errorf("expected collapsed newlines, got %q", got)
	}
}

// TestClaudeRepoStatus_NonExistent verifies error handling for missing paths.
func TestClaudeRepoStatus_NonExistent(t *testing.T) {
	m := &ClaudeSessionModule{}
	td := findClaudeTool(t, m, "claude_repo_status")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo_path": "/nonexistent/path/that/does/not/exist",
	}

	// TypedHandler returns errors in the result content, not as Go errors.
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for nonexistent path")
	}
}

// TestClaudeRepoStatus_TempGitRepo tests against a real temp git repo.
func TestClaudeRepoStatus_TempGitRepo(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo.
	runTestGit(t, dir, "init")
	runTestGit(t, dir, "config", "user.email", "test@test.com")
	runTestGit(t, dir, "config", "user.name", "Test")

	// Create a file and commit.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, dir, "add", "README.md")
	runTestGit(t, dir, "commit", "-m", "initial commit")

	// Create an unstaged change.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Modified"), 0644); err != nil {
		t.Fatal(err)
	}

	m := &ClaudeSessionModule{}
	td := findClaudeTool(t, m, "claude_repo_status")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo_path": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out RepoStatusOutput
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal: %v; text=%s", err, text)
	}

	if !out.IsGitRepo {
		t.Error("expected is_git_repo=true")
	}
	if out.LastCommitMsg != "initial commit" {
		t.Errorf("expected last commit msg 'initial commit', got %q", out.LastCommitMsg)
	}
	if out.UnstagedCount != 1 {
		t.Errorf("expected 1 unstaged file, got %d", out.UnstagedCount)
	}
	if !out.HasUncommitted {
		t.Error("expected has_uncommitted=true")
	}
}

// TestClaudeRepoDiff_TempGitRepo tests diff detection.
func TestClaudeRepoDiff_TempGitRepo(t *testing.T) {
	dir := t.TempDir()

	runTestGit(t, dir, "init")
	runTestGit(t, dir, "config", "user.email", "test@test.com")
	runTestGit(t, dir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, dir, "add", "file.txt")
	runTestGit(t, dir, "commit", "-m", "init")

	m := &ClaudeSessionModule{}
	td := findClaudeTool(t, m, "claude_repo_diff")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo_path": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out RepoDiffOutput
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal: %v; text=%s", err, text)
	}

	if !out.IsGitRepo {
		t.Error("expected is_git_repo=true")
	}
}

// TestLoadSessionTasks_MockFixture tests task loading from a mock directory.
func TestLoadSessionTasks_MockFixture(t *testing.T) {
	dir := t.TempDir()

	// Override claude dir for test.
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	defer os.Setenv("HOME", origHome)

	taskDir := filepath.Join(dir, ".claude", "tasks", "test-session-123")
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatal(err)
	}

	tasks := []SessionTask{
		{ID: "1", Subject: "Build feature A", Status: "completed"},
		{ID: "2", Subject: "Write tests", Status: "in_progress"},
		{ID: "3", Subject: "Deploy", Status: "pending"},
	}
	for _, task := range tasks {
		data, _ := json.Marshal(task)
		if err := os.WriteFile(filepath.Join(taskDir, task.ID+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	loaded, err := loadSessionTasks("test-session-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(loaded))
	}

	statusMap := make(map[string]string)
	for _, task := range loaded {
		statusMap[task.ID] = task.Status
	}
	if statusMap["1"] != "completed" {
		t.Errorf("task 1 should be completed, got %s", statusMap["1"])
	}
	if statusMap["2"] != "in_progress" {
		t.Errorf("task 2 should be in_progress, got %s", statusMap["2"])
	}
	if statusMap["3"] != "pending" {
		t.Errorf("task 3 should be pending, got %s", statusMap["3"])
	}
}

// TestSessionScan_MockFixture tests session scanning with mock session files.
func TestSessionScan_MockFixture(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	sessDir := filepath.Join(dir, ".claude", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file with a dead PID (very high number).
	meta := claudeSessionMeta{
		PID:       999999998,
		SessionID: "dead-session-001",
		CWD:       "/tmp/fake-repo",
		StartedAt: time.Now().Add(-1 * time.Hour).UnixMilli(),
		Kind:      "interactive",
		Name:      "test-session",
	}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(sessDir, "999999998.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create empty history.jsonl.
	if err := os.WriteFile(filepath.Join(dir, ".claude", "history.jsonl"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	sessions, err := scanSessions(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Status != "dead" {
		t.Errorf("expected status=dead, got %s", sessions[0].Status)
	}
	if sessions[0].SessionID != "dead-session-001" {
		t.Errorf("expected session ID dead-session-001, got %s", sessions[0].SessionID)
	}
	if sessions[0].Name != "test-session" {
		t.Errorf("expected name test-session, got %s", sessions[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func findClaudeTool(t *testing.T, m *ClaudeSessionModule, name string) registry.ToolDefinition {
	t.Helper()
	for _, td := range m.Tools() {
		if td.Tool.Name == name {
			return td
		}
	}
	t.Fatalf("tool %q not found", name)
	return registry.ToolDefinition{}
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := runGit(dir, args...); err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
}
