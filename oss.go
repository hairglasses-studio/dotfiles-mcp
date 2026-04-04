package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// OSS Module — open-source readiness scoring
// ---------------------------------------------------------------------------

type OSSModule struct{}

func (m *OSSModule) Name() string        { return "oss" }
func (m *OSSModule) Description() string { return "Open-source readiness scoring for repositories" }

func (m *OSSModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[OSSScoreInput, OSSScoreOutput](
			"dotfiles_oss_score",
			"Score a repository's open-source readiness (0-100) across 8 categories based on GitHub best practices, OpenSSF Scorecard criteria, and Go module conventions",
			func(_ context.Context, input OSSScoreInput) (OSSScoreOutput, error) {
				if input.RepoPath == "" {
					return OSSScoreOutput{}, fmt.Errorf("[%s] repo_path is required", handler.ErrInvalidParam)
				}
				info, err := os.Stat(input.RepoPath)
				if err != nil || !info.IsDir() {
					return OSSScoreOutput{}, fmt.Errorf("[%s] repo_path %q is not a valid directory", handler.ErrInvalidParam, input.RepoPath)
				}
				return runFullScore(input.RepoPath, input.SkipTests), nil
			},
		),
		handler.TypedHandler[OSSCheckInput, OSSCategoryResult](
			"dotfiles_oss_check",
			"Run open-source readiness checks for a single category (community, readme, gomod, testing, cicd, security, release, maintenance)",
			func(_ context.Context, input OSSCheckInput) (OSSCategoryResult, error) {
				if input.RepoPath == "" {
					return OSSCategoryResult{}, fmt.Errorf("[%s] repo_path is required", handler.ErrInvalidParam)
				}
				info, err := os.Stat(input.RepoPath)
				if err != nil || !info.IsDir() {
					return OSSCategoryResult{}, fmt.Errorf("[%s] repo_path %q is not a valid directory", handler.ErrInvalidParam, input.RepoPath)
				}
				return runCategory(input.RepoPath, input.Category, input.SkipTests), nil
			},
		),
	}
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type OSSScoreInput struct {
	RepoPath  string `json:"repo_path" jsonschema:"required,description=Absolute path to the repository to score"`
	SkipTests bool   `json:"skip_tests,omitempty" jsonschema:"description=Skip go test/vet execution (faster but incomplete scoring)"`
}

type OSSCheckInput struct {
	RepoPath  string `json:"repo_path" jsonschema:"required,description=Absolute path to the repository"`
	Category  string `json:"category" jsonschema:"required,description=Category to check,enum=community,enum=readme,enum=gomod,enum=testing,enum=cicd,enum=security,enum=release,enum=maintenance"`
	SkipTests bool   `json:"skip_tests,omitempty" jsonschema:"description=Skip go test/vet execution"`
}

