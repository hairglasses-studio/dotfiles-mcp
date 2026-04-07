package dotfiles

import (
	"testing"

	"github.com/hairglasses-studio/mcpkit/mcptest"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// DesktopInteractModule Registration
// ---------------------------------------------------------------------------

func TestDesktopInteractModuleRegistration(t *testing.T) {
	m := &DesktopInteractModule{}

	if m.Name() != "desktop_interact" {
		t.Errorf("Name() = %q, want %q", m.Name(), "desktop_interact")
	}
	if m.Description() != "Composed see-think-act desktop automation workflows" {
		t.Errorf("Description() = %q, want %q", m.Description(), "Composed see-think-act desktop automation workflows")
	}

	tools := m.Tools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 desktop_interact tools, got %d", len(tools))
	}

	reg := registry.NewToolRegistry()
	reg.RegisterModule(m)
	srv := mcptest.NewServer(t, reg)

	for _, want := range []string{
		"desktop_screenshot_ocr",
		"desktop_find_text",
		"desktop_click_text",
		"desktop_wait_for_text",
	} {
		if !srv.HasTool(want) {
			t.Errorf("missing tool: %s", want)
		}
	}
}

// ---------------------------------------------------------------------------
// TSV Parsing
// ---------------------------------------------------------------------------

func TestParseTesseractTSV_Basic(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t100\t200\t50\t20\t95.5\tHello\n" +
		"5\t1\t1\t1\t1\t2\t160\t200\t60\t20\t92.3\tWorld\n" +
		"5\t1\t1\t1\t2\t1\t100\t250\t80\t20\t88.0\tFoo\n"

	words := parseTesseractTSV(tsv)
	if len(words) != 3 {
		t.Fatalf("expected 3 words, got %d", len(words))
	}

	// Check first word
	w := words[0]
	if w.Text != "Hello" {
		t.Errorf("word[0].Text = %q, want %q", w.Text, "Hello")
	}
	if w.Left != 100 || w.Top != 200 {
		t.Errorf("word[0] position = (%d, %d), want (100, 200)", w.Left, w.Top)
	}
	if w.Width != 50 || w.Height != 20 {
		t.Errorf("word[0] size = (%d, %d), want (50, 20)", w.Width, w.Height)
	}
	if w.Conf != 95.5 {
		t.Errorf("word[0].Conf = %f, want 95.5", w.Conf)
	}
	if w.LineNum != 1 {
		t.Errorf("word[0].LineNum = %d, want 1", w.LineNum)
	}

	// Check last word is on a different line
	if words[2].LineNum != 2 {
		t.Errorf("word[2].LineNum = %d, want 2", words[2].LineNum)
	}
}

func TestParseTesseractTSV_Empty(t *testing.T) {
	words := parseTesseractTSV("")
	if len(words) != 0 {
		t.Errorf("expected 0 words for empty input, got %d", len(words))
	}
}

func TestParseTesseractTSV_HeaderOnly(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"
	words := parseTesseractTSV(tsv)
	if len(words) != 0 {
		t.Errorf("expected 0 words for header-only input, got %d", len(words))
	}
}

func TestParseTesseractTSV_SkipsEmptyText(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t100\t200\t50\t20\t95.5\tHello\n" +
		"4\t1\t1\t1\t1\t0\t0\t0\t0\t0\t-1\t\n" +
		"5\t1\t1\t1\t1\t2\t160\t200\t60\t20\t92.3\tWorld\n"

	words := parseTesseractTSV(tsv)
	if len(words) != 2 {
		t.Fatalf("expected 2 words (skipping empty), got %d", len(words))
	}
	if words[0].Text != "Hello" || words[1].Text != "World" {
		t.Errorf("words = [%q, %q], want [Hello, World]", words[0].Text, words[1].Text)
	}
}

// ---------------------------------------------------------------------------
// findTextInWords
// ---------------------------------------------------------------------------

func TestFindTextInWords_SingleWord(t *testing.T) {
	words := []tsvWord{
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 1, WordNum: 1, Left: 100, Top: 200, Width: 50, Height: 20, Conf: 95.0, Text: "Hello"},
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 1, WordNum: 2, Left: 160, Top: 200, Width: 60, Height: 20, Conf: 92.0, Text: "World"},
	}

	match := findTextInWords(words, "Hello", false)
	if !match.Found {
		t.Fatal("expected to find 'Hello'")
	}
	if match.Text != "Hello" {
		t.Errorf("match.Text = %q, want %q", match.Text, "Hello")
	}
	if match.X != 100 || match.Y != 200 {
		t.Errorf("match position = (%d, %d), want (100, 200)", match.X, match.Y)
	}
}

func TestFindTextInWords_CaseInsensitive(t *testing.T) {
	words := []tsvWord{
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 1, WordNum: 1, Left: 100, Top: 200, Width: 50, Height: 20, Conf: 95.0, Text: "HELLO"},
	}

	match := findTextInWords(words, "hello", false)
	if !match.Found {
		t.Fatal("expected case-insensitive match for 'hello'")
	}

	match = findTextInWords(words, "hello", true)
	if match.Found {
		t.Error("expected case-sensitive search to NOT find 'hello' when text is 'HELLO'")
	}
}

