// mod_ops.go — Standardized SDLC operations for autonomous Claude Code development loops
package dotfiles

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/dotfiles-mcp/internal/remediation"
	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	opsBuildTimeout = 120 * time.Second
	opsTestTimeout  = 120 * time.Second
	opsCIPollMax    = 300 * time.Second
	opsCIPollDelay  = 15 * time.Second
)

// ---------------------------------------------------------------------------
// Session state — file-based persistence at ~/.local/state/ops/
// ---------------------------------------------------------------------------

var opsSessionMu sync.RWMutex // guards concurrent file access

type OpsSession struct {
	ID          string            `json:"id"`
	Repo        string            `json:"repo"`
	Branch      string            `json:"branch"`
	Description string            `json:"description,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	Iterations  []IterationRecord `json:"iterations"`
}

type IterationRecord struct {
	Number      int    `json:"number"`
	StartedAt   int64  `json:"started_at"`
	DurationMs  int64  `json:"duration_ms"`
	Status      string `json:"status"`
	ErrorCount  int    `json:"error_count"`
	BuildOK     bool   `json:"build_ok"`
	TestsPassed int    `json:"tests_passed"`
	TestsFailed int    `json:"tests_failed"`
}

type IterationSummary struct {
	Number     int    `json:"number"`
	Status     string `json:"status"`
	ErrorCount int    `json:"error_count"`
	DurationMs int64  `json:"duration_ms"`
}

func opsID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	return hex.EncodeToString(b)
}

func opsStateDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".local", "state", "ops")
}

func opsSessionDir(id string) string {
	return filepath.Join(opsStateDir(), id)
}

func opsWriteSession(session *OpsSession) error {
	dir := opsSessionDir(session.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "session.json"), data, 0o644)
}

func opsReadSession(id string) (*OpsSession, error) {
	data, err := os.ReadFile(filepath.Join(opsSessionDir(id), "session.json"))
	if err != nil {
		return nil, fmt.Errorf("[%s] session %q not found", handler.ErrNotFound, id)
	}
	var session OpsSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("corrupt session %q: %w", id, err)
	}
	return &session, nil
}

func opsAppendIteration(id string, record IterationRecord) error {
	dir := opsSessionDir(id)
	f, err := os.OpenFile(filepath.Join(dir, "iterations.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, _ := json.Marshal(record)
	_, err = f.Write(append(data, '\n'))
	return err
}

func opsReadIterations(id string) []IterationRecord {
	data, err := os.ReadFile(filepath.Join(opsSessionDir(id), "iterations.jsonl"))
	if err != nil {
		return nil
	}
	var records []IterationRecord
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		var r IterationRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err == nil {
			records = append(records, r)
		}
	}
	return records
}

func opsListSessionIDs() []string {
	entries, err := os.ReadDir(opsStateDir())
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids
}

// ---------------------------------------------------------------------------
// Shared types
// ---------------------------------------------------------------------------

type CompileError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
	Package string `json:"package,omitempty"`
}

type TestFailure struct {
	Package string  `json:"package"`
	Test    string  `json:"test"`
	Output  string  `json:"output"`
	Elapsed float64 `json:"elapsed_sec"`
}

type AnalyzedIssue struct {
	Category    string                   `json:"category"`
	File        string                   `json:"file"`
	Line        int                      `json:"line,omitempty"`
	Message     string                   `json:"message"`
	Suggestion  string                   `json:"suggestion"`
	Severity    string                   `json:"severity"`
	ErrorCode   string                   `json:"error_code,omitempty"`
	Remediation *remediation.Remediation `json:"remediation,omitempty"`
}

type ChangedFile struct {
	Path       string `json:"path"`
	Status     string `json:"status"`
	Insertions int    `json:"insertions"`
	Deletions  int    `json:"deletions"`
	GoPackage  string `json:"go_package,omitempty"`
}

type CICheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion,omitempty"`
	URL        string `json:"url,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

type PrePushStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	ErrorCount int    `json:"error_count,omitempty"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var compileErrorRe = regexp.MustCompile(`^(.+):(\d+):(\d+): (.+)$`)

// opsHasNPMScript checks if a package.json has a given script defined.
func opsHasNPMScript(repoPath, script string) bool {
	data, err := os.ReadFile(filepath.Join(repoPath, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	_, ok := pkg.Scripts[script]
	return ok
}

var conventionalRe = regexp.MustCompile(`^(feat|fix|chore|docs|refactor|test|ci|perf|build)(\(.+\))?: .+`)

func opsResolveRepo(repo string) (string, error) {
	if repo != "" {
		abs, err := filepath.Abs(repo)
		if err != nil {
			return "", fmt.Errorf("[%s] invalid repo path: %w", handler.ErrInvalidParam, err)
		}
		return abs, nil
	}
	return os.Getwd()
}

func opsDetectLanguage(repoPath string) string {
	if _, err := os.Stat(repoPath); err != nil {
		return "unknown"
	}
	dir, err := filepath.Abs(repoPath)
	if err != nil {
		dir = repoPath
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return "go"
		}
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			return "node"
		}
		if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
			return "python"
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "unknown"
}

func opsDefaultBase(repoPath string) string {
	if out, err := runGit(repoPath, "rev-parse", "--verify", "main"); err == nil && out != "" {
		// Check if we're on main — if so, diff against HEAD
		branch, _ := runGit(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
		if strings.TrimSpace(branch) == "main" {
			return "HEAD"
		}
		return "main"
	}
	if _, err := runGit(repoPath, "rev-parse", "--verify", "master"); err == nil {
		return "master"
	}
	return "HEAD"
}

func opsCurrentBranch(repoPath string) string {
	out, err := runGit(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func opsRunWithTimeout(timeout time.Duration, repoPath, name string, args ...string) (string, string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = repoPath
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return stdout.String(), stderr.String(), -1, err
		}
	}
	return stdout.String(), stderr.String(), exitCode, nil
}

func opsParseCompileErrors(stderr string) []CompileError {
	var errors []CompileError
	for _, line := range strings.Split(stderr, "\n") {
		m := compileErrorRe.FindStringSubmatch(strings.TrimSpace(line))
		if m != nil {
			lineNum, _ := strconv.Atoi(m[2])
			col, _ := strconv.Atoi(m[3])
			errors = append(errors, CompileError{
				File:    m[1],
				Line:    lineNum,
				Column:  col,
				Message: m[4],
			})
		}
	}
	return errors
}

type goTestEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

func opsParseGoTestJSON(output string) (passed []string, failures []TestFailure, skipped int) {
	// Collect output per test
	testOutput := make(map[string]*strings.Builder)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var evt goTestEvent
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
			continue
		}
		if evt.Test == "" {
			continue // package-level event
		}
		key := evt.Package + "/" + evt.Test
		switch evt.Action {
		case "output":
			if testOutput[key] == nil {
				testOutput[key] = &strings.Builder{}
			}
			testOutput[key].WriteString(evt.Output)
		case "pass":
			passed = append(passed, key)
		case "fail":
			out := ""
			if b := testOutput[key]; b != nil {
				out = b.String()
			}
			failures = append(failures, TestFailure{
				Package: evt.Package,
				Test:    evt.Test,
				Output:  out,
				Elapsed: evt.Elapsed,
			})
		case "skip":
			skipped++
		}
	}
	return
}

// opsParseNodeTestJSON parses Jest or Vitest JSON output into pass/fail/skip.
// Detects format automatically: Jest uses testResults[].testResults[],
// Vitest uses testResults[].assertionResults[].
func opsParseNodeTestJSON(output string) (passed []string, failures []TestFailure, skipped int) {
	// Try to parse as generic JSON first to detect format
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		// Not JSON — fallback to line matching
		return opsParseNodeTestLines(output)
	}

	// Check for Vitest format (has assertionResults at suite level)
	// Try Jest format first (more common)
	type jestTest struct {
		FullName string `json:"fullName"`
		Title    string `json:"title"`
		Status   string `json:"status"`
		Duration int    `json:"duration"`
	}
	type jestSuite struct {
		TestFilePath     string     `json:"testFilePath"`
		Name             string     `json:"name"`
		TestResults      []jestTest `json:"testResults"`
		AssertionResults []jestTest `json:"assertionResults"` // Vitest uses this field name
		Message          string     `json:"message"`
	}
	var result struct {
		TestResults []jestSuite `json:"testResults"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return opsParseNodeTestLines(output)
	}

	for _, suite := range result.TestResults {
		filePath := suite.TestFilePath
		if filePath == "" {
			filePath = suite.Name
		}
		// Use whichever field has data (Jest=TestResults, Vitest=AssertionResults)
		tests := suite.TestResults
		if len(tests) == 0 {
			tests = suite.AssertionResults
		}
		for _, test := range tests {
			name := test.FullName
			if name == "" {
				name = test.Title
			}
			key := filePath + "/" + name
			switch test.Status {
			case "passed":
				passed = append(passed, key)
			case "failed":
				failures = append(failures, TestFailure{
					Package: filePath,
					Test:    name,
					Output:  suite.Message,
					Elapsed: float64(test.Duration) / 1000.0,
				})
			case "pending", "skipped", "todo":
				skipped++
			}
		}
	}
	return
}

// opsParseNodeTestLines is the text-based fallback for non-JSON test output.
func opsParseNodeTestLines(output string) (passed []string, failures []TestFailure, skipped int) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Match common test runner output patterns
		if strings.Contains(line, "✓") || strings.Contains(line, "PASS") {
			passed = append(passed, line)
		} else if strings.Contains(line, "✗") || strings.Contains(line, "FAIL") || strings.Contains(line, "✕") {
			failures = append(failures, TestFailure{Test: line, Output: line})
		}
	}
	return
}

// opsParsePytestOutput parses pytest -v output into pass/fail/skip.
// Handles standard verbose format: "test_file.py::test_name PASSED/FAILED/SKIPPED"
func opsParsePytestOutput(output string) (passed []string, failures []TestFailure, skipped int) {
	// Collect failure output blocks using index (not pointer, since append can relocate)
	currentFailIdx := -1
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)

		// Match "path::test_name PASSED/FAILED/SKIPPED"
		if strings.HasSuffix(trimmed, " PASSED") {
			passed = append(passed, strings.TrimSuffix(trimmed, " PASSED"))
			currentFailIdx = -1
		} else if strings.HasSuffix(trimmed, " FAILED") {
			name := strings.TrimSuffix(trimmed, " FAILED")
			failures = append(failures, TestFailure{Test: name, Output: ""})
			currentFailIdx = len(failures) - 1
		} else if strings.HasSuffix(trimmed, " SKIPPED") || strings.HasSuffix(trimmed, " XFAIL") {
			skipped++
			currentFailIdx = -1
		} else if currentFailIdx >= 0 && trimmed != "" {
			// Accumulate failure output (traceback lines)
			failures[currentFailIdx].Output += line + "\n"
		}

		// Also match summary line: "X passed, Y failed, Z skipped"
		if strings.Contains(trimmed, " passed") && strings.Contains(trimmed, " failed") {
			// This is the summary — we already have per-test data, skip
			continue
		}
	}
	return
}

func opsCategorizeError(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "undefined:") || strings.Contains(lower, "undeclared name") || strings.Contains(lower, "invalid identifier"):
		return "type_error"
	case strings.Contains(lower, "cannot use") || strings.Contains(lower, "not enough arguments") || strings.Contains(lower, "too many arguments"):
		return "type_error"
	case strings.Contains(lower, "cannot convert") || strings.Contains(lower, "incompatible type"):
		return "type_error"
	case strings.Contains(lower, "cannot find package") || strings.Contains(lower, "no required module") || strings.Contains(lower, "missing go.sum entry"):
		return "missing_dep"
	case strings.Contains(lower, "import cycle"):
		return "import_cycle"
	case strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "test timed out"):
		return "timeout"
	case strings.Contains(lower, "fatal error") || strings.Contains(lower, "undefined reference"):
		return "fatal_error"
	case strings.Contains(lower, "expected") && strings.Contains(lower, "got"):
		return "test_assertion"
	case strings.Contains(lower, "panic:"):
		return "test_assertion"
	case strings.Contains(lower, "syntax error") || strings.Contains(lower, "unexpected"):
		return "syntax_error"
	default:
		return "compile_error"
	}
}

// opsCategoryToCode maps an opsCategorizeError category to a registered
// remediation.ErrorCode. Returns empty string when no structured remediation
// is available for the category — callers should leave ErrorCode blank in
// that case rather than attaching an incorrect fix.
//
// Categories deliberately left unmapped (require human judgment):
//   - type_error / import_cycle / fatal_error / test_assertion: no safe auto-fix
//   - syntax_error: goimports doesn't fix syntax; surfacing raw is the right move
func opsCategoryToCode(category string) remediation.ErrorCode {
	switch category {
	case "missing_dep":
		return remediation.CodeGoMissingDep
	case "compile_error":
		return remediation.CodeGoLintViolation
	case "timeout":
		return remediation.CodeGoTimeout
	default:
		return ""
	}
}

// attachRemediation fills in Remediation + ErrorCode on an issue when the
// issue's Category maps to a known remediation. It is a no-op when the
// category has no registered remediation, so callers can chain it blindly.
func attachRemediation(issue *AnalyzedIssue) {
	code := opsCategoryToCode(issue.Category)
	if code == "" {
		return
	}
	rem, ok := remediation.Lookup(code)
	if !ok {
		return
	}
	issue.ErrorCode = string(code)
	issue.Remediation = &rem
}

func opsChangedGoPackages(repoPath, base string) ([]string, []string, error) {
	// Get changed files
	out, err := runGit(repoPath, "diff", "--name-only", base)
	if err != nil {
		// Fallback: unstaged changes
		out, err = runGit(repoPath, "diff", "--name-only")
		if err != nil {
			return nil, nil, err
		}
	}
	var changedFiles []string
	pkgSet := make(map[string]bool)
	for _, f := range strings.Split(strings.TrimSpace(out), "\n") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		changedFiles = append(changedFiles, f)
		if strings.HasSuffix(f, ".go") {
			pkg := "./" + filepath.Dir(f)
			pkgSet[pkg] = true
		}
	}
	var pkgs []string
	for p := range pkgSet {
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		pkgs = []string{"./..."}
	}
	return pkgs, changedFiles, nil
}

// ---------------------------------------------------------------------------
// Input/Output types
// ---------------------------------------------------------------------------

type OpsBuildInput struct {
	Repo string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
}
type OpsBuildOutput struct {
	Repo       string         `json:"repo"`
	Language   string         `json:"language"`
	Success    bool           `json:"success"`
	DurationMs int64          `json:"duration_ms"`
	Errors     []CompileError `json:"errors,omitempty"`
	ErrorCount int            `json:"error_count"`
}

type OpsTestSmartInput struct {
	Repo    string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Base    string `json:"base,omitempty" jsonschema:"description=Git ref to diff against. Default: auto (HEAD or main)."`
	All     bool   `json:"all,omitempty" jsonschema:"description=Run all tests instead of only changed packages."`
	Verbose bool   `json:"verbose,omitempty" jsonschema:"description=Include passing test names in output."`
	Timeout string `json:"timeout,omitempty" jsonschema:"description=Per-package timeout (e.g. 60s 5m). Default: 120s."`
}
type OpsTestSmartOutput struct {
	Repo           string        `json:"repo"`
	Strategy       string        `json:"strategy"`
	BaseRef        string        `json:"base_ref"`
	PackagesTested int           `json:"packages_tested"`
	Passed         int           `json:"passed"`
	Failed         int           `json:"failed"`
	Skipped        int           `json:"skipped"`
	DurationMs     int64         `json:"duration_ms"`
	Failures       []TestFailure `json:"failures,omitempty"`
	PassedTests    []string      `json:"passed_tests,omitempty"`
	ChangedFiles   []string      `json:"changed_files,omitempty"`
	FallbackReason string        `json:"fallback_reason,omitempty"`
}

type OpsChangedFilesInput struct {
	Repo   string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Base   string `json:"base,omitempty" jsonschema:"description=Git ref to diff against. Default: auto."`
	Staged bool   `json:"staged,omitempty" jsonschema:"description=Only show staged changes."`
}
type OpsChangedFilesOutput struct {
	Repo         string        `json:"repo"`
	BaseRef      string        `json:"base_ref"`
	Files        []ChangedFile `json:"files"`
	TotalChanged int           `json:"total_changed"`
	GoPackages   []string      `json:"go_packages,omitempty"`
	Insertions   int           `json:"insertions"`
	Deletions    int           `json:"deletions"`
}

type OpsAnalyzeInput struct {
	BuildErrors  []CompileError `json:"build_errors,omitempty" jsonschema:"description=Compile errors from ops_build"`
	TestFailures []TestFailure  `json:"test_failures,omitempty" jsonschema:"description=Test failures from ops_test_smart"`
}
type OpsAnalyzeOutput struct {
	Summary        string          `json:"summary"`
	TotalIssues    int             `json:"total_issues"`
	Categories     map[string]int  `json:"categories"`
	Issues         []AnalyzedIssue `json:"issues"`
	AffectedFiles  []string        `json:"affected_files"`
	SuggestedOrder []string        `json:"suggested_fix_order"`
}

type OpsBranchInput struct {
	Repo    string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Name    string `json:"name" jsonschema:"required,description=Branch name (auto-prefixed with type/ if no slash)"`
	Type    string `json:"type,omitempty" jsonschema:"description=Branch type prefix,enum=feat,enum=fix,enum=chore,enum=docs,enum=refactor"`
	From    string `json:"from,omitempty" jsonschema:"description=Base ref. Default: main."`
	Execute bool   `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}
type OpsBranchOutput struct {
	Branch     string `json:"branch"`
	BaseBranch string `json:"base_branch"`
	Created    bool   `json:"created"`
	DryRun     bool   `json:"dry_run"`
}

type OpsCommitInput struct {
	Repo    string   `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Message string   `json:"message" jsonschema:"required,description=Commit message (conventional: feat: fix: chore: etc.)"`
	Files   []string `json:"files,omitempty" jsonschema:"description=Specific files to stage. Default: all modified tracked files."`
	Execute bool     `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}
type OpsCommitOutput struct {
	SHA         string   `json:"sha,omitempty"`
	Message     string   `json:"message"`
	FilesStaged []string `json:"files_staged"`
	Insertions  int      `json:"insertions"`
	Deletions   int      `json:"deletions"`
	DryRun      bool     `json:"dry_run"`
	Warnings    []string `json:"warnings,omitempty"`
}

type OpsPRCreateInput struct {
	Repo    string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Title   string `json:"title" jsonschema:"required,description=PR title (under 70 chars)"`
	Body    string `json:"body,omitempty" jsonschema:"description=PR body markdown"`
	Base    string `json:"base,omitempty" jsonschema:"description=Base branch. Default: main."`
	Draft   bool   `json:"draft,omitempty" jsonschema:"description=Create as draft PR"`
	Execute bool   `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}
type OpsPRCreateOutput struct {
	URL     string `json:"url,omitempty"`
	Number  int    `json:"number,omitempty"`
	Title   string `json:"title"`
	Base    string `json:"base"`
	Head    string `json:"head"`
	Commits int    `json:"commits"`
	DryRun  bool   `json:"dry_run"`
	Pushed  bool   `json:"pushed"`
}

type OpsCIStatusInput struct {
	Repo   string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Branch string `json:"branch,omitempty" jsonschema:"description=Branch to check. Default: current branch."`
	PR     int    `json:"pr,omitempty" jsonschema:"description=PR number to check."`
	Wait   bool   `json:"wait,omitempty" jsonschema:"description=Wait up to 5 minutes for checks to complete."`
}
type OpsCIStatusOutput struct {
	Branch    string    `json:"branch"`
	SHA       string    `json:"sha"`
	Overall   string    `json:"overall"`
	Checks    []CICheck `json:"checks"`
	WaitedSec int       `json:"waited_sec,omitempty"`
}

type OpsPrePushInput struct {
	Repo     string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	SkipLint bool   `json:"skip_lint,omitempty" jsonschema:"description=Skip golangci-lint."`
}
type OpsPrePushOutput struct {
	Repo       string         `json:"repo"`
	Overall    string         `json:"overall"`
	DurationMs int64          `json:"duration_ms"`
	Steps      []PrePushStep  `json:"steps"`
	FailedStep string         `json:"failed_step,omitempty"`
	Errors     []CompileError `json:"errors,omitempty"`
	Failures   []TestFailure  `json:"failures,omitempty"`
}

type OpsIterateInput struct {
	Repo      string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Ops session ID. Auto-created if empty."`
}
type NextAction struct {
	File       string `json:"file"`
	Line       int    `json:"line,omitempty"`
	Category   string `json:"category"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type OpsIterateOutput struct {
	SessionID   string              `json:"session_id"`
	Iteration   int                 `json:"iteration"`
	Build       *OpsBuildOutput     `json:"build"`
	Test        *OpsTestSmartOutput `json:"test,omitempty"`
	Analysis    *OpsAnalyzeOutput   `json:"analysis,omitempty"`
	Status      string              `json:"status"`
	DurationMs  int64               `json:"duration_ms"`
	History     []IterationSummary  `json:"history"`
	NextActions []NextAction        `json:"next_actions"`
}

type OpsShipInput struct {
	Repo    string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Message string `json:"message" jsonschema:"required,description=Conventional commit message"`
	Title   string `json:"title,omitempty" jsonschema:"description=PR title. Defaults to commit message."`
	Body    string `json:"body,omitempty" jsonschema:"description=PR body markdown."`
	Draft   bool   `json:"draft,omitempty" jsonschema:"description=Create PR as draft."`
	Execute bool   `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}
type OpsShipOutput struct {
	DryRun    bool               `json:"dry_run"`
	PrePush   *OpsPrePushOutput  `json:"pre_push"`
	Commit    *OpsCommitOutput   `json:"commit,omitempty"`
	PR        *OpsPRCreateOutput `json:"pr,omitempty"`
	Overall   string             `json:"overall"`
	BlockedAt string             `json:"blocked_at,omitempty"`
	Error     string             `json:"error,omitempty"`
}

type OpsSessionCreateInput struct {
	Repo        string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Description string `json:"description,omitempty" jsonschema:"description=What this SDLC cycle is working on"`
}
type OpsSessionCreateOutput struct {
	SessionID string `json:"session_id"`
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	CreatedAt string `json:"created_at"`
}

type OpsSessionStatusInput struct {
	SessionID string `json:"session_id" jsonschema:"required,description=Ops session ID"`
}
type OpsSessionStatusOutput struct {
	SessionID    string             `json:"session_id"`
	Repo         string             `json:"repo"`
	Branch       string             `json:"branch"`
	Iterations   int                `json:"iterations"`
	TotalTimeMs  int64              `json:"total_time_ms"`
	CurrentState string             `json:"current_state"`
	ErrorTrend   []int              `json:"error_trend"`
	Converging   bool               `json:"converging"`
	History      []IterationSummary `json:"history"`
}

type OpsRevertInput struct {
	Repo    string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Execute bool   `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}
type OpsRevertOutput struct {
	Method    string `json:"method"` // soft_reset, revert_commit
	SHA       string `json:"sha"`
	Message   string `json:"message"`
	WasPushed bool   `json:"was_pushed"`
	DryRun    bool   `json:"dry_run"`
}

type OpsFleetIterateInput struct {
	Dir      string `json:"dir,omitempty" jsonschema:"description=Base directory to scan for repos. Default: ~/hairglasses-studio"`
	Language string `json:"language,omitempty" jsonschema:"description=Filter by language: go/node/python/all. Default: all,enum=go,enum=node,enum=python,enum=all"`
	MaxRepos int    `json:"max_repos,omitempty" jsonschema:"description=Maximum repos to test (safety limit). Default: 20"`
}

type FleetRepoResult struct {
	Repo        string `json:"repo"`
	Language    string `json:"language"`
	BuildOK     bool   `json:"build_ok"`
	TestsPassed int    `json:"tests_passed"`
	TestsFailed int    `json:"tests_failed"`
	ErrorCount  int    `json:"error_count"`
	Status      string `json:"status"` // pass, build_fail, test_fail, skip
	DurationMs  int64  `json:"duration_ms"`
}

type OpsFleetIterateOutput struct {
	Total   int               `json:"total"`
	Passing int               `json:"passing"`
	Failing int               `json:"failing"`
	Skipped int               `json:"skipped"`
	Results []FleetRepoResult `json:"results"`
}

type OpsReleaseInput struct {
	Repo          string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
	Version       string `json:"version" jsonschema:"required,description=New version (e.g. 1.2.3 or v1.2.3)"`
	AutoChangelog bool   `json:"auto_changelog,omitempty" jsonschema:"description=Prepend entry to CHANGELOG.md"`
	Push          bool   `json:"push,omitempty" jsonschema:"description=Push commit and tag to origin"`
	Execute       bool   `json:"execute,omitempty" jsonschema:"description=Set true to execute (default: dry-run)"`
}

type OpsReleaseOutput struct {
	CurrentVersion string   `json:"current_version"`
	NewVersion     string   `json:"new_version"`
	FilesModified  []string `json:"files_modified"`
	Tag            string   `json:"tag"`
	ChangelogEntry string   `json:"changelog_entry,omitempty"`
	Committed      bool     `json:"committed"`
	Pushed         bool     `json:"pushed"`
	DryRun         bool     `json:"dry_run"`
}

type OpsRepoAnalyzeInput struct {
	Repo string `json:"repo,omitempty" jsonschema:"description=Absolute repo path. Defaults to cwd."`
}
type OpsRepoAnalyzeOutput struct {
	Repo           string   `json:"repo"`
	Name           string   `json:"name"`
	Language       string   `json:"language"`
	Languages      []string `json:"languages"`
	IsMCP          bool     `json:"is_mcp"`
	Protocols      []string `json:"protocols"`
	Frameworks     []string `json:"frameworks"`
	KeyDeps        []string `json:"key_dependencies"`
	GoModules      int      `json:"go_modules,omitempty"`
	TestCount      int      `json:"test_count"`
	HasCI          bool     `json:"has_ci"`
	HasCLAUDEMD    bool     `json:"has_claude_md"`
	HasReadme      bool     `json:"has_readme"`
	HasLicense     bool     `json:"has_license"`
	Tags           []string `json:"tags"`
	AnalysisTimeMs int64    `json:"analysis_time_ms"`
}

type OpsDepGraphInput struct {
	Dir    string `json:"dir,omitempty" jsonschema:"description=Base directory with go.work or go.mod. Default: ~/hairglasses-studio"`
	Filter string `json:"filter,omitempty" jsonschema:"description=Filter: internal (org repos only) or all (include external deps),enum=internal,enum=all"`
	Format string `json:"format,omitempty" jsonschema:"description=Output format: mermaid or dot,enum=mermaid,enum=dot"`
}
type OpsDepGraphOutput struct {
	Graph       string   `json:"graph"`
	Format      string   `json:"format"`
	ModuleCount int      `json:"module_count"`
	EdgeCount   int      `json:"edge_count"`
	OrgModules  []string `json:"org_modules,omitempty"`
}

type OpsSessionListInput struct{}
type OpsSessionListOutput struct {
	Sessions []SessionSummary `json:"sessions"`
}
type SessionSummary struct {
	SessionID   string `json:"session_id"`
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`
	Iterations  int    `json:"iterations"`
	State       string `json:"state"`
	StartedAt   string `json:"started_at"`
	Description string `json:"description,omitempty"`
}

// --- Coverage Report ---

type OpsCoverageReportInput struct {
	Repo      string  `json:"repo,omitempty" jsonschema:"description=Repository path (default: cwd)"`
	Threshold float64 `json:"threshold,omitempty" jsonschema:"description=Minimum coverage percentage to pass gate (0=no gate)"`
}

type OpsCoveragePackage struct {
	Name        string  `json:"name"`
	CoveragePct float64 `json:"coverage_pct"`
}

type OpsCoverageReportOutput struct {
	Language   string               `json:"language"`
	OverallPct float64              `json:"overall_pct"`
	GatePassed bool                 `json:"gate_passed"`
	Threshold  float64              `json:"threshold"`
	Packages   []OpsCoveragePackage `json:"packages"`
}

// --- Lint Fix ---

type OpsLintFixInput struct {
	Repo string `json:"repo,omitempty" jsonschema:"description=Repository path (default: cwd)"`
}

type OpsLintFixOutput struct {
	Language        string `json:"language"`
	ToolUsed        string `json:"tool_used"`
	FilesFixed      int    `json:"files_fixed"`
	ErrorsRemaining int    `json:"errors_remaining"`
	Output          string `json:"output"`
}

// --- Changelog Generate ---

type OpsChangelogGenerateInput struct {
	Repo  string `json:"repo,omitempty" jsonschema:"description=Repository path (default: cwd)"`
	Write bool   `json:"write,omitempty" jsonschema:"description=If true append to CHANGELOG.md (default: preview only)"`
}

type OpsChangelogGenerateOutput struct {
	Markdown    string         `json:"markdown"`
	CommitCount int            `json:"commit_count"`
	Groups      map[string]int `json:"groups"`
	Written     bool           `json:"written"`
	LastTag     string         `json:"last_tag"`
}

// ---------------------------------------------------------------------------
// Wave 7 — Auto-fix, Fleet Intelligence, Knowledge, Iteration Patterns
// ---------------------------------------------------------------------------

// --- ops_auto_fix ---

// OpsAutoFixInput is the request payload for ops_auto_fix.
type OpsAutoFixInput struct {
	Repo    string          `json:"repo,omitempty" jsonschema:"description=Repository path (default: cwd)"`
	Issues  []AnalyzedIssue `json:"issues" jsonschema:"description=Issues from ops_analyze_failures"`
	Execute bool            `json:"execute,omitempty" jsonschema:"description=Apply patches (default: dry-run preview)"`
}

type Patch struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Action  string `json:"action"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
	Applied bool   `json:"applied"`
}

type OpsAutoFixOutput struct {
	Repo            string          `json:"repo"`
	PatchCount      int             `json:"patch_count"`
	AppliedCount    int             `json:"applied_count"`
	Patches         []Patch         `json:"patches"`
	RemainingIssues []AnalyzedIssue `json:"remaining_issues"`
	DryRun          bool            `json:"dry_run"`
}

// --- ops_fleet_diff ---

// OpsFleetDiffInput is the request payload for ops_fleet_diff.
type OpsFleetDiffInput struct {
	Dir      string `json:"dir,omitempty" jsonschema:"description=Root directory (default: ~/hairglasses-studio)"`
	Since    string `json:"since" jsonschema:"description=Date (2024-04-01) or relative (3d/1w) or git ref"`
	Language string `json:"language,omitempty" jsonschema:"description=Filter by language (go/node/python/all)"`
	MaxRepos int    `json:"max_repos,omitempty" jsonschema:"description=Max repos to scan (default: 30)"`
}

type FleetRepoDiff struct {
	Repo        string         `json:"repo"`
	Commits     int            `json:"commits"`
	Insertions  int            `json:"insertions"`
	Deletions   int            `json:"deletions"`
	CommitTypes map[string]int `json:"commit_types"`
	Authors     []string       `json:"authors,omitempty"`
}

type OpsFleetDiffOutput struct {
	Since           string          `json:"since"`
	TotalRepos      int             `json:"total_repos"`
	ActiveRepos     int             `json:"active_repos"`
	TotalCommits    int             `json:"total_commits"`
	TotalInsertions int             `json:"total_insertions"`
	TotalDeletions  int             `json:"total_deletions"`
	MostActive      []string        `json:"most_active"`
	Repos           []FleetRepoDiff `json:"repos"`
}

// --- ops_tech_debt ---

// OpsTechDebtInput is the request payload for ops_tech_debt.
type OpsTechDebtInput struct {
	Repo  string `json:"repo,omitempty" jsonschema:"description=Single repo path (omit for fleet mode)"`
	Dir   string `json:"dir,omitempty" jsonschema:"description=Fleet root directory (default: ~/hairglasses-studio)"`
	Store bool   `json:"store,omitempty" jsonschema:"description=Persist scores for trend tracking"`
}

type TechDebtScore struct {
	Repo        string         `json:"repo"`
	Overall     int            `json:"overall"`
	Dimensions  map[string]int `json:"dimensions"`
	ActionItems []string       `json:"action_items"`
	Trend       string         `json:"trend,omitempty"`
}

type OpsTechDebtOutput struct {
	Scores     []TechDebtScore `json:"scores"`
	FleetAvg   int             `json:"fleet_avg,omitempty"`
	WorstRepos []string        `json:"worst_repos,omitempty"`
}

// --- ops_research_check ---

// OpsResearchCheckInput is the request payload for ops_research_check.
type OpsResearchCheckInput struct {
	Query      string `json:"query" jsonschema:"description=Topic or keywords to search for"`
	DocsPath   string `json:"docs_path,omitempty" jsonschema:"description=Docs repo path (default: ~/hairglasses-studio/docs)"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"description=Max results (default: 10)"`
}

type ResearchMatch struct {
	Path      string   `json:"path"`
	Title     string   `json:"title"`
	Relevance float64  `json:"relevance"`
	Excerpt   string   `json:"excerpt"`
	Tags      []string `json:"tags,omitempty"`
}

type OpsResearchCheckOutput struct {
	Query      string          `json:"query"`
	Results    []ResearchMatch `json:"results"`
	TotalDocs  int             `json:"total_docs"`
	Gaps       []string        `json:"gaps"`
	Suggestion string          `json:"suggestion,omitempty"`
}

// --- ops_session_handoff ---

// OpsSessionHandoffInput is the request payload for ops_session_handoff.
type OpsSessionHandoffInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Ops session ID (default: most recent)"`
	Repo      string `json:"repo,omitempty" jsonschema:"description=Repository path (default: cwd)"`
	Write     bool   `json:"write,omitempty" jsonschema:"description=Write handoff doc to repo"`
}

