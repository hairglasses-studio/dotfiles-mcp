// mod_notification.go — SwayNotificationCenter control via swaync-client
package dotfiles

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const swayncCmd = "swaync-client"
const swayncTimeout = 5 * time.Second

// swayncCheckTool checks if swaync-client is available on PATH.
func swayncCheckTool() error {
	_, err := exec.LookPath(swayncCmd)
	if err != nil {
		return fmt.Errorf("%s not found on PATH — install swaync (e.g. pacman -S swaync)", swayncCmd)
	}
	return nil
}

// swayncRunCmd executes a swaync-client command with a timeout and returns
// trimmed stdout. Returns a descriptive error if the process fails.
func swayncRunCmd(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), swayncTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, swayncCmd, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("%s %s failed: %w: %s", swayncCmd, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------

// NotifyHistoryInput defines the input for the notify_history tool.
type NotifyHistoryInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"description=Maximum number of notifications to return (0 or omit for all)"`
}

type NotifyHistoryEntriesInput struct {
	Limit            int  `json:"limit,omitempty" jsonschema:"description=Maximum number of entries to return. Defaults to 25."`
	IncludeDismissed bool `json:"include_dismissed,omitempty" jsonschema:"description=Include entries that have already been dismissed or cleared."`
}

type NotifyHistoryEntriesOutput struct {
	Entries       []notificationHistoryEntry `json:"entries"`
	Total         int                        `json:"total"`
	Visible       int                        `json:"visible"`
	BackendReady  bool                       `json:"backend_ready"`
	LogPath       string                     `json:"log_path"`
	ListenerAlive bool                       `json:"listener_alive"`
}

type NotifyHistoryClearInput struct {
	Purge bool `json:"purge,omitempty" jsonschema:"description=When true, remove the stored history entirely instead of marking entries dismissed."`
}

type NotifyHistoryClearOutput struct {
	Cleared int  `json:"cleared"`
	Purged  bool `json:"purged"`
}

// NotifyDNDInput defines the input for the notify_dnd tool.
type NotifyDNDInput struct {
	Action string `json:"action" jsonschema:"required,description=DND action: get (query state) / toggle / on / off,enum=get,enum=toggle,enum=on,enum=off"`
}

// NotifyDismissInput defines the input for the notify_dismiss tool.
type NotifyDismissInput struct {
	Scope string `json:"scope" jsonschema:"required,description=Which notifications to dismiss: all or latest,enum=all,enum=latest"`
}

// NotifyPanelInput defines the input for the notify_panel tool.
type NotifyPanelInput struct {
	Action string `json:"action" jsonschema:"required,description=Panel action: toggle / open / close,enum=toggle,enum=open,enum=close"`
}

// NotifyResult wraps a string result so the MCP response is a JSON object.
type NotifyResult struct {
	Result string `json:"result"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// NotificationModule provides SwayNotificationCenter management tools via
// swaync-client. Separate from NotifyModule (which wraps notify-send).
type NotificationModule struct{}

func (m *NotificationModule) Name() string { return "notification" }
func (m *NotificationModule) Description() string {
	return "SwayNotificationCenter control via swaync-client"
}

func (m *NotificationModule) Tools() []registry.ToolDefinition {
	// ── notify_history ─────────────────────────────────────
	// Note: swaync-client does not expose notification history via CLI.
	// -s (subscribe) returns a status snapshot {count, dnd, visible, inhibited}.
	// We return this status snapshot as the best available summary.
	notifyHistory := handler.TypedHandler[NotifyHistoryInput, any](
		"notify_history",
		"Get SwayNotificationCenter status snapshot: notification count, DND state, panel visibility, inhibitor state. (swaync does not expose individual notification history via CLI.)",
		func(_ context.Context, _ NotifyHistoryInput) (any, error) {
			if err := swayncCheckTool(); err != nil {
				return nil, err
			}

			// Get count and DND state via dedicated flags (reliable, no timeout)
			countRaw, _ := swayncRunCmd("-c")
			dndRaw, _ := swayncRunCmd("-D")

			count := 0
			if countRaw != "" {
				_, _ = fmt.Sscanf(countRaw, "%d", &count)
			}

			entries, _ := readNotificationHistoryEntries()
			tracked := len(entries)
			visible := 0
			for _, entry := range entries {
				if entry.Visible && !entry.Dismissed {
					visible++
				}
			}

			return map[string]any{
				"count":                  count,
				"dnd":                    strings.TrimSpace(dndRaw) == "true",
				"tracked_entries":        tracked,
				"tracked_visible":        visible,
				"history_log_path":       dotfilesNotificationHistoryLogPath(),
				"history_listener_alive": notificationHistoryListenerRunning(),
				"note":                   "swaync does not expose individual notification history via CLI; detailed history is provided by the local desktop-control log",
			}, nil
		},
	)

	notifyHistoryEntries := handler.TypedHandler[NotifyHistoryEntriesInput, NotifyHistoryEntriesOutput](
		"notify_history_entries",
		"Get the locally logged notification history entries with app, summary, body, urgency, timestamp, and dismissal state.",
		func(_ context.Context, input NotifyHistoryEntriesInput) (NotifyHistoryEntriesOutput, error) {
			entries, err := readNotificationHistoryEntries()
			if err != nil {
				return NotifyHistoryEntriesOutput{}, err
			}

			visible := 0
			filtered := make([]notificationHistoryEntry, 0, len(entries))
			for i := len(entries) - 1; i >= 0; i-- {
				entry := entries[i]
				if entry.Visible && !entry.Dismissed {
					visible++
				}
				if !input.IncludeDismissed && (entry.Dismissed || !entry.Visible) {
					continue
				}
				filtered = append(filtered, entry)
			}

			limit := input.Limit
			if limit <= 0 {
				limit = 25
			}
			if len(filtered) > limit {
				filtered = filtered[:limit]
			}

			return NotifyHistoryEntriesOutput{
				Entries:       filtered,
				Total:         len(entries),
				Visible:       visible,
				BackendReady:  pathExists(dotfilesNotificationHistoryListenerPath()) && hasCmd("python3") && hasCmd("dbus-monitor"),
				LogPath:       dotfilesNotificationHistoryLogPath(),
				ListenerAlive: notificationHistoryListenerRunning(),
			}, nil
		},
	)

	notifyHistoryClear := handler.TypedHandler[NotifyHistoryClearInput, NotifyHistoryClearOutput](
		"notify_history_clear",
		"DESTRUCTIVE: Clear the locally logged notification history. By default entries are marked dismissed; set purge=true to remove them entirely.",
		func(_ context.Context, input NotifyHistoryClearInput) (NotifyHistoryClearOutput, error) {
			cleared, err := clearNotificationHistory(input.Purge)
			if err != nil {
				return NotifyHistoryClearOutput{}, err
			}
			return NotifyHistoryClearOutput{
				Cleared: cleared,
				Purged:  input.Purge,
			}, nil
		},
	)
	notifyHistoryClear.IsWrite = true

	// ── notify_dnd ─────────────────────────────────────────
	notifyDND := handler.TypedHandler[NotifyDNDInput, NotifyResult](
		"notify_dnd",
		"Manage Do Not Disturb mode in SwayNotificationCenter. Actions: get (query current state), toggle, on, off.",
		func(_ context.Context, input NotifyDNDInput) (NotifyResult, error) {
			if err := swayncCheckTool(); err != nil {
				return NotifyResult{}, err
			}

			switch input.Action {
			case "get":
				out, err := swayncRunCmd("-D")
				if err != nil {
					return NotifyResult{}, fmt.Errorf("failed to get DND state: %w", err)
				}
				enabled := strings.ToLower(out) == "true"
				if enabled {
					return NotifyResult{Result: "DND is enabled"}, nil
				}
				return NotifyResult{Result: "DND is disabled"}, nil

			case "toggle":
				out, err := swayncRunCmd("-d")
				if err != nil {
					return NotifyResult{}, fmt.Errorf("failed to toggle DND: %w", err)
				}
				return NotifyResult{Result: fmt.Sprintf("DND toggled (new state: %s)", out)}, nil

			case "on":
				if _, err := swayncRunCmd("-dn"); err != nil {
					return NotifyResult{}, fmt.Errorf("failed to enable DND: %w", err)
				}
				return NotifyResult{Result: "DND enabled"}, nil

			case "off":
				if _, err := swayncRunCmd("-df"); err != nil {
					return NotifyResult{}, fmt.Errorf("failed to disable DND: %w", err)
				}
				return NotifyResult{Result: "DND disabled"}, nil

			default:
				return NotifyResult{}, fmt.Errorf("[%s] action must be one of: get, toggle, on, off", handler.ErrInvalidParam)
			}
		},
	)
	notifyDND.IsWrite = true

	// ── notify_dismiss ─────────────────────────────────────
	notifyDismiss := handler.TypedHandler[NotifyDismissInput, NotifyResult](
		"notify_dismiss",
		"DESTRUCTIVE: Dismiss notifications in SwayNotificationCenter. Scope: all (dismiss everything) or latest (dismiss most recent only).",
		func(_ context.Context, input NotifyDismissInput) (NotifyResult, error) {
			if err := swayncCheckTool(); err != nil {
				return NotifyResult{}, err
			}

			switch input.Scope {
			case "all":
				if _, err := swayncRunCmd("-C"); err != nil {
					return NotifyResult{}, fmt.Errorf("failed to dismiss all notifications: %w", err)
				}
				_, _ = markNotificationHistoryDismissed(0)
				return NotifyResult{Result: "All notifications dismissed"}, nil

			case "latest":
				if _, err := swayncRunCmd("--close-latest"); err != nil {
					return NotifyResult{}, fmt.Errorf("failed to dismiss latest notification: %w", err)
				}
				_, _ = markNotificationHistoryDismissed(1)
				return NotifyResult{Result: "Latest notification dismissed"}, nil

			default:
				return NotifyResult{}, fmt.Errorf("[%s] scope must be one of: all, latest", handler.ErrInvalidParam)
			}
		},
	)
	notifyDismiss.IsWrite = true

	// ── notify_count ───────────────────────────────────────
	notifyCount := handler.TypedHandler[struct{}, NotifyResult](
		"notify_count",
		"Get the current notification count from SwayNotificationCenter.",
		func(_ context.Context, _ struct{}) (NotifyResult, error) {
			if err := swayncCheckTool(); err != nil {
				return NotifyResult{}, err
			}

			out, err := swayncRunCmd("-c")
			if err != nil {
				return NotifyResult{}, fmt.Errorf("failed to get notification count: %w", err)
			}

			// Validate it's actually a number.
			count, err := strconv.Atoi(out)
			if err != nil {
				return NotifyResult{}, fmt.Errorf("unexpected output from swaync-client -c: %q", out)
			}

			return NotifyResult{Result: fmt.Sprintf("%d", count)}, nil
		},
	)

	// ── notify_panel ───────────────────────────────────────
	notifyPanel := handler.TypedHandler[NotifyPanelInput, NotifyResult](
		"notify_panel",
		"DESTRUCTIVE: Control the SwayNotificationCenter panel visibility. Actions: toggle, open, close.",
		func(_ context.Context, input NotifyPanelInput) (NotifyResult, error) {
			if err := swayncCheckTool(); err != nil {
				return NotifyResult{}, err
			}

			switch input.Action {
			case "toggle":
				if _, err := swayncRunCmd("-t"); err != nil {
					return NotifyResult{}, fmt.Errorf("failed to toggle panel: %w", err)
				}
				return NotifyResult{Result: "Notification panel toggled"}, nil

			case "open":
				if _, err := swayncRunCmd("-op"); err != nil {
					return NotifyResult{}, fmt.Errorf("failed to open panel: %w", err)
				}
				return NotifyResult{Result: "Notification panel opened"}, nil

			case "close":
				if _, err := swayncRunCmd("-cp"); err != nil {
					return NotifyResult{}, fmt.Errorf("failed to close panel: %w", err)
				}
				return NotifyResult{Result: "Notification panel closed"}, nil

			default:
				return NotifyResult{}, fmt.Errorf("[%s] action must be one of: toggle, open, close", handler.ErrInvalidParam)
			}
		},
	)
	notifyPanel.IsWrite = true

	return []registry.ToolDefinition{
		notifyHistory,
		notifyHistoryEntries,
		notifyHistoryClear,
		notifyDND,
		notifyDismiss,
		notifyCount,
		notifyPanel,
	}
}
