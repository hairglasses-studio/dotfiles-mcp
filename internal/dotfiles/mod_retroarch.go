// mod_retroarch.go — RetroArch workstation audit + runtime control tools.
//
// Exposes the existing Python scripts (retroarch-workstation-audit,
// retroarch-playlist-audit, retroarch-mounts-audit, retroarch-command)
// as first-class MCP tools so agent sessions can probe + dispatch
// without shelling out through the Bash tool.
package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// retroarchScript returns an absolute path to a `scripts/<name>` script
// inside the repo root, resolved via DOTFILES_DIR (same convention as
// the rest of the dotfiles-mcp modules).
func retroarchScript(name string) string {
	return filepath.Join(dotfilesDir(), "scripts", name)
}

// retroarchRunJSON executes a retroarch-* script with the given argv,
// expects a JSON stdout payload, and returns the parsed document or an
// error with stderr attached.
func retroarchRunJSON(ctx context.Context, script string, args ...string) (map[string]any, string, error) {
	cmd := exec.CommandContext(ctx, "python3", append([]string{retroarchScript(script)}, args...)...)
	out, err := cmd.Output()
	stdout := string(out)
	var stderr string
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr = string(exitErr.Stderr)
	}
	// Several retroarch scripts print the report path on stdout then
	// the summary line; the JSON payload lives at --report or
	// XDG_STATE_HOME. For the MCP tool we pass --json so the payload
	// comes back on stdout directly.
	parsed := map[string]any{}
	if stdout = strings.TrimSpace(stdout); stdout != "" {
		if jerr := json.Unmarshal([]byte(stdout), &parsed); jerr != nil {
			// Not valid JSON — return the raw output as-is under "raw"
			// so callers can still inspect.
			parsed = map[string]any{"raw": stdout, "parse_error": jerr.Error()}
		}
	}
	return parsed, stderr, err
}

