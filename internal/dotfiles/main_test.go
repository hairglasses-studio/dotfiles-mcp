package dotfiles

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestModuleRegistration(t *testing.T) {
	m := &DotfilesModule{}

	tools := m.Tools()
	if len(tools) != 32 {
		t.Fatalf("expected 32 tools, got %d", len(tools))
	}

	// Verify no panics by registering with mcptest server.
	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	names := srv.ToolNames()
	if len(names) != 32 {
		t.Fatalf("expected 32 registered tools, got %d", len(names))
	}

	// Spot-check a few tool names.
	for _, want := range []string{
		"dotfiles_validate_config",
		"dotfiles_rice_check",
		"dotfiles_list_configs",
		"dotfiles_check_symlinks",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestRegisterDotfilesModules_DefaultProfile(t *testing.T) {
	t.Setenv("DOTFILES_MCP_PROFILE", "default")

	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)

	if !reg.IsDeferred("dotfiles_validate_config") {
		t.Fatal("expected dotfiles_validate_config to be deferred in default profile")
	}
	if reg.IsDeferred("dotfiles_tool_search") {
		t.Fatal("expected discovery tools to stay eager")
	}
	if _, ok := reg.GetTool("dotfiles_tool_catalog"); !ok {
		t.Fatal("expected discovery tool dotfiles_tool_catalog to be registered")
	}
}

func TestValidateConfig_ValidTOML(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_validate_config")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": "[section]\nkey = \"value\"\n",
		"format":  "toml",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	out := unmarshalValidateOutput(t, result)
	if !out.Valid {
		t.Errorf("expected valid=true, got false; error=%s", out.Error)
	}
}

func TestValidateConfig_InvalidTOML(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_validate_config")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": "[section\nkey = broken",
		"format":  "toml",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	out := unmarshalValidateOutput(t, result)
	if out.Valid {
		t.Error("expected valid=false for broken TOML")
	}
	if out.Error == "" {
		t.Error("expected non-empty error for broken TOML")
	}
}

func TestValidateConfig_ValidJSON(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_validate_config")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": `{"key": "value", "nested": {"a": 1}}`,
		"format":  "json",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	out := unmarshalValidateOutput(t, result)
	if !out.Valid {
		t.Errorf("expected valid=true for valid JSON, got false; error=%s", out.Error)
	}
}

func TestRiceCheck_Quick(t *testing.T) {
	m := &DotfilesModule{}
	td := findTool(t, m, "dotfiles_rice_check")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"level": "quick",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}

	// Parse the text content as RiceCheckOutput.
	text := extractText(t, result)
	var out RiceCheckOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal rice check output: %v", err)
	}

	if out.Compositor == "" {
		t.Error("compositor field is empty")
	}
	if out.Shader == "" {
		t.Error("shader field is empty")
	}
	if out.Services == nil {
		t.Error("services field is nil")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func findTool(t *testing.T, m *DotfilesModule, name string) registry.ToolDefinition {
	t.Helper()
	for _, td := range m.Tools() {
		if td.Tool.Name == name {
			return td
		}
	}
	t.Fatalf("tool %q not found", name)
	return registry.ToolDefinition{} // unreachable
}

func extractText(t *testing.T, result *registry.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(registry.TextContent)
	if !ok {
		t.Fatalf("content is not TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

func unmarshalValidateOutput(t *testing.T, result *registry.CallToolResult) ValidateConfigOutput {
	t.Helper()
	text := extractText(t, result)
	var out ValidateConfigOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal validate output: %v; text=%s", err, text)
	}
	return out
}
