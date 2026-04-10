package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/prompts"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/mcpkit/resources"
)

type dotfilesToolSearchInput struct {
	Query string `json:"query" jsonschema:"required,description=Keywords to search across tool names descriptions tags and search terms"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Maximum results to return. Default 10."`
}

type dotfilesToolSearchResult struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	SearchTerms []string `json:"search_terms,omitempty"`
	MatchType   string   `json:"match_type"`
	Deferred    bool     `json:"deferred"`
}

type dotfilesToolSearchOutput struct {
	Results []dotfilesToolSearchResult `json:"results"`
	Total   int                        `json:"total"`
}

type dotfilesToolSchemaInput struct {
	Name string `json:"name" jsonschema:"required,description=Exact tool name to inspect"`
}

type dotfilesToolSchemaOutput struct {
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	Tags           []string       `json:"tags,omitempty"`
	SearchTerms    []string       `json:"search_terms,omitempty"`
	IsWrite        bool           `json:"is_write"`
	Deferred       bool           `json:"deferred"`
	InputSchema    map[string]any `json:"input_schema,omitempty"`
	OutputSchema   map[string]any `json:"output_schema,omitempty"`
	DescriptorMeta map[string]any `json:"descriptor_meta,omitempty"`
}

type dotfilesToolCatalogInput struct {
	Category string `json:"category,omitempty" jsonschema:"description=Optional category filter"`
}

type dotfilesToolCatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Deferred    bool   `json:"deferred"`
}

type dotfilesToolCatalogGroup struct {
	Category      string                     `json:"category"`
	ToolCount     int                        `json:"tool_count"`
	DeferredCount int                        `json:"deferred_count"`
	Tools         []dotfilesToolCatalogEntry `json:"tools"`
}

type dotfilesToolCatalogOutput struct {
	Groups []dotfilesToolCatalogGroup `json:"groups"`
}

type dotfilesToolStatsInput struct{}

type dotfilesToolStatsOutput struct {
	TotalTools      int            `json:"total_tools"`
	ModuleCount     int            `json:"module_count"`
	DeferredTools   int            `json:"deferred_tools"`
	ResourceCount   int            `json:"resource_count"`
	PromptCount     int            `json:"prompt_count"`
	WorkflowCount   int            `json:"workflow_count"`
	SkillCount      int            `json:"skill_count"`
	ByCategory      map[string]int `json:"by_category"`
	ByRuntimeGroup  map[string]int `json:"by_runtime_group"`
	WriteToolsCount int            `json:"write_tools_count"`
	ReadOnlyCount   int            `json:"read_only_count"`
}

type dotfilesServerHealthInput struct{}

type dotfilesServerHealthOutput struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Status          string   `json:"status"`
	Profile         string   `json:"profile"`
	RuntimeOS       string   `json:"runtime_os"`
	TotalTools      int      `json:"total_tools"`
	ModuleCount     int      `json:"module_count"`
	DeferredTools   int      `json:"deferred_tools"`
	ResourceCount   int      `json:"resource_count"`
	PromptCount     int      `json:"prompt_count"`
	WorkflowCount   int      `json:"workflow_count"`
	SkillCount      int      `json:"skill_count"`
	PrioritySummary any      `json:"priority_summary,omitempty"`
	DiscoveryTools  []string `json:"discovery_tools"`
}

type dotfilesDesktopStatusInput struct{}

type dotfilesDesktopRuntime struct {
	XDGRuntimeDir             string `json:"xdg_runtime_dir,omitempty"`
	WaylandDisplay            string `json:"wayland_display,omitempty"`
	HyprlandInstanceSignature string `json:"hyprland_instance_signature,omitempty"`
	HyprlandSocketDir         string `json:"hyprland_socket_dir,omitempty"`
}

type dotfilesDesktopCapability struct {
	Ready   bool     `json:"ready"`
	Details []string `json:"details,omitempty"`
	Missing []string `json:"missing,omitempty"`
}

type dotfilesDesktopStatusOutput struct {
	Profile         string                    `json:"profile"`
	Status          string                    `json:"status"`
	Runtime         dotfilesDesktopRuntime    `json:"runtime"`
	Hyprland        dotfilesDesktopCapability `json:"hyprland"`
	Shell           dotfilesDesktopCapability `json:"shell"`
	Screenshot      dotfilesDesktopCapability `json:"screenshot"`
	OCR             dotfilesDesktopCapability `json:"ocr"`
	Input           dotfilesDesktopCapability `json:"input"`
	Accessibility   dotfilesDesktopCapability `json:"accessibility"`
	DesktopSession  dotfilesDesktopCapability `json:"desktop_session"`
	Eww             dotfilesDesktopCapability `json:"eww"`
	Notifications   dotfilesDesktopCapability `json:"notifications"`
	Terminal        dotfilesDesktopCapability `json:"terminal"`
	Shader          dotfilesDesktopCapability `json:"shader"`
	MissingCommands []string                  `json:"missing_commands,omitempty"`
}