type CheckResult struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	Points     int    `json:"points"`
	MaxPoints  int    `json:"max_points"`
	Detail     string `json:"detail,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

type OSSCategoryResult struct {
	Category string        `json:"category"`
	Score    int           `json:"score"`
	MaxScore int           `json:"max_score"`
	Checks   []CheckResult `json:"checks"`
}

type OSSScoreOutput struct {
	RepoPath   string              `json:"repo_path"`
	RepoName   string              `json:"repo_name"`
	TotalScore int                 `json:"total_score"`
	MaxScore   int                 `json:"max_score"`
	Grade      string              `json:"grade"`
	Categories []OSSCategoryResult `json:"categories"`
	TopActions []string            `json:"top_actions"`
}

// ---------------------------------------------------------------------------
// Scoring engine
// ---------------------------------------------------------------------------

func runFullScore(repoPath string, skipTests bool) OSSScoreOutput {
	cats := []string{"community", "readme", "gomod", "testing", "cicd", "security", "release", "maintenance"}
	var categories []OSSCategoryResult
	total, maxTotal := 0, 0

	for _, cat := range cats {
		r := runCategory(repoPath, cat, skipTests)
		total += r.Score
		maxTotal += r.MaxScore
		categories = append(categories, r)
	}

	// Collect top action items sorted by recoverable points
	type action struct {
		points int
		text   string
	}
	var actions []action
	for _, cat := range categories {
		for _, ch := range cat.Checks {
			if !ch.Passed && ch.Suggestion != "" {
				actions = append(actions, action{ch.MaxPoints - ch.Points, ch.Suggestion})
			}
		}
	}
	// Simple sort by points descending
	for i := 0; i < len(actions); i++ {
		for j := i + 1; j < len(actions); j++ {
			if actions[j].points > actions[i].points {
				actions[i], actions[j] = actions[j], actions[i]
			}
		}
	}
	var topActions []string
	for i, a := range actions {
		if i >= 10 {
			break
		}
		topActions = append(topActions, fmt.Sprintf("[+%d pts] %s", a.points, a.text))
	}

	return OSSScoreOutput{
		RepoPath:   repoPath,
		RepoName:   filepath.Base(repoPath),
		TotalScore: total,
		MaxScore:   maxTotal,
		Grade:      letterGrade(total, maxTotal),
		Categories: categories,
		TopActions: topActions,
	}
}

func letterGrade(score, max int) string {
	if max == 0 {
		return "F"
	}
	pct := float64(score) / float64(max) * 100
	switch {
	case pct >= 90:
		return "A"
	case pct >= 80:
		return "B"
	case pct >= 70:
		return "C"
	case pct >= 60:
		return "D"
	default:
		return "F"
	}
}

func runCategory(repoPath, category string, skipTests bool) OSSCategoryResult {
	var checks []CheckResult
	switch category {
	case "community":
		checks = checkCommunity(repoPath)
	case "readme":
		checks = checkReadme(repoPath)
	case "gomod":
		checks = checkGoMod(repoPath)
	case "testing":
		checks = checkTesting(repoPath, skipTests)
	case "cicd":
		checks = checkCICD(repoPath)
	case "security":
		checks = checkSecurity(repoPath)
	case "release":
		checks = checkRelease(repoPath)
	case "maintenance":
		checks = checkMaintenance(repoPath)
	default:
		return OSSCategoryResult{Category: category, Checks: []CheckResult{{Name: "unknown", Detail: "unknown category"}}}
	}
	score, maxScore := 0, 0
	for _, c := range checks {
		score += c.Points
		maxScore += c.MaxPoints
	}
	return OSSCategoryResult{Category: category, Score: score, MaxScore: maxScore, Checks: checks}
}

// ---------------------------------------------------------------------------
// Category: Community Files (20 pts)
// ---------------------------------------------------------------------------

func checkCommunity(repo string) []CheckResult {
	return []CheckResult{
		fileCheck(repo, "README.md", 5, "Add a README.md describing the project, installation, and usage"),
		fileCheckMulti(repo, []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "COPYING"}, "LICENSE", 5, "Add an MIT LICENSE file"),
		fileCheckMulti(repo, []string{"CONTRIBUTING.md", ".github/CONTRIBUTING.md", "docs/CONTRIBUTING.md"}, "CONTRIBUTING.md", 3, "Add a CONTRIBUTING.md with development setup and PR process"),
		fileCheckMulti(repo, []string{"CODE_OF_CONDUCT.md", ".github/CODE_OF_CONDUCT.md"}, "CODE_OF_CONDUCT.md", 2, "Add a CODE_OF_CONDUCT.md (Contributor Covenant is standard)"),
		fileCheckMulti(repo, []string{"SECURITY.md", ".github/SECURITY.md"}, "SECURITY.md", 3, "Add a SECURITY.md with vulnerability reporting instructions"),
		fileCheckMulti(repo, []string{"CHANGELOG.md", "CHANGES.md", "HISTORY.md"}, "CHANGELOG.md", 2, "Add a CHANGELOG.md tracking releases"),
	}
}

// ---------------------------------------------------------------------------
// Category: README Quality (15 pts)
// ---------------------------------------------------------------------------

func checkReadme(repo string) []CheckResult {
	content, err := os.ReadFile(filepath.Join(repo, "README.md"))
	if err != nil {
		return []CheckResult{{Name: "readme_exists", MaxPoints: 15, Detail: "README.md not found"}}
	}
	text := string(content)
	lower := strings.ToLower(text)

	return []CheckResult{
		contentCheck("badges", text, 2,
			func(s string) bool { return strings.Contains(s, "![") || strings.Contains(s, "shields.io") || strings.Contains(s, "badge") },
			"Present", "Add shields.io badges (build status, coverage, Go version, license)"),
		contentCheck("install_instructions", lower, 3,
			func(s string) bool {
				return strings.Contains(s, "go install") || strings.Contains(s, "go get") || strings.Contains(s, "## install") || strings.Contains(s, "## getting started") || strings.Contains(s, "## quick start")
			},
			"Present", "Add installation instructions (go install, go get, or build from source)"),
		contentCheck("usage_examples", lower, 3,
			func(s string) bool {
				return strings.Contains(s, "## usage") || strings.Contains(s, "## example") || strings.Contains(s, "```go") || strings.Contains(s, "```bash") || strings.Contains(s, "```shell")
			},
			"Present", "Add usage examples with code blocks"),
		contentCheck("description_length", text, 2,
			func(s string) bool {
				lines := strings.SplitN(s, "\n", 10)
				descLen := 0
				for _, l := range lines {
					l = strings.TrimSpace(l)
					if l != "" && !strings.HasPrefix(l, "#") && !strings.HasPrefix(l, "![") {
						descLen += len(l)
					}
				}
				return descLen > 50
			},
			"Adequate", "Expand the project description (aim for 50+ characters in the first paragraph)"),
		contentCheck("api_docs", lower, 3,
			func(s string) bool {
				return strings.Contains(s, "## tool") || strings.Contains(s, "## api") || strings.Contains(s, "## package") || strings.Contains(s, "## feature") || strings.Contains(s, "| tool") || strings.Contains(s, "| feature")
			},
			"Present", "Add API/tool documentation (table of features, function reference, or tool list)"),
		contentCheck("license_section", lower, 2,
			func(s string) bool {
				return strings.Contains(s, "## license") || strings.Contains(s, "licensed under") || strings.Contains(s, "mit license") || strings.Contains(s, "apache")
			},
			"Present", "Add a License section at the bottom of README"),
	}
}

