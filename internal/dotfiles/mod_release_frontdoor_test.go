package dotfiles

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

func initGitRepoForFrontdoorTest(t *testing.T, repoDir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v (%s)", args, err, string(out))
		}
	}
}

func commitAllFrontdoorTest(t *testing.T, repoDir, msg string) {
	t.Helper()
	add := exec.Command("git", "add", "-A")
	add.Dir = repoDir
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v (%s)", err, string(out))
	}
	commit := exec.Command("git", "commit", "-m", msg)
	commit.Dir = repoDir
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v (%s)", err, string(out))
	}
}

func tagFrontdoorTest(t *testing.T, repoDir, tag string) {
	t.Helper()
	cmd := exec.Command("git", "tag", tag)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git tag failed: %v (%s)", err, string(out))
	}
}

func TestDotfilesPipelineStatus_Filtered(t *testing.T) {
	dir := t.TempDir()

	goRepo := filepath.Join(dir, "go-repo")
	if err := os.MkdirAll(goRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goRepo, "go.mod"), []byte("module example.com/go-repo\n\ngo 1.26.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initGitRepoForFrontdoorTest(t, goRepo)
	commitAllFrontdoorTest(t, goRepo, "chore: init go repo")

	nodeRepo := filepath.Join(dir, "node-repo")
	if err := os.MkdirAll(nodeRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeRepo, "package.json"), []byte("{\"name\":\"node-repo\",\"version\":\"0.1.0\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initGitRepoForFrontdoorTest(t, nodeRepo)
	commitAllFrontdoorTest(t, nodeRepo, "chore: init node repo")

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_pipeline_status")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir":        dir,
		"repos":            []any{"node-repo"},
		"include_passing":  true,
		"refresh_baseline": false,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	var out PipelineStatusOutput
	if err := json.Unmarshal([]byte(extractText(t, result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 1 {
		t.Fatalf("total = %d, want 1", out.Total)
	}
	if len(out.Repos) != 1 {
		t.Fatalf("repos = %d, want 1", len(out.Repos))
	}
	if out.Repos[0].Name != "node-repo" {
		t.Fatalf("repo = %q, want node-repo", out.Repos[0].Name)
	}
	if out.Repos[0].Language != "node" {
		t.Fatalf("language = %q, want node", out.Repos[0].Language)
	}
}

func TestDotfilesChangelogGen_SpecificRepo(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "changelog-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# changelog repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initGitRepoForFrontdoorTest(t, repoDir)
	commitAllFrontdoorTest(t, repoDir, "chore: init repo")
	tagFrontdoorTest(t, repoDir, "v0.1.0")
	if err := os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAllFrontdoorTest(t, repoDir, "feat: add fleet status")

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_changelog_gen")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
		"repos":     []any{"changelog-repo"},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	var out DotfilesChangelogGenOutput
	if err := json.Unmarshal([]byte(extractText(t, result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 1 || out.Generated != 1 || out.Failed != 0 {
		t.Fatalf("unexpected totals: %+v", out)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(out.Results))
	}
	if out.Results[0].Status != "generated" {
		t.Fatalf("status = %q, want generated", out.Results[0].Status)
	}
	if out.Results[0].LastTag != "v0.1.0" {
		t.Fatalf("last_tag = %q, want v0.1.0", out.Results[0].LastTag)
	}
	if out.Results[0].Groups["feat"] != 1 {
		t.Fatalf("feat group = %d, want 1", out.Results[0].Groups["feat"])
	}
}

func TestDotfilesRelease_DryRun(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "release-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte("{\"name\":\"release-repo\",\"version\":\"0.1.0\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initGitRepoForFrontdoorTest(t, repoDir)
	commitAllFrontdoorTest(t, repoDir, "chore: init release repo")

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_release")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
		"targets": []any{
			map[string]any{
				"repo":    "release-repo",
				"version": "0.2.0",
			},
		},
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	var out DotfilesReleaseOutput
	if err := json.Unmarshal([]byte(extractText(t, result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total != 1 || out.Released != 0 || out.Failed != 0 {
		t.Fatalf("unexpected totals: %+v", out)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(out.Results))
	}
	if out.Results[0].Status != "dry-run" {
		t.Fatalf("status = %q, want dry-run", out.Results[0].Status)
	}
	if out.Results[0].Tag != "v0.2.0" {
		t.Fatalf("tag = %q, want v0.2.0", out.Results[0].Tag)
	}
	if out.Results[0].CurrentVersion != "0.1.0" {
		t.Fatalf("current_version = %q, want 0.1.0", out.Results[0].CurrentVersion)
	}
}