type DotfilesDiscoveryModule struct {
	reg       *registry.ToolRegistry
	resources *resources.ResourceRegistry
	prompts   *prompts.PromptRegistry
	version   string
}

func (m *DotfilesDiscoveryModule) Name() string { return "discovery" }
func (m *DotfilesDiscoveryModule) Description() string {
	return "Discovery tools for the dotfiles catalog"
}

func (m *DotfilesDiscoveryModule) Tools() []registry.ToolDefinition {
	search := handler.TypedHandler[dotfilesToolSearchInput, dotfilesToolSearchOutput](
		"dotfiles_tool_search",
		"Search the dotfiles tool catalog by keyword before invoking a desktop or workflow tool.",
		func(_ context.Context, input dotfilesToolSearchInput) (dotfilesToolSearchOutput, error) {
			limit := input.Limit
			if limit <= 0 {
				limit = 10
			}
			results := m.reg.SearchTools(input.Query)
			if len(results) > limit {
				results = results[:limit]
			}
			out := make([]dotfilesToolSearchResult, 0, len(results))
			for _, result := range results {
				out = append(out, dotfilesToolSearchResult{
					Name:        result.Tool.Tool.Name,
					Description: result.Tool.Tool.Description,
					Category:    result.Tool.Category,
					Tags:        result.Tool.Tags,
					SearchTerms: result.Tool.SearchTerms,
					MatchType:   result.MatchType,
					Deferred:    m.reg.IsDeferred(result.Tool.Tool.Name),
				})
			}
			return dotfilesToolSearchOutput{Results: out, Total: len(out)}, nil
		},
	)
	search.Category = "discovery"
	search.SearchTerms = []string{"find tool", "which tool", "tool discovery", "catalog search"}

	schema := handler.TypedHandler[dotfilesToolSchemaInput, dotfilesToolSchemaOutput](
		"dotfiles_tool_schema",
		"Inspect one tool descriptor including schemas, search terms, and deferred-loading hints.",
		func(_ context.Context, input dotfilesToolSchemaInput) (dotfilesToolSchemaOutput, error) {
			td, ok := m.reg.GetTool(input.Name)
			if !ok {
				return dotfilesToolSchemaOutput{}, fmt.Errorf("tool not found: %s", input.Name)
			}
			annotated := registry.ApplyToolMetadata(td, "", m.reg.IsDeferred(td.Tool.Name))
			return dotfilesToolSchemaOutput{
				Name:           td.Tool.Name,
				Description:    td.Tool.Description,
				Category:       td.Category,
				Tags:           td.Tags,
				SearchTerms:    td.SearchTerms,
				IsWrite:        td.IsWrite,
				Deferred:       m.reg.IsDeferred(td.Tool.Name),
				InputSchema:    schemaMap(td.Tool.InputSchema),
				OutputSchema:   schemaMap(annotated.Tool.OutputSchema),
				DescriptorMeta: toolMetaMap(annotated.Tool),
			}, nil
		},
	)
	schema.Category = "discovery"
	schema.SearchTerms = []string{"tool schema", "tool descriptor", "input schema", "output schema"}

	catalog := handler.TypedHandler[dotfilesToolCatalogInput, dotfilesToolCatalogOutput](
		"dotfiles_tool_catalog",
		"Summarize the dotfiles tool catalog by category with deferred-loading hints.",
		func(_ context.Context, input dotfilesToolCatalogInput) (dotfilesToolCatalogOutput, error) {
			groups := make([]dotfilesToolCatalogGroup, 0)
			catalog := m.reg.GetToolCatalog()
			categories := make([]string, 0, len(catalog))
			for category := range catalog {
				if input.Category == "" || input.Category == category {
					categories = append(categories, category)
				}
			}
			sort.Strings(categories)

			for _, category := range categories {
				group := dotfilesToolCatalogGroup{Category: category}
				subcategories := catalog[category]
				subcategoryNames := make([]string, 0, len(subcategories))
				for subcategory := range subcategories {
					subcategoryNames = append(subcategoryNames, subcategory)
				}
				sort.Strings(subcategoryNames)
				for _, subcategory := range subcategoryNames {
					tools := subcategories[subcategory]
					sort.Slice(tools, func(i, j int) bool { return tools[i].Tool.Name < tools[j].Tool.Name })
					for _, td := range tools {
						deferred := m.reg.IsDeferred(td.Tool.Name)
						group.ToolCount++
						if deferred {
							group.DeferredCount++
						}
						group.Tools = append(group.Tools, dotfilesToolCatalogEntry{
							Name:        td.Tool.Name,
							Description: td.Tool.Description,
							Deferred:    deferred,
						})
					}
				}
				groups = append(groups, group)
			}

			return dotfilesToolCatalogOutput{Groups: groups}, nil
		},
	)
	catalog.Category = "discovery"
	catalog.SearchTerms = []string{"tool catalog", "tool list", "categories", "browse tools"}

	stats := handler.TypedHandler[dotfilesToolStatsInput, dotfilesToolStatsOutput](
		"dotfiles_tool_stats",
		"Show high-level catalog statistics, including how many tools are marked deferred in the active profile.",
		func(_ context.Context, _ dotfilesToolStatsInput) (dotfilesToolStatsOutput, error) {
			toolStats := m.reg.GetToolStats()
			totalTools := len(m.reg.GetAllToolDefinitions())
			resourceCount := 0
			promptCount := 0
			if m.resources != nil {
				resourceCount = m.resources.ResourceCount() + m.resources.TemplateCount()
			}
			if m.prompts != nil {
				promptCount = m.prompts.PromptCount()
			}
			return dotfilesToolStatsOutput{
				TotalTools:      totalTools,
				ModuleCount:     toolStats.ModuleCount,
				DeferredTools:   len(m.reg.ListDeferredTools()),
				ResourceCount:   resourceCount,
				PromptCount:     promptCount,
				WorkflowCount:   len(dotfilesWorkflowCatalog()),
				SkillCount:      len(dotfilesSkillCatalog()),
				ByCategory:      toolStats.ByCategory,
				ByRuntimeGroup:  toolStats.ByRuntimeGroup,
				WriteToolsCount: toolStats.WriteToolsCount,
				ReadOnlyCount:   toolStats.ReadOnlyCount,
			}, nil
		},
	)
	stats.Category = "discovery"
	stats.SearchTerms = []string{"tool stats", "catalog stats", "tool counts"}

	serverHealth := handler.TypedHandler[dotfilesServerHealthInput, dotfilesServerHealthOutput](
		"dotfiles_server_health",
		"Show the active dotfiles-mcp contract shape: discovery coverage, tool counts, profile, and resource/prompt availability.",
		func(_ context.Context, _ dotfilesServerHealthInput) (dotfilesServerHealthOutput, error) {
			toolStats := m.reg.GetToolStats()
			totalTools := len(m.reg.GetAllToolDefinitions())
			resourceCount := 0
			promptCount := 0
			if m.resources != nil {
				resourceCount = m.resources.ResourceCount() + m.resources.TemplateCount()
			}
			if m.prompts != nil {
				promptCount = m.prompts.PromptCount()
			}

			discoveryTools := make([]string, 0, len(dotfilesDiscoveryToolNames()))
			for _, name := range dotfilesDiscoveryToolNames() {
				if _, ok := m.reg.GetTool(name); ok {
					discoveryTools = append(discoveryTools, name)
				}
			}

			status := "ok"
			if len(discoveryTools) < len(dotfilesDiscoveryToolNames()) || resourceCount == 0 || promptCount == 0 {
				status = "degraded"
			}

			return dotfilesServerHealthOutput{
				Name:            "dotfiles-mcp",
				Version:         m.version,
				Status:          status,
				Profile:         dotfilesProfile(),
				RuntimeOS:       runtime.GOOS,
				TotalTools:      totalTools,
				ModuleCount:     toolStats.ModuleCount,
				DeferredTools:   len(m.reg.ListDeferredTools()),
				ResourceCount:   resourceCount,
				PromptCount:     promptCount,
				WorkflowCount:   len(dotfilesWorkflowCatalog()),
				SkillCount:      len(dotfilesSkillCatalog()),
				PrioritySummary: buildDotfilesPrioritySummary(),
				DiscoveryTools:  discoveryTools,
			}, nil
		},
	)
	serverHealth.Category = "discovery"
	serverHealth.SearchTerms = []string{"server health", "mcp health", "contract status", "server overview"}

	desktopStatus := handler.TypedHandler[dotfilesDesktopStatusInput, dotfilesDesktopStatusOutput](
		"dotfiles_desktop_status",
		"Report desktop control readiness, including Hyprland, AT-SPI semantic targeting, session helpers, screenshot/OCR, input injection, and the kitty-to-ghostty terminal shader pipeline.",
		func(_ context.Context, _ dotfilesDesktopStatusInput) (dotfilesDesktopStatusOutput, error) {
			runtimeDir := dotfilesRuntimeDir()
			waylandDisplay := dotfilesWaylandDisplay(runtimeDir)
			hyprSignature := dotfilesHyprlandSignature(runtimeDir)
			hyprSocketDir := ""
			if runtimeDir != "" {
				candidate := filepath.Join(runtimeDir, "hypr")
				if info, err := os.Stat(candidate); err == nil && info.IsDir() {
					hyprSocketDir = candidate
				}
			}

			missingCommands := make([]string, 0)

			hyprlandMissing := make([]string, 0)
			hyprlandDetails := make([]string, 0)
			if hasCmd("hyprctl") {
				hyprlandDetails = append(hyprlandDetails, "hyprctl available")
			} else {
				hyprlandMissing = append(hyprlandMissing, "hyprctl")
				missingCommands = append(missingCommands, "hyprctl")
			}
			if waylandDisplay != "" {
				hyprlandDetails = append(hyprlandDetails, "WAYLAND_DISPLAY="+waylandDisplay)
			} else {
				hyprlandMissing = append(hyprlandMissing, "WAYLAND_DISPLAY")
			}
			if hyprSignature != "" {
				hyprlandDetails = append(hyprlandDetails, "HYPRLAND_INSTANCE_SIGNATURE resolved")
			} else {
				hyprlandMissing = append(hyprlandMissing, "HYPRLAND_INSTANCE_SIGNATURE")
			}
			if hyprSocketDir != "" {
				hyprlandDetails = append(hyprlandDetails, "Hyprland socket dir "+hyprSocketDir)
			}

			shellMissing := make([]string, 0)
			shellDetails := make([]string, 0)
			for _, cmd := range []string{"hyprshell", "hypr-dock", "hyprdynamicmonitors", "hyprland-autoname-workspaces"} {
				if hasCmd(cmd) {
					shellDetails = append(shellDetails, cmd+" available")
				} else {
					shellMissing = append(shellMissing, cmd)
					missingCommands = append(missingCommands, cmd)
				}
			}
			for _, component := range []struct {
				name    string
				running bool
			}{
				{name: "hyprshell", running: processRunningExact("hyprshell")},
				{name: "hypr-dock", running: processRunningExact("hypr-dock")},
				{name: "hyprdynamicmonitors", running: processRunningExact("hyprdynamicmonitors")},
				{name: "hyprland-autoname-workspaces", running: processRunningExact("hyprland-autoname-workspaces")},
			} {
				if component.running {
					shellDetails = append(shellDetails, component.name+" running")
				} else {
					shellDetails = append(shellDetails, component.name+" not running")
				}
			}
			monitorInclude := desktopMonitorIncludePath()
			if pathExists(monitorInclude) {
				shellDetails = append(shellDetails, "dynamic monitor include present at "+monitorInclude)
			} else {
				shellMissing = append(shellMissing, "monitors.dynamic.conf")
			}

			screenshotMissing := make([]string, 0)
			screenshotDetails := make([]string, 0)
			if hasCmd("wayshot") {
				screenshotDetails = append(screenshotDetails, "wayshot available")
			} else {
				screenshotMissing = append(screenshotMissing, "wayshot")
				missingCommands = append(missingCommands, "wayshot")
			}
			if hasCmd("magick") {
				screenshotDetails = append(screenshotDetails, "ImageMagick inline resize enabled")
			} else {
				screenshotMissing = append(screenshotMissing, "magick")
				missingCommands = append(missingCommands, "magick")
			}
			if waylandDisplay == "" {
				screenshotMissing = append(screenshotMissing, "WAYLAND_DISPLAY")
			}

			ocrMissing := make([]string, 0)
			ocrDetails := make([]string, 0)
			if hasCmd("tesseract") {
				ocrDetails = append(ocrDetails, "tesseract available")
			} else {
				ocrMissing = append(ocrMissing, "tesseract")
				missingCommands = append(missingCommands, "tesseract")
			}
			if hasCmd("magick") {
				ocrDetails = append(ocrDetails, "ImageMagick preprocessing enabled")
			} else {
				ocrMissing = append(ocrMissing, "magick")
			}
			if !hasCmd("wayshot") {
				ocrMissing = append(ocrMissing, "wayshot")
			}
			if waylandDisplay == "" {
				ocrMissing = append(ocrMissing, "WAYLAND_DISPLAY")
			}

			inputMissing := make([]string, 0)
			inputDetails := make([]string, 0)
			if hasCmd("ydotool") {
				inputDetails = append(inputDetails, "ydotool pointer and key injection available")
			} else {
				inputMissing = append(inputMissing, "ydotool")
				missingCommands = append(missingCommands, "ydotool")
			}
			if hasCmd("wtype") {
				inputDetails = append(inputDetails, "wtype text entry available")
			} else {
				inputMissing = append(inputMissing, "wtype")
				missingCommands = append(missingCommands, "wtype")
			}

			accessibilityMissing := make([]string, 0)
			accessibilityDetails := make([]string, 0)
			if hasCmd("python3") {
				accessibilityDetails = append(accessibilityDetails, "python3 available")
			} else {
				accessibilityMissing = append(accessibilityMissing, "python3")
				missingCommands = append(missingCommands, "python3")
			}
			if helperPath, err := dotfilesSemanticHelperPath(); err == nil {
				accessibilityDetails = append(accessibilityDetails, "embedded AT-SPI helper present at "+helperPath)
			} else {
				accessibilityMissing = append(accessibilityMissing, "embedded AT-SPI helper")
				accessibilityDetails = append(accessibilityDetails, err.Error())
			}
			if hasCmd("python3") {
				cmd := exec.Command("python3", "-c", "import pyatspi")
				if out, err := cmd.CombinedOutput(); err == nil {
					accessibilityDetails = append(accessibilityDetails, "pyatspi import succeeded")
				} else {
					accessibilityMissing = append(accessibilityMissing, "pyatspi")
					accessibilityDetails = append(accessibilityDetails, "pyatspi import failed: "+strings.TrimSpace(string(out)))
				}
			}
			if os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
				accessibilityDetails = append(accessibilityDetails, "DBUS_SESSION_BUS_ADDRESS present")
			} else {
				accessibilityDetails = append(accessibilityDetails, "DBUS_SESSION_BUS_ADDRESS not detected")
			}

			sessionMissing := make([]string, 0)
			sessionDetails := make([]string, 0)
			if hasCmd("wayland-info") {
				sessionDetails = append(sessionDetails, "wayland-info available")
			} else {
				sessionMissing = append(sessionMissing, "wayland-info")
				missingCommands = append(missingCommands, "wayland-info")
			}
			if hasCmd("grim") {
				sessionDetails = append(sessionDetails, "grim available for session screenshots")
			} else {
				sessionMissing = append(sessionMissing, "grim")
				missingCommands = append(missingCommands, "grim")
			}
			if hasCmd("wl-copy") {
				sessionDetails = append(sessionDetails, "wl-copy available for session clipboard writes")
			} else {
				sessionMissing = append(sessionMissing, "wl-copy")
				missingCommands = append(missingCommands, "wl-copy")
			}
			if hasCmd("wl-paste") {
				sessionDetails = append(sessionDetails, "wl-paste available for session clipboard reads")
			} else {
				sessionMissing = append(sessionMissing, "wl-paste")
				missingCommands = append(missingCommands, "wl-paste")
			}
			if hasCmd("kwin_wayland") {
				sessionDetails = append(sessionDetails, "kwin_wayland available for virtual sessions")
			} else {
				sessionDetails = append(sessionDetails, "kwin_wayland not detected; virtual sessions unavailable")
			}
			if waylandDisplay != "" {
				sessionDetails = append(sessionDetails, "live Wayland session detected via "+waylandDisplay)
			} else {
				sessionDetails = append(sessionDetails, "live Wayland session not detected")
			}

			ewwStatus := currentEwwStatus()
			ewwMissing := make([]string, 0)
			ewwDetails := make([]string, 0)
			if hasCmd("eww") {
				ewwDetails = append(ewwDetails, "eww available")
			} else {
				ewwMissing = append(ewwMissing, "eww")
				missingCommands = append(missingCommands, "eww")
			}
			ewwConfig := dotfilesEwwConfigDir()
			if pathExists(ewwConfig) {
				ewwDetails = append(ewwDetails, "config rooted at "+ewwConfig)
			} else {
				ewwMissing = append(ewwMissing, "eww config")
			}
			if ewwStatus.DaemonRunning {
				ewwDetails = append(ewwDetails, fmt.Sprintf("daemon running (%d processes)", ewwStatus.DaemonCount))
			} else {
				ewwMissing = append(ewwMissing, "eww daemon")
			}
			ewwDetails = append(ewwDetails, fmt.Sprintf("%d active windows", len(ewwStatus.Windows)))
			ewwDetails = append(ewwDetails, fmt.Sprintf("%d defined windows", len(ewwStatus.DefinedWindows)))

			notificationMissing := make([]string, 0)
			notificationDetails := make([]string, 0)
			if hasCmd("swaync-client") {
				notificationDetails = append(notificationDetails, "swaync-client available")
			} else {
				notificationMissing = append(notificationMissing, "swaync-client")
				missingCommands = append(missingCommands, "swaync-client")
			}
			if hasCmd("dbus-monitor") {
				notificationDetails = append(notificationDetails, "dbus-monitor available")
			} else {
				notificationMissing = append(notificationMissing, "dbus-monitor")
				missingCommands = append(missingCommands, "dbus-monitor")
			}
			if hasCmd("python3") {
				notificationDetails = append(notificationDetails, "python3 available for notification history listener")
			} else {
				notificationMissing = append(notificationMissing, "python3")
				missingCommands = append(missingCommands, "python3")
			}
			listenerPath := dotfilesNotificationHistoryListenerPath()
			if pathExists(listenerPath) {
				notificationDetails = append(notificationDetails, "history listener script present at "+listenerPath)
			} else {
				notificationMissing = append(notificationMissing, "notification-history-listener.py")
			}
			historyEntries, _ := readNotificationHistoryEntries()
			notificationDetails = append(notificationDetails, fmt.Sprintf("%d tracked notification entries", len(historyEntries)))
			if notificationHistoryListenerRunning() {
				notificationDetails = append(notificationDetails, "notification history listener running")
			} else {
				notificationDetails = append(notificationDetails, "notification history listener not running")
			}

			kittyConfigPath := filepath.Join(dotfilesDir(), "kitty", "kitty.conf")
			ghosttyConfigPath := filepath.Join(dotfilesDir(), "ghostty", "config")
			terminalMissing := make([]string, 0)
			terminalDetails := make([]string, 0)
			kittyInstalled := hasCmd("kitty")
			ghosttyInstalled := hasCmd("ghostty")
			if kittyInstalled {
				terminalDetails = append(terminalDetails, "kitty available")
			} else {
				terminalMissing = append(terminalMissing, "kitty")
				missingCommands = append(missingCommands, "kitty")
			}
			if ghosttyInstalled {
				terminalDetails = append(terminalDetails, "ghostty available")
			} else {
				terminalMissing = append(terminalMissing, "ghostty")
				missingCommands = append(missingCommands, "ghostty")
			}
			if _, err := os.Stat(kittyConfigPath); err == nil {
				terminalDetails = append(terminalDetails, "kitty config rooted at "+kittyConfigPath)
			}
			if _, err := os.Stat(ghosttyConfigPath); err == nil {
				terminalDetails = append(terminalDetails, "ghostty state-aware config rooted at "+ghosttyConfigPath)
			}

			shaderScriptPath := filepath.Join(dotfilesDir(), "scripts", "kitty-shader-playlist.sh")
			shaderDir := filepath.Join(dotfilesDir(), "kitty", "shaders", "crtty")
			shaderMissing := make([]string, 0)
			shaderDetails := make([]string, 0)
			if _, err := os.Stat(shaderScriptPath); err == nil {
				shaderDetails = append(shaderDetails, "kitty shader playlist script available")
			} else {
				shaderMissing = append(shaderMissing, "kitty-shader-playlist.sh")
			}
			if _, err := os.Stat(shaderDir); err == nil {
				shaderDetails = append(shaderDetails, "kitty shader source dir available")
			} else {
				shaderMissing = append(shaderMissing, "kitty shader source dir")
			}
			if kittyInstalled {
				shaderDetails = append(shaderDetails, "kitty is the active shader write target")
			} else {
				shaderMissing = append(shaderMissing, "kitty")
			}
			if _, err := os.Stat(ghosttyConfigPath); err == nil {
				shaderDetails = append(shaderDetails, "ghostty config present for state-aware terminal parity")
			}

			output := dotfilesDesktopStatusOutput{
				Profile: dotfilesProfile(),
				Status:  "ok",
				Runtime: dotfilesDesktopRuntime{
					XDGRuntimeDir:             runtimeDir,
					WaylandDisplay:            waylandDisplay,
					HyprlandInstanceSignature: hyprSignature,
					HyprlandSocketDir:         hyprSocketDir,
				},
				Hyprland: dotfilesDesktopCapability{
					Ready:   len(hyprlandMissing) == 0,
					Details: hyprlandDetails,
					Missing: uniqueSortedStrings(hyprlandMissing),
				},
				Shell: dotfilesDesktopCapability{
					Ready:   len(shellMissing) == 0,
					Details: shellDetails,
					Missing: uniqueSortedStrings(shellMissing),
				},
				Screenshot: dotfilesDesktopCapability{
					Ready:   len(screenshotMissing) == 0,
					Details: screenshotDetails,
					Missing: uniqueSortedStrings(screenshotMissing),
				},
				OCR: dotfilesDesktopCapability{
					Ready:   len(ocrMissing) == 0,
					Details: ocrDetails,
					Missing: uniqueSortedStrings(ocrMissing),
				},
				Input: dotfilesDesktopCapability{
					Ready:   len(inputMissing) == 0,
					Details: inputDetails,
					Missing: uniqueSortedStrings(inputMissing),
				},
				Accessibility: dotfilesDesktopCapability{
					Ready:   len(accessibilityMissing) == 0,
					Details: accessibilityDetails,
					Missing: uniqueSortedStrings(accessibilityMissing),
				},
				DesktopSession: dotfilesDesktopCapability{
					Ready:   (waylandDisplay != "" || hasCmd("kwin_wayland")) && hasCmd("wayland-info"),
					Details: sessionDetails,
					Missing: uniqueSortedStrings(sessionMissing),
				},
				Eww: dotfilesDesktopCapability{
					Ready:   len(ewwMissing) == 0,
					Details: ewwDetails,
					Missing: uniqueSortedStrings(ewwMissing),
				},
				Notifications: dotfilesDesktopCapability{
					Ready:   len(notificationMissing) == 0,
					Details: notificationDetails,
					Missing: uniqueSortedStrings(notificationMissing),
				},
				Terminal: dotfilesDesktopCapability{
					Ready:   kittyInstalled || ghosttyInstalled,
					Details: terminalDetails,
					Missing: uniqueSortedStrings(terminalMissing),
				},
				Shader: dotfilesDesktopCapability{
					Ready:   len(shaderMissing) == 0,
					Details: shaderDetails,
					Missing: uniqueSortedStrings(shaderMissing),
				},
				MissingCommands: uniqueSortedStrings(missingCommands),
			}
			if !(output.Hyprland.Ready && output.Shell.Ready && output.Screenshot.Ready && output.OCR.Ready && output.Input.Ready && output.Accessibility.Ready && output.DesktopSession.Ready && output.Eww.Ready && output.Notifications.Ready && output.Terminal.Ready && output.Shader.Ready) {
				output.Status = "degraded"
			}
			return output, nil
		},
	)
	desktopStatus.Category = "discovery"
	desktopStatus.SearchTerms = []string{
		"desktop status",
		"desktop readiness",
		"hyprland status",
		"hyprshell",
		"hypr-dock",
		"hyprdynamicmonitors",
		"autoname",
		"wayland env",
		"ocr readiness",
		"click automation",
		"eww",
		"notification history",
		"kitty",
		"ghostty",
		"terminal shader",
	}

	workstationDiagnostics := m.workstationDiagnosticsTool()
	workspaceScene := m.workspaceSceneTool()

	return []registry.ToolDefinition{search, schema, catalog, stats, serverHealth, desktopStatus, workstationDiagnostics, workspaceScene}
}

