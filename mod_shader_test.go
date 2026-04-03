package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// ShaderModule Registration
// ---------------------------------------------------------------------------

func TestShaderModuleRegistration(t *testing.T) {
	m := &ShaderModule{}
	tools := m.Tools()
	if len(tools) != 13 {
		t.Fatalf("expected 13 shader tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"shader_list", "shader_set", "shader_cycle", "shader_random",
		"shader_status", "shader_meta", "shader_test", "shader_build",
		"shader_playlist", "shader_get_state",
		"wallpaper_set", "wallpaper_random", "wallpaper_list",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Shader path helpers
// ---------------------------------------------------------------------------

func TestShadersDir(t *testing.T) {
	// With DOTFILES_DIR set
	t.Setenv("DOTFILES_DIR", "/tmp/test-dotfiles")
	got := shadersDir()
	want := "/tmp/test-dotfiles/ghostty/shaders"
	if got != want {
		t.Errorf("shadersDir() = %q, want %q", got, want)
	}
}

func TestWallpaperShadersDir(t *testing.T) {
	t.Setenv("DOTFILES_DIR", "/tmp/test-dotfiles")
	got := wallpaperShadersDir()
	want := "/tmp/test-dotfiles/wallpaper-shaders"
	if got != want {
		t.Errorf("wallpaperShadersDir() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Shader list (integration — needs shaders dir)
// ---------------------------------------------------------------------------

func TestShaderList(t *testing.T) {
	dir := shadersDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("shaders dir not found: %s", dir)
	}

	m := &ShaderModule{}
	td := findShaderTool(t, m, "shader_list")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := shaderExtractText(t, result)
	var out ShaderListOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Count == 0 {
		t.Error("expected at least 1 shader")
	}
}

// ---------------------------------------------------------------------------
// Shader status
// ---------------------------------------------------------------------------

func TestShaderStatus(t *testing.T) {
	dir := shadersDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("shaders dir not found: %s", dir)
	}

	m := &ShaderModule{}
	td := findShaderTool(t, m, "shader_status")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := shaderExtractText(t, result)
	var out ShaderStatusOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ActiveShader == "" {
		t.Log("no active shader (ok if none set)")
	}
}

// ---------------------------------------------------------------------------
// Shader meta query
// ---------------------------------------------------------------------------

func TestShaderMeta_KnownShader(t *testing.T) {
	dir := shadersDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("shaders dir not found: %s", dir)
	}

	// First get a shader name from the list
	m := &ShaderModule{}
	listTd := findShaderTool(t, m, "shader_list")
	listReq := registry.CallToolRequest{}
	listReq.Params.Arguments = map[string]any{}
	listResult, err := listTd.Handler(context.Background(), listReq)
	if err != nil {
		t.Skipf("shader_list failed: %v", err)
	}

	var listOut ShaderListOutput
	if err := json.Unmarshal([]byte(shaderExtractText(t, listResult)), &listOut); err != nil {
		t.Skipf("unmarshal list: %v", err)
	}
	if len(listOut.Shaders) == 0 {
		t.Skip("no shaders available")
	}

	// Query meta for the first shader
	shaderName := listOut.Shaders[0].Name
	td := findShaderTool(t, m, "shader_meta")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": shaderName}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := shaderExtractText(t, result)
	var out ShaderMetaOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Name == "" {
		t.Error("shader name is empty")
	}
}

// ---------------------------------------------------------------------------
// Shader get_state
// ---------------------------------------------------------------------------

func TestShaderGetState(t *testing.T) {
	m := &ShaderModule{}
	td := findShaderTool(t, m, "shader_get_state")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := shaderExtractText(t, result)
	var out ShaderGetStateOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Active shader may be empty if none set, but should not error
}

// ---------------------------------------------------------------------------
// Wallpaper list
// ---------------------------------------------------------------------------

func TestWallpaperList(t *testing.T) {
	dir := wallpaperShadersDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("wallpaper shaders dir not found: %s", dir)
	}

	m := &ShaderModule{}
	td := findShaderTool(t, m, "wallpaper_list")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := shaderExtractText(t, result)
	var out WallpaperListOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func findShaderTool(t *testing.T, m *ShaderModule, name string) registry.ToolDefinition {
	t.Helper()
	for _, td := range m.Tools() {
		if td.Tool.Name == name {
			return td
		}
	}
	t.Fatalf("tool %q not found", name)
	return registry.ToolDefinition{}
}

func shaderExtractText(t *testing.T, result *registry.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(registry.TextContent)
	if !ok {
		t.Fatalf("content is not TextContent, got %T", result.Content[0])
	}
	return tc.Text
}
