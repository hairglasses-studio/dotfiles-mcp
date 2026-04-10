// mod_input.go — Input device, BT, controller, MIDI, juhradial tools (migrated from input-mcp)
package dotfiles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func inputDotfilesDir() string { return dotfilesDir() }

func juhradialDir() string          { return filepath.Join(inputDotfilesDir(), "juhradial") }
func juhradialConfigPath() string   { return filepath.Join(juhradialDir(), "config.json") }
func juhradialProfilesPath() string { return filepath.Join(juhradialDir(), "profiles.json") }
func makimaDir() string             { return filepath.Join(inputDotfilesDir(), "makima") }
func midiDir() string               { return filepath.Join(inputDotfilesDir(), "midi") }

func inputRunCmd(name string, args ...string) (string, string, error) {
	return inputRunCmdEnv(nil, name, args...)
}

func inputRunCmdEnv(extraEnv []string, name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func inputRuntimeDir() string {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return runtimeDir
	}
	return filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
}

func juhradialSessionEnv() []string {
	runtimeDir := inputRuntimeDir()
	return []string{
		"XDG_RUNTIME_DIR=" + runtimeDir,
		"DBUS_SESSION_BUS_ADDRESS=unix:path=" + filepath.Join(runtimeDir, "bus"),
	}
}

func juhradialRunCmd(name string, args ...string) (string, string, error) {
	return inputRunCmdEnv(juhradialSessionEnv(), name, args...)
}

func juhradialSystemctl(args ...string) (string, string, error) {
	return juhradialRunCmd("systemctl", append([]string{"--user"}, args...)...)
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func validateJSONDocument(content string) error {
	var parsed any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return err
	}
	return nil
}

func parseJuhradialBatteryOutput(raw string) (InputGetJuhradialBatteryOutput, error) {
	out := strings.TrimSpace(raw)
	out = strings.TrimPrefix(out, "(")
	out = strings.TrimSuffix(out, ")")
	parts := strings.Split(out, ",")
	if len(parts) < 2 {
		return InputGetJuhradialBatteryOutput{}, fmt.Errorf("unexpected battery output: %q", raw)
	}

	left := strings.Fields(strings.TrimSpace(parts[0]))
	if len(left) == 0 {
		return InputGetJuhradialBatteryOutput{}, fmt.Errorf("unexpected battery value: %q", raw)
	}
	percent, err := strconv.Atoi(strings.Trim(left[len(left)-1], "'\""))
	if err != nil {
		return InputGetJuhradialBatteryOutput{}, fmt.Errorf("parse battery percent: %w", err)
	}

	chargingToken := strings.ToLower(strings.Trim(strings.TrimSpace(parts[1]), "'\""))
	return InputGetJuhradialBatteryOutput{
		Percent:  percent,
		Charging: strings.HasPrefix(chargingToken, "true"),
		Source:   "dbus",
	}, nil
}

func fallbackJuhradialBattery() (InputGetJuhradialBatteryOutput, error) {
	devices, err := btListPaired()
	if err != nil {
		return InputGetJuhradialBatteryOutput{}, err
	}

	bestIndex := -1
	bestScore := -1
	for i := range devices {
		btEnrichDevice(&devices[i])
		if devices[i].Battery < 0 {
			continue
		}

		score := 0
		lowerName := strings.ToLower(devices[i].Name)
		if strings.Contains(lowerName, "mx master") {
			score += 4
		}
		if strings.Contains(lowerName, "logitech") {
			score += 2
		}
		if devices[i].Connected {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}

	if bestIndex == -1 {
		return InputGetJuhradialBatteryOutput{}, fmt.Errorf("no Logitech battery information available")
	}

	return InputGetJuhradialBatteryOutput{
		Device:   devices[bestIndex].Name,
		Percent:  devices[bestIndex].Battery,
		Charging: false,
		Source:   "bluetoothctl",
	}, nil
}

func juhradialBatteryStatus() (InputGetJuhradialBatteryOutput, error) {
	out, _, err := juhradialRunCmd(
		"gdbus", "call",
		"--session",
		"--dest", "org.kde.juhradialmx",
		"--object-path", "/org/kde/juhradialmx/Daemon",
		"--method", "org.kde.juhradialmx.Daemon.GetBatteryStatus",
	)
	if err == nil && strings.TrimSpace(out) != "" {
		status, parseErr := parseJuhradialBatteryOutput(out)
		if parseErr == nil && status.Percent > 0 && status.Percent <= 100 {
			return status, nil
		}
	}

	return fallbackJuhradialBattery()
}

// btInteractivePair runs bluetoothctl interactively with an agent registered,
// which is required for BLE devices (e.g. Logitech MX Master) that need an
// active pairing agent for the authentication handshake.
func btInteractivePair(mac string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bluetoothctl")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start bluetoothctl: %w", err)
	}

	commands := []string{
		"agent on",
		"default-agent",
		"pair " + mac,
	}
	for _, c := range commands {
		fmt.Fprintln(stdin, c)
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for pairing to complete or timeout
	time.Sleep(5 * time.Second)
	fmt.Fprintln(stdin, "quit")
	stdin.Close()
	cmd.Wait()

	output := out.String()
	if strings.Contains(output, "Pairing successful") || strings.Contains(output, "already exists") {
		return output, nil
	}
	if strings.Contains(output, "AuthenticationFailed") || strings.Contains(output, "Failed to pair") {
		return output, fmt.Errorf("authentication failed (device may need re-pairing: remove + re-scan + pair)")
	}
	return output, fmt.Errorf("pairing did not confirm success: %s", output)
}

