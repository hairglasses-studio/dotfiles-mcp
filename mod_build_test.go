package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// dotfiles_pipeline_run — input validation
// ---------------------------------------------------------------------------

func TestPipelineRun_MissingRepoPath(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_pipeline_run")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo_path": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty repo_path")
	}
}

func TestPipelineRun_BuildOnlyFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_pipeline_run")

	// Script won't exist, but we verify the handler doesn't panic
	// and returns a result (with the script error).
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo_path":  "/tmp/some-repo",
		"build_only": true,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	// Result should have repo name extracted from path.
	text := extractText(t, result)
	var out PipelineRunOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Repo != "some-repo" {
		t.Errorf("repo = %q, want some-repo", out.Repo)
	}
}

func TestPipelineRun_TestOnlyFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_pipeline_run")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo_path": "/tmp/another-repo",
		"test_only": true,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := extractText(t, result)
	var out PipelineRunOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Repo != "another-repo" {
		t.Errorf("repo = %q, want another-repo", out.Repo)
	}
}

// ---------------------------------------------------------------------------
// dotfiles_create_repo — input validation
// ---------------------------------------------------------------------------

func TestCreateRepo_MissingName(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_create_repo")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty name")
	}
}

func TestCreateRepo_WithLanguageAndPrivate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_create_repo")

	// Script won't exist, but we test the handler doesn't panic.
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":     "test-new-repo",
		"language": "go",
		"private":  true,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	// Should return a result (likely fail status since script is missing).
	text := extractText(t, result)
	var out CreateRepoOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.RepoPath == "" {
		t.Error("repo_path should not be empty")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_cascade_reload — input validation (known services)
// ---------------------------------------------------------------------------

func TestCascadeReload_UnknownService(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_cascade_reload")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"services": []any{"nonexistent-service-xyz"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("cascade_reload should succeed (skipping unknown services), not error")
	}

	text := extractText(t, result)
	var out CascadeReloadOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(out.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(out.Results))
	}
	if out.Results[0].Action != "skipped" {
		t.Errorf("action = %q, want skipped", out.Results[0].Action)
	}
	if out.Results[0].Message != "unknown service" {
		t.Errorf("message = %q, want 'unknown service'", out.Results[0].Message)
	}
}

// ---------------------------------------------------------------------------
// dotfiles_onboard_repo — input validation
// ---------------------------------------------------------------------------

func TestOnboardRepo_MissingRepoPath(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_onboard_repo")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo_path": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty repo_path")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_eww_get — input validation
// ---------------------------------------------------------------------------

func TestEwwGet_MissingVariable(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_eww_get")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"variable": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty variable")
	}
}

// ---------------------------------------------------------------------------
// JSON round-trip tests for build/pipeline output types
// ---------------------------------------------------------------------------

