package dotfiles

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

func callNamedModuleTool(t *testing.T, provider toolProvider, name string, args map[string]any) *registry.CallToolResult {
	t.Helper()
	td := findModuleTool(t, provider, name)
	req := registry.CallToolRequest{}
	req.Params.Arguments = args
	result, err := td.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s handler error: %v", name, err)
	}
	return result
}

func writeInputBluetoothFixtures(t *testing.T, binDir string) {
	t.Helper()

	writeTestExecutable(t, binDir, "systemctl", `#!/bin/sh
if [ "$1" = "--user" ]; then
  shift
fi
if [ "$1" = "is-active" ]; then
  case "$2" in
    juhradialmx-daemon.service) exit 0 ;;
    ydotool.service) exit 3 ;;
    makima.service) exit 0 ;;
    *) exit 3 ;;
  esac
fi
exit 3
`)

	writeTestExecutable(t, binDir, "gdbus", `#!/bin/sh
case "${DOTFILES_TEST_GDBUS_MODE:-success}" in
  success)
    printf '%s\n' '(uint32 88, false)'
    ;;
  invalid)
    printf '%s\n' 'not-a-battery-tuple'
    ;;
  fail)
    exit 1
    ;;
esac
`)

	writeTestExecutable(t, binDir, "bluetoothctl", `#!/bin/sh
case "$1" in
  devices)
    case "$2" in
      Connected)
        printf '%s\n' 'Device D2:8E:C5:DE:9F:CB MX Master 4'
        ;;
      Trusted)
        printf '%s\n' 'Device D2:8E:C5:DE:9F:CB MX Master 4'
        printf '%s\n' 'Device 00:11:22:33:44:55 Kitchen Speaker'
        ;;
      Paired)
        printf '%s\n' 'Device D2:8E:C5:DE:9F:CB MX Master 4'
        printf '%s\n' 'Device 00:11:22:33:44:55 Kitchen Speaker'
        ;;
      *)
        printf '%s\n' 'Device D2:8E:C5:DE:9F:CB MX Master 4'
        printf '%s\n' 'Device 00:11:22:33:44:55 Kitchen Speaker'
        printf '%s\n' 'Device AA:BB:CC:DD:EE:FF Logitech K380'
        ;;
    esac
    ;;
  info)
    case "$2" in
      D2:8E:C5:DE:9F:CB)
        printf '%s\n' 'Name: MX Master 4' 'Connected: yes' 'Paired: yes' 'Trusted: yes' 'Blocked: no' 'Battery Percentage: 0x43 (67)' 'UUID: Human Interface Device' 'UUID: Battery Service'
        ;;
      00:11:22:33:44:55)
        printf '%s\n' 'Name: Kitchen Speaker' 'Connected: yes' 'Paired: yes' 'Trusted: no' 'Blocked: no' 'Battery Percentage: 0x5c (92)' 'UUID: Audio Sink'
        ;;
      AA:BB:CC:DD:EE:FF)
        printf '%s\n' 'Name: Logitech K380' 'Connected: no' 'Paired: yes' 'Trusted: yes' 'Blocked: no' 'Battery Percentage: 0x1e (30)' 'UUID: Human Interface Device'
        ;;
      *)
        exit 1
        ;;
    esac
    ;;
  *)
    exit 1
    ;;
esac
`)
}

func TestInputStatusWithFixtureServicesAndBattery(t *testing.T) {
	binDir := t.TempDir()
	runtimeDir := t.TempDir()
	writeInputBluetoothFixtures(t, binDir)

	t.Setenv("PATH", binDir)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("DOTFILES_TEST_GDBUS_MODE", "success")

	result := callNamedModuleTool(t, &InputModule{}, "input_status", map[string]any{})

	var out InputStatusOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal input status: %v", err)
	}

	if len(out.Services) != 3 {
		t.Fatalf("service count = %d, want 3", len(out.Services))
	}

	serviceStates := make(map[string]bool, len(out.Services))
	for _, service := range out.Services {
		serviceStates[service.Name] = service.Active
	}

	if !serviceStates["juhradialmx-daemon"] {
		t.Fatal("expected juhradialmx-daemon to be active")
	}
	if serviceStates["ydotool"] {
		t.Fatal("expected ydotool to be inactive")
	}
	if !serviceStates["makima"] {
		t.Fatal("expected makima to be active")
	}
	if out.Battery == nil {
		t.Fatal("expected battery details")
	}
	if out.Battery.Percent != 88 || out.Battery.Source != "dbus" || out.Battery.Charging {
		t.Fatalf("unexpected battery output: %+v", *out.Battery)
	}
}

