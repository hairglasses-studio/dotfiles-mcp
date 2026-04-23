package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type hyprMonitorPresetSaveInput struct {
	Name string `json:"name" jsonschema:"required,description=Preset name to save under the local desktop-control state directory"`
}

type hyprMonitorPresetRestoreInput struct {
	Name   string `json:"name" jsonschema:"required,description=Saved monitor preset name"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"description=Validate and show the generated hyprctl commands without applying them"`
}

type hyprMonitorPresetListInput struct{}

type hyprLayoutSaveInput struct {
	Name           string            `json:"name" jsonschema:"required,description=Layout name to save under the local desktop-control state directory"`
	WorkspaceIDs   []int             `json:"workspace_ids,omitempty" jsonschema:"description=Optional workspace filter; omit to capture all currently mapped windows"`
	LaunchCommands map[string]string `json:"launch_commands,omitempty" jsonschema:"description=Optional launch commands keyed by class, initial_class, or title to allow missing-window restoration later"`
}

type hyprLayoutRestoreInput struct {
	Name          string `json:"name" jsonschema:"required,description=Saved layout name"`
	DryRun        bool   `json:"dry_run,omitempty" jsonschema:"description=Show the actions without applying them"`
	LaunchMissing *bool  `json:"launch_missing,omitempty" jsonschema:"description=Whether to launch missing apps when the saved layout contains launch commands. Defaults to true."`
}

type hyprLayoutListInput struct{}

type desktopProjectOpenInput struct {
	RepoPath      string `json:"repo_path" jsonschema:"required,description=Absolute path to the project repo or workspace directory"`
	MonitorPreset string `json:"monitor_preset,omitempty" jsonschema:"description=Optional saved monitor preset to restore before opening the project scene"`
	Layout        string `json:"layout,omitempty" jsonschema:"description=Optional saved window/workspace layout to restore"`
	TmuxSession   string `json:"tmux_session,omitempty" jsonschema:"description=Optional tmux session name; defaults to the repo directory basename"`
	TmuxCommand   string `json:"tmux_command,omitempty" jsonschema:"description=Optional initial tmux command to run when creating a new session"`
	LaunchMissing *bool  `json:"launch_missing,omitempty" jsonschema:"description=Whether to launch missing apps referenced by the saved layout. Defaults to true."`
	DryRun        bool   `json:"dry_run,omitempty" jsonschema:"description=Preview the scene actions without applying them"`
}

type hyprMonitorPreset struct {
	Name     string                `json:"name"`
	SavedAt  string                `json:"saved_at"`
	Monitors []hyprMonitorSnapshot `json:"monitors"`
}

