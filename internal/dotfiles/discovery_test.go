package dotfiles

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// dotfilesProfile
// ---------------------------------------------------------------------------

func TestDotfilesProfile(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{"", "default"},
		{"default", "default"},
		{"Default", "default"},
		{"DEFAULT", "default"},
		{"desktop", "desktop"},
		{"Desktop", "desktop"},
		{"DESKTOP", "desktop"},
		{"ops", "ops"},
		{"Ops", "ops"},
		{"OPS", "ops"},
		{"full", "full"},
		{"Full", "full"},
		{"FULL", "full"},
		{"  full  ", "full"},
		{"unknown", "default"},
		{"random_value", "default"},
	}
	for _, tc := range tests {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv("DOTFILES_MCP_PROFILE", tc.env)
			got := dotfilesProfile()
			if got != tc.want {
				t.Errorf("dotfilesProfile() with env=%q: got %q, want %q", tc.env, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// shouldDeferDotfilesTool
// ---------------------------------------------------------------------------

func TestShouldDeferDotfilesTool(t *testing.T) {
	tests := []struct {
		profile  string
		toolName string
		want     bool
	}{
		// full profile: nothing deferred
		{"full", "shader_list", false},
		{"full", "dotfiles_validate_config", false},
		{"full", "hypr_list_windows", false},

		// desktop profile: desktop control surfaces eager, non-desktop deferred
		{"desktop", "hypr_list_windows", false},
		{"desktop", "screen_screenshot", false},
		{"desktop", "desktop_find_text", false},
		{"desktop", "desktop_snapshot", false},
		{"desktop", "desktop_act", false},
		{"desktop", "desktop_project_open", false},
		{"desktop", "session_connect", false},
		{"desktop", "session_accessibility_tree", false},
		{"desktop", "shader_status", false},
		{"desktop", "input_type_text", false},
		{"desktop", "dotfiles_rice_check", false},
		{"desktop", "hypr_monitor_preset_list", false},
		{"desktop", "dotfiles_eww_inspect", false},
		{"desktop", "dotfiles_fleet_audit", true},
		{"desktop", "bt_connect", true},
		{"desktop", "midi_list_devices", true},
		{"desktop", "system_info", true},

		// ops profile: dotfiles_, workflow_, oss_ are NOT deferred
		{"ops", "dotfiles_validate_config", false},
		{"ops", "dotfiles_rice_check", false},
		{"ops", "workflow_sync", false},
		{"ops", "oss_score", false},
		{"ops", "shader_list", true},
		{"ops", "hypr_list_windows", true},
		{"ops", "bt_connect", true},

		// default profile: everything deferred
		{"default", "shader_list", true},
		{"default", "dotfiles_validate_config", true},
		{"default", "hypr_list_windows", true},
	}
	for _, tc := range tests {
		t.Run(tc.profile+"_"+tc.toolName, func(t *testing.T) {
			td := registry.ToolDefinition{}
			td.Tool.Name = tc.toolName
			got := shouldDeferDotfilesTool(tc.profile, td)
			if got != tc.want {
				t.Errorf("shouldDeferDotfilesTool(%q, %q) = %v, want %v", tc.profile, tc.toolName, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// schemaMap
// ---------------------------------------------------------------------------

func TestSchemaMap(t *testing.T) {
	// Nil input.
	if m := schemaMap(nil); m != nil {
		t.Errorf("schemaMap(nil) = %v, want nil", m)
	}

	// Valid struct.
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	m := schemaMap(input)
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if m["type"] != "object" {
		t.Errorf("type = %v, want object", m["type"])
	}
}

// ---------------------------------------------------------------------------
// registerDotfilesModules profiles
// ---------------------------------------------------------------------------

func TestRegisterDotfilesModules_FullProfile(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "full")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

	// In full profile, nothing should be deferred.
	if reg.IsDeferred("shader_list") {
		t.Error("shader_list should NOT be deferred in full profile")
	}
	if reg.IsDeferred("hypr_list_windows") {
		t.Error("hypr_list_windows should NOT be deferred in full profile")
	}
}

func TestRegisterDotfilesModules_OpsProfile(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "ops")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

	// dotfiles_* should be eager in ops profile.
	if reg.IsDeferred("dotfiles_validate_config") {
		t.Error("dotfiles_validate_config should NOT be deferred in ops profile")
	}
	// Non-dotfiles tools should be deferred.
	if !reg.IsDeferred("shader_list") {
		t.Error("shader_list should be deferred in ops profile")
	}
}

func TestRegisterDotfilesModules_DesktopProfile(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "desktop")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

	for _, toolName := range []string{
		"hypr_list_windows",
		"screen_screenshot",
		"desktop_find_text",
		"desktop_snapshot",
		"desktop_act",
		"desktop_project_open",
		"session_connect",
		"session_accessibility_tree",
		"shader_status",
		"input_type_text",
		"dotfiles_rice_check",
		"hypr_monitor_preset_list",
		"dotfiles_eww_inspect",
	} {
		if reg.IsDeferred(toolName) {
			t.Fatalf("%s should NOT be deferred in desktop profile", toolName)
		}
	}
	for _, toolName := range []string{
		"dotfiles_fleet_audit",
		"bt_connect",
		"midi_list_devices",
		"system_info",
	} {
		if !reg.IsDeferred(toolName) {
			t.Fatalf("%s should be deferred in desktop profile", toolName)
		}
	}
}

// ---------------------------------------------------------------------------
// DotfilesDiscoveryModule
// ---------------------------------------------------------------------------

func TestDiscoveryModuleRegistration(t *testing.T) {
	reg := registry.NewToolRegistry()
	m := &DotfilesDiscoveryModule{reg: reg, version: dotfilesMCPVersion}

	if m.Name() != "discovery" {
		t.Fatalf("expected name discovery, got %s", m.Name())
	}

	tools := m.Tools()
	if len(tools) != 6 {
		t.Fatalf("expected 6 discovery tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, td := range tools {
		names[td.Tool.Name] = true
	}
	for _, want := range []string{
		"dotfiles_tool_search",
		"dotfiles_tool_schema",
		"dotfiles_tool_catalog",
		"dotfiles_tool_stats",
		"dotfiles_server_health",
		"dotfiles_desktop_status",
	} {
		if !names[want] {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Tool stats (integration)
// ---------------------------------------------------------------------------

func TestToolStats(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "full")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

	disc := &DotfilesDiscoveryModule{reg: reg}
	tools := disc.Tools()

	var statsTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_tool_stats" {
			statsTool = td
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := statsTool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out dotfilesToolStatsOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TotalTools == 0 {
		t.Error("expected non-zero total tools")
	}
	if out.ModuleCount == 0 {
		t.Error("expected non-zero module count")
	}
	if out.ResourceCount != 0 {
		t.Errorf("expected resource count 0 without registries, got %d", out.ResourceCount)
	}
	if out.PromptCount != 0 {
		t.Errorf("expected prompt count 0 without registries, got %d", out.PromptCount)
	}
}

// ---------------------------------------------------------------------------
// Tool schema — tool not found
// ---------------------------------------------------------------------------

func TestToolSchema_NotFound(t *testing.T) {
	reg := registry.NewToolRegistry()
	disc := &DotfilesDiscoveryModule{reg: reg}
	tools := disc.Tools()

	var schemaTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_tool_schema" {
			schemaTool = td
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "nonexistent_tool_xyz"}
	result, err := schemaTool.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for nonexistent tool")
	}
}

// ---------------------------------------------------------------------------
// Tool catalog — with category filter
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Description (coverage for DotfilesDiscoveryModule.Description)
// ---------------------------------------------------------------------------

func TestDiscoveryModuleDescription(t *testing.T) {
	reg := registry.NewToolRegistry()
	m := &DotfilesDiscoveryModule{reg: reg, version: dotfilesMCPVersion}
	desc := m.Description()
	if desc == "" {
		t.Error("expected non-empty description")
	}
}

func TestServerHealth_WithSurfaceRegistries(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "full")

	reg := registry.NewToolRegistry()
	promptReg := buildDotfilesPromptRegistry()
	resReg := buildDotfilesResourceRegistry(reg, promptReg)
	registerDotfilesModules(reg, resReg, promptReg, dotfilesMCPVersion)

	disc := &DotfilesDiscoveryModule{
		reg:       reg,
		resources: resReg,
		prompts:   promptReg,
		version:   dotfilesMCPVersion,
	}
	tools := disc.Tools()

	var healthTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_server_health" {
			healthTool = td
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := healthTool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out dotfilesServerHealthOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "ok" {
		t.Fatalf("expected ok status, got %q", out.Status)
	}
	if out.ResourceCount == 0 {
		t.Fatal("expected non-zero resource count")
	}
	if out.PromptCount == 0 {
		t.Fatal("expected non-zero prompt count")
	}
	if out.WorkflowCount != 9 {
		t.Fatalf("expected 9 workflows, got %d", out.WorkflowCount)
	}
	if out.SkillCount != 5 {
		t.Fatalf("expected 5 skills, got %d", out.SkillCount)
	}
	prioritySummary, ok := out.PrioritySummary.(map[string]any)
	if !ok {
		t.Fatalf("expected priority summary map, got %T", out.PrioritySummary)
	}
	if got, ok := prioritySummary["missing_front_door_count"].(float64); !ok || int(got) != 0 {
		t.Fatalf("expected zero missing front doors in priority summary, got %#v", prioritySummary["missing_front_door_count"])
	}
	if len(out.DiscoveryTools) != 6 {
		t.Fatalf("expected 6 discovery tools, got %d", len(out.DiscoveryTools))
	}
}

// ---------------------------------------------------------------------------
// toolMetaMap
// ---------------------------------------------------------------------------

func TestToolMetaMap_NilMeta(t *testing.T) {
	tool := registry.Tool{Name: "test"}
	got := toolMetaMap(tool)
	if got != nil {
		t.Errorf("expected nil for nil Meta, got %v", got)
	}
}

func TestToolMetaMap_WithMeta(t *testing.T) {
	meta := mcp.NewMetaFromMap(map[string]any{
		"category": "discovery",
		"deferred": true,
	})
	tool := registry.Tool{
		Name: "test",
		Meta: meta,
	}
	got := toolMetaMap(tool)
	if got == nil {
		t.Fatal("expected non-nil map")
	}
	if got["category"] != "discovery" {
		t.Errorf("category = %v, want discovery", got["category"])
	}
	if got["deferred"] != true {
		t.Errorf("deferred = %v, want true", got["deferred"])
	}
}

// ---------------------------------------------------------------------------
// Tool search — integration
// ---------------------------------------------------------------------------

func TestToolSearch_Integration(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "full")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

	disc := &DotfilesDiscoveryModule{reg: reg}
	tools := disc.Tools()

	var searchTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_tool_search" {
			searchTool = td
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"query": "shader"}
	result, err := searchTool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out dotfilesToolSearchOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Total == 0 {
		t.Error("expected at least 1 result for 'shader' query")
	}
}

// ---------------------------------------------------------------------------
// Tool catalog — no filter (all categories)
// ---------------------------------------------------------------------------

func TestToolCatalog_AllCategories(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "full")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

	disc := &DotfilesDiscoveryModule{reg: reg}
	tools := disc.Tools()

	var catalogTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_tool_catalog" {
			catalogTool = td
			break
		}
	}

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := catalogTool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out dotfilesToolCatalogOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Groups) == 0 {
		t.Error("expected at least 1 category group when no filter applied")
	}
}

func TestToolCatalog_WithFilter(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "full")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

	disc := &DotfilesDiscoveryModule{reg: reg}
	tools := disc.Tools()

	var catalogTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_tool_catalog" {
			catalogTool = td
			break
		}
	}

	// With a non-matching category filter, should return empty groups.
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"category": "nonexistent_category_xyz"}
	result, err := catalogTool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out dotfilesToolCatalogOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Groups) != 0 {
		t.Errorf("expected 0 groups for nonexistent category, got %d", len(out.Groups))
	}
}