// btInteractiveConnect runs connect with retry for flaky BLE connections.
func btInteractiveConnect(mac string, retries int) (string, error) {
	if retries <= 0 {
		retries = 3
	}
	var lastOut, lastStderr string
	var lastErr error
	for i := 0; i < retries; i++ {
		lastOut, lastStderr, lastErr = inputRunCmd("bluetoothctl", "connect", mac)
		if lastErr == nil {
			return lastOut, nil
		}
		if strings.Contains(lastOut, "le-connection-abort") || strings.Contains(lastStderr, "le-connection-abort") {
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		break // non-retryable error
	}
	return lastOut, fmt.Errorf("connect failed after %d attempts: %s %s", retries, lastOut, lastStderr)
}

// resolveAnyDevice resolves a name against all known devices (paired + recently scanned).
func resolveAnyDevice(query string) (string, error) {
	if macRe.MatchString(query) {
		return query, nil
	}
	out, _, err := inputRunCmd("bluetoothctl", "devices")
	if err != nil {
		return "", err
	}
	q := strings.ToLower(query)
	for _, line := range strings.Split(out, "\n") {
		m := deviceRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		if strings.Contains(strings.ToLower(m[2]), q) {
			return m[1], nil
		}
	}
	return "", fmt.Errorf("no device matching %q", query)
}

// ---------------------------------------------------------------------------
// Bluetooth helpers
// ---------------------------------------------------------------------------

var (
	deviceRe  = regexp.MustCompile(`^Device ([0-9A-Fa-f:]{17}) (.+)$`)
	batteryRe = regexp.MustCompile(`\((\d+)\)`)
	macRe     = regexp.MustCompile(`^[0-9A-Fa-f:]{17}$`)
)

type btDevice struct {
	MAC       string `json:"mac"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
	Paired    bool   `json:"paired"`
	Trusted   bool   `json:"trusted"`
	Battery   int    `json:"battery,omitempty"` // -1 if unknown
}

func btListPaired() ([]btDevice, error) {
	out, _, err := inputRunCmd("bluetoothctl", "devices")
	if err != nil {
		return nil, fmt.Errorf("bluetoothctl devices: %w", err)
	}
	devices := make([]btDevice, 0)
	for _, line := range strings.Split(out, "\n") {
		m := deviceRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		devices = append(devices, btDevice{MAC: m[1], Name: m[2], Battery: -1})
	}
	return devices, nil
}

func btEnrichDevice(d *btDevice) {
	out, _, err := inputRunCmd("bluetoothctl", "info", d.MAC)
	if err != nil {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Connected: ") {
			d.Connected = strings.Contains(line, "yes")
		} else if strings.HasPrefix(line, "Paired: ") {
			d.Paired = strings.Contains(line, "yes")
		} else if strings.HasPrefix(line, "Trusted: ") {
			d.Trusted = strings.Contains(line, "yes")
		} else if strings.HasPrefix(line, "Battery Percentage:") {
			if m := batteryRe.FindStringSubmatch(line); m != nil {
				d.Battery, _ = strconv.Atoi(m[1])
			}
		} else if strings.HasPrefix(line, "Name: ") {
			d.Name = strings.TrimPrefix(line, "Name: ")
		}
	}
}

// ---------------------------------------------------------------------------
// Controller Detection
// ---------------------------------------------------------------------------

var vendorBrands = map[string]string{
	"045e": "xbox",
	"054c": "playstation",
	"057e": "nintendo",
	"28de": "valve",
	"2dc8": "8bitdo",
}

type controllerInfo struct {
	Name       string   `json:"name"`
	VendorID   string   `json:"vendor_id"`
	ProductID  string   `json:"product_id"`
	EventPath  string   `json:"event_path"`
	Brand      string   `json:"brand"`
	HasProfile bool     `json:"has_profile"`
	Features   []string `json:"features"`
}

func parseInputDevices() []controllerInfo {
	data, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		return nil
	}

	var controllers []controllerInfo
	blocks := strings.Split(string(data), "\n\n")

	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		var name, vendor, product, eventPath string
		var hasKey, hasAbs, hasFF bool
		isVirtual := false

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "N: Name=") {
				name = strings.Trim(strings.TrimPrefix(line, "N: Name="), "\"")
				lower := strings.ToLower(name)
				if strings.Contains(lower, "virtual") || strings.Contains(lower, "ydotool") ||
					strings.Contains(lower, "antimicrox") || strings.Contains(lower, "makima") ||
					strings.Contains(lower, "juhradial") {
					isVirtual = true
				}
			} else if strings.HasPrefix(line, "I: ") {
				for _, part := range strings.Fields(line) {
					if strings.HasPrefix(part, "Vendor=") {
						vendor = strings.ToLower(strings.TrimPrefix(part, "Vendor="))
					} else if strings.HasPrefix(part, "Product=") {
						product = strings.ToLower(strings.TrimPrefix(part, "Product="))
					}
				}
			} else if strings.HasPrefix(line, "H: Handlers=") {
				for _, h := range strings.Fields(strings.TrimPrefix(line, "H: Handlers=")) {
					if strings.HasPrefix(h, "event") {
						eventPath = "/dev/input/" + h
					}
				}
			} else if strings.HasPrefix(line, "B: KEY=") {
				hasKey = true
			} else if strings.HasPrefix(line, "B: ABS=") {
				val := strings.TrimPrefix(line, "B: ABS=")
				if len(val) > 1 {
					hasAbs = true
				}
			} else if strings.HasPrefix(line, "B: FF=") {
				val := strings.TrimPrefix(line, "B: FF=")
				if val != "0" {
					hasFF = true
				}
			}
		}

		if isVirtual || name == "" || eventPath == "" {
			continue
		}

		if !hasKey || !hasAbs {
			continue
		}

		lower := strings.ToLower(name)
		isGamepad := false
		if strings.Contains(lower, "controller") || strings.Contains(lower, "gamepad") ||
			strings.Contains(lower, "joystick") || strings.Contains(lower, "xbox") ||
			strings.Contains(lower, "playstation") || strings.Contains(lower, "dualshock") ||
			strings.Contains(lower, "dualsense") || strings.Contains(lower, "wireless controller") {
			isGamepad = true
		}
		if _, ok := vendorBrands[vendor]; ok {
			isGamepad = true
		}
		if !isGamepad {
			continue
		}

		brand := "generic"
		if b, ok := vendorBrands[vendor]; ok {
			brand = b
		}

		var features []string
		if hasAbs {
			features = append(features, "analog_sticks", "triggers")
		}
		if hasFF {
			features = append(features, "rumble")
		}
		features = append(features, "dpad")

		profilePath := filepath.Join(makimaDir(), name+".toml")
		_, profileErr := os.Stat(profilePath)

		controllers = append(controllers, controllerInfo{
			Name:       name,
			VendorID:   vendor,
			ProductID:  product,
			EventPath:  eventPath,
			Brand:      brand,
			HasProfile: profileErr == nil,
			Features:   features,
		})
	}

	return controllers
}

// ---------------------------------------------------------------------------
// Controller Templates
// ---------------------------------------------------------------------------

var controllerTemplates = map[string]string{
	"desktop": `# makima — %s :: Hyprland Desktop
# Template: desktop — window management + rice controls
# Brand: %s
# %s

[commands]
BTN_SOUTH = ["hyprctl dispatch focusurgentorlast"]
BTN_EAST = ["hyprctl dispatch killactive"]
BTN_WEST = ["$HOME/.local/bin/kitty-visual-launch"]
BTN_NORTH = ["$HOME/.local/bin/app-launcher"]
BTN_TL = ["hyprctl dispatch movefocus l"]
BTN_TR = ["hyprctl dispatch movefocus r"]
BTN_SELECT = ["wayshot --stdout | wl-copy"]
BTN_START = ["wlogout -b 5"]
BTN_MODE = ["hyprctl dispatch togglespecialworkspace dashboard"]
BTN_THUMBL = ["hyprctl dispatch workspace e-1"]
BTN_THUMBR = ["hyprctl dispatch workspace e+1"]

BTN_DPAD_LEFT = ["hyprctl dispatch workspace e-1"]
BTN_DPAD_RIGHT = ["hyprctl dispatch workspace e+1"]
BTN_DPAD_UP = ["hyprctl dispatch fullscreen 0"]
BTN_DPAD_DOWN = ["hyprctl dispatch togglefloating"]

[remap]
ABS_Z = ["BTN_LEFT"]
ABS_RZ = ["BTN_RIGHT"]

[settings]
LSTICK = "cursor"
RSTICK = "scroll"
LSTICK_SENSITIVITY = "6"
LSTICK_DEADZONE = "5"
RSTICK_SENSITIVITY = "6"
RSTICK_DEADZONE = "10"
CURSOR_SPEED = "10"
CURSOR_ACCEL = "0.5"
SCROLL_SPEED = "8"
16_BIT_AXIS = "true"
GRAB_DEVICE = "false"
`,
	"claude-code": `# makima — %s :: Claude Code
# Template: claude-code — terminal interaction optimized
# Brand: %s
# %s

[remap]
BTN_SOUTH = ["KEY_ENTER"]
BTN_EAST = ["KEY_ESC"]
BTN_WEST = ["KEY_SLASH"]
BTN_NORTH = ["KEY_Y"]
BTN_THUMBL = ["KEY_LEFTSHIFT", "KEY_TAB"]
BTN_THUMBR = ["KEY_TAB"]
BTN_START = ["KEY_LEFTCTRL", "KEY_EQUAL"]
BTN_SELECT = ["KEY_LEFTCTRL", "KEY_MINUS"]
BTN_DPAD_UP = ["KEY_1"]
BTN_DPAD_DOWN = ["KEY_2"]
BTN_DPAD_LEFT = ["KEY_UP"]
BTN_DPAD_RIGHT = ["KEY_DOWN"]
ABS_Z = ["KEY_PAGEUP"]
ABS_RZ = ["KEY_PAGEDOWN"]

[commands]
BTN_TL = ["hyprctl dispatch movefocus l"]
BTN_TR = ["hyprctl dispatch movefocus r"]
`,
	"gaming": `# makima — %s :: Gaming (minimal)
# Template: gaming — sticks + triggers only, no command remapping
# Brand: %s
# %s

[remap]
ABS_Z = ["BTN_LEFT"]
ABS_RZ = ["BTN_RIGHT"]

[settings]
LSTICK = "cursor"
RSTICK = "scroll"
LSTICK_SENSITIVITY = "8"
LSTICK_DEADZONE = "3"
RSTICK_SENSITIVITY = "8"
RSTICK_DEADZONE = "5"
CURSOR_SPEED = "12"
CURSOR_ACCEL = "0.3"
SCROLL_SPEED = "10"
16_BIT_AXIS = "true"
GRAB_DEVICE = "false"
`,
	"media": `# makima — %s :: Media Control
# Template: media — playerctl + volume + brightness
# Brand: %s
# %s

[commands]
BTN_SOUTH = ["playerctl play-pause"]
BTN_EAST = ["playerctl next"]
BTN_WEST = ["playerctl previous"]
BTN_NORTH = ["playerctl stop"]
BTN_TL = ["wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%%-"]
BTN_TR = ["wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%%+"]
BTN_DPAD_UP = ["brightnessctl set +5%%"]
BTN_DPAD_DOWN = ["brightnessctl set 5%%-"]
BTN_DPAD_LEFT = ["hyprctl dispatch workspace e-1"]
BTN_DPAD_RIGHT = ["hyprctl dispatch workspace e+1"]
BTN_START = ["wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle"]

[remap]
ABS_Z = ["BTN_LEFT"]
ABS_RZ = ["BTN_RIGHT"]

[settings]
LSTICK = "cursor"
RSTICK = "scroll"
LSTICK_SENSITIVITY = "6"
LSTICK_DEADZONE = "5"
CURSOR_SPEED = "10"
16_BIT_AXIS = "true"
GRAB_DEVICE = "false"
`,
	"macropad": `# makima — %s :: Macropad
# Template: macropad — generic USB keypad with rotary encoders
# Brand: %s
# %s
# NOTE: Key codes are defaults — run evtest to discover your device's actual codes,
# then update this profile. Rotary encoders may send KEY_VOLUMEUP/DOWN or REL_WHEEL.

[commands]
# Top row keys (often F13-F24 or numpad keys — update after evtest)
KEY_F13 = ["hyprctl dispatch workspace 1"]
KEY_F14 = ["hyprctl dispatch workspace 2"]
KEY_F15 = ["hyprctl dispatch workspace 3"]
KEY_F16 = ["hyprctl dispatch workspace 4"]
KEY_F17 = ["hyprctl dispatch workspace 5"]
KEY_F18 = ["$HOME/.local/bin/kitty-visual-launch"]
KEY_F19 = ["$HOME/.local/bin/app-launcher"]
KEY_F20 = ["hyprctl dispatch killactive"]
KEY_F21 = ["hyprctl dispatch fullscreen 0"]

# Encoder press (often KEY_MUTE or KEY_PLAYPAUSE)
KEY_MUTE = ["wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle"]

[remap]
# Rotary encoder rotation (update after evtest)
KEY_VOLUMEUP = ["KEY_PAGEUP"]
KEY_VOLUMEDOWN = ["KEY_PAGEDOWN"]
`,
}

var brandLabels = map[string]string{
	"xbox":        "BTN_SOUTH=A, BTN_EAST=B, BTN_WEST=X, BTN_NORTH=Y",
	"playstation": "BTN_SOUTH=Cross, BTN_EAST=Circle, BTN_WEST=Square, BTN_NORTH=Triangle",
	"nintendo":    "BTN_SOUTH=B, BTN_EAST=A, BTN_WEST=Y, BTN_NORTH=X",
	"generic":     "BTN_SOUTH=South, BTN_EAST=East, BTN_WEST=West, BTN_NORTH=North",
	"valve":       "BTN_SOUTH=A, BTN_EAST=B, BTN_WEST=X, BTN_NORTH=Y",
	"8bitdo":      "BTN_SOUTH=B, BTN_EAST=A, BTN_WEST=Y, BTN_NORTH=X",
	"macropad":    "Keys vary by device — run evtest to identify keycodes",
}

// ---------------------------------------------------------------------------
// MIDI Detection
// ---------------------------------------------------------------------------

type midiDevice struct {
	CardNum    int    `json:"card_num"`
	DeviceNum  int    `json:"device_num"`
	Name       string `json:"name"`
	RawPath    string `json:"raw_path"`
	DevicePath string `json:"device_path"`
	HasMapping bool   `json:"has_mapping"`
}

func detectMidiDevices() []midiDevice {
	var devices []midiDevice

	out, _, err := inputRunCmd("amidi", "-l")
	if err == nil && out != "" {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "Dir") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			hw := fields[1]
			name := strings.Join(fields[2:], " ")

			parts := strings.Split(strings.TrimPrefix(hw, "hw:"), ",")
			card, _ := strconv.Atoi(parts[0])
			dev := 0
			if len(parts) > 1 {
				dev, _ = strconv.Atoi(parts[1])
			}

			devPath := fmt.Sprintf("/dev/snd/midiC%dD%d", card, dev)
			mappingPath := filepath.Join(midiDir(), name+".toml")
			_, mapErr := os.Stat(mappingPath)

			devices = append(devices, midiDevice{
				CardNum:    card,
				DeviceNum:  dev,
				Name:       name,
				RawPath:    hw,
				DevicePath: devPath,
				HasMapping: mapErr == nil,
			})
		}
		return devices
	}

	matches, _ := filepath.Glob("/dev/snd/midiC*D*")
	for _, m := range matches {
		base := filepath.Base(m)
		var card, dev int
		fmt.Sscanf(base, "midiC%dD%d", &card, &dev)

		nameBytes, _ := os.ReadFile(fmt.Sprintf("/proc/asound/card%d/id", card))
		name := strings.TrimSpace(string(nameBytes))
		if name == "" {
			name = fmt.Sprintf("Card %d Device %d", card, dev)
		}

		hw := fmt.Sprintf("hw:%d,%d", card, dev)
		mappingPath := filepath.Join(midiDir(), name+".toml")
		_, mapErr := os.Stat(mappingPath)

		devices = append(devices, midiDevice{
			CardNum:    card,
			DeviceNum:  dev,
			Name:       name,
			RawPath:    hw,
			DevicePath: m,
			HasMapping: mapErr == nil,
		})
	}

	return devices
}

// ---------------------------------------------------------------------------
// MIDI Templates
// ---------------------------------------------------------------------------

var midiTemplates = map[string]string{
	"desktop-control": `# MIDI mapping — %s
# Template: desktop-control — volume, brightness, workspace switching
# Device: %s

[device]
name = "%s"
hw = "%s"

[cc]
# Faders / knobs → system controls
1 = { type = "command", action = "wpctl set-volume @DEFAULT_AUDIO_SINK@ {value}%%" }
7 = { type = "command", action = "brightnessctl set {value}%%" }

[note]
# Pads / keys → workspace switching
36 = { type = "command", action = "hyprctl dispatch workspace 1" }
37 = { type = "command", action = "hyprctl dispatch workspace 2" }
38 = { type = "command", action = "hyprctl dispatch workspace 3" }
39 = { type = "command", action = "hyprctl dispatch workspace 4" }
40 = { type = "command", action = "hyprctl dispatch workspace 5" }
41 = { type = "command", action = "hyprctl dispatch workspace 6" }
42 = { type = "command", action = "hyprctl dispatch workspace 7" }
43 = { type = "command", action = "hyprctl dispatch workspace 8" }
`,
	"shader-control": `# MIDI mapping — %s
# Template: shader-control — shader cycling, wallpaper control
# Device: %s

[device]
name = "%s"
hw = "%s"

[cc]
# CC1 = shader cycle
1 = { type = "command", action = "hg shader next" }
# CC7 = wallpaper cycle
7 = { type = "command", action = "hg wallpaper next" }

[note]
# Pads → specific shader presets
36 = { type = "command", action = "hg shader set bloom-soft" }
37 = { type = "command", action = "hg shader set crt-chromatic" }
38 = { type = "command", action = "hg shader set cyberpunk" }
39 = { type = "command", action = "hg shader set vaporwave" }
40 = { type = "command", action = "hg shader set underwater" }
41 = { type = "command", action = "hg shader set halftone" }
42 = { type = "command", action = "hg shader set auroras" }
43 = { type = "command", action = "hg shader random" }
`,
	"volume-mixer": `# MIDI mapping — %s
# Template: volume-mixer — per-sink volume control via PipeWire
# Device: %s

[device]
name = "%s"
hw = "%s"

[cc]
# Faders → individual audio sinks (adjust sink IDs for your system)
1 = { type = "command", action = "wpctl set-volume @DEFAULT_AUDIO_SINK@ {value}%%" }
2 = { type = "command", action = "wpctl set-volume @DEFAULT_AUDIO_SOURCE@ {value}%%" }

[note]
# Pads → mute toggles
36 = { type = "command", action = "wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle" }
37 = { type = "command", action = "wpctl set-mute @DEFAULT_AUDIO_SOURCE@ toggle" }
`,
	"vj-control": `# MIDI mapping — %s
# Template: vj-control — VJ/DJ live performance (Resolume, TouchDesigner, OBS)
# Device: %s

[device]
name = "%s"
hw = "%s"

[cc]
# Encoders → OSC/WebSocket outputs for VJ software
# Row 1 (CC 32-35): Layer opacity / effect intensity
32 = { type = "osc", action = "/composition/layers/1/video/opacity {value}" }
33 = { type = "osc", action = "/composition/layers/2/video/opacity {value}" }
34 = { type = "osc", action = "/composition/layers/3/video/opacity {value}" }
35 = { type = "osc", action = "/composition/layers/4/video/opacity {value}" }
# Row 2 (CC 36-39): Effect parameters
36 = { type = "osc", action = "/composition/layers/1/video/effects/1/param1 {value}" }
37 = { type = "osc", action = "/composition/layers/2/video/effects/1/param1 {value}" }
38 = { type = "osc", action = "/composition/layers/3/video/effects/1/param1 {value}" }
39 = { type = "osc", action = "/composition/layers/4/video/effects/1/param1 {value}" }
# Row 3 (CC 40-43): Clip transport / speed
40 = { type = "osc", action = "/composition/layers/1/clips/1/transport/position {value}" }
41 = { type = "osc", action = "/composition/layers/2/clips/1/transport/position {value}" }
42 = { type = "osc", action = "/composition/layers/3/clips/1/transport/position {value}" }
43 = { type = "osc", action = "/composition/layers/4/clips/1/transport/position {value}" }
# Row 4 (CC 44-47): Master / crossfader / BPM
44 = { type = "osc", action = "/composition/master/video/opacity {value}" }
45 = { type = "osc", action = "/composition/crossfader/phase {value}" }
46 = { type = "osc", action = "/composition/tempocontroller/tempo {value}" }
47 = { type = "osc", action = "/composition/master/audio/volume {value}" }

[note]
# Encoder push buttons → clip triggers / layer toggles
32 = { type = "osc", action = "/composition/layers/1/clips/1/connect 1" }
33 = { type = "osc", action = "/composition/layers/2/clips/1/connect 1" }
34 = { type = "osc", action = "/composition/layers/3/clips/1/connect 1" }
35 = { type = "osc", action = "/composition/layers/4/clips/1/connect 1" }
36 = { type = "osc", action = "/composition/layers/1/bypassed 1" }
37 = { type = "osc", action = "/composition/layers/2/bypassed 1" }
38 = { type = "osc", action = "/composition/layers/3/bypassed 1" }
39 = { type = "osc", action = "/composition/layers/4/bypassed 1" }
`,
	"en16-default": `# MIDI mapping — %s
# Template: en16-default — Intech Studio Grid EN16 (16 encoders + push buttons)
# Device: %s
# Default MIDI: CC 32-47 ch0 (encoders), Note On/Off (push buttons)

[device]
name = "%s"
hw = "%s"

[cc]
# 16 endless encoders (CC 32-47, factory default)
# Row 1: System controls
32 = { type = "command", action = "wpctl set-volume @DEFAULT_AUDIO_SINK@ {value}%%" }
33 = { type = "command", action = "wpctl set-volume @DEFAULT_AUDIO_SOURCE@ {value}%%" }
34 = { type = "command", action = "brightnessctl set {value}%%" }
35 = { type = "command", action = "playerctl volume {value}" }
# Row 2: Workspace navigation
36 = { type = "command", action = "hyprctl dispatch workspace 1" }
37 = { type = "command", action = "hyprctl dispatch workspace 2" }
38 = { type = "command", action = "hyprctl dispatch workspace 3" }
39 = { type = "command", action = "hyprctl dispatch workspace 4" }
# Row 3: Shader / wallpaper
40 = { type = "command", action = "hg shader next" }
41 = { type = "command", action = "hg shader random" }
42 = { type = "command", action = "hg wallpaper next" }
43 = { type = "command", action = "hg wallpaper random" }
# Row 4: Media
44 = { type = "command", action = "playerctl previous" }
45 = { type = "command", action = "playerctl play-pause" }
46 = { type = "command", action = "playerctl next" }
47 = { type = "command", action = "playerctl position {value}" }

[note]
# Encoder push buttons (same note numbers as CC)
32 = { type = "command", action = "wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle" }
33 = { type = "command", action = "wpctl set-mute @DEFAULT_AUDIO_SOURCE@ toggle" }
40 = { type = "command", action = "hg shader set bloom-soft" }
41 = { type = "command", action = "hg shader set crt-chromatic" }
44 = { type = "command", action = "playerctl previous" }
45 = { type = "command", action = "playerctl play-pause" }
46 = { type = "command", action = "playerctl next" }
47 = { type = "command", action = "playerctl stop" }
`,
}

// ===========================================================================
// I/O types
// ===========================================================================

// ── InputModule ────────────────────────────────────────────────────────────

type InputStatusInput struct{}

type serviceStatus struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type InputStatusOutput struct {
	Services []serviceStatus                 `json:"services"`
	Battery  *InputGetJuhradialBatteryOutput `json:"battery,omitempty"`
}

type InputGetJuhradialConfigInput struct{}
type InputGetJuhradialConfigOutput struct {
	Content string `json:"content"`
	Path    string `json:"path"`
}

type InputSetJuhradialConfigInput struct {
	Content string `json:"content" jsonschema:"required,description=Full juhradial config.json content in JSON format"`
}
type InputSetJuhradialConfigOutput struct {
	Written string `json:"written"`
	Valid   bool   `json:"valid"`
}

type InputGetJuhradialProfilesInput struct{}
type InputGetJuhradialProfilesOutput struct {
	Content string `json:"content"`
	Path    string `json:"path"`
}

type InputSetJuhradialProfilesInput struct {
	Content string `json:"content" jsonschema:"required,description=Full juhradial profiles.json content in JSON format"`
}
type InputSetJuhradialProfilesOutput struct {
	Written string `json:"written"`
	Valid   bool   `json:"valid"`
}

type InputGetJuhradialBatteryInput struct{}
type InputGetJuhradialBatteryOutput struct {
	Device   string `json:"device,omitempty"`
	Percent  int    `json:"percent"`
	Charging bool   `json:"charging"`
	Source   string `json:"source"`
}

type InputListMakimaInput struct{}

type makimaProfile struct {
	Name     string         `json:"name"`
	File     string         `json:"file"`
	Remap    map[string]any `json:"remap,omitempty"`
	Commands map[string]any `json:"commands,omitempty"`
}

type InputListMakimaOutput struct {
	Profiles []makimaProfile `json:"profiles"`
}

type InputGetMakimaInput struct {
	Name string `json:"name" jsonschema:"required,description=Profile name (e.g. 'MX Master 4 Mouse::firefox'). .toml extension optional."`
}
type InputGetMakimaOutput struct {
	Content string `json:"content"`
}

type InputSetMakimaInput struct {
	Name    string `json:"name" jsonschema:"required,description=Profile name (e.g. 'MX Master 4 Mouse::firefox'). .toml extension optional."`
	Content string `json:"content" jsonschema:"required,description=Full TOML content for the makima profile"`
}
type InputSetMakimaOutput struct {
	Written string `json:"written"`
	Valid   bool   `json:"valid"`
}

type InputDeleteMakimaInput struct {
	Name string `json:"name" jsonschema:"required,description=Profile name to delete. .toml extension optional."`
}
type InputDeleteMakimaOutput struct {
	Deleted string `json:"deleted"`
}

type InputRestartInput struct {
	Service string `json:"service" jsonschema:"required,description=Which service group to restart,enum=mouse,enum=controller,enum=all"`
}
type InputRestartOutput struct {
	Services []serviceStatus `json:"services"`
}

// ── BluetoothModule ────────────────────────────────────────────────────────

type BTListInput struct {
	Filter string `json:"filter,omitempty" jsonschema:"description=Filter devices by state,enum=paired,enum=connected,enum=trusted"`
}
type BTListOutput struct {
	Devices []btDevice `json:"devices"`
}

type BTDeviceInfoInput struct {
	Device string `json:"device" jsonschema:"required,description=MAC address or device name (case-insensitive substring match)"`
}
type BTDeviceInfoOutput struct {
	MAC       string   `json:"mac"`
	Name      string   `json:"name"`
	Connected bool     `json:"connected"`
	Paired    bool     `json:"paired"`
	Trusted   bool     `json:"trusted"`
	Blocked   bool     `json:"blocked"`
	Battery   int      `json:"battery"`
	UUIDs     []string `json:"uuids,omitempty"`
}

type BTConnectInput struct {
	Device string `json:"device" jsonschema:"required,description=MAC address or device name"`
}
type BTConnectOutput struct {
	Status string `json:"status"`
	MAC    string `json:"mac"`
	Output string `json:"output"`
}

type BTDisconnectInput struct {
	Device string `json:"device" jsonschema:"required,description=MAC address or device name"`
}
type BTDisconnectOutput struct {
	Status string `json:"status"`
	MAC    string `json:"mac"`
	Output string `json:"output"`
}

type BTPairInput struct {
	Device      string `json:"device" jsonschema:"required,description=MAC address of the device to pair"`
	RemoveFirst bool   `json:"remove_first,omitempty" jsonschema:"description=Remove existing pairing before re-pairing (fixes stale BLE bonds)"`
}
type BTPairOutput struct {
	Status string `json:"status"`
	MAC    string `json:"mac"`
	Output string `json:"output"`
}

type BTRemoveInput struct {
	Device string `json:"device" jsonschema:"required,description=MAC address or device name"`
}
type BTRemoveOutput struct {
	Status string `json:"status"`
	MAC    string `json:"mac"`
	Output string `json:"output"`
}

type BTTrustInput struct {
	Device string `json:"device" jsonschema:"required,description=MAC address or device name"`
	Trust  *bool  `json:"trust,omitempty" jsonschema:"description=True to trust false to untrust. Default true."`
}
type BTTrustOutput struct {
	Status string `json:"status"`
	MAC    string `json:"mac"`
	Output string `json:"output"`
}

type BTScanInput struct {
	Action  string `json:"action" jsonschema:"required,description=Scan action,enum=start,enum=stop,enum=list"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=Scan duration in seconds (default: 8)"`
}
type BTScanOutput struct {
	Status  string     `json:"status"`
	Devices []btDevice `json:"devices,omitempty"`
}

type BTPowerInput struct {
	On bool `json:"on" jsonschema:"required,description=True to power on false to power off"`
}
type BTPowerOutput struct {
	Status string `json:"status"`
	Output string `json:"output"`
}

// ── ControllerModule ───────────────────────────────────────────────────────

type DetectControllersInput struct{}
type DetectControllersOutput struct {
	Controllers []controllerInfo `json:"controllers"`
}

type GenerateProfileInput struct {
	DeviceName string `json:"device_name" jsonschema:"required,description=Exact device name from input_detect_controllers"`
	Template   string `json:"template" jsonschema:"required,description=Preset mapping template,enum=desktop,enum=claude-code,enum=gaming,enum=media,enum=macropad"`
	AppID      string `json:"app_id,omitempty" jsonschema:"description=Per-app override ID (e.g. 'com.mitchellh.ghostty'). Creates DeviceName::app_id.toml"`
	Force      bool   `json:"force,omitempty" jsonschema:"description=Overwrite existing profile if it exists. Default false."`
}
type GenerateProfileOutput struct {
	Written  string `json:"written"`
	Template string `json:"template"`
	Brand    string `json:"brand"`
	Preview  string `json:"preview"`
}

type ControllerTestInput struct {
	DeviceName string `json:"device_name,omitempty" jsonschema:"description=Device name (looked up in /proc/bus/input/devices)"`
	EventPath  string `json:"event_path,omitempty" jsonschema:"description=Direct event device path (e.g. /dev/input/event4)"`
}

type controllerEvent struct {
	Type  string `json:"type"`
	Code  string `json:"code"`
	Value string `json:"value"`
}

type ControllerTestOutput struct {
	EventPath string            `json:"event_path"`
	Status    string            `json:"status"`
	Events    []controllerEvent `json:"events"`
}

// ── WorkflowModule (composed tools) ───────────────────────────────────────

type AutoSetupControllerInput struct {
	Template string `json:"template,omitempty" jsonschema:"description=Profile template: gaming desktop or media (default: desktop),enum=gaming,enum=desktop,enum=media"`
	Execute  bool   `json:"execute,omitempty" jsonschema:"description=Set true to write profiles and restart (default: dry-run)"`
}

type controllerSetupResult struct {
	Name     string `json:"name"`
	Brand    string `json:"brand"`
	Profile  string `json:"profile"`
	Written  bool   `json:"written"`
	Skipped  bool   `json:"skipped,omitempty"`
	SkipNote string `json:"skip_note,omitempty"`
	Preview  string `json:"preview"`
}

type AutoSetupControllerOutput struct {
	DryRun        bool                    `json:"dry_run"`
	Controllers   []controllerSetupResult `json:"controllers"`
	MakimaRestart string                  `json:"makima_restart,omitempty"`
}

type BTDiscoverConnectInput struct {
	DevicePattern string `json:"device_pattern" jsonschema:"required,description=Substring match on device name"`
	AutoTrust     *bool  `json:"auto_trust,omitempty" jsonschema:"description=Auto-trust device after pairing (default: true)"`
	Timeout       int    `json:"timeout,omitempty" jsonschema:"description=Scan timeout in seconds (default: 10)"`
}

type btDiscoverStep struct {
	Step   string `json:"step"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type BTDiscoverConnectOutput struct {
	DeviceName string           `json:"device_name"`
	MAC        string           `json:"mac"`
	Steps      []btDiscoverStep `json:"steps"`
	Connected  bool             `json:"connected"`
}

// ── MidiModule ─────────────────────────────────────────────────────────────

type MidiListInput struct{}
type MidiListOutput struct {
	Devices []midiDevice `json:"devices"`
}

type MidiGenerateInput struct {
	DeviceName string `json:"device_name" jsonschema:"required,description=MIDI device name from midi_list_devices"`
	Template   string `json:"template" jsonschema:"required,description=Preset mapping template,enum=desktop-control,enum=shader-control,enum=volume-mixer"`
}
type MidiGenerateOutput struct {
	Written  string `json:"written"`
	Template string `json:"template"`
	Preview  string `json:"preview"`
}

type MidiGetInput struct {
	Name string `json:"name" jsonschema:"required,description=Mapping config name. .toml extension optional."`
}
type MidiGetOutput struct {
	Content string `json:"content"`
}

type MidiSetInput struct {
	Name    string `json:"name" jsonschema:"required,description=Mapping config name. .toml extension optional."`
	Content string `json:"content" jsonschema:"required,description=Full TOML content for the MIDI mapping"`
}
type MidiSetOutput struct {
	Written string `json:"written"`
	Valid   bool   `json:"valid"`
}

// ── JuhradialModule ────────────────────────────────────────────────────────

// ===========================================================================
// Modules
// ===========================================================================

// ── InputModule ────────────────────────────────────────────────────────────

type InputModule struct{}

func (m *InputModule) Name() string { return "input" }
func (m *InputModule) Description() string {
	return "Input device management (juhradial-mx, makima, services)"
}

func (m *InputModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[InputStatusInput, InputStatusOutput](
			"input_status",
			"Show running state of juhradial mouse services, the makima controller service, and MX battery status when available.",
			func(_ context.Context, _ InputStatusInput) (InputStatusOutput, error) {
				var result InputStatusOutput

				type serviceCheck struct {
					name string
					user bool
				}
				for _, svc := range []serviceCheck{
					{name: "juhradialmx-daemon", user: true},
					{name: "ydotool", user: true},
					{name: "makima", user: false},
				} {
					var err error
					if svc.user {
						_, _, err = juhradialSystemctl("is-active", svc.name+".service")
					} else {
						_, _, err = inputRunCmd("systemctl", "is-active", svc.name+".service")
					}
					result.Services = append(result.Services, serviceStatus{
						Name:   svc.name,
						Active: err == nil,
					})
				}

				battery, err := juhradialBatteryStatus()
				if err == nil {
					result.Battery = &battery
				}

				return result, nil
			},
		),

		handler.TypedHandler[InputListMakimaInput, InputListMakimaOutput](
			"input_list_makima_profiles",
			"List all makima per-app button remapping profiles with their remap and command bindings.",
			func(_ context.Context, _ InputListMakimaInput) (InputListMakimaOutput, error) {
				dir := makimaDir()
				entries, err := os.ReadDir(dir)
				if err != nil {
					return InputListMakimaOutput{}, fmt.Errorf("read makima dir: %w", err)
				}

				var profiles []makimaProfile
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
						continue
					}
					p := makimaProfile{
						Name: strings.TrimSuffix(e.Name(), ".toml"),
						File: e.Name(),
					}

					data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
					var parsed map[string]any
					if _, terr := toml.Decode(string(data), &parsed); terr == nil {
						if r, ok := parsed["remap"]; ok {
							p.Remap, _ = r.(map[string]any)
						}
						if c, ok := parsed["commands"]; ok {
							p.Commands, _ = c.(map[string]any)
						}
					}

					profiles = append(profiles, p)
				}

				return InputListMakimaOutput{Profiles: profiles}, nil
			},
		),

		handler.TypedHandler[InputGetMakimaInput, InputGetMakimaOutput](
			"input_get_makima_profile",
			"Read a specific makima per-app profile by name.",
			func(_ context.Context, input InputGetMakimaInput) (InputGetMakimaOutput, error) {
				if input.Name == "" {
					return InputGetMakimaOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				name := input.Name
				if !strings.HasSuffix(name, ".toml") {
					name += ".toml"
				}
				data, err := os.ReadFile(filepath.Join(makimaDir(), name))
				if err != nil {
					return InputGetMakimaOutput{}, fmt.Errorf("[%s] read profile: %w", handler.ErrNotFound, err)
				}
				return InputGetMakimaOutput{Content: string(data)}, nil
			},
		),

		handler.TypedHandler[InputSetMakimaInput, InputSetMakimaOutput](
			"input_set_makima_profile",
			"Create or update a makima per-app profile. Validates TOML before writing.",
			func(_ context.Context, input InputSetMakimaInput) (InputSetMakimaOutput, error) {
				if input.Name == "" {
					return InputSetMakimaOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				if input.Content == "" {
					return InputSetMakimaOutput{}, fmt.Errorf("[%s] content is required", handler.ErrInvalidParam)
				}
				name := input.Name
				if !strings.HasSuffix(name, ".toml") {
					name += ".toml"
				}

				var parsed map[string]any
				if _, err := toml.Decode(input.Content, &parsed); err != nil {
					return InputSetMakimaOutput{}, fmt.Errorf("[%s] invalid TOML: %w", handler.ErrInvalidParam, err)
				}

				path := filepath.Join(makimaDir(), name)
				if err := os.WriteFile(path, []byte(input.Content), 0644); err != nil {
					return InputSetMakimaOutput{}, fmt.Errorf("write profile: %w", err)
				}

				return InputSetMakimaOutput{Written: path, Valid: true}, nil
			},
		),

		handler.TypedHandler[InputDeleteMakimaInput, InputDeleteMakimaOutput](
			"input_delete_makima_profile",
			"Delete a makima per-app profile.",
			func(_ context.Context, input InputDeleteMakimaInput) (InputDeleteMakimaOutput, error) {
				if input.Name == "" {
					return InputDeleteMakimaOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				name := input.Name
				if !strings.HasSuffix(name, ".toml") {
					name += ".toml"
				}
				path := filepath.Join(makimaDir(), name)
				if err := os.Remove(path); err != nil {
					return InputDeleteMakimaOutput{}, fmt.Errorf("[%s] delete profile: %w", handler.ErrNotFound, err)
				}
				return InputDeleteMakimaOutput{Deleted: path}, nil
			},
		),

		handler.TypedHandler[InputRestartInput, InputRestartOutput](
			"input_restart_services",
			"Restart juhradial mouse services or the makima controller service. Restarting the controller service may require elevated systemd permissions.",
			func(_ context.Context, input InputRestartInput) (InputRestartOutput, error) {
				var userTargets []string
				var systemTargets []string
				switch input.Service {
				case "mouse":
					userTargets = []string{"ydotool", "juhradialmx-daemon"}
				case "controller":
					systemTargets = []string{"makima"}
				case "all":
					userTargets = []string{"ydotool", "juhradialmx-daemon"}
					systemTargets = []string{"makima"}
				default:
					return InputRestartOutput{}, fmt.Errorf("[%s] service must be mouse, controller, or all", handler.ErrInvalidParam)
				}

				var result InputRestartOutput
				for _, t := range userTargets {
					_, stderr, err := juhradialSystemctl("restart", t+".service")
					status := serviceStatus{Name: t, Active: err == nil}
					if err != nil {
						status.Name += " (error: " + stderr + ")"
					}
					result.Services = append(result.Services, status)
				}
				for _, t := range systemTargets {
					_, stderr, err := inputRunCmd("systemctl", "restart", t+".service")
					status := serviceStatus{Name: t, Active: err == nil}
					if err != nil {
						status.Name += " (error: " + stderr + ")"
					}
					result.Services = append(result.Services, status)
				}

				return result, nil
			},
		),
	}
}

// ── BluetoothModule ────────────────────────────────────────────────────────

type BluetoothModule struct{}

func (m *BluetoothModule) Name() string        { return "bluetooth" }
func (m *BluetoothModule) Description() string { return "Bluetooth device management via bluetoothctl" }

func (m *BluetoothModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[BTListInput, BTListOutput](
			"bt_list_devices",
			"List Bluetooth devices with connection status and battery levels.",
			func(_ context.Context, input BTListInput) (BTListOutput, error) {
				var cmdArgs []string
				switch input.Filter {
				case "connected":
					cmdArgs = []string{"devices", "Connected"}
				case "trusted":
					cmdArgs = []string{"devices", "Trusted"}
				case "paired", "":
					cmdArgs = []string{"devices", "Paired"}
				default:
					return BTListOutput{}, fmt.Errorf("[%s] filter must be paired, connected, or trusted", handler.ErrInvalidParam)
				}

				out, _, err := inputRunCmd("bluetoothctl", cmdArgs...)
				if err != nil {
					return BTListOutput{}, fmt.Errorf("bluetoothctl: %w", err)
				}

				devices := make([]btDevice, 0)
				for _, line := range strings.Split(out, "\n") {
					m := deviceRe.FindStringSubmatch(strings.TrimSpace(line))
					if m == nil {
						continue
					}
					d := btDevice{MAC: m[1], Name: m[2], Battery: -1}
					btEnrichDevice(&d)
					devices = append(devices, d)
				}

				return BTListOutput{Devices: devices}, nil
			},
		),

		handler.TypedHandler[BTDeviceInfoInput, BTDeviceInfoOutput](
			"bt_device_info",
			"Get detailed info for a Bluetooth device including battery, profiles, and trust status.",
			func(_ context.Context, input BTDeviceInfoInput) (BTDeviceInfoOutput, error) {
				if input.Device == "" {
					return BTDeviceInfoOutput{}, fmt.Errorf("[%s] device is required (MAC address or name)", handler.ErrInvalidParam)
				}

				mac, err := resolveAnyDevice(input.Device)
				if err != nil {
					return BTDeviceInfoOutput{}, fmt.Errorf("[%s] %w", handler.ErrNotFound, err)
				}

				out, _, err := inputRunCmd("bluetoothctl", "info", mac)
				if err != nil {
					return BTDeviceInfoOutput{}, fmt.Errorf("bluetoothctl info: %w", err)
				}

				info := BTDeviceInfoOutput{MAC: mac, Battery: -1}
				for _, line := range strings.Split(out, "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "Name: ") {
						info.Name = strings.TrimPrefix(line, "Name: ")
					} else if strings.HasPrefix(line, "Connected: ") {
						info.Connected = strings.Contains(line, "yes")
					} else if strings.HasPrefix(line, "Paired: ") {
						info.Paired = strings.Contains(line, "yes")
					} else if strings.HasPrefix(line, "Trusted: ") {
						info.Trusted = strings.Contains(line, "yes")
					} else if strings.HasPrefix(line, "Blocked: ") {
						info.Blocked = strings.Contains(line, "yes")
					} else if strings.HasPrefix(line, "Battery Percentage:") {
						if bm := batteryRe.FindStringSubmatch(line); bm != nil {
							info.Battery, _ = strconv.Atoi(bm[1])
						}
					} else if strings.HasPrefix(line, "UUID: ") {
						info.UUIDs = append(info.UUIDs, strings.TrimPrefix(line, "UUID: "))
					}
				}

				return info, nil
			},
		),

		handler.TypedHandler[BTConnectInput, BTConnectOutput](
			"bt_connect",
			"Connect to a Bluetooth device with BLE retry logic. Resolves names against all known devices (paired + scanned).",
			func(_ context.Context, input BTConnectInput) (BTConnectOutput, error) {
				if input.Device == "" {
					return BTConnectOutput{}, fmt.Errorf("[%s] device is required", handler.ErrInvalidParam)
				}
				mac, err := resolveAnyDevice(input.Device)
				if err != nil {
					return BTConnectOutput{}, fmt.Errorf("[%s] %w", handler.ErrNotFound, err)
				}
				out, err := btInteractiveConnect(mac, 3)
				if err != nil {
					return BTConnectOutput{}, err
				}
				return BTConnectOutput{Status: "connected", MAC: mac, Output: out}, nil
			},
		),

		handler.TypedHandler[BTDisconnectInput, BTDisconnectOutput](
			"bt_disconnect",
			"Disconnect a Bluetooth device.",
			func(_ context.Context, input BTDisconnectInput) (BTDisconnectOutput, error) {
				if input.Device == "" {
					return BTDisconnectOutput{}, fmt.Errorf("[%s] device is required", handler.ErrInvalidParam)
				}
				mac, err := resolveAnyDevice(input.Device)
				if err != nil {
					return BTDisconnectOutput{}, fmt.Errorf("[%s] %w", handler.ErrNotFound, err)
				}
				out, stderr, err := inputRunCmd("bluetoothctl", "disconnect", mac)
				if err != nil {
					return BTDisconnectOutput{}, fmt.Errorf("disconnect failed: %s %s", out, stderr)
				}
				return BTDisconnectOutput{Status: "disconnected", MAC: mac, Output: out}, nil
			},
		),

		handler.TypedHandler[BTPairInput, BTPairOutput](
			"bt_pair",
			"Pair with a Bluetooth device using interactive agent (handles BLE auth). Set remove_first=true to clear stale bonds before re-pairing.",
			func(_ context.Context, input BTPairInput) (BTPairOutput, error) {
				if input.Device == "" {
					return BTPairOutput{}, fmt.Errorf("[%s] device is required (MAC address)", handler.ErrInvalidParam)
				}
				if !macRe.MatchString(input.Device) {
					return BTPairOutput{}, fmt.Errorf("[%s] device must be a MAC address for pairing", handler.ErrInvalidParam)
				}

				// Optionally remove stale pairing first (fixes BLE re-pair failures)
				if input.RemoveFirst {
					inputRunCmd("bluetoothctl", "remove", input.Device)
					time.Sleep(time.Second)
				}

				// Use interactive pairing with agent for BLE compatibility
				out, err := btInteractivePair(input.Device)
				if err != nil {
					return BTPairOutput{}, fmt.Errorf("pair failed: %w\nOutput: %s", err, out)
				}
				// Auto-trust after pairing
				inputRunCmd("bluetoothctl", "trust", input.Device)
				return BTPairOutput{Status: "paired", MAC: input.Device, Output: out}, nil
			},
		),

		handler.TypedHandler[BTRemoveInput, BTRemoveOutput](
			"bt_remove",
			"Remove (forget) a paired Bluetooth device.",
			func(_ context.Context, input BTRemoveInput) (BTRemoveOutput, error) {
				if input.Device == "" {
					return BTRemoveOutput{}, fmt.Errorf("[%s] device is required", handler.ErrInvalidParam)
				}
				mac, err := resolveAnyDevice(input.Device)
				if err != nil {
					return BTRemoveOutput{}, fmt.Errorf("[%s] %w", handler.ErrNotFound, err)
				}
				out, stderr, err := inputRunCmd("bluetoothctl", "remove", mac)
				if err != nil {
					return BTRemoveOutput{}, fmt.Errorf("remove failed: %s %s", out, stderr)
				}
				return BTRemoveOutput{Status: "removed", MAC: mac, Output: out}, nil
			},
		),

		handler.TypedHandler[BTTrustInput, BTTrustOutput](
			"bt_trust",
			"Trust or untrust a Bluetooth device.",
			func(_ context.Context, input BTTrustInput) (BTTrustOutput, error) {
				if input.Device == "" {
					return BTTrustOutput{}, fmt.Errorf("[%s] device is required", handler.ErrInvalidParam)
				}
				trust := true
				if input.Trust != nil {
					trust = *input.Trust
				}
				mac, err := resolveAnyDevice(input.Device)
				if err != nil {
					return BTTrustOutput{}, fmt.Errorf("[%s] %w", handler.ErrNotFound, err)
				}
				action := "trust"
				if !trust {
					action = "untrust"
				}
				out, stderr, err := inputRunCmd("bluetoothctl", action, mac)
				if err != nil {
					return BTTrustOutput{}, fmt.Errorf("%s failed: %s %s", action, out, stderr)
				}
				return BTTrustOutput{Status: action + "ed", MAC: mac, Output: out}, nil
			},
		),

		handler.TypedHandler[BTScanInput, BTScanOutput](
			"bt_scan",
			"Scan for nearby Bluetooth devices. 'start' scans for 8 seconds (configurable) and returns discovered devices.",
			func(_ context.Context, input BTScanInput) (BTScanOutput, error) {
				switch input.Action {
				case "start":
					timeout := input.Timeout
					if timeout <= 0 {
						timeout = 8
					}
					cmd := exec.Command("bluetoothctl", "--timeout", fmt.Sprintf("%d", timeout), "scan", "on")
					if err := cmd.Start(); err != nil {
						return BTScanOutput{}, fmt.Errorf("start scan: %w", err)
					}
					time.Sleep(time.Duration(timeout) * time.Second)
					cmd.Process.Kill()
					cmd.Wait()

					out, _, _ := inputRunCmd("bluetoothctl", "devices")
					devices := make([]btDevice, 0)
					for _, line := range strings.Split(out, "\n") {
						dm := deviceRe.FindStringSubmatch(strings.TrimSpace(line))
						if dm != nil {
							devices = append(devices, btDevice{MAC: dm[1], Name: dm[2], Battery: -1})
						}
					}
					return BTScanOutput{Status: "scan_complete", Devices: devices}, nil

				case "stop":
					inputRunCmd("bluetoothctl", "scan", "off")
					return BTScanOutput{Status: "scan_stopped"}, nil

				case "list":
					out, _, _ := inputRunCmd("bluetoothctl", "devices")
					devices := make([]btDevice, 0)
					for _, line := range strings.Split(out, "\n") {
						dm := deviceRe.FindStringSubmatch(strings.TrimSpace(line))
						if dm != nil {
							devices = append(devices, btDevice{MAC: dm[1], Name: dm[2], Battery: -1})
						}
					}
					return BTScanOutput{Devices: devices}, nil

				default:
					return BTScanOutput{}, fmt.Errorf("[%s] action must be start, stop, or list", handler.ErrInvalidParam)
				}
			},
		),

		handler.TypedHandler[BTPowerInput, BTPowerOutput](
			"bt_power",
			"Toggle Bluetooth adapter power on or off.",
			func(_ context.Context, input BTPowerInput) (BTPowerOutput, error) {
				state := "off"
				if input.On {
					state = "on"
				}
				out, stderr, err := inputRunCmd("bluetoothctl", "power", state)
				if err != nil {
					return BTPowerOutput{}, fmt.Errorf("power %s failed: %s %s", state, out, stderr)
				}
				return BTPowerOutput{Status: "power_" + state, Output: out}, nil
			},
		),
	}
}

// ── ControllerModule ───────────────────────────────────────────────────────

type ControllerModule struct{}

func (m *ControllerModule) Name() string { return "controller" }
func (m *ControllerModule) Description() string {
	return "Gamepad/controller detection and profile generation"
}

func (m *ControllerModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[DetectControllersInput, DetectControllersOutput](
			"input_detect_controllers",
			"Scan for connected gamepad/controller devices. Returns brand (xbox/playstation/nintendo), capabilities, event path, and whether a makima profile exists.",
			func(_ context.Context, _ DetectControllersInput) (DetectControllersOutput, error) {
				controllers := parseInputDevices()
				if controllers == nil {
					controllers = []controllerInfo{}
				}
				return DetectControllersOutput{Controllers: controllers}, nil
			},
		),

		handler.TypedHandler[GenerateProfileInput, GenerateProfileOutput](
			"input_generate_controller_profile",
			"Generate a starter makima TOML profile for a controller with preset mappings (desktop, claude-code, gaming, media). Includes brand-aware button label comments.",
			func(_ context.Context, input GenerateProfileInput) (GenerateProfileOutput, error) {
				if input.DeviceName == "" {
					return GenerateProfileOutput{}, fmt.Errorf("[%s] device_name is required", handler.ErrInvalidParam)
				}
				if input.Template == "" {
					return GenerateProfileOutput{}, fmt.Errorf("[%s] template is required", handler.ErrInvalidParam)
				}

				templateStr, ok := controllerTemplates[input.Template]
				if !ok {
					return GenerateProfileOutput{}, fmt.Errorf("[%s] unknown template: %s", handler.ErrInvalidParam, input.Template)
				}

				brand := "generic"
				for _, c := range parseInputDevices() {
					if c.Name == input.DeviceName {
						brand = c.Brand
						break
					}
				}

				labels := brandLabels[brand]
				if labels == "" {
					labels = brandLabels["generic"]
				}

				content := fmt.Sprintf(templateStr, input.DeviceName, brand, labels)

				filename := input.DeviceName + ".toml"
				if input.AppID != "" {
					filename = input.DeviceName + "::" + input.AppID + ".toml"
				}

				path := filepath.Join(makimaDir(), filename)

				if !input.Force {
					if _, err := os.Stat(path); err == nil {
						return GenerateProfileOutput{}, fmt.Errorf("profile already exists: %s (use force=true to overwrite)", filename)
					}
				}

				var parsed map[string]any
				if _, err := toml.Decode(content, &parsed); err != nil {
					return GenerateProfileOutput{}, fmt.Errorf("generated invalid TOML: %w", err)
				}

				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					return GenerateProfileOutput{}, fmt.Errorf("write profile: %w", err)
				}

				lines := strings.Split(content, "\n")
				preview := content
				if len(lines) > 15 {
					preview = strings.Join(lines[:15], "\n") + "\n..."
				}

				return GenerateProfileOutput{
					Written:  path,
					Template: input.Template,
					Brand:    brand,
					Preview:  preview,
				}, nil
			},
		),

		handler.TypedHandler[ControllerTestInput, ControllerTestOutput](
			"input_controller_test",
			"Read events from a controller for 3 seconds to verify it works. Returns last 10 button/axis events.",
			func(_ context.Context, input ControllerTestInput) (ControllerTestOutput, error) {
				if input.DeviceName == "" && input.EventPath == "" {
					return ControllerTestOutput{}, fmt.Errorf("[%s] device_name or event_path is required", handler.ErrInvalidParam)
				}

				eventPath := input.EventPath
				if eventPath == "" {
					for _, c := range parseInputDevices() {
						if c.Name == input.DeviceName {
							eventPath = c.EventPath
							break
						}
					}
					if eventPath == "" {
						return ControllerTestOutput{}, fmt.Errorf("[%s] controller not found: %s", handler.ErrNotFound, input.DeviceName)
					}
				}

				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				cmd := exec.CommandContext(ctx, "evtest", eventPath)
				var out bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &out
				_ = cmd.Run()

				eventRe := regexp.MustCompile(`type (\d+) \((\w+)\), code (\d+) \((\w+)\), value (\S+)`)
				var events []controllerEvent
				for _, line := range strings.Split(out.String(), "\n") {
					em := eventRe.FindStringSubmatch(line)
					if em != nil {
						events = append(events, controllerEvent{
							Type:  em[2],
							Code:  em[4],
							Value: em[5],
						})
					}
				}

				if len(events) > 10 {
					events = events[len(events)-10:]
				}

				status := "ok"
				if len(events) == 0 {
					status = "no_events"
				}

				return ControllerTestOutput{
					EventPath: eventPath,
					Status:    status,
					Events:    events,
				}, nil
			},
		),
	}
}

// ── MidiModule ─────────────────────────────────────────────────────────────

type MidiModule struct{}

func (m *MidiModule) Name() string        { return "midi" }
func (m *MidiModule) Description() string { return "MIDI device detection and mapping management" }

func (m *MidiModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[MidiListInput, MidiListOutput](
			"midi_list_devices",
			"Detect connected USB MIDI controllers via ALSA. Returns card/device numbers, names, raw MIDI paths, and whether a mapping config exists in dotfiles.",
			func(_ context.Context, _ MidiListInput) (MidiListOutput, error) {
				devices := detectMidiDevices()
				if devices == nil {
					devices = []midiDevice{}
				}
				return MidiListOutput{Devices: devices}, nil
			},
		),

		handler.TypedHandler[MidiGenerateInput, MidiGenerateOutput](
			"midi_generate_mapping",
			"Generate a MIDI controller mapping config (TOML) with preset templates: desktop-control (volume/brightness/workspaces), shader-control (shader/wallpaper cycling), volume-mixer (per-sink audio).",
			func(_ context.Context, input MidiGenerateInput) (MidiGenerateOutput, error) {
				if input.DeviceName == "" {
					return MidiGenerateOutput{}, fmt.Errorf("[%s] device_name is required", handler.ErrInvalidParam)
				}
				if input.Template == "" {
					return MidiGenerateOutput{}, fmt.Errorf("[%s] template is required", handler.ErrInvalidParam)
				}

				templateStr, ok := midiTemplates[input.Template]
				if !ok {
					return MidiGenerateOutput{}, fmt.Errorf("[%s] unknown template: %s", handler.ErrInvalidParam, input.Template)
				}

				hw := "hw:0,0"
				for _, d := range detectMidiDevices() {
					if d.Name == input.DeviceName {
						hw = d.RawPath
						break
					}
				}

				content := fmt.Sprintf(templateStr, input.DeviceName, hw, input.DeviceName, hw)

				if err := os.MkdirAll(midiDir(), 0755); err != nil {
					return MidiGenerateOutput{}, fmt.Errorf("create midi dir: %w", err)
				}

				filename := input.DeviceName + ".toml"
				path := filepath.Join(midiDir(), filename)

				var parsed map[string]any
				if _, err := toml.Decode(content, &parsed); err != nil {
					return MidiGenerateOutput{}, fmt.Errorf("generated invalid TOML: %w", err)
				}

				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					return MidiGenerateOutput{}, fmt.Errorf("write mapping: %w", err)
				}

				lines := strings.Split(content, "\n")
				preview := content
				if len(lines) > 15 {
					preview = strings.Join(lines[:15], "\n") + "\n..."
				}

				return MidiGenerateOutput{
					Written:  path,
					Template: input.Template,
					Preview:  preview,
				}, nil
			},
		),

		handler.TypedHandler[MidiGetInput, MidiGetOutput](
			"midi_get_mapping",
			"Read an existing MIDI controller mapping config from dotfiles.",
			func(_ context.Context, input MidiGetInput) (MidiGetOutput, error) {
				if input.Name == "" {
					return MidiGetOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				name := input.Name
				if !strings.HasSuffix(name, ".toml") {
					name += ".toml"
				}
				data, err := os.ReadFile(filepath.Join(midiDir(), name))
				if err != nil {
					return MidiGetOutput{}, fmt.Errorf("[%s] read mapping: %w", handler.ErrNotFound, err)
				}
				return MidiGetOutput{Content: string(data)}, nil
			},
		),

		handler.TypedHandler[MidiSetInput, MidiSetOutput](
			"midi_set_mapping",
			"Create or update a MIDI controller mapping config. Validates TOML before writing.",
			func(_ context.Context, input MidiSetInput) (MidiSetOutput, error) {
				if input.Name == "" {
					return MidiSetOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				if input.Content == "" {
					return MidiSetOutput{}, fmt.Errorf("[%s] content is required", handler.ErrInvalidParam)
				}
				name := input.Name
				if !strings.HasSuffix(name, ".toml") {
					name += ".toml"
				}

				var parsed map[string]any
				if _, err := toml.Decode(input.Content, &parsed); err != nil {
					return MidiSetOutput{}, fmt.Errorf("[%s] invalid TOML: %w", handler.ErrInvalidParam, err)
				}

				if err := os.MkdirAll(midiDir(), 0755); err != nil {
					return MidiSetOutput{}, fmt.Errorf("create midi dir: %w", err)
				}

				path := filepath.Join(midiDir(), name)
				if err := os.WriteFile(path, []byte(input.Content), 0644); err != nil {
					return MidiSetOutput{}, fmt.Errorf("write mapping: %w", err)
				}

				return MidiSetOutput{Written: path, Valid: true}, nil
			},
		),
	}
}

// ── JuhradialModule ────────────────────────────────────────────────────────

type JuhradialModule struct{}

func (m *JuhradialModule) Name() string { return "juhradial" }
func (m *JuhradialModule) Description() string {
	return "MX Master 4 juhradial-mx config and battery management"
}

func (m *JuhradialModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[InputGetJuhradialConfigInput, InputGetJuhradialConfigOutput](
			"input_get_juhradial_config",
			"Read the tracked juhradial config.json from dotfiles.",
			func(_ context.Context, _ InputGetJuhradialConfigInput) (InputGetJuhradialConfigOutput, error) {
				data, err := os.ReadFile(juhradialConfigPath())
				if err != nil {
					return InputGetJuhradialConfigOutput{}, fmt.Errorf("[%s] read juhradial config: %w", handler.ErrNotFound, err)
				}
				return InputGetJuhradialConfigOutput{Content: string(data), Path: juhradialConfigPath()}, nil
			},
		),

		handler.TypedHandler[InputSetJuhradialConfigInput, InputSetJuhradialConfigOutput](
			"input_set_juhradial_config",
			"Write dotfiles juhradial config.json after validating that the content is valid JSON.",
			func(_ context.Context, input InputSetJuhradialConfigInput) (InputSetJuhradialConfigOutput, error) {
				if strings.TrimSpace(input.Content) == "" {
					return InputSetJuhradialConfigOutput{}, fmt.Errorf("[%s] content is required", handler.ErrInvalidParam)
				}
				if err := validateJSONDocument(input.Content); err != nil {
					return InputSetJuhradialConfigOutput{}, fmt.Errorf("[%s] invalid JSON: %w", handler.ErrInvalidParam, err)
				}
				if err := writeFileAtomic(juhradialConfigPath(), []byte(input.Content), 0644); err != nil {
					return InputSetJuhradialConfigOutput{}, fmt.Errorf("write juhradial config: %w", err)
				}
				return InputSetJuhradialConfigOutput{Written: juhradialConfigPath(), Valid: true}, nil
			},
		),

		handler.TypedHandler[InputGetJuhradialProfilesInput, InputGetJuhradialProfilesOutput](
			"input_get_juhradial_profiles",
			"Read the tracked juhradial profiles.json from dotfiles.",
			func(_ context.Context, _ InputGetJuhradialProfilesInput) (InputGetJuhradialProfilesOutput, error) {
				data, err := os.ReadFile(juhradialProfilesPath())
				if err != nil {
					return InputGetJuhradialProfilesOutput{}, fmt.Errorf("[%s] read juhradial profiles: %w", handler.ErrNotFound, err)
				}
				return InputGetJuhradialProfilesOutput{Content: string(data), Path: juhradialProfilesPath()}, nil
			},
		),

		handler.TypedHandler[InputSetJuhradialProfilesInput, InputSetJuhradialProfilesOutput](
			"input_set_juhradial_profiles",
			"Write dotfiles juhradial profiles.json after validating that the content is valid JSON.",
			func(_ context.Context, input InputSetJuhradialProfilesInput) (InputSetJuhradialProfilesOutput, error) {
				if strings.TrimSpace(input.Content) == "" {
					return InputSetJuhradialProfilesOutput{}, fmt.Errorf("[%s] content is required", handler.ErrInvalidParam)
				}
				if err := validateJSONDocument(input.Content); err != nil {
					return InputSetJuhradialProfilesOutput{}, fmt.Errorf("[%s] invalid JSON: %w", handler.ErrInvalidParam, err)
				}
				if err := writeFileAtomic(juhradialProfilesPath(), []byte(input.Content), 0644); err != nil {
					return InputSetJuhradialProfilesOutput{}, fmt.Errorf("write juhradial profiles: %w", err)
				}
				return InputSetJuhradialProfilesOutput{Written: juhradialProfilesPath(), Valid: true}, nil
			},
		),

		handler.TypedHandler[InputGetJuhradialBatteryInput, InputGetJuhradialBatteryOutput](
			"input_get_juhradial_battery",
			"Read MX battery status from the juhradial D-Bus daemon, falling back to bluetoothctl when D-Bus is unavailable.",
			func(_ context.Context, _ InputGetJuhradialBatteryInput) (InputGetJuhradialBatteryOutput, error) {
				status, err := juhradialBatteryStatus()
				if err != nil {
					return InputGetJuhradialBatteryOutput{}, fmt.Errorf("juhradial battery: %w", err)
				}
				return status, nil
			},
		),
	}
}

// ── WorkflowModule ────────────────────────────────────────────────────────

type WorkflowModule struct{}

func (m *WorkflowModule) Name() string { return "workflow" }
func (m *WorkflowModule) Description() string {
	return "Composed workflow tools for multi-step operations"
}

func (m *WorkflowModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[AutoSetupControllerInput, AutoSetupControllerOutput](
			"input_auto_setup_controller",
			"Detect all connected controllers, generate makima profiles for each, and optionally restart makima. Single tool replaces detect + generate + restart.",
			func(_ context.Context, input AutoSetupControllerInput) (AutoSetupControllerOutput, error) {
				template := input.Template
				if template == "" {
					template = "desktop"
				}
				templateStr, ok := controllerTemplates[template]
				if !ok {
					return AutoSetupControllerOutput{}, fmt.Errorf("[%s] unknown template: %s (valid: gaming, desktop, media)", handler.ErrInvalidParam, template)
				}

				controllers := parseInputDevices()
				if len(controllers) == 0 {
					return AutoSetupControllerOutput{
						DryRun:      !input.Execute,
						Controllers: []controllerSetupResult{},
					}, nil
				}

				var results []controllerSetupResult
				for _, c := range controllers {
					labels := brandLabels[c.Brand]
					if labels == "" {
						labels = brandLabels["generic"]
					}
					content := fmt.Sprintf(templateStr, c.Name, c.Brand, labels)
					filename := c.Name + ".toml"
					path := filepath.Join(makimaDir(), filename)

					lines := strings.Split(content, "\n")
					preview := content
					if len(lines) > 10 {
						preview = strings.Join(lines[:10], "\n") + "\n..."
					}

					r := controllerSetupResult{
						Name:    c.Name,
						Brand:   c.Brand,
						Profile: path,
						Preview: preview,
					}

					if !input.Execute {
						r.Written = false
						results = append(results, r)
						continue
					}

					// Check if profile already exists
					if _, err := os.Stat(path); err == nil {
						r.Skipped = true
						r.SkipNote = "profile already exists (not overwritten)"
						r.Written = false
						results = append(results, r)
						continue
					}

					// Validate TOML before writing
					var parsed map[string]any
					if _, err := toml.Decode(content, &parsed); err != nil {
						r.Skipped = true
						r.SkipNote = fmt.Sprintf("generated invalid TOML: %v", err)
						r.Written = false
						results = append(results, r)
						continue
					}

					if err := os.WriteFile(path, []byte(content), 0644); err != nil {
						r.Skipped = true
						r.SkipNote = fmt.Sprintf("write error: %v", err)
						r.Written = false
						results = append(results, r)
						continue
					}

					r.Written = true
					results = append(results, r)
				}

				result := AutoSetupControllerOutput{
					DryRun:      !input.Execute,
					Controllers: results,
				}

				// Restart mapitall if we actually wrote any profiles
				if input.Execute {
					wroteAny := false
					for _, r := range results {
						if r.Written {
							wroteAny = true
							break
						}
					}
					if wroteAny {
						_, stderr, err := inputRunCmd("sudo", "systemctl", "restart", "mapitall.service")
						if err != nil {
							result.MakimaRestart = fmt.Sprintf("restart failed: %s", stderr)
						} else {
							result.MakimaRestart = "restarted"
						}
					} else {
						result.MakimaRestart = "skipped (no new profiles written)"
					}
				}

				return result, nil
			},
		),

		handler.TypedHandler[BTDiscoverConnectInput, BTDiscoverConnectOutput](
			"bt_discover_and_connect",
			"Full BLE-safe workflow: scan, find device by name, remove stale bond, pair with agent, trust, connect with retry. Handles BLE re-pairing (MAC changes, auth failures).",
			func(_ context.Context, input BTDiscoverConnectInput) (BTDiscoverConnectOutput, error) {
				if input.DevicePattern == "" {
					return BTDiscoverConnectOutput{}, fmt.Errorf("[%s] device_pattern is required", handler.ErrInvalidParam)
				}

				autoTrust := true
				if input.AutoTrust != nil {
					autoTrust = *input.AutoTrust
				}

				timeout := input.Timeout
				if timeout <= 0 {
					timeout = 10
				}

				var steps []btDiscoverStep
				result := BTDiscoverConnectOutput{}

				// Step 1: Remove stale pairing if device was previously known
				// BLE devices get new MACs in pairing mode; stale bonds cause auth failures
				pattern := strings.ToLower(input.DevicePattern)
				existingDevices, _ := btListPaired()
				for _, d := range existingDevices {
					if strings.Contains(strings.ToLower(d.Name), pattern) {
						inputRunCmd("bluetoothctl", "remove", d.MAC)
						steps = append(steps, btDiscoverStep{
							Step:   "remove_stale",
							Status: "removed",
							Detail: fmt.Sprintf("cleared stale bond for %s (%s)", d.Name, d.MAC),
						})
						time.Sleep(time.Second)
						break
					}
				}

				// Step 2: Scan for devices
				cmd := exec.Command("bluetoothctl", "--timeout", fmt.Sprintf("%d", timeout), "scan", "on")
				if err := cmd.Start(); err != nil {
					return BTDiscoverConnectOutput{}, fmt.Errorf("start scan: %w", err)
				}
				time.Sleep(time.Duration(timeout) * time.Second)
				cmd.Process.Kill()
				cmd.Wait()

				steps = append(steps, btDiscoverStep{
					Step:   "scan",
					Status: "complete",
					Detail: fmt.Sprintf("scanned for %d seconds", timeout),
				})

				// Step 3: Find matching device
				out, _, _ := inputRunCmd("bluetoothctl", "devices")
				var matchMAC, matchName string
				for _, line := range strings.Split(out, "\n") {
					dm := deviceRe.FindStringSubmatch(strings.TrimSpace(line))
					if dm == nil {
						continue
					}
					if strings.Contains(strings.ToLower(dm[2]), pattern) {
						matchMAC = dm[1]
						matchName = dm[2]
						break
					}
				}

				if matchMAC == "" {
					steps = append(steps, btDiscoverStep{
						Step:   "find",
						Status: "not_found",
						Detail: fmt.Sprintf("no device matching %q found", input.DevicePattern),
					})
					result.Steps = steps
					return result, fmt.Errorf("[%s] no device matching %q found after %ds scan", handler.ErrNotFound, input.DevicePattern, timeout)
				}

				steps = append(steps, btDiscoverStep{
					Step:   "find",
					Status: "found",
					Detail: fmt.Sprintf("%s (%s)", matchName, matchMAC),
				})
				result.DeviceName = matchName
				result.MAC = matchMAC

				// Step 4: Pair using interactive agent (BLE-safe)
				pairOut, pairErr := btInteractivePair(matchMAC)
				if pairErr != nil {
					if strings.Contains(pairOut, "Already Exists") || strings.Contains(pairOut, "already exists") {
						steps = append(steps, btDiscoverStep{
							Step:   "pair",
							Status: "already_paired",
						})
					} else {
						steps = append(steps, btDiscoverStep{
							Step:   "pair",
							Status: "failed",
							Detail: strings.TrimSpace(pairOut),
						})
						result.Steps = steps
						return result, fmt.Errorf("pair failed: %w", pairErr)
					}
				} else {
					steps = append(steps, btDiscoverStep{
						Step:   "pair",
						Status: "paired",
					})
				}

				// Step 5: Trust (if auto_trust)
				if autoTrust {
					trustOut, trustStderr, trustErr := inputRunCmd("bluetoothctl", "trust", matchMAC)
					if trustErr != nil {
						steps = append(steps, btDiscoverStep{
							Step:   "trust",
							Status: "failed",
							Detail: strings.TrimSpace(trustOut + " " + trustStderr),
						})
					} else {
						steps = append(steps, btDiscoverStep{
							Step:   "trust",
							Status: "trusted",
						})
					}
				} else {
					steps = append(steps, btDiscoverStep{
						Step:   "trust",
						Status: "skipped",
					})
				}

				// Step 6: Connect with retry (BLE connections are flaky)
				connOut, connErr := btInteractiveConnect(matchMAC, 3)
				if connErr != nil {
					steps = append(steps, btDiscoverStep{
						Step:   "connect",
						Status: "failed",
						Detail: connOut,
					})
					result.Steps = steps
					return result, connErr
				}

				steps = append(steps, btDiscoverStep{
					Step:   "connect",
					Status: "connected",
				})

				result.Steps = steps
				result.Connected = true
				return result, nil
			},
		),
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------
