package dotfiles

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/dotfiles-mcp/internal/remediation"
	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type HyprGetOptionInput struct {
	Option string `json:"option" jsonschema:"required,description=Hyprland option name such as general:gaps_in or decoration:rounding"`
}

type HyprKeywordInput struct {
	Keyword string `json:"keyword" jsonschema:"required,description=Hyprland keyword name to update"`
	Value   string `json:"value" jsonschema:"required,description=Value to assign to the keyword"`
}

type HyprDispatchInput struct {
	Dispatcher string `json:"dispatcher" jsonschema:"required,description=Dispatcher name such as exec workspace focuswindow movetoworkspace"`
	Argument   string `json:"argument,omitempty" jsonschema:"description=Optional dispatcher argument payload"`
}

type HyprNotifyInput struct {
	Icon       int    `json:"icon,omitempty" jsonschema:"description=Hyprland notification icon id. Defaults to 1."`
	DurationMS int    `json:"duration_ms,omitempty" jsonschema:"description=Notification duration in milliseconds. Defaults to 3000."`
	Color      string `json:"color,omitempty" jsonschema:"description=Notification color such as rgb(ff00ff) or rgba(ff00ffff). Defaults to rgb(33cc99)."`
	Message    string `json:"message" jsonschema:"required,description=Notification message text"`
}

type HyprSetCursorInput struct {
	Theme string `json:"theme" jsonschema:"required,description=Cursor theme name"`
	Size  int    `json:"size" jsonschema:"required,description=Cursor size in pixels"`
}

type HyprSetPropInput struct {
	Address  string `json:"address,omitempty" jsonschema:"description=Window address from hypr_list_windows"`
	Class    string `json:"class,omitempty" jsonschema:"description=Window class name. First case-insensitive match is used."`
	Property string `json:"property" jsonschema:"required,description=Property name to update"`
	Value    string `json:"value" jsonschema:"required,description=Property value"`
	Lock     bool   `json:"lock,omitempty" jsonschema:"description=Lock the property against client updates"`
}

type HyprGetPropInput struct {
	Address  string `json:"address,omitempty" jsonschema:"description=Window address from hypr_list_windows"`
	Class    string `json:"class,omitempty" jsonschema:"description=Window class name. First case-insensitive match is used."`
	Property string `json:"property" jsonschema:"required,description=Property name to read"`
}

type HyprSwitchXKBLayoutInput struct {
	Device  string `json:"device" jsonschema:"required,description=Keyboard device name from hypr_list_devices"`
	Command string `json:"command" jsonschema:"required,description=Layout command such as next prev 0 1"`
}

type HyprSubcommandInput struct {
	Action string   `json:"action" jsonschema:"required,description=Subcommand or action name"`
	Args   []string `json:"args,omitempty" jsonschema:"description=Additional arguments to append after the action"`
}

type HyprRollingLogInput struct {
	Lines int `json:"lines,omitempty" jsonschema:"description=Maximum number of recent lines to return. Defaults to 100."`
}

type HyprEventCaptureInput struct {
	Event     string `json:"event,omitempty" jsonschema:"description=Optional event name filter"`
	Count     int    `json:"count,omitempty" jsonschema:"description=Maximum number of matching events to capture before returning. Defaults to 5."`
	TimeoutMS int    `json:"timeout_ms,omitempty" jsonschema:"description=How long to wait for events before returning. Defaults to 5000ms."`
}

type HyprWaitEventInput struct {
	Event     string `json:"event" jsonschema:"required,description=Exact Hyprland event name to wait for"`
	TimeoutMS int    `json:"timeout_ms,omitempty" jsonschema:"description=How long to wait for the event before timing out. Defaults to 10000ms."`
}

type hyprConfigErrorsOutput struct {
	Healthy     bool                     `json:"healthy"`
	Messages    []string                 `json:"messages,omitempty"`
	Raw         string                   `json:"raw,omitempty"`
	ErrorCode   string                   `json:"error_code,omitempty"`
	Remediation *remediation.Remediation `json:"remediation,omitempty"`
}

type hyprEventRecord struct {
	Name string `json:"name"`
	Data string `json:"data,omitempty"`
	Raw  string `json:"raw"`
}

type hyprEventCaptureOutput struct {
	Socket   string            `json:"socket"`
	Events   []hyprEventRecord `json:"events"`
	TimedOut bool              `json:"timed_out"`
}

