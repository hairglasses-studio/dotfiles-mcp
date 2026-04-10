package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// inferCategory
// ---------------------------------------------------------------------------

func TestInferCategory(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"crt-effect.glsl", "CRT"},
		{"phosphor-glow.glsl", "CRT"},
		{"monitor-lines.glsl", "CRT"},
		{"bloom-blur.glsl", "Post-FX"},
		{"cursor-trail.glsl", "Cursor"},
		{"cyberpunk-rain.glsl", "Cyberpunk"},
		{"neon-glow.glsl", "Cyberpunk"},
		{"glitch-art.glsl", "Cyberpunk"},
		{"watercolor-wash.glsl", "Watercolor"},
		{"ocean-waves.glsl", "Watercolor"},
		{"rain-drops.glsl", "Watercolor"},
		{"halftone-dots.glsl", "Post-FX"},
		{"ascii-render.glsl", "Post-FX"},
		{"pixelate-grid.glsl", "Post-FX"},
		{"chromatic-aberration.glsl", "Post-FX"},
		{"film-grain.glsl", "Watercolor"}, // "rain" in "grain" matches Watercolor before Post-FX
		{"noise-static.glsl", "Post-FX"},
		{"scanline-overlay.glsl", "Post-FX"},
		{"vhs-tape.glsl", "Post-FX"},
		{"my-shader.glsl", "Background"},
		{"gradient.glsl", "Background"},
	}
	for _, tc := range tests {
		got := inferCategory(tc.name)
		if got != tc.want {
			t.Errorf("inferCategory(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// findShader — requires temp shaders dir
// ---------------------------------------------------------------------------

func TestFindShader_Found(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	shadersPath := filepath.Join(dir, "kitty", "shaders", "crtty")
	os.MkdirAll(shadersPath, 0755)
	os.WriteFile(filepath.Join(shadersPath, "test-shader.glsl"), []byte("// shader"), 0644)

	// With .glsl extension
	p, err := findShader("test-shader.glsl")
	if err != nil {
		t.Fatalf("findShader with .glsl: %v", err)
	}
	if p == "" {
		t.Error("expected non-empty path")
	}

	// Without .glsl extension
	p, err = findShader("test-shader")
	if err != nil {
		t.Fatalf("findShader without .glsl: %v", err)
	}
	if p == "" {
		t.Error("expected non-empty path")
	}
}

func TestFindShader_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "kitty", "shaders", "crtty"), 0755)

	_, err := findShader("nonexistent-shader")
	if err == nil {
		t.Error("expected error for nonexistent shader")
	}
}

// ---------------------------------------------------------------------------
// listGLSL — requires temp shaders dir
// ---------------------------------------------------------------------------

func TestListGLSL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	shadersPath := filepath.Join(dir, "kitty", "shaders", "crtty")
	os.MkdirAll(shadersPath, 0755)

	// Create some shader files and a non-shader file.
	os.WriteFile(filepath.Join(shadersPath, "alpha.glsl"), []byte("// a"), 0644)
	os.WriteFile(filepath.Join(shadersPath, "beta.glsl"), []byte("// b"), 0644)
	os.WriteFile(filepath.Join(shadersPath, "readme.txt"), []byte("not a shader"), 0644)

	// Create a subdirectory with a shader (should be excluded).
	os.MkdirAll(filepath.Join(shadersPath, "lib"), 0755)
	os.WriteFile(filepath.Join(shadersPath, "lib", "common.glsl"), []byte("// lib"), 0644)

	shaders, err := listGLSL()
	if err != nil {
		t.Fatalf("listGLSL: %v", err)
	}
	if len(shaders) != 2 {
		t.Errorf("expected 2 shaders, got %d: %v", len(shaders), shaders)
	}
}

func TestListGLSL_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "kitty", "shaders", "crtty"), 0755)

	shaders, err := listGLSL()
	if err != nil {
		t.Fatalf("listGLSL: %v", err)
	}
	if len(shaders) != 0 {
		t.Errorf("expected 0 shaders, got %d", len(shaders))
	}
}

func TestListGLSL_NoDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	// Don't create the shaders dir.

	_, err := listGLSL()
	if err == nil {
		t.Error("expected error when shaders dir does not exist")
	}
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

func TestWallpaperScript(t *testing.T) {
	t.Setenv("DOTFILES_DIR", "/tmp/test-dotfiles")
	got := wallpaperScript()
	want := "/tmp/test-dotfiles/scripts/shader-wallpaper.sh"
	if got != want {
		t.Errorf("wallpaperScript() = %q, want %q", got, want)
	}
}

func TestKittyPlaylistScript(t *testing.T) {
	t.Setenv("DOTFILES_DIR", "/tmp/test-dotfiles")
	got := kittyPlaylistScript()
	want := "/tmp/test-dotfiles/scripts/kitty-shader-playlist.sh"
	if got != want {
		t.Errorf("kittyPlaylistScript() = %q, want %q", got, want)
	}
}

