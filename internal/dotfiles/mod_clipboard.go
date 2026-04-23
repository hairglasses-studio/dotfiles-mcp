// mod_clipboard.go — Wayland clipboard read/write tools via wl-copy/wl-paste
package dotfiles

import (
	"bytes"
	"context"
	"encoding/base64"
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

// clipCheckTool checks if a CLI tool is available on PATH.
func clipCheckTool(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found on PATH — install it first (e.g. pacman -S wl-clipboard)", name)
	}
	return nil
}

// clipRunCmd executes a command and returns combined output.
func clipRunCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s failed: %w: %s", name, err, string(out))
	}
	return string(out), nil
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// ClipboardModule provides Wayland clipboard tools via wl-copy/wl-paste.
type ClipboardModule struct{}

func (m *ClipboardModule) Name() string { return "clipboard" }
func (m *ClipboardModule) Description() string {
	return "Wayland clipboard read/write via wl-copy/wl-paste"
}

func (m *ClipboardModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── clipboard_read ────────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "clipboard_read",
				Description: "Read the current Wayland clipboard contents as text. Uses wl-paste.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(_ context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := clipCheckTool("wl-paste"); err != nil {
					return handler.ErrorResult(err), nil
				}

				out, err := clipRunCmd("wl-paste", "--no-newline")
				if err != nil {
					// wl-paste exits non-zero when clipboard is empty
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: "Clipboard is empty or contains no text content.",
							},
						},
					}, nil
				}

				if strings.TrimSpace(out) == "" {
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: "Clipboard is empty.",
							},
						},
					}, nil
				}

				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: out,
						},
					},
				}, nil
			},
		},

		// ── clipboard_write ───────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "clipboard_write",
				Description: "Write text to the Wayland clipboard. Uses wl-copy. Optionally specify a MIME type.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"text": map[string]any{
							"type":        "string",
							"description": "Text to write to the clipboard",
						},
						"mime_type": map[string]any{
							"type":        "string",
							"description": "MIME type to advertise (e.g. text/html, application/json). Defaults to text/plain.",
						},
					},
					Required: []string{"text"},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := clipCheckTool("wl-copy"); err != nil {
					return handler.ErrorResult(err), nil
				}

				var input struct {
					Text     string `json:"text"`
					MimeType string `json:"mime_type"`
				}
				if req.Params.Arguments != nil {
					b, _ := json.Marshal(req.Params.Arguments)
					// Ignore error — unmarshal failure leaves input at zero-value
					// and downstream handler validation surfaces the missing required
					// fields via ErrInvalidParam. Keeping the explicit _ = marker so
					// errcheck stays clean and the intent is obvious.
					_ = json.Unmarshal(b, &input)
				}

				if input.Text == "" {
					return handler.CodedErrorResult(handler.ErrInvalidParam, fmt.Errorf("text must not be empty")), nil
				}

				// Build wl-copy args
				var args []string
				if input.MimeType != "" {
					args = append(args, "--type", input.MimeType)
				}

				cmd := exec.Command("wl-copy", args...)
				cmd.Stdin = bytes.NewBufferString(input.Text)
				out, err := cmd.CombinedOutput()
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("wl-copy failed: %w: %s", err, string(out))), nil
				}

				mime := input.MimeType
				if mime == "" {
					mime = "text/plain"
				}

				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("Copied %d bytes to clipboard (type: %s)", len(input.Text), mime),
						},
					},
				}, nil
			},
		},

		// ── clipboard_read_image ──────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "clipboard_read_image",
				Description: "Read an image from the Wayland clipboard. Uses wl-paste --type image/png. Returns the image inline as base64-encoded PNG.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(_ context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := clipCheckTool("wl-paste"); err != nil {
					return handler.ErrorResult(err), nil
				}

				cmd := exec.Command("wl-paste", "--type", "image/png")
				out, err := cmd.Output()
				if err != nil {
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: "Clipboard does not contain an image (image/png). Copy an image first.",
							},
						},
					}, nil
				}

				if len(out) == 0 {
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: "Clipboard image data is empty.",
							},
						},
					}, nil
				}

				b64 := base64.StdEncoding.EncodeToString(out)

				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("Clipboard image: %d bytes", len(out)),
						},
						mcp.ImageContent{
							Type:     "image",
							Data:     b64,
							MIMEType: "image/png",
						},
					},
				}, nil
			},
		},

		// ── cliphist_list ─────────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "cliphist_list",
				Description: "List recent clipboard history entries via cliphist. Returns a structured JSON array of the most recent 50 entries, each with id, preview text, and detected type.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(_ context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := clipCheckTool("cliphist"); err != nil {
					return handler.ErrorResult(err), nil
				}

				out, err := clipRunCmd("cliphist", "list")
				if err != nil {
					return handler.ErrorResult(err), nil
				}

				type entry struct {
					ID      string `json:"id"`
					Preview string `json:"preview"`
					Type    string `json:"type"`
				}

				var entries []entry
				lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
				for _, line := range lines {
					if line == "" {
						continue
					}
					parts := strings.SplitN(line, "\t", 2)
					id := parts[0]
					preview := ""
					if len(parts) == 2 {
						preview = parts[1]
					}

					// Detect type from preview heuristics
					entryType := "text"
					if strings.HasPrefix(preview, "[[") || strings.Contains(preview, "binary data") {
						entryType = "binary"
					} else if strings.HasPrefix(preview, "http://") || strings.HasPrefix(preview, "https://") {
						entryType = "url"
					}

					entries = append(entries, entry{ID: id, Preview: preview, Type: entryType})
					if len(entries) >= 50 {
						break
					}
				}

				result, _ := json.Marshal(entries)
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: string(result),
						},
					},
				}, nil
			},
		},

		// ── cliphist_get ──────────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "cliphist_get",
				Description: "Retrieve a specific clipboard history entry by ID via cliphist decode. Returns the full content of the entry.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"id": map[string]any{
							"type":        "string",
							"description": "The cliphist entry ID (from cliphist_list)",
						},
					},
					Required: []string{"id"},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := clipCheckTool("cliphist"); err != nil {
					return handler.ErrorResult(err), nil
				}

				var input struct {
					ID string `json:"id"`
				}
				b, _ := json.Marshal(req.Params.Arguments)
				// Ignore error — unmarshal failure leaves input at zero-value
				// and downstream handler validation surfaces the missing required
				// fields via ErrInvalidParam. Keeping the explicit _ = marker so
				// errcheck stays clean and the intent is obvious.
				_ = json.Unmarshal(b, &input)

				if input.ID == "" {
					return handler.CodedErrorResult(handler.ErrInvalidParam, fmt.Errorf("id must not be empty")), nil
				}

				out, err := clipRunCmd("cliphist", "decode", input.ID)
				if err != nil {
					return handler.ErrorResult(err), nil
				}

				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: out,
						},
					},
				}, nil
			},
		},

		// ── cliphist_delete ───────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "cliphist_delete",
				Description: "Delete clipboard history entries matching a query via cliphist delete-query. Dry-run by default — set execute to true to actually delete.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Search query to match entries for deletion",
						},
						"execute": map[string]any{
							"type":        "boolean",
							"description": "Set to true to actually delete matching entries. Defaults to false (dry-run).",
						},
					},
					Required: []string{"query"},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := clipCheckTool("cliphist"); err != nil {
					return handler.ErrorResult(err), nil
				}

				var input struct {
					Query   string `json:"query"`
					Execute bool   `json:"execute"`
				}
				b, _ := json.Marshal(req.Params.Arguments)
				// Ignore error — unmarshal failure leaves input at zero-value
				// and downstream handler validation surfaces the missing required
				// fields via ErrInvalidParam. Keeping the explicit _ = marker so
				// errcheck stays clean and the intent is obvious.
				_ = json.Unmarshal(b, &input)

				if input.Query == "" {
					return handler.CodedErrorResult(handler.ErrInvalidParam, fmt.Errorf("query must not be empty")), nil
				}

				if !input.Execute {
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: fmt.Sprintf("Dry-run: would run `cliphist delete-query %q`. Set execute=true to delete matching entries.", input.Query),
							},
						},
					}, nil
				}

				out, err := clipRunCmd("cliphist", "delete-query", input.Query)
				if err != nil {
					return handler.ErrorResult(err), nil
				}

				msg := fmt.Sprintf("Deleted clipboard history entries matching %q.", input.Query)
				if strings.TrimSpace(out) != "" {
					msg += "\n" + out
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

		// ── cliphist_clear ────────────────────────────────
		{
			Tool: mcp.Tool{
				Name:        "cliphist_clear",
				Description: "Clear all clipboard history via cliphist wipe. Dry-run by default — set execute to true to actually wipe. This is a destructive operation and cannot be undone.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"execute": map[string]any{
							"type":        "boolean",
							"description": "Set to true to actually wipe all clipboard history. Defaults to false (dry-run). This action is irreversible.",
						},
					},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := clipCheckTool("cliphist"); err != nil {
					return handler.ErrorResult(err), nil
				}

				var input struct {
					Execute bool `json:"execute"`
				}
				b, _ := json.Marshal(req.Params.Arguments)
				// Ignore error — unmarshal failure leaves input at zero-value
				// and downstream handler validation surfaces the missing required
				// fields via ErrInvalidParam. Keeping the explicit _ = marker so
				// errcheck stays clean and the intent is obvious.
				_ = json.Unmarshal(b, &input)

				if !input.Execute {
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: "Dry-run: would run `cliphist wipe` to clear all clipboard history. Set execute=true to proceed. This action is irreversible.",
							},
						},
					}, nil
				}

				out, err := clipRunCmd("cliphist", "wipe")
				if err != nil {
					return handler.ErrorResult(err), nil
				}

				msg := "Clipboard history wiped."
				if strings.TrimSpace(out) != "" {
					msg += "\n" + out
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
	}
}
