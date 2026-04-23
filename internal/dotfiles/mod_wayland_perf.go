// mod_wayland_perf.go — Wayland + NVIDIA perf tuning, palette pipeline, and
// shader tier discovery tools.
//
// Wraps the shell scripts introduced in the Phase 1-4 Wayland Graphics Pipeline
// consolidation (see ROADMAP "Wayland Graphics Pipeline Consolidation"):
//   - scripts/hypr-perf-mode.sh       → hypr_perf_mode
//   - hyprctl keyword debug:overlay   → hypr_frame_overlay
//   - hyprctl monitors -j             → hypr_vrr_status
//   - scripts/palette-propagate.sh    → color_pipeline_apply
//   - kitty/shaders/bin/shader-tier.sh → shader_tier
package dotfiles

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// WaylandPerfModule
// ---------------------------------------------------------------------------

type WaylandPerfModule struct{}

func (m *WaylandPerfModule) Name() string { return "wayland_perf" }
func (m *WaylandPerfModule) Description() string {
	return "Wayland + NVIDIA perf tuning, palette pipeline, shader tier discovery"
}

// ---------------------------------------------------------------------------
// Script path helpers (use DOTFILES_DIR when available)
// ---------------------------------------------------------------------------

func perfModeScript() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "scripts", "hypr-perf-mode.sh")
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "scripts", "hypr-perf-mode.sh")
}

func palettePropagateScript() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "scripts", "palette-propagate.sh")
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "scripts", "palette-propagate.sh")
}

func shaderTierScript() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return filepath.Join(d, "kitty", "shaders", "bin", "shader-tier.sh")
	}
	return filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "kitty", "shaders", "bin", "shader-tier.sh")
}

func tierPlaylistPath(tier string) string {
	base := ""
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		base = filepath.Join(d, "kitty", "shaders", "playlists")
	} else {
		base = filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "dotfiles", "kitty", "shaders", "playlists")
	}
	return filepath.Join(base, "tier-"+tier+".txt")
}

// ---------------------------------------------------------------------------
// Input/Output types
// ---------------------------------------------------------------------------

// hypr_perf_mode

type HyprPerfModeInput struct {
	Mode string `json:"mode,omitempty" jsonschema:"enum=status,enum=quality,enum=performance,enum=auto,description=Desired mode (default: status — read current without changing)"`
}

type HyprPerfModeOutput struct {
	Mode       string `json:"mode"`
	Applied    bool   `json:"applied"`
	ScriptPath string `json:"script_path,omitempty"`
	Output     string `json:"output,omitempty"`
}

// hypr_vrr_status

type HyprVRRStatusInput struct{}

