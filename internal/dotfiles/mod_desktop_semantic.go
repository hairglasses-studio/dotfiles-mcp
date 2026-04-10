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

type desktopSemanticQueryInput struct {
	App   string `json:"app" jsonschema:"required,description=Application name or unique substring"`
	Name  string `json:"name,omitempty" jsonschema:"description=Element name or substring"`
	Role  string `json:"role,omitempty" jsonschema:"description=Optional exact role filter"`
	Path  string `json:"path,omitempty" jsonschema:"description=Optional child-index path such as 0/2/1 from a previous semantic result"`
	Exact bool   `json:"exact,omitempty" jsonschema:"description=Require exact case-insensitive name matching"`
}

type desktopSemanticWaitInput struct {
	App     string `json:"app" jsonschema:"required,description=Application name or unique substring"`
	Name    string `json:"name,omitempty" jsonschema:"description=Element name or substring"`
	Role    string `json:"role,omitempty" jsonschema:"description=Optional exact role filter"`
	Path    string `json:"path,omitempty" jsonschema:"description=Optional child-index path such as 0/2/1 from a previous semantic result"`
	Exact   bool   `json:"exact,omitempty" jsonschema:"description=Require exact case-insensitive name matching"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds (default 5)"`
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
	Action     string                    `json:"action,omitempty"`
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
	caps := desktopSemanticCapabilities()
	if !caps.Ready {
		return nil, caps.HelperPath, fmt.Errorf("AT-SPI not ready: %s", strings.Join(caps.Missing, ", "))
	}

	cmdArgs := append([]string{caps.HelperPath}, args...)
	cmd := exec.CommandContext(ctx, "python3", cmdArgs...)
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
	if strings.TrimSpace(input.Name) == "" && strings.TrimSpace(input.Path) == "" {
		return nil, fmt.Errorf("[%s] name or path is required", handler.ErrInvalidParam)
	}

	args := []string{"--app", input.App}
	if strings.TrimSpace(input.Name) != "" {
		args = append(args, "--name", input.Name)
	}
	if strings.TrimSpace(input.Role) != "" {
		args = append(args, "--role", input.Role)
	}
	if strings.TrimSpace(input.Path) != "" {
		args = append(args, "--path", input.Path)
	}
	if input.Exact {
		args = append(args, "--exact")
	}
	return args, nil
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
		"Find a semantic element by app plus name, role, or a previously returned path.",
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
			return desktopSemanticElementOutput{
				HelperPath: helperPath,
				App:        input.App,
				Query:      input,
				Matched:    result["matched"] == true,
				Element:    semanticMapValue(result["element"]),
				Error:      semanticErrorValue(result),
			}, nil
		},
	)
	find.Category = "desktop"
	find.SearchTerms = []string{"find semantic element", "find button", "accessibility search", "at-spi find"}

	click := handler.TypedHandler[desktopSemanticQueryInput, desktopSemanticElementOutput](
		"desktop_click",
		"Click a semantic element by path or by app plus name and optional role.",
		func(ctx context.Context, input desktopSemanticQueryInput) (desktopSemanticElementOutput, error) {
			args, err := semanticQueryArgs(input)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			parsed, helperPath, err := runDesktopSemanticHelper(ctx, append([]string{"click"}, args...)...)
			if err != nil {
				return desktopSemanticElementOutput{}, err
			}
			result := semanticMapValue(parsed)
			action, _ := result["action"].(string)
			return desktopSemanticElementOutput{
				HelperPath: helperPath,
				App:        input.App,
				Query:      input,
				Matched:    result["matched"] == true,
				Clicked:    result["clicked"] == true,
				Action:     action,
				Element:    semanticMapValue(result["element"]),
				Error:      semanticErrorValue(result),
			}, nil
		},
	)
	click.Category = "desktop"
	click.SearchTerms = []string{"semantic click", "click button by label", "at-spi click"}

	wait := handler.TypedHandler[desktopSemanticWaitInput, desktopSemanticElementOutput](
		"desktop_wait_for_element",
		"Wait for a semantic element to appear, then return its resolved element record.",
		func(ctx context.Context, input desktopSemanticWaitInput) (desktopSemanticElementOutput, error) {
			query := desktopSemanticQueryInput{
				App:   input.App,
				Name:  input.Name,
				Role:  input.Role,
				Path:  input.Path,
				Exact: input.Exact,
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
			result := semanticMapValue(parsed)
			return desktopSemanticElementOutput{
				HelperPath: helperPath,
				App:        input.App,
				Query:      query,
				Matched:    result["matched"] == true,
				Element:    semanticMapValue(result["element"]),
				Error:      semanticErrorValue(result),
			}, nil
		},
	)
	wait.Category = "desktop"
	wait.SearchTerms = []string{"wait for button", "wait for dialog", "wait semantic element"}

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
		targetWindows,
		find,
		click,
		wait,
		typeText,
		key,
		capabilities,
	}
}
