package dotfiles

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/mcpkit/registry"
)

func writeTestExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", name, err)
	}
	return path
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func saveTestDesktopSessionRecord(t *testing.T, record desktopSessionRecord) desktopSessionRecord {
	t.Helper()
	if strings.TrimSpace(record.ID) == "" {
		record.ID = "test-session"
	}
	if strings.TrimSpace(record.StartedAt) == "" {
		record.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(record.Status) == "" {
		record.Status = "connected"
	}
	if err := saveDesktopSessionRecord(record); err != nil {
		t.Fatalf("save session record: %v", err)
	}
	return record
}

func callDesktopSessionTool(t *testing.T, name string, args map[string]any) *registry.CallToolResult {
	t.Helper()
	td := findModuleTool(t, &DesktopSessionModule{}, name)
	req := registry.CallToolRequest{}
	req.Params.Arguments = args
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s handler error: %v", name, err)
	}
	return result
}

func unmarshalDesktopSessionRecordResult(t *testing.T, result *registry.CallToolResult) desktopSessionRecord {
	t.Helper()
	var out desktopSessionRecord
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal desktop session record: %v", err)
	}
	return out
}

func unmarshalSessionWindowsResult(t *testing.T, result *registry.CallToolResult) SessionWindowsOutput {
	t.Helper()
	var out SessionWindowsOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal session windows output: %v", err)
	}
	return out
}

func unmarshalSessionListResult(t *testing.T, result *registry.CallToolResult) SessionListOutput {
	t.Helper()
	var out SessionListOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal session list output: %v", err)
	}
	return out
}

func unmarshalSessionStatusResult(t *testing.T, result *registry.CallToolResult) SessionStatusOutput {
	t.Helper()
	var out SessionStatusOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal session status output: %v", err)
	}
	return out
}

func unmarshalSessionLogResult(t *testing.T, result *registry.CallToolResult) SessionLogOutput {
	t.Helper()
	var out SessionLogOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal session log output: %v", err)
	}
	return out
}

func unmarshalSessionAppsResult(t *testing.T, result *registry.CallToolResult) SessionAppsOutput {
	t.Helper()
	var out SessionAppsOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal session apps output: %v", err)
	}
	return out
}

func unmarshalSessionWaitReadyResult(t *testing.T, result *registry.CallToolResult) SessionWaitReadyOutput {
	t.Helper()
	var out SessionWaitReadyOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal session wait-ready output: %v", err)
	}
	return out
}

func callDesktopSemanticTool(t *testing.T, name string, args map[string]any) *registry.CallToolResult {
	t.Helper()
	td := findModuleTool(t, &DesktopSemanticModule{}, name)
	req := registry.CallToolRequest{}
	req.Params.Arguments = args
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s handler error: %v", name, err)
	}
	return result
}

func writeSemanticFixturePython(t *testing.T, dir string) {
	t.Helper()
	writeTestExecutable(t, dir, "python3", `#!/bin/sh
if [ "$1" = "-c" ]; then
  exit 0
fi
cmd="$2"
case "$cmd" in
  list_apps)
    printf '%s\n' '{"apps":[{"id":0,"name":"Fixture App","role":"application"}]}'
    ;;
  list_windows)
    printf '%s\n' '{"windows":[{"name":"Fixture Window","ref":"ref_0","path":"0","bounds":{"x":10,"y":20,"width":800,"height":600},"app":{"name":"Fixture App"}}]}'
    ;;
  get_tree)
    printf '%s\n' '{"tree":{"name":"Fixture Window","role":"frame","ref":"ref_0","path":"0","children":[{"name":"Save","role":"push button","ref":"ref_0_0","path":"0/0","actions":["press"]},{"name":"","role":"entry","ref":"ref_0_1","path":"0/1","value":"fixture-text","value_kind":"text","relations":{"labelled by":["Full Name"]},"attributes":{"placeholder":"Type full name"}},{"name":"Volume","role":"slider","ref":"ref_0_2","path":"0/2","value":5,"value_kind":"numeric"},{"name":"Accept Terms","role":"check box","ref":"ref_0_3","path":"0/3"},{"name":"Submit","role":"push button","ref":"ref_0_4","path":"0/4","actions":["press"]}]}}'
    ;;
  find)
    printf '%s\n' '{"matched":true,"element":{"name":"Save","role":"push button","ref":"ref_0_0","path":"0/0"}}'
    ;;
  find_all)
    printf '%s\n' '{"matched":true,"count":2,"elements":[{"name":"Save","role":"push button","ref":"ref_0_0","path":"0/0"},{"name":"Cancel","role":"push button","ref":"ref_0_2","path":"0/2"}]}'
    ;;
  focus)
    printf '%s\n' '{"matched":true,"focused":true,"element":{"name":"Fixture Window","role":"frame","ref":"ref_0","path":"0"}}'
    ;;
  read_value)
    printf '%s\n' '{"matched":true,"value":"fixture-text","value_kind":"text","element":{"name":"Name","role":"entry","ref":"ref_0_1","path":"0/1"}}'
    ;;
  set_text)
    printf '%s\n' '{"matched":true,"updated":true,"value":"patched","value_kind":"text","element":{"name":"Name","role":"entry","ref":"ref_0_1","path":"0/1"}}'
    ;;
  set_value)
    printf '%s\n' '{"matched":true,"updated":true,"value":42,"value_kind":"numeric","element":{"name":"Slider","role":"slider","ref":"ref_0_3","path":"0/3"}}'
    ;;
  click)
    printf '%s\n' '{"matched":true,"clicked":true,"element":{"name":"Save","role":"push button","ref":"ref_0_0","path":"0/0"}}'
    ;;
  act)
    printf '%s\n' '{"matched":true,"invoked":true,"action":"press","element":{"name":"Save","role":"push button","ref":"ref_0_0","path":"0/0"}}'
    ;;
  wait)
    printf '%s\n' '{"matched":true,"focused":true,"element":{"name":"Save","role":"push button","ref":"ref_0_0","path":"0/0"}}'
    ;;
  *)
    printf '%s\n' '{"error":"unexpected semantic fixture command"}'
    exit 1
    ;;
esac
`)
}

