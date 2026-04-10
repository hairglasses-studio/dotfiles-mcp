package dotfiles

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

//go:embed atspi_helper.py
var embeddedATSPIHelper string

type DesktopSemanticModule struct{}

func (m *DesktopSemanticModule) Name() string { return "desktop_semantic" }
func (m *DesktopSemanticModule) Description() string {
	return "Semantic desktop control via AT-SPI with an embedded helper"
}

type desktopSemanticTreeInput struct {
	App   string `json:"app" jsonschema:"required,description=Application name or unique substring"`
	Depth int    `json:"depth,omitempty" jsonschema:"description=Max tree depth (default 5)"`
}

type desktopSemanticWindowsInput struct {
	App string `json:"app,omitempty" jsonschema:"description=Optional application name filter for semantic window enumeration"`
}

type desktopSemanticQueryInput struct {
	App    string   `json:"app" jsonschema:"required,description=Application name or unique substring"`
	Name   string   `json:"name,omitempty" jsonschema:"description=Element name or substring"`
	Role   string   `json:"role,omitempty" jsonschema:"description=Optional exact role filter"`
	Ref    string   `json:"ref,omitempty" jsonschema:"description=Optional semantic reference such as ref_0_2_1 from a previous semantic result"`
	Path   string   `json:"path,omitempty" jsonschema:"description=Optional child-index path such as 0/2/1 from a previous semantic result"`
	States []string `json:"states,omitempty" jsonschema:"description=Optional required AT-SPI states such as focused or enabled"`
	Limit  int      `json:"limit,omitempty" jsonschema:"description=Optional max matches for multi-match queries (default 20)"`
	Exact  bool     `json:"exact,omitempty" jsonschema:"description=Require exact case-insensitive name matching"`
}

type desktopSemanticActionInput struct {
	App    string   `json:"app" jsonschema:"required,description=Application name or unique substring"`
	Name   string   `json:"name,omitempty" jsonschema:"description=Element name or substring"`
	Role   string   `json:"role,omitempty" jsonschema:"description=Optional exact role filter"`
	Ref    string   `json:"ref,omitempty" jsonschema:"description=Optional semantic reference such as ref_0_2_1 from a previous semantic result"`
	Path   string   `json:"path,omitempty" jsonschema:"description=Optional child-index path such as 0/2/1 from a previous semantic result"`
	States []string `json:"states,omitempty" jsonschema:"description=Optional required AT-SPI states such as focused or enabled"`
	Action string   `json:"action,omitempty" jsonschema:"description=Optional explicit action name to invoke, such as activate or press"`
	Exact  bool     `json:"exact,omitempty" jsonschema:"description=Require exact case-insensitive name matching"`
}

type desktopSemanticWaitInput struct {
	App     string   `json:"app" jsonschema:"required,description=Application name or unique substring"`
	Name    string   `json:"name,omitempty" jsonschema:"description=Element name or substring"`
	Role    string   `json:"role,omitempty" jsonschema:"description=Optional exact role filter"`
	Ref     string   `json:"ref,omitempty" jsonschema:"description=Optional semantic reference such as ref_0_2_1 from a previous semantic result"`
	Path    string   `json:"path,omitempty" jsonschema:"description=Optional child-index path such as 0/2/1 from a previous semantic result"`
	States  []string `json:"states,omitempty" jsonschema:"description=Optional required AT-SPI states such as focused or enabled"`
	Exact   bool     `json:"exact,omitempty" jsonschema:"description=Require exact case-insensitive name matching"`
	Timeout int      `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds (default 5)"`
}

type desktopSemanticWindowInput struct {
	App           string `json:"app,omitempty" jsonschema:"description=AT-SPI application name when focusing a window by semantic ref or path"`
	Class         string `json:"class,omitempty" jsonschema:"description=Application name substring to match when focusing a window semantically"`
	TitleContains string `json:"title_contains,omitempty" jsonschema:"description=Window title substring to match when focusing a window semantically"`
	Ref           string `json:"ref,omitempty" jsonschema:"description=Semantic window reference from desktop_list_windows"`
	Path          string `json:"path,omitempty" jsonschema:"description=Semantic child-index path from desktop_list_windows"`
}