// ---------------------------------------------------------------------------
// Category: Go Module (15 pts)
// ---------------------------------------------------------------------------

func checkGoMod(repo string) []CheckResult {
	gomodPath := filepath.Join(repo, "go.mod")
	gomodBytes, err := os.ReadFile(gomodPath)
	if err != nil {
		return []CheckResult{{Name: "go.mod", MaxPoints: 15, Detail: "go.mod not found — not a Go module (skipping Go checks)", Suggestion: "Initialize a Go module with go mod init"}}
	}
	gomod := string(gomodBytes)

	var results []CheckResult

	// go.mod exists
	results = append(results, CheckResult{Name: "gomod_exists", Passed: true, Points: 3, MaxPoints: 3, Detail: "go.mod present"})

	// No replace directives
	hasReplace := strings.Contains(gomod, "\nreplace ")
	results = append(results, CheckResult{
		Name: "no_replace", Passed: !hasReplace, Points: boolInt(!hasReplace, 3), MaxPoints: 3,
		Detail: ternary(hasReplace, "replace directive found", "No replace directives"),
		Suggestion: ternary(hasReplace, "Remove replace directives and point to published module versions", ""),
	})

	// go.sum present
	_, sumErr := os.Stat(filepath.Join(repo, "go.sum"))
	hasSUM := sumErr == nil
	results = append(results, CheckResult{
		Name: "gosum_present", Passed: hasSUM, Points: boolInt(hasSUM, 2), MaxPoints: 2,
		Detail: ternary(hasSUM, "go.sum present", "go.sum missing"),
		Suggestion: ternary(hasSUM, "", "Run go mod tidy to generate go.sum"),
	})

	// Go version >= 1.21
	goVer := extractGoVersion(gomod)
	goodVer := goVer >= "1.21"
	results = append(results, CheckResult{
		Name: "go_version", Passed: goodVer, Points: boolInt(goodVer, 2), MaxPoints: 2,
		Detail: fmt.Sprintf("Go version: %s", goVer),
		Suggestion: ternary(goodVer, "", "Update to Go 1.21+ for improved module support"),
	})

	// No private imports (check for non-github.com or internal paths in require)
	hasPrivate := false
	for _, line := range strings.Split(gomod, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require") || strings.HasPrefix(line, ")") || line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "module") || strings.HasPrefix(line, "go ") || strings.HasPrefix(line, "replace") || strings.HasPrefix(line, "exclude") || strings.HasPrefix(line, "retract") {
			continue
		}
		// Check for non-standard module paths that might be private
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			mod := parts[0]
			if !strings.Contains(mod, ".") {
				hasPrivate = true
			}
		}
	}
	results = append(results, CheckResult{
		Name: "no_private_imports", Passed: !hasPrivate, Points: boolInt(!hasPrivate, 3), MaxPoints: 3,
		Detail: ternary(hasPrivate, "Possible private module imports detected", "All imports appear public"),
		Suggestion: ternary(hasPrivate, "Ensure all dependencies are publicly accessible", ""),
	})

	// go vet passes
	results = append(results, goVetCheck(repo, 2)...)

	return results
}

