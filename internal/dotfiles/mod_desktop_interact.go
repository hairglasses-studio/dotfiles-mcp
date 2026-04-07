// mod_desktop_interact.go — Composed see-think-act desktop automation workflows
package dotfiles

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// TSV parsing helpers
// ---------------------------------------------------------------------------

// tsvWord represents a single word from tesseract TSV output.
type tsvWord struct {
	Level    int
	PageNum  int
	BlockNum int
	ParNum   int
	LineNum  int
	WordNum  int
	Left     int
	Top      int
	Width    int
	Height   int
	Conf     float64
	Text     string
}

// parseTesseractTSV parses tesseract --tsv output into a slice of tsvWord.
// TSV columns: level, page_num, block_num, par_num, line_num, word_num,
// left, top, width, height, conf, text
func parseTesseractTSV(tsv string) []tsvWord {
	lines := strings.Split(strings.TrimSpace(tsv), "\n")
	if len(lines) < 2 {
		return nil
	}

	var words []tsvWord
	// Skip header line (index 0)
	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}

		text := fields[11]
		if strings.TrimSpace(text) == "" {
			continue
		}

		level, _ := strconv.Atoi(fields[0])
		pageNum, _ := strconv.Atoi(fields[1])
		blockNum, _ := strconv.Atoi(fields[2])
		parNum, _ := strconv.Atoi(fields[3])
		lineNum, _ := strconv.Atoi(fields[4])
		wordNum, _ := strconv.Atoi(fields[5])
		left, _ := strconv.Atoi(fields[6])
		top, _ := strconv.Atoi(fields[7])
		width, _ := strconv.Atoi(fields[8])
		height, _ := strconv.Atoi(fields[9])
		conf, _ := strconv.ParseFloat(fields[10], 64)

		words = append(words, tsvWord{
			Level:    level,
			PageNum:  pageNum,
			BlockNum: blockNum,
			ParNum:   parNum,
			LineNum:  lineNum,
			WordNum:  wordNum,
			Left:     left,
			Top:      top,
			Width:    width,
			Height:   height,
			Conf:     conf,
			Text:     text,
		})
	}
	return words
}