type OpsSessionHandoffOutput struct {
	Handoff     string `json:"handoff"`
	SessionID   string `json:"session_id,omitempty"`
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`
	WrittenPath string `json:"written_path,omitempty"`
}

// --- ops_iteration_patterns ---

// OpsIterationPatternsInput is the request payload for ops_iteration_patterns.
type OpsIterationPatternsInput struct {
	Window string `json:"window,omitempty" jsonschema:"description=Time window (default: 30d)"`
	Repo   string `json:"repo,omitempty" jsonschema:"description=Filter to specific repo"`
}

type HotFile struct {
	File        string `json:"file"`
	Appearances int    `json:"appearances"`
}

type OpsIterationPatternsOutput struct {
	TotalSessions   int            `json:"total_sessions"`
	TotalIterations int            `json:"total_iterations"`
	AvgIterations   float64        `json:"avg_iterations"`
	ConvergenceRate float64        `json:"convergence_rate"`
	CommonErrors    map[string]int `json:"common_errors"`
	HotFiles        []HotFile      `json:"hot_files"`
	Recommendations []string       `json:"recommendations"`
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

type OpsModule struct{}

func (m *OpsModule) Name() string { return "ops" }
func (m *OpsModule) Description() string {
	return "SDLC operations for autonomous development loops: build, test, analyze, commit, ship"
}

func (m *OpsModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// Atomic — Build & Test
		handler.TypedHandler[OpsBuildInput, OpsBuildOutput](
			"ops_build",
			"Run go build and parse compiler errors into structured JSON with file, line, column, message, and package. Returns success/failure with error count.",
			opsBuild,
		),
		handler.TypedHandler[OpsTestSmartInput, OpsTestSmartOutput](
			"ops_test_smart",
			"Run go test -json on only changed packages (via git diff), returning per-test pass/fail/skip results. Use all=true to test everything. Smart filtering avoids running the full suite on every iteration.",
			opsTestSmart,
		),
		handler.TypedHandler[OpsChangedFilesInput, OpsChangedFilesOutput](
			"ops_changed_files",
			"List changed files with diff stats and Go package mapping. Useful for understanding what was modified before building or testing.",
			opsChangedFiles,
		),
		// Atomic — Analysis
		handler.TypedHandler[OpsAnalyzeInput, OpsAnalyzeOutput](
			"ops_analyze_failures",
			"Categorize build/test failures into types (compile_error, type_error, missing_dep, test_assertion, timeout, import_cycle) with suggested fix order. Pass build_errors from ops_build and/or test_failures from ops_test_smart.",
			opsAnalyze,
		),
		// Atomic — Git & GitHub
		handler.TypedHandler[OpsBranchInput, OpsBranchOutput](
			"ops_branch_create",
			"Create a feature branch with conventional naming (feat/fix/chore prefix). Dry-run by default — pass execute=true to create.",
			opsBranchCreate,
		),
		handler.TypedHandler[OpsCommitInput, OpsCommitOutput](
			"ops_commit",
			"Stage changed files and create a conventional commit. Validates message format, never amends. Dry-run by default — pass execute=true to commit.",
			opsCommit,
		),
		handler.TypedHandler[OpsPRCreateInput, OpsPRCreateOutput](
			"ops_pr_create",
			"Push branch and create a GitHub PR via gh CLI. Returns PR URL and number. Dry-run by default — pass execute=true to create.",
			opsPRCreate,
		),
		// Atomic — CI
		handler.TypedHandler[OpsCIStatusInput, OpsCIStatusOutput](
			"ops_ci_status",
			"Check GitHub Actions check status for a branch or PR. Returns per-check pass/fail with overall verdict. Use wait=true to poll until all checks complete (up to 5 minutes).",
			opsCIStatus,
		),
		// Composed
		handler.TypedHandler[OpsPrePushInput, OpsPrePushOutput](
			"ops_pre_push",
			"Pre-push gate: run vet → lint → build → test (changed packages only). Short-circuits on first failure. Returns structured per-step results.",
			opsPrePush,
		),
		handler.TypedHandler[OpsIterateInput, OpsIterateOutput](
			"ops_iterate",
			"Core SDLC loop tool: build → test (changed packages) → analyze failures → track iteration. Call this after each round of edits. Returns structured next_actions with files to fix. Tracks iteration count and error trends for convergence detection.",
			opsIterate,
		),
		handler.TypedHandler[OpsShipInput, OpsShipOutput](
			"ops_ship",
			"Ship changes: pre-push gate → commit → push → create PR. Only proceeds if gate passes. Dry-run by default — pass execute=true to ship.",
			opsShip,
		),
		// Sessions
		handler.TypedHandler[OpsSessionCreateInput, OpsSessionCreateOutput](
			"ops_session_create",
			"Create a new SDLC session for iteration tracking. Optional — ops_iterate auto-creates one if absent.",
			opsSessionCreate,
		),
		handler.TypedHandler[OpsSessionStatusInput, OpsSessionStatusOutput](
			"ops_session_status",
			"Get SDLC session status: iteration count, error trend, convergence detection, total time spent.",
			opsSessionStatus,
		),
		handler.TypedHandler[OpsSessionListInput, OpsSessionListOutput](
			"ops_session_list",
			"List all active SDLC sessions with summary stats. Auto-cleans sessions older than 7 days.",
			opsSessionList,
		),
		handler.TypedHandler[OpsRevertInput, OpsRevertOutput](
			"ops_revert",
			"Safely undo the last commit. If unpushed: soft reset (keeps changes staged). If pushed: creates a revert commit. Never force-pushes. Dry-run by default — pass execute=true.",
			opsRevert,
		),
		// Fleet + Release
		handler.TypedHandler[OpsFleetIterateInput, OpsFleetIterateOutput](
			"ops_fleet_iterate",
			"Run build+test across all repos in a directory. Returns per-repo health matrix with build/test status, error counts, and overall fleet summary. Scans for Go/Node/Python projects.",
			opsFleetIterate,
		),
		handler.TypedHandler[OpsReleaseInput, OpsReleaseOutput](
			"ops_release",
			"Bump version, generate changelog entry, commit, and tag. Detects language (Go/Node/Python) and updates the appropriate version file. Dry-run by default — pass execute=true.",
			opsRelease,
		),
		// Coverage, Lint, Changelog
		handler.TypedHandler[OpsCoverageReportInput, OpsCoverageReportOutput](
			"ops_coverage_report",
			"Run test coverage and return per-package percentages. Set threshold>0 to gate on minimum overall coverage. Go: go test -coverprofile, Node: nyc/c8, Python: coverage.py.",
			opsCoverageReport,
		),
		handler.TypedHandler[OpsLintFixInput, OpsLintFixOutput](
			"ops_lint_fix",
			"Auto-fix lint issues in-place. Go: goimports + golangci-lint --fix, Node: eslint --fix + prettier, Python: black + isort. Reports files changed and remaining errors.",
			opsLintFix,
		),
		handler.TypedHandler[OpsChangelogGenerateInput, OpsChangelogGenerateOutput](
			"ops_changelog_generate",
			"Generate markdown changelog from conventional commits since last tag. Groups by feat/fix/chore/etc. Pass write=true to prepend to CHANGELOG.md.",
			opsChangelogGenerate,
		),
		// Codebase Intelligence
		handler.TypedHandler[OpsRepoAnalyzeInput, OpsRepoAnalyzeOutput](
			"ops_repo_analyze",
			"Analyze a repo and tag it with languages, protocols (gRPC, REST, MCP, WebSocket), frameworks, key dependencies, and project metadata (CI, tests, MCP server detection). Returns structured repo profile.",
			opsRepoAnalyze,
		),
		handler.TypedHandler[OpsDepGraphInput, OpsDepGraphOutput](
			"ops_dep_graph",
			"Generate a dependency graph for a Go workspace or module. Uses go mod graph for accurate module-level dependencies. Outputs Mermaid markdown (for GitHub rendering) or DOT format. Filter to internal org modules or include all transitive deps.",
			opsDepGraph,
		),
		// Wave 7 — Intelligence & Autonomy
		handler.TypedHandler[OpsAutoFixInput, OpsAutoFixOutput](
			"ops_auto_fix",
			"Auto-fix mechanical failures from ops_analyze_failures: missing deps (go mod tidy), missing imports (goimports), unused vars (remove). Pass issues from analyze output. Dry-run by default — pass execute=true to apply patches.",
			opsAutoFix,
		),
		handler.TypedHandler[OpsFleetDiffInput, OpsFleetDiffOutput](
			"ops_fleet_diff",
			"Show what changed across all repos since a date or ref. Returns per-repo commit counts, insertions/deletions, commit type breakdown (feat/fix/chore), and fleet-wide totals. Use since='3d' for relative or '2024-04-01' for absolute.",
			opsFleetDiff,
		),
		handler.TypedHandler[OpsTechDebtInput, OpsTechDebtOutput](
			"ops_tech_debt",
			"Score tech debt 0-100 across 6 dimensions: dependency freshness, test coverage, lint cleanliness, CI health, documentation, code age. Single repo or fleet mode. Set store=true for trend tracking.",
			opsTechDebt,
		),
		handler.TypedHandler[OpsResearchCheckInput, OpsResearchCheckOutput](
			"ops_research_check",
			"Search the docs knowledge base for existing research on a topic. Returns matching documents with relevance scores and identifies gaps where no research exists. Check this before starting new research to avoid duplication.",
			opsResearchCheck,
		),
		handler.TypedHandler[OpsSessionHandoffInput, OpsSessionHandoffOutput](
			"ops_session_handoff",
			"Generate an Agent Handoff Protocol document from the current ops session and git state. Captures branch, dirty files, iteration history, and pending work. Set write=true to persist to repo.",
			opsSessionHandoff,
		),
		handler.TypedHandler[OpsIterationPatternsInput, OpsIterationPatternsOutput](
			"ops_iteration_patterns",
			"Analyze historical SDLC sessions for patterns: common failure types, average iterations to convergence, hot files that appear in most failures. Useful for identifying systemic issues and improving auto-fix heuristics.",
			opsIterationPatterns,
		),
	}
}

// ---------------------------------------------------------------------------
// Atomic handlers — Build & Test
// ---------------------------------------------------------------------------

func opsBuild(_ context.Context, input OpsBuildInput) (OpsBuildOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsBuildOutput{}, err
	}
	lang := opsDetectLanguage(repo)

	start := time.Now()

	switch lang {
	case "go":
		// Direct Go build with structured error parsing
		_, stderr, exitCode, err := opsRunWithTimeout(opsBuildTimeout, repo, "go", "build", "./...")
		if err != nil {
			return OpsBuildOutput{}, fmt.Errorf("build command failed: %w", err)
		}
		duration := time.Since(start).Milliseconds()
		errors := opsParseCompileErrors(stderr)
		return OpsBuildOutput{
			Repo: repo, Language: lang, Success: exitCode == 0,
			DurationMs: duration, Errors: errors, ErrorCount: len(errors),
		}, nil

	case "node":
		// Node.js: npm run build (or npm install if no build script)
		stdout, stderr, exitCode, _ := opsRunWithTimeout(opsBuildTimeout, repo, "npm", "run", "build")
		duration := time.Since(start).Milliseconds()
		var errors []CompileError
		if exitCode != 0 {
			// Parse TypeScript/build errors from combined output
			errors = opsParseCompileErrors(stderr + "\n" + stdout)
		}
		return OpsBuildOutput{
			Repo: repo, Language: lang, Success: exitCode == 0,
			DurationMs: duration, Errors: errors, ErrorCount: len(errors),
		}, nil

	case "python":
		// Python: check syntax via py_compile, or run build if setup exists
		stdout, stderr, exitCode, _ := opsRunWithTimeout(opsBuildTimeout, repo,
			"python3", "-m", "py_compile", "*.py")
		if exitCode != 0 {
			// Fallback: try pip install in dry-run
			stdout, stderr, exitCode, _ = opsRunWithTimeout(opsBuildTimeout, repo,
				"pip", "install", "--dry-run", "-e", ".")
		}
		duration := time.Since(start).Milliseconds()
		var errors []CompileError
		if exitCode != 0 {
			errors = opsParseCompileErrors(stderr + "\n" + stdout)
		}
		return OpsBuildOutput{
			Repo: repo, Language: lang, Success: exitCode == 0,
			DurationMs: duration, Errors: errors, ErrorCount: len(errors),
		}, nil

	default:
		return OpsBuildOutput{}, fmt.Errorf("[%s] unsupported language: %s (need go.mod, package.json, or pyproject.toml)", handler.ErrInvalidParam, lang)
	}
}

func opsTestSmart(_ context.Context, input OpsTestSmartInput) (OpsTestSmartOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsTestSmartOutput{}, err
	}

	base := input.Base
	if base == "" {
		base = opsDefaultBase(repo)
	}

	timeout := opsTestTimeout
	if input.Timeout != "" {
		if d, err := time.ParseDuration(input.Timeout); err == nil {
			timeout = d
		}
	}

	lang := opsDetectLanguage(repo)
	strategy := "changed_packages"
	fallbackReason := ""
	var changedFiles []string
	packagesTested := 0

	start := time.Now()
	var passed []string
	var failures []TestFailure
	var skipped int

	switch lang {
	case "go":
		var pkgs []string
		if input.All {
			strategy = "all"
			pkgs = []string{"./..."}
		} else {
			pkgs, changedFiles, err = opsChangedGoPackages(repo, base)
			if err != nil {
				strategy = "all"
				fallbackReason = "Could not determine changed packages; running full suite"
				pkgs = []string{"./..."}
			}
		}
		packagesTested = len(pkgs)
		args := append([]string{"test", "-json", "-count=1", "-timeout", timeout.String()}, pkgs...)
		stdout, _, _, _ := opsRunWithTimeout(timeout+10*time.Second, repo, "go", args...)
		passed, failures, skipped = opsParseGoTestJSON(stdout)

	case "node":
		strategy = "all"
		fallbackReason = "Smart filtering not supported for Node.js; running full suite"
		packagesTested = 1
		stdout, _, _, _ := opsRunWithTimeout(timeout+10*time.Second, repo, "npx", "jest", "--json", "--forceExit")
		passed, failures, skipped = opsParseNodeTestJSON(stdout)

	case "python":
		strategy = "all"
		fallbackReason = "Smart filtering not supported for Python; running full suite"
		packagesTested = 1
		stdout, _, _, _ := opsRunWithTimeout(timeout+10*time.Second, repo, "pytest", "-v", "--tb=short", "-q")
		passed, failures, skipped = opsParsePytestOutput(stdout)

	default:
		return OpsTestSmartOutput{}, fmt.Errorf("[%s] unsupported language: %s", handler.ErrInvalidParam, lang)
	}

	duration := time.Since(start).Milliseconds()

	out := OpsTestSmartOutput{
		Repo:           repo,
		Strategy:       strategy,
		BaseRef:        base,
		PackagesTested: packagesTested,
		Passed:         len(passed),
		Failed:         len(failures),
		Skipped:        skipped,
		DurationMs:     duration,
		Failures:       failures,
		ChangedFiles:   changedFiles,
	}
	if input.Verbose {
		out.PassedTests = passed
	}
	if fallbackReason != "" {
		out.FallbackReason = fallbackReason
	}
	return out, nil
}

func opsChangedFiles(_ context.Context, input OpsChangedFilesInput) (OpsChangedFilesOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsChangedFilesOutput{}, err
	}

	base := input.Base
	if base == "" {
		base = opsDefaultBase(repo)
	}

	diffArgs := []string{"diff", "--numstat", base}
	if input.Staged {
		diffArgs = []string{"diff", "--numstat", "--cached"}
	}

	out, err := runGit(repo, diffArgs...)
	if err != nil {
		return OpsChangedFilesOutput{}, fmt.Errorf("git diff: %w", err)
	}

	var files []ChangedFile
	pkgSet := make(map[string]bool)
	totalIns, totalDel := 0, 0

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		ins, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		path := parts[2]
		totalIns += ins
		totalDel += del

		cf := ChangedFile{
			Path:       path,
			Status:     "modified",
			Insertions: ins,
			Deletions:  del,
		}
		if strings.HasSuffix(path, ".go") {
			pkg := "./" + filepath.Dir(path)
			cf.GoPackage = pkg
			pkgSet[pkg] = true
		}
		files = append(files, cf)
	}

	var pkgs []string
	for p := range pkgSet {
		pkgs = append(pkgs, p)
	}

	return OpsChangedFilesOutput{
		Repo:         repo,
		BaseRef:      base,
		Files:        files,
		TotalChanged: len(files),
		GoPackages:   pkgs,
		Insertions:   totalIns,
		Deletions:    totalDel,
	}, nil
}

// ---------------------------------------------------------------------------
// Atomic handlers — Analysis
// ---------------------------------------------------------------------------

func opsAnalyze(_ context.Context, input OpsAnalyzeInput) (OpsAnalyzeOutput, error) {
	categories := make(map[string]int)
	var issues []AnalyzedIssue
	fileSet := make(map[string]bool)

	for _, ce := range input.BuildErrors {
		cat := opsCategorizeError(ce.Message)
		categories[cat]++
		fileSet[ce.File] = true
		issue := AnalyzedIssue{
			Category:   cat,
			File:       ce.File,
			Line:       ce.Line,
			Message:    ce.Message,
			Suggestion: suggestFix(cat, ce.Message),
			Severity:   "blocker",
		}
		attachRemediation(&issue)
		issues = append(issues, issue)
	}

	for _, tf := range input.TestFailures {
		cat := opsCategorizeError(tf.Output)
		categories[cat]++
		// Extract file from package path
		pkgDir := strings.TrimPrefix(tf.Package, "github.com/")
		if idx := strings.Index(pkgDir, "/"); idx > 0 {
			pkgDir = pkgDir[strings.Index(pkgDir[idx+1:], "/")+idx+2:]
		}
		fileSet[pkgDir] = true
		issue := AnalyzedIssue{
			Category:   cat,
			File:       pkgDir,
			Message:    fmt.Sprintf("Test %s failed", tf.Test),
			Suggestion: suggestFix(cat, tf.Output),
			Severity:   "blocker",
		}
		attachRemediation(&issue)
		issues = append(issues, issue)
	}

	var affected []string
	for f := range fileSet {
		affected = append(affected, f)
	}

	summary := fmt.Sprintf("%d issue(s): ", len(issues))
	for cat, count := range categories {
		summary += fmt.Sprintf("%s(%d) ", cat, count)
	}

	return OpsAnalyzeOutput{
		Summary:        strings.TrimSpace(summary),
		TotalIssues:    len(issues),
		Categories:     categories,
		Issues:         issues,
		AffectedFiles:  affected,
		SuggestedOrder: affected, // simplified: just the affected files
	}, nil
}

func suggestFix(category, msg string) string {
	switch category {
	case "type_error":
		return "Check type signatures and function arguments"
	case "missing_dep":
		return "Run 'go mod tidy' or add the missing import"
	case "import_cycle":
		return "Break the import cycle by extracting shared types to a separate package"
	case "timeout":
		return "Check for infinite loops, slow external calls, or increase test timeout"
	case "test_assertion":
		return "Review the test assertion and expected vs actual values"
	default:
		return "Fix the compile error"
	}
}

// ---------------------------------------------------------------------------
// Atomic handlers — Git & GitHub
// ---------------------------------------------------------------------------

func opsBranchCreate(_ context.Context, input OpsBranchInput) (OpsBranchOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsBranchOutput{}, err
	}

	branchType := input.Type
	if branchType == "" {
		branchType = "feat"
	}
	branch := input.Name
	if !strings.Contains(branch, "/") {
		branch = branchType + "/" + branch
	}

	base := input.From
	if base == "" {
		base = opsDefaultBase(repo)
		if base == "HEAD" {
			base = "main"
		}
	}

	if !input.Execute {
		return OpsBranchOutput{
			Branch:     branch,
			BaseBranch: base,
			Created:    false,
			DryRun:     true,
		}, nil
	}

	if _, err := runGit(repo, "checkout", "-b", branch, base); err != nil {
		return OpsBranchOutput{}, fmt.Errorf("git checkout -b: %w", err)
	}

	return OpsBranchOutput{
		Branch:     branch,
		BaseBranch: base,
		Created:    true,
		DryRun:     false,
	}, nil
}

func opsCommit(_ context.Context, input OpsCommitInput) (OpsCommitOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsCommitOutput{}, err
	}

	var warnings []string
	if !conventionalRe.MatchString(input.Message) {
		warnings = append(warnings, "Message doesn't follow conventional commit format (feat:, fix:, chore:, etc.)")
	}

	// Stage files
	if len(input.Files) > 0 {
		args := append([]string{"add"}, input.Files...)
		if _, err := runGit(repo, args...); err != nil {
			return OpsCommitOutput{}, fmt.Errorf("git add: %w", err)
		}
	} else {
		if _, err := runGit(repo, "add", "-u"); err != nil {
			return OpsCommitOutput{}, fmt.Errorf("git add -u: %w", err)
		}
	}

	// Get staged files
	staged, _ := runGit(repo, "diff", "--cached", "--name-only")
	stagedFiles := strings.Split(strings.TrimSpace(staged), "\n")
	if len(stagedFiles) == 1 && stagedFiles[0] == "" {
		stagedFiles = nil
	}

	// Early detection: nothing to commit
	if len(stagedFiles) == 0 {
		return OpsCommitOutput{
			Message:  input.Message,
			DryRun:   true,
			Warnings: append(warnings, "Nothing to commit — no staged changes found"),
		}, nil
	}

	// Get stats
	stat, _ := runGit(repo, "diff", "--cached", "--stat")
	ins, del := 0, 0
	for _, line := range strings.Split(stat, "\n") {
		if strings.Contains(line, "insertion") || strings.Contains(line, "deletion") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if strings.HasPrefix(p, "insertion") && i > 0 {
					ins, _ = strconv.Atoi(parts[i-1])
				}
				if strings.HasPrefix(p, "deletion") && i > 0 {
					del, _ = strconv.Atoi(parts[i-1])
				}
			}
		}
	}

	if !input.Execute {
		// Unstage in dry-run mode
		runGit(repo, "reset", "HEAD")
		return OpsCommitOutput{
			Message:     input.Message,
			FilesStaged: stagedFiles,
			Insertions:  ins,
			Deletions:   del,
			DryRun:      true,
			Warnings:    warnings,
		}, nil
	}

	// Commit
	if _, err := runGit(repo, "commit", "-m", input.Message); err != nil {
		return OpsCommitOutput{}, fmt.Errorf("git commit: %w", err)
	}

	sha, _ := runGit(repo, "rev-parse", "--short", "HEAD")

	return OpsCommitOutput{
		SHA:         strings.TrimSpace(sha),
		Message:     input.Message,
		FilesStaged: stagedFiles,
		Insertions:  ins,
		Deletions:   del,
		DryRun:      false,
		Warnings:    warnings,
	}, nil
}

func opsPRCreate(_ context.Context, input OpsPRCreateInput) (OpsPRCreateOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsPRCreateOutput{}, err
	}

	head := opsCurrentBranch(repo)
	base := input.Base
	if base == "" {
		base = "main"
	}

	// Count commits ahead of base
	countOut, _ := runGit(repo, "rev-list", "--count", base+".."+head)
	commits, _ := strconv.Atoi(strings.TrimSpace(countOut))

	if !input.Execute {
		return OpsPRCreateOutput{
			Title:   input.Title,
			Base:    base,
			Head:    head,
			Commits: commits,
			DryRun:  true,
			Pushed:  false,
		}, nil
	}

	// Push
	if _, err := runGit(repo, "push", "-u", "origin", head); err != nil {
		return OpsPRCreateOutput{}, fmt.Errorf("git push: %w", err)
	}

	// Create PR
	args := []string{"pr", "create", "--title", input.Title, "--base", base, "--json", "url,number"}
	if input.Body != "" {
		args = append(args, "--body", input.Body)
	}
	if input.Draft {
		args = append(args, "--draft")
	}

	ghOut, ghErr, _, ghRunErr := opsRunWithTimeout(30*time.Second, repo, "gh", args...)
	if ghRunErr != nil {
		return OpsPRCreateOutput{}, fmt.Errorf("gh pr create: %s %s", ghOut, ghErr)
	}

	var prData struct {
		URL    string `json:"url"`
		Number int    `json:"number"`
	}
	if err := json.Unmarshal([]byte(ghOut), &prData); err != nil {
		return OpsPRCreateOutput{}, fmt.Errorf("parse gh output: %w (raw: %s)", err, ghOut)
	}

	return OpsPRCreateOutput{
		URL:     prData.URL,
		Number:  prData.Number,
		Title:   input.Title,
		Base:    base,
		Head:    head,
		Commits: commits,
		DryRun:  false,
		Pushed:  true,
	}, nil
}

// ---------------------------------------------------------------------------
// Atomic handlers — CI
// ---------------------------------------------------------------------------

func opsCIStatus(_ context.Context, input OpsCIStatusInput) (OpsCIStatusOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsCIStatusOutput{}, err
	}

	branch := input.Branch
	if branch == "" {
		branch = opsCurrentBranch(repo)
	}

	sha, _ := runGit(repo, "rev-parse", "HEAD")

	pollChecks := func() ([]CICheck, string) {
		var args []string
		if input.PR > 0 {
			args = []string{"pr", "checks", strconv.Itoa(input.PR), "--json", "name,state,conclusion,detailsUrl,elapsedTime"}
		} else {
			args = []string{"run", "list", "--branch", branch, "--limit", "20", "--json", "name,status,conclusion,url,databaseId"}
		}
		ghOut, _, _, _ := opsRunWithTimeout(15*time.Second, repo, "gh", args...)

		var checks []CICheck
		var rawChecks []map[string]any
		if err := json.Unmarshal([]byte(ghOut), &rawChecks); err != nil || len(rawChecks) == 0 {
			return checks, "no_ci"
		}

		allDone := true
		anyFail := false
		for _, rc := range rawChecks {
			name, _ := rc["name"].(string)
			status, _ := rc["status"].(string)
			if status == "" {
				if s, ok := rc["state"].(string); ok {
					status = s
				}
			}
			conclusion, _ := rc["conclusion"].(string)
			url, _ := rc["url"].(string)
			if url == "" {
				if u, ok := rc["detailsUrl"].(string); ok {
					url = u
				}
			}

			if status != "completed" && status != "SUCCESS" && status != "FAILURE" {
				allDone = false
			}
			if conclusion == "failure" || status == "FAILURE" {
				anyFail = true
			}
			checks = append(checks, CICheck{
				Name:       name,
				Status:     status,
				Conclusion: conclusion,
				URL:        url,
			})
		}

		overall := "pending"
		if allDone {
			if anyFail {
				overall = "fail"
			} else {
				overall = "pass"
			}
		}
		return checks, overall
	}

	checks, overall := pollChecks()
	waited := 0

	if input.Wait && overall == "pending" {
		maxWait := opsCIPollMax
		for time.Duration(waited)*time.Second < maxWait {
			time.Sleep(opsCIPollDelay)
			waited += int(opsCIPollDelay.Seconds())
			checks, overall = pollChecks()
			if overall != "pending" {
				break
			}
		}
	}

	return OpsCIStatusOutput{
		Branch:    branch,
		SHA:       strings.TrimSpace(sha),
		Overall:   overall,
		Checks:    checks,
		WaitedSec: waited,
	}, nil
}

// ---------------------------------------------------------------------------
// Composed handlers
// ---------------------------------------------------------------------------

func opsPrePush(ctx context.Context, input OpsPrePushInput) (OpsPrePushOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsPrePushOutput{}, err
	}

	totalStart := time.Now()
	var steps []PrePushStep
	var allErrors []CompileError
	var allFailures []TestFailure
	lang := opsDetectLanguage(repo)

	// Step 1: Language-specific static analysis
	switch lang {
	case "go":
		// Go vet
		start := time.Now()
		_, vetStderr, vetCode, _ := opsRunWithTimeout(30*time.Second, repo, "go", "vet", "./...")
		vetStep := PrePushStep{Name: "vet", Status: "pass", DurationMs: time.Since(start).Milliseconds()}
		if vetCode != 0 {
			vetStep.Status = "fail"
			vetStep.ErrorCount = len(strings.Split(strings.TrimSpace(vetStderr), "\n"))
			steps = append(steps, vetStep)
			return OpsPrePushOutput{
				Repo: repo, Overall: "fail", DurationMs: time.Since(totalStart).Milliseconds(),
				Steps: steps, FailedStep: "vet",
			}, nil
		}
		steps = append(steps, vetStep)

		// Go lint (optional)
		if !input.SkipLint {
			start = time.Now()
			_, _, lintCode, _ := opsRunWithTimeout(60*time.Second, repo, "golangci-lint", "run", "./...")
			lintStep := PrePushStep{Name: "lint", Status: "pass", DurationMs: time.Since(start).Milliseconds()}
			if lintCode != 0 {
				lintStep.Status = "fail"
				steps = append(steps, lintStep)
				return OpsPrePushOutput{
					Repo: repo, Overall: "fail", DurationMs: time.Since(totalStart).Milliseconds(),
					Steps: steps, FailedStep: "lint",
				}, nil
			}
			steps = append(steps, lintStep)
		}

	case "node":
		// Check if lint script exists in package.json before running
		if !input.SkipLint {
			if opsHasNPMScript(repo, "lint") {
				start := time.Now()
				_, _, lintCode, _ := opsRunWithTimeout(60*time.Second, repo, "npm", "run", "lint")
				lintStep := PrePushStep{Name: "lint", Status: "pass", DurationMs: time.Since(start).Milliseconds()}
				if lintCode != 0 {
					lintStep.Status = "fail"
					steps = append(steps, lintStep)
					return OpsPrePushOutput{
						Repo: repo, Overall: "fail", DurationMs: time.Since(totalStart).Milliseconds(),
						Steps: steps, FailedStep: "lint",
					}, nil
				}
				steps = append(steps, lintStep)
			} else {
				steps = append(steps, PrePushStep{Name: "lint", Status: "skip", DurationMs: 0})
			}
		}

	case "python":
		// Check if ruff is available before running
		if !input.SkipLint {
			if _, err := exec.LookPath("ruff"); err == nil {
				start := time.Now()
				_, _, lintCode, _ := opsRunWithTimeout(60*time.Second, repo, "ruff", "check", ".")
				lintStep := PrePushStep{Name: "lint", Status: "pass", DurationMs: time.Since(start).Milliseconds()}
				if lintCode != 0 {
					lintStep.Status = "fail"
					steps = append(steps, lintStep)
					return OpsPrePushOutput{
						Repo: repo, Overall: "fail", DurationMs: time.Since(totalStart).Milliseconds(),
						Steps: steps, FailedStep: "lint",
					}, nil
				}
				steps = append(steps, lintStep)
			} else {
				steps = append(steps, PrePushStep{Name: "lint", Status: "skip", DurationMs: 0})
			}
		}
	}

	// Step 3: build
	buildOut, _ := opsBuild(ctx, OpsBuildInput{Repo: repo})
	buildStep := PrePushStep{Name: "build", Status: "pass", DurationMs: buildOut.DurationMs, ErrorCount: buildOut.ErrorCount}
	if !buildOut.Success {
		buildStep.Status = "fail"
		allErrors = buildOut.Errors
		steps = append(steps, buildStep)
		return OpsPrePushOutput{
			Repo: repo, Overall: "fail", DurationMs: time.Since(totalStart).Milliseconds(),
			Steps: steps, FailedStep: "build", Errors: allErrors,
		}, nil
	}
	steps = append(steps, buildStep)

	// Step 4: test (changed packages only)
	testOut, _ := opsTestSmart(ctx, OpsTestSmartInput{Repo: repo})
	testStep := PrePushStep{Name: "test", Status: "pass", DurationMs: testOut.DurationMs}
	if testOut.Failed > 0 {
		testStep.Status = "fail"
		testStep.ErrorCount = testOut.Failed
		allFailures = testOut.Failures
		steps = append(steps, testStep)
		return OpsPrePushOutput{
			Repo: repo, Overall: "fail", DurationMs: time.Since(totalStart).Milliseconds(),
			Steps: steps, FailedStep: "test", Failures: allFailures,
		}, nil
	}
	steps = append(steps, testStep)

	return OpsPrePushOutput{
		Repo:       repo,
		Overall:    "pass",
		DurationMs: time.Since(totalStart).Milliseconds(),
		Steps:      steps,
	}, nil
}

func opsIterate(ctx context.Context, input OpsIterateInput) (OpsIterateOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsIterateOutput{}, err
	}

	// Get or create session
	sessionID := input.SessionID
	if sessionID == "" {
		// Auto-create session
		createOut, _ := opsSessionCreate(ctx, OpsSessionCreateInput{Repo: repo})
		sessionID = createOut.SessionID
	}

	opsSessionMu.RLock()
	session, err := opsReadSession(sessionID)
	if err != nil {
		opsSessionMu.RUnlock()
		return OpsIterateOutput{}, err
	}
	existingIters := opsReadIterations(sessionID)
	iterNum := len(existingIters) + 1
	opsSessionMu.RUnlock()
	_ = session // used for context

	iterStart := time.Now()

	// Step 1: Build
	buildOut, _ := opsBuild(ctx, OpsBuildInput{Repo: repo})

	var testOut *OpsTestSmartOutput
	var analysis *OpsAnalyzeOutput
	var nextActions []NextAction
	status := "all_pass"
	errorCount := 0

	// Build rich NextActions from analysis issues
	buildNextActions := func(analyzeOut *OpsAnalyzeOutput) []NextAction {
		var actions []NextAction
		for _, issue := range analyzeOut.Issues {
			actions = append(actions, NextAction{
				File:       issue.File,
				Line:       issue.Line,
				Category:   issue.Category,
				Message:    issue.Message,
				Suggestion: issue.Suggestion,
			})
		}
		return actions
	}

	if !buildOut.Success {
		status = "build_fail"
		errorCount = buildOut.ErrorCount
		analyzeOut, _ := opsAnalyze(ctx, OpsAnalyzeInput{BuildErrors: buildOut.Errors})
		analysis = &analyzeOut
		nextActions = buildNextActions(&analyzeOut)
	} else {
		// Step 2: Test (only if build passed)
		testResult, _ := opsTestSmart(ctx, OpsTestSmartInput{Repo: repo})
		testOut = &testResult

		if testResult.Failed > 0 {
			status = "test_fail"
			errorCount = testResult.Failed
			analyzeOut, _ := opsAnalyze(ctx, OpsAnalyzeInput{TestFailures: testResult.Failures})
			analysis = &analyzeOut
			nextActions = buildNextActions(&analyzeOut)
		}
	}

	duration := time.Since(iterStart).Milliseconds()

	// Record iteration
	record := IterationRecord{
		Number:      iterNum,
		StartedAt:   iterStart.UnixMilli(),
		DurationMs:  duration,
		Status:      status,
		ErrorCount:  errorCount,
		BuildOK:     buildOut.Success,
		TestsPassed: 0,
		TestsFailed: 0,
	}
	if testOut != nil {
		record.TestsPassed = testOut.Passed
		record.TestsFailed = testOut.Failed
	}

	opsSessionMu.Lock()
	opsAppendIteration(sessionID, record)
	// Build history summary under lock
	allIters := opsReadIterations(sessionID)
	var history []IterationSummary
	for _, iter := range allIters {
		history = append(history, IterationSummary{
			Number:     iter.Number,
			Status:     iter.Status,
			ErrorCount: iter.ErrorCount,
			DurationMs: iter.DurationMs,
		})
	}
	opsSessionMu.Unlock()

	return OpsIterateOutput{
		SessionID:   sessionID,
		Iteration:   iterNum,
		Build:       &buildOut,
		Test:        testOut,
		Analysis:    analysis,
		Status:      status,
		DurationMs:  duration,
		History:     history,
		NextActions: nextActions,
	}, nil
}

func opsShip(ctx context.Context, input OpsShipInput) (OpsShipOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsShipOutput{}, err
	}

	// Step 1: Pre-push gate
	prePush, _ := opsPrePush(ctx, OpsPrePushInput{Repo: repo})

	if prePush.Overall != "pass" {
		return OpsShipOutput{
			DryRun:    true,
			PrePush:   &prePush,
			Overall:   "blocked",
			BlockedAt: "pre_push:" + prePush.FailedStep,
		}, nil
	}

	if !input.Execute {
		// Preview commit
		commitPreview, _ := opsCommit(ctx, OpsCommitInput{Repo: repo, Message: input.Message})
		title := input.Title
		if title == "" {
			title = input.Message
		}
		prPreview, _ := opsPRCreate(ctx, OpsPRCreateInput{Repo: repo, Title: title, Body: input.Body, Draft: input.Draft})
		return OpsShipOutput{
			DryRun:  true,
			PrePush: &prePush,
			Commit:  &commitPreview,
			PR:      &prPreview,
			Overall: "dry_run",
		}, nil
	}

	// Step 2: Commit
	commitOut, err := opsCommit(ctx, OpsCommitInput{Repo: repo, Message: input.Message, Execute: true})
	if err != nil {
		return OpsShipOutput{
			PrePush:   &prePush,
			Overall:   "blocked",
			BlockedAt: "commit",
			Error:     err.Error(),
		}, nil
	}

	// Step 3: Push + PR
	title := input.Title
	if title == "" {
		title = input.Message
	}
	prOut, err := opsPRCreate(ctx, OpsPRCreateInput{Repo: repo, Title: title, Body: input.Body, Draft: input.Draft, Execute: true})
	if err != nil {
		return OpsShipOutput{
			PrePush:   &prePush,
			Commit:    &commitOut,
			Overall:   "blocked",
			BlockedAt: "pr_create",
			Error:     err.Error(),
		}, nil
	}

	return OpsShipOutput{
		DryRun:  false,
		PrePush: &prePush,
		Commit:  &commitOut,
		PR:      &prOut,
		Overall: "shipped",
	}, nil
}

// ---------------------------------------------------------------------------
// Session handlers
// ---------------------------------------------------------------------------

func opsSessionCreate(_ context.Context, input OpsSessionCreateInput) (OpsSessionCreateOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsSessionCreateOutput{}, err
	}

	id := opsID()
	branch := opsCurrentBranch(repo)
	now := time.Now()

	session := &OpsSession{
		ID:          id,
		Repo:        repo,
		Branch:      branch,
		Description: input.Description,
		CreatedAt:   now,
	}

	opsSessionMu.Lock()
	err = opsWriteSession(session)
	opsSessionMu.Unlock()
	if err != nil {
		return OpsSessionCreateOutput{}, fmt.Errorf("write session: %w", err)
	}

	return OpsSessionCreateOutput{
		SessionID: id,
		Repo:      repo,
		Branch:    branch,
		CreatedAt: now.Format(time.RFC3339),
	}, nil
}

func opsSessionStatus(_ context.Context, input OpsSessionStatusInput) (OpsSessionStatusOutput, error) {
	opsSessionMu.RLock()
	session, err := opsReadSession(input.SessionID)
	opsSessionMu.RUnlock()
	if err != nil {
		return OpsSessionStatusOutput{}, err
	}

	iterations := opsReadIterations(input.SessionID)

	var totalTime int64
	var errorTrend []int
	var history []IterationSummary
	currentState := "untested"

	for _, iter := range iterations {
		totalTime += iter.DurationMs
		errorTrend = append(errorTrend, iter.ErrorCount)
		history = append(history, IterationSummary{
			Number:     iter.Number,
			Status:     iter.Status,
			ErrorCount: iter.ErrorCount,
			DurationMs: iter.DurationMs,
		})
		currentState = iter.Status
	}

	converging := false
	if len(errorTrend) >= 3 {
		last3 := errorTrend[len(errorTrend)-3:]
		converging = last3[0] > last3[1] && last3[1] > last3[2]
	}

	return OpsSessionStatusOutput{
		SessionID:    session.ID,
		Repo:         session.Repo,
		Branch:       session.Branch,
		Iterations:   len(iterations),
		TotalTimeMs:  totalTime,
		CurrentState: currentState,
		ErrorTrend:   errorTrend,
		Converging:   converging,
		History:      history,
	}, nil
}

func opsSessionList(_ context.Context, _ OpsSessionListInput) (OpsSessionListOutput, error) {
	opsSessionMu.RLock()
	defer opsSessionMu.RUnlock()

	var sessions []SessionSummary
	for _, id := range opsListSessionIDs() {
		s, err := opsReadSession(id)
		if err != nil {
			continue
		}
		// Auto-cleanup sessions older than 7 days
		if time.Since(s.CreatedAt) > 7*24*time.Hour {
			os.RemoveAll(opsSessionDir(id))
			continue
		}
		iterations := opsReadIterations(id)
		state := "untested"
		if len(iterations) > 0 {
			state = iterations[len(iterations)-1].Status
		}
		sessions = append(sessions, SessionSummary{
			SessionID:   s.ID,
			Repo:        s.Repo,
			Branch:      s.Branch,
			Iterations:  len(iterations),
			State:       state,
			StartedAt:   s.CreatedAt.Format(time.RFC3339),
			Description: s.Description,
		})
	}

	return OpsSessionListOutput{Sessions: sessions}, nil
}

// ---------------------------------------------------------------------------
// Revert handler
// ---------------------------------------------------------------------------

func opsRevert(_ context.Context, input OpsRevertInput) (OpsRevertOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsRevertOutput{}, err
	}

	// Get last commit info
	sha, _ := runGit(repo, "rev-parse", "--short", "HEAD")
	sha = strings.TrimSpace(sha)
	msg, _ := runGit(repo, "log", "-1", "--format=%s")
	msg = strings.TrimSpace(msg)

	// Check if last commit is pushed
	branch := opsCurrentBranch(repo)
	pushed := false
	if _, err := runGit(repo, "rev-parse", "origin/"+branch); err == nil {
		// Check if HEAD is an ancestor of origin/branch (meaning it's pushed)
		_, err := runGit(repo, "merge-base", "--is-ancestor", "HEAD", "origin/"+branch)
		pushed = err == nil
	}

	method := "soft_reset"
	if pushed {
		method = "revert_commit"
	}

	if !input.Execute {
		return OpsRevertOutput{
			Method:    method,
			SHA:       sha,
			Message:   msg,
			WasPushed: pushed,
			DryRun:    true,
		}, nil
	}

	if pushed {
		// Safe: create revert commit
		if _, err := runGit(repo, "revert", "--no-edit", "HEAD"); err != nil {
			return OpsRevertOutput{}, fmt.Errorf("git revert: %w", err)
		}
	} else {
		// Safe: soft reset (keeps changes staged)
		if _, err := runGit(repo, "reset", "--soft", "HEAD~1"); err != nil {
			return OpsRevertOutput{}, fmt.Errorf("git reset --soft: %w", err)
		}
	}

	return OpsRevertOutput{
		Method:    method,
		SHA:       sha,
		Message:   msg,
		WasPushed: pushed,
		DryRun:    false,
	}, nil
}

// ---------------------------------------------------------------------------
// Fleet iteration handler
// ---------------------------------------------------------------------------

func opsFleetIterate(ctx context.Context, input OpsFleetIterateInput) (OpsFleetIterateOutput, error) {
	dir := input.Dir
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), "hairglasses-studio")
	}

	langFilter := input.Language
	if langFilter == "" {
		langFilter = "all"
	}

	maxRepos := input.MaxRepos
	if maxRepos <= 0 {
		maxRepos = 20
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return OpsFleetIterateOutput{}, fmt.Errorf("[%s] cannot read directory: %w", handler.ErrInvalidParam, err)
	}

	var results []FleetRepoResult
	passing, failing, skipped := 0, 0, 0

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if len(results) >= maxRepos {
			break
		}

		repoPath := filepath.Join(dir, e.Name())
		lang := opsDetectLanguage(repoPath)

		// Skip non-project directories
		if lang == "unknown" {
			continue
		}
		// Apply language filter
		if langFilter != "all" && lang != langFilter {
			skipped++
			continue
		}

		start := time.Now()

		// Build
		buildOut, buildErr := opsBuild(ctx, OpsBuildInput{Repo: repoPath})
		if buildErr != nil {
			results = append(results, FleetRepoResult{
				Repo: e.Name(), Language: lang, Status: "skip",
				DurationMs: time.Since(start).Milliseconds(),
			})
			skipped++
			continue
		}

		if !buildOut.Success {
			results = append(results, FleetRepoResult{
				Repo: e.Name(), Language: lang, BuildOK: false,
				ErrorCount: buildOut.ErrorCount, Status: "build_fail",
				DurationMs: time.Since(start).Milliseconds(),
			})
			failing++
			continue
		}

		// Test
		testOut, _ := opsTestSmart(ctx, OpsTestSmartInput{Repo: repoPath, All: true})
		status := "pass"
		if testOut.Failed > 0 {
			status = "test_fail"
			failing++
		} else {
			passing++
		}

		results = append(results, FleetRepoResult{
			Repo:        e.Name(),
			Language:    lang,
			BuildOK:     true,
			TestsPassed: testOut.Passed,
			TestsFailed: testOut.Failed,
			ErrorCount:  testOut.Failed,
			Status:      status,
			DurationMs:  time.Since(start).Milliseconds(),
		})
	}

	return OpsFleetIterateOutput{
		Total:   len(results),
		Passing: passing,
		Failing: failing,
		Skipped: skipped,
		Results: results,
	}, nil
}

// ---------------------------------------------------------------------------
// Release handler
// ---------------------------------------------------------------------------

func opsRelease(_ context.Context, input OpsReleaseInput) (OpsReleaseOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsReleaseOutput{}, err
	}

	version := strings.TrimPrefix(input.Version, "v")
	tag := "v" + version
	lang := opsDetectLanguage(repo)

	// Detect current version
	currentVersion := ""
	var filesToModify []string

	switch lang {
	case "go":
		// Go: version from git tags
		if out, err := runGit(repo, "describe", "--tags", "--abbrev=0"); err == nil {
			currentVersion = strings.TrimSpace(out)
		}
		// No file to modify for Go (version comes from git tags)

	case "node":
		// Node: read package.json
		data, err := os.ReadFile(filepath.Join(repo, "package.json"))
		if err == nil {
			var pkg map[string]any
			_ = json.Unmarshal(data, &pkg) // parse failure leaves pkg nil; the `ok` check below handles it
			if v, ok := pkg["version"].(string); ok {
				currentVersion = v
			}
			filesToModify = append(filesToModify, "package.json")
		}

	case "python":
		// Python: read pyproject.toml
		data, err := os.ReadFile(filepath.Join(repo, "pyproject.toml"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "version") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						currentVersion = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
					}
				}
			}
			filesToModify = append(filesToModify, "pyproject.toml")
		}
	}

	// Generate changelog entry
	changelogEntry := ""
	if input.AutoChangelog {
		// Get commits since last tag
		lastTag := currentVersion
		if lastTag == "" {
			lastTag = "HEAD~10"
		}
		commits, _ := runGit(repo, "log", lastTag+"..HEAD", "--oneline", "--no-merges")
		changelogEntry = fmt.Sprintf("## %s\n\n%s\n", tag, strings.TrimSpace(commits))
		filesToModify = append(filesToModify, "CHANGELOG.md")
	}

	if !input.Execute {
		return OpsReleaseOutput{
			CurrentVersion: currentVersion,
			NewVersion:     version,
			FilesModified:  filesToModify,
			Tag:            tag,
			ChangelogEntry: changelogEntry,
			DryRun:         true,
		}, nil
	}

	// Execute: update version files.
	// Errors from the write path are intentionally not propagated here —
	// the caller (release tool) drives a \`git add -A && git commit\`
	// afterward, which re-surfaces any missing/stale file via the commit
	// diff. Leaving the \`_ =\` marker so errcheck stays clean and the
	// intent is explicit; a future refactor should capture the error and
	// include it in the tool result's `warnings` field.
	switch lang {
	case "node":
		data, _ := os.ReadFile(filepath.Join(repo, "package.json"))
		var pkg map[string]any
		_ = json.Unmarshal(data, &pkg)
		pkg["version"] = version
		updated, _ := json.MarshalIndent(pkg, "", "  ")
		_ = os.WriteFile(filepath.Join(repo, "package.json"), append(updated, '\n'), 0o644)

	case "python":
		data, _ := os.ReadFile(filepath.Join(repo, "pyproject.toml"))
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "version") && strings.Contains(line, "=") {
				lines[i] = fmt.Sprintf(`version = "%s"`, version)
				break
			}
		}
		_ = os.WriteFile(filepath.Join(repo, "pyproject.toml"), []byte(strings.Join(lines, "\n")), 0o644)
	}

	// Update CHANGELOG.md
	if input.AutoChangelog && changelogEntry != "" {
		clPath := filepath.Join(repo, "CHANGELOG.md")
		existing, _ := os.ReadFile(clPath)
		newContent := changelogEntry + "\n" + string(existing)
		_ = os.WriteFile(clPath, []byte(newContent), 0o644)
	}

	// Commit
	runGit(repo, "add", "-A")
	runGit(repo, "commit", "-m", fmt.Sprintf("chore: release %s", tag))

	// Tag
	runGit(repo, "tag", "-a", tag, "-m", fmt.Sprintf("Release %s", tag))

	// Push
	pushed := false
	if input.Push {
		runGit(repo, "push", "origin", opsCurrentBranch(repo))
		runGit(repo, "push", "origin", tag)
		pushed = true
	}

	return OpsReleaseOutput{
		CurrentVersion: currentVersion,
		NewVersion:     version,
		FilesModified:  filesToModify,
		Tag:            tag,
		ChangelogEntry: changelogEntry,
		Committed:      true,
		Pushed:         pushed,
		DryRun:         false,
	}, nil
}

// ---------------------------------------------------------------------------
// Coverage Report
// ---------------------------------------------------------------------------

func opsCoverageReport(_ context.Context, input OpsCoverageReportInput) (OpsCoverageReportOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsCoverageReportOutput{}, err
	}
	lang := opsDetectLanguage(repo)

	switch lang {
	case "go":
		coverFile := filepath.Join(os.TempDir(), fmt.Sprintf("cover-%s.out", opsID()))
		defer os.Remove(coverFile)

		// Run coverage
		_, stderr, exitCode, err := opsRunWithTimeout(opsBuildTimeout, repo,
			"go", "test", "-coverprofile="+coverFile, "./...")
		if err != nil {
			return OpsCoverageReportOutput{}, fmt.Errorf("coverage command failed: %w", err)
		}
		if exitCode != 0 {
			// Tests failed but may still have partial coverage
			if _, statErr := os.Stat(coverFile); statErr != nil {
				return OpsCoverageReportOutput{
					Language:   lang,
					GatePassed: false,
					Threshold:  input.Threshold,
				}, fmt.Errorf("[%s] tests failed (exit %d): %s", handler.ErrInvalidParam, exitCode, strings.TrimSpace(stderr))
			}
		}

		// Parse coverage func output
		stdout, _, _, err := opsRunWithTimeout(30*time.Second, repo,
			"go", "tool", "cover", "-func="+coverFile)
		if err != nil {
			return OpsCoverageReportOutput{}, fmt.Errorf("go tool cover failed: %w", err)
		}

		var overallPct float64
		pkgCoverage := make(map[string][]float64) // package -> list of function percentages

		for _, line := range strings.Split(stdout, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Total line: "total:	(statements)	72.3%"
			if strings.HasPrefix(line, "total:") {
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					pctStr := strings.TrimSuffix(fields[len(fields)-1], "%")
					if v, err := strconv.ParseFloat(pctStr, 64); err == nil {
						overallPct = v
					}
				}
				continue
			}

			// Function line: "github.com/pkg/file.go:42:	FuncName	85.7%"
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				pctStr := strings.TrimSuffix(fields[len(fields)-1], "%")
				pct, err := strconv.ParseFloat(pctStr, 64)
				if err != nil {
					continue
				}
				// Extract package from file path: "github.com/org/repo/pkg/file.go:42:"
				filePart := fields[0]
				idx := strings.LastIndex(filePart, "/")
				pkgName := filePart
				if idx > 0 {
					pkgName = filePart[:idx]
				}
				pkgCoverage[pkgName] = append(pkgCoverage[pkgName], pct)
			}
		}

		// Aggregate per-package
		var packages []OpsCoveragePackage
		for name, pcts := range pkgCoverage {
			var sum float64
			for _, p := range pcts {
				sum += p
			}
			avg := sum / float64(len(pcts))
			packages = append(packages, OpsCoveragePackage{
				Name:        name,
				CoveragePct: math.Round(avg*10) / 10,
			})
		}

		gatePassed := true
		if input.Threshold > 0 && overallPct < input.Threshold {
			gatePassed = false
		}

		return OpsCoverageReportOutput{
			Language:   lang,
			OverallPct: overallPct,
			GatePassed: gatePassed,
			Threshold:  input.Threshold,
			Packages:   packages,
		}, nil

	case "node":
		// Try nyc or c8
		stdout, stderr, exitCode, _ := opsRunWithTimeout(opsBuildTimeout, repo,
			"npx", "c8", "report", "--reporter=text")
		if exitCode != 0 {
			stdout, stderr, exitCode, _ = opsRunWithTimeout(opsBuildTimeout, repo,
				"npx", "nyc", "report", "--reporter=text")
		}
		if exitCode != 0 {
			return OpsCoverageReportOutput{
				Language: lang,
			}, fmt.Errorf("[%s] no coverage tool found (tried c8, nyc): %s", handler.ErrInvalidParam, strings.TrimSpace(stderr))
		}

		// Parse "All files" summary line from text reporter
		var overallPct float64
		for _, line := range strings.Split(stdout, "\n") {
			if strings.Contains(line, "All files") {
				fields := strings.Fields(line)
				for _, f := range fields {
					if v, err := strconv.ParseFloat(f, 64); err == nil {
						overallPct = v
						break
					}
				}
			}
		}

		gatePassed := true
		if input.Threshold > 0 && overallPct < input.Threshold {
			gatePassed = false
		}

		return OpsCoverageReportOutput{
			Language:   lang,
			OverallPct: overallPct,
			GatePassed: gatePassed,
			Threshold:  input.Threshold,
		}, nil

	case "python":
		_, stderr, exitCode, _ := opsRunWithTimeout(opsBuildTimeout, repo,
			"python3", "-m", "coverage", "run", "-m", "pytest")
		if exitCode != 0 {
			return OpsCoverageReportOutput{
				Language: lang,
			}, fmt.Errorf("[%s] coverage run failed: %s", handler.ErrInvalidParam, strings.TrimSpace(stderr))
		}
		stdout, _, _, _ := opsRunWithTimeout(30*time.Second, repo,
			"python3", "-m", "coverage", "report")

		var overallPct float64
		for _, line := range strings.Split(stdout, "\n") {
			if strings.HasPrefix(line, "TOTAL") {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					pctStr := strings.TrimSuffix(fields[len(fields)-1], "%")
					if v, err := strconv.ParseFloat(pctStr, 64); err == nil {
						overallPct = v
					}
				}
			}
		}

		gatePassed := true
		if input.Threshold > 0 && overallPct < input.Threshold {
			gatePassed = false
		}

		return OpsCoverageReportOutput{
			Language:   lang,
			OverallPct: overallPct,
			GatePassed: gatePassed,
			Threshold:  input.Threshold,
		}, nil

	default:
		return OpsCoverageReportOutput{}, fmt.Errorf("[%s] unsupported language: %s", handler.ErrInvalidParam, lang)
	}
}

// ---------------------------------------------------------------------------
// Lint Fix
// ---------------------------------------------------------------------------

func opsLintFix(_ context.Context, input OpsLintFixInput) (OpsLintFixOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsLintFixOutput{}, err
	}
	lang := opsDetectLanguage(repo)
	lintTimeout := 60 * time.Second

	// Snapshot changed files before lint
	beforeDiff, _ := runGit(repo, "diff", "--name-only")
	beforeFiles := make(map[string]bool)
	for _, f := range strings.Split(strings.TrimSpace(beforeDiff), "\n") {
		if f != "" {
			beforeFiles[f] = true
		}
	}

	var toolUsed string
	var combinedOutput strings.Builder
	var errorsRemaining int

	switch lang {
	case "go":
		// Try goimports first
		_, goimportsErr := exec.LookPath("goimports")
		if goimportsErr == nil {
			stdout, stderr, _, _ := opsRunWithTimeout(lintTimeout, repo, "goimports", "-w", ".")
			toolUsed = "goimports"
			combinedOutput.WriteString(stdout)
			combinedOutput.WriteString(stderr)
		}

		// Then try golangci-lint --fix
		_, golintErr := exec.LookPath("golangci-lint")
		if golintErr == nil {
			stdout, stderr, exitCode, _ := opsRunWithTimeout(lintTimeout, repo, "golangci-lint", "run", "--fix", "./...")
			if toolUsed != "" {
				toolUsed += " + golangci-lint"
			} else {
				toolUsed = "golangci-lint"
			}
			combinedOutput.WriteString(stdout)
			combinedOutput.WriteString(stderr)
			if exitCode != 0 {
				// Count remaining errors from output
				for _, line := range strings.Split(stderr+"\n"+stdout, "\n") {
					if compileErrorRe.MatchString(strings.TrimSpace(line)) {
						errorsRemaining++
					}
				}
			}
		}

		if goimportsErr != nil && golintErr != nil {
			return OpsLintFixOutput{
				Language: lang,
				Output:   "No Go lint tools found. Install with: go install golang.org/x/tools/cmd/goimports@latest && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest",
			}, nil
		}

	case "node":
		// Try eslint --fix first, then prettier
		stdout, stderr, exitCode, _ := opsRunWithTimeout(lintTimeout, repo, "npx", "eslint", "--fix", ".")
		toolUsed = "eslint"
		combinedOutput.WriteString(stdout)
		combinedOutput.WriteString(stderr)
		if exitCode != 0 {
			for _, line := range strings.Split(stderr+"\n"+stdout, "\n") {
				line = strings.TrimSpace(line)
				if strings.Contains(line, "error") && strings.Contains(line, ":") {
					errorsRemaining++
				}
			}
		}

		// Also run prettier
		stdout2, stderr2, _, _ := opsRunWithTimeout(lintTimeout, repo, "npx", "prettier", "--write", ".")
		toolUsed += " + prettier"
		combinedOutput.WriteString(stdout2)
		combinedOutput.WriteString(stderr2)

	case "python":
		// black + isort
		_, blackErr := exec.LookPath("black")
		_, isortErr := exec.LookPath("isort")

		if blackErr != nil && isortErr != nil {
			return OpsLintFixOutput{
				Language: lang,
				Output:   "No Python lint tools found. Install with: pip install black isort",
			}, nil
		}

		if blackErr == nil {
			stdout, stderr, _, _ := opsRunWithTimeout(lintTimeout, repo, "black", ".")
			toolUsed = "black"
			combinedOutput.WriteString(stdout)
			combinedOutput.WriteString(stderr)
		}
		if isortErr == nil {
			stdout, stderr, _, _ := opsRunWithTimeout(lintTimeout, repo, "isort", ".")
			if toolUsed != "" {
				toolUsed += " + isort"
			} else {
				toolUsed = "isort"
			}
			combinedOutput.WriteString(stdout)
			combinedOutput.WriteString(stderr)
		}

	default:
		return OpsLintFixOutput{}, fmt.Errorf("[%s] unsupported language: %s", handler.ErrInvalidParam, lang)
	}

	// Count files changed by lint
	afterDiff, _ := runGit(repo, "diff", "--name-only")
	filesFixed := 0
	for _, f := range strings.Split(strings.TrimSpace(afterDiff), "\n") {
		if f != "" && !beforeFiles[f] {
			filesFixed++
		}
	}

	// Truncate output to avoid huge payloads
	output := combinedOutput.String()
	if len(output) > 4096 {
		output = output[:4096] + "\n... (truncated)"
	}

	return OpsLintFixOutput{
		Language:        lang,
		ToolUsed:        toolUsed,
		FilesFixed:      filesFixed,
		ErrorsRemaining: errorsRemaining,
		Output:          strings.TrimSpace(output),
	}, nil
}

// ---------------------------------------------------------------------------
// Changelog Generate
// ---------------------------------------------------------------------------

var changelogPrefixRe = regexp.MustCompile(`^(feat|fix|chore|docs|refactor|test|ci|perf|build|breaking)(\(.+?\))?(!)?:\s*(.+)`)

func opsChangelogGenerate(_ context.Context, input OpsChangelogGenerateInput) (OpsChangelogGenerateOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsChangelogGenerateOutput{}, err
	}

	// Find last tag
	lastTag := ""
	tagOut, tagErr := runGit(repo, "describe", "--tags", "--abbrev=0", "HEAD")
	if tagErr == nil && strings.TrimSpace(tagOut) != "" {
		lastTag = strings.TrimSpace(tagOut)
	}

	// Get commits since last tag (or all commits)
	var gitLogArgs []string
	if lastTag != "" {
		gitLogArgs = []string{"log", "--oneline", "--no-merges", lastTag + "..HEAD"}
	} else {
		gitLogArgs = []string{"log", "--oneline", "--no-merges"}
	}
	commitsOut, err := runGit(repo, gitLogArgs...)
	if err != nil {
		return OpsChangelogGenerateOutput{}, fmt.Errorf("[%s] git log failed: %w", handler.ErrInvalidParam, err)
	}

	lines := strings.Split(strings.TrimSpace(commitsOut), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return OpsChangelogGenerateOutput{
			LastTag:  lastTag,
			Groups:   map[string]int{},
			Markdown: "No new commits since last tag.",
		}, nil
	}

	// Parse and group commits
	groups := make(map[string][]string) // category -> messages
	groupCounts := make(map[string]int)
	uncategorized := []string{}

	for _, line := range lines {
		// Strip short hash prefix: "abc1234 feat: message" -> "feat: message"
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		msg := parts[1]

		m := changelogPrefixRe.FindStringSubmatch(msg)
		if m != nil {
			category := m[1]
			// Treat BREAKING CHANGE / ! suffix as breaking
			if m[3] == "!" {
				category = "breaking"
			}
			description := m[4]
			groups[category] = append(groups[category], description)
			groupCounts[category]++
		} else {
			// Check for "BREAKING CHANGE:" prefix
			if strings.HasPrefix(strings.ToUpper(msg), "BREAKING CHANGE") {
				groups["breaking"] = append(groups["breaking"], msg)
				groupCounts["breaking"]++
			} else {
				uncategorized = append(uncategorized, msg)
				groupCounts["other"]++
			}
		}
	}

	// Generate markdown
	var md strings.Builder
	date := time.Now().Format("2006-01-02")
	if lastTag != "" {
		md.WriteString(fmt.Sprintf("## Unreleased (since %s) — %s\n\n", lastTag, date))
	} else {
		md.WriteString(fmt.Sprintf("## Unreleased — %s\n\n", date))
	}

	// Ordered sections
	sectionOrder := []struct {
		key   string
		title string
	}{
		{"breaking", "Breaking Changes"},
		{"feat", "Features"},
		{"fix", "Bug Fixes"},
		{"perf", "Performance"},
		{"refactor", "Refactoring"},
		{"docs", "Documentation"},
		{"test", "Tests"},
		{"ci", "CI/CD"},
		{"build", "Build"},
		{"chore", "Chores"},
	}

	for _, sec := range sectionOrder {
		items, ok := groups[sec.key]
		if !ok || len(items) == 0 {
			continue
		}
		md.WriteString(fmt.Sprintf("### %s\n\n", sec.title))
		for _, item := range items {
			md.WriteString(fmt.Sprintf("- %s\n", item))
		}
		md.WriteString("\n")
	}

	if len(uncategorized) > 0 {
		md.WriteString("### Other\n\n")
		for _, item := range uncategorized {
			md.WriteString(fmt.Sprintf("- %s\n", item))
		}
		md.WriteString("\n")
	}

	markdown := md.String()

	// Write to CHANGELOG.md if requested
	written := false
	if input.Write {
		clPath := filepath.Join(repo, "CHANGELOG.md")
		existing, _ := os.ReadFile(clPath)
		newContent := markdown + string(existing)
		if err := os.WriteFile(clPath, []byte(newContent), 0o644); err != nil {
			return OpsChangelogGenerateOutput{}, fmt.Errorf("failed to write CHANGELOG.md: %w", err)
		}
		written = true
	}

	return OpsChangelogGenerateOutput{
		Markdown:    markdown,
		CommitCount: len(lines),
		Groups:      groupCounts,
		Written:     written,
		LastTag:     lastTag,
	}, nil
}

// ---------------------------------------------------------------------------
// Codebase Intelligence handlers
// ---------------------------------------------------------------------------

// Protocol detection maps
var protocolImports = map[string]string{
	"google.golang.org/grpc":          "gRPC",
	"github.com/gorilla/websocket":    "WebSocket",
	"nhooyr.io/websocket":             "WebSocket",
	"github.com/mark3labs/mcp-go":     "MCP",
	"github.com/modelcontextprotocol": "MCP",
	"github.com/gin-gonic/gin":        "REST",
	"github.com/labstack/echo":        "REST",
	"github.com/gofiber/fiber":        "REST",
	"net/http":                        "HTTP",
}

var frameworkPatterns = map[string][]string{
	"LLM":           {"anthropic", "openai", "claude", "gemini"},
	"Database":      {"postgres", "sqlite", "mysql", "mongodb", "gorm", "sqlx", "pgx"},
	"Cache":         {"redis", "memcached", "ristretto"},
	"Messaging":     {"kafka", "rabbitmq", "nats", "mqtt"},
	"CLI":           {"cobra", "urfave/cli", "kingpin"},
	"TUI":           {"bubbletea", "lipgloss", "charm"},
	"Observability": {"opentelemetry", "prometheus", "jaeger", "zap", "slog"},
	"Testing":       {"testify", "gomock", "ginkgo"},
}

func opsRepoAnalyze(_ context.Context, input OpsRepoAnalyzeInput) (OpsRepoAnalyzeOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsRepoAnalyzeOutput{}, err
	}
	start := time.Now()
	name := filepath.Base(repo)
	lang := opsDetectLanguage(repo)

	out := OpsRepoAnalyzeOutput{
		Repo:     repo,
		Name:     name,
		Language: lang,
	}

	// Detect all languages present
	var languages []string
	if lang != "unknown" {
		languages = append(languages, lang)
	}
	if _, err := os.Stat(filepath.Join(repo, "go.mod")); err == nil && lang != "go" {
		languages = append(languages, "go")
	}
	if _, err := os.Stat(filepath.Join(repo, "package.json")); err == nil && lang != "node" {
		languages = append(languages, "node")
	}
	if _, err := os.Stat(filepath.Join(repo, "pyproject.toml")); err == nil && lang != "python" {
		languages = append(languages, "python")
	}
	// Check for shell scripts, Rust, etc.
	if entries, err := filepath.Glob(filepath.Join(repo, "*.sh")); err == nil && len(entries) > 0 {
		languages = append(languages, "shell")
	}
	if _, err := os.Stat(filepath.Join(repo, "Cargo.toml")); err == nil {
		languages = append(languages, "rust")
	}
	out.Languages = languages

	// MCP detection (replicates hg-pipeline.sh logic)
	isMCP := false
	if _, err := os.Stat(filepath.Join(repo, ".mcp.json")); err == nil {
		isMCP = true
	}
	if strings.HasSuffix(name, "-mcp") {
		isMCP = true
	}
	if entries, _ := filepath.Glob(filepath.Join(repo, "cmd", "*mcp*")); len(entries) > 0 {
		isMCP = true
	}
	out.IsMCP = isMCP

	// Protocol and framework detection from Go imports
	protocolSet := make(map[string]bool)
	frameworkSet := make(map[string]bool)
	var keyDeps []string

	if lang == "go" {
		// Parse go.mod for dependencies
		gomodData, err := os.ReadFile(filepath.Join(repo, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(gomodData), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "require") || strings.HasPrefix(line, ")") || strings.HasPrefix(line, "module") || line == "" {
					continue
				}
				// Extract import path (first field of require line)
				parts := strings.Fields(line)
				if len(parts) >= 1 {
					dep := parts[0]
					// Check protocols
					for pattern, proto := range protocolImports {
						if strings.HasPrefix(dep, pattern) {
							protocolSet[proto] = true
						}
					}
					// Check frameworks
					for tag, patterns := range frameworkPatterns {
						for _, p := range patterns {
							if strings.Contains(strings.ToLower(dep), p) {
								frameworkSet[tag] = true
							}
						}
					}
					// Key deps (org-internal or well-known)
					if strings.Contains(dep, "hairglasses-studio") || strings.Contains(dep, "mcpkit") {
						keyDeps = append(keyDeps, dep)
					}
				}
			}
		}
	} else if lang == "node" {
		// Parse package.json
		data, err := os.ReadFile(filepath.Join(repo, "package.json"))
		if err == nil {
			var pkg struct {
				Dependencies    map[string]string `json:"dependencies"`
				DevDependencies map[string]string `json:"devDependencies"`
			}
			_ = json.Unmarshal(data, &pkg) // parse failure leaves both maps nil; the range loops below no-op
			allDeps := make(map[string]string)
			for k, v := range pkg.Dependencies {
				allDeps[k] = v
			}
			for k, v := range pkg.DevDependencies {
				allDeps[k] = v
			}
			for dep := range allDeps {
				lower := strings.ToLower(dep)
				if strings.Contains(lower, "grpc") {
					protocolSet["gRPC"] = true
				}
				if strings.Contains(lower, "express") || strings.Contains(lower, "fastify") || strings.Contains(lower, "koa") {
					protocolSet["REST"] = true
				}
				if strings.Contains(lower, "ws") || strings.Contains(lower, "socket.io") {
					protocolSet["WebSocket"] = true
				}
				if strings.Contains(lower, "mcp") || strings.Contains(lower, "modelcontextprotocol") {
					protocolSet["MCP"] = true
				}
				for tag, patterns := range frameworkPatterns {
					for _, p := range patterns {
						if strings.Contains(lower, p) {
							frameworkSet[tag] = true
						}
					}
				}
			}
		}
	}

	for p := range protocolSet {
		out.Protocols = append(out.Protocols, p)
	}
	for f := range frameworkSet {
		out.Frameworks = append(out.Frameworks, f)
	}
	out.KeyDeps = keyDeps

	// Count test files
	testCount := 0
	filepath.Walk(repo, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			// Skip vendor, node_modules, .git
			if info != nil && info.IsDir() && (info.Name() == "vendor" || info.Name() == "node_modules" || info.Name() == ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".test.js") || strings.HasSuffix(name, ".spec.ts") || strings.HasPrefix(name, "test_") {
			testCount++
		}
		return nil
	})
	out.TestCount = testCount

	// Project metadata
	out.HasCI = sandboxFileExists(filepath.Join(repo, ".github", "workflows"))
	out.HasCLAUDEMD = sandboxFileExists(filepath.Join(repo, "CLAUDE.md"))
	out.HasReadme = sandboxFileExists(filepath.Join(repo, "README.md"))
	out.HasLicense = sandboxFileExists(filepath.Join(repo, "LICENSE"))

	// Build composite tags
	var tags []string
	if isMCP {
		tags = append(tags, "mcp-server")
	}
	if testCount > 0 {
		tags = append(tags, "tested")
	}
	if out.HasCI {
		tags = append(tags, "ci")
	}
	for p := range protocolSet {
		tags = append(tags, strings.ToLower(p))
	}
	out.Tags = tags

	out.AnalysisTimeMs = time.Since(start).Milliseconds()
	return out, nil
}

func opsDepGraph(_ context.Context, input OpsDepGraphInput) (OpsDepGraphOutput, error) {
	dir := input.Dir
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), "hairglasses-studio")
	}

	filter := input.Filter
	if filter == "" {
		filter = "internal"
	}
	format := input.Format
	if format == "" {
		format = "mermaid"
	}

	// Run go mod graph
	stdout, stderr, exitCode, err := opsRunWithTimeout(30*time.Second, dir, "go", "mod", "graph")
	if err != nil || exitCode != 0 {
		return OpsDepGraphOutput{}, fmt.Errorf("go mod graph failed: %s %s", stdout, stderr)
	}

	// Parse edges: "module1@version module2@version"
	type edge struct {
		from, to string
	}
	var edges []edge
	moduleSet := make(map[string]bool)
	orgPrefix := "github.com/hairglasses-studio/"

	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		from := parts[0]
		to := parts[1]

		// Strip version for display
		fromName := strings.Split(from, "@")[0]
		toName := strings.Split(to, "@")[0]

		if filter == "internal" {
			// Only include edges where at least one side is internal
			fromInternal := strings.HasPrefix(fromName, orgPrefix)
			toInternal := strings.HasPrefix(toName, orgPrefix)
			if !fromInternal && !toInternal {
				continue
			}
		}

		edges = append(edges, edge{from: fromName, to: toName})
		moduleSet[fromName] = true
		moduleSet[toName] = true
	}

	// Collect org modules
	var orgModules []string
	for mod := range moduleSet {
		if strings.HasPrefix(mod, orgPrefix) {
			orgModules = append(orgModules, strings.TrimPrefix(mod, orgPrefix))
		}
	}

	// Generate output
	var graph string
	switch format {
	case "mermaid":
		var sb strings.Builder
		sb.WriteString("graph LR\n")
		// Shorten names for readability
		shortName := func(mod string) string {
			if strings.HasPrefix(mod, orgPrefix) {
				return strings.TrimPrefix(mod, orgPrefix)
			}
			// External: use last two path segments
			parts := strings.Split(mod, "/")
			if len(parts) >= 2 {
				return parts[len(parts)-2] + "/" + parts[len(parts)-1]
			}
			return mod
		}
		seen := make(map[string]bool)
		for _, e := range edges {
			key := e.from + "->" + e.to
			if seen[key] {
				continue
			}
			seen[key] = true
			from := shortName(e.from)
			to := shortName(e.to)
			// Sanitize for Mermaid (replace special chars)
			from = strings.ReplaceAll(strings.ReplaceAll(from, "/", "_"), "-", "_")
			to = strings.ReplaceAll(strings.ReplaceAll(to, "/", "_"), "-", "_")
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", from, to))
		}
		graph = sb.String()

	case "dot":
		var sb strings.Builder
		sb.WriteString("digraph deps {\n")
		sb.WriteString("  rankdir=LR;\n")
		sb.WriteString("  node [shape=box];\n")
		seen := make(map[string]bool)
		for _, e := range edges {
			key := e.from + "->" + e.to
			if seen[key] {
				continue
			}
			seen[key] = true
			sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", e.from, e.to))
		}
		sb.WriteString("}\n")
		graph = sb.String()
	}

	return OpsDepGraphOutput{
		Graph:       graph,
		Format:      format,
		ModuleCount: len(moduleSet),
		EdgeCount:   len(edges),
		OrgModules:  orgModules,
	}, nil
}

// ---------------------------------------------------------------------------
// Wave 7 handlers — Auto-fix
// ---------------------------------------------------------------------------

func opsAutoFix(_ context.Context, input OpsAutoFixInput) (OpsAutoFixOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsAutoFixOutput{}, err
	}

	var patches []Patch
	var remaining []AnalyzedIssue

	for _, issue := range input.Issues {
		patch, fixable := opsGeneratePatch(issue)
		if fixable {
			patches = append(patches, patch)
		} else {
			remaining = append(remaining, issue)
		}
	}

	applied := 0
	if input.Execute {
		needsModTidy := false
		needsGoimports := false
		goimportsDirs := make(map[string]bool)

		for i, p := range patches {
			switch p.Action {
			case "go_mod_tidy":
				needsModTidy = true
				patches[i].Applied = true
				applied++
			case "goimports":
				needsGoimports = true
				if p.File != "" && strings.HasSuffix(p.File, ".go") {
					goimportsDirs[filepath.Dir(filepath.Join(repo, p.File))] = true
				}
				patches[i].Applied = true
				applied++
			case "remove_line":
				if err := opsRemoveLine(repo, p.File, p.Line); err == nil {
					patches[i].Applied = true
					applied++
				}
			case "remove_unused_import":
				needsGoimports = true
				patches[i].Applied = true
				applied++
			}
		}
		if needsModTidy {
			opsRunWithTimeout(30*time.Second, repo, "go", "mod", "tidy")
		}
		if needsGoimports {
			for dir := range goimportsDirs {
				opsRunWithTimeout(15*time.Second, dir, "goimports", "-w", ".")
			}
			if len(goimportsDirs) == 0 {
				opsRunWithTimeout(15*time.Second, repo, "goimports", "-w", ".")
			}
		}
	}

	return OpsAutoFixOutput{
		Repo:            repo,
		PatchCount:      len(patches),
		AppliedCount:    applied,
		Patches:         patches,
		RemainingIssues: remaining,
		DryRun:          !input.Execute,
	}, nil
}

func opsGeneratePatch(issue AnalyzedIssue) (Patch, bool) {
	switch issue.Category {
	case "missing_dep":
		return Patch{
			File: issue.File, Line: issue.Line, Action: "go_mod_tidy",
			Before: issue.Message, After: "go mod tidy",
		}, true
	case "type_error":
		if strings.Contains(issue.Message, "undefined:") || strings.Contains(issue.Message, "undeclared name") {
			return Patch{
				File: issue.File, Line: issue.Line, Action: "goimports",
				Before: issue.Message, After: "goimports -w (auto-add missing imports)",
			}, true
		}
		return Patch{}, false
	case "unused_var":
		return Patch{
			File: issue.File, Line: issue.Line, Action: "remove_line",
			Before: issue.Message, After: "(line removed)",
		}, true
	case "unused_import":
		return Patch{
			File: issue.File, Line: issue.Line, Action: "remove_unused_import",
			Before: issue.Message, After: "goimports -w (remove unused import)",
		}, true
	case "import_cycle", "test_assertion", "timeout":
		return Patch{}, false
	default:
		if issue.Severity == "blocker" && issue.File != "" && strings.HasSuffix(issue.File, ".go") {
			return Patch{
				File: issue.File, Line: issue.Line, Action: "goimports",
				Before: issue.Message, After: "goimports -w (best-effort)",
			}, true
		}
		return Patch{}, false
	}
}

func opsRemoveLine(repo, file string, line int) error {
	if line <= 0 || file == "" {
		return fmt.Errorf("invalid file/line")
	}
	path := filepath.Join(repo, file)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	if line > len(lines) {
		return fmt.Errorf("line %d out of range", line)
	}
	lines = append(lines[:line-1], lines[line:]...)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// ---------------------------------------------------------------------------
// Wave 7 handlers — Fleet Diff
// ---------------------------------------------------------------------------

func opsFleetDiff(_ context.Context, input OpsFleetDiffInput) (OpsFleetDiffOutput, error) {
	dir := input.Dir
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), "hairglasses-studio")
	}
	since := input.Since
	if since == "" {
		since = "7d"
	}
	gitSince := opsResolveSince(since)
	maxRepos := input.MaxRepos
	if maxRepos <= 0 {
		maxRepos = 30
	}
	langFilter := input.Language
	if langFilter == "" {
		langFilter = "all"
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return OpsFleetDiffOutput{}, fmt.Errorf("[%s] cannot read directory: %w", handler.ErrInvalidParam, err)
	}

	var repos []FleetRepoDiff
	totalCommits, totalIns, totalDel := 0, 0, 0
	scanned := 0

	for _, e := range entries {
		if !e.IsDir() || scanned >= maxRepos {
			continue
		}
		repoPath := filepath.Join(dir, e.Name())
		if !sandboxFileExists(filepath.Join(repoPath, ".git")) {
			continue
		}
		if langFilter != "all" && opsDetectLanguage(repoPath) != langFilter {
			continue
		}
		scanned++

		logOut, err := runGit(repoPath, "log", "--oneline", "--since="+gitSince)
		if err != nil || strings.TrimSpace(logOut) == "" {
			continue
		}

		lines := strings.Split(strings.TrimSpace(logOut), "\n")
		commitCount := len(lines)

		// Categorize by conventional prefix
		types := make(map[string]int)
		for _, line := range lines {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) < 2 {
				continue
			}
			prefix := "other"
			for _, p := range []string{"feat", "fix", "chore", "docs", "refactor", "test", "style", "perf", "ci"} {
				if strings.HasPrefix(parts[1], p+":") || strings.HasPrefix(parts[1], p+"(") {
					prefix = p
					break
				}
			}
			types[prefix]++
		}

		// Aggregate insertions/deletions from log --shortstat
		logStatOut, _ := runGit(repoPath, "log", "--shortstat", "--since="+gitSince)
		ins, del := opsAggregateLogShortstat(logStatOut)

		// Authors
		authorsOut, _ := runGit(repoPath, "log", "--format=%aN", "--since="+gitSince)
		authorSet := make(map[string]bool)
		for _, a := range strings.Split(strings.TrimSpace(authorsOut), "\n") {
			if a != "" {
				authorSet[a] = true
			}
		}
		var authors []string
		for a := range authorSet {
			authors = append(authors, a)
		}

		repos = append(repos, FleetRepoDiff{
			Repo: e.Name(), Commits: commitCount,
			Insertions: ins, Deletions: del,
			CommitTypes: types, Authors: authors,
		})
		totalCommits += commitCount
		totalIns += ins
		totalDel += del
	}

	// Sort by most active
	for i := 0; i < len(repos); i++ {
		for j := i + 1; j < len(repos); j++ {
			if repos[j].Commits > repos[i].Commits {
				repos[i], repos[j] = repos[j], repos[i]
			}
		}
	}
	var mostActive []string
	for i, r := range repos {
		if i >= 5 {
			break
		}
		mostActive = append(mostActive, fmt.Sprintf("%s (%d)", r.Repo, r.Commits))
	}

	return OpsFleetDiffOutput{
		Since: gitSince, TotalRepos: scanned, ActiveRepos: len(repos),
		TotalCommits: totalCommits, TotalInsertions: totalIns, TotalDeletions: totalDel,
		MostActive: mostActive, Repos: repos,
	}, nil
}

func opsResolveSince(since string) string {
	if len(since) >= 2 {
		numStr := since[:len(since)-1]
		unit := since[len(since)-1]
		if num, err := strconv.Atoi(numStr); err == nil {
			switch unit {
			case 'd':
				return fmt.Sprintf("%d days ago", num)
			case 'w':
				return fmt.Sprintf("%d weeks ago", num)
			case 'm':
				return fmt.Sprintf("%d months ago", num)
			}
		}
	}
	return since
}

func opsAggregateLogShortstat(out string) (int, int) {
	totalIns, totalDel := 0, 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "changed") {
			continue
		}
		parts := strings.Fields(line)
		for i, p := range parts {
			if strings.HasPrefix(p, "insertion") && i > 0 {
				n, _ := strconv.Atoi(parts[i-1])
				totalIns += n
			}
			if strings.HasPrefix(p, "deletion") && i > 0 {
				n, _ := strconv.Atoi(parts[i-1])
				totalDel += n
			}
		}
	}
	return totalIns, totalDel
}

// ---------------------------------------------------------------------------
// Wave 7 handlers — Tech Debt
// ---------------------------------------------------------------------------

func opsTechDebt(_ context.Context, input OpsTechDebtInput) (OpsTechDebtOutput, error) {
	if input.Repo != "" {
		repo, err := opsResolveRepo(input.Repo)
		if err != nil {
			return OpsTechDebtOutput{}, err
		}
		score := opsScoreRepoDebt(repo, filepath.Base(repo))
		if input.Store {
			opsStoreTechDebt(score)
		}
		return OpsTechDebtOutput{Scores: []TechDebtScore{score}}, nil
	}

	dir := input.Dir
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), "hairglasses-studio")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return OpsTechDebtOutput{}, fmt.Errorf("[%s] cannot read directory: %w", handler.ErrInvalidParam, err)
	}

	var scores []TechDebtScore
	totalScore := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		repoPath := filepath.Join(dir, e.Name())
		if opsDetectLanguage(repoPath) == "unknown" {
			continue
		}
		score := opsScoreRepoDebt(repoPath, e.Name())
		if input.Store {
			opsStoreTechDebt(score)
		}
		scores = append(scores, score)
		totalScore += score.Overall
	}

	// Sort by worst debt (lowest score first)
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].Overall < scores[i].Overall {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	var worstRepos []string
	for i, s := range scores {
		if i >= 5 {
			break
		}
		worstRepos = append(worstRepos, fmt.Sprintf("%s (%d)", s.Repo, s.Overall))
	}

	avg := 0
	if len(scores) > 0 {
		avg = totalScore / len(scores)
	}

	return OpsTechDebtOutput{Scores: scores, FleetAvg: avg, WorstRepos: worstRepos}, nil
}

func opsScoreRepoDebt(repoPath, name string) TechDebtScore {
	dims := make(map[string]int)
	var actions []string

	// 1. Dependency freshness
	depScore := 100
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err == nil {
		stdout, _, exitCode, _ := opsRunWithTimeout(15*time.Second, repoPath, "go", "list", "-m", "-u", "-json", "all")
		if exitCode == 0 && stdout != "" {
			outdated := strings.Count(stdout, `"Update"`)
			total := strings.Count(stdout, `"Path"`)
			if total > 0 {
				depScore = int(100 * (1 - float64(outdated)/float64(total)))
				if depScore < 0 {
					depScore = 0
				}
			}
			if outdated > 5 {
				actions = append(actions, fmt.Sprintf("Update %d outdated Go deps", outdated))
			}
		}
	}
	dims["dep_freshness"] = depScore

	// 2. Test coverage
	testCount, goCount := 0, 0
	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			if info != nil && info.IsDir() && (info.Name() == "vendor" || info.Name() == "node_modules" || info.Name() == ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), "_test.go") {
			testCount++
		} else if strings.HasSuffix(info.Name(), ".go") {
			goCount++
		}
		return nil
	})
	testScore := 0
	if goCount > 0 {
		testScore = int(math.Min(100, float64(testCount)/float64(goCount)*200))
	}
	if testCount == 0 {
		actions = append(actions, "Add tests")
	} else if testScore < 50 {
		actions = append(actions, fmt.Sprintf("Improve test ratio (%d tests / %d sources)", testCount, goCount))
	}
	dims["test_coverage"] = testScore

	// 3. Lint cleanliness
	lintScore := 100
	_, stderr, exitCode, _ := opsRunWithTimeout(15*time.Second, repoPath, "go", "vet", "./...")
	if exitCode != 0 {
		issues := strings.Count(stderr, "\n")
		lintScore = int(math.Max(0, float64(100-issues*10)))
		actions = append(actions, fmt.Sprintf("Fix %d vet issues", issues))
	}
	dims["lint_clean"] = lintScore

	// 4. CI health
	ciScore := 0
	switch {
	case sandboxFileExists(filepath.Join(repoPath, ".github", "workflows")):
		ciScore = 100
	case sandboxFileExists(filepath.Join(repoPath, "Makefile")):
		ciScore = 50
		actions = append(actions, "Add CI workflow")
	default:
		actions = append(actions, "Add CI/CD pipeline")
	}
	dims["ci_health"] = ciScore

	// 5. Documentation
	docScore := 0
	if sandboxFileExists(filepath.Join(repoPath, "README.md")) {
		info, err := os.Stat(filepath.Join(repoPath, "README.md"))
		if err == nil && info.Size() > 200 {
			docScore += 40
		} else {
			docScore += 20
			actions = append(actions, "Expand README.md")
		}
	} else {
		actions = append(actions, "Create README.md")
	}
	if sandboxFileExists(filepath.Join(repoPath, "CLAUDE.md")) {
		docScore += 30
	}
	if sandboxFileExists(filepath.Join(repoPath, "LICENSE")) {
		docScore += 30
	} else {
		actions = append(actions, "Add LICENSE")
	}
	dims["documentation"] = docScore

	// 6. Code age
	ageScore := 50
	totalOut, _ := runGit(repoPath, "rev-list", "--count", "HEAD")
	recentOut, _ := runGit(repoPath, "rev-list", "--count", "--since=6 months ago", "HEAD")
	total, _ := strconv.Atoi(strings.TrimSpace(totalOut))
	recent, _ := strconv.Atoi(strings.TrimSpace(recentOut))
	if total > 0 {
		ageScore = int(math.Min(100, float64(recent)/float64(total)*100))
	}
	if recent == 0 {
		actions = append(actions, "Stale: no commits in 6 months")
	}
	dims["code_age"] = ageScore

	overall := (depScore*20 + testScore*25 + lintScore*15 + ciScore*15 + docScore*15 + ageScore*10) / 100
	trend := opsTechDebtTrend(name)

	return TechDebtScore{Repo: name, Overall: overall, Dimensions: dims, ActionItems: actions, Trend: trend}
}

func opsStoreTechDebt(score TechDebtScore) {
	dir := filepath.Join(opsStateDir(), "tech-debt")
	os.MkdirAll(dir, 0o755)
	entry := struct {
		TechDebtScore
		Timestamp int64 `json:"timestamp"`
	}{score, time.Now().Unix()}
	data, _ := json.Marshal(entry)
	f, err := os.OpenFile(filepath.Join(dir, score.Repo+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(data, '\n'))
}

func opsTechDebtTrend(repo string) string {
	path := filepath.Join(opsStateDir(), "tech-debt", repo+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var scores []int
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		var entry struct {
			Overall int `json:"overall"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) == nil {
			scores = append(scores, entry.Overall)
		}
	}
	if len(scores) < 2 {
		return ""
	}
	prev := scores[len(scores)-2]
	curr := scores[len(scores)-1]
	if curr > prev+5 {
		return "improving"
	} else if curr < prev-5 {
		return "degrading"
	}
	return "stable"
}

