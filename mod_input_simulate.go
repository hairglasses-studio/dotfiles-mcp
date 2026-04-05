// mod_input_simulate.go — Input simulation tools (keyboard, mouse, screen interaction)
package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// hasCmd checks whether a command is available on PATH.
func hasCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// simRunCmd executes a command and returns its combined output.
func simRunCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s failed: %w: %s", name, err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// ydotoolButtonCode maps a button name to the Linux input event code string.
func ydotoolButtonCode(button string) string {
	switch strings.ToLower(button) {
	case "right":
		return "0x111" // BTN_RIGHT
	case "middle":
		return "0x112" // BTN_MIDDLE
	default:
		return "0x110" // BTN_LEFT
	}
}

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------

type SimTypeTextInput struct {
	Text    string `json:"text" jsonschema:"required,description=Text to type as if from keyboard"`
	DelayMs int    `json:"delay_ms,omitempty" jsonschema:"description=Inter-key delay in milliseconds (default: no delay)"`
}

type SimKeyPressInput struct {
	Keys string `json:"keys" jsonschema:"required,description=Key combination to press (e.g. ctrl+shift+t or Return or super). Uses wtype -M/-m for modifiers and -k for keys."`
}

type SimMouseMoveInput struct {
	X        int  `json:"x" jsonschema:"required,description=X coordinate to move to"`
	Y        int  `json:"y" jsonschema:"required,description=Y coordinate to move to"`
	Relative bool `json:"relative,omitempty" jsonschema:"description=If true move relative to current position instead of absolute"`
}

type SimMouseClickInput struct {
	Button string `json:"button,omitempty" jsonschema:"description=Mouse button: left right or middle (default: left)"`
	X      int    `json:"x,omitempty" jsonschema:"description=X coordinate to click at (omit to click at current position)"`
	Y      int    `json:"y,omitempty" jsonschema:"description=Y coordinate to click at (omit to click at current position)"`
}

type SimMouseScrollInput struct {
	Direction string `json:"direction" jsonschema:"required,description=Scroll direction: up or down"`
	Amount    int    `json:"amount,omitempty" jsonschema:"description=Number of scroll steps (default: 3)"`
}

