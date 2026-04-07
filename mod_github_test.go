package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// dotfiles_gh_list_personal_repos — input validation
// ---------------------------------------------------------------------------

func TestGHListPersonalRepos_MissingOwner(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_list_personal_repos")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"owner": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty owner")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_transfer_repos — input validation
// ---------------------------------------------------------------------------

func TestGHTransferRepos_MissingOwner(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_transfer_repos")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"owner":      "",
		"target_org": "test-org",
		"repos":      []any{"repo1"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty owner")
	}
}

func TestGHTransferRepos_MissingTargetOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_transfer_repos")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"owner":      "testuser",
		"target_org": "",
		"repos":      []any{"repo1"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty target_org")
	}
}

func TestGHTransferRepos_MissingRepos(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_transfer_repos")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"owner":      "testuser",
		"target_org": "test-org",
		"repos":      []any{},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty repos")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_recreate_forks — input validation
// ---------------------------------------------------------------------------

func TestGHRecreateForks_MissingOwner(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_recreate_forks")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"owner":      "",
		"target_org": "test-org",
		"repos":      []any{"repo1"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty owner")
	}
}

func TestGHRecreateForks_MissingTargetOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_recreate_forks")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"owner":      "testuser",
		"target_org": "",
		"repos":      []any{"repo1"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty target_org")
	}
}

func TestGHRecreateForks_MissingRepos(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_recreate_forks")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"owner":      "testuser",
		"target_org": "test-org",
		"repos":      []any{},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty repos")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_onboard_repos — input validation
// ---------------------------------------------------------------------------

func TestGHOnboardRepos_MissingTargetOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_onboard_repos")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"target_org": "",
		"repos":      []any{"user/repo"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty target_org")
	}
}

func TestGHOnboardRepos_MissingRepos(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_onboard_repos")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"target_org": "test-org",
		"repos":      []any{},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty repos")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_list_org_repos — input validation
// ---------------------------------------------------------------------------

func TestGHListOrgRepos_MissingOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_list_org_repos")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"org": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty org")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_local_sync_audit — input validation
// ---------------------------------------------------------------------------

func TestGHLocalSyncAudit_MissingOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_local_sync_audit")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"org": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty org")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_bulk_archive — input validation
// ---------------------------------------------------------------------------

func TestGHBulkArchive_MissingOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_bulk_archive")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"org":   "",
		"repos": []any{"repo1"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty org")
	}
}

func TestGHBulkArchive_MissingRepos(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_bulk_archive")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"org":   "test-org",
		"repos": []any{},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty repos")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_bulk_settings — input validation
// ---------------------------------------------------------------------------

func TestGHBulkSettings_MissingOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_bulk_settings")

	boolTrue := true
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"org":                     "",
		"delete_branch_on_merge": boolTrue,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty org")
	}
}

func TestGHBulkSettings_NoSettings(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_bulk_settings")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"org": "test-org",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for no settings specified")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_bulk_clone — input validation
// ---------------------------------------------------------------------------

func TestGHBulkClone_MissingOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_bulk_clone")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"org": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty org")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_clean_stale — input validation
// ---------------------------------------------------------------------------

func TestGHCleanStale_MissingOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_clean_stale")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"org": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty org")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_gh_full_sync — input validation
// ---------------------------------------------------------------------------

func TestGHFullSync_MissingOrg(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_full_sync")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"org": "",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for empty org")
	}
}

// ---------------------------------------------------------------------------
// JSON round-trip tests for GitHub output types
// ---------------------------------------------------------------------------

func TestGHOutputTypes_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "GHListReposOutput",
			fn: func(t *testing.T) {
				out := GHListReposOutput{
					Total:     2,
					Originals: 1,
					Forks:     1,
					Repos: []RepoInfo{
						{Name: "r1", FullName: "user/r1", Private: false, Fork: false, Language: "Go"},
						{Name: "r2", FullName: "user/r2", Private: true, Fork: true, Language: "Python"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHListReposOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Total != 2 {
					t.Errorf("total = %d, want 2", decoded.Total)
				}
			},
		},
		{
			name: "GHTransferReposOutput",
			fn: func(t *testing.T) {
				out := GHTransferReposOutput{
					Results: []TransferResult{
						{Repo: "r1", Action: "transferred"},
						{Repo: "r2", Action: "skipped", Message: "already exists"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHTransferReposOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Results) != 2 {
					t.Errorf("results = %d, want 2", len(decoded.Results))
				}
			},
		},
		{
			name: "FleetAuditOutput",
			fn: func(t *testing.T) {
				out := FleetAuditOutput{
					Total:   10,
					Passing: 8,
					Failing: 2,
					GoRepos: 7,
					Repos: []RepoAuditInfo{
						{Name: "r1", Language: "go", GoVersion: "1.26.1", CIStatus: "pass", TestCount: 10, HasPipelineMk: true, HasCLAUDEmd: true, HasCI: true},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded FleetAuditOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Total != 10 {
					t.Errorf("total = %d, want 10", decoded.Total)
				}
			},
		},
		{
			name: "GHBulkSettingsOutput",
			fn: func(t *testing.T) {
				out := GHBulkSettingsOutput{
					Results: []SettingsResult{
						{Repo: "r1", Action: "applied", Applied: []string{"has_wiki"}, Previous: map[string]bool{"has_wiki": true}},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHBulkSettingsOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(decoded.Results) != 1 {
					t.Errorf("results = %d, want 1", len(decoded.Results))
				}
			},
		},
		{
			name: "GHPullAllOutput",
			fn: func(t *testing.T) {
				out := GHPullAllOutput{
					Total:    5,
					Updated:  2,
					Current:  1,
					Dirty:    1,
					Detached: 1,
					Failed:   0,
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHPullAllOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Total != 5 {
					t.Errorf("total = %d, want 5", decoded.Total)
				}
			},
		},
		{
			name: "GHFullSyncOutput",
			fn: func(t *testing.T) {
				out := GHFullSyncOutput{
					Pulled:   3,
					Current:  2,
					Dirty:    1,
					Cloned:   1,
					Orphaned: 1,
					Failed:   0,
					Details: []FullSyncDetail{
						{Repo: "r1", Action: "pulled"},
					},
				}
				data, err := json.Marshal(out)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var decoded GHFullSyncOutput
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.Pulled != 3 {
					t.Errorf("pulled = %d, want 3", decoded.Pulled)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.fn)
	}
}
