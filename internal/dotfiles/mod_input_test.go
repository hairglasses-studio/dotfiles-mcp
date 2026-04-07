package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// BluetoothModule Registration
// ---------------------------------------------------------------------------

func TestBluetoothModuleRegistration(t *testing.T) {
	m := &BluetoothModule{}
	tools := m.Tools()
	if len(tools) != 9 {
		t.Fatalf("expected 9 BT tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"bt_list_devices", "bt_device_info", "bt_connect", "bt_disconnect",
		"bt_pair", "bt_remove", "bt_trust", "bt_scan", "bt_power",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// ControllerModule Registration
// ---------------------------------------------------------------------------

func TestControllerModuleRegistration(t *testing.T) {
	m := &ControllerModule{}
	tools := m.Tools()
	if len(tools) != 3 {
		t.Fatalf("expected 3 controller tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"input_detect_controllers",
		"input_generate_controller_profile",
		"input_controller_test",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// WorkflowModule Registration
// ---------------------------------------------------------------------------

func TestWorkflowModuleRegistration(t *testing.T) {
	m := &WorkflowModule{}
	tools := m.Tools()
	if len(tools) < 1 {
		t.Fatal("expected at least 1 workflow tool")
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	if !srv.HasTool("bt_discover_and_connect") {
		t.Error("missing tool: bt_discover_and_connect")
	}
}

// ---------------------------------------------------------------------------
// BT helper unit tests
// ---------------------------------------------------------------------------

func TestMacRegex(t *testing.T) {
	valid := []string{
		"D2:8E:C5:DE:9F:CB",
		"00:11:22:33:44:55",
		"aa:bb:cc:dd:ee:ff",
		"AA:BB:CC:DD:EE:FF",
	}
	invalid := []string{
		"",
		"not-a-mac",
		"D2:8E:C5:DE:9F",       // too short
		"D2:8E:C5:DE:9F:CB:00", // too long
		"G2:8E:C5:DE:9F:CB",    // invalid hex char
	}

	for _, mac := range valid {
		if !macRe.MatchString(mac) {
			t.Errorf("macRe should match %q", mac)
		}
	}
	for _, mac := range invalid {
		if macRe.MatchString(mac) {
			t.Errorf("macRe should NOT match %q", mac)
		}
	}
}

func TestDeviceRegex(t *testing.T) {
	tests := []struct {
		line     string
		wantMAC  string
		wantName string
	}{
		{"Device D2:8E:C5:DE:9F:CB MX Master 4", "D2:8E:C5:DE:9F:CB", "MX Master 4"},
		{"Device 00:11:22:33:44:55 Simple Name", "00:11:22:33:44:55", "Simple Name"},
		{"Device AA:BB:CC:DD:EE:FF Name With Spaces And Numbers 123", "AA:BB:CC:DD:EE:FF", "Name With Spaces And Numbers 123"},
	}

	for _, tc := range tests {
		m := deviceRe.FindStringSubmatch(tc.line)
		if m == nil {
			t.Errorf("deviceRe did not match %q", tc.line)
			continue
		}
		if m[1] != tc.wantMAC {
			t.Errorf("MAC: got %q, want %q", m[1], tc.wantMAC)
		}
		if m[2] != tc.wantName {
			t.Errorf("Name: got %q, want %q", m[2], tc.wantName)
		}
	}

	// Should not match
	noMatch := []string{
		"",
		"not a device line",
		"Device INVALID MX Master",
	}
	for _, line := range noMatch {
		if deviceRe.MatchString(line) {
			t.Errorf("deviceRe should NOT match %q", line)
		}
	}
}

func TestBatteryRegex(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"Battery Percentage: 0x2d (45)", "45"},
		{"(100)", "100"},
		{"(0)", "0"},
	}

	for _, tc := range tests {
		m := batteryRe.FindStringSubmatch(tc.line)
		if m == nil {
			t.Errorf("batteryRe did not match %q", tc.line)
			continue
		}
		if m[1] != tc.want {
			t.Errorf("got %q, want %q", m[1], tc.want)
		}
	}
}

func TestResolveAnyDevice_MAC(t *testing.T) {
	// MAC addresses should be returned directly without lookup
	mac := "D2:8E:C5:DE:9F:CB"
	result, err := resolveAnyDevice(mac)
	if err != nil {
		t.Fatalf("resolveAnyDevice(%q) error: %v", mac, err)
	}
	if result != mac {
		t.Errorf("got %q, want %q", result, mac)
	}
}

func TestResolveAnyDevice_EmptyName(t *testing.T) {
	_, err := resolveAnyDevice("nonexistent-device-xyz-99999")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

// ---------------------------------------------------------------------------
// BT tool input validation
// ---------------------------------------------------------------------------

func TestBTPair_EmptyDevice(t *testing.T) {
	m := &BluetoothModule{}
	td := findModuleTool(t, m, "bt_pair")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"device": ""}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty device")
	}
}

func TestBTPair_InvalidMAC(t *testing.T) {
	m := &BluetoothModule{}
	td := findModuleTool(t, m, "bt_pair")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"device": "not-a-mac"}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for invalid MAC")
	}
}

func TestBTScan_InvalidAction(t *testing.T) {
	m := &BluetoothModule{}
	td := findModuleTool(t, m, "bt_scan")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"action": "invalid"}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for invalid scan action")
	}
}

