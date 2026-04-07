// mod_tmux.go — tmux session, window, and pane management tools
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// tmuxRunCmd executes a command and returns stdout and stderr separately.
func tmuxRunCmd(name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimRight(stdout.String(), "\n"), strings.TrimRight(stderr.String(), "\n"), err
}

// tmuxRun executes a tmux subcommand and returns stdout or a wrapped error.
func tmuxRun(args ...string) (string, error) {
	stdout, stderr, err := tmuxRunCmd("tmux", args...)
	if err != nil {
		return "", fmt.Errorf("tmux %s: %s: %w", strings.Join(args, " "), stderr, err)
	}
	return stdout, nil
}

// tmuxIsNoServer returns true if the error indicates tmux has no running server.
func tmuxIsNoServer(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no server running") ||
		strings.Contains(msg, "no current client") ||
		strings.Contains(msg, "error connecting")
}

// tmuxTarget builds a tmux target string like "session:window.pane".
func tmuxTarget(session, window, pane string) string {
	t := session
	if window != "" {
		t += ":" + window
	}
	if pane != "" {
		t += "." + pane
	}
	return t
}

// ---------------------------------------------------------------------------
// I/O types
// ---------------------------------------------------------------------------

// --- list_sessions ---

type tmuxListSessionsInput struct{}

type tmuxSessionInfo struct {
	Name     string `json:"name"`
	Windows  int    `json:"windows"`
	Created  string `json:"created"`
	Attached bool   `json:"attached"`
}

type tmuxListSessionsOutput struct {
	Sessions []tmuxSessionInfo `json:"sessions"`
}

// --- list_windows ---

type tmuxListWindowsInput struct {
	Session string `json:"session" jsonschema:"required,description=Tmux session name"`
}

type tmuxWindowInfo struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Panes  int    `json:"panes"`
	Active bool   `json:"active"`
	Layout string `json:"layout"`
}

type tmuxListWindowsOutput struct {
	Session string           `json:"session"`
	Windows []tmuxWindowInfo `json:"windows"`
}

// --- list_panes ---

type tmuxListPanesInput struct {
	Session string `json:"session" jsonschema:"required,description=Tmux session name"`
	Window  string `json:"window,omitempty" jsonschema:"description=Window index or name"`
}

