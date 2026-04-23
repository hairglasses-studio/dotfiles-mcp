// mod_shader.go — Terminal shader pipeline tools (kitty via CRTty + Kitty themes)
package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/mcpkit/resources"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func shadersDir() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "kitty", "shaders", "crtty")
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "kitty", "shaders", "crtty")
}

func wallpaperShadersDir() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "wallpaper-shaders")
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "wallpaper-shaders")
}

func wallpaperScript() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "scripts", "shader-wallpaper.sh")
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "scripts", "shader-wallpaper.sh")
}

func kittyPlaylistScript() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "scripts", "kitty-shader-playlist.sh")
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "scripts", "kitty-shader-playlist.sh")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// listGLSL returns all .glsl files in the shaders directory (not in subdirs like lib/ or bin/).
func listGLSL() ([]string, error) {
	dir := shadersDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read shaders dir %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".glsl") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// inferCategory guesses a category from the shader filename.
func inferCategory(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "crt") || strings.Contains(lower, "monitor") || strings.Contains(lower, "phosphor"):
		return "CRT"
	case strings.Contains(lower, "bloom"):
		return "Post-FX"
	case strings.Contains(lower, "cursor"):
		return "Cursor"
	case strings.Contains(lower, "cyberpunk") || strings.Contains(lower, "neon") || strings.Contains(lower, "glitch"):
		return "Cyberpunk"
	case strings.Contains(lower, "water") || strings.Contains(lower, "ocean") || strings.Contains(lower, "rain"):
		return "Watercolor"
	case strings.Contains(lower, "halftone") || strings.Contains(lower, "ascii") || strings.Contains(lower, "pixelate") ||
		strings.Contains(lower, "chromatic") || strings.Contains(lower, "film") || strings.Contains(lower, "noise") ||
		strings.Contains(lower, "scanline") || strings.Contains(lower, "vhs"):
		return "Post-FX"
	default:
		return "Background"
	}
}

// findShader locates a shader by name (with or without .glsl extension).
func findShader(name string) (string, error) {
	if !strings.HasSuffix(name, ".glsl") {
		name += ".glsl"
	}
	p := filepath.Join(shadersDir(), name)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("shader not found: %s", name)
	}
	return p, nil
}

// atomicSetShader applies a shader via kitty-shader-playlist.sh set <name>.
// Logs the change to the JSONL history.
func atomicSetShader(shaderPath string, source ...string) error {
	name := strings.TrimSuffix(filepath.Base(shaderPath), ".glsl")
	if _, err := runKittyPlaylist("set", name); err != nil {
		return err
	}

	src := "mcp:shader_set"
	if len(source) > 0 && source[0] != "" {
		src = source[0]
	}
	_ = appendShaderHistory(ShaderHistoryEntry{
		Action: "set",
		Shader: name,
		Source: src,
	})
	return nil
}

func runKittyPlaylist(args ...string) (string, error) {
	cmdArgs := append([]string{kittyPlaylistScript()}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if output == "" {
			return "", fmt.Errorf("kitty-shader-playlist %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("kitty-shader-playlist %s: %s: %w", strings.Join(args, " "), output, err)
	}
	return output, nil
}

// readActiveShader reads the current shader from kitty-shader-playlist.sh current.
func readActiveShader() (string, error) {
	out, err := runKittyPlaylist("current")
	if err != nil {
		return "", nil
	}
	name := strings.TrimSpace(out)
	if name == "" {
		return "", nil
	}
	// Return the full path to the source GLSL for compatibility
	p := filepath.Join(shadersDir(), name+".glsl")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	return name, nil
}

func readKittyStateValue(name string) string {
	data, err := os.ReadFile(filepath.Join(playlistStateDir(), name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readActiveTheme() string {
	return readKittyStateValue("current-theme")
}

func readVisualLabel() string {
	return readKittyStateValue("current-label")
}

func playlistPositionForShader(playlist, shader string) (int, int, error) {
	if playlist == "" {
		return 0, 0, nil
	}
	shaders, err := loadPlaylist(playlist)
	if err != nil {
		return 0, 0, err
	}
	if len(shaders) == 0 {
		return 0, 0, nil
	}
	target := strings.TrimSuffix(filepath.Base(shader), ".glsl")
	for idx, entry := range shaders {
		if strings.TrimSuffix(filepath.Base(entry), ".glsl") == target {
			return idx + 1, len(shaders), nil
		}
	}
	return 0, len(shaders), nil
}

// ---------------------------------------------------------------------------
// Manifest
// ---------------------------------------------------------------------------

// ShaderManifest represents the parsed shaders.toml file.
type ShaderManifest struct {
	Shaders map[string]ShaderMeta `toml:"shaders"`
}

// ShaderMeta holds manifest metadata for a single shader.
type ShaderMeta struct {
	Category    string   `toml:"category" json:"category"`
	Cost        string   `toml:"cost" json:"cost"`
	Source      string   `toml:"source" json:"source"`
	Description string   `toml:"description" json:"description"`
	Playlists   []string `toml:"playlists" json:"playlists"`
}

// loadManifest reads and parses the shaders.toml manifest.
// Returns nil (not error) if the file doesn't exist so the server degrades gracefully.
func loadManifest() (map[string]ShaderMeta, error) {
	p := filepath.Join(shadersDir(), "shaders.toml")
	var m ShaderManifest
	if _, err := toml.DecodeFile(p, &m); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return m.Shaders, nil
}

// ---------------------------------------------------------------------------
// Playlists
// ---------------------------------------------------------------------------

func playlistsDir() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "kitty", "shaders", "playlists")
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "kitty", "shaders", "playlists")
}

func playlistStateDir() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "state", "kitty-shaders")
}

// activePlaylistName returns the currently configured auto-rotate playlist.
func activePlaylistName() string {
	p := filepath.Join(playlistStateDir(), "auto-rotate-playlist")
	data, err := os.ReadFile(p)
	if err != nil {
		return "ambient" // default
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "ambient"
	}
	return name
}

// readPlaylistIndex reads the current index for a playlist from its state file.
func readPlaylistIndex(playlist string) (int, error) {
	p := filepath.Join(playlistStateDir(), playlist+".idx")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	idx, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, nil
	}
	return idx, nil
}

// writePlaylistIndex writes the playlist index to its state file.
func writePlaylistIndex(playlist string, idx int) error {
	dir := playlistStateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, playlist+".idx"), []byte(strconv.Itoa(idx)), 0o644)
}

// readAnimationState detects whether the active shader uses time-based animation.
func readAnimationState() bool {
	active, err := readActiveShader()
	if err != nil || active == "" {
		return false
	}
	data, err := os.ReadFile(active)
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, "iTime") || strings.Contains(s, "u_time") || strings.Contains(s, "ghostty_time")
}

