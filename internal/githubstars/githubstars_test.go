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

func TestBuildMarkdownAudit(t *testing.T) {
	sources := []MarkdownSource{
		{
			Name:  "hyprland",
			Path:  "/tmp/hyprland.md",
			Repos: []string{"hyprwm/Hyprland", "WillPower3309/swayfx"},
		},
		{
			Name:  "kitty",
			Path:  "/tmp/kitty.md",
			Repos: []string{"kovidgoyal/kitty"},
		},
	}
	lists := []UserList{
		{
			Name: "Hyprland",
			Items: []StarredListItem{
				{NameWithOwner: "hyprwm/hyprland"},
				{NameWithOwner: "WayfireWM/wayfire"},
			},
		},
		{
			Name: "kitty",
			Items: []StarredListItem{
				{NameWithOwner: "kovidgoyal/kitty"},
			},
		},
	}

	audit := BuildMarkdownAudit(sources, lists)
	if audit.ExactMatch {
		t.Fatal("expected audit drift")
	}
	if audit.UniqueRepos != 3 {
		t.Fatalf("UniqueRepos = %d, want 3", audit.UniqueRepos)
	}
	if len(audit.Lists) != 2 {
		t.Fatalf("expected 2 list audits, got %d", len(audit.Lists))
	}
	if audit.Lists[0].Name != "hyprland" {
		t.Fatalf("expected hyprland first after sorting, got %q", audit.Lists[0].Name)
	}
	if !stringSlicesEqual(audit.Lists[0].Missing, []string{"WillPower3309/swayfx"}) {
		t.Fatalf("unexpected missing repos: %v", audit.Lists[0].Missing)
	}
	if !stringSlicesEqual(audit.Lists[0].Extra, []string{"WayfireWM/wayfire"}) {
		t.Fatalf("unexpected extra repos: %v", audit.Lists[0].Extra)
	}
	if len(audit.Lists[1].Missing) != 0 || len(audit.Lists[1].Extra) != 0 {
		t.Fatalf("expected kitty to match exactly, got %+v", audit.Lists[1])
	}
}

func TestTrimMarkdownAudit(t *testing.T) {
	audit := MarkdownAudit{
		Lists: []MarkdownListAudit{
			{
				Name:    "hyprland",
				Missing: []string{"a", "b", "c"},
				Extra:   []string{"x", "y", "z"},
			},
		},
	}

	trimmed := TrimMarkdownAudit(audit, 2)
	if !stringSlicesEqual(trimmed.Lists[0].Missing, []string{"a", "b"}) {
		t.Fatalf("unexpected trimmed missing repos: %v", trimmed.Lists[0].Missing)
	}
	if !stringSlicesEqual(trimmed.Lists[0].Extra, []string{"x", "y"}) {
		t.Fatalf("unexpected trimmed extra repos: %v", trimmed.Lists[0].Extra)
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

func TestBuildTaxonomyAuditCaseInsensitiveRepoNames(t *testing.T) {
	repos := []StarredRepository{
		{
			NameWithOwner: "WayfireWM/wayfire",
			Lists:         []ListRef{{Name: "hyprland"}, {Name: "wayland"}},
		},
	}
	assignments := []TaxonomyAssignment{
		{Repo: "wayfirewm/wayfire", Lists: []string{"hyprland", "wayland"}},
	}

	audit := BuildTaxonomyAudit(repos, assignments, "")
	if len(audit.ReposWithDrift) != 0 {
		t.Fatalf("expected no drift for case-only repo differences, got %+v", audit.ReposWithDrift)
	}
	if len(audit.DesiredReposMissing) != 0 {
		t.Fatalf("expected no missing repos for case-only repo differences, got %+v", audit.DesiredReposMissing)
	}
}

func TestParseMarkdownSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kitty.md")
	markdown := strings.Join([]string{
		"# Kitty",
		"https://github.com/kovidgoyal/kitty",
		"https://github.com/kovidgoyal/kitty/blob/master/docs/remote-control.rst",
		"https://github.com/hyprwm/hyprland-guiutils?ref=itsfoss.com",
		"`https://github.com/WayfireWM/wayfire`",
		"https://github.com/topics/wayland",
		"https://github.com/orgs/lmstudio-ai",
		"https://github.com/datvodinh/rag-chatbot.git",
		"https://github.com/owner/repo/issues/123",
	}, "\n")
	if err := os.WriteFile(path, []byte(markdown), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	sources, err := ParseMarkdownSources(dir, nil)
	if err != nil {
		t.Fatalf("ParseMarkdownSources() error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected one source, got %d", len(sources))
	}
	if sources[0].Name != "kitty" {
		t.Fatalf("source name = %q, want kitty", sources[0].Name)
	}
	wantRepos := []string{"datvodinh/rag-chatbot", "hyprwm/hyprland-guiutils", "kovidgoyal/kitty", "owner/repo", "WayfireWM/wayfire"}
	if !stringSlicesEqual(sources[0].Repos, wantRepos) {
		t.Fatalf("repos = %v, want %v", sources[0].Repos, wantRepos)
	}
}

func TestBuildExactListRequests(t *testing.T) {
	current := []UserList{
		{
			Name: "hyprland",
			Items: []StarredListItem{
				{NameWithOwner: "hyprwm/Hyprland"},
				{NameWithOwner: "WayfireWM/wayfire"},
			},
		},
		{
			Name: "wayland",
			Items: []StarredListItem{
				{NameWithOwner: "WayfireWM/wayfire"},
			},
		},
		{
			Name: "MCP / Production",
			Items: []StarredListItem{
				{NameWithOwner: "WayfireWM/wayfire"},
			},
		},
	}
	assignments := []TaxonomyAssignment{
		{Repo: "hyprwm/Hyprland", Lists: []string{"hyprland", "wayland"}},
		{Repo: "be5invis/iosevka", Lists: []string{"kitty"}},
	}
	targets := []EnsureListSpec{{Name: "hyprland"}, {Name: "wayland"}, {Name: "kitty"}}

	requests := BuildExactListRequests(current, assignments, targets, true, true)
	got := map[string][]string{}
	for _, request := range requests {
		got[strings.ToLower(request.Repo)] = request.TargetLists
		if request.Operation != OperationReplace {
			t.Fatalf("expected replace operation, got %s", request.Operation)
		}
		if !request.CreateMissing || !request.StarMissing {
			t.Fatalf("expected create-missing and star-missing enabled: %+v", request)
		}
	}

	if !stringSlicesEqual(got["hyprwm/hyprland"], []string{"hyprland", "wayland"}) {
		t.Fatalf("hyprland repo lists = %v", got["hyprwm/hyprland"])
	}
	if !stringSlicesEqual(got["wayfirewm/wayfire"], []string{"MCP / Production"}) {
		t.Fatalf("wayfire repo should keep only non-target lists, got %v", got["wayfirewm/wayfire"])
	}
	if !stringSlicesEqual(got["be5invis/iosevka"], []string{"kitty"}) {
		t.Fatalf("new repo lists = %v", got["be5invis/iosevka"])
	}
}
