// mod_hyprland.go — Hyprland desktop control tools (migrated from hyprland-mcp)
package dotfiles

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------- environment helpers ----------

// hyprInstanceSig finds the Hyprland instance signature for IPC.
func hyprInstanceSig() string {
	if sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); sig != "" {
		return sig
	}
	return dotfilesHyprlandSignature(dotfilesRuntimeDir())
}

// hyprctlCmd builds an exec.Cmd for hyprctl, inheriting the Wayland env.
func hyprctlCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("hyprctl", args...)
	// Inherit env — HYPRLAND_INSTANCE_SIGNATURE, WAYLAND_DISPLAY, XDG_RUNTIME_DIR
	// are passed through from the parent process environment.
	return cmd
}

// runCmd executes a command and returns its combined output.
func runHyprCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s failed: %w: %s", name, err, string(out))
	}
	return string(out), nil
}

// runHyprctl executes a hyprctl command and returns its output.
func runHyprctl(args ...string) (string, error) {
	cmd := hyprctlCmd(args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("hyprctl %s failed: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

func appendHyprJSONStrings(dst []string, value any) []string {
	switch v := value.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v != "" {
			dst = append(dst, v)
		}
	case []any:
		for _, item := range v {
			dst = appendHyprJSONStrings(dst, item)
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			dst = appendHyprJSONStrings(dst, v[key])
		}
	}
	return dst
}

func hyprConfigErrorMessages(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return appendHyprJSONStrings(nil, parsed)
	}

	if strings.Contains(strings.ToLower(raw), "no errors") {
		return nil
	}

	var messages []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			messages = append(messages, line)
		}
	}
	return messages
}

func hyprConfigHealthy(raw string) bool {
	return len(hyprConfigErrorMessages(raw)) == 0
}

// ---------- input/output types ----------

type ScreenshotInput struct {
	Output string `json:"output,omitempty" jsonschema:"description=Monitor output name (e.g. DP-1). Omit for all outputs."`
}

type EmptyInput struct{}

type FocusWindowInput struct {
	Address string `json:"address,omitempty" jsonschema:"description=Window address (hex) from hypr_list_windows"`
	Class   string `json:"class,omitempty" jsonschema:"description=Window class name to focus"`
}

type SwitchWorkspaceInput struct {
	ID int `json:"id" jsonschema:"required,description=Workspace ID to switch to"`
}

type ClickInput struct {
	X      int    `json:"x" jsonschema:"required,description=X coordinate"`
	Y      int    `json:"y" jsonschema:"required,description=Y coordinate"`
	Button string `json:"button,omitempty" jsonschema:"description=Mouse button (left right middle). Defaults to left."`
}

type TypeTextInput struct {
	Text string `json:"text" jsonschema:"required,description=Text to type"`
}

type KeyInput struct {
	Keys string `json:"keys" jsonschema:"required,description=Key combo for ydotool (e.g. 29:1 29:0 for ctrl tap)"`
}

type SetMonitorInput struct {
	Name       string `json:"name" jsonschema:"required,description=Monitor output name (e.g. DP-1)"`
	Resolution string `json:"resolution,omitempty" jsonschema:"description=Resolution and refresh rate (e.g. 5120x1440@120)"`
	Position   string `json:"position,omitempty" jsonschema:"description=Position on the virtual desktop (e.g. 0x0)"`
	Scale      string `json:"scale,omitempty" jsonschema:"description=Scale factor (e.g. 1 or 1.5 or 2)"`
}

type monitorInfo struct {
	Name            string `json:"name"`
	Resolution      string `json:"resolution"`
	RefreshRate     string `json:"refreshRate"`
	Position        string `json:"position"`
	Scale           string `json:"scale"`
	ActiveWorkspace int    `json:"activeWorkspace"`
	Focused         bool   `json:"focused"`
}

type monitorsResult struct {
	Monitors []monitorInfo `json:"monitors"`
}

// ---------- hyprctl JSON types ----------

type hyprClient struct {
	Address   string `json:"address"`
	At        [2]int `json:"at"`
	Size      [2]int `json:"size"`
	Class     string `json:"class"`
	Title     string `json:"title"`
	Monitor   int    `json:"monitor"`
	Workspace struct {
		ID int `json:"id"`
	} `json:"workspace"`
}

type hyprMonitor struct {
	ID    int     `json:"id"`
	Scale float64 `json:"scale"`
}

// windowRegion calculates the physical pixel region for screenshot capture from logical
// coordinates and a monitor scale factor.
func windowRegion(x, y, w, h int, scale float64) (px, py, pw, ph int) {
	px = int(math.Round(float64(x) * scale))
	py = int(math.Round(float64(y) * scale))
	pw = int(math.Round(float64(w) * scale))
	ph = int(math.Round(float64(h) * scale))
	return
}

// ---------- window resolution ----------

