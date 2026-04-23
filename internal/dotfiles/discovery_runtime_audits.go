package dotfiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type dotfilesLauncherAuditInput struct {
	LogLines int `json:"log_lines,omitempty" jsonschema:"description=Number of recent journal lines to inspect. Default 30."`
}

type dotfilesCommandAvailability struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

type dotfilesLauncherScriptsStatus struct {
	AppLauncherPath   string `json:"app_launcher_path"`
	AppLauncherExists bool   `json:"app_launcher_exists"`
	AppSwitcherPath   string `json:"app_switcher_path"`
	AppSwitcherExists bool   `json:"app_switcher_exists"`
	LibraryPath       string `json:"library_path"`
	LibraryExists     bool   `json:"library_exists"`
}

type dotfilesLauncherHyprshellAudit struct {
	CommandAvailable bool   `json:"command_available"`
	ProcessRunning   bool   `json:"process_running"`
	ServiceState     string `json:"service_state"`
	SocketPath       string `json:"socket_path,omitempty"`
	SocketExists     bool   `json:"socket_exists"`
	OverviewVisible  bool   `json:"overview_visible"`
	SwitchVisible    bool   `json:"switch_visible"`
}

type dotfilesLauncherAuditOutput struct {
	Profile         string                         `json:"profile"`
	Status          string                         `json:"status"`
	Runtime         dotfilesDesktopRuntime         `json:"runtime"`
	Scripts         dotfilesLauncherScriptsStatus  `json:"scripts"`
	Commands        []dotfilesCommandAvailability  `json:"commands"`
	Hyprshell       dotfilesLauncherHyprshellAudit `json:"hyprshell"`
	FallbackReady   bool                           `json:"fallback_ready"`
	LogSignals      []string                       `json:"log_signals,omitempty"`
	RecentLogs      []string                       `json:"recent_logs,omitempty"`
	Recommendations []string                       `json:"recommendations,omitempty"`
	SuggestedTools  []string                       `json:"suggested_tools,omitempty"`
	Errors          []string                       `json:"errors,omitempty"`
	Summary         string                         `json:"summary"`
	ReportMarkdown  string                         `json:"report_markdown"`
}

type dotfilesBarAuditInput struct {
	LogLines int `json:"log_lines,omitempty" jsonschema:"description=Number of recent journal lines to inspect. Default 30."`
}

type ironbarWidget struct {
	Type string `toml:"type"`
}

type ironbarLayout struct {
	Start  []ironbarWidget `toml:"start"`
	Center []ironbarWidget `toml:"center"`
	End    []ironbarWidget `toml:"end"`
}

type ironbarConfig struct {
	Start    []ironbarWidget          `toml:"start"`
	Center   []ironbarWidget          `toml:"center"`
	End      []ironbarWidget          `toml:"end"`
	Monitors map[string]ironbarLayout `toml:"monitors"`
}

type dotfilesBarMonitorLayout struct {
	Name          string   `json:"name"`
	Source        string   `json:"source,omitempty"`
	StartModules  []string `json:"start_modules,omitempty"`
	CenterModules []string `json:"center_modules,omitempty"`
	EndModules    []string `json:"end_modules,omitempty"`
	StartCount    int      `json:"start_count"`
	CenterCount   int      `json:"center_count"`
	EndCount      int      `json:"end_count"`
	TotalModules  int      `json:"total_modules"`
	Signals       []string `json:"signals,omitempty"`
}