func writeSessionCommandFixtures(t *testing.T, dir string) {
	t.Helper()
	writeTestExecutable(t, dir, "grim", "#!/bin/sh\nprintf 'PNG' >\"$1\"\n")
	writeTestExecutable(t, dir, "wl-paste", "#!/bin/sh\nprintf '%s' 'fixture clipboard'\n")
	writeTestExecutable(t, dir, "wl-copy", "#!/bin/sh\ncat >/dev/null\n")
	writeTestExecutable(t, dir, "wayland-info", "#!/bin/sh\nprintf '%s\n' 'interface: wl_compositor'\n")
	writeTestExecutable(t, dir, "wtype", "#!/bin/sh\nexit 0\n")
	writeTestExecutable(t, dir, "dbus-send", "#!/bin/sh\nprintf '%s\n' 'method return time=0.0 sender=:1.2 -> destination=:1.3 serial=4 reply_serial=5'\n")
}

func TestDesktopSemanticCapabilities_MissingPython(t *testing.T) {
	stateDir := t.TempDir()
	emptyBin := t.TempDir()

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", emptyBin)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/tmp/test-bus")

	out := desktopSemanticCapabilities()

	if out.Ready {
		t.Fatal("expected semantic capabilities to be not ready without python3")
	}
	if out.PythonAvailable {
		t.Fatal("expected python3 to be unavailable")
	}
	if out.PyATSPIAvailable {
		t.Fatal("expected pyatspi to be unavailable without python3")
	}
	if !containsString(out.Missing, "python3") {
		t.Fatalf("expected missing list to include python3, got %v", out.Missing)
	}
	if strings.TrimSpace(out.HelperPath) == "" {
		t.Fatal("expected helper path to be populated")
	}
	if !pathExists(out.HelperPath) {
		t.Fatalf("expected helper path to exist: %s", out.HelperPath)
	}
	if out.WaylandDisplay != "wayland-test" {
		t.Fatalf("wayland display = %q, want %q", out.WaylandDisplay, "wayland-test")
	}
	if !out.BusAddressPresent {
		t.Fatal("expected dbus session bus to be detected")
	}
}

func TestDesktopSemanticCapabilities_ReadyWithStubPython(t *testing.T) {
	stateDir := t.TempDir()
	binDir := t.TempDir()

	writeTestExecutable(t, binDir, "python3", "#!/bin/sh\nexit 0\n")

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", binDir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-77")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/tmp/test-bus")

	out := desktopSemanticCapabilities()

	if !out.Ready {
		t.Fatalf("expected semantic capabilities to be ready, got missing=%v details=%v", out.Missing, out.Details)
	}
	if !out.PythonAvailable {
		t.Fatal("expected python3 to be available")
	}
	if !out.PyATSPIAvailable {
		t.Fatal("expected pyatspi probe to succeed")
	}
	if len(out.Missing) != 0 {
		t.Fatalf("expected no missing items, got %v", out.Missing)
	}
	if strings.TrimSpace(out.HelperPath) == "" {
		t.Fatal("expected helper path to be populated")
	}
}

func TestDesktopSessionConnect_MissingWaylandDisplay(t *testing.T) {
	stateDir := t.TempDir()
	runtimeDir := t.TempDir()

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")

	result := callDesktopSessionTool(t, "session_connect", map[string]any{})
	if result == nil || !result.IsError {
		t.Fatal("expected error result when live session WAYLAND_DISPLAY cannot be resolved")
	}
	if text := extractTextFromResult(t, result); !strings.Contains(text, "unable to resolve WAYLAND_DISPLAY") {
		t.Fatalf("expected WAYLAND_DISPLAY error, got %q", text)
	}
}