// ---------------------------------------------------------------------------
// Wave 7 handlers — Research Check
// ---------------------------------------------------------------------------

func opsResearchCheck(_ context.Context, input OpsResearchCheckInput) (OpsResearchCheckOutput, error) {
	docsPath := input.DocsPath
	if docsPath == "" {
		docsPath = filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "docs")
	}
	maxResults := input.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}
	query := strings.ToLower(input.Query)
	queryTerms := strings.Fields(query)
	if len(queryTerms) == 0 {
		return OpsResearchCheckOutput{}, fmt.Errorf("[%s] query is required", handler.ErrInvalidParam)
	}

	var allDocs []ResearchMatch
	totalDocs := 0

	filepath.Walk(docsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			if info != nil && info.IsDir() && (info.Name() == ".git" || info.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		totalDocs++

		relPath, _ := filepath.Rel(docsPath, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		if len(content) > 2048 {
			content = content[:2048]
		}
		lower := strings.ToLower(content)

		// Extract title
		title := info.Name()
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "# ") {
				title = strings.TrimPrefix(line, "# ")
				break
			}
		}

		// Score relevance
		score := 0.0
		for _, term := range queryTerms {
			if strings.Contains(strings.ToLower(title), term) {
				score += 3.0
			}
			if strings.Contains(strings.ToLower(relPath), term) {
				score += 2.0
			}
			count := strings.Count(lower, term)
			if count > 0 {
				score += math.Min(float64(count)*0.5, 5.0)
			}
		}

		if score > 0 {
			excerpt := ""
			for _, line := range strings.Split(content, "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "---") && len(line) > 20 {
					excerpt = line
					if len(excerpt) > 200 {
						excerpt = excerpt[:200] + "..."
					}
					break
				}
			}
			var tags []string
			for _, part := range strings.Split(filepath.Dir(relPath), string(filepath.Separator)) {
				if part != "." && part != "" {
					tags = append(tags, part)
				}
			}
			allDocs = append(allDocs, ResearchMatch{
				Path: relPath, Title: title, Relevance: score, Excerpt: excerpt, Tags: tags,
			})
		}
		return nil
	})

	// Sort by relevance desc
	for i := 0; i < len(allDocs); i++ {
		for j := i + 1; j < len(allDocs); j++ {
			if allDocs[j].Relevance > allDocs[i].Relevance {
				allDocs[i], allDocs[j] = allDocs[j], allDocs[i]
			}
		}
	}
	results := allDocs
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	// Gap detection
	knownDomains := []string{"mcp", "agents", "orchestration", "cost-optimization", "go-ecosystem", "terminal", "competitive"}
	var gaps []string
	for _, term := range queryTerms {
		found := false
		for _, doc := range results {
			if strings.Contains(strings.ToLower(doc.Path), term) || strings.Contains(strings.ToLower(doc.Title), term) {
				found = true
				break
			}
		}
		if !found {
			gaps = append(gaps, term)
		}
	}

	suggestion := ""
	if len(gaps) > 0 {
		domain := "research"
		for _, d := range knownDomains {
			for _, g := range gaps {
				if strings.Contains(d, g) || strings.Contains(g, d) {
					domain = "research/" + d
				}
			}
		}
		suggestion = fmt.Sprintf("No docs found for %v — consider adding to docs/%s/", gaps, domain)
	}

	return OpsResearchCheckOutput{
		Query: input.Query, Results: results, TotalDocs: totalDocs,
		Gaps: gaps, Suggestion: suggestion,
	}, nil
}

