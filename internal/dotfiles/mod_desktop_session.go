package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type DesktopSessionModule struct{}

func (m *DesktopSessionModule) Name() string { return "desktop_session" }
func (m *DesktopSessionModule) Description() string {
	return "Desktop session handles for live Wayland sessions and optional KWin virtual backends"
}

type desktopSessionRecord struct {
	ID                        string                 `json:"id"`
	Name                      string                 `json:"name"`
	Backend                   string                 `json:"backend"`
	Status                    string                 `json:"status"`
	PID                       int                    `json:"pid,omitempty"`
	WaylandDisplay            string                 `json:"wayland_display,omitempty"`
	XDGRuntimeDir             string                 `json:"xdg_runtime_dir,omitempty"`
	HyprlandInstanceSignature string                 `json:"hyprland_instance_signature,omitempty"`
	LogPath                   string                 `json:"log_path,omitempty"`
	StartedAt                 string                 `json:"started_at"`
	StoppedAt                 string                 `json:"stopped_at,omitempty"`
	Notes                     []string               `json:"notes,omitempty"`
	AppLogs                   []desktopSessionAppLog `json:"app_logs,omitempty"`
}

type desktopSessionAppLog struct {
	App       string `json:"app"`
	Path      string `json:"path"`
	PID       int    `json:"pid,omitempty"`
	StartedAt string `json:"started_at"`
}

type SessionStartInput struct {
	Name    string `json:"name,omitempty" jsonschema:"description=Optional session name"`
	Backend string `json:"backend,omitempty" jsonschema:"description=Backend to use,enum=kwin_virtual,enum=live"`
}

type SessionConnectInput struct {
	SessionID                 string `json:"session_id,omitempty" jsonschema:"description=Optional existing session handle to rehydrate"`
	Name                      string `json:"name,omitempty" jsonschema:"description=Optional friendly name for a live session handle"`
	WaylandDisplay            string `json:"wayland_display,omitempty" jsonschema:"description=Explicit WAYLAND_DISPLAY override"`
	XDGRuntimeDir             string `json:"xdg_runtime_dir,omitempty" jsonschema:"description=Explicit XDG_RUNTIME_DIR override"`
	HyprlandInstanceSignature string `json:"hyprland_instance_signature,omitempty" jsonschema:"description=Explicit HYPRLAND_INSTANCE_SIGNATURE override"`
}

type SessionRefInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Session handle to use. Defaults to the newest saved session."`
}

type SessionScreenshotInput struct {
	SessionID  string `json:"session_id,omitempty" jsonschema:"description=Session handle to use. Defaults to the newest saved session."`
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=Explicit PNG path. Defaults to the session state directory."`
}

type SessionLaunchInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Session handle to use. Defaults to the newest saved session."`
	App       string `json:"app" jsonschema:"required,description=Command to launch inside the session"`
}

type SessionWindowInput struct {
	SessionID     string `json:"session_id,omitempty" jsonschema:"description=Session handle to use. Defaults to the newest saved session."`
	Address       string `json:"address,omitempty" jsonschema:"description=Hyprland window address when using a Hyprland-backed session"`
	Class         string `json:"class,omitempty" jsonschema:"description=Window class when using a Hyprland-backed session"`
	TitleContains string `json:"title_contains,omitempty" jsonschema:"description=Window title substring when using a Hyprland-backed session"`
}

type SessionClipboardSetInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Session handle to use. Defaults to the newest saved session."`
	Text      string `json:"text" jsonschema:"required,description=Text to place on the session clipboard"`
	MimeType  string `json:"mime_type,omitempty" jsonschema:"description=Optional MIME type, defaults to text/plain"`
}

type SessionLogInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Session handle to use. Defaults to the newest saved session."`
	App       string `json:"app,omitempty" jsonschema:"description=Optional app substring to pick a specific launch log"`
	Lines     int    `json:"lines,omitempty" jsonschema:"description=Tail line count (default 80)"`
}

type SessionWindowsOutput struct {
	Session     desktopSessionRecord `json:"session"`
	Mode        string               `json:"mode"`
	Count       int                  `json:"count"`
	Windows     []hyprClient         `json:"windows,omitempty"`
	Unsupported string               `json:"unsupported,omitempty"`
}

type SessionScreenshotOutput struct {
	Session    desktopSessionRecord `json:"session"`
	OutputPath string               `json:"output_path"`
	Bytes      int64                `json:"bytes"`
}

type SessionClipboardOutput struct {
	Session desktopSessionRecord `json:"session"`
	Text    string               `json:"text"`
}

