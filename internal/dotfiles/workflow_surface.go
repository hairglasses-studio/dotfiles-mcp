package dotfiles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type repoBootstrapWorkflowInput struct {
	Action   string `json:"action" jsonschema:"required,description=Workflow action: create or onboard"`
	Name     string `json:"name,omitempty" jsonschema:"description=Repo name when action=create"`
	RepoPath string `json:"repo_path,omitempty" jsonschema:"description=Existing repo path when action=onboard"`
	Language string `json:"language,omitempty" jsonschema:"description=Optional language override"`
	Private  bool   `json:"private,omitempty" jsonschema:"description=Create the repo as private when action=create"`
	DryRun   bool   `json:"dry_run,omitempty" jsonschema:"description=Pass --dry-run when the selected script supports it"`
}

type repoBootstrapWorkflowOutput struct {
	Action           string   `json:"action"`
	Status           string   `json:"status"`
	RepoPath         string   `json:"repo_path,omitempty"`
	Summary          string   `json:"summary"`
	Command          []string `json:"command"`
	OutputPreview    string   `json:"output_preview,omitempty"`
	ErrorPreview     string   `json:"error_preview,omitempty"`
	RecommendedTools []string `json:"recommended_tools"`
}

type repoSyncWorkflowInput struct {
	Execute bool `json:"execute,omitempty" jsonschema:"description=When false, run workflow sync in dry-run mode. When true, apply updates."`
	Push    bool `json:"push,omitempty" jsonschema:"description=When execute=true, push committed workflow updates"`
}

type repoSyncWorkflowOutput struct {
	Mode             string   `json:"mode"`
	Status           string   `json:"status"`
	Summary          string   `json:"summary"`
	Command          []string `json:"command"`
	OutputPreview    string   `json:"output_preview,omitempty"`
	RecommendedTools []string `json:"recommended_tools"`
}

type codexAuditWorkflowInput struct {
	WriteArtifacts bool `json:"write_artifacts,omitempty" jsonschema:"description=Persist JSON/markdown audit artifacts into the docs repo and workspace cache"`
}

type codexAuditWorkflowOutput struct {
	Status           string            `json:"status"`
	Summary          string            `json:"summary"`
	Metrics          map[string]string `json:"metrics"`
	OutputPreview    []string          `json:"output_preview,omitempty"`
	RecommendedTools []string          `json:"recommended_tools"`
}

type fleetHealthWorkflowOutput struct {
	Status                string   `json:"status"`
	Summary               string   `json:"summary"`
	FleetHealthPreview    []string `json:"fleet_health_preview"`
	DependencySkewPreview []string `json:"dependency_skew_preview"`
	RecommendedTools      []string `json:"recommended_tools"`
}

type repoGitHygieneWorkflowInput struct {
	RepoPath            string   `json:"repo_path" jsonschema:"required,description=Repo path to scan or clean"`
	Execute             bool     `json:"execute,omitempty" jsonschema:"description=When false, only plan cleanup. When true, apply the selected cleanup actions."`
	Fetch               bool     `json:"fetch,omitempty" jsonschema:"description=Refresh origin refs with git fetch --prune before scanning"`
	PruneAdmin          bool     `json:"prune_admin,omitempty" jsonschema:"description=Run git worktree prune after cleanup when execute=true"`
	CleanupLocalMerged  bool     `json:"cleanup_local_merged,omitempty" jsonschema:"description=Delete merged local branches other than the default/current branch"`
	CleanupRemoteMerged bool     `json:"cleanup_remote_merged,omitempty" jsonschema:"description=Delete merged origin branches other than the default branch"`
	CleanupWorktrees    bool     `json:"cleanup_worktrees,omitempty" jsonschema:"description=Remove extra clean worktrees whose branch tip is merged into the default branch"`
	PruneManagedState   bool     `json:"prune_managed_state,omitempty" jsonschema:"description=Also prune Codex-managed worktree metadata via hg-codex-worktree-prune.sh"`
	BranchPrefixes      []string `json:"branch_prefixes,omitempty" jsonschema:"description=Optional branch prefixes that limit cleanup candidates"`
}