type hyprMonitorSnapshot struct {
	Name        string  `json:"name"`
	Enabled     bool    `json:"enabled"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	RefreshRate float64 `json:"refresh_rate_hz,omitempty"`
	X           int     `json:"x,omitempty"`
	Y           int     `json:"y,omitempty"`
	Scale       float64 `json:"scale,omitempty"`
	Transform   int     `json:"transform,omitempty"`
	Description string  `json:"description,omitempty"`
}

type hyprMonitorPresetSaveOutput struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	SavedAt      string `json:"saved_at"`
	MonitorCount int    `json:"monitor_count"`
}

type hyprMonitorPresetListEntry struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	SavedAt      string `json:"saved_at"`
	MonitorCount int    `json:"monitor_count"`
}

type hyprMonitorPresetListOutput struct {
	Presets []hyprMonitorPresetListEntry `json:"presets"`
}

type hyprMonitorPresetRestoreOutput struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Applied   bool     `json:"applied"`
	Commands  []string `json:"commands"`
	Restored  []string `json:"restored,omitempty"`
	Disabled  []string `json:"disabled,omitempty"`
	Errors    []string `json:"errors,omitempty"`
	Validated bool     `json:"validated"`
}

type hyprLayoutSnapshot struct {
	Name       string               `json:"name"`
	SavedAt    string               `json:"saved_at"`
	Windows    []hyprSavedWindow    `json:"windows"`
	Workspaces []hyprSavedWorkspace `json:"workspaces,omitempty"`
}

type hyprSavedWorkspace struct {
	ID      int    `json:"id"`
	Name    string `json:"name,omitempty"`
	Monitor string `json:"monitor,omitempty"`
}

type hyprSavedWindow struct {
	Class          string `json:"class,omitempty"`
	Title          string `json:"title,omitempty"`
	InitialClass   string `json:"initial_class,omitempty"`
	InitialTitle   string `json:"initial_title,omitempty"`
	Workspace      int    `json:"workspace"`
	WorkspaceName  string `json:"workspace_name,omitempty"`
	Monitor        int    `json:"monitor,omitempty"`
	MonitorName    string `json:"monitor_name,omitempty"`
	Floating       bool   `json:"floating"`
	Mapped         bool   `json:"mapped"`
	Pinned         bool   `json:"pinned"`
	Fullscreen     bool   `json:"fullscreen"`
	FullscreenMode int    `json:"fullscreen_mode,omitempty"`
	Position       [2]int `json:"position,omitempty"`
	Size           [2]int `json:"size,omitempty"`
	LaunchCommand  string `json:"launch_command,omitempty"`
}

type hyprClientState struct {
	Address        string
	Class          string
	Title          string
	InitialClass   string
	InitialTitle   string
	Workspace      int
	WorkspaceName  string
	Monitor        int
	MonitorName    string
	Floating       bool
	Mapped         bool
	Pinned         bool
	Fullscreen     bool
	FullscreenMode int
	Position       [2]int
	Size           [2]int
}

type hyprLayoutSaveOutput struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SavedAt     string `json:"saved_at"`
	WindowCount int    `json:"window_count"`
}

type hyprLayoutListEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SavedAt     string `json:"saved_at"`
	WindowCount int    `json:"window_count"`
}

type hyprLayoutListOutput struct {
	Layouts []hyprLayoutListEntry `json:"layouts"`
}

type hyprLayoutRestoreWindowStatus struct {
	Class        string   `json:"class,omitempty"`
	Title        string   `json:"title,omitempty"`
	Status       string   `json:"status"`
	MatchAddress string   `json:"match_address,omitempty"`
	Actions      []string `json:"actions,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

type hyprLayoutRestoreOutput struct {
	Name       string                          `json:"name"`
	Path       string                          `json:"path"`
	Applied    bool                            `json:"applied"`
	Launched   []string                        `json:"launched,omitempty"`
	Unresolved []string                        `json:"unresolved,omitempty"`
	Windows    []hyprLayoutRestoreWindowStatus `json:"windows"`
	Errors     []string                        `json:"errors,omitempty"`
}

type desktopProjectOpenOutput struct {
	RepoPath       string                          `json:"repo_path"`
	MonitorPreset  string                          `json:"monitor_preset,omitempty"`
	Layout         string                          `json:"layout,omitempty"`
	TmuxSession    string                          `json:"tmux_session,omitempty"`
	Actions        []string                        `json:"actions,omitempty"`
	Warnings       []string                        `json:"warnings,omitempty"`
	MonitorRestore *hyprMonitorPresetRestoreOutput `json:"monitor_restore,omitempty"`
	LayoutRestore  *hyprLayoutRestoreOutput        `json:"layout_restore,omitempty"`
}

func hyprPersistenceToolDefinitions() []registry.ToolDefinition {
	monitorSave := handler.TypedHandler[hyprMonitorPresetSaveInput, hyprMonitorPresetSaveOutput](
		"hypr_monitor_preset_save",
		"Save the current Hyprland monitor arrangement into a local preset, including resolution, refresh, position, scale, transform, and enabled state.",
		func(_ context.Context, input hyprMonitorPresetSaveInput) (hyprMonitorPresetSaveOutput, error) {
			if strings.TrimSpace(input.Name) == "" {
				return hyprMonitorPresetSaveOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
			}
			preset, path, err := saveCurrentHyprMonitorPreset(input.Name)
			if err != nil {
				return hyprMonitorPresetSaveOutput{}, err
			}
			return hyprMonitorPresetSaveOutput{
				Name:         preset.Name,
				Path:         path,
				SavedAt:      preset.SavedAt,
				MonitorCount: len(preset.Monitors),
			}, nil
		},
	)
	monitorSave.IsWrite = true

	monitorRestore := handler.TypedHandler[hyprMonitorPresetRestoreInput, hyprMonitorPresetRestoreOutput](
		"hypr_monitor_preset_restore",
		"Restore a saved Hyprland monitor preset. Validates the generated hyprctl commands first, then applies them unless dry_run=true.",
		func(_ context.Context, input hyprMonitorPresetRestoreInput) (hyprMonitorPresetRestoreOutput, error) {
			if strings.TrimSpace(input.Name) == "" {
				return hyprMonitorPresetRestoreOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
			}
			return restoreHyprMonitorPreset(input.Name, input.DryRun)
		},
	)
	monitorRestore.IsWrite = true

	monitorList := handler.TypedHandler[hyprMonitorPresetListInput, hyprMonitorPresetListOutput](
		"hypr_monitor_preset_list",
		"List saved Hyprland monitor presets from local state.",
		func(_ context.Context, _ hyprMonitorPresetListInput) (hyprMonitorPresetListOutput, error) {
			return listHyprMonitorPresets()
		},
	)

	layoutSave := handler.TypedHandler[hyprLayoutSaveInput, hyprLayoutSaveOutput](
		"hypr_layout_save",
		"Save the current Hyprland window/workspace scene with class, title, workspace, monitor, floating state, size, position, fullscreen, pinning, and optional launch commands.",
		func(_ context.Context, input hyprLayoutSaveInput) (hyprLayoutSaveOutput, error) {
			if strings.TrimSpace(input.Name) == "" {
				return hyprLayoutSaveOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
			}
			snapshot, path, err := saveCurrentHyprLayout(input)
			if err != nil {
				return hyprLayoutSaveOutput{}, err
			}
			return hyprLayoutSaveOutput{
				Name:        snapshot.Name,
				Path:        path,
				SavedAt:     snapshot.SavedAt,
				WindowCount: len(snapshot.Windows),
			}, nil
		},
	)
	layoutSave.IsWrite = true

	layoutRestore := handler.TypedHandler[hyprLayoutRestoreInput, hyprLayoutRestoreOutput](
		"hypr_layout_restore",
		"Best-effort restore of a saved Hyprland layout. Reuses matching windows first and only launches missing apps when the saved scene has explicit launch commands.",
		func(_ context.Context, input hyprLayoutRestoreInput) (hyprLayoutRestoreOutput, error) {
			if strings.TrimSpace(input.Name) == "" {
				return hyprLayoutRestoreOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
			}
			return restoreHyprLayout(input.Name, input.DryRun, defaultBool(input.LaunchMissing, true))
		},
	)
	layoutRestore.IsWrite = true

	layoutList := handler.TypedHandler[hyprLayoutListInput, hyprLayoutListOutput](
		"hypr_layout_list",
		"List saved Hyprland layout snapshots from local state.",
		func(_ context.Context, _ hyprLayoutListInput) (hyprLayoutListOutput, error) {
			return listHyprLayouts()
		},
	)

	projectOpen := handler.TypedHandler[desktopProjectOpenInput, desktopProjectOpenOutput](
		"desktop_project_open",
		"Open a project scene by combining an optional monitor preset, optional Hyprland layout, and an optional tmux bootstrap in the target repo path.",
		func(_ context.Context, input desktopProjectOpenInput) (desktopProjectOpenOutput, error) {
			return openDesktopProject(input)
		},
	)
	projectOpen.IsWrite = true
	projectOpen.Category = "desktop"
	projectOpen.SearchTerms = []string{"project scene", "workspace scene", "project layout", "desktop project"}

	return []registry.ToolDefinition{
		monitorSave,
		monitorRestore,
		monitorList,
		layoutSave,
		layoutRestore,
		layoutList,
		projectOpen,
	}
}

func defaultBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func safeStateName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "unnamed"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unnamed"
	}
	return out
}

