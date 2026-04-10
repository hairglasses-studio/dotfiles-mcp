package dotfiles

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

func writeExitZeroCommands(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		writeTestExecutable(t, dir, name, "#!/bin/sh\nexit 0\n")
	}
}

func callDiscoveryTool(t *testing.T, name string, args map[string]any) *registry.CallToolResult {
	t.Helper()
	module := &DotfilesDiscoveryModule{
		reg:     registry.NewToolRegistry(),
		version: dotfilesMCPVersion,
	}
	td := findModuleTool(t, module, name)
	req := registry.CallToolRequest{}
	req.Params.Arguments = args
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s handler error: %v", name, err)
	}
	return result
}

func unmarshalDesktopStatusResult(t *testing.T, result *registry.CallToolResult) dotfilesDesktopStatusOutput {
	t.Helper()
	var out dotfilesDesktopStatusOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal desktop status: %v", err)
	}
	return out
}

func detailContainsSubstring(details []string, needle string) bool {
	for _, detail := range details {
		if strings.Contains(detail, needle) {
			return true
		}
	}
	return false
}

func countString(items []string, want string) int {
	count := 0
	for _, item := range items {
		if item == want {
			count++
		}
	}
	return count
}

func writeDesktopStatusCommandFixtures(t *testing.T, binDir string) {
	t.Helper()

	writeExitZeroCommands(t, binDir,
		"hyprshell",
		"hypr-dock",
		"hyprdynamicmonitors",
		"hyprland-autoname-workspaces",
		"wayshot",
		"magick",
		"tesseract",
		"ydotool",
		"wtype",
		"wayland-info",
		"grim",
		"wl-copy",
		"wl-paste",
		"swaync-client",
		"dbus-monitor",
		"kitty",
		"ghostty",
	)

	writeTestExecutable(t, binDir, "python3", `#!/bin/sh
if [ "$1" = "-c" ]; then
  exit 0
fi
printf '%s\n' '{}'
`)

	writeTestExecutable(t, binDir, "pgrep", `#!/bin/sh
if [ "$1" = "-c" ] && [ "$2" = "eww" ]; then
  printf '%s\n' "${DOTFILES_TEST_PGREP_EWW_COUNT:-0}"
  exit 0
fi
if [ "$1" = "-x" ]; then
  case "|${DOTFILES_TEST_PGREP_RUNNING_EXACT:-}|" in
    *"|$2|"*) exit 0 ;;
  esac
  exit 1
fi
if [ "$1" = "-f" ]; then
  case "|${DOTFILES_TEST_PGREP_RUNNING_PATTERN:-}|" in
    *"|$2|"*) exit 0 ;;
  esac
  exit 1
fi
exit 1
`)

	writeTestExecutable(t, binDir, "hyprctl", `#!/bin/sh
case "$1" in
  layers)
    printf '%s\n' 'Monitor DP-1:' 'level 1, namespace: sidebar, xywh: 0 0 100 30' 'Monitor DP-2:' 'level 1, namespace: bar, xywh: 0 0 100 30'
    ;;
  *)
    exit 0
    ;;
esac
`)

	writeTestExecutable(t, binDir, "eww", `#!/bin/sh
if [ "$1" = "-c" ]; then
  shift 2
fi
case "$1" in
  list-windows)
    printf '%s\n' 'sidebar' 'bar'
    ;;
  active-windows)
    printf '%s\n' 'DP-1: sidebar' 'DP-2: bar'
    ;;
  state)
    printf '%s\n' '{"bar_shader":"crt"}'
    ;;
  get)
    case "$2" in
      bar_workspaces_dp1) printf '%s\n' '1 2 3' ;;
      bar_workspaces_dp2) printf '%s\n' '4 5 6' ;;
      bar_cpu) printf '%s\n' '12%%' ;;
      bar_mem) printf '%s\n' '44%%' ;;
      bar_vol) printf '%s\n' '50%%' ;;
      bar_shader) printf '%s\n' 'crt' ;;
      *) exit 1 ;;
    esac
    ;;
  ping)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`)
}

