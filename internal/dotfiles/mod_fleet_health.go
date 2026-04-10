package dotfiles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type manifestRepoPolicy struct {
	Name            string `json:"name"`
	Scope           string `json:"scope"`
	Lifecycle       string `json:"lifecycle"`
	Language        string `json:"language"`
	BaselineProfile string `json:"baseline_profile"`
	WorkflowPolicy  string `json:"workflow_policy"`
	WorkflowFamily  string `json:"workflow_family"`
}

type workspaceManifest struct {
	Repos []manifestRepoPolicy `json:"repos"`
}

type FleetBaselineCache struct {
	GeneratedAt string                 `json:"generated_at"`
	LocalDir    string                 `json:"local_dir"`
	Repos       []RepoBaselineSnapshot `json:"repos"`
}

type githubRunSummary struct {
	Conclusion string `json:"conclusion"`
	UpdatedAt  string `json:"updatedAt"`
}

func fleetRoot(localDir string) string {
	if strings.TrimSpace(localDir) != "" {
		return localDir
	}
	return filepath.Join(homeDir(), "hairglasses-studio")
}

func fleetBaselineCachePath(localDir string) string {
	return filepath.Join(fleetRoot(localDir), "docs", "agent-parity", "workspace-health-matrix.json")
}

func listWorkspaceRepos(localDir string) ([]string, error) {
	root := fleetRoot(localDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	repos := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		repoPath := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
			repos = append(repos, repoPath)
		}
	}
	sort.Strings(repos)
	return repos, nil
}

func loadWorkspacePolicies(localDir string) map[string]manifestRepoPolicy {
	path := filepath.Join(fleetRoot(localDir), "workspace", "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]manifestRepoPolicy{}
	}
	var manifest workspaceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return map[string]manifestRepoPolicy{}
	}
	out := make(map[string]manifestRepoPolicy, len(manifest.Repos))
	for _, repo := range manifest.Repos {
		repo.BaselineProfile = strings.TrimSpace(repo.BaselineProfile)
		repo.WorkflowPolicy = strings.TrimSpace(repo.WorkflowPolicy)
		repo.WorkflowFamily = strings.TrimSpace(repo.WorkflowFamily)
		out[repo.Name] = repo
	}
	return out
}

func normalizeRepoPolicy(repoPath string, policy manifestRepoPolicy) manifestRepoPolicy {
	if policy.Name == "" {
		policy.Name = filepath.Base(repoPath)
	}
	if policy.BaselineProfile == "" {
		policy.BaselineProfile = inferBaselineProfile(repoPath, policy.Language, policy.Scope)
	}
	if policy.WorkflowPolicy == "" {
		policy.WorkflowPolicy = inferWorkflowPolicy(repoPath)
	}
	if policy.WorkflowFamily == "" {
		policy.WorkflowFamily = inferWorkflowFamily(repoPath, policy.Language)
	}
	return policy
}

func inferBaselineProfile(repoPath, language, scope string) string {
	if repoHasMakeCheck(repoPath) {
		return "make_check"
	}
	if scope == "compatibility_only" {
		return "informational"
	}
	switch {
	case strings.Contains(language, "Go"):
		return "go_test"
	case strings.Contains(language, "TypeScript"), strings.Contains(language, "Node"):
		return "npm_test"
	case strings.Contains(language, "Python"):
		return "python_pytest"
	default:
		return "informational"
	}
}

func inferWorkflowPolicy(repoPath string) string {
	if repoHasWorkflows(repoPath) {
		return "repo_owned"
	}
	return "retired"
}

func inferWorkflowFamily(repoPath, language string) string {
	if !repoHasWorkflows(repoPath) {
		return "none"
	}
	switch {
	case strings.Contains(language, "Go"):
		return "go-ci"
	case strings.Contains(language, "TypeScript"), strings.Contains(language, "Node"):
		return "node-ci"
	case strings.Contains(language, "Python"):
		return "python-ci"
	default:
		return "misc-ci"
	}
}

