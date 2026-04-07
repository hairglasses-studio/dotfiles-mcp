// mod_shader.go — Ghostty shader pipeline tools (migrated from shader-mcp)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
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
)

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

func shadersDir() string {
	// Prefer DOTFILES_DIR if set, otherwise follow the symlink at ~/.config/ghostty/shaders
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "ghostty", "shaders")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "ghostty", "shaders")
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

func ghosttyConfig() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "ghostty", "config")
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

// atomicSetShader replaces the custom-shader line in the Ghostty config
// using temp-file + rename for atomic writes. Also detects whether the
// shader uses animation uniforms and sets custom-shader-animation accordingly.
// Optionally logs the change to the JSONL history.
func atomicSetShader(shaderPath string, source ...string) error {
	cfgPath := ghosttyConfig()

	// Detect animation need.
	anim := "false"
	if data, err := os.ReadFile(shaderPath); err == nil {
		s := string(data)
		if strings.Contains(s, "ghostty_time") || strings.Contains(s, "iTime") || strings.Contains(s, "u_time") {
			anim = "true"
		}
	}

	f, err := os.Open(cfgPath)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	tmp, err := os.CreateTemp(filepath.Dir(cfgPath), "ghostty-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	scanner := bufio.NewScanner(f)
	w := bufio.NewWriter(tmp)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "custom-shader =") || strings.HasPrefix(trimmed, "# custom-shader ="):
			fmt.Fprintf(w, "custom-shader = %s\n", shaderPath)
		case strings.HasPrefix(trimmed, "custom-shader-animation ="):
			fmt.Fprintf(w, "custom-shader-animation = %s\n", anim)
		default:
			fmt.Fprintln(w, line)
		}
	}
	if err := scanner.Err(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("scan config: %w", err)
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("flush: %w", err)
	}
	tmp.Close()

	// Preserve original permissions.
	if info, err := os.Stat(cfgPath); err == nil {
		os.Chmod(tmpPath, info.Mode())
	}

	if err := os.Rename(tmpPath, cfgPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	// Log to JSONL history
	src := "mcp:shader_set"
	if len(source) > 0 && source[0] != "" {
		src = source[0]
	}
	_ = appendShaderHistory(ShaderHistoryEntry{
		Action: "set",
		Shader: strings.TrimSuffix(filepath.Base(shaderPath), ".glsl"),
		Source: src,
	})
	return nil
}

// readActiveShader reads the current custom-shader value from the Ghostty config.
func readActiveShader() (string, error) {
	data, err := os.ReadFile(ghosttyConfig())
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "custom-shader =") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", fmt.Errorf("no custom-shader line found in config")
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
	return filepath.Join(shadersDir(), "playlists")
}

func playlistStateDir() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "state", "ghostty")
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

// readAnimationState reads the current custom-shader-animation value from Ghostty config.
func readAnimationState() bool {
	data, err := os.ReadFile(ghosttyConfig())
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "custom-shader-animation =") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]) == "true"
			}
		}
	}
	return false
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
}

type ShaderRandomInput struct{}