func TestInputGetJuhradialBatteryFallsBackToBluetooth(t *testing.T) {
	binDir := t.TempDir()
	runtimeDir := t.TempDir()
	writeInputBluetoothFixtures(t, binDir)

	t.Setenv("PATH", binDir)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("DOTFILES_TEST_GDBUS_MODE", "fail")

	result := callNamedModuleTool(t, &JuhradialModule{}, "input_get_juhradial_battery", map[string]any{})
	if result.IsError {
		t.Fatalf("expected successful fallback battery result, got %q", extractTextFromResult(t, result))
	}

	var out InputGetJuhradialBatteryOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal juhradial battery: %v", err)
	}

	if out.Source != "bluetoothctl" {
		t.Fatalf("source = %q, want bluetoothctl", out.Source)
	}
	if out.Device != "MX Master 4" {
		t.Fatalf("device = %q, want MX Master 4", out.Device)
	}
	if out.Percent != 67 {
		t.Fatalf("percent = %d, want 67", out.Percent)
	}
}

func TestBTListDevicesFixtureParsing(t *testing.T) {
	binDir := t.TempDir()
	writeInputBluetoothFixtures(t, binDir)

	t.Setenv("PATH", binDir)

	result := callNamedModuleTool(t, &BluetoothModule{}, "bt_list_devices", map[string]any{
		"filter": "connected",
	})
	if result.IsError {
		t.Fatalf("expected successful bt_list_devices result, got %q", extractTextFromResult(t, result))
	}

	var out BTListOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal bt list: %v", err)
	}

	if len(out.Devices) != 1 {
		t.Fatalf("device count = %d, want 1", len(out.Devices))
	}
	device := out.Devices[0]
	if device.MAC != "D2:8E:C5:DE:9F:CB" {
		t.Fatalf("mac = %q, want D2:8E:C5:DE:9F:CB", device.MAC)
	}
	if !device.Connected || !device.Paired || !device.Trusted {
		t.Fatalf("expected connected/paired/trusted device, got %+v", device)
	}
	if device.Battery != 67 {
		t.Fatalf("battery = %d, want 67", device.Battery)
	}
}

func TestBTDeviceInfoFixtureResolution(t *testing.T) {
	binDir := t.TempDir()
	writeInputBluetoothFixtures(t, binDir)

	t.Setenv("PATH", binDir)

	result := callNamedModuleTool(t, &BluetoothModule{}, "bt_device_info", map[string]any{
		"device": "master",
	})
	if result.IsError {
		t.Fatalf("expected successful bt_device_info result, got %q", extractTextFromResult(t, result))
	}

	var out BTDeviceInfoOutput
	if err := json.Unmarshal([]byte(extractTextFromResult(t, result)), &out); err != nil {
		t.Fatalf("unmarshal bt device info: %v", err)
	}

	if out.MAC != "D2:8E:C5:DE:9F:CB" {
		t.Fatalf("mac = %q, want D2:8E:C5:DE:9F:CB", out.MAC)
	}
	if out.Name != "MX Master 4" {
		t.Fatalf("name = %q, want MX Master 4", out.Name)
	}
	if !out.Connected || !out.Paired || !out.Trusted || out.Blocked {
		t.Fatalf("unexpected connection flags: %+v", out)
	}
	if out.Battery != 67 {
		t.Fatalf("battery = %d, want 67", out.Battery)
	}
	if len(out.UUIDs) != 2 {
		t.Fatalf("uuid count = %d, want 2", len(out.UUIDs))
	}
}