type SessionLogOutput struct {
	Session desktopSessionRecord `json:"session"`
	App     string               `json:"app,omitempty"`
	Path    string               `json:"path,omitempty"`
	Lines   int                  `json:"lines"`
	Output  string               `json:"output,omitempty"`
}

type SessionCommandOutput struct {
	Session desktopSessionRecord `json:"session"`
	Output  string               `json:"output,omitempty"`
	Path    string               `json:"path,omitempty"`
	Mode    string               `json:"mode,omitempty"`
}

func desktopSessionsRootDir() string {
	return dotfilesManagedStateDir("sessions")
}

func ensureDesktopSessionDir(id string) (string, error) {
	return ensureDotfilesManagedStateDir("sessions", id)
}

func desktopSessionRecordPath(id string) string {
	return filepath.Join(desktopSessionsRootDir(), id+".json")
}

func desktopSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}

func saveDesktopSessionRecord(record desktopSessionRecord) error {
	if _, err := ensureDotfilesManagedStateDir("sessions"); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session record: %w", err)
	}
	if err := os.WriteFile(desktopSessionRecordPath(record.ID), data, 0o644); err != nil {
		return fmt.Errorf("write session record: %w", err)
	}
	return nil
}

func loadDesktopSessionRecord(id string) (desktopSessionRecord, error) {
	data, err := os.ReadFile(desktopSessionRecordPath(id))
	if err != nil {
		return desktopSessionRecord{}, fmt.Errorf("read session record %s: %w", id, err)
	}
	var record desktopSessionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return desktopSessionRecord{}, fmt.Errorf("parse session record %s: %w", id, err)
	}
	return record, nil
}

func listDesktopSessionRecords() ([]desktopSessionRecord, error) {
	if !pathExists(desktopSessionsRootDir()) {
		return nil, nil
	}
	entries, err := os.ReadDir(desktopSessionsRootDir())
	if err != nil {
		return nil, fmt.Errorf("read session dir: %w", err)
	}
	var records []desktopSessionRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		record, err := loadDesktopSessionRecord(id)
		if err == nil {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt > records[j].StartedAt
	})
	return records, nil
}

func resolveDesktopSession(id string) (desktopSessionRecord, error) {
	if strings.TrimSpace(id) != "" {
		return loadDesktopSessionRecord(id)
	}
	records, err := listDesktopSessionRecords()
	if err != nil {
		return desktopSessionRecord{}, err
	}
	if len(records) == 0 {
		return desktopSessionRecord{}, fmt.Errorf("no desktop sessions found")
	}
	return records[0], nil
}

func desktopSessionEnv(record desktopSessionRecord) []string {
	env := os.Environ()
	setEnv := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		prefix := key + "="
		replaced := false
		for i := range env {
			if strings.HasPrefix(env[i], prefix) {
				env[i] = prefix + value
				replaced = true
			}
		}
		if !replaced {
			env = append(env, prefix+value)
		}
	}

	setEnv("XDG_RUNTIME_DIR", record.XDGRuntimeDir)
	setEnv("WAYLAND_DISPLAY", record.WaylandDisplay)
	setEnv("HYPRLAND_INSTANCE_SIGNATURE", record.HyprlandInstanceSignature)
	setEnv("XDG_SESSION_TYPE", "wayland")
	return env
}

func desktopSessionAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func runDesktopSessionCommand(record desktopSessionRecord, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = desktopSessionEnv(record)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return trimmed, fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, trimmed)
	}
	return trimmed, nil
}

func runDesktopSessionHyprctl(record desktopSessionRecord, args ...string) (string, error) {
	if strings.TrimSpace(record.HyprlandInstanceSignature) == "" {
		return "", fmt.Errorf("session %s is not Hyprland-backed", record.ID)
	}
	return runDesktopSessionCommand(record, "hyprctl", args...)
}

func sessionDefaultName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, time.Now().Format("20060102-150405"))
}

func sanitizeSessionLogName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "app"
	}
	var out []rune
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= '0' && r <= '9':
			out = append(out, r)
		case r == '-', r == '_':
			out = append(out, r)
		case r == ' ', r == '/', r == ':', r == '.':
			out = append(out, '-')
		}
	}
	slug := strings.Trim(strings.ReplaceAll(string(out), "--", "-"), "-")
	if slug == "" {
		return "app"
	}
	return slug
}

