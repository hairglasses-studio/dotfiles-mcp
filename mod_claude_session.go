// mod_claude_session.go — Claude Code session discovery, crash detection, and recovery analysis tools
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// claudeDir returns the Claude Code config directory.
func claudeDir() string {
	home := homeDir()
	return filepath.Join(home, ".claude")
}

// isProcessAlive checks if a process with the given PID is running.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// parseDurationString parses human-friendly duration strings like "2h", "30m", "1d".
func parseDurationString(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	// Handle "d" suffix for days.
	if strings.HasSuffix(s, "d") {
		s = strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(s, "%d", &days); err != nil {
			return 0, fmt.Errorf("invalid duration: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// encodeRepoPath converts an absolute path to Claude's encoded project directory name.
// e.g. /home/hg/hairglasses-studio -> -home-hg-hairglasses-studio
func encodeRepoPath(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}

// readJSONLTail reads the last n JSON lines from a JSONL file.
// Returns parsed entries, skipping corrupt lines.
func readJSONLTail(path string, n int) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read all lines (for tail behavior we need to scan through).
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large lines
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Take last n lines.
	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}
	lines = lines[start:]

	var entries []map[string]any
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // skip corrupt lines
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// readJSONLAll reads all JSON entries from a JSONL file.
func readJSONLAll(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []map[string]any
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// runGit executes a git command in the given repo and returns stdout.
func runGit(repoPath string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", repoPath}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, string(exitErr.Stderr))
		}
		return string(out), err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// msToTime converts Unix milliseconds to time.Time.
func msToTime(ms float64) time.Time {
	return time.UnixMilli(int64(ms))
}

// msToRFC3339 converts Unix milliseconds to RFC3339 string.
func msToRFC3339(ms float64) string {
	if ms == 0 {
		return ""
	}
	return msToTime(ms).Format(time.RFC3339)
}

// ---------------------------------------------------------------------------
// Session metadata types (from ~/.claude/sessions/*.json)
// ---------------------------------------------------------------------------

type claudeSessionMeta struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	CWD        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"` // Unix ms
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
	Name       string `json:"name,omitempty"`
}

// loadAllSessionMeta reads all session files from ~/.claude/sessions/.
func loadAllSessionMeta() ([]claudeSessionMeta, error) {
	dir := filepath.Join(claudeDir(), "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []claudeSessionMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var meta claudeSessionMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		sessions = append(sessions, meta)
	}
	return sessions, nil
}

// loadHistoryIndex builds a map of sessionID -> last activity timestamp from history.jsonl.
func loadHistoryIndex() (map[string]time.Time, map[string]string, error) {
	path := filepath.Join(claudeDir(), "history.jsonl")
	entries, err := readJSONLAll(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	lastActivity := make(map[string]time.Time)
	lastDisplay := make(map[string]string)
	for _, entry := range entries {
		sid, _ := entry["sessionId"].(string)
		if sid == "" {
			continue
		}
		ts, _ := entry["timestamp"].(float64)
		if ts > 0 {
			t := msToTime(ts)
			if t.After(lastActivity[sid]) {
				lastActivity[sid] = t
			}
		}
		if d, ok := entry["display"].(string); ok {
			lastDisplay[sid] = d
		}
	}
	return lastActivity, lastDisplay, nil
}

// repoNameFromPath extracts the last path component as repo name.
func repoNameFromPath(path string) string {
	return filepath.Base(path)
}

// findSessionProjectDir finds the project directory for a session by its CWD.
func findSessionProjectDir(cwd string) string {
	encoded := encodeRepoPath(cwd)
	dir := filepath.Join(claudeDir(), "projects", encoded)
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

// findSessionJSONL finds the JSONL file for a session within a project directory.
func findSessionJSONL(projectDir, sessionID string) string {
	// Try direct file: <projectDir>/<sessionID>.jsonl
	direct := filepath.Join(projectDir, sessionID+".jsonl")
	if _, err := os.Stat(direct); err == nil {
		return direct
	}
	// Try subdirectory: <projectDir>/<sessionID>/<sessionID>.jsonl
	subdir := filepath.Join(projectDir, sessionID, sessionID+".jsonl")
	if _, err := os.Stat(subdir); err == nil {
		return subdir
	}
	return ""
}

// loadSessionTasks reads all task files for a session.
func loadSessionTasks(sessionID string) ([]SessionTask, error) {
	dir := filepath.Join(claudeDir(), "tasks", sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var tasks []SessionTask
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var task SessionTask
		if err := json.Unmarshal(data, &task); err != nil {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// findPlanFileForSession searches plan files for references to a session ID.
func findPlanFileForSession(sessionID string) string {
	dir := filepath.Join(claudeDir(), "plans")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), sessionID) {
			return e.Name()
		}
	}
	return ""
}

// findRecentPlanFile searches session JSONL for plan file references.
func findRecentPlanFile(projectDir, sessionID string) string {
	jsonlPath := findSessionJSONL(projectDir, sessionID)
	if jsonlPath == "" {
		return ""
	}
	// Read last 100 entries looking for plan references.
	entries, err := readJSONLTail(jsonlPath, 100)
	if err != nil {
		return ""
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		// Check for plan mode system messages.
		raw, _ := json.Marshal(entry)
		s := string(raw)
		if strings.Contains(s, "plans/") {
			// Extract plan file name.
			idx := strings.Index(s, "plans/")
			if idx >= 0 {
				rest := s[idx+6:]
				endIdx := strings.IndexAny(rest, `"' `)
				if endIdx > 0 {
					name := rest[:endIdx]
					if strings.HasSuffix(name, ".md") {
						return name
					}
				}
			}
		}
	}
	return ""
}

// loadSessionMemory reads memory files for a project.
func loadSessionMemory(projectDir string) []MemoryFile {
	memDir := filepath.Join(projectDir, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return nil
	}
	var files []MemoryFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "MEMORY.md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memDir, e.Name()))
		if err != nil {
			continue
		}
		files = append(files, MemoryFile{
			Name:    e.Name(),
			Content: string(data),
		})
	}
	return files
}

// ---------------------------------------------------------------------------
// Input / Output types
// ---------------------------------------------------------------------------

// ── claude_crash_detect ──

type CrashDetectInput struct {
	Window    string `json:"window,omitempty" jsonschema:"description=Time window to check for crashes (e.g. '2h'\\, '30m'\\, '1d'). Defaults to '4h'."`
	Threshold int    `json:"threshold,omitempty" jsonschema:"description=Minimum number of dead sessions to trigger crash detection. Defaults to 2."`
}

type CrashDetectOutput struct {
	CrashDetected   bool                `json:"crash_detected"`
	Severity        string              `json:"severity"` // minor, major, catastrophic
	CrashedSessions []CrashedSessionRef `json:"crashed_sessions"`
	TotalSessions   int                 `json:"total_sessions"`
	AliveSessions   int                 `json:"alive_sessions"`
	DeadSessions    int                 `json:"dead_sessions"`
	CrashIndicators []string            `json:"crash_indicators,omitempty"`
	Window          string              `json:"window"`
}

type CrashedSessionRef struct {
	SessionID    string `json:"session_id"`
	Repo         string `json:"repo"`
	Name         string `json:"name,omitempty"`
	LastActivity string `json:"last_activity,omitempty"`
	PID          int    `json:"pid"`
}

// ── claude_session_scan ──

type SessionScanInput struct {
	Window string `json:"window,omitempty" jsonschema:"description=Only include sessions with activity within this window (e.g. '2h'\\, '1d'). Omit for all sessions."`
}

type SessionScanOutput struct {
	Sessions []SessionScanEntry `json:"sessions"`
	Total    int                `json:"total"`
	Alive    int                `json:"alive"`
	Dead     int                `json:"dead"`
}

type SessionScanEntry struct {
	SessionID    string `json:"session_id"`
	Repo         string `json:"repo"`
	RepoName     string `json:"repo_name"`
	Name         string `json:"name,omitempty"`
	Status       string `json:"status"` // alive, dead
	LastActivity string `json:"last_activity,omitempty"`
	StartedAt    string `json:"started_at"`
	PID          int    `json:"pid"`
}

// ── claude_session_detail ──

type SessionDetailInput struct {
	SessionID string `json:"session_id" jsonschema:"required,description=The session UUID to inspect"`
	LogLines  int    `json:"log_lines,omitempty" jsonschema:"description=Number of recent log entries to include. Defaults to 20."`
}

type SessionDetailOutput struct {
	SessionID    string        `json:"session_id"`
	Repo         string        `json:"repo"`
	RepoName     string        `json:"repo_name"`
	Name         string        `json:"name,omitempty"`
	Status       string        `json:"status"`
	PID          int           `json:"pid"`
	StartedAt    string        `json:"started_at"`
	LastActivity string        `json:"last_activity,omitempty"`
	PlanFile     string        `json:"plan_file,omitempty"`
	Tasks        []SessionTask `json:"tasks,omitempty"`
	OpenTasks    int           `json:"open_tasks"`
	TotalTasks   int           `json:"total_tasks"`
	RecentLogs   []LogEntry    `json:"recent_logs,omitempty"`
	MemoryFiles  []MemoryFile  `json:"memory_files,omitempty"`
}

type SessionTask struct {
	ID          string `json:"id"`
	Subject     string `json:"subject"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"` // pending, in_progress, completed
}

type LogEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

type MemoryFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// ── claude_session_logs ──

type SessionLogsInput struct {
	SessionID string `json:"session_id" jsonschema:"required,description=The session UUID to read logs from"`
	Lines     int    `json:"lines,omitempty" jsonschema:"description=Number of recent log entries to return. Defaults to 50."`
}

type SessionLogsOutput struct {
	SessionID string     `json:"session_id"`
	Entries   []LogEntry `json:"entries"`
	Total     int        `json:"total"`
}

// ── claude_session_tag ──

type SessionTagInput struct {
	SessionID string `json:"session_id" jsonschema:"required,description=The session UUID to tag"`
}

type SessionTagOutput struct {
	SessionID          string `json:"session_id"`
	RepoName           string `json:"repo_name"`
	RepoBranch         string `json:"repo_branch,omitempty"`
	LastCommitTime     string `json:"last_commit_time,omitempty"`
	LastCommitHash     string `json:"last_commit_hash,omitempty"`
	LastSessionTime    string `json:"last_session_time,omitempty"`
	PlanFile           string `json:"plan_file,omitempty"`
	OpenTaskCount      int    `json:"open_task_count"`
	CompletedTaskCount int    `json:"completed_task_count"`
	TotalTaskCount     int    `json:"total_task_count"`
}

// ── claude_repo_status ──

type RepoStatusInput struct {
	RepoPath string `json:"repo_path" jsonschema:"required,description=Absolute path to the repository"`
}

type RepoStatusOutput struct {
	RepoPath         string   `json:"repo_path"`
	RepoName         string   `json:"repo_name"`
	Branch           string   `json:"branch"`
	LastCommitHash   string   `json:"last_commit_hash"`
	LastCommitMsg    string   `json:"last_commit_message"`
	LastCommitTime   string   `json:"last_commit_time"`
	RemoteTracking   string   `json:"remote_tracking,omitempty"`
	Staged           []string `json:"staged,omitempty"`
	Unstaged         []string `json:"unstaged,omitempty"`
	Untracked        []string `json:"untracked,omitempty"`
	StagedCount      int      `json:"staged_count"`
	UnstagedCount    int      `json:"unstaged_count"`
	UntrackedCount   int      `json:"untracked_count"`
	HasUncommitted   bool     `json:"has_uncommitted"`
	IsGitRepo        bool     `json:"is_git_repo"`
}

// ── claude_repo_diff ──

type RepoDiffInput struct {
	RepoPath string `json:"repo_path" jsonschema:"required,description=Absolute path to the repository"`
}

type RepoDiffOutput struct {
	RepoPath          string           `json:"repo_path"`
	RepoName          string           `json:"repo_name"`
	UnpushedCommits   []CommitSummary  `json:"unpushed_commits,omitempty"`
	UnpushedCount     int              `json:"unpushed_count"`
	LocalOnlyBranches []string         `json:"local_only_branches,omitempty"`
	StashEntries      []string         `json:"stash_entries,omitempty"`
	IsGitRepo         bool             `json:"is_git_repo"`
}

type CommitSummary struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

// ── claude_recovery_report ──

type RecoveryReportInput struct {
	Window     string `json:"window,omitempty" jsonschema:"description=Time window to check sessions (e.g. '4h'\\, '1d'). Defaults to '4h'."`
	StudioPath string `json:"studio_path,omitempty" jsonschema:"description=Path to the studio directory. Defaults to ~/hairglasses-studio."`
}

type RecoveryReport struct {
	GeneratedAt     string                  `json:"generated_at"`
	CrashDetected   bool                    `json:"crash_detected"`
	Severity        string                  `json:"severity"`
	Sessions        []RecoverySessionEntry  `json:"sessions"`
	Repos           []RecoveryRepoEntry     `json:"repos,omitempty"`
	OrphanedRepos   []RecoveryOrphanEntry   `json:"orphaned_repos,omitempty"`
	PriorityQueue   []RecoveryPriorityEntry `json:"priority_queue,omitempty"`
	AliveCount      int                     `json:"alive_count"`
	DeadCount       int                     `json:"dead_count"`
	TotalSessions   int                     `json:"total_sessions"`
}

type RecoverySessionEntry struct {
	SessionID    string `json:"session_id"`
	Repo         string `json:"repo"`
	RepoName     string `json:"repo_name"`
	Name         string `json:"name,omitempty"`
	Status       string `json:"status"`
	LastActivity string `json:"last_activity,omitempty"`
	PlanFile     string `json:"plan_file,omitempty"`
	OpenTasks    int    `json:"open_tasks"`
	TotalTasks   int    `json:"total_tasks"`
	ResumeCmd    string `json:"resume_cmd,omitempty"`
}

type RecoveryRepoEntry struct {
	RepoPath       string `json:"repo_path"`
	RepoName       string `json:"repo_name"`
	Branch         string `json:"branch"`
	HasUncommitted bool   `json:"has_uncommitted"`
	UnpushedCount  int    `json:"unpushed_count"`
	LastCommitTime string `json:"last_commit_time,omitempty"`
	ActiveSession  string `json:"active_session,omitempty"`
}

type RecoveryOrphanEntry struct {
	RepoPath      string `json:"repo_path"`
	RepoName      string `json:"repo_name"`
	Branch        string `json:"branch"`
	StagedCount   int    `json:"staged_count"`
	UnstagedCount int    `json:"unstaged_count"`
	UntrackedCount int   `json:"untracked_count"`
}

type RecoveryPriorityEntry struct {
	Rank         int    `json:"rank"`
	SessionID    string `json:"session_id"`
	RepoName     string `json:"repo_name"`
	Name         string `json:"name,omitempty"`
	OpenTasks    int    `json:"open_tasks"`
	LastActivity string `json:"last_activity,omitempty"`
	ResumeCmd    string `json:"resume_cmd"`
}

// ---------------------------------------------------------------------------
// Core logic (shared by tools)
// ---------------------------------------------------------------------------

// scanSessions scans all Claude Code sessions and enriches with PID liveness + history.
func scanSessions(windowDur time.Duration) ([]SessionScanEntry, error) {
	metas, err := loadAllSessionMeta()
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to load session metadata: %w", handler.ErrNotFound, err)
	}

	lastActivity, _, err := loadHistoryIndex()
	if err != nil {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}

	cutoff := time.Time{}
	if windowDur > 0 {
		cutoff = time.Now().Add(-windowDur)
	}

	var results []SessionScanEntry
	for _, m := range metas {
		startedAt := msToTime(float64(m.StartedAt))
		la := lastActivity[m.SessionID]
		if la.IsZero() {
			la = startedAt
		}

		// Filter by window: include if started or last active within window.
		if !cutoff.IsZero() && la.Before(cutoff) && startedAt.Before(cutoff) {
			continue
		}

		status := "dead"
		if isProcessAlive(m.PID) {
			status = "alive"
		}

		results = append(results, SessionScanEntry{
			SessionID:    m.SessionID,
			Repo:         m.CWD,
			RepoName:     repoNameFromPath(m.CWD),
			Name:         m.Name,
			Status:       status,
			LastActivity: la.Format(time.RFC3339),
			StartedAt:    startedAt.Format(time.RFC3339),
			PID:          m.PID,
		})
	}

	// Sort by last activity descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].LastActivity > results[j].LastActivity
	})

	return results, nil
}

// getRepoStatus returns git status for a repository.
func getRepoStatus(repoPath string) (RepoStatusOutput, error) {
	out := RepoStatusOutput{
		RepoPath: repoPath,
		RepoName: repoNameFromPath(repoPath),
	}

	// Check if it's a git repo.
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		out.IsGitRepo = false
		return out, nil
	}
	out.IsGitRepo = true

	// Current branch.
	if branch, err := runGit(repoPath, "branch", "--show-current"); err == nil {
		out.Branch = branch
	}

	// Last commit.
	if logOut, err := runGit(repoPath, "log", "-1", "--format=%H|||%s|||%aI"); err == nil && logOut != "" {
		parts := strings.SplitN(logOut, "|||", 3)
		if len(parts) == 3 {
			out.LastCommitHash = parts[0]
			out.LastCommitMsg = parts[1]
			out.LastCommitTime = parts[2]
		}
	}

	// Remote tracking.
	if tracking, err := runGit(repoPath, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		out.RemoteTracking = tracking
	}

	// Porcelain status.
	if status, err := runGit(repoPath, "status", "--porcelain"); err == nil && status != "" {
		for _, line := range strings.Split(status, "\n") {
			if len(line) < 3 {
				continue
			}
			x, y := line[0], line[1]
			file := strings.TrimSpace(line[3:])
			if x != ' ' && x != '?' {
				out.Staged = append(out.Staged, file)
			}
			if y != ' ' && y != '?' {
				out.Unstaged = append(out.Unstaged, file)
			}
			if x == '?' && y == '?' {
				out.Untracked = append(out.Untracked, file)
			}
		}
	}
	out.StagedCount = len(out.Staged)
	out.UnstagedCount = len(out.Unstaged)
	out.UntrackedCount = len(out.Untracked)
	out.HasUncommitted = out.StagedCount > 0 || out.UnstagedCount > 0

	return out, nil
}