// resolveHyprWindow finds a window by address (exact match) or class (first
// case-insensitive match) and returns the canonical "address:0x..." selector
// string usable in hyprctl dispatch commands. The clientsJSON parameter, if
// non-empty, is used directly instead of calling hyprctl (useful for testing).
func resolveHyprWindow(address, class, clientsJSON string) (string, error) {
	if address == "" && class == "" {
		return "", fmt.Errorf("[%s] must specify either address or class", handler.ErrInvalidParam)
	}

	if clientsJSON == "" {
		out, err := runHyprctl("clients", "-j")
		if err != nil {
			return "", fmt.Errorf("hyprctl clients failed: %w", err)
		}
		clientsJSON = out
	}

	var clients []hyprClient
	if err := json.Unmarshal([]byte(clientsJSON), &clients); err != nil {
		return "", fmt.Errorf("parse clients: %w", err)
	}

	// Address match takes priority — search it first across all clients.
	if address != "" {
		for i := range clients {
			if clients[i].Address == address {
				return "address:" + clients[i].Address, nil
			}
		}
	}

	// Fall back to class match (first case-insensitive match).
	if class != "" {
		for i := range clients {
			if strings.EqualFold(clients[i].Class, class) {
				return "address:" + clients[i].Address, nil
			}
		}
	}

	selector := address
	if selector == "" {
		selector = class
	}
	return "", fmt.Errorf("window not found: %s", selector)
}

// ---------- module ----------

type HyprlandModule struct{}

func (m *HyprlandModule) Name() string        { return "hyprland" }
func (m *HyprlandModule) Description() string { return "Hyprland desktop control tools" }

