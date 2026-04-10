package dotfiles

import (
	"context"
	"fmt"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type EwwInspectInput struct {
	IncludeRaw bool `json:"include_raw,omitempty" jsonschema:"description=Include raw eww state and window output alongside the parsed summary"`
}

type EwwInspectOutput struct {
	DaemonRunning  bool           `json:"daemon_running"`
	DaemonCount    int            `json:"daemon_count"`
	WaybarRunning  bool           `json:"waybar_running"`
	ActiveWindows  []string       `json:"active_windows"`
	DefinedWindows []string       `json:"defined_windows"`
	Layers         []EwwLayerInfo `json:"layers"`
	Variables      map[string]any `json:"variables,omitempty"`
	RawState       string         `json:"raw_state,omitempty"`
	RawWindows     string         `json:"raw_windows,omitempty"`
}

type EwwReloadInput struct {
	Window           string `json:"window,omitempty" jsonschema:"description=Specific eww window to reopen or open. When omitted, performs a plain eww reload."`
	OpenIfMissing    bool   `json:"open_if_missing,omitempty" jsonschema:"description=Open the target window if it is defined but not currently active"`
	RestartOnFailure *bool  `json:"restart_on_failure,omitempty" jsonschema:"description=Fall back to a full daemon restart when targeted reload or eww reload fails. Defaults to true."`
}

type EwwReloadOutput struct {
	Window       string   `json:"window,omitempty"`
	Mode         string   `json:"mode"`
	Fallback     bool     `json:"fallback"`
	Actions      []string `json:"actions,omitempty"`
	ActiveBefore []string `json:"active_before,omitempty"`
	ActiveAfter  []string `json:"active_after,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type EwwDesktopModule struct{}

func (m *EwwDesktopModule) Name() string        { return "eww_desktop" }
func (m *EwwDesktopModule) Description() string { return "Eww inspection and targeted reload tools" }

func (m *EwwDesktopModule) Tools() []registry.ToolDefinition {
	inspect := handler.TypedHandler[EwwInspectInput, EwwInspectOutput](
		"dotfiles_eww_inspect",
		"Inspect the current eww daemon state, active and defined windows, visible layer bindings, and parsed variable map.",
		func(_ context.Context, input EwwInspectInput) (EwwInspectOutput, error) {
			status := currentEwwStatus()
			state, rawState, stateErr := ewwStateSnapshot()
			rawWindows, windowsErr := runEww("list-windows")

			output := EwwInspectOutput{
				DaemonRunning:  status.DaemonRunning,
				DaemonCount:    status.DaemonCount,
				WaybarRunning:  status.WaybarRunning,
				ActiveWindows:  status.Windows,
				DefinedWindows: status.DefinedWindows,
				Layers:         status.Layers,
				Variables:      state,
			}
			if input.IncludeRaw {
				output.RawState = rawState
				output.RawWindows = rawWindows
			}
			if stateErr != nil && windowsErr != nil {
				return output, fmt.Errorf("eww inspect failed: %v; %v", stateErr, windowsErr)
			}
			if stateErr != nil {
				return output, stateErr
			}
			if windowsErr != nil {
				return output, windowsErr
			}
			return output, nil
		},
	)

	reload := handler.TypedHandler[EwwReloadInput, EwwReloadOutput](
		"dotfiles_eww_reload",
		"Reload eww narrowly when possible by reopening one target window, falling back to full eww reload and then daemon restart only when needed.",
		func(_ context.Context, input EwwReloadInput) (EwwReloadOutput, error) {
			if !hasCmd("eww") {
				return EwwReloadOutput{}, fmt.Errorf("eww not found on PATH")
			}

			restartOnFailure := defaultBool(input.RestartOnFailure, true)

			before, _ := ewwActiveWindows()
			defined, _ := ewwDefinedWindows()
			output := EwwReloadOutput{
				Window:       strings.TrimSpace(input.Window),
				Mode:         "reload",
				ActiveBefore: before,
			}

			targetedSucceeded := false
			if output.Window != "" {
				windowIsActive := stringInSlice(output.Window, before)
				windowIsDefined := stringInSlice(output.Window, defined)
				switch {
				case windowIsActive:
					output.Mode = "targeted"
					output.Actions = append(output.Actions, "close "+output.Window, "open "+output.Window)
					if _, err := runEww("close", output.Window); err == nil {
						if _, err := runEww("open", output.Window); err == nil {
							targetedSucceeded = true
						} else {
							output.Error = err.Error()
						}
					} else {
						output.Error = err.Error()
					}
				case windowIsDefined || input.OpenIfMissing:
					output.Mode = "targeted"
					output.Actions = append(output.Actions, "open "+output.Window)
					if _, err := runEww("open", output.Window); err == nil {
						targetedSucceeded = true
					} else {
						output.Error = err.Error()
					}
				default:
					output.Error = fmt.Sprintf("window %q is not defined in the current eww config", output.Window)
				}
			}

			if !targetedSucceeded {
				output.Actions = append(output.Actions, "reload")
				if _, err := runEww("reload"); err != nil {
					output.Error = err.Error()
					if restartOnFailure {
						output.Fallback = true
						output.Mode = "restart"
						output.Actions = append(output.Actions, "restart daemon")
						restart := restartEwwBars()
						if restart.Error != "" {
							output.Error = restart.Error
						} else {
							output.Error = ""
						}
					}
				}
			}

			after, _ := ewwActiveWindows()
			output.ActiveAfter = after
			return output, nil
		},
	)
	reload.IsWrite = true

	return []registry.ToolDefinition{inspect, reload}
}

func stringInSlice(needle string, haystack []string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