func TestDesktopSessionConnect_WithExplicitOverrides(t *testing.T) {
	stateDir := t.TempDir()
	runtimeDir := t.TempDir()

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/tmp/live-bus")
	t.Setenv("AT_SPI_BUS_ADDRESS", "unix:path=/tmp/atspi-bus")

	result := callDesktopSessionTool(t, "session_connect", map[string]any{
		"name":            "Live Override",
		"xdg_runtime_dir": runtimeDir,
		"wayland_display": "wayland-55",
	})
	if result == nil || result.IsError {
		t.Fatalf("expected successful session_connect result, got %q", extractTextFromResult(t, result))
	}

	out := unmarshalDesktopSessionRecordResult(t, result)
	if out.Name != "Live Override" {
		t.Fatalf("name = %q, want %q", out.Name, "Live Override")
	}
	if out.Backend != "live_wayland" {
		t.Fatalf("backend = %q, want %q", out.Backend, "live_wayland")
	}
	if out.WaylandDisplay != "wayland-55" {
		t.Fatalf("wayland_display = %q, want %q", out.WaylandDisplay, "wayland-55")
	}
	if out.XDGRuntimeDir != runtimeDir {
		t.Fatalf("xdg_runtime_dir = %q, want %q", out.XDGRuntimeDir, runtimeDir)
	}
	if out.DBUSSessionBusAddress != "unix:path=/tmp/live-bus" {
		t.Fatalf("dbus session bus = %q, want %q", out.DBUSSessionBusAddress, "unix:path=/tmp/live-bus")
	}
	if !pathExists(desktopSessionRecordPath(out.ID)) {
		t.Fatalf("expected persisted session record at %s", desktopSessionRecordPath(out.ID))
	}
}

func TestDesktopSessionStart_MissingKWinWayland(t *testing.T) {
	stateDir := t.TempDir()
	emptyBin := t.TempDir()

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", emptyBin)

	result := callDesktopSessionTool(t, "session_start", map[string]any{
		"backend": "kwin_virtual",
	})
	if result == nil || !result.IsError {
		t.Fatal("expected error result when kwin_wayland is unavailable")
	}
	if text := extractTextFromResult(t, result); !strings.Contains(text, "kwin_wayland not found") {
		t.Fatalf("expected kwin_wayland error, got %q", text)
	}
}

func TestDesktopSessionStart_MissingDBusRunSession(t *testing.T) {
	stateDir := t.TempDir()
	binDir := t.TempDir()

	writeTestExecutable(t, binDir, "kwin_wayland", "#!/bin/sh\nexit 0\n")

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", binDir)

	result := callDesktopSessionTool(t, "session_start", map[string]any{
		"backend": "kwin_virtual",
	})
	if result == nil || !result.IsError {
		t.Fatal("expected error result when dbus-run-session is unavailable")
	}
	if text := extractTextFromResult(t, result); !strings.Contains(text, "dbus-run-session not found") {
		t.Fatalf("expected dbus-run-session error, got %q", text)
	}
}

func TestDesktopSessionScreenshot_MissingGrim(t *testing.T) {
	stateDir := t.TempDir()
	emptyBin := t.TempDir()

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", emptyBin)

	record := saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:             "session-grim",
		Name:           "Screenshot Session",
		Backend:        "live_wayland",
		Status:         "connected",
		WaylandDisplay: "wayland-0",
		XDGRuntimeDir:  t.TempDir(),
	})

	result := callDesktopSessionTool(t, "session_screenshot", map[string]any{
		"session_id": record.ID,
	})
	if result == nil || !result.IsError {
		t.Fatal("expected error result when grim is unavailable")
	}
	if text := extractTextFromResult(t, result); !strings.Contains(text, "grim not found") {
		t.Fatalf("expected grim error, got %q", text)
	}
}

func TestDesktopSessionClipboardGet_MissingWLPaste(t *testing.T) {
	stateDir := t.TempDir()
	emptyBin := t.TempDir()

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", emptyBin)

	record := saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:             "session-wlpaste",
		Name:           "Clipboard Read Session",
		Backend:        "live_wayland",
		Status:         "connected",
		WaylandDisplay: "wayland-0",
		XDGRuntimeDir:  t.TempDir(),
	})

	result := callDesktopSessionTool(t, "session_clipboard_get", map[string]any{
		"session_id": record.ID,
	})
	if result == nil || !result.IsError {
		t.Fatal("expected error result when wl-paste is unavailable")
	}
	if text := extractTextFromResult(t, result); !strings.Contains(text, "wl-paste not found") {
		t.Fatalf("expected wl-paste error, got %q", text)
	}
}

func TestDesktopSessionClipboardSet_MissingWLCopy(t *testing.T) {
	stateDir := t.TempDir()
	emptyBin := t.TempDir()

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", emptyBin)

	record := saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:             "session-wlcopy",
		Name:           "Clipboard Write Session",
		Backend:        "live_wayland",
		Status:         "connected",
		WaylandDisplay: "wayland-0",
		XDGRuntimeDir:  t.TempDir(),
	})

	result := callDesktopSessionTool(t, "session_clipboard_set", map[string]any{
		"session_id": record.ID,
		"text":       "hello",
	})
	if result == nil || !result.IsError {
		t.Fatal("expected error result when wl-copy is unavailable")
	}
	if text := extractTextFromResult(t, result); !strings.Contains(text, "wl-copy not found") {
		t.Fatalf("expected wl-copy error, got %q", text)
	}
}