// ---------------------------------------------------------------------------
// Category: Testing (15 pts)
// ---------------------------------------------------------------------------

func checkTesting(repo string, skipTests bool) []CheckResult {
	var results []CheckResult

	// Has test files
	testFiles := countFiles(repo, "*_test.go")
	hasTests := testFiles > 0
	results = append(results, CheckResult{
		Name: "has_tests", Passed: hasTests, Points: boolInt(hasTests, 5), MaxPoints: 5,
		Detail: fmt.Sprintf("%d test files found", testFiles),
		Suggestion: ternary(hasTests, "", "Add _test.go files with unit tests"),
	})

	if skipTests || !fileExists(filepath.Join(repo, "go.mod")) {
		// Can't run Go tests without go.mod or when skipped
		results = append(results, CheckResult{Name: "coverage", MaxPoints: 5, Detail: "Skipped (skip_tests=true or not a Go module)", Suggestion: "Run with skip_tests=false for coverage scoring"})
		results = append(results, CheckResult{Name: "race_detection", MaxPoints: 3, Detail: "Skipped"})
		results = append(results, CheckResult{Name: "benchmarks", Passed: countFiles(repo, "*_test.go") > 0 && grepFiles(repo, "func Benchmark"), MaxPoints: 2, Points: boolInt(grepFiles(repo, "func Benchmark"), 2), Detail: ternary(grepFiles(repo, "func Benchmark"), "Benchmark functions found", "No benchmarks"), Suggestion: ternary(grepFiles(repo, "func Benchmark"), "", "Add Benchmark functions for performance-critical paths")})
		return results
	}

	// Coverage check (run go test)
	covPct := goTestCoverage(repo)
	goodCov := covPct >= 80.0
	results = append(results, CheckResult{
		Name: "coverage", Passed: goodCov, Points: boolInt(goodCov, 5), MaxPoints: 5,
		Detail: fmt.Sprintf("%.1f%% coverage", covPct),
		Suggestion: ternary(goodCov, "", fmt.Sprintf("Increase test coverage to 80%%+ (currently %.1f%%)", covPct)),
	})

	// Race detection
	raceOK := goTestRace(repo)
	results = append(results, CheckResult{
		Name: "race_detection", Passed: raceOK, Points: boolInt(raceOK, 3), MaxPoints: 3,
		Detail: ternary(raceOK, "Tests pass with -race", "Race detection failed or timed out"),
		Suggestion: ternary(raceOK, "", "Fix data races detected by go test -race"),
	})

	// Benchmarks
	hasBench := grepFiles(repo, "func Benchmark")
	results = append(results, CheckResult{
		Name: "benchmarks", Passed: hasBench, Points: boolInt(hasBench, 2), MaxPoints: 2,
		Detail: ternary(hasBench, "Benchmark functions found", "No benchmarks"),
		Suggestion: ternary(hasBench, "", "Add Benchmark functions for performance-critical paths"),
	})

	return results
}