func writeDesktopStatusFixtureTree(t *testing.T, homeDir, dotfilesRoot, stateDir, runtimeDir string) {
	t.Helper()

	for _, dir := range []string{
		filepath.Join(homeDir, ".config", "eww"),
		filepath.Join(dotfilesRoot, "scripts"),
		filepath.Join(dotfilesRoot, "kitty", "shaders", "crtty"),
		filepath.Join(dotfilesRoot, "ghostty"),
		filepath.Join(runtimeDir, "hypr"),
		filepath.Join(stateDir, "hypr"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	for path, content := range map[string]string{
		filepath.Join(dotfilesRoot, "scripts", "notification-history-listener.py"): "#!/usr/bin/env python3\n",
		filepath.Join(dotfilesRoot, "scripts", "kitty-shader-playlist.sh"):         "#!/bin/sh\nexit 0\n",
		filepath.Join(dotfilesRoot, "kitty", "kitty.conf"):                         "allow_remote_control yes\n",
		filepath.Join(dotfilesRoot, "ghostty", "config"):                           "theme = fixture\n",
		filepath.Join(stateDir, "hypr", "monitors.dynamic.conf"):                   "monitor=DP-1,preferred,auto,1\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if err := writeNotificationHistoryEntries([]notificationHistoryEntry{
		{
			ID:        "fixture-entry",
			App:       "Fixture App",
			Summary:   "Hello",
			Timestamp: "2026-04-10T00:00:00Z",
			Visible:   true,
			Dismissed: false,
		},
	}); err != nil {
		t.Fatalf("write notification history: %v", err)
	}
}

func TestDotfilesDesktopStatusReadyWithFixtures(t *testing.T) {
	homeDir := t.TempDir()
	dotfilesRoot := t.TempDir()
	stateDir := t.TempDir()
	runtimeDir := t.TempDir()
	binDir := t.TempDir()

	t.Setenv("HOME", homeDir)
	t.Setenv("DOTFILES_DIR", dotfilesRoot)
	t.Setenv("DOTFILES_MCP_PROFILE", "desktop")
	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-fixture")
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "fixture-hypr")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/tmp/fixture-bus")
	t.Setenv("PATH", binDir)
	t.Setenv("DOTFILES_TEST_PGREP_RUNNING_EXACT", "hyprshell|hypr-dock|hyprdynamicmonitors|hyprland-autoname-workspaces|eww")
	t.Setenv("DOTFILES_TEST_PGREP_RUNNING_PATTERN", "notification-history-listener.py")
	t.Setenv("DOTFILES_TEST_PGREP_EWW_COUNT", "2")

	writeDesktopStatusFixtureTree(t, homeDir, dotfilesRoot, stateDir, runtimeDir)
	writeDesktopStatusCommandFixtures(t, binDir)

	result := callDiscoveryTool(t, "dotfiles_desktop_status", map[string]any{})
	out := unmarshalDesktopStatusResult(t, result)

	if out.Profile != "desktop" {
		t.Fatalf("profile = %q, want desktop", out.Profile)
	}
	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok", out.Status)
	}
	if out.Runtime.XDGRuntimeDir != runtimeDir {
		t.Fatalf("runtime dir = %q, want %q", out.Runtime.XDGRuntimeDir, runtimeDir)
	}
	if out.Runtime.WaylandDisplay != "wayland-fixture" {
		t.Fatalf("wayland display = %q, want wayland-fixture", out.Runtime.WaylandDisplay)
	}
	if out.Runtime.HyprlandInstanceSignature != "fixture-hypr" {
		t.Fatalf("hyprland signature = %q, want fixture-hypr", out.Runtime.HyprlandInstanceSignature)
	}
	if out.Runtime.HyprlandSocketDir != filepath.Join(runtimeDir, "hypr") {
		t.Fatalf("hypr socket dir = %q, want %q", out.Runtime.HyprlandSocketDir, filepath.Join(runtimeDir, "hypr"))
	}
	if len(out.MissingCommands) != 0 {
		t.Fatalf("expected no missing commands, got %v", out.MissingCommands)
	}

	for name, capability := range map[string]dotfilesDesktopCapability{
		"hyprland":       out.Hyprland,
		"shell":          out.Shell,
		"screenshot":     out.Screenshot,
		"ocr":            out.OCR,
		"input":          out.Input,
		"accessibility":  out.Accessibility,
		"desktopSession": out.DesktopSession,
		"eww":            out.Eww,
		"notifications":  out.Notifications,
		"terminal":       out.Terminal,
		"shader":         out.Shader,
	} {
		if !capability.Ready {
			t.Fatalf("%s should be ready, missing=%v details=%v", name, capability.Missing, capability.Details)
		}
	}

	if !detailContainsSubstring(out.Eww.Details, "daemon running (2 processes)") {
		t.Fatalf("expected eww detail to report daemon count, got %v", out.Eww.Details)
	}
	if !detailContainsSubstring(out.Notifications.Details, "1 tracked notification entries") {
		t.Fatalf("expected notification history detail, got %v", out.Notifications.Details)
	}
	if !detailContainsSubstring(out.Shader.Details, "kitty shader playlist script available") {
		t.Fatalf("expected shader script detail, got %v", out.Shader.Details)
	}
}

