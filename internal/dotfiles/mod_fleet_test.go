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

// ---------------------------------------------------------------------------
// dotfiles_fleet_audit — temp dir tests
// ---------------------------------------------------------------------------

func TestFleetAudit_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_fleet_audit")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	text := extractText(t, result)
	var out FleetAuditOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Total != 0 {
		t.Errorf("total = %d, want 0", out.Total)
	}
	if len(out.Repos) != 0 {
		t.Errorf("repos count = %d, want 0", len(out.Repos))
	}
}

func TestFleetAudit_NonexistentDir(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_fleet_audit")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": "/tmp/nonexistent-fleet-audit-test-xyz",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result for nonexistent dir")
	}
}

func TestFleetAudit_WithGoRepo(t *testing.T) {
	dir := t.TempDir()

	// Create a fake Go repo with .git, go.mod, and test files.
	repoDir := filepath.Join(dir, "test-go-repo")
	os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module example.com/test\n\ngo 1.26.1\n"), 0644)
	os.WriteFile(filepath.Join(repoDir, "main_test.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(repoDir, "CLAUDE.md"), []byte("# test"), 0644)

	// Create a pipeline.mk file.
	os.WriteFile(filepath.Join(repoDir, "pipeline.mk"), []byte("include pipeline.mk"), 0644)

	// Create a CI dir.
	os.MkdirAll(filepath.Join(repoDir, ".github", "workflows"), 0755)
	os.WriteFile(filepath.Join(repoDir, ".github", "workflows", "ci.yml"), []byte("name: CI"), 0644)

	// Initialize a real git repo so git log works.
	initCmd := exec.Command("git", "init")
	initCmd.Dir = repoDir
	initCmd.Run()

	configCmd := exec.Command("git", "config", "user.email", "test@test.com")
	configCmd.Dir = repoDir
	configCmd.Run()

	configNameCmd := exec.Command("git", "config", "user.name", "Test")
	configNameCmd.Dir = repoDir
	configNameCmd.Run()

	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = repoDir
	addCmd.Run()

	commitCmd := exec.Command("git", "commit", "-m", "init")
	commitCmd.Dir = repoDir
	commitCmd.Run()

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_fleet_audit")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	text := extractText(t, result)
	var out FleetAuditOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Total != 1 {
		t.Fatalf("total = %d, want 1", out.Total)
	}
	if out.GoRepos != 1 {
		t.Errorf("go_repos = %d, want 1", out.GoRepos)
	}
	if len(out.Repos) != 1 {
		t.Fatalf("repos count = %d, want 1", len(out.Repos))
	}

	repo := out.Repos[0]
	if repo.Name != "test-go-repo" {
		t.Errorf("name = %q, want test-go-repo", repo.Name)
	}
	if repo.Language != "go" {
		t.Errorf("language = %q, want go", repo.Language)
	}
	if repo.GoVersion != "1.26.1" {
		t.Errorf("go_version = %q, want 1.26.1", repo.GoVersion)
	}
	if !repo.HasPipelineMk {
		t.Error("has_pipeline_mk should be true")
	}
	if !repo.HasCLAUDEmd {
		t.Error("has_claude_md should be true")
	}
	if !repo.HasCI {
		t.Error("has_ci should be true")
	}
	if repo.TestCount < 1 {
		t.Errorf("test_count = %d, want >= 1", repo.TestCount)
	}
}

func TestFleetAudit_WithNodeRepo(t *testing.T) {
	dir := t.TempDir()

	repoDir := filepath.Join(dir, "test-node-repo")
	os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"test"}`), 0644)

	// Init real git repo.
	initCmd := exec.Command("git", "init")
	initCmd.Dir = repoDir
	initCmd.Run()
	configCmd := exec.Command("git", "config", "user.email", "test@test.com")
	configCmd.Dir = repoDir
	configCmd.Run()
	configNameCmd := exec.Command("git", "config", "user.name", "Test")
	configNameCmd.Dir = repoDir
	configNameCmd.Run()
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = repoDir
	addCmd.Run()
	commitCmd := exec.Command("git", "commit", "-m", "init")
	commitCmd.Dir = repoDir
	commitCmd.Run()

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_fleet_audit")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	text := extractText(t, result)
	var out FleetAuditOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Total != 1 {
		t.Fatalf("total = %d, want 1", out.Total)
	}
	repo := out.Repos[0]
	if repo.Language != "node" {
		t.Errorf("language = %q, want node", repo.Language)
	}
	if repo.GoVersion != "" {
		t.Errorf("go_version = %q, want empty", repo.GoVersion)
	}
}

func TestFleetAudit_SkipsNonGitDirs(t *testing.T) {
	dir := t.TempDir()

	// A directory without .git should be skipped.
	os.MkdirAll(filepath.Join(dir, "not-a-repo"), 0755)
	os.WriteFile(filepath.Join(dir, "not-a-repo", "README.md"), []byte("# not a repo"), 0644)

	// A file should be skipped.
	os.WriteFile(filepath.Join(dir, "somefile.txt"), []byte("hello"), 0644)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_fleet_audit")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}

	text := extractText(t, result)
	var out FleetAuditOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Total != 0 {
		t.Errorf("total = %d, want 0 (non-git dirs should be skipped)", out.Total)
	}
}

func TestFleetAudit_MakefileInclude(t *testing.T) {
	dir := t.TempDir()

	repoDir := filepath.Join(dir, "test-repo")
	os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module example.com/test\n\ngo 1.26.1\n"), 0644)
	// No pipeline.mk file, but Makefile includes it.
	os.WriteFile(filepath.Join(repoDir, "Makefile"), []byte("include pipeline.mk\n\nbuild:\n\tgo build ./...\n"), 0644)

	initCmd := exec.Command("git", "init")
	initCmd.Dir = repoDir
	initCmd.Run()
	configCmd := exec.Command("git", "config", "user.email", "test@test.com")
	configCmd.Dir = repoDir
	configCmd.Run()
	configNameCmd := exec.Command("git", "config", "user.name", "Test")
	configNameCmd.Dir = repoDir
	configNameCmd.Run()
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = repoDir
	addCmd.Run()
	commitCmd := exec.Command("git", "commit", "-m", "init")
	commitCmd.Dir = repoDir
	commitCmd.Run()

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_fleet_audit")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out FleetAuditOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(out.Repos) != 1 {
		t.Fatalf("repos count = %d, want 1", len(out.Repos))
	}
	if !out.Repos[0].HasPipelineMk {
		t.Error("has_pipeline_mk should be true when Makefile includes pipeline.mk")
	}
}

// ---------------------------------------------------------------------------
// dotfiles_pull_all — temp dir tests (no gh needed)
// ---------------------------------------------------------------------------

func TestPullAll_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_pull_all")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result for empty dir")
	}

	text := extractText(t, result)
	var out GHPullAllOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Total != 0 {
		t.Errorf("total = %d, want 0", out.Total)
	}
}

func TestPullAll_DetachedHead(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "detached-repo")
	os.MkdirAll(repoDir, 0755)

	// Create a git repo, then detach HEAD.
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}
	os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("x"), 0644)
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "init"},
		{"git", "checkout", "--detach", "HEAD"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_pull_all")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out GHPullAllOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Detached != 1 {
		t.Errorf("detached = %d, want 1", out.Detached)
	}
	if out.Total != 1 {
		t.Errorf("total = %d, want 1", out.Total)
	}
}

func TestPullAll_DirtyRepo(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "dirty-repo")
	os.MkdirAll(repoDir, 0755)

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}
	os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("x"), 0644)
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}

	// Make it dirty.
	os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("uncommitted"), 0644)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_pull_all")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir":  dir,
		"fetch_only": false,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out GHPullAllOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Dirty != 1 {
		t.Errorf("dirty = %d, want 1", out.Dirty)
	}
}

func TestPullAll_FetchOnly(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "fetch-repo")
	os.MkdirAll(repoDir, 0755)

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}
	os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("x"), 0644)
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}

	// Add an untracked file — should NOT be flagged as dirty in fetch_only mode.
	os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("new"), 0644)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_gh_pull_all")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir":  dir,
		"fetch_only": true,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out GHPullAllOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// fetch_only skips dirty check, so should attempt fetch.
	if out.Dirty != 0 {
		t.Errorf("dirty = %d, want 0 in fetch_only mode", out.Dirty)
	}
	if out.Total != 1 {
		t.Errorf("total = %d, want 1", out.Total)
	}
}

// ---------------------------------------------------------------------------
// dotfiles_bulk_pipeline — temp dir tests (no external scripts needed)
// ---------------------------------------------------------------------------

func TestBulkPipeline_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir) // Scripts dir won't exist, but we test with empty repos.

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_bulk_pipeline")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result for empty dir")
	}

	text := extractText(t, result)
	var out BulkPipelineOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Total != 0 {
		t.Errorf("total = %d, want 0", out.Total)
	}
}

func TestBulkPipeline_LanguageFilter(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	// Create a Go repo.
	goRepo := filepath.Join(dir, "go-repo")
	os.MkdirAll(goRepo, 0755)
	os.WriteFile(filepath.Join(goRepo, "go.mod"), []byte("module test\n\ngo 1.26.1"), 0644)

	// Create a Node repo.
	nodeRepo := filepath.Join(dir, "node-repo")
	os.MkdirAll(nodeRepo, 0755)
	os.WriteFile(filepath.Join(nodeRepo, "package.json"), []byte(`{}`), 0644)

	// Create a Python repo.
	pyRepo := filepath.Join(dir, "py-repo")
	os.MkdirAll(pyRepo, 0755)
	os.WriteFile(filepath.Join(pyRepo, "pyproject.toml"), []byte("[project]\nname=\"test\""), 0644)

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_bulk_pipeline")

	// Filter by Go only.
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
		"language":  "go",
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

	if out.Total != 1 {
		t.Errorf("total = %d, want 1 (only Go repo)", out.Total)
	}
	if out.Total == 1 && out.Results[0].Repo != "go-repo" {
		t.Errorf("repo = %q, want go-repo", out.Results[0].Repo)
	}
}

func TestBulkPipeline_SpecificRepos(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	// Create repos.
	for _, name := range []string{"repo-a", "repo-b", "repo-c"} {
		repoDir := filepath.Join(dir, name)
		os.MkdirAll(repoDir, 0755)
		os.WriteFile(filepath.Join(repoDir, "Makefile"), []byte("build:\n"), 0644)
	}

	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_bulk_pipeline")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"local_dir": dir,
		"repos":     []any{"repo-a", "repo-c"},
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
		t.Errorf("total = %d, want 2 (repo-a and repo-c)", out.Total)
	}
}
