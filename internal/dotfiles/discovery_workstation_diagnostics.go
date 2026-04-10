package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type dotfilesWorkstationDiagnosticsInput struct {
	Symptom   string `json:"symptom,omitempty" jsonschema:"description=Short description of the workstation symptom or context to anchor the report"`
	RiceLevel string `json:"rice_level,omitempty" jsonschema:"description=Rice scan level to include,enum=quick,enum=full"`
}

type dotfilesDesktopCapabilitySummary struct {
	Ready    int `json:"ready"`
	Degraded int `json:"degraded"`
	Total    int `json:"total"`
}

type dotfilesWorkstationDiagnosticIssue struct {
	Severity         string   `json:"severity"`
	Component        string   `json:"component"`
	Summary          string   `json:"summary"`
	Details          []string `json:"details,omitempty"`
	Missing          []string `json:"missing,omitempty"`
	RecommendedTools []string `json:"recommended_tools,omitempty"`
}

type dotfilesWorkstationDiagnosticsOutput struct {
	Profile         string                               `json:"profile"`
	Status          string                               `json:"status"`
	Symptom         string                               `json:"symptom,omitempty"`
	RiceLevel       string                               `json:"rice_level"`
	Headline        string                               `json:"headline"`
	Summary         string                               `json:"summary"`
	WorkflowURI     string                               `json:"workflow_uri"`
	PromptName      string                               `json:"prompt_name"`
	Capabilities    dotfilesDesktopCapabilitySummary     `json:"capabilities"`
	IssueCount      int                                  `json:"issue_count"`
	System          SystemHealthCheckOutput              `json:"system"`
	Desktop         dotfilesDesktopStatusOutput          `json:"desktop"`
	Rice            RiceCheckOutput                      `json:"rice"`
	Issues          []dotfilesWorkstationDiagnosticIssue `json:"issues"`
	Recommendations []string                             `json:"recommendations,omitempty"`
	SuggestedTools  []string                             `json:"suggested_tools,omitempty"`
	Errors          []string                             `json:"errors,omitempty"`
	ReportMarkdown  string                               `json:"report_markdown"`
}

func (m *DotfilesDiscoveryModule) workstationDiagnosticsTool() registry.ToolDefinition {
	td := handler.TypedHandler[dotfilesWorkstationDiagnosticsInput, dotfilesWorkstationDiagnosticsOutput](
		"dotfiles_workstation_diagnostics",
		"Build a publishable workstation diagnostics snapshot by composing machine health, desktop readiness, rice state, and actionable follow-up recommendations.",
		func(ctx context.Context, input dotfilesWorkstationDiagnosticsInput) (dotfilesWorkstationDiagnosticsOutput, error) {
			return m.buildWorkstationDiagnostics(ctx, input), nil
		},
	)
	td.Category = "discovery"
	td.SearchTerms = []string{
		"workstation diagnostics",
		"desktop diagnostics",
		"machine health snapshot",
		"publishable diagnostics",
		"workstation diagnose",
	}
	return td
}

func (m *DotfilesDiscoveryModule) buildWorkstationDiagnostics(ctx context.Context, input dotfilesWorkstationDiagnosticsInput) dotfilesWorkstationDiagnosticsOutput {
	riceLevel := strings.TrimSpace(input.RiceLevel)
	if riceLevel == "" {
		riceLevel = "quick"
	}

	out := dotfilesWorkstationDiagnosticsOutput{
		Profile:     dotfilesProfile(),
		Symptom:     strings.TrimSpace(input.Symptom),
		RiceLevel:   riceLevel,
		WorkflowURI: "dotfiles://workflows/workstation-diagnose",
		PromptName:  "dotfiles_diagnose_workstation",
	}

	systemOut, err := systemHealthCheck(ctx, SystemHealthCheckInput{})
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("system_health_check: %v", err))
	} else {
		out.System = systemOut
	}

	desktopOut, err := invokeToolJSON[dotfilesDesktopStatusOutput](ctx, m.Tools(), "dotfiles_desktop_status", map[string]any{})
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("dotfiles_desktop_status: %v", err))
	} else {
		out.Desktop = desktopOut
	}

	riceOut, err := invokeToolJSON[RiceCheckOutput](ctx, (&DotfilesModule{}).Tools(), "dotfiles_rice_check", map[string]any{"level": riceLevel})
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("dotfiles_rice_check: %v", err))
	} else {
		out.Rice = riceOut
	}

	out.Capabilities = summarizeDesktopCapabilities(out.Desktop)
	out.Issues = collectWorkstationIssues(out.System, out.Desktop, out.Rice, out.Errors)
	out.IssueCount = len(out.Issues)
	out.Recommendations = collectWorkstationRecommendations(out.Issues, out.Errors)
	out.SuggestedTools = orderedUniqueStrings(append([]string{
		"dotfiles_workstation_diagnostics",
		"system_health_check",
		"dotfiles_desktop_status",
		"dotfiles_rice_check",
	}, collectIssueTools(out.Issues)...))
	out.Status = diagnosticsOverallStatus(out.System.Overall, out.Desktop.Status, out.Issues, out.Errors)
	out.Headline = diagnosticsHeadline(out.Status, out.Symptom, out.IssueCount)
	out.Summary = diagnosticsSummary(out)
	out.ReportMarkdown = renderWorkstationDiagnosticsMarkdown(out)
	return out
}

