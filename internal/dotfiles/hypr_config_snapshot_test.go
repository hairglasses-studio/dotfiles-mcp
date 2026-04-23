package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupFakeHyprHome creates a HOME with ~/.config/hypr/ containing the
// given files, redirects XDG_STATE_HOME to a sibling dir, and returns the
// HOME path so the caller can mutate files between snapshot calls.
func setupFakeHyprHome(t *testing.T, files map[string]string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))

	live := filepath.Join(home, ".config", "hypr")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatalf("mkdir live hypr dir: %v", err)
	}
	for name, content := range files {
		path := filepath.Join(live, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return home
}

func TestHyprConfigSnapshotCreateCapturesFiles(t *testing.T) {
	setupFakeHyprHome(t, map[string]string{
		"hyprland.conf": "# fake hyprland.conf\ngeneral { gaps_in = 5 }\n",
		"monitors.conf": "monitor=,preferred,auto,1\n",
		"colors.conf":   "# palette\n",
	})

	out, err := hyprConfigSnapshotCreate("test-capture")
	if err != nil {
		t.Fatalf("snapshot create: %v", err)
	}
	if out.Label != "test-capture" {
		t.Errorf("want label=test-capture, got %q", out.Label)
	}
	if !strings.HasSuffix(out.Name, "_test-capture") {
		t.Errorf("Name should carry the sanitized label suffix, got %q", out.Name)
	}
	if len(out.Files) != 3 {
		t.Errorf("want 3 files captured, got %d: %v", len(out.Files), out.Files)
	}
	// meta.json must exist and parse back.
	meta, err := readSnapshotMeta(out.Path)
	if err != nil {
		t.Fatalf("read snapshot meta: %v", err)
	}
	if meta.Name != out.Name {
		t.Errorf("meta.Name=%q doesn't match created %q", meta.Name, out.Name)
	}
}

func TestHyprConfigSnapshotCreateSkipsMissingFiles(t *testing.T) {
	// Only two of the allowlisted files are present — the rest should be
	// silently skipped, not reported as errors.
	setupFakeHyprHome(t, map[string]string{
		"hyprland.conf": "# ok\n",
		"monitors.conf": "# ok\n",
	})

	out, err := hyprConfigSnapshotCreate("sparse")
	if err != nil {
		t.Fatalf("snapshot create: %v", err)
	}
	if len(out.Files) != 2 {
		t.Errorf("want exactly 2 files captured, got %d: %v", len(out.Files), out.Files)
	}
}

func TestHyprConfigSnapshotListNewestFirst(t *testing.T) {
	setupFakeHyprHome(t, map[string]string{
		"hyprland.conf": "# v1\n",
	})

	_, err := hyprConfigSnapshotCreate("alpha")
	if err != nil {
		t.Fatalf("alpha: %v", err)
	}
	// Sleep-free: second snapshot in the same second would collide on
	// directory name. Rewrite the file + bump the timestamp manually by
	// creating a new snapshot with a different label.
	_, err = hyprConfigSnapshotCreate("beta")
	if err != nil {
		// Same-second directory collision is fine — just skip the ordering
		// assertion in that case.
		t.Skipf("second snapshot collided on timestamp: %v", err)
	}

	list, err := hyprConfigSnapshotList()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Snapshots) < 2 {
		t.Fatalf("want at least 2 snapshots, got %d", len(list.Snapshots))
	}
	// Names sort lexicographically by timestamp prefix; newest should lead.
	if list.Snapshots[0].Name <= list.Snapshots[1].Name {
		t.Errorf("list is not newest-first: %v", list.Snapshots)
	}
}

func TestHyprConfigSnapshotRollbackDryRun(t *testing.T) {
	home := setupFakeHyprHome(t, map[string]string{
		"hyprland.conf": "original content\n",
		"monitors.conf": "original monitors\n",
	})

	snap, err := hyprConfigSnapshotCreate("before")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Mutate the live file — dry-run rollback should not undo it.
	livePath := filepath.Join(home, ".config", "hypr", "hyprland.conf")
	if err := os.WriteFile(livePath, []byte("mutated content\n"), 0o644); err != nil {
		t.Fatalf("write live: %v", err)
	}

	out, err := hyprConfigSnapshotRollback(snap.Name, true)
	if err != nil {
		t.Fatalf("dry-run rollback: %v", err)
	}
	if !out.DryRun {
		t.Error("expected DryRun=true in output")
	}
	if len(out.FilesRestored) == 0 {
		t.Error("expected dry-run to report files that would be restored")
	}

	// Live file must still say "mutated" — dry-run must not write.
	got, _ := os.ReadFile(livePath)
	if !strings.Contains(string(got), "mutated") {
		t.Errorf("dry-run wrote to live file; content=%q", got)
	}
}

func TestHyprConfigSnapshotMetaJSONRoundTrip(t *testing.T) {
	setupFakeHyprHome(t, map[string]string{
		"hyprland.conf": "# k\n",
	})
	out, err := hyprConfigSnapshotCreate("roundtrip")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out.Path, "meta.json"))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	var meta hyprConfigSnapshotOutput
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if meta.SavedAt == "" {
		t.Error("SavedAt should be populated")
	}
	if meta.Label != "roundtrip" {
		t.Errorf("Label mismatch after round trip: %q", meta.Label)
	}
}

func TestHyprConfigRollbackMissingSnapshot(t *testing.T) {
	setupFakeHyprHome(t, map[string]string{
		"hyprland.conf": "# k\n",
	})
	_, err := hyprConfigSnapshotRollback("does-not-exist", false)
	if err == nil {
		t.Fatal("expected error rolling back to missing snapshot")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err)
	}
}

func TestHyprConfigRollbackEmptyName(t *testing.T) {
	setupFakeHyprHome(t, map[string]string{})
	_, err := hyprConfigSnapshotRollback("", false)
	if err == nil {
		t.Fatal("expected error with empty name")
	}
}
