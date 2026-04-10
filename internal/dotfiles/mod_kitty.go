package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type KittyTargetInput struct {
	To string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address. Defaults to KITTY_LISTEN_ON or the controlling kitty window."`
}

type KittyMatchInput struct {
	To       string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match    string `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression"`
	MatchTab string `json:"match_tab,omitempty" jsonschema:"description=Optional kitty tab match expression"`
}

type KittyLoadConfigInput struct {
	To              string   `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Files           []string `json:"files,omitempty" jsonschema:"description=Config files to load. Leave empty to reload the currently active config."`
	Overrides       []string `json:"overrides,omitempty" jsonschema:"description=Optional kitty config overrides in name=value form"`
	IgnoreOverrides bool     `json:"ignore_overrides,omitempty" jsonschema:"description=Ignore previously applied overrides"`
}

type KittyFontSizeInput struct {
	To   string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Size string `json:"size" jsonschema:"required,description=Font size in points or a relative delta such as +2 -1 *1.1 /1.1 or 0 to reset"`
	All  bool   `json:"all,omitempty" jsonschema:"description=Apply to all OS windows"`
}

type KittyOpacityInput struct {
	To       string  `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Opacity  float64 `json:"opacity" jsonschema:"required,description=Background opacity between 0 and 1"`
	Toggle   bool    `json:"toggle,omitempty" jsonschema:"description=Reset to default when the current opacity already matches"`
	All      bool    `json:"all,omitempty" jsonschema:"description=Apply to all OS windows"`
	Match    string  `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression"`
	MatchTab string  `json:"match_tab,omitempty" jsonschema:"description=Optional kitty tab match expression"`
}

type KittyThemeInput struct {
	To         string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Theme      string `json:"theme,omitempty" jsonschema:"description=Theme name or local theme filename stem under DOTFILES_DIR/kitty"`
	File       string `json:"file,omitempty" jsonschema:"description=Explicit kitty color config file path"`
	All        bool   `json:"all,omitempty" jsonschema:"description=Apply to all windows"`
	Configured bool   `json:"configured,omitempty" jsonschema:"description=Also change the configured colors for new windows"`
	Match      string `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression"`
	MatchTab   string `json:"match_tab,omitempty" jsonschema:"description=Optional kitty tab match expression"`
}

type KittyLayoutInput struct {
	To     string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Layout string `json:"layout" jsonschema:"required,description=Layout name such as tall fat stack grid splits or vertical"`
	Match  string `json:"match,omitempty" jsonschema:"description=Optional kitty tab match expression"`
}

type KittyTitleInput struct {
	To        string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Title     string `json:"title,omitempty" jsonschema:"description=Window title to set. Leave empty to reset to the child-managed title."`
	Temporary bool   `json:"temporary,omitempty" jsonschema:"description=Allow the child process to change the title again later"`
	Match     string `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression"`
}

type KittySendTextInput struct {
	To       string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Text     string `json:"text" jsonschema:"required,description=Text payload to send using kitty escaping rules"`
	Match    string `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression"`
	MatchTab string `json:"match_tab,omitempty" jsonschema:"description=Optional kitty tab match expression"`
}

type KittyShowImageInput struct {
	To        string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Path      string `json:"path" jsonschema:"required,description=Image path visible to the kitty process"`
	Match     string `json:"match,omitempty" jsonschema:"description=Optional kitty tab match expression"`
	Title     string `json:"title,omitempty" jsonschema:"description=Optional overlay window title"`
	KeepFocus bool   `json:"keep_focus,omitempty" jsonschema:"description=Keep focus on the current kitty window"`
}

type KittyRunRemoteInput struct {
	To    string   `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Args  []string `json:"args" jsonschema:"required,description=Arguments to pass after kitty @"`
	Stdin string   `json:"stdin,omitempty" jsonschema:"description=Optional stdin payload for the remote command"`
}

type KittyTabMatchInput struct {
	To    string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match string `json:"match,omitempty" jsonschema:"description=Optional kitty tab match expression"`
}

type KittyWindowActionInput struct {
	To            string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match         string `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression"`
	Self          bool   `json:"self,omitempty" jsonschema:"description=Target the current kitty window when the command runs inside kitty"`
	IgnoreNoMatch bool   `json:"ignore_no_match,omitempty" jsonschema:"description=Ignore no-match conditions instead of failing"`
}

