package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

func unmarshalLauncherAuditResult(t *testing.T, result *registry.CallToolResult) dotfilesLauncherAuditOutput {
	t.Helper()
	var out dotfilesLauncherAuditOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal launcher audit: %v", err)
	}
	return out
}

func unmarshalBarAuditResult(t *testing.T, result *registry.CallToolResult) dotfilesBarAuditOutput {
	t.Helper()
	var out dotfilesBarAuditOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal bar audit: %v", err)
	}
	return out
}

func auditCommandMap(items []dotfilesCommandAvailability) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item.Name] = item.Available
	}
	return out
}

func containsSubstring(items []string, needle string) bool {
	for _, item := range items {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}

func TestDotfilesLauncherAuditReady(t *testing.T) {
	homeDir := t.TempDir()
	dotfilesRoot := t.TempDir()
	runtimeDir := t.TempDir()
	binDir := t.TempDir()

	t.Setenv("HOME", homeDir)
	t.Setenv("DOTFILES_DIR", dotfilesRoot)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-fixture")
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "fixture-hypr")
	t.Setenv("PATH", binDir)

	for _, dir := range []string{
		filepath.Join(dotfilesRoot, "scripts", "lib"),
		filepath.Join(runtimeDir, "hypr", "fixture-hypr"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	for path, content := range map[string]string{
		filepath.Join(dotfilesRoot, "scripts", "app-launcher.sh"):           "#!/bin/sh\nexit 0\n",
		filepath.Join(dotfilesRoot, "scripts", "app-switcher.sh"):           "#!/bin/sh\nexit 0\n",
		filepath.Join(dotfilesRoot, "scripts", "lib", "launcher.sh"):        "#!/bin/sh\nexit 0\n",
		filepath.Join(runtimeDir, "hypr", "fixture-hypr", "hyprshell.sock"): "",
	} {
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	writeTestExecutable(t, binDir, "hyprshell", "#!/bin/sh\nexit 0\n")
	writeTestExecutable(t, binDir, "wofi", "#!/bin/sh\nexit 0\n")
	writeTestExecutable(t, binDir, "jq", "#!/bin/sh\nexit 0\n")
	writeTestExecutable(t, binDir, "systemctl", `#!/bin/sh
if [ "$1" = "--user" ] && [ "$2" = "is-active" ] && [ "$3" = "dotfiles-hyprshell.service" ]; then
  printf '%s\n' 'active'
  exit 0
fi
printf '%s\n' 'inactive'
exit 3
`)
	writeTestExecutable(t, binDir, "journalctl", "#!/bin/sh\nprintf '%s\n' '-- No entries --'\n")
	writeTestExecutable(t, binDir, "hyprctl", `#!/bin/sh
if [ "$1" = "layers" ]; then
  printf '%s\n' '{"layers":[{"namespace":"hyprshell_switch"}]}'
  exit 0
fi
printf '%s\n' '{}'
`)
	writeTestExecutable(t, binDir, "pgrep", `#!/bin/sh
if [ "$1" = "-x" ] && [ "$2" = "hyprshell" ]; then
  exit 0
fi
exit 1
`)

	result := callDiscoveryTool(t, "dotfiles_launcher_audit", map[string]any{})
	out := unmarshalLauncherAuditResult(t, result)
	commandMap := auditCommandMap(out.Commands)

	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok", out.Status)
	}
	if !out.Scripts.AppLauncherExists || !out.Scripts.AppSwitcherExists || !out.Scripts.LibraryExists {
		t.Fatalf("expected launcher scripts present, got %+v", out.Scripts)
	}
	if !out.Hyprshell.CommandAvailable || !out.Hyprshell.ProcessRunning || !out.Hyprshell.SocketExists {
		t.Fatalf("expected hyprshell ready, got %+v", out.Hyprshell)
	}
	if out.Hyprshell.ServiceState != "active" {
		t.Fatalf("service state = %q, want active", out.Hyprshell.ServiceState)
	}
	if !out.Hyprshell.SwitchVisible {
		t.Fatal("expected switch layer visible")
	}
	if !out.FallbackReady {
		t.Fatal("expected fallback launcher ready")
	}
	if !commandMap["wofi"] || !commandMap["hyprshell"] || !commandMap["jq"] {
		t.Fatalf("unexpected command availability: %+v", commandMap)
	}
	if len(out.LogSignals) != 0 {
		t.Fatalf("expected no log signals, got %v", out.LogSignals)
	}
}

func TestDotfilesBarAuditDetectsMissingMonitorCoverage(t *testing.T) {
	dotfilesRoot := t.TempDir()
	binDir := t.TempDir()

	t.Setenv("DOTFILES_DIR", dotfilesRoot)
	t.Setenv("PATH", binDir)

	if err := os.MkdirAll(filepath.Join(dotfilesRoot, "ironbar"), 0o755); err != nil {
		t.Fatalf("mkdir ironbar: %v", err)
	}
	config := `
[monitors."DP-3"]
[[monitors."DP-3".start]]
type = "workspaces"
[[monitors."DP-3".end]]
type = "tray"

[monitors."DP-2"]
[[monitors."DP-2".start]]
type = "workspaces"
[[monitors."DP-2".end]]
type = "clock"
`
	if err := os.WriteFile(filepath.Join(dotfilesRoot, "ironbar", "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	writeTestExecutable(t, binDir, "systemctl", `#!/bin/sh
if [ "$1" = "--user" ] && [ "$2" = "is-active" ] && [ "$3" = "ironbar.service" ]; then
  printf '%s\n' 'active'
  exit 0
fi
printf '%s\n' 'inactive'
exit 3
`)
	writeTestExecutable(t, binDir, "journalctl", `#!/bin/sh
printf '%s\n' 'Apr 14 10:00:00 host ironbar[123]: Attempted to update menu at /tmp/tray but could not find it'
`)
	writeTestExecutable(t, binDir, "hyprctl", `#!/bin/sh
if [ "$1" = "monitors" ] && [ "$2" = "-j" ]; then
  printf '%s\n' '[{"name":"DP-3","width":3840,"height":1080,"refreshRate":120.0,"x":0,"y":0,"scale":2.0,"focused":true,"activeWorkspace":{"id":1}},{"name":"DP-2","width":2560,"height":1440,"refreshRate":180.0,"x":3840,"y":0,"scale":2.0,"focused":false,"activeWorkspace":{"id":2}},{"name":"DP-1","width":1920,"height":1080,"refreshRate":60.0,"x":6400,"y":0,"scale":1.0,"focused":false,"activeWorkspace":{"id":3}}]'
  exit 0
fi
printf '%s\n' '[]'
`)

	result := callDiscoveryTool(t, "dotfiles_bar_audit", map[string]any{})
	out := unmarshalBarAuditResult(t, result)

	if out.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", out.Status)
	}
	if !out.ConfigExists || !out.MonitorSpecific {
		t.Fatalf("expected config + monitor-specific layout, got exists=%v monitorSpecific=%v", out.ConfigExists, out.MonitorSpecific)
	}
	if out.ServiceState != "active" {
		t.Fatalf("service state = %q, want active", out.ServiceState)
	}
	if len(out.ConfiguredMonitors) != 2 {
		t.Fatalf("configured monitor count = %d, want 2", len(out.ConfiguredMonitors))
	}
	if len(out.LiveMonitors) != 3 {
		t.Fatalf("live monitor count = %d, want 3", len(out.LiveMonitors))
	}
	if !containsString(out.MissingMonitorConfigs, "DP-1") {
		t.Fatalf("expected DP-1 missing coverage, got %v", out.MissingMonitorConfigs)
	}
	if len(out.StaleMonitorConfigs) != 0 {
		t.Fatalf("expected no stale configs, got %v", out.StaleMonitorConfigs)
	}
	if !containsSubstring(out.LogSignals, "tray/menu sync warnings") {
		t.Fatalf("expected tray/menu log signal, got %v", out.LogSignals)
	}
	if !containsSubstring(out.Recommendations, "DP-1") {
		t.Fatalf("expected recommendation mentioning DP-1, got %v", out.Recommendations)
	}
}
