// mod_notify.go — Desktop notification tools via notify-send
package dotfiles

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------

// NotifySendInput defines the input for the notify_send tool.
type NotifySendInput struct {
	Title     string `json:"title" jsonschema:"required,description=Notification title"`
	Body      string `json:"body" jsonschema:"required,description=Notification body text"`
	Urgency   string `json:"urgency,omitempty" jsonschema:"description=Urgency level: low/normal/critical,enum=low,enum=normal,enum=critical"`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"description=Auto-dismiss timeout in milliseconds"`
	Icon      string `json:"icon,omitempty" jsonschema:"description=Icon name or path"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// NotifyModule provides desktop notification tools via notify-send.
type NotifyModule struct{}

func (m *NotifyModule) Name() string        { return "notify" }
func (m *NotifyModule) Description() string { return "Desktop notification tools via notify-send" }

func (m *NotifyModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[NotifySendInput, string](
			"notify_send",
			"Send a desktop notification via notify-send. Supports urgency levels, timeout, and custom icons.",
			func(_ context.Context, input NotifySendInput) (string, error) {
				if _, err := exec.LookPath("notify-send"); err != nil {
					return "", fmt.Errorf("notify-send not found on PATH — install libnotify (e.g. pacman -S libnotify)")
				}

				if input.Title == "" {
					return "", fmt.Errorf("[%s] title must not be empty", handler.ErrInvalidParam)
				}

				// Build notify-send args
				args := []string{}

				if input.Urgency != "" {
					switch input.Urgency {
					case "low", "normal", "critical":
						args = append(args, "-u", input.Urgency)
					default:
						return "", fmt.Errorf("[%s] urgency must be one of: low, normal, critical", handler.ErrInvalidParam)
					}
				}

				if input.TimeoutMs > 0 {
					args = append(args, "-t", strconv.Itoa(input.TimeoutMs))
				}

				if input.Icon != "" {
					args = append(args, "-i", input.Icon)
				}

				args = append(args, input.Title, input.Body)

				cmd := exec.Command("notify-send", args...)
				out, err := cmd.CombinedOutput()
				if err != nil {
					return "", fmt.Errorf("notify-send failed: %w: %s", err, string(out))
				}

				urgencyStr := input.Urgency
				if urgencyStr == "" {
					urgencyStr = "normal"
				}

				return fmt.Sprintf("Notification sent: %q (urgency: %s)", input.Title, urgencyStr), nil
			},
		),
	}
}
