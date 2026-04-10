package githubstars

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCalculateDesiredLists(t *testing.T) {
	tests := []struct {
		name          string
		current       []string
		target        []string
		operation     MembershipOperation
		managedPrefix string
		want          []string
		wantErr       bool
	}{
		{
			name:      "merge",
			current:   []string{"MCP / Production"},
			target:    []string{"MCP / Alpha"},
			operation: OperationMerge,
			want:      []string{"MCP / Alpha", "MCP / Production"},
		},
		{
			name:      "replace",
			current:   []string{"MCP / Production"},
			target:    []string{"MCP / Alpha"},
			operation: OperationReplace,
			want:      []string{"MCP / Alpha"},
		},
		{
			name:      "remove",
			current:   []string{"MCP / Production", "MCP / Alpha"},
			target:    []string{"MCP / Alpha"},
			operation: OperationRemove,
			want:      []string{"MCP / Production"},
		},
		{
			name:          "replace managed",
			current:       []string{"MCP / Production", "ComfyUI Nodes"},
			target:        []string{"MCP / Alpha"},
			operation:     OperationReplaceManaged,
			managedPrefix: "MCP / ",
			want:          []string{"ComfyUI Nodes", "MCP / Alpha"},
		},
		{
			name:      "replace managed missing prefix",
			current:   []string{"MCP / Production"},
			target:    []string{"MCP / Alpha"},
			operation: OperationReplaceManaged,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := calculateDesiredLists(tc.current, tc.target, tc.operation, tc.managedPrefix)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !stringSlicesEqual(got, tc.want) {
				t.Fatalf("calculateDesiredLists() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadEnvFileValue(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "export GITHUB_PAT=\"ghp_test_123\"\nOTHER=value\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if got := readEnvFileValue(envPath, "GITHUB_PAT"); got != "ghp_test_123" {
		t.Fatalf("readEnvFileValue() = %q, want ghp_test_123", got)
	}
}

func TestReplaceManagedBlock(t *testing.T) {
	block := RenderCodexBlock("/tmp/dotfiles")

	inserted := replaceManagedBlock("approval_policy = \"never\"\n", block)
	if !strings.Contains(inserted, CodexStartMarker) {
		t.Fatal("expected inserted block to include start marker")
	}

	replaced := replaceManagedBlock(inserted, strings.ReplaceAll(block, "github_stars_workflow", "github_stars_workspace"))
	if strings.Count(replaced, CodexStartMarker) != 1 {
		t.Fatal("expected exactly one managed block after replacement")
	}
	if !strings.Contains(replaced, "github_stars_workspace") {
		t.Fatal("expected replacement block content")
	}
}

func TestInstallCodexConfigDryRun(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	dotfilesDir := filepath.Join(dir, ".codex", "worktrees", "dotfiles", "example")
	if err := os.MkdirAll(filepath.Join(dotfilesDir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dotfilesDir, "AGENTS.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dotfilesDir, "scripts", "run-dotfiles-mcp.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write run-dotfiles-mcp.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dotfilesDir, "scripts", "hg-github-stars.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write hg-github-stars.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dotfilesDir, "scripts", "hg-github-official-mcp.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write hg-github-official-mcp.sh: %v", err)
	}
	result, err := InstallCodexConfig(configPath, dotfilesDir, false)
	if err != nil {
		t.Fatalf("InstallCodexConfig() error = %v", err)
	}
	if result.Status != "dry-run" {
		t.Fatalf("status = %q, want dry-run", result.Status)
	}
	if result.Warning == "" {
		t.Fatal("expected warning for worktree-based dotfiles path")
	}
	if !strings.Contains(result.BlockPreview, "github_stars_official") {
		t.Fatal("expected official server in block preview")
	}
	if !strings.Contains(result.BlockPreview, "[mcp_servers.github_stars_official.env]") {
		t.Fatal("expected official env block in preview")
	}
	if !strings.Contains(result.BlockPreview, "dotfiles_gh_stars_summary") {
		t.Fatal("expected new summary tool in preview")
	}
}

func TestSuggestTaxonomy(t *testing.T) {
	repos := []StarredRepository{
		{NameWithOwner: "github/github-mcp-server", Description: "Official GitHub MCP Server", Topics: []string{"mcp", "github"}, PrimaryLanguage: "Go"},
		{NameWithOwner: "microsoft/playwright-mcp", Description: "Playwright MCP server", Topics: []string{"mcp", "browser"}, PrimaryLanguage: "TypeScript"},
		{NameWithOwner: "SynapticSage/ganger", Description: "GitHub star folders for MCP", Topics: []string{"mcp", "github"}, PrimaryLanguage: "Python"},
	}
	suggestions := SuggestTaxonomy(repos, []string{"GitHub", "MCP"}, 5)
	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}
	foundMCP := false
	for _, suggestion := range suggestions {
		if strings.EqualFold(suggestion.Name, "MCP") {
			foundMCP = true
			break
		}
	}
	if !foundMCP {
		t.Fatalf("expected MCP suggestion, got %v", suggestions)
	}
}

func TestSummarizeStars(t *testing.T) {
	repos := []StarredRepository{
		{
			NameWithOwner:   "github/github-mcp-server",
			PrimaryLanguage: "Go",
			Topics:          []string{"mcp", "github"},
			Lists:           []ListRef{{Name: "MCP / Production"}},
		},
		{
			NameWithOwner:   "microsoft/playwright-mcp",
			PrimaryLanguage: "TypeScript",
			Topics:          []string{"mcp", "browser"},
			IsFork:          true,
		},
		{
			NameWithOwner:   "SynapticSage/ganger",
			PrimaryLanguage: "Python",
			Topics:          []string{"mcp", "github"},
			IsArchived:      true,
			Lists:           []ListRef{{Name: "MCP / Alpha"}},
		},
	}

	summary := SummarizeStars(repos, 5, "MCP / ")
	if summary.TotalStars != 3 {
		t.Fatalf("TotalStars = %d, want 3", summary.TotalStars)
	}
	if summary.ArchivedCount != 1 || summary.ForkCount != 1 {
		t.Fatalf("unexpected archived/fork counts: %+v", summary)
	}
	if summary.UnlistedCount != 1 || summary.MissingManaged != 1 {
		t.Fatalf("unexpected list coverage counts: %+v", summary)
	}
	if len(summary.Lists) == 0 || summary.Lists[0].Name != "MCP / Alpha" && summary.Lists[0].Name != "MCP / Production" {
		t.Fatalf("expected list buckets, got %+v", summary.Lists)
	}
}

func TestFindCleanupCandidates(t *testing.T) {
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	repos := []StarredRepository{
		{
			NameWithOwner:   "archived/repo",
			IsArchived:      true,
			UpdatedAt:       now.AddDate(-2, 0, 0).Format(time.RFC3339),
			PrimaryLanguage: "Go",
		},
		{
			NameWithOwner: "managed/repo",
			UpdatedAt:     now.AddDate(0, -1, 0).Format(time.RFC3339),
			Lists:         []ListRef{{Name: "MCP / Production"}},
		},
	}

	candidates := FindCleanupCandidates(repos, CleanupOptions{
		InactiveDays:      365,
		IncludeArchived:   true,
		IncludeForks:      true,
		RequireUnlisted:   true,
		ManagedListPrefix: "MCP / ",
		Now:               now,
	})
	if len(candidates) != 1 {
		t.Fatalf("expected 1 cleanup candidate, got %d", len(candidates))
	}
	if candidates[0].Repo != "archived/repo" {
		t.Fatalf("unexpected cleanup candidate: %+v", candidates[0])
	}
	if len(candidates[0].Reasons) < 2 {
		t.Fatalf("expected multiple reasons, got %+v", candidates[0])
	}
}

func TestBuildTaxonomyAudit(t *testing.T) {
	repos := []StarredRepository{
		{
			NameWithOwner: "github/github-mcp-server",
			Lists:         []ListRef{{Name: "MCP / Alpha"}},
		},
		{
			NameWithOwner: "example/unlisted",
		},
		{
			NameWithOwner: "example/outside-profile",
			Lists:         []ListRef{{Name: "MCP / Experimental"}},
		},
	}
	assignments := []TaxonomyAssignment{
		{Repo: "github/github-mcp-server", Lists: []string{"MCP / Production"}},
		{Repo: "missing/repo", Lists: []string{"MCP / Alpha"}},
	}

	audit := BuildTaxonomyAudit(repos, assignments, "MCP / ")
	if len(audit.ReposWithDrift) != 1 {
		t.Fatalf("expected 1 drift item, got %+v", audit.ReposWithDrift)
	}
	if len(audit.ReposOutsideProfile) != 1 {
		t.Fatalf("expected 1 outside-profile repo, got %+v", audit.ReposOutsideProfile)
	}
	if len(audit.ReposMissingManaged) != 1 {
		t.Fatalf("expected 1 missing-managed repo, got %+v", audit.ReposMissingManaged)
	}
	if len(audit.DesiredReposMissing) != 1 || audit.DesiredReposMissing[0] != "missing/repo" {
		t.Fatalf("unexpected desired missing repos: %+v", audit.DesiredReposMissing)
	}
}