func TestDesktopSessionWaylandInfo_MissingBinary(t *testing.T) {
	stateDir := t.TempDir()
	emptyBin := t.TempDir()

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", emptyBin)

	record := saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:             "session-wayland-info",
		Name:           "Wayland Info Session",
		Backend:        "live_wayland",
		Status:         "connected",
		WaylandDisplay: "wayland-0",
		XDGRuntimeDir:  t.TempDir(),
	})

	result := callDesktopSessionTool(t, "session_wayland_info", map[string]any{
		"session_id": record.ID,
	})
	if result == nil || !result.IsError {
		t.Fatal("expected error result when wayland-info is unavailable")
	}
	if text := extractTextFromResult(t, result); !strings.Contains(text, "wayland-info not found") {
		t.Fatalf("expected wayland-info error, got %q", text)
	}
}

func TestDesktopSessionListWindows_UnsupportedWithoutDBus(t *testing.T) {
	stateDir := t.TempDir()

	t.Setenv("XDG_STATE_HOME", stateDir)

	record := saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:                    "session-no-dbus",
		Name:                  "No DBus Session",
		Backend:               "live_wayland",
		Status:                "connected",
		WaylandDisplay:        "wayland-0",
		XDGRuntimeDir:         t.TempDir(),
		DBUSSessionBusAddress: "",
	})

	result := callDesktopSessionTool(t, "session_list_windows", map[string]any{
		"session_id": record.ID,
	})
	if result == nil || result.IsError {
		t.Fatalf("expected unsupported-mode payload, got %q", extractTextFromResult(t, result))
	}

	out := unmarshalSessionWindowsResult(t, result)
	if out.Mode != "unsupported" {
		t.Fatalf("mode = %q, want %q", out.Mode, "unsupported")
	}
	if !strings.Contains(out.Unsupported, "DBUS_SESSION_BUS_ADDRESS") {
		t.Fatalf("expected unsupported message to mention DBUS_SESSION_BUS_ADDRESS, got %q", out.Unsupported)
	}
}

func TestDesktopSemanticFixtureFlows(t *testing.T) {
	stateDir := t.TempDir()
	binDir := t.TempDir()
	origPath := os.Getenv("PATH")

	writeSemanticFixturePython(t, binDir)

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)
	t.Setenv("WAYLAND_DISPLAY", "wayland-fixture")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/tmp/fixture-bus")

	snapshotResult := callDesktopSemanticTool(t, "desktop_snapshot", map[string]any{})
	if snapshotResult == nil || snapshotResult.IsError {
		t.Fatalf("expected successful desktop_snapshot result, got %q", extractTextFromResult(t, snapshotResult))
	}
	var snapshot desktopSemanticSnapshotOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, snapshotResult)), &snapshot); err != nil {
		t.Fatalf("unmarshal desktop snapshot output: %v", err)
	}
	if len(snapshot.Apps) != 1 || stringValue(snapshot.Apps[0]["name"]) != "Fixture App" {
		t.Fatalf("unexpected snapshot apps: %#v", snapshot.Apps)
	}

	findResult := callDesktopSemanticTool(t, "desktop_find", map[string]any{
		"app":  "Fixture App",
		"name": "Full Name",
		"role": "entry",
	})
	if findResult == nil || findResult.IsError {
		t.Fatalf("expected successful desktop_find result, got %q", extractTextFromResult(t, findResult))
	}
	var found desktopSemanticElementOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, findResult)), &found); err != nil {
		t.Fatalf("unmarshal desktop find output: %v", err)
	}
	if !found.Matched || stringValue(found.Element["role"]) != "entry" || found.Query.Path != "0/1" {
		t.Fatalf("unexpected desktop_find output: %#v", found)
	}

	focusResult := callDesktopSemanticTool(t, "desktop_focus_window", map[string]any{
		"title_contains": "Fixture",
	})
	if focusResult == nil || focusResult.IsError {
		t.Fatalf("expected successful desktop_focus_window result, got %q", extractTextFromResult(t, focusResult))
	}
	var focused desktopSemanticElementOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, focusResult)), &focused); err != nil {
		t.Fatalf("unmarshal desktop focus output: %v", err)
	}
	if !focused.Focused {
		t.Fatalf("expected focused result, got %#v", focused)
	}

	readValueResult := callDesktopSemanticTool(t, "desktop_read_value", map[string]any{
		"app":  "Fixture App",
		"name": "Full Name",
	})
	if readValueResult == nil || readValueResult.IsError {
		t.Fatalf("expected successful desktop_read_value result, got %q", extractTextFromResult(t, readValueResult))
	}
	var read desktopSemanticElementOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, readValueResult)), &read); err != nil {
		t.Fatalf("unmarshal desktop read value output: %v", err)
	}
	if read.ValueKind != "text" || read.Value != "fixture-text" || read.Query.Path != "0/1" {
		t.Fatalf("unexpected read value output: %#v", read)
	}

	setTextResult := callDesktopSemanticTool(t, "desktop_set_text", map[string]any{
		"app":  "Fixture App",
		"name": "Full Name",
		"text": "patched",
	})
	if setTextResult == nil || setTextResult.IsError {
		t.Fatalf("expected successful desktop_set_text result, got %q", extractTextFromResult(t, setTextResult))
	}
	var updated desktopSemanticElementOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, setTextResult)), &updated); err != nil {
		t.Fatalf("unmarshal desktop set text output: %v", err)
	}
	if !updated.Updated || updated.Value != "patched" || updated.Query.Path != "0/1" {
		t.Fatalf("unexpected set text output: %#v", updated)
	}

	waitResult := callDesktopSemanticTool(t, "desktop_wait_for_element", map[string]any{
		"app":     "Fixture App",
		"name":    "Type full name",
		"role":    "entry",
		"timeout": 1,
	})
	if waitResult == nil || waitResult.IsError {
		t.Fatalf("expected successful desktop_wait_for_element result, got %q", extractTextFromResult(t, waitResult))
	}
	var waited desktopSemanticElementOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, waitResult)), &waited); err != nil {
		t.Fatalf("unmarshal desktop wait output: %v", err)
	}
	if !waited.Matched || stringValue(waited.Element["role"]) != "entry" || waited.Query.Path != "0/1" {
		t.Fatalf("unexpected desktop wait output: %#v", waited)
	}
}