func summarizeDesktopCapabilities(out dotfilesDesktopStatusOutput) dotfilesDesktopCapabilitySummary {
	capabilities := []dotfilesDesktopCapability{
		out.Hyprland,
		out.Shell,
		out.Screenshot,
		out.OCR,
		out.Input,
		out.Accessibility,
		out.DesktopSession,
		out.Eww,
		out.Notifications,
		out.Terminal,
		out.Shader,
	}

	summary := dotfilesDesktopCapabilitySummary{Total: len(capabilities)}
	for _, capability := range capabilities {
		if capability.Ready {
			summary.Ready++
		} else {
			summary.Degraded++
		}
	}
	return summary
}

func collectWorkstationIssues(systemOut SystemHealthCheckOutput, desktopOut dotfilesDesktopStatusOutput, riceOut RiceCheckOutput, errors []string) []dotfilesWorkstationDiagnosticIssue {
	issues := make([]dotfilesWorkstationDiagnosticIssue, 0)

	for _, errText := range errors {
		issues = append(issues, dotfilesWorkstationDiagnosticIssue{
			Severity:         "warn",
			Component:        "collection",
			Summary:          errText,
			RecommendedTools: []string{"dotfiles_workstation_diagnostics"},
		})
	}

	for _, subsystem := range systemOut.Subsystems {
		severity := normalizeDiagnosticsSeverity(subsystem.Status)
		if severity == "ok" {
			continue
		}
		issues = append(issues, dotfilesWorkstationDiagnosticIssue{
			Severity:         severity,
			Component:        "system." + subsystem.Name,
			Summary:          fmt.Sprintf("%s is %s", strings.ReplaceAll(subsystem.Name, "_", " "), subsystem.Value),
			Details:          truncateStrings([]string{fmt.Sprintf("System health status: %s", strings.ToUpper(subsystem.Status))}, 3),
			RecommendedTools: issueToolsForComponent("system." + subsystem.Name),
		})
	}

	for _, item := range desktopCapabilitiesForDiagnostics(desktopOut) {
		if item.Capability.Ready {
			continue
		}
		issues = append(issues, dotfilesWorkstationDiagnosticIssue{
			Severity:         "warn",
			Component:        "desktop." + item.Name,
			Summary:          fmt.Sprintf("%s readiness is degraded", strings.ReplaceAll(item.Name, "_", " ")),
			Details:          truncateStrings(item.Capability.Details, 4),
			Missing:          orderedUniqueStrings(item.Capability.Missing),
			RecommendedTools: issueToolsForComponent("desktop." + item.Name),
		})
	}

	if strings.EqualFold(riceOut.Compositor, "unknown") {
		issues = append(issues, dotfilesWorkstationDiagnosticIssue{
			Severity:         "warn",
			Component:        "rice.compositor",
			Summary:          "rice compositor could not be identified",
			RecommendedTools: issueToolsForComponent("rice.compositor"),
		})
	}
	if strings.TrimSpace(riceOut.MonitorIncludePath) != "" && !riceOut.MonitorIncludePresent {
		issues = append(issues, dotfilesWorkstationDiagnosticIssue{
			Severity:         "warn",
			Component:        "rice.monitors",
			Summary:          "dynamic monitor include is missing",
			Details:          truncateStrings([]string{riceOut.MonitorIncludePath}, 1),
			RecommendedTools: issueToolsForComponent("rice.monitors"),
		})
	}
	if len(riceOut.PaletteViolations) > 0 {
		details := make([]string, 0, min(len(riceOut.PaletteViolations), 5))
		for _, violation := range riceOut.PaletteViolations {
			details = append(details, fmt.Sprintf("%s:%d %s", violation.File, violation.Line, violation.Color))
			if len(details) == 5 {
				break
			}
		}
		issues = append(issues, dotfilesWorkstationDiagnosticIssue{
			Severity:         "warn",
			Component:        "rice.palette",
			Summary:          fmt.Sprintf("%d palette violations detected", len(riceOut.PaletteViolations)),
			Details:          details,
			RecommendedTools: issueToolsForComponent("rice.palette"),
		})
	}
	for _, service := range riceOut.Services {
		if (service.Service == "hyprland" || service.Service == "eww") && service.Action != "running" {
			issues = append(issues, dotfilesWorkstationDiagnosticIssue{
				Severity:         "warn",
				Component:        "rice." + service.Service,
				Summary:          fmt.Sprintf("%s is %s", service.Service, service.Action),
				RecommendedTools: issueToolsForComponent("rice." + service.Service),
			})
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		left := diagnosticsSeverityRank(issues[i].Severity)
		right := diagnosticsSeverityRank(issues[j].Severity)
		if left != right {
			return left > right
		}
		return issues[i].Component < issues[j].Component
	})

	return issues
}