// listPlaylists returns available playlist names (without .txt extension).
func listPlaylists() ([]string, error) {
	entries, err := os.ReadDir(playlistsDir())
	if err != nil {
		return nil, fmt.Errorf("read playlists dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
			names = append(names, strings.TrimSuffix(e.Name(), ".txt"))
		}
	}
	return names, nil
}

// loadPlaylist reads a playlist file and returns shader names (without .glsl).
func loadPlaylist(name string) ([]string, error) {
	p := filepath.Join(playlistsDir(), name+".txt")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read playlist %s: %w", name, err)
	}
	var shaders []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		shaders = append(shaders, strings.TrimSuffix(line, ".glsl"))
	}
	return shaders, nil
}

// ---------------------------------------------------------------------------
// MCP Tool I/O types
// ---------------------------------------------------------------------------

type ShaderListInput struct {
	Category string `json:"category,omitempty" jsonschema:"description=Filter by category (CRT Post-FX Cursor Background Cyberpunk Watercolor)"`
}

type ShaderEntry struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Category    string   `json:"category"`
	Cost        string   `json:"cost,omitempty"`
	Description string   `json:"description,omitempty"`
	Source      string   `json:"source,omitempty"`
	Playlists   []string `json:"playlists,omitempty"`
}

type ShaderListOutput struct {
	Shaders []ShaderEntry `json:"shaders"`
	Count   int           `json:"count"`
}

type ShaderSetInput struct {
	Name string `json:"name" jsonschema:"required,description=Shader filename (with or without .glsl)"`
}

type ShaderSetOutput struct {
	Applied string `json:"applied"`
	Path    string `json:"path"`
	Theme   string `json:"theme,omitempty"`
	Label   string `json:"label,omitempty"`
}

type ShaderRandomInput struct{}

type ShaderRandomOutput struct {
	Applied  string `json:"applied"`
	Path     string `json:"path"`
	Theme    string `json:"theme,omitempty"`
	Label    string `json:"label,omitempty"`
	Playlist string `json:"playlist,omitempty"`
	Position int    `json:"position,omitempty"`
	Total    int    `json:"total,omitempty"`
}

type ShaderTestInput struct {
	Name string `json:"name,omitempty" jsonschema:"description=Shader to test (omit to test all)"`
}

type ShaderTestResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Output string `json:"output,omitempty"`
}

type ShaderTestOutput struct {
	Results []ShaderTestResult `json:"results"`
	Passed  int                `json:"passed"`
	Failed  int                `json:"failed"`
}

type ShaderBenchmarkInput struct {
	Name string `json:"name,omitempty" jsonschema:"description=Shader to benchmark (omit to benchmark all)"`
}

type ShaderBenchResult struct {
	Name      string  `json:"name"`
	CompileMs float64 `json:"compile_ms"`
	SizeBytes int64   `json:"size_bytes"`
	Category  string  `json:"category"`
	Passed    bool    `json:"passed"`
}

type ShaderBenchmarkOutput struct {
	Results   []ShaderBenchResult `json:"results"`
	Count     int                 `json:"count"`
	AvgMs     float64             `json:"avg_compile_ms"`
	MaxMs     float64             `json:"max_compile_ms"`
	MaxShader string              `json:"max_shader"`
	TotalKB   float64             `json:"total_kb"`
}

type ShaderGetStateInput struct{}

type ShaderGetStateOutput struct {
	ActiveShader string `json:"active_shader"`
	ShaderName   string `json:"shader_name"`
	ActiveTheme  string `json:"active_theme,omitempty"`
	VisualLabel  string `json:"visual_label,omitempty"`
	Playlist     string `json:"playlist,omitempty"`
}

type ShaderMetaInput struct {
	Name string `json:"name" jsonschema:"required,description=Shader name (with or without .glsl)"`
}