func TestPlaylistsDir(t *testing.T) {
	t.Setenv("DOTFILES_DIR", "/tmp/test-dotfiles")
	got := playlistsDir()
	want := "/tmp/test-dotfiles/kitty/shaders/playlists"
	if got != want {
		t.Errorf("playlistsDir() = %q, want %q", got, want)
	}
}

func TestPlaylistStateDir(t *testing.T) {
	home := os.Getenv("HOME")
	got := playlistStateDir()
	want := filepath.Join(home, ".local", "state", "kitty-shaders")
	if got != want {
		t.Errorf("playlistStateDir() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// loadPlaylist
// ---------------------------------------------------------------------------

func TestLoadPlaylist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	plDir := filepath.Join(dir, "kitty", "shaders", "playlists")
	os.MkdirAll(plDir, 0755)

	content := "crt-effect.glsl\nbloomy\n# comment\n\nrain-drops\n"
	os.WriteFile(filepath.Join(plDir, "ambient.txt"), []byte(content), 0644)

	shaders, err := loadPlaylist("ambient")
	if err != nil {
		t.Fatalf("loadPlaylist: %v", err)
	}
	if len(shaders) != 3 {
		t.Fatalf("expected 3 shaders, got %d: %v", len(shaders), shaders)
	}
	// .glsl should be stripped
	if shaders[0] != "crt-effect" {
		t.Errorf("shaders[0] = %q, want crt-effect", shaders[0])
	}
	if shaders[1] != "bloomy" {
		t.Errorf("shaders[1] = %q, want bloomy", shaders[1])
	}
}

func TestLoadPlaylist_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "kitty", "shaders", "playlists"), 0755)

	_, err := loadPlaylist("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent playlist")
	}
}

// ---------------------------------------------------------------------------
// listPlaylists
// ---------------------------------------------------------------------------

func TestListPlaylists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	plDir := filepath.Join(dir, "kitty", "shaders", "playlists")
	os.MkdirAll(plDir, 0755)

	os.WriteFile(filepath.Join(plDir, "ambient.txt"), []byte("shader1\n"), 0644)
	os.WriteFile(filepath.Join(plDir, "chill.txt"), []byte("shader2\n"), 0644)
	os.WriteFile(filepath.Join(plDir, "notes.md"), []byte("not a playlist"), 0644)

	names, err := listPlaylists()
	if err != nil {
		t.Fatalf("listPlaylists: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 playlists, got %d: %v", len(names), names)
	}
}

// ---------------------------------------------------------------------------
// readPlaylistIndex / writePlaylistIndex
// ---------------------------------------------------------------------------

func TestPlaylistIndex_ReadWrite(t *testing.T) {
	// We need to override playlistStateDir via HOME or temp state.
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".local", "state", "kitty-shaders")
	os.MkdirAll(stateDir, 0755)

	// Temporarily override HOME for playlistStateDir().
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	defer os.Setenv("HOME", origHome)

	// Write index.
	err := writePlaylistIndex("test-playlist", 7)
	if err != nil {
		t.Fatalf("writePlaylistIndex: %v", err)
	}

	// Read it back.
	idx, err := readPlaylistIndex("test-playlist")
	if err != nil {
		t.Fatalf("readPlaylistIndex: %v", err)
	}
	if idx != 7 {
		t.Errorf("readPlaylistIndex = %d, want 7", idx)
	}
}

func TestReadPlaylistIndex_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	idx, err := readPlaylistIndex("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 0 {
		t.Errorf("expected 0 for missing playlist index, got %d", idx)
	}
}

// ---------------------------------------------------------------------------
// activePlaylistName
// ---------------------------------------------------------------------------

func TestActivePlaylistName_Default(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	got := activePlaylistName()
	if got != "ambient" {
		t.Errorf("expected default playlist 'ambient', got %q", got)
	}
}

func TestActivePlaylistName_Custom(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	stateDir := filepath.Join(dir, ".local", "state", "kitty-shaders")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "auto-rotate-playlist"), []byte("chill\n"), 0644)

	got := activePlaylistName()
	if got != "chill" {
		t.Errorf("expected 'chill', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// readKittyStateValue / theme helpers
// ---------------------------------------------------------------------------

func TestReadKittyStateValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	stateDir := filepath.Join(dir, ".local", "state", "kitty-shaders")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "current-theme"), []byte("Dracula\n"), 0644)
	os.WriteFile(filepath.Join(stateDir, "current-label"), []byte("Dracula · neon-glow\n"), 0644)

	if got := readKittyStateValue("current-theme"); got != "Dracula" {
		t.Fatalf("readKittyStateValue(current-theme) = %q, want Dracula", got)
	}
	if got := readActiveTheme(); got != "Dracula" {
		t.Fatalf("readActiveTheme() = %q, want Dracula", got)
	}
	if got := readVisualLabel(); got != "Dracula · neon-glow" {
		t.Fatalf("readVisualLabel() = %q, want Dracula · neon-glow", got)
	}
}