func (m *HyprlandModule) Tools() []registry.ToolDefinition {
	tools := []registry.ToolDefinition{
		// ── hypr_screenshot ────────────────────────────
		// Raw handler because TypedHandler marshals output as JSON text,
		// but screenshots need to return mcp.ImageContent directly.
		{
			Tool: mcp.Tool{
				Name:        "hypr_screenshot",
				Description: "Capture a screenshot. Specify a monitor name for a single output, or omit to capture all monitors combined. For per-monitor detail (especially menubars), use hypr_screenshot_monitors instead.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"output": map[string]any{
							"type":        "string",
							"description": "Monitor output name (e.g. DP-1, HDMI-A-1). Omit to capture all outputs.",
						},
					},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				var input ScreenshotInput
				if req.Params.Arguments != nil {
					b, _ := json.Marshal(req.Params.Arguments)
					_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
				}

				raw := "/tmp/hypr-screenshot-raw.png"
				resized := "/tmp/hypr-screenshot-resized.png"
				defer os.Remove(raw)
				defer os.Remove(resized)

				// Capture screenshot
				if err := screenshotCapture(raw, "", input.Output); err != nil {
					return handler.ErrorResult(fmt.Errorf("screenshot capture failed: %w", err)), nil
				}

				// Resize with ImageMagick (preserve aspect ratio, max 1568x1568)
				if _, err := runHyprCmd("magick", raw, "-resize", "1568x1568>", resized); err != nil {
					return handler.ErrorResult(fmt.Errorf("magick resize failed: %w", err)), nil
				}

				// Read and base64 encode
				data, err := os.ReadFile(resized)
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("failed to read screenshot: %w", err)), nil
				}
				b64 := base64.StdEncoding.EncodeToString(data)

				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.ImageContent{
							Type:     "image",
							Data:     b64,
							MIMEType: "image/png",
						},
					},
				}, nil
			},
		},

		// ── hypr_screenshot_monitors ───────────────────
		// Captures each monitor separately for higher detail (especially menubars).
		{
			Tool: mcp.Tool{
				Name:        "hypr_screenshot_monitors",
				Description: "Capture separate screenshots of each monitor. Returns multiple images, one per active monitor, each resized individually for better detail than a single combined capture. Also captures a cropped menubar view (top 48px) of each monitor for bar inspection.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(_ context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				// Get monitor list
				monsJSON, err := runHyprctl("monitors", "-j")
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("hyprctl monitors failed: %w", err)), nil
				}

				var monitors []struct {
					Name   string  `json:"name"`
					Width  int     `json:"width"`
					Height int     `json:"height"`
					Scale  float64 `json:"scale"`
				}
				if err := json.Unmarshal([]byte(monsJSON), &monitors); err != nil {
					return handler.ErrorResult(fmt.Errorf("parse monitors: %w", err)), nil
				}

				var content []mcp.Content

				for _, mon := range monitors {
					// Full monitor screenshot
					full := fmt.Sprintf("/tmp/hypr-mon-%s-full.png", mon.Name)
					resized := fmt.Sprintf("/tmp/hypr-mon-%s-resized.png", mon.Name)
					bar := fmt.Sprintf("/tmp/hypr-mon-%s-bar.png", mon.Name)

					if err := screenshotCapture(full, "", mon.Name); err != nil {
						content = append(content, mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("%s: capture failed: %v", mon.Name, err),
						})
						continue
					}

					// Resize full monitor to reasonable size (max 1568px wide)
					runHyprCmd("magick", full, "-resize", "1568x1568>", resized)

					// Crop top bar region (48 logical px × scale factor for physical px)
					scale := mon.Scale
					if scale < 1 {
						scale = 1
					}
					barHeight := int(48 * scale)
					runHyprCmd("magick", full, "-crop", fmt.Sprintf("%dx%d+0+0", mon.Width, barHeight), "-resize", "1568x>", bar)

					// Add full monitor image
					if data, err := os.ReadFile(resized); err == nil {
						content = append(content, mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("── %s (%dx%d) ──", mon.Name, mon.Width, mon.Height),
						})
						content = append(content, mcp.ImageContent{
							Type:     "image",
							Data:     base64.StdEncoding.EncodeToString(data),
							MIMEType: "image/png",
						})
					}

					// Add menubar crop
					if data, err := os.ReadFile(bar); err == nil {
						content = append(content, mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("── %s menubar (top 48px) ──", mon.Name),
						})
						content = append(content, mcp.ImageContent{
							Type:     "image",
							Data:     base64.StdEncoding.EncodeToString(data),
							MIMEType: "image/png",
						})
					}

					// Cleanup
					os.Remove(full)
					os.Remove(resized)
					os.Remove(bar)
				}

				return &registry.CallToolResult{Content: content}, nil
			},
		},

		// ── hypr_list_windows ─────────────────────────
		handler.TypedHandler[EmptyInput, string](
			"hypr_list_windows",
			"List all windows managed by Hyprland with their address, title, class, workspace, size, position, and focus state.",
			func(_ context.Context, _ EmptyInput) (string, error) {
				out, err := runHyprctl("clients", "-j")
				if err != nil {
					return "", err
				}

				var clients []map[string]interface{}
				if err := json.Unmarshal([]byte(out), &clients); err != nil {
					return "", fmt.Errorf("failed to parse clients JSON: %w", err)
				}

				type windowInfo struct {
					Address   string `json:"address"`
					Title     string `json:"title"`
					Class     string `json:"class"`
					Workspace int    `json:"workspace"`
					Size      [2]int `json:"size"`
					Position  [2]int `json:"position"`
					Mapped    bool   `json:"mapped"`
					Focused   bool   `json:"focused"`
					Floating  bool   `json:"floating"`
				}

				var windows []windowInfo
				for _, c := range clients {
					w := windowInfo{}
					if v, ok := c["address"].(string); ok {
						w.Address = v
					}
					if v, ok := c["title"].(string); ok {
						w.Title = v
					}
					if v, ok := c["class"].(string); ok {
						w.Class = v
					}
					if ws, ok := c["workspace"].(map[string]interface{}); ok {
						if id, ok := ws["id"].(float64); ok {
							w.Workspace = int(id)
						}
					}
					if v, ok := c["size"].([]interface{}); ok && len(v) == 2 {
						if x, ok := v[0].(float64); ok {
							w.Size[0] = int(x)
						}
						if y, ok := v[1].(float64); ok {
							w.Size[1] = int(y)
						}
					}
					if v, ok := c["at"].([]interface{}); ok && len(v) == 2 {
						if x, ok := v[0].(float64); ok {
							w.Position[0] = int(x)
						}
						if y, ok := v[1].(float64); ok {
							w.Position[1] = int(y)
						}
					}
					if v, ok := c["mapped"].(bool); ok {
						w.Mapped = v
					}
					if v, ok := c["focusHistoryID"].(float64); ok {
						w.Focused = int(v) == 0
					}
					if v, ok := c["floating"].(bool); ok {
						w.Floating = v
					}
					windows = append(windows, w)
				}

				b, _ := json.MarshalIndent(windows, "", "  ")
				return string(b), nil
			},
		),

		// ── hypr_focus_window ─────────────────────────
		handler.TypedHandler[FocusWindowInput, string](
			"hypr_focus_window",
			"Focus a window by its address or class name.",
			func(_ context.Context, input FocusWindowInput) (string, error) {
				selector, err := resolveHyprWindow(input.Address, input.Class, "")
				if err != nil {
					return "", err
				}

				out, err := runHyprctl("dispatch", "focuswindow", selector)
				if err != nil {
					return "", err
				}
				return strings.TrimSpace(out), nil
			},
		),

		// ── hypr_list_workspaces ──────────────────────
		handler.TypedHandler[EmptyInput, string](
			"hypr_list_workspaces",
			"List all Hyprland workspaces with window count, monitor, and focused status.",
			func(_ context.Context, _ EmptyInput) (string, error) {
				wsOut, err := runHyprctl("workspaces", "-j")
				if err != nil {
					return "", err
				}
				activeOut, err := runHyprctl("activeworkspace", "-j")
				if err != nil {
					return "", err
				}

				var workspaces []map[string]interface{}
				if err := json.Unmarshal([]byte(wsOut), &workspaces); err != nil {
					return "", fmt.Errorf("failed to parse workspaces: %w", err)
				}

				var active map[string]interface{}
				if err := json.Unmarshal([]byte(activeOut), &active); err != nil {
					return "", fmt.Errorf("failed to parse active workspace: %w", err)
				}

				activeID := -1
				if id, ok := active["id"].(float64); ok {
					activeID = int(id)
				}

				type workspaceInfo struct {
					ID      int    `json:"id"`
					Name    string `json:"name"`
					Monitor string `json:"monitor"`
					Windows int    `json:"windows"`
					Focused bool   `json:"focused"`
				}

				var result []workspaceInfo
				for _, ws := range workspaces {
					w := workspaceInfo{}
					if v, ok := ws["id"].(float64); ok {
						w.ID = int(v)
					}
					if v, ok := ws["name"].(string); ok {
						w.Name = v
					}
					if v, ok := ws["monitor"].(string); ok {
						w.Monitor = v
					}
					if v, ok := ws["windows"].(float64); ok {
						w.Windows = int(v)
					}
					w.Focused = w.ID == activeID
					result = append(result, w)
				}

				b, _ := json.MarshalIndent(result, "", "  ")
				return string(b), nil
			},
		),

		// ── hypr_switch_workspace ─────────────────────
		handler.TypedHandler[SwitchWorkspaceInput, string](
			"hypr_switch_workspace",
			"Switch to a workspace by ID.",
			func(_ context.Context, input SwitchWorkspaceInput) (string, error) {
				out, err := runHyprctl("dispatch", "workspace", strconv.Itoa(input.ID))
				if err != nil {
					return "", err
				}
				return strings.TrimSpace(out), nil
			},
		),

		// ── hypr_reload_config ────────────────────────
		handler.TypedHandler[EmptyInput, string](
			"hypr_reload_config",
			"Reload the Hyprland configuration and check for config errors.",
			func(_ context.Context, _ EmptyInput) (string, error) {
				reloadOut, err := runHyprctl("reload")
				if err != nil {
					return "", err
				}

				errorsOut, configErr := runHyprctl("-j", "configerrors")
				configSummary := "none"
				messages := hyprConfigErrorMessages(errorsOut)
				if len(messages) > 0 {
					configSummary = strings.Join(messages, "; ")
				} else if configErr != nil && strings.TrimSpace(errorsOut) == "" {
					configSummary = strings.TrimSpace(configErr.Error())
				}

				result := fmt.Sprintf("reload: %s\nconfigerrors: %s", strings.TrimSpace(reloadOut), configSummary)
				return result, nil
			},
		),

		// ── hypr_click ────────────────────────────────
		handler.TypedHandler[ClickInput, string](
			"hypr_click",
			"Click at absolute screen coordinates using ydotool.",
			func(_ context.Context, input ClickInput) (string, error) {
				// Map button name to ydotool button code
				// ydotool uses Linux input event codes: BTN_LEFT=0x110, BTN_RIGHT=0x111, BTN_MIDDLE=0x112
				buttonCode := "0x110" // left
				switch strings.ToLower(input.Button) {
				case "right":
					buttonCode = "0x111"
				case "middle":
					buttonCode = "0x112"
				}

				// Move cursor then click
				if _, err := runHyprCmd("ydotool", "mousemove", "--absolute", "-x", strconv.Itoa(input.X), "-y", strconv.Itoa(input.Y)); err != nil {
					return "", fmt.Errorf("mousemove failed: %w", err)
				}

				if _, err := runHyprCmd("ydotool", "click", "--next-delay", "0", buttonCode); err != nil {
					return "", fmt.Errorf("click failed: %w", err)
				}

				return fmt.Sprintf("clicked %s at (%d, %d)", input.Button, input.X, input.Y), nil
			},
		),

		// ── hypr_type_text ────────────────────────────
		handler.TypedHandler[TypeTextInput, string](
			"hypr_type_text",
			"Type text at the current cursor position using wtype (Wayland).",
			func(_ context.Context, input TypeTextInput) (string, error) {
				if input.Text == "" {
					return "", fmt.Errorf("[%s] text must not be empty", handler.ErrInvalidParam)
				}

				out, err := runHyprCmd("wtype", input.Text)
				if err != nil {
					return "", fmt.Errorf("wtype failed: %w", err)
				}
				return fmt.Sprintf("typed %d chars. %s", len(input.Text), strings.TrimSpace(out)), nil
			},
		),

		// ── hypr_key ──────────────────────────────────
		handler.TypedHandler[KeyInput, string](
			"hypr_key",
			"Send key events using ydotool. Use Linux input event codes (e.g. '29:1 29:0' for Ctrl tap, '56:1 31:1 31:0 56:0' for Alt+S).",
			func(_ context.Context, input KeyInput) (string, error) {
				if input.Keys == "" {
					return "", fmt.Errorf("[%s] keys must not be empty", handler.ErrInvalidParam)
				}

				args := append([]string{"key"}, strings.Fields(input.Keys)...)
				out, err := runHyprCmd("ydotool", args...)
				if err != nil {
					return "", fmt.Errorf("ydotool key failed: %w", err)
				}
				return fmt.Sprintf("sent keys: %s. %s", input.Keys, strings.TrimSpace(out)), nil
			},
		),

		// ── hypr_get_monitors ─────────────────────────
		handler.TypedHandler[EmptyInput, monitorsResult](
			"hypr_get_monitors",
			"List connected monitors with resolution, refresh rate, position, scale, and active workspace.",
			func(_ context.Context, _ EmptyInput) (monitorsResult, error) {
				out, err := runHyprctl("monitors", "-j")
				if err != nil {
					return monitorsResult{}, err
				}

				var monitors []map[string]interface{}
				if err := json.Unmarshal([]byte(out), &monitors); err != nil {
					return monitorsResult{}, fmt.Errorf("failed to parse monitors JSON: %w", err)
				}

				var result []monitorInfo
				for _, m := range monitors {
					mi := monitorInfo{}
					if v, ok := m["name"].(string); ok {
						mi.Name = v
					}
					w, h := 0, 0
					if v, ok := m["width"].(float64); ok {
						w = int(v)
					}
					if v, ok := m["height"].(float64); ok {
						h = int(v)
					}
					mi.Resolution = fmt.Sprintf("%dx%d", w, h)
					if v, ok := m["refreshRate"].(float64); ok {
						mi.RefreshRate = fmt.Sprintf("%.2f", v)
					}
					x, y := 0, 0
					if v, ok := m["x"].(float64); ok {
						x = int(v)
					}
					if v, ok := m["y"].(float64); ok {
						y = int(v)
					}
					mi.Position = fmt.Sprintf("%dx%d", x, y)
					if v, ok := m["scale"].(float64); ok {
						mi.Scale = fmt.Sprintf("%.2f", v)
					}
					if ws, ok := m["activeWorkspace"].(map[string]interface{}); ok {
						if id, ok := ws["id"].(float64); ok {
							mi.ActiveWorkspace = int(id)
						}
					}
					if v, ok := m["focused"].(bool); ok {
						mi.Focused = v
					}
					result = append(result, mi)
				}

				return monitorsResult{Monitors: result}, nil
			},
		),

		// ── hypr_set_monitor ──────────────────────────
		handler.TypedHandler[SetMonitorInput, string](
			"hypr_set_monitor",
			"Configure a monitor's resolution, position, or scale.",
			func(_ context.Context, input SetMonitorInput) (string, error) {
				if input.Name == "" {
					return "", fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}

				// Query current monitor values to fill in omitted parameters
				resolution := input.Resolution
				position := input.Position
				scale := input.Scale

				if resolution == "" || position == "" || scale == "" {
					out, err := runHyprctl("monitors", "-j")
					if err != nil {
						return "", fmt.Errorf("failed to query current monitors: %w", err)
					}

					var monitors []map[string]interface{}
					if err := json.Unmarshal([]byte(out), &monitors); err != nil {
						return "", fmt.Errorf("failed to parse monitors JSON: %w", err)
					}

					var found bool
					for _, m := range monitors {
						name, _ := m["name"].(string)
						if name != input.Name {
							continue
						}
						found = true
						if resolution == "" {
							w, _ := m["width"].(float64)
							h, _ := m["height"].(float64)
							rate, _ := m["refreshRate"].(float64)
							resolution = fmt.Sprintf("%dx%d@%.0f", int(w), int(h), rate)
						}
						if position == "" {
							x, _ := m["x"].(float64)
							y, _ := m["y"].(float64)
							position = fmt.Sprintf("%dx%d", int(x), int(y))
						}
						if scale == "" {
							s, _ := m["scale"].(float64)
							scale = fmt.Sprintf("%.2f", s)
						}
						break
					}
					if !found {
						return "", fmt.Errorf("monitor %q not found", input.Name)
					}
				}

				// hyprctl keyword monitor <name>,<resolution>,<position>,<scale>
				monitorArg := fmt.Sprintf("%s,%s,%s,%s", input.Name, resolution, position, scale)
				out, err := runHyprctl("keyword", "monitor", monitorArg)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("monitor %s configured: %s. %s", input.Name, monitorArg, strings.TrimSpace(out)), nil
			},
		),

		// ── hypr_move_window ──────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hypr_move_window",
				Description: "Move a window to exact pixel coordinates. Resolves window by address or class.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"address": map[string]any{
							"type":        "string",
							"description": "Window address (e.g. '0x5a3b2c1d'). Get from hypr_list_windows.",
						},
						"class": map[string]any{
							"type":        "string",
							"description": "Window class name (e.g. 'foot', 'firefox'). First match is used.",
						},
						"x": map[string]any{
							"type":        "integer",
							"description": "Target X coordinate in pixels.",
						},
						"y": map[string]any{
							"type":        "integer",
							"description": "Target Y coordinate in pixels.",
						},
					},
					Required: []string{"x", "y"},
				},
			},
			Handler: handleHyprMoveWindow,
		},

		// ── hypr_resize_window ───────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hypr_resize_window",
				Description: "Resize a window to exact pixel dimensions. Resolves window by address or class.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"address": map[string]any{
							"type":        "string",
							"description": "Window address (e.g. '0x5a3b2c1d'). Get from hypr_list_windows.",
						},
						"class": map[string]any{
							"type":        "string",
							"description": "Window class name (e.g. 'foot', 'firefox'). First match is used.",
						},
						"width": map[string]any{
							"type":        "integer",
							"description": "Target width in pixels.",
						},
						"height": map[string]any{
							"type":        "integer",
							"description": "Target height in pixels.",
						},
					},
					Required: []string{"width", "height"},
				},
			},
			Handler: handleHyprResizeWindow,
		},

		// ── hypr_close_window ────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hypr_close_window",
				Description: "Close a window. Resolves window by address or class.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"address": map[string]any{
							"type":        "string",
							"description": "Window address (e.g. '0x5a3b2c1d'). Get from hypr_list_windows.",
						},
						"class": map[string]any{
							"type":        "string",
							"description": "Window class name (e.g. 'foot', 'firefox'). First match is used.",
						},
					},
				},
			},
			Handler: handleHyprCloseWindow,
		},

		// ── hypr_toggle_floating ─────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hypr_toggle_floating",
				Description: "Toggle floating state of a window. Resolves window by address or class.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"address": map[string]any{
							"type":        "string",
							"description": "Window address (e.g. '0x5a3b2c1d'). Get from hypr_list_windows.",
						},
						"class": map[string]any{
							"type":        "string",
							"description": "Window class name (e.g. 'foot', 'firefox'). First match is used.",
						},
					},
				},
			},
			Handler: handleHyprToggleFloating,
		},

		// ── hypr_minimize_window ─────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hypr_minimize_window",
				Description: "Minimize a window by moving it to the special:minimized workspace. Resolves window by address or class.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"address": map[string]any{
							"type":        "string",
							"description": "Window address (e.g. '0x5a3b2c1d'). Get from hypr_list_windows.",
						},
						"class": map[string]any{
							"type":        "string",
							"description": "Window class name (e.g. 'foot', 'firefox'). First match is used.",
						},
					},
				},
			},
			Handler: handleHyprMinimizeWindow,
		},

		// ── hypr_fullscreen_window ───────────────────
		{
			Tool: mcp.Tool{
				Name:        "hypr_fullscreen_window",
				Description: "Toggle fullscreen for a window. Focuses the window first, then applies fullscreen. Resolves window by address or class.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"address": map[string]any{
							"type":        "string",
							"description": "Window address (e.g. '0x5a3b2c1d'). Get from hypr_list_windows.",
						},
						"class": map[string]any{
							"type":        "string",
							"description": "Window class name (e.g. 'foot', 'firefox'). First match is used.",
						},
						"mode": map[string]any{
							"type":        "integer",
							"description": "Fullscreen mode: 0 = real fullscreen, 1 = maximize (keep gaps/bar). Defaults to 0.",
							"default":     0,
						},
					},
				},
			},
			Handler: handleHyprFullscreenWindow,
		},

		// ── hypr_screenshot_window ────────────────────
		// Raw handler: returns mcp.ImageContent directly.
		{
			Tool: mcp.Tool{
				Name:        "hypr_screenshot_window",
				Description: "Take a screenshot of a specific window by address or class name. Returns the image inline for LLM vision.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"address": map[string]any{
							"type":        "string",
							"description": "Window address (e.g. '0x5a3b2c1d'). Get from hypr_list_windows.",
						},
						"class": map[string]any{
							"type":        "string",
							"description": "Window class name (e.g. 'foot', 'firefox'). First match is used.",
						},
						"max_size": map[string]any{
							"type":        "integer",
							"description": "Max dimension for resizing (default 1568). Smaller = fewer tokens.",
							"default":     1568,
						},
					},
				},
			},
			Handler:  handleHyprScreenshotWindow,
			Category: "hyprland",
		},

		// ── hypr_screenshot_region ────────────────────
		// Raw handler: returns mcp.ImageContent directly.
		{
			Tool: mcp.Tool{
				Name:        "hypr_screenshot_region",
				Description: "Capture a screenshot of a selected screen region using slurp + wayshot. Launches slurp for interactive region selection, then captures with wayshot.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"save": map[string]any{
							"type":        "boolean",
							"description": "Save the screenshot to a file (default true).",
							"default":     true,
						},
						"clipboard": map[string]any{
							"type":        "boolean",
							"description": "Copy the screenshot to the clipboard via wl-copy (default false).",
							"default":     false,
						},
					},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				var input struct {
					Save      *bool `json:"save"`
					Clipboard bool  `json:"clipboard"`
				}
				// Default save to true
				defaultSave := true
				input.Save = &defaultSave
				if req.Params.Arguments != nil {
					b, _ := json.Marshal(req.Params.Arguments)
					_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
				}
				doSave := input.Save == nil || *input.Save

				// Run slurp to get region geometry
				slurpCmd := exec.Command("slurp")
				slurpOut, err := slurpCmd.Output()
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("slurp failed (region selection cancelled or slurp not installed): %w", err)), nil
				}
				geometry := strings.TrimSpace(string(slurpOut))
				if geometry == "" {
					return handler.ErrorResult(fmt.Errorf("slurp returned empty geometry")), nil
				}

				ts := time.Now().UnixMilli()
				outPath := fmt.Sprintf("/tmp/hypr-region-%d.png", ts)
				if !doSave {
					defer os.Remove(outPath)
				}

				// Capture region to file using wayshot
				if err := screenshotCapture(outPath, geometry, ""); err != nil {
					return handler.ErrorResult(fmt.Errorf("wayshot region capture failed: %w", err)), nil
				}

				// Optionally copy to clipboard via wl-copy
				if input.Clipboard {
					wlCopyCmd := exec.Command("wl-copy", "--type", "image/png")
					f, openErr := os.Open(outPath)
					if openErr != nil {
						return handler.ErrorResult(fmt.Errorf("open screenshot for clipboard: %w", openErr)), nil
					}
					defer f.Close()
					wlCopyCmd.Stdin = f
					_ = wlCopyCmd.Run()
				}

				// Read and base64 encode for inline preview
				data, err := os.ReadFile(outPath)
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("failed to read region screenshot: %w", err)), nil
				}
				b64 := base64.StdEncoding.EncodeToString(data)

				caption := fmt.Sprintf("Region: %s", geometry)
				if doSave {
					caption += fmt.Sprintf(" — saved to %s", outPath)
				}
				if input.Clipboard {
					caption += " — copied to clipboard"
				}

				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{Type: "text", Text: caption},
						mcp.ImageContent{
							Type:     "image",
							Data:     b64,
							MIMEType: "image/png",
						},
					},
				}, nil
			},
			Category: "hyprland",
		},
	}
	tools = append(tools, hyprExtendedToolDefinitions()...)
	tools = append(tools, hyprPersistenceToolDefinitions()...)
	tools = append(tools, hyprConfigPersistenceTools()...)
	return tools
}