type ShaderMetaOutput struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Category    string   `json:"category"`
	Cost        string   `json:"cost"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	Playlists   []string `json:"playlists"`
	InManifest  bool     `json:"in_manifest"`
}

// Wallpaper shader types

type WallpaperListInput struct{}

type WallpaperEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type WallpaperListOutput struct {
	Shaders []WallpaperEntry `json:"shaders"`
	Count   int              `json:"count"`
}

type WallpaperSetInput struct {
	Name string `json:"name" jsonschema:"required,description=Wallpaper shader name (with or without .frag)"`
}

type WallpaperSetOutput struct {
	Applied string `json:"applied"`
	Output  string `json:"output,omitempty"`
}

type WallpaperRandomInput struct{}

type WallpaperRandomOutput struct {
	Applied string `json:"applied"`
	Output  string `json:"output,omitempty"`
}

type ShaderCycleInput struct {
	Direction string `json:"direction,omitempty" jsonschema:"description=Direction: next or prev (default: next),enum=next,enum=prev"`
	Playlist  string `json:"playlist,omitempty" jsonschema:"description=Playlist name (default: current active playlist)"`
}

type ShaderCycleOutput struct {
	Applied  string `json:"applied"`
	Path     string `json:"path"`
	Theme    string `json:"theme,omitempty"`
	Label    string `json:"label,omitempty"`
	Playlist string `json:"playlist"`
	Position int    `json:"position"`
	Total    int    `json:"total"`
}

type ShaderStatusInput struct{}

type ShaderStatusOutput struct {
	ActiveShader string `json:"active_shader"`
	ShaderName   string `json:"shader_name"`
	ActiveTheme  string `json:"active_theme,omitempty"`
	VisualLabel  string `json:"visual_label,omitempty"`
	Animation    bool   `json:"animation"`
	Playlist     string `json:"playlist,omitempty"`
	Position     int    `json:"position,omitempty"`
	Total        int    `json:"total,omitempty"`
	AutoRotate   bool   `json:"auto_rotate"`
}

type ShaderBuildInput struct {
	Shader string `json:"shader,omitempty" jsonschema:"description=Specific shader to test (default: test all)"`
}

type ShaderBuildResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Output string `json:"output,omitempty"`
	Errors string `json:"errors,omitempty"`
}

type ShaderBuildOutput struct {
	Results []ShaderBuildResult `json:"results"`
	Passed  int                 `json:"passed"`
	Failed  int                 `json:"failed"`
	Summary string              `json:"summary"`
}

type ShaderPlaylistInput struct {
	Name   string `json:"name,omitempty" jsonschema:"description=Playlist name (omit to list all playlists)"`
	Action string `json:"action,omitempty" jsonschema:"description=Action: list (default) or random (pick and apply a random shader from the playlist)"`
}

type PlaylistInfo struct {
	Name    string   `json:"name"`
	Count   int      `json:"count"`
	Shaders []string `json:"shaders,omitempty"`
}

type ShaderPlaylistOutput struct {
	Playlists []PlaylistInfo      `json:"playlists,omitempty"`
	Applied   *ShaderRandomOutput `json:"applied,omitempty"`
}

// ---------------------------------------------------------------------------
// History (JSONL append-only log)
// ---------------------------------------------------------------------------

// ShaderHistoryEntry is a single entry in the shader change log.
type ShaderHistoryEntry struct {
	Timestamp string            `json:"timestamp"`
	Action    string            `json:"action"` // set, cycle, random, preview, revert
	Shader    string            `json:"shader"`
	Source    string            `json:"source"` // mcp:shader_set, mcp:shader_cycle, etc.
	Details   map[string]string `json:"details,omitempty"`
}

func shaderHistoryPath() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "state", "kitty-shaders", "shader-history.jsonl")
}

func appendShaderHistory(entry ShaderHistoryEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	dir := filepath.Dir(shaderHistoryPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(shaderHistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(entry)
}

func readShaderHistory(limit int, since time.Time) ([]ShaderHistoryEntry, error) {
	data, err := os.ReadFile(shaderHistoryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var all []ShaderHistoryEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e ShaderHistoryEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if !since.IsZero() {
			t, err := time.Parse(time.RFC3339, e.Timestamp)
			if err != nil || t.Before(since) {
				continue
			}
		}
		all = append(all, e)
	}
	// Return most recent entries up to limit
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

// ---------------------------------------------------------------------------
// Presets
// ---------------------------------------------------------------------------

// presetsFilePath returns the path to kitty/shaders/presets.toml.
func presetsFilePath() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "kitty", "shaders", "presets.toml")
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "kitty", "shaders", "presets.toml")
}

// presetsTOML mirrors the TOML structure: shaders.<name>.presets.<preset>.
type presetsTOML struct {
	Shaders map[string]shaderPresetsBlock `toml:"shaders"`
}

type shaderPresetsBlock struct {
	Presets map[string]presetBlock `toml:"presets"`
}

type presetBlock struct {
	Description string            `toml:"description"`
	Params      map[string]string `toml:"params"`
}

// loadPresets reads and parses the presets TOML file.
func loadPresets() (presetsTOML, error) {
	var p presetsTOML
	path := presetsFilePath()
	if _, err := toml.DecodeFile(path, &p); err != nil {
		if os.IsNotExist(err) {
			return presetsTOML{}, fmt.Errorf("presets file not found: %s", path)
		}
		return presetsTOML{}, fmt.Errorf("parse presets: %w", err)
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// New tool I/O types
// ---------------------------------------------------------------------------

type ShaderHotReloadInput struct{}

type ShaderHotReloadOutput struct {
	Reloaded bool   `json:"reloaded"`
	Method   string `json:"method"` // "touch" or "sigusr1"
}

// Preset list / apply types

type ShaderPresetListInput struct {
	Shader string `json:"shader,omitempty" jsonschema:"description=Filter to a specific shader name (omit to list all presets)"`
}

type PresetEntry struct {
	Shader      string            `json:"shader"`
	Preset      string            `json:"preset"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
}

type ShaderPresetListOutput struct {
	Presets []PresetEntry `json:"presets"`
	Count   int           `json:"count"`
}

type ShaderPresetApplyInput struct {
	Shader string `json:"shader" jsonschema:"required,description=Shader name (with or without .glsl)"`
	Preset string `json:"preset" jsonschema:"required,description=Preset name to apply (e.g. default sharp heavy)"`
}

type ShaderPresetApplyOutput struct {
	Shader      string            `json:"shader"`
	Preset      string            `json:"preset"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
	Applied     bool              `json:"applied"`
	Note        string            `json:"note,omitempty"`
}

type ShaderDiffInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"description=Number of recent changes to compare (default: 2)"`
}

type DiffEntry struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type ShaderDiffOutput struct {
	Changes []DiffEntry `json:"changes"`
	Before  string      `json:"before"`
	After   string      `json:"after"`
}

type ShaderLogInput struct {
	Limit int    `json:"limit,omitempty" jsonschema:"description=Max entries to return (default: 20)"`
	Since string `json:"since,omitempty" jsonschema:"description=Only show entries after this RFC3339 timestamp"`
}

type ShaderLogEntry struct {
	Shader    string `json:"shader"`
	Action    string `json:"action"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
	Duration  string `json:"duration,omitempty"`
}

type ShaderLogOutput struct {
	Entries []ShaderLogEntry `json:"entries"`
	Count   int              `json:"count"`
}

type ShaderPreviewInput struct {
	Name     string `json:"name" jsonschema:"required,description=Shader to preview (with or without .glsl)"`
	Duration int    `json:"duration,omitempty" jsonschema:"description=Preview duration in seconds (default: 10)"`
}

type ShaderPreviewOutput struct {
	Previewing string `json:"previewing"`
	Duration   int    `json:"duration"`
	RevertTo   string `json:"revert_to"`
}

type ShaderAuditTrailInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"description=Max entries to return (default: 50)"`
	Format string `json:"format,omitempty" jsonschema:"description=Output format: json or text (default: json),enum=json,enum=text"`
}

type ShaderAuditTrailOutput struct {
	Entries []ShaderHistoryEntry `json:"entries,omitempty"`
	Text    string               `json:"text,omitempty"`
	Count   int                  `json:"count"`
}

type currentVisualState struct {
	ActiveShader string
	ShaderName   string
	ActiveTheme  string
	VisualLabel  string
	Playlist     string
	Position     int
	Total        int
}