// annotatedModule wraps a ToolModule to apply MCP annotations and circuit breaker groups.
type annotatedModule struct {
	inner registry.ToolModule
}

func (m *annotatedModule) Name() string        { return m.inner.Name() }
func (m *annotatedModule) Description() string { return m.inner.Description() }
func (m *annotatedModule) Tools() []registry.ToolDefinition {
	tools := m.inner.Tools()
	for i := range tools {
		tools[i] = registry.ApplyMCPAnnotations(tools[i], "dotfiles_")
		if strings.HasPrefix(tools[i].Tool.Name, "dotfiles_gh_") {
			tools[i].CircuitBreakerGroup = "github"
		}
	}
	return tools
}

func registerDotfilesModules(reg *registry.ToolRegistry, resReg *resources.ResourceRegistry, promptReg *prompts.PromptRegistry, version string) {
	reg.RegisterModule(&DotfilesDiscoveryModule{reg: reg, resources: resReg, prompts: promptReg, version: version})

	profile := dotfilesProfile()
	for _, module := range dotfilesModules() {
		wrapped := &annotatedModule{inner: module}
		deferred := make(map[string]bool)
		for _, td := range wrapped.Tools() {
			if shouldDeferDotfilesTool(profile, td) {
				deferred[td.Tool.Name] = true
			}
		}
		reg.RegisterDeferredModule(wrapped, deferred)
	}
}

