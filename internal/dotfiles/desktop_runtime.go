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

func xdgStateHome() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); dir != "" {
		return dir
	}
	return filepath.Join(homeDir(), ".local", "state")
}

func dotfilesManagedStateDir(parts ...string) string {
	base := []string{xdgStateHome(), "dotfiles", "desktop-control"}
	base = append(base, parts...)
	return filepath.Join(base...)
}

func ensureDotfilesManagedStateDir(parts ...string) (string, error) {
	dir := dotfilesManagedStateDir(parts...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create state dir %s: %w", dir, err)
	}
	return dir, nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func processRunningExact(name string) bool {
	return exec.Command("pgrep", "-x", name).Run() == nil
}

func processRunningPattern(pattern string) bool {
	return exec.Command("pgrep", "-f", pattern).Run() == nil
}

func desktopMonitorIncludePath() string {
	return filepath.Join(xdgStateHome(), "hypr", "monitors.dynamic.conf")
}

func dotfilesNotificationHistoryDir() string {
	return dotfilesManagedStateDir("notifications")
}

func dotfilesNotificationHistoryLogPath() string {
	return filepath.Join(dotfilesNotificationHistoryDir(), "history.jsonl")
}

func dotfilesNotificationHistoryListenerPath() string {
	return filepath.Join(dotfilesDir(), "scripts", "notification-history-listener.py")
}

func notificationHistoryListenerRunning() bool {
	return processRunningPattern("notification-history-listener.py")
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
		if line == "" {
			continue
		}
		windows = append(windows, line)
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
	return map[string]any{
		"_raw": out,
	}, out, nil
}

func ewwLayerBindings() []EwwLayerInfo {
	cmd := exec.Command("hyprctl", "layers")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}

	var layers []EwwLayerInfo
	var currentMonitor string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Monitor ") {
			currentMonitor = strings.TrimSuffix(strings.TrimPrefix(line, "Monitor "), ":")
			continue
		}
		if !strings.Contains(line, "namespace:") {
			continue
		}
		parts := strings.Split(line, "namespace: ")
		if len(parts) != 2 {
			continue
		}
		namespace := strings.Split(parts[1], ",")[0]
		position := ""
		if xywhIdx := strings.Index(line, "xywh: "); xywhIdx >= 0 {
			position = strings.Split(line[xywhIdx+6:], ",")[0]
		}
		layers = append(layers, EwwLayerInfo{
			Monitor:   currentMonitor,
			Namespace: namespace,
			Position:  position,
		})
	}
	return layers
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
		fmt.Sscanf(strings.TrimSpace(string(countOut)), "%d", &status.DaemonCount)
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
		"bar_workspaces_dp1", "bar_workspaces_dp2",
		"bar_cpu", "bar_mem", "bar_vol", "bar_shader",
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
		fmt.Sscanf(strings.TrimSpace(string(countOut)), "%d", &result.Killed)
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