func TestDotfilesDesktopStatusDegradedWithoutCommands(t *testing.T) {
	homeDir := t.TempDir()
	dotfilesRoot := t.TempDir()
	stateDir := t.TempDir()
	runtimeDir := t.TempDir()
	emptyBin := t.TempDir()

	t.Setenv("HOME", homeDir)
	t.Setenv("DOTFILES_DIR", dotfilesRoot)
	t.Setenv("DOTFILES_MCP_PROFILE", "default")
	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("PATH", emptyBin)

	result := callDiscoveryTool(t, "dotfiles_desktop_status", map[string]any{})
	out := unmarshalDesktopStatusResult(t, result)

	if out.Profile != "default" {
		t.Fatalf("profile = %q, want default", out.Profile)
	}
	if out.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", out.Status)
	}
	if out.Hyprland.Ready {
		t.Fatal("expected hyprland readiness to be false")
	}
	if out.Accessibility.Ready {
		t.Fatal("expected accessibility readiness to be false")
	}
	if out.DesktopSession.Ready {
		t.Fatal("expected desktop session readiness to be false")
	}
	if out.Terminal.Ready {
		t.Fatal("expected terminal readiness to be false")
	}
	if !containsString(out.MissingCommands, "hyprctl") {
		t.Fatalf("expected missing commands to include hyprctl, got %v", out.MissingCommands)
	}
	if !containsString(out.MissingCommands, "python3") {
		t.Fatalf("expected missing commands to include python3, got %v", out.MissingCommands)
	}
	if !containsString(out.MissingCommands, "wayland-info") {
		t.Fatalf("expected missing commands to include wayland-info, got %v", out.MissingCommands)
	}
	if countString(out.MissingCommands, "magick") != 1 {
		t.Fatalf("expected missing command magick once after dedupe, got %v", out.MissingCommands)
	}
	if !containsString(out.Accessibility.Missing, "python3") {
		t.Fatalf("expected accessibility missing to include python3, got %v", out.Accessibility.Missing)
	}
	if !containsString(out.Eww.Missing, "eww") {
		t.Fatalf("expected eww missing to include eww, got %v", out.Eww.Missing)
	}
	if !containsString(out.Notifications.Missing, "swaync-client") {
		t.Fatalf("expected notifications missing to include swaync-client, got %v", out.Notifications.Missing)
	}
	if !containsString(out.Shader.Missing, "kitty-shader-playlist.sh") {
		t.Fatalf("expected shader missing to include playlist script, got %v", out.Shader.Missing)
	}
}
