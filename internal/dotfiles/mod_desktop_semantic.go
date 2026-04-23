package dotfiles

import (
	"context"
	_ "embed"
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

const (
	semanticLookupDepth  = 12
	semanticDefaultLimit = 20
	semanticPollInterval = 350 * time.Millisecond
)

type desktopSemanticFormFieldsInput struct {
	App            string `json:"app" jsonschema:"required,description=Application name or unique substring"`
	Ref            string `json:"ref,omitempty" jsonschema:"description=Optional semantic reference that scopes discovery to one subtree, such as ref_0_2_1"`
	Path           string `json:"path,omitempty" jsonschema:"description=Optional child-index path that scopes discovery to one subtree, such as 0/2/1"`
	Depth          int    `json:"depth,omitempty" jsonschema:"description=Max AT-SPI tree depth to inspect before deriving fields (default 8)"`
	Limit          int    `json:"limit,omitempty" jsonschema:"description=Optional max number of form fields to return"`
	IncludeActions bool   `json:"include_actions,omitempty" jsonschema:"description=Include action-oriented controls such as buttons and combo boxes in addition to editable fields"`
}

type semanticFormFillItemInput struct {
	Name    string   `json:"name,omitempty" jsonschema:"description=Semantic field label, name, description, or placeholder text to match"`
	Role    string   `json:"role,omitempty" jsonschema:"description=Optional exact AT-SPI role filter"`
	Ref     string   `json:"ref,omitempty" jsonschema:"description=Optional semantic reference such as ref_0_2_1 from a previous semantic result"`
	Path    string   `json:"path,omitempty" jsonschema:"description=Optional child-index path such as 0/2/1 from a previous semantic result"`
	Exact   bool     `json:"exact,omitempty" jsonschema:"description=Require exact case-insensitive label or name matching"`
	Text    *string  `json:"text,omitempty" jsonschema:"description=Text to write into an editable semantic field"`
	Number  *float64 `json:"number,omitempty" jsonschema:"description=Numeric value to write into a slider, spinbox, or similar field"`
	Checked *bool    `json:"checked,omitempty" jsonschema:"description=Desired toggle state for a checkbox, radio button, switch, or toggle button"`
	Action  string   `json:"action,omitempty" jsonschema:"description=Explicit AT-SPI action name to invoke, such as activate, press, or select"`
}

type desktopSemanticFormFillInput struct {
	App             string                      `json:"app" jsonschema:"required,description=Application name or unique substring"`
	ScopeRef        string                      `json:"scope_ref,omitempty" jsonschema:"description=Optional semantic reference that scopes matching to one subtree, such as ref_0_2_1"`
	ScopePath       string                      `json:"scope_path,omitempty" jsonschema:"description=Optional child-index path that scopes matching to one subtree, such as 0/2/1"`
	Depth           int                         `json:"depth,omitempty" jsonschema:"description=Max AT-SPI tree depth to inspect before resolving form fields (default 8)"`
	Preview         bool                        `json:"preview,omitempty" jsonschema:"description=When true, resolve targets and strategies without mutating the UI"`
	ContinueOnError *bool                       `json:"continue_on_error,omitempty" jsonschema:"description=Continue filling remaining fields after an error. Defaults to true."`
	Fields          []semanticFormFillItemInput `json:"fields" jsonschema:"required,description=Fields to fill or actions to invoke"`
}

type semanticFormField struct {
	Name         string              `json:"name,omitempty"`
	Description  string              `json:"description,omitempty"`
	Role         string              `json:"role,omitempty"`
	FieldType    string              `json:"field_type,omitempty"`
	FillStrategy string              `json:"fill_strategy,omitempty"`
	Ref          string              `json:"ref,omitempty"`
	Path         string              `json:"path,omitempty"`
	Value        any                 `json:"value,omitempty"`
	ValueKind    string              `json:"value_kind,omitempty"`
	Labels       []string            `json:"labels,omitempty"`
	States       []string            `json:"states,omitempty"`
	Actions      []string            `json:"actions,omitempty"`
	Attributes   map[string]string   `json:"attributes,omitempty"`
	Relations    map[string][]string `json:"relations,omitempty"`
	Element      map[string]any      `json:"element,omitempty"`
}

type desktopSemanticFormFieldsOutput struct {
	HelperPath     string              `json:"helper_path,omitempty"`
	App            string              `json:"app"`
	ScopeRef       string              `json:"scope_ref,omitempty"`
	ScopePath      string              `json:"scope_path,omitempty"`
	Depth          int                 `json:"depth"`
	IncludeActions bool                `json:"include_actions"`
	Count          int                 `json:"count"`
	Fields         []semanticFormField `json:"fields,omitempty"`
	Error          string              `json:"error,omitempty"`
}

type semanticFormFillItemOutput struct {
	Request      semanticFormFillItemInput `json:"request"`
	Matched      bool                      `json:"matched"`
	Planned      bool                      `json:"planned,omitempty"`
	Applied      bool                      `json:"applied,omitempty"`
	Strategy     string                    `json:"strategy,omitempty"`
	FieldType    string                    `json:"field_type,omitempty"`
	Field        *semanticFormField        `json:"field,omitempty"`
	Element      map[string]any            `json:"element,omitempty"`
	CurrentValue any                       `json:"current_value,omitempty"`
	Error        string                    `json:"error,omitempty"`
}

type desktopSemanticFormFillOutput struct {
	HelperPath      string                       `json:"helper_path,omitempty"`
	App             string                       `json:"app"`
	ScopeRef        string                       `json:"scope_ref,omitempty"`
	ScopePath       string                       `json:"scope_path,omitempty"`
	Depth           int                          `json:"depth"`
	Preview         bool                         `json:"preview"`
	ContinueOnError bool                         `json:"continue_on_error"`
	Requested       int                          `json:"requested"`
	Matched         int                          `json:"matched"`
	Planned         int                          `json:"planned"`
	Applied         int                          `json:"applied"`
	Results         []semanticFormFillItemOutput `json:"results,omitempty"`
	Error           string                       `json:"error,omitempty"`
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
	waylandDisplay := strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY"))
	if waylandDisplay == "" {
		waylandDisplay = dotfilesWaylandDisplay(dotfilesRuntimeDir())
	}
	out := desktopSemanticCapabilitiesOutput{
		WaylandDisplay:    waylandDisplay,
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

type semanticRunner func(ctx context.Context, args ...string) (any, string, error)

func semanticStringSlice(v any) []string {
	switch raw := v.(type) {
	case []string:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if trimmed := strings.TrimSpace(stringValue(item)); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	default:
		return nil
	}
}

func semanticStringMapValue(v any) map[string]string {
	switch raw := v.(type) {
	case map[string]string:
		if len(raw) == 0 {
			return nil
		}
		out := make(map[string]string, len(raw))
		for key, value := range raw {
			if strings.TrimSpace(key) == "" {
				continue
			}
			out[key] = strings.TrimSpace(value)
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case map[string]any:
		if len(raw) == 0 {
			return nil
		}
		out := make(map[string]string, len(raw))
		for key, value := range raw {
			if strings.TrimSpace(key) == "" {
				continue
			}
			out[key] = strings.TrimSpace(stringValue(value))
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func semanticStringSlicesMapValue(v any) map[string][]string {
	raw, ok := v.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string][]string, len(raw))
	for key, value := range raw {
		items := semanticStringSlice(value)
		if len(items) == 0 {
			continue
		}
		out[key] = items
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func semanticNormalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func semanticRefToPath(ref string) string {
	value := semanticNormalize(ref)
	if value == "" {
		return ""
	}
	if value == "ref_root" {
		return ""
	}
	if strings.HasPrefix(value, "ref_") {
		return strings.ReplaceAll(ref[4:], "_", "/")
	}
	return ""
}

func semanticElementPath(element map[string]any) string {
	if element == nil {
		return ""
	}
	if path := strings.TrimSpace(stringValue(element["path"])); path != "" {
		return path
	}
	return semanticRefToPath(stringValue(element["ref"]))
}

func semanticElementChildren(element map[string]any) []map[string]any {
	if element == nil {
		return nil
	}
	return semanticMapSliceValue(element["children"])
}

func semanticLocateElement(root map[string]any, ref string, path string) map[string]any {
	if root == nil {
		return nil
	}
	targetPath := strings.TrimSpace(path)
	if targetPath == "" {
		targetPath = semanticRefToPath(ref)
	}
	if targetPath == "" {
		return root
	}
	current := root
	for _, raw := range strings.Split(targetPath, "/") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return nil
		}
		children := semanticElementChildren(current)
		if idx < 0 || idx >= len(children) {
			return nil
		}
		current = children[idx]
	}
	return current
}

func semanticCollectElements(root map[string]any) []map[string]any {
	if root == nil {
		return nil
	}
	out := []map[string]any{root}
	for _, child := range semanticElementChildren(root) {
		out = append(out, semanticCollectElements(child)...)
	}
	return out
}

func semanticUniqueStringsPreserveOrder(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := semanticNormalize(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func semanticRelationLabelValues(relations map[string][]string) []string {
	if len(relations) == 0 {
		return nil
	}
	keys := make([]string, 0, len(relations))
	for key := range relations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	labels := make([]string, 0)
	for _, key := range keys {
		normKey := semanticNormalize(key)
		if !strings.Contains(normKey, "label") && !strings.Contains(normKey, "describ") {
			continue
		}
		labels = append(labels, relations[key]...)
	}
	return semanticUniqueStringsPreserveOrder(labels)
}

func semanticElementLabels(element map[string]any) []string {
	if element == nil {
		return nil
	}
	labels := []string{
		stringValue(element["name"]),
		stringValue(element["description"]),
	}
	relations := semanticStringSlicesMapValue(element["relations"])
	labels = append(labels, semanticRelationLabelValues(relations)...)
	attributes := semanticStringMapValue(element["attributes"])
	for _, key := range []string{
		"label",
		"accessible-name",
		"accessible_name",
		"accessible-description",
		"accessible_description",
		"aria-label",
		"aria-labelledby",
		"aria-description",
		"aria-describedby",
		"title",
		"placeholder",
		"placeholder-text",
		"placeholder_text",
		"placeholder-value",
		"description",
		"tooltip",
		"tooltip-text",
		"tool-tip",
		"help-text",
		"help",
		"hint",
		"labelled-by",
		"described-by",
	} {
		if value := strings.TrimSpace(attributes[key]); value != "" {
			labels = append(labels, value)
		}
	}
	return semanticUniqueStringsPreserveOrder(labels)
}

func semanticRoleMatches(role string, want string) bool {
	if strings.TrimSpace(want) == "" {
		return true
	}
	return semanticNormalize(role) == semanticNormalize(want)
}

func semanticHasAction(actions []string, terms ...string) bool {
	if len(actions) == 0 {
		return false
	}
	for _, action := range actions {
		normAction := semanticNormalize(action)
		for _, term := range terms {
			if strings.Contains(normAction, semanticNormalize(term)) {
				return true
			}
		}
	}
	return false
}

func semanticFieldSummary(element map[string]any, includeActions bool) (semanticFormField, bool) {
	field := semanticFormField{
		Name:        stringValue(element["name"]),
		Description: stringValue(element["description"]),
		Role:        stringValue(element["role"]),
		Ref:         stringValue(element["ref"]),
		Path:        semanticElementPath(element),
		Value:       element["value"],
		ValueKind:   stringValue(element["value_kind"]),
		Labels:      semanticElementLabels(element),
		States:      semanticStringSlice(element["states"]),
		Actions:     semanticStringSlice(element["actions"]),
		Attributes:  semanticStringMapValue(element["attributes"]),
		Relations:   semanticStringSlicesMapValue(element["relations"]),
		Element:     element,
	}

	role := semanticNormalize(field.Role)
	switch {
	case field.ValueKind == "text",
		role == "entry",
		role == "password text",
		role == "text",
		role == "document text",
		role == "search box":
		field.FieldType = "text"
		field.FillStrategy = "text"
		return field, true
	case field.ValueKind == "numeric",
		role == "slider",
		role == "spin button",
		role == "spinbutton",
		role == "dial",
		role == "scroll bar",
		role == "level bar":
		field.FieldType = "numeric"
		field.FillStrategy = "value"
		return field, true
	case role == "check box",
		role == "check menu item",
		role == "radio button",
		role == "toggle button",
		role == "switch":
		field.FieldType = "toggle"
		field.FillStrategy = "toggle"
		return field, true
	case includeActions && (role == "combo box" ||
		role == "list box" ||
		role == "push button" ||
		role == "button" ||
		role == "menu item" ||
		semanticHasAction(field.Actions, "press", "activate", "click", "select", "open")):
		field.FieldType = "action"
		field.FillStrategy = "action"
		return field, true
	default:
		return field, false
	}
}

func semanticCollectFormFields(root map[string]any, includeActions bool, limit int) []semanticFormField {
	elements := semanticCollectElements(root)
	fields := make([]semanticFormField, 0)
	for _, element := range elements {
		field, ok := semanticFieldSummary(element, includeActions)
		if !ok {
			continue
		}
		fields = append(fields, field)
		if limit > 0 && len(fields) >= limit {
			break
		}
	}
	return fields
}

func semanticMatchScore(values []string, needle string, exact bool) int {
	target := semanticNormalize(needle)
	if target == "" {
		return 0
	}
	best := 0
	for _, value := range values {
		candidate := semanticNormalize(value)
		if candidate == "" {
			continue
		}
		switch {
		case exact && candidate == target:
			return 100
		case exact:
			continue
		case candidate == target:
			if best < 100 {
				best = 100
			}
		case strings.HasPrefix(candidate, target):
			if best < 80 {
				best = 80
			}
		case strings.Contains(candidate, target):
			if best < 60 {
				best = 60
			}
		}
	}
	return best
}

func semanticResolveRequestElement(root map[string]any, fields []semanticFormField, request semanticFormFillItemInput) (*semanticFormField, map[string]any) {
	if root == nil {
		return nil, nil
	}
	if strings.TrimSpace(request.Ref) != "" || strings.TrimSpace(request.Path) != "" {
		element := semanticLocateElement(root, request.Ref, request.Path)
		if element == nil || !semanticRoleMatches(stringValue(element["role"]), request.Role) {
			return nil, nil
		}
		field, ok := semanticFieldSummary(element, true)
		if ok {
			return &field, element
		}
		return nil, element
	}

	target := strings.TrimSpace(request.Name)
	if target == "" {
		return nil, nil
	}

	var bestField *semanticFormField
	bestScore := 0
	for i := range fields {
		field := &fields[i]
		if !semanticRoleMatches(field.Role, request.Role) {
			continue
		}
		score := semanticMatchScore(field.Labels, target, request.Exact)
		if score > bestScore {
			bestScore = score
			bestField = field
		}
	}
	if bestField != nil {
		return bestField, bestField.Element
	}

	elements := semanticCollectElements(root)
	var bestElement map[string]any
	for _, element := range elements {
		if !semanticRoleMatches(stringValue(element["role"]), request.Role) {
			continue
		}
		score := semanticMatchScore(semanticElementLabels(element), target, request.Exact)
		if score > bestScore {
			bestScore = score
			bestElement = element
		}
	}
	if bestElement == nil {
		return nil, nil
	}
	field, ok := semanticFieldSummary(bestElement, true)
	if ok {
		return &field, bestElement
	}
	return nil, bestElement
}

func semanticResolveScope(root map[string]any, ref string, path string) (map[string]any, error) {
	if root == nil {
		return nil, fmt.Errorf("semantic tree is empty")
	}
	if strings.TrimSpace(ref) == "" && strings.TrimSpace(path) == "" {
		return root, nil
	}
	element := semanticLocateElement(root, ref, path)
	if element == nil {
		return nil, fmt.Errorf("semantic scope not found for ref=%q path=%q", ref, path)
	}
	return element, nil
}

func semanticOperationForRequest(request semanticFormFillItemInput) (string, error) {
	count := 0
	op := ""
	if request.Text != nil {
		count++
		op = "text"
	}
	if request.Number != nil {
		count++
		op = "number"
	}
	if request.Checked != nil {
		count++
		op = "checked"
	}
	if strings.TrimSpace(request.Action) != "" {
		count++
		op = "action"
	}
	switch {
	case count == 0:
		return "", fmt.Errorf("[%s] each form request needs one of text, number, checked, or action", handler.ErrInvalidParam)
	case count > 1:
		return "", fmt.Errorf("[%s] each form request can set only one of text, number, checked, or action", handler.ErrInvalidParam)
	default:
		return op, nil
	}
}

func semanticElementCheckedState(element map[string]any) (bool, bool) {
	role := semanticNormalize(stringValue(element["role"]))
	states := semanticStringSlice(element["states"])
	for _, state := range states {
		switch semanticNormalize(state) {
		case "checked", "selected", "pressed", "expanded", "on":
			return true, true
		}
	}
	attributes := semanticStringMapValue(element["attributes"])
	for _, key := range []string{"checked", "selected", "pressed", "expanded", "toggle-state", "aria-checked"} {
		switch semanticNormalize(attributes[key]) {
		case "true", "1", "checked", "selected", "pressed", "expanded", "on":
			return true, true
		case "false", "0", "unchecked", "unselected", "off":
			return false, true
		}
	}
	switch role {
	case "check box", "check menu item", "radio button", "toggle button", "switch":
		return false, true
	default:
		return false, false
	}
}

func semanticQueryForElement(app string, element map[string]any, role string) desktopSemanticQueryInput {
	query := desktopSemanticQueryInput{
		App:  app,
		Role: strings.TrimSpace(role),
		Ref:  strings.TrimSpace(stringValue(element["ref"])),
		Path: semanticElementPath(element),
	}
	if query.Role == "" {
		query.Role = stringValue(element["role"])
	}
	if strings.TrimSpace(query.Ref) == "" && strings.TrimSpace(query.Path) == "" {
		query.Name = stringValue(element["name"])
	}
	return query
}

func semanticResolvedQueryForElement(query desktopSemanticQueryInput, element map[string]any) desktopSemanticQueryInput {
	resolved := semanticQueryForElement(query.App, element, query.Role)
	resolved.States = append([]string(nil), query.States...)
	resolved.Limit = query.Limit
	resolved.Exact = query.Exact
	return resolved
}

func semanticFetchTree(ctx context.Context, runner semanticRunner, app string, depth int) (map[string]any, string, int, error) {
	if strings.TrimSpace(app) == "" {
		return nil, "", 0, fmt.Errorf("[%s] app is required", handler.ErrInvalidParam)
	}
	if depth <= 0 {
		depth = 8
	}
	parsed, helperPath, err := runner(ctx, "get_tree", "--app", app, "--depth", strconv.Itoa(depth))
	if err != nil {
		return nil, helperPath, depth, err
	}
	result := semanticMapValue(parsed)
	if errText := semanticErrorValue(result); errText != "" {
		return nil, helperPath, depth, fmt.Errorf("%s", errText)
	}
	return semanticMapValue(result["tree"]), helperPath, depth, nil
}

func semanticElementStateMatches(element map[string]any, requiredStates []string) bool {
	if len(requiredStates) == 0 {
		return true
	}
	current := make(map[string]struct{}, len(requiredStates))
	for _, state := range semanticStringSlice(element["states"]) {
		norm := semanticNormalize(state)
		if norm == "" {
			continue
		}
		current[norm] = struct{}{}
	}
	for _, state := range requiredStates {
		norm := semanticNormalize(state)
		if norm == "" {
			continue
		}
		if _, ok := current[norm]; !ok {
			return false
		}
	}
	return true
}

func semanticQueryNeedsTreeLookup(query desktopSemanticQueryInput) bool {
	return strings.TrimSpace(query.Name) != "" &&
		strings.TrimSpace(query.Ref) == "" &&
		strings.TrimSpace(query.Path) == ""
}

type semanticScoredElement struct {
	Element map[string]any
	Score   int
	Index   int
}

func semanticTreeLimit(limit int) int {
	if limit <= 0 {
		return semanticDefaultLimit
	}
	return limit
}

func semanticQueryMatchScore(element map[string]any, query desktopSemanticQueryInput) int {
	if element == nil {
		return 0
	}
	if !semanticRoleMatches(stringValue(element["role"]), query.Role) {
		return 0
	}
	if !semanticElementStateMatches(element, query.States) {
		return 0
	}
	if strings.TrimSpace(query.Name) == "" {
		return 1
	}
	return semanticMatchScore(semanticElementLabels(element), query.Name, query.Exact)
}

func semanticFindTreeMatches(root map[string]any, query desktopSemanticQueryInput, limit int) []map[string]any {
	if root == nil {
		return nil
	}
	elements := semanticCollectElements(root)
	scored := make([]semanticScoredElement, 0, len(elements))
	for idx, element := range elements {
		score := semanticQueryMatchScore(element, query)
		if score <= 0 {
			continue
		}
		scored = append(scored, semanticScoredElement{
			Element: element,
			Score:   score,
			Index:   idx,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Index < scored[j].Index
		}
		return scored[i].Score > scored[j].Score
	})
	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]map[string]any, 0, len(scored))
	for _, match := range scored {
		out = append(out, match.Element)
	}
	return out
}

func semanticFindMatchesWithRunner(ctx context.Context, runner semanticRunner, query desktopSemanticQueryInput) ([]map[string]any, string, error) {
	tree, helperPath, _, err := semanticFetchTree(ctx, runner, query.App, semanticLookupDepth)
	if err != nil {
		return nil, helperPath, err
	}
	return semanticFindTreeMatches(tree, query, semanticTreeLimit(query.Limit)), helperPath, nil
}

func semanticResolveQueryWithRunner(ctx context.Context, runner semanticRunner, query desktopSemanticQueryInput) (desktopSemanticQueryInput, map[string]any, string, error) {
	matches, helperPath, err := semanticFindMatchesWithRunner(ctx, runner, query)
	if err != nil {
		return query, nil, helperPath, err
	}
	if len(matches) == 0 {
		return query, nil, helperPath, nil
	}
	return semanticResolvedQueryForElement(query, matches[0]), matches[0], helperPath, nil
}

func semanticWaitForQueryWithRunner(ctx context.Context, runner semanticRunner, query desktopSemanticQueryInput, timeout int) (desktopSemanticQueryInput, map[string]any, string, string, error) {
	if timeout <= 0 {
		timeout = 5
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	helperPath := ""
	for {
		matches, currentHelperPath, err := semanticFindMatchesWithRunner(ctx, runner, query)
		if currentHelperPath != "" {
			helperPath = currentHelperPath
		}
		if err != nil {
			return query, nil, helperPath, "", err
		}
		if len(matches) > 0 {
			return semanticResolvedQueryForElement(query, matches[0]), matches[0], helperPath, "", nil
		}
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return query, nil, helperPath, "", ctx.Err()
		case <-time.After(semanticPollInterval):
		}
	}
	return query, nil, helperPath, fmt.Sprintf("Timeout waiting for element in %s", query.App), nil
}

func semanticBuildFormFields(ctx context.Context, runner semanticRunner, input desktopSemanticFormFieldsInput) (desktopSemanticFormFieldsOutput, error) {
	tree, helperPath, depth, err := semanticFetchTree(ctx, runner, input.App, input.Depth)
	if err != nil {
		return desktopSemanticFormFieldsOutput{}, err
	}
	scope, err := semanticResolveScope(tree, input.Ref, input.Path)
	if err != nil {
		return desktopSemanticFormFieldsOutput{}, err
	}
	fields := semanticCollectFormFields(scope, input.IncludeActions, input.Limit)
	return desktopSemanticFormFieldsOutput{
		HelperPath:     helperPath,
		App:            input.App,
		ScopeRef:       strings.TrimSpace(input.Ref),
		ScopePath:      strings.TrimSpace(input.Path),
		Depth:          depth,
		IncludeActions: input.IncludeActions,
		Count:          len(fields),
		Fields:         fields,
	}, nil
}

func semanticApplyFormRequest(ctx context.Context, runner semanticRunner, app string, request semanticFormFillItemInput, field *semanticFormField, element map[string]any, preview bool) (semanticFormFillItemOutput, string, error) {
	result := semanticFormFillItemOutput{
		Request:   request,
		Matched:   element != nil,
		FieldType: "",
		Element:   element,
	}
	if field != nil {
		fieldCopy := *field
		result.Field = &fieldCopy
		result.FieldType = field.FieldType
		result.CurrentValue = field.Value
	}
	if element == nil {
		result.Error = "semantic target not found"
		return result, "", nil
	}
	if result.CurrentValue == nil {
		result.CurrentValue = element["value"]
	}

	operation, err := semanticOperationForRequest(request)
	if err != nil {
		result.Error = err.Error()
		return result, "", nil
	}
	query := semanticQueryForElement(app, element, request.Role)
	args, err := semanticQueryArgs(query)
	if err != nil {
		result.Error = err.Error()
		return result, "", nil
	}

	command := ""
	switch operation {
	case "text":
		command = "set_text"
		result.Strategy = "set_text"
		args = append(args, "--text", *request.Text)
	case "number":
		command = "set_value"
		result.Strategy = "set_value"
		args = append(args, "--value", strconv.FormatFloat(*request.Number, 'f', -1, 64))
	case "action":
		command = "act"
		result.Strategy = "action"
		args = append(args, "--action", request.Action)
	case "checked":
		current, known := semanticElementCheckedState(element)
		if known && current == *request.Checked {
			result.Planned = true
			result.Strategy = "toggle_noop"
			return result, "", nil
		}
		if !*request.Checked && !known {
			result.Error = "unable to safely infer current toggle state for an unchecked request"
			return result, "", nil
		}
		command = "click"
		result.Strategy = "toggle"
	default:
		result.Error = "unsupported semantic fill operation"
		return result, "", nil
	}

	if preview {
		result.Planned = true
		return result, "", nil
	}

	parsed, helperPath, err := runner(ctx, append([]string{command}, args...)...)
	if err != nil {
		return result, helperPath, err
	}
	helperResult := semanticMapValue(parsed)
	result.Element = semanticMapValue(helperResult["element"])
	if result.Element == nil {
		result.Element = element
	}
	if field != nil {
		fieldCopy := *field
		fieldCopy.Element = result.Element
		fieldCopy.Value = helperResult["value"]
		if fieldCopy.Value == nil {
			fieldCopy.Value = result.Element["value"]
		}
		fieldCopy.ValueKind = stringValue(helperResult["value_kind"])
		if fieldCopy.ValueKind == "" {
			fieldCopy.ValueKind = stringValue(result.Element["value_kind"])
		}
		result.Field = &fieldCopy
		result.FieldType = fieldCopy.FieldType
		result.CurrentValue = fieldCopy.Value
	} else {
		result.CurrentValue = helperResult["value"]
		if result.CurrentValue == nil {
			result.CurrentValue = result.Element["value"]
		}
	}
	if errText := semanticErrorValue(helperResult); errText != "" {
		result.Error = errText
		return result, helperPath, nil
	}
	result.Applied = boolValue(helperResult["updated"]) || boolValue(helperResult["invoked"]) || boolValue(helperResult["clicked"])
	if operation == "checked" && result.Strategy == "toggle" && result.Error == "" && !result.Applied {
		result.Applied = true
	}
	return result, helperPath, nil
}

func semanticExecuteFormFill(ctx context.Context, runner semanticRunner, input desktopSemanticFormFillInput) (desktopSemanticFormFillOutput, error) {
	if len(input.Fields) == 0 {
		return desktopSemanticFormFillOutput{}, fmt.Errorf("[%s] fields must not be empty", handler.ErrInvalidParam)
	}
	tree, helperPath, depth, err := semanticFetchTree(ctx, runner, input.App, input.Depth)
	if err != nil {
		return desktopSemanticFormFillOutput{}, err
	}
	scope, err := semanticResolveScope(tree, input.ScopeRef, input.ScopePath)
	if err != nil {
		return desktopSemanticFormFillOutput{}, err
	}
	fields := semanticCollectFormFields(scope, true, 0)
	continueOnError := true
	if input.ContinueOnError != nil {
		continueOnError = *input.ContinueOnError
	}

	out := desktopSemanticFormFillOutput{
		HelperPath:      helperPath,
		App:             input.App,
		ScopeRef:        strings.TrimSpace(input.ScopeRef),
		ScopePath:       strings.TrimSpace(input.ScopePath),
		Depth:           depth,
		Preview:         input.Preview,
		ContinueOnError: continueOnError,
		Requested:       len(input.Fields),
		Results:         make([]semanticFormFillItemOutput, 0, len(input.Fields)),
	}

	for _, request := range input.Fields {
		field, element := semanticResolveRequestElement(scope, fields, request)
		item, latestHelperPath, err := semanticApplyFormRequest(ctx, runner, input.App, request, field, element, input.Preview)
		if latestHelperPath != "" {
			out.HelperPath = latestHelperPath
		}
		if err != nil {
			return out, err
		}
		out.Results = append(out.Results, item)
		if item.Matched {
			out.Matched++
		}
		if item.Planned {
			out.Planned++
		}
		if item.Applied {
			out.Applied++
		}
		if item.Error != "" && !continueOnError {
			break
		}
	}
	return out, nil
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
			if semanticQueryNeedsTreeLookup(input) {
				matches, helperPath, err := semanticFindMatchesWithRunner(ctx, runDesktopSemanticHelper, input)
				if err != nil {
					return desktopSemanticElementOutput{}, err
				}
				out := desktopSemanticElementOutput{
					HelperPath: helperPath,
					App:        input.App,
					Query:      input,
					Matched:    len(matches) > 0,
				}
				if len(matches) == 0 {
					out.Error = fmt.Sprintf("Element not found in %s", input.App)
					return out, nil
				}
				out.Query = semanticResolvedQueryForElement(input, matches[0])
				out.Element = matches[0]
				return out, nil
			}
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
			if semanticQueryNeedsTreeLookup(input) {
				matches, helperPath, err := semanticFindMatchesWithRunner(ctx, runDesktopSemanticHelper, input)
				if err != nil {
					return desktopSemanticMatchesOutput{}, err
				}
				return desktopSemanticMatchesOutput{
					HelperPath: helperPath,
					App:        input.App,
					Query:      input,
					Matched:    len(matches) > 0,
					Count:      len(matches),
					Matches:    matches,
				}, nil
			}
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
			query := input
			var matchedElement map[string]any
			var resolvedHelperPath string
			var err error
			if semanticQueryNeedsTreeLookup(query) {
				query, matchedElement, resolvedHelperPath, err = semanticResolveQueryWithRunner(ctx, runDesktopSemanticHelper, query)
				if err != nil {
					return desktopSemanticElementOutput{}, err
				}
				if matchedElement == nil {
					return desktopSemanticElementOutput{
						HelperPath: resolvedHelperPath,
						App:        input.App,
						Query:      input,
						Error:      fmt.Sprintf("Element not found in %s", input.App),
					}, nil
				}
			}
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"focus"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			out := desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed))
			if out.Element == nil && matchedElement != nil {
				out.Element = matchedElement
			}
			return out, nil
		},
	)
	focus.Category = "desktop"
	focus.SearchTerms = []string{"focus semantic element", "focus window control", "at-spi focus", "grab focus"}

	readValue := handler.TypedHandler[desktopSemanticQueryInput, desktopSemanticElementOutput](
		"desktop_read_value",
		"Read the current text or numeric value from a semantic element, such as an entry, label, slider, or spinbox.",
		func(ctx context.Context, input desktopSemanticQueryInput) (desktopSemanticElementOutput, error) {
			query := input
			var matchedElement map[string]any
			var resolvedHelperPath string
			var err error
			if semanticQueryNeedsTreeLookup(query) {
				query, matchedElement, resolvedHelperPath, err = semanticResolveQueryWithRunner(ctx, runDesktopSemanticHelper, query)
				if err != nil {
					return desktopSemanticElementOutput{}, err
				}
				if matchedElement == nil {
					return desktopSemanticElementOutput{
						HelperPath: resolvedHelperPath,
						App:        input.App,
						Query:      input,
						Error:      fmt.Sprintf("Element not found in %s", input.App),
					}, nil
				}
			}
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"read_value"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			out := desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed))
			if out.Element == nil && matchedElement != nil {
				out.Element = matchedElement
			}
			return out, nil
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
			var matchedElement map[string]any
			var resolvedHelperPath string
			var err error
			if semanticQueryNeedsTreeLookup(query) {
				query, matchedElement, resolvedHelperPath, err = semanticResolveQueryWithRunner(ctx, runDesktopSemanticHelper, query)
				if err != nil {
					return desktopSemanticElementOutput{}, err
				}
				if matchedElement == nil {
					return desktopSemanticElementOutput{
						HelperPath: resolvedHelperPath,
						App:        input.App,
						Query:      desktopSemanticQueryFromTextInput(input),
						Error:      fmt.Sprintf("Element not found in %s", input.App),
					}, nil
				}
			}
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			args = append(args, "--text", *input.Text)
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"set_text"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			out := desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed))
			if out.Element == nil && matchedElement != nil {
				out.Element = matchedElement
			}
			return out, nil
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
			var matchedElement map[string]any
			var resolvedHelperPath string
			var err error
			if semanticQueryNeedsTreeLookup(query) {
				query, matchedElement, resolvedHelperPath, err = semanticResolveQueryWithRunner(ctx, runDesktopSemanticHelper, query)
				if err != nil {
					return desktopSemanticElementOutput{}, err
				}
				if matchedElement == nil {
					return desktopSemanticElementOutput{
						HelperPath: resolvedHelperPath,
						App:        input.App,
						Query:      desktopSemanticQueryFromValueInput(input),
						Error:      fmt.Sprintf("Element not found in %s", input.App),
					}, nil
				}
			}
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			args = append(args, "--value", strconv.FormatFloat(*input.Value, 'f', -1, 64))
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"set_value"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			out := desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed))
			if out.Element == nil && matchedElement != nil {
				out.Element = matchedElement
			}
			return out, nil
		},
	)
	setValue.Category = "desktop"
	setValue.SearchTerms = []string{"set semantic value", "set slider", "set spinbox", "adjust accessible value"}

	formFields := handler.TypedHandler[desktopSemanticFormFieldsInput, desktopSemanticFormFieldsOutput](
		"desktop_form_fields",
		"Derive fillable semantic form fields from an AT-SPI application subtree, including labels, strategies, and refs.",
		func(ctx context.Context, input desktopSemanticFormFieldsInput) (desktopSemanticFormFieldsOutput, error) {
			return semanticBuildFormFields(ctx, runDesktopSemanticHelper, input)
		},
	)
	formFields.Category = "desktop"
	formFields.SearchTerms = []string{"semantic form fields", "form discovery", "discover fields", "at-spi form"}

	fillForm := handler.TypedHandler[desktopSemanticFormFillInput, desktopSemanticFormFillOutput](
		"desktop_fill_form",
		"Batch-resolve and fill semantic desktop form fields by label, ref, or path, with optional preview mode.",
		func(ctx context.Context, input desktopSemanticFormFillInput) (desktopSemanticFormFillOutput, error) {
			return semanticExecuteFormFill(ctx, runDesktopSemanticHelper, input)
		},
	)
	fillForm.Category = "desktop"
	fillForm.SearchTerms = []string{"semantic form fill", "batch fill fields", "fill desktop form", "at-spi batch input"}

	click := handler.TypedHandler[desktopSemanticQueryInput, desktopSemanticElementOutput](
		"desktop_click",
		"Invoke the default clickable semantic action by ref, path, or by app plus name and optional role.",
		func(ctx context.Context, input desktopSemanticQueryInput) (desktopSemanticElementOutput, error) {
			query := input
			var matchedElement map[string]any
			var resolvedHelperPath string
			var err error
			if semanticQueryNeedsTreeLookup(query) {
				query, matchedElement, resolvedHelperPath, err = semanticResolveQueryWithRunner(ctx, runDesktopSemanticHelper, query)
				if err != nil {
					return desktopSemanticElementOutput{}, err
				}
				if matchedElement == nil {
					return desktopSemanticElementOutput{
						HelperPath: resolvedHelperPath,
						App:        input.App,
						Query:      input,
						Error:      fmt.Sprintf("Element not found in %s", input.App),
					}, nil
				}
			}
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"click"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			out := desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed))
			if out.Element == nil && matchedElement != nil {
				out.Element = matchedElement
			}
			return out, nil
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
			var matchedElement map[string]any
			var resolvedHelperPath string
			var err error
			if semanticQueryNeedsTreeLookup(query) {
				query, matchedElement, resolvedHelperPath, err = semanticResolveQueryWithRunner(ctx, runDesktopSemanticHelper, query)
				if err != nil {
					return desktopSemanticElementOutput{}, err
				}
				if matchedElement == nil {
					return desktopSemanticElementOutput{
						HelperPath: resolvedHelperPath,
						App:        input.App,
						Query: desktopSemanticQueryInput{
							App:    input.App,
							Name:   input.Name,
							Role:   input.Role,
							Ref:    input.Ref,
							Path:   input.Path,
							States: input.States,
							Exact:  input.Exact,
						},
						Error: fmt.Sprintf("Element not found in %s", input.App),
					}, nil
				}
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
			out := desktopSemanticElementFromResult(helperPath, query, semanticMapValue(parsed))
			if out.Element == nil && matchedElement != nil {
				out.Element = matchedElement
			}
			return out, nil
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
			timeout := input.Timeout
			if timeout <= 0 {
				timeout = 5
			}
			if semanticQueryNeedsTreeLookup(query) {
				resolvedQuery, matchedElement, helperPath, errorText, err := semanticWaitForQueryWithRunner(ctx, runDesktopSemanticHelper, query, timeout)
				if err != nil {
					return desktopSemanticElementOutput{}, err
				}
				out := desktopSemanticElementOutput{
					HelperPath: helperPath,
					App:        query.App,
					Query:      resolvedQuery,
					Matched:    matchedElement != nil,
					Element:    matchedElement,
					Error:      errorText,
				}
				return out, nil
			}
			args, err := semanticQueryArgs(query)
			if err != nil {
				return desktopSemanticElementOutput{}, err
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
		formFields,
		fillForm,
		click,
		act,
		wait,
		typeText,
		key,
		capabilities,
	}
}