type desktopSemanticTextInput struct {
	App    string   `json:"app" jsonschema:"required,description=Application name or unique substring"`
	Name   string   `json:"name,omitempty" jsonschema:"description=Element name or substring"`
	Role   string   `json:"role,omitempty" jsonschema:"description=Optional exact role filter"`
	Ref    string   `json:"ref,omitempty" jsonschema:"description=Optional semantic reference such as ref_0_2_1 from a previous semantic result"`
	Path   string   `json:"path,omitempty" jsonschema:"description=Optional child-index path such as 0/2/1 from a previous semantic result"`
	States []string `json:"states,omitempty" jsonschema:"description=Optional required AT-SPI states such as focused or enabled"`
	Exact  bool     `json:"exact,omitempty" jsonschema:"description=Require exact case-insensitive name matching"`
	Text   *string  `json:"text" jsonschema:"required,description=Text contents to set. Use an empty string to clear the field."`
}

type desktopSemanticValueInput struct {
	App    string   `json:"app" jsonschema:"required,description=Application name or unique substring"`
	Name   string   `json:"name,omitempty" jsonschema:"description=Element name or substring"`
	Role   string   `json:"role,omitempty" jsonschema:"description=Optional exact role filter"`
	Ref    string   `json:"ref,omitempty" jsonschema:"description=Optional semantic reference such as ref_0_2_1 from a previous semantic result"`
	Path   string   `json:"path,omitempty" jsonschema:"description=Optional child-index path such as 0/2/1 from a previous semantic result"`
	States []string `json:"states,omitempty" jsonschema:"description=Optional required AT-SPI states such as focused or enabled"`
	Exact  bool     `json:"exact,omitempty" jsonschema:"description=Require exact case-insensitive name matching"`
	Value  *float64 `json:"value" jsonschema:"required,description=Numeric value to set, such as a slider or spinbox value."`
}

type desktopSemanticCapabilitiesOutput struct {
	Ready             bool     `json:"ready"`
	PythonAvailable   bool     `json:"python_available"`
	PyATSPIAvailable  bool     `json:"pyatspi_available"`
	HelperPath        string   `json:"helper_path,omitempty"`
	WaylandDisplay    string   `json:"wayland_display,omitempty"`
	BusAddressPresent bool     `json:"bus_address_present"`
	Details           []string `json:"details,omitempty"`
	Missing           []string `json:"missing,omitempty"`
}

type desktopSemanticSnapshotOutput struct {
	HelperPath string           `json:"helper_path,omitempty"`
	Apps       []map[string]any `json:"apps,omitempty"`
}

type desktopSemanticWindowsOutput struct {
	HelperPath string           `json:"helper_path,omitempty"`
	App        string           `json:"app,omitempty"`
	Count      int              `json:"count"`
	Windows    []map[string]any `json:"windows,omitempty"`
	Error      string           `json:"error,omitempty"`
}

type desktopSemanticMatchesOutput struct {
	HelperPath string                    `json:"helper_path,omitempty"`
	App        string                    `json:"app"`
	Query      desktopSemanticQueryInput `json:"query"`
	Matched    bool                      `json:"matched"`
	Count      int                       `json:"count"`
	Matches    []map[string]any          `json:"matches,omitempty"`
	Error      string                    `json:"error,omitempty"`
}