type SimScreenshotClickInput struct {
	Description string `json:"description" jsonschema:"required,description=What to click — for agent context. The actual click happens by coordinates passed via input_mouse_click after reviewing a screenshot."`
	X           int    `json:"x" jsonschema:"required,description=X coordinate to click"`
	Y           int    `json:"y" jsonschema:"required,description=Y coordinate to click"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// InputSimulateModule provides tools that simulate keyboard and mouse input.
// All tools are marked IsWrite because they produce real input events.
type InputSimulateModule struct{}

func (m *InputSimulateModule) Name() string        { return "input_simulate" }
func (m *InputSimulateModule) Description() string { return "Input simulation tools (keyboard, mouse, screen interaction)" }

func (m *InputSimulateModule) Tools() []registry.ToolDefinition {
	// ── input_type_text ──────────────────────────────────
	typeText := handler.TypedHandler[SimTypeTextInput, string](
		"input_type_text",
		"DESTRUCTIVE: Type text as if from keyboard using wtype (Wayland) or ydotool as fallback. This simulates real keystrokes — requires explicit approval.",
		func(_ context.Context, input SimTypeTextInput) (string, error) {
			if input.Text == "" {
				return "", fmt.Errorf("[%s] text must not be empty", handler.ErrInvalidParam)
			}

			if hasCmd("wtype") {
				args := []string{}
				if input.DelayMs > 0 {
					args = append(args, "-d", strconv.Itoa(input.DelayMs))
				}
				args = append(args, input.Text)
				if _, err := simRunCmd("wtype", args...); err != nil {
					return "", fmt.Errorf("wtype failed: %w", err)
				}
				return fmt.Sprintf("typed %d chars via wtype", len(input.Text)), nil
			}

			if hasCmd("ydotool") {
				args := []string{"type"}
				if input.DelayMs > 0 {
					args = append(args, "--key-delay", strconv.Itoa(input.DelayMs))
				}
				args = append(args, "--", input.Text)
				if _, err := simRunCmd("ydotool", args...); err != nil {
					return "", fmt.Errorf("ydotool type failed: %w", err)
				}
				return fmt.Sprintf("typed %d chars via ydotool", len(input.Text)), nil
			}

			return "", fmt.Errorf("neither wtype nor ydotool found on PATH")
		},
	)
	typeText.IsWrite = true
	typeText.Category = "input_simulate"
	typeText.Tags = []string{"input", "keyboard", "type", "simulate"}
	typeText.SearchTerms = []string{"type text", "keyboard input", "simulate typing", "wtype", "ydotool"}

	// ── input_key_press ──────────────────────────────────
	keyPress := handler.TypedHandler[SimKeyPressInput, string](
		"input_key_press",
		"DESTRUCTIVE: Press a key combination (e.g. ctrl+shift+t, Return, super) using wtype -k or ydotool key. Simulates real key events — requires explicit approval.",
		func(_ context.Context, input SimKeyPressInput) (string, error) {
			if input.Keys == "" {
				return "", fmt.Errorf("[%s] keys must not be empty", handler.ErrInvalidParam)
			}

			if hasCmd("wtype") {
				// Parse key combo: split on + for modifiers
				// wtype uses -M <mod> to hold modifier, then -k <key>, then -m <mod> to release
				parts := strings.Split(input.Keys, "+")
				args := []string{}

				if len(parts) == 1 {
					// Single key, no modifiers
					args = append(args, "-k", parts[0])
				} else {
					// Hold modifiers, press key, release modifiers
					modifiers := parts[:len(parts)-1]
					key := parts[len(parts)-1]
					for _, mod := range modifiers {
						args = append(args, "-M", mod)
					}
					args = append(args, "-k", key)
					for i := len(modifiers) - 1; i >= 0; i-- {
						args = append(args, "-m", modifiers[i])
					}
				}

				if _, err := simRunCmd("wtype", args...); err != nil {
					return "", fmt.Errorf("wtype key failed: %w", err)
				}
				return fmt.Sprintf("pressed %s via wtype", input.Keys), nil
			}

			if hasCmd("ydotool") {
				// ydotool key accepts key names like "key 29:1 29:0" for raw codes
				// or higher-level via xdotool compat. Pass through as-is.
				args := append([]string{"key"}, strings.Fields(input.Keys)...)
				if _, err := simRunCmd("ydotool", args...); err != nil {
					return "", fmt.Errorf("ydotool key failed: %w", err)
				}
				return fmt.Sprintf("pressed %s via ydotool", input.Keys), nil
			}

			return "", fmt.Errorf("neither wtype nor ydotool found on PATH")
		},
	)
	keyPress.IsWrite = true
	keyPress.Category = "input_simulate"
	keyPress.Tags = []string{"input", "keyboard", "key", "hotkey", "simulate"}
	keyPress.SearchTerms = []string{"key press", "hotkey", "keyboard shortcut", "key combo", "simulate key"}

	// ── input_mouse_move ─────────────────────────────────
	mouseMove := handler.TypedHandler[SimMouseMoveInput, string](
		"input_mouse_move",
		"DESTRUCTIVE: Move mouse cursor to coordinates using ydotool mousemove. Simulates real mouse movement — requires explicit approval.",
		func(_ context.Context, input SimMouseMoveInput) (string, error) {
			if !hasCmd("ydotool") {
				return "", fmt.Errorf("ydotool not found on PATH")
			}

			args := []string{"mousemove"}
			if !input.Relative {
				args = append(args, "--absolute")
			}
			args = append(args, "-x", strconv.Itoa(input.X), "-y", strconv.Itoa(input.Y))

			if _, err := simRunCmd("ydotool", args...); err != nil {
				return "", fmt.Errorf("mousemove failed: %w", err)
			}

			mode := "absolute"
			if input.Relative {
				mode = "relative"
			}
			return fmt.Sprintf("moved mouse to (%d, %d) [%s]", input.X, input.Y, mode), nil
		},
	)
	mouseMove.IsWrite = true
	mouseMove.Category = "input_simulate"
	mouseMove.Tags = []string{"input", "mouse", "move", "cursor", "simulate"}
	mouseMove.SearchTerms = []string{"mouse move", "cursor position", "move cursor", "ydotool mousemove"}

	// ── input_mouse_click ────────────────────────────────
	mouseClick := handler.TypedHandler[SimMouseClickInput, string](
		"input_mouse_click",
		"DESTRUCTIVE: Click mouse button at current or specified position using ydotool. Simulates a real mouse click — requires explicit approval.",
		func(_ context.Context, input SimMouseClickInput) (string, error) {
			if !hasCmd("ydotool") {
				return "", fmt.Errorf("ydotool not found on PATH")
			}

			button := input.Button
			if button == "" {
				button = "left"
			}

			// Move to position first if coordinates provided
			if input.X != 0 || input.Y != 0 {
				if _, err := simRunCmd("ydotool", "mousemove", "--absolute",
					"-x", strconv.Itoa(input.X), "-y", strconv.Itoa(input.Y)); err != nil {
					return "", fmt.Errorf("mousemove failed: %w", err)
				}
			}

			// Click
			if _, err := simRunCmd("ydotool", "click", "--next-delay", "0", ydotoolButtonCode(button)); err != nil {
				return "", fmt.Errorf("click failed: %w", err)
			}

			if input.X != 0 || input.Y != 0 {
				return fmt.Sprintf("clicked %s at (%d, %d)", button, input.X, input.Y), nil
			}
			return fmt.Sprintf("clicked %s at current position", button), nil
		},
	)
	mouseClick.IsWrite = true
	mouseClick.Category = "input_simulate"
	mouseClick.Tags = []string{"input", "mouse", "click", "simulate"}
	mouseClick.SearchTerms = []string{"mouse click", "click button", "left click", "right click", "ydotool click"}

	// ── input_mouse_scroll ───────────────────────────────
	mouseScroll := handler.TypedHandler[SimMouseScrollInput, string](
		"input_mouse_scroll",
		"DESTRUCTIVE: Scroll the mouse wheel up or down using ydotool. Simulates real scroll events — requires explicit approval.",
		func(_ context.Context, input SimMouseScrollInput) (string, error) {
			if !hasCmd("ydotool") {
				return "", fmt.Errorf("ydotool not found on PATH")
			}

			dir := strings.ToLower(input.Direction)
			if dir != "up" && dir != "down" {
				return "", fmt.Errorf("[%s] direction must be 'up' or 'down', got %q", handler.ErrInvalidParam, input.Direction)
			}

			amount := input.Amount
			if amount <= 0 {
				amount = 3
			}

			// ydotool mousemove --wheel: negative = up, positive = down
			scrollAmount := amount
			if dir == "up" {
				scrollAmount = -amount
			}

			if _, err := simRunCmd("ydotool", "mousemove", "--wheel",
				"-x", "0", "-y", strconv.Itoa(scrollAmount)); err != nil {
				return "", fmt.Errorf("scroll failed: %w", err)
			}

			return fmt.Sprintf("scrolled %s by %d steps", dir, amount), nil
		},
	)
	mouseScroll.IsWrite = true
	mouseScroll.Category = "input_simulate"
	mouseScroll.Tags = []string{"input", "mouse", "scroll", "wheel", "simulate"}
	mouseScroll.SearchTerms = []string{"mouse scroll", "scroll wheel", "scroll up", "scroll down"}

	// ── input_screenshot_click ───────────────────────────
	screenshotClick := handler.TypedHandler[SimScreenshotClickInput, string](
		"input_screenshot_click",
		"DESTRUCTIVE: Take a screenshot with grim, then click at the specified coordinates. Combines screenshot capture for agent visual context with coordinate-based clicking. Requires explicit approval.",
		func(_ context.Context, input SimScreenshotClickInput) (string, error) {
			if !hasCmd("grim") {
				return "", fmt.Errorf("grim not found on PATH")
			}
			if !hasCmd("ydotool") {
				return "", fmt.Errorf("ydotool not found on PATH")
			}

			// Capture screenshot for logging/context
			screenshotPath := "/tmp/input-screenshot-click.png"
			if _, err := simRunCmd("grim", screenshotPath); err != nil {
				return "", fmt.Errorf("grim screenshot failed: %w", err)
			}

			// Move and click
			if _, err := simRunCmd("ydotool", "mousemove", "--absolute",
				"-x", strconv.Itoa(input.X), "-y", strconv.Itoa(input.Y)); err != nil {
				return "", fmt.Errorf("mousemove failed: %w", err)
			}
			if _, err := simRunCmd("ydotool", "click", "--next-delay", "0", "0x110"); err != nil {
				return "", fmt.Errorf("click failed: %w", err)
			}

			return fmt.Sprintf("screenshot saved to %s, clicked at (%d, %d) for: %s",
				screenshotPath, input.X, input.Y, input.Description), nil
		},
	)
	screenshotClick.IsWrite = true
	screenshotClick.Category = "input_simulate"
	screenshotClick.Tags = []string{"input", "screenshot", "click", "visual", "simulate"}
	screenshotClick.SearchTerms = []string{"screenshot click", "visual click", "click element", "grim click", "screen interaction"}

	return []registry.ToolDefinition{
		typeText,
		keyPress,
		mouseMove,
		mouseClick,
		mouseScroll,
		screenshotClick,
	}
}