func TestDesktopSemanticFormTools(t *testing.T) {
	stateDir := t.TempDir()
	binDir := t.TempDir()
	origPath := os.Getenv("PATH")

	writeSemanticFixturePython(t, binDir)

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)
	t.Setenv("WAYLAND_DISPLAY", "wayland-fixture")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/tmp/fixture-bus")

	formFieldsResult := callDesktopSemanticTool(t, "desktop_form_fields", map[string]any{
		"app":             "Fixture App",
		"include_actions": true,
	})
	if formFieldsResult == nil || formFieldsResult.IsError {
		t.Fatalf("expected successful desktop_form_fields result, got %q", extractTextFromResult(t, formFieldsResult))
	}
	var fieldsOut desktopSemanticFormFieldsOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, formFieldsResult)), &fieldsOut); err != nil {
		t.Fatalf("unmarshal desktop form fields output: %v", err)
	}
	if fieldsOut.Count < 5 {
		t.Fatalf("expected at least 5 semantic form fields, got %#v", fieldsOut)
	}
	foundLabeledEntry := false
	for _, field := range fieldsOut.Fields {
		if field.FieldType == "text" && containsString(field.Labels, "Full Name") {
			foundLabeledEntry = true
		}
	}
	if !foundLabeledEntry {
		t.Fatalf("expected labelled text field in semantic form fields, got %#v", fieldsOut.Fields)
	}

	previewResult := callDesktopSemanticTool(t, "desktop_fill_form", map[string]any{
		"app":     "Fixture App",
		"preview": true,
		"fields": []map[string]any{
			{"name": "Full Name", "text": "patched"},
			{"name": "Volume", "number": 42},
			{"name": "Accept Terms", "checked": true},
			{"name": "Submit", "action": "press"},
		},
	})
	if previewResult == nil || previewResult.IsError {
		t.Fatalf("expected successful desktop_fill_form preview result, got %q", extractTextFromResult(t, previewResult))
	}
	var previewOut desktopSemanticFormFillOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, previewResult)), &previewOut); err != nil {
		t.Fatalf("unmarshal desktop fill form preview output: %v", err)
	}
	if !previewOut.Preview || previewOut.Planned != 4 || previewOut.Applied != 0 {
		t.Fatalf("unexpected desktop fill form preview output: %#v", previewOut)
	}

	fillResult := callDesktopSemanticTool(t, "desktop_fill_form", map[string]any{
		"app": "Fixture App",
		"fields": []map[string]any{
			{"name": "Full Name", "text": "patched"},
			{"name": "Volume", "number": 42},
			{"name": "Accept Terms", "checked": true},
			{"name": "Submit", "action": "press"},
		},
	})
	if fillResult == nil || fillResult.IsError {
		t.Fatalf("expected successful desktop_fill_form result, got %q", extractTextFromResult(t, fillResult))
	}
	var fillOut desktopSemanticFormFillOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, fillResult)), &fillOut); err != nil {
		t.Fatalf("unmarshal desktop fill form output: %v", err)
	}
	if fillOut.Matched != 4 || fillOut.Applied != 4 {
		t.Fatalf("unexpected desktop fill form output: %#v", fillOut)
	}
}

