package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type dotfilesWorkspaceSceneInput struct {
	IncludeSaved *bool `json:"include_saved,omitempty" jsonschema:"description=Include saved monitor presets and layouts. Defaults to true."`
	LimitWindows int   `json:"limit_windows,omitempty" jsonschema:"description=Optional limit for returned live windows; zero means all windows."`
}

type hyprSceneWorkspaceInfo struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Monitor string `json:"monitor"`
	Windows int    `json:"windows"`
	Focused bool   `json:"focused"`
}

type hyprSceneWindowInfo struct {
	Address   string `json:"address"`
	Title     string `json:"title"`
	Class     string `json:"class"`
	Workspace int    `json:"workspace"`
	Size      [2]int `json:"size"`
	Position  [2]int `json:"position"`
	Mapped    bool   `json:"mapped"`
	Focused   bool   `json:"focused"`
	Floating  bool   `json:"floating"`
}

type dotfilesWorkspaceSceneOutput struct {
	Profile             string                       `json:"profile"`
	Status              string                       `json:"status"`
	Runtime             dotfilesDesktopRuntime       `json:"runtime"`
	MonitorCount        int                          `json:"monitor_count"`
	WorkspaceCount      int                          `json:"workspace_count"`
	WindowCount         int                          `json:"window_count"`
	FocusedMonitor      string                       `json:"focused_monitor,omitempty"`
	FocusedWorkspace    int                          `json:"focused_workspace,omitempty"`
	FocusedWindow       string                       `json:"focused_window,omitempty"`
	Monitors            []monitorInfo                `json:"monitors,omitempty"`
	Workspaces          []hyprSceneWorkspaceInfo     `json:"workspaces,omitempty"`
	Windows             []hyprSceneWindowInfo        `json:"windows,omitempty"`
	SavedMonitorPresets []hyprMonitorPresetListEntry `json:"saved_monitor_presets,omitempty"`
	SavedLayouts        []hyprLayoutListEntry        `json:"saved_layouts,omitempty"`
	Recommendations     []string                     `json:"recommendations,omitempty"`
	SuggestedTools      []string                     `json:"suggested_tools,omitempty"`
	Errors              []string                     `json:"errors,omitempty"`
	Summary             string                       `json:"summary"`
	ReportMarkdown      string                       `json:"report_markdown"`
}

func (m *DotfilesDiscoveryModule) workspaceSceneTool() registry.ToolDefinition {
	td := handler.TypedHandler[dotfilesWorkspaceSceneInput, dotfilesWorkspaceSceneOutput](
		"dotfiles_workspace_scene",
		"Capture a live workspace scene snapshot by combining Hyprland monitors, workspaces, windows, and the saved layout or monitor-preset inventory used for restoration.",
		func(ctx context.Context, input dotfilesWorkspaceSceneInput) (dotfilesWorkspaceSceneOutput, error) {
			return m.buildWorkspaceScene(ctx, input), nil
		},
	)
	td.Category = "discovery"
	td.SearchTerms = []string{
		"workspace scene",
		"layout snapshot",
		"restore scene",
		"monitor presets",
		"window scene report",
	}
	return td
}