func TestFindTextInWords_MultiWord(t *testing.T) {
	words := []tsvWord{
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 1, WordNum: 1, Left: 100, Top: 200, Width: 50, Height: 20, Conf: 95.0, Text: "Click"},
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 1, WordNum: 2, Left: 160, Top: 200, Width: 40, Height: 20, Conf: 90.0, Text: "Here"},
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 2, WordNum: 1, Left: 100, Top: 250, Width: 70, Height: 20, Conf: 88.0, Text: "Other"},
	}

	match := findTextInWords(words, "Click Here", false)
	if !match.Found {
		t.Fatal("expected to find 'Click Here'")
	}
	if match.Text != "Click Here" {
		t.Errorf("match.Text = %q, want %q", match.Text, "Click Here")
	}
	// Bounding box should span both words
	if match.X != 100 {
		t.Errorf("match.X = %d, want 100", match.X)
	}
	expectedWidth := (160 + 40) - 100 // last.Left + last.Width - first.Left
	if match.Width != expectedWidth {
		t.Errorf("match.Width = %d, want %d", match.Width, expectedWidth)
	}
}

func TestFindTextInWords_MultiWordCrossingLines(t *testing.T) {
	// Words on different lines should NOT match
	words := []tsvWord{
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 1, WordNum: 1, Left: 100, Top: 200, Width: 50, Height: 20, Conf: 95.0, Text: "Click"},
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 2, WordNum: 1, Left: 100, Top: 250, Width: 40, Height: 20, Conf: 90.0, Text: "Here"},
	}

	match := findTextInWords(words, "Click Here", false)
	if match.Found {
		t.Error("expected NOT to find 'Click Here' when words are on different lines")
	}
}

func TestFindTextInWords_NotFound(t *testing.T) {
	words := []tsvWord{
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 1, WordNum: 1, Left: 100, Top: 200, Width: 50, Height: 20, Conf: 95.0, Text: "Hello"},
	}

	match := findTextInWords(words, "Goodbye", false)
	if match.Found {
		t.Error("expected NOT to find 'Goodbye'")
	}
}

func TestFindTextInWords_EmptyTarget(t *testing.T) {
	words := []tsvWord{
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 1, WordNum: 1, Left: 100, Top: 200, Width: 50, Height: 20, Conf: 95.0, Text: "Hello"},
	}

	match := findTextInWords(words, "", false)
	if match.Found {
		t.Error("expected NOT to find empty string")
	}
}

func TestFindTextInWords_PartialMatch(t *testing.T) {
	words := []tsvWord{
		{Level: 5, BlockNum: 1, ParNum: 1, LineNum: 1, WordNum: 1, Left: 100, Top: 200, Width: 80, Height: 20, Conf: 95.0, Text: "Submit"},
	}

	// "Sub" is contained in "Submit"
	match := findTextInWords(words, "Sub", false)
	if !match.Found {
		t.Fatal("expected partial match for 'Sub' in 'Submit'")
	}
}

// ---------------------------------------------------------------------------
// Coordinate mapping
// ---------------------------------------------------------------------------

func TestScreenshotToDesktop_Scale1(t *testing.T) {
	// At scale 1, screenshot pixels = logical pixels
	x, y := screenshotToDesktop(100, 200, 1.0, 0, 0)
	if x != 100 || y != 200 {
		t.Errorf("at scale 1 with no offset: got (%d, %d), want (100, 200)", x, y)
	}
}

func TestScreenshotToDesktop_Scale2(t *testing.T) {
	// At scale 2, screenshot pixel 200 = logical 100
	x, y := screenshotToDesktop(200, 400, 2.0, 0, 0)
	if x != 100 || y != 200 {
		t.Errorf("at scale 2 with no offset: got (%d, %d), want (100, 200)", x, y)
	}
}

func TestScreenshotToDesktop_WithOffset(t *testing.T) {
	// Window at logical position (50, 30), scale 1
	x, y := screenshotToDesktop(100, 200, 1.0, 50, 30)
	if x != 150 || y != 230 {
		t.Errorf("at scale 1 with offset (50,30): got (%d, %d), want (150, 230)", x, y)
	}
}

func TestScreenshotToDesktop_Scale2WithOffset(t *testing.T) {
	// Window at logical position (50, 30), scale 2
	// Screenshot pixel 200 → logical 100 → + offset 50 = 150
	x, y := screenshotToDesktop(200, 400, 2.0, 50, 30)
	if x != 150 || y != 230 {
		t.Errorf("at scale 2 with offset (50,30): got (%d, %d), want (150, 230)", x, y)
	}
}

func TestScreenshotToDesktop_ZeroScale(t *testing.T) {
	// Zero scale should fallback to 1
	x, y := screenshotToDesktop(100, 200, 0, 0, 0)
	if x != 100 || y != 200 {
		t.Errorf("at scale 0 (fallback to 1): got (%d, %d), want (100, 200)", x, y)
	}
}

func TestScreenshotToDesktop_FractionalScale(t *testing.T) {
	// Scale 1.5: screenshot pixel 150 → logical 100
	x, y := screenshotToDesktop(150, 300, 1.5, 0, 0)
	if x != 100 || y != 200 {
		t.Errorf("at scale 1.5: got (%d, %d), want (100, 200)", x, y)
	}
}
