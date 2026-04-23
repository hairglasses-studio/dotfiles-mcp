package dotfiles

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func dotfilesIronbarConfigDir() string {
	homeConfig := filepath.Join(homeDir(), ".config", "ironbar")
	if pathExists(filepath.Join(homeConfig, "config.toml")) {
		return homeConfig
	}
	return filepath.Join(dotfilesDir(), "ironbar")
}

func systemdUserUnitActive(name string) bool {
	if exec.Command("systemctl", "--user", "--quiet", "is-active", name).Run() == nil {
		return true
	}
	runtime := dotfilesResolveDesktopRuntime()
	if machine := dotfilesRuntimeSystemdMachine(runtime.XDGRuntimeDir); machine != "" {
		return exec.Command("systemctl", "--user", "--machine="+machine, "--quiet", "is-active", name).Run() == nil
	}
	return false
}

func hyprLayerBindings(namespaces ...string) []EwwLayerInfo {
	cmd := exec.Command("hyprctl", "layers")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}

	allowed := map[string]bool{}
	for _, namespace := range namespaces {
		namespace = strings.TrimSpace(namespace)
		if namespace != "" {
			allowed[namespace] = true
		}
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
		if len(allowed) > 0 && !allowed[namespace] {
			continue
		}
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

func ironbarLayerBindings() []EwwLayerInfo {
	return hyprLayerBindings("ironbar")
}

type ironbarRuntimeStatus struct {
	BinaryAvailable bool
	ConfigPath      string
	ConfigPresent   bool
	ServiceActive   bool
	Running         bool
	ProcessCount    int
	Layers          []EwwLayerInfo
	IPCSocket       string
	IPCReady        bool
}

func currentIronbarStatus() ironbarRuntimeStatus {
	status := ironbarRuntimeStatus{
		BinaryAvailable: hasCmd("ironbar"),
		ConfigPath:      dotfilesIronbarConfigDir(),
	}
	status.ConfigPresent = pathExists(filepath.Join(status.ConfigPath, "config.toml"))
	status.ServiceActive = systemdUserUnitActive("ironbar.service")

	countOut, err := exec.Command("pgrep", "-c", "ironbar").CombinedOutput()
	if err == nil {
		_, _ = fmt.Sscanf(strings.TrimSpace(string(countOut)), "%d", &status.ProcessCount)
	}
	status.Running = status.ProcessCount > 0 || status.ServiceActive || processRunningExact("ironbar")
	status.Layers = ironbarLayerBindings()

	if runtimeDir := dotfilesRuntimeDir(); runtimeDir != "" {
		status.IPCSocket = filepath.Join(runtimeDir, "ironbar-ipc.sock")
		status.IPCReady = pathExists(status.IPCSocket)
	}

	return status
}