// ---------- window manipulation handlers ----------

func handleHyprMoveWindow(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
	var input struct {
		Address string  `json:"address"`
		Class   string  `json:"class"`
		X       float64 `json:"x"`
		Y       float64 `json:"y"`
	}
	if req.Params.Arguments != nil {
		b, _ := json.Marshal(req.Params.Arguments)
		_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
	}

	selector, err := resolveHyprWindow(input.Address, input.Class, "")
	if err != nil {
		return handler.ErrorResult(err), nil
	}

	arg := fmt.Sprintf("exact %d %d,%s", int(input.X), int(input.Y), selector)
	out, err := runHyprctl("dispatch", "movewindowpixel", arg)
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("movewindowpixel failed: %w", err)), nil
	}
	return &registry.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("moved window to (%d, %d): %s", int(input.X), int(input.Y), strings.TrimSpace(out)),
		}},
	}, nil
}

func handleHyprResizeWindow(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
	var input struct {
		Address string  `json:"address"`
		Class   string  `json:"class"`
		Width   float64 `json:"width"`
		Height  float64 `json:"height"`
	}
	if req.Params.Arguments != nil {
		b, _ := json.Marshal(req.Params.Arguments)
		_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
	}

	selector, err := resolveHyprWindow(input.Address, input.Class, "")
	if err != nil {
		return handler.ErrorResult(err), nil
	}

	arg := fmt.Sprintf("exact %d %d,%s", int(input.Width), int(input.Height), selector)
	out, err := runHyprctl("dispatch", "resizewindowpixel", arg)
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("resizewindowpixel failed: %w", err)), nil
	}
	return &registry.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("resized window to %dx%d: %s", int(input.Width), int(input.Height), strings.TrimSpace(out)),
		}},
	}, nil
}