func dotfilesModules() []registry.ToolModule {
	return []registry.ToolModule{
		&DotfilesModule{},
		&GitHubStarsModule{},
		&HyprlandModule{},
		&KittyModule{},
		&ShaderModule{},
		&InputModule{},
		&BluetoothModule{},
		&ControllerModule{},
		&MidiModule{},
		&JuhradialModule{},
		&WorkflowModule{},
		&OSSModule{},
		&MappingEngineModule{},
		&LearnModule{},
		&MappingStatusModule{},
		&MappingDaemonModule{},
		&ScreenModule{},
		&ClipboardModule{},
		&NotifyModule{},
		&NotificationModule{},
		&EwwDesktopModule{},
		&InputSimulateModule{},
		&DesktopInteractModule{},
		&DesktopSemanticModule{},
		&DesktopSessionModule{},
		&ClaudeSessionModule{},
		&PromptRegistryModule{},
		&SandboxModule{},
		&OpsModule{},
		&AudioModule{},
		&NetworkModule{},
		&ArchModule{},
		&SystemModule{},
		&SystemdModule{},
		&TmuxModule{},
		&ProcessModule{},
	}
}

func dotfilesProfile() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DOTFILES_MCP_PROFILE"))) {
	case "", "default":
		return "default"
	case "desktop":
		return "desktop"
	case "ops":
		return "ops"
	case "full":
		return "full"
	default:
		return "default"
	}
}