// getRepoDiff returns unpushed commits, local-only branches, and stashes.
func getRepoDiff(repoPath string) (RepoDiffOutput, error) {
	out := RepoDiffOutput{
		RepoPath: repoPath,
		RepoName: repoNameFromPath(repoPath),
	}

	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		out.IsGitRepo = false
		return out, nil
	}
	out.IsGitRepo = true

	// Unpushed commits (try origin/main, then origin/master).
	for _, base := range []string{"origin/main", "origin/master"} {
		if logOut, err := runGit(repoPath, "log", base+"..HEAD", "--oneline"); err == nil {
			if logOut != "" {
				for _, line := range strings.Split(logOut, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					parts := strings.SplitN(line, " ", 2)
					cs := CommitSummary{Hash: parts[0]}
					if len(parts) > 1 {
						cs.Message = parts[1]
					}
					out.UnpushedCommits = append(out.UnpushedCommits, cs)
				}
			}
			break // found a valid base
		}
	}
	out.UnpushedCount = len(out.UnpushedCommits)

	// Local-only branches (no upstream).
	if branchOut, err := runGit(repoPath, "branch", "-vv"); err == nil {
		for _, line := range strings.Split(branchOut, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			line = strings.TrimPrefix(line, "* ")
			// Branches with upstream show [origin/...], local-only don't.
			if !strings.Contains(line, "[") {
				parts := strings.Fields(line)
				if len(parts) >= 1 && parts[0] != "main" && parts[0] != "master" {
					out.LocalOnlyBranches = append(out.LocalOnlyBranches, parts[0])
				}
			}
		}
	}

	// Stash list.
	if stashOut, err := runGit(repoPath, "stash", "list"); err == nil && stashOut != "" {
		out.StashEntries = strings.Split(stashOut, "\n")
	}

	return out, nil
}