func handleHyprCloseWindow(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
	var input struct {
		Address string `json:"address"`
		Class   string `json:"class"`
	}
	if req.Params.Arguments != nil {
		b, _ := json.Marshal(req.Params.Arguments)
		_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
	}

	selector, err := resolveHyprWindow(input.Address, input.Class, "")
	if err != nil {
		return handler.ErrorResult(err), nil
	}

	out, err := runHyprctl("dispatch", "closewindow", selector)
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("closewindow failed: %w", err)), nil
	}
	return &registry.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("closed window: %s", strings.TrimSpace(out)),
		}},
	}, nil
}

func handleHyprToggleFloating(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
	var input struct {
		Address string `json:"address"`
		Class   string `json:"class"`
	}
	if req.Params.Arguments != nil {
		b, _ := json.Marshal(req.Params.Arguments)
		_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
	}

	selector, err := resolveHyprWindow(input.Address, input.Class, "")
	if err != nil {
		return handler.ErrorResult(err), nil
	}

	out, err := runHyprctl("dispatch", "togglefloating", selector)
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("togglefloating failed: %w", err)), nil
	}
	return &registry.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("toggled floating: %s", strings.TrimSpace(out)),
		}},
	}, nil
}

func handleHyprMinimizeWindow(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
	var input struct {
		Address string `json:"address"`
		Class   string `json:"class"`
	}
	if req.Params.Arguments != nil {
		b, _ := json.Marshal(req.Params.Arguments)
		_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
	}

	selector, err := resolveHyprWindow(input.Address, input.Class, "")
	if err != nil {
		return handler.ErrorResult(err), nil
	}

	arg := fmt.Sprintf("special:minimized,%s", selector)
	out, err := runHyprctl("dispatch", "movetoworkspacesilent", arg)
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("minimize failed: %w", err)), nil
	}
	return &registry.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("minimized window: %s", strings.TrimSpace(out)),
		}},
	}, nil
}