// ---------------------------------------------------------------------------
// Category: CI/CD (10 pts)
// ---------------------------------------------------------------------------

func checkCICD(repo string) []CheckResult {
	workflowDir := filepath.Join(repo, ".github", "workflows")
	entries, _ := os.ReadDir(workflowDir)
	hasWorkflows := len(entries) > 0

	var results []CheckResult
	results = append(results, CheckResult{
		Name: "has_workflows", Passed: hasWorkflows, Points: boolInt(hasWorkflows, 3), MaxPoints: 3,
		Detail: fmt.Sprintf("%d workflow files in .github/workflows/", len(entries)),
		Suggestion: ternary(hasWorkflows, "", "Add GitHub Actions CI workflow (.github/workflows/ci.yml)"),
	})

	if !hasWorkflows {
		results = append(results, CheckResult{Name: "runs_tests", MaxPoints: 3, Suggestion: "CI should run go test"})
		results = append(results, CheckResult{Name: "runs_lint", MaxPoints: 2, Suggestion: "CI should run golangci-lint or go vet"})
		results = append(results, CheckResult{Name: "coverage_reporting", MaxPoints: 2, Suggestion: "CI should report coverage to Codecov or Coveralls"})
		return results
	}

	// Scan workflow content
	allContent := ""
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".yml") || strings.HasSuffix(e.Name(), ".yaml") {
			b, _ := os.ReadFile(filepath.Join(workflowDir, e.Name()))
			allContent += string(b) + "\n"
		}
	}
	lower := strings.ToLower(allContent)

	runsTests := strings.Contains(lower, "go test") || strings.Contains(lower, "make test")
	results = append(results, CheckResult{
		Name: "runs_tests", Passed: runsTests, Points: boolInt(runsTests, 3), MaxPoints: 3,
		Detail: ternary(runsTests, "CI runs tests", "No test step found in workflows"),
		Suggestion: ternary(runsTests, "", "Add 'go test ./...' step to CI workflow"),
	})

	runsLint := strings.Contains(lower, "golangci") || strings.Contains(lower, "go vet") || strings.Contains(lower, "staticcheck") || strings.Contains(lower, "make lint")
	results = append(results, CheckResult{
		Name: "runs_lint", Passed: runsLint, Points: boolInt(runsLint, 2), MaxPoints: 2,
		Detail: ternary(runsLint, "CI runs linting", "No lint step found"),
		Suggestion: ternary(runsLint, "", "Add golangci-lint or go vet step to CI"),
	})

	hasCoverage := strings.Contains(lower, "coverprofile") || strings.Contains(lower, "codecov") || strings.Contains(lower, "coveralls") || strings.Contains(lower, "coverage")
	results = append(results, CheckResult{
		Name: "coverage_reporting", Passed: hasCoverage, Points: boolInt(hasCoverage, 2), MaxPoints: 2,
		Detail: ternary(hasCoverage, "Coverage reporting configured", "No coverage reporting"),
		Suggestion: ternary(hasCoverage, "", "Add coverage reporting (codecov-action or coveralls)"),
	})

	return results
}

// ---------------------------------------------------------------------------
// Category: Security (10 pts)
// ---------------------------------------------------------------------------

