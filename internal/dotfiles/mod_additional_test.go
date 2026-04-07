package dotfiles

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// notify_send — input validation (handler-level)
// ---------------------------------------------------------------------------

func TestNotifySend_EmptyTitle(t *testing.T) {
	m := &NotifyModule{}
	td := findModuleTool(t, m, "notify_send")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"title": "",
		"body":  "test body",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty title")
	}
}

func TestNotifySend_InvalidUrgency(t *testing.T) {
	m := &NotifyModule{}
	td := findModuleTool(t, m, "notify_send")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"title":   "test",
		"body":    "test body",
		"urgency": "invalid-urgency",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for invalid urgency")
	}
}

// ---------------------------------------------------------------------------
// clipboard helpers
// ---------------------------------------------------------------------------

func TestClipCheckTool(t *testing.T) {
	// "ls" should always be on PATH.
	err := clipCheckTool("ls")
	if err != nil {
		t.Errorf("expected no error for 'ls', got: %v", err)
	}

	// Nonexistent tool.
	err = clipCheckTool("nonexistent-tool-abc-xyz-123")
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestClipRunCmd(t *testing.T) {
	// Simple command that should succeed.
	out, err := clipRunCmd("echo", "hello")
	if err != nil {
		t.Fatalf("clipRunCmd(echo hello) error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output from echo")
	}

	// Command that fails.
	_, err = clipRunCmd("false")
	if err == nil {
		t.Error("expected error from 'false' command")
	}
}

// ---------------------------------------------------------------------------
// screen helpers
// ---------------------------------------------------------------------------

func TestScreenCheckTool(t *testing.T) {
	err := screenCheckTool("ls")
	if err != nil {
		t.Errorf("expected no error for 'ls', got: %v", err)
	}

	err = screenCheckTool("nonexistent-screen-tool-xyz")
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestScreenRunCmd(t *testing.T) {
	out, err := screenRunCmd("echo", "test")
	if err != nil {
		t.Fatalf("screenRunCmd(echo test) error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestScreenTimestamp(t *testing.T) {
	ts := screenTimestamp()
	if len(ts) < 15 {
		t.Errorf("timestamp too short: %q", ts)
	}
}

// ---------------------------------------------------------------------------
// mapping_helpers — listMappingProfiles with temp dir
// ---------------------------------------------------------------------------

func TestListMappingProfiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	// Create empty makima dir.
	os.MkdirAll(filepath.Join(dir, "makima"), 0755)

	profiles, err := listMappingProfiles()
	if err != nil {
		t.Fatalf("listMappingProfiles error: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestListMappingProfiles_WithProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	mDir := filepath.Join(dir, "makima")
	os.MkdirAll(mDir, 0755)
	os.WriteFile(filepath.Join(mDir, "Xbox Controller.toml"), []byte("[remap]\nBTN_SOUTH = [\"KEY_ENTER\"]\n"), 0644)

	profiles, err := listMappingProfiles()
	if err != nil {
		t.Fatalf("listMappingProfiles error: %v", err)
	}
	if len(profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(profiles))
	}
}

// ---------------------------------------------------------------------------
// dotfiles_eww_restart — handler exercises (will fail gracefully without eww)
// ---------------------------------------------------------------------------

func TestEwwRestart_Handler(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_eww_restart")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	// Handler should return a result (even if eww isn't running).
	if result == nil {
		t.Fatal("result is nil")
	}

	text := extractText(t, result)
	var out EwwRestartOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

// ---------------------------------------------------------------------------
// dotfiles_eww_status — handler exercises
// ---------------------------------------------------------------------------

func TestEwwStatus_Handler(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_eww_status")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}

	text := extractText(t, result)
	var out EwwStatusOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

// ---------------------------------------------------------------------------
// JSON round-trip for additional output types
// ---------------------------------------------------------------------------

func TestAdditionalOutputTypes_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "ValidateConfigOutput",
			fn: func(t *testing.T) {
				out := ValidateConfigOutput{
					Valid: true,
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded ValidateConfigOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if !decoded.Valid {
					t.Error("valid should be true")
				}
			},
		},
		{
			name: "ValidateConfigOutput_Invalid",
			fn: func(t *testing.T) {
				out := ValidateConfigOutput{
					Valid: false,
					Error: "unexpected eof",
					Line:  5,
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded ValidateConfigOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Valid {
					t.Error("valid should be false")
				}
				if decoded.Line != 5 {
					t.Errorf("line = %d, want 5", decoded.Line)
				}
			},
		},
		{
			name: "ReloadServiceOutput",
			fn: func(t *testing.T) {
				out := ReloadServiceOutput{
					Service: "hyprland",
					Success: true,
					Output:  "reloaded",
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded ReloadServiceOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if !decoded.Success {
					t.Error("success should be true")
				}
			},
		},
		{
			name: "CheckSymlinksOutput",
			fn: func(t *testing.T) {
				out := CheckSymlinksOutput{
					Symlinks: []SymlinkStatus{
						{Source: "/a", Target: "/b", Status: "healthy"},
						{Source: "/c", Target: "/d", Status: "broken", Actual: "/e"},
						{Source: "/f", Target: "/g", Status: "missing"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded CheckSymlinksOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Symlinks) != 3 {
					t.Errorf("symlinks = %d, want 3", len(decoded.Symlinks))
				}
			},
		},
		{
			name: "ListConfigsOutput",
			fn: func(t *testing.T) {
				out := ListConfigsOutput{
					Configs: []ConfigEntry{
						{Name: "ghostty", Path: "/dotfiles/ghostty", Format: "toml"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded ListConfigsOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Configs) != 1 {
					t.Errorf("configs = %d, want 1", len(decoded.Configs))
				}
			},
		},
		{
			name: "GHLocalSyncAuditOutput",
			fn: func(t *testing.T) {
				out := GHLocalSyncAuditOutput{
					Synced:     5,
					Orphaned:   1,
					Missing:    2,
					Mismatched: 1,
					Entries: []SyncAuditEntry{
						{Name: "r1", Status: "orphaned", Details: "no org repo"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHLocalSyncAuditOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Synced != 5 {
					t.Errorf("synced = %d, want 5", decoded.Synced)
				}
			},
		},
		{
			name: "GHBulkCloneOutput",
			fn: func(t *testing.T) {
				out := GHBulkCloneOutput{
					Results: []CloneResult{
						{Repo: "r1", Action: "cloned"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHBulkCloneOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Results) != 1 {
					t.Errorf("results = %d, want 1", len(decoded.Results))
				}
			},
		},
		{
			name: "GHCleanStaleOutput",
			fn: func(t *testing.T) {
				out := GHCleanStaleOutput{
					Results: []TransferResult{
						{Repo: "old-repo", Action: "removed"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHCleanStaleOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Results) != 1 {
					t.Errorf("results = %d, want 1", len(decoded.Results))
				}
			},
		},
		{
			name: "GHRecreateForkOutput",
			fn: func(t *testing.T) {
				out := GHRecreateForkOutput{
					Results: []TransferResult{
						{Repo: "fork-repo", Action: "recreated"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHRecreateForkOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Results) != 1 {
					t.Errorf("results = %d, want 1", len(decoded.Results))
				}
			},
		},
		{
			name: "GHOnboardReposOutput",
			fn: func(t *testing.T) {
				out := GHOnboardReposOutput{
					Results: []GHOnboardResult{
						{Repo: "user/repo", Action: "onboarded"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHOnboardReposOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Results) != 1 {
					t.Errorf("results = %d, want 1", len(decoded.Results))
				}
			},
		},
		{
			name: "GHBulkArchiveOutput",
			fn: func(t *testing.T) {
				out := GHBulkArchiveOutput{
					Results: []TransferResult{
						{Repo: "old-repo", Action: "archived"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHBulkArchiveOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Results) != 1 {
					t.Errorf("results = %d, want 1", len(decoded.Results))
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.fn)
	}
}

// ---------------------------------------------------------------------------
// prompt_score — raw text scoring (without hash)
// ---------------------------------------------------------------------------

func TestPromptScore_RawText(t *testing.T) {
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
		"prompt": "Write a Go function that parses TOML configuration files and returns a structured config object. Handle missing fields with sensible defaults. Add comprehensive error messages.",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result for raw text scoring")
	}

	text := extractTextFromResult(t, result)
	var out promptScoreOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Score < 0 || out.Score > 100 {
		t.Errorf("score = %d, should be 0-100", out.Score)
	}
	if out.Grade == "" {
		t.Error("grade should not be empty")
	}
}

func TestPromptScore_EmptyBothHashAndPrompt(t *testing.T) {
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
		"hash":   "",
		"prompt": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result when both hash and prompt are empty")
	}
}

// ---------------------------------------------------------------------------
// prompt_improve — input validation
// ---------------------------------------------------------------------------

func TestPromptImprove_EmptyBothHashAndPrompt(t *testing.T) {
	dir := t.TempDir()
	origBaseDir := promptsBaseDir
	promptsBaseDir = func() string { return dir }
	defer func() { promptsBaseDir = origBaseDir }()

	globalPromptIndex.mu.Lock()
	globalPromptIndex.records = make(map[string]*PromptRecord)
	globalPromptIndex.loaded = false
	globalPromptIndex.mu.Unlock()

	m := &PromptRegistryModule{}
	td := findModuleTool(t, m, "prompt_improve")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"hash":   "",
		"prompt": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result when both hash and prompt are empty")
	}
}

// ---------------------------------------------------------------------------
// Learn module — input validation (only what we can test without evtest)
// ---------------------------------------------------------------------------

func TestLearnModule_Registration(t *testing.T) {
	m := &LearnModule{}

	if m.Name() != "learn" {
		t.Fatalf("expected name learn, got %s", m.Name())
	}
	if m.Description() == "" {
		t.Fatal("expected non-empty description")
	}

	tools := m.Tools()
	if len(tools) == 0 {
		t.Fatal("expected at least 1 tool")
	}

	for _, td := range tools {
		if td.Tool.Name == "" {
			t.Error("tool has empty name")
		}
		if td.Handler == nil {
			t.Errorf("tool %q has nil handler", td.Tool.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// Screen module — record status (can test without wayshot/wf-recorder)
// ---------------------------------------------------------------------------

func TestScreenRecordStatus_NoActiveRecording(t *testing.T) {
	m := &ScreenModule{}
	var td registry.ToolDefinition
	for _, tool := range m.Tools() {
		if tool.Tool.Name == "screen_record_status" {
			td = tool
			break
		}
	}
	if td.Tool.Name == "" {
		t.Fatal("screen_record_status tool not found")
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}

	text := extractText(t, result)
	var out ScreenRecordStatusOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// No recording should be active in tests.
	if out.Active {
		t.Error("expected active=false when no recording is in progress")
	}
}
