package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
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
	if len(tools) != 16 {
		t.Fatalf("expected 16 tools, got %d", len(tools))
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
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

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
		{"/home/user/projects", "-home-user-projects"},
		{"/home/user/projects/dotfiles", "-home-user-projects-dotfiles"},
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

// TestClaudeSessionModuleHas15Tools verifies the expanded tool count.
func TestClaudeSessionModuleHas16Tools(t *testing.T) {
	m := &ClaudeSessionModule{}
	tools := m.Tools()
	if len(tools) != 16 {
		t.Fatalf("expected 16 tools, got %d", len(tools))
	}

	for _, want := range []string{
		"claude_session_health",
		"claude_session_compare",
		"claude_session_replay",
		"claude_workspace_snapshot",
		"claude_repo_roadmap_status",
		"claude_recovery_history",
		"claude_fleet_recovery",
	} {
		found := false
		for _, td := range tools {
			if td.Tool.Name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// TestClaudeSessionHealth_MockFixture tests health scoring with mock data.
func TestClaudeSessionHealth_MockFixture(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Create a dead session.
	sessDir := filepath.Join(dir, ".claude", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := claudeSessionMeta{
		PID: 999999997, SessionID: "health-test-001",
		CWD: dir, StartedAt: time.Now().Add(-2 * time.Hour).UnixMilli(),
		Kind: "interactive", Name: "health-test",
	}
	data, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(sessDir, "999999997.json"), data, 0644)

	// Create history.
	histEntry := fmt.Sprintf(`{"sessionId":"health-test-001","timestamp":%d,"display":"test"}`, time.Now().Add(-30*time.Minute).UnixMilli())
	os.WriteFile(filepath.Join(dir, ".claude", "history.jsonl"), []byte(histEntry+"\n"), 0644)

	// Create tasks.
	taskDir := filepath.Join(dir, ".claude", "tasks", "health-test-001")
	os.MkdirAll(taskDir, 0755)
	for _, task := range []SessionTask{
		{ID: "1", Subject: "Done", Status: "completed"},
		{ID: "2", Subject: "Open", Status: "pending"},
	} {
		d, _ := json.Marshal(task)
		os.WriteFile(filepath.Join(taskDir, task.ID+".json"), d, 0644)
	}

	m := &ClaudeSessionModule{}
	td := findClaudeTool(t, m, "claude_session_health")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"session_id": "health-test-001"}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out SessionHealthOutput
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v; text=%s", err, text)
	}

	if out.IsAlive {
		t.Error("expected dead session")
	}
	if out.OverallScore < 0 || out.OverallScore > 100 {
		t.Errorf("score out of range: %d", out.OverallScore)
	}
	if len(out.Dimensions) != 5 {
		t.Errorf("expected 5 dimensions, got %d", len(out.Dimensions))
	}
	if len(out.Recommendations) == 0 {
		t.Error("expected at least one recommendation for dead session")
	}
}

// TestClaudeSessionCompare_SameSession tests comparing a session to itself.
func TestClaudeSessionCompare_SameSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	sessDir := filepath.Join(dir, ".claude", "sessions")
	os.MkdirAll(sessDir, 0755)
	os.WriteFile(filepath.Join(dir, ".claude", "history.jsonl"), []byte(""), 0644)

	for _, m := range []claudeSessionMeta{
		{PID: 999999991, SessionID: "cmp-001", CWD: "/tmp/repo-a", StartedAt: time.Now().UnixMilli(), Kind: "interactive"},
		{PID: 999999992, SessionID: "cmp-002", CWD: "/tmp/repo-a", StartedAt: time.Now().UnixMilli(), Kind: "interactive"},
	} {
		d, _ := json.Marshal(m)
		os.WriteFile(filepath.Join(sessDir, fmt.Sprintf("%d.json", m.PID)), d, 0644)
	}

	mod := &ClaudeSessionModule{}
	td := findClaudeTool(t, mod, "claude_session_compare")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"session_id_1": "cmp-001", "session_id_2": "cmp-002"}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out SessionCompareOutput
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v; text=%s", err, text)
	}
	if !out.SameRepo {
		t.Error("expected same_repo=true")
	}
}