func readCurrentVisualState() (currentVisualState, error) {
	state := currentVisualState{
		ActiveTheme: readActiveTheme(),
		VisualLabel: readVisualLabel(),
		Playlist:    activePlaylistName(),
	}
	active, err := readActiveShader()
	if err != nil {
		return state, err
	}
	state.ActiveShader = active
	state.ShaderName = strings.TrimSuffix(filepath.Base(active), ".glsl")
	if state.ShaderName != "" {
		if position, total, err := playlistPositionForShader(state.Playlist, state.ShaderName); err == nil {
			state.Position = position
			state.Total = total
		}
	}
	return state, nil
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// previewState tracks an in-progress shader preview for auto-revert.
type previewState struct {
	mu       sync.Mutex
	timer    *time.Timer
	original string // shader path to revert to
}

var preview = &previewState{}

type ShaderModule struct {
	manifest     map[string]ShaderMeta
	manifestOnce sync.Once
}

func (m *ShaderModule) Name() string        { return "shader" }
func (m *ShaderModule) Description() string { return "Terminal shader pipeline tools (kitty)" }

func (m *ShaderModule) getManifest() map[string]ShaderMeta {
	m.manifestOnce.Do(func() {
		m.manifest, _ = loadManifest()
	})
	return m.manifest
}

func (m *ShaderModule) resolveCategory(name string) string {
	if manifest := m.getManifest(); manifest != nil {
		if meta, ok := manifest[name]; ok {
			return meta.Category
		}
	}
	return inferCategory(name)
}
func (m *ShaderModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── shader_list ────────────────────────────────
		handler.TypedHandler[ShaderListInput, ShaderListOutput](
			"shader_list",
			"List all GLSL shaders in the source shaders directory. Optionally filter by category.",
			func(_ context.Context, input ShaderListInput) (ShaderListOutput, error) {
				files, err := listGLSL()
				if err != nil {
					return ShaderListOutput{}, err
				}
				dir := shadersDir()
				manifest := m.getManifest()
				var entries []ShaderEntry
				for _, f := range files {
					name := strings.TrimSuffix(f, ".glsl")
					cat := m.resolveCategory(name)
					if input.Category != "" && !strings.EqualFold(cat, input.Category) {
						continue
					}
					entry := ShaderEntry{
						Name:     name,
						Path:     filepath.Join(dir, f),
						Category: cat,
					}
					if manifest != nil {
						if meta, ok := manifest[name]; ok {
							entry.Cost = meta.Cost
							entry.Description = meta.Description
							entry.Source = meta.Source
							entry.Playlists = meta.Playlists
						}
					}
					entries = append(entries, entry)
				}
				return ShaderListOutput{Shaders: entries, Count: len(entries)}, nil
			},
		),

		// ── shader_set ────────────────────────────────
		handler.TypedHandler[ShaderSetInput, ShaderSetOutput](
			"shader_set",
			"Apply a CRTty shader to kitty via kitty-shader-playlist.sh.",
			func(_ context.Context, input ShaderSetInput) (ShaderSetOutput, error) {
				p, err := findShader(input.Name)
				if err != nil {
					return ShaderSetOutput{}, err
				}
				if err := atomicSetShader(p); err != nil {
					return ShaderSetOutput{}, fmt.Errorf("failed to apply shader: %w", err)
				}
				state, err := readCurrentVisualState()
				if err != nil {
					return ShaderSetOutput{}, err
				}
				return ShaderSetOutput{
					Applied: strings.TrimSuffix(filepath.Base(p), ".glsl"),
					Path:    p,
					Theme:   state.ActiveTheme,
					Label:   state.VisualLabel,
				}, nil
			},
		),

		// ── shader_random ─────────────────────────────
		handler.TypedHandler[ShaderRandomInput, ShaderRandomOutput](
			"shader_random",
			"Pick a random paired Kitty visual from the active playlist and apply it.",
			func(_ context.Context, _ ShaderRandomInput) (ShaderRandomOutput, error) {
				playlist := activePlaylistName()
				if _, err := runKittyPlaylist("random", playlist); err != nil {
					return ShaderRandomOutput{}, fmt.Errorf("failed to apply random kitty visual: %w", err)
				}
				state, err := readCurrentVisualState()
				if err != nil {
					return ShaderRandomOutput{}, err
				}
				_ = appendShaderHistory(ShaderHistoryEntry{
					Action: "random",
					Shader: state.ShaderName,
					Source: "mcp:shader_random",
					Details: map[string]string{
						"playlist": playlist,
						"theme":    state.ActiveTheme,
					},
				})
				return ShaderRandomOutput{
					Applied:  state.ShaderName,
					Path:     state.ActiveShader,
					Theme:    state.ActiveTheme,
					Label:    state.VisualLabel,
					Playlist: playlist,
					Position: state.Position,
					Total:    state.Total,
				}, nil
			},
		),

		// ── shader_test ───────────────────────────────
		handler.TypedHandler[ShaderTestInput, ShaderTestOutput](
			"shader_test",
			"Compile-test shaders via glslangValidator. If name is provided, test that shader; otherwise test all.",
			func(_ context.Context, input ShaderTestInput) (ShaderTestOutput, error) {
				var targets []string
				if input.Name != "" {
					p, err := findShader(input.Name)
					if err != nil {
						return ShaderTestOutput{}, err
					}
					targets = append(targets, p)
				} else {
					files, err := listGLSL()
					if err != nil {
						return ShaderTestOutput{}, err
					}
					dir := shadersDir()
					for _, f := range files {
						targets = append(targets, filepath.Join(dir, f))
					}
				}

				var results []ShaderTestResult
				passed, failed := 0, 0
				for _, t := range targets {
					cmd := exec.Command("glslangValidator", "-S", "frag", t)
					out, err := cmd.CombinedOutput()
					r := ShaderTestResult{
						Name:   strings.TrimSuffix(filepath.Base(t), ".glsl"),
						Output: strings.TrimSpace(string(out)),
					}
					if err != nil {
						r.Passed = false
						failed++
					} else {
						r.Passed = true
						passed++
					}
					results = append(results, r)
				}
				return ShaderTestOutput{
					Results: results,
					Passed:  passed,
					Failed:  failed,
				}, nil
			},
		),

		// ── shader_benchmark ─────────────────────────
		handler.TypedHandler[ShaderBenchmarkInput, ShaderBenchmarkOutput](
			"shader_benchmark",
			"Benchmark shader compilation time and file size via glslangValidator. Reports per-shader and aggregate stats.",
			func(_ context.Context, input ShaderBenchmarkInput) (ShaderBenchmarkOutput, error) {
				var targets []string
				if input.Name != "" {
					p, err := findShader(input.Name)
					if err != nil {
						return ShaderBenchmarkOutput{}, err
					}
					targets = append(targets, p)
				} else {
					files, err := listGLSL()
					if err != nil {
						return ShaderBenchmarkOutput{}, err
					}
					dir := shadersDir()
					for _, f := range files {
						targets = append(targets, filepath.Join(dir, f))
					}
				}

				var results []ShaderBenchResult
				var totalMs float64
				var maxMs float64
				var maxShader string
				var totalBytes int64

				for _, t := range targets {
					name := strings.TrimSuffix(filepath.Base(t), ".glsl")
					info, _ := os.Stat(t)
					var sizeBytes int64
					if info != nil {
						sizeBytes = info.Size()
					}

					start := time.Now()
					cmd := exec.Command("glslangValidator", "-S", "frag", t)
					err := cmd.Run()
					elapsed := time.Since(start).Seconds() * 1000

					r := ShaderBenchResult{
						Name:      name,
						CompileMs: elapsed,
						SizeBytes: sizeBytes,
						Category:  inferCategory(name),
						Passed:    err == nil,
					}
					results = append(results, r)
					totalMs += elapsed
					totalBytes += sizeBytes

					if elapsed > maxMs {
						maxMs = elapsed
						maxShader = name
					}
				}

				var avgMs float64
				if len(results) > 0 {
					avgMs = totalMs / float64(len(results))
				}

				return ShaderBenchmarkOutput{
					Results:   results,
					Count:     len(results),
					AvgMs:     avgMs,
					MaxMs:     maxMs,
					MaxShader: maxShader,
					TotalKB:   float64(totalBytes) / 1024,
				}, nil
			},
		),

		// ── shader_get_state ──────────────────────────
		handler.TypedHandler[ShaderGetStateInput, ShaderGetStateOutput](
			"shader_get_state",
			"Read the current Kitty shader and theme state from kitty-shader-playlist state files.",
			func(_ context.Context, _ ShaderGetStateInput) (ShaderGetStateOutput, error) {
				state, err := readCurrentVisualState()
				if err != nil {
					return ShaderGetStateOutput{}, err
				}
				return ShaderGetStateOutput{
					ActiveShader: state.ActiveShader,
					ShaderName:   state.ShaderName,
					ActiveTheme:  state.ActiveTheme,
					VisualLabel:  state.VisualLabel,
					Playlist:     state.Playlist,
				}, nil
			},
		),

		// ── shader_meta ───────────────────────────────
		handler.TypedHandler[ShaderMetaInput, ShaderMetaOutput](
			"shader_meta",
			"Get full metadata for a shader from the manifest (category, cost, source, description, playlists).",
			func(_ context.Context, input ShaderMetaInput) (ShaderMetaOutput, error) {
				p, err := findShader(input.Name)
				if err != nil {
					return ShaderMetaOutput{}, fmt.Errorf("[%s] %w", handler.ErrNotFound, err)
				}
				name := strings.TrimSuffix(filepath.Base(p), ".glsl")
				out := ShaderMetaOutput{
					Name: name,
					Path: p,
				}
				if manifest := m.getManifest(); manifest != nil {
					if meta, ok := manifest[name]; ok {
						out.Category = meta.Category
						out.Cost = meta.Cost
						out.Source = meta.Source
						out.Description = meta.Description
						out.Playlists = meta.Playlists
						out.InManifest = true
						return out, nil
					}
				}
				out.Category = inferCategory(name)
				return out, nil
			},
		),

		// ── wallpaper_list ────────────────────────────
		handler.TypedHandler[WallpaperListInput, WallpaperListOutput](
			"wallpaper_list",
			"List available wallpaper shaders (GLSL fragment shaders rendered as live animated wallpapers via shaderbg).",
			func(_ context.Context, _ WallpaperListInput) (WallpaperListOutput, error) {
				dir := wallpaperShadersDir()
				entries, err := os.ReadDir(dir)
				if err != nil {
					return WallpaperListOutput{}, fmt.Errorf("read wallpaper shaders dir %s: %w", dir, err)
				}
				var out []WallpaperEntry
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".frag") {
						continue
					}
					out = append(out, WallpaperEntry{
						Name: strings.TrimSuffix(e.Name(), ".frag"),
						Path: filepath.Join(dir, e.Name()),
					})
				}
				return WallpaperListOutput{Shaders: out, Count: len(out)}, nil
			},
		),

		// ── wallpaper_set ─────────────────────────────
		handler.TypedHandler[WallpaperSetInput, WallpaperSetOutput](
			"wallpaper_set",
			"Set a specific wallpaper shader by name. Launches shaderbg to render the shader as a live animated wallpaper.",
			func(_ context.Context, input WallpaperSetInput) (WallpaperSetOutput, error) {
				name := input.Name
				if !strings.HasSuffix(name, ".frag") {
					name += ".frag"
				}
				shaderPath := filepath.Join(wallpaperShadersDir(), name)
				if _, err := os.Stat(shaderPath); err != nil {
					return WallpaperSetOutput{}, fmt.Errorf("wallpaper shader not found: %s", input.Name)
				}
				cmd := exec.Command(wallpaperScript(), "set", shaderPath)
				out, err := cmd.CombinedOutput()
				if err != nil {
					return WallpaperSetOutput{}, fmt.Errorf("shader-wallpaper.sh set failed: %s: %w", strings.TrimSpace(string(out)), err)
				}
				return WallpaperSetOutput{
					Applied: strings.TrimSuffix(name, ".frag"),
					Output:  strings.TrimSpace(string(out)),
				}, nil
			},
		),

		// ── wallpaper_random ──────────────────────────
		handler.TypedHandler[WallpaperRandomInput, WallpaperRandomOutput](
			"wallpaper_random",
			"Set a random wallpaper shader. Picks a random GLSL fragment shader and launches it as a live animated wallpaper.",
			func(_ context.Context, _ WallpaperRandomInput) (WallpaperRandomOutput, error) {
				cmd := exec.Command(wallpaperScript(), "random")
				out, err := cmd.CombinedOutput()
				if err != nil {
					return WallpaperRandomOutput{}, fmt.Errorf("shader-wallpaper.sh random failed: %s: %w", strings.TrimSpace(string(out)), err)
				}
				// Parse the output to extract the shader name
				output := strings.TrimSpace(string(out))
				applied := output
				// The script outputs "Shader: <name>" when not in a terminal
				if strings.HasPrefix(output, "Shader: ") {
					applied = strings.TrimPrefix(output, "Shader: ")
				}
				return WallpaperRandomOutput{
					Applied: applied,
					Output:  output,
				}, nil
			},
		),

		// ── shader_cycle ──────────────────────────────
		handler.TypedHandler[ShaderCycleInput, ShaderCycleOutput](
			"shader_cycle",
			"Advance the active Kitty visual playlist (next/prev) via kitty-shader-playlist.sh.",
			func(_ context.Context, input ShaderCycleInput) (ShaderCycleOutput, error) {
				direction := input.Direction
				if direction == "" {
					direction = "next"
				}
				playlist := input.Playlist
				if playlist == "" {
					playlist = activePlaylistName()
				}
				if _, err := runKittyPlaylist(direction, playlist); err != nil {
					return ShaderCycleOutput{}, fmt.Errorf("failed to apply kitty visual: %w", err)
				}
				state, err := readCurrentVisualState()
				if err != nil {
					return ShaderCycleOutput{}, err
				}
				_ = appendShaderHistory(ShaderHistoryEntry{
					Action: "cycle",
					Shader: state.ShaderName,
					Source: "mcp:shader_cycle:" + playlist,
					Details: map[string]string{
						"direction": direction,
						"theme":     state.ActiveTheme,
					},
				})
				return ShaderCycleOutput{
					Applied:  state.ShaderName,
					Path:     state.ActiveShader,
					Theme:    state.ActiveTheme,
					Label:    state.VisualLabel,
					Playlist: playlist,
					Position: state.Position,
					Total:    state.Total,
				}, nil
			},
		),

		// ── shader_status ─────────────────────────────
		handler.TypedHandler[ShaderStatusInput, ShaderStatusOutput](
			"shader_status",
			"Rich status: current shader, Kitty theme, visual label, animation state, playlist position, and auto-rotate timer status.",
			func(_ context.Context, _ ShaderStatusInput) (ShaderStatusOutput, error) {
				state, err := readCurrentVisualState()
				if err != nil {
					return ShaderStatusOutput{}, err
				}
				out := ShaderStatusOutput{
					ActiveShader: state.ActiveShader,
					ShaderName:   state.ShaderName,
					ActiveTheme:  state.ActiveTheme,
					VisualLabel:  state.VisualLabel,
					Animation:    readAnimationState(),
					Playlist:     state.Playlist,
					Position:     state.Position,
					Total:        state.Total,
				}

				// Check systemd timer
				cmd := exec.Command("systemctl", "--user", "is-active", "shader-rotate.timer")
				if timerOut, err := cmd.Output(); err == nil {
					out.AutoRotate = strings.TrimSpace(string(timerOut)) == "active"
				}

				return out, nil
			},
		),

		// ── shader_build ──────────────────────────────
		handler.TypedHandler[ShaderBuildInput, ShaderBuildOutput](
			"shader_build",
			"Run glslangValidator to preprocess and validate shaders. Test a single shader or all shaders. Returns structured results.",
			func(_ context.Context, input ShaderBuildInput) (ShaderBuildOutput, error) {
				var targets []string
				if input.Shader != "" {
					p, err := findShader(input.Shader)
					if err != nil {
						return ShaderBuildOutput{}, err
					}
					targets = append(targets, p)
				} else {
					files, err := listGLSL()
					if err != nil {
						return ShaderBuildOutput{}, err
					}
					dir := shadersDir()
					for _, f := range files {
						targets = append(targets, filepath.Join(dir, f))
					}
				}

				var results []ShaderBuildResult
				passed, failed := 0, 0
				for _, t := range targets {
					cmd := exec.Command("glslangValidator", "-S", "frag", t)
					out, err := cmd.CombinedOutput()
					output := strings.TrimSpace(string(out))
					r := ShaderBuildResult{
						Name:   strings.TrimSuffix(filepath.Base(t), ".glsl"),
						Output: output,
					}
					if err != nil {
						r.Passed = false
						r.Errors = output
						failed++
					} else {
						r.Passed = true
						passed++
					}
					results = append(results, r)
				}

				summary := fmt.Sprintf("%d/%d passed", passed, passed+failed)
				if failed > 0 {
					summary += fmt.Sprintf(", %d failed", failed)
				}

				return ShaderBuildOutput{
					Results: results,
					Passed:  passed,
					Failed:  failed,
					Summary: summary,
				}, nil
			},
		),

		// ── shader_playlist ───────────────────────────
		handler.TypedHandler[ShaderPlaylistInput, ShaderPlaylistOutput](
			"shader_playlist",
			"List curated shader playlists or pick a random shader from one. Omit name to list all playlists.",
			func(_ context.Context, input ShaderPlaylistInput) (ShaderPlaylistOutput, error) {
				if input.Name == "" {
					names, err := listPlaylists()
					if err != nil {
						return ShaderPlaylistOutput{}, err
					}
					var infos []PlaylistInfo
					for _, n := range names {
						shaders, err := loadPlaylist(n)
						if err != nil {
							continue
						}
						infos = append(infos, PlaylistInfo{Name: n, Count: len(shaders)})
					}
					return ShaderPlaylistOutput{Playlists: infos}, nil
				}

				shaders, err := loadPlaylist(input.Name)
				if err != nil {
					return ShaderPlaylistOutput{}, fmt.Errorf("[%s] %w", handler.ErrNotFound, err)
				}

				if strings.EqualFold(input.Action, "random") {
					if len(shaders) == 0 {
						return ShaderPlaylistOutput{}, fmt.Errorf("playlist %s is empty", input.Name)
					}
					if _, err := runKittyPlaylist("random", input.Name); err != nil {
						return ShaderPlaylistOutput{}, fmt.Errorf("failed to apply kitty visual: %w", err)
					}
					state, err := readCurrentVisualState()
					if err != nil {
						return ShaderPlaylistOutput{}, err
					}
					_ = appendShaderHistory(ShaderHistoryEntry{
						Action: "random",
						Shader: state.ShaderName,
						Source: "mcp:shader_playlist:" + input.Name,
						Details: map[string]string{
							"playlist": input.Name,
							"theme":    state.ActiveTheme,
						},
					})
					return ShaderPlaylistOutput{
						Applied: &ShaderRandomOutput{
							Applied:  state.ShaderName,
							Path:     state.ActiveShader,
							Theme:    state.ActiveTheme,
							Label:    state.VisualLabel,
							Playlist: input.Name,
							Position: state.Position,
							Total:    state.Total,
						},
					}, nil
				}

				return ShaderPlaylistOutput{
					Playlists: []PlaylistInfo{{
						Name:    input.Name,
						Count:   len(shaders),
						Shaders: shaders,
					}},
				}, nil
			},
		),

		// ── shader_hot_reload ─────────────────────────
		handler.TypedHandler[ShaderHotReloadInput, ShaderHotReloadOutput](
			"shader_hot_reload",
			"Force kitty to reload shader config via SIGUSR1.",
			func(_ context.Context, _ ShaderHotReloadInput) (ShaderHotReloadOutput, error) {
				// Send SIGUSR1 to kitty to reload config
				cmd := exec.Command("pkill", "-USR1", "kitty")
				if err := cmd.Run(); err != nil {
					return ShaderHotReloadOutput{}, fmt.Errorf("SIGUSR1 to kitty failed: %w", err)
				}
				return ShaderHotReloadOutput{Reloaded: true, Method: "sigusr1"}, nil
			},
		),

		// ── shader_diff ───────────────────────────────
		handler.TypedHandler[ShaderDiffInput, ShaderDiffOutput](
			"shader_diff",
			"Compare the last two shader changes from the history log. Shows what changed between shader switches.",
			func(_ context.Context, input ShaderDiffInput) (ShaderDiffOutput, error) {
				limit := input.Limit
				if limit < 2 {
					limit = 2
				}
				entries, err := readShaderHistory(limit, time.Time{})
				if err != nil {
					return ShaderDiffOutput{}, fmt.Errorf("read history: %w", err)
				}
				if len(entries) < 2 {
					return ShaderDiffOutput{}, fmt.Errorf("need at least 2 history entries for diff, found %d", len(entries))
				}
				before := entries[len(entries)-2]
				after := entries[len(entries)-1]

				var changes []DiffEntry
				if before.Shader != after.Shader {
					changes = append(changes, DiffEntry{Field: "shader", Before: before.Shader, After: after.Shader})
				}
				if before.Action != after.Action {
					changes = append(changes, DiffEntry{Field: "action", Before: before.Action, After: after.Action})
				}
				if before.Source != after.Source {
					changes = append(changes, DiffEntry{Field: "source", Before: before.Source, After: after.Source})
				}
				if before.Timestamp != after.Timestamp {
					changes = append(changes, DiffEntry{Field: "timestamp", Before: before.Timestamp, After: after.Timestamp})
				}

				return ShaderDiffOutput{
					Changes: changes,
					Before:  before.Shader,
					After:   after.Shader,
				}, nil
			},
		),

		// ── shader_log ────────────────────────────────
		handler.TypedHandler[ShaderLogInput, ShaderLogOutput](
			"shader_log",
			"View shader change history with computed durations. Shows when each shader was active and for how long.",
			func(_ context.Context, input ShaderLogInput) (ShaderLogOutput, error) {
				limit := input.Limit
				if limit <= 0 {
					limit = 20
				}
				var since time.Time
				if input.Since != "" {
					var err error
					since, err = time.Parse(time.RFC3339, input.Since)
					if err != nil {
						return ShaderLogOutput{}, fmt.Errorf("invalid since timestamp: %w", err)
					}
				}
				entries, err := readShaderHistory(limit+1, since) // +1 to compute last duration
				if err != nil {
					return ShaderLogOutput{}, fmt.Errorf("read history: %w", err)
				}

				var out []ShaderLogEntry
				for i, e := range entries {
					le := ShaderLogEntry{
						Shader:    e.Shader,
						Action:    e.Action,
						Source:    e.Source,
						Timestamp: e.Timestamp,
					}
					// Compute duration from this entry to the next
					if i+1 < len(entries) {
						t1, err1 := time.Parse(time.RFC3339, e.Timestamp)
						t2, err2 := time.Parse(time.RFC3339, entries[i+1].Timestamp)
						if err1 == nil && err2 == nil {
							le.Duration = t2.Sub(t1).Round(time.Second).String()
						}
					}
					out = append(out, le)
				}
				// Trim to requested limit
				if len(out) > limit {
					out = out[len(out)-limit:]
				}

				return ShaderLogOutput{Entries: out, Count: len(out)}, nil
			},
		),

		// ── shader_preview ────────────────────────────
		handler.TypedHandler[ShaderPreviewInput, ShaderPreviewOutput](
			"shader_preview",
			"Preview a shader for N seconds, then automatically revert to the previous shader. Cancels any in-progress preview.",
			func(_ context.Context, input ShaderPreviewInput) (ShaderPreviewOutput, error) {
				p, err := findShader(input.Name)
				if err != nil {
					return ShaderPreviewOutput{}, err
				}

				duration := input.Duration
				if duration <= 0 {
					duration = 10
				}

				// Read current shader to revert to
				original, err := readActiveShader()
				if err != nil {
					return ShaderPreviewOutput{}, fmt.Errorf("read current shader: %w", err)
				}

				// Cancel any in-progress preview
				preview.mu.Lock()
				if preview.timer != nil {
					preview.timer.Stop()
				}

				// Apply preview shader
				if err := atomicSetShader(p, "mcp:shader_preview"); err != nil {
					preview.mu.Unlock()
					return ShaderPreviewOutput{}, fmt.Errorf("apply preview shader: %w", err)
				}

				// Schedule revert
				revertPath := original
				preview.original = revertPath
				preview.timer = time.AfterFunc(time.Duration(duration)*time.Second, func() {
					_ = atomicSetShader(filepath.Join(shadersDir(), filepath.Base(revertPath)), "mcp:shader_preview:revert")
					preview.mu.Lock()
					preview.timer = nil
					preview.original = ""
					preview.mu.Unlock()
				})
				preview.mu.Unlock()

				return ShaderPreviewOutput{
					Previewing: strings.TrimSuffix(filepath.Base(p), ".glsl"),
					Duration:   duration,
					RevertTo:   strings.TrimSuffix(filepath.Base(original), ".glsl"),
				}, nil
			},
		),

		// ── shader_preset_list ────────────────────────
		handler.TypedHandler[ShaderPresetListInput, ShaderPresetListOutput](
			"shader_preset_list",
			"List named parameter presets for DarkWindow shaders. Presets document the configurable #define values and top-level constants in each shader's GLSL source. Optionally filter to a specific shader name.",
			func(_ context.Context, input ShaderPresetListInput) (ShaderPresetListOutput, error) {
				p, err := loadPresets()
				if err != nil {
					return ShaderPresetListOutput{}, err
				}
				var entries []PresetEntry
				for shaderName, block := range p.Shaders {
					if input.Shader != "" {
						norm := strings.TrimSuffix(input.Shader, ".glsl")
						if !strings.EqualFold(shaderName, norm) {
							continue
						}
					}
					for presetName, preset := range block.Presets {
						entries = append(entries, PresetEntry{
							Shader:      shaderName,
							Preset:      presetName,
							Description: preset.Description,
							Params:      preset.Params,
						})
					}
				}
				return ShaderPresetListOutput{Presets: entries, Count: len(entries)}, nil
			},
		),

		// ── shader_preset_apply ───────────────────────
		handler.TypedHandler[ShaderPresetApplyInput, ShaderPresetApplyOutput](
			"shader_preset_apply",
			"Apply a named preset to a DarkWindow shader. Reads the preset from presets.toml, rewrites matching parameter tokens in a temporary copy of the GLSL source, then applies it via shader_set. The original shader source is never modified.",
			func(_ context.Context, input ShaderPresetApplyInput) (ShaderPresetApplyOutput, error) {
				// Normalise shader name
				shaderName := strings.TrimSuffix(input.Shader, ".glsl")

				// Locate the source GLSL
				srcPath, err := findShader(shaderName)
				if err != nil {
					return ShaderPresetApplyOutput{}, err
				}

				// Load presets and look up the requested one
				p, err := loadPresets()
				if err != nil {
					return ShaderPresetApplyOutput{}, err
				}
				shaderBlock, ok := p.Shaders[shaderName]
				if !ok {
					return ShaderPresetApplyOutput{}, fmt.Errorf("[%s] no presets defined for shader %q — check kitty/shaders/presets.toml", handler.ErrNotFound, shaderName)
				}
				preset, ok := shaderBlock.Presets[input.Preset]
				if !ok {
					var available []string
					for k := range shaderBlock.Presets {
						available = append(available, k)
					}
					return ShaderPresetApplyOutput{}, fmt.Errorf("[%s] preset %q not found for shader %q; available: %s", handler.ErrNotFound, input.Preset, shaderName, strings.Join(available, ", "))
				}

				out := ShaderPresetApplyOutput{
					Shader:      shaderName,
					Preset:      input.Preset,
					Description: preset.Description,
					Params:      preset.Params,
				}

				// If no params, just apply the shader as-is and return the preset metadata.
				if len(preset.Params) == 0 {
					if err := atomicSetShader(srcPath, "mcp:shader_preset_apply"); err != nil {
						return ShaderPresetApplyOutput{}, fmt.Errorf("apply shader: %w", err)
					}
					out.Applied = true
					out.Note = "No parameter substitutions; shader applied without modification."
					return out, nil
				}

				// Read GLSL source
				src, err := os.ReadFile(srcPath)
				if err != nil {
					return ShaderPresetApplyOutput{}, fmt.Errorf("read shader source: %w", err)
				}
				glsl := string(src)

				// Substitute #define and top-level float parameter values.
				// Pattern: "#define PARAM <oldval>" → "#define PARAM <newval>"
				//           "float param = <oldval>;" → "float param = <newval>;"
				for param, val := range preset.Params {
					// Try #define substitution first (matches: #define PARAM <anything>)
					definePattern := "#define " + param + " "
					if idx := strings.Index(glsl, definePattern); idx >= 0 {
						lineEnd := strings.IndexByte(glsl[idx:], '\n')
						if lineEnd < 0 {
							lineEnd = len(glsl) - idx
						}
						oldLine := glsl[idx : idx+lineEnd]
						newLine := definePattern + val
						glsl = strings.Replace(glsl, oldLine, newLine, 1)
						continue
					}
					// Try top-level float/int variable: "float param = <val>;"
					for _, typ := range []string{"float ", "int "} {
						varPattern := typ + param + " = "
						if idx := strings.Index(glsl, varPattern); idx >= 0 {
							lineEnd := strings.IndexByte(glsl[idx:], ';')
							if lineEnd >= 0 {
								oldDecl := glsl[idx : idx+lineEnd+1]
								newDecl := varPattern + val + ";"
								glsl = strings.Replace(glsl, oldDecl, newDecl, 1)
							}
							break
						}
					}
				}

				// Write to a temporary file alongside the source so the playlist
				// script can resolve it by name within the shaders directory.
				tmpName := shaderName + "__preset_" + input.Preset + ".glsl"
				tmpPath := filepath.Join(filepath.Dir(srcPath), tmpName)
				if err := os.WriteFile(tmpPath, []byte(glsl), 0o644); err != nil {
					return ShaderPresetApplyOutput{}, fmt.Errorf("write temp shader: %w", err)
				}

				// Apply via the playlist script (uses name lookup, strips .glsl)
				if err := atomicSetShader(tmpPath, "mcp:shader_preset_apply"); err != nil {
					_ = os.Remove(tmpPath) // best-effort cleanup on failure
					return ShaderPresetApplyOutput{}, fmt.Errorf("apply preset shader: %w", err)
				}

				out.Applied = true
				out.Note = fmt.Sprintf("Applied with %d parameter substitution(s). Temp file: %s", len(preset.Params), tmpPath)
				return out, nil
			},
		),

		// ── shader_audit_trail ────────────────────────
		handler.TypedHandler[ShaderAuditTrailInput, ShaderAuditTrailOutput](
			"shader_audit_trail",
			"View the raw, append-only shader change history log. Returns all recorded shader changes with timestamps and sources.",
			func(_ context.Context, input ShaderAuditTrailInput) (ShaderAuditTrailOutput, error) {
				limit := input.Limit
				if limit <= 0 {
					limit = 50
				}
				entries, err := readShaderHistory(limit, time.Time{})
				if err != nil {
					return ShaderAuditTrailOutput{}, fmt.Errorf("read history: %w", err)
				}

				if strings.EqualFold(input.Format, "text") {
					var sb strings.Builder
					for _, e := range entries {
						fmt.Fprintf(&sb, "[%s] %s %s (via %s)", e.Timestamp, e.Action, e.Shader, e.Source)
						if len(e.Details) > 0 {
							for k, v := range e.Details {
								fmt.Fprintf(&sb, " %s=%s", k, v)
							}
						}
						sb.WriteString("\n")
					}
					return ShaderAuditTrailOutput{Text: sb.String(), Count: len(entries)}, nil
				}

				return ShaderAuditTrailOutput{Entries: entries, Count: len(entries)}, nil
			},
		),
	}
}