func TestDesktopSessionSemanticFixtureFlows(t *testing.T) {
	stateDir := t.TempDir()
	binDir := t.TempDir()
	origPath := os.Getenv("PATH")

	writeSemanticFixturePython(t, binDir)

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	record := saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:                    "session-semantic-fixture",
		Name:                  "Semantic Fixture Session",
		Backend:               "live_wayland",
		Status:                "connected",
		WaylandDisplay:        "wayland-0",
		XDGRuntimeDir:         t.TempDir(),
		DBUSSessionBusAddress: "unix:path=/tmp/session-bus",
	})

	listWindowsResult := callDesktopSessionTool(t, "session_list_windows", map[string]any{
		"session_id": record.ID,
	})
	if listWindowsResult == nil || listWindowsResult.IsError {
		t.Fatalf("expected successful session_list_windows result, got %q", extractTextFromResult(t, listWindowsResult))
	}
	listed := unmarshalSessionWindowsResult(t, listWindowsResult)
	if listed.Mode != "atspi" || listed.Count != 1 {
		t.Fatalf("unexpected session window output: %#v", listed)
	}

	listAppsResult := callDesktopSessionTool(t, "session_list_apps", map[string]any{
		"session_id": record.ID,
	})
	if listAppsResult == nil || listAppsResult.IsError {
		t.Fatalf("expected successful session_list_apps result, got %q", extractTextFromResult(t, listAppsResult))
	}
	appsOut := unmarshalSessionAppsResult(t, listAppsResult)
	if appsOut.Count != 1 || stringValue(appsOut.Apps[0]["name"]) != "Fixture App" {
		t.Fatalf("unexpected session apps output: %#v", appsOut)
	}

	findResult := callDesktopSessionTool(t, "session_find_ui_element", map[string]any{
		"session_id": record.ID,
		"app":        "Fixture App",
		"name":       "Full Name",
		"role":       "entry",
	})
	if findResult == nil || findResult.IsError {
		t.Fatalf("expected successful session_find_ui_element result, got %q", extractTextFromResult(t, findResult))
	}
	var found SessionSemanticElementOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, findResult)), &found); err != nil {
		t.Fatalf("unmarshal session find output: %v", err)
	}
	if !found.Matched || stringValue(found.Element["role"]) != "entry" || found.Query.Path != "0/1" {
		t.Fatalf("unexpected session semantic find output: %#v", found)
	}

	setTextResult := callDesktopSessionTool(t, "session_set_text", map[string]any{
		"session_id": record.ID,
		"app":        "Fixture App",
		"name":       "Full Name",
		"text":       "patched",
	})
	if setTextResult == nil || setTextResult.IsError {
		t.Fatalf("expected successful session_set_text result, got %q", extractTextFromResult(t, setTextResult))
	}
	var updated SessionSemanticElementOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, setTextResult)), &updated); err != nil {
		t.Fatalf("unmarshal session set text output: %v", err)
	}
	if !updated.Updated || updated.Value != "patched" || updated.Query.Path != "0/1" {
		t.Fatalf("unexpected session set text output: %#v", updated)
	}

	waitResult := callDesktopSessionTool(t, "session_wait_for_element", map[string]any{
		"session_id": record.ID,
		"app":        "Fixture App",
		"name":       "Type full name",
		"role":       "entry",
		"timeout":    1,
	})
	if waitResult == nil || waitResult.IsError {
		t.Fatalf("expected successful session_wait_for_element result, got %q", extractTextFromResult(t, waitResult))
	}
	var waited SessionSemanticElementOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, waitResult)), &waited); err != nil {
		t.Fatalf("unmarshal session wait output: %v", err)
	}
	if !waited.Matched || stringValue(waited.Element["role"]) != "entry" || waited.Query.Path != "0/1" {
		t.Fatalf("unexpected session wait output: %#v", waited)
	}

	clickResult := callDesktopSessionTool(t, "session_click_element", map[string]any{
		"session_id": record.ID,
		"app":        "Fixture App",
		"name":       "Save",
	})
	if clickResult == nil || clickResult.IsError {
		t.Fatalf("expected successful session_click_element result, got %q", extractTextFromResult(t, clickResult))
	}
	var clicked SessionSemanticElementOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, clickResult)), &clicked); err != nil {
		t.Fatalf("unmarshal session click output: %v", err)
	}
	if !clicked.Clicked {
		t.Fatalf("expected clicked result, got %#v", clicked)
	}
}

func TestDesktopSessionFormTools(t *testing.T) {
	stateDir := t.TempDir()
	binDir := t.TempDir()
	origPath := os.Getenv("PATH")

	writeSemanticFixturePython(t, binDir)

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	record := saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:                    "session-form-fixture",
		Name:                  "Form Fixture Session",
		Backend:               "live_wayland",
		Status:                "connected",
		WaylandDisplay:        "wayland-0",
		XDGRuntimeDir:         t.TempDir(),
		DBUSSessionBusAddress: "unix:path=/tmp/session-bus",
	})

	formFieldsResult := callDesktopSessionTool(t, "session_form_fields", map[string]any{
		"session_id":      record.ID,
		"app":             "Fixture App",
		"include_actions": true,
	})
	if formFieldsResult == nil || formFieldsResult.IsError {
		t.Fatalf("expected successful session_form_fields result, got %q", extractTextFromResult(t, formFieldsResult))
	}
	var fieldsOut SessionSemanticFormFieldsOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, formFieldsResult)), &fieldsOut); err != nil {
		t.Fatalf("unmarshal session form fields output: %v", err)
	}
	if fieldsOut.Session.ID != record.ID || fieldsOut.Count < 5 {
		t.Fatalf("unexpected session form fields output: %#v", fieldsOut)
	}

	fillResult := callDesktopSessionTool(t, "session_fill_form", map[string]any{
		"session_id": record.ID,
		"app":        "Fixture App",
		"fields": []map[string]any{
			{"name": "Full Name", "text": "patched"},
			{"name": "Volume", "number": 42},
			{"name": "Accept Terms", "checked": true},
			{"name": "Submit", "action": "press"},
		},
	})
	if fillResult == nil || fillResult.IsError {
		t.Fatalf("expected successful session_fill_form result, got %q", extractTextFromResult(t, fillResult))
	}
	var fillOut SessionSemanticFormFillOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, fillResult)), &fillOut); err != nil {
		t.Fatalf("unmarshal session fill form output: %v", err)
	}
	if fillOut.Session.ID != record.ID || fillOut.Matched != 4 || fillOut.Applied != 4 {
		t.Fatalf("unexpected session fill form output: %#v", fillOut)
	}
}

