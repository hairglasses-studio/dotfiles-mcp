// Command dotfiles-mcp is an MCP server for dotfiles configuration management.
//
// It provides validated config editing, symlink health checks, and service
// reloading over the Model Context Protocol (stdio transport).
//
// Usage:
//
//	DOTFILES_DIR=$HOME/hairglasses-studio/dotfiles dotfiles-mcp
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func dotfilesDir() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "hairglasses-studio", "dotfiles")
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// detectFormat guesses a config format from the file extension.
func detectFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		return "toml"
	case ".json", ".jsonc":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".ini", ".conf":
		return "ini"
	default:
		return "unknown"
	}
}

// reloadCommands maps service name -> reload command.
var reloadCommands = map[string][]string{
	"hyprland": {"hyprctl", "reload"},
	"mako":     {"makoctl", "reload"},
	"eww":      {"eww", "reload"},
	"waybar":   {"pkill", "-SIGUSR2", "waybar"},
	"sway":     {"swaymsg", "reload"},
	"tmux":     {"tmux", "source-file", "~/.tmux.conf"},
}

// expectedSymlinks returns the symlink mapping from install.sh logic.
func expectedSymlinks() []struct{ src, dst string } {
	dir := dotfilesDir()
	home := homeDir()
	cfg := filepath.Join(home, ".config")

	// Common (all platforms).
	links := []struct{ src, dst string }{
		{filepath.Join(dir, "zsh/zshrc"), filepath.Join(home, ".zshrc")},
		{filepath.Join(dir, "zsh/p10k.zsh"), filepath.Join(home, ".p10k.zsh")},
		{filepath.Join(dir, "zsh/zshenv"), filepath.Join(home, ".zshenv")},
		{filepath.Join(dir, "git/gitconfig"), filepath.Join(home, ".gitconfig")},
		{filepath.Join(dir, "ssh/config"), filepath.Join(home, ".ssh/config")},
		{filepath.Join(dir, "starship/starship.toml"), filepath.Join(cfg, "starship.toml")},
		{filepath.Join(dir, "ghostty"), filepath.Join(cfg, "ghostty")},
		{filepath.Join(dir, "nvim"), filepath.Join(cfg, "nvim")},
		{filepath.Join(dir, "bat"), filepath.Join(cfg, "bat")},
		{filepath.Join(dir, "fastfetch"), filepath.Join(cfg, "fastfetch")},
		{filepath.Join(dir, "git/delta"), filepath.Join(cfg, "delta")},
		{filepath.Join(dir, "git/ignore"), filepath.Join(cfg, "git/ignore")},
		{filepath.Join(dir, "gh"), filepath.Join(cfg, "gh")},
		{filepath.Join(dir, "k9s"), filepath.Join(cfg, "k9s")},
		{filepath.Join(dir, "lazygit"), filepath.Join(cfg, "lazygit")},
		{filepath.Join(dir, "btop"), filepath.Join(cfg, "btop")},
		{filepath.Join(dir, "yazi"), filepath.Join(cfg, "yazi")},
		{filepath.Join(dir, "cava"), filepath.Join(cfg, "cava")},
		{filepath.Join(dir, "glow"), filepath.Join(cfg, "glow")},
		{filepath.Join(dir, "tmux/tmux.conf"), filepath.Join(home, ".tmux.conf")},
	}

	// Platform-specific links.
	if runtime.GOOS == "darwin" {
		links = append(links,
			struct{ src, dst string }{filepath.Join(dir, "aerospace/aerospace.toml"), filepath.Join(home, ".aerospace.toml")},
			struct{ src, dst string }{filepath.Join(dir, "sketchybar"), filepath.Join(cfg, "sketchybar")},
			struct{ src, dst string }{filepath.Join(dir, "borders"), filepath.Join(cfg, "borders")},
			struct{ src, dst string }{filepath.Join(dir, "tattoo/tattoy.toml"), filepath.Join(home, "Library/Application Support/tattoy/tattoy.toml")},
		)
	} else if runtime.GOOS == "linux" {
		links = append(links,
			struct{ src, dst string }{filepath.Join(dir, "sway/config"), filepath.Join(cfg, "sway/config")},
			struct{ src, dst string }{filepath.Join(dir, "waybar/config.jsonc"), filepath.Join(cfg, "waybar/config")},
			struct{ src, dst string }{filepath.Join(dir, "waybar/style.css"), filepath.Join(cfg, "waybar/style.css")},
			struct{ src, dst string }{filepath.Join(dir, "mako/config"), filepath.Join(cfg, "mako/config")},
			struct{ src, dst string }{filepath.Join(dir, "wofi/config"), filepath.Join(cfg, "wofi/config")},
			struct{ src, dst string }{filepath.Join(dir, "wofi/style.css"), filepath.Join(cfg, "wofi/style.css")},
			struct{ src, dst string }{filepath.Join(dir, "foot/foot.ini"), filepath.Join(cfg, "foot/foot.ini")},
			struct{ src, dst string }{filepath.Join(dir, "hyprland"), filepath.Join(cfg, "hypr")},
			struct{ src, dst string }{filepath.Join(dir, "eww"), filepath.Join(cfg, "eww")},
			struct{ src, dst string }{filepath.Join(dir, "tattoy/tattoy.toml"), filepath.Join(cfg, "tattoy/tattoy.toml")},
		)
	}

	return links
}

// parseStringArray extracts a []string from an interface{} that may be []any.
func parseStringArray(v any) ([]string, error) {
	if v == nil {
		return nil, fmt.Errorf("nil value")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("not an array")
	}
	var out []string
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// MCP Tool I/O types
// ---------------------------------------------------------------------------

// Tool 1: dotfiles_list_configs

type ListConfigsInput struct{}

type ConfigEntry struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	SymlinkTarget  string `json:"symlink_target,omitempty"`
	SymlinkHealthy *bool  `json:"symlink_healthy,omitempty"`
	Format         string `json:"format"`
}

type ListConfigsOutput struct {
	Configs []ConfigEntry `json:"configs"`
}

// Tool 2: dotfiles_validate_config

type ValidateConfigInput struct {
	Content string `json:"content" jsonschema:"required,description=Config file content to validate"`
	Format  string `json:"format" jsonschema:"required,description=Config format,enum=toml,enum=json"`
}

type ValidateConfigOutput struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
	Line  int    `json:"line,omitempty"`
}

// Tool 3: dotfiles_reload_service

type ReloadServiceInput struct {
	Service string `json:"service" jsonschema:"required,description=Service to reload,enum=hyprland,enum=mako,enum=eww,enum=waybar,enum=sway,enum=tmux"`
}

type ReloadServiceOutput struct {
	Service string `json:"service"`
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Tool 4: dotfiles_check_symlinks

type CheckSymlinksInput struct{}

type SymlinkStatus struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Status string `json:"status"`
	Actual string `json:"actual,omitempty"`
}

type CheckSymlinksOutput struct {
	Symlinks []SymlinkStatus `json:"symlinks"`
}

// Tool 5: dotfiles_gh_list_personal_repos

type GHListReposInput struct {
	Owner      string `json:"owner" jsonschema:"required,description=GitHub username whose repos to list"`
	ExcludeOrg string `json:"exclude_org,omitempty" jsonschema:"description=Exclude repos already in this org (e.g. hairglasses-studio)"`
}

type RepoInfo struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Private     bool   `json:"private"`
	Fork        bool   `json:"fork"`
	Language    string `json:"language,omitempty"`
	Branch      string `json:"default_branch"`
	Archived    bool   `json:"archived"`
	Stars       int    `json:"stargazers_count"`
	Forks       int    `json:"forks_count"`
	Description string `json:"description,omitempty"`
}

type GHListReposOutput struct {
	Total     int        `json:"total"`
	Originals int        `json:"originals"`
	Forks     int        `json:"forks"`
	Repos     []RepoInfo `json:"repos"`
}

// Tool 6: dotfiles_gh_transfer_repos

type GHTransferReposInput struct {
	Owner     string   `json:"owner" jsonschema:"required,description=Source GitHub username"`
	TargetOrg string   `json:"target_org" jsonschema:"required,description=Target GitHub organization"`
	Repos     []string `json:"repos" jsonschema:"required,description=Array of repo names to transfer"`
	Execute   bool     `json:"execute,omitempty" jsonschema:"description=Set true to execute transfers (default: dry-run)"`
}

type TransferResult struct {
	Repo    string `json:"repo"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

type GHTransferReposOutput struct {
	Results []TransferResult `json:"results"`
}

// Tool 7: dotfiles_gh_recreate_forks

type GHRecreateForkInput struct {
	Owner     string   `json:"owner" jsonschema:"required,description=Source GitHub username (fork owner)"`
	TargetOrg string   `json:"target_org" jsonschema:"required,description=Target GitHub organization"`
	Repos     []string `json:"repos" jsonschema:"required,description=Array of forked repo names to recreate"`
	Execute   bool     `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}

type GHRecreateForkOutput struct {
	Results []TransferResult `json:"results"`
}

// Tool 8: dotfiles_gh_onboard_repos

type GHOnboardReposInput struct {
	Repos     []string `json:"repos" jsonschema:"required,description=Source repos in owner/name format (e.g. user/repo)"`
	TargetOrg string   `json:"target_org" jsonschema:"required,description=GitHub org to onboard into (e.g. hairglasses-studio)"`
	Execute   bool     `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
	LocalDir  string   `json:"local_dir,omitempty" jsonschema:"description=Local clone directory (default: ~/hairglasses-studio)"`
}

type GHOnboardResult struct {
	Repo    string `json:"repo"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

type GHOnboardReposOutput struct {
	Results []GHOnboardResult `json:"results"`
}

// Tool 9: dotfiles_gh_list_org_repos

type GHListOrgReposInput struct {
	Org             string   `json:"org" jsonschema:"required,description=GitHub organization name"`
	LocalDir        string   `json:"local_dir,omitempty" jsonschema:"description=Local directory to check clone status (default: ~/hairglasses-studio)"`
	Languages       []string `json:"languages,omitempty" jsonschema:"description=Filter by language (e.g. Go or Python)"`
	ExcludeArchived bool     `json:"exclude_archived,omitempty" jsonschema:"description=Exclude archived repos"`
	OnlyMissing     bool     `json:"only_missing,omitempty" jsonschema:"description=Only show repos not cloned locally"`
}

type OrgRepoInfo struct {
	Name        string `json:"name"`
	Private     bool   `json:"private"`
	Fork        bool   `json:"fork"`
	Archived    bool   `json:"archived"`
	Language    string `json:"language,omitempty"`
	Description string `json:"description,omitempty"`
	LocalStatus string `json:"local_status"` // cloned, missing, wrong-remote
}

type GHListOrgReposOutput struct {
	Total   int           `json:"total"`
	Cloned  int           `json:"cloned"`
	Missing int           `json:"missing"`
	Repos   []OrgRepoInfo `json:"repos"`
}

// Tool 10: dotfiles_gh_local_sync_audit

type GHLocalSyncAuditInput struct {
	Org      string `json:"org" jsonschema:"required,description=GitHub organization name"`
	LocalDir string `json:"local_dir,omitempty" jsonschema:"description=Local directory to audit (default: ~/hairglasses-studio)"`
}

type SyncAuditEntry struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // orphaned, missing, mismatched, synced
	Details string `json:"details,omitempty"`
}

type GHLocalSyncAuditOutput struct {
	Synced     int              `json:"synced"`
	Orphaned   int              `json:"orphaned"`
	Missing    int              `json:"missing"`
	Mismatched int              `json:"mismatched"`
	Entries    []SyncAuditEntry `json:"entries"`
}

// Tool 11: dotfiles_gh_bulk_archive

type GHBulkArchiveInput struct {
	Org     string   `json:"org" jsonschema:"required,description=GitHub organization name"`
	Repos   []string `json:"repos" jsonschema:"required,description=Array of repo names to archive"`
	Execute bool     `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}

type GHBulkArchiveOutput struct {
	Results []TransferResult `json:"results"`
}

// Tool 12: dotfiles_gh_bulk_settings

type GHBulkSettingsInput struct {
	Org                 string   `json:"org" jsonschema:"required,description=GitHub organization name"`
	Repos               []string `json:"repos,omitempty" jsonschema:"description=Specific repos (default: all non-archived in org)"`
	DeleteBranchOnMerge *bool    `json:"delete_branch_on_merge,omitempty" jsonschema:"description=Auto-delete head branches after merge"`
	HasWiki             *bool    `json:"has_wiki,omitempty" jsonschema:"description=Enable/disable wiki"`
	HasProjects         *bool    `json:"has_projects,omitempty" jsonschema:"description=Enable/disable projects"`
	AllowSquashMerge    *bool    `json:"allow_squash_merge,omitempty" jsonschema:"description=Allow squash merging"`
	AllowMergeCommit    *bool    `json:"allow_merge_commit,omitempty" jsonschema:"description=Allow merge commits"`
	AllowRebaseMerge    *bool    `json:"allow_rebase_merge,omitempty" jsonschema:"description=Allow rebase merging"`
	Execute             bool     `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}

