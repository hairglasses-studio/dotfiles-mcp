package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecutableScript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable script %s: %v", path, err)
	}
}

func TestSafeStateName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "Primary Desk", want: "primary-desk"},
		{input: "  Mixed_CASE 123  ", want: "mixed-case-123"},
		{input: "###", want: "unnamed"},
		{input: "alpha---beta", want: "alpha-beta"},
	}

	for _, tc := range tests {
		if got := safeStateName(tc.input); got != tc.want {
			t.Errorf("safeStateName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestValidateMonitorPreset(t *testing.T) {
	preset := hyprMonitorPreset{
		Name: "desk",
		Monitors: []hyprMonitorSnapshot{
			{
				Name:        "DP-1",
				Enabled:     true,
				Width:       5120,
				Height:      1440,
				RefreshRate: 119.88,
				X:           0,
				Y:           0,
				Scale:       1.25,
				Transform:   0,
			},
			{
				Name:    "HDMI-A-1",
				Enabled: false,
			},
		},
	}

	commands, errs := validateMonitorPreset(preset)
	if len(errs) != 0 {
		t.Fatalf("validateMonitorPreset returned unexpected errors: %v", errs)
	}
	want := []string{
		"DP-1,5120x1440@119.88,0x0,1.25,0",
		"HDMI-A-1,disable",
	}
	if strings.Join(commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %v, want %v", commands, want)
	}

	_, errs = validateMonitorPreset(hyprMonitorPreset{
		Monitors: []hyprMonitorSnapshot{
			{Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
			{Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
			{Name: "DP-2", Enabled: true, Width: 0, Height: 1080, RefreshRate: 60, Scale: 1},
			{Name: "DP-3", Enabled: true, Width: 1920, Height: 1080, RefreshRate: 60, Scale: 0},
		},
	})
	if len(errs) != 3 {
		t.Fatalf("error count = %d, want 3; errs=%v", len(errs), errs)
	}
}

func TestWindowMatchScore(t *testing.T) {
	saved := hyprSavedWindow{
		Class:        "firefox",
		Title:        "Docs",
		InitialClass: "Firefox",
		InitialTitle: "Docs - Mozilla Firefox",
		Workspace:    3,
	}

	match := hyprClientState{
		Class:        "Firefox",
		Title:        "Docs",
		InitialClass: "Firefox",
		InitialTitle: "Docs - Mozilla Firefox",
		Workspace:    3,
		Mapped:       true,
	}
	weak := hyprClientState{
		Class:        "Firefox",
		Title:        "Docs - something else",
		InitialClass: "Firefox",
		Workspace:    2,
		Mapped:       true,
	}
	miss := hyprClientState{
		Class:  "foot",
		Title:  "terminal",
		Mapped: true,
	}

	matchScore := windowMatchScore(saved, match)
	weakScore := windowMatchScore(saved, weak)
	missScore := windowMatchScore(saved, miss)

	if matchScore <= weakScore {
		t.Fatalf("expected exact match score > weak score, got %d <= %d", matchScore, weakScore)
	}
	if missScore != -1 {
		t.Fatalf("expected class mismatch to return -1, got %d", missScore)
	}
}

func TestListHyprStateFixtures(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))

	monitorDir := filepath.Join(root, "state", "dotfiles", "desktop-control", "hypr", "monitor-presets")
	layoutDir := filepath.Join(root, "state", "dotfiles", "desktop-control", "hypr", "layouts")
	for _, dir := range []string{monitorDir, layoutDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	monitorFixture := hyprMonitorPreset{
		Name:    "desk-a",
		SavedAt: "2026-04-10T10:00:00Z",
		Monitors: []hyprMonitorSnapshot{
			{Name: "DP-1", Enabled: true, Width: 1920, Height: 1080, RefreshRate: 60, Scale: 1},
		},
	}
	layoutFixture := hyprLayoutSnapshot{
		Name:    "coding",
		SavedAt: "2026-04-10T11:00:00Z",
		Windows: []hyprSavedWindow{
			{Class: "foot", Title: "editor", Workspace: 1, Mapped: true},
			{Class: "firefox", Title: "docs", Workspace: 2, Mapped: true},
		},
	}

	writeJSONFixture(t, filepath.Join(monitorDir, "desk-a.json"), monitorFixture)
	writeJSONFixture(t, filepath.Join(layoutDir, "coding.json"), layoutFixture)

	monitors, err := listHyprMonitorPresets()
	if err != nil {
		t.Fatalf("list monitor presets: %v", err)
	}
	if len(monitors.Presets) != 1 {
		t.Fatalf("preset count = %d, want 1", len(monitors.Presets))
	}
	if monitors.Presets[0].Name != "desk-a" || monitors.Presets[0].MonitorCount != 1 {
		t.Fatalf("unexpected preset listing: %#v", monitors.Presets[0])
	}

	layouts, err := listHyprLayouts()
	if err != nil {
		t.Fatalf("list layouts: %v", err)
	}
	if len(layouts.Layouts) != 1 {
		t.Fatalf("layout count = %d, want 1", len(layouts.Layouts))
	}
	if layouts.Layouts[0].Name != "coding" || layouts.Layouts[0].WindowCount != 2 {
		t.Fatalf("unexpected layout listing: %#v", layouts.Layouts[0])
	}
}

func TestRestoreHyprLayoutDryRun(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	stateHome := filepath.Join(root, "state")
	binDir := filepath.Join(root, "bin")
	clientsPath := filepath.Join(root, "clients.json")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, dir := range []string{
		home,
		stateHome,
		binDir,
		filepath.Join(stateHome, "dotfiles", "desktop-control", "hypr", "layouts"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	clientsJSON := `[
  {
    "address": "0xabc",
    "class": "foot",
    "title": "editor",
    "initialClass": "foot",
    "initialTitle": "editor",
    "workspace": {"id": 1, "name": "1"},
    "monitor": 0,
    "floating": false,
    "mapped": true,
    "pinned": false,
    "fullscreen": false,
    "fullscreenMode": 0,
    "at": [100, 100],
    "size": [1200, 800]
  }
]`
	if err := os.WriteFile(clientsPath, []byte(clientsJSON), 0o644); err != nil {
		t.Fatalf("write clients fixture: %v", err)
	}
	writeExecutableScript(t, filepath.Join(binDir, "hyprctl"), `#!/usr/bin/env bash
set -euo pipefail
case "${1:-} ${2:-}" in
	"clients -j")
		cat "${FAKE_HYPR_CLIENTS_JSON}"
		;;
	"monitors -j")
		printf '[{"name":"DP-1","width":2560,"height":1440,"refreshRate":144,"x":0,"y":0,"scale":1,"transform":0}]'
		;;
	"monitors all")
		if [[ "${2:-}" == "all" && "${3:-}" == "-j" ]]; then
			printf '[{"name":"DP-1","width":2560,"height":1440,"refreshRate":144,"x":0,"y":0,"scale":1,"transform":0}]'
			exit 0
		fi
		;;
	*)
		printf 'unexpected hyprctl invocation: %s\n' "$*" >&2
		exit 1
		;;
esac
`)
	t.Setenv("FAKE_HYPR_CLIENTS_JSON", clientsPath)

	layout := hyprLayoutSnapshot{
		Name:    "coding",
		SavedAt: "2026-04-10T12:00:00Z",
		Windows: []hyprSavedWindow{
			{
				Class:        "foot",
				Title:        "editor",
				InitialClass: "foot",
				InitialTitle: "editor",
				Workspace:    2,
				Floating:     true,
				Mapped:       true,
				Pinned:       true,
				Position:     [2]int{50, 60},
				Size:         [2]int{1400, 900},
			},
			{
				Class:  "firefox",
				Title:  "docs",
				Mapped: true,
			},
		},
	}
	layoutPath, err := hyprLayoutPath(layout.Name)
	if err != nil {
		t.Fatalf("hyprLayoutPath: %v", err)
	}
	writeJSONFixture(t, layoutPath, layout)

	out, err := restoreHyprLayout(layout.Name, true, false)
	if err != nil {
		t.Fatalf("restoreHyprLayout dry-run: %v", err)
	}

	if out.Applied {
		t.Fatal("expected dry-run restore to leave applied=false")
	}
	if len(out.Windows) != 2 {
		t.Fatalf("window status count = %d, want 2", len(out.Windows))
	}
	if out.Windows[0].Status != "matched" {
		t.Fatalf("first window status = %q, want matched", out.Windows[0].Status)
	}
	actions := strings.Join(out.Windows[0].Actions, "|")
	for _, want := range []string{
		"movetoworkspacesilent 2,address:0xabc",
		"togglefloating address:0xabc",
		"movewindowpixel exact 50 60,address:0xabc",
		"resizewindowpixel exact 1400 900,address:0xabc",
		"pin address:0xabc",
	} {
		if !strings.Contains(actions, want) {
			t.Errorf("expected actions to contain %q; actions=%q", want, actions)
		}
	}
	if out.Windows[1].Status != "unresolved" {
		t.Fatalf("second window status = %q, want unresolved", out.Windows[1].Status)
	}
	if len(out.Unresolved) != 1 || out.Unresolved[0] != "firefox docs" {
		t.Fatalf("unresolved = %v, want [firefox docs]", out.Unresolved)
	}
}

func writeJSONFixture(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}