func repoHasMakeCheck(repoPath string) bool {
	makefile := filepath.Join(repoPath, "Makefile")
	data, err := os.ReadFile(makefile)
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, "pipeline.mk") || strings.Contains(text, "\ncheck:") || strings.HasPrefix(text, "check:")
}

func repoHasWorkflows(repoPath string) bool {
	info, err := os.Stat(filepath.Join(repoPath, ".github", "workflows"))
	return err == nil && info.IsDir()
}

func readFleetBaselineCache(localDir string) (map[string]RepoBaselineSnapshot, error) {
	path := fleetBaselineCachePath(localDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]RepoBaselineSnapshot{}, nil
		}
		return nil, err
	}
	var cache FleetBaselineCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	out := make(map[string]RepoBaselineSnapshot, len(cache.Repos))
	for _, repo := range cache.Repos {
		out[repo.Repo] = repo
	}
	return out, nil
}

func writeFleetBaselineCache(localDir string, repos []RepoBaselineSnapshot) error {
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Repo < repos[j].Repo
	})
	cache := FleetBaselineCache{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		LocalDir:    fleetRoot(localDir),
		Repos:       repos,
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	path := fleetBaselineCachePath(localDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func runFleetBaselineRefresh(input FleetBaselineRefreshInput) (FleetBaselineRefreshOutput, error) {
	localDir := fleetRoot(input.LocalDir)
	repoPaths, err := listWorkspaceRepos(localDir)
	if err != nil {
		return FleetBaselineRefreshOutput{}, err
	}
	policies := loadWorkspacePolicies(localDir)
	selected := make(map[string]struct{}, len(input.Repos))
	for _, repo := range input.Repos {
		selected[repo] = struct{}{}
	}

	repos := make([]RepoBaselineSnapshot, 0, len(repoPaths))
	for _, repoPath := range repoPaths {
		name := filepath.Base(repoPath)
		if len(selected) > 0 {
			if _, ok := selected[name]; !ok {
				continue
			}
		}
		policy := normalizeRepoPolicy(repoPath, policies[name])
		repos = append(repos, runBaselineSnapshot(repoPath, policy))
	}
	if err := writeFleetBaselineCache(localDir, repos); err != nil {
		return FleetBaselineRefreshOutput{}, fmt.Errorf("write cache: %w", err)
	}
	return FleetBaselineRefreshOutput{
		Checked:   len(repos),
		Updated:   len(repos),
		CachePath: fleetBaselineCachePath(localDir),
		Repos:     repos,
	}, nil
}

func runBaselineSnapshot(repoPath string, policy manifestRepoPolicy) RepoBaselineSnapshot {
	now := time.Now().UTC().Format(time.RFC3339)
	out := RepoBaselineSnapshot{
		Repo:              filepath.Base(repoPath),
		BaselineProfile:   policy.BaselineProfile,
		WorkflowPolicy:    policy.WorkflowPolicy,
		WorkflowFamily:    policy.WorkflowFamily,
		WorkflowStatus:    computeWorkflowStatus(repoPath, policy),
		BaselineCheckedAt: now,
	}

	branch := gitString(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	out.CurrentBranch = branch
	out.BaselineCommit = gitString(repoPath, "rev-parse", "HEAD")

	command := baselineCommand(policy.BaselineProfile)
	out.BaselineCommand = strings.Join(command, " ")
	switch policy.BaselineProfile {
	case "informational":
		out.LocalBaselineStatus = "informational"
		return out
	case "":
		out.LocalBaselineStatus = "missing_profile"
		return out
	}
	if branch != "main" && branch != "master" {
		out.LocalBaselineStatus = "not_main"
		return out
	}
	if gitIsDirty(repoPath) {
		out.LocalBaselineStatus = "dirty"
		return out
	}
	if len(command) == 0 {
		out.LocalBaselineStatus = "missing_profile"
		return out
	}

	cmd := exec.CommandContext(context.Background(), command[0], command[1:]...)
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		out.LocalBaselineStatus = "fail"
		return out
	}
	out.LocalBaselineStatus = "pass"
	return out
}

func baselineCommand(profile string) []string {
	switch profile {
	case "make_check":
		return []string{"make", "check"}
	case "go_test":
		return []string{"go", "test", "./...", "-count=1"}
	case "npm_test":
		return []string{"npm", "test"}
	case "python_pytest":
		return []string{"python", "-m", "pytest"}
	default:
		return nil
	}
}

func gitString(repoPath string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func gitIsDirty(repoPath string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false
	}
	return strings.TrimSpace(out.String()) != ""
}

func workflowDirDirty(repoPath string) bool {
	cmd := exec.Command("git", "status", "--porcelain", "--", ".github/workflows")
	cmd.Dir = repoPath
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false
	}
	return strings.TrimSpace(out.String()) != ""
}

func readRemoteCIStatus(repoPath string) (string, string) {
	repoSlug := githubRepoSlugFromRemote(repoPath)
	if repoSlug == "" {
		return "none", ""
	}
	cmd := exec.Command("gh", "run", "list", "--repo", repoSlug, "--limit", "1", "--json", "conclusion,updatedAt")
	cmd.Dir = repoPath
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "none", ""
	}
	var runs []githubRunSummary
	if err := json.Unmarshal(out.Bytes(), &runs); err != nil || len(runs) == 0 {
		return "none", ""
	}
	switch strings.TrimSpace(runs[0].Conclusion) {
	case "success":
		return "pass", runs[0].UpdatedAt
	case "failure":
		return "fail", runs[0].UpdatedAt
	case "":
		return "none", runs[0].UpdatedAt
	default:
		return runs[0].Conclusion, runs[0].UpdatedAt
	}
}

