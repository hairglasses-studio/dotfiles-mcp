// mod_hyprshade.go — Hyprshade screen shader management tools via the hyprshade CLI
package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// hyprshadeCheck verifies that the hyprshade binary is available on PATH.
func hyprshadeCheck() error {
	_, err := exec.LookPath("hyprshade")
	if err != nil {
		return fmt.Errorf("hyprshade not found on PATH — install it first (e.g. yay -S hyprshade)")
	}
	return nil
}

// hyprshadeRun executes a hyprshade sub-command and returns combined output.
func hyprshadeRun(args ...string) (string, error) {
	cmd := exec.Command("hyprshade", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("hyprshade %s failed: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// HyprshadeModule provides tools for managing Hyprshade screen shader state.
type HyprshadeModule struct{}

func (m *HyprshadeModule) Name() string { return "hyprshade" }
func (m *HyprshadeModule) Description() string {
	return "Manage Hyprshade screen shaders via the hyprshade CLI"
}

func (m *HyprshadeModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{

		// ── hyprshade_list ────────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hyprshade_list",
				Description: "List available Hyprshade screen shaders. Runs `hyprshade ls` and returns a structured list of shader names.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(_ context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := hyprshadeCheck(); err != nil {
					return handler.ErrorResult(err), nil
				}

				out, err := hyprshadeRun("ls")
				if err != nil {
					return handler.ErrorResult(err), nil
				}

				lines := strings.Split(strings.TrimSpace(out), "\n")
				var shaders []string
				for _, line := range lines {
					if s := strings.TrimSpace(line); s != "" {
						shaders = append(shaders, s)
					}
				}

				type listResult struct {
					Shaders []string `json:"shaders"`
					Count   int      `json:"count"`
				}
				result := listResult{
					Shaders: shaders,
					Count:   len(shaders),
				}
				if result.Shaders == nil {
					result.Shaders = []string{}
				}

				b, _ := json.Marshal(result)
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: string(b),
						},
					},
				}, nil
			},
		},

		// ── hyprshade_set ─────────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hyprshade_set",
				Description: "Activate a Hyprshade screen shader by name. Runs `hyprshade on <shader>`.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"shader": map[string]any{
							"type":        "string",
							"description": "Name of the shader to activate (as returned by hyprshade_list)",
						},
					},
					Required: []string{"shader"},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := hyprshadeCheck(); err != nil {
					return handler.ErrorResult(err), nil
				}

				var input struct {
					Shader string `json:"shader"`
				}
				if req.Params.Arguments != nil {
					b, _ := json.Marshal(req.Params.Arguments)
					_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
				}

				if strings.TrimSpace(input.Shader) == "" {
					return handler.CodedErrorResult(handler.ErrInvalidParam, fmt.Errorf("shader must not be empty")), nil
				}

				out, err := hyprshadeRun("on", input.Shader)
				if err != nil {
					return handler.ErrorResult(err), nil
				}

				msg := fmt.Sprintf("Activated shader %q", input.Shader)
				if s := strings.TrimSpace(out); s != "" {
					msg = fmt.Sprintf("%s: %s", msg, s)
				}
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: msg,
						},
					},
				}, nil
			},
		},

		// ── hyprshade_toggle ──────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hyprshade_toggle",
				Description: "Toggle a Hyprshade screen shader on or off by name. Runs `hyprshade toggle <shader>`.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"shader": map[string]any{
							"type":        "string",
							"description": "Name of the shader to toggle",
						},
					},
					Required: []string{"shader"},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := hyprshadeCheck(); err != nil {
					return handler.ErrorResult(err), nil
				}

				var input struct {
					Shader string `json:"shader"`
				}
				if req.Params.Arguments != nil {
					b, _ := json.Marshal(req.Params.Arguments)
					_ = json.Unmarshal(b, &input) // zero-value input on malformed args; downstream validation surfaces missing fields
				}

				if strings.TrimSpace(input.Shader) == "" {
					return handler.CodedErrorResult(handler.ErrInvalidParam, fmt.Errorf("shader must not be empty")), nil
				}

				out, err := hyprshadeRun("toggle", input.Shader)
				if err != nil {
					return handler.ErrorResult(err), nil
				}

				msg := fmt.Sprintf("Toggled shader %q", input.Shader)
				if s := strings.TrimSpace(out); s != "" {
					msg = fmt.Sprintf("%s: %s", msg, s)
				}
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: msg,
						},
					},
				}, nil
			},
		},

		// ── hyprshade_off ─────────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hyprshade_off",
				Description: "Disable the currently active Hyprshade screen shader. Runs `hyprshade off`.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(_ context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := hyprshadeCheck(); err != nil {
					return handler.ErrorResult(err), nil
				}

				out, err := hyprshadeRun("off")
				if err != nil {
					return handler.ErrorResult(err), nil
				}

				msg := "Screen shader disabled"
				if s := strings.TrimSpace(out); s != "" {
					msg = fmt.Sprintf("%s: %s", msg, s)
				}
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: msg,
						},
					},
				}, nil
			},
		},

		// ── hyprshade_status ──────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "hyprshade_status",
				Description: "Get the currently active Hyprshade screen shader. Runs `hyprshade current` and returns structured status.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(_ context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := hyprshadeCheck(); err != nil {
					return handler.ErrorResult(err), nil
				}

				out, err := hyprshadeRun("current")
				if err != nil {
					// hyprshade exits non-zero when no shader is active on some versions
					type statusResult struct {
						Active  bool   `json:"active"`
						Shader  string `json:"shader"`
						Message string `json:"message,omitempty"`
					}
					result := statusResult{Active: false, Shader: "", Message: "No shader currently active"}
					b, _ := json.Marshal(result)
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: string(b),
							},
						},
					}, nil
				}

				current := strings.TrimSpace(out)

				type statusResult struct {
					Active bool   `json:"active"`
					Shader string `json:"shader"`
				}
				result := statusResult{
					Active: current != "",
					Shader: current,
				}
				b, _ := json.Marshal(result)
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: string(b),
						},
					},
				}, nil
			},
		},
	}
}