type repoGitHygieneBranch struct {
	Name                 string `json:"name"`
	DefaultBranch        string `json:"default_branch,omitempty"`
	Current              bool   `json:"current"`
	MergedIntoDefault    bool   `json:"merged_into_default"`
	EligibleForCleanup   bool   `json:"eligible_for_cleanup"`
	CheckedOutInWorktree bool   `json:"checked_out_in_worktree,omitempty"`
	Ahead                int    `json:"ahead,omitempty"`
	Behind               int    `json:"behind,omitempty"`
}

type repoGitHygieneWorktree struct {
	Path               string `json:"path"`
	Branch             string `json:"branch,omitempty"`
	Current            bool   `json:"current"`
	Missing            bool   `json:"missing"`
	Dirty              bool   `json:"dirty"`
	MergedIntoDefault  bool   `json:"merged_into_default"`
	EligibleForCleanup bool   `json:"eligible_for_cleanup"`
	Prunable           bool   `json:"prunable"`
}

type repoGitHygieneAction struct {
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type repoGitHygieneSummary struct {
	LocalBranchCount            int `json:"local_branch_count"`
	LocalMergedCount            int `json:"local_merged_count"`
	LocalCleanupCandidateCount  int `json:"local_cleanup_candidate_count"`
	RemoteBranchCount           int `json:"remote_branch_count"`
	RemoteMergedCount           int `json:"remote_merged_count"`
	RemoteCleanupCandidateCount int `json:"remote_cleanup_candidate_count"`
	ExtraWorktreeCount          int `json:"extra_worktree_count"`
	CleanMergedWorktreeCount    int `json:"clean_merged_worktree_count"`
	DirtyWorktreeCount          int `json:"dirty_worktree_count"`
	BlockedWorktreeCount        int `json:"blocked_worktree_count"`
	ActionCount                 int `json:"action_count"`
	CompletedActionCount        int `json:"completed_action_count"`
	PlannedActionCount          int `json:"planned_action_count"`
	FailedActionCount           int `json:"failed_action_count"`
}

type repoGitHygieneWorkflowOutput struct {
	Repo                string                   `json:"repo"`
	DefaultBranch       string                   `json:"default_branch"`
	CurrentBranch       string                   `json:"current_branch"`
	Mode                string                   `json:"mode"`
	Summary             repoGitHygieneSummary    `json:"summary"`
	LocalBranches       []repoGitHygieneBranch   `json:"local_branches"`
	RemoteBranches      []repoGitHygieneBranch   `json:"remote_branches"`
	Worktrees           []repoGitHygieneWorktree `json:"worktrees"`
	Actions             []repoGitHygieneAction   `json:"actions"`
	ManagedStatePreview []string                 `json:"managed_state_preview,omitempty"`
	Command             []string                 `json:"command"`
	RecommendedTools    []string                 `json:"recommended_tools"`
}

type FleetWorkflowSurfaceModule struct{}

func (m *FleetWorkflowSurfaceModule) Name() string { return "fleet_workflows" }
func (m *FleetWorkflowSurfaceModule) Description() string {
	return "Composed repo bootstrap, sync, audit, and fleet-health workflows"
}

func (m *FleetWorkflowSurfaceModule) Tools() []registry.ToolDefinition {
	repoGitHygiene := handler.TypedHandler[repoGitHygieneWorkflowInput, repoGitHygieneWorkflowOutput](
		"dotfiles_repo_git_hygiene",
		"Scan or safely clean merged branches, extra worktrees, and managed worktree residue for one repo with dry-run-first output.",
		func(_ context.Context, input repoGitHygieneWorkflowInput) (repoGitHygieneWorkflowOutput, error) {
			return runRepoGitHygieneWorkflow(input)
		},
	)
	repoGitHygiene.Category = "workflow"
	repoGitHygiene.SearchTerms = []string{"repo hygiene", "branch cleanup", "worktree cleanup", "git cleanup", "merged branches"}
	repoGitHygiene.IsWrite = true

	repoBootstrap := handler.TypedHandler[repoBootstrapWorkflowInput, repoBootstrapWorkflowOutput](
		"dotfiles_repo_bootstrap",
		"Create or onboard a repo through the standard studio scripts, returning a short summary plus the next recommended Codex tools.",
		func(_ context.Context, input repoBootstrapWorkflowInput) (repoBootstrapWorkflowOutput, error) {
			action := strings.ToLower(strings.TrimSpace(input.Action))
			var cmd *exec.Cmd
			var command []string
			repoPath := input.RepoPath

			switch action {
			case "create":
				if strings.TrimSpace(input.Name) == "" {
					return repoBootstrapWorkflowOutput{}, fmt.Errorf("[%s] name is required when action=create", handler.ErrInvalidParam)
				}
				script := filepath.Join(dotfilesDir(), "scripts", "hg-new-repo.sh")
				args := []string{input.Name}
				if input.Language != "" {
					args = append(args, "--language="+input.Language)
				}
				if input.Private {
					args = append(args, "--private")
				}
				command = append([]string{script}, args...)
				cmd = exec.Command(script, args...)
				repoPath = filepath.Join(homeDir(), "hairglasses-studio", input.Name)
			case "onboard":
				if strings.TrimSpace(input.RepoPath) == "" {
					return repoBootstrapWorkflowOutput{}, fmt.Errorf("[%s] repo_path is required when action=onboard", handler.ErrInvalidParam)
				}
				script := filepath.Join(dotfilesDir(), "scripts", "hg-onboard-repo.sh")
				args := []string{input.RepoPath}
				if input.Language != "" {
					args = append(args, "--language="+input.Language)
				}
				if input.DryRun {
					args = append(args, "--dry-run")
				}
				command = append([]string{script}, args...)
				cmd = exec.Command(script, args...)
			default:
				return repoBootstrapWorkflowOutput{}, fmt.Errorf("[%s] action must be create or onboard", handler.ErrInvalidParam)
			}

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			status := "ok"
			if err != nil {
				status = "fail"
			}

			return repoBootstrapWorkflowOutput{
				Action:           action,
				Status:           status,
				RepoPath:         repoPath,
				Summary:          summarizeWorkflow(status, stdout.String(), stderr.String(), action+" workflow"),
				Command:          command,
				OutputPreview:    previewText(stdout.String(), 12),
				ErrorPreview:     previewText(stderr.String(), 8),
				RecommendedTools: []string{"dotfiles_repo_sync_summary", "dotfiles_codex_audit_run"},
			}, nil
		},
	)
	repoBootstrap.Category = "workflow"
	repoBootstrap.SearchTerms = []string{"repo bootstrap", "create repo", "onboard repo", "scaffold repo"}
	repoBootstrap.IsWrite = true

	repoSync := handler.TypedHandler[repoSyncWorkflowInput, repoSyncWorkflowOutput](
		"dotfiles_repo_sync_summary",
		"Run the workflow-sync script with bounded output, defaulting to dry-run so agents can inspect drift before pushing changes.",
		func(_ context.Context, input repoSyncWorkflowInput) (repoSyncWorkflowOutput, error) {
			script := filepath.Join(dotfilesDir(), "scripts", "hg-workflow-sync.sh")
			args := []string{}
			mode := "dry-run"
			if input.Execute {
				mode = "commit"
				if input.Push {
					mode = "push"
					args = append(args, "--push")
				} else {
					args = append(args, "--commit")
				}
			} else {
				args = append(args, "--dry-run")
			}

			cmd := exec.Command(script, args...)
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stdout
			err := cmd.Run()
			status := "ok"
			if err != nil {
				status = "fail"
			}

			return repoSyncWorkflowOutput{
				Mode:             mode,
				Status:           status,
				Summary:          summarizeWorkflow(status, stdout.String(), "", "workflow sync"),
				Command:          append([]string{script}, args...),
				OutputPreview:    previewText(stdout.String(), 18),
				RecommendedTools: []string{"dotfiles_codex_audit_run", "dotfiles_fleet_health_summary"},
			}, nil
		},
	)
	repoSync.Category = "workflow"
	repoSync.SearchTerms = []string{"repo sync", "workflow sync", "ci sync", "sync workflows"}
	repoSync.IsWrite = true

	codexAudit := handler.TypedHandler[codexAuditWorkflowInput, codexAuditWorkflowOutput](
		"dotfiles_codex_audit_run",
		"Run the Codex fleet audit with a compact metric summary and explicit next-step tools.",
		func(_ context.Context, input codexAuditWorkflowInput) (codexAuditWorkflowOutput, error) {
			script := filepath.Join(dotfilesDir(), "scripts", "hg-codex-audit.sh")
			args := []string{}
			if input.WriteArtifacts {
				args = append(args, "--write-workspace-cache", "--write-wiki-docs", "--write-json")
			}
			cmd := exec.Command(script, args...)
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stdout
			err := cmd.Run()
			status := "ok"
			if err != nil {
				status = "fail"
			}

			metrics := map[string]string{}
			for _, line := range strings.Split(stdout.String(), "\n") {
				if key, value, ok := strings.Cut(line, ": "); ok {
					metrics[key] = value
				}
			}

			return codexAuditWorkflowOutput{
				Status:        status,
				Summary:       summarizeWorkflow(status, stdout.String(), "", "codex audit"),
				Metrics:       metrics,
				OutputPreview: previewLines(stdout.String(), 16),
				RecommendedTools: []string{
					"dotfiles_repo_sync_summary",
					"dotfiles_fleet_health_summary",
				},
			}, nil
		},
	)
	codexAudit.Category = "workflow"
	codexAudit.SearchTerms = []string{"codex audit", "mcp audit", "fleet audit", "migration inventory"}

	fleetHealth := handler.TypedHandler[struct{}, fleetHealthWorkflowOutput](
		"dotfiles_fleet_health_summary",
		"Run fleet-health and dependency-skew scripts together, returning short previews instead of forcing agents to reconstruct the shell workflow.",
		func(_ context.Context, _ struct{}) (fleetHealthWorkflowOutput, error) {
			fleetScript := filepath.Join(dotfilesDir(), "scripts", "hg-fleet-health.sh")
			depScript := filepath.Join(dotfilesDir(), "scripts", "hg-dep-audit.sh")

			fleetCmd := exec.Command(fleetScript)
			depCmd := exec.Command(depScript)

			var fleetOut, depOut bytes.Buffer
			fleetCmd.Stdout = &fleetOut
			fleetCmd.Stderr = &fleetOut
			depCmd.Stdout = &depOut
			depCmd.Stderr = &depOut

			fleetErr := fleetCmd.Run()
			depErr := depCmd.Run()
			status := "ok"
			if fleetErr != nil || depErr != nil {
				status = "fail"
			}

			summary := "fleet health and dependency skew summarized"
			if status != "ok" {
				summary = "fleet health or dependency skew workflow reported failures"
			}

			return fleetHealthWorkflowOutput{
				Status:                status,
				Summary:               summary,
				FleetHealthPreview:    previewLines(fleetOut.String(), 14),
				DependencySkewPreview: previewLines(depOut.String(), 14),
				RecommendedTools:      []string{"dotfiles_repo_sync_summary", "dotfiles_codex_audit_run"},
			}, nil
		},
	)
	fleetHealth.Category = "workflow"
	fleetHealth.SearchTerms = []string{"fleet health", "dependency skew", "dep audit", "repo health"}

	return []registry.ToolDefinition{repoGitHygiene, repoBootstrap, repoSync, codexAudit, fleetHealth}
}

func summarizeWorkflow(status, stdout, stderr, label string) string {
	if status != "ok" {
		if text := strings.TrimSpace(stderr); text != "" {
			return fmt.Sprintf("%s failed: %s", label, firstLine(text))
		}
		if text := strings.TrimSpace(stdout); text != "" {
			return fmt.Sprintf("%s failed: %s", label, firstLine(text))
		}
		return fmt.Sprintf("%s failed", label)
	}
	if text := strings.TrimSpace(stdout); text != "" {
		return fmt.Sprintf("%s completed: %s", label, firstLine(text))
	}
	return fmt.Sprintf("%s completed", label)
}

func previewText(text string, limit int) string {
	lines := previewLines(text, limit)
	return strings.Join(lines, "\n")
}

func previewLines(text string, limit int) []string {
	if limit <= 0 {
		limit = 10
	}
	out := make([]string, 0, limit)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func runRepoGitHygieneWorkflow(input repoGitHygieneWorkflowInput) (repoGitHygieneWorkflowOutput, error) {
	repoPath := strings.TrimSpace(input.RepoPath)
	if repoPath == "" {
		return repoGitHygieneWorkflowOutput{}, fmt.Errorf("[%s] repo_path is required", handler.ErrInvalidParam)
	}

	cleanupLocalMerged := input.CleanupLocalMerged
	cleanupWorktrees := input.CleanupWorktrees
	if !cleanupLocalMerged && !cleanupWorktrees && !input.CleanupRemoteMerged && !input.PruneAdmin && !input.PruneManagedState {
		cleanupLocalMerged = true
		cleanupWorktrees = true
	}

	script := filepath.Join(dotfilesDir(), "scripts", "hg-git-hygiene.sh")
	args := []string{"--repo", repoPath, "--json"}
	if input.Fetch {
		args = append(args, "--fetch")
	}
	if input.PruneAdmin {
		args = append(args, "--prune-admin")
	}
	if cleanupLocalMerged {
		args = append(args, "--delete-merged-local")
	}
	if input.CleanupRemoteMerged {
		args = append(args, "--delete-merged-remote")
	}
	if cleanupWorktrees {
		args = append(args, "--delete-clean-worktrees")
	}
	if input.PruneManagedState {
		args = append(args, "--prune-managed-state")
	}
	for _, prefix := range input.BranchPrefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" {
			args = append(args, "--branch-prefix", prefix)
		}
	}
	if input.Execute {
		args = append(args, "--execute")
	}

	cmd := exec.Command(script, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = strings.TrimSpace(stdout.String())
		}
		if errText == "" {
			errText = err.Error()
		}
		return repoGitHygieneWorkflowOutput{}, fmt.Errorf("repo git hygiene: %s", errText)
	}

	var raw struct {
		Repo                string                   `json:"repo"`
		DefaultBranch       string                   `json:"default_branch"`
		CurrentBranch       string                   `json:"current_branch"`
		Mode                string                   `json:"mode"`
		Summary             repoGitHygieneSummary    `json:"summary"`
		LocalBranches       []repoGitHygieneBranch   `json:"local_branches"`
		RemoteBranches      []repoGitHygieneBranch   `json:"remote_branches"`
		Worktrees           []repoGitHygieneWorktree `json:"worktrees"`
		Actions             []repoGitHygieneAction   `json:"actions"`
		ManagedStatePreview []string                 `json:"managed_state_preview"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return repoGitHygieneWorkflowOutput{}, fmt.Errorf("parse repo git hygiene output: %w", err)
	}

	return repoGitHygieneWorkflowOutput{
		Repo:                raw.Repo,
		DefaultBranch:       raw.DefaultBranch,
		CurrentBranch:       raw.CurrentBranch,
		Mode:                raw.Mode,
		Summary:             raw.Summary,
		LocalBranches:       raw.LocalBranches,
		RemoteBranches:      raw.RemoteBranches,
		Worktrees:           raw.Worktrees,
		Actions:             raw.Actions,
		ManagedStatePreview: raw.ManagedStatePreview,
		Command:             append([]string{script}, args...),
		RecommendedTools:    []string{"dotfiles_pipeline_run", "dotfiles_gh_local_sync_audit", "dotfiles_repo_sync_summary"},
	}, nil
}