type tmuxPaneInfo struct {
	Index   int    `json:"index"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Command string `json:"command"`
	Active  bool   `json:"active"`
}

type tmuxListPanesOutput struct {
	Target string         `json:"target"`
	Panes  []tmuxPaneInfo `json:"panes"`
}

// --- capture_pane ---

type tmuxCaptureInput struct {
	Session string `json:"session" jsonschema:"required,description=Tmux session name"`
	Window  string `json:"window,omitempty" jsonschema:"description=Window index or name"`
	Pane    string `json:"pane,omitempty" jsonschema:"description=Pane index"`
	Lines   int    `json:"lines,omitempty" jsonschema:"description=Number of lines to capture. Default 50."`
}

type tmuxCaptureOutput struct {
	Target  string `json:"target"`
	Content string `json:"content"`
}

// --- send_keys ---

type tmuxSendKeysInput struct {
	Session string `json:"session" jsonschema:"required,description=Tmux session name"`
	Window  string `json:"window,omitempty" jsonschema:"description=Window index or name"`
	Pane    string `json:"pane,omitempty" jsonschema:"description=Pane index"`
	Keys    string `json:"keys" jsonschema:"required,description=Keys to send. Literal text or tmux key names like Enter or C-c."`
}

type tmuxSendKeysOutput struct {
	Target string `json:"target"`
	Sent   string `json:"sent"`
}

// --- new_session ---

type tmuxNewSessionInput struct {
	Name      string `json:"name" jsonschema:"required,description=Session name"`
	Command   string `json:"command,omitempty" jsonschema:"description=Initial command to run"`
	Directory string `json:"directory,omitempty" jsonschema:"description=Working directory for the session"`
}

type tmuxNewSessionOutput struct {
	Session string `json:"session"`
	Created bool   `json:"created"`
}

// --- new_window ---

type tmuxNewWindowInput struct {
	Session string `json:"session" jsonschema:"required,description=Tmux session name"`
	Name    string `json:"name,omitempty" jsonschema:"description=Window name"`
	Command string `json:"command,omitempty" jsonschema:"description=Command to run in the new window"`
}

type tmuxNewWindowOutput struct {
	Session string `json:"session"`
	Window  string `json:"window"`
}

// --- kill_session ---

type tmuxKillSessionInput struct {
	Name string `json:"name" jsonschema:"required,description=Session name to kill"`
}

type tmuxKillSessionOutput struct {
	Session string `json:"session"`
	Killed  bool   `json:"killed"`
}

// --- wait_for_text ---

type tmuxWaitForTextInput struct {
	Session     string `json:"session" jsonschema:"required,description=tmux session name"`
	WindowIndex int    `json:"window_index,omitempty" jsonschema:"description=window index (default 0)"`
	PaneIndex   int    `json:"pane_index,omitempty" jsonschema:"description=pane index (default 0)"`
	Text        string `json:"text" jsonschema:"required,description=substring to wait for"`
	TimeoutSecs int    `json:"timeout_secs,omitempty" jsonschema:"description=timeout in seconds (default 30)"`
}

type tmuxWaitForTextOutput struct {
	Found        bool   `json:"found"`
	MatchingLine string `json:"matching_line"`
	ElapsedMs    int64  `json:"elapsed_ms"`
}

// --- search_panes ---

type tmuxSearchPanesInput struct {
	Session string `json:"session" jsonschema:"required,description=tmux session name"`
	Pattern string `json:"pattern" jsonschema:"required,description=regex pattern to search for"`
}

type tmuxPaneMatch struct {
	WindowIndex int      `json:"window_index"`
	WindowName  string   `json:"window_name"`
	PaneIndex   int      `json:"pane_index"`
	Lines       []string `json:"lines"`
}

type tmuxSearchPanesOutput struct {
	Session string          `json:"session"`
	Pattern string          `json:"pattern"`
	Matches []tmuxPaneMatch `json:"matches"`
}

// --- workspace (composed) ---

type tmuxWorkspacePane struct {
	Command string `json:"command,omitempty" jsonschema:"description=Command to run in this pane"`
	Dir     string `json:"dir,omitempty" jsonschema:"description=Working directory for this pane"`
}

type tmuxWorkspaceWindow struct {
	Name   string              `json:"name,omitempty" jsonschema:"description=Window name"`
	Layout string              `json:"layout,omitempty" jsonschema:"description=Tmux layout: tiled\\, even-horizontal\\, even-vertical\\, main-horizontal\\, main-vertical"`
	Panes  []tmuxWorkspacePane `json:"panes,omitempty" jsonschema:"description=Panes in this window"`
}

type tmuxWorkspaceInput struct {
	Session string                `json:"session" jsonschema:"required,description=Session name"`
	Dir     string                `json:"dir,omitempty" jsonschema:"description=Base working directory for all panes"`
	Windows []tmuxWorkspaceWindow `json:"windows" jsonschema:"required,description=Windows to create"`
}

type tmuxWorkspaceOutput struct {
	Session     string `json:"session"`
	WindowCount int    `json:"windows_created"`
	PaneCount   int    `json:"panes_created"`
}

// ---------------------------------------------------------------------------
// TmuxModule
// ---------------------------------------------------------------------------

// TmuxModule provides tmux session, window, and pane management tools.
type TmuxModule struct{}

func (m *TmuxModule) Name() string { return "tmux" }
func (m *TmuxModule) Description() string {
	return "tmux session management: sessions, windows, panes, capture, send-keys"
}

func (m *TmuxModule) Tools() []registry.ToolDefinition {
	// ---------------------------------------------------------------
	// tmux_list_sessions — read-only
	// ---------------------------------------------------------------
	listSessions := handler.TypedHandler[tmuxListSessionsInput, tmuxListSessionsOutput](
		"tmux_list_sessions",
		"List all tmux sessions with window count, creation time, and attached status.",
		func(_ context.Context, _ tmuxListSessionsInput) (tmuxListSessionsOutput, error) {
			out, err := tmuxRun("list-sessions", "-F",
				"#{session_name}\t#{session_windows}\t#{session_created}\t#{session_attached}")
			if err != nil {
				if tmuxIsNoServer(err) {
					return tmuxListSessionsOutput{Sessions: []tmuxSessionInfo{}}, nil
				}
				return tmuxListSessionsOutput{}, fmt.Errorf("[%s] list sessions: %w", handler.ErrAPIError, err)
			}

			var sessions []tmuxSessionInfo
			for line := range strings.SplitSeq(out, "\n") {
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "\t", 4)
				if len(parts) < 4 {
					continue
				}
				windows, _ := strconv.Atoi(parts[1])
				attached := parts[3] == "1"
				sessions = append(sessions, tmuxSessionInfo{
					Name:     parts[0],
					Windows:  windows,
					Created:  parts[2],
					Attached: attached,
				})
			}
			if sessions == nil {
				sessions = []tmuxSessionInfo{}
			}
			return tmuxListSessionsOutput{Sessions: sessions}, nil
		},
	)
	listSessions.IsWrite = false

	// ---------------------------------------------------------------
	// tmux_list_windows — read-only
	// ---------------------------------------------------------------
	listWindows := handler.TypedHandler[tmuxListWindowsInput, tmuxListWindowsOutput](
		"tmux_list_windows",
		"List windows in a tmux session with index, name, pane count, active status, and layout.",
		func(_ context.Context, input tmuxListWindowsInput) (tmuxListWindowsOutput, error) {
			out, err := tmuxRun("list-windows", "-t", input.Session, "-F",
				"#{window_index}\t#{window_name}\t#{window_panes}\t#{window_active}\t#{window_layout}")
			if err != nil {
				if tmuxIsNoServer(err) {
					return tmuxListWindowsOutput{Session: input.Session, Windows: []tmuxWindowInfo{}}, nil
				}
				return tmuxListWindowsOutput{}, fmt.Errorf("[%s] list windows: %w", handler.ErrNotFound, err)
			}

			var windows []tmuxWindowInfo
			for line := range strings.SplitSeq(out, "\n") {
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "\t", 5)
				if len(parts) < 5 {
					continue
				}
				idx, _ := strconv.Atoi(parts[0])
				panes, _ := strconv.Atoi(parts[2])
				active := parts[3] == "1"
				windows = append(windows, tmuxWindowInfo{
					Index:  idx,
					Name:   parts[1],
					Panes:  panes,
					Active: active,
					Layout: parts[4],
				})
			}
			if windows == nil {
				windows = []tmuxWindowInfo{}
			}
			return tmuxListWindowsOutput{
				Session: input.Session,
				Windows: windows,
			}, nil
		},
	)
	listWindows.IsWrite = false

	// ---------------------------------------------------------------
	// tmux_list_panes — read-only
	// ---------------------------------------------------------------
	listPanes := handler.TypedHandler[tmuxListPanesInput, tmuxListPanesOutput](
		"tmux_list_panes",
		"List panes in a session or window with index, dimensions, current command, and active status.",
		func(_ context.Context, input tmuxListPanesInput) (tmuxListPanesOutput, error) {
			target := tmuxTarget(input.Session, input.Window, "")
			out, err := tmuxRun("list-panes", "-t", target, "-F",
				"#{pane_index}\t#{pane_width}\t#{pane_height}\t#{pane_current_command}\t#{pane_active}")
			if err != nil {
				if tmuxIsNoServer(err) {
					return tmuxListPanesOutput{Target: target, Panes: []tmuxPaneInfo{}}, nil
				}
				return tmuxListPanesOutput{}, fmt.Errorf("[%s] list panes: %w", handler.ErrNotFound, err)
			}

			var panes []tmuxPaneInfo
			for line := range strings.SplitSeq(out, "\n") {
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "\t", 5)
				if len(parts) < 5 {
					continue
				}
				idx, _ := strconv.Atoi(parts[0])
				width, _ := strconv.Atoi(parts[1])
				height, _ := strconv.Atoi(parts[2])
				active := parts[4] == "1"
				panes = append(panes, tmuxPaneInfo{
					Index:   idx,
					Width:   width,
					Height:  height,
					Command: parts[3],
					Active:  active,
				})
			}
			if panes == nil {
				panes = []tmuxPaneInfo{}
			}
			return tmuxListPanesOutput{
				Target: target,
				Panes:  panes,
			}, nil
		},
	)
	listPanes.IsWrite = false

	// ---------------------------------------------------------------
	// tmux_capture_pane — read-only
	// ---------------------------------------------------------------
	capturePane := handler.TypedHandler[tmuxCaptureInput, tmuxCaptureOutput](
		"tmux_capture_pane",
		"Capture visible text content from a tmux pane. Default 50 lines of scrollback.",
		func(_ context.Context, input tmuxCaptureInput) (tmuxCaptureOutput, error) {
			target := tmuxTarget(input.Session, input.Window, input.Pane)
			lines := input.Lines
			if lines <= 0 {
				lines = 50
			}
			out, err := tmuxRun("capture-pane", "-p", "-t", target,
				"-S", fmt.Sprintf("-%d", lines))
			if err != nil {
				return tmuxCaptureOutput{}, fmt.Errorf("[%s] capture pane: %w", handler.ErrNotFound, err)
			}
			return tmuxCaptureOutput{
				Target:  target,
				Content: out,
			}, nil
		},
	)
	capturePane.IsWrite = false
	capturePane.SearchTerms = []string{"scrollback", "pane output", "terminal output", "copy pane"}
	capturePane.MaxResultChars = 8000

	// ---------------------------------------------------------------
	// tmux_send_keys — write
	// ---------------------------------------------------------------
	sendKeys := handler.TypedHandler[tmuxSendKeysInput, tmuxSendKeysOutput](
		"tmux_send_keys",
		"Send keystrokes to a tmux pane. Keys can be literal text or tmux key names like Enter, C-c, etc.",
		func(_ context.Context, input tmuxSendKeysInput) (tmuxSendKeysOutput, error) {
			target := tmuxTarget(input.Session, input.Window, input.Pane)
			slog.Info("sending keys", "target", target, "keys", input.Keys)
			_, err := tmuxRun("send-keys", "-t", target, input.Keys)
			if err != nil {
				slog.Error("send keys failed", "target", target, "error", err)
				return tmuxSendKeysOutput{}, fmt.Errorf("[%s] send keys: %w", handler.ErrNotFound, err)
			}
			return tmuxSendKeysOutput{
				Target: target,
				Sent:   input.Keys,
			}, nil
		},
	)
	sendKeys.IsWrite = true

	// ---------------------------------------------------------------
	// tmux_new_session — write
	// ---------------------------------------------------------------
	newSession := handler.TypedHandler[tmuxNewSessionInput, tmuxNewSessionOutput](
		"tmux_new_session",
		"Create a new detached tmux session with optional working directory and initial command.",
		func(_ context.Context, input tmuxNewSessionInput) (tmuxNewSessionOutput, error) {
			slog.Info("creating session", "session", input.Name, "directory", input.Directory)
			args := []string{"new-session", "-d", "-s", input.Name}
			if input.Directory != "" {
				args = append(args, "-c", input.Directory)
			}
			if input.Command != "" {
				args = append(args, input.Command)
			}
			_, err := tmuxRun(args...)
			if err != nil {
				slog.Error("session creation failed", "session", input.Name, "error", err)
				return tmuxNewSessionOutput{}, fmt.Errorf("[%s] new session: %w", handler.ErrAPIError, err)
			}
			slog.Info("session created", "session", input.Name)
			return tmuxNewSessionOutput{
				Session: input.Name,
				Created: true,
			}, nil
		},
	)
	newSession.IsWrite = true

	// ---------------------------------------------------------------
	// tmux_new_window — write
	// ---------------------------------------------------------------
	newWindow := handler.TypedHandler[tmuxNewWindowInput, tmuxNewWindowOutput](
		"tmux_new_window",
		"Create a new window in an existing tmux session with optional name and command.",
		func(_ context.Context, input tmuxNewWindowInput) (tmuxNewWindowOutput, error) {
			args := []string{"new-window", "-t", input.Session, "-P", "-F", "#{window_index}"}
			if input.Name != "" {
				args = append(args, "-n", input.Name)
			}
			if input.Command != "" {
				args = append(args, input.Command)
			}
			out, err := tmuxRun(args...)
			if err != nil {
				return tmuxNewWindowOutput{}, fmt.Errorf("[%s] new window: %w", handler.ErrNotFound, err)
			}
			return tmuxNewWindowOutput{
				Session: input.Session,
				Window:  out,
			}, nil
		},
	)
	newWindow.IsWrite = true

	// ---------------------------------------------------------------
	// tmux_kill_session — write, complex
	// ---------------------------------------------------------------
	killSession := handler.TypedHandler[tmuxKillSessionInput, tmuxKillSessionOutput](
		"tmux_kill_session",
		"Kill a tmux session by name.",
		func(_ context.Context, input tmuxKillSessionInput) (tmuxKillSessionOutput, error) {
			slog.Info("killing session", "session", input.Name)
			_, err := tmuxRun("kill-session", "-t", input.Name)
			if err != nil {
				slog.Error("session kill failed", "session", input.Name, "error", err)
				return tmuxKillSessionOutput{}, fmt.Errorf("[%s] kill session: %w", handler.ErrNotFound, err)
			}
			slog.Info("session killed", "session", input.Name)
			return tmuxKillSessionOutput{
				Session: input.Name,
				Killed:  true,
			}, nil
		},
	)
	killSession.IsWrite = true
	killSession.Complexity = registry.ComplexityComplex

	// ---------------------------------------------------------------
	// tmux_wait_for_text — read-only polling
	// ---------------------------------------------------------------
	waitForText := handler.TypedHandler[tmuxWaitForTextInput, tmuxWaitForTextOutput](
		"tmux_wait_for_text",
		"Poll a tmux pane until specific text appears. Returns the matching line or times out.",
		func(ctx context.Context, input tmuxWaitForTextInput) (tmuxWaitForTextOutput, error) {
			if input.Text == "" {
				return tmuxWaitForTextOutput{}, fmt.Errorf("[%s] text is required", handler.ErrInvalidParam)
			}

			timeout := input.TimeoutSecs
			if timeout <= 0 {
				timeout = 30
			}

			// Build target: only include window/pane if explicitly set (non-zero)
			winStr := ""
			if input.WindowIndex != 0 {
				winStr = strconv.Itoa(input.WindowIndex)
			}
			paneStr := ""
			if input.PaneIndex != 0 {
				paneStr = strconv.Itoa(input.PaneIndex)
			}
			target := tmuxTarget(input.Session, winStr, paneStr)

			deadline := time.Now().Add(time.Duration(timeout) * time.Second)
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			start := time.Now()

			for {
				out, err := tmuxRun("capture-pane", "-p", "-t", target)
				if err == nil {
					for line := range strings.SplitSeq(out, "\n") {
						if strings.Contains(line, input.Text) {
							return tmuxWaitForTextOutput{
								Found:        true,
								MatchingLine: line,
								ElapsedMs:    time.Since(start).Milliseconds(),
							}, nil
						}
					}
				}

				if time.Now().After(deadline) {
					return tmuxWaitForTextOutput{}, fmt.Errorf(
						"[%s] text %q not found in %s after %ds",
						handler.ErrAPIError, input.Text, target, timeout,
					)
				}

				select {
				case <-ctx.Done():
					return tmuxWaitForTextOutput{}, fmt.Errorf("[%s] context cancelled: %w", handler.ErrAPIError, ctx.Err())
				case <-ticker.C:
				}
			}
		},
	)
	waitForText.IsWrite = false

	// ---------------------------------------------------------------
	// tmux_search_panes — read-only search
	// ---------------------------------------------------------------
	searchPanes := handler.TypedHandler[tmuxSearchPanesInput, tmuxSearchPanesOutput](
		"tmux_search_panes",
		"Search across all panes in a session for a regex pattern. Returns matching lines with window/pane indices.",
		func(_ context.Context, input tmuxSearchPanesInput) (tmuxSearchPanesOutput, error) {
			if input.Session == "" {
				return tmuxSearchPanesOutput{}, fmt.Errorf("[%s] session is required", handler.ErrInvalidParam)
			}

			re, err := regexp.Compile(input.Pattern)
			if err != nil {
				return tmuxSearchPanesOutput{}, fmt.Errorf("[%s] invalid regex pattern: %w", handler.ErrInvalidParam, err)
			}

			// List all windows in the session
			winOut, err := tmuxRun("list-windows", "-t", input.Session, "-F",
				"#{window_index}\t#{window_name}")
			if err != nil {
				if tmuxIsNoServer(err) {
					return tmuxSearchPanesOutput{Session: input.Session, Pattern: input.Pattern, Matches: []tmuxPaneMatch{}}, nil
				}
				return tmuxSearchPanesOutput{}, fmt.Errorf("[%s] list windows: %w", handler.ErrNotFound, err)
			}

			var matches []tmuxPaneMatch

			for winLine := range strings.SplitSeq(winOut, "\n") {
				if winLine == "" {
					continue
				}
				winParts := strings.SplitN(winLine, "\t", 2)
				if len(winParts) < 2 {
					continue
				}
				winIdx, _ := strconv.Atoi(winParts[0])
				winName := winParts[1]

				// List panes in this window
				paneOut, err := tmuxRun("list-panes", "-t",
					fmt.Sprintf("%s:%d", input.Session, winIdx), "-F", "#{pane_index}")
				if err != nil {
					continue
				}

				for paneLine := range strings.SplitSeq(paneOut, "\n") {
					if paneLine == "" {
						continue
					}
					paneIdx, _ := strconv.Atoi(strings.TrimSpace(paneLine))

					// Capture pane content
					target := fmt.Sprintf("%s:%d.%d", input.Session, winIdx, paneIdx)
					content, err := tmuxRun("capture-pane", "-p", "-t", target)
					if err != nil {
						continue
					}

					var matchedLines []string
					for line := range strings.SplitSeq(content, "\n") {
						if re.MatchString(line) {
							matchedLines = append(matchedLines, line)
						}
					}

					if len(matchedLines) > 0 {
						matches = append(matches, tmuxPaneMatch{
							WindowIndex: winIdx,
							WindowName:  winName,
							PaneIndex:   paneIdx,
							Lines:       matchedLines,
						})
					}
				}
			}

			if matches == nil {
				matches = []tmuxPaneMatch{}
			}

			return tmuxSearchPanesOutput{
				Session: input.Session,
				Pattern: input.Pattern,
				Matches: matches,
			}, nil
		},
	)
	searchPanes.IsWrite = false
	searchPanes.SearchTerms = []string{"grep panes", "find in panes", "search session output"}

	// ---------------------------------------------------------------
	// tmux_workspace (composed) — write
	// ---------------------------------------------------------------
	workspace := handler.TypedHandler[tmuxWorkspaceInput, tmuxWorkspaceOutput](
		"tmux_workspace",
		"Create a multi-window, multi-pane tmux workspace from a declarative spec. Single tool replaces new_session + new_window + send_keys sequences.",
		func(_ context.Context, input tmuxWorkspaceInput) (tmuxWorkspaceOutput, error) {
			if input.Session == "" {
				return tmuxWorkspaceOutput{}, fmt.Errorf("[%s] session name is required", handler.ErrInvalidParam)
			}
			if len(input.Windows) == 0 {
				return tmuxWorkspaceOutput{}, fmt.Errorf("[%s] at least one window is required", handler.ErrInvalidParam)
			}

			slog.Info("creating workspace", "session", input.Session, "windows", len(input.Windows))

			baseDir := input.Dir
			totalPanes := 0

			// Create session with first window's first pane
			firstWin := input.Windows[0]
			sessionArgs := []string{"new-session", "-d", "-s", input.Session}
			if firstWin.Name != "" {
				sessionArgs = append(sessionArgs, "-n", firstWin.Name)
			}

			// Determine directory for first pane
			firstDir := baseDir
			if len(firstWin.Panes) > 0 && firstWin.Panes[0].Dir != "" {
				firstDir = firstWin.Panes[0].Dir
			}
			if firstDir != "" {
				sessionArgs = append(sessionArgs, "-c", firstDir)
			}

			_, err := tmuxRun(sessionArgs...)
			if err != nil {
				return tmuxWorkspaceOutput{}, fmt.Errorf("[%s] create session: %w", handler.ErrAPIError, err)
			}
			totalPanes++

			// Send command to first pane if specified
			if len(firstWin.Panes) > 0 && firstWin.Panes[0].Command != "" {
				tmuxRun("send-keys", "-t", input.Session, firstWin.Panes[0].Command, "Enter")
			}

			// Add remaining panes to first window
			for i := 1; i < len(firstWin.Panes); i++ {
				pane := firstWin.Panes[i]
				splitArgs := []string{"split-window", "-t", input.Session}
				paneDir := pane.Dir
				if paneDir == "" {
					paneDir = baseDir
				}
				if paneDir != "" {
					splitArgs = append(splitArgs, "-c", paneDir)
				}
				tmuxRun(splitArgs...)
				totalPanes++

				if pane.Command != "" {
					tmuxRun("send-keys", "-t", input.Session, pane.Command, "Enter")
				}
			}

			// Apply layout to first window
			if firstWin.Layout != "" {
				tmuxRun("select-layout", "-t", input.Session, firstWin.Layout)
			}

			// Create additional windows
			for w := 1; w < len(input.Windows); w++ {
				win := input.Windows[w]
				winArgs := []string{"new-window", "-t", input.Session}
				if win.Name != "" {
					winArgs = append(winArgs, "-n", win.Name)
				}

				// First pane directory
				winDir := baseDir
				if len(win.Panes) > 0 && win.Panes[0].Dir != "" {
					winDir = win.Panes[0].Dir
				}
				if winDir != "" {
					winArgs = append(winArgs, "-c", winDir)
				}

				tmuxRun(winArgs...)
				totalPanes++

				// Send command to first pane
				if len(win.Panes) > 0 && win.Panes[0].Command != "" {
					target := input.Session + ":" + strconv.Itoa(w)
					tmuxRun("send-keys", "-t", target, win.Panes[0].Command, "Enter")
				}

				// Additional panes
				for i := 1; i < len(win.Panes); i++ {
					pane := win.Panes[i]
					splitArgs := []string{"split-window", "-t", input.Session + ":" + strconv.Itoa(w)}
					paneDir := pane.Dir
					if paneDir == "" {
						paneDir = baseDir
					}
					if paneDir != "" {
						splitArgs = append(splitArgs, "-c", paneDir)
					}
					tmuxRun(splitArgs...)
					totalPanes++

					if pane.Command != "" {
						target := input.Session + ":" + strconv.Itoa(w)
						tmuxRun("send-keys", "-t", target, pane.Command, "Enter")
					}
				}

				// Apply layout
				if win.Layout != "" {
					tmuxRun("select-layout", "-t", input.Session+":"+strconv.Itoa(w), win.Layout)
				}
			}

			// Select first window
			tmuxRun("select-window", "-t", input.Session+":0")

			slog.Info("workspace created", "session", input.Session, "windows", len(input.Windows), "panes", totalPanes)
			return tmuxWorkspaceOutput{
				Session:     input.Session,
				WindowCount: len(input.Windows),
				PaneCount:   totalPanes,
			}, nil
		},
	)
	workspace.IsWrite = true

	return []registry.ToolDefinition{
		listSessions,
		listWindows,
		listPanes,
		capturePane,
		sendKeys,
		newSession,
		newWindow,
		killSession,
		waitForText,
		searchPanes,
		workspace,
	}
}