// readJSONFile reads a JSON file into a map; empty map if it doesn't
// exist. Used for the now-playing cache + state reports.
func readJSONFile(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	parsed := map[string]any{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return map[string]any{"raw": string(data), "parse_error": err.Error()}
	}
	return parsed
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// RetroArchModule provides audit + command tools for the RetroArch
// workstation. Tools:
//
//	retroarch_audit        — composite status (workstation + playlist + mounts)
//	retroarch_command      — send a UDP network command to a running RetroArch
//	retroarch_now_playing  — read the ticker cache + content_history head
type RetroArchModule struct{}

func (m *RetroArchModule) Name() string { return "retroarch" }

func (m *RetroArchModule) Description() string {
	return "RetroArch workstation audit + runtime command tools backed by scripts/retroarch-*"
}

func (m *RetroArchModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── retroarch_audit ─────────────────────────────────────
		{
			Tool: mcp.Tool{
				Name: "retroarch_audit",
				Description: "Composite RetroArch workstation audit: cores, BIOS, playlists, rclone mounts. " +
					"Shells out to scripts/retroarch-{workstation-audit,playlist-audit,mounts-audit}.py and " +
					"merges their JSON reports into a single document. Safe to call on any workstation; reads " +
					"the most recently cached reports when invocation flags don't force regeneration.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(ctx context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				state := xdgStateHome()
				report := map[string]any{
					"workstation_audit": map[string]any{},
					"playlist_audit":    map[string]any{},
					"mounts_audit":      map[string]any{},
					"errors":            []string{},
				}
				errs := []string{}

				wsCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()
				if _, err := exec.CommandContext(wsCtx, "python3",
					retroarchScript("retroarch-workstation-audit.py")).CombinedOutput(); err != nil {
					errs = append(errs, fmt.Sprintf("workstation-audit: %v", err))
				}
				report["workstation_audit"] = readJSONFile(filepath.Join(state, "retroarch", "workstation-audit.json"))

				plCtx, cancel2 := context.WithTimeout(ctx, 30*time.Second)
				defer cancel2()
				if _, err := exec.CommandContext(plCtx, "python3",
					retroarchScript("retroarch-playlist-audit.py")).CombinedOutput(); err != nil {
					// Non-zero means broken entries — not a tool failure.
					if _, ok := err.(*exec.ExitError); !ok {
						errs = append(errs, fmt.Sprintf("playlist-audit: %v", err))
					}
				}
				report["playlist_audit"] = readJSONFile(filepath.Join(state, "retroarch", "playlist-audit.json"))

				mtCtx, cancel3 := context.WithTimeout(ctx, 30*time.Second)
				defer cancel3()
				if _, err := exec.CommandContext(mtCtx, "python3",
					retroarchScript("retroarch-mounts-audit.py")).CombinedOutput(); err != nil {
					if _, ok := err.(*exec.ExitError); !ok {
						errs = append(errs, fmt.Sprintf("mounts-audit: %v", err))
					}
				}
				report["mounts_audit"] = readJSONFile(filepath.Join(state, "retroarch", "mounts-audit.json"))

				if len(errs) > 0 {
					report["errors"] = errs
				}
				payload, _ := json.MarshalIndent(report, "", "  ")
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{Type: "text", Text: string(payload)},
					},
				}, nil
			},
		},

		// ── retroarch_command ──────────────────────────────────
		{
			Tool: mcp.Tool{
				Name: "retroarch_command",
				Description: "Send a UDP network command to a running RetroArch instance. Wraps " +
					"scripts/retroarch-command.py --json. Requires network_cmd_enable=true in retroarch.cfg " +
					"and an active RetroArch process. See retroarch-command --list for the command taxonomy.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"command": map[string]any{
							"type":        "string",
							"description": "RetroArch UDP command (e.g. 'VERSION', 'SHOW_MSG \"hello\"', 'PAUSE_TOGGLE'). Required.",
						},
						"wait_for_ready": map[string]any{
							"type":        "boolean",
							"description": "Poll VERSION until the socket answers before sending (default false).",
						},
						"timeout": map[string]any{
							"type":        "number",
							"description": "Socket timeout in seconds (default 1.0).",
						},
					},
					Required: []string{"command"},
				},
			},
			Handler: func(ctx context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				b, _ := json.Marshal(req.Params.Arguments)
				var input struct {
					Command      string  `json:"command"`
					WaitForReady bool    `json:"wait_for_ready"`
					Timeout      float64 `json:"timeout"`
				}
				if err := json.Unmarshal(b, &input); err != nil {
					return handler.CodedErrorResult(handler.ErrInvalidParam, err), nil
				}
				if strings.TrimSpace(input.Command) == "" {
					return handler.CodedErrorResult(handler.ErrInvalidParam,
						fmt.Errorf("command must not be empty")), nil
				}

				args := []string{retroarchScript("retroarch-command.py"), "--json"}
				if input.WaitForReady {
					args = append(args, "--wait-for-ready")
				}
				if input.Timeout > 0 {
					args = append(args, "--timeout", fmt.Sprintf("%g", input.Timeout))
				}
				// Split the command line on whitespace so SHOW_MSG "hi" parses
				// as two args — the script's argparse accepts them as
				// command + positional args.
				parts := strings.Fields(input.Command)
				args = append(args, parts...)

				cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(cmdCtx, "python3", args...)
				out, runErr := cmd.Output()
				stdout := strings.TrimSpace(string(out))
				// The script exits non-zero when the UDP call fails (ok:false),
				// but stdout still contains the structured result. Surface it.
				if stdout == "" && runErr != nil {
					return handler.ErrorResult(runErr), nil
				}
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{Type: "text", Text: stdout},
					},
				}, nil
			},
		},

		// ── retroarch_now_playing ──────────────────────────────
		{
			Tool: mcp.Tool{
				Name: "retroarch_now_playing",
				Description: "Summarize the most recent RetroArch session: reads the ticker cache at " +
					"/tmp/bar-retroarch.txt (written every 30s by bar-retroarch.timer) and the first entry of " +
					"content_history.lpl. Returns JSON with title, core, path, mount_health, and the raw " +
					"ticker label. Safe when nothing has been played yet — the fields are just empty.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(ctx context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				// Ticker cache
				tickerPath := "/tmp/bar-retroarch.txt"
				tickerLabel := ""
				if data, err := os.ReadFile(tickerPath); err == nil {
					tickerLabel = strings.TrimSpace(string(data))
				}

				// content_history head
				home, _ := os.UserHomeDir()
				historyPath := filepath.Join(home, ".config", "retroarch", "playlists", "builtin", "content_history.lpl")
				historyItem := map[string]any{}
				if data, err := os.ReadFile(historyPath); err == nil {
					var doc map[string]any
					if json.Unmarshal(data, &doc) == nil {
						if items, ok := doc["items"].([]any); ok && len(items) > 0 {
							if first, ok := items[0].(map[string]any); ok {
								historyItem = first
							}
						}
					}
				}

				// Mount health
				mountsAudit := readJSONFile(filepath.Join(xdgStateHome(), "retroarch", "mounts-audit.json"))
				mountHealth := map[string]any{}
				if s, ok := mountsAudit["summary"].(map[string]any); ok {
					mountHealth = s
				}

				result := map[string]any{
					"ticker_label":   tickerLabel,
					"history_item":   historyItem,
					"mount_health":   mountHealth,
					"ticker_path":    tickerPath,
					"history_path":   historyPath,
					"retroarch_live": false, // best-effort; mounts-audit covers this more directly
				}
				// Mark retroarch_live=true when the ticker cache is non-empty
				// AND the mounts summary shows any active mount (proxy for
				// "recent activity" since we don't probe the process here).
				if tickerLabel != "" {
					result["retroarch_live"] = true
				}

				payload, _ := json.MarshalIndent(result, "", "  ")
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{Type: "text", Text: string(payload)},
					},
				}, nil
			},
		},
	}
}