func (m *DotfilesDiscoveryModule) buildWorkspaceScene(ctx context.Context, input dotfilesWorkspaceSceneInput) dotfilesWorkspaceSceneOutput {
	includeSaved := true
	if input.IncludeSaved != nil {
		includeSaved = *input.IncludeSaved
	}

	runtimeDir := dotfilesRuntimeDir()
	hyprSocketDir := ""
	if runtimeDir != "" {
		candidate := filepathJoin(runtimeDir, "hypr")
		if pathExists(candidate) {
			hyprSocketDir = candidate
		}
	}

	out := dotfilesWorkspaceSceneOutput{
		Profile: dotfilesProfile(),
		Status:  "ok",
		Runtime: dotfilesDesktopRuntime{
			XDGRuntimeDir:             runtimeDir,
			WaylandDisplay:            dotfilesWaylandDisplay(runtimeDir),
			HyprlandInstanceSignature: dotfilesHyprlandSignature(runtimeDir),
			HyprlandSocketDir:         hyprSocketDir,
		},
		SuggestedTools: []string{
			"dotfiles_workspace_scene",
			"hypr_get_monitors",
			"hypr_list_workspaces",
			"hypr_list_windows",
			"hypr_monitor_preset_save",
			"hypr_layout_save",
			"hypr_monitor_preset_restore",
			"hypr_layout_restore",
			"desktop_project_open",
		},
	}

	monitors, err := invokeToolJSON[monitorsResult](ctx, (&HyprlandModule{}).Tools(), "hypr_get_monitors", map[string]any{})
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("hypr_get_monitors: %v", err))
	} else {
		out.Monitors = monitors.Monitors
		out.MonitorCount = len(out.Monitors)
		for _, monitor := range out.Monitors {
			if monitor.Focused {
				out.FocusedMonitor = monitor.Name
				if out.FocusedWorkspace == 0 {
					out.FocusedWorkspace = monitor.ActiveWorkspace
				}
				break
			}
		}
	}

	workspacesText, err := invokeToolJSON[string](ctx, (&HyprlandModule{}).Tools(), "hypr_list_workspaces", map[string]any{})
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("hypr_list_workspaces: %v", err))
	} else {
		var workspaces []hyprSceneWorkspaceInfo
		if err := json.Unmarshal([]byte(workspacesText), &workspaces); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("hypr_list_workspaces: parse payload: %v", err))
		}
		out.Workspaces = workspaces
		out.WorkspaceCount = len(out.Workspaces)
		for _, workspace := range out.Workspaces {
			if workspace.Focused {
				out.FocusedWorkspace = workspace.ID
				if out.FocusedMonitor == "" {
					out.FocusedMonitor = workspace.Monitor
				}
				break
			}
		}
	}

	windowsText, err := invokeToolJSON[string](ctx, (&HyprlandModule{}).Tools(), "hypr_list_windows", map[string]any{})
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("hypr_list_windows: %v", err))
	} else {
		var windows []hyprSceneWindowInfo
		if err := json.Unmarshal([]byte(windowsText), &windows); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("hypr_list_windows: parse payload: %v", err))
		}
		out.WindowCount = len(windows)
		for _, window := range windows {
			if out.FocusedWindow == "" && window.Focused {
				out.FocusedWindow = sceneWindowLabel(window)
			}
		}
		if input.LimitWindows > 0 && len(windows) > input.LimitWindows {
			out.Windows = windows[:input.LimitWindows]
			out.Recommendations = append(out.Recommendations, fmt.Sprintf("Window list was limited to %d entries; rerun with a larger `limit_windows` value for the full scene.", input.LimitWindows))
		} else {
			out.Windows = windows
		}
	}

	if includeSaved {
		presets, err := listHyprMonitorPresets()
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("hypr_monitor_preset_list: %v", err))
		} else {
			out.SavedMonitorPresets = presets.Presets
		}

		layouts, err := listHyprLayouts()
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("hypr_layout_list: %v", err))
		} else {
			out.SavedLayouts = layouts.Layouts
		}
	}

	if len(out.Errors) > 0 {
		out.Status = "degraded"
	}

	if out.MonitorCount == 0 {
		out.Recommendations = append(out.Recommendations, "Live monitor capture is empty; confirm Hyprland IPC readiness with `dotfiles_desktop_status` and `hypr_get_monitors`.")
	}
	if out.WindowCount == 0 {
		out.Recommendations = append(out.Recommendations, "No mapped windows were captured; use `hypr_list_windows` directly before attempting a restore.")
	}
	if len(out.SavedMonitorPresets) == 0 {
		out.Recommendations = append(out.Recommendations, "No saved monitor presets are available; use `hypr_monitor_preset_save` to checkpoint the current monitor arrangement.")
	}
	if len(out.SavedLayouts) == 0 {
		out.Recommendations = append(out.Recommendations, "No saved layouts are available; use `hypr_layout_save` to checkpoint the current workspace scene before making larger changes.")
	}
	if len(out.SavedMonitorPresets) > 0 || len(out.SavedLayouts) > 0 {
		out.Recommendations = append(out.Recommendations, "Use `hypr_monitor_preset_restore`, `hypr_layout_restore`, or `desktop_project_open` when you need to rehydrate a saved scene.")
	}

	out.Recommendations = orderedUniqueStrings(out.Recommendations)
	out.Summary = fmt.Sprintf(
		"%d monitors, %d workspaces, %d live windows, %d saved monitor presets, %d saved layouts.",
		out.MonitorCount,
		out.WorkspaceCount,
		out.WindowCount,
		len(out.SavedMonitorPresets),
		len(out.SavedLayouts),
	)
	out.ReportMarkdown = renderWorkspaceSceneMarkdown(out)
	return out
}

func renderWorkspaceSceneMarkdown(out dotfilesWorkspaceSceneOutput) string {
	lines := []string{
		"# Workspace Scene",
		"",
		fmt.Sprintf("- Profile: `%s`", out.Profile),
		fmt.Sprintf("- Status: `%s`", out.Status),
		fmt.Sprintf("- Summary: %s", out.Summary),
	}
	if out.FocusedMonitor != "" {
		lines = append(lines, fmt.Sprintf("- Focused monitor: `%s`", out.FocusedMonitor))
	}
	if out.FocusedWorkspace != 0 {
		lines = append(lines, fmt.Sprintf("- Focused workspace: `%d`", out.FocusedWorkspace))
	}
	if out.FocusedWindow != "" {
		lines = append(lines, fmt.Sprintf("- Focused window: `%s`", out.FocusedWindow))
	}
	lines = append(lines,
		"",
		"## Restore Surface",
		fmt.Sprintf("- Saved monitor presets: `%d`", len(out.SavedMonitorPresets)),
		fmt.Sprintf("- Saved layouts: `%d`", len(out.SavedLayouts)),
		fmt.Sprintf("- Suggested tools: `%s`", strings.Join(out.SuggestedTools, "`, `")),
	)
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

func sceneWindowLabel(window hyprSceneWindowInfo) string {
	switch {
	case strings.TrimSpace(window.Title) != "":
		return window.Title
	case strings.TrimSpace(window.Class) != "":
		return window.Class
	default:
		return window.Address
	}
}

func filepathJoin(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	current := parts[0]
	for _, part := range parts[1:] {
		if current == "" {
			current = part
			continue
		}
		if strings.HasSuffix(current, "/") {
			current += strings.TrimPrefix(part, "/")
		} else {
			current += "/" + strings.TrimPrefix(part, "/")
		}
	}
	return current
}