func shouldDeferDotfilesTool(profile string, td registry.ToolDefinition) bool {
	switch profile {
	case "full":
		return false
	case "desktop":
		return !isDesktopProfileTool(td.Tool.Name)
	case "ops":
		return !(strings.HasPrefix(td.Tool.Name, "dotfiles_") ||
			strings.HasPrefix(td.Tool.Name, "workflow_") ||
			strings.HasPrefix(td.Tool.Name, "oss_") ||
			strings.HasPrefix(td.Tool.Name, "archwiki_") ||
			strings.HasPrefix(td.Tool.Name, "arch_"))
	default:
		return true
	}
}

func dotfilesDiscoveryToolNames() []string {
	return []string{
		"dotfiles_tool_search",
		"dotfiles_tool_schema",
		"dotfiles_tool_catalog",
		"dotfiles_tool_stats",
		"dotfiles_server_health",
		"dotfiles_desktop_status",
		"dotfiles_workstation_diagnostics",
		"dotfiles_workspace_scene",
	}
}

func isDesktopProfileTool(name string) bool {
	switch {
	case strings.HasPrefix(name, "hypr_"),
		strings.HasPrefix(name, "kitty_"),
		strings.HasPrefix(name, "screen_"),
		strings.HasPrefix(name, "desktop_"),
		strings.HasPrefix(name, "session_"),
		strings.HasPrefix(name, "shader_"),
		strings.HasPrefix(name, "clipboard_"),
		strings.HasPrefix(name, "notify_"),
		strings.HasPrefix(name, "notification_"),
		strings.HasPrefix(name, "input_"):
		return true
	}

	switch name {
	case "dotfiles_list_configs",
		"dotfiles_validate_config",
		"dotfiles_reload_service",
		"dotfiles_cascade_reload",
		"dotfiles_rice_check",
		"dotfiles_eww_status",
		"dotfiles_eww_get",
		"dotfiles_eww_restart",
		"dotfiles_eww_inspect",
		"dotfiles_eww_reload":
		return true
	default:
		return false
	}
}