type dotfilesBarAuditOutput struct {
	Profile               string                     `json:"profile"`
	Status                string                     `json:"status"`
	Runtime               dotfilesDesktopRuntime     `json:"runtime"`
	ConfigPath            string                     `json:"config_path"`
	ConfigExists          bool                       `json:"config_exists"`
	ServiceState          string                     `json:"service_state"`
	MonitorSpecific       bool                       `json:"monitor_specific"`
	DefaultBar            *dotfilesBarMonitorLayout  `json:"default_bar,omitempty"`
	ConfiguredMonitors    []dotfilesBarMonitorLayout `json:"configured_monitors,omitempty"`
	AppliedMonitors       []dotfilesBarMonitorLayout `json:"applied_monitors,omitempty"`
	LiveMonitors          []monitorInfo              `json:"live_monitors,omitempty"`
	MissingMonitorConfigs []string                   `json:"missing_monitor_configs,omitempty"`
	StaleMonitorConfigs   []string                   `json:"stale_monitor_configs,omitempty"`
	LogSignals            []string                   `json:"log_signals,omitempty"`
	RecentLogs            []string                   `json:"recent_logs,omitempty"`
	Recommendations       []string                   `json:"recommendations,omitempty"`
	SuggestedTools        []string                   `json:"suggested_tools,omitempty"`
	Errors                []string                   `json:"errors,omitempty"`
	Summary               string                     `json:"summary"`
	ReportMarkdown        string                     `json:"report_markdown"`
}

func (m *DotfilesDiscoveryModule) launcherAuditTool() registry.ToolDefinition {
	td := handler.TypedHandler[dotfilesLauncherAuditInput, dotfilesLauncherAuditOutput](
		"dotfiles_launcher_audit",
		"Audit the launcher stack by checking wrapper scripts, hyprshell socket readiness, fallback launchers, and recent hyprshell journal signals.",
		func(ctx context.Context, input dotfilesLauncherAuditInput) (dotfilesLauncherAuditOutput, error) {
			return m.buildLauncherAudit(ctx, input), nil
		},
	)
	td.Category = "discovery"
	td.SearchTerms = []string{
		"launcher audit",
		"mod d broken",
		"wofi fallback",
		"hyprshell socket",
		"launcher logs",
		"switcher audit",
	}
	return td
}

func (m *DotfilesDiscoveryModule) barAuditTool() registry.ToolDefinition {
	td := handler.TypedHandler[dotfilesBarAuditInput, dotfilesBarAuditOutput](
		"dotfiles_bar_audit",
		"Audit Ironbar layout readiness by checking live monitors, monitor-specific config coverage, service state, and recent bar log noise.",
		func(ctx context.Context, input dotfilesBarAuditInput) (dotfilesBarAuditOutput, error) {
			return m.buildBarAudit(ctx, input), nil
		},
	)
	td.Category = "discovery"
	td.SearchTerms = []string{
		"bar audit",
		"ironbar audit",
		"menubar cut off",
		"monitor specific bar",
		"ironbar logs",
		"boot menubar",
	}
	return td
}

