package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hairglasses-studio/mcpkit/prompts"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/mcpkit/resources"
	"github.com/mark3labs/mcp-go/mcp"
)

const dotfilesMCPVersion = "2.2.0"

type dotfilesResourceModule struct {
	reg       *registry.ToolRegistry
	promptReg *prompts.PromptRegistry
}

func (m *dotfilesResourceModule) Name() string { return "dotfiles_context" }
func (m *dotfilesResourceModule) Description() string {
	return "Reusable workflow guides and server overview for dotfiles-mcp"
}

func (m *dotfilesResourceModule) Resources() []resources.ResourceDefinition {
	return []resources.ResourceDefinition{
		{
			Resource: mcp.NewResource(
				"dotfiles://server/overview",
				"dotfiles-mcp Overview",
				mcp.WithResourceDescription("Server card covering profile shape, discovery-first usage, and highest-value workflows"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://server/overview",
						MIMEType: "text/markdown",
						Text:     m.overviewMarkdown(),
					},
				}, nil
			},
			Category: "overview",
			Tags:     []string{"dotfiles", "desktop", "fleet", "overview"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://workflows/fleet-maintenance",
				"Fleet Maintenance Workflow",
				mcp.WithResourceDescription("Compact workflow for org sync, dependency drift, and fleet auditing"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://workflows/fleet-maintenance",
						MIMEType: "text/markdown",
						Text: strings.Join([]string{
							"1. Start with `dotfiles_fleet_audit` for the broad per-repo matrix.",
							"2. Use `dotfiles_dep_audit` when the issue is dependency drift or Go version skew.",
							"3. Use `dotfiles_gh_local_sync_audit` to measure org-vs-local clone drift before any clone or delete action.",
							"4. Use `dotfiles_gh_full_sync` or `dotfiles_workflow_sync` only after the read path identifies the exact sync task.",
							"5. Keep write flows dry-run first when the tool supports `execute=true`.",
						}, "\n"),
					},
				}, nil
			},
			Category: "workflow",
			Tags:     []string{"fleet", "sync", "audit", "workflow"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://workflows/desktop-triage",
				"Desktop Triage Workflow",
				mcp.WithResourceDescription("Compact read-first workflow for compositor, bar, shader, and service issues"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://workflows/desktop-triage",
						MIMEType: "text/markdown",
						Text: strings.Join([]string{
							"1. Start with `dotfiles_rice_check` for compositor, shader, wallpaper, and service state.",
							"2. Use `system_health_check` if the symptom may be machine-wide rather than desktop-specific.",
							"3. Use `dotfiles_eww_status`, `hypr_list_windows`, or `hypr_get_monitors` to narrow the failing surface.",
							"4. Only run `dotfiles_cascade_reload` or `dotfiles_reload_service` after the read path explains which layer is stale.",
						}, "\n"),
					},
				}, nil
			},
			Category: "workflow",
			Tags:     []string{"desktop", "hyprland", "eww", "workflow"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://workflows/session-recovery",
				"Claude Session Recovery Workflow",
				mcp.WithResourceDescription("Compact workflow for dead-session triage and recovery planning"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://workflows/session-recovery",
						MIMEType: "text/markdown",
						Text: strings.Join([]string{
							"1. Start with `claude_recovery_report` for the cross-session recovery queue.",
							"2. Use `claude_session_detail` or `claude_session_logs` to inspect one session deeply.",
							"3. Use `claude_session_health` to rank how recoverable a candidate session is.",
							"4. Use `claude_fleet_recovery` only after confirming the exact sessions worth resuming.",
						}, "\n"),
					},
				}, nil
			},
			Category: "workflow",
			Tags:     []string{"claude", "session", "recovery", "workflow"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://config/ghostty",
				"Ghostty Config",
				mcp.WithResourceDescription("Current Ghostty terminal configuration"),
				mcp.WithMIMEType("text/plain"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				path := filepath.Join(dotfilesDir(), "ghostty", "config")
				data, err := os.ReadFile(path)
				if err != nil {
					return nil, fmt.Errorf("read ghostty config: %w", err)
				}
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: "dotfiles://config/ghostty", MIMEType: "text/plain", Text: string(data)},
				}, nil
			},
			Category: "config",
			Tags:     []string{"ghostty", "terminal", "config"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://config/hyprland",
				"Hyprland Config",
				mcp.WithResourceDescription("Current Hyprland window manager configuration"),
				mcp.WithMIMEType("text/plain"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				path := filepath.Join(dotfilesDir(), "hyprland", "hyprland.conf")
				data, err := os.ReadFile(path)
				if err != nil {
					return nil, fmt.Errorf("read hyprland config: %w", err)
				}
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: "dotfiles://config/hyprland", MIMEType: "text/plain", Text: string(data)},
				}, nil
			},
			Category: "config",
			Tags:     []string{"hyprland", "wm", "config"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://palette/snazzy",
				"Snazzy Color Palette",
				mcp.WithResourceDescription("Snazzy terminal color palette as JSON"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				palette := map[string]string{
					"bg":      "#000000",
					"fg":      "#f1f1f0",
					"cyan":    "#57c7ff",
					"magenta": "#ff6ac1",
					"green":   "#5af78e",
					"yellow":  "#f3f99d",
					"red":     "#ff5c57",
					"blue":    "#57c7ff",
				}
				data, _ := json.MarshalIndent(palette, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: "dotfiles://palette/snazzy", MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "config",
			Tags:     []string{"palette", "colors", "snazzy"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://shader/current",
				"Active Shader",
				mcp.WithResourceDescription("Currently active Ghostty shader name and path"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				configPath := filepath.Join(dotfilesDir(), "ghostty", "config")
				data, err := os.ReadFile(configPath)
				if err != nil {
					return nil, fmt.Errorf("read ghostty config: %w", err)
				}
				shaderPath := ""
				for _, line := range strings.Split(string(data), "\n") {
					if strings.HasPrefix(strings.TrimSpace(line), "custom-shader") {
						parts := strings.SplitN(line, "=", 2)
						if len(parts) == 2 {
							shaderPath = strings.TrimSpace(parts[1])
						}
					}
				}
				result := map[string]string{
					"path": shaderPath,
					"name": filepath.Base(shaderPath),
				}
				out, _ := json.MarshalIndent(result, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: "dotfiles://shader/current", MIMEType: "application/json", Text: string(out)},
				}, nil
			},
			Category: "config",
			Tags:     []string{"shader", "ghostty", "glsl"},
		},
	}
}