func collectWorkstationRecommendations(issues []dotfilesWorkstationDiagnosticIssue, errors []string) []string {
	recommendations := make([]string, 0, len(issues)+2)
	if len(errors) > 0 {
		recommendations = append(recommendations, "Re-run `dotfiles_workstation_diagnostics` after restoring the missing helper commands so the snapshot is complete.")
	}
	for _, issue := range issues {
		for _, recommendation := range recommendationsForIssue(issue) {
			if recommendation != "" {
				recommendations = append(recommendations, recommendation)
			}
		}
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "No immediate follow-up is required; use the embedded report as the current workstation health snapshot.")
	}
	return orderedUniqueStrings(recommendations)
}

func recommendationsForIssue(issue dotfilesWorkstationDiagnosticIssue) []string {
	switch {
	case strings.HasPrefix(issue.Component, "system."):
		return []string{"Use `system_health_check` plus the targeted `system_*` reads in `suggested_tools` to isolate machine-wide pressure before touching desktop services."}
	case strings.HasPrefix(issue.Component, "desktop."):
		return []string{"Use `dotfiles_desktop_status` and the issue-specific desktop tools in `suggested_tools` before attempting any UI write or reload."}
	case strings.HasPrefix(issue.Component, "rice."):
		return []string{"Use `dotfiles_rice_check`, then prefer `dotfiles_reload_service` or `dotfiles_cascade_reload` only for the layer identified in the report."}
	case issue.Component == "collection":
		return []string{"Resolve the collection error first so the workstation report is based on the full read path."}
	default:
		return nil
	}
}

func collectIssueTools(issues []dotfilesWorkstationDiagnosticIssue) []string {
	tools := make([]string, 0, len(issues)*2)
	for _, issue := range issues {
		tools = append(tools, issue.RecommendedTools...)
	}
	return orderedUniqueStrings(tools)
}

func desktopCapabilitiesForDiagnostics(out dotfilesDesktopStatusOutput) []struct {
	Name       string
	Capability dotfilesDesktopCapability
} {
	return []struct {
		Name       string
		Capability dotfilesDesktopCapability
	}{
		{Name: "hyprland", Capability: out.Hyprland},
		{Name: "shell", Capability: out.Shell},
		{Name: "screenshot", Capability: out.Screenshot},
		{Name: "ocr", Capability: out.OCR},
		{Name: "input", Capability: out.Input},
		{Name: "accessibility", Capability: out.Accessibility},
		{Name: "desktop_session", Capability: out.DesktopSession},
		{Name: "eww", Capability: out.Eww},
		{Name: "notifications", Capability: out.Notifications},
		{Name: "terminal", Capability: out.Terminal},
		{Name: "shader", Capability: out.Shader},
	}
}