func githubRepoSlugFromRemote(repoPath string) string {
	cmd := exec.Command("git", "config", "remote.origin.url")
	cmd.Dir = repoPath
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return filepath.Base(repoPath)
	}
	url := strings.TrimSuffix(strings.TrimSpace(out.String()), ".git")
	if idx := strings.Index(url, "github.com"); idx >= 0 {
		slug := url[idx+len("github.com"):]
		slug = strings.TrimPrefix(slug, ":")
		slug = strings.TrimPrefix(slug, "/")
		if slug != "" {
			return slug
		}
	}
	return filepath.Base(repoPath)
}

func computeWorkflowStatus(repoPath string, policy manifestRepoPolicy) string {
	hasWorkflows := repoHasWorkflows(repoPath)
	switch policy.WorkflowPolicy {
	case "repo_owned":
		if !hasWorkflows {
			return "missing_owned_workflow"
		}
		if policy.WorkflowFamily == "" || policy.WorkflowFamily == "none" {
			return "unexpected_workflow"
		}
		if workflowDirDirty(repoPath) {
			return "repo_owned_drift"
		}
		return "clean"
	default:
		if !hasWorkflows {
			return "clean"
		}
		if workflowDirDirty(repoPath) {
			return "unexpected_workflow"
		}
		return "retired_residue"
	}
}

func computeSignalVerdict(remoteCIStatus, localBaselineStatus, workflowStatus string) string {
	switch workflowStatus {
	case "repo_owned_drift", "retired_residue", "unexpected_workflow", "missing_owned_workflow":
		return "governance"
	}

	switch localBaselineStatus {
	case "pass":
		if remoteCIStatus == "fail" {
			return "stale_remote"
		}
		return "green"
	case "fail":
		return "red"
	case "informational":
		if remoteCIStatus == "fail" {
			return "red"
		}
		return "green"
	default:
		if remoteCIStatus == "fail" {
			return "red"
		}
		if remoteCIStatus == "pass" {
			return "green"
		}
		return "unknown"
	}
}

func signalFreshnessDays(values ...string) int {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, value)
		if err != nil {
			continue
		}
		return int(time.Since(ts).Hours() / 24)
	}
	return -1
}