// Resources returns the MCP resources provided by ShaderModule.
func (m *ShaderModule) Resources() []resources.ResourceDefinition {
	return []resources.ResourceDefinition{
		{
			Resource: mcp.NewResource(
				"shader://current",
				"Current Shader State",
				mcp.WithResourceDescription("Currently active kitty shader, playlist position, auto-rotate status, and kitty theme"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				state, err := readCurrentVisualState()
				if err != nil {
					return nil, fmt.Errorf("read shader state: %w", err)
				}

				autoRotate := false
				cmd := exec.Command("systemctl", "--user", "is-active", "shader-rotate.timer")
				if out, err := cmd.Output(); err == nil {
					autoRotate = strings.TrimSpace(string(out)) == "active"
				}

				result := map[string]any{
					"active_shader": state.ActiveShader,
					"shader_name":   state.ShaderName,
					"active_theme":  state.ActiveTheme,
					"visual_label":  state.VisualLabel,
					"playlist":      state.Playlist,
					"position":      state.Position,
					"total":         state.Total,
					"auto_rotate":   autoRotate,
				}
				data, _ := json.MarshalIndent(result, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "shader://current",
						MIMEType: "application/json",
						Text:     string(data),
					},
				}, nil
			},
			Category: "shader",
			Tags:     []string{"shader", "kitty", "theme", "state"},
		},
		{
			Resource: mcp.NewResource(
				"shader://categories",
				"Shader Category Breakdown",
				mcp.WithResourceDescription("Count of GLSL shaders per category (crt, neon, glitch, etc.) from the DarkWindow shader collection"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				shaders, err := listGLSL()
				if err != nil {
					return nil, fmt.Errorf("list shaders: %w", err)
				}
				cats := make(map[string]int)
				for _, s := range shaders {
					cat := inferCategory(s)
					cats[cat]++
				}
				result := map[string]any{
					"categories": cats,
					"total":      len(shaders),
				}
				data, _ := json.MarshalIndent(result, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "shader://categories",
						MIMEType: "application/json",
						Text:     string(data),
					},
				}, nil
			},
			Category: "shader",
			Tags:     []string{"shader", "category", "glsl"},
		},
	}
}

func (m *ShaderModule) Templates() []resources.TemplateDefinition { return nil }

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------