func hyprExtendedToolDefinitions() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[EmptyInput, string](
			"hypr_get_active_window",
			"Return the active Hyprland window as raw JSON from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("activewindow") },
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_get_active_workspace",
			"Return the active Hyprland workspace as raw JSON from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("activeworkspace") },
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_list_binds",
			"Return the configured Hyprland keybinds as raw JSON from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("binds") },
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_list_devices",
			"Return the connected Hyprland devices as raw JSON from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("devices") },
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_list_layers",
			"Return the Hyprland layer-shell surfaces as raw JSON from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("layers") },
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_list_layouts",
			"Return the available Hyprland layouts as raw JSON or text from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("layouts") },
		),
		handler.TypedHandler[EmptyInput, hyprConfigErrorsOutput](
			"hypr_get_config_errors",
			"Return Hyprland config parse errors and whether the active config is healthy. When errors are present, attaches a structured Remediation record callers can dispatch directly.",
			func(_ context.Context, _ EmptyInput) (hyprConfigErrorsOutput, error) {
				raw, err := hyprQueryMaybeJSON("configerrors")
				if err != nil {
					return hyprConfigErrorsOutput{}, err
				}
				messages := hyprConfigErrorMessages(raw)
				out := hyprConfigErrorsOutput{
					Healthy:  len(messages) == 0,
					Messages: messages,
					Raw:      raw,
				}
				if !out.Healthy {
					// Attach the generic hypr_config_errors remediation so the
					// model / a hook can dispatch the reload without parsing
					// the message text. Specific codes (parse vs. option-does-
					// not-exist) can be discriminated later as detectors land.
					if rem, ok := remediation.Lookup(remediation.CodeHyprConfigErrors); ok {
						out.ErrorCode = string(remediation.CodeHyprConfigErrors)
						out.Remediation = &rem
					}
				}
				return out, nil
			},
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_get_cursor_position",
			"Return the current cursor position as raw JSON from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("cursorpos") },
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_get_version",
			"Return Hyprland version data as raw JSON or text from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("version") },
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_get_system_info",
			"Return Hyprland system information from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("systeminfo") },
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_list_workspace_rules",
			"Return Hyprland workspace rules as raw JSON or text from hyprctl.",
			func(_ context.Context, _ EmptyInput) (string, error) { return hyprQueryMaybeJSON("workspacerules") },
		),
		handler.TypedHandler[HyprGetOptionInput, string](
			"hypr_get_option",
			"Read a Hyprland option value via hyprctl getoption.",
			func(_ context.Context, input HyprGetOptionInput) (string, error) {
				if strings.TrimSpace(input.Option) == "" {
					return "", fmt.Errorf("[%s] option is required", handler.ErrInvalidParam)
				}
				return hyprQueryMaybeJSON("getoption", input.Option)
			},
		),
		handler.TypedHandler[HyprKeywordInput, string](
			"hypr_set_keyword",
			"Set a Hyprland keyword via hyprctl keyword.",
			func(_ context.Context, input HyprKeywordInput) (string, error) {
				if strings.TrimSpace(input.Keyword) == "" || strings.TrimSpace(input.Value) == "" {
					return "", fmt.Errorf("[%s] keyword and value are required", handler.ErrInvalidParam)
				}
				out, err := runHyprctl("keyword", input.Keyword, input.Value)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[HyprDispatchInput, string](
			"hypr_dispatch",
			"Run an arbitrary Hyprland dispatcher via hyprctl dispatch.",
			func(_ context.Context, input HyprDispatchInput) (string, error) {
				if strings.TrimSpace(input.Dispatcher) == "" {
					return "", fmt.Errorf("[%s] dispatcher is required", handler.ErrInvalidParam)
				}
				args := []string{"dispatch", input.Dispatcher}
				if strings.TrimSpace(input.Argument) != "" {
					args = append(args, input.Argument)
				}
				out, err := runHyprctl(args...)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[HyprNotifyInput, string](
			"hypr_notify",
			"Show a Hyprland notification using hyprctl notify.",
			func(_ context.Context, input HyprNotifyInput) (string, error) {
				if strings.TrimSpace(input.Message) == "" {
					return "", fmt.Errorf("[%s] message is required", handler.ErrInvalidParam)
				}
				icon := input.Icon
				if icon == 0 {
					icon = 1
				}
				duration := input.DurationMS
				if duration <= 0 {
					duration = 3000
				}
				color := strings.TrimSpace(input.Color)
				if color == "" {
					color = "rgb(33cc99)"
				}
				out, err := runHyprctl("notify", strconv.Itoa(icon), strconv.Itoa(duration), color, input.Message)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[EmptyInput, string](
			"hypr_dismiss_notify",
			"Dismiss the most recent Hyprland notification via hyprctl dismissnotify.",
			func(_ context.Context, _ EmptyInput) (string, error) {
				out, err := runHyprctl("dismissnotify")
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[HyprSetCursorInput, string](
			"hypr_set_cursor",
			"Set the active Hyprland cursor theme and size.",
			func(_ context.Context, input HyprSetCursorInput) (string, error) {
				if strings.TrimSpace(input.Theme) == "" || input.Size <= 0 {
					return "", fmt.Errorf("[%s] theme and size are required", handler.ErrInvalidParam)
				}
				out, err := runHyprctl("setcursor", input.Theme, strconv.Itoa(input.Size))
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[HyprSetPropInput, string](
			"hypr_set_prop",
			"Set a Hyprland window property via hyprctl setprop.",
			func(_ context.Context, input HyprSetPropInput) (string, error) {
				if strings.TrimSpace(input.Property) == "" || strings.TrimSpace(input.Value) == "" {
					return "", fmt.Errorf("[%s] property and value are required", handler.ErrInvalidParam)
				}
				selector, err := resolveHyprWindow(input.Address, input.Class, "")
				if err != nil {
					return "", err
				}
				args := []string{"setprop", selector, input.Property, input.Value}
				if input.Lock {
					args = append(args, "lock")
				}
				out, err := runHyprctl(args...)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[HyprGetPropInput, string](
			"hypr_get_prop",
			"Read a Hyprland window property via hyprctl getprop.",
			func(_ context.Context, input HyprGetPropInput) (string, error) {
				if strings.TrimSpace(input.Property) == "" {
					return "", fmt.Errorf("[%s] property is required", handler.ErrInvalidParam)
				}
				selector, err := resolveHyprWindow(input.Address, input.Class, "")
				if err != nil {
					return "", err
				}
				return hyprQueryMaybeJSON("getprop", selector, input.Property)
			},
		),
		handler.TypedHandler[HyprSwitchXKBLayoutInput, string](
			"hypr_switch_xkb_layout",
			"Switch a Hyprland keyboard layout via hyprctl switchxkblayout.",
			func(_ context.Context, input HyprSwitchXKBLayoutInput) (string, error) {
				if strings.TrimSpace(input.Device) == "" || strings.TrimSpace(input.Command) == "" {
					return "", fmt.Errorf("[%s] device and command are required", handler.ErrInvalidParam)
				}
				out, err := runHyprctl("switchxkblayout", input.Device, input.Command)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[HyprSubcommandInput, string](
			"hypr_output",
			"Run a Hyprland output subcommand such as create remove list or dpms through hyprctl output.",
			func(_ context.Context, input HyprSubcommandInput) (string, error) {
				return hyprRunSubcommand("output", input)
			},
		),
		handler.TypedHandler[HyprSubcommandInput, string](
			"hypr_plugin",
			"Run a Hyprland plugin subcommand through hyprctl plugin.",
			func(_ context.Context, input HyprSubcommandInput) (string, error) {
				return hyprRunSubcommand("plugin", input)
			},
		),
		handler.TypedHandler[HyprSubcommandInput, string](
			"hypr_hyprpaper",
			"Run a Hyprland hyprpaper subcommand through hyprctl hyprpaper.",
			func(_ context.Context, input HyprSubcommandInput) (string, error) {
				return hyprRunSubcommand("hyprpaper", input)
			},
		),
		handler.TypedHandler[HyprSubcommandInput, string](
			"hypr_hyprsunset",
			"Run a Hyprland hyprsunset subcommand through hyprctl hyprsunset.",
			func(_ context.Context, input HyprSubcommandInput) (string, error) {
				return hyprRunSubcommand("hyprsunset", input)
			},
		),
		handler.TypedHandler[HyprRollingLogInput, string](
			"hypr_rolling_log",
			"Return a recent snapshot of the Hyprland rolling log.",
			func(_ context.Context, input HyprRollingLogInput) (string, error) {
				lines := input.Lines
				if lines <= 0 {
					lines = 100
				}
				out, err := runHyprctlTimeout(2*time.Second, "rollinglog")
				if err != nil {
					return "", err
				}
				return trimTailLines(out, lines), nil
			},
		),
		handler.TypedHandler[HyprEventCaptureInput, hyprEventCaptureOutput](
			"hypr_capture_events",
			"Capture one or more Hyprland IPC events from .socket2.sock.",
			func(_ context.Context, input HyprEventCaptureInput) (hyprEventCaptureOutput, error) {
				count := input.Count
				if count <= 0 {
					count = 5
				}
				timeout := time.Duration(input.TimeoutMS) * time.Millisecond
				if timeout <= 0 {
					timeout = 5 * time.Second
				}
				socket, events, timedOut, err := hyprReadEvents(input.Event, count, timeout)
				if err != nil {
					return hyprEventCaptureOutput{}, err
				}
				return hyprEventCaptureOutput{Socket: socket, Events: events, TimedOut: timedOut}, nil
			},
		),
		handler.TypedHandler[HyprWaitEventInput, hyprEventRecord](
			"hypr_wait_for_event",
			"Wait for a specific Hyprland IPC event on .socket2.sock.",
			func(_ context.Context, input HyprWaitEventInput) (hyprEventRecord, error) {
				if strings.TrimSpace(input.Event) == "" {
					return hyprEventRecord{}, fmt.Errorf("[%s] event is required", handler.ErrInvalidParam)
				}
				timeout := time.Duration(input.TimeoutMS) * time.Millisecond
				if timeout <= 0 {
					timeout = 10 * time.Second
				}
				_, events, _, err := hyprReadEvents(input.Event, 1, timeout)
				if err != nil {
					return hyprEventRecord{}, err
				}
				if len(events) == 0 {
					return hyprEventRecord{}, fmt.Errorf("timed out waiting for Hyprland event %q", input.Event)
				}
				return events[0], nil
			},
		),
	}
}

func hyprQueryMaybeJSON(command string, args ...string) (string, error) {
	jsonArgs := append([]string{command}, args...)
	jsonArgs = append(jsonArgs, "-j")
	out, err := runHyprctl(jsonArgs...)
	if err == nil {
		return strings.TrimSpace(out), nil
	}
	out, rawErr := runHyprctl(append([]string{command}, args...)...)
	if rawErr != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func hyprRunSubcommand(base string, input HyprSubcommandInput) (string, error) {
	if strings.TrimSpace(input.Action) == "" {
		return "", fmt.Errorf("[%s] action is required", handler.ErrInvalidParam)
	}
	args := []string{base, input.Action}
	args = append(args, input.Args...)
	out, err := runHyprctl(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runHyprctlTimeout(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "hyprctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("hyprctl %s failed: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

func hyprSocket2Path() (string, error) {
	sig := hyprInstanceSig()
	if strings.TrimSpace(sig) == "" {
		return "", fmt.Errorf("HYPRLAND_INSTANCE_SIGNATURE not found")
	}
	runtimeDir := dotfilesRuntimeDir()
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	socket := filepath.Join(runtimeDir, "hypr", sig, ".socket2.sock")
	if _, err := os.Stat(socket); err != nil {
		return "", fmt.Errorf("hyprland event socket not found: %s", socket)
	}
	return socket, nil
}

func hyprParseEventLine(line string) hyprEventRecord {
	line = strings.TrimSpace(line)
	parts := strings.SplitN(line, ">>", 2)
	if len(parts) == 2 {
		return hyprEventRecord{Name: strings.TrimSpace(parts[0]), Data: strings.TrimSpace(parts[1]), Raw: line}
	}
	return hyprEventRecord{Name: line, Raw: line}
}

func hyprReadEvents(filter string, count int, timeout time.Duration) (string, []hyprEventRecord, bool, error) {
	socket, err := hyprSocket2Path()
	if err != nil {
		return "", nil, false, err
	}
	if count <= 0 {
		count = 1
	}

	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return socket, nil, false, fmt.Errorf("connect Hyprland event socket: %w", err)
	}
	defer conn.Close()

	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	events := make([]hyprEventRecord, 0, count)
	for scanner.Scan() {
		event := hyprParseEventLine(scanner.Text())
		if strings.TrimSpace(filter) != "" && event.Name != filter {
			continue
		}
		events = append(events, event)
		if len(events) >= count {
			return socket, events, false, nil
		}
	}

	if err := scanner.Err(); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return socket, events, true, nil
		}
		return socket, events, false, fmt.Errorf("read Hyprland events: %w", err)
	}
	return socket, events, false, nil
}

func trimTailLines(raw string, limit int) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if limit <= 0 || len(lines) <= limit {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-limit:], "\n")
}