func (m *DotfilesDiscoveryModule) buildLauncherAudit(ctx context.Context, input dotfilesLauncherAuditInput) dotfilesLauncherAuditOutput {
	logLines := input.LogLines
	if logLines <= 0 {
		logLines = 30
	}

	runtime := dotfilesDiscoveryRuntime()
	appLauncherPath := filepath.Join(dotfilesDir(), "scripts", "app-launcher.sh")
	appSwitcherPath := filepath.Join(dotfilesDir(), "scripts", "app-switcher.sh")
	libraryPath := filepath.Join(dotfilesDir(), "scripts", "lib", "launcher.sh")

	commands, availability := dotfilesCommandMatrix("hyprshell", "wofi", "rofi", "hyprctl", "jq", "systemctl", "journalctl")
	socketPath, socketExists := dotfilesHyprshellSocket(runtime.XDGRuntimeDir, runtime.HyprlandInstanceSignature)

	out := dotfilesLauncherAuditOutput{
		Profile: dotfilesProfile(),
		Status:  "ok",
		Runtime: runtime,
		Scripts: dotfilesLauncherScriptsStatus{
			AppLauncherPath:   appLauncherPath,
			AppLauncherExists: pathExists(appLauncherPath),
			AppSwitcherPath:   appSwitcherPath,
			AppSwitcherExists: pathExists(appSwitcherPath),
			LibraryPath:       libraryPath,
			LibraryExists:     pathExists(libraryPath),
		},
		Commands: commands,
		Hyprshell: dotfilesLauncherHyprshellAudit{
			CommandAvailable: availability["hyprshell"],
			ProcessRunning:   processRunningExact("hyprshell"),
			ServiceState:     dotfilesUserUnitState(ctx, "dotfiles-hyprshell.service"),
			SocketPath:       socketPath,
			SocketExists:     socketExists,
		},
		FallbackReady: availability["wofi"] || availability["rofi"],
		SuggestedTools: []string{
			"dotfiles_launcher_audit",
			"dotfiles_desktop_status",
			"hypr_list_layers",
			"systemd_status",
			"systemd_logs",
			"hypr_screenshot_monitors",
		},
	}

	if layersRaw, err := dotfilesHyprLayersRaw(); err == nil {
		out.Hyprshell.OverviewVisible = strings.Contains(layersRaw, "hyprshell_overview") || strings.Contains(layersRaw, "hyprshell_launcher")
		out.Hyprshell.SwitchVisible = strings.Contains(layersRaw, "hyprshell_switch")
	} else if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("hyprctl layers: %v", err))
	}

	logs, err := dotfilesJournalTail(ctx, "dotfiles-hyprshell.service", logLines)
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("journalctl dotfiles-hyprshell.service: %v", err))
	} else {
		out.RecentLogs = logs
		out.LogSignals = dotfilesLogSignals(logs, map[string]string{
			"invalid transfer received":  "hyprshell invalid transfer warnings present",
			"failed to create windows":   "hyprshell window creation failures present",
			"no active hyprland monitor": "hyprshell monitor-detection race warnings present",
			"failed to create overview":  "hyprshell overview creation failures present",
			"failed to create switch":    "hyprshell switch creation failures present",
		})
	}

	hyprshellReady := out.Hyprshell.CommandAvailable && out.Hyprshell.ProcessRunning && out.Hyprshell.SocketExists
	if !out.Scripts.AppLauncherExists || !out.Scripts.AppSwitcherExists || !out.Scripts.LibraryExists {
		out.Status = "degraded"
		out.Recommendations = append(out.Recommendations, "Restore the launcher wrapper scripts under dotfiles/scripts so Mod+D and switcher bindings have stable entrypoints.")
	}
	if !hyprshellReady && !out.FallbackReady {
		out.Status = "degraded"
		out.Recommendations = append(out.Recommendations, "No launcher surface is ready. Restore hyprshell readiness or install a fallback launcher such as wofi or rofi.")
	}
	if out.Hyprshell.CommandAvailable && !hyprshellReady {
		out.Recommendations = append(out.Recommendations, "hyprshell is installed but not fully ready; verify dotfiles-hyprshell.service, runtime env, and socket path.")
	}
	if len(out.LogSignals) > 0 {
		out.Status = "degraded"
		out.Recommendations = append(out.Recommendations, "Recent hyprshell warnings remain in the journal; inspect log signals before trusting the native overview path.")
	}
	if out.FallbackReady {
		out.Recommendations = append(out.Recommendations, "Fallback launcher path is available; wrappers can still open wofi/rofi while hyprshell is being repaired.")
	}
	if hyprshellReady && out.FallbackReady && len(out.LogSignals) == 0 && len(out.Errors) == 0 {
		out.Recommendations = append(out.Recommendations, "Launcher stack looks healthy: both hyprshell and fallback launchers are available for recovery.")
	}
	if len(out.Errors) > 0 {
		out.Status = "degraded"
	}

	hyprshellState := "not ready"
	if hyprshellReady {
		hyprshellState = "ready"
	}
	fallbackState := "missing"
	if out.FallbackReady {
		fallbackState = "ready"
	}
	out.Recommendations = orderedUniqueStrings(out.Recommendations)
	out.Summary = fmt.Sprintf("hyprshell %s, fallback launcher %s, %d recent hyprshell log signals.", hyprshellState, fallbackState, len(out.LogSignals))
	out.ReportMarkdown = renderLauncherAuditMarkdown(out)
	return out
}