// summarizeLogEntry creates a LogEntry from a raw JSONL entry.
func summarizeLogEntry(entry map[string]any) LogEntry {
	le := LogEntry{}
	if t, ok := entry["type"].(string); ok {
		le.Type = t
	}
	if ts, ok := entry["timestamp"].(string); ok {
		le.Timestamp = ts
	}

	// Build summary based on type.
	switch le.Type {
	case "user":
		if msg, ok := entry["message"].(map[string]any); ok {
			if content, ok := msg["content"].(string); ok {
				le.Summary = truncate(content, 200)
			}
		}
	case "assistant":
		if msg, ok := entry["message"].(map[string]any); ok {
			if content, ok := msg["content"].([]any); ok && len(content) > 0 {
				if first, ok := content[0].(map[string]any); ok {
					if text, ok := first["text"].(string); ok {
						le.Summary = truncate(text, 200)
					} else if first["type"] == "tool_use" {
						name, _ := first["name"].(string)
						le.Summary = fmt.Sprintf("[tool_use: %s]", name)
					}
				}
			}
		}
	case "system":
		if sub, ok := entry["subtype"].(string); ok {
			le.Summary = fmt.Sprintf("[system: %s]", sub)
		}
	case "permission-mode":
		mode, _ := entry["permissionMode"].(string)
		le.Summary = fmt.Sprintf("[permission-mode: %s]", mode)
	case "file-history-snapshot":
		le.Summary = "[file-history-snapshot]"
	default:
		le.Summary = fmt.Sprintf("[%s]", le.Type)
	}

	return le
}

