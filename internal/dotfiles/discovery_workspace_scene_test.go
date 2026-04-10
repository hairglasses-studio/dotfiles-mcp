package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkspaceSceneHyprctlFixture(t *testing.T, binDir string) {
	t.Helper()
	writeTestExecutable(t, binDir, "hyprctl", `#!/bin/sh
if [ "$1" = "monitors" ] && [ "$2" = "-j" ]; then
  cat <<'EOF'
[{"name":"DP-1","width":5120,"height":1440,"refreshRate":120.0,"x":0,"y":0,"scale":1.0,"activeWorkspace":{"id":2},"focused":true},{"name":"HDMI-A-1","width":1920,"height":1080,"refreshRate":60.0,"x":5120,"y":0,"scale":1.0,"activeWorkspace":{"id":5},"focused":false}]
EOF
  exit 0
fi
if [ "$1" = "workspaces" ] && [ "$2" = "-j" ]; then
  cat <<'EOF'
[{"id":2,"name":"2","monitor":"DP-1","windows":2},{"id":5,"name":"5","monitor":"HDMI-A-1","windows":1}]
EOF
  exit 0
fi
if [ "$1" = "activeworkspace" ] && [ "$2" = "-j" ]; then
  printf '%s\n' '{"id":2}'
  exit 0
fi
if [ "$1" = "clients" ] && [ "$2" = "-j" ]; then
  cat <<'EOF'
[{"address":"0x1","title":"Ghostty","class":"ghostty","workspace":{"id":2},"size":[1600,900],"at":[0,0],"mapped":true,"focusHistoryID":0,"floating":false},{"address":"0x2","title":"Firefox","class":"firefox","workspace":{"id":5},"size":[1400,900],"at":[1600,0],"mapped":true,"focusHistoryID":1,"floating":true}]
EOF
  exit 0
fi
exit 1
`)
}

func writeWorkspaceSceneStateFixtures(t *testing.T, stateDir string) {
	t.Helper()

	presetDir := filepath.Join(stateDir, "dotfiles", "desktop-control", "hypr", "monitor-presets")
	layoutDir := filepath.Join(stateDir, "dotfiles", "desktop-control", "hypr", "layouts")
	for _, dir := range []string{presetDir, layoutDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	preset := hyprMonitorPreset{
		Name:    "studio-dual",
		SavedAt: "2026-04-10T00:00:00Z",
		Monitors: []hyprMonitorSnapshot{
			{Name: "DP-1", Enabled: true},
			{Name: "HDMI-A-1", Enabled: true},
		},
	}
	presetJSON, err := json.Marshal(preset)
	if err != nil {
		t.Fatalf("marshal preset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(presetDir, "studio-dual.json"), presetJSON, 0o644); err != nil {
		t.Fatalf("write preset: %v", err)
	}

	layout := hyprLayoutSnapshot{
		Name:    "coding-stack",
		SavedAt: "2026-04-10T00:00:00Z",
		Windows: []hyprSavedWindow{
			{Class: "ghostty", Workspace: 2},
			{Class: "firefox", Workspace: 5},
		},
	}
	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("marshal layout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layoutDir, "coding-stack.json"), layoutJSON, 0o644); err != nil {
		t.Fatalf("write layout: %v", err)
	}
}

func TestDotfilesWorkspaceSceneWithSavedState(t *testing.T) {
	homeDir := t.TempDir()
	stateDir := t.TempDir()
	runtimeDir := t.TempDir()
	binDir := t.TempDir()

	t.Setenv("HOME", homeDir)
	t.Setenv("DOTFILES_MCP_PROFILE", "desktop")
	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-fixture")
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "fixture-hypr")
	t.Setenv("PATH", binDir+":/usr/bin:/bin")

	if err := os.MkdirAll(filepath.Join(runtimeDir, "hypr"), 0o755); err != nil {
		t.Fatalf("mkdir hypr runtime: %v", err)
	}

	writeWorkspaceSceneHyprctlFixture(t, binDir)
	writeWorkspaceSceneStateFixtures(t, stateDir)

	result := callDiscoveryTool(t, "dotfiles_workspace_scene", map[string]any{})
	var out dotfilesWorkspaceSceneOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal workspace scene: %v", err)
	}

	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok; errors=%v summary=%q", out.Status, out.Errors, out.Summary)
	}
	if out.MonitorCount != 2 || out.WorkspaceCount != 2 || out.WindowCount != 2 {
		t.Fatalf("unexpected scene counts: monitors=%d workspaces=%d windows=%d", out.MonitorCount, out.WorkspaceCount, out.WindowCount)
	}
	if out.FocusedMonitor != "DP-1" {
		t.Fatalf("focused monitor = %q", out.FocusedMonitor)
	}
	if out.FocusedWorkspace != 2 {
		t.Fatalf("focused workspace = %d", out.FocusedWorkspace)
	}
	if out.FocusedWindow != "Ghostty" {
		t.Fatalf("focused window = %q", out.FocusedWindow)
	}
	if len(out.SavedMonitorPresets) != 1 || len(out.SavedLayouts) != 1 {
		t.Fatalf("unexpected saved state inventory: presets=%d layouts=%d", len(out.SavedMonitorPresets), len(out.SavedLayouts))
	}
	if !containsString(out.SuggestedTools, "hypr_layout_restore") {
		t.Fatalf("expected restore tool in suggested tools, got %v", out.SuggestedTools)
	}
	if !strings.Contains(out.ReportMarkdown, "Workspace Scene") {
		t.Fatalf("expected markdown report header, got %q", out.ReportMarkdown)
	}
}

func TestDotfilesWorkspaceSceneDegradedWithoutHyprctl(t *testing.T) {
	homeDir := t.TempDir()
	stateDir := t.TempDir()
	runtimeDir := t.TempDir()
	binDir := t.TempDir()

	t.Setenv("HOME", homeDir)
	t.Setenv("DOTFILES_MCP_PROFILE", "desktop")
	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("PATH", binDir)

	result := callDiscoveryTool(t, "dotfiles_workspace_scene", map[string]any{})
	var out dotfilesWorkspaceSceneOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal workspace scene: %v", err)
	}

	if out.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", out.Status)
	}
	if len(out.Errors) == 0 {
		t.Fatal("expected collection errors when hyprctl is unavailable")
	}
}