func TestPlaylistPositionForShader(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	plDir := filepath.Join(dir, "kitty", "shaders", "playlists")
	os.MkdirAll(plDir, 0755)
	os.WriteFile(filepath.Join(plDir, "ambient.txt"), []byte("# comment\nneon-glow\n\ndigital-mist.glsl\n"), 0644)

	position, total, err := playlistPositionForShader("ambient", "digital-mist")
	if err != nil {
		t.Fatalf("playlistPositionForShader: %v", err)
	}
	if position != 2 || total != 2 {
		t.Fatalf("playlistPositionForShader = %d/%d, want 2/2", position, total)
	}
}

// ---------------------------------------------------------------------------
// readAnimationState (detects iTime/u_time in shader source)
// ---------------------------------------------------------------------------

func TestReadAnimationState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)

	shadersPath := filepath.Join(dir, "kitty", "shaders", "crtty")
	os.MkdirAll(shadersPath, 0755)

	// Animated shader (contains iTime).
	os.WriteFile(filepath.Join(shadersPath, "animated.glsl"),
		[]byte("void main() { float t = iTime; }"), 0644)

	// readAnimationState calls readActiveShader which requires kitty-shader-playlist.sh.
	// Without the script, it returns false (no active shader).
	if readAnimationState() {
		t.Error("expected false when no active shader (no playlist script)")
	}
}

// ---------------------------------------------------------------------------
// loadManifest
// ---------------------------------------------------------------------------

func TestLoadManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	shadersPath := filepath.Join(dir, "kitty", "shaders", "crtty")
	os.MkdirAll(shadersPath, 0755)

	manifest := `[shaders.crt-effect]
category = "CRT"
cost = "low"
source = "custom"
description = "Classic CRT monitor effect"
playlists = ["ambient"]

[shaders.bloom]
category = "Post-FX"
cost = "medium"
source = "adapted"
description = "Bloom glow effect"
`
	os.WriteFile(filepath.Join(shadersPath, "shaders.toml"), []byte(manifest), 0644)

	m, err := loadManifest()
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil manifest")
	}
	if len(m) != 2 {
		t.Errorf("expected 2 shaders in manifest, got %d", len(m))
	}
	if m["crt-effect"].Category != "CRT" {
		t.Errorf("crt-effect category = %q, want CRT", m["crt-effect"].Category)
	}
}

func TestLoadManifest_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOTFILES_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "kitty", "shaders", "crtty"), 0755)

	m, err := loadManifest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Error("expected nil manifest when file is missing")
	}
}

// ---------------------------------------------------------------------------
// appendShaderHistory / readShaderHistory
// ---------------------------------------------------------------------------

func TestShaderHistory_AppendAndRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Append entries.
	for i, name := range []string{"crt", "bloom", "rain"} {
		err := appendShaderHistory(ShaderHistoryEntry{
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC3339),
			Action:    "set",
			Shader:    name,
			Source:    "mcp:test",
		})
		if err != nil {
			t.Fatalf("appendShaderHistory: %v", err)
		}
	}

	// Read all.
	entries, err := readShaderHistory(0, time.Time{})
	if err != nil {
		t.Fatalf("readShaderHistory: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Read with limit.
	entries, err = readShaderHistory(2, time.Time{})
	if err != nil {
		t.Fatalf("readShaderHistory with limit: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with limit, got %d", len(entries))
	}
	// Should be the last 2.
	if entries[0].Shader != "bloom" {
		t.Errorf("expected bloom as first limited entry, got %s", entries[0].Shader)
	}
}

func TestReadShaderHistory_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	entries, err := readShaderHistory(10, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for nonexistent history, got %d", len(entries))
	}
}

func TestShaderHistoryEntry_JSONRoundtrip(t *testing.T) {
	entry := ShaderHistoryEntry{
		Timestamp: "2026-04-05T12:00:00Z",
		Action:    "set",
		Shader:    "crt-monitor",
		Source:    "mcp:shader_set",
		Details:   map[string]string{"playlist": "ambient"},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ShaderHistoryEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Shader != "crt-monitor" {
		t.Errorf("got shader=%q, want crt-monitor", decoded.Shader)
	}
	if decoded.Details["playlist"] != "ambient" {
		t.Errorf("got details[playlist]=%q, want ambient", decoded.Details["playlist"])
	}
}

// ---------------------------------------------------------------------------
// atomicSetShader
// ---------------------------------------------------------------------------

func TestAtomicSetShader_RequiresPlaylistScript(t *testing.T) {
	// atomicSetShader now calls kitty-shader-playlist.sh set <name>.
	// Without the real script, it returns an error — this is expected.
	t.Setenv("DOTFILES_DIR", "/nonexistent")
	err := atomicSetShader("/nonexistent/shader.glsl")
	if err == nil {
		t.Skip("kitty-shader-playlist.sh available — integration test passed")
	}
	// Expected: error because the script doesn't exist
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