func TestDesktopSessionCommandFixtureFlows(t *testing.T) {
	stateDir := t.TempDir()
	binDir := t.TempDir()
	origPath := os.Getenv("PATH")

	writeSessionCommandFixtures(t, binDir)

	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	record := saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:                    "session-command-fixture",
		Name:                  "Command Fixture Session",
		Backend:               "live_wayland",
		Status:                "connected",
		WaylandDisplay:        "wayland-0",
		XDGRuntimeDir:         t.TempDir(),
		DBUSSessionBusAddress: "unix:path=/tmp/session-bus",
	})

	screenshotResult := callDesktopSessionTool(t, "session_screenshot", map[string]any{
		"session_id": record.ID,
	})
	if screenshotResult == nil || screenshotResult.IsError {
		t.Fatalf("expected successful session_screenshot result, got %q", extractTextFromResult(t, screenshotResult))
	}
	var screenshotOut SessionScreenshotOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, screenshotResult)), &screenshotOut); err != nil {
		t.Fatalf("unmarshal session screenshot output: %v", err)
	}
	if screenshotOut.Bytes <= 0 || !pathExists(screenshotOut.OutputPath) {
		t.Fatalf("unexpected screenshot output: %#v", screenshotOut)
	}

	clipboardGetResult := callDesktopSessionTool(t, "session_clipboard_get", map[string]any{
		"session_id": record.ID,
	})
	if clipboardGetResult == nil || clipboardGetResult.IsError {
		t.Fatalf("expected successful session_clipboard_get result, got %q", extractTextFromResult(t, clipboardGetResult))
	}
	var clipboardOut SessionClipboardOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, clipboardGetResult)), &clipboardOut); err != nil {
		t.Fatalf("unmarshal clipboard output: %v", err)
	}
	if clipboardOut.Text != "fixture clipboard" {
		t.Fatalf("clipboard text = %q, want %q", clipboardOut.Text, "fixture clipboard")
	}

	clipboardSetResult := callDesktopSessionTool(t, "session_clipboard_set", map[string]any{
		"session_id": record.ID,
		"text":       "hello",
	})
	if clipboardSetResult == nil || clipboardSetResult.IsError {
		t.Fatalf("expected successful session_clipboard_set result, got %q", extractTextFromResult(t, clipboardSetResult))
	}
	var clipboardSetOut SessionCommandOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, clipboardSetResult)), &clipboardSetOut); err != nil {
		t.Fatalf("unmarshal clipboard set output: %v", err)
	}
	if !strings.Contains(clipboardSetOut.Output, "copied 5 bytes as text/plain") {
		t.Fatalf("unexpected clipboard set output: %#v", clipboardSetOut)
	}

	waylandInfoResult := callDesktopSessionTool(t, "session_wayland_info", map[string]any{
		"session_id": record.ID,
	})
	if waylandInfoResult == nil || waylandInfoResult.IsError {
		t.Fatalf("expected successful session_wayland_info result, got %q", extractTextFromResult(t, waylandInfoResult))
	}
	var waylandInfoOut SessionCommandOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, waylandInfoResult)), &waylandInfoOut); err != nil {
		t.Fatalf("unmarshal wayland info output: %v", err)
	}
	if !strings.Contains(waylandInfoOut.Output, "wl_compositor") {
		t.Fatalf("unexpected wayland info output: %#v", waylandInfoOut)
	}

	typeTextResult := callDesktopSessionTool(t, "session_type_text", map[string]any{
		"session_id": record.ID,
		"text":       "fixture",
	})
	if typeTextResult == nil || typeTextResult.IsError {
		t.Fatalf("expected successful session_type_text result, got %q", extractTextFromResult(t, typeTextResult))
	}
	var typeTextOut SessionCommandOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, typeTextResult)), &typeTextOut); err != nil {
		t.Fatalf("unmarshal session type output: %v", err)
	}
	if typeTextOut.Mode != "wtype" || !strings.Contains(typeTextOut.Output, "typed 7 chars") {
		t.Fatalf("unexpected type text output: %#v", typeTextOut)
	}

	dbusCallResult := callDesktopSessionTool(t, "session_dbus_call", map[string]any{
		"session_id": record.ID,
		"service":    "org.kde.KWin",
		"path":       "/KWin",
		"interface":  "org.kde.KWin",
		"method":     "reconfigure",
	})
	if dbusCallResult == nil || dbusCallResult.IsError {
		t.Fatalf("expected successful session_dbus_call result, got %q", extractTextFromResult(t, dbusCallResult))
	}
	var dbusOut SessionCommandOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, dbusCallResult)), &dbusOut); err != nil {
		t.Fatalf("unmarshal dbus output: %v", err)
	}
	if dbusOut.Mode != "dbus-send" || !strings.Contains(dbusOut.Output, "method return") {
		t.Fatalf("unexpected dbus output: %#v", dbusOut)
	}
}