func (m *dotfilesResourceModule) Templates() []resources.TemplateDefinition { return nil }

func (m *dotfilesResourceModule) overviewMarkdown() string {
	toolCount := 0
	promptCount := 0
	if m.reg != nil {
		toolCount = m.reg.ToolCount()
	}
	if m.promptReg != nil {
		promptCount = m.promptReg.PromptCount()
	}

	return fmt.Sprintf(strings.Join([]string{
		"# dotfiles-mcp",
		"",
		"- Version: `%s`",
		"- Profile: `%s`",
		"- Runtime OS: `%s`",
		"- Registered tools: `%d`",
		"- Registered prompt workflows: `%d`",
		"",
		"Start with discovery before reaching for the large desktop and fleet surface:",
		"",
		"1. Use `dotfiles_tool_search` to find the smallest relevant tool set.",
		"2. Use `dotfiles_tool_schema` for exact parameters and write-safety details.",
		"3. Prefer composed workflows such as `dotfiles_fleet_audit`, `dotfiles_cascade_reload`, `dotfiles_gh_full_sync`, and `claude_fleet_recovery` over shell reconstruction.",
		"",
		"Highest-value paths:",
		"",
		"- Fleet maintenance: `dotfiles_fleet_audit` -> `dotfiles_dep_audit` / `dotfiles_gh_local_sync_audit` -> `dotfiles_workflow_sync` or `dotfiles_gh_full_sync`.",
		"- Desktop triage: `dotfiles_rice_check` -> `system_health_check` / targeted Hyprland or eww reads -> reload only the failing layer.",
		"- Repo onboarding: `dotfiles_create_repo` or `dotfiles_onboard_repo`, then `dotfiles_workflow_sync`.",
		"- Session recovery: `claude_recovery_report` -> `claude_session_detail` / `claude_session_health` -> `claude_fleet_recovery`.",
	}, "\n"), dotfilesMCPVersion, dotfilesProfile(), runtime.GOOS, toolCount, promptCount)
}

type dotfilesPromptModule struct{}