type KittyTabActionInput struct {
	To            string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match         string `json:"match,omitempty" jsonschema:"description=Optional kitty tab match expression"`
	Self          bool   `json:"self,omitempty" jsonschema:"description=Target the current kitty tab when the command runs inside kitty"`
	IgnoreNoMatch bool   `json:"ignore_no_match,omitempty" jsonschema:"description=Ignore no-match conditions instead of failing"`
}

type KittyGetTextInput struct {
	To             string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match          string `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression"`
	Extent         string `json:"extent,omitempty" jsonschema:"description=Text extent such as screen all selection last_cmd_output or last_non_empty_output"`
	ANSI           bool   `json:"ansi,omitempty" jsonschema:"description=Include ANSI formatting sequences"`
	AddCursor      bool   `json:"add_cursor,omitempty" jsonschema:"description=Append ANSI cursor position/style information"`
	AddWrapMarkers bool   `json:"add_wrap_markers,omitempty" jsonschema:"description=Add carriage-return wrap markers at line wraps"`
	ClearSelection bool   `json:"clear_selection,omitempty" jsonschema:"description=Clear the current selection after reading it"`
	Self           bool   `json:"self,omitempty" jsonschema:"description=Read from the current kitty window instead of the active window"`
}

type KittyLaunchInput struct {
	To           string   `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match        string   `json:"match,omitempty" jsonschema:"description=Optional kitty tab match expression"`
	SourceWindow string   `json:"source_window,omitempty" jsonschema:"description=Optional kitty window match expression used as the source window"`
	Title        string   `json:"title,omitempty" jsonschema:"description=Optional title for the new kitty window"`
	TabTitle     string   `json:"tab_title,omitempty" jsonschema:"description=Optional title when launching a new tab"`
	Type         string   `json:"type,omitempty" jsonschema:"description=Launch type such as window tab os-window overlay overlay-main or background"`
	KeepFocus    bool     `json:"keep_focus,omitempty" jsonschema:"description=Keep focus on the currently active kitty window"`
	Cwd          string   `json:"cwd,omitempty" jsonschema:"description=Working directory for the launched process"`
	Location     string   `json:"location,omitempty" jsonschema:"description=Placement hint such as default after before split hsplit or vsplit"`
	NextTo       string   `json:"next_to,omitempty" jsonschema:"description=Optional kitty window match expression used as the placement anchor"`
	Env          []string `json:"env,omitempty" jsonschema:"description=Environment variables in NAME=VALUE form"`
	Vars         []string `json:"vars,omitempty" jsonschema:"description=Kitty user variables in NAME=VALUE form"`
	Hold         bool     `json:"hold,omitempty" jsonschema:"description=Keep the launched window open at a shell prompt after the process exits"`
	CopyColors   bool     `json:"copy_colors,omitempty" jsonschema:"description=Copy colors from the source window"`
	CopyCmdline  bool     `json:"copy_cmdline,omitempty" jsonschema:"description=Reuse the command line from the source window"`
	CopyEnv      bool     `json:"copy_env,omitempty" jsonschema:"description=Copy the environment captured when the source window was created"`
	Args         []string `json:"args,omitempty" jsonschema:"description=Command and arguments to run. Leave empty to launch the default shell"`
}

type KittyResizeWindowInput struct {
	To        string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match     string `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression"`
	Increment int    `json:"increment,omitempty" jsonschema:"description=Cell increment applied during resize. Defaults to 2 when omitted"`
	Axis      string `json:"axis,omitempty" jsonschema:"description=Resize axis: horizontal vertical or reset"`
	Self      bool   `json:"self,omitempty" jsonschema:"description=Resize the current kitty window instead of the active window"`
}