func TestBTConnect_EmptyDevice(t *testing.T) {
	m := &BluetoothModule{}
	td := findModuleTool(t, m, "bt_connect")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"device": ""}
	result, err := td.Handler(context.Background(), req)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for empty device")
	}
}

// ---------------------------------------------------------------------------
// BT integration tests (skip when bluetoothctl unavailable)
// ---------------------------------------------------------------------------

func TestBTListDevices(t *testing.T) {
	if _, err := exec.LookPath("bluetoothctl"); err != nil {
		t.Skip("bluetoothctl not available")
	}

	m := &BluetoothModule{}
	td := findModuleTool(t, m, "bt_list_devices")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out BTListOutput
	text := extractTextFromResult(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Devices == nil {
		t.Error("devices should be non-nil")
	}
}

func TestBTScan_List(t *testing.T) {
	if _, err := exec.LookPath("bluetoothctl"); err != nil {
		t.Skip("bluetoothctl not available")
	}

	m := &BluetoothModule{}
	td := findModuleTool(t, m, "bt_scan")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"action": "list"}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out BTScanOutput
	text := extractTextFromResult(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Vendor brand detection
// ---------------------------------------------------------------------------

func TestVendorBrands(t *testing.T) {
	// Spot check a few known brands
	if vendorBrands["045e"] != "xbox" {
		t.Errorf("Microsoft (045e) should map to xbox, got %q", vendorBrands["045e"])
	}
	if vendorBrands["054c"] != "playstation" {
		t.Errorf("Sony (054c) should map to playstation, got %q", vendorBrands["054c"])
	}
	if vendorBrands["057e"] != "nintendo" {
		t.Errorf("Nintendo (057e) should map to nintendo, got %q", vendorBrands["057e"])
	}
}

// ---------------------------------------------------------------------------
// Controller detection (no hardware required)
// ---------------------------------------------------------------------------

func TestDetectControllers(t *testing.T) {
	m := &ControllerModule{}
	td := findModuleTool(t, m, "input_detect_controllers")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out DetectControllersOutput
	text := extractTextFromResult(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Controllers list may be empty but should not be nil
	if out.Controllers == nil {
		t.Error("controllers should be non-nil")
	}
}

// ---------------------------------------------------------------------------
// Input status
// ---------------------------------------------------------------------------

func TestInputStatus(t *testing.T) {
	m := &InputModule{}
	td := findModuleTool(t, m, "input_status")
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var out InputStatusOutput
	text := extractTextFromResult(t, result)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Should have service statuses
	if out.Services == nil {
		t.Error("services should be non-nil")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type toolProvider interface {
	Tools() []registry.ToolDefinition
}

func findModuleTool(t *testing.T, m toolProvider, name string) registry.ToolDefinition {
	t.Helper()
	for _, td := range m.Tools() {
		if td.Tool.Name == name {
			return td
		}
	}
	t.Fatalf("tool %q not found in module", name)
	return registry.ToolDefinition{}
}

func extractTextFromResult(t *testing.T, result *registry.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(registry.TextContent)
	if !ok {
		// Try string conversion for error content
		if result.IsError {
			return strings.Join(func() []string {
				var ss []string
				for _, c := range result.Content {
					if tc, ok := c.(registry.TextContent); ok {
						ss = append(ss, tc.Text)
					}
				}
				return ss
			}(), " ")
		}
		t.Fatalf("content is not TextContent, got %T", result.Content[0])
	}
	return tc.Text
}
