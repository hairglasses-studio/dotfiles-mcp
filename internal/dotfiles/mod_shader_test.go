package dotfiles

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	if len(tools) != 21 {
		t.Fatalf("expected 21 shader tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"shader_list", "shader_set", "shader_cycle", "shader_random",
		"shader_status", "shader_meta", "shader_test", "shader_build",
		"shader_playlist", "shader_get_state",
		"wallpaper_set", "wallpaper_random", "wallpaper_list",
		"shader_hot_reload", "shader_diff", "shader_log",
		"shader_preview", "shader_audit_trail",
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
	want := "/tmp/test-dotfiles/kitty/shaders/crtty"
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
	dir := setupFakeKittyPlaylistEnv(t)
	stateDir := filepath.Join(dir, ".local", "state", "kitty-shaders")
	os.WriteFile(filepath.Join(stateDir, "current"), []byte("digital-mist"), 0644)
	os.WriteFile(filepath.Join(stateDir, "current-theme"), []byte("Dracula"), 0644)
	os.WriteFile(filepath.Join(stateDir, "current-label"), []byte("Dracula · digital-mist"), 0644)
	os.WriteFile(filepath.Join(stateDir, "auto-rotate-playlist"), []byte("ambient"), 0644)

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
	if out.ShaderName != "digital-mist" {
		t.Fatalf("shader_get_state shader = %q, want digital-mist", out.ShaderName)
	}
	if out.ActiveTheme != "Dracula" {
		t.Fatalf("shader_get_state theme = %q, want Dracula", out.ActiveTheme)
	}
	if out.VisualLabel != "Dracula · digital-mist" {
		t.Fatalf("shader_get_state label = %q, want Dracula · digital-mist", out.VisualLabel)
	}
	if out.Playlist != "ambient" {
		t.Fatalf("shader_get_state playlist = %q, want ambient", out.Playlist)
	}
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

func setupFakeKittyPlaylistEnv(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	t.Setenv("HOME", dir)

	shadersDir := filepath.Join(dir, "kitty", "shaders", "crtty")
	playlistsDir := filepath.Join(dir, "kitty", "shaders", "playlists")
	themePlaylistsDir := filepath.Join(dir, "kitty", "themes", "playlists")
	stateDir := filepath.Join(dir, ".local", "state", "kitty-shaders")
	scriptDir := filepath.Join(dir, "scripts")

	for _, path := range []string{shadersDir, playlistsDir, themePlaylistsDir, stateDir, scriptDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	for _, shader := range []string{"digital-mist", "neon-glow"} {
		if err := os.WriteFile(filepath.Join(shadersDir, shader+".glsl"), []byte("// "+shader), 0o644); err != nil {
			t.Fatalf("write shader %s: %v", shader, err)
		}
	}
	if err := os.WriteFile(filepath.Join(playlistsDir, "ambient.txt"), []byte("digital-mist\nneon-glow\n"), 0o644); err != nil {
		t.Fatalf("write shader playlist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playlistsDir, "best-of.txt"), []byte("neon-glow\ndigital-mist\n"), 0o644); err != nil {
		t.Fatalf("write shader playlist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(themePlaylistsDir, "ambient.txt"), []byte("Dracula\nGruvbox Dark\n"), 0o644); err != nil {
		t.Fatalf("write theme playlist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(themePlaylistsDir, "best-of.txt"), []byte("Nord\nDracula\n"), 0o644); err != nil {
		t.Fatalf("write theme playlist: %v", err)
	}

	scriptPath := filepath.Join(scriptDir, "kitty-shader-playlist.sh")
	script := `#!/usr/bin/env bash
set -euo pipefail

state_dir="${HOME}/.local/state/kitty-shaders"
playlist_dir="${DOTFILES_DIR}/kitty/shaders/playlists"
theme_dir="${DOTFILES_DIR}/kitty/themes/playlists"
mkdir -p "$state_dir"

pick_first() {
  awk 'NF && $1 !~ /^#/{sub(/\.glsl$/, "", $1); print $1; exit}' "$1"
}

pick_first_theme() {
  awk 'NF && $1 !~ /^#/{print; exit}' "$1"
}

cmd="${1:-current}"
shift || true
case "$cmd" in
  current)
    [[ -f "$state_dir/current" ]] && cat "$state_dir/current"
    ;;
  set)
    shader="${1:?shader required}"
    theme="${2:-$(cat "$state_dir/current-theme" 2>/dev/null || true)}"
    [[ -n "$theme" ]] || theme="$(pick_first_theme "$theme_dir/ambient.txt")"
    printf '%s' "$shader" > "$state_dir/current"
    printf '%s' "$theme" > "$state_dir/current-theme"
    printf '%s' "$theme · $shader" > "$state_dir/current-label"
    printf '%s' "ambient" > "$state_dir/auto-rotate-playlist"
    printf '%s · %s\n' "$theme" "$shader"
    ;;
  next|prev|random)
    playlist="${1:-ambient}"
    shader="$(pick_first "$playlist_dir/${playlist}.txt")"
    theme="$(pick_first_theme "$theme_dir/${playlist}.txt")"
    printf '%s' "$shader" > "$state_dir/current"
    printf '%s' "$theme" > "$state_dir/current-theme"
    printf '%s' "$theme · $shader" > "$state_dir/current-label"
    printf '%s' "$playlist" > "$state_dir/auto-rotate-playlist"
    printf '%s · %s\n' "$theme" "$shader"
    ;;
  *)
    exit 1
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kitty-shader-playlist: %v", err)
	}

	return dir
}

func TestShaderCycleUsesKittyPlaylistState(t *testing.T) {
	setupFakeKittyPlaylistEnv(t)

	m := &ShaderModule{}
	td := findShaderTool(t, m, "shader_cycle")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"direction": "next",
		"playlist":  "ambient",
	}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out ShaderCycleOutput
	if err := json.Unmarshal([]byte(shaderExtractText(t, result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Applied != "digital-mist" {
		t.Fatalf("shader_cycle applied = %q, want digital-mist", out.Applied)
	}
	if out.Theme != "Dracula" {
		t.Fatalf("shader_cycle theme = %q, want Dracula", out.Theme)
	}
	if out.Label != "Dracula · digital-mist" {
		t.Fatalf("shader_cycle label = %q, want Dracula · digital-mist", out.Label)
	}
	if out.Playlist != "ambient" || out.Position != 1 || out.Total != 2 {
		t.Fatalf("shader_cycle playlist state = %q %d/%d, want ambient 1/2", out.Playlist, out.Position, out.Total)
	}
}

func TestShaderRandomUsesActivePlaylistState(t *testing.T) {
	dir := setupFakeKittyPlaylistEnv(t)
	stateDir := filepath.Join(dir, ".local", "state", "kitty-shaders")
	os.WriteFile(filepath.Join(stateDir, "auto-rotate-playlist"), []byte("ambient"), 0o644)

	m := &ShaderModule{}
	td := findShaderTool(t, m, "shader_random")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out ShaderRandomOutput
	if err := json.Unmarshal([]byte(shaderExtractText(t, result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Applied != "digital-mist" || out.Theme != "Dracula" || out.Playlist != "ambient" {
		t.Fatalf("shader_random = %+v, want digital-mist/Dracula/ambient", out)
	}
	if out.Position != 1 || out.Total != 2 {
		t.Fatalf("shader_random position = %d/%d, want 1/2", out.Position, out.Total)
	}
}

func TestShaderPlaylistRandomUsesNamedPlaylistState(t *testing.T) {
	setupFakeKittyPlaylistEnv(t)

	m := &ShaderModule{}
	td := findShaderTool(t, m, "shader_playlist")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":   "best-of",
		"action": "random",
	}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out ShaderPlaylistOutput
	if err := json.Unmarshal([]byte(shaderExtractText(t, result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Applied == nil {
		t.Fatal("expected applied playlist result")
	}
	if out.Applied.Applied != "neon-glow" || out.Applied.Theme != "Nord" || out.Applied.Playlist != "best-of" {
		t.Fatalf("shader_playlist random = %+v, want neon-glow/Nord/best-of", out.Applied)
	}
	if out.Applied.Position != 1 || out.Applied.Total != 2 {
		t.Fatalf("shader_playlist random position = %d/%d, want 1/2", out.Applied.Position, out.Applied.Total)
	}
}

func TestShaderStatusIncludesThemeAndLabel(t *testing.T) {
	dir := setupFakeKittyPlaylistEnv(t)
	stateDir := filepath.Join(dir, ".local", "state", "kitty-shaders")
	os.WriteFile(filepath.Join(stateDir, "current"), []byte("neon-glow"), 0o644)
	os.WriteFile(filepath.Join(stateDir, "current-theme"), []byte("Gruvbox Dark"), 0o644)
	os.WriteFile(filepath.Join(stateDir, "current-label"), []byte("Gruvbox Dark · neon-glow"), 0o644)
	os.WriteFile(filepath.Join(stateDir, "auto-rotate-playlist"), []byte("ambient"), 0o644)

	m := &ShaderModule{}
	td := findShaderTool(t, m, "shader_status")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out ShaderStatusOutput
	if err := json.Unmarshal([]byte(shaderExtractText(t, result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ShaderName != "neon-glow" {
		t.Fatalf("shader_status shader = %q, want neon-glow", out.ShaderName)
	}
	if out.ActiveTheme != "Gruvbox Dark" {
		t.Fatalf("shader_status theme = %q, want Gruvbox Dark", out.ActiveTheme)
	}
	if out.VisualLabel != "Gruvbox Dark · neon-glow" {
		t.Fatalf("shader_status label = %q, want Gruvbox Dark · neon-glow", out.VisualLabel)
	}
	if out.Position != 2 || out.Total != 2 {
		t.Fatalf("shader_status position = %d/%d, want 2/2", out.Position, out.Total)
	}
}