func issueToolsForComponent(component string) []string {
	switch {
	case strings.HasPrefix(component, "system.cpu"), strings.HasPrefix(component, "system.gpu"):
		return []string{"system_health_check", "system_temps", "system_gpu", "system_info"}
	case strings.HasPrefix(component, "system.memory"):
		return []string{"system_health_check", "system_memory", "system_info"}
	case strings.HasPrefix(component, "system.disk"):
		return []string{"system_health_check", "system_disk", "system_info"}
	case strings.HasPrefix(component, "system.load"):
		return []string{"system_health_check", "system_uptime", "system_info"}
	case strings.HasPrefix(component, "system.updates"):
		return []string{"system_health_check", "system_updates"}
	case strings.HasPrefix(component, "desktop.hyprland"):
		return []string{"dotfiles_desktop_status", "hypr_list_windows", "hypr_get_monitors"}
	case strings.HasPrefix(component, "desktop.shell"):
		return []string{"dotfiles_desktop_status", "dotfiles_rice_check"}
	case strings.HasPrefix(component, "desktop.screenshot"), strings.HasPrefix(component, "desktop.ocr"):
		return []string{"dotfiles_desktop_status", "desktop_screenshot_ocr", "desktop_find_text"}
	case strings.HasPrefix(component, "desktop.input"):
		return []string{"dotfiles_desktop_status", "input_status"}
	case strings.HasPrefix(component, "desktop.accessibility"):
		return []string{"dotfiles_desktop_status", "desktop_capabilities", "desktop_snapshot"}
	case strings.HasPrefix(component, "desktop.desktop_session"):
		return []string{"dotfiles_desktop_status", "session_connect", "session_wayland_info"}
	case strings.HasPrefix(component, "desktop.eww"):
		return []string{"dotfiles_desktop_status", "dotfiles_eww_status", "dotfiles_eww_inspect"}
	case strings.HasPrefix(component, "desktop.notifications"):
		return []string{"dotfiles_desktop_status", "notify_history_entries", "notify_history"}
	case strings.HasPrefix(component, "desktop.terminal"):
		return []string{"dotfiles_desktop_status", "kitty_status", "kitty_list_windows"}
	case strings.HasPrefix(component, "desktop.shader"):
		return []string{"dotfiles_desktop_status", "shader_status", "shader_get_state"}
	case strings.HasPrefix(component, "rice."):
		return []string{"dotfiles_rice_check", "dotfiles_reload_service", "dotfiles_cascade_reload"}
	default:
		return []string{"dotfiles_workstation_diagnostics"}
	}
}

func diagnosticsOverallStatus(systemStatus, desktopStatus string, issues []dotfilesWorkstationDiagnosticIssue, errors []string) string {
	status := "ok"
	if strings.EqualFold(systemStatus, "CRIT") {
		status = "crit"
	} else if strings.EqualFold(systemStatus, "WARN") {
		status = "warn"
	}
	if strings.EqualFold(desktopStatus, "degraded") && diagnosticsSeverityRank(status) < diagnosticsSeverityRank("warn") {
		status = "warn"
	}
	if len(errors) > 0 && diagnosticsSeverityRank(status) < diagnosticsSeverityRank("warn") {
		status = "warn"
	}
	for _, issue := range issues {
		if diagnosticsSeverityRank(issue.Severity) > diagnosticsSeverityRank(status) {
			status = normalizeDiagnosticsSeverity(issue.Severity)
		}
	}
	return status
}

func diagnosticsHeadline(status, symptom string, issueCount int) string {
	prefix := strings.ToUpper(strings.TrimSpace(status))
	if prefix == "" {
		prefix = "OK"
	}
	if strings.TrimSpace(symptom) == "" {
		return fmt.Sprintf("%s workstation diagnostics (%d issues)", prefix, issueCount)
	}
	return fmt.Sprintf("%s workstation diagnostics for %q (%d issues)", prefix, symptom, issueCount)
}