func (m *DotfilesDiscoveryModule) buildBarAudit(ctx context.Context, input dotfilesBarAuditInput) dotfilesBarAuditOutput {
	logLines := input.LogLines
	if logLines <= 0 {
		logLines = 30
	}

	runtime := dotfilesDiscoveryRuntime()
	configPath := filepath.Join(dotfilesDir(), "ironbar", "config.toml")
	out := dotfilesBarAuditOutput{
		Profile:      dotfilesProfile(),
		Status:       "ok",
		Runtime:      runtime,
		ConfigPath:   configPath,
		ConfigExists: pathExists(configPath),
		ServiceState: dotfilesUserUnitState(ctx, "ironbar.service"),
		SuggestedTools: []string{
			"dotfiles_bar_audit",
			"dotfiles_desktop_status",
			"hypr_get_monitors",
			"hypr_screenshot_monitors",
			"systemd_status",
			"systemd_logs",
		},
	}

	var cfg ironbarConfig
	if out.ConfigExists {
		if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("decode ironbar config: %v", err))
		}
	} else {
		out.Errors = append(out.Errors, "ironbar config is missing")
	}

	defaultLayout := summarizeIronbarLayout("default", "default", ironbarLayout{
		Start:  cfg.Start,
		Center: cfg.Center,
		End:    cfg.End,
	})
	if defaultLayout.TotalModules > 0 {
		out.DefaultBar = &defaultLayout
	}

	configuredNames := make([]string, 0, len(cfg.Monitors))
	for name, layout := range cfg.Monitors {
		configuredNames = append(configuredNames, name)
		out.ConfiguredMonitors = append(out.ConfiguredMonitors, summarizeIronbarLayout(name, "explicit", layout))
	}
	sort.Strings(configuredNames)
	sort.Slice(out.ConfiguredMonitors, func(i, j int) bool { return out.ConfiguredMonitors[i].Name < out.ConfiguredMonitors[j].Name })
	out.MonitorSpecific = len(out.ConfiguredMonitors) > 0

	liveMonitors, err := invokeToolJSON[monitorsResult](ctx, (&HyprlandModule{}).Tools(), "hypr_get_monitors", map[string]any{})
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("hypr_get_monitors: %v", err))
	} else {
		out.LiveMonitors = liveMonitors.Monitors
	}

	layoutByName := make(map[string]dotfilesBarMonitorLayout, len(out.ConfiguredMonitors))
	for _, layout := range out.ConfiguredMonitors {
		layoutByName[layout.Name] = layout
	}
	liveNames := make([]string, 0, len(out.LiveMonitors))
	liveNameSet := make(map[string]struct{}, len(out.LiveMonitors))
	for _, monitor := range out.LiveMonitors {
		liveNames = append(liveNames, monitor.Name)
		liveNameSet[monitor.Name] = struct{}{}
		applied, ok := layoutByName[monitor.Name]
		switch {
		case ok:
			applied.Source = "explicit"
		case out.DefaultBar != nil:
			applied = *out.DefaultBar
			applied.Name = monitor.Name
			applied.Source = "default"
		default:
			out.MissingMonitorConfigs = append(out.MissingMonitorConfigs, monitor.Name)
			applied = dotfilesBarMonitorLayout{Name: monitor.Name, Source: "missing"}
		}
		applied.Signals = barLayoutSignals(monitor, applied, len(out.LiveMonitors), out.DefaultBar != nil && !out.MonitorSpecific)
		out.AppliedMonitors = append(out.AppliedMonitors, applied)
	}
	sort.Strings(liveNames)
	sort.Slice(out.AppliedMonitors, func(i, j int) bool { return out.AppliedMonitors[i].Name < out.AppliedMonitors[j].Name })
	for _, configured := range configuredNames {
		if _, ok := liveNameSet[configured]; !ok {
			out.StaleMonitorConfigs = append(out.StaleMonitorConfigs, configured)
		}
	}

	logs, err := dotfilesJournalTail(ctx, "ironbar.service", logLines)
	if err != nil {
		out.Errors = append(out.Errors, fmt.Sprintf("journalctl ironbar.service: %v", err))
	} else {
		out.RecentLogs = logs
		out.LogSignals = dotfilesLogSignals(logs, map[string]string{
			"could not find it": "ironbar tray/menu sync warnings present",
			"workspace event received before initialization": "ironbar workspace init race warnings present",
			"vk_suboptimal_khr": "ironbar vulkan swapchain warnings present",
		})
	}

	if out.ServiceState != "" && out.ServiceState != "active" && out.ServiceState != "unknown" && out.ServiceState != "unavailable" {
		out.Status = "degraded"
		out.Recommendations = append(out.Recommendations, "ironbar.service is not active; restore the service before expecting a boot menubar.")
	}
	if len(out.MissingMonitorConfigs) > 0 {
		out.Status = "degraded"
		out.Recommendations = append(out.Recommendations, fmt.Sprintf("Live monitors missing explicit or default bar coverage: %s.", strings.Join(out.MissingMonitorConfigs, ", ")))
	}
	if len(out.StaleMonitorConfigs) > 0 {
		out.Recommendations = append(out.Recommendations, fmt.Sprintf("Configured monitor entries no longer match live outputs: %s.", strings.Join(out.StaleMonitorConfigs, ", ")))
	}
	for _, applied := range out.AppliedMonitors {
		for _, signal := range applied.Signals {
			out.Recommendations = append(out.Recommendations, fmt.Sprintf("%s: %s.", applied.Name, signal))
		}
	}
	if out.DefaultBar != nil && !out.MonitorSpecific && len(out.LiveMonitors) > 1 && out.DefaultBar.TotalModules >= 8 {
		out.Recommendations = append(out.Recommendations, "Single default Ironbar layout is dense and will clone across multiple outputs; consider monitor-specific bars.")
	}
	if len(out.LogSignals) > 0 {
		out.Status = "degraded"
		out.Recommendations = append(out.Recommendations, "Recent Ironbar warnings remain in the journal; remove recurring boot noise before calling the bar clean.")
	}
	if len(out.Errors) > 0 {
		out.Status = "degraded"
	}
	if out.ConfigExists && len(out.Errors) == 0 && len(out.MissingMonitorConfigs) == 0 && len(out.LogSignals) == 0 {
		out.Recommendations = append(out.Recommendations, "Ironbar layout coverage looks complete for current outputs; use monitor-specific sections to keep aspect-ratio tuning durable.")
	}

	out.Recommendations = orderedUniqueStrings(out.Recommendations)
	out.Summary = fmt.Sprintf("%d live monitors, %d configured monitor layouts, %d missing monitor mappings, %d recent Ironbar log signals.", len(out.LiveMonitors), len(out.ConfiguredMonitors), len(out.MissingMonitorConfigs), len(out.LogSignals))
	out.ReportMarkdown = renderBarAuditMarkdown(out)
	return out
}