func (m *dotfilesPromptModule) Name() string { return "dotfiles_prompts" }
func (m *dotfilesPromptModule) Description() string {
	return "Prompt workflows for fleet maintenance, desktop triage, onboarding, and recovery"
}

func (m *dotfilesPromptModule) Prompts() []prompts.PromptDefinition {
	return []prompts.PromptDefinition{
		{
			Prompt: mcp.NewPrompt(
				"dotfiles_audit_fleet",
				mcp.WithPromptDescription("Run a bounded fleet audit and sync analysis before changing repos"),
				mcp.WithArgument("local_dir", mcp.ArgumentDescription("Fleet root directory. Defaults to ~/hairglasses-studio")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				localDir := req.Params.Arguments["local_dir"]
				if strings.TrimSpace(localDir) == "" {
					localDir = "~/hairglasses-studio"
				}
				return mcp.NewGetPromptResult("Audit the dotfiles fleet", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Audit the workstation fleet rooted at %q. Start with `dotfiles_fleet_audit` for the broad repo matrix. If sync drift is part of the issue, follow with `dotfiles_gh_local_sync_audit`; if dependency drift matters, use `dotfiles_dep_audit`. Only use `dotfiles_gh_full_sync` or other write flows after the read path identifies the exact change to make.",
						localDir,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"fleet", "audit", "sync"},
		},
		{
			Prompt: mcp.NewPrompt(
				"dotfiles_triage_desktop",
				mcp.WithPromptDescription("Investigate a desktop symptom with read-first tools before reloads"),
				mcp.WithArgument("symptom", mcp.RequiredArgument(), mcp.ArgumentDescription("Short description of the desktop symptom")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				symptom := req.Params.Arguments["symptom"]
				return mcp.NewGetPromptResult("Triage desktop issue", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Triage this desktop issue: %q. Start with `dotfiles_rice_check`. Use `system_health_check` if the symptom might be machine-wide. Use `dotfiles_eww_status`, `hypr_list_windows`, or `hypr_get_monitors` to narrow the failing layer. Only use `dotfiles_cascade_reload` or `dotfiles_reload_service` after the read path shows which layer is stale.",
						symptom,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"desktop", "triage", "hyprland", "eww"},
		},
		{
			Prompt: mcp.NewPrompt(
				"dotfiles_onboard_repository",
				mcp.WithPromptDescription("Onboard an existing repo or create a new one with the standard studio baseline"),
				mcp.WithArgument("repo_path", mcp.RequiredArgument(), mcp.ArgumentDescription("Existing repo path or desired repo path")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				repoPath := req.Params.Arguments["repo_path"]
				return mcp.NewGetPromptResult("Onboard repository", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Onboard the repository at %q. If the repo already exists, start with `dotfiles_onboard_repo`; if it needs to be created from scratch, use `dotfiles_create_repo`. Finish by checking workflow drift with `dotfiles_workflow_sync`, and prefer dry-run or read paths first when available.",
						repoPath,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"repo", "onboarding", "workflow"},
		},
		{
			Prompt: mcp.NewPrompt(
				"dotfiles_recover_sessions",
				mcp.WithPromptDescription("Investigate dead or degraded Claude Code sessions before attempting recovery"),
				mcp.WithArgument("window", mcp.ArgumentDescription("Time window such as 24h or 7d")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				window := strings.TrimSpace(req.Params.Arguments["window"])
				if window == "" {
					window = "7d"
				}
				return mcp.NewGetPromptResult("Recover Claude sessions", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Investigate Claude Code session health for the last %s. Start with `claude_recovery_report` for the fleet view, then inspect the highest-priority candidates with `claude_session_detail` or `claude_session_logs`, rank them with `claude_session_health`, and only then use `claude_fleet_recovery` if resuming sessions is justified.",
						window,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"claude", "sessions", "recovery"},
		},
	}
}

func buildDotfilesResourceRegistry(reg *registry.ToolRegistry, promptReg *prompts.PromptRegistry) *resources.ResourceRegistry {
	resReg := resources.NewResourceRegistry()
	resReg.RegisterModule(&dotfilesResourceModule{reg: reg, promptReg: promptReg})
	return resReg
}

func buildDotfilesPromptRegistry() *prompts.PromptRegistry {
	promptReg := prompts.NewPromptRegistry()
	promptReg.RegisterModule(&dotfilesPromptModule{})
	return promptReg
}