func diagnosticsSummary(out dotfilesWorkstationDiagnosticsOutput) string {
	return fmt.Sprintf(
		"%s workstation snapshot: %d issues, %d/%d desktop capabilities ready, system %s, desktop %s, rice %s.",
		strings.ToUpper(out.Status),
		out.IssueCount,
		out.Capabilities.Ready,
		out.Capabilities.Total,
		defaultDiagnosticsStatus(out.System.Overall),
		defaultDiagnosticsStatus(out.Desktop.Status),
		out.RiceLevel,
	)
}

func renderWorkstationDiagnosticsMarkdown(out dotfilesWorkstationDiagnosticsOutput) string {
	lines := []string{
		"# Workstation Diagnostics",
		"",
		fmt.Sprintf("- Headline: %s", out.Headline),
		fmt.Sprintf("- Profile: `%s`", out.Profile),
		fmt.Sprintf("- Overall status: `%s`", out.Status),
		fmt.Sprintf("- Desktop capabilities ready: `%d/%d`", out.Capabilities.Ready, out.Capabilities.Total),
		fmt.Sprintf("- Suggested tools: `%s`", strings.Join(out.SuggestedTools, "`, `")),
	}
	if out.Symptom != "" {
		lines = append(lines, fmt.Sprintf("- Symptom: %s", out.Symptom))
	}
	lines = append(lines,
		"",
		"## Summary",
		out.Summary,
		"",
		"## Issues",
	)
	if len(out.Issues) == 0 {
		lines = append(lines, "- No issues detected.")
	} else {
		for _, issue := range out.Issues {
			line := fmt.Sprintf("- `%s` `%s`: %s", issue.Severity, issue.Component, issue.Summary)
			if len(issue.Missing) > 0 {
				line += fmt.Sprintf(" Missing: `%s`.", strings.Join(issue.Missing, "`, `"))
			}
			if len(issue.RecommendedTools) > 0 {
				line += fmt.Sprintf(" Tools: `%s`.", strings.Join(issue.RecommendedTools, "`, `"))
			}
			lines = append(lines, line)
		}
	}
	if len(out.Recommendations) > 0 {
		lines = append(lines, "", "## Recommendations")
		for _, recommendation := range out.Recommendations {
			lines = append(lines, "- "+recommendation)
		}
	}
	if len(out.Errors) > 0 {
		lines = append(lines, "", "## Collection Errors")
		for _, errText := range out.Errors {
			lines = append(lines, "- "+errText)
		}
	}
	return strings.Join(lines, "\n")
}

func invokeToolJSON[T any](ctx context.Context, tools []registry.ToolDefinition, name string, args map[string]any) (T, error) {
	var out T
	for _, td := range tools {
		if td.Tool.Name != name {
			continue
		}
		req := registry.CallToolRequest{}
		req.Params.Arguments = args
		result, err := td.Handler(ctx, req)
		if err != nil {
			return out, err
		}
		text, err := toolResultText(result)
		if err != nil {
			return out, err
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			return out, fmt.Errorf("decode %s: %w", name, err)
		}
		return out, nil
	}
	return out, fmt.Errorf("tool not found: %s", name)
}

func toolResultText(result *registry.CallToolResult) (string, error) {
	if result == nil || len(result.Content) == 0 {
		return "", fmt.Errorf("result has no content")
	}
	texts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if text, ok := content.(registry.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	if len(texts) == 0 {
		return "", fmt.Errorf("result did not contain text content")
	}
	return strings.Join(texts, "\n"), nil
}

func normalizeDiagnosticsSeverity(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "crit", "critical":
		return "crit"
	case "warn", "warning", "degraded":
		return "warn"
	default:
		return "ok"
	}
}

func diagnosticsSeverityRank(status string) int {
	switch normalizeDiagnosticsSeverity(status) {
	case "crit":
		return 2
	case "warn":
		return 1
	default:
		return 0
	}
}

func orderedUniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func truncateStrings(items []string, max int) []string {
	if max <= 0 || len(items) <= max {
		return orderedUniqueStrings(items)
	}
	return orderedUniqueStrings(items[:max])
}

func defaultDiagnosticsStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "unknown"
	}
	return status
}
