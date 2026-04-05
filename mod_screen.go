// mod_screen.go — Screen capture, recording, OCR, and color-pick tools
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// screenRunCmd executes a command and returns combined output.
func screenRunCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s failed: %w: %s", name, err, string(out))
	}
	return string(out), nil
}

// screenCheckTool checks if a CLI tool is available on PATH.
func screenCheckTool(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found on PATH — install it first (e.g. pacman -S %s)", name, name)
	}
	return nil
}

// screenTimestamp returns a timestamp string for filenames.
func screenTimestamp() string {
	return time.Now().Format("20060102-150405")
}

// ---------------------------------------------------------------------------
// Recording state — stores PID and output path for active recording
// ---------------------------------------------------------------------------

var (
	recordMu     sync.Mutex
	recordPID    int
	recordOutput string
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------

type ScreenScreenshotInput struct {
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=File path to save the screenshot. Defaults to /tmp/screenshot-TIMESTAMP.png"`
	Region     string `json:"region,omitempty" jsonschema:"description=Region to capture in 'x,y WxH' format (e.g. '100,200 800x600'). Omit for full screen."`
}

type ScreenRecordStartInput struct {
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=File path to save the recording. Defaults to /tmp/recording-TIMESTAMP.mp4"`
	Audio      bool   `json:"audio,omitempty" jsonschema:"description=Capture audio with the recording (requires PipeWire/PulseAudio)"`
	Region     string `json:"region,omitempty" jsonschema:"description=Region to record in 'x,y WxH' format (e.g. '100,200 800x600'). Omit for full screen."`
}

type ScreenRecordStopInput struct{}

type ScreenOCRInput struct {
	Region string `json:"region,omitempty" jsonschema:"description=Region to capture for OCR in 'x,y WxH' format. Omit for full screen."`
}

type ScreenColorPickInput struct{}

type ScreenRecordStatusInput struct{}

type ScreenRecordStatusOutput struct {
	Active     bool    `json:"active"`
	PID        int     `json:"pid,omitempty"`
	OutputPath string  `json:"output_path,omitempty"`
	ElapsedS   float64 `json:"elapsed_s,omitempty"`
}

type ScreenAnnotateInput struct {
	FilePath string `json:"file_path" jsonschema:"description=Path to the screenshot file to open in swappy for annotation"`
}

type ScreenScreenshotAnnotatedInput struct {
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=File path to save the screenshot. Defaults to /tmp/screenshot-TIMESTAMP.png"`
	Region     string `json:"region,omitempty" jsonschema:"description=Region to capture in 'x,y WxH' format (e.g. '100,200 800x600'). Omit for full screen."`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

type ScreenModule struct{}

func (m *ScreenModule) Name() string        { return "screen" }
func (m *ScreenModule) Description() string { return "Screen capture, recording, OCR, and color-pick tools" }

func (m *ScreenModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── screen_screenshot ─────────────────────────
		// Returns image content directly, so we use a raw handler.
		{
			Tool: mcp.Tool{
				Name:        "screen_screenshot",
				Description: "Take a screenshot of the current screen or a region. Uses grim (Wayland). Returns the image inline and the saved file path.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"output_path": map[string]any{
							"type":        "string",
							"description": "File path to save the screenshot. Defaults to /tmp/screenshot-TIMESTAMP.png",
						},
						"region": map[string]any{
							"type":        "string",
							"description": "Region to capture in 'x,y WxH' format (e.g. '100,200 800x600'). Omit for full screen.",
						},
					},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := screenCheckTool("grim"); err != nil {
					return handler.ErrorResult(err), nil
				}

				var input ScreenScreenshotInput
				if req.Params.Arguments != nil {
					b, _ := json.Marshal(req.Params.Arguments)
					json.Unmarshal(b, &input)
				}

				outPath := input.OutputPath
				if outPath == "" {
					outPath = fmt.Sprintf("/tmp/screenshot-%s.png", screenTimestamp())
				}

				// Build grim args
				var grimArgs []string
				if input.Region != "" {
					grimArgs = append(grimArgs, "-g", input.Region)
				}
				grimArgs = append(grimArgs, outPath)

				if _, err := screenRunCmd("grim", grimArgs...); err != nil {
					return handler.ErrorResult(fmt.Errorf("grim capture failed: %w", err)), nil
				}

				// Resize for inline display (max 1568x1568)
				resized := outPath + ".resized.png"
				defer os.Remove(resized)

				if _, err := screenRunCmd("magick", outPath, "-resize", "1568x1568>", resized); err != nil {
					// If magick is not available, return just the path
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: fmt.Sprintf("Screenshot saved to %s (magick not available for inline preview)", outPath),
							},
						},
					}, nil
				}

				data, err := os.ReadFile(resized)
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("failed to read screenshot: %w", err)), nil
				}
				b64 := base64.StdEncoding.EncodeToString(data)

				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("Screenshot saved to %s", outPath),
						},
						mcp.ImageContent{
							Type:     "image",
							Data:     b64,
							MIMEType: "image/png",
						},
					},
				}, nil
			},
		},

		// ── screen_record_start ───────────────────────
		handler.TypedHandler[ScreenRecordStartInput, string](
			"screen_record_start",
			"Start recording the screen using wf-recorder (Wayland). Runs in the background. Use screen_record_stop to finish.",
			func(_ context.Context, input ScreenRecordStartInput) (string, error) {
				if err := screenCheckTool("wf-recorder"); err != nil {
					return "", err
				}

				recordMu.Lock()
				defer recordMu.Unlock()

				if recordPID != 0 {
					return "", fmt.Errorf("[%s] a recording is already in progress (PID %d). Stop it first with screen_record_stop", handler.ErrInvalidParam, recordPID)
				}

				outPath := input.OutputPath
				if outPath == "" {
					outPath = fmt.Sprintf("/tmp/recording-%s.mp4", screenTimestamp())
				}

				// Build wf-recorder args
				args := []string{"-f", outPath}
				if input.Audio {
					args = append(args, "--audio")
				}
				if input.Region != "" {
					args = append(args, "-g", input.Region)
				}

				cmd := exec.Command("wf-recorder", args...)
				if err := cmd.Start(); err != nil {
					return "", fmt.Errorf("failed to start wf-recorder: %w", err)
				}

				recordPID = cmd.Process.Pid
				recordOutput = outPath

				// Detach — let wf-recorder run in the background.
				go cmd.Wait()

				return fmt.Sprintf("Recording started (PID %d). Saving to %s", recordPID, outPath), nil
			},
		),

		// ── screen_record_stop ────────────────────────
		handler.TypedHandler[ScreenRecordStopInput, string](
			"screen_record_stop",
			"Stop an active screen recording started by screen_record_start. Returns the output file path.",
			func(_ context.Context, _ ScreenRecordStopInput) (string, error) {
				recordMu.Lock()
				defer recordMu.Unlock()

				if recordPID == 0 {
					return "", fmt.Errorf("[%s] no active recording to stop", handler.ErrInvalidParam)
				}

				// Send SIGINT to wf-recorder for a clean stop
				proc, err := os.FindProcess(recordPID)
				if err != nil {
					recordPID = 0
					recordOutput = ""
					return "", fmt.Errorf("could not find recording process (PID %d): %w", recordPID, err)
				}

				if err := proc.Signal(os.Interrupt); err != nil {
					recordPID = 0
					recordOutput = ""
					return "", fmt.Errorf("failed to stop recording (PID %d): %w", recordPID, err)
				}

				// Wait briefly for the file to be finalized
				time.Sleep(500 * time.Millisecond)

				outPath := recordOutput
				recordPID = 0
				recordOutput = ""

				// Check that the output file exists
				info, err := os.Stat(outPath)
				if err != nil {
					return fmt.Sprintf("Recording stopped. Output file: %s (warning: file may still be finalizing)", outPath), nil
				}

				return fmt.Sprintf("Recording stopped. Saved to %s (%d bytes)", outPath, info.Size()), nil
			},
		),

		// ── screen_ocr ────────────────────────────────
		handler.TypedHandler[ScreenOCRInput, string](
			"screen_ocr",
			"Take a screenshot and extract text via OCR using tesseract. Returns the extracted text.",
			func(_ context.Context, input ScreenOCRInput) (string, error) {
				if err := screenCheckTool("grim"); err != nil {
					return "", err
				}
				if err := screenCheckTool("tesseract"); err != nil {
					return "", err
				}

				tmpImg := fmt.Sprintf("/tmp/ocr-%s.png", screenTimestamp())
				defer os.Remove(tmpImg)

				// Capture screenshot
				var grimArgs []string
				if input.Region != "" {
					grimArgs = append(grimArgs, "-g", input.Region)
				}
				grimArgs = append(grimArgs, tmpImg)

				if _, err := screenRunCmd("grim", grimArgs...); err != nil {
					return "", fmt.Errorf("grim capture failed: %w", err)
				}

				// Run tesseract OCR (output to stdout)
				out, err := screenRunCmd("tesseract", tmpImg, "stdout")
				if err != nil {
					return "", fmt.Errorf("tesseract OCR failed: %w", err)
				}

				text := strings.TrimSpace(out)
				if text == "" {
					return "No text detected in the captured region.", nil
				}

				return text, nil
			},
		),

		// ── screen_color_pick ─────────────────────────
		// Returns raw pixel color — uses hyprpicker if available, falls back to grim+pixel sampling.
		{
			Tool: mcp.Tool{
				Name:        "screen_color_pick",
				Description: "Pick a color from the screen. Uses hyprpicker if available, otherwise captures a 1x1 screenshot at cursor position via grim. Returns the hex color value.",
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]any{},
				},
			},
			Handler: func(_ context.Context, _ registry.CallToolRequest) (*registry.CallToolResult, error) {
				// Try hyprpicker first (interactive, user clicks to pick)
				if err := screenCheckTool("hyprpicker"); err == nil {
					out, err := screenRunCmd("hyprpicker", "--autocopy", "--no-fancy")
					if err != nil {
						return handler.ErrorResult(fmt.Errorf("hyprpicker failed: %w", err)), nil
					}
					hex := strings.TrimSpace(out)
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: fmt.Sprintf("Color: %s", hex),
							},
						},
					}, nil
				}

				// Fallback: capture full screen, get cursor position, sample pixel
				if err := screenCheckTool("grim"); err != nil {
					return handler.ErrorResult(err), nil
				}
				if err := screenCheckTool("hyprctl"); err != nil {
					return handler.ErrorResult(fmt.Errorf("neither hyprpicker nor hyprctl available for color picking")), nil
				}

				// Get cursor position from hyprctl
				cursorJSON, err := screenRunCmd("hyprctl", "cursorpos", "-j")
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("failed to get cursor position: %w", err)), nil
				}

				var cursor struct {
					X int `json:"x"`
					Y int `json:"y"`
				}
				if err := json.Unmarshal([]byte(cursorJSON), &cursor); err != nil {
					return handler.ErrorResult(fmt.Errorf("failed to parse cursor position: %w", err)), nil
				}

				// Capture a 1x1 region at cursor
				tmpImg := fmt.Sprintf("/tmp/colorpick-%s.png", screenTimestamp())
				defer os.Remove(tmpImg)

				region := fmt.Sprintf("%d,%d 1x1", cursor.X, cursor.Y)
				if _, err := screenRunCmd("grim", "-g", region, tmpImg); err != nil {
					return handler.ErrorResult(fmt.Errorf("grim capture failed: %w", err)), nil
				}

				// Read the pixel color
				f, err := os.Open(tmpImg)
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("failed to open pixel capture: %w", err)), nil
				}
				defer f.Close()

				img, err := png.Decode(f)
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("failed to decode pixel capture: %w", err)), nil
				}

				r, g, b, _ := img.At(0, 0).RGBA()
				hex := fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)

				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("Color at (%d, %d): %s\nRGB: (%d, %d, %d)", cursor.X, cursor.Y, hex, r>>8, g>>8, b>>8),
						},
					},
				}, nil
			},
		},

		// ── screen_record_status ──────────────────────
		handler.TypedHandler[ScreenRecordStatusInput, ScreenRecordStatusOutput](
			"screen_record_status",
			"Return the current recording state: active/inactive, PID, output path, and elapsed duration. No input required.",
			func(_ context.Context, _ ScreenRecordStatusInput) (ScreenRecordStatusOutput, error) {
				recordMu.Lock()
				pid := recordPID
				outPath := recordOutput
				recordMu.Unlock()

				if pid == 0 {
					return ScreenRecordStatusOutput{Active: false}, nil
				}

				// Check if process is still alive via /proc/{PID}/stat
				statPath := fmt.Sprintf("/proc/%d/stat", pid)
				statData, err := os.ReadFile(statPath)
				if err != nil {
					// Process is gone — clear stale state
					recordMu.Lock()
					recordPID = 0
					recordOutput = ""
					recordMu.Unlock()
					return ScreenRecordStatusOutput{Active: false}, nil
				}

				// Parse start time from /proc/{PID}/stat field 22 (starttime in clock ticks)
				var elapsed float64
				fields := strings.Fields(string(statData))
				if len(fields) >= 22 {
					startTicks, parseErr := strconv.ParseUint(fields[21], 10, 64)
					if parseErr == nil {
						// Read system uptime
						uptimeData, uptimeErr := os.ReadFile("/proc/uptime")
						if uptimeErr == nil {
							uptimeParts := strings.Fields(string(uptimeData))
							if len(uptimeParts) >= 1 {
								uptimeSec, parseErr2 := strconv.ParseFloat(uptimeParts[0], 64)
								if parseErr2 == nil {
									// clock ticks per second (usually 100 on Linux)
									clkTck := 100.0
									procStartSec := float64(startTicks) / clkTck
									elapsed = uptimeSec - procStartSec
								}
							}
						}
					}
				}

				return ScreenRecordStatusOutput{
					Active:     true,
					PID:        pid,
					OutputPath: outPath,
					ElapsedS:   elapsed,
				}, nil
			},
		),

		// ── screen_annotate ───────────────────────────
		handler.TypedHandler[ScreenAnnotateInput, string](
			"screen_annotate",
			"Open a screenshot file in swappy for annotation. The swappy GUI opens as a fire-and-forget process. Not useful if swappy is not installed.",
			func(_ context.Context, input ScreenAnnotateInput) (string, error) {
				if err := screenCheckTool("swappy"); err != nil {
					return "", err
				}

				if input.FilePath == "" {
					return "", fmt.Errorf("[%s] file_path is required", handler.ErrInvalidParam)
				}

				if _, err := os.Stat(input.FilePath); err != nil {
					return "", fmt.Errorf("[%s] file not found: %s", handler.ErrInvalidParam, input.FilePath)
				}

				cmd := exec.Command("swappy", "-f", input.FilePath)
				if err := cmd.Start(); err != nil {
					return "", fmt.Errorf("failed to launch swappy: %w", err)
				}

				// Detach — swappy is a GUI app, don't wait for it.
				go cmd.Wait()

				return fmt.Sprintf("Opened %s in swappy for annotation", input.FilePath), nil
			},
		),

		// ── screen_screenshot_annotated ───────────────
		// Composed: capture screenshot with grim, then open in swappy.
		// Returns image content inline, so we use a raw handler.
		{
			Tool: mcp.Tool{
				Name:        "screen_screenshot_annotated",
				Description: "Take a screenshot and immediately open it in swappy for annotation. Composed: grim capture then swappy launch.",
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]any{
						"output_path": map[string]any{
							"type":        "string",
							"description": "File path to save the screenshot. Defaults to /tmp/screenshot-TIMESTAMP.png",
						},
						"region": map[string]any{
							"type":        "string",
							"description": "Region to capture in 'x,y WxH' format (e.g. '100,200 800x600'). Omit for full screen.",
						},
					},
				},
			},
			Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
				if err := screenCheckTool("grim"); err != nil {
					return handler.ErrorResult(err), nil
				}
				if err := screenCheckTool("swappy"); err != nil {
					return handler.ErrorResult(err), nil
				}

				var input ScreenScreenshotAnnotatedInput
				if req.Params.Arguments != nil {
					b, _ := json.Marshal(req.Params.Arguments)
					json.Unmarshal(b, &input)
				}

				outPath := input.OutputPath
				if outPath == "" {
					outPath = fmt.Sprintf("/tmp/screenshot-%s.png", screenTimestamp())
				}

				// Step 1: Capture with grim
				var grimArgs []string
				if input.Region != "" {
					grimArgs = append(grimArgs, "-g", input.Region)
				}
				grimArgs = append(grimArgs, outPath)

				if _, err := screenRunCmd("grim", grimArgs...); err != nil {
					return handler.ErrorResult(fmt.Errorf("grim capture failed: %w", err)), nil
				}

				// Step 2: Launch swappy (fire-and-forget)
				swappyCmd := exec.Command("swappy", "-f", outPath)
				if err := swappyCmd.Start(); err != nil {
					// Screenshot succeeded but swappy failed — still return the screenshot
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: fmt.Sprintf("Screenshot saved to %s (warning: failed to launch swappy: %v)", outPath, err),
							},
						},
					}, nil
				}
				go swappyCmd.Wait()

				// Resize for inline display (max 1568x1568)
				resized := outPath + ".resized.png"
				defer os.Remove(resized)

				if _, err := screenRunCmd("magick", outPath, "-resize", "1568x1568>", resized); err != nil {
					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: "text",
								Text: fmt.Sprintf("Screenshot saved to %s — opened in swappy for annotation (magick not available for inline preview)", outPath),
							},
						},
					}, nil
				}

				data, err := os.ReadFile(resized)
				if err != nil {
					return handler.ErrorResult(fmt.Errorf("failed to read screenshot: %w", err)), nil
				}
				b64 := base64.StdEncoding.EncodeToString(data)

				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("Screenshot saved to %s — opened in swappy for annotation", outPath),
						},
						mcp.ImageContent{
							Type:     "image",
							Data:     b64,
							MIMEType: "image/png",
						},
					},
				}, nil
			},
		},
	}
}