func TestDesktopSessionListStatusAndReadLog(t *testing.T) {
	stateDir := t.TempDir()
	logDir := t.TempDir()
	runtimeDir := t.TempDir()
	socketPath := filepath.Join(runtimeDir, "wayland-0")
	envPath := filepath.Join(logDir, "session.env")
	logPath := filepath.Join(logDir, "kwin.log")
	appLogPath := filepath.Join(logDir, "app.log")

	if err := os.WriteFile(socketPath, []byte("socket"), 0o600); err != nil {
		t.Fatalf("write socket fixture: %v", err)
	}
	if err := os.WriteFile(envPath, []byte("DBUS_SESSION_BUS_ADDRESS=unix:path=/tmp/test-bus\nAT_SPI_BUS_ADDRESS=unix:path=/tmp/test-atspi\n"), 0o644); err != nil {
		t.Fatalf("write env fixture: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("line 1\nline 2\nline 3\nline 4\n"), 0o644); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	if err := os.WriteFile(appLogPath, []byte("app output\n"), 0o644); err != nil {
		t.Fatalf("write app log fixture: %v", err)
	}

	t.Setenv("XDG_STATE_HOME", stateDir)

	newest := saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:                        "session-inspect-newest",
		Name:                      "Newest Session",
		Backend:                   "live_hyprland",
		Status:                    "connected",
		WaylandDisplay:            "wayland-0",
		XDGRuntimeDir:             runtimeDir,
		HyprlandInstanceSignature: "hypr-fixture",
		DBUSSessionBusAddress:     "unix:path=/tmp/test-bus",
		ATSPIBusAddress:           "unix:path=/tmp/test-atspi",
		EnvPath:                   envPath,
		LogPath:                   logPath,
		StartedAt:                 "2026-04-10T12:00:00Z",
		AppLogs: []desktopSessionAppLog{
			{App: "fixture-app", Path: appLogPath, StartedAt: "2026-04-10T12:05:00Z"},
		},
	})
	saveTestDesktopSessionRecord(t, desktopSessionRecord{
		ID:        "session-inspect-older",
		Name:      "Older Session",
		Backend:   "kwin_virtual",
		Status:    "stopped",
		StartedAt: "2026-04-10T10:00:00Z",
		StoppedAt: "2026-04-10T10:30:00Z",
	})

	listResult := callDesktopSessionTool(t, "session_list", map[string]any{
		"limit": 1,
	})
	if listResult == nil || listResult.IsError {
		t.Fatalf("expected successful session_list result, got %q", extractTextFromResult(t, listResult))
	}
	listed := unmarshalSessionListResult(t, listResult)
	if listed.Count != 1 || len(listed.Sessions) != 1 {
		t.Fatalf("unexpected session list output: %#v", listed)
	}
	if listed.Sessions[0].SessionID != newest.ID || listed.Sessions[0].ResolvedStatus != "connected" {
		t.Fatalf("unexpected first session list entry: %#v", listed.Sessions[0])
	}

	statusResult := callDesktopSessionTool(t, "session_status", map[string]any{
		"session_id": newest.ID,
	})
	if statusResult == nil || statusResult.IsError {
		t.Fatalf("expected successful session_status result, got %q", extractTextFromResult(t, statusResult))
	}
	status := unmarshalSessionStatusResult(t, statusResult)
	if status.Session.ID != newest.ID || status.ResolvedStatus != "connected" {
		t.Fatalf("unexpected session status output: %#v", status)
	}
	if !status.HyprlandBacked || !status.SocketPresent || !status.DBUSReady || !status.ATSPIReady {
		t.Fatalf("expected ready session status, got %#v", status)
	}
	if status.AppLogCount != 1 || len(status.AppLogs) != 1 {
		t.Fatalf("expected one app log in status output, got %#v", status)
	}
	if !containsString(status.Recommendations, "Use session_read_app_log to inspect the newest launched application output.") {
		t.Fatalf("expected app log recommendation, got %#v", status.Recommendations)
	}

	waitReadyResult := callDesktopSessionTool(t, "session_wait_ready", map[string]any{
		"session_id": newest.ID,
		"timeout":    1,
	})
	if waitReadyResult == nil || waitReadyResult.IsError {
		t.Fatalf("expected successful session_wait_ready result, got %q", extractTextFromResult(t, waitReadyResult))
	}
	waitOut := unmarshalSessionWaitReadyResult(t, waitReadyResult)
	if !waitOut.Ready || waitOut.Status.ResolvedStatus != "connected" || waitOut.Timeout != 1 {
		t.Fatalf("unexpected session wait-ready output: %#v", waitOut)
	}

	logResult := callDesktopSessionTool(t, "session_read_log", map[string]any{
		"session_id": newest.ID,
		"lines":      2,
	})
	if logResult == nil || logResult.IsError {
		t.Fatalf("expected successful session_read_log result, got %q", extractTextFromResult(t, logResult))
	}
	logOut := unmarshalSessionLogResult(t, logResult)
	if logOut.Path != logPath || strings.TrimSpace(logOut.Output) != "line 3\nline 4" {
		t.Fatalf("unexpected session log output: %#v", logOut)
	}
}
