package dotfiles

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
				"dotfiles://catalog/workflows",
				"Workflow Catalog",
				mcp.WithResourceDescription("Canonical workflow catalog for the highest-value dotfiles operator entrypoints"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				data, _ := json.MarshalIndent(dotfilesWorkflowCatalog(), "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: "dotfiles://catalog/workflows", MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "catalog",
			Tags:     []string{"catalog", "workflow", "dotfiles"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://catalog/skills",
				"Skill Catalog",
				mcp.WithResourceDescription("Canonical skill-to-workflow routing map for dotfiles operator work"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				data, _ := json.MarshalIndent(dotfilesSkillCatalog(), "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: "dotfiles://catalog/skills", MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "catalog",
			Tags:     []string{"catalog", "skills", "dotfiles"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://catalog/priorities",
				"Workflow Priorities",
				mcp.WithResourceDescription("Front-door coverage summary for canonical dotfiles workflows"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				data, _ := json.MarshalIndent(buildDotfilesPrioritySummary(), "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: "dotfiles://catalog/priorities", MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "catalog",
			Tags:     []string{"catalog", "priorities", "workflow"},
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
				"dotfiles://workflows/config-repair",
				"Config Repair Workflow",
				mcp.WithResourceDescription("Compact read-first workflow for config inspection, validation, and smallest-safe reloads"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://workflows/config-repair",
						MIMEType: "text/markdown",
						Text: strings.Join([]string{
							"1. Start with `dotfiles_list_configs` or one of the config resources to confirm the exact file you are touching.",
							"2. Use `dotfiles_validate_config` on the narrowest changed TOML or JSON payload before any reload.",
							"3. Use `dotfiles_reload_service` for one service when the blast radius is small; reserve `dotfiles_cascade_reload` for layered desktop changes.",
							"4. Re-check the visible outcome after reload instead of assuming syntax validity fixed the runtime symptom.",
						}, "\n"),
					},
				}, nil
			},
			Category: "workflow",
			Tags:     []string{"config", "validate", "reload", "workflow"},
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
							"3. Use `dotfiles_eww_status`, `dotfiles_eww_inspect`, `notify_history_entries`, `hypr_list_windows`, or `hypr_get_monitors` to narrow the failing surface.",
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
				"dotfiles://workflows/desktop-control",
				"Desktop Control Workflow",
				mcp.WithResourceDescription("Compact workflow for desktop capability checks, OCR-assisted targeting, input actions, and the smallest safe reload"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://workflows/desktop-control",
						MIMEType: "text/markdown",
						Text: strings.Join([]string{
							"1. Start with `dotfiles_desktop_status` and `dotfiles_rice_check` to confirm Wayland, Hyprland, shell-stack, Eww, notification-history, semantic AT-SPI, OCR, input, and shader readiness before trying UI writes.",
							"2. Use `desktop_capabilities`, `desktop_snapshot`, `desktop_target_windows`, `desktop_find`, or `desktop_act` first when the target is a labeled UI element; use `hypr_list_windows` or `hypr_get_monitors` when the task is scene- or window-oriented.",
							"3. Use `screen_screenshot`, `desktop_screenshot_ocr`, or `desktop_find_text` to prove the visible state and coordinates only when semantic targeting is unavailable or insufficient.",
							"4. Use `session_connect` or `session_start` when the task needs an explicit session handle, then prefer `session_accessibility_tree`, `session_find_ui_element`, `session_click_element`, `session_invoke_action`, `session_type_text`, `session_dbus_call`, `session_screenshot`, `session_launch_app`, or clipboard/session helpers over ad-hoc shell reconstruction.",
							"5. Use `hypr_monitor_preset_list`, `hypr_layout_list`, `hypr_monitor_preset_restore`, `hypr_layout_restore`, or `desktop_project_open` when the task is a scene change rather than a single click.",
							"6. Use `desktop_click`, `desktop_type`, `desktop_key`, `input_type_text`, `desktop_click_text`, `hypr_click`, or other narrow write tools only after the read path proves the target.",
							"7. Prefer `dotfiles_eww_reload`, `dotfiles_reload_service`, or `hypr_reload_config` for one stale layer; reserve `dotfiles_cascade_reload` for broader desktop refreshes.",
						}, "\n"),
					},
				}, nil
			},
			Category: "workflow",
			Tags:     []string{"desktop", "automation", "ocr", "hyprland", "workflow"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://workflows/workstation-diagnose",
				"Workstation Diagnose Workflow",
				mcp.WithResourceDescription("Compact read-first workflow for machine health, services, and local workstation failures"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://workflows/workstation-diagnose",
						MIMEType: "text/markdown",
						Text: strings.Join([]string{
							"1. Start with `system_health_check` to establish whether the symptom is machine-wide.",
							"2. Use targeted reads such as `system_info`, `system_updates`, `system_disk`, or `system_memory` to isolate the failing subsystem.",
							"3. Use `systemd_failed` when background services may be the cause.",
							"4. Use `dotfiles_rice_check` only when the failure looks desktop-specific rather than machine-wide.",
						}, "\n"),
					},
				}, nil
			},
			Category: "workflow",
			Tags:     []string{"workstation", "health", "services", "workflow"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://workflows/repo-validate",
				"Repo Validate Workflow",
				mcp.WithResourceDescription("Compact workflow for repo readiness, build validation, and baseline drift checks"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://workflows/repo-validate",
						MIMEType: "text/markdown",
						Text: strings.Join([]string{
							"1. Start with `dotfiles_oss_check` or `dotfiles_oss_score` to read repo readiness before making changes.",
							"2. Use `dotfiles_pipeline_run` to validate build and test behavior in the target repo.",
							"3. Use `dotfiles_workflow_sync` in dry-run mode if CI or baseline files may be stale.",
							"4. Escalate to write flows only after the read path shows the concrete validation gap.",
						}, "\n"),
					},
				}, nil
			},
			Category: "workflow",
			Tags:     []string{"repo", "validate", "pipeline", "workflow"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://workflows/repo-hygiene",
				"Repo Hygiene Workflow",
				mcp.WithResourceDescription("Compact workflow for dry-run-first branch, worktree, and managed worktree cleanup"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://workflows/repo-hygiene",
						MIMEType: "text/markdown",
						Text: strings.Join([]string{
							"1. Start with `dotfiles_repo_git_hygiene` in dry-run mode for the target repo, usually with `cleanup_local_merged=true` and `cleanup_worktrees=true`.",
							"2. Review blocked branches or dirty worktrees before enabling `execute=true`; only enable `cleanup_remote_merged=true` once the merged remote branch list is clearly safe.",
							"3. Add `branch_prefixes` when you want to constrain cleanup to a family such as `codex/`, `claude/`, or `dependabot/`.",
							"4. Use `prune_managed_state=true` when Codex-managed worktree metadata or orphaned agent worktrees are part of the repo debt.",
							"5. Follow with `dotfiles_pipeline_run` or the repo baseline when cleanup coincides with merge or integration work.",
						}, "\n"),
					},
				}, nil
			},
			Category: "workflow",
			Tags:     []string{"repo", "git", "worktree", "cleanup", "workflow"},
		},
		{
			Resource: mcp.NewResource(
				"dotfiles://workflows/repo-onboarding",
				"Repo Onboarding Workflow",
				mcp.WithResourceDescription("Compact workflow for creating or onboarding repos into the shared studio baseline"),
				mcp.WithMIMEType("text/markdown"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://workflows/repo-onboarding",
						MIMEType: "text/markdown",
						Text: strings.Join([]string{
							"1. Use `dotfiles_create_repo` for a new repo and `dotfiles_onboard_repo` for an existing checkout.",
							"2. Keep onboarding dry-run first when the script path supports it.",
							"3. Finish with `dotfiles_workflow_sync` so the repo lands on the current baseline.",
							"4. Use `dotfiles_oss_check` if you need a readiness readout before opening follow-up work.",
						}, "\n"),
					},
				}, nil
			},
			Category: "workflow",
			Tags:     []string{"repo", "onboarding", "baseline", "workflow"},
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
		"3. Read `dotfiles://catalog/workflows` or `dotfiles://catalog/priorities` when the task matches a repeated operator workflow.",
		"4. Prefer composed workflows such as `dotfiles_fleet_audit`, `dotfiles_cascade_reload`, `dotfiles_gh_full_sync`, and `claude_fleet_recovery` over shell reconstruction.",
		"",
		"Highest-value paths:",
		"",
		"- Desktop control: `dotfiles_desktop_status` -> `dotfiles_rice_check` -> `desktop_capabilities` / `desktop_snapshot` / `desktop_target_windows` / `desktop_find` / `desktop_act` -> `session_connect` / `session_accessibility_tree` / `session_find_ui_element` / `session_click_element` when a session handle exists -> `hypr_list_windows` / `hypr_get_monitors` / `hypr_monitor_preset_list` / `hypr_layout_list` -> OCR only if needed -> narrow input, semantic action, or scene restore action.",
		"- Fleet maintenance: `dotfiles_fleet_audit` -> `dotfiles_dep_audit` / `dotfiles_gh_local_sync_audit` -> `dotfiles_workflow_sync` or `dotfiles_gh_full_sync`.",
		"- Desktop triage: `dotfiles_desktop_status` / `dotfiles_rice_check` -> `system_health_check` / targeted Hyprland or eww reads / `notify_history_entries` -> reload only the failing layer.",
		"- Config repair: `dotfiles_list_configs` / config resources -> `dotfiles_validate_config` -> smallest-safe reload.",
		"- Workstation diagnosis: `system_health_check` -> subsystem reads -> `systemd_failed` before desktop-specific escalation.",
		"- Repo validation: `dotfiles_oss_check` / `dotfiles_oss_score` -> `dotfiles_pipeline_run` -> `dotfiles_workflow_sync` in dry-run mode.",
		"- Repo onboarding: `dotfiles_create_repo` or `dotfiles_onboard_repo`, then `dotfiles_workflow_sync`.",
		"- Session recovery: `claude_recovery_report` -> `claude_session_detail` / `claude_session_health` -> `claude_fleet_recovery`.",
	}, "\n"), dotfilesMCPVersion, dotfilesProfile(), runtime.GOOS, toolCount, promptCount)
}