func checkSecurity(repo string) []CheckResult {
	var results []CheckResult

	// .gitignore covers .env
	gitignoreContent, _ := os.ReadFile(filepath.Join(repo, ".gitignore"))
	coversEnv := strings.Contains(string(gitignoreContent), ".env")
	results = append(results, CheckResult{
		Name: "gitignore_env", Passed: coversEnv, Points: boolInt(coversEnv, 2), MaxPoints: 2,
		Detail: ternary(coversEnv, ".gitignore covers .env", ".env not in .gitignore"),
		Suggestion: ternary(coversEnv, "", "Add .env to .gitignore to prevent accidental secret commits"),
	})

	// No hardcoded secrets
	secretPatterns := []string{"sk-ant-api", "sk-svcacct", "AIzaSy", "ghp_", "gho_", "github_pat_", "AKIA", "password\\s*=\\s*[\"'][^\"']+[\"']"}
	hasSecrets := false
	secretDetail := "No hardcoded secrets detected"
	for _, pat := range secretPatterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			continue
		}
		if scanFilesForPattern(repo, re) {
			hasSecrets = true
			secretDetail = fmt.Sprintf("Potential secret pattern matched: %s", pat)
			break
		}
	}
	results = append(results, CheckResult{
		Name: "no_hardcoded_secrets", Passed: !hasSecrets, Points: boolInt(!hasSecrets, 3), MaxPoints: 3,
		Detail: secretDetail,
		Suggestion: ternary(hasSecrets, "Remove hardcoded secrets and use environment variables", ""),
	})

	// SECURITY.md
	hasSecurity := fileExistsMulti(repo, []string{"SECURITY.md", ".github/SECURITY.md"})
	results = append(results, CheckResult{
		Name: "security_policy", Passed: hasSecurity, Points: boolInt(hasSecurity, 2), MaxPoints: 2,
		Detail: ternary(hasSecurity, "SECURITY.md present", "No security policy"),
		Suggestion: ternary(hasSecurity, "", "Add SECURITY.md with vulnerability reporting instructions"),
	})

	// Dependabot
	hasDependabot := fileExists(filepath.Join(repo, ".github", "dependabot.yml")) || fileExists(filepath.Join(repo, ".github", "dependabot.yaml"))
	results = append(results, CheckResult{
		Name: "dependabot", Passed: hasDependabot, Points: boolInt(hasDependabot, 3), MaxPoints: 3,
		Detail: ternary(hasDependabot, "Dependabot configured", "No Dependabot config"),
		Suggestion: ternary(hasDependabot, "", "Add .github/dependabot.yml for automated dependency updates"),
	})

	return results
}

// ---------------------------------------------------------------------------
// Category: Release (10 pts)
// ---------------------------------------------------------------------------

func checkRelease(repo string) []CheckResult {
	var results []CheckResult

	// Semver tags
	hasTags := false
	cmd := exec.Command("git", "tag", "-l", "v*")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err == nil {
		tags := strings.TrimSpace(string(out))
		hasTags = tags != ""
	}
	results = append(results, CheckResult{
		Name: "semver_tags", Passed: hasTags, Points: boolInt(hasTags, 3), MaxPoints: 3,
		Detail: ternary(hasTags, "Semver tags found", "No version tags (v*.*.*)"),
		Suggestion: ternary(hasTags, "", "Tag releases with semantic versions (git tag v0.1.0)"),
	})

	// CHANGELOG
	hasChangelog := fileExistsMulti(repo, []string{"CHANGELOG.md", "CHANGES.md", "HISTORY.md"})
	results = append(results, CheckResult{
		Name: "changelog", Passed: hasChangelog, Points: boolInt(hasChangelog, 2), MaxPoints: 2,
		Detail: ternary(hasChangelog, "CHANGELOG present", "No changelog"),
		Suggestion: ternary(hasChangelog, "", "Add CHANGELOG.md following Keep a Changelog format"),
	})

	// pkg.go.dev reachable (check go.mod module path looks like a valid public path)
	gomodBytes, _ := os.ReadFile(filepath.Join(repo, "go.mod"))
	modPath := extractModulePath(string(gomodBytes))
	isPublicPath := strings.Contains(modPath, "github.com/") || strings.Contains(modPath, "golang.org/") || strings.Contains(modPath, "go.dev/")
	results = append(results, CheckResult{
		Name: "public_module_path", Passed: isPublicPath, Points: boolInt(isPublicPath, 3), MaxPoints: 3,
		Detail: fmt.Sprintf("Module: %s", modPath),
		Suggestion: ternary(isPublicPath, "", "Use a public module path (github.com/org/repo) for pkg.go.dev indexing"),
	})

	// go install works (check for main package)
	hasMain := fileExists(filepath.Join(repo, "main.go")) || fileExists(filepath.Join(repo, "cmd"))
	results = append(results, CheckResult{
		Name: "installable", Passed: hasMain, Points: boolInt(hasMain, 2), MaxPoints: 2,
		Detail: ternary(hasMain, "main package or cmd/ found", "No main package — library only"),
		Suggestion: ternary(hasMain, "", "Add a main package or cmd/ for go install support (libraries score this anyway)"),
	})

	return results
}