func handleHyprFullscreenWindow(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
	var input struct {
		Address string  `json:"address"`
		Class   string  `json:"class"`
		Mode    float64 `json:"mode"`
	}
	if req.Params.Arguments != nil {
		b, _ := json.Marshal(req.Params.Arguments)
		_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
	}

	selector, err := resolveHyprWindow(input.Address, input.Class, "")
	if err != nil {
		return handler.ErrorResult(err), nil
	}

	// Focus the window first so fullscreen applies to it
	if _, err := runHyprctl("dispatch", "focuswindow", selector); err != nil {
		return handler.ErrorResult(fmt.Errorf("focuswindow failed: %w", err)), nil
	}

	mode := int(input.Mode)
	out, err := runHyprctl("dispatch", "fullscreen", strconv.Itoa(mode))
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("fullscreen failed: %w", err)), nil
	}
	modeLabel := "fullscreen"
	if mode == 1 {
		modeLabel = "maximize"
	}
	return &registry.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("toggled %s: %s", modeLabel, strings.TrimSpace(out)),
		}},
	}, nil
}

// ---------- hypr_screenshot_window handler ----------

func handleHyprScreenshotWindow(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
	var address, class string
	maxSize := 1568

	if req.Params.Arguments != nil {
		var input struct {
			Address string  `json:"address"`
			Class   string  `json:"class"`
			MaxSize float64 `json:"max_size"`
		}
		b, _ := json.Marshal(req.Params.Arguments)
		_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
		address = input.Address
		class = input.Class
		if input.MaxSize > 0 {
			maxSize = int(input.MaxSize)
		}
	}

	if address == "" && class == "" {
		return handler.ErrorResult(fmt.Errorf("[%s] must specify either address or class", handler.ErrInvalidParam)), nil
	}

	// 1. Get window list from hyprctl
	clientsJSON, err := runHyprctl("clients", "-j")
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("hyprctl clients failed: %w", err)), nil
	}

	var clients []hyprClient
	if err := json.Unmarshal([]byte(clientsJSON), &clients); err != nil {
		return handler.ErrorResult(fmt.Errorf("parse clients: %w", err)), nil
	}

	// 2. Find the target window
	var target *hyprClient
	for i := range clients {
		c := &clients[i]
		if address != "" && c.Address == address {
			target = c
			break
		}
		if class != "" && strings.EqualFold(c.Class, class) {
			target = c
			break
		}
	}
	if target == nil {
		selector := address
		if selector == "" {
			selector = class
		}
		return handler.ErrorResult(fmt.Errorf("window not found: %s", selector)), nil
	}

	// 3. Get monitor scale
	monsJSON, err := runHyprctl("monitors", "-j")
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("hyprctl monitors failed: %w", err)), nil
	}

	var monitors []hyprMonitor
	if err := json.Unmarshal([]byte(monsJSON), &monitors); err != nil {
		return handler.ErrorResult(fmt.Errorf("parse monitors: %w", err)), nil
	}

	scale := 1.0
	for _, m := range monitors {
		if m.ID == target.Monitor {
			scale = m.Scale
			break
		}
	}
	if scale < 1 {
		scale = 1
	}

	// 4. Calculate physical pixel region
	px, py, pw, ph := windowRegion(target.At[0], target.At[1], target.Size[0], target.Size[1], scale)
	region := fmt.Sprintf("%d,%d %dx%d", px, py, pw, ph)

	ts := time.Now().UnixMilli()
	raw := fmt.Sprintf("/tmp/window-screenshot-%d.png", ts)
	resized := fmt.Sprintf("/tmp/window-screenshot-%d-resized.png", ts)
	defer os.Remove(raw)
	defer os.Remove(resized)

	// 5. Capture screenshot
	if err := screenshotCapture(raw, region, ""); err != nil {
		return handler.ErrorResult(fmt.Errorf("screenshot capture failed (region %s): %w", region, err)), nil
	}

	// 6. Resize with ImageMagick
	resizeSpec := fmt.Sprintf("%dx%d>", maxSize, maxSize)
	if _, err := runHyprCmd("magick", raw, "-resize", resizeSpec, resized); err != nil {
		return handler.ErrorResult(fmt.Errorf("magick resize failed: %w", err)), nil
	}

	// 7. Read and base64 encode
	data, err := os.ReadFile(resized)
	if err != nil {
		return handler.ErrorResult(fmt.Errorf("failed to read screenshot: %w", err)), nil
	}
	b64 := base64.StdEncoding.EncodeToString(data)

	return &registry.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Window: %s (%s) — workspace %d, region %s, file %s",
					target.Class, target.Title, target.Workspace.ID, region, resized),
			},
			mcp.ImageContent{
				Type:     "image",
				Data:     b64,
				MIMEType: "image/png",
			},
		},
	}, nil
}

// ---------- main ----------

func init() {
	sig := hyprInstanceSig()
	if sig != "" {
		os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", sig)
	}
}