// TestClaudeRepoRoadmapStatus_ParsesCheckboxes tests ROADMAP.md parsing.
func TestClaudeRepoRoadmapStatus_ParsesCheckboxes(t *testing.T) {
	dir := t.TempDir()

	roadmap := `# ROADMAP
- [x] Build feature A
- [x] Write tests
- [ ] Deploy to production
- [ ] Update docs
`
	os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte(roadmap), 0644)

	m := &ClaudeSessionModule{}
	td := findClaudeTool(t, m, "claude_repo_roadmap_status")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"repo_path": dir}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out RepoRoadmapOutput
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v; text=%s", err, text)
	}

	if out.TotalCount != 4 {
		t.Errorf("expected 4 items, got %d", out.TotalCount)
	}
	if out.CompletedCount != 2 {
		t.Errorf("expected 2 completed, got %d", out.CompletedCount)
	}
	if out.ProgressPercent != 50 {
		t.Errorf("expected 50%%, got %d%%", out.ProgressPercent)
	}
}

// TestClaudeRecoveryHistory_EmptyHistory tests with no history file.
func TestClaudeRecoveryHistory_EmptyHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// No history.jsonl file.
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)

	m := &ClaudeSessionModule{}
	td := findClaudeTool(t, m, "claude_recovery_history")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"days": 7}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out RecoveryHistoryOutput
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v; text=%s", err, text)
	}
	if out.Total != 0 {
		t.Errorf("expected 0 events, got %d", out.Total)
	}
}

// TestClaudeFleetRecovery_DryRun tests fleet recovery in dry-run mode.
func TestClaudeFleetRecovery_DryRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	sessDir := filepath.Join(dir, ".claude", "sessions")
	os.MkdirAll(sessDir, 0755)
	os.WriteFile(filepath.Join(dir, ".claude", "history.jsonl"), []byte(""), 0644)

	// Create 2 dead sessions.
	for i, m := range []claudeSessionMeta{
		{PID: 999999993, SessionID: "fleet-001", CWD: "/tmp/repo-a", StartedAt: time.Now().UnixMilli(), Kind: "interactive", Name: "session-a"},
		{PID: 999999994, SessionID: "fleet-002", CWD: "/tmp/repo-b", StartedAt: time.Now().UnixMilli(), Kind: "interactive", Name: "session-b"},
	} {
		d, _ := json.Marshal(m)
		os.WriteFile(filepath.Join(sessDir, fmt.Sprintf("%d.json", 999999993+i)), d, 0644)
	}

	mod := &ClaudeSessionModule{}
	td := findClaudeTool(t, mod, "claude_fleet_recovery")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"window": "4h"}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out FleetRecoveryOutput
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v; text=%s", err, text)
	}

	if out.TotalDeadSessions != 2 {
		t.Errorf("expected 2 dead sessions, got %d", out.TotalDeadSessions)
	}
	if !out.DryRun {
		t.Error("expected dry_run=true")
	}
	if len(out.Steps) < 2 {
		t.Errorf("expected at least 2 steps, got %d", len(out.Steps))
	}
}

// TestClaudeWorkspaceSnapshot_CreatesFile tests snapshot creation.
func TestClaudeWorkspaceSnapshot_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Create minimal claude dir.
	os.MkdirAll(filepath.Join(dir, ".claude", "sessions"), 0755)
	os.WriteFile(filepath.Join(dir, ".claude", "history.jsonl"), []byte(""), 0644)

	// Create a fake studio dir with a git repo.
	studioDir := filepath.Join(dir, "studio")
	repoDir := filepath.Join(studioDir, "test-repo")
	os.MkdirAll(repoDir, 0755)
	runTestGit(t, repoDir, "init")
	runTestGit(t, repoDir, "config", "user.email", "t@t.com")
	runTestGit(t, repoDir, "config", "user.name", "T")
	os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("x"), 0644)
	runTestGit(t, repoDir, "add", "f.txt")
	runTestGit(t, repoDir, "commit", "-m", "init")

	m := &ClaudeSessionModule{}
	td := findClaudeTool(t, m, "claude_workspace_snapshot")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"studio_path": studioDir}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out WorkspaceSnapshotOutput
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v; text=%s", err, text)
	}

	if out.SnapshotID == "" {
		t.Error("expected non-empty snapshot_id")
	}
	if out.RepoCount != 1 {
		t.Errorf("expected 1 repo, got %d", out.RepoCount)
	}
	if out.Size == 0 {
		t.Error("expected non-zero size")
	}
	// Check file exists.
	if _, err := os.Stat(out.SavedTo); err != nil {
		t.Errorf("snapshot file not found: %s", out.SavedTo)
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