func formatDecimal(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

func hyprMonitorPresetPath(name string) (string, error) {
	dir, err := ensureDotfilesManagedStateDir("hypr", "monitor-presets")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, safeStateName(name)+".json"), nil
}

func hyprLayoutPath(name string) (string, error) {
	dir, err := ensureDotfilesManagedStateDir("hypr", "layouts")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, safeStateName(name)+".json"), nil
}

func readHyprMonitors() ([]hyprMonitorSnapshot, error) {
	out, err := runHyprctl("monitors", "all", "-j")
	if err != nil {
		out, err = runHyprctl("monitors", "-j")
		if err != nil {
			return nil, err
		}
	}

	var raw []map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse monitors: %w", err)
	}

	monitors := make([]hyprMonitorSnapshot, 0, len(raw))
	for _, item := range raw {
		mon := hyprMonitorSnapshot{
			Enabled: true,
			Scale:   1,
		}
		if v, ok := item["name"].(string); ok {
			mon.Name = v
		}
		if v, ok := item["width"].(float64); ok {
			mon.Width = int(v)
		}
		if v, ok := item["height"].(float64); ok {
			mon.Height = int(v)
		}
		if v, ok := item["refreshRate"].(float64); ok {
			mon.RefreshRate = v
		}
		if v, ok := item["x"].(float64); ok {
			mon.X = int(v)
		}
		if v, ok := item["y"].(float64); ok {
			mon.Y = int(v)
		}
		if v, ok := item["scale"].(float64); ok && v > 0 {
			mon.Scale = v
		}
		if v, ok := item["transform"].(float64); ok {
			mon.Transform = int(v)
		}
		if v, ok := item["description"].(string); ok {
			mon.Description = v
		}
		if v, ok := item["disabled"].(bool); ok {
			mon.Enabled = !v
		}
		monitors = append(monitors, mon)
	}

	sort.Slice(monitors, func(i, j int) bool {
		if monitors[i].Enabled == monitors[j].Enabled {
			return monitors[i].Name < monitors[j].Name
		}
		return monitors[i].Enabled && !monitors[j].Enabled
	})
	return monitors, nil
}

