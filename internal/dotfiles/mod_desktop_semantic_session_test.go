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