type HyprMonitorVRR struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	RefreshRate float64 `json:"refresh_rate"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Scale       float64 `json:"scale"`
	VRR         bool    `json:"vrr"`
}

type HyprVRRStatusOutput struct {
	Monitors []HyprMonitorVRR `json:"monitors"`
	Count    int              `json:"count"`
}

// hypr_vrr_set

// HyprVRRSetInput toggles per-monitor VRR. Mode values match Hyprland's
// `vrr` keyword: 0=off, 1=always-on, 2=fullscreen-only (the safest option
// per nvidia-wayland.md rules — reduces DSC handshake risk on DP-2).
type HyprVRRSetInput struct {
	Monitor string `json:"monitor" jsonschema:"required,description=Monitor name as shown by hypr_get_monitors (e.g. DP-2)"`
	Mode    int    `json:"mode" jsonschema:"required,enum=0,enum=1,enum=2,description=VRR mode: 0=off, 1=always-on, 2=fullscreen-only"`
}

type HyprVRRSetOutput struct {
	Monitor string `json:"monitor"`
	Mode    int    `json:"mode"`
	Applied bool   `json:"applied"`
	// MonitorArg is the full `hyprctl keyword monitor ...` argument that
	// was issued — useful for audit / capture.
	MonitorArg string `json:"monitor_arg"`
	Output     string `json:"output,omitempty"`
}

// hypr_frame_overlay

type HyprFrameOverlayInput struct {
	Enable *bool `json:"enable,omitempty" jsonschema:"description=Set to true/false; omit to read current state"`
}

type HyprFrameOverlayOutput struct {
	Enabled bool   `json:"enabled"`
	Changed bool   `json:"changed"`
	Message string `json:"message,omitempty"`
}

// color_pipeline_apply

type ColorPipelineApplyInput struct {
	UseWallpaper bool `json:"use_wallpaper,omitempty" jsonschema:"description=Extract accent colors from current wallpaper via matugen (default: false — use fixed palette.env)"`
	NoReload     bool `json:"no_reload,omitempty" jsonschema:"description=Skip post-render reload hooks (default: false — fire reloads)"`
	DryRun       bool `json:"dry_run,omitempty" jsonschema:"description=Report targets without writing files (default: false)"`
}

type ColorPipelineApplyOutput struct {
	Success     bool   `json:"success"`
	Output      string `json:"output,omitempty"`
	TargetCount int    `json:"target_count"`
}

// shader_tier

type ShaderTierInput struct {
	Tier       string `json:"tier,omitempty" jsonschema:"enum=cheap,enum=mid,enum=heavy,description=Filter by perf tier; omit to return all three"`
	Regenerate bool   `json:"regenerate,omitempty" jsonschema:"description=Regenerate tier playlists from the current shader catalog before returning"`
}

type ShaderTierEntry struct {
	Tier    string   `json:"tier"`
	Count   int      `json:"count"`
	Shaders []string `json:"shaders"`
}

type ShaderTierOutput struct {
	Tiers []ShaderTierEntry `json:"tiers"`
	Total int               `json:"total"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func readLinesTrim(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s != "" && !strings.HasPrefix(s, "#") {
			lines = append(lines, s)
		}
	}
	return lines, sc.Err()
}