type ShaderRandomOutput struct {
	Applied string `json:"applied"`
	Path    string `json:"path"`
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

type ShaderGetStateInput struct{}

type ShaderGetStateOutput struct {
	ActiveShader string `json:"active_shader"`
	ShaderName   string `json:"shader_name"`
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
	Playlist string `json:"playlist"`
	Position int    `json:"position"`
	Total    int    `json:"total"`
}

type ShaderStatusInput struct{}

type ShaderStatusOutput struct {
	ActiveShader string `json:"active_shader"`
	ShaderName   string `json:"shader_name"`
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
	return filepath.Join(os.Getenv("HOME"), ".local", "state", "ghostty", "shader-history.jsonl")
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
// New tool I/O types
// ---------------------------------------------------------------------------

type ShaderHotReloadInput struct{}

type ShaderHotReloadOutput struct {
	Reloaded bool   `json:"reloaded"`
	Method   string `json:"method"` // "touch" or "sigusr1"
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
func (m *ShaderModule) Description() string { return "Ghostty shader pipeline tools" }

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
			"List all GLSL shaders in the Ghostty shaders directory. Optionally filter by category.",
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
			"Apply a shader to Ghostty by updating the config with an atomic write. Ghostty auto-reloads.",
			func(_ context.Context, input ShaderSetInput) (ShaderSetOutput, error) {
				p, err := findShader(input.Name)
				if err != nil {
					return ShaderSetOutput{}, err
				}
				if err := atomicSetShader(p); err != nil {
					return ShaderSetOutput{}, fmt.Errorf("failed to apply shader: %w", err)
				}
				return ShaderSetOutput{
					Applied: strings.TrimSuffix(filepath.Base(p), ".glsl"),
					Path:    p,
				}, nil
			},
		),

		// ── shader_random ─────────────────────────────
		handler.TypedHandler[ShaderRandomInput, ShaderRandomOutput](
			"shader_random",
			"Pick a random shader and apply it to Ghostty.",
			func(_ context.Context, _ ShaderRandomInput) (ShaderRandomOutput, error) {
				files, err := listGLSL()
				if err != nil {
					return ShaderRandomOutput{}, err
				}
				if len(files) == 0 {
					return ShaderRandomOutput{}, fmt.Errorf("no shaders found")
				}
				pick := files[rand.IntN(len(files))]
				p := filepath.Join(shadersDir(), pick)
				if err := atomicSetShader(p, "mcp:shader_random"); err != nil {
					return ShaderRandomOutput{}, fmt.Errorf("failed to apply shader: %w", err)
				}
				return ShaderRandomOutput{
					Applied: strings.TrimSuffix(pick, ".glsl"),
					Path:    p,
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

		// ── shader_get_state ──────────────────────────
		handler.TypedHandler[ShaderGetStateInput, ShaderGetStateOutput](
			"shader_get_state",
			"Read the currently active shader from the Ghostty config.",
			func(_ context.Context, _ ShaderGetStateInput) (ShaderGetStateOutput, error) {
				active, err := readActiveShader()
				if err != nil {
					return ShaderGetStateOutput{}, err
				}
				name := strings.TrimSuffix(filepath.Base(active), ".glsl")
				return ShaderGetStateOutput{
					ActiveShader: active,
					ShaderName:   name,
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
			"Advance the shader playlist (next/prev). Reads current playlist state, advances position, and applies the next shader.",
			func(_ context.Context, input ShaderCycleInput) (ShaderCycleOutput, error) {
				playlist := input.Playlist
				if playlist == "" {
					playlist = activePlaylistName()
				}

				shaders, err := loadPlaylist(playlist)
				if err != nil {
					return ShaderCycleOutput{}, fmt.Errorf("[%s] %w", handler.ErrNotFound, err)
				}
				if len(shaders) == 0 {
					return ShaderCycleOutput{}, fmt.Errorf("playlist %s is empty", playlist)
				}

				idx, _ := readPlaylistIndex(playlist)

				switch input.Direction {
				case "prev":
					idx = (idx - 1 + len(shaders)) % len(shaders)
				default: // "next" or empty
					idx = (idx + 1) % len(shaders)
				}

				pick := shaders[idx]
				p, err := findShader(pick)
				if err != nil {
					return ShaderCycleOutput{}, fmt.Errorf("shader %s from playlist %s not found: %w", pick, playlist, err)
				}

				if err := atomicSetShader(p, "mcp:shader_cycle:"+playlist); err != nil {
					return ShaderCycleOutput{}, fmt.Errorf("failed to apply shader: %w", err)
				}

				if err := writePlaylistIndex(playlist, idx); err != nil {
					return ShaderCycleOutput{}, fmt.Errorf("failed to save playlist index: %w", err)
				}

				return ShaderCycleOutput{
					Applied:  strings.TrimSuffix(filepath.Base(p), ".glsl"),
					Path:     p,
					Playlist: playlist,
					Position: idx + 1,
					Total:    len(shaders),
				}, nil
			},
		),

		// ── shader_status ─────────────────────────────
		handler.TypedHandler[ShaderStatusInput, ShaderStatusOutput](
			"shader_status",
			"Rich status: current shader, animation state, active playlist, position, and auto-rotate timer status.",
			func(_ context.Context, _ ShaderStatusInput) (ShaderStatusOutput, error) {
				active, err := readActiveShader()
				if err != nil {
					return ShaderStatusOutput{}, err
				}
				name := strings.TrimSuffix(filepath.Base(active), ".glsl")
				anim := readAnimationState()

				out := ShaderStatusOutput{
					ActiveShader: active,
					ShaderName:   name,
					Animation:    anim,
				}

				// Playlist info
				playlist := activePlaylistName()
				shaders, plErr := loadPlaylist(playlist)
				if plErr == nil && len(shaders) > 0 {
					out.Playlist = playlist
					idx, _ := readPlaylistIndex(playlist)
					out.Position = idx + 1
					out.Total = len(shaders)
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
					pick := shaders[rand.IntN(len(shaders))]
					p, err := findShader(pick)
					if err != nil {
						return ShaderPlaylistOutput{}, err
					}
					if err := atomicSetShader(p, "mcp:shader_playlist:"+input.Name); err != nil {
						return ShaderPlaylistOutput{}, fmt.Errorf("failed to apply shader: %w", err)
					}
					return ShaderPlaylistOutput{
						Applied: &ShaderRandomOutput{
							Applied: strings.TrimSuffix(filepath.Base(p), ".glsl"),
							Path:    p,
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
			"Force Ghostty to reload its config. Touches the config file to trigger inotify; falls back to SIGUSR1.",
			func(_ context.Context, _ ShaderHotReloadInput) (ShaderHotReloadOutput, error) {
				cfgPath := ghosttyConfig()

				// Method 1: re-write config atomically (new inode triggers inotify)
				data, err := os.ReadFile(cfgPath)
				if err != nil {
					return ShaderHotReloadOutput{}, fmt.Errorf("read config: %w", err)
				}
				tmp, err := os.CreateTemp(filepath.Dir(cfgPath), "ghostty-reload-*.tmp")
				if err != nil {
					return ShaderHotReloadOutput{}, fmt.Errorf("create temp: %w", err)
				}
				tmpPath := tmp.Name()
				if _, err := tmp.Write(data); err != nil {
					tmp.Close()
					os.Remove(tmpPath)
					return ShaderHotReloadOutput{}, err
				}
				tmp.Close()
				if info, statErr := os.Stat(cfgPath); statErr == nil {
					os.Chmod(tmpPath, info.Mode())
				}
				if err := os.Rename(tmpPath, cfgPath); err != nil {
					os.Remove(tmpPath)
					// Fallback: SIGUSR1
					cmd := exec.Command("pkill", "-USR1", "ghostty")
					if err := cmd.Run(); err != nil {
						return ShaderHotReloadOutput{}, fmt.Errorf("both touch and SIGUSR1 failed")
					}
					return ShaderHotReloadOutput{Reloaded: true, Method: "sigusr1"}, nil
				}
				return ShaderHotReloadOutput{Reloaded: true, Method: "touch"}, nil
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

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