// ---------------------------------------------------------------------------
// Wave 7 handlers — Session Handoff
// ---------------------------------------------------------------------------

func opsSessionHandoff(_ context.Context, input OpsSessionHandoffInput) (OpsSessionHandoffOutput, error) {
	repo, err := opsResolveRepo(input.Repo)
	if err != nil {
		return OpsSessionHandoffOutput{}, err
	}

	sessionID := input.SessionID
	if sessionID == "" {
		ids := opsListSessionIDs()
		var newest string
		var newestTime time.Time
		for _, id := range ids {
			info, err := os.Stat(filepath.Join(opsSessionDir(id), "session.json"))
			if err == nil && info.ModTime().After(newestTime) {
				newestTime = info.ModTime()
				newest = id
			}
		}
		sessionID = newest
	}

	branch := opsCurrentBranch(repo)
	head, _ := runGit(repo, "rev-parse", "--short", "HEAD")
	head = strings.TrimSpace(head)

	statusOut, _ := runGit(repo, "status", "--porcelain")
	var dirtyFiles []string
	for _, line := range strings.Split(strings.TrimSpace(statusOut), "\n") {
		if line != "" {
			dirtyFiles = append(dirtyFiles, strings.TrimSpace(line))
		}
	}

	unpushedOut, _ := runGit(repo, "log", "--oneline", "@{upstream}..HEAD")
	var unpushed []string
	for _, line := range strings.Split(strings.TrimSpace(unpushedOut), "\n") {
		if line != "" {
			unpushed = append(unpushed, line)
		}
	}

	var sb strings.Builder
	sb.WriteString("# Agent Handoff Protocol\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Repo:** %s\n", filepath.Base(repo)))
	sb.WriteString(fmt.Sprintf("**Branch:** %s @ %s\n\n", branch, head))

	if sessionID != "" {
		session, err := opsReadSession(sessionID)
		if err == nil {
			sb.WriteString("## Session State\n\n")
			sb.WriteString(fmt.Sprintf("- **Session ID:** %s\n", session.ID))
			sb.WriteString(fmt.Sprintf("- **Description:** %s\n", session.Description))
			sb.WriteString(fmt.Sprintf("- **Created:** %s\n", session.CreatedAt.Format(time.RFC3339)))
			sb.WriteString(fmt.Sprintf("- **Iterations:** %d\n", len(session.Iterations)))
			if len(session.Iterations) > 0 {
				last := session.Iterations[len(session.Iterations)-1]
				sb.WriteString(fmt.Sprintf("- **Last Status:** %s\n", last.Status))
				sb.WriteString(fmt.Sprintf("- **Last Error Count:** %d\n", last.ErrorCount))
				if len(session.Iterations) >= 2 {
					sb.WriteString("- **Error Trend:** ")
					for i, iter := range session.Iterations {
						if i > 0 {
							sb.WriteString(" -> ")
						}
						sb.WriteString(fmt.Sprintf("%d", iter.ErrorCount))
					}
					sb.WriteString("\n")
				}
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("## Sources of Truth\n\n")
	sb.WriteString(fmt.Sprintf("- Git: `%s` branch `%s`\n", filepath.Base(repo), branch))
	if sandboxFileExists(filepath.Join(repo, "CLAUDE.md")) {
		sb.WriteString("- CLAUDE.md: project conventions\n")
	}
	if sandboxFileExists(filepath.Join(repo, "go.mod")) {
		sb.WriteString("- go.mod: dependency versions\n")
	}
	sb.WriteString("\n")

	sb.WriteString("## Current State\n\n")
	if len(dirtyFiles) > 0 {
		sb.WriteString(fmt.Sprintf("**Dirty files (%d):**\n", len(dirtyFiles)))
		for _, f := range dirtyFiles {
			sb.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
	} else {
		sb.WriteString("Working tree is clean.\n")
	}
	sb.WriteString("\n")

	if len(unpushed) > 0 {
		sb.WriteString(fmt.Sprintf("**Unpushed commits (%d):**\n", len(unpushed)))
		for _, c := range unpushed {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Receiving Agent Instructions\n\n")
	sb.WriteString("1. Read CLAUDE.md for project conventions\n")
	sb.WriteString("2. Run `ops_session_status` to check iteration state\n")
	sb.WriteString("3. Run `ops_iterate` to continue the build/test loop\n")
	if len(dirtyFiles) > 0 {
		sb.WriteString("4. Review dirty files before continuing\n")
	}
	if len(unpushed) > 0 {
		sb.WriteString("5. Push unpushed commits or continue iterating\n")
	}

	handoff := sb.String()
	writtenPath := ""
	if input.Write {
		writtenPath = filepath.Join(repo, "HANDOFF.md")
		if err := os.WriteFile(writtenPath, []byte(handoff), 0o644); err != nil {
			writtenPath = ""
		}
	}

	return OpsSessionHandoffOutput{
		Handoff: handoff, SessionID: sessionID,
		Repo: filepath.Base(repo), Branch: branch, WrittenPath: writtenPath,
	}, nil
}

// ---------------------------------------------------------------------------
// Wave 7 handlers — Iteration Patterns
// ---------------------------------------------------------------------------

func opsIterationPatterns(_ context.Context, input OpsIterationPatternsInput) (OpsIterationPatternsOutput, error) {
	window := input.Window
	if window == "" {
		window = "30d"
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	if len(window) >= 2 {
		numStr := window[:len(window)-1]
		unit := window[len(window)-1]
		if num, err := strconv.Atoi(numStr); err == nil {
			switch unit {
			case 'd':
				cutoff = time.Now().Add(-time.Duration(num) * 24 * time.Hour)
			case 'w':
				cutoff = time.Now().Add(-time.Duration(num) * 7 * 24 * time.Hour)
			case 'm':
				cutoff = time.Now().Add(-time.Duration(num) * 30 * 24 * time.Hour)
			}
		}
	}

	ids := opsListSessionIDs()
	totalSessions, totalIterations, converged := 0, 0, 0
	errorCounts := make(map[string]int)
	repoCounts := make(map[string]int)

	for _, id := range ids {
		session, err := opsReadSession(id)
		if err != nil || session.CreatedAt.Before(cutoff) {
			continue
		}
		if input.Repo != "" && !strings.Contains(session.Repo, input.Repo) {
			continue
		}

		totalSessions++
		iterations := opsReadIterations(id)
		totalIterations += len(iterations)

		if len(iterations) > 0 {
			last := iterations[len(iterations)-1]
			if last.ErrorCount == 0 && last.BuildOK {
				converged++
			}
		}
		for _, iter := range iterations {
			if iter.Status != "" {
				errorCounts[iter.Status]++
			}
		}
		if session.Repo != "" {
			repoCounts[filepath.Base(session.Repo)]++
		}
	}

	var hotFiles []HotFile
	for file, count := range repoCounts {
		if count >= 2 {
			hotFiles = append(hotFiles, HotFile{File: file, Appearances: count})
		}
	}
	for i := 0; i < len(hotFiles); i++ {
		for j := i + 1; j < len(hotFiles); j++ {
			if hotFiles[j].Appearances > hotFiles[i].Appearances {
				hotFiles[i], hotFiles[j] = hotFiles[j], hotFiles[i]
			}
		}
	}
	if len(hotFiles) > 10 {
		hotFiles = hotFiles[:10]
	}

	avgIter, convergenceRate := 0.0, 0.0
	if totalSessions > 0 {
		avgIter = float64(totalIterations) / float64(totalSessions)
		convergenceRate = float64(converged) / float64(totalSessions) * 100
	}

	var recs []string
	if avgIter > 5 {
		recs = append(recs, "Avg iterations >5 — improve auto-fix for common failures")
	}
	if convergenceRate < 50 && totalSessions > 0 {
		recs = append(recs, fmt.Sprintf("Convergence rate %.0f%% — many sessions don't reach green", convergenceRate))
	}
	if errorCounts["build_fail"] > errorCounts["test_fail"]*2 {
		recs = append(recs, "Build failures dominate — add stricter linting or pre-commit hooks")
	}
	if len(recs) == 0 {
		recs = append(recs, "Iteration health looks good")
	}

	return OpsIterationPatternsOutput{
		TotalSessions: totalSessions, TotalIterations: totalIterations,
		AvgIterations:   math.Round(avgIter*10) / 10,
		ConvergenceRate: math.Round(convergenceRate*10) / 10,
		CommonErrors:    errorCounts, HotFiles: hotFiles, Recommendations: recs,
	}, nil
}