// hyprctlOut runs `hyprctl <args...>` and returns combined output.
func hyprctlOut(args ...string) (string, error) {
	cmd := exec.Command("hyprctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("hyprctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// ---------------------------------------------------------------------------
// Tools
// ---------------------------------------------------------------------------

func (m *WaylandPerfModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{

		// ── hypr_perf_mode ─────────────────────────────
		handler.TypedHandler[HyprPerfModeInput, HyprPerfModeOutput](
			"hypr_perf_mode",
			"Get or toggle the Hyprland perf-mode profile (quality or performance). `performance` reduces blur, disables shadows, enables VFR, and turns on the frame overlay — useful for 240Hz tuning on NVIDIA. `auto` toggles between the two. Omit `mode` to read current state.",
			func(_ context.Context, input HyprPerfModeInput) (HyprPerfModeOutput, error) {
				script := perfModeScript()
				if _, err := os.Stat(script); err != nil {
					return HyprPerfModeOutput{}, fmt.Errorf("perf-mode script missing: %s", script)
				}
				mode := strings.ToLower(strings.TrimSpace(input.Mode))
				if mode == "" {
					mode = "status"
				}
				cmd := exec.Command("bash", script, mode)
				out, err := cmd.CombinedOutput()
				if err != nil {
					return HyprPerfModeOutput{}, fmt.Errorf("hypr-perf-mode.sh %s: %w: %s", mode, err, string(out))
				}
				return HyprPerfModeOutput{
					Mode:       mode,
					Applied:    mode != "status",
					ScriptPath: script,
					Output:     strings.TrimSpace(string(out)),
				}, nil
			},
		),

		// ── hypr_vrr_status ────────────────────────────
		handler.TypedHandler[HyprVRRStatusInput, HyprVRRStatusOutput](
			"hypr_vrr_status",
			"Report per-monitor VRR state via `hyprctl monitors -j`. Returns name, description, resolution, refresh rate, scale, and VRR enablement for each connected monitor.",
			func(_ context.Context, _ HyprVRRStatusInput) (HyprVRRStatusOutput, error) {
				out, err := hyprctlOut("monitors", "-j")
				if err != nil {
					return HyprVRRStatusOutput{}, err
				}
				var raw []map[string]any
				if err := json.Unmarshal([]byte(out), &raw); err != nil {
					return HyprVRRStatusOutput{}, fmt.Errorf("parse hyprctl monitors: %w", err)
				}
				var monitors []HyprMonitorVRR
				for _, r := range raw {
					mm := HyprMonitorVRR{}
					if v, ok := r["name"].(string); ok {
						mm.Name = v
					}
					if v, ok := r["description"].(string); ok {
						mm.Description = v
					}
					if v, ok := r["refreshRate"].(float64); ok {
						mm.RefreshRate = v
					}
					if v, ok := r["width"].(float64); ok {
						mm.Width = int(v)
					}
					if v, ok := r["height"].(float64); ok {
						mm.Height = int(v)
					}
					if v, ok := r["scale"].(float64); ok {
						mm.Scale = v
					}
					if v, ok := r["vrr"].(bool); ok {
						mm.VRR = v
					}
					monitors = append(monitors, mm)
				}
				return HyprVRRStatusOutput{Monitors: monitors, Count: len(monitors)}, nil
			},
		),

		// ── hypr_vrr_set ───────────────────────────────
		handler.TypedHandler[HyprVRRSetInput, HyprVRRSetOutput](
			"hypr_vrr_set",
			"Set per-monitor VRR mode via `hyprctl keyword monitor`. Read the current monitor spec first so mode/position/scale stay intact, then issue the full line with the new `vrr,<mode>` tail. On NVIDIA 590.48.01 this is safe at runtime (A/B tested) but not at compositor startup — see .claude/rules/nvidia-wayland.md. Watch `journalctl -k --since '10 sec ago'` for planePitch errors after each flip.",
			func(_ context.Context, input HyprVRRSetInput) (HyprVRRSetOutput, error) {
				out := HyprVRRSetOutput{Monitor: input.Monitor, Mode: input.Mode}
				if input.Monitor == "" {
					return out, fmt.Errorf("[%s] monitor is required", handler.ErrInvalidParam)
				}
				if input.Mode < 0 || input.Mode > 2 {
					return out, fmt.Errorf("[%s] mode must be 0, 1, or 2", handler.ErrInvalidParam)
				}

				rawJSON, err := hyprctlOut("monitors", "-j")
				if err != nil {
					return out, err
				}
				var monitors []map[string]any
				if err := json.Unmarshal([]byte(rawJSON), &monitors); err != nil {
					return out, fmt.Errorf("parse hyprctl monitors: %w", err)
				}
				var found bool
				var mode, pos, scale string
				for _, m := range monitors {
					name, _ := m["name"].(string)
					if name != input.Monitor {
						continue
					}
					found = true
					w, _ := m["width"].(float64)
					h, _ := m["height"].(float64)
					rate, _ := m["refreshRate"].(float64)
					x, _ := m["x"].(float64)
					y, _ := m["y"].(float64)
					sc, _ := m["scale"].(float64)
					mode = fmt.Sprintf("%dx%d@%.0f", int(w), int(h), rate)
					pos = fmt.Sprintf("%dx%d", int(x), int(y))
					scale = fmt.Sprintf("%.2f", sc)
					break
				}
				if !found {
					return out, fmt.Errorf("[%s] monitor %q not connected", handler.ErrInvalidParam, input.Monitor)
				}

				arg := fmt.Sprintf("%s,%s,%s,%s,vrr,%d", input.Monitor, mode, pos, scale, input.Mode)
				out.MonitorArg = arg
				stdout, err := hyprctlOut("keyword", "monitor", arg)
				out.Output = strings.TrimSpace(stdout)
				if err != nil {
					return out, err
				}
				out.Applied = true
				return out, nil
			},
		),

		// ── hypr_frame_overlay ─────────────────────────
		handler.TypedHandler[HyprFrameOverlayInput, HyprFrameOverlayOutput](
			"hypr_frame_overlay",
			"Toggle the Hyprland compositor FPS/frametime overlay (debug:overlay). Pass enable=true/false to set; omit to read current state.",
			func(_ context.Context, input HyprFrameOverlayInput) (HyprFrameOverlayOutput, error) {
				// Read current state
				cur, err := hyprctlOut("getoption", "debug:overlay", "-j")
				if err != nil {
					return HyprFrameOverlayOutput{}, err
				}
				var parsed map[string]any
				_ = json.Unmarshal([]byte(cur), &parsed)
				currentEnabled := false
				if v, ok := parsed["int"].(float64); ok && v != 0 {
					currentEnabled = true
				}
				if input.Enable == nil {
					return HyprFrameOverlayOutput{Enabled: currentEnabled, Changed: false}, nil
				}
				want := *input.Enable
				desiredInt := "0"
				if want {
					desiredInt = "1"
				}
				if currentEnabled == want {
					return HyprFrameOverlayOutput{
						Enabled: currentEnabled,
						Changed: false,
						Message: "already in desired state",
					}, nil
				}
				if _, err := hyprctlOut("keyword", "debug:overlay", desiredInt); err != nil {
					return HyprFrameOverlayOutput{}, err
				}
				return HyprFrameOverlayOutput{Enabled: want, Changed: true}, nil
			},
		),

		// ── color_pipeline_apply ───────────────────────
		handler.TypedHandler[ColorPipelineApplyInput, ColorPipelineApplyOutput](
			"color_pipeline_apply",
			"Render all palette templates via scripts/palette-propagate.sh — fans out Hairglasses Neon to 12 consumers (5 GTK CSS + kitty + hyprland + hyprlock + btop + yazi + zsh-fzf + cava). Pass use_wallpaper=true for matugen-derived accents from current swww wallpaper.",
			func(_ context.Context, input ColorPipelineApplyInput) (ColorPipelineApplyOutput, error) {
				script := palettePropagateScript()
				if _, err := os.Stat(script); err != nil {
					return ColorPipelineApplyOutput{}, fmt.Errorf("palette-propagate script missing: %s", script)
				}
				args := []string{script}
				if input.DryRun {
					args = append(args, "--dry-run")
				}
				if input.NoReload {
					args = append(args, "--no-reload")
				}
				if input.UseWallpaper {
					args = append(args, "--wallpaper")
				}
				cmd := exec.Command("bash", args...)
				out, err := cmd.CombinedOutput()
				output := strings.TrimSpace(string(out))
				if err != nil {
					return ColorPipelineApplyOutput{Success: false, Output: output}, fmt.Errorf("palette-propagate.sh: %w", err)
				}
				// Count "write" or "dry" lines to report consumer count
				count := 0
				for _, line := range strings.Split(output, "\n") {
					s := strings.TrimSpace(line)
					if strings.Contains(s, "[write]") || strings.Contains(s, "[dry]") {
						count++
					}
				}
				return ColorPipelineApplyOutput{Success: true, Output: output, TargetCount: count}, nil
			},
		),

		// ── shader_tier ────────────────────────────────
		handler.TypedHandler[ShaderTierInput, ShaderTierOutput](
			"shader_tier",
			"List DarkWindow shaders grouped by perf tier (cheap/mid/heavy). Filter by tier, or pass regenerate=true to rebuild playlists from the current catalog first.",
			func(_ context.Context, input ShaderTierInput) (ShaderTierOutput, error) {
				if input.Regenerate {
					script := shaderTierScript()
					if _, err := os.Stat(script); err != nil {
						return ShaderTierOutput{}, fmt.Errorf("shader-tier script missing: %s", script)
					}
					cmd := exec.Command("bash", script, "generate")
					if out, err := cmd.CombinedOutput(); err != nil {
						return ShaderTierOutput{}, fmt.Errorf("shader-tier generate: %w: %s", err, string(out))
					}
				}
				tiers := []string{"cheap", "mid", "heavy"}
				if input.Tier != "" {
					tiers = []string{strings.ToLower(input.Tier)}
				}
				var entries []ShaderTierEntry
				total := 0
				for _, tier := range tiers {
					path := tierPlaylistPath(tier)
					shaders, err := readLinesTrim(path)
					if err != nil {
						if os.IsNotExist(err) {
							return ShaderTierOutput{}, fmt.Errorf("tier playlist missing: %s — run with regenerate=true", path)
						}
						return ShaderTierOutput{}, err
					}
					sort.Strings(shaders)
					entries = append(entries, ShaderTierEntry{
						Tier:    tier,
						Count:   len(shaders),
						Shaders: shaders,
					})
					total += len(shaders)
				}
				return ShaderTierOutput{Tiers: entries, Total: total}, nil
			},
		),
	}
}