func validateMonitorPreset(preset hyprMonitorPreset) ([]string, []string) {
	if len(preset.Monitors) == 0 {
		return nil, []string{"preset does not contain any monitors"}
	}
	var commands []string
	var errors []string
	seen := map[string]struct{}{}
	for _, mon := range preset.Monitors {
		if mon.Name == "" {
			errors = append(errors, "monitor entry is missing name")
			continue
		}
		if _, ok := seen[mon.Name]; ok {
			errors = append(errors, fmt.Sprintf("duplicate monitor entry %q", mon.Name))
			continue
		}
		seen[mon.Name] = struct{}{}

		if !mon.Enabled {
			commands = append(commands, fmt.Sprintf("%s,disable", mon.Name))
			continue
		}
		if mon.Width <= 0 || mon.Height <= 0 {
			errors = append(errors, fmt.Sprintf("monitor %s is missing width/height", mon.Name))
			continue
		}
		if mon.Scale <= 0 {
			errors = append(errors, fmt.Sprintf("monitor %s has invalid scale %.2f", mon.Name, mon.Scale))
			continue
		}
		resolution := fmt.Sprintf("%dx%d@%s", mon.Width, mon.Height, formatDecimal(mon.RefreshRate))
		position := fmt.Sprintf("%dx%d", mon.X, mon.Y)
		command := fmt.Sprintf("%s,%s,%s,%s,%d", mon.Name, resolution, position, formatDecimal(mon.Scale), mon.Transform)
		commands = append(commands, command)
	}
	return commands, errors
}

func saveCurrentHyprMonitorPreset(name string) (hyprMonitorPreset, string, error) {
	monitors, err := readHyprMonitors()
	if err != nil {
		return hyprMonitorPreset{}, "", err
	}
	preset := hyprMonitorPreset{
		Name:     strings.TrimSpace(name),
		SavedAt:  time.Now().Format(time.RFC3339),
		Monitors: monitors,
	}
	path, err := hyprMonitorPresetPath(name)
	if err != nil {
		return hyprMonitorPreset{}, "", err
	}
	data, err := json.MarshalIndent(preset, "", "  ")
	if err != nil {
		return hyprMonitorPreset{}, "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return hyprMonitorPreset{}, "", fmt.Errorf("write preset: %w", err)
	}
	return preset, path, nil
}

func loadHyprMonitorPreset(name string) (hyprMonitorPreset, string, error) {
	path, err := hyprMonitorPresetPath(name)
	if err != nil {
		return hyprMonitorPreset{}, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return hyprMonitorPreset{}, path, fmt.Errorf("read preset: %w", err)
	}
	var preset hyprMonitorPreset
	if err := json.Unmarshal(data, &preset); err != nil {
		return hyprMonitorPreset{}, path, fmt.Errorf("parse preset: %w", err)
	}
	if preset.Name == "" {
		preset.Name = strings.TrimSpace(name)
	}
	return preset, path, nil
}

func restoreHyprMonitorPreset(name string, dryRun bool) (hyprMonitorPresetRestoreOutput, error) {
	preset, path, err := loadHyprMonitorPreset(name)
	if err != nil {
		return hyprMonitorPresetRestoreOutput{}, err
	}
	commands, validationErrors := validateMonitorPreset(preset)
	output := hyprMonitorPresetRestoreOutput{
		Name:      preset.Name,
		Path:      path,
		Commands:  commands,
		Errors:    validationErrors,
		Validated: len(validationErrors) == 0,
	}
	if len(validationErrors) > 0 || dryRun {
		return output, nil
	}

	for idx, command := range commands {
		monitor := preset.Monitors[idx]
		if _, err := runHyprctl("keyword", "monitor", command); err != nil {
			output.Errors = append(output.Errors, fmt.Sprintf("%s: %v", monitor.Name, err))
			continue
		}
		if monitor.Enabled {
			output.Restored = append(output.Restored, monitor.Name)
		} else {
			output.Disabled = append(output.Disabled, monitor.Name)
		}
	}
	output.Applied = len(output.Errors) == 0
	return output, nil
}

