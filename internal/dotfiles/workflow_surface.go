package main

import (
	"bytes"
	"context"
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
	Action            string   `json:"action"`
	Status            string   `json:"status"`
	RepoPath          string   `json:"repo_path,omitempty"`
	Summary           string   `json:"summary"`
	Command           []string `json:"command"`
	OutputPreview     string   `json:"output_preview,omitempty"`
	ErrorPreview      string   `json:"error_preview,omitempty"`
	RecommendedTools  []string `json:"recommended_tools"`
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

type FleetWorkflowSurfaceModule struct{}

func (m *FleetWorkflowSurfaceModule) Name() string        { return "fleet_workflows" }
func (m *FleetWorkflowSurfaceModule) Description() string { return "Composed repo bootstrap, sync, audit, and fleet-health workflows" }

func (m *FleetWorkflowSurfaceModule) Tools() []registry.ToolDefinition {
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

	return []registry.ToolDefinition{repoBootstrap, repoSync, codexAudit, fleetHealth}
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