type dotfilesPromptModule struct{}

func (m *dotfilesPromptModule) Name() string { return "dotfiles_prompts" }
func (m *dotfilesPromptModule) Description() string {
	return "Prompt workflows for desktop control, fleet maintenance, desktop triage, config repair, workstation diagnosis, onboarding, repo validation, and recovery"
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
				"dotfiles_repair_config",
				mcp.WithPromptDescription("Inspect and repair one config surface before reloading services"),
				mcp.WithArgument("path", mcp.RequiredArgument(), mcp.ArgumentDescription("Config path or config resource URI to inspect")),
				mcp.WithArgument("service", mcp.ArgumentDescription("Optional service to reload after validation")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				path := req.Params.Arguments["path"]
				service := strings.TrimSpace(req.Params.Arguments["service"])
				reloadHint := "Use `dotfiles_reload_service` for the smallest affected layer or `dotfiles_cascade_reload` only if the change spans multiple desktop services."
				if service != "" {
					reloadHint = fmt.Sprintf("After validation, prefer `dotfiles_reload_service` with service %q before any broader reload.", service)
				}
				return mcp.NewGetPromptResult("Repair config surface", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Repair the config surface at %q. Start with `dotfiles_list_configs` or a matching config resource to confirm the target file. Use `dotfiles_validate_config` on the narrowest changed content before any reload. %s Re-check the visible outcome after reload instead of assuming syntax validity fixed the issue.",
						path,
						reloadHint,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"config", "repair", "validate", "reload"},
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
						"Triage this desktop issue: %q. Start with `dotfiles_rice_check`. Use `system_health_check` if the symptom might be machine-wide. Use `dotfiles_eww_status`, `dotfiles_eww_inspect`, `notify_history_entries`, `hypr_list_windows`, or `hypr_get_monitors` to narrow the failing layer. Only use `dotfiles_cascade_reload` or `dotfiles_reload_service` after the read path shows which layer is stale.",
						symptom,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"desktop", "triage", "hyprland", "eww"},
		},
		{
			Prompt: mcp.NewPrompt(
				"dotfiles_control_desktop",
				mcp.WithPromptDescription("Drive a desktop automation task with capability checks, OCR-assisted targeting, and the smallest write action"),
				mcp.WithArgument("objective", mcp.RequiredArgument(), mcp.ArgumentDescription("Short description of the desktop task to complete")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				objective := req.Params.Arguments["objective"]
				return mcp.NewGetPromptResult("Control desktop surface", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Complete this desktop control task: %q. Start with `dotfiles_desktop_status` and `dotfiles_rice_check` to confirm runtime readiness. Prefer `desktop_capabilities`, `desktop_snapshot`, `desktop_target_windows`, `desktop_find`, or `desktop_act` when the target is a semantic UI element, then use `hypr_list_windows`, `hypr_get_monitors`, `hypr_monitor_preset_list`, or `hypr_layout_list` to confirm the scene. Use `session_connect` or `session_start` when the task benefits from an explicit session handle, then prefer `session_accessibility_tree`, `session_find_ui_element`, `session_click_element`, `session_invoke_action`, `session_type_text`, `session_dbus_call`, `session_screenshot`, or `session_launch_app` over ad-hoc shell reconstruction. Use `screen_screenshot`, `desktop_screenshot_ocr`, or `desktop_find_text` only when semantic targeting is unavailable or insufficient. After the target is confirmed, prefer narrow actions such as `desktop_click`, `desktop_type`, `desktop_key`, `input_type_text`, `desktop_click_text`, `hypr_click`, `hypr_focus_window`, `session_connect`, `session_screenshot`, `hypr_monitor_preset_restore`, `hypr_layout_restore`, or `desktop_project_open`, and only use `dotfiles_eww_reload`, `dotfiles_reload_service`, `hypr_reload_config`, or `dotfiles_cascade_reload` when the failing layer is clear.",
						objective,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"desktop", "control", "ocr", "hyprland"},
		},
		{
			Prompt: mcp.NewPrompt(
				"dotfiles_diagnose_workstation",
				mcp.WithPromptDescription("Investigate a workstation symptom with read-first machine health tools"),
				mcp.WithArgument("symptom", mcp.ArgumentDescription("Short description of the workstation symptom")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				symptom := strings.TrimSpace(req.Params.Arguments["symptom"])
				if symptom == "" {
					symptom = "general workstation instability"
				}
				return mcp.NewGetPromptResult("Diagnose workstation issue", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Diagnose this workstation issue: %q. Start with `system_health_check` to classify whether the problem is machine-wide. Use targeted reads such as `system_info`, `system_updates`, `system_disk`, or `system_memory` to isolate the subsystem. Use `systemd_failed` if background services may be involved, and only then narrow to desktop-specific reads like `dotfiles_rice_check` if needed.",
						symptom,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"workstation", "health", "diagnose"},
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
				"dotfiles_validate_repository",
				mcp.WithPromptDescription("Validate repo readiness, build behavior, and baseline drift before making repo-wide changes"),
				mcp.WithArgument("repo_path", mcp.RequiredArgument(), mcp.ArgumentDescription("Repo path to validate")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				repoPath := req.Params.Arguments["repo_path"]
				return mcp.NewGetPromptResult("Validate repository", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Validate the repository at %q before changing it. Start with `dotfiles_oss_check` or `dotfiles_oss_score` for a read-only readiness view. Use `dotfiles_pipeline_run` to confirm build and test behavior. If baseline files may be stale, use `dotfiles_workflow_sync` in dry-run mode before any write path.",
						repoPath,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"repo", "validate", "pipeline"},
		},
		{
			Prompt: mcp.NewPrompt(
				"dotfiles_cleanup_repo_hygiene",
				mcp.WithPromptDescription("Scan or clean repo branch and worktree drift with a dry-run-first workflow"),
				mcp.WithArgument("repo_path", mcp.RequiredArgument(), mcp.ArgumentDescription("Repo path to scan or clean")),
				mcp.WithArgument("branch_prefixes", mcp.ArgumentDescription("Optional comma-separated branch prefixes such as codex/,claude/,dependabot/")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				repoPath := req.Params.Arguments["repo_path"]
				branchPrefixes := strings.TrimSpace(req.Params.Arguments["branch_prefixes"])
				prefixHint := ""
				if branchPrefixes != "" {
					prefixHint = fmt.Sprintf(" Limit cleanup candidates to the branch prefixes %q.", branchPrefixes)
				}
				return mcp.NewGetPromptResult("Clean repo git hygiene", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Clean git branch and worktree drift for %q. Start with `dotfiles_repo_git_hygiene` in dry-run mode with `cleanup_local_merged=true` and `cleanup_worktrees=true`, then inspect blocked or dirty candidates before enabling `execute=true`. Only enable `cleanup_remote_merged=true` once the merged remote branch list is clearly safe, and use `prune_managed_state=true` if Codex-managed worktree residue is part of the problem.%s",
						repoPath,
						prefixHint,
					))),
				}), nil
			},
			Category: "workflow",
			Tags:     []string{"repo", "git", "cleanup", "worktree"},
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
	resReg.RegisterModule(&archResourceModule{})
	return resReg
}

func buildDotfilesPromptRegistry() *prompts.PromptRegistry {
	promptReg := prompts.NewPromptRegistry()
	promptReg.RegisterModule(&dotfilesPromptModule{})
	promptReg.RegisterModule(&archPromptModule{})
	return promptReg
}