func listHyprMonitorPresets() (hyprMonitorPresetListOutput, error) {
	dir, err := ensureDotfilesManagedStateDir("hypr", "monitor-presets")
	if err != nil {
		return hyprMonitorPresetListOutput{}, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return hyprMonitorPresetListOutput{}, err
	}
	var out hyprMonitorPresetListOutput
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var preset hyprMonitorPreset
		if err := json.Unmarshal(data, &preset); err != nil {
			continue
		}
		if preset.Name == "" {
			preset.Name = strings.TrimSuffix(filepath.Base(path), ".json")
		}
		out.Presets = append(out.Presets, hyprMonitorPresetListEntry{
			Name:         preset.Name,
			Path:         path,
			SavedAt:      preset.SavedAt,
			MonitorCount: len(preset.Monitors),
		})
	}
	sort.Slice(out.Presets, func(i, j int) bool { return out.Presets[i].Name < out.Presets[j].Name })
	return out, nil
}

func readHyprClients() ([]hyprClientState, error) {
	out, err := runHyprctl("clients", "-j")
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse clients: %w", err)
	}

	monitorNames := map[int]string{}
	monitors, _ := readHyprMonitors()
	for idx, mon := range monitors {
		monitorNames[idx] = mon.Name
	}

	clients := make([]hyprClientState, 0, len(raw))
	for _, item := range raw {
		client := hyprClientState{Mapped: true}
		if v, ok := item["address"].(string); ok {
			client.Address = v
		}
		if v, ok := item["class"].(string); ok {
			client.Class = v
		}
		if v, ok := item["title"].(string); ok {
			client.Title = v
		}
		if v, ok := item["initialClass"].(string); ok {
			client.InitialClass = v
		}
		if v, ok := item["initialTitle"].(string); ok {
			client.InitialTitle = v
		}
		if ws, ok := item["workspace"].(map[string]any); ok {
			if v, ok := ws["id"].(float64); ok {
				client.Workspace = int(v)
			}
			if v, ok := ws["name"].(string); ok {
				client.WorkspaceName = v
			}
		}
		if v, ok := item["monitor"].(float64); ok {
			client.Monitor = int(v)
			client.MonitorName = monitorNames[client.Monitor]
		}
		if v, ok := item["floating"].(bool); ok {
			client.Floating = v
		}
		if v, ok := item["mapped"].(bool); ok {
			client.Mapped = v
		}
		if v, ok := item["pinned"].(bool); ok {
			client.Pinned = v
		}
		if v, ok := item["fullscreen"].(bool); ok {
			client.Fullscreen = v
		} else if v, ok := item["fullscreenClient"].(bool); ok {
			client.Fullscreen = v
		} else if v, ok := item["fullscreen"].(float64); ok {
			client.Fullscreen = int(v) != 0
		}
		if v, ok := item["fullscreenMode"].(float64); ok {
			client.FullscreenMode = int(v)
		}
		if pos, ok := item["at"].([]any); ok && len(pos) == 2 {
			if v, ok := pos[0].(float64); ok {
				client.Position[0] = int(v)
			}
			if v, ok := pos[1].(float64); ok {
				client.Position[1] = int(v)
			}
		}
		if size, ok := item["size"].([]any); ok && len(size) == 2 {
			if v, ok := size[0].(float64); ok {
				client.Size[0] = int(v)
			}
			if v, ok := size[1].(float64); ok {
				client.Size[1] = int(v)
			}
		}
		clients = append(clients, client)
	}
	return clients, nil
}

func readHyprWorkspaces() ([]hyprSavedWorkspace, error) {
	out, err := runHyprctl("workspaces", "-j")
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse workspaces: %w", err)
	}
	workspaces := make([]hyprSavedWorkspace, 0, len(raw))
	for _, item := range raw {
		ws := hyprSavedWorkspace{}
		if v, ok := item["id"].(float64); ok {
			ws.ID = int(v)
		}
		if v, ok := item["name"].(string); ok {
			ws.Name = v
		}
		if v, ok := item["monitor"].(string); ok {
			ws.Monitor = v
		}
		workspaces = append(workspaces, ws)
	}
	sort.Slice(workspaces, func(i, j int) bool { return workspaces[i].ID < workspaces[j].ID })
	return workspaces, nil
}

