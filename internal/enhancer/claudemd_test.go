package enhancer

import (
	"strings"
	"testing"
)

func TestCheckClaudeMD_ExcessiveLength(t *testing.T) {
	content := strings.Repeat("line\n", 250)
	path := writeTempCLAUDEMD(t, content)

	results, err := CheckClaudeMD(path)
	if err != nil {
		t.Fatal(err)
	}
	assertClaudeMDCategory(t, results, "excessive-length")
}

func TestCheckClaudeMD_OvertriggerLanguage(t *testing.T) {
	content := "# Rules\n\nCRITICAL: You MUST always follow the coding standards.\nIMPORTANT: You SHOULD never skip tests."
	path := writeTempCLAUDEMD(t, content)

	results, err := CheckClaudeMD(path)
	if err != nil {
		t.Fatal(err)
	}
	assertClaudeMDCategory(t, results, "overtrigger-language")
}

func TestCheckClaudeMD_InlineCode(t *testing.T) {
	content := "# Code\n\n```go\nfunc a() {}\n```\n\n```go\nfunc b() {}\n```\n\n```go\nfunc c() {}\n```\n\n```go\nfunc d() {}\n```\n"
	path := writeTempCLAUDEMD(t, content)

	results, err := CheckClaudeMD(path)
	if err != nil {
		t.Fatal(err)
	}
	assertClaudeMDCategory(t, results, "inline-code")
}

func TestCheckClaudeMD_Healthy(t *testing.T) {
	content := "# Project\n\nThis is a simple Go project.\n\n## Standards\n\nUse gofmt."
	path := writeTempCLAUDEMD(t, content)

	results, err := CheckClaudeMD(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) > 0 {
		t.Errorf("Healthy CLAUDE.md should have no findings, got %d", len(results))
	}
}

func TestCheckClaudeMD_MissingFile(t *testing.T) {
	_, err := CheckClaudeMD("/nonexistent/CLAUDE.md")
	if err == nil {
		t.Error("Should return error for missing file")
	}
}

func TestCheckClaudeMD_AggressiveCaps(t *testing.T) {
	content := "CRITICAL rule one.\nIMPORTANT rule two.\nMUST do three.\nALWAYS do four.\nNEVER skip five."
	path := writeTempCLAUDEMD(t, content)

	results, err := CheckClaudeMD(path)
	if err != nil {
		t.Fatal(err)
	}
	assertClaudeMDCategory(t, results, "aggressive-caps")
}

func TestCheckClaudeMD_StyleGuide(t *testing.T) {
	content := "# Style\n\nIndent with tabs.\nUse snake_case naming convention.\nSort imports alphabetically.\nLine length should be 120."
	path := writeTempCLAUDEMD(t, content)

	results, err := CheckClaudeMD(path)
	if err != nil {
		t.Fatal(err)
	}
	assertClaudeMDCategory(t, results, "style-guide-content")
}

func TestCheckClaudeMD_MissingHeaders(t *testing.T) {
	// More than 20 lines, no headers
	content := strings.Repeat("This is a line without any headers.\n", 25)
	path := writeTempCLAUDEMD(t, content)

	results, err := CheckClaudeMD(path)
	if err != nil {
		t.Fatal(err)
	}
	assertClaudeMDCategory(t, results, "missing-headers")
}

// assertClaudeMDCategory checks that at least one ClaudeMDResult has the given category.
func assertClaudeMDCategory(t *testing.T, results []ClaudeMDResult, category string) {
	t.Helper()
	for _, r := range results {
		if r.Category == category {
			return
		}
	}
	t.Errorf("expected ClaudeMD category %q, not found in %d results", category, len(results))
}