func dotfilesRuntimeDir() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); dir != "" {
		return dir
	}
	dir := fmt.Sprintf("/run/user/%d", os.Getuid())
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

func dotfilesWaylandDisplay(runtimeDir string) string {
	if display := strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")); display != "" {
		return display
	}
	if runtimeDir == "" {
		return ""
	}
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return ""
	}
	sockets := make([]string, 0)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "wayland-") {
			sockets = append(sockets, entry.Name())
		}
	}
	sort.Strings(sockets)
	for _, preferred := range []string{"wayland-1", "wayland-0"} {
		for _, socket := range sockets {
			if socket == preferred {
				return socket
			}
		}
	}
	if len(sockets) > 0 {
		return sockets[0]
	}
	return ""
}

func dotfilesHyprlandSignature(runtimeDir string) string {
	if sig := strings.TrimSpace(os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")); sig != "" {
		return sig
	}
	if runtimeDir == "" {
		return ""
	}
	hyprDir := filepath.Join(runtimeDir, "hypr")
	entries, err := os.ReadDir(hyprDir)
	if err != nil {
		return ""
	}
	signatures := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			signatures = append(signatures, entry.Name())
		}
	}
	sort.Strings(signatures)
	if len(signatures) > 0 {
		return signatures[0]
	}
	return ""
}

func schemaMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func toolMetaMap(tool registry.Tool) map[string]any {
	if tool.Meta == nil {
		return nil
	}
	data, err := json.Marshal(tool.Meta)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