func saveCurrentHyprLayout(input hyprLayoutSaveInput) (hyprLayoutSnapshot, string, error) {
	clients, err := readHyprClients()
	if err != nil {
		return hyprLayoutSnapshot{}, "", err
	}
	workspaceFilter := map[int]struct{}{}
	for _, id := range input.WorkspaceIDs {
		workspaceFilter[id] = struct{}{}
	}

	workspaces, _ := readHyprWorkspaces()
	windows := make([]hyprSavedWindow, 0, len(clients))
	for _, client := range clients {
		if len(workspaceFilter) > 0 {
			if _, ok := workspaceFilter[client.Workspace]; !ok {
				continue
			}
		}
		launch := input.LaunchCommands[client.Class]
		if launch == "" {
			launch = input.LaunchCommands[client.InitialClass]
		}
		if launch == "" {
			launch = input.LaunchCommands[client.Title]
		}
		windows = append(windows, hyprSavedWindow{
			Class:          client.Class,
			Title:          client.Title,
			InitialClass:   client.InitialClass,
			InitialTitle:   client.InitialTitle,
			Workspace:      client.Workspace,
			WorkspaceName:  client.WorkspaceName,
			Monitor:        client.Monitor,
			MonitorName:    client.MonitorName,
			Floating:       client.Floating,
			Mapped:         client.Mapped,
			Pinned:         client.Pinned,
			Fullscreen:     client.Fullscreen,
			FullscreenMode: client.FullscreenMode,
			Position:       client.Position,
			Size:           client.Size,
			LaunchCommand:  launch,
		})
	}
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].Workspace == windows[j].Workspace {
			if windows[i].Class == windows[j].Class {
				return windows[i].Title < windows[j].Title
			}
			return windows[i].Class < windows[j].Class
		}
		return windows[i].Workspace < windows[j].Workspace
	})

	snapshot := hyprLayoutSnapshot{
		Name:       strings.TrimSpace(input.Name),
		SavedAt:    time.Now().Format(time.RFC3339),
		Windows:    windows,
		Workspaces: workspaces,
	}
	path, err := hyprLayoutPath(input.Name)
	if err != nil {
		return hyprLayoutSnapshot{}, "", err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return hyprLayoutSnapshot{}, "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return hyprLayoutSnapshot{}, "", fmt.Errorf("write layout: %w", err)
	}
	return snapshot, path, nil
}

func loadHyprLayout(name string) (hyprLayoutSnapshot, string, error) {
	path, err := hyprLayoutPath(name)
	if err != nil {
		return hyprLayoutSnapshot{}, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return hyprLayoutSnapshot{}, path, fmt.Errorf("read layout: %w", err)
	}
	var snapshot hyprLayoutSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return hyprLayoutSnapshot{}, path, fmt.Errorf("parse layout: %w", err)
	}
	if snapshot.Name == "" {
		snapshot.Name = strings.TrimSpace(name)
	}
	return snapshot, path, nil
}

func listHyprLayouts() (hyprLayoutListOutput, error) {
	dir, err := ensureDotfilesManagedStateDir("hypr", "layouts")
	if err != nil {
		return hyprLayoutListOutput{}, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return hyprLayoutListOutput{}, err
	}
	var out hyprLayoutListOutput
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var snapshot hyprLayoutSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue
		}
		if snapshot.Name == "" {
			snapshot.Name = strings.TrimSuffix(filepath.Base(path), ".json")
		}
		out.Layouts = append(out.Layouts, hyprLayoutListEntry{
			Name:        snapshot.Name,
			Path:        path,
			SavedAt:     snapshot.SavedAt,
			WindowCount: len(snapshot.Windows),
		})
	}
	sort.Slice(out.Layouts, func(i, j int) bool { return out.Layouts[i].Name < out.Layouts[j].Name })
	return out, nil
}

func windowMatchScore(saved hyprSavedWindow, current hyprClientState) int {
	if !current.Mapped {
		return -1
	}
	score := 0
	switch {
	case saved.Class != "" && strings.EqualFold(saved.Class, current.Class):
		score += 50
	case saved.InitialClass != "" && strings.EqualFold(saved.InitialClass, current.InitialClass):
		score += 45
	default:
		return -1
	}

	if saved.Title != "" {
		switch {
		case current.Title == saved.Title:
			score += 35
		case strings.Contains(strings.ToLower(current.Title), strings.ToLower(saved.Title)):
			score += 20
		}
	}
	if saved.InitialTitle != "" {
		switch {
		case current.InitialTitle == saved.InitialTitle:
			score += 15
		case strings.Contains(strings.ToLower(current.InitialTitle), strings.ToLower(saved.InitialTitle)):
			score += 10
		}
	}
	if saved.Workspace > 0 && saved.Workspace == current.Workspace {
		score += 5
	}
	return score
}