// textMatch holds the bounding box and metadata for a found text match.
type textMatch struct {
	Found      bool    `json:"found"`
	Text       string  `json:"text"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Confidence float64 `json:"confidence"`
}

// findTextInWords searches for target text in parsed TSV words.
// It handles single-word and multi-word matches by combining consecutive
// words on the same line. Returns the bounding box in screenshot-pixel space.
func findTextInWords(words []tsvWord, target string, caseSensitive bool) textMatch {
	if !caseSensitive {
		target = strings.ToLower(target)
	}

	targetWords := strings.Fields(target)
	if len(targetWords) == 0 {
		return textMatch{Found: false}
	}

	// Single-word match
	if len(targetWords) == 1 {
		for _, w := range words {
			wText := w.Text
			if !caseSensitive {
				wText = strings.ToLower(wText)
			}
			if strings.Contains(wText, targetWords[0]) {
				return textMatch{
					Found:      true,
					Text:       w.Text,
					X:          w.Left,
					Y:          w.Top,
					Width:      w.Width,
					Height:     w.Height,
					Confidence: w.Conf,
				}
			}
		}
		return textMatch{Found: false}
	}

	// Multi-word match: find consecutive words on the same line
	for i := 0; i <= len(words)-len(targetWords); i++ {
		matched := true
		for j, tw := range targetWords {
			if i+j >= len(words) {
				matched = false
				break
			}
			w := words[i+j]
			// Must be on the same line (same block, par, line)
			if j > 0 {
				prev := words[i+j-1]
				if w.BlockNum != prev.BlockNum || w.ParNum != prev.ParNum || w.LineNum != prev.LineNum {
					matched = false
					break
				}
			}
			wText := w.Text
			if !caseSensitive {
				wText = strings.ToLower(wText)
			}
			if !strings.Contains(wText, tw) {
				matched = false
				break
			}
		}

		if matched {
			first := words[i]
			last := words[i+len(targetWords)-1]
			// Bounding box spans from first word's left to last word's right
			x := first.Left
			y := first.Top
			right := last.Left + last.Width
			bottom := first.Top
			// Use max height across all matched words
			maxH := 0
			totalConf := 0.0
			for j := range targetWords {
				w := words[i+j]
				if w.Top < y {
					y = w.Top
				}
				if w.Top+w.Height > bottom+maxH {
					bottom = w.Top
					maxH = w.Height
				}
				if w.Height > maxH {
					maxH = w.Height
				}
				totalConf += w.Conf
			}
			height := (bottom + maxH) - y
			if height < maxH {
				height = maxH
			}

			// Collect matched text
			var parts []string
			for j := range targetWords {
				parts = append(parts, words[i+j].Text)
			}

			return textMatch{
				Found:      true,
				Text:       strings.Join(parts, " "),
				X:          x,
				Y:          y,
				Width:      right - x,
				Height:     height,
				Confidence: totalConf / float64(len(targetWords)),
			}
		}
	}

	return textMatch{Found: false}
}

// screenshotToDesktop converts screenshot-pixel coordinates to desktop logical
// coordinates. On Hyprland with a given scale factor, wayshot captures at physical
// pixels (logical * scale). To get back to logical coords (which ydotool needs),
// divide by scale and add the window's logical position offset.
func screenshotToDesktop(ssX, ssY int, scale float64, winX, winY int) (int, int) {
	if scale <= 0 {
		scale = 1
	}
	logX := int(math.Round(float64(ssX) / scale))
	logY := int(math.Round(float64(ssY) / scale))
	return logX + winX, logY + winY
}

// ---------------------------------------------------------------------------
// Internal capture + OCR pipeline
// ---------------------------------------------------------------------------

// diCaptureResult holds the result of a screenshot+OCR capture.
type diCaptureResult struct {
	imgPath   string
	b64       string
	ocrText   string
	tsvOutput string
	winX      int // window logical X (0 for full screen)
	winY      int // window logical Y (0 for full screen)
	scale     float64
}

// diCapture performs the screenshot capture → magick resize → tesseract OCR pipeline.
// address/class select a window; region captures a specific area; maxSize limits
// the resized image dimension.
func diCapture(address, class, region string, maxSize int, tsvMode bool) (*diCaptureResult, error) {

	ts := time.Now().UnixMilli()
	rawPath := fmt.Sprintf("/tmp/di-capture-%d.png", ts)
	resizedPath := fmt.Sprintf("/tmp/di-capture-%d-resized.png", ts)

	var captureRegion string
	var winX, winY int
	scale := 1.0

	// If address or class is specified, find the window and calculate its region
	if address != "" || class != "" {
		clientsJSON, err := runHyprctl("clients", "-j")
		if err != nil {
			return nil, fmt.Errorf("hyprctl clients failed: %w", err)
		}

		var clients []hyprClient
		if err := json.Unmarshal([]byte(clientsJSON), &clients); err != nil {
			return nil, fmt.Errorf("parse clients: %w", err)
		}

		var target *hyprClient
		for i := range clients {
			c := &clients[i]
			if address != "" && c.Address == address {
				target = c
				break
			}
			if class != "" && strings.EqualFold(c.Class, class) {
				target = c
				break
			}
		}
		if target == nil {
			selector := address
			if selector == "" {
				selector = class
			}
			return nil, fmt.Errorf("window not found: %s", selector)
		}

		winX = target.At[0]
		winY = target.At[1]

		// Get monitor scale
		monsJSON, err := runHyprctl("monitors", "-j")
		if err != nil {
			return nil, fmt.Errorf("hyprctl monitors failed: %w", err)
		}
		var monitors []hyprMonitor
		if err := json.Unmarshal([]byte(monsJSON), &monitors); err != nil {
			return nil, fmt.Errorf("parse monitors: %w", err)
		}
		for _, m := range monitors {
			if m.ID == target.Monitor {
				scale = m.Scale
				break
			}
		}
		if scale < 1 {
			scale = 1
		}

		px, py, pw, ph := windowRegion(target.At[0], target.At[1], target.Size[0], target.Size[1], scale)
		captureRegion = fmt.Sprintf("%d,%d %dx%d", px, py, pw, ph)
	} else if region != "" {
		captureRegion = region
	}

	// Capture screenshot
	if err := screenshotCapture(rawPath, captureRegion, ""); err != nil {
		return nil, fmt.Errorf("screenshot capture failed: %w", err)
	}

	// Resize with magick
	if maxSize <= 0 {
		maxSize = 1568
	}
	resizeSpec := fmt.Sprintf("%dx%d>", maxSize, maxSize)
	if _, err := screenRunCmd("magick", rawPath, "-resize", resizeSpec, resizedPath); err != nil {
		// Fallback: use raw if magick unavailable
		resizedPath = rawPath
	}

	// Read resized image for base64
	data, err := os.ReadFile(resizedPath)
	if err != nil {
		os.Remove(rawPath)
		if resizedPath != rawPath {
			os.Remove(resizedPath)
		}
		return nil, fmt.Errorf("failed to read image: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(data)

	// OCR
	result := &diCaptureResult{
		imgPath: rawPath,
		b64:     b64,
		winX:    winX,
		winY:    winY,
		scale:   scale,
	}

	if err := screenCheckTool("tesseract"); err != nil {
		// No tesseract — return image only
		os.Remove(rawPath)
		if resizedPath != rawPath {
			os.Remove(resizedPath)
		}
		return result, nil
	}

	if tsvMode {
		tsvOut, err := screenRunCmd("tesseract", rawPath, "stdout", "--tsv")
		if err != nil {
			// OCR failed but we still have the image
			os.Remove(rawPath)
			if resizedPath != rawPath {
				os.Remove(resizedPath)
			}
			return result, nil
		}
		result.tsvOutput = tsvOut
		result.ocrText = "" // TSV mode doesn't produce plain text directly
	} else {
		ocrOut, err := screenRunCmd("tesseract", rawPath, "stdout")
		if err != nil {
			os.Remove(rawPath)
			if resizedPath != rawPath {
				os.Remove(resizedPath)
			}
			return result, nil
		}
		result.ocrText = strings.TrimSpace(ocrOut)
	}

	// Cleanup temp files
	os.Remove(rawPath)
	if resizedPath != rawPath {
		os.Remove(resizedPath)
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------

type DesktopScreenshotOCRInput struct {
	Address string `json:"address,omitempty" jsonschema:"description=Window address (hex) to capture. Get from hypr_list_windows."`
	Class   string `json:"class,omitempty" jsonschema:"description=Window class name to capture (e.g. foot or firefox). First match is used."`
	Region  string `json:"region,omitempty" jsonschema:"description=Region to capture in 'x,y WxH' slurp format. Ignored if address or class is set."`
	MaxSize int    `json:"max_size,omitempty" jsonschema:"description=Max image dimension for resizing (default 1568). Smaller = fewer tokens."`
}

type DesktopFindTextInput struct {
	Text          string `json:"text" jsonschema:"required,description=Text to find on screen"`
	Address       string `json:"address,omitempty" jsonschema:"description=Window address to search in"`
	Class         string `json:"class,omitempty" jsonschema:"description=Window class name to search in"`
	CaseSensitive bool   `json:"case_sensitive,omitempty" jsonschema:"description=Case-sensitive text matching (default false)"`
}

type DesktopClickTextInput struct {
	Text          string `json:"text" jsonschema:"required,description=Text to find and click on screen"`
	Address       string `json:"address,omitempty" jsonschema:"description=Window address to search in"`
	Class         string `json:"class,omitempty" jsonschema:"description=Window class name to search in"`
	Button        string `json:"button,omitempty" jsonschema:"description=Mouse button: left (default), right, or middle"`
	CaseSensitive bool   `json:"case_sensitive,omitempty" jsonschema:"description=Case-sensitive text matching (default false)"`
}

type DesktopWaitForTextInput struct {
	Text          string `json:"text" jsonschema:"required,description=Text to wait for on screen"`
	Address       string `json:"address,omitempty" jsonschema:"description=Window address to search in"`
	Class         string `json:"class,omitempty" jsonschema:"description=Window class name to search in"`
	TimeoutS      int    `json:"timeout_s,omitempty" jsonschema:"description=Timeout in seconds (default 10)"`
	IntervalMs    int    `json:"interval_ms,omitempty" jsonschema:"description=Poll interval in milliseconds (default 500)"`
	CaseSensitive bool   `json:"case_sensitive,omitempty" jsonschema:"description=Case-sensitive text matching (default false)"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

// DesktopInteractModule provides composed see-think-act desktop automation tools.
type DesktopInteractModule struct{}

func (m *DesktopInteractModule) Name() string { return "desktop_interact" }
func (m *DesktopInteractModule) Description() string {
	return "Composed see-think-act desktop automation workflows"
}

func (m *DesktopInteractModule) Tools() []registry.ToolDefinition {
	// ── desktop_screenshot_ocr ────────────────────────
	// Returns ImageContent + TextContent, so we use a raw handler.
	screenshotOCR := registry.ToolDefinition{
		Tool: mcp.Tool{
			Name:        "desktop_screenshot_ocr",
			Description: "Screenshot the full desktop or a specific window (by address/class) and extract text via OCR. Returns both the image (for LLM vision) and extracted text. Use for seeing what is on screen.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"address": map[string]any{
						"type":        "string",
						"description": "Window address (hex) to capture. Get from hypr_list_windows.",
					},
					"class": map[string]any{
						"type":        "string",
						"description": "Window class name to capture (e.g. foot, firefox). First match is used.",
					},
					"region": map[string]any{
						"type":        "string",
						"description": "Region to capture in 'x,y WxH' slurp format. Ignored if address or class is set.",
					},
					"max_size": map[string]any{
						"type":        "integer",
						"description": "Max image dimension for resizing (default 1568). Smaller = fewer tokens.",
						"default":     1568,
					},
				},
			},
		},
		Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
			var input DesktopScreenshotOCRInput
			if req.Params.Arguments != nil {
				b, _ := json.Marshal(req.Params.Arguments)
				json.Unmarshal(b, &input)
			}

			cap, err := diCapture(input.Address, input.Class, input.Region, input.MaxSize, false)
			if err != nil {
				return handler.ErrorResult(err), nil
			}

			var content []mcp.Content

			// Image content
			content = append(content, mcp.ImageContent{
				Type:     "image",
				Data:     cap.b64,
				MIMEType: "image/png",
			})

			// Text content: OCR text + window geometry info
			info := map[string]any{
				"ocr_text": cap.ocrText,
				"window": map[string]any{
					"x":     cap.winX,
					"y":     cap.winY,
					"scale": cap.scale,
				},
			}
			infoJSON, _ := json.MarshalIndent(info, "", "  ")

			content = append(content, mcp.TextContent{
				Type: "text",
				Text: string(infoJSON),
			})

			return &registry.CallToolResult{Content: content}, nil
		},
		Category: "desktop_interact",
		Tags:     []string{"desktop", "screenshot", "ocr", "vision", "see"},
	}
	screenshotOCR.SearchTerms = []string{"screenshot ocr", "screen text", "read screen", "capture text", "desktop vision"}

	// ── desktop_find_text ─────────────────────────────
	findText := registry.ToolDefinition{
		Tool: mcp.Tool{
			Name:        "desktop_find_text",
			Description: "Screenshot + OCR + locate text on screen. Returns bounding box coordinates in desktop space suitable for ydotool. Use to find where a button or label appears before clicking.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "Text to find on screen.",
					},
					"address": map[string]any{
						"type":        "string",
						"description": "Window address to search in.",
					},
					"class": map[string]any{
						"type":        "string",
						"description": "Window class name to search in.",
					},
					"case_sensitive": map[string]any{
						"type":        "boolean",
						"description": "Case-sensitive text matching (default false).",
						"default":     false,
					},
				},
				Required: []string{"text"},
			},
		},
		Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
			var input DesktopFindTextInput
			if req.Params.Arguments != nil {
				b, _ := json.Marshal(req.Params.Arguments)
				json.Unmarshal(b, &input)
			}

			if input.Text == "" {
				return handler.ErrorResult(fmt.Errorf("[%s] text is required", handler.ErrInvalidParam)), nil
			}

			cap, err := diCapture(input.Address, input.Class, "", 0, true)
			if err != nil {
				return handler.ErrorResult(err), nil
			}

			if cap.tsvOutput == "" {
				return handler.ErrorResult(fmt.Errorf("tesseract produced no TSV output")), nil
			}

			words := parseTesseractTSV(cap.tsvOutput)
			match := findTextInWords(words, input.Text, input.CaseSensitive)

			if !match.Found {
				result := map[string]any{
					"found": false,
					"text":  input.Text,
				}
				resultJSON, _ := json.MarshalIndent(result, "", "  ")
				return &registry.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{Type: "text", Text: string(resultJSON)},
					},
				}, nil
			}

			// Map screenshot-space coordinates to desktop-space
			desktopX, desktopY := screenshotToDesktop(match.X, match.Y, cap.scale, cap.winX, cap.winY)
			desktopW := int(math.Round(float64(match.Width) / cap.scale))
			desktopH := int(math.Round(float64(match.Height) / cap.scale))

			result := map[string]any{
				"found":      true,
				"text":       match.Text,
				"x":          desktopX,
				"y":          desktopY,
				"width":      desktopW,
				"height":     desktopH,
				"confidence": match.Confidence,
			}
			resultJSON, _ := json.MarshalIndent(result, "", "  ")

			return &registry.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: string(resultJSON)},
				},
			}, nil
		},
		Category: "desktop_interact",
		Tags:     []string{"desktop", "find", "text", "ocr", "locate"},
	}
	findText.SearchTerms = []string{"find text", "locate text", "text position", "text bounding box", "ocr find"}

	// ── desktop_click_text ────────────────────────────
	clickText := registry.ToolDefinition{
		Tool: mcp.Tool{
			Name:        "desktop_click_text",
			Description: "DESTRUCTIVE: Find text on screen via OCR and click at its center. The atomic 'click the button labeled X' primitive. Requires explicit approval.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "Text to find and click.",
					},
					"address": map[string]any{
						"type":        "string",
						"description": "Window address to search in.",
					},
					"class": map[string]any{
						"type":        "string",
						"description": "Window class name to search in.",
					},
					"button": map[string]any{
						"type":        "string",
						"description": "Mouse button: left (default), right, or middle.",
						"default":     "left",
					},
					"case_sensitive": map[string]any{
						"type":        "boolean",
						"description": "Case-sensitive text matching (default false).",
						"default":     false,
					},
				},
				Required: []string{"text"},
			},
		},
		Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
			var input DesktopClickTextInput
			if req.Params.Arguments != nil {
				b, _ := json.Marshal(req.Params.Arguments)
				json.Unmarshal(b, &input)
			}

			if input.Text == "" {
				return handler.ErrorResult(fmt.Errorf("[%s] text is required", handler.ErrInvalidParam)), nil
			}

			if !hasCmd("ydotool") {
				return handler.ErrorResult(fmt.Errorf("ydotool not found on PATH")), nil
			}

			cap, err := diCapture(input.Address, input.Class, "", 0, true)
			if err != nil {
				return handler.ErrorResult(err), nil
			}

			if cap.tsvOutput == "" {
				return handler.ErrorResult(fmt.Errorf("tesseract produced no TSV output")), nil
			}

			words := parseTesseractTSV(cap.tsvOutput)
			match := findTextInWords(words, input.Text, input.CaseSensitive)

			if !match.Found {
				return handler.ErrorResult(fmt.Errorf("text %q not found on screen", input.Text)), nil
			}

			// Calculate center in screenshot space, then map to desktop space
			centerSSX := match.X + match.Width/2
			centerSSY := match.Y + match.Height/2
			clickX, clickY := screenshotToDesktop(centerSSX, centerSSY, cap.scale, cap.winX, cap.winY)

			// Move cursor and click
			if _, err := screenRunCmd("ydotool", "mousemove", "--absolute",
				"-x", strconv.Itoa(clickX), "-y", strconv.Itoa(clickY)); err != nil {
				return handler.ErrorResult(fmt.Errorf("mousemove failed: %w", err)), nil
			}

			button := input.Button
			if button == "" {
				button = "left"
			}
			buttonCode := ydotoolButtonCode(button)

			if _, err := screenRunCmd("ydotool", "click", "--next-delay", "0", buttonCode); err != nil {
				return handler.ErrorResult(fmt.Errorf("click failed: %w", err)), nil
			}

			return &registry.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("clicked %s on %q at desktop (%d, %d)", button, match.Text, clickX, clickY),
					},
				},
			}, nil
		},
		IsWrite:  true,
		Category: "desktop_interact",
		Tags:     []string{"desktop", "click", "text", "ocr", "interact"},
	}
	clickText.SearchTerms = []string{"click text", "click button", "click label", "ocr click", "desktop click"}

	// ── desktop_wait_for_text ─────────────────────────
	// Returns ImageContent on success, so raw handler.
	waitForText := registry.ToolDefinition{
		Tool: mcp.Tool{
			Name:        "desktop_wait_for_text",
			Description: "Poll screenshot+OCR until text appears on screen or timeout. Returns coordinates and final screenshot when found. Use to wait for UI state changes (dialogs, loading, notifications).",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "Text to wait for on screen.",
					},
					"address": map[string]any{
						"type":        "string",
						"description": "Window address to search in.",
					},
					"class": map[string]any{
						"type":        "string",
						"description": "Window class name to search in.",
					},
					"timeout_s": map[string]any{
						"type":        "integer",
						"description": "Timeout in seconds (default 10).",
						"default":     10,
					},
					"interval_ms": map[string]any{
						"type":        "integer",
						"description": "Poll interval in milliseconds (default 500).",
						"default":     500,
					},
					"case_sensitive": map[string]any{
						"type":        "boolean",
						"description": "Case-sensitive text matching (default false).",
						"default":     false,
					},
				},
				Required: []string{"text"},
			},
		},
		Handler: func(_ context.Context, req registry.CallToolRequest) (*registry.CallToolResult, error) {
			var input DesktopWaitForTextInput
			if req.Params.Arguments != nil {
				b, _ := json.Marshal(req.Params.Arguments)
				json.Unmarshal(b, &input)
			}

			if input.Text == "" {
				return handler.ErrorResult(fmt.Errorf("[%s] text is required", handler.ErrInvalidParam)), nil
			}

			timeout := input.TimeoutS
			if timeout <= 0 {
				timeout = 10
			}
			interval := input.IntervalMs
			if interval <= 0 {
				interval = 500
			}

			deadline := time.Now().Add(time.Duration(timeout) * time.Second)
			start := time.Now()

			for {
				cap, err := diCapture(input.Address, input.Class, "", 0, true)
				if err != nil {
					// Capture failed — retry unless timed out
					if time.Now().After(deadline) {
						return handler.ErrorResult(fmt.Errorf("timeout after %ds waiting for %q: %w", timeout, input.Text, err)), nil
					}
					time.Sleep(time.Duration(interval) * time.Millisecond)
					continue
				}

				if cap.tsvOutput != "" {
					words := parseTesseractTSV(cap.tsvOutput)
					match := findTextInWords(words, input.Text, input.CaseSensitive)

					if match.Found {
						elapsed := time.Since(start).Milliseconds()

						// Map to desktop coordinates
						desktopX, desktopY := screenshotToDesktop(match.X, match.Y, cap.scale, cap.winX, cap.winY)

						result := map[string]any{
							"found":      true,
							"elapsed_ms": elapsed,
							"text":       match.Text,
							"x":          desktopX,
							"y":          desktopY,
						}
						resultJSON, _ := json.MarshalIndent(result, "", "  ")

						var content []mcp.Content
						content = append(content, mcp.TextContent{
							Type: "text",
							Text: string(resultJSON),
						})
						// Include final screenshot
						content = append(content, mcp.ImageContent{
							Type:     "image",
							Data:     cap.b64,
							MIMEType: "image/png",
						})

						return &registry.CallToolResult{Content: content}, nil
					}
				}

				if time.Now().After(deadline) {
					elapsed := time.Since(start).Milliseconds()
					result := map[string]any{
						"found":      false,
						"elapsed_ms": elapsed,
						"text":       input.Text,
					}
					resultJSON, _ := json.MarshalIndent(result, "", "  ")

					return &registry.CallToolResult{
						Content: []mcp.Content{
							mcp.TextContent{Type: "text", Text: string(resultJSON)},
						},
					}, nil
				}

				time.Sleep(time.Duration(interval) * time.Millisecond)
			}
		},
		Category: "desktop_interact",
		Tags:     []string{"desktop", "wait", "text", "poll", "ocr"},
	}
	waitForText.SearchTerms = []string{"wait for text", "poll screen", "wait ui", "text appears", "wait for button"}

	return []registry.ToolDefinition{screenshotOCR, findText, clickText, waitForText}
}