func runFleetAudit(localDir string) (FleetAuditOutput, error) {
	root := fleetRoot(localDir)
	repoPaths, err := listWorkspaceRepos(root)
	if err != nil {
		return FleetAuditOutput{}, err
	}
	policies := loadWorkspacePolicies(root)
	snapshots, err := readFleetBaselineCache(root)
	if err != nil {
		return FleetAuditOutput{}, fmt.Errorf("read baseline cache: %w", err)
	}

	var repos []RepoAuditInfo
	var total, passing, failing, goRepos, stale, governance, unknown int

	for _, repoPath := range repoPaths {
		total++
		name := filepath.Base(repoPath)
		policy := normalizeRepoPolicy(repoPath, policies[name])
		info := RepoAuditInfo{Name: name}

		switch {
		case fleetFileExists(filepath.Join(repoPath, "go.mod")):
			info.Language = "go"
			goRepos++
			info.GoVersion = goVersionFromMod(filepath.Join(repoPath, "go.mod"))
		case fleetFileExists(filepath.Join(repoPath, "package.json")):
			info.Language = "node"
		case fleetFileExists(filepath.Join(repoPath, "pyproject.toml")):
			info.Language = "python"
		default:
			info.Language = "shell"
		}

		info.LastCommitDays = lastCommitDays(repoPath)
		info.HasPipelineMk = fleetFileExists(filepath.Join(repoPath, "pipeline.mk")) || repoHasMakeCheck(repoPath)
		info.HasCLAUDEmd = fleetFileExists(filepath.Join(repoPath, "CLAUDE.md"))
		info.HasCI = repoHasWorkflows(repoPath)
		info.RemoteCIStatus, info.RemoteCIUpdatedAt = readRemoteCIStatus(repoPath)
		info.CIStatus = info.RemoteCIStatus
		info.BaselineProfile = policy.BaselineProfile
		info.WorkflowPolicy = policy.WorkflowPolicy
		info.WorkflowFamily = policy.WorkflowFamily
		info.WorkflowStatus = computeWorkflowStatus(repoPath, policy)

		snapshot, ok := snapshots[name]
		if ok {
			info.LocalBaselineStatus = snapshot.LocalBaselineStatus
			info.BaselineCheckedAt = snapshot.BaselineCheckedAt
			info.BaselineCommit = snapshot.BaselineCommit
			info.BaselineCommand = snapshot.BaselineCommand
			if snapshot.BaselineProfile != "" {
				info.BaselineProfile = snapshot.BaselineProfile
			}
		} else {
			info.LocalBaselineStatus = "unknown"
		}
		info.SignalVerdict = computeSignalVerdict(info.RemoteCIStatus, info.LocalBaselineStatus, info.WorkflowStatus)
		info.SignalFreshnessDays = signalFreshnessDays(info.BaselineCheckedAt, info.RemoteCIUpdatedAt)

		switch info.SignalVerdict {
		case "green":
			passing++
		case "red":
			failing++
		case "stale_remote":
			stale++
		case "governance":
			governance++
		default:
			unknown++
		}

		info.TestCount = repoTestCount(repoPath, info.Language)
		repos = append(repos, info)
	}

	return FleetAuditOutput{
		Total:      total,
		Passing:    passing,
		Failing:    failing,
		GoRepos:    goRepos,
		Stale:      stale,
		Governance: governance,
		Unknown:    unknown,
		Repos:      repos,
	}, nil
}

func fleetFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func goVersionFromMod(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "go ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

func lastCommitDays(repoPath string) int {
	ts := gitString(repoPath, "log", "-1", "--format=%ct")
	if ts == "" {
		return 0
	}
	var epoch int64
	if _, err := fmt.Sscanf(ts, "%d", &epoch); err != nil {
		return 0
	}
	return int((time.Now().Unix() - epoch) / 86400)
}

func repoTestCount(repoPath, language string) int {
	var shell string
	switch language {
	case "go":
		shell = "find . -name '*_test.go' -not -path './vendor/*' 2>/dev/null | wc -l"
	case "node":
		shell = "find . -name '*.test.*' -o -name '*.spec.*' 2>/dev/null | wc -l"
	case "python":
		shell = "find . -name 'test_*.py' -o -name '*_test.py' 2>/dev/null | wc -l"
	default:
		return 0
	}
	cmd := exec.Command("bash", "-c", shell)
	cmd.Dir = repoPath
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0
	}
	var count int
	_, _ = fmt.Sscanf(strings.TrimSpace(out.String()), "%d", &count)
	return count
}