func dotfilesDiscoveryRuntime() dotfilesDesktopRuntime {
	runtimeDir := dotfilesRuntimeDir()
	hyprSocketDir := ""
	if runtimeDir != "" {
		candidate := filepath.Join(runtimeDir, "hypr")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			hyprSocketDir = candidate
		}
	}
	return dotfilesDesktopRuntime{
		XDGRuntimeDir:             runtimeDir,
		WaylandDisplay:            dotfilesWaylandDisplay(runtimeDir),
		HyprlandInstanceSignature: dotfilesHyprlandSignature(runtimeDir),
		HyprlandSocketDir:         hyprSocketDir,
	}
}

func dotfilesCommandMatrix(names ...string) ([]dotfilesCommandAvailability, map[string]bool) {
	out := make([]dotfilesCommandAvailability, 0, len(names))
	availability := make(map[string]bool, len(names))
	for _, name := range names {
		ready := hasCmd(name)
		out = append(out, dotfilesCommandAvailability{Name: name, Available: ready})
		availability[name] = ready
	}
	return out, availability
}

func dotfilesHyprshellSocket(runtimeDir, signature string) (string, bool) {
	if runtimeDir == "" {
		return "", false
	}
	if signature != "" {
		instanceSocket := filepath.Join(runtimeDir, "hypr", signature, "hyprshell.sock")
		if pathExists(instanceSocket) {
			return instanceSocket, true
		}
		runtimeSocket := filepath.Join(runtimeDir, "hyprshell.sock")
		if pathExists(runtimeSocket) {
			return runtimeSocket, true
		}
		return instanceSocket, false
	}
	runtimeSocket := filepath.Join(runtimeDir, "hyprshell.sock")
	return runtimeSocket, pathExists(runtimeSocket)
}

