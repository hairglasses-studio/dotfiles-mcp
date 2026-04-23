package dotfiles

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func TestEwwDesktopModuleRegistration(t *testing.T) {
	m := &EwwDesktopModule{}
	tools := m.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 eww desktop tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"dotfiles_eww_inspect",
		"dotfiles_eww_reload",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestEwwInspectHandler(t *testing.T) {
	env := setupFakeEwwEnv(t)

	m := &EwwDesktopModule{}
	td := findEwwTool(t, m, "dotfiles_eww_inspect")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"include_raw": true,
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out EwwInspectOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal inspect output: %v; text=%s", err, text)
	}

	if !out.DaemonRunning {
		t.Fatal("expected daemon_running=true")
	}
	if out.DaemonCount != 1 {
		t.Fatalf("daemon_count = %d, want 1", out.DaemonCount)
	}
	if out.WaybarRunning {
		t.Fatal("expected waybar_running=false")
	}
	if got := strings.Join(out.ActiveWindows, ","); got != "music,sidebar" {
		t.Fatalf("active_windows = %q, want music,sidebar", got)
	}
	if got := strings.Join(out.DefinedWindows, ","); got != "music,sidebar" {
		t.Fatalf("defined_windows = %q, want music,sidebar", got)
	}
	if len(out.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(out.Layers))
	}
	if out.Variables["active"] != "yes" {
		t.Fatalf("variables[active] = %v, want yes", out.Variables["active"])
	}
	if !strings.Contains(out.RawState, `"bar_cpu":"42"`) {
		t.Fatalf("raw_state missing expected JSON: %q", out.RawState)
	}
	if !strings.Contains(out.RawWindows, "sidebar") {
		t.Fatalf("raw_windows missing expected window names: %q", out.RawWindows)
	}

	logData, err := os.ReadFile(env.logPath)
	if err != nil {
		t.Fatalf("read fake eww log: %v", err)
	}
	logText := string(logData)
	for _, want := range []string{
		"get bar_workspaces_dp1",
		"get bar_workspaces_dp2",
		"get bar_cpu",
		"get bar_mem",
		"get bar_vol",
		"get bar_shader",
	} {
		if !strings.Contains(logText, want) {
			t.Errorf("expected inspect path to query %q; log=%q", want, logText)
		}
	}
}

func TestEwwReloadHandler_TargetedWindow(t *testing.T) {
	env := setupFakeEwwEnv(t)

	m := &EwwDesktopModule{}
	td := findEwwTool(t, m, "dotfiles_eww_reload")

	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"window": "sidebar",
	}

	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := extractText(t, result)
	var out EwwReloadOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal reload output: %v; text=%s", err, text)
	}

	if out.Mode != "targeted" {
		t.Fatalf("mode = %q, want targeted", out.Mode)
	}
	if out.Fallback {
		t.Fatal("expected fallback=false")
	}
	if got := strings.Join(out.Actions, ","); got != "close sidebar,open sidebar" {
		t.Fatalf("actions = %q, want close/open targeted sequence", got)
	}
	if got := strings.Join(out.ActiveBefore, ","); got != "music,sidebar" {
		t.Fatalf("active_before = %q, want music,sidebar", got)
	}
	if got := strings.Join(out.ActiveAfter, ","); got != "music,sidebar" {
		t.Fatalf("active_after = %q, want music,sidebar", got)
	}
	if out.Error != "" {
		t.Fatalf("expected empty error, got %q", out.Error)
	}

	logData, err := os.ReadFile(env.logPath)
	if err != nil {
		t.Fatalf("read fake eww log: %v", err)
	}
	logText := string(logData)
	for _, want := range []string{"close sidebar", "open sidebar"} {
		if !strings.Contains(logText, want) {
			t.Errorf("missing %q in fake eww log: %q", want, logText)
		}
	}
	if strings.Contains(logText, "reload") {
		t.Fatalf("unexpected fallback reload in fake eww log: %q", logText)
	}
}

func findEwwTool(t *testing.T, m *EwwDesktopModule, name string) registry.ToolDefinition {
	t.Helper()
	for _, td := range m.Tools() {
		if td.Tool.Name == name {
			return td
		}
	}
	t.Fatalf("tool %q not found", name)
	return registry.ToolDefinition{}
}

type fakeEwwEnv struct {
	logPath string
}

func setupFakeEwwEnv(t *testing.T) fakeEwwEnv {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	stateHome := filepath.Join(root, "state")
	dotfiles := filepath.Join(root, "dotfiles")
	binDir := filepath.Join(root, "bin")

	for _, dir := range []string{
		home,
		stateHome,
		filepath.Join(dotfiles, "eww"),
		binDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	logPath := filepath.Join(root, "eww.log")

	writeEwwExecutableScript(t, filepath.Join(binDir, "eww"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "-c" ]]; then
	shift 2
fi
cmd="${1:-}"
if [[ $# -gt 0 ]]; then
	shift
fi
	case "${cmd}" in
		list-windows)
			printf 'sidebar\nmusic\n'
			;;
		active-windows)
			printf 'DP-1: sidebar\nDP-2: music\n'
			;;
		state)
			printf '{"bar_cpu":"42","bar_shader":"crt","active":"yes"}'
			;;
		get)
			if [[ -n "${FAKE_EWW_LOG:-}" ]]; then
				printf '%s %s\n' "${cmd}" "$*" >>"${FAKE_EWW_LOG}"
			fi
			printf 'value-for-%s' "${1:-}"
			;;
	close|open|reload|daemon|ping)
		if [[ -n "${FAKE_EWW_LOG:-}" ]]; then
			printf '%s %s\n' "${cmd}" "$*" >>"${FAKE_EWW_LOG}"
		fi
		if [[ "${cmd}" == "reload" && "${FAKE_EWW_RELOAD_FAIL:-0}" == "1" ]]; then
			printf 'reload failed\n' >&2
			exit 1
		fi
		;;
	*)
		printf 'unexpected eww command: %s %s\n' "${cmd}" "$*" >&2
		exit 1
		;;
esac
`)

	writeEwwExecutableScript(t, filepath.Join(binDir, "pgrep"), `#!/usr/bin/env bash
set -euo pipefail
case "${1:-} ${2:-}" in
	"-x eww")
		exit 0
		;;
	"-x waybar")
		exit 1
		;;
	"-c eww")
		printf '1\n'
		exit 0
		;;
	"-o eww")
		printf '4242\n'
		exit 0
		;;
	*)
		exit 1
		;;
esac
`)

	writeEwwExecutableScript(t, filepath.Join(binDir, "hyprctl"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "layers" ]]; then
	cat <<'EOF'
Monitor DP-1:
layer 0 namespace: sidebar, address: 0x1, xywh: 0 0 100 100
Monitor DP-2:
layer 0 namespace: music, address: 0x2, xywh: 10 20 200 120
EOF
	exit 0
fi
printf 'unexpected hyprctl invocation: %s\n' "$*" >&2
exit 1
`)

	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("DOTFILES_DIR", dotfiles)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_EWW_LOG", logPath)

	return fakeEwwEnv{logPath: logPath}
}

func writeEwwExecutableScript(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write script %s: %v", path, err)
	}
}
