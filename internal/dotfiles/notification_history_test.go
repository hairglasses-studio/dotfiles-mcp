package dotfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotificationHistoryLifecycle(t *testing.T) {
	setupNotificationHistoryEnv(t)

	if err := appendNotificationHistoryEntry(notificationHistoryEntry{
		App:     "mako",
		Summary: "first",
	}); err != nil {
		t.Fatalf("append first entry: %v", err)
	}
	if err := appendNotificationHistoryEntry(notificationHistoryEntry{
		ID:      "custom-id",
		App:     "swaync",
		Summary: "second",
	}); err != nil {
		t.Fatalf("append second entry: %v", err)
	}

	entries, err := readNotificationHistoryEntries()
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[0].ID == "" || entries[0].Timestamp == "" {
		t.Fatalf("first entry missing generated id/timestamp: %#v", entries[0])
	}
	if entries[1].ID != "custom-id" {
		t.Fatalf("second entry id = %q, want custom-id", entries[1].ID)
	}
	if !entries[0].Visible || entries[0].Dismissed {
		t.Fatalf("expected first entry visible and not dismissed: %#v", entries[0])
	}

	updated, err := markNotificationHistoryDismissed(1)
	if err != nil {
		t.Fatalf("mark dismissed: %v", err)
	}
	if updated != 1 {
		t.Fatalf("mark dismissed updated = %d, want 1", updated)
	}

	entries, err = readNotificationHistoryEntries()
	if err != nil {
		t.Fatalf("read after dismiss: %v", err)
	}
	if entries[1].Visible || !entries[1].Dismissed {
		t.Fatalf("expected last entry dismissed/invisible: %#v", entries[1])
	}
	if !entries[0].Visible || entries[0].Dismissed {
		t.Fatalf("expected first entry to remain visible: %#v", entries[0])
	}

	cleared, err := clearNotificationHistory(false)
	if err != nil {
		t.Fatalf("clear history: %v", err)
	}
	if cleared != 1 {
		t.Fatalf("clear updated = %d, want 1", cleared)
	}
	cleared, err = clearNotificationHistory(false)
	if err != nil {
		t.Fatalf("clear history second pass: %v", err)
	}
	if cleared != 0 {
		t.Fatalf("clear second pass updated = %d, want 0", cleared)
	}

	if err := appendNotificationHistoryEntry(notificationHistoryEntry{
		App:     "notify-send",
		Summary: "third",
	}); err != nil {
		t.Fatalf("append third entry: %v", err)
	}

	purged, err := clearNotificationHistory(true)
	if err != nil {
		t.Fatalf("purge history: %v", err)
	}
	if purged != 3 {
		t.Fatalf("purged count = %d, want 3", purged)
	}

	entries, err = readNotificationHistoryEntries()
	if err != nil {
		t.Fatalf("read after purge: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected purge to leave 0 entries, got %d", len(entries))
	}
}

func TestReadNotificationHistoryEntriesLegacyAndInvalidLines(t *testing.T) {
	setupNotificationHistoryEnv(t)

	path := dotfilesNotificationHistoryLogPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir notification dir: %v", err)
	}
	content := strings.Join([]string{
		`{"summary":"legacy entry","visible":true}`,
		`{not-json}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write history log: %v", err)
	}

	entries, err := readNotificationHistoryEntries()
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}
	if !strings.HasPrefix(entries[0].ID, "legacy-") {
		t.Fatalf("legacy id = %q, want legacy-*", entries[0].ID)
	}
	if entries[0].Timestamp == "" {
		t.Fatal("expected legacy entry timestamp to be backfilled")
	}
	if entries[0].Summary != "legacy entry" {
		t.Fatalf("summary = %q, want legacy entry", entries[0].Summary)
	}
}

func setupNotificationHistoryEnv(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
}