type KittyResizeOSWindowInput struct {
	To           string   `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match        string   `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression selecting the OS window"`
	Action       string   `json:"action,omitempty" jsonschema:"description=OS window action such as resize hide show toggle-fullscreen toggle-maximized toggle-visibility or os-panel"`
	Unit         string   `json:"unit,omitempty" jsonschema:"description=Size unit: cells or pixels"`
	Width        int      `json:"width,omitempty" jsonschema:"description=Window width or width delta depending on incremental"`
	Height       int      `json:"height,omitempty" jsonschema:"description=Window height or height delta depending on incremental"`
	Incremental  bool     `json:"incremental,omitempty" jsonschema:"description=Treat width and height as deltas instead of absolute values"`
	Self         bool     `json:"self,omitempty" jsonschema:"description=Resize the current kitty OS window instead of the active window"`
	PanelOptions []string `json:"panel_options,omitempty" jsonschema:"description=Additional panel settings passed through when action=os-panel"`
}

type KittySetTabTitleInput struct {
	To    string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match string `json:"match,omitempty" jsonschema:"description=Optional kitty tab match expression"`
	Title string `json:"title,omitempty" jsonschema:"description=Title to set. Leave empty to fall back to the active window title in the tab"`
}

type KittySendKeyInput struct {
	To            string   `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match         string   `json:"match,omitempty" jsonschema:"description=Optional kitty window match expression"`
	MatchTab      string   `json:"match_tab,omitempty" jsonschema:"description=Optional kitty tab match expression"`
	Keys          []string `json:"keys" jsonschema:"required,description=Keys to send such as ctrl+c alt+left or enter"`
	All           bool     `json:"all,omitempty" jsonschema:"description=Send the keys to all windows"`
	ExcludeActive bool     `json:"exclude_active,omitempty" jsonschema:"description=Do not send keys to the active window even when it matches"`
}

type KittyRestoreLayoutInput struct {
	To    string `json:"to,omitempty" jsonschema:"description=Optional kitty remote-control address"`
	Match string `json:"match,omitempty" jsonschema:"description=Optional kitty tab match expression"`
	All   bool   `json:"all,omitempty" jsonschema:"description=Apply to all matched tabs"`
}

type kittyStatusOutput struct {
	Version       string   `json:"version"`
	ListenOn      string   `json:"listen_on,omitempty"`
	Connected     bool     `json:"connected"`
	OSWindowCount int      `json:"os_window_count"`
	TabCount      int      `json:"tab_count"`
	WindowCount   int      `json:"window_count"`
	LocalThemes   []string `json:"local_themes,omitempty"`
	Message       string   `json:"message,omitempty"`
}

type kittyTabSummary struct {
	OSWindowID  int    `json:"os_window_id"`
	TabID       int    `json:"tab_id"`
	Title       string `json:"title,omitempty"`
	Layout      string `json:"layout,omitempty"`
	Focused     bool   `json:"focused"`
	WindowCount int    `json:"window_count"`
}

type kittyWindowSummary struct {
	OSWindowID int      `json:"os_window_id"`
	TabID      int      `json:"tab_id"`
	WindowID   int      `json:"window_id"`
	Title      string   `json:"title,omitempty"`
	Focused    bool     `json:"focused"`
	PID        int      `json:"pid,omitempty"`
	Cwd        string   `json:"cwd,omitempty"`
	Cmdline    []string `json:"cmdline,omitempty"`
}

type kittyLaunchOutput struct {
	WindowID int    `json:"window_id,omitempty"`
	Raw      string `json:"raw,omitempty"`
}

type kittyTextOutput struct {
	Text string `json:"text"`
}

type KittyModule struct{}

func (m *KittyModule) Name() string { return "kitty" }
func (m *KittyModule) Description() string {
	return "Kitty remote-control tools for runtime terminal management"
}

func (m *KittyModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[KittyTargetInput, kittyStatusOutput](
			"kitty_status",
			"Show kitty remote-control readiness, active connection status, and local theme candidates.",
			func(_ context.Context, input KittyTargetInput) (kittyStatusOutput, error) {
				status := kittyStatusOutput{
					ListenOn:    strings.TrimSpace(input.To),
					LocalThemes: kittyLocalThemes(),
				}
				if status.ListenOn == "" {
					status.ListenOn = strings.TrimSpace(os.Getenv("KITTY_LISTEN_ON"))
				}
				versionOut, err := sysRunCmd("kitty", "--version")
				if err == nil {
					status.Version = strings.TrimSpace(versionOut)
				} else {
					status.Message = err.Error()
					return status, nil
				}
				raw, err := kittyRun(input.To, "ls")
				if err != nil {
					status.Message = err.Error()
					return status, nil
				}
				tabs, windows, osWindows, err := kittySummariesFromLS(raw)
				if err != nil {
					status.Message = err.Error()
					return status, nil
				}
				status.Connected = true
				status.OSWindowCount = osWindows
				status.TabCount = len(tabs)
				status.WindowCount = len(windows)
				return status, nil
			},
		),
		handler.TypedHandler[KittyTargetInput, []kittyTabSummary](
			"kitty_list_tabs",
			"List kitty tabs with OS window, title, layout, focus, and window count.",
			func(_ context.Context, input KittyTargetInput) ([]kittyTabSummary, error) {
				raw, err := kittyRun(input.To, "ls")
				if err != nil {
					return nil, err
				}
				tabs, _, _, err := kittySummariesFromLS(raw)
				return tabs, err
			},
		),
		handler.TypedHandler[KittyTargetInput, []kittyWindowSummary](
			"kitty_list_windows",
			"List kitty windows with IDs, titles, cwd, pid, focus state, and command lines.",
			func(_ context.Context, input KittyTargetInput) ([]kittyWindowSummary, error) {
				raw, err := kittyRun(input.To, "ls")
				if err != nil {
					return nil, err
				}
				_, windows, _, err := kittySummariesFromLS(raw)
				return windows, err
			},
		),
		handler.TypedHandler[KittyWindowActionInput, string](
			"kitty_focus_window",
			"Focus the active or matched kitty window.",
			func(_ context.Context, input KittyWindowActionInput) (string, error) {
				args := []string{"focus-window"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				out, err := kittyRun(input.To, args...)
				return kittyActionResult(out, "focused kitty window"), err
			},
		),
		handler.TypedHandler[KittyTabMatchInput, string](
			"kitty_focus_tab",
			"Focus the active window in a matched kitty tab.",
			func(_ context.Context, input KittyTabMatchInput) (string, error) {
				args := []string{"focus-tab"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				out, err := kittyRun(input.To, args...)
				return kittyActionResult(out, "focused kitty tab"), err
			},
		),
		handler.TypedHandler[KittyGetTextInput, kittyTextOutput](
			"kitty_get_text",
			"Read plain or ANSI-formatted text from the active or matched kitty window.",
			func(_ context.Context, input KittyGetTextInput) (kittyTextOutput, error) {
				args := []string{"get-text"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.Extent) != "" {
					args = append(args, "--extent", input.Extent)
				}
				if input.ANSI {
					args = append(args, "--ansi")
				}
				if input.AddCursor {
					args = append(args, "--add-cursor")
				}
				if input.AddWrapMarkers {
					args = append(args, "--add-wrap-markers")
				}
				if input.ClearSelection {
					args = append(args, "--clear-selection")
				}
				if input.Self {
					args = append(args, "--self")
				}
				out, err := kittyRun(input.To, args...)
				return kittyTextOutput{Text: out}, err
			},
		),
		handler.TypedHandler[KittyLaunchInput, kittyLaunchOutput](
			"kitty_launch",
			"Launch a new kitty window, tab, overlay, background task, or OS window.",
			func(_ context.Context, input KittyLaunchInput) (kittyLaunchOutput, error) {
				args := []string{"launch"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.SourceWindow) != "" {
					args = append(args, "--source-window", input.SourceWindow)
				}
				if strings.TrimSpace(input.Title) != "" {
					args = append(args, "--title", input.Title)
				}
				if strings.TrimSpace(input.TabTitle) != "" {
					args = append(args, "--tab-title", input.TabTitle)
				}
				if strings.TrimSpace(input.Type) != "" {
					args = append(args, "--type", input.Type)
				}
				if input.KeepFocus {
					args = append(args, "--keep-focus")
				}
				if strings.TrimSpace(input.Cwd) != "" {
					args = append(args, "--cwd", input.Cwd)
				}
				if strings.TrimSpace(input.Location) != "" {
					args = append(args, "--location", input.Location)
				}
				if strings.TrimSpace(input.NextTo) != "" {
					args = append(args, "--next-to", input.NextTo)
				}
				for _, envVar := range input.Env {
					if strings.TrimSpace(envVar) != "" {
						args = append(args, "--env", envVar)
					}
				}
				for _, userVar := range input.Vars {
					if strings.TrimSpace(userVar) != "" {
						args = append(args, "--var", userVar)
					}
				}
				if input.Hold {
					args = append(args, "--hold")
				}
				if input.CopyColors {
					args = append(args, "--copy-colors")
				}
				if input.CopyCmdline {
					args = append(args, "--copy-cmdline")
				}
				if input.CopyEnv {
					args = append(args, "--copy-env")
				}
				args = append(args, input.Args...)
				out, err := kittyRun(input.To, args...)
				if err != nil {
					return kittyLaunchOutput{}, err
				}
				return kittyLaunchResult(out), nil
			},
		),
		handler.TypedHandler[KittyLoadConfigInput, string](
			"kitty_load_config",
			"Reload one or more kitty config files, optionally with transient overrides.",
			func(_ context.Context, input KittyLoadConfigInput) (string, error) {
				args := []string{"load-config"}
				if input.IgnoreOverrides {
					args = append(args, "--ignore-overrides")
				}
				for _, override := range input.Overrides {
					if strings.TrimSpace(override) != "" {
						args = append(args, "--override", override)
					}
				}
				for _, file := range input.Files {
					if strings.TrimSpace(file) != "" {
						args = append(args, file)
					}
				}
				out, err := kittyRun(input.To, args...)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[KittyFontSizeInput, string](
			"kitty_set_font_size",
			"Set the kitty font size in points or with a relative delta.",
			func(_ context.Context, input KittyFontSizeInput) (string, error) {
				if strings.TrimSpace(input.Size) == "" {
					return "", fmt.Errorf("[%s] size is required", handler.ErrInvalidParam)
				}
				args := []string{"set-font-size"}
				if input.All {
					args = append(args, "--all")
				}
				args = append(args, input.Size)
				out, err := kittyRun(input.To, args...)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[KittyOpacityInput, string](
			"kitty_set_opacity",
			"Set the kitty background opacity for the active, matched, or all windows.",
			func(_ context.Context, input KittyOpacityInput) (string, error) {
				if input.Opacity < 0 || input.Opacity > 1 {
					return "", fmt.Errorf("[%s] opacity must be between 0 and 1", handler.ErrInvalidParam)
				}
				args := []string{"set-background-opacity"}
				if input.All {
					args = append(args, "--all")
				}
				if input.Toggle {
					args = append(args, "--toggle")
				}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.MatchTab) != "" {
					args = append(args, "--match-tab", input.MatchTab)
				}
				args = append(args, fmt.Sprintf("%.2f", input.Opacity))
				out, err := kittyRun(input.To, args...)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[KittyThemeInput, string](
			"kitty_set_theme",
			"Apply a kitty color theme from DOTFILES_DIR/kitty or by dumping a named kitten theme.",
			func(_ context.Context, input KittyThemeInput) (string, error) {
				themeFile, cleanup, err := resolveKittyThemeFile(input.Theme, input.File)
				if err != nil {
					return "", err
				}
				if cleanup != nil {
					defer cleanup()
				}
				args := []string{"set-colors"}
				if input.All {
					args = append(args, "--all")
				}
				if input.Configured {
					args = append(args, "--configured")
				}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.MatchTab) != "" {
					args = append(args, "--match-tab", input.MatchTab)
				}
				args = append(args, themeFile)
				_, err = kittyRun(input.To, args...)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("applied kitty theme from %s", themeFile), nil
			},
		),
		handler.TypedHandler[KittyLayoutInput, string](
			"kitty_set_layout",
			"Set the kitty tab layout for the active or matched tabs.",
			func(_ context.Context, input KittyLayoutInput) (string, error) {
				if strings.TrimSpace(input.Layout) == "" {
					return "", fmt.Errorf("[%s] layout is required", handler.ErrInvalidParam)
				}
				args := []string{"goto-layout"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				args = append(args, input.Layout)
				out, err := kittyRun(input.To, args...)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[KittyRestoreLayoutInput, string](
			"kitty_last_used_layout",
			"Restore the previous kitty layout for the active or matched tabs.",
			func(_ context.Context, input KittyRestoreLayoutInput) (string, error) {
				args := []string{"last-used-layout"}
				if input.All {
					args = append(args, "--all")
				}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				out, err := kittyRun(input.To, args...)
				return kittyActionResult(out, "restored kitty layout"), err
			},
		),
		handler.TypedHandler[KittyTitleInput, string](
			"kitty_set_title",
			"Set or reset the title of the active or matched kitty windows.",
			func(_ context.Context, input KittyTitleInput) (string, error) {
				args := []string{"set-window-title"}
				if input.Temporary {
					args = append(args, "--temporary")
				}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.Title) != "" {
					args = append(args, input.Title)
				}
				out, err := kittyRun(input.To, args...)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[KittySetTabTitleInput, string](
			"kitty_set_tab_title",
			"Set or reset the title of the active or matched kitty tabs.",
			func(_ context.Context, input KittySetTabTitleInput) (string, error) {
				args := []string{"set-tab-title"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.Title) != "" {
					args = append(args, input.Title)
				}
				out, err := kittyRun(input.To, args...)
				return kittyActionResult(out, "updated kitty tab title"), err
			},
		),
		handler.TypedHandler[KittySendTextInput, string](
			"kitty_send_text",
			"Send arbitrary text to the active or matched kitty windows.",
			func(_ context.Context, input KittySendTextInput) (string, error) {
				if input.Text == "" {
					return "", fmt.Errorf("[%s] text is required", handler.ErrInvalidParam)
				}
				args := []string{"send-text"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.MatchTab) != "" {
					args = append(args, "--match-tab", input.MatchTab)
				}
				args = append(args, input.Text)
				out, err := kittyRun(input.To, args...)
				return strings.TrimSpace(out), err
			},
		),
		handler.TypedHandler[KittySendKeyInput, string](
			"kitty_send_key",
			"Send key chords directly to the active or matched kitty windows.",
			func(_ context.Context, input KittySendKeyInput) (string, error) {
				if len(input.Keys) == 0 {
					return "", fmt.Errorf("[%s] keys must not be empty", handler.ErrInvalidParam)
				}
				args := []string{"send-key"}
				if input.All {
					args = append(args, "--all")
				}
				if input.ExcludeActive {
					args = append(args, "--exclude-active")
				}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.MatchTab) != "" {
					args = append(args, "--match-tab", input.MatchTab)
				}
				args = append(args, input.Keys...)
				out, err := kittyRun(input.To, args...)
				return kittyActionResult(out, "sent kitty keys"), err
			},
		),
		handler.TypedHandler[KittyResizeWindowInput, string](
			"kitty_resize_window",
			"Resize a kitty pane within the current layout.",
			func(_ context.Context, input KittyResizeWindowInput) (string, error) {
				args := []string{"resize-window"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if input.Increment != 0 {
					args = append(args, "--increment", strconv.Itoa(input.Increment))
				}
				if strings.TrimSpace(input.Axis) != "" {
					args = append(args, "--axis", input.Axis)
				}
				if input.Self {
					args = append(args, "--self")
				}
				out, err := kittyRun(input.To, args...)
				return kittyActionResult(out, "resized kitty window"), err
			},
		),
		handler.TypedHandler[KittyResizeOSWindowInput, string](
			"kitty_resize_os_window",
			"Resize or toggle state for a kitty OS window.",
			func(_ context.Context, input KittyResizeOSWindowInput) (string, error) {
				args := []string{"resize-os-window"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.Action) != "" {
					args = append(args, "--action", input.Action)
				}
				if strings.TrimSpace(input.Unit) != "" {
					args = append(args, "--unit", input.Unit)
				}
				if input.Width != 0 {
					args = append(args, "--width", strconv.Itoa(input.Width))
				}
				if input.Height != 0 {
					args = append(args, "--height", strconv.Itoa(input.Height))
				}
				if input.Incremental {
					args = append(args, "--incremental")
				}
				if input.Self {
					args = append(args, "--self")
				}
				args = append(args, input.PanelOptions...)
				out, err := kittyRun(input.To, args...)
				return kittyActionResult(out, "updated kitty OS window"), err
			},
		),
		handler.TypedHandler[KittyShowImageInput, string](
			"kitty_show_image",
			"Show an image in an overlay kitty window using kitten icat.",
			func(_ context.Context, input KittyShowImageInput) (string, error) {
				if strings.TrimSpace(input.Path) == "" {
					return "", fmt.Errorf("[%s] path is required", handler.ErrInvalidParam)
				}
				if _, err := os.Stat(input.Path); err != nil {
					return "", fmt.Errorf("image path not readable: %w", err)
				}
				args := []string{"launch", "--type=overlay"}
				if input.KeepFocus {
					args = append(args, "--keep-focus")
				}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if strings.TrimSpace(input.Title) != "" {
					args = append(args, "--title", input.Title)
				}
				args = append(args, "kitten", "icat", input.Path)
				out, err := kittyRun(input.To, args...)
				if err != nil {
					return "", err
				}
				return strings.TrimSpace(out), nil
			},
		),
		handler.TypedHandler[KittyWindowActionInput, string](
			"kitty_close_window",
			"Close the active or matched kitty windows.",
			func(_ context.Context, input KittyWindowActionInput) (string, error) {
				args := []string{"close-window"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if input.Self {
					args = append(args, "--self")
				}
				if input.IgnoreNoMatch {
					args = append(args, "--ignore-no-match")
				}
				out, err := kittyRun(input.To, args...)
				return kittyActionResult(out, "closed kitty window"), err
			},
		),
		handler.TypedHandler[KittyTabActionInput, string](
			"kitty_close_tab",
			"Close the active or matched kitty tabs.",
			func(_ context.Context, input KittyTabActionInput) (string, error) {
				args := []string{"close-tab"}
				if strings.TrimSpace(input.Match) != "" {
					args = append(args, "--match", input.Match)
				}
				if input.Self {
					args = append(args, "--self")
				}
				if input.IgnoreNoMatch {
					args = append(args, "--ignore-no-match")
				}
				out, err := kittyRun(input.To, args...)
				return kittyActionResult(out, "closed kitty tab"), err
			},
		),
		handler.TypedHandler[KittyRunRemoteInput, string](
			"kitty_run_remote",
			"Run an arbitrary kitty remote-control subcommand when the dedicated tools do not cover the exact action.",
			func(_ context.Context, input KittyRunRemoteInput) (string, error) {
				if len(input.Args) == 0 {
					return "", fmt.Errorf("[%s] args must not be empty", handler.ErrInvalidParam)
				}
				out, err := kittyRunWithInput(input.To, input.Stdin, input.Args...)
				return strings.TrimSpace(out), err
			},
		),
	}
}

func kittyRun(to string, args ...string) (string, error) {
	return kittyRunWithInput(to, "", args...)
}

func kittyRunWithInput(to string, stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fullArgs := []string{"@"}
	if strings.TrimSpace(to) != "" {
		fullArgs = append(fullArgs, "--to", to)
	}
	fullArgs = append(fullArgs, args...)
	cmd := exec.CommandContext(ctx, "kitty", fullArgs...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("kitty %s failed: %w: %s", strings.Join(fullArgs, " "), err, string(out))
	}
	return string(out), nil
}

func kittyActionResult(out string, fallback string) string {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func kittyLaunchResult(raw string) kittyLaunchOutput {
	trimmed := strings.TrimSpace(raw)
	out := kittyLaunchOutput{Raw: trimmed}
	if trimmed == "" {
		return out
	}
	if id, err := strconv.Atoi(trimmed); err == nil {
		out.WindowID = id
	}
	return out
}

func kittyLocalThemes() []string {
	base := filepath.Join(dotfilesDir(), "kitty")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var themes []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".conf") {
			continue
		}
		switch name {
		case "kitty.conf", "kitty.extras.conf", "open-actions.conf":
			continue
		}
		themes = append(themes, strings.TrimSuffix(name, ".conf"))
	}
	sort.Strings(themes)
	return themes
}

func resolveKittyThemeFile(theme, file string) (string, func(), error) {
	if strings.TrimSpace(file) != "" {
		resolved := file
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(dotfilesDir(), "kitty", resolved)
		}
		if _, err := os.Stat(resolved); err != nil {
			return "", nil, fmt.Errorf("theme file not found: %w", err)
		}
		return resolved, nil, nil
	}
	theme = strings.TrimSpace(theme)
	if theme == "" {
		return "", nil, fmt.Errorf("[%s] theme or file is required", handler.ErrInvalidParam)
	}

	localCandidates := []string{
		filepath.Join(dotfilesDir(), "kitty", theme),
		filepath.Join(dotfilesDir(), "kitty", theme+".conf"),
	}
	for _, candidate := range localCandidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil, nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kitty", "+kitten", "themes", "--dump-theme", theme)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("dump kitty theme %q: %w: %s", theme, err, string(out))
	}
	tmp, err := os.CreateTemp("", "kitty-theme-*.conf")
	if err != nil {
		return "", nil, err
	}
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", nil, err
	}
	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
}

func kittySummariesFromLS(raw string) ([]kittyTabSummary, []kittyWindowSummary, int, error) {
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, nil, 0, fmt.Errorf("parse kitty ls JSON: %w", err)
	}
	tabs := make([]kittyTabSummary, 0)
	windows := make([]kittyWindowSummary, 0)
	for _, osWindow := range parsed {
		osWindowID := intValue(osWindow["id"])
		tabList, _ := osWindow["tabs"].([]any)
		for _, tabValue := range tabList {
			tab, ok := tabValue.(map[string]any)
			if !ok {
				continue
			}
			tabID := intValue(tab["id"])
			windowList, _ := tab["windows"].([]any)
			tabSummary := kittyTabSummary{
				OSWindowID:  osWindowID,
				TabID:       tabID,
				Title:       stringValue(tab["title"]),
				Layout:      stringValue(tab["layout"]),
				Focused:     boolValue(tab["is_focused"]) || boolValue(tab["is_active"]),
				WindowCount: len(windowList),
			}
			tabs = append(tabs, tabSummary)
			for _, windowValue := range windowList {
				window, ok := windowValue.(map[string]any)
				if !ok {
					continue
				}
				summary := kittyWindowSummary{
					OSWindowID: osWindowID,
					TabID:      tabID,
					WindowID:   intValue(window["id"]),
					Title:      stringValue(window["title"]),
					Focused:    boolValue(window["is_focused"]) || boolValue(window["is_active"]),
					PID:        intValue(window["pid"]),
					Cwd:        firstNonEmptyString(window["cwd"], window["current_working_directory"], window["working_directory"]),
				}
				if cmdline, ok := toStringSlice(window["cmdline"]); ok {
					summary.Cmdline = cmdline
				}
				windows = append(windows, summary)
			}
		}
	}
	sort.Slice(tabs, func(i, j int) bool {
		if tabs[i].OSWindowID == tabs[j].OSWindowID {
			return tabs[i].TabID < tabs[j].TabID
		}
		return tabs[i].OSWindowID < tabs[j].OSWindowID
	})
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].OSWindowID == windows[j].OSWindowID {
			if windows[i].TabID == windows[j].TabID {
				return windows[i].WindowID < windows[j].WindowID
			}
			return windows[i].TabID < windows[j].TabID
		}
		return windows[i].OSWindowID < windows[j].OSWindowID
	})
	return tabs, windows, len(parsed), nil
}

func intValue(v any) int {
	switch value := v.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return 0
	}
}

func boolValue(v any) bool {
	value, _ := v.(bool)
	return value
}

func stringValue(v any) string {
	value, _ := v.(string)
	return strings.TrimSpace(value)
}

func toStringSlice(v any) ([]string, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out, true
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s := stringValue(value); s != "" {
			return s
		}
	}
	return ""
}
