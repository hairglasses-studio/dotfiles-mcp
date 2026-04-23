// mod_theme.go — Hairglasses Neon palette and theme state resources
package dotfiles

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/mcpkit/resources"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// ThemeModule
// ---------------------------------------------------------------------------

// ThemeModule provides palette and theme state resources.
type ThemeModule struct{}

func (m *ThemeModule) Name() string        { return "theme" }
func (m *ThemeModule) Description() string { return "Hairglasses Neon palette and theme state" }
func (m *ThemeModule) Tools() []registry.ToolDefinition {
	return nil
}

// Resources returns the MCP resources provided by ThemeModule.
func (m *ThemeModule) Resources() []resources.ResourceDefinition {
	return []resources.ResourceDefinition{
		{
			Resource: mcp.NewResource(
				"dotfiles://palette",
				"Hairglasses Neon Palette",
				mcp.WithResourceDescription("Hairglasses Neon color palette tokens parsed from theme/palette.env"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				paletteFile := filepath.Join(dotfilesDir(), "theme", "palette.env")
				f, err := os.Open(paletteFile)
				if err != nil {
					return nil, fmt.Errorf("read palette.env: %w", err)
				}
				defer f.Close()

				tokens := make(map[string]string)
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					// Skip blank lines and comments
					if line == "" || strings.HasPrefix(line, "#") {
						continue
					}
					parts := strings.SplitN(line, "=", 2)
					if len(parts) != 2 {
						continue
					}
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					// Strip surrounding quotes
					val = strings.Trim(val, `"'`)
					if key != "" {
						tokens[key] = val
					}
				}
				if err := scanner.Err(); err != nil {
					return nil, fmt.Errorf("scan palette.env: %w", err)
				}

				data, _ := json.MarshalIndent(tokens, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://palette",
						MIMEType: "application/json",
						Text:     string(data),
					},
				}, nil
			},
			Category: "theme",
			Tags:     []string{"palette", "colors", "theme", "neon"},
		},
	}
}

func (m *ThemeModule) Templates() []resources.TemplateDefinition { return nil }