// ---------------------------------------------------------------------------
// Category: Maintenance (5 pts)
// ---------------------------------------------------------------------------

func checkMaintenance(repo string) []CheckResult {
	var results []CheckResult

	// Recent commit (within 90 days)
	cmd := exec.Command("git", "log", "-1", "--format=%ci")
	cmd.Dir = repo
	out, err := cmd.Output()
	recentCommit := false
	commitAge := "unknown"
	if err == nil {
		dateStr := strings.TrimSpace(string(out))
		if t, err := time.Parse("2006-01-02 15:04:05 -0700", dateStr); err == nil {
			days := int(time.Since(t).Hours() / 24)
			recentCommit = days <= 90
			commitAge = fmt.Sprintf("%d days ago", days)
		}
	}
	results = append(results, CheckResult{
		Name: "recent_commit", Passed: recentCommit, Points: boolInt(recentCommit, 2), MaxPoints: 2,
		Detail: fmt.Sprintf("Last commit: %s", commitAge),
		Suggestion: ternary(recentCommit, "", "Repository appears unmaintained (>90 days since last commit)"),
	})

	// Issue templates
	hasIssueTemplates := fileExists(filepath.Join(repo, ".github", "ISSUE_TEMPLATE")) || fileExists(filepath.Join(repo, ".github", "ISSUE_TEMPLATE.md"))
	results = append(results, CheckResult{
		Name: "issue_templates", Passed: hasIssueTemplates, Points: boolInt(hasIssueTemplates, 1), MaxPoints: 1,
		Detail: ternary(hasIssueTemplates, "Issue templates present", "No issue templates"),
		Suggestion: ternary(hasIssueTemplates, "", "Add .github/ISSUE_TEMPLATE/ with bug report and feature request templates"),
	})

	// PR template
	hasPRTemplate := fileExists(filepath.Join(repo, ".github", "pull_request_template.md")) || fileExists(filepath.Join(repo, ".github", "PULL_REQUEST_TEMPLATE.md"))
	results = append(results, CheckResult{
		Name: "pr_template", Passed: hasPRTemplate, Points: boolInt(hasPRTemplate, 1), MaxPoints: 1,
		Detail: ternary(hasPRTemplate, "PR template present", "No PR template"),
		Suggestion: ternary(hasPRTemplate, "", "Add .github/pull_request_template.md"),
	})

	// .editorconfig
	hasEditorconfig := fileExists(filepath.Join(repo, ".editorconfig"))
	results = append(results, CheckResult{
		Name: "editorconfig", Passed: hasEditorconfig, Points: boolInt(hasEditorconfig, 1), MaxPoints: 1,
		Detail: ternary(hasEditorconfig, ".editorconfig present", "No .editorconfig"),
		Suggestion: ternary(hasEditorconfig, "", "Add .editorconfig for consistent formatting across editors"),
	})

	return results
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileExistsMulti(repo string, names []string) bool {
	for _, n := range names {
		if fileExists(filepath.Join(repo, n)) {
			return true
		}
	}
	return false
}

func fileCheck(repo, name string, points int, suggestion string) CheckResult {
	exists := fileExists(filepath.Join(repo, name))
	return CheckResult{
		Name: strings.ToLower(strings.ReplaceAll(name, ".", "_")),
		Passed: exists, Points: boolInt(exists, points), MaxPoints: points,
		Detail:     ternary(exists, name+" present", name+" missing"),
		Suggestion: ternary(exists, "", suggestion),
	}
}

func fileCheckMulti(repo string, paths []string, displayName string, points int, suggestion string) CheckResult {
	exists := fileExistsMulti(repo, paths)
	return CheckResult{
		Name: strings.ToLower(strings.ReplaceAll(displayName, ".", "_")),
		Passed: exists, Points: boolInt(exists, points), MaxPoints: points,
		Detail:     ternary(exists, displayName+" present", displayName+" missing"),
		Suggestion: ternary(exists, "", suggestion),
	}
}

func contentCheck(name, text string, points int, check func(string) bool, passDetail, suggestion string) CheckResult {
	ok := check(text)
	return CheckResult{
		Name: name, Passed: ok, Points: boolInt(ok, points), MaxPoints: points,
		Detail:     ternary(ok, passDetail, "Not found"),
		Suggestion: ternary(ok, "", suggestion),
	}
}

func countFiles(repo, pattern string) int {
	count := 0
	filepath.Walk(repo, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "vendor" || base == "node_modules" || base == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}
		if matched, _ := filepath.Match(pattern, info.Name()); matched {
			count++
		}
		return nil
	})
	return count
}