func truncate(s string, max int) string {
	// Collapse newlines for summary.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

type ClaudeSessionModule struct{}

func (m *ClaudeSessionModule) Name() string        { return "claude_session" }
func (m *ClaudeSessionModule) Description() string { return "Claude Code session discovery, crash detection, and recovery analysis" }

func (m *ClaudeSessionModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── claude_crash_detect ────────────────────────
		handler.TypedHandler[CrashDetectInput, CrashDetectOutput](
			"claude_crash_detect",
			"Detect Claude Code session crashes by checking PID liveness, dead session count within a time window, and crash indicators in history. Returns severity assessment (minor/major/catastrophic).",
			func(_ context.Context, input CrashDetectInput) (CrashDetectOutput, error) {
				window := input.Window
				if window == "" {
					window = "4h"
				}
				threshold := input.Threshold
				if threshold <= 0 {
					threshold = 2
				}

				windowDur, err := parseDurationString(window)
				if err != nil {
					return CrashDetectOutput{}, fmt.Errorf("[%s] invalid window: %w", handler.ErrInvalidParam, err)
				}

				sessions, err := scanSessions(windowDur)
				if err != nil {
					return CrashDetectOutput{}, err
				}

				var alive, dead int
				var crashed []CrashedSessionRef
				for _, s := range sessions {
					if s.Status == "alive" {
						alive++
					} else {
						dead++
						crashed = append(crashed, CrashedSessionRef{
							SessionID:    s.SessionID,
							Repo:         s.RepoName,
							Name:         s.Name,
							LastActivity: s.LastActivity,
							PID:          s.PID,
						})
					}
				}

				// Check history for crash indicators.
				var indicators []string
				histPath := filepath.Join(claudeDir(), "history.jsonl")
				if entries, err := readJSONLAll(histPath); err == nil {
					cutoff := time.Now().Add(-windowDur)
					for _, entry := range entries {
						ts, _ := entry["timestamp"].(float64)
						if ts > 0 && msToTime(ts).Before(cutoff) {
							continue
						}
						display, _ := entry["display"].(string)
						lower := strings.ToLower(display)
						if strings.Contains(lower, "session crashed") || strings.Contains(lower, "--resume") {
							indicators = append(indicators, truncate(display, 100))
						}
					}
				}

				severity := "none"
				crashDetected := dead >= threshold
				if crashDetected {
					switch {
					case dead >= 6:
						severity = "catastrophic"
					case dead >= 3:
						severity = "major"
					default:
						severity = "minor"
					}
				}

				return CrashDetectOutput{
					CrashDetected:   crashDetected,
					Severity:        severity,
					CrashedSessions: crashed,
					TotalSessions:   len(sessions),
					AliveSessions:   alive,
					DeadSessions:    dead,
					CrashIndicators: indicators,
					Window:          window,
				}, nil
			},
		),

		// ── claude_session_scan ────────────────────────
		handler.TypedHandler[SessionScanInput, SessionScanOutput](
			"claude_session_scan",
			"Scan all Claude Code sessions, check PID liveness, and correlate with history for last activity timestamps. Returns per-session status (alive/dead), repo, name, and timestamps.",
			func(_ context.Context, input SessionScanInput) (SessionScanOutput, error) {
				var windowDur time.Duration
				if input.Window != "" {
					var err error
					windowDur, err = parseDurationString(input.Window)
					if err != nil {
						return SessionScanOutput{}, fmt.Errorf("[%s] invalid window: %w", handler.ErrInvalidParam, err)
					}
				}

				sessions, err := scanSessions(windowDur)
				if err != nil {
					return SessionScanOutput{}, err
				}

				var alive, dead int
				for _, s := range sessions {
					if s.Status == "alive" {
						alive++
					} else {
						dead++
					}
				}

				return SessionScanOutput{
					Sessions: sessions,
					Total:    len(sessions),
					Alive:    alive,
					Dead:     dead,
				}, nil
			},
		),

		// ── claude_session_detail ──────────────────────
		handler.TypedHandler[SessionDetailInput, SessionDetailOutput](
			"claude_session_detail",
			"Deep inspection of a single Claude Code session: metadata, recent interaction logs, associated plan file, task list with statuses, and memory files.",
			func(_ context.Context, input SessionDetailInput) (SessionDetailOutput, error) {
				if input.SessionID == "" {
					return SessionDetailOutput{}, fmt.Errorf("[%s] session_id is required", handler.ErrInvalidParam)
				}
				logLines := input.LogLines
				if logLines <= 0 {
					logLines = 20
				}

				// Find session metadata.
				metas, err := loadAllSessionMeta()
				if err != nil {
					return SessionDetailOutput{}, fmt.Errorf("[%s] failed to load sessions: %w", handler.ErrNotFound, err)
				}

				var meta *claudeSessionMeta
				for _, m := range metas {
					if m.SessionID == input.SessionID {
						meta = &m
						break
					}
				}
				if meta == nil {
					return SessionDetailOutput{}, fmt.Errorf("[%s] session %s not found", handler.ErrNotFound, input.SessionID)
				}

				// Build output.
				out := SessionDetailOutput{
					SessionID: meta.SessionID,
					Repo:      meta.CWD,
					RepoName:  repoNameFromPath(meta.CWD),
					Name:      meta.Name,
					PID:       meta.PID,
					StartedAt: msToRFC3339(float64(meta.StartedAt)),
				}

				if isProcessAlive(meta.PID) {
					out.Status = "alive"
				} else {
					out.Status = "dead"
				}

				// Last activity from history.
				lastActivity, _, _ := loadHistoryIndex()
				if la, ok := lastActivity[meta.SessionID]; ok {
					out.LastActivity = la.Format(time.RFC3339)
				}

				// Find project dir and session logs.
				projectDir := findSessionProjectDir(meta.CWD)
				if projectDir != "" {
					// Plan file.
					planFile := findRecentPlanFile(projectDir, meta.SessionID)
					if planFile == "" {
						planFile = findPlanFileForSession(meta.SessionID)
					}
					out.PlanFile = planFile

					// Recent logs.
					jsonlPath := findSessionJSONL(projectDir, meta.SessionID)
					if jsonlPath != "" {
						if entries, err := readJSONLTail(jsonlPath, logLines); err == nil {
							for _, entry := range entries {
								out.RecentLogs = append(out.RecentLogs, summarizeLogEntry(entry))
							}
						}
					}

					// Memory files.
					out.MemoryFiles = loadSessionMemory(projectDir)
				}

				// Tasks.
				tasks, _ := loadSessionTasks(meta.SessionID)
				out.Tasks = tasks
				out.TotalTasks = len(tasks)
				for _, t := range tasks {
					if t.Status != "completed" {
						out.OpenTasks++
					}
				}

				return out, nil
			},
		),

		// ── claude_session_logs ────────────────────────
		handler.TypedHandler[SessionLogsInput, SessionLogsOutput](
			"claude_session_logs",
			"Read the last N structured log entries from a Claude Code session's JSONL interaction log. Returns parsed entries with type, timestamp, and content summary.",
			func(_ context.Context, input SessionLogsInput) (SessionLogsOutput, error) {
				if input.SessionID == "" {
					return SessionLogsOutput{}, fmt.Errorf("[%s] session_id is required", handler.ErrInvalidParam)
				}
				lines := input.Lines
				if lines <= 0 {
					lines = 50
				}

				// Find session to get CWD.
				metas, err := loadAllSessionMeta()
				if err != nil {
					return SessionLogsOutput{}, fmt.Errorf("[%s] failed to load sessions: %w", handler.ErrNotFound, err)
				}

				var meta *claudeSessionMeta
				for _, m := range metas {
					if m.SessionID == input.SessionID {
						meta = &m
						break
					}
				}
				if meta == nil {
					return SessionLogsOutput{}, fmt.Errorf("[%s] session %s not found", handler.ErrNotFound, input.SessionID)
				}

				projectDir := findSessionProjectDir(meta.CWD)
				if projectDir == "" {
					return SessionLogsOutput{}, fmt.Errorf("[%s] project directory not found for %s", handler.ErrNotFound, meta.CWD)
				}

				jsonlPath := findSessionJSONL(projectDir, meta.SessionID)
				if jsonlPath == "" {
					return SessionLogsOutput{}, fmt.Errorf("[%s] session JSONL not found for %s", handler.ErrNotFound, input.SessionID)
				}

				entries, err := readJSONLTail(jsonlPath, lines)
				if err != nil {
					return SessionLogsOutput{}, fmt.Errorf("failed to read session logs: %w", err)
				}

				var logEntries []LogEntry
				for _, entry := range entries {
					logEntries = append(logEntries, summarizeLogEntry(entry))
				}

				// Count total lines.
				total := 0
				if f, err := os.Open(jsonlPath); err == nil {
					scanner := bufio.NewScanner(f)
					scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
					for scanner.Scan() {
						if strings.TrimSpace(scanner.Text()) != "" {
							total++
						}
					}
					f.Close()
				}

				return SessionLogsOutput{
					SessionID: input.SessionID,
					Entries:   logEntries,
					Total:     total,
				}, nil
			},
		),

		// ── claude_session_tag ─────────────────────────
		handler.TypedHandler[SessionTagInput, SessionTagOutput](
			"claude_session_tag",
			"Extract structured metadata tags for a Claude Code session: repo name, last commit timestamp, last session activity, plan file, and task counts.",
			func(_ context.Context, input SessionTagInput) (SessionTagOutput, error) {
				if input.SessionID == "" {
					return SessionTagOutput{}, fmt.Errorf("[%s] session_id is required", handler.ErrInvalidParam)
				}

				metas, err := loadAllSessionMeta()
				if err != nil {
					return SessionTagOutput{}, fmt.Errorf("[%s] failed to load sessions: %w", handler.ErrNotFound, err)
				}

				var meta *claudeSessionMeta
				for _, m := range metas {
					if m.SessionID == input.SessionID {
						meta = &m
						break
					}
				}
				if meta == nil {
					return SessionTagOutput{}, fmt.Errorf("[%s] session %s not found", handler.ErrNotFound, input.SessionID)
				}

				out := SessionTagOutput{
					SessionID: meta.SessionID,
					RepoName:  repoNameFromPath(meta.CWD),
				}

				// Git info.
				if branch, err := runGit(meta.CWD, "branch", "--show-current"); err == nil {
					out.RepoBranch = branch
				}
				if logOut, err := runGit(meta.CWD, "log", "-1", "--format=%H|||%aI"); err == nil && logOut != "" {
					parts := strings.SplitN(logOut, "|||", 2)
					if len(parts) == 2 {
						out.LastCommitHash = parts[0]
						out.LastCommitTime = parts[1]
					}
				}

				// Session last activity.
				lastActivity, _, _ := loadHistoryIndex()
				if la, ok := lastActivity[meta.SessionID]; ok {
					out.LastSessionTime = la.Format(time.RFC3339)
				}

				// Plan file.
				projectDir := findSessionProjectDir(meta.CWD)
				if projectDir != "" {
					planFile := findRecentPlanFile(projectDir, meta.SessionID)
					if planFile == "" {
						planFile = findPlanFileForSession(meta.SessionID)
					}
					out.PlanFile = planFile
				}

				// Tasks.
				tasks, _ := loadSessionTasks(meta.SessionID)
				out.TotalTaskCount = len(tasks)
				for _, t := range tasks {
					if t.Status == "completed" {
						out.CompletedTaskCount++
					} else {
						out.OpenTaskCount++
					}
				}

				return out, nil
			},
		),

		// ── claude_repo_status ─────────────────────────
		handler.TypedHandler[RepoStatusInput, RepoStatusOutput](
			"claude_repo_status",
			"Get git status for a repository: current branch, last commit, staged/unstaged/untracked files, and remote tracking status.",
			func(_ context.Context, input RepoStatusInput) (RepoStatusOutput, error) {
				if input.RepoPath == "" {
					return RepoStatusOutput{}, fmt.Errorf("[%s] repo_path is required", handler.ErrInvalidParam)
				}
				if _, err := os.Stat(input.RepoPath); err != nil {
					return RepoStatusOutput{}, fmt.Errorf("[%s] path does not exist: %s", handler.ErrNotFound, input.RepoPath)
				}
				return getRepoStatus(input.RepoPath)
			},
		),

		// ── claude_repo_diff ──────────────────────────
		handler.TypedHandler[RepoDiffInput, RepoDiffOutput](
			"claude_repo_diff",
			"Compare a repository against origin/main: unpushed commits, local-only branches, and stash entries.",
			func(_ context.Context, input RepoDiffInput) (RepoDiffOutput, error) {
				if input.RepoPath == "" {
					return RepoDiffOutput{}, fmt.Errorf("[%s] repo_path is required", handler.ErrInvalidParam)
				}
				if _, err := os.Stat(input.RepoPath); err != nil {
					return RepoDiffOutput{}, fmt.Errorf("[%s] path does not exist: %s", handler.ErrNotFound, input.RepoPath)
				}
				return getRepoDiff(input.RepoPath)
			},
		),

		// ── claude_recovery_report ────────────────────
		handler.TypedHandler[RecoveryReportInput, RecoveryReport](
			"claude_recovery_report",
			"Comprehensive cross-session, cross-repo recovery report. Scans all sessions, correlates with git state, identifies orphaned repos, and builds a priority queue for session resumption ranked by incomplete tasks.",
			func(_ context.Context, input RecoveryReportInput) (RecoveryReport, error) {
				window := input.Window
				if window == "" {
					window = "4h"
				}
				studioPath := input.StudioPath
				if studioPath == "" {
					studioPath = filepath.Join(homeDir(), "hairglasses-studio")
				}
				// Expand ~ if needed.
				if strings.HasPrefix(studioPath, "~/") {
					studioPath = filepath.Join(homeDir(), studioPath[2:])
				}

				windowDur, err := parseDurationString(window)
				if err != nil {
					return RecoveryReport{}, fmt.Errorf("[%s] invalid window: %w", handler.ErrInvalidParam, err)
				}

				// Scan sessions.
				sessions, err := scanSessions(windowDur)
				if err != nil {
					return RecoveryReport{}, err
				}

				report := RecoveryReport{
					GeneratedAt:   time.Now().Format(time.RFC3339),
					TotalSessions: len(sessions),
				}

				// Build session entries with task info.
				repoHasActiveSession := make(map[string]string) // repoPath -> sessionID
				for _, s := range sessions {
					if s.Status == "alive" {
						report.AliveCount++
						repoHasActiveSession[s.Repo] = s.SessionID
					} else {
						report.DeadCount++
					}

					entry := RecoverySessionEntry{
						SessionID:    s.SessionID,
						Repo:         s.Repo,
						RepoName:     s.RepoName,
						Name:         s.Name,
						Status:       s.Status,
						LastActivity: s.LastActivity,
					}

					// Task info.
					tasks, _ := loadSessionTasks(s.SessionID)
					entry.TotalTasks = len(tasks)
					for _, t := range tasks {
						if t.Status != "completed" {
							entry.OpenTasks++
						}
					}

					// Plan file.
					projectDir := findSessionProjectDir(s.Repo)
					if projectDir != "" {
						planFile := findRecentPlanFile(projectDir, s.SessionID)
						if planFile == "" {
							planFile = findPlanFileForSession(s.SessionID)
						}
						entry.PlanFile = planFile
					}

					if s.Status == "dead" {
						entry.ResumeCmd = fmt.Sprintf("claude --resume %s", s.SessionID)
					}

					report.Sessions = append(report.Sessions, entry)
				}

				// Crash detection.
				report.CrashDetected = report.DeadCount >= 2
				switch {
				case report.DeadCount >= 6:
					report.Severity = "catastrophic"
				case report.DeadCount >= 3:
					report.Severity = "major"
				case report.DeadCount >= 2:
					report.Severity = "minor"
				default:
					report.Severity = "none"
				}

				// Scan repos for git state (parallel with semaphore).
				repoPaths := make(map[string]bool)
				for _, s := range sessions {
					repoPaths[s.Repo] = true
				}

				// Also scan studio directory for repos with uncommitted changes.
				if entries, err := os.ReadDir(studioPath); err == nil {
					for _, e := range entries {
						if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
							rp := filepath.Join(studioPath, e.Name())
							repoPaths[rp] = true
						}
					}
				}

				type repoResult struct {
					status RepoStatusOutput
					diff   RepoDiffOutput
				}

				var mu sync.Mutex
				repoResults := make(map[string]repoResult)
				sem := make(chan struct{}, 8) // limit to 8 concurrent git ops
				var wg sync.WaitGroup

				for rp := range repoPaths {
					rp := rp
					wg.Add(1)
					go func() {
						defer wg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()

						status, _ := getRepoStatus(rp)
						diff, _ := getRepoDiff(rp)
						mu.Lock()
						repoResults[rp] = repoResult{status: status, diff: diff}
						mu.Unlock()
					}()
				}
				wg.Wait()

				// Build repo entries and identify orphans.
				for rp, res := range repoResults {
					if !res.status.IsGitRepo {
						continue
					}

					repoEntry := RecoveryRepoEntry{
						RepoPath:       rp,
						RepoName:       repoNameFromPath(rp),
						Branch:         res.status.Branch,
						HasUncommitted: res.status.HasUncommitted,
						UnpushedCount:  res.diff.UnpushedCount,
						LastCommitTime: res.status.LastCommitTime,
					}
					if sid, ok := repoHasActiveSession[rp]; ok {
						repoEntry.ActiveSession = sid
					}
					report.Repos = append(report.Repos, repoEntry)

					// Orphaned: has uncommitted changes but no active session.
					if res.status.HasUncommitted || res.diff.UnpushedCount > 0 {
						if _, hasActive := repoHasActiveSession[rp]; !hasActive {
							report.OrphanedRepos = append(report.OrphanedRepos, RecoveryOrphanEntry{
								RepoPath:       rp,
								RepoName:       repoNameFromPath(rp),
								Branch:         res.status.Branch,
								StagedCount:    res.status.StagedCount,
								UnstagedCount:  res.status.UnstagedCount,
								UntrackedCount: res.status.UntrackedCount,
							})
						}
					}
				}

				// Sort repos by name.
				sort.Slice(report.Repos, func(i, j int) bool {
					return report.Repos[i].RepoName < report.Repos[j].RepoName
				})

				// Build priority queue from dead sessions with open tasks.
				rank := 1
				for _, s := range report.Sessions {
					if s.Status == "dead" {
						report.PriorityQueue = append(report.PriorityQueue, RecoveryPriorityEntry{
							SessionID:    s.SessionID,
							RepoName:     s.RepoName,
							Name:         s.Name,
							OpenTasks:    s.OpenTasks,
							LastActivity: s.LastActivity,
							ResumeCmd:    s.ResumeCmd,
						})
					}
				}
				// Sort by open tasks descending, then last activity descending.
				sort.Slice(report.PriorityQueue, func(i, j int) bool {
					if report.PriorityQueue[i].OpenTasks != report.PriorityQueue[j].OpenTasks {
						return report.PriorityQueue[i].OpenTasks > report.PriorityQueue[j].OpenTasks
					}
					return report.PriorityQueue[i].LastActivity > report.PriorityQueue[j].LastActivity
				})
				for i := range report.PriorityQueue {
					report.PriorityQueue[i].Rank = rank
					rank++
				}

				return report, nil
			},
		),
	}
}

// Ensure unused import is consumed.
var _ io.Reader