func dotfilesUserUnitState(ctx context.Context, unit string) string {
	if !hasCmd("systemctl") {
		return "unavailable"
	}
	stdout, stderr, err := systemdRunCmd(ctx, "systemctl", "--user", "is-active", unit)
	state := strings.TrimSpace(stdout)
	if state == "" {
		state = strings.TrimSpace(stderr)
	}
	if state == "" {
		if err != nil {
			return "unknown"
		}
		return "inactive"
	}
	return state
}

func dotfilesJournalTail(ctx context.Context, unit string, lines int) ([]string, error) {
	if !hasCmd("journalctl") {
		return nil, fmt.Errorf("journalctl unavailable")
	}
	if lines <= 0 {
		lines = 20
	}
	stdout, stderr, err := systemdRunCmd(ctx, "journalctl", "--user-unit", unit, "-b", "-n", strconv.Itoa(lines), "--no-pager")
	raw := strings.TrimSpace(stdout)
	if raw == "" {
		raw = strings.TrimSpace(stderr)
	}
	if err != nil && raw == "" {
		return nil, err
	}
	if raw == "" || raw == "-- No entries --" {
		return nil, nil
	}
	out := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "-- No entries --" {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}

func dotfilesLogSignals(lines []string, patterns map[string]string) []string {
	if len(lines) == 0 || len(patterns) == 0 {
		return nil
	}
	counts := make(map[string]int, len(patterns))
	for _, line := range lines {
		lower := strings.ToLower(line)
		for needle, label := range patterns {
			if strings.Contains(lower, needle) {
				counts[label]++
			}
		}
	}
	signals := make([]string, 0, len(counts))
	for label, count := range counts {
		signals = append(signals, fmt.Sprintf("%dx %s", count, label))
	}
	sort.Strings(signals)
	return signals
}

func dotfilesHyprLayersRaw() (string, error) {
	if !hasCmd("hyprctl") {
		return "", fmt.Errorf("hyprctl unavailable")
	}
	out, err := runHyprctl("layers", "-j")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func summarizeIronbarLayout(name, source string, layout ironbarLayout) dotfilesBarMonitorLayout {
	start := ironbarWidgetTypes(layout.Start)
	center := ironbarWidgetTypes(layout.Center)
	end := ironbarWidgetTypes(layout.End)
	return dotfilesBarMonitorLayout{
		Name:          name,
		Source:        source,
		StartModules:  start,
		CenterModules: center,
		EndModules:    end,
		StartCount:    len(start),
		CenterCount:   len(center),
		EndCount:      len(end),
		TotalModules:  len(start) + len(center) + len(end),
	}
}

func ironbarWidgetTypes(widgets []ironbarWidget) []string {
	out := make([]string, 0, len(widgets))
	for _, widget := range widgets {
		widgetType := strings.TrimSpace(widget.Type)
		if widgetType == "" {
			widgetType = "unknown"
		}
		out = append(out, widgetType)
	}
	return out
}

func barLayoutSignals(monitor monitorInfo, layout dotfilesBarMonitorLayout, liveMonitorCount int, defaultCloned bool) []string {
	signals := make([]string, 0)
	width, height, ok := parseResolutionWH(monitor.Resolution)
	if ok {
		aspect := float64(width) / float64(height)
		if aspect >= 3.0 && layout.TotalModules >= 10 {
			signals = append(signals, "ultrawide monitor uses a dense bar; keep truncation lengths short")
		}
		if aspect <= 2.0 && layout.TotalModules >= 8 {
			signals = append(signals, "standard-width monitor uses a dense bar; compact modules may prevent clipping")
		}
	}
	if defaultCloned && liveMonitorCount > 1 && layout.TotalModules >= 8 {
		signals = append(signals, "default layout will clone across every monitor")
	}
	if layout.Source == "missing" {
		signals = append(signals, "no layout applies to this live monitor")
	}
	return orderedUniqueStrings(signals)
}

func parseResolutionWH(resolution string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(resolution), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil || height == 0 {
		return 0, 0, false
	}
	return width, height, true
}

func renderLauncherAuditMarkdown(out dotfilesLauncherAuditOutput) string {
	lines := []string{
		"# Launcher Audit",
		"",
		fmt.Sprintf("- Profile: `%s`", out.Profile),
		fmt.Sprintf("- Status: `%s`", out.Status),
		fmt.Sprintf("- Summary: %s", out.Summary),
		fmt.Sprintf("- Hyprshell service: `%s`", out.Hyprshell.ServiceState),
		fmt.Sprintf("- Hyprshell socket: `%s`", boolLabel(out.Hyprshell.SocketExists)),
		fmt.Sprintf("- Fallback launcher ready: `%s`", boolLabel(out.FallbackReady)),
	}
	if out.Hyprshell.SocketPath != "" {
		lines = append(lines, fmt.Sprintf("- Socket path: `%s`", out.Hyprshell.SocketPath))
	}
	if len(out.LogSignals) > 0 {
		lines = append(lines, "", "## Log Signals")
		for _, signal := range out.LogSignals {
			lines = append(lines, "- "+signal)
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

func renderBarAuditMarkdown(out dotfilesBarAuditOutput) string {
	lines := []string{
		"# Bar Audit",
		"",
		fmt.Sprintf("- Profile: `%s`", out.Profile),
		fmt.Sprintf("- Status: `%s`", out.Status),
		fmt.Sprintf("- Summary: %s", out.Summary),
		fmt.Sprintf("- Ironbar service: `%s`", out.ServiceState),
		fmt.Sprintf("- Config path: `%s`", out.ConfigPath),
		fmt.Sprintf("- Monitor-specific config: `%s`", boolLabel(out.MonitorSpecific)),
	}
	if len(out.MissingMonitorConfigs) > 0 {
		lines = append(lines, fmt.Sprintf("- Missing monitor mappings: `%s`", strings.Join(out.MissingMonitorConfigs, "`, `")))
	}
	if len(out.StaleMonitorConfigs) > 0 {
		lines = append(lines, fmt.Sprintf("- Stale configured monitors: `%s`", strings.Join(out.StaleMonitorConfigs, "`, `")))
	}
	if len(out.LogSignals) > 0 {
		lines = append(lines, "", "## Log Signals")
		for _, signal := range out.LogSignals {
			lines = append(lines, "- "+signal)
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

func boolLabel(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
