package dotfiles

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type EwwRestartInput struct{}

type EwwRestartOutput struct {
	Killed    int      `json:"killed"`
	WaybarOff bool     `json:"waybar_killed"`
	DaemonPID string   `json:"daemon_pid,omitempty"`
	BarsOpen  []string `json:"bars_opened,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type EwwStatusInput struct{}

type EwwStatusOutput struct {
	DaemonRunning  bool              `json:"daemon_running"`
	DaemonCount    int               `json:"daemon_count"`
	WaybarRunning  bool              `json:"waybar_running"`
	Windows        []string          `json:"windows"`
	DefinedWindows []string          `json:"defined_windows,omitempty"`
	Layers         []EwwLayerInfo    `json:"layers"`
	Variables      map[string]string `json:"variables,omitempty"`
}

type EwwGetInput struct {
	Variable string `json:"variable" jsonschema:"required,description=eww variable name to query"`
}

type EwwGetOutput struct {
	Variable string `json:"variable"`
	Value    any    `json:"value"`
}

func dotfilesEwwConfigDir() string {
	homeConfig := filepath.Join(homeDir(), ".config", "eww")
	if pathExists(homeConfig) {
		return homeConfig
	}
	return filepath.Join(dotfilesDir(), "eww")
}

func ewwCmd(args ...string) *exec.Cmd {
	baseArgs := []string{"-c", dotfilesEwwConfigDir()}
	baseArgs = append(baseArgs, args...)
	return exec.Command("eww", baseArgs...)
}

func runEww(args ...string) (string, error) {
	cmd := ewwCmd(args...)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return trimmed, fmt.Errorf("eww %s failed: %w: %s", strings.Join(args, " "), err, trimmed)
	}
	return trimmed, nil
}

func ewwDefinedWindows() ([]string, error) {
	out, err := runEww("list-windows")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var windows []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			windows = append(windows, line)
		}
	}
	return uniqueSortedStrings(windows), nil
}

func ewwActiveWindows() ([]string, error) {
	out, err := runEww("active-windows")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var windows []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			windows = append(windows, strings.TrimSpace(parts[1]))
			continue
		}
		windows = append(windows, line)
	}
	return uniqueSortedStrings(windows), nil
}

func ewwStateSnapshot() (map[string]any, string, error) {
	out, err := runEww("state")
	if err != nil {
		return nil, out, err
	}
	if out == "" {
		return map[string]any{}, out, nil
	}
	var state map[string]any
	if json.Unmarshal([]byte(out), &state) == nil {
		return state, out, nil
	}
	return map[string]any{"_raw": out}, out, nil
}

func ewwLayerBindings() []EwwLayerInfo {
	return hyprLayerBindings()
}

func currentEwwStatus() EwwStatusOutput {
	status := EwwStatusOutput{
		Variables: make(map[string]string),
	}

	if processRunningExact("eww") {
		status.DaemonRunning = true
	}
	countOut, err := exec.Command("pgrep", "-c", "eww").CombinedOutput()
	if err == nil {
		_, _ = fmt.Sscanf(strings.TrimSpace(string(countOut)), "%d", &status.DaemonCount)
		status.DaemonRunning = status.DaemonCount > 0
	}
	status.WaybarRunning = processRunningExact("waybar")

	if active, err := ewwActiveWindows(); err == nil {
		status.Windows = active
	}
	if defined, err := ewwDefinedWindows(); err == nil {
		status.DefinedWindows = defined
	}
	status.Layers = ewwLayerBindings()

	varsToCheck := []string{
		"bar_workspaces_dp1",
		"bar_workspaces_dp2",
		"bar_cpu",
		"bar_mem",
		"bar_vol",
		"bar_shader",
	}
	for _, variable := range varsToCheck {
		value, err := runEww("get", variable)
		if err == nil {
			status.Variables[variable] = value
		}
	}
	return status
}

func restartEwwBars() EwwRestartOutput {
	result := EwwRestartOutput{}

	if exec.Command("killall", "waybar").Run() == nil {
		result.WaybarOff = true
	}

	countOut, err := exec.Command("pgrep", "-c", "eww").CombinedOutput()
	if err == nil {
		_, _ = fmt.Sscanf(strings.TrimSpace(string(countOut)), "%d", &result.Killed)
	}

	_ = exec.Command("killall", "-9", "eww").Run()
	time.Sleep(300 * time.Millisecond)

	socketMatches, _ := filepath.Glob(filepath.Join(dotfilesRuntimeDir(), "eww-server_*"))
	for _, match := range socketMatches {
		_ = os.Remove(match)
	}

	daemon := ewwCmd("daemon", "--restart")
	daemon.Dir = homeDir()
	if err := daemon.Start(); err != nil {
		result.Error = fmt.Sprintf("daemon start failed: %v", err)
		return result
	}

	time.Sleep(1500 * time.Millisecond)

	if _, err := runEww("ping"); err != nil {
		result.Error = "daemon started but not responding to ping"
		return result
	}

	pidOut, err := exec.Command("pgrep", "-o", "eww").CombinedOutput()
	if err == nil {
		result.DaemonPID = strings.TrimSpace(string(pidOut))
	}

	for _, window := range []string{"sidebar"} {
		if _, err := runEww("open", window); err == nil {
			result.BarsOpen = append(result.BarsOpen, window)
		}
	}
	return result
}