type SettingsResult struct {
	Repo     string          `json:"repo"`
	Action   string          `json:"action"`
	Applied  []string        `json:"applied,omitempty"`
	Previous map[string]bool `json:"previous,omitempty"`
	Message  string          `json:"message,omitempty"`
}

type GHBulkSettingsOutput struct {
	Results []SettingsResult `json:"results"`
}

// Tool 13: dotfiles_gh_bulk_clone

type GHBulkCloneInput struct {
	Org      string `json:"org" jsonschema:"required,description=GitHub organization name"`
	LocalDir string `json:"local_dir,omitempty" jsonschema:"description=Local directory to clone into (default: ~/hairglasses-studio)"`
	Execute  bool   `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}

type CloneResult struct {
	Repo    string `json:"repo"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

type GHBulkCloneOutput struct {
	Results []CloneResult `json:"results"`
}

// Tool 14: dotfiles_gh_pull_all

type GHPullAllInput struct {
	LocalDir  string `json:"local_dir,omitempty" jsonschema:"description=Local directory containing repos (default: ~/hairglasses-studio)"`
	FetchOnly bool   `json:"fetch_only,omitempty" jsonschema:"description=Only fetch without merging (default: pull with ff-only)"`
}

type PullResult struct {
	Repo    string `json:"repo"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

type GHPullAllOutput struct {
	Total    int          `json:"total"`
	Updated  int          `json:"updated"`
	Current  int          `json:"current"`
	Dirty    int          `json:"dirty"`
	Detached int          `json:"detached"`
	Failed   int          `json:"failed"`
	Results  []PullResult `json:"results"`
}

// Tool 15: dotfiles_gh_clean_stale

type GHCleanStaleInput struct {
	Org      string `json:"org" jsonschema:"required,description=GitHub organization name"`
	LocalDir string `json:"local_dir,omitempty" jsonschema:"description=Local directory to clean (default: ~/hairglasses-studio)"`
	Execute  bool   `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}

type GHCleanStaleOutput struct {
	Results []TransferResult `json:"results"`
}

// Tool 16: dotfiles_gh_full_sync

type GHFullSyncInput struct {
	Org      string `json:"org" jsonschema:"required,description=GitHub organization name"`
	LocalDir string `json:"local_dir,omitempty" jsonschema:"description=Local directory (default: ~/hairglasses-studio)"`
	Execute  bool   `json:"execute,omitempty" jsonschema:"description=Set true to clone missing repos (default: dry-run for clones)"`
}

type FullSyncDetail struct {
	Repo   string `json:"repo"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

type GHFullSyncOutput struct {
	Pulled   int              `json:"pulled"`
	Current  int              `json:"current"`
	Dirty    int              `json:"dirty"`
	Cloned   int              `json:"cloned"`
	Orphaned int              `json:"orphaned"`
	Failed   int              `json:"failed"`
	Details  []FullSyncDetail `json:"details"`
}

// Tool 17: dotfiles_mcpkit_version_sync

type MCPKitVersionSyncInput struct {
	LocalDir string `json:"local_dir,omitempty" jsonschema:"description=Local directory (default: ~/hairglasses-studio)"`
	Execute  bool   `json:"execute,omitempty" jsonschema:"description=Set true to run go get + go mod tidy (default: dry-run)"`
}

type MCPKitSyncResult struct {
	Repo       string `json:"repo"`
	Action     string `json:"action"` // updated, already-current, skipped, failed
	OldVersion string `json:"old_version,omitempty"`
	NewVersion string `json:"new_version,omitempty"`
	Message    string `json:"message,omitempty"`
}

type MCPKitVersionSyncOutput struct {
	LatestVersion string             `json:"latest_version"`
	Results       []MCPKitSyncResult `json:"results"`
}

// Tool 18: dotfiles_create_repo

type CreateRepoInput struct {
	Name     string `json:"name" jsonschema:"required,description=Repository name (e.g. my-new-tool)"`
	Language string `json:"language,omitempty" jsonschema:"description=Primary language,enum=go,enum=node,enum=python,enum=shell"`
	Private  bool   `json:"private,omitempty" jsonschema:"description=Create as private repo (default: true)"`
}

type CreateRepoOutput struct {
	RepoPath string `json:"repo_path"`
	RepoURL  string `json:"repo_url,omitempty"`
	Status   string `json:"status"`
	Output   string `json:"output"`
}

// Tool 18: dotfiles_fleet_audit

type FleetAuditInput struct {
	LocalDir string `json:"local_dir,omitempty" jsonschema:"description=Local directory (default: ~/hairglasses-studio)"`
}

type RepoAuditInfo struct {
	Name           string `json:"name"`
	Language       string `json:"language"`
	GoVersion      string `json:"go_version,omitempty"`
	CIStatus       string `json:"ci_status"` // pass, fail, running, none
	TestCount      int    `json:"test_count"`
	LastCommitDays int    `json:"last_commit_days"`
	HasPipelineMk  bool   `json:"has_pipeline_mk"`
	HasCLAUDEmd    bool   `json:"has_claude_md"`
	HasCI          bool   `json:"has_ci"`
}

type FleetAuditOutput struct {
	Total   int             `json:"total"`
	Passing int             `json:"passing"`
	Failing int             `json:"failing"`
	GoRepos int             `json:"go_repos"`
	Repos   []RepoAuditInfo `json:"repos"`
}

// Tool 19: dotfiles_cascade_reload

type CascadeReloadInput struct {
	Services []string `json:"services,omitempty" jsonschema:"description=Services to reload in order (default: hyprland then mako then eww)"`
}

type ServiceReloadStatus struct {
	Service string `json:"service"`
	Action  string `json:"action"` // reloaded, failed, skipped
	Message string `json:"message,omitempty"`
}

type CascadeReloadOutput struct {
	Results []ServiceReloadStatus `json:"results"`
}

// Tool 20: dotfiles_rice_check

type RiceCheckInput struct {
	Level string `json:"level,omitempty" jsonschema:"description=Check level: quick (services only) or full (+ palette scan),enum=quick,enum=full"`
}

type PaletteViolation struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Color string `json:"color"`
}

type RiceCheckOutput struct {
	Compositor        string                `json:"compositor"`
	Shader            string                `json:"shader"`
	Wallpaper         string                `json:"wallpaper"`
	Services          []ServiceReloadStatus `json:"services"`
	PaletteViolations []PaletteViolation    `json:"palette_violations,omitempty"`
}

// Tool 21: dotfiles_bulk_pipeline

type BulkPipelineInput struct {
	LocalDir  string   `json:"local_dir,omitempty" jsonschema:"description=Local directory (default: ~/hairglasses-studio)"`
	Repos     []string `json:"repos,omitempty" jsonschema:"description=Specific repos (default: all with Makefile)"`
	Language  string   `json:"language,omitempty" jsonschema:"description=Filter by language,enum=go,enum=node,enum=python"`
	BuildOnly bool     `json:"build_only,omitempty" jsonschema:"description=Only run build step"`
	TestOnly  bool     `json:"test_only,omitempty" jsonschema:"description=Only run test step"`
}

type PipelineResult struct {
	Repo   string `json:"repo"`
	Status string `json:"status"` // pass, build-fail, test-fail, vet-fail, skip
	Output string `json:"output,omitempty"`
}

type BulkPipelineOutput struct {
	Total   int              `json:"total"`
	Passed  int              `json:"passed"`
	Failed  int              `json:"failed"`
	Results []PipelineResult `json:"results"`
}

// Tool 22: dotfiles_eww_restart

type EwwRestartInput struct{}

type EwwRestartOutput struct {
	Killed    int      `json:"killed"`
	WaybarOff bool     `json:"waybar_killed"`
	DaemonPID string   `json:"daemon_pid,omitempty"`
	BarsOpen  []string `json:"bars_opened"`
	Error     string   `json:"error,omitempty"`
}

// Tool 9: dotfiles_eww_status

type EwwStatusInput struct{}

type EwwLayerInfo struct {
	Monitor   string `json:"monitor"`
	Namespace string `json:"namespace"`
	Position  string `json:"position"`
}

type EwwStatusOutput struct {
	DaemonRunning bool              `json:"daemon_running"`
	DaemonCount   int               `json:"daemon_count"`
	WaybarRunning bool              `json:"waybar_running"`
	Windows       []string          `json:"windows"`
	Layers        []EwwLayerInfo    `json:"layers"`
	Variables     map[string]string `json:"variables,omitempty"`
}

// Tool 10: dotfiles_eww_get

type EwwGetInput struct {
	Variable string `json:"variable" jsonschema:"required,description=eww variable name to query"`
}

type EwwGetOutput struct {
	Variable string `json:"variable"`
	Value    any    `json:"value"`
}

// Tool 11: dotfiles_onboard_repo

type OnboardRepoInput struct {
	RepoPath string `json:"repo_path" jsonschema:"required,description=Absolute path to the repo directory"`
	Language string `json:"language,omitempty" jsonschema:"description=Language override (auto-detected if omitted),enum=auto,enum=go,enum=node,enum=python,enum=shell"`
	DryRun   bool   `json:"dry_run,omitempty" jsonschema:"description=Preview what would be added without making changes"`
}

type OnboardRepoOutput struct {
	Repo   string `json:"repo"`
	Status string `json:"status"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Tool 12: dotfiles_pipeline_run

type PipelineRunInput struct {
	RepoPath  string `json:"repo_path" jsonschema:"required,description=Absolute path to the repo directory"`
	BuildOnly bool   `json:"build_only,omitempty" jsonschema:"description=Only run the build step"`
	TestOnly  bool   `json:"test_only,omitempty" jsonschema:"description=Only run the test step"`
}

type PipelineRunOutput struct {
	Repo   string `json:"repo"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
	Result any    `json:"result,omitempty"`
}

// Tool 13: dotfiles_health_check

type HealthCheckInput struct{}

type HealthCheckOutput struct {
	Output string `json:"output"`
}

// Tool 14: dotfiles_dep_audit

type DepAuditInput struct{}

type DepAuditOutput struct {
	Output string `json:"output"`
}

// Tool 15: dotfiles_workflow_sync

type WorkflowSyncInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"description=Preview changes without modifying files"`
	Push   bool `json:"push,omitempty" jsonschema:"description=Commit and push changes to each repo"`
}

type WorkflowSyncOutput struct {
	Output string `json:"output"`
}

// Tool 16: dotfiles_go_sync

type GoSyncInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"description=Preview which repos would be updated"`
	Tidy   bool `json:"tidy,omitempty" jsonschema:"description=Run go mod tidy after updating go.mod"`
}

type GoSyncOutput struct {
	Output string `json:"output"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

type DotfilesModule struct{}

func (m *DotfilesModule) Name() string        { return "dotfiles" }
func (m *DotfilesModule) Description() string { return "Dotfiles configuration management tools" }

func (m *DotfilesModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── dotfiles_list_configs ──────────────────────
		handler.TypedHandler[ListConfigsInput, ListConfigsOutput](
			"dotfiles_list_configs",
			"List all dotfiles config directories with symlink health and detected format.",
			func(_ context.Context, _ ListConfigsInput) (ListConfigsOutput, error) {
				dir := dotfilesDir()
				entries, err := os.ReadDir(dir)
				if err != nil {
					return ListConfigsOutput{}, fmt.Errorf("[%s] read dotfiles dir %s: %w", handler.ErrInvalidParam, dir, err)
				}

				home := homeDir()
				configDir := filepath.Join(home, ".config")

				var configs []ConfigEntry
				for _, e := range entries {
					if !e.IsDir() {
						continue
					}
					name := e.Name()
					if strings.HasPrefix(name, ".") {
						continue
					}
					srcPath := filepath.Join(dir, name)

					ce := ConfigEntry{
						Name: name,
						Path: srcPath,
					}

					subEntries, _ := os.ReadDir(srcPath)
					for _, se := range subEntries {
						if se.IsDir() {
							continue
						}
						f := detectFormat(se.Name())
						if f != "unknown" {
							ce.Format = f
							break
						}
					}
					if ce.Format == "" {
						ce.Format = "unknown"
					}

					symlinkPath := filepath.Join(configDir, name)
					if target, lerr := os.Readlink(symlinkPath); lerr == nil {
						ce.SymlinkTarget = target
						healthy := target == srcPath
						ce.SymlinkHealthy = &healthy
					}

					configs = append(configs, ce)
				}

				return ListConfigsOutput{Configs: configs}, nil
			},
		),

		// ── dotfiles_validate_config ──────────────────
		handler.TypedHandler[ValidateConfigInput, ValidateConfigOutput](
			"dotfiles_validate_config",
			"Validate config file content syntax (TOML or JSON).",
			func(_ context.Context, input ValidateConfigInput) (ValidateConfigOutput, error) {
				if input.Content == "" {
					return ValidateConfigOutput{}, fmt.Errorf("[%s] content is required", handler.ErrInvalidParam)
				}
				if input.Format == "" {
					return ValidateConfigOutput{}, fmt.Errorf("[%s] format is required (toml, json)", handler.ErrInvalidParam)
				}

				vr := ValidateConfigOutput{Valid: true}

				switch strings.ToLower(input.Format) {
				case "toml":
					_, err := toml.NewDecoder(strings.NewReader(input.Content)).Decode(new(map[string]any))
					if err != nil {
						vr.Valid = false
						vr.Error = err.Error()
						if pe, ok := err.(toml.ParseError); ok {
							msg := pe.Error()
							var line int
							if _, serr := fmt.Sscanf(msg, "toml: line %d", &line); serr == nil {
								vr.Line = line
							}
						}
					}

				case "json":
					var dst any
					dec := json.NewDecoder(strings.NewReader(input.Content))
					if err := dec.Decode(&dst); err != nil {
						vr.Valid = false
						vr.Error = err.Error()
						if se, ok := err.(*json.SyntaxError); ok {
							line := 1 + strings.Count(input.Content[:se.Offset], "\n")
							vr.Line = line
						}
					}

				default:
					return ValidateConfigOutput{}, fmt.Errorf("[%s] unsupported format %q: supported formats are toml, json", handler.ErrInvalidParam, input.Format)
				}

				return vr, nil
			},
		),

		// ── dotfiles_reload_service ───────────────────
		handler.TypedHandler[ReloadServiceInput, ReloadServiceOutput](
			"dotfiles_reload_service",
			"Reload a desktop service after config changes.",
			func(_ context.Context, input ReloadServiceInput) (ReloadServiceOutput, error) {
				if input.Service == "" {
					return ReloadServiceOutput{}, fmt.Errorf("[%s] service is required", handler.ErrInvalidParam)
				}

				cmdParts, ok := reloadCommands[strings.ToLower(input.Service)]
				if !ok {
					supported := make([]string, 0, len(reloadCommands))
					for k := range reloadCommands {
						supported = append(supported, k)
					}
					return ReloadServiceOutput{}, fmt.Errorf("[%s] unknown service %q; supported: %s", handler.ErrInvalidParam, input.Service, strings.Join(supported, ", "))
				}

				cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				rr := ReloadServiceOutput{Service: input.Service}
				if err := cmd.Run(); err != nil {
					rr.Success = false
					rr.Error = fmt.Sprintf("%v: %s", err, strings.TrimSpace(stderr.String()))
				} else {
					rr.Success = true
				}
				out := strings.TrimSpace(stdout.String())
				if out != "" {
					rr.Output = out
				}

				return rr, nil
			},
		),

		// ── dotfiles_check_symlinks ───────────────────
		handler.TypedHandler[CheckSymlinksInput, CheckSymlinksOutput](
			"dotfiles_check_symlinks",
			"Check health of all expected dotfiles symlinks (healthy, broken, or missing).",
			func(_ context.Context, _ CheckSymlinksInput) (CheckSymlinksOutput, error) {
				links := expectedSymlinks()
				var results []SymlinkStatus

				for _, l := range links {
					ss := SymlinkStatus{
						Source: l.src,
						Target: l.dst,
					}

					target, err := os.Readlink(l.dst)
					if err != nil {
						ss.Status = "missing"
					} else if target == l.src {
						ss.Status = "healthy"
					} else {
						ss.Status = "broken"
						ss.Actual = target
					}

					results = append(results, ss)
				}

				return CheckSymlinksOutput{Symlinks: results}, nil
			},
		),

		// ── dotfiles_gh_list_personal_repos ───────────
		handler.TypedHandler[GHListReposInput, GHListReposOutput](
			"dotfiles_gh_list_personal_repos",
			"List GitHub repos under a personal account, optionally excluding repos already in an org. Shows fork status, visibility, and language.",
			func(_ context.Context, input GHListReposInput) (GHListReposOutput, error) {
				if input.Owner == "" {
					return GHListReposOutput{}, fmt.Errorf("[%s] owner is required", handler.ErrInvalidParam)
				}

				cmd := exec.Command("gh", "api", "--paginate",
					fmt.Sprintf("/users/%s/repos?per_page=100&type=owner&sort=updated", input.Owner),
					"--jq", ".[] | {name, full_name, private, fork, language, default_branch, archived, stargazers_count, forks_count, description}")
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				if err := cmd.Run(); err != nil {
					return GHListReposOutput{}, fmt.Errorf("gh api failed: %v: %s", err, strings.TrimSpace(stderr.String()))
				}

				var repos []RepoInfo
				dec := json.NewDecoder(strings.NewReader(stdout.String()))
				for dec.More() {
					var r RepoInfo
					if err := dec.Decode(&r); err != nil {
						continue
					}
					if input.ExcludeOrg != "" {
						orgPrefix := input.ExcludeOrg + "/"
						if strings.HasPrefix(r.FullName, orgPrefix) {
							continue
						}
					}
					repos = append(repos, r)
				}

				out := GHListReposOutput{Total: len(repos), Repos: repos}
				for _, r := range repos {
					if r.Fork {
						out.Forks++
					} else {
						out.Originals++
					}
				}

				return out, nil
			},
		),

		// ── dotfiles_gh_transfer_repos ────────────────
		handler.TypedHandler[GHTransferReposInput, GHTransferReposOutput](
			"dotfiles_gh_transfer_repos",
			"Bulk transfer non-fork repos from a personal account to a GitHub org. Skips repos that already exist in the target. Set execute=true to run (dry-run by default).",
			func(_ context.Context, input GHTransferReposInput) (GHTransferReposOutput, error) {
				if input.Owner == "" {
					return GHTransferReposOutput{}, fmt.Errorf("[%s] owner is required", handler.ErrInvalidParam)
				}
				if input.TargetOrg == "" {
					return GHTransferReposOutput{}, fmt.Errorf("[%s] target_org is required", handler.ErrInvalidParam)
				}
				if len(input.Repos) == 0 {
					return GHTransferReposOutput{}, fmt.Errorf("[%s] repos is required (array of repo names)", handler.ErrInvalidParam)
				}

				dryRun := !input.Execute
				var results []TransferResult

				for _, repo := range input.Repos {
					checkCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", input.TargetOrg, repo), "--jq", ".full_name")
					if checkCmd.Run() == nil {
						results = append(results, TransferResult{Repo: repo, Action: "skipped", Message: "already exists in " + input.TargetOrg})
						continue
					}

					metaCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", input.Owner, repo), "--jq", ".fork")
					var metaOut bytes.Buffer
					metaCmd.Stdout = &metaOut
					if err := metaCmd.Run(); err != nil {
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "source not found"})
						continue
					}
					if strings.TrimSpace(metaOut.String()) == "true" {
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "is a fork — use dotfiles_gh_recreate_forks instead"})
						continue
					}

					if dryRun {
						results = append(results, TransferResult{Repo: repo, Action: "dry-run", Message: "would transfer to " + input.TargetOrg})
						continue
					}

					transferCmd := exec.Command("gh", "api", "--method", "POST",
						fmt.Sprintf("repos/%s/%s/transfer", input.Owner, repo),
						"-f", "new_owner="+input.TargetOrg, "--silent")
					var transferErr bytes.Buffer
					transferCmd.Stderr = &transferErr
					if err := transferCmd.Run(); err != nil {
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: strings.TrimSpace(transferErr.String())})
					} else {
						results = append(results, TransferResult{Repo: repo, Action: "transferred", Message: "transferred to " + input.TargetOrg})
					}
				}

				return GHTransferReposOutput{Results: results}, nil
			},
		),

		// ── dotfiles_gh_recreate_forks ────────────────
		handler.TypedHandler[GHRecreateForkInput, GHRecreateForkOutput](
			"dotfiles_gh_recreate_forks",
			"Recreate forked repos as fresh repos under a GitHub org. Clones each fork, squashes all history to a single initial commit on the default branch, creates a new repo in the org, pushes, and deletes the original fork. Set execute=true to run (dry-run by default).",
			func(_ context.Context, input GHRecreateForkInput) (GHRecreateForkOutput, error) {
				if input.Owner == "" {
					return GHRecreateForkOutput{}, fmt.Errorf("[%s] owner is required", handler.ErrInvalidParam)
				}
				if input.TargetOrg == "" {
					return GHRecreateForkOutput{}, fmt.Errorf("[%s] target_org is required", handler.ErrInvalidParam)
				}
				if len(input.Repos) == 0 {
					return GHRecreateForkOutput{}, fmt.Errorf("[%s] repos is required (array of repo names)", handler.ErrInvalidParam)
				}

				dryRun := !input.Execute
				var results []TransferResult

				for _, repo := range input.Repos {
					checkCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", input.TargetOrg, repo), "--jq", ".full_name")
					if checkCmd.Run() == nil {
						results = append(results, TransferResult{Repo: repo, Action: "skipped", Message: "already exists in " + input.TargetOrg})
						continue
					}

					metaCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", input.Owner, repo),
						"--jq", "{default_branch: .default_branch, private: .private, description: .description}")
					var metaOut bytes.Buffer
					metaCmd.Stdout = &metaOut
					if err := metaCmd.Run(); err != nil {
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "could not fetch metadata"})
						continue
					}

					var meta struct {
						DefaultBranch string `json:"default_branch"`
						Private       bool   `json:"private"`
						Description   string `json:"description"`
					}
					if err := json.Unmarshal(metaOut.Bytes(), &meta); err != nil {
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "bad metadata: " + err.Error()})
						continue
					}

					visibility := "public"
					if meta.Private {
						visibility = "private"
					}

					if dryRun {
						results = append(results, TransferResult{
							Repo:    repo,
							Action:  "dry-run",
							Message: fmt.Sprintf("would clone, squash to 1 commit on %s, create %s/%s (%s), delete fork", meta.DefaultBranch, input.TargetOrg, repo, visibility),
						})
						continue
					}

					tmpDir, err := os.MkdirTemp("", "fork-recreate-"+repo+"-")
					if err != nil {
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "mkdtemp: " + err.Error()})
						continue
					}

					cloneCmd := exec.Command("git", "clone", "--quiet",
						fmt.Sprintf("https://github.com/%s/%s.git", input.Owner, repo), tmpDir)
					if out, err := cloneCmd.CombinedOutput(); err != nil {
						os.RemoveAll(tmpDir)
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "clone failed: " + strings.TrimSpace(string(out))})
						continue
					}

					runGit := func(args ...string) (string, error) {
						c := exec.Command("git", args...)
						c.Dir = tmpDir
						out, err := c.CombinedOutput()
						return strings.TrimSpace(string(out)), err
					}

					runGit("checkout", meta.DefaultBranch)

					if _, err := runGit("checkout", "--orphan", "squashed"); err != nil {
						os.RemoveAll(tmpDir)
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "orphan checkout failed"})
						continue
					}
					runGit("add", "-A")
					if _, err := runGit("commit", "-m", fmt.Sprintf("Initial commit (migrated from %s/%s)", input.Owner, repo)); err != nil {
						os.RemoveAll(tmpDir)
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "squash commit failed (empty repo?)"})
						continue
					}
					runGit("branch", "-M", meta.DefaultBranch)

					createArgs := []string{"repo", "create", fmt.Sprintf("%s/%s", input.TargetOrg, repo), "--" + visibility, "--source", tmpDir, "--push"}
					if meta.Description != "" {
						createArgs = append(createArgs[:5], append([]string{"--description", meta.Description}, createArgs[5:]...)...)
					}
					createCmd := exec.Command("gh", createArgs...)
					if out, err := createCmd.CombinedOutput(); err != nil {
						os.RemoveAll(tmpDir)
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "create/push failed: " + strings.TrimSpace(string(out))})
						continue
					}

					os.RemoveAll(tmpDir)

					delCmd := exec.Command("gh", "api", "--method", "DELETE", fmt.Sprintf("repos/%s/%s", input.Owner, repo), "--silent")
					if err := delCmd.Run(); err != nil {
						results = append(results, TransferResult{Repo: repo, Action: "recreated", Message: "created in " + input.TargetOrg + " but failed to delete original fork"})
					} else {
						results = append(results, TransferResult{Repo: repo, Action: "recreated", Message: "squashed and recreated in " + input.TargetOrg + ", fork deleted"})
					}
				}

				return GHRecreateForkOutput{Results: results}, nil
			},
		),

		// ── dotfiles_eww_restart ──────────────────────
		handler.TypedHandler[EwwRestartInput, EwwRestartOutput](
			"dotfiles_eww_restart",
			"Kill all eww and waybar processes, restart eww daemon, and open both bars (bar and bar-secondary). Use after editing eww config files.",
			func(_ context.Context, _ EwwRestartInput) (EwwRestartOutput, error) {
				r := EwwRestartOutput{}

				waybar := exec.Command("killall", "waybar")
				if waybar.Run() == nil {
					r.WaybarOff = true
				}

				countCmd := exec.Command("pgrep", "-c", "eww")
				var countOut bytes.Buffer
				countCmd.Stdout = &countOut
				if countCmd.Run() == nil {
					fmt.Sscanf(strings.TrimSpace(countOut.String()), "%d", &r.Killed)
				}

				exec.Command("killall", "-9", "eww").Run()
				exec.Command("sleep", "1").Run()

				home := homeDir()
				sockets, _ := filepath.Glob(filepath.Join("/run/user/1000", "eww-server_*"))
				for _, s := range sockets {
					os.Remove(s)
				}

				daemon := exec.Command("eww", "daemon", "--restart")
				daemon.Dir = home
				if err := daemon.Start(); err != nil {
					r.Error = fmt.Sprintf("daemon start failed: %v", err)
					return r, nil
				}

				exec.Command("sleep", "3").Run()

				ping := exec.Command("eww", "ping")
				var pingOut bytes.Buffer
				ping.Stdout = &pingOut
				if err := ping.Run(); err != nil {
					r.Error = "daemon started but not responding to ping"
					return r, nil
				}

				pidCmd := exec.Command("pgrep", "-o", "eww")
				var pidOut bytes.Buffer
				pidCmd.Stdout = &pidOut
				if pidCmd.Run() == nil {
					r.DaemonPID = strings.TrimSpace(pidOut.String())
				}

				bars := []string{"bar", "bar-secondary"}
				for _, bar := range bars {
					openCmd := exec.Command("eww", "open", bar)
					if err := openCmd.Run(); err == nil {
						r.BarsOpen = append(r.BarsOpen, bar)
					}
				}

				return r, nil
			},
		),

		// ── dotfiles_eww_status ───────────────────────
		handler.TypedHandler[EwwStatusInput, EwwStatusOutput](
			"dotfiles_eww_status",
			"Show eww bar status: daemon health, open windows, layer surfaces, key variable values, and whether waybar is incorrectly running.",
			func(_ context.Context, _ EwwStatusInput) (EwwStatusOutput, error) {
				st := EwwStatusOutput{
					Variables: make(map[string]string),
				}

				countCmd := exec.Command("pgrep", "-c", "eww")
				var countOut bytes.Buffer
				countCmd.Stdout = &countOut
				if countCmd.Run() == nil {
					fmt.Sscanf(strings.TrimSpace(countOut.String()), "%d", &st.DaemonCount)
					st.DaemonRunning = st.DaemonCount > 0
				}

				waybarCmd := exec.Command("pgrep", "-x", "waybar")
				st.WaybarRunning = waybarCmd.Run() == nil

				winCmd := exec.Command("eww", "active-windows")
				var winOut bytes.Buffer
				winCmd.Stdout = &winOut
				if winCmd.Run() == nil {
					for _, line := range strings.Split(strings.TrimSpace(winOut.String()), "\n") {
						line = strings.TrimSpace(line)
						if line == "" {
							continue
						}
						if parts := strings.SplitN(line, ":", 2); len(parts) > 0 {
							st.Windows = append(st.Windows, strings.TrimSpace(parts[0]))
						}
					}
				}

				layerCmd := exec.Command("hyprctl", "layers")
				var layerOut bytes.Buffer
				layerCmd.Stdout = &layerOut
				if layerCmd.Run() == nil {
					var currentMonitor string
					for _, line := range strings.Split(layerOut.String(), "\n") {
						line = strings.TrimSpace(line)
						if strings.HasPrefix(line, "Monitor ") {
							currentMonitor = strings.TrimSuffix(strings.TrimPrefix(line, "Monitor "), ":")
						}
						if strings.Contains(line, "namespace:") {
							parts := strings.Split(line, "namespace: ")
							if len(parts) == 2 {
								ns := strings.Split(parts[1], ",")[0]
								xywh := ""
								if xywhIdx := strings.Index(line, "xywh: "); xywhIdx >= 0 {
									xywh = strings.Split(line[xywhIdx+6:], ",")[0]
								}
								st.Layers = append(st.Layers, EwwLayerInfo{
									Monitor:   currentMonitor,
									Namespace: ns,
									Position:  xywh,
								})
							}
						}
					}
				}

				varsToCheck := []string{
					"bar_workspaces_dp1", "bar_workspaces_dp2",
					"bar_cpu", "bar_mem", "bar_vol", "bar_shader",
				}
				for _, v := range varsToCheck {
					getCmd := exec.Command("eww", "get", v)
					var getOut bytes.Buffer
					getCmd.Stdout = &getOut
					if getCmd.Run() == nil {
						st.Variables[v] = strings.TrimSpace(getOut.String())
					}
				}

				return st, nil
			},
		),

		// ── dotfiles_eww_get ──────────────────────────
		handler.TypedHandler[EwwGetInput, EwwGetOutput](
			"dotfiles_eww_get",
			"Get the current value of an eww variable. Useful for debugging bar widgets (e.g. bar_workspaces_dp1, bar_cpu, bar_shader, rg_fleet).",
			func(_ context.Context, input EwwGetInput) (EwwGetOutput, error) {
				if input.Variable == "" {
					return EwwGetOutput{}, fmt.Errorf("[%s] variable name is required", handler.ErrInvalidParam)
				}

				cmd := exec.Command("eww", "get", input.Variable)
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				if err := cmd.Run(); err != nil {
					return EwwGetOutput{}, fmt.Errorf("eww get %s failed: %v: %s", input.Variable, err, strings.TrimSpace(stderr.String()))
				}

				value := strings.TrimSpace(stdout.String())

				var parsed any
				if json.Unmarshal([]byte(value), &parsed) == nil {
					return EwwGetOutput{Variable: input.Variable, Value: parsed}, nil
				}

				return EwwGetOutput{Variable: input.Variable, Value: value}, nil
			},
		),

		// ── dotfiles_onboard_repo ─────────────────────
		handler.TypedHandler[OnboardRepoInput, OnboardRepoOutput](
			"dotfiles_onboard_repo",
			"Onboard a repo with hairglasses-studio standard files (.editorconfig, CI workflows, LICENSE, CONTRIBUTING.md, pre-commit hooks). Detects language and adds appropriate config.",
			func(_ context.Context, input OnboardRepoInput) (OnboardRepoOutput, error) {
				if input.RepoPath == "" {
					return OnboardRepoOutput{}, fmt.Errorf("[%s] repo_path is required", handler.ErrInvalidParam)
				}

				scriptArgs := []string{input.RepoPath}
				if input.Language != "" {
					scriptArgs = append(scriptArgs, "--language="+input.Language)
				}
				if input.DryRun {
					scriptArgs = append(scriptArgs, "--dry-run")
				}

				script := filepath.Join(dotfilesDir(), "scripts", "hg-onboard-repo.sh")
				cmd := exec.Command(script, scriptArgs...)
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				err := cmd.Run()
				status := "ok"
				if err != nil {
					status = "fail"
				}
				return OnboardRepoOutput{
					Repo:   filepath.Base(input.RepoPath),
					Status: status,
					Output: stdout.String(),
					Error:  stderr.String(),
				}, nil
			},
		),

		// ── dotfiles_pipeline_run ─────────────────────
		handler.TypedHandler[PipelineRunInput, PipelineRunOutput](
			"dotfiles_pipeline_run",
			"Run the build+test pipeline (hg-pipeline.sh) on a repo. Supports Go, Node.js, and Python. Returns JSON results with per-step timing.",
			func(_ context.Context, input PipelineRunInput) (PipelineRunOutput, error) {
				if input.RepoPath == "" {
					return PipelineRunOutput{}, fmt.Errorf("[%s] repo_path is required", handler.ErrInvalidParam)
				}

				scriptArgs := []string{input.RepoPath, "--json"}
				if input.BuildOnly {
					scriptArgs = append(scriptArgs, "--build-only")
				}
				if input.TestOnly {
					scriptArgs = append(scriptArgs, "--test-only")
				}

				script := filepath.Join(dotfilesDir(), "scripts", "hg-pipeline.sh")
				cmd := exec.Command(script, scriptArgs...)
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				cmd.Run()

				lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
				lastLine := ""
				if len(lines) > 0 {
					lastLine = lines[len(lines)-1]
				}

				var result any
				if json.Unmarshal([]byte(lastLine), &result) == nil {
					return PipelineRunOutput{
						Repo:   filepath.Base(input.RepoPath),
						Result: result,
					}, nil
				}
				return PipelineRunOutput{
					Repo:   filepath.Base(input.RepoPath),
					Output: stdout.String(),
					Error:  stderr.String(),
				}, nil
			},
		),

		// ── dotfiles_health_check ─────────────────────
		handler.TypedHandler[HealthCheckInput, HealthCheckOutput](
			"dotfiles_health_check",
			"Run org-wide health dashboard across all hairglasses-studio repos. Shows build status, Go version, pipeline.mk inclusion, and CI workflow presence for every repo.",
			func(_ context.Context, _ HealthCheckInput) (HealthCheckOutput, error) {
				script := filepath.Join(dotfilesDir(), "scripts", "hg-health.sh")
				cmd := exec.Command(script)
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stdout
				cmd.Run()
				return HealthCheckOutput{Output: stdout.String()}, nil
			},
		),

		// ── dotfiles_dep_audit ────────────────────────
		handler.TypedHandler[DepAuditInput, DepAuditOutput](
			"dotfiles_dep_audit",
			"Audit Go dependency version skew across all hairglasses-studio repos. Reports which deps are unified vs which have version drift.",
			func(_ context.Context, _ DepAuditInput) (DepAuditOutput, error) {
				script := filepath.Join(dotfilesDir(), "scripts", "hg-dep-audit.sh")
				cmd := exec.Command(script)
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stdout
				cmd.Run()
				return DepAuditOutput{Output: stdout.String()}, nil
			},
		),

		// ── dotfiles_workflow_sync ─────────────────────
		handler.TypedHandler[WorkflowSyncInput, WorkflowSyncOutput](
			"dotfiles_workflow_sync",
			"Sync CI workflow files across all repos from canonical sources. Detects stale workflows and optionally updates, commits, and pushes.",
			func(_ context.Context, input WorkflowSyncInput) (WorkflowSyncOutput, error) {
				scriptArgs := []string{}
				if input.DryRun {
					scriptArgs = append(scriptArgs, "--dry-run")
				}
				if input.Push {
					scriptArgs = append(scriptArgs, "--push")
				} else {
					scriptArgs = append(scriptArgs, "--commit")
				}

				script := filepath.Join(dotfilesDir(), "scripts", "hg-workflow-sync.sh")
				cmd := exec.Command(script, scriptArgs...)
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stdout
				cmd.Run()
				return WorkflowSyncOutput{Output: stdout.String()}, nil
			},
		),

		// ── dotfiles_go_sync ──────────────────────────
		handler.TypedHandler[GoSyncInput, GoSyncOutput](
			"dotfiles_go_sync",
			"Sync Go version across all repos to match dotfiles/make/go-version. Updates go.mod and optionally runs go mod tidy.",
			func(_ context.Context, input GoSyncInput) (GoSyncOutput, error) {
				scriptArgs := []string{}
				if input.DryRun {
					scriptArgs = append(scriptArgs, "--dry-run")
				}
				if input.Tidy {
					scriptArgs = append(scriptArgs, "--tidy")
				}

				script := filepath.Join(dotfilesDir(), "scripts", "hg-go-sync.sh")
				cmd := exec.Command(script, scriptArgs...)
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stdout
				cmd.Run()
				return GoSyncOutput{Output: stdout.String()}, nil
			},
		),

		// ── dotfiles_gh_onboard_repos ─────────────────
		handler.TypedHandler[GHOnboardReposInput, GHOnboardReposOutput](
			"dotfiles_gh_onboard_repos",
			"Onboard public repos into a GitHub org for architecture reference. For each repo: forks to org (if not already there), clones locally, strips git history to a single squashed commit on the default branch, and force-pushes. Designed for batch operations on 8-15+ repos. Set execute=true to run (dry-run by default).",
			func(_ context.Context, input GHOnboardReposInput) (GHOnboardReposOutput, error) {
				if input.TargetOrg == "" {
					return GHOnboardReposOutput{}, fmt.Errorf("[%s] target_org is required", handler.ErrInvalidParam)
				}
				if len(input.Repos) == 0 {
					return GHOnboardReposOutput{}, fmt.Errorf("[%s] repos is required (array of owner/name strings)", handler.ErrInvalidParam)
				}

				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				dryRun := !input.Execute
				var results []GHOnboardResult

				for _, sourceRepo := range input.Repos {
					parts := strings.SplitN(sourceRepo, "/", 2)
					if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
						results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "failed", Message: "invalid format — use owner/name"})
						continue
					}
					repoName := parts[1]

					// Check if already exists in target org.
					checkCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", input.TargetOrg, repoName), "--jq", ".full_name")
					alreadyExists := checkCmd.Run() == nil

					// Fetch source metadata.
					metaCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", parts[0], repoName),
						"--jq", "{default_branch: .default_branch, private: .private, description: .description}")
					var metaOut bytes.Buffer
					metaCmd.Stdout = &metaOut
					if err := metaCmd.Run(); err != nil {
						results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "failed", Message: "source repo not found"})
						continue
					}

					var meta struct {
						DefaultBranch string `json:"default_branch"`
						Private       bool   `json:"private"`
						Description   string `json:"description"`
					}
					if err := json.Unmarshal(metaOut.Bytes(), &meta); err != nil {
						results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "failed", Message: "bad metadata: " + err.Error()})
						continue
					}

					if dryRun {
						action := "would fork, squash, clone"
						if alreadyExists {
							action = "would re-squash existing"
						}
						results = append(results, GHOnboardResult{
							Repo:    sourceRepo,
							Action:  "dry-run",
							Message: fmt.Sprintf("%s → %s/%s (branch: %s)", action, input.TargetOrg, repoName, meta.DefaultBranch),
						})
						continue
					}

					// Step 1: Fork to org if not already there.
					if !alreadyExists {
						forkCmd := exec.Command("gh", "repo", "fork", sourceRepo, "--org", input.TargetOrg, "--clone=false")
						if out, err := forkCmd.CombinedOutput(); err != nil {
							// gh repo fork fails if already forked — try creating fresh.
							results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "failed", Message: "fork failed: " + strings.TrimSpace(string(out))})
							continue
						}
					}

					// Step 2: Clone to temp dir.
					tmpDir, err := os.MkdirTemp("", "onboard-"+repoName+"-")
					if err != nil {
						results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "failed", Message: "mkdtemp: " + err.Error()})
						continue
					}

					cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", input.TargetOrg, repoName)
					cloneCmd := exec.Command("git", "clone", "--quiet", cloneURL, tmpDir)
					if out, err := cloneCmd.CombinedOutput(); err != nil {
						os.RemoveAll(tmpDir)
						results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "failed", Message: "clone failed: " + strings.TrimSpace(string(out))})
						continue
					}

					runGit := func(args ...string) (string, error) {
						c := exec.Command("git", args...)
						c.Dir = tmpDir
						out, err := c.CombinedOutput()
						return strings.TrimSpace(string(out)), err
					}

					// Step 3: Squash to single commit.
					runGit("checkout", meta.DefaultBranch)
					if _, err := runGit("checkout", "--orphan", "squashed"); err != nil {
						os.RemoveAll(tmpDir)
						results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "failed", Message: "orphan checkout failed"})
						continue
					}
					runGit("add", "-A")
					commitMsg := fmt.Sprintf("Initial commit (onboarded from %s)", sourceRepo)
					if _, err := runGit("commit", "-m", commitMsg); err != nil {
						os.RemoveAll(tmpDir)
						results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "failed", Message: "squash commit failed (empty repo?)"})
						continue
					}
					runGit("branch", "-M", meta.DefaultBranch)

					// Step 4: Force push squashed history.
					if _, err := runGit("push", "--force", "origin", meta.DefaultBranch); err != nil {
						os.RemoveAll(tmpDir)
						results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "failed", Message: "force push failed"})
						continue
					}

					// Delete non-default remote branches.
					branchOut, _ := runGit("branch", "-r")
					for _, line := range strings.Split(branchOut, "\n") {
						branch := strings.TrimSpace(line)
						branch = strings.TrimPrefix(branch, "origin/")
						if branch == "" || branch == meta.DefaultBranch || strings.HasPrefix(branch, "HEAD") {
							continue
						}
						runGit("push", "origin", "--delete", branch)
					}

					os.RemoveAll(tmpDir)

					// Step 5: Clone locally.
					localPath := filepath.Join(localDir, repoName)
					os.RemoveAll(localPath) // Remove stale local clone if exists.
					localClone := exec.Command("gh", "repo", "clone", fmt.Sprintf("%s/%s", input.TargetOrg, repoName), localPath)
					if out, err := localClone.CombinedOutput(); err != nil {
						results = append(results, GHOnboardResult{Repo: sourceRepo, Action: "partial", Message: "squashed and pushed but local clone failed: " + strings.TrimSpace(string(out))})
						continue
					}

					results = append(results, GHOnboardResult{
						Repo:    sourceRepo,
						Action:  "onboarded",
						Message: fmt.Sprintf("forked → %s/%s, squashed to 1 commit, cloned to %s", input.TargetOrg, repoName, localPath),
					})
				}

				return GHOnboardReposOutput{Results: results}, nil
			},
		),

		// ── dotfiles_gh_list_org_repos ────────────────
		handler.TypedHandler[GHListOrgReposInput, GHListOrgReposOutput](
			"dotfiles_gh_list_org_repos",
			"List all repos in a GitHub org with local clone sync status. Shows whether each repo is cloned locally, missing, or has a wrong remote.",
			func(_ context.Context, input GHListOrgReposInput) (GHListOrgReposOutput, error) {
				if input.Org == "" {
					return GHListOrgReposOutput{}, fmt.Errorf("[%s] org is required", handler.ErrInvalidParam)
				}

				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				cmd := exec.Command("gh", "api", "--paginate",
					fmt.Sprintf("/orgs/%s/repos?per_page=100&sort=updated", input.Org),
					"--jq", ".[] | {name, private, fork, archived, language, description}")
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				if err := cmd.Run(); err != nil {
					return GHListOrgReposOutput{}, fmt.Errorf("gh api failed: %v: %s", err, strings.TrimSpace(stderr.String()))
				}

				var repos []OrgRepoInfo
				var cloned, missing int
				dec := json.NewDecoder(strings.NewReader(stdout.String()))
				for dec.More() {
					var r OrgRepoInfo
					if err := dec.Decode(&r); err != nil {
						continue
					}

					localPath := filepath.Join(localDir, r.Name)
					if info, err := os.Stat(localPath); err != nil || !info.IsDir() {
						r.LocalStatus = "missing"
						missing++
					} else {
						remoteCmd := exec.Command("git", "remote", "get-url", "origin")
						remoteCmd.Dir = localPath
						var remoteOut bytes.Buffer
						remoteCmd.Stdout = &remoteOut
						if err := remoteCmd.Run(); err != nil {
							r.LocalStatus = "missing"
							missing++
						} else {
							remoteURL := strings.TrimSpace(remoteOut.String())
							expected1 := fmt.Sprintf("git@github.com:%s/%s.git", input.Org, r.Name)
							expected2 := fmt.Sprintf("https://github.com/%s/%s.git", input.Org, r.Name)
							expected3 := fmt.Sprintf("https://github.com/%s/%s", input.Org, r.Name)
							if remoteURL == expected1 || remoteURL == expected2 || remoteURL == expected3 {
								r.LocalStatus = "cloned"
								cloned++
							} else {
								r.LocalStatus = "wrong-remote"
							}
						}
					}

					// Apply filters.
					if input.ExcludeArchived && r.Archived {
						continue
					}
					if input.OnlyMissing && r.LocalStatus != "missing" {
						continue
					}
					if len(input.Languages) > 0 {
						match := false
						for _, lang := range input.Languages {
							if strings.EqualFold(r.Language, lang) {
								match = true
								break
							}
						}
						if !match {
							continue
						}
					}

					repos = append(repos, r)
				}

				return GHListOrgReposOutput{
					Total:   len(repos),
					Cloned:  cloned,
					Missing: missing,
					Repos:   repos,
				}, nil
			},
		),

		// ── dotfiles_gh_local_sync_audit ──────────────
		handler.TypedHandler[GHLocalSyncAuditInput, GHLocalSyncAuditOutput](
			"dotfiles_gh_local_sync_audit",
			"Audit local directory against GitHub org repos. Finds orphaned local dirs (no org repo), missing clones (org repo not cloned), and remote mismatches.",
			func(_ context.Context, input GHLocalSyncAuditInput) (GHLocalSyncAuditOutput, error) {
				if input.Org == "" {
					return GHLocalSyncAuditOutput{}, fmt.Errorf("[%s] org is required", handler.ErrInvalidParam)
				}

				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				// Get org repos.
				cmd := exec.Command("gh", "api", "--paginate",
					fmt.Sprintf("/orgs/%s/repos?per_page=100", input.Org),
					"--jq", ".[] | .name")
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				if err := cmd.Run(); err != nil {
					return GHLocalSyncAuditOutput{}, fmt.Errorf("gh api failed: %v", err)
				}

				orgRepos := make(map[string]bool)
				for _, name := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
					name = strings.TrimSpace(name)
					if name != "" {
						orgRepos[name] = true
					}
				}

				// Get local dirs.
				localEntries, err := os.ReadDir(localDir)
				if err != nil {
					return GHLocalSyncAuditOutput{}, fmt.Errorf("read local dir: %v", err)
				}

				localDirs := make(map[string]bool)
				for _, e := range localEntries {
					if e.IsDir() {
						localDirs[e.Name()] = true
					}
				}

				var entries []SyncAuditEntry
				var synced, orphaned, missingCount, mismatched int

				// Check each local dir.
				for name := range localDirs {
					localPath := filepath.Join(localDir, name)
					gitDir := filepath.Join(localPath, ".git")
					if _, err := os.Stat(gitDir); err != nil {
						continue // Not a git repo, skip.
					}

					if !orgRepos[name] {
						entries = append(entries, SyncAuditEntry{Name: name, Status: "orphaned", Details: "local dir has no matching org repo"})
						orphaned++
						continue
					}

					remoteCmd := exec.Command("git", "remote", "get-url", "origin")
					remoteCmd.Dir = localPath
					var remoteOut bytes.Buffer
					remoteCmd.Stdout = &remoteOut
					if err := remoteCmd.Run(); err != nil {
						entries = append(entries, SyncAuditEntry{Name: name, Status: "mismatched", Details: "no origin remote"})
						mismatched++
						continue
					}

					remoteURL := strings.TrimSpace(remoteOut.String())
					if !strings.Contains(remoteURL, input.Org+"/"+name) {
						entries = append(entries, SyncAuditEntry{Name: name, Status: "mismatched", Details: "origin=" + remoteURL})
						mismatched++
					} else {
						synced++
					}
				}

				// Check for missing clones.
				for name := range orgRepos {
					if !localDirs[name] {
						entries = append(entries, SyncAuditEntry{Name: name, Status: "missing", Details: "org repo not cloned locally"})
						missingCount++
					}
				}

				return GHLocalSyncAuditOutput{
					Synced:     synced,
					Orphaned:   orphaned,
					Missing:    missingCount,
					Mismatched: mismatched,
					Entries:    entries,
				}, nil
			},
		),

		// ── dotfiles_gh_bulk_archive ──────────────────
		handler.TypedHandler[GHBulkArchiveInput, GHBulkArchiveOutput](
			"dotfiles_gh_bulk_archive",
			"Archive multiple repos in a GitHub org. Useful for housekeeping old reference repos. Set execute=true to run (dry-run by default).",
			func(_ context.Context, input GHBulkArchiveInput) (GHBulkArchiveOutput, error) {
				if input.Org == "" {
					return GHBulkArchiveOutput{}, fmt.Errorf("[%s] org is required", handler.ErrInvalidParam)
				}
				if len(input.Repos) == 0 {
					return GHBulkArchiveOutput{}, fmt.Errorf("[%s] repos is required", handler.ErrInvalidParam)
				}

				dryRun := !input.Execute
				var results []TransferResult

				for _, repo := range input.Repos {
					// Check if already archived.
					checkCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", input.Org, repo), "--jq", ".archived")
					var checkOut bytes.Buffer
					checkCmd.Stdout = &checkOut
					if err := checkCmd.Run(); err != nil {
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "repo not found"})
						continue
					}
					if strings.TrimSpace(checkOut.String()) == "true" {
						results = append(results, TransferResult{Repo: repo, Action: "skipped", Message: "already archived"})
						continue
					}

					if dryRun {
						results = append(results, TransferResult{Repo: repo, Action: "dry-run", Message: "would archive " + input.Org + "/" + repo})
						continue
					}

					archiveCmd := exec.Command("gh", "api", "--method", "PATCH",
						fmt.Sprintf("repos/%s/%s", input.Org, repo),
						"-f", "archived=true", "--silent")
					if out, err := archiveCmd.CombinedOutput(); err != nil {
						results = append(results, TransferResult{Repo: repo, Action: "failed", Message: "archive failed: " + strings.TrimSpace(string(out))})
					} else {
						results = append(results, TransferResult{Repo: repo, Action: "archived", Message: "archived " + input.Org + "/" + repo})
					}
				}

				return GHBulkArchiveOutput{Results: results}, nil
			},
		),

		// ── dotfiles_gh_bulk_settings ─────────────────
		handler.TypedHandler[GHBulkSettingsInput, GHBulkSettingsOutput](
			"dotfiles_gh_bulk_settings",
			"Batch-apply repo settings across multiple org repos. Supports auto-delete head branches, wiki, projects, and merge strategy toggles. Set execute=true to run (dry-run by default).",
			func(_ context.Context, input GHBulkSettingsInput) (GHBulkSettingsOutput, error) {
				if input.Org == "" {
					return GHBulkSettingsOutput{}, fmt.Errorf("[%s] org is required", handler.ErrInvalidParam)
				}

				// Build the settings payload.
				settings := make(map[string]bool)
				var settingNames []string
				if input.DeleteBranchOnMerge != nil {
					settings["delete_branch_on_merge"] = *input.DeleteBranchOnMerge
					settingNames = append(settingNames, "delete_branch_on_merge")
				}
				if input.HasWiki != nil {
					settings["has_wiki"] = *input.HasWiki
					settingNames = append(settingNames, "has_wiki")
				}
				if input.HasProjects != nil {
					settings["has_projects"] = *input.HasProjects
					settingNames = append(settingNames, "has_projects")
				}
				if input.AllowSquashMerge != nil {
					settings["allow_squash_merge"] = *input.AllowSquashMerge
					settingNames = append(settingNames, "allow_squash_merge")
				}
				if input.AllowMergeCommit != nil {
					settings["allow_merge_commit"] = *input.AllowMergeCommit
					settingNames = append(settingNames, "allow_merge_commit")
				}
				if input.AllowRebaseMerge != nil {
					settings["allow_rebase_merge"] = *input.AllowRebaseMerge
					settingNames = append(settingNames, "allow_rebase_merge")
				}

				if len(settings) == 0 {
					return GHBulkSettingsOutput{}, fmt.Errorf("[%s] at least one setting must be specified", handler.ErrInvalidParam)
				}

				// Determine target repos.
				repos := input.Repos
				if len(repos) == 0 {
					cmd := exec.Command("gh", "api", "--paginate",
						fmt.Sprintf("/orgs/%s/repos?per_page=100", input.Org),
						"--jq", ".[] | select(.archived == false) | .name")
					var stdout bytes.Buffer
					cmd.Stdout = &stdout
					if err := cmd.Run(); err != nil {
						return GHBulkSettingsOutput{}, fmt.Errorf("failed to list org repos: %v", err)
					}
					for _, name := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
						name = strings.TrimSpace(name)
						if name != "" {
							repos = append(repos, name)
						}
					}
				}

				dryRun := !input.Execute
				var results []SettingsResult

				for _, repo := range repos {
					// Fetch current settings for before/after comparison.
					jqFields := []string{}
					for _, name := range settingNames {
						jqFields = append(jqFields, name)
					}
					jqExpr := "{" + strings.Join(jqFields, ", ") + "}"
					currentCmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/%s", input.Org, repo), "--jq", jqExpr)
					var currentOut bytes.Buffer
					currentCmd.Stdout = &currentOut
					var previous map[string]bool
					if err := currentCmd.Run(); err == nil {
						json.Unmarshal(currentOut.Bytes(), &previous)
					}

					// Check if all settings already match.
					allMatch := previous != nil && len(previous) == len(settings)
					if allMatch {
						for k, v := range settings {
							if previous[k] != v {
								allMatch = false
								break
							}
						}
					}
					if allMatch {
						results = append(results, SettingsResult{Repo: repo, Action: "already-correct", Applied: settingNames, Previous: previous})
						continue
					}

					if dryRun {
						results = append(results, SettingsResult{
							Repo:     repo,
							Action:   "dry-run",
							Applied:  settingNames,
							Previous: previous,
							Message:  fmt.Sprintf("would apply %d settings to %s/%s", len(settings), input.Org, repo),
						})
						continue
					}

					// Build gh api args.
					args := []string{"api", "--method", "PATCH", fmt.Sprintf("repos/%s/%s", input.Org, repo)}
					for k, v := range settings {
						val := "false"
						if v {
							val = "true"
						}
						args = append(args, "-F", k+"="+val)
					}
					args = append(args, "--silent")

					patchCmd := exec.Command("gh", args...)
					if out, err := patchCmd.CombinedOutput(); err != nil {
						results = append(results, SettingsResult{Repo: repo, Action: "failed", Message: "patch failed: " + strings.TrimSpace(string(out))})
					} else {
						results = append(results, SettingsResult{Repo: repo, Action: "applied", Applied: settingNames, Previous: previous})
					}
				}

				return GHBulkSettingsOutput{Results: results}, nil
			},
		),

		// ── dotfiles_gh_bulk_clone ────────────────────
		handler.TypedHandler[GHBulkCloneInput, GHBulkCloneOutput](
			"dotfiles_gh_bulk_clone",
			"Clone all missing org repos to the local directory. Skips repos already cloned. Pairs with dotfiles_gh_local_sync_audit to act on missing repos. Set execute=true to run (dry-run by default).",
			func(_ context.Context, input GHBulkCloneInput) (GHBulkCloneOutput, error) {
				if input.Org == "" {
					return GHBulkCloneOutput{}, fmt.Errorf("[%s] org is required", handler.ErrInvalidParam)
				}

				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				// List org repos.
				cmd := exec.Command("gh", "api", "--paginate",
					fmt.Sprintf("/orgs/%s/repos?per_page=100", input.Org),
					"--jq", ".[] | select(.archived == false) | .name")
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				if err := cmd.Run(); err != nil {
					return GHBulkCloneOutput{}, fmt.Errorf("gh api failed: %v", err)
				}

				dryRun := !input.Execute
				var results []CloneResult

				for _, name := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
					name = strings.TrimSpace(name)
					if name == "" {
						continue
					}

					localPath := filepath.Join(localDir, name)
					if info, err := os.Stat(localPath); err == nil && info.IsDir() {
						results = append(results, CloneResult{Repo: name, Action: "skipped", Message: "already exists"})
						continue
					}

					if dryRun {
						results = append(results, CloneResult{Repo: name, Action: "dry-run", Message: "would clone to " + localPath})
						continue
					}

					cloneCmd := exec.Command("gh", "repo", "clone", fmt.Sprintf("%s/%s", input.Org, name), localPath)
					if out, err := cloneCmd.CombinedOutput(); err != nil {
						results = append(results, CloneResult{Repo: name, Action: "failed", Message: strings.TrimSpace(string(out))})
					} else {
						results = append(results, CloneResult{Repo: name, Action: "cloned", Message: localPath})
					}
				}

				return GHBulkCloneOutput{Results: results}, nil
			},
		),

		// ── dotfiles_gh_pull_all ──────────────────────
		handler.TypedHandler[GHPullAllInput, GHPullAllOutput](
			"dotfiles_gh_pull_all",
			"Fetch or pull updates for all git repos in the local directory. Uses --ff-only by default to avoid merge conflicts. Set fetch_only=true to just fetch without merging.",
			func(_ context.Context, input GHPullAllInput) (GHPullAllOutput, error) {
				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				entries, err := os.ReadDir(localDir)
				if err != nil {
					return GHPullAllOutput{}, fmt.Errorf("read dir: %v", err)
				}

				var results []PullResult
				var total, updated, current, dirty, detached, failed int

				for _, e := range entries {
					if !e.IsDir() {
						continue
					}

					repoPath := filepath.Join(localDir, e.Name())
					gitDir := filepath.Join(repoPath, ".git")
					if _, err := os.Stat(gitDir); err != nil {
						continue // Not a git repo.
					}
					total++

					// Check for detached HEAD.
					headCmd := exec.Command("git", "symbolic-ref", "HEAD")
					headCmd.Dir = repoPath
					if err := headCmd.Run(); err != nil {
						results = append(results, PullResult{Repo: e.Name(), Action: "detached", Message: "detached HEAD — skipped"})
						detached++
						continue
					}

					// Check for dirty working tree (only matters for pull, not fetch).
					if !input.FetchOnly {
						statusCmd := exec.Command("git", "status", "--porcelain")
						statusCmd.Dir = repoPath
						var statusOut bytes.Buffer
						statusCmd.Stdout = &statusOut
						statusCmd.Run()
						if strings.TrimSpace(statusOut.String()) != "" {
							results = append(results, PullResult{Repo: e.Name(), Action: "dirty", Message: "has uncommitted changes — skipped"})
							dirty++
							continue
						}
					}

					var gitCmd *exec.Cmd
					if input.FetchOnly {
						gitCmd = exec.Command("git", "fetch", "--prune")
					} else {
						gitCmd = exec.Command("git", "pull", "--ff-only")
					}
					gitCmd.Dir = repoPath
					out, err := gitCmd.CombinedOutput()
					outStr := strings.TrimSpace(string(out))

					if err != nil {
						results = append(results, PullResult{Repo: e.Name(), Action: "failed", Message: outStr})
						failed++
					} else if strings.Contains(outStr, "Already up to date") || strings.Contains(outStr, "Already up-to-date") || (input.FetchOnly && outStr == "") {
						current++
					} else {
						results = append(results, PullResult{Repo: e.Name(), Action: "updated", Message: outStr})
						updated++
					}
				}

				return GHPullAllOutput{
					Total:    total,
					Updated:  updated,
					Current:  current,
					Dirty:    dirty,
					Detached: detached,
					Failed:   failed,
					Results:  results,
				}, nil
			},
		),

		// ── dotfiles_gh_clean_stale ──────────────────
		handler.TypedHandler[GHCleanStaleInput, GHCleanStaleOutput](
			"dotfiles_gh_clean_stale",
			"Remove local clones that have no matching repo in the GitHub org. Identifies orphaned directories (local git repos whose origin doesn't point to the org). Set execute=true to delete (dry-run by default).",
			func(_ context.Context, input GHCleanStaleInput) (GHCleanStaleOutput, error) {
				if input.Org == "" {
					return GHCleanStaleOutput{}, fmt.Errorf("[%s] org is required", handler.ErrInvalidParam)
				}

				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				// Get org repos.
				cmd := exec.Command("gh", "api", "--paginate",
					fmt.Sprintf("/orgs/%s/repos?per_page=100", input.Org),
					"--jq", ".[] | .name")
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				if err := cmd.Run(); err != nil {
					return GHCleanStaleOutput{}, fmt.Errorf("gh api failed: %v", err)
				}

				orgRepos := make(map[string]bool)
				for _, name := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
					name = strings.TrimSpace(name)
					if name != "" {
						orgRepos[name] = true
					}
				}

				entries, err := os.ReadDir(localDir)
				if err != nil {
					return GHCleanStaleOutput{}, fmt.Errorf("read dir: %v", err)
				}

				dryRun := !input.Execute
				var results []TransferResult

				for _, e := range entries {
					if !e.IsDir() {
						continue
					}

					name := e.Name()
					repoPath := filepath.Join(localDir, name)
					gitDir := filepath.Join(repoPath, ".git")
					if _, err := os.Stat(gitDir); err != nil {
						continue // Not a git repo, skip.
					}

					if orgRepos[name] {
						continue // Matches an org repo, keep it.
					}

					// Safety: check for uncommitted changes.
					statusCmd := exec.Command("git", "status", "--porcelain")
					statusCmd.Dir = repoPath
					var statusOut bytes.Buffer
					statusCmd.Stdout = &statusOut
					statusCmd.Run()
					if strings.TrimSpace(statusOut.String()) != "" {
						results = append(results, TransferResult{Repo: name, Action: "skipped", Message: "has uncommitted changes"})
						continue
					}

					// Safety: check for unpushed commits.
					unpushedCmd := exec.Command("git", "log", "@{u}..", "--oneline")
					unpushedCmd.Dir = repoPath
					var unpushedOut bytes.Buffer
					unpushedCmd.Stdout = &unpushedOut
					unpushedCmd.Run()
					unpushedLines := strings.TrimSpace(unpushedOut.String())
					if unpushedLines != "" {
						count := len(strings.Split(unpushedLines, "\n"))
						results = append(results, TransferResult{Repo: name, Action: "skipped", Message: fmt.Sprintf("has %d unpushed commits", count)})
						continue
					}

					if dryRun {
						results = append(results, TransferResult{Repo: name, Action: "dry-run", Message: "would remove " + repoPath})
					} else {
						if err := os.RemoveAll(repoPath); err != nil {
							results = append(results, TransferResult{Repo: name, Action: "failed", Message: err.Error()})
						} else {
							results = append(results, TransferResult{Repo: name, Action: "removed", Message: "deleted " + repoPath})
						}
					}
				}

				return GHCleanStaleOutput{Results: results}, nil
			},
		),

		// ── dotfiles_gh_full_sync ─────────────────────
		handler.TypedHandler[GHFullSyncInput, GHFullSyncOutput](
			"dotfiles_gh_full_sync",
			"One-command fleet sync: pulls all local repos, identifies missing org repos, clones them, and reports orphaned dirs. Set execute=true to clone missing repos (dry-run by default for clones; pull always runs).",
			func(_ context.Context, input GHFullSyncInput) (GHFullSyncOutput, error) {
				if input.Org == "" {
					return GHFullSyncOutput{}, fmt.Errorf("[%s] org is required", handler.ErrInvalidParam)
				}

				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				var details []FullSyncDetail
				var pulled, current, dirty, cloned, orphaned, failed int

				// Step 1: Pull all local repos.
				entries, err := os.ReadDir(localDir)
				if err != nil {
					return GHFullSyncOutput{}, fmt.Errorf("read dir: %v", err)
				}

				localDirs := make(map[string]bool)
				for _, e := range entries {
					if !e.IsDir() {
						continue
					}
					repoPath := filepath.Join(localDir, e.Name())
					gitDir := filepath.Join(repoPath, ".git")
					if _, err := os.Stat(gitDir); err != nil {
						continue
					}
					localDirs[e.Name()] = true

					// Check detached HEAD.
					headCmd := exec.Command("git", "symbolic-ref", "HEAD")
					headCmd.Dir = repoPath
					if err := headCmd.Run(); err != nil {
						details = append(details, FullSyncDetail{Repo: e.Name(), Action: "detached", Detail: "detached HEAD — skipped pull"})
						continue
					}

					// Check dirty.
					statusCmd := exec.Command("git", "status", "--porcelain")
					statusCmd.Dir = repoPath
					var statusOut bytes.Buffer
					statusCmd.Stdout = &statusOut
					statusCmd.Run()
					if strings.TrimSpace(statusOut.String()) != "" {
						dirty++
						details = append(details, FullSyncDetail{Repo: e.Name(), Action: "dirty", Detail: "uncommitted changes — skipped pull"})
						continue
					}

					// Pull.
					pullCmd := exec.Command("git", "pull", "--ff-only")
					pullCmd.Dir = repoPath
					out, err := pullCmd.CombinedOutput()
					outStr := strings.TrimSpace(string(out))
					if err != nil {
						failed++
						details = append(details, FullSyncDetail{Repo: e.Name(), Action: "pull-failed", Detail: outStr})
					} else if strings.Contains(outStr, "Already up to date") || strings.Contains(outStr, "Already up-to-date") {
						current++
					} else {
						pulled++
						details = append(details, FullSyncDetail{Repo: e.Name(), Action: "pulled", Detail: outStr})
					}
				}

				// Step 2: Cross-reference with org repos.
				orgCmd := exec.Command("gh", "api", "--paginate",
					fmt.Sprintf("/orgs/%s/repos?per_page=100", input.Org),
					"--jq", ".[] | select(.archived == false) | .name")
				var orgOut bytes.Buffer
				orgCmd.Stdout = &orgOut
				if err := orgCmd.Run(); err != nil {
					return GHFullSyncOutput{}, fmt.Errorf("gh api failed: %v", err)
				}

				orgRepos := make(map[string]bool)
				for _, name := range strings.Split(strings.TrimSpace(orgOut.String()), "\n") {
					name = strings.TrimSpace(name)
					if name != "" {
						orgRepos[name] = true
					}
				}

				// Step 3: Clone missing repos.
				for name := range orgRepos {
					if localDirs[name] {
						continue
					}

					localPath := filepath.Join(localDir, name)
					if !input.Execute {
						details = append(details, FullSyncDetail{Repo: name, Action: "missing", Detail: "would clone to " + localPath})
						continue
					}

					cloneCmd := exec.Command("gh", "repo", "clone", fmt.Sprintf("%s/%s", input.Org, name), localPath)
					if out, err := cloneCmd.CombinedOutput(); err != nil {
						failed++
						details = append(details, FullSyncDetail{Repo: name, Action: "clone-failed", Detail: strings.TrimSpace(string(out))})
					} else {
						cloned++
						details = append(details, FullSyncDetail{Repo: name, Action: "cloned", Detail: localPath})
					}
				}

				// Step 4: Report orphaned local dirs.
				for name := range localDirs {
					if !orgRepos[name] {
						orphaned++
						details = append(details, FullSyncDetail{Repo: name, Action: "orphaned", Detail: "local dir has no matching org repo"})
					}
				}

				return GHFullSyncOutput{
					Pulled:   pulled,
					Current:  current,
					Dirty:    dirty,
					Cloned:   cloned,
					Orphaned: orphaned,
					Failed:   failed,
					Details:  details,
				}, nil
			},
		),

		// ── dotfiles_create_repo ──────────────────────
		handler.TypedHandler[CreateRepoInput, CreateRepoOutput](
			"dotfiles_create_repo",
			"Scaffold a new hairglasses-studio repo with standard files (CI, LICENSE, .editorconfig, pre-commit hooks). Auto-detects or uses specified language for templates.",
			func(_ context.Context, input CreateRepoInput) (CreateRepoOutput, error) {
				if input.Name == "" {
					return CreateRepoOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}

				scriptArgs := []string{input.Name}
				if input.Language != "" {
					scriptArgs = append(scriptArgs, "--language="+input.Language)
				}
				if input.Private {
					scriptArgs = append(scriptArgs, "--private")
				}

				script := filepath.Join(dotfilesDir(), "scripts", "hg-new-repo.sh")
				cmd := exec.Command(script, scriptArgs...)
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				err := cmd.Run()
				status := "ok"
				if err != nil {
					status = "fail"
				}

				repoPath := filepath.Join(homeDir(), "hairglasses-studio", input.Name)
				return CreateRepoOutput{
					RepoPath: repoPath,
					Status:   status,
					Output:   stdout.String(),
				}, nil
			},
		),

		// ── dotfiles_fleet_audit ──────────────────────
		handler.TypedHandler[FleetAuditInput, FleetAuditOutput](
			"dotfiles_fleet_audit",
			"Comprehensive fleet audit: per-repo language, Go version, CI status, test count, last commit age, pipeline.mk and CLAUDE.md presence. Single tool replaces manual health+dep+CI checks.",
			func(_ context.Context, input FleetAuditInput) (FleetAuditOutput, error) {
				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				entries, err := os.ReadDir(localDir)
				if err != nil {
					return FleetAuditOutput{}, fmt.Errorf("read dir: %v", err)
				}

				var repos []RepoAuditInfo
				var total, passing, failing, goRepos int

				for _, e := range entries {
					if !e.IsDir() {
						continue
					}
					repoPath := filepath.Join(localDir, e.Name())
					gitDir := filepath.Join(repoPath, ".git")
					if _, err := os.Stat(gitDir); err != nil {
						continue
					}
					total++

					info := RepoAuditInfo{Name: e.Name()}

					// Detect language.
					if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err == nil {
						info.Language = "go"
						goRepos++
						// Read Go version.
						goModCmd := exec.Command("grep", "-m1", "^go ", filepath.Join(repoPath, "go.mod"))
						var goModOut bytes.Buffer
						goModCmd.Stdout = &goModOut
						if goModCmd.Run() == nil {
							parts := strings.Fields(strings.TrimSpace(goModOut.String()))
							if len(parts) >= 2 {
								info.GoVersion = parts[1]
							}
						}
					} else if _, err := os.Stat(filepath.Join(repoPath, "package.json")); err == nil {
						info.Language = "node"
					} else if _, err := os.Stat(filepath.Join(repoPath, "pyproject.toml")); err == nil {
						info.Language = "python"
					} else {
						info.Language = "shell"
					}

					// Last commit age.
					logCmd := exec.Command("git", "log", "-1", "--format=%ct")
					logCmd.Dir = repoPath
					var logOut bytes.Buffer
					logCmd.Stdout = &logOut
					if logCmd.Run() == nil {
						ts := strings.TrimSpace(logOut.String())
						if ts != "" {
							var epoch int64
							fmt.Sscanf(ts, "%d", &epoch)
							nowCmd := exec.Command("date", "+%s")
							var nowOut bytes.Buffer
							nowCmd.Stdout = &nowOut
							nowCmd.Run()
							var nowEpoch int64
							fmt.Sscanf(strings.TrimSpace(nowOut.String()), "%d", &nowEpoch)
							info.LastCommitDays = int((nowEpoch - epoch) / 86400)
						}
					}

					// CI status — derive org/repo from git remote.
					ciRemoteCmd := exec.Command("git", "config", "remote.origin.url")
					ciRemoteCmd.Dir = repoPath
					var ciRemoteOut bytes.Buffer
					ciRemoteCmd.Stdout = &ciRemoteOut
					repoSlug := e.Name()
					if ciRemoteCmd.Run() == nil {
						url := strings.TrimSpace(ciRemoteOut.String())
						url = strings.TrimSuffix(url, ".git")
						if idx := strings.Index(url, "github.com"); idx >= 0 {
							slug := url[idx+len("github.com"):]
							slug = strings.TrimPrefix(slug, ":")
							slug = strings.TrimPrefix(slug, "/")
							if slug != "" {
								repoSlug = slug
							}
						}
					}
					ciCmd := exec.Command("gh", "run", "list", "--repo", repoSlug, "--limit", "1", "--json", "conclusion", "--jq", ".[0].conclusion")
					ciCmd.Dir = repoPath
					var ciOut bytes.Buffer
					ciCmd.Stdout = &ciOut
					if ciCmd.Run() == nil {
						conclusion := strings.TrimSpace(ciOut.String())
						switch conclusion {
						case "success":
							info.CIStatus = "pass"
							passing++
						case "failure":
							info.CIStatus = "fail"
							failing++
						case "":
							info.CIStatus = "none"
						default:
							info.CIStatus = conclusion
						}
					} else {
						info.CIStatus = "none"
					}

					// Test count.
					var testCount int
					switch info.Language {
					case "go":
						countCmd := exec.Command("bash", "-c", "find . -name '*_test.go' -not -path './vendor/*' 2>/dev/null | wc -l")
						countCmd.Dir = repoPath
						var countOut bytes.Buffer
						countCmd.Stdout = &countOut
						countCmd.Run()
						fmt.Sscanf(strings.TrimSpace(countOut.String()), "%d", &testCount)
					case "node":
						countCmd := exec.Command("bash", "-c", "find . -name '*.test.*' -o -name '*.spec.*' 2>/dev/null | wc -l")
						countCmd.Dir = repoPath
						var countOut bytes.Buffer
						countCmd.Stdout = &countOut
						countCmd.Run()
						fmt.Sscanf(strings.TrimSpace(countOut.String()), "%d", &testCount)
					case "python":
						countCmd := exec.Command("bash", "-c", "find . -name 'test_*.py' -o -name '*_test.py' 2>/dev/null | wc -l")
						countCmd.Dir = repoPath
						var countOut bytes.Buffer
						countCmd.Stdout = &countOut
						countCmd.Run()
						fmt.Sscanf(strings.TrimSpace(countOut.String()), "%d", &testCount)
					}
					info.TestCount = testCount

					// pipeline.mk check.
					if _, err := os.Stat(filepath.Join(repoPath, "pipeline.mk")); err == nil {
						info.HasPipelineMk = true
					} else {
						// Check Makefile for include.
						makeCmd := exec.Command("grep", "-q", "pipeline.mk", filepath.Join(repoPath, "Makefile"))
						info.HasPipelineMk = makeCmd.Run() == nil
					}

					// CLAUDE.md check.
					_, err := os.Stat(filepath.Join(repoPath, "CLAUDE.md"))
					info.HasCLAUDEmd = err == nil

					// CI workflows check.
					_, err = os.Stat(filepath.Join(repoPath, ".github", "workflows"))
					info.HasCI = err == nil

					repos = append(repos, info)
				}

				return FleetAuditOutput{
					Total:   total,
					Passing: passing,
					Failing: failing,
					GoRepos: goRepos,
					Repos:   repos,
				}, nil
			},
		),

		// ── dotfiles_cascade_reload ──────────────────
		handler.TypedHandler[CascadeReloadInput, CascadeReloadOutput](
			"dotfiles_cascade_reload",
			"Atomic multi-service reload with health verification. Reloads services in order (default: hyprland → mako → eww), verifying each is healthy before proceeding to the next.",
			func(_ context.Context, input CascadeReloadInput) (CascadeReloadOutput, error) {
				services := input.Services
				if len(services) == 0 {
					services = []string{"hyprland", "mako", "eww"}
				}

				var results []ServiceReloadStatus

				for _, svc := range services {
					cmds, ok := reloadCommands[svc]
					if !ok {
						results = append(results, ServiceReloadStatus{Service: svc, Action: "skipped", Message: "unknown service"})
						continue
					}

					cmd := exec.Command(cmds[0], cmds[1:]...)
					out, err := cmd.CombinedOutput()
					if err != nil {
						results = append(results, ServiceReloadStatus{
							Service: svc,
							Action:  "failed",
							Message: strings.TrimSpace(string(out)),
						})
						continue
					}

					// Verify health after reload.
					var healthy bool
					switch svc {
					case "hyprland":
						checkCmd := exec.Command("hyprctl", "configerrors")
						var checkOut bytes.Buffer
						checkCmd.Stdout = &checkOut
						if checkCmd.Run() == nil {
							output := strings.TrimSpace(checkOut.String())
							healthy = output == "" || strings.Contains(output, "no errors")
						}
					case "eww":
						checkCmd := exec.Command("eww", "ping")
						healthy = checkCmd.Run() == nil
					default:
						healthy = true // No health check available.
					}

					if healthy {
						results = append(results, ServiceReloadStatus{Service: svc, Action: "reloaded", Message: "healthy"})
					} else {
						results = append(results, ServiceReloadStatus{Service: svc, Action: "reloaded", Message: "reload succeeded but health check failed"})
					}
				}

				return CascadeReloadOutput{Results: results}, nil
			},
		),

		// ── dotfiles_rice_check ──────────────────────
		handler.TypedHandler[RiceCheckInput, RiceCheckOutput](
			"dotfiles_rice_check",
			"Rice health check: reports compositor, shader, wallpaper, service states, and (with level=full) scans all configs for non-Snazzy palette violations.",
			func(_ context.Context, input RiceCheckInput) (RiceCheckOutput, error) {
				level := input.Level
				if level == "" {
					level = "quick"
				}

				result := RiceCheckOutput{}
				dir := dotfilesDir()

				// Compositor detection.
				if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" {
					result.Compositor = "hyprland"
				} else if os.Getenv("SWAYSOCK") != "" {
					result.Compositor = "sway"
				} else {
					result.Compositor = "unknown"
				}

				// Active shader.
				ghosttyConfig := filepath.Join(homeDir(), ".config", "ghostty", "config")
				shaderCmd := exec.Command("grep", "-m1", "^custom-shader = ", ghosttyConfig)
				var shaderOut bytes.Buffer
				shaderCmd.Stdout = &shaderOut
				if shaderCmd.Run() == nil {
					line := strings.TrimSpace(shaderOut.String())
					line = strings.TrimPrefix(line, "custom-shader = ")
					if line != "" && line != "none" {
						result.Shader = filepath.Base(strings.TrimSuffix(line, ".glsl"))
					} else {
						result.Shader = "none"
					}
				} else {
					result.Shader = "none"
				}

				// Wallpaper.
				if exec.Command("pgrep", "-f", "shaderbg").Run() == nil {
					stateFile := filepath.Join(homeDir(), ".local", "state", "shader-wallpaper", "current")
					if data, err := os.ReadFile(stateFile); err == nil {
						result.Wallpaper = "shader:" + filepath.Base(strings.TrimSuffix(strings.TrimSpace(string(data)), ".frag"))
					} else {
						result.Wallpaper = "shader:unknown"
					}
				} else {
					result.Wallpaper = "static"
				}

				// Service status.
				checkServices := []string{"hyprland", "eww", "mako", "waybar", "swww-daemon", "hypridle"}
				for _, svc := range checkServices {
					pgrepCmd := exec.Command("pgrep", "-x", svc)
					action := "stopped"
					if pgrepCmd.Run() == nil {
						action = "running"
					}
					result.Services = append(result.Services, ServiceReloadStatus{Service: svc, Action: action})
				}

				// Palette scan (full mode only).
				if level == "full" {
					snazzyAllowed := map[string]bool{
						"57c7ff": true, "ff6ac1": true, "5af78e": true, "f3f99d": true,
						"ff5c57": true, "686868": true, "9aedfe": true, "eff0eb": true,
						"f1f1f0": true, "000000": true, "1a1a1a": true, "1a1b26": true,
					}
					scanDirs := []string{
						filepath.Join(dir, "hyprland"),
						filepath.Join(dir, "eww"),
						filepath.Join(dir, "mako"),
						filepath.Join(dir, "wofi"),
						filepath.Join(dir, "waybar"),
						filepath.Join(dir, "foot"),
					}

					for _, scanDir := range scanDirs {
						grepCmd := exec.Command("grep", "-rnE", `#[0-9a-fA-F]{6}`, scanDir)
						var grepOut bytes.Buffer
						grepCmd.Stdout = &grepOut
						grepCmd.Run() // Ignore errors (no matches is fine).

						for _, line := range strings.Split(grepOut.String(), "\n") {
							line = strings.TrimSpace(line)
							if line == "" {
								continue
							}
							// Extract hex color from line.
							for i := 0; i < len(line); i++ {
								if line[i] == '#' && i+6 < len(line) {
									hex := strings.ToLower(line[i+1 : i+7])
									isHex := true
									for _, c := range hex {
										if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
											isHex = false
											break
										}
									}
									if isHex && !snazzyAllowed[hex] {
										parts := strings.SplitN(line, ":", 3)
										file := ""
										lineNum := 0
										if len(parts) >= 2 {
											file = parts[0]
											fmt.Sscanf(parts[1], "%d", &lineNum)
										}
										result.PaletteViolations = append(result.PaletteViolations, PaletteViolation{
											File:  filepath.Base(file),
											Line:  lineNum,
											Color: "#" + hex,
										})
									}
								}
							}
						}
					}
				}

				return result, nil
			},
		),

		// ── dotfiles_bulk_pipeline ────────────────────
		handler.TypedHandler[BulkPipelineInput, BulkPipelineOutput](
			"dotfiles_bulk_pipeline",
			"Run build+test pipeline across multiple repos. Detects language and runs appropriate build/test steps. Returns per-repo pass/fail with output.",
			func(_ context.Context, input BulkPipelineInput) (BulkPipelineOutput, error) {
				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				script := filepath.Join(dotfilesDir(), "scripts", "hg-pipeline.sh")

				// Determine repos.
				repos := input.Repos
				if len(repos) == 0 {
					entries, err := os.ReadDir(localDir)
					if err != nil {
						return BulkPipelineOutput{}, fmt.Errorf("read dir: %v", err)
					}
					for _, e := range entries {
						if !e.IsDir() {
							continue
						}
						repoPath := filepath.Join(localDir, e.Name())
						// Filter by language if specified.
						switch input.Language {
						case "go":
							if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err != nil {
								continue
							}
						case "node":
							if _, err := os.Stat(filepath.Join(repoPath, "package.json")); err != nil {
								continue
							}
						case "python":
							if _, err := os.Stat(filepath.Join(repoPath, "pyproject.toml")); err != nil {
								continue
							}
						default:
							// Must have a Makefile or language marker.
							hasLang := false
							for _, marker := range []string{"go.mod", "package.json", "pyproject.toml", "Makefile"} {
								if _, err := os.Stat(filepath.Join(repoPath, marker)); err == nil {
									hasLang = true
									break
								}
							}
							if !hasLang {
								continue
							}
						}
						repos = append(repos, e.Name())
					}
				}

				var results []PipelineResult
				var passed, failed int

				for _, repo := range repos {
					repoPath := filepath.Join(localDir, repo)
					args := []string{repoPath, "--json"}
					if input.BuildOnly {
						args = append(args, "--build-only")
					}
					if input.TestOnly {
						args = append(args, "--test-only")
					}

					cmd := exec.Command(script, args...)
					var stdout bytes.Buffer
					cmd.Stdout = &stdout
					cmd.Stderr = &stdout
					err := cmd.Run()

					status := "pass"
					if err != nil {
						exitCode := 1
						if exitErr, ok := err.(*exec.ExitError); ok {
							exitCode = exitErr.ExitCode()
						}
						switch exitCode {
						case 1:
							status = "build-fail"
						case 2:
							status = "test-fail"
						case 3:
							status = "vet-fail"
						default:
							status = "fail"
						}
						failed++
					} else {
						passed++
					}

					results = append(results, PipelineResult{
						Repo:   repo,
						Status: status,
						Output: strings.TrimSpace(stdout.String()),
					})
				}

				return BulkPipelineOutput{
					Total:   len(results),
					Passed:  passed,
					Failed:  failed,
					Results: results,
				}, nil
			},
		),

		// ── dotfiles_mcpkit_version_sync ──────────────
		handler.TypedHandler[MCPKitVersionSyncInput, MCPKitVersionSyncOutput](
			"dotfiles_mcpkit_version_sync",
			"Sync mcpkit dependency version across all thin MCP servers. Finds latest mcpkit version and updates go.mod in each MCP repo. Set execute=true to run go get + go mod tidy (dry-run by default).",
			func(_ context.Context, input MCPKitVersionSyncInput) (MCPKitVersionSyncOutput, error) {
				localDir := input.LocalDir
				if localDir == "" {
					localDir = filepath.Join(homeDir(), "hairglasses-studio")
				}

				mcpRepos := []string{"dotfiles-mcp", "process-mcp", "systemd-mcp", "tmux-mcp"}
				dryRun := !input.Execute

				// Get latest mcpkit version from the first repo that has it.
				var latestVersion string
				for _, repo := range mcpRepos {
					goModPath := filepath.Join(localDir, repo, "go.mod")
					grepCmd := exec.Command("grep", "github.com/hairglasses-studio/mcpkit", goModPath)
					var grepOut bytes.Buffer
					grepCmd.Stdout = &grepOut
					if grepCmd.Run() == nil {
						line := strings.TrimSpace(grepOut.String())
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							latestVersion = fields[len(fields)-1]
							break
						}
					}
				}

				// Get truly latest from Go proxy.
				listCmd := exec.Command("go", "list", "-m", "-versions", "github.com/hairglasses-studio/mcpkit@latest")
				listCmd.Dir = filepath.Join(localDir, mcpRepos[0])
				var listOut bytes.Buffer
				listCmd.Stdout = &listOut
				if listCmd.Run() == nil {
					parts := strings.Fields(strings.TrimSpace(listOut.String()))
					if len(parts) >= 2 {
						latestVersion = parts[len(parts)-1]
					}
				}

				var results []MCPKitSyncResult

				for _, repo := range mcpRepos {
					repoPath := filepath.Join(localDir, repo)
					goModPath := filepath.Join(repoPath, "go.mod")

					if _, err := os.Stat(goModPath); err != nil {
						results = append(results, MCPKitSyncResult{Repo: repo, Action: "skipped", Message: "no go.mod"})
						continue
					}

					// Get current version.
					grepCmd := exec.Command("grep", "github.com/hairglasses-studio/mcpkit", goModPath)
					var grepOut bytes.Buffer
					grepCmd.Stdout = &grepOut
					currentVersion := ""
					if grepCmd.Run() == nil {
						line := strings.TrimSpace(grepOut.String())
						// Skip replace directives.
						if strings.Contains(line, "=>") {
							results = append(results, MCPKitSyncResult{Repo: repo, Action: "skipped", Message: "uses local replace directive"})
							continue
						}
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							currentVersion = fields[len(fields)-1]
						}
					} else {
						results = append(results, MCPKitSyncResult{Repo: repo, Action: "skipped", Message: "mcpkit not in go.mod"})
						continue
					}

					if currentVersion == latestVersion {
						results = append(results, MCPKitSyncResult{Repo: repo, Action: "already-current", OldVersion: currentVersion, NewVersion: latestVersion})
						continue
					}

					if dryRun {
						results = append(results, MCPKitSyncResult{
							Repo:       repo,
							Action:     "dry-run",
							OldVersion: currentVersion,
							NewVersion: latestVersion,
							Message:    fmt.Sprintf("would update %s → %s", currentVersion, latestVersion),
						})
						continue
					}

					// Run go get to update.
					getCmd := exec.Command("go", "get", "github.com/hairglasses-studio/mcpkit@"+latestVersion)
					getCmd.Dir = repoPath
					if out, err := getCmd.CombinedOutput(); err != nil {
						results = append(results, MCPKitSyncResult{Repo: repo, Action: "failed", OldVersion: currentVersion, Message: "go get failed: " + strings.TrimSpace(string(out))})
						continue
					}

					// Run go mod tidy.
					tidyCmd := exec.Command("go", "mod", "tidy")
					tidyCmd.Dir = repoPath
					tidyCmd.CombinedOutput()

					results = append(results, MCPKitSyncResult{
						Repo:       repo,
						Action:     "updated",
						OldVersion: currentVersion,
						NewVersion: latestVersion,
					})
				}

				return MCPKitVersionSyncOutput{
					LatestVersion: latestVersion,
					Results:       results,
				}, nil
			},
		),
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	reg := registry.NewToolRegistry(registry.Config{
		Middleware: []registry.Middleware{
			registry.AuditMiddleware(""),
			registry.SafetyTierMiddleware(),
		},
	})
	registerDotfilesModules(reg)

	s := registry.NewMCPServer("dotfiles-mcp", "2.1.0")
	reg.RegisterWithServer(s)

	if err := registry.ServeAuto(s); err != nil {
		log.Fatal(err)
	}
}