func TestBuildOutputTypes_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "PipelineRunOutput",
			fn: func(t *testing.T) {
				out := PipelineRunOutput{
					Repo:   "test-repo",
					Output: "build: ok\ntest: ok",
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded PipelineRunOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Repo != "test-repo" {
					t.Errorf("repo = %q, want test-repo", decoded.Repo)
				}
			},
		},
		{
			name: "BulkPipelineOutput",
			fn: func(t *testing.T) {
				out := BulkPipelineOutput{
					Total:  3,
					Passed: 2,
					Failed: 1,
					Results: []PipelineResult{
						{Repo: "r1", Status: "pass"},
						{Repo: "r2", Status: "pass"},
						{Repo: "r3", Status: "build-fail", Output: "error in main.go"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded BulkPipelineOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Total != 3 {
					t.Errorf("total = %d, want 3", decoded.Total)
				}
				if decoded.Failed != 1 {
					t.Errorf("failed = %d, want 1", decoded.Failed)
				}
			},
		},
		{
			name: "CreateRepoOutput",
			fn: func(t *testing.T) {
				out := CreateRepoOutput{
					RepoPath: "/home/user/repo",
					RepoURL:  "https://github.com/org/repo",
					Status:   "ok",
					Output:   "created successfully",
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded CreateRepoOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Status != "ok" {
					t.Errorf("status = %q, want ok", decoded.Status)
				}
			},
		},
		{
			name: "CascadeReloadOutput",
			fn: func(t *testing.T) {
				out := CascadeReloadOutput{
					Results: []ServiceReloadStatus{
						{Service: "hyprland", Action: "reloaded"},
						{Service: "mako", Action: "reloaded"},
						{Service: "eww", Action: "failed", Message: "not running"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded CascadeReloadOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Results) != 3 {
					t.Errorf("results = %d, want 3", len(decoded.Results))
				}
			},
		},
		{
			name: "MCPKitVersionSyncOutput",
			fn: func(t *testing.T) {
				out := MCPKitVersionSyncOutput{
					LatestVersion: "v0.3.0",
					Results: []MCPKitSyncResult{
						{Repo: "dotfiles-mcp", Action: "already-current", OldVersion: "v0.3.0", NewVersion: "v0.3.0"},
						{Repo: "process-mcp", Action: "updated", OldVersion: "v0.2.0", NewVersion: "v0.3.0"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded MCPKitVersionSyncOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.LatestVersion != "v0.3.0" {
					t.Errorf("latest_version = %q, want v0.3.0", decoded.LatestVersion)
				}
			},
		},
		{
			name: "OnboardRepoOutput",
			fn: func(t *testing.T) {
				out := OnboardRepoOutput{
					Repo:   "test",
					Status: "ok",
					Output: "added LICENSE",
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded OnboardRepoOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Status != "ok" {
					t.Errorf("status = %q, want ok", decoded.Status)
				}
			},
		},
		{
			name: "EwwRestartOutput",
			fn: func(t *testing.T) {
				out := EwwRestartOutput{
					Killed:    2,
					WaybarOff: true,
					DaemonPID: "12345",
					BarsOpen:  []string{"bar-left", "bar-right"},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded EwwRestartOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Killed != 2 {
					t.Errorf("killed = %d, want 2", decoded.Killed)
				}
			},
		},
		{
			name: "EwwStatusOutput",
			fn: func(t *testing.T) {
				out := EwwStatusOutput{
					DaemonRunning: true,
					DaemonCount:   1,
					WaybarRunning: false,
					Windows:       []string{"bar-left"},
					Variables:     map[string]string{"workspace": "1"},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded EwwStatusOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if !decoded.DaemonRunning {
					t.Error("daemon_running should be true")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.fn)
	}
}

// ---------------------------------------------------------------------------
// dotfiles_mcpkit_version_sync — temp dir test (exercises dir scanning)
// ---------------------------------------------------------------------------

func TestMCPKitVersionSync_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_mcpkit_version_sync")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	// This will fail to find any mcpkit version (no go.mod files exist in temp dir).
	// The handler should still return a result without panicking.
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_go_sync — exercises handler path
// ---------------------------------------------------------------------------

func TestGoSync_DryRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_go_sync")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"dry_run": true,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_workflow_sync — exercises handler path
// ---------------------------------------------------------------------------

func TestWorkflowSync_DryRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_workflow_sync")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"dry_run": true,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_health_check — exercises handler path
// ---------------------------------------------------------------------------

func TestHealthCheck_Run(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_health_check")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_dep_audit — exercises handler path
// ---------------------------------------------------------------------------

func TestDepAudit_Run(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_dep_audit")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_bulk_pipeline — default language scan
// ---------------------------------------------------------------------------

func TestBulkPipeline_DefaultLanguageScan(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	// Create repos with various language markers.
	goRepo := filepath.Join(dir, "go-repo")
	os.MkdirAll(goRepo, 0755)
	os.WriteFile(filepath.Join(goRepo, "go.mod"), []byte("module test\n\ngo 1.26.1"), 0644)

	makeRepo := filepath.Join(dir, "make-repo")
	os.MkdirAll(makeRepo, 0755)
	os.WriteFile(filepath.Join(makeRepo, "Makefile"), []byte("build:\n"), 0644)

	// No markers, should be skipped.
	emptyRepo := filepath.Join(dir, "empty-repo")
	os.MkdirAll(emptyRepo, 0755)
	os.WriteFile(filepath.Join(emptyRepo, "README.md"), []byte("# empty"), 0644)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_bulk_pipeline")

	// No language filter — should pick up go-repo and make-repo.
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out BulkPipelineOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Total != 2 {
		t.Errorf("total = %d, want 2 (go-repo and make-repo)", out.Total)
	}
}