type desktopSemanticTreeOutput struct {
	HelperPath string         `json:"helper_path,omitempty"`
	App        string         `json:"app"`
	Depth      int            `json:"depth"`
	Matched    bool           `json:"matched"`
	Tree       map[string]any `json:"tree,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type desktopSemanticElementOutput struct {
	HelperPath string                    `json:"helper_path,omitempty"`
	App        string                    `json:"app"`
	Query      desktopSemanticQueryInput `json:"query"`
	Matched    bool                      `json:"matched"`
	Clicked    bool                      `json:"clicked,omitempty"`
	Invoked    bool                      `json:"invoked,omitempty"`
	Focused    bool                      `json:"focused,omitempty"`
	Updated    bool                      `json:"updated,omitempty"`
	Action     string                    `json:"action,omitempty"`
	Value      any                       `json:"value,omitempty"`
	ValueKind  string                    `json:"value_kind,omitempty"`
	Element    map[string]any            `json:"element,omitempty"`
	Error      string                    `json:"error,omitempty"`
}

func dotfilesSemanticHelperPath() (string, error) {
	dir, err := ensureDotfilesManagedStateDir("semantic")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "atspi_helper.py")
	if err := os.WriteFile(path, []byte(embeddedATSPIHelper), 0o755); err != nil {
		return "", fmt.Errorf("write embedded AT-SPI helper: %w", err)
	}
	return path, nil
}

func desktopSemanticCapabilities() desktopSemanticCapabilitiesOutput {
	out := desktopSemanticCapabilitiesOutput{
		WaylandDisplay:    dotfilesWaylandDisplay(dotfilesRuntimeDir()),
		BusAddressPresent: strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")) != "",
	}

	if hasCmd("python3") {
		out.PythonAvailable = true
		out.Details = append(out.Details, "python3 available")
	} else {
		out.Missing = append(out.Missing, "python3")
	}

	if path, err := dotfilesSemanticHelperPath(); err == nil {
		out.HelperPath = path
		out.Details = append(out.Details, "embedded AT-SPI helper available")
	} else {
		out.Missing = append(out.Missing, "embedded helper")
		out.Details = append(out.Details, err.Error())
	}

	if out.PythonAvailable {
		cmd := exec.Command("python3", "-c", "import pyatspi")
		if helperPath := strings.TrimSpace(out.HelperPath); helperPath != "" {
			cmd.Env = append(os.Environ(), "DOTFILES_ATSPI_HELPER="+helperPath)
		}
		if raw, err := cmd.CombinedOutput(); err == nil {
			out.PyATSPIAvailable = true
			out.Details = append(out.Details, "pyatspi import succeeded")
		} else {
			out.Missing = append(out.Missing, "pyatspi")
			trimmed := strings.TrimSpace(string(raw))
			if trimmed == "" {
				trimmed = err.Error()
			}
			out.Details = append(out.Details, "pyatspi import failed: "+trimmed)
		}
	}

	if out.WaylandDisplay != "" {
		out.Details = append(out.Details, "WAYLAND_DISPLAY="+out.WaylandDisplay)
	} else {
		out.Details = append(out.Details, "WAYLAND_DISPLAY not detected")
	}
	if out.BusAddressPresent {
		out.Details = append(out.Details, "DBUS_SESSION_BUS_ADDRESS present")
	} else {
		out.Details = append(out.Details, "DBUS_SESSION_BUS_ADDRESS not detected")
	}

	out.Missing = uniqueSortedStrings(out.Missing)
	out.Ready = out.PythonAvailable && out.PyATSPIAvailable && out.HelperPath != ""
	return out
}

func runDesktopSemanticHelper(ctx context.Context, args ...string) (any, string, error) {
	return runDesktopSemanticHelperWithEnv(ctx, os.Environ(), args...)
}

func runDesktopSemanticHelperWithEnv(ctx context.Context, env []string, args ...string) (any, string, error) {
	caps := desktopSemanticCapabilities()
	if !caps.Ready {
		return nil, caps.HelperPath, fmt.Errorf("AT-SPI not ready: %s", strings.Join(caps.Missing, ", "))
	}

	cmdArgs := append([]string{caps.HelperPath}, args...)
	cmd := exec.CommandContext(ctx, "python3", cmdArgs...)
	if len(env) > 0 {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return nil, caps.HelperPath, fmt.Errorf("AT-SPI helper failed: %w: %s", err, trimmed)
	}

	var parsed any
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, caps.HelperPath, fmt.Errorf("parse AT-SPI helper output: %w", err)
	}
	return parsed, caps.HelperPath, nil
}

func semanticMapValue(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func semanticMapSliceValue(v any) []map[string]any {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if mapped := semanticMapValue(item); mapped != nil {
			out = append(out, mapped)
		}
	}
	return out
}

func semanticErrorValue(v map[string]any) string {
	if v == nil {
		return ""
	}
	if s, ok := v["error"].(string); ok {
		return s
	}
	return ""
}

func semanticQueryArgs(input desktopSemanticQueryInput) ([]string, error) {
	if strings.TrimSpace(input.App) == "" {
		return nil, fmt.Errorf("[%s] app is required", handler.ErrInvalidParam)
	}
	if strings.TrimSpace(input.Name) == "" && strings.TrimSpace(input.Path) == "" && strings.TrimSpace(input.Ref) == "" {
		return nil, fmt.Errorf("[%s] name, path, or ref is required", handler.ErrInvalidParam)
	}

	args := []string{"--app", input.App}
	if strings.TrimSpace(input.Name) != "" {
		args = append(args, "--name", input.Name)
	}
	if strings.TrimSpace(input.Role) != "" {
		args = append(args, "--role", input.Role)
	}
	if strings.TrimSpace(input.Ref) != "" {
		args = append(args, "--ref", input.Ref)
	}
	if strings.TrimSpace(input.Path) != "" {
		args = append(args, "--path", input.Path)
	}
	for _, state := range input.States {
		if strings.TrimSpace(state) != "" {
			args = append(args, "--state", state)
		}
	}
	if input.Limit > 0 {
		args = append(args, "--limit", strconv.Itoa(input.Limit))
	}
	if input.Exact {
		args = append(args, "--exact")
	}
	return args, nil
}

func desktopSemanticQueryFromTextInput(input desktopSemanticTextInput) desktopSemanticQueryInput {
	return desktopSemanticQueryInput{
		App:    input.App,
		Name:   input.Name,
		Role:   input.Role,
		Ref:    input.Ref,
		Path:   input.Path,
		States: input.States,
		Exact:  input.Exact,
	}
}

func desktopSemanticQueryFromValueInput(input desktopSemanticValueInput) desktopSemanticQueryInput {
	return desktopSemanticQueryInput{
		App:    input.App,
		Name:   input.Name,
		Role:   input.Role,
		Ref:    input.Ref,
		Path:   input.Path,
		States: input.States,
		Exact:  input.Exact,
	}
}

func desktopSemanticElementFromResult(helperPath string, query desktopSemanticQueryInput, result map[string]any) desktopSemanticElementOutput {
	return desktopSemanticElementOutput{
		HelperPath: helperPath,
		App:        query.App,
		Query:      query,
		Matched:    boolValue(result["matched"]),
		Clicked:    boolValue(result["clicked"]),
		Invoked:    boolValue(result["invoked"]),
		Focused:    boolValue(result["focused"]),
		Updated:    boolValue(result["updated"]),
		Action:     stringValue(result["action"]),
		Value:      result["value"],
		ValueKind:  stringValue(result["value_kind"]),
		Element:    semanticMapValue(result["element"]),
		Error:      semanticErrorValue(result),
	}
}

func desktopResolveSemanticWindow(ctx context.Context, input desktopSemanticWindowInput) (desktopSemanticQueryInput, string, error) {
	appName := strings.TrimSpace(input.App)
	if appName != "" && (strings.TrimSpace(input.Ref) != "" || strings.TrimSpace(input.Path) != "") {
		return desktopSemanticQueryInput{
			App:  appName,
			Ref:  strings.TrimSpace(input.Ref),
			Path: strings.TrimSpace(input.Path),
		}, "", nil
	}

	args := []string{"list_windows"}
	if appName != "" {
		args = append(args, "--app", appName)
	}
	parsed, helperPath, err := runDesktopSemanticHelper(ctx, args...)
	if err != nil {
		return desktopSemanticQueryInput{}, helperPath, err
	}

	result := semanticMapValue(parsed)
	windows := semanticMapSliceValue(result["windows"])
	for _, window := range windows {
		app := semanticMapValue(window["app"])
		windowTitle := strings.ToLower(stringValue(window["name"]))
		windowApp := strings.ToLower(stringValue(app["name"]))
		if title := strings.TrimSpace(strings.ToLower(input.TitleContains)); title != "" && strings.Contains(windowTitle, title) {
			return desktopSemanticQueryInput{
				App:  stringValue(app["name"]),
				Ref:  stringValue(window["ref"]),
				Path: stringValue(window["path"]),
			}, helperPath, nil
		}
		if class := strings.TrimSpace(strings.ToLower(input.Class)); class != "" && strings.Contains(windowApp, class) {
			return desktopSemanticQueryInput{
				App:  stringValue(app["name"]),
				Ref:  stringValue(window["ref"]),
				Path: stringValue(window["path"]),
			}, helperPath, nil
		}
	}

	if strings.TrimSpace(input.TitleContains) != "" {
		return desktopSemanticQueryInput{}, helperPath, fmt.Errorf("no semantic window title matched %q", input.TitleContains)
	}
	if strings.TrimSpace(input.Class) != "" {
		return desktopSemanticQueryInput{}, helperPath, fmt.Errorf("no semantic window app matched %q", input.Class)
	}
	if appName != "" {
		return desktopSemanticQueryInput{}, helperPath, fmt.Errorf("app %q requires a semantic ref or path", appName)
	}
	return desktopSemanticQueryInput{}, helperPath, fmt.Errorf("[%s] title_contains, class, or app+ref/path is required", handler.ErrInvalidParam)
}

func (m *DesktopSemanticModule) Tools() []registry.ToolDefinition {
	snapshot := handler.TypedHandler[EmptyInput, desktopSemanticSnapshotOutput](
		"desktop_snapshot",
		"List running AT-SPI applications and high-level semantic metadata before targeting a window.",
		func(ctx context.Context, _ EmptyInput) (desktopSemanticSnapshotOutput, error) {
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, "list_apps")
			if err != nil {
				return desktopSemanticSnapshotOutput{}, err
			}
			result := semanticMapValue(parsed)
			return desktopSemanticSnapshotOutput{
				HelperPath: helperPath,
				Apps:       semanticMapSliceValue(result["apps"]),
			}, nil
		},
	)
	snapshot.Category = "desktop"
	snapshot.SearchTerms = []string{"semantic desktop", "accessibility tree", "at-spi apps", "desktop snapshot"}

	listWindows := handler.TypedHandler[desktopSemanticWindowsInput, desktopSemanticWindowsOutput](
		"desktop_list_windows",
		"List semantic desktop windows with their app identity, refs, bounds, and current value metadata.",
		func(ctx context.Context, input desktopSemanticWindowsInput) (desktopSemanticWindowsOutput, error) {
			args := []string{"list_windows"}
			if strings.TrimSpace(input.App) != "" {
				args = append(args, "--app", input.App)
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, args...)
			if err != nil {
				return desktopSemanticWindowsOutput{}, err
			}
			result := semanticMapValue(parsed)
			windows := semanticMapSliceValue(result["windows"])
			return desktopSemanticWindowsOutput{
				HelperPath: helperPath,
				App:        strings.TrimSpace(input.App),
				Count:      len(windows),
				Windows:    windows,
				Error:      semanticErrorValue(result),
			}, nil
		},
	)
	listWindows.Category = "desktop"
	listWindows.SearchTerms = []string{"semantic windows", "desktop windows", "list accessibility windows", "at-spi windows"}

	targetWindows := handler.TypedHandler[desktopSemanticTreeInput, desktopSemanticTreeOutput](
		"desktop_target_windows",
		"Read the semantic accessibility tree for one application, bounded by depth.",
		func(ctx context.Context, input desktopSemanticTreeInput) (desktopSemanticTreeOutput, error) {
			if strings.TrimSpace(input.App) == "" {
				return desktopSemanticTreeOutput{}, fmt.Errorf("[%s] app is required", handler.ErrInvalidParam)
			}
			depth := input.Depth
			if depth <= 0 {
				depth = 5
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, "get_tree", "--app", input.App, "--depth", strconv.Itoa(depth))
			if err != nil {
				return desktopSemanticTreeOutput{}, err
			}
			result := semanticMapValue(parsed)
			return desktopSemanticTreeOutput{
				HelperPath: helperPath,
				App:        input.App,
				Depth:      depth,
				Matched:    semanticErrorValue(result) == "",
				Tree:       semanticMapValue(result["tree"]),
				Error:      semanticErrorValue(result),
			}, nil
		},
	)
	targetWindows.Category = "desktop"
	targetWindows.SearchTerms = []string{"semantic tree", "window tree", "accessibility tree", "target window"}

	find := handler.TypedHandler[desktopSemanticQueryInput, desktopSemanticElementOutput](
		"desktop_find",
		"Find a semantic element by app plus name, role, states, ref, or a previously returned path.",
		func(ctx context.Context, input desktopSemanticQueryInput) (desktopSemanticElementOutput, error) {
			args, err := semanticQueryArgs(input)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"find"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			result := semanticMapValue(parsed)
			return desktopSemanticElementFromResult(helperPath, input, result), nil
		},
	)
	find.Category = "desktop"
	find.SearchTerms = []string{"find semantic element", "find button", "accessibility search", "at-spi find", "semantic ref"}

	findAll := handler.TypedHandler[desktopSemanticQueryInput, desktopSemanticMatchesOutput](
		"desktop_find_all",
		"Find all semantic elements matching an app plus name, role, states, ref, or path.",
		func(ctx context.Context, input desktopSemanticQueryInput) (desktopSemanticMatchesOutput, error) {
			args, err := semanticQueryArgs(input)
			if err != nil {
				return desktopSemanticMatchesOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"find_all"}, args...)...)
			if err != nil {
				return desktopSemanticMatchesOutput{}, err
			}
			result := semanticMapValue(parsed)
			return desktopSemanticMatchesOutput{
				HelperPath: helperPath,
				App:        input.App,
				Query:      input,
				Matched:    result["matched"] == true,
				Count:      intValue(result["count"]),
				Matches:    semanticMapSliceValue(result["elements"]),
				Error:      semanticErrorValue(result),
			}, nil
		},
	)
	findAll.Category = "desktop"
	findAll.SearchTerms = []string{"find all semantic elements", "list matching buttons", "at-spi multiple matches"}

	focusWindow := handler.TypedHandler[desktopSemanticWindowInput, desktopSemanticElementOutput](
		"desktop_focus_window",
		"Focus a top-level semantic desktop window by title, app/class substring, or a previously returned semantic ref/path.",
		func(ctx context.Context, input desktopSemanticWindowInput) (desktopSemanticElementOutput, error) {
			query, _, err := desktopResolveSemanticWindow(ctx, input)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"focus"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			return desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed)), nil
		},
	)
	focusWindow.Category = "desktop"
	focusWindow.SearchTerms = []string{"focus semantic window", "focus desktop window", "at-spi window focus", "window title focus"}

	focus := handler.TypedHandler[desktopSemanticQueryInput, desktopSemanticElementOutput](
		"desktop_focus",
		"Focus a semantic element by ref, path, or by app plus name and optional role, using AT-SPI component focus or a focus action fallback.",
		func(ctx context.Context, input desktopSemanticQueryInput) (desktopSemanticElementOutput, error) {
			args, err := semanticQueryArgs(input)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"focus"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			return desktopSemanticElementFromResult(helperPath, input, semanticMapValue(parsed)), nil
		},
	)
	focus.Category = "desktop"
	focus.SearchTerms = []string{"focus semantic element", "focus window control", "at-spi focus", "grab focus"}

	readValue := handler.TypedHandler[desktopSemanticQueryInput, desktopSemanticElementOutput](
		"desktop_read_value",
		"Read the current text or numeric value from a semantic element, such as an entry, label, slider, or spinbox.",
		func(ctx context.Context, input desktopSemanticQueryInput) (desktopSemanticElementOutput, error) {
			args, err := semanticQueryArgs(input)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"read_value"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			return desktopSemanticElementFromResult(helperPath, input, semanticMapValue(parsed)), nil
		},
	)
	readValue.Category = "desktop"
	readValue.SearchTerms = []string{"read semantic value", "read text field", "read slider value", "at-spi value"}

	setText := handler.TypedHandler[desktopSemanticTextInput, desktopSemanticElementOutput](
		"desktop_set_text",
		"Set the text contents of an editable semantic element. Pass an empty string to clear the field.",
		func(ctx context.Context, input desktopSemanticTextInput) (desktopSemanticElementOutput, error) {
			if input.Text == nil {
				return desktopSemanticElementOutput{}, fmt.Errorf("[%s] text is required", handler.ErrInvalidParam)
			}
			query := desktopSemanticQueryFromTextInput(input)
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			args = append(args, "--text", *input.Text)
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"set_text"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			return desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed)), nil
		},
	)
	setText.Category = "desktop"
	setText.SearchTerms = []string{"set semantic text", "fill input field", "replace entry text", "clear text field"}

	setValue := handler.TypedHandler[desktopSemanticValueInput, desktopSemanticElementOutput](
		"desktop_set_value",
		"Set the numeric value of a semantic element such as a slider, dial, or spinbox.",
		func(ctx context.Context, input desktopSemanticValueInput) (desktopSemanticElementOutput, error) {
			if input.Value == nil {
				return desktopSemanticElementOutput{}, fmt.Errorf("[%s] value is required", handler.ErrInvalidParam)
			}
			query := desktopSemanticQueryFromValueInput(input)
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			args = append(args, "--value", strconv.FormatFloat(*input.Value, 'f', -1, 64))
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"set_value"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			return desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed)), nil
		},
	)
	setValue.Category = "desktop"
	setValue.SearchTerms = []string{"set semantic value", "set slider", "set spinbox", "adjust accessible value"}

	click := handler.TypedHandler[desktopSemanticQueryInput, desktopSemanticElementOutput](
		"desktop_click",
		"Invoke the default clickable semantic action by ref, path, or by app plus name and optional role.",
		func(ctx context.Context, input desktopSemanticQueryInput) (desktopSemanticElementOutput, error) {
			args, err := semanticQueryArgs(input)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"click"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			return desktopSemanticElementFromResult(helperPath, input, semanticMapValue(parsed)), nil
		},
	)
	click.Category = "desktop"
	click.SearchTerms = []string{"semantic click", "click button by label", "at-spi click", "invoke default action"}

	act := handler.TypedHandler[desktopSemanticActionInput, desktopSemanticElementOutput](
		"desktop_act",
		"Invoke a specific AT-SPI action on a semantic element, such as activate, press, select, or grab focus.",
		func(ctx context.Context, input desktopSemanticActionInput) (desktopSemanticElementOutput, error) {
			query := desktopSemanticQueryInput{
				App:    input.App,
				Name:   input.Name,
				Role:   input.Role,
				Ref:    input.Ref,
				Path:   input.Path,
				States: input.States,
				Exact:  input.Exact,
			}
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			if strings.TrimSpace(input.Action) != "" {
				args = append(args, "--action", input.Action)
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"act"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			return desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed)), nil
		},
	)
	act.Category = "desktop"
	act.SearchTerms = []string{"semantic action", "invoke at-spi action", "focus widget", "activate widget"}

	wait := handler.TypedHandler[desktopSemanticWaitInput, desktopSemanticElementOutput](
		"desktop_wait_for_element",
		"Wait for a semantic element to appear or satisfy the requested states, then return its resolved element record.",
		func(ctx context.Context, input desktopSemanticWaitInput) (desktopSemanticElementOutput, error) {
			query := desktopSemanticQueryInput{
				App:    input.App,
				Name:   input.Name,
				Role:   input.Role,
				Ref:    input.Ref,
				Path:   input.Path,
				States: input.States,
				Exact:  input.Exact,
			}
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			timeout := input.Timeout
			if timeout <= 0 {
				timeout = 5
			}
			args = append(args, "--timeout", strconv.Itoa(timeout))
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"wait"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			return desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed)), nil
		},
	)
	wait.Category = "desktop"
	wait.SearchTerms = []string{"wait for button", "wait for dialog", "wait semantic element", "wait for state"}

	typeText := handler.TypedHandler[TypeTextInput, string](
		"desktop_type",
		"Type text as a semantic desktop follow-up after a semantic target has focus.",
		func(_ context.Context, input TypeTextInput) (string, error) {
			if strings.TrimSpace(input.Text) == "" {
				return "", fmt.Errorf("[%s] text must not be empty", handler.ErrInvalidParam)
			}
			if hasCmd("wtype") {
				out, err := exec.Command("wtype", input.Text).CombinedOutput()
				if err != nil {
					return "", fmt.Errorf("wtype failed: %w: %s", err, strings.TrimSpace(string(out)))
				}
				return fmt.Sprintf("typed %d chars with wtype", len(input.Text)), nil
			}
			if hasCmd("ydotool") {
				out, err := exec.Command("ydotool", "type", "--key-delay", "0", input.Text).CombinedOutput()
				if err != nil {
					return "", fmt.Errorf("ydotool type failed: %w: %s", err, strings.TrimSpace(string(out)))
				}
				return fmt.Sprintf("typed %d chars with ydotool", len(input.Text)), nil
			}
			return "", fmt.Errorf("neither wtype nor ydotool is available")
		},
	)
	typeText.Category = "desktop"
	typeText.SearchTerms = []string{"semantic type", "type into focused field", "desktop type"}

	key := handler.TypedHandler[KeyInput, string](
		"desktop_key",
		"Send raw ydotool key events after a semantic target has focus.",
		func(_ context.Context, input KeyInput) (string, error) {
			if strings.TrimSpace(input.Keys) == "" {
				return "", fmt.Errorf("[%s] keys must not be empty", handler.ErrInvalidParam)
			}
			if !hasCmd("ydotool") {
				return "", fmt.Errorf("ydotool is required for desktop_key")
			}
			args := append([]string{"key"}, strings.Fields(input.Keys)...)
			out, err := exec.Command("ydotool", args...).CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("ydotool key failed: %w: %s", err, strings.TrimSpace(string(out)))
			}
			return fmt.Sprintf("sent keys: %s", input.Keys), nil
		},
	)
	key.Category = "desktop"
	key.SearchTerms = []string{"semantic key", "desktop keypress", "ydotool key"}

	capabilities := handler.TypedHandler[EmptyInput, desktopSemanticCapabilitiesOutput](
		"desktop_capabilities",
		"Check whether the embedded AT-SPI helper, python3, and pyatspi are available on this host.",
		func(_ context.Context, _ EmptyInput) (desktopSemanticCapabilitiesOutput, error) {
			return desktopSemanticCapabilities(), nil
		},
	)
	capabilities.Category = "desktop"
	capabilities.SearchTerms = []string{"AT-SPI status", "semantic desktop readiness", "desktop capabilities"}

	return []registry.ToolDefinition{
		snapshot,
		listWindows,
		targetWindows,
		find,
		findAll,
		focusWindow,
		focus,
		readValue,
		setText,
		setValue,
		click,
		act,
		wait,
		typeText,
		key,
		capabilities,
	}
}