func latestSessionAppLog(record desktopSessionRecord, app string) *desktopSessionAppLog {
	for i := len(record.AppLogs) - 1; i >= 0; i-- {
		entry := record.AppLogs[i]
		if app == "" || strings.Contains(strings.ToLower(entry.App), strings.ToLower(app)) {
			return &entry
		}
	}
	return nil
}

func sessionFindHyprWindow(record desktopSessionRecord, input SessionWindowInput) (string, error) {
	clientsJSON, err := runDesktopSessionHyprctl(record, "clients", "-j")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(input.TitleContains) != "" {
		var clients []hyprClient
		if err := json.Unmarshal([]byte(clientsJSON), &clients); err != nil {
			return "", fmt.Errorf("parse Hyprland clients: %w", err)
		}
		for _, client := range clients {
			if strings.Contains(strings.ToLower(client.Title), strings.ToLower(input.TitleContains)) {
				return "address:" + client.Address, nil
			}
		}
		return "", fmt.Errorf("no window title matched %q", input.TitleContains)
	}
	return resolveHyprWindow(input.Address, input.Class, clientsJSON)
}

func (m *DesktopSessionModule) Tools() []registry.ToolDefinition {
	start := handler.TypedHandler[SessionStartInput, desktopSessionRecord](
		"session_start",
		"Start and persist a desktop session handle. Supports a live handle or an optional KWin virtual backend.",
		func(_ context.Context, input SessionStartInput) (desktopSessionRecord, error) {
			backend := strings.TrimSpace(strings.ToLower(input.Backend))
			if backend == "" {
				backend = "kwin_virtual"
			}
			if backend == "live" {
				connectInput := SessionConnectInput{Name: input.Name}
				return sessionConnect(connectInput)
			}
			if backend != "kwin_virtual" {
				return desktopSessionRecord{}, fmt.Errorf("[%s] unsupported backend %q", handler.ErrInvalidParam, backend)
			}
			if !hasCmd("kwin_wayland") {
				return desktopSessionRecord{}, fmt.Errorf("kwin_wayland not found")
			}

			id := desktopSessionID()
			name := strings.TrimSpace(input.Name)
			if name == "" {
				name = sessionDefaultName("kwin-virtual")
			}
			dir, err := ensureDesktopSessionDir(id)
			if err != nil {
				return desktopSessionRecord{}, err
			}
			runtimeDir := filepath.Join(dir, "runtime")
			if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
				return desktopSessionRecord{}, fmt.Errorf("create session runtime dir: %w", err)
			}
			logPath := filepath.Join(dir, "kwin.log")
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return desktopSessionRecord{}, fmt.Errorf("open session log: %w", err)
			}
			defer logFile.Close()

			record := desktopSessionRecord{
				ID:             id,
				Name:           name,
				Backend:        backend,
				Status:         "starting",
				WaylandDisplay: "wayland-0",
				XDGRuntimeDir:  runtimeDir,
				LogPath:        logPath,
				StartedAt:      time.Now().UTC().Format(time.RFC3339),
				Notes:          []string{"best-effort KWin virtual session start"},
			}

			cmd := exec.Command("kwin_wayland", "--virtual", "--no-lockscreen")
			cmd.Env = desktopSessionEnv(record)
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			if err := cmd.Start(); err != nil {
				return desktopSessionRecord{}, fmt.Errorf("start kwin_wayland: %w", err)
			}

			record.PID = cmd.Process.Pid
			if err := saveDesktopSessionRecord(record); err != nil {
				return desktopSessionRecord{}, err
			}
			return record, nil
		},
	)
	start.Category = "desktop"
	start.SearchTerms = []string{"session start", "kwin virtual", "desktop session", "wayland session"}

	connect := handler.TypedHandler[SessionConnectInput, desktopSessionRecord](
		"session_connect",
		"Create or rehydrate a session handle for the current live Wayland desktop.",
		func(_ context.Context, input SessionConnectInput) (desktopSessionRecord, error) {
			return sessionConnect(input)
		},
	)
	connect.Category = "desktop"
	connect.SearchTerms = []string{"connect session", "live wayland session", "session handle"}

	stop := handler.TypedHandler[SessionRefInput, desktopSessionRecord](
		"session_stop",
		"Stop a tracked desktop session. Live handles are marked stopped without killing the current compositor.",
		func(_ context.Context, input SessionRefInput) (desktopSessionRecord, error) {
			record, err := resolveDesktopSession(input.SessionID)
			if err != nil {
				return desktopSessionRecord{}, err
			}
			if record.PID > 0 && desktopSessionAlive(record.PID) {
				if err := syscall.Kill(record.PID, syscall.SIGTERM); err != nil {
					return desktopSessionRecord{}, fmt.Errorf("stop session %s: %w", record.ID, err)
				}
			}
			record.Status = "stopped"
			record.StoppedAt = time.Now().UTC().Format(time.RFC3339)
			if err := saveDesktopSessionRecord(record); err != nil {
				return desktopSessionRecord{}, err
			}
			return record, nil
		},
	)
	stop.Category = "desktop"
	stop.SearchTerms = []string{"stop session", "kill virtual session", "close session"}

	screenshot := handler.TypedHandler[SessionScreenshotInput, SessionScreenshotOutput](
		"session_screenshot",
		"Take a screenshot inside a tracked session using grim.",
		func(_ context.Context, input SessionScreenshotInput) (SessionScreenshotOutput, error) {
			record, err := resolveDesktopSession(input.SessionID)
			if err != nil {
				return SessionScreenshotOutput{}, err
			}
			if !hasCmd("grim") {
				return SessionScreenshotOutput{}, fmt.Errorf("grim not found")
			}

			dir, err := ensureDesktopSessionDir(record.ID)
			if err != nil {
				return SessionScreenshotOutput{}, err
			}
			outputPath := strings.TrimSpace(input.OutputPath)
			if outputPath == "" {
				outputPath = filepath.Join(dir, "screenshot-"+time.Now().UTC().Format("20060102-150405")+".png")
			}
			if _, err := runDesktopSessionCommand(record, "grim", outputPath); err != nil {
				return SessionScreenshotOutput{}, err
			}
			info, err := os.Stat(outputPath)
			if err != nil {
				return SessionScreenshotOutput{}, fmt.Errorf("stat screenshot: %w", err)
			}
			return SessionScreenshotOutput{
				Session:    record,
				OutputPath: outputPath,
				Bytes:      info.Size(),
			}, nil
		},
	)
	screenshot.Category = "desktop"
	screenshot.SearchTerms = []string{"session screenshot", "grim screenshot", "virtual session screenshot"}

	listWindows := handler.TypedHandler[SessionRefInput, SessionWindowsOutput](
		"session_list_windows",
		"List windows for a Hyprland-backed session. Returns an unsupported note for non-Hyprland sessions.",
		func(_ context.Context, input SessionRefInput) (SessionWindowsOutput, error) {
			record, err := resolveDesktopSession(input.SessionID)
			if err != nil {
				return SessionWindowsOutput{}, err
			}
			if strings.TrimSpace(record.HyprlandInstanceSignature) == "" {
				return SessionWindowsOutput{
					Session:     record,
					Mode:        "unsupported",
					Unsupported: "window inventory currently supports Hyprland-backed sessions only",
				}, nil
			}
			clientsJSON, err := runDesktopSessionHyprctl(record, "clients", "-j")
			if err != nil {
				return SessionWindowsOutput{}, err
			}
			var windows []hyprClient
			if err := json.Unmarshal([]byte(clientsJSON), &windows); err != nil {
				return SessionWindowsOutput{}, fmt.Errorf("parse Hyprland clients: %w", err)
			}
			return SessionWindowsOutput{
				Session: record,
				Mode:    "hyprctl",
				Count:   len(windows),
				Windows: windows,
			}, nil
		},
	)
	listWindows.Category = "desktop"
	listWindows.SearchTerms = []string{"session windows", "list windows in session", "hypr session clients"}

	focusWindow := handler.TypedHandler[SessionWindowInput, SessionCommandOutput](
		"session_focus_window",
		"Focus a window inside a Hyprland-backed session by address, class, or title substring.",
		func(_ context.Context, input SessionWindowInput) (SessionCommandOutput, error) {
			record, err := resolveDesktopSession(input.SessionID)
			if err != nil {
				return SessionCommandOutput{}, err
			}
			if strings.TrimSpace(record.HyprlandInstanceSignature) == "" {
				return SessionCommandOutput{
					Session: record,
					Mode:    "unsupported",
					Output:  "focus is currently supported for Hyprland-backed sessions only",
				}, nil
			}
			selector, err := sessionFindHyprWindow(record, input)
			if err != nil {
				return SessionCommandOutput{}, err
			}
			out, err := runDesktopSessionHyprctl(record, "dispatch", "focuswindow", selector)
			if err != nil {
				return SessionCommandOutput{}, err
			}
			return SessionCommandOutput{
				Session: record,
				Mode:    "hyprctl",
				Output:  strings.TrimSpace(out),
			}, nil
		},
	)
	focusWindow.Category = "desktop"
	focusWindow.SearchTerms = []string{"focus session window", "focus by class", "focus by title"}

	launchApp := handler.TypedHandler[SessionLaunchInput, SessionCommandOutput](
		"session_launch_app",
		"Launch an application inside a tracked session and persist a launch log.",
		func(_ context.Context, input SessionLaunchInput) (SessionCommandOutput, error) {
			record, err := resolveDesktopSession(input.SessionID)
			if err != nil {
				return SessionCommandOutput{}, err
			}
			if strings.TrimSpace(input.App) == "" {
				return SessionCommandOutput{}, fmt.Errorf("[%s] app is required", handler.ErrInvalidParam)
			}
			dir, err := ensureDesktopSessionDir(record.ID)
			if err != nil {
				return SessionCommandOutput{}, err
			}
			logDir := filepath.Join(dir, "logs")
			if err := os.MkdirAll(logDir, 0o755); err != nil {
				return SessionCommandOutput{}, fmt.Errorf("create session log dir: %w", err)
			}
			logPath := filepath.Join(logDir, sanitizeSessionLogName(input.App)+"-"+time.Now().UTC().Format("20060102-150405")+".log")
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return SessionCommandOutput{}, fmt.Errorf("open app log: %w", err)
			}
			defer logFile.Close()

			cmd := exec.Command("sh", "-lc", input.App)
			cmd.Env = desktopSessionEnv(record)
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			if err := cmd.Start(); err != nil {
				return SessionCommandOutput{}, fmt.Errorf("launch app: %w", err)
			}

			record.AppLogs = append(record.AppLogs, desktopSessionAppLog{
				App:       input.App,
				Path:      logPath,
				PID:       cmd.Process.Pid,
				StartedAt: time.Now().UTC().Format(time.RFC3339),
			})
			if err := saveDesktopSessionRecord(record); err != nil {
				return SessionCommandOutput{}, err
			}

			return SessionCommandOutput{
				Session: record,
				Path:    logPath,
				Output:  fmt.Sprintf("launched %q with pid %d", input.App, cmd.Process.Pid),
			}, nil
		},
	)
	launchApp.Category = "desktop"
	launchApp.SearchTerms = []string{"launch app in session", "session exec", "virtual session app"}

	clipboardGet := handler.TypedHandler[SessionRefInput, SessionClipboardOutput](
		"session_clipboard_get",
		"Read the clipboard for a tracked session using wl-paste.",
		func(_ context.Context, input SessionRefInput) (SessionClipboardOutput, error) {
			record, err := resolveDesktopSession(input.SessionID)
			if err != nil {
				return SessionClipboardOutput{}, err
			}
			if !hasCmd("wl-paste") {
				return SessionClipboardOutput{}, fmt.Errorf("wl-paste not found")
			}
			out, err := runDesktopSessionCommand(record, "wl-paste", "--no-newline")
			if err != nil {
				return SessionClipboardOutput{}, err
			}
			return SessionClipboardOutput{Session: record, Text: out}, nil
		},
	)
	clipboardGet.Category = "desktop"
	clipboardGet.SearchTerms = []string{"session clipboard read", "wl-paste session", "clipboard get"}

	clipboardSet := handler.TypedHandler[SessionClipboardSetInput, SessionCommandOutput](
		"session_clipboard_set",
		"Write text into a tracked session clipboard using wl-copy.",
		func(_ context.Context, input SessionClipboardSetInput) (SessionCommandOutput, error) {
			record, err := resolveDesktopSession(input.SessionID)
			if err != nil {
				return SessionCommandOutput{}, err
			}
			if !hasCmd("wl-copy") {
				return SessionCommandOutput{}, fmt.Errorf("wl-copy not found")
			}
			if strings.TrimSpace(input.Text) == "" {
				return SessionCommandOutput{}, fmt.Errorf("[%s] text is required", handler.ErrInvalidParam)
			}
			mimeType := strings.TrimSpace(input.MimeType)
			if mimeType == "" {
				mimeType = "text/plain"
			}
			cmd := exec.Command("wl-copy", "--type", mimeType)
			cmd.Env = desktopSessionEnv(record)
			cmd.Stdin = strings.NewReader(input.Text)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return SessionCommandOutput{}, fmt.Errorf("wl-copy failed: %w: %s", err, strings.TrimSpace(string(out)))
			}
			return SessionCommandOutput{
				Session: record,
				Output:  fmt.Sprintf("copied %d bytes as %s", len(input.Text), mimeType),
			}, nil
		},
	)
	clipboardSet.Category = "desktop"
	clipboardSet.SearchTerms = []string{"session clipboard write", "wl-copy session", "clipboard set"}

	waylandInfo := handler.TypedHandler[SessionRefInput, SessionCommandOutput](
		"session_wayland_info",
		"Read wayland-info for a tracked session.",
		func(_ context.Context, input SessionRefInput) (SessionCommandOutput, error) {
			record, err := resolveDesktopSession(input.SessionID)
			if err != nil {
				return SessionCommandOutput{}, err
			}
			if !hasCmd("wayland-info") {
				return SessionCommandOutput{}, fmt.Errorf("wayland-info not found")
			}
			out, err := runDesktopSessionCommand(record, "wayland-info")
			if err != nil {
				return SessionCommandOutput{}, err
			}
			return SessionCommandOutput{Session: record, Output: out}, nil
		},
	)
	waylandInfo.Category = "desktop"
	waylandInfo.SearchTerms = []string{"wayland info", "session wayland", "wayland capabilities"}

	readAppLog := handler.TypedHandler[SessionLogInput, SessionLogOutput](
		"session_read_app_log",
		"Read the most recent launch log for a tracked session, optionally filtered by app substring.",
		func(_ context.Context, input SessionLogInput) (SessionLogOutput, error) {
			record, err := resolveDesktopSession(input.SessionID)
			if err != nil {
				return SessionLogOutput{}, err
			}
			lines := input.Lines
			if lines <= 0 {
				lines = 80
			}
			entry := latestSessionAppLog(record, input.App)
			if entry == nil {
				return SessionLogOutput{Session: record, Lines: lines}, nil
			}
			data, err := os.ReadFile(entry.Path)
			if err != nil {
				return SessionLogOutput{}, fmt.Errorf("read app log: %w", err)
			}
			return SessionLogOutput{
				Session: record,
				App:     entry.App,
				Path:    entry.Path,
				Lines:   lines,
				Output:  trimTailLines(string(data), lines),
			}, nil
		},
	)
	readAppLog.Category = "desktop"
	readAppLog.SearchTerms = []string{"session app log", "launch log", "read session log"}

	return []registry.ToolDefinition{
		start,
		connect,
		stop,
		screenshot,
		listWindows,
		focusWindow,
		launchApp,
		clipboardGet,
		clipboardSet,
		waylandInfo,
		readAppLog,
	}
}