func matchSavedWindow(saved hyprSavedWindow, clients []hyprClientState, used map[int]bool) (int, int) {
	bestIdx := -1
	bestScore := -1
	for idx, client := range clients {
		if used[idx] {
			continue
		}
		score := windowMatchScore(saved, client)
		if score > bestScore {
			bestIdx = idx
			bestScore = score
		}
	}
	return bestIdx, bestScore
}

func applySavedWindowState(saved hyprSavedWindow, current *hyprClientState, dryRun bool) ([]string, []string) {
	selector := "address:" + current.Address
	actions := make([]string, 0)
	errors := make([]string, 0)

	run := func(label string, args ...string) {
		actions = append(actions, label)
		if dryRun {
			return
		}
		if _, err := runHyprctl(args...); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", label, err))
		}
	}

	if saved.Workspace > 0 && saved.Workspace != current.Workspace {
		arg := fmt.Sprintf("%d,%s", saved.Workspace, selector)
		run("movetoworkspacesilent "+arg, "dispatch", "movetoworkspacesilent", arg)
		current.Workspace = saved.Workspace
	}

	if saved.Floating != current.Floating {
		run("togglefloating "+selector, "dispatch", "togglefloating", selector)
		current.Floating = saved.Floating
	}

	if saved.Floating {
		moveArg := fmt.Sprintf("exact %d %d,%s", saved.Position[0], saved.Position[1], selector)
		sizeArg := fmt.Sprintf("exact %d %d,%s", saved.Size[0], saved.Size[1], selector)
		run("movewindowpixel "+moveArg, "dispatch", "movewindowpixel", moveArg)
		run("resizewindowpixel "+sizeArg, "dispatch", "resizewindowpixel", sizeArg)
	}

	if saved.Pinned != current.Pinned {
		run("pin "+selector, "dispatch", "pin", selector)
		current.Pinned = saved.Pinned
	}

	if saved.Fullscreen != current.Fullscreen {
		run("focuswindow "+selector, "dispatch", "focuswindow", selector)
		run("fullscreen "+strconv.Itoa(saved.FullscreenMode), "dispatch", "fullscreen", strconv.Itoa(saved.FullscreenMode))
		current.Fullscreen = saved.Fullscreen
	}

	return actions, errors
}

func restoreHyprLayout(name string, dryRun bool, launchMissing bool) (hyprLayoutRestoreOutput, error) {
	snapshot, path, err := loadHyprLayout(name)
	if err != nil {
		return hyprLayoutRestoreOutput{}, err
	}
	output := hyprLayoutRestoreOutput{
		Name:    snapshot.Name,
		Path:    path,
		Applied: !dryRun,
		Windows: make([]hyprLayoutRestoreWindowStatus, 0, len(snapshot.Windows)),
	}

	clients, err := readHyprClients()
	if err != nil {
		return output, err
	}
	used := map[int]bool{}
	pendingLaunch := make([]hyprSavedWindow, 0)
	statuses := make([]hyprLayoutRestoreWindowStatus, 0, len(snapshot.Windows))

	for _, saved := range snapshot.Windows {
		idx, score := matchSavedWindow(saved, clients, used)
		if idx >= 0 && score >= 50 {
			used[idx] = true
			actions, applyErrors := applySavedWindowState(saved, &clients[idx], dryRun)
			status := hyprLayoutRestoreWindowStatus{
				Class:        saved.Class,
				Title:        saved.Title,
				Status:       "matched",
				MatchAddress: clients[idx].Address,
				Actions:      actions,
			}
			if len(applyErrors) > 0 {
				status.Status = "partial"
				status.Reason = strings.Join(applyErrors, "; ")
				output.Errors = append(output.Errors, applyErrors...)
			}
			statuses = append(statuses, status)
			continue
		}

		if launchMissing && strings.TrimSpace(saved.LaunchCommand) != "" {
			pendingLaunch = append(pendingLaunch, saved)
			statuses = append(statuses, hyprLayoutRestoreWindowStatus{
				Class:   saved.Class,
				Title:   saved.Title,
				Status:  "launching",
				Reason:  saved.LaunchCommand,
				Actions: []string{"launch " + saved.LaunchCommand},
			})
			continue
		}

		label := strings.TrimSpace(strings.Join([]string{saved.Class, saved.Title}, " "))
		output.Unresolved = append(output.Unresolved, strings.TrimSpace(label))
		statuses = append(statuses, hyprLayoutRestoreWindowStatus{
			Class:  saved.Class,
			Title:  saved.Title,
			Status: "unresolved",
			Reason: "no running window matched and no launch command was saved",
		})
	}

	if len(pendingLaunch) > 0 && !dryRun {
		for _, saved := range pendingLaunch {
			cmd := exec.Command("sh", "-lc", saved.LaunchCommand)
			if err := cmd.Start(); err != nil {
				output.Errors = append(output.Errors, fmt.Sprintf("launch %s: %v", saved.LaunchCommand, err))
				continue
			}
			output.Launched = append(output.Launched, saved.LaunchCommand)
		}
		time.Sleep(1200 * time.Millisecond)
		clients, _ = readHyprClients()
		used = map[int]bool{}
	}

	if len(pendingLaunch) > 0 {
		for idx := range statuses {
			if statuses[idx].Status != "launching" {
				continue
			}
			saved := pendingLaunch[0]
			pendingLaunch = pendingLaunch[1:]
			matchIdx, score := matchSavedWindow(saved, clients, used)
			if matchIdx >= 0 && score >= 50 {
				used[matchIdx] = true
				actions, applyErrors := applySavedWindowState(saved, &clients[matchIdx], dryRun)
				statuses[idx].Status = "launched"
				statuses[idx].MatchAddress = clients[matchIdx].Address
				statuses[idx].Actions = append(statuses[idx].Actions, actions...)
				if len(applyErrors) > 0 {
					statuses[idx].Status = "partial"
					statuses[idx].Reason = strings.Join(applyErrors, "; ")
					output.Errors = append(output.Errors, applyErrors...)
				}
				continue
			}
			statuses[idx].Status = "unresolved"
			statuses[idx].Reason = "launch command ran but no matching window appeared yet"
			output.Unresolved = append(output.Unresolved, strings.TrimSpace(strings.Join([]string{saved.Class, saved.Title}, " ")))
		}
	}

	output.Windows = statuses
	if dryRun {
		output.Applied = false
	} else if len(output.Errors) > 0 || len(output.Unresolved) > 0 {
		output.Applied = false
	}
	return output, nil
}