func grepFiles(repo, pattern string) bool {
	found := false
	re := regexp.MustCompile(pattern)
	filepath.Walk(repo, func(path string, info os.FileInfo, err error) error {
		if found || err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				base := info.Name()
				if base == ".git" || base == "vendor" || base == "node_modules" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if re.MatchString(scanner.Text()) {
				found = true
				return nil
			}
		}
		return nil
	})
	return found
}

func scanFilesForPattern(repo string, re *regexp.Regexp) bool {
	found := false
	exts := map[string]bool{".go": true, ".json": true, ".toml": true, ".yaml": true, ".yml": true, ".md": true, ".sh": true, ".env": true}
	filepath.Walk(repo, func(path string, info os.FileInfo, err error) error {
		if found || err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				base := info.Name()
				if base == ".git" || base == "vendor" || base == "node_modules" || base == "testdata" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !exts[filepath.Ext(path)] && info.Name() != ".env" {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if re.MatchString(scanner.Text()) {
				found = true
				return nil
			}
		}
		return nil
	})
	return found
}

func goVetCheck(repo string, points int) []CheckResult {
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	passed := err == nil
	detail := "go vet passes"
	if !passed {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 3 {
			lines = lines[:3]
		}
		detail = "go vet failed: " + strings.Join(lines, "; ")
	}
	return []CheckResult{{
		Name: "go_vet", Passed: passed, Points: boolInt(passed, points), MaxPoints: points,
		Detail:     detail,
		Suggestion: ternary(passed, "", "Fix go vet issues"),
	}}
}

func goTestCoverage(repo string) float64 {
	cmd := exec.Command("go", "test", "-count=1", "-coverprofile=/tmp/oss-cov.out", "-short", "./...")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GOFLAGS=-count=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	// Parse coverage from output lines like "coverage: 90.5% of statements"
	lines := strings.Split(string(out), "\n")
	var totalCov, count float64
	for _, line := range lines {
		if idx := strings.Index(line, "coverage:"); idx >= 0 {
			sub := line[idx+len("coverage:"):]
			sub = strings.TrimSpace(sub)
			if pctIdx := strings.Index(sub, "%"); pctIdx > 0 {
				var pct float64
				fmt.Sscanf(sub[:pctIdx], "%f", &pct)
				totalCov += pct
				count++
			}
		}
	}
	if count == 0 {
		return 0
	}
	return totalCov / count
}

func goTestRace(repo string) bool {
	cmd := exec.Command("go", "test", "-count=1", "-race", "-short", "./...")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GOFLAGS=-count=1")
	err := cmd.Run()
	return err == nil
}

func extractGoVersion(gomod string) string {
	for _, line := range strings.Split(gomod, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			return strings.TrimPrefix(line, "go ")
		}
	}
	return "unknown"
}

func extractModulePath(gomod string) string {
	for _, line := range strings.Split(gomod, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimPrefix(line, "module ")
		}
	}
	return "unknown"
}

func boolInt(b bool, v int) int {
	if b {
		return v
	}
	return 0
}

func ternary(cond bool, t, f string) string {
	if cond {
		return t
	}
	return f
}
