package dotfiles

import "sort"

type dotfilesWorkflowCatalogEntry struct {
	Name         string   `json:"name"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	PrimarySkill string   `json:"primary_skill"`
	ResourceURI  string   `json:"resource_uri"`
	PromptName   string   `json:"prompt_name"`
	KeyTools     []string `json:"key_tools"`
}

type dotfilesSkillCatalogEntry struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	WorkflowNames []string `json:"workflow_names"`
	ResourceURIs  []string `json:"resource_uris"`
	PromptNames   []string `json:"prompt_names"`
	KeyTools      []string `json:"key_tools"`
}

type dotfilesPriorityCandidate struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	MissingSurfaces []string `json:"missing_surfaces,omitempty"`
	PriorityScore   int      `json:"priority_score"`
	Recommendation  string   `json:"recommendation"`
}

type dotfilesPrioritySummary struct {
	Source                  string                      `json:"source"`
	WorkflowCount           int                         `json:"workflow_count"`
	WorkflowResourceCount   int                         `json:"workflow_resource_count"`
	WorkflowPromptCount     int                         `json:"workflow_prompt_count"`
	WorkflowSkillCount      int                         `json:"workflow_skill_count"`
	MissingFrontDoorCount   int                         `json:"missing_front_door_count"`
	HighestPriorityWorkflow string                      `json:"highest_priority_workflow,omitempty"`
	TopCandidates           []dotfilesPriorityCandidate `json:"top_candidates,omitempty"`
	NextFocus               string                      `json:"next_focus"`
}

func dotfilesWorkflowCatalog() []dotfilesWorkflowCatalogEntry {
	return []dotfilesWorkflowCatalogEntry{
		{
			Name:         "fleet_maintenance",
			Title:        "Fleet Maintenance",
			Description:  "Audit repo drift, workflow sync state, and dependency skew before changing the workstation fleet.",
			PrimarySkill: "dotfiles_ops",
			ResourceURI:  "dotfiles://workflows/fleet-maintenance",
			PromptName:   "dotfiles_audit_fleet",
			KeyTools:     []string{"dotfiles_fleet_audit", "dotfiles_dep_audit", "dotfiles_gh_local_sync_audit", "dotfiles_workflow_sync"},
		},
		{
			Name:         "desktop_triage",
			Title:        "Desktop Triage",
			Description:  "Inspect compositor, widget, shader, and monitor state before reloading desktop services.",
			PrimarySkill: "dotfiles_ui",
			ResourceURI:  "dotfiles://workflows/desktop-triage",
			PromptName:   "dotfiles_triage_desktop",
			KeyTools:     []string{"dotfiles_rice_check", "system_health_check", "dotfiles_eww_status", "dotfiles_eww_inspect", "notify_history_entries", "hypr_list_windows", "hypr_get_monitors", "dotfiles_cascade_reload"},
		},
		{
			Name:         "desktop_control",
			Title:        "Desktop Control",
			Description:  "Validate desktop runtime readiness, inspect semantic and visible targets, then drive Hyprland, AT-SPI, session-local accessibility, OCR, input, and targeted reload actions from a single workflow.",
			PrimarySkill: "dotfiles_desktop_control",
			ResourceURI:  "dotfiles://workflows/desktop-control",
			PromptName:   "dotfiles_control_desktop",
			KeyTools:     []string{"dotfiles_desktop_status", "dotfiles_rice_check", "desktop_capabilities", "desktop_snapshot", "desktop_list_windows", "desktop_target_windows", "desktop_find", "desktop_find_all", "desktop_focus_window", "desktop_focus", "desktop_read_value", "desktop_set_text", "desktop_set_value", "desktop_act", "desktop_wait_for_element", "hypr_list_windows", "hypr_get_monitors", "hypr_monitor_preset_list", "hypr_layout_list", "session_connect", "session_accessibility_tree", "session_find_ui_element", "session_find_ui_elements", "session_focus_element", "session_read_value", "session_set_text", "session_set_value", "session_click_element", "session_invoke_action", "session_type_text", "session_dbus_call", "desktop_project_open", "desktop_screenshot_ocr", "desktop_find_text", "input_type_text", "desktop_click_text", "dotfiles_eww_reload", "dotfiles_reload_service", "dotfiles_cascade_reload"},
		},
		{
			Name:         "config_repair",
			Title:        "Config Repair",
			Description:  "Inspect a config surface, validate the narrowest file, and only then reload the smallest affected service.",
			PrimarySkill: "dotfiles_ops",
			ResourceURI:  "dotfiles://workflows/config-repair",
			PromptName:   "dotfiles_repair_config",
			KeyTools:     []string{"dotfiles_list_configs", "dotfiles_validate_config", "dotfiles_reload_service", "dotfiles_cascade_reload"},
		},
		{
			Name:         "workstation_diagnose",
			Title:        "Workstation Diagnose",
			Description:  "Collect machine health, update pressure, and service failures before escalating to repo or desktop-specific fixes.",
			PrimarySkill: "dotfiles_ops",
			ResourceURI:  "dotfiles://workflows/workstation-diagnose",
			PromptName:   "dotfiles_diagnose_workstation",
			KeyTools:     []string{"system_health_check", "system_info", "system_updates", "system_disk", "systemd_failed", "dotfiles_rice_check"},
		},
		{
			Name:         "repo_validate",
			Title:        "Repo Validate",
			Description:  "Review repo readiness, build/test behavior, and baseline config drift before applying repo-wide changes.",
			PrimarySkill: "dotfiles_ops",
			ResourceURI:  "dotfiles://workflows/repo-validate",
			PromptName:   "dotfiles_validate_repository",
			KeyTools:     []string{"dotfiles_oss_check", "dotfiles_oss_score", "dotfiles_pipeline_run", "dotfiles_workflow_sync"},
		},
		{
			Name:         "repo_hygiene",
			Title:        "Repo Hygiene",
			Description:  "Scan or safely clean merged branches, extra worktrees, and managed worktree residue with a dry-run-first workflow.",
			PrimarySkill: "dotfiles_git_hygiene",
			ResourceURI:  "dotfiles://workflows/repo-hygiene",
			PromptName:   "dotfiles_cleanup_repo_hygiene",
			KeyTools:     []string{"dotfiles_repo_git_hygiene", "dotfiles_pipeline_run", "dotfiles_gh_local_sync_audit"},
		},
		{
			Name:         "repo_onboarding",
			Title:        "Repo Onboarding",
			Description:  "Create or onboard a repo into the shared studio baseline and finish with workflow drift checks.",
			PrimarySkill: "dotfiles_ops",
			ResourceURI:  "dotfiles://workflows/repo-onboarding",
			PromptName:   "dotfiles_onboard_repository",
			KeyTools:     []string{"dotfiles_create_repo", "dotfiles_onboard_repo", "dotfiles_workflow_sync"},
		},
		{
			Name:         "session_recovery",
			Title:        "Session Recovery",
			Description:  "Investigate interrupted Claude/Codex sessions, inspect repo context, and recover only justified candidates.",
			PrimarySkill: "dotfiles_recovery",
			ResourceURI:  "dotfiles://workflows/session-recovery",
			PromptName:   "dotfiles_recover_sessions",
			KeyTools:     []string{"claude_recovery_report", "claude_session_detail", "claude_session_logs", "claude_session_health", "claude_fleet_recovery"},
		},
	}
}

func dotfilesSkillCatalog() []dotfilesSkillCatalogEntry {
	workflowCatalog := dotfilesWorkflowCatalog()
	skillDescriptions := map[string]string{
		"dotfiles_desktop_control": "Desktop control workflow for the dotfiles repo: capability checks, Hyprland targeting, OCR inspection, input automation, and targeted reloads.",
		"dotfiles_ops":             "Workstation operations, repo tooling, onboarding, and fleet maintenance for the dotfiles repo.",
		"dotfiles_ui":              "Desktop UI, rice, shader, eww, Hyprland, and screenshot workflow for the dotfiles repo.",
		"dotfiles_recovery":        "Claude and Codex session recovery, forensic analysis, and handoff workflow for the dotfiles repo.",
		"dotfiles_git_hygiene":     "Dry-run-first repo branch, worktree, and managed cleanup workflow for the dotfiles repo.",
	}

	index := map[string]*dotfilesSkillCatalogEntry{}
	for _, workflow := range workflowCatalog {
		entry := index[workflow.PrimarySkill]
		if entry == nil {
			entry = &dotfilesSkillCatalogEntry{
				Name:        workflow.PrimarySkill,
				Description: skillDescriptions[workflow.PrimarySkill],
			}
			index[workflow.PrimarySkill] = entry
		}
		entry.WorkflowNames = append(entry.WorkflowNames, workflow.Name)
		entry.ResourceURIs = append(entry.ResourceURIs, workflow.ResourceURI)
		entry.PromptNames = append(entry.PromptNames, workflow.PromptName)
		entry.KeyTools = append(entry.KeyTools, workflow.KeyTools...)
	}

	names := make([]string, 0, len(index))
	for name := range index {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]dotfilesSkillCatalogEntry, 0, len(names))
	for _, name := range names {
		entry := *index[name]
		sort.Strings(entry.WorkflowNames)
		sort.Strings(entry.ResourceURIs)
		sort.Strings(entry.PromptNames)
		entry.KeyTools = uniqueSortedStrings(entry.KeyTools)
		out = append(out, entry)
	}
	return out
}

func buildDotfilesPrioritySummary() dotfilesPrioritySummary {
	workflows := dotfilesWorkflowCatalog()
	summary := dotfilesPrioritySummary{
		Source:        "dotfiles_workflow_catalog",
		WorkflowCount: len(workflows),
		NextFocus:     "All canonical dotfiles workflows currently have read-first resources, prompts, and a mapped primary skill. The next tranche should be usage-led adoption and narrower write paths rather than more catalog growth.",
	}

	candidates := make([]dotfilesPriorityCandidate, 0)
	for _, workflow := range workflows {
		if workflow.ResourceURI != "" {
			summary.WorkflowResourceCount++
		}
		if workflow.PromptName != "" {
			summary.WorkflowPromptCount++
		}
		if workflow.PrimarySkill != "" {
			summary.WorkflowSkillCount++
		}

		missing := make([]string, 0, 3)
		score := 0
		if workflow.ResourceURI == "" {
			missing = append(missing, "resource")
			score += 30
		}
		if workflow.PromptName == "" {
			missing = append(missing, "prompt")
			score += 20
		}
		if workflow.PrimarySkill == "" {
			missing = append(missing, "skill")
			score += 15
		}
		if len(missing) == 0 {
			continue
		}
		summary.MissingFrontDoorCount++
		candidates = append(candidates, dotfilesPriorityCandidate{
			Name:            workflow.Name,
			Description:     workflow.Description,
			MissingSurfaces: missing,
			PriorityScore:   score,
			Recommendation:  "Add the missing front-door surfaces before creating new low-level helpers.",
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].PriorityScore == candidates[j].PriorityScore {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].PriorityScore > candidates[j].PriorityScore
	})
	if len(candidates) > 0 {
		summary.HighestPriorityWorkflow = candidates[0].Name
		if len(candidates) > 5 {
			summary.TopCandidates = append([]dotfilesPriorityCandidate(nil), candidates[:5]...)
		} else {
			summary.TopCandidates = append([]dotfilesPriorityCandidate(nil), candidates...)
		}
		summary.NextFocus = "Close the missing front-door surfaces on the highest-priority workflow before adding more narrow tools."
	}
	return summary
}

func uniqueSortedStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
