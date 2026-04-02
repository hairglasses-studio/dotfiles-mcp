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

// Tool 8: dotfiles_eww_restart

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
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	reg := registry.NewToolRegistry()
	reg.RegisterModule(&DotfilesModule{})

	s := registry.NewMCPServer("dotfiles-mcp", "1.0.0")
	reg.RegisterWithServer(s)

	if err := registry.ServeStdio(s); err != nil {
		log.Fatal(err)
	}
}