func openDesktopProject(input desktopProjectOpenInput) (desktopProjectOpenOutput, error) {
	repoPath := strings.TrimSpace(input.RepoPath)
	if repoPath == "" {
		return desktopProjectOpenOutput{}, fmt.Errorf("[%s] repo_path is required", handler.ErrInvalidParam)
	}
	info, err := os.Stat(repoPath)
	if err != nil {
		return desktopProjectOpenOutput{}, fmt.Errorf("stat repo_path: %w", err)
	}
	if !info.IsDir() {
		return desktopProjectOpenOutput{}, fmt.Errorf("[%s] repo_path must be a directory", handler.ErrInvalidParam)
	}

	output := desktopProjectOpenOutput{
		RepoPath:      repoPath,
		MonitorPreset: strings.TrimSpace(input.MonitorPreset),
		Layout:        strings.TrimSpace(input.Layout),
	}

	if output.MonitorPreset != "" {
		restore, err := restoreHyprMonitorPreset(output.MonitorPreset, input.DryRun)
		if err != nil {
			return output, err
		}
		output.MonitorRestore = &restore
		output.Actions = append(output.Actions, "monitor preset "+output.MonitorPreset)
		if len(restore.Errors) > 0 {
			output.Warnings = append(output.Warnings, restore.Errors...)
		}
	}

	if output.Layout != "" {
		restore, err := restoreHyprLayout(output.Layout, input.DryRun, defaultBool(input.LaunchMissing, true))
		if err != nil {
			return output, err
		}
		output.LayoutRestore = &restore
		output.Actions = append(output.Actions, "layout "+output.Layout)
		if len(restore.Errors) > 0 {
			output.Warnings = append(output.Warnings, restore.Errors...)
		}
	}

	sessionName := strings.TrimSpace(input.TmuxSession)
	if sessionName == "" {
		sessionName = filepath.Base(repoPath)
	}
	output.TmuxSession = sessionName

	if input.DryRun {
		output.Actions = append(output.Actions, "tmux session "+sessionName)
		return output, nil
	}

	if exec.Command("tmux", "has-session", "-t", sessionName).Run() == nil {
		output.Actions = append(output.Actions, "tmux session exists "+sessionName)
		return output, nil
	}

	args := []string{"new-session", "-d", "-s", sessionName, "-c", repoPath}
	if strings.TrimSpace(input.TmuxCommand) != "" {
		args = append(args, input.TmuxCommand)
	}
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output.Warnings = append(output.Warnings, fmt.Sprintf("tmux bootstrap failed: %v: %s", err, strings.TrimSpace(string(out))))
		return output, nil
	}
	output.Actions = append(output.Actions, "tmux session created "+sessionName)
	return output, nil
}
