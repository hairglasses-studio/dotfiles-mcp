package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSystemHealthFixtures(t *testing.T, binDir string) {
	t.Helper()

	writeTestExecutable(t, binDir, "sensors", `#!/bin/sh
cat <<'EOF'
{"k10temp-pci-00c3":{"Tctl":{"temp1_input":63.0}}}
EOF
`)
	writeTestExecutable(t, binDir, "nvidia-smi", `#!/bin/sh
printf '%s\n' 'NVIDIA GeForce RTX 3080, 55, 24, 18, 1024, 10240, 90.5'
`)
	writeTestExecutable(t, binDir, "df", `#!/bin/sh
cat <<'EOF'
Mounted Size Used Avail Use%
/ 100G 40G 60G 40%
/home 500G 120G 380G 24%
EOF
`)
	writeTestExecutable(t, binDir, "checkupdates", "#!/bin/sh\nexit 2\n")
	writeTestExecutable(t, binDir, "yay", "#!/bin/sh\nexit 0\n")
	writeTestExecutable(t, binDir, "who", "#!/bin/sh\nprintf '%s\n' '         system boot  2026-04-09 10:30'\n")
}

func unmarshalWorkstationDiagnosticsResult(t *testing.T, resultText string) dotfilesWorkstationDiagnosticsOutput {
	t.Helper()
	var out dotfilesWorkstationDiagnosticsOutput
	if err := json.Unmarshal([]byte(resultText), &out); err != nil {
		t.Fatalf("unmarshal workstation diagnostics: %v", err)
	}
	return out
}

func hasIssueComponent(issues []dotfilesWorkstationDiagnosticIssue, component string) bool {
	for _, issue := range issues {
		if issue.Component == component {
			return true
		}
	}
	return false
}

func TestDotfilesWorkstationDiagnosticsReadyWithFixtures(t *testing.T) {
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
	t.Setenv("PATH", binDir+":/usr/bin:/bin")
	t.Setenv("DOTFILES_TEST_PGREP_RUNNING_EXACT", "hyprland|eww|mako|swww-daemon|hypridle|hyprshell|hypr-dock|hyprdynamicmonitors|hyprland-autoname-workspaces|swaync")
	t.Setenv("DOTFILES_TEST_PGREP_RUNNING_PATTERN", "notification-history-listener.py")
	t.Setenv("DOTFILES_TEST_PGREP_EWW_COUNT", "2")

	writeDesktopStatusFixtureTree(t, homeDir, dotfilesRoot, stateDir, runtimeDir)
	writeDesktopStatusCommandFixtures(t, binDir)
	writeSystemHealthFixtures(t, binDir)

	result := callDiscoveryTool(t, "dotfiles_workstation_diagnostics", map[string]any{
		"symptom":         "bar missing after login",
		"rice_level":      "quick",
		"warn_memory_pct": 100,
	})
	out := unmarshalWorkstationDiagnosticsResult(t, extractTextFromResult(t, result))

	if out.Status != "ok" {
		t.Fatalf("status = %q, want ok; issues=%+v errors=%v capabilities=%+v", out.Status, out.Issues, out.Errors, out.Capabilities)
	}
	if out.Profile != "desktop" {
		t.Fatalf("profile = %q, want desktop", out.Profile)
	}
	if out.WorkflowURI != "dotfiles://workflows/workstation-diagnose" {
		t.Fatalf("workflow uri = %q", out.WorkflowURI)
	}
	if out.PromptName != "dotfiles_diagnose_workstation" {
		t.Fatalf("prompt name = %q", out.PromptName)
	}
	if out.Capabilities.Ready != out.Capabilities.Total || out.Capabilities.Total != 11 {
		t.Fatalf("unexpected capability summary: %+v", out.Capabilities)
	}
	if out.IssueCount != 0 || len(out.Issues) != 0 {
		t.Fatalf("expected no issues, got %+v", out.Issues)
	}
	if len(out.Errors) != 0 {
		t.Fatalf("expected no collection errors, got %v", out.Errors)
	}
	if !strings.Contains(out.ReportMarkdown, "Workstation Diagnostics") {
		t.Fatalf("expected markdown report header, got %q", out.ReportMarkdown)
	}
	if !containsString(out.SuggestedTools, "dotfiles_workstation_diagnostics") {
		t.Fatalf("expected suggested tools to include diagnostics front door, got %v", out.SuggestedTools)
	}
}

func TestDotfilesWorkstationDiagnosticsDegradedWhenDesktopReadinessFails(t *testing.T) {
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
	t.Setenv("PATH", binDir+":/usr/bin:/bin")

	if err := os.MkdirAll(filepath.Join(dotfilesRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	writeSystemHealthFixtures(t, binDir)

	result := callDiscoveryTool(t, "dotfiles_workstation_diagnostics", map[string]any{
		"symptom": "semantic targeting unavailable",
	})
	out := unmarshalWorkstationDiagnosticsResult(t, extractTextFromResult(t, result))

	if out.Status != "warn" {
		t.Fatalf("status = %q, want warn", out.Status)
	}
	if out.IssueCount == 0 {
		t.Fatal("expected degraded desktop issues")
	}
	if !hasIssueComponent(out.Issues, "desktop.hyprland") {
		t.Fatalf("expected desktop.hyprland issue, got %+v", out.Issues)
	}
	if !hasIssueComponent(out.Issues, "desktop.accessibility") {
		t.Fatalf("expected desktop.accessibility issue, got %+v", out.Issues)
	}
	if out.Capabilities.Degraded == 0 {
		t.Fatalf("expected degraded capabilities, got %+v", out.Capabilities)
	}
}