func sessionConnect(input SessionConnectInput) (desktopSessionRecord, error) {
	if strings.TrimSpace(input.SessionID) != "" {
		record, err := loadDesktopSessionRecord(input.SessionID)
		if err != nil {
			return desktopSessionRecord{}, err
		}
		if record.Status == "" {
			record.Status = "connected"
			if err := saveDesktopSessionRecord(record); err != nil {
				return desktopSessionRecord{}, err
			}
		}
		return record, nil
	}

	runtimeDir := strings.TrimSpace(input.XDGRuntimeDir)
	if runtimeDir == "" {
		runtimeDir = dotfilesRuntimeDir()
	}
	waylandDisplay := strings.TrimSpace(input.WaylandDisplay)
	if waylandDisplay == "" {
		waylandDisplay = dotfilesWaylandDisplay(runtimeDir)
	}
	hyprSignature := strings.TrimSpace(input.HyprlandInstanceSignature)
	if hyprSignature == "" {
		hyprSignature = dotfilesHyprlandSignature(runtimeDir)
	}
	if strings.TrimSpace(waylandDisplay) == "" {
		return desktopSessionRecord{}, fmt.Errorf("unable to resolve WAYLAND_DISPLAY for live session")
	}

	id := desktopSessionID()
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = sessionDefaultName("live")
	}
	record := desktopSessionRecord{
		ID:                        id,
		Name:                      name,
		Backend:                   "live_wayland",
		Status:                    "connected",
		WaylandDisplay:            waylandDisplay,
		XDGRuntimeDir:             runtimeDir,
		HyprlandInstanceSignature: hyprSignature,
		StartedAt:                 time.Now().UTC().Format(time.RFC3339),
	}
	if strings.TrimSpace(hyprSignature) != "" {
		record.Backend = "live_hyprland"
		record.Notes = append(record.Notes, "Hyprland live session")
	}
	if err := saveDesktopSessionRecord(record); err != nil {
		return desktopSessionRecord{}, err
	}
	return record, nil
}
