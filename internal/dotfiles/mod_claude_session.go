// mod_claude_session.go — Claude Code session discovery, crash detection, and recovery analysis tools
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
// e.g. /home/user/projects -> -home-user-projects
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
	RepoPath       string   `json:"repo_path"`
	RepoName       string   `json:"repo_name"`
	Branch         string   `json:"branch"`
	LastCommitHash string   `json:"last_commit_hash"`
	LastCommitMsg  string   `json:"last_commit_message"`
	LastCommitTime string   `json:"last_commit_time"`
	RemoteTracking string   `json:"remote_tracking,omitempty"`
	Staged         []string `json:"staged,omitempty"`
	Unstaged       []string `json:"unstaged,omitempty"`
	Untracked      []string `json:"untracked,omitempty"`
	StagedCount    int      `json:"staged_count"`
	UnstagedCount  int      `json:"unstaged_count"`
	UntrackedCount int      `json:"untracked_count"`
	HasUncommitted bool     `json:"has_uncommitted"`
	IsGitRepo      bool     `json:"is_git_repo"`
}

// ── claude_repo_diff ──

type RepoDiffInput struct {
	RepoPath string `json:"repo_path" jsonschema:"required,description=Absolute path to the repository"`
}

type RepoDiffOutput struct {
	RepoPath          string          `json:"repo_path"`
	RepoName          string          `json:"repo_name"`
	UnpushedCommits   []CommitSummary `json:"unpushed_commits,omitempty"`
	UnpushedCount     int             `json:"unpushed_count"`
	LocalOnlyBranches []string        `json:"local_only_branches,omitempty"`
	StashEntries      []string        `json:"stash_entries,omitempty"`
	IsGitRepo         bool            `json:"is_git_repo"`
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
	GeneratedAt   string                  `json:"generated_at"`
	CrashDetected bool                    `json:"crash_detected"`
	Severity      string                  `json:"severity"`
	Sessions      []RecoverySessionEntry  `json:"sessions"`
	Repos         []RecoveryRepoEntry     `json:"repos,omitempty"`
	OrphanedRepos []RecoveryOrphanEntry   `json:"orphaned_repos,omitempty"`
	PriorityQueue []RecoveryPriorityEntry `json:"priority_queue,omitempty"`
	AliveCount    int                     `json:"alive_count"`
	DeadCount     int                     `json:"dead_count"`
	TotalSessions int                     `json:"total_sessions"`
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
	RepoPath       string `json:"repo_path"`
	RepoName       string `json:"repo_name"`
	Branch         string `json:"branch"`
	StagedCount    int    `json:"staged_count"`
	UnstagedCount  int    `json:"unstaged_count"`
	UntrackedCount int    `json:"untracked_count"`
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

// ── claude_session_health ──

type SessionHealthInput struct {
	SessionID string `json:"session_id" jsonschema:"required,description=The session UUID to score"`
}

type SessionHealthOutput struct {
	SessionID       string            `json:"session_id"`
	IsAlive         bool              `json:"is_alive"`
	OverallScore    int               `json:"overall_score"` // 0-100
	Dimensions      []HealthDimension `json:"dimensions"`
	Recommendations []string          `json:"recommendations,omitempty"`
}

type HealthDimension struct {
	Category string `json:"category"`
	Score    int    `json:"score"` // 0-100
	Detail   string `json:"detail,omitempty"`
}

// ── claude_session_compare ──

type SessionCompareInput struct {
	SessionID1 string `json:"session_id_1" jsonschema:"required,description=First session UUID"`
	SessionID2 string `json:"session_id_2" jsonschema:"required,description=Second session UUID"`
}

type SessionCompareOutput struct {
	SessionID1 string             `json:"session_id_1"`
	SessionID2 string             `json:"session_id_2"`
	Repo       string             `json:"repo"`
	SameRepo   bool               `json:"same_repo"`
	Diff       SessionCompareDiff `json:"diff"`
}

type SessionCompareDiff struct {
	Session1Tasks       int    `json:"session1_tasks"`
	Session1Completed   int    `json:"session1_completed"`
	Session1Status      string `json:"session1_status"`
	Session2Tasks       int    `json:"session2_tasks"`
	Session2Completed   int    `json:"session2_completed"`
	Session2Status      string `json:"session2_status"`
	TaskCompletionDelta int    `json:"task_completion_delta"`
	TimeBetween         string `json:"time_between,omitempty"`
}

// ── claude_session_replay ──

type SessionReplayInput struct {
	SessionID    string `json:"session_id" jsonschema:"required,description=Session UUID to replay"`
	MaxExchanges int    `json:"max_exchanges,omitempty" jsonschema:"description=Max conversation exchanges to return. Defaults to 50."`
}

type SessionReplayOutput struct {
	SessionID  string                 `json:"session_id"`
	Repo       string                 `json:"repo"`
	TotalLines int                    `json:"total_lines"`
	Exchanges  []ConversationExchange `json:"exchanges"`
}

type ConversationExchange struct {
	Order            int      `json:"order"`
	UserText         string   `json:"user_text,omitempty"`
	AssistantSummary string   `json:"assistant_summary,omitempty"`
	ToolsUsed        []string `json:"tools_used,omitempty"`
	Timestamp        string   `json:"timestamp,omitempty"`
}

// ── claude_workspace_snapshot ──

type WorkspaceSnapshotInput struct {
	StudioPath string `json:"studio_path,omitempty" jsonschema:"description=Path to studio directory. Defaults to ~/hairglasses-studio."`
}

type WorkspaceSnapshotOutput struct {
	SnapshotID   string `json:"snapshot_id"`
	SavedTo      string `json:"saved_to"`
	SessionCount int    `json:"session_count"`
	RepoCount    int    `json:"repo_count"`
	Size         int64  `json:"size_bytes"`
}

type WorkspaceSnapshot struct {
	Timestamp  string             `json:"timestamp"`
	StudioPath string             `json:"studio_path"`
	SnapshotID string             `json:"snapshot_id"`
	Sessions   []SessionScanEntry `json:"sessions"`
	Repos      []RepoStatusOutput `json:"repos"`
}

// ── claude_repo_roadmap_status ──

type RepoRoadmapInput struct {
	RepoPath  string `json:"repo_path" jsonschema:"required,description=Absolute path to repo with ROADMAP.md"`
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Optional session UUID to compare tasks against roadmap"`
}

type RepoRoadmapOutput struct {
	RepoPath              string        `json:"repo_path"`
	RepoName              string        `json:"repo_name"`
	TotalCount            int           `json:"total_count"`
	CompletedCount        int           `json:"completed_count"`
	ProgressPercent       int           `json:"progress_percent"`
	Items                 []RoadmapItem `json:"items,omitempty"`
	SessionTaskCount      int           `json:"session_task_count,omitempty"`
	SessionCompletedCount int           `json:"session_completed_count,omitempty"`
}

type RoadmapItem struct {
	Text       string `json:"text"`
	IsChecked  bool   `json:"is_checked"`
	LineNumber int    `json:"line_number"`
}

// ── claude_recovery_history ──

type RecoveryHistoryInput struct {
	Days int `json:"days,omitempty" jsonschema:"description=Look back N days. Defaults to 7."`
}

type RecoveryHistoryOutput struct {
	LookbackDays int             `json:"lookback_days"`
	Events       []RecoveryEvent `json:"events,omitempty"`
	Total        int             `json:"total"`
}

type RecoveryEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"` // crash, resume
	SessionID string `json:"session_id,omitempty"`
	RepoName  string `json:"repo_name,omitempty"`
	Display   string `json:"display,omitempty"`
}

// ── claude_session_search ──

type SessionSearchInput struct {
	Query      string `json:"query" jsonschema:"required,description=Keyword or regex pattern to search for across all session logs. Wrap in /slashes/ for regex."`
	Repo       string `json:"repo,omitempty" jsonschema:"description=Filter to a specific repo name (e.g. 'dotfiles-mcp')"`
	Window     string `json:"window,omitempty" jsonschema:"description=Time window (e.g. '7d'\\, '24h'). Default searches all sessions."`
	Status     string `json:"status,omitempty" jsonschema:"description=Filter by session status: alive\\, dead\\, or all (default all)"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"description=Max results to return. Default 10\\, max 50."`
}

type SessionSearchOutput struct {
	Query   string               `json:"query"`
	Results []SessionSearchMatch `json:"results"`
	Total   int                  `json:"total"`
	Scanned int                  `json:"files_scanned"`
	Elapsed string               `json:"elapsed"`
}

type SessionSearchMatch struct {
	SessionID    string   `json:"session_id"`
	Repo         string   `json:"repo"`
	RepoName     string   `json:"repo_name"`
	Title        string   `json:"title,omitempty"`
	Status       string   `json:"status"`
	LastActivity string   `json:"last_activity,omitempty"`
	Relevance    int      `json:"relevance"`
	Snippets     []string `json:"snippets"`
	ResumeCmd    string   `json:"resume_cmd"`
}

// ── claude_fleet_recovery ──

type FleetRecoveryInput struct {
	Window     string `json:"window,omitempty" jsonschema:"description=Time window (e.g. '4h'). Default '4h'."`
	StudioPath string `json:"studio_path,omitempty" jsonschema:"description=Path to studio. Default ~/hairglasses-studio."`
	Execute    bool   `json:"execute,omitempty" jsonschema:"description=Set to true to execute recovery. Default false (dry-run)."`
}

type FleetRecoveryOutput struct {
	TotalDeadSessions int            `json:"total_dead_sessions"`
	Steps             []RecoveryStep `json:"steps"`
	DryRun            bool           `json:"dry_run"`
	Summary           string         `json:"summary"`
	Warnings          []string       `json:"warnings,omitempty"`
}

type RecoveryStep struct {
	Rank      int    `json:"rank"`
	SessionID string `json:"session_id"`
	RepoName  string `json:"repo_name"`
	OpenTasks int    `json:"open_tasks"`
	Action    string `json:"action"` // stash, resume, verify
	Message   string `json:"message"`
	ResumeCmd string `json:"resume_cmd,omitempty"`
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
// Session search
// ---------------------------------------------------------------------------

// cwdFromJSONL extracts the CWD from the first user message in a JSONL file.
func cwdFromJSONL(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		// Quick check before full parse.
		if !strings.Contains(line, `"cwd"`) {
			continue
		}
		var entry struct {
			Type string `json:"type"`
			CWD  string `json:"cwd"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil && entry.CWD != "" {
			return entry.CWD
		}
	}
	return ""
}

// titleFromJSONL extracts the best title from a JSONL file (customTitle > agentName > slug).
func titleFromJSONL(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var slug string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"custom-title"`) || strings.Contains(line, `"customTitle"`) {
			var entry struct {
				Type        string `json:"type"`
				CustomTitle string `json:"customTitle"`
			}
			if json.Unmarshal([]byte(line), &entry) == nil && entry.CustomTitle != "" {
				return entry.CustomTitle
			}
		}
		if strings.Contains(line, `"agent-name"`) {
			var entry struct {
				Type      string `json:"type"`
				AgentName string `json:"agentName"`
			}
			if json.Unmarshal([]byte(line), &entry) == nil && entry.AgentName != "" {
				return entry.AgentName
			}
		}
		if slug == "" && strings.Contains(line, `"slug"`) {
			var entry struct {
				Slug string `json:"slug"`
			}
			if json.Unmarshal([]byte(line), &entry) == nil && entry.Slug != "" {
				slug = entry.Slug
			}
		}
	}
	return slug
}

// searchSessionFile scans a single JSONL file for keyword matches.
// Returns hit count and up to maxSnippets context snippets.
func searchSessionFile(path string, queryLower string, useRegex bool, re *regexp.Regexp, maxSnippets int) (int, []string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, nil
	}
	defer f.Close()

	hits := 0
	var snippets []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Fast pre-check: does the raw line contain the query at all?
		var matched bool
		if useRegex {
			matched = re.MatchString(line)
		} else {
			matched = strings.Contains(strings.ToLower(line), queryLower)
		}
		if !matched {
			continue
		}
		hits++

		// Extract a readable snippet from matching lines.
		if len(snippets) < maxSnippets {
			snippet := extractSnippet(line, queryLower)
			if snippet != "" {
				snippets = append(snippets, snippet)
			}
		}
	}
	return hits, snippets
}

// extractSnippet extracts a human-readable snippet from a matching JSONL line.
func extractSnippet(line, queryLower string) string {
	var entry map[string]any
	if json.Unmarshal([]byte(line), &entry) != nil {
		return ""
	}

	entryType, _ := entry["type"].(string)

	// Extract text content based on entry type.
	var text string
	switch entryType {
	case "user":
		if msg, ok := entry["message"].(map[string]any); ok {
			text, _ = msg["content"].(string)
		}
	case "assistant":
		if msg, ok := entry["message"].(map[string]any); ok {
			if content, ok := msg["content"].([]any); ok {
				for _, block := range content {
					if b, ok := block.(map[string]any); ok {
						if t, ok := b["text"].(string); ok {
							text = t
							break
						}
					}
				}
			}
		}
	case "custom-title":
		text, _ = entry["customTitle"].(string)
	case "agent-name":
		text, _ = entry["agentName"].(string)
	}

	if text == "" {
		return ""
	}

	// Find the query location and extract surrounding context.
	lower := strings.ToLower(text)
	idx := strings.Index(lower, queryLower)
	if idx < 0 {
		return truncate(text, 150)
	}

	// Window around the match.
	start := idx - 60
	if start < 0 {
		start = 0
	}
	end := idx + len(queryLower) + 60
	if end > len(text) {
		end = len(text)
	}
	snippet := text[start:end]
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	snippet = strings.Join(strings.Fields(snippet), " ")
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}
	return snippet
}

// searchSessions performs keyword search across all session JSONL files.
func searchSessions(query string, repoFilter string, windowDur time.Duration, statusFilter string, maxResults int) ([]SessionSearchMatch, int, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 50 {
		maxResults = 50
	}

	// Determine search mode.
	queryLower := strings.ToLower(query)
	var useRegex bool
	var re *regexp.Regexp
	if strings.HasPrefix(query, "/") && strings.HasSuffix(query, "/") && len(query) > 2 {
		pattern := query[1 : len(query)-1]
		var err error
		re, err = regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, 0, fmt.Errorf("[%s] invalid regex: %w", handler.ErrInvalidParam, err)
		}
		useRegex = true
	}

	// Load metadata for status checking and title enrichment.
	metas, _ := loadAllSessionMeta()
	metaBySession := make(map[string]claudeSessionMeta)
	for _, m := range metas {
		metaBySession[m.SessionID] = m
	}

	lastActivity, lastDisplay, _ := loadHistoryIndex()

	cutoff := time.Time{}
	if windowDur > 0 {
		cutoff = time.Now().Add(-windowDur)
	}

	// Discover all session JSONL files.
	projectsDir := filepath.Join(claudeDir(), "projects")
	projectDirs, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, 0, fmt.Errorf("[%s] cannot read projects dir: %w", handler.ErrNotFound, err)
	}

	type scanTarget struct {
		path      string
		sessionID string
	}
	var targets []scanTarget

	for _, pd := range projectDirs {
		if !pd.IsDir() {
			continue
		}
		pdPath := filepath.Join(projectsDir, pd.Name())
		files, err := os.ReadDir(pdPath)
		if err != nil {
			continue
		}
		for _, f := range files {
			name := f.Name()
			if !strings.HasSuffix(name, ".jsonl") || f.IsDir() {
				continue
			}
			sessionID := strings.TrimSuffix(name, ".jsonl")
			// Skip non-UUID filenames.
			if len(sessionID) < 36 {
				continue
			}
			targets = append(targets, scanTarget{
				path:      filepath.Join(pdPath, name),
				sessionID: sessionID,
			})
		}
	}

	// Parallel scan with semaphore.
	type scanResult struct {
		match SessionSearchMatch
		hits  int
	}
	var mu sync.Mutex
	var results []scanResult
	scanned := 0

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	for _, t := range targets {
		wg.Add(1)
		go func(tgt scanTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			mu.Lock()
			scanned++
			mu.Unlock()

			// Determine session status.
			meta, hasMeta := metaBySession[tgt.sessionID]
			status := "dead"
			if hasMeta && isProcessAlive(meta.PID) {
				status = "alive"
			}

			// Apply status filter.
			if statusFilter != "" && statusFilter != "all" && status != statusFilter {
				return
			}

			// Get CWD and repo info.
			cwd := ""
			if hasMeta {
				cwd = meta.CWD
			}
			if cwd == "" {
				cwd = cwdFromJSONL(tgt.path)
			}
			repoName := repoNameFromPath(cwd)

			// Apply repo filter.
			if repoFilter != "" && !strings.EqualFold(repoName, repoFilter) {
				return
			}

			// Apply time window filter.
			la := lastActivity[tgt.sessionID]
			if la.IsZero() && hasMeta {
				la = msToTime(float64(meta.StartedAt))
			}
			if la.IsZero() {
				if info, err := os.Stat(tgt.path); err == nil {
					la = info.ModTime()
				}
			}
			if !cutoff.IsZero() && la.Before(cutoff) {
				return
			}

			// Scan file for matches.
			hits, snippets := searchSessionFile(tgt.path, queryLower, useRegex, re, 3)
			if hits == 0 {
				return
			}

			// Build title.
			title := ""
			if hasMeta && meta.Name != "" {
				title = meta.Name
			}
			if title == "" {
				if d, ok := lastDisplay[tgt.sessionID]; ok {
					title = truncate(d, 80)
				}
			}
			if title == "" {
				title = titleFromJSONL(tgt.path)
			}

			// Build resume command.
			resumeCmd := fmt.Sprintf("claude --resume %s", tgt.sessionID)
			if cwd != "" {
				resumeCmd = fmt.Sprintf("cd %s && claude --resume %s", cwd, tgt.sessionID)
			}

			match := SessionSearchMatch{
				SessionID:    tgt.sessionID,
				Repo:         cwd,
				RepoName:     repoName,
				Title:        title,
				Status:       status,
				LastActivity: la.Format(time.RFC3339),
				Relevance:    hits,
				Snippets:     snippets,
				ResumeCmd:    resumeCmd,
			}

			mu.Lock()
			results = append(results, scanResult{match: match, hits: hits})
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	// Sort by relevance (hit count) descending, alive sessions boosted.
	sort.Slice(results, func(i, j int) bool {
		boostI, boostJ := 0, 0
		if results[i].match.Status == "alive" {
			boostI = 1000
		}
		if results[j].match.Status == "alive" {
			boostJ = 1000
		}
		return (results[i].hits + boostI) > (results[j].hits + boostJ)
	})

	// Trim to max results.
	total := len(results)
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	matches := make([]SessionSearchMatch, len(results))
	for i, r := range results {
		matches[i] = r.match
	}
	return matches, total, nil
}

// ---------------------------------------------------------------------------
// Module
// ---------------------------------------------------------------------------

type ClaudeSessionModule struct{}

func (m *ClaudeSessionModule) Name() string { return "claude_session" }
func (m *ClaudeSessionModule) Description() string {
	return "Claude Code session discovery, crash detection, and recovery analysis"
}

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

		// ── claude_session_health ─────────────────────
		handler.TypedHandler[SessionHealthInput, SessionHealthOutput](
			"claude_session_health",
			"Health score (0-100) for a Claude Code session across 5 dimensions: process_alive, tasks_completion, git_clean, recency, plan_progress. Returns per-dimension scores and actionable recommendations.",
			func(_ context.Context, input SessionHealthInput) (SessionHealthOutput, error) {
				if input.SessionID == "" {
					return SessionHealthOutput{}, fmt.Errorf("[%s] session_id is required", handler.ErrInvalidParam)
				}

				metas, err := loadAllSessionMeta()
				if err != nil {
					return SessionHealthOutput{}, fmt.Errorf("[%s] failed to load sessions: %w", handler.ErrNotFound, err)
				}

				var meta *claudeSessionMeta
				for _, m := range metas {
					if m.SessionID == input.SessionID {
						meta = &m
						break
					}
				}
				if meta == nil {
					return SessionHealthOutput{}, fmt.Errorf("[%s] session %s not found", handler.ErrNotFound, input.SessionID)
				}

				out := SessionHealthOutput{SessionID: meta.SessionID, IsAlive: isProcessAlive(meta.PID)}

				var dims []HealthDimension
				var total int

				// Dimension 1: Process alive.
				procScore := 0
				if out.IsAlive {
					procScore = 100
				}
				dims = append(dims, HealthDimension{Category: "process_alive", Score: procScore, Detail: fmt.Sprintf("PID %d", meta.PID)})
				total += procScore

				// Dimension 2: Task completion.
				tasks, _ := loadSessionTasks(meta.SessionID)
				taskScore := 100
				openCount := 0
				for _, t := range tasks {
					if t.Status != "completed" {
						openCount++
					}
				}
				if len(tasks) > 0 {
					taskScore = (len(tasks) - openCount) * 100 / len(tasks)
				}
				dims = append(dims, HealthDimension{Category: "tasks_completion", Score: taskScore, Detail: fmt.Sprintf("%d/%d completed", len(tasks)-openCount, len(tasks))})
				total += taskScore

				// Dimension 3: Git clean.
				gitScore := 100
				repoStatus, _ := getRepoStatus(meta.CWD)
				if repoStatus.IsGitRepo {
					if repoStatus.HasUncommitted {
						gitScore -= 40
					}
					if repoStatus.UntrackedCount > 5 {
						gitScore -= 20
					}
					if gitScore < 0 {
						gitScore = 0
					}
				}
				dims = append(dims, HealthDimension{Category: "git_clean", Score: gitScore, Detail: fmt.Sprintf("staged=%d unstaged=%d untracked=%d", repoStatus.StagedCount, repoStatus.UnstagedCount, repoStatus.UntrackedCount)})
				total += gitScore

				// Dimension 4: Recency.
				lastActivity, _, _ := loadHistoryIndex()
				recencyScore := 0
				la := lastActivity[meta.SessionID]
				if !la.IsZero() {
					age := time.Since(la)
					switch {
					case age < 1*time.Hour:
						recencyScore = 100
					case age < 4*time.Hour:
						recencyScore = 75
					case age < 24*time.Hour:
						recencyScore = 50
					case age < 7*24*time.Hour:
						recencyScore = 25
					}
				}
				dims = append(dims, HealthDimension{Category: "recency", Score: recencyScore, Detail: fmt.Sprintf("last active: %s", la.Format(time.RFC3339))})
				total += recencyScore

				// Dimension 5: Plan progress.
				planScore := 50 // neutral if no plan
				projectDir := findSessionProjectDir(meta.CWD)
				if projectDir != "" {
					planFile := findRecentPlanFile(projectDir, meta.SessionID)
					if planFile != "" {
						planScore = 75 // has a plan
						if taskScore > 50 {
							planScore = 90 // plan + good task progress
						}
					}
				}
				dims = append(dims, HealthDimension{Category: "plan_progress", Score: planScore, Detail: ""})
				total += planScore

				out.Dimensions = dims
				out.OverallScore = total / len(dims)

				// Recommendations.
				if procScore == 0 {
					out.Recommendations = append(out.Recommendations, fmt.Sprintf("Session is dead. Resume with: claude --resume %s", meta.SessionID))
				}
				if openCount > 0 {
					out.Recommendations = append(out.Recommendations, fmt.Sprintf("%d tasks still open — review task list", openCount))
				}
				if repoStatus.HasUncommitted {
					out.Recommendations = append(out.Recommendations, "Uncommitted changes detected — commit or stash")
				}
				if recencyScore < 50 {
					out.Recommendations = append(out.Recommendations, "Session inactive for >4 hours — check if work is complete")
				}

				return out, nil
			},
		),

		// ── claude_session_compare ────────────────────
		handler.TypedHandler[SessionCompareInput, SessionCompareOutput](
			"claude_session_compare",
			"Compare progress between two Claude Code sessions: task completion delta, commit delta, file change count, and time gap.",
			func(_ context.Context, input SessionCompareInput) (SessionCompareOutput, error) {
				if input.SessionID1 == "" || input.SessionID2 == "" {
					return SessionCompareOutput{}, fmt.Errorf("[%s] both session_id_1 and session_id_2 are required", handler.ErrInvalidParam)
				}

				metas, err := loadAllSessionMeta()
				if err != nil {
					return SessionCompareOutput{}, fmt.Errorf("[%s] failed to load sessions: %w", handler.ErrNotFound, err)
				}

				findMeta := func(id string) *claudeSessionMeta {
					for _, m := range metas {
						if m.SessionID == id {
							return &m
						}
					}
					return nil
				}

				m1 := findMeta(input.SessionID1)
				m2 := findMeta(input.SessionID2)
				if m1 == nil {
					return SessionCompareOutput{}, fmt.Errorf("[%s] session %s not found", handler.ErrNotFound, input.SessionID1)
				}
				if m2 == nil {
					return SessionCompareOutput{}, fmt.Errorf("[%s] session %s not found", handler.ErrNotFound, input.SessionID2)
				}

				out := SessionCompareOutput{
					SessionID1: input.SessionID1,
					SessionID2: input.SessionID2,
				}

				// Task comparison.
				tasks1, _ := loadSessionTasks(input.SessionID1)
				tasks2, _ := loadSessionTasks(input.SessionID2)
				completed1, completed2 := 0, 0
				for _, t := range tasks1 {
					if t.Status == "completed" {
						completed1++
					}
				}
				for _, t := range tasks2 {
					if t.Status == "completed" {
						completed2++
					}
				}
				out.Diff.Session1Tasks = len(tasks1)
				out.Diff.Session1Completed = completed1
				out.Diff.Session2Tasks = len(tasks2)
				out.Diff.Session2Completed = completed2
				out.Diff.TaskCompletionDelta = completed2 - completed1

				// Time comparison.
				lastActivity, _, _ := loadHistoryIndex()
				la1, la2 := lastActivity[input.SessionID1], lastActivity[input.SessionID2]
				if !la1.IsZero() && !la2.IsZero() {
					out.Diff.TimeBetween = la2.Sub(la1).String()
				}

				// Repo comparison (if same repo).
				if m1.CWD == m2.CWD {
					out.Repo = m1.CWD
					out.SameRepo = true
				} else {
					out.Repo = fmt.Sprintf("%s vs %s", repoNameFromPath(m1.CWD), repoNameFromPath(m2.CWD))
				}

				// Status.
				out.Diff.Session1Status = "dead"
				if isProcessAlive(m1.PID) {
					out.Diff.Session1Status = "alive"
				}
				out.Diff.Session2Status = "dead"
				if isProcessAlive(m2.PID) {
					out.Diff.Session2Status = "alive"
				}

				return out, nil
			},
		),

		// ── claude_session_replay ─────────────────────
		handler.TypedHandler[SessionReplayInput, SessionReplayOutput](
			"claude_session_replay",
			"Reconstruct the conversation thread from a session's JSONL log as user/assistant exchange pairs with tool use summaries.",
			func(_ context.Context, input SessionReplayInput) (SessionReplayOutput, error) {
				if input.SessionID == "" {
					return SessionReplayOutput{}, fmt.Errorf("[%s] session_id is required", handler.ErrInvalidParam)
				}
				maxExchanges := input.MaxExchanges
				if maxExchanges <= 0 {
					maxExchanges = 50
				}

				metas, err := loadAllSessionMeta()
				if err != nil {
					return SessionReplayOutput{}, fmt.Errorf("[%s] failed to load sessions: %w", handler.ErrNotFound, err)
				}

				var meta *claudeSessionMeta
				for _, m := range metas {
					if m.SessionID == input.SessionID {
						meta = &m
						break
					}
				}
				if meta == nil {
					return SessionReplayOutput{}, fmt.Errorf("[%s] session %s not found", handler.ErrNotFound, input.SessionID)
				}

				projectDir := findSessionProjectDir(meta.CWD)
				if projectDir == "" {
					return SessionReplayOutput{}, fmt.Errorf("[%s] project directory not found", handler.ErrNotFound)
				}

				jsonlPath := findSessionJSONL(projectDir, meta.SessionID)
				if jsonlPath == "" {
					return SessionReplayOutput{}, fmt.Errorf("[%s] session JSONL not found", handler.ErrNotFound)
				}

				allEntries, err := readJSONLAll(jsonlPath)
				if err != nil {
					return SessionReplayOutput{}, fmt.Errorf("failed to read JSONL: %w", err)
				}

				// Build conversation exchanges.
				var exchanges []ConversationExchange
				order := 0
				var currentExchange *ConversationExchange

				for _, entry := range allEntries {
					entryType, _ := entry["type"].(string)
					ts, _ := entry["timestamp"].(string)

					switch entryType {
					case "user":
						// Start new exchange.
						if currentExchange != nil {
							exchanges = append(exchanges, *currentExchange)
						}
						order++
						currentExchange = &ConversationExchange{Order: order, Timestamp: ts}
						if msg, ok := entry["message"].(map[string]any); ok {
							if content, ok := msg["content"].(string); ok {
								currentExchange.UserText = truncate(content, 300)
							}
						}

					case "assistant":
						if currentExchange == nil {
							order++
							currentExchange = &ConversationExchange{Order: order, Timestamp: ts}
						}
						if msg, ok := entry["message"].(map[string]any); ok {
							if content, ok := msg["content"].([]any); ok {
								for _, block := range content {
									if b, ok := block.(map[string]any); ok {
										switch b["type"] {
										case "text":
											text, _ := b["text"].(string)
											currentExchange.AssistantSummary = truncate(text, 300)
										case "tool_use":
											name, _ := b["name"].(string)
											currentExchange.ToolsUsed = append(currentExchange.ToolsUsed, name)
										}
									}
								}
							}
						}
					}
				}
				// Flush last exchange.
				if currentExchange != nil {
					exchanges = append(exchanges, *currentExchange)
				}

				// Take last N exchanges.
				if len(exchanges) > maxExchanges {
					exchanges = exchanges[len(exchanges)-maxExchanges:]
				}

				return SessionReplayOutput{
					SessionID:  input.SessionID,
					Repo:       meta.CWD,
					TotalLines: len(allEntries),
					Exchanges:  exchanges,
				}, nil
			},
		),

		// ── claude_workspace_snapshot ─────────────────
		handler.TypedHandler[WorkspaceSnapshotInput, WorkspaceSnapshotOutput](
			"claude_workspace_snapshot",
			"Capture a point-in-time snapshot of all Claude Code sessions and repo git states across the studio directory. Saves to ~/.claude/snapshots/ for later comparison.",
			func(_ context.Context, input WorkspaceSnapshotInput) (WorkspaceSnapshotOutput, error) {
				studioPath := input.StudioPath
				if studioPath == "" {
					studioPath = filepath.Join(homeDir(), "hairglasses-studio")
				}
				if strings.HasPrefix(studioPath, "~/") {
					studioPath = filepath.Join(homeDir(), studioPath[2:])
				}

				// Scan sessions.
				sessions, err := scanSessions(0)
				if err != nil {
					return WorkspaceSnapshotOutput{}, err
				}

				// Scan repos (parallel, semaphore=8).
				var repoPaths []string
				if entries, err := os.ReadDir(studioPath); err == nil {
					for _, e := range entries {
						if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
							repoPaths = append(repoPaths, filepath.Join(studioPath, e.Name()))
						}
					}
				}

				type repoSnap struct {
					path   string
					status RepoStatusOutput
				}
				var mu sync.Mutex
				var repoSnaps []repoSnap
				sem := make(chan struct{}, 8)
				var wg sync.WaitGroup
				for _, rp := range repoPaths {
					rp := rp
					wg.Add(1)
					go func() {
						defer wg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()
						status, _ := getRepoStatus(rp)
						if status.IsGitRepo {
							mu.Lock()
							repoSnaps = append(repoSnaps, repoSnap{path: rp, status: status})
							mu.Unlock()
						}
					}()
				}
				wg.Wait()

				sort.Slice(repoSnaps, func(i, j int) bool {
					return repoSnaps[i].status.RepoName < repoSnaps[j].status.RepoName
				})

				// Build snapshot.
				snap := WorkspaceSnapshot{
					Timestamp:  time.Now().Format(time.RFC3339),
					StudioPath: studioPath,
					SnapshotID: fmt.Sprintf("%d", time.Now().UnixMilli()),
					Sessions:   sessions,
				}
				for _, rs := range repoSnaps {
					snap.Repos = append(snap.Repos, rs.status)
				}

				// Save to disk.
				snapDir := filepath.Join(claudeDir(), "snapshots")
				if err := os.MkdirAll(snapDir, 0755); err != nil {
					return WorkspaceSnapshotOutput{}, fmt.Errorf("failed to create snapshots dir: %w", err)
				}

				snapFile := filepath.Join(snapDir, fmt.Sprintf("%s.json", snap.SnapshotID))
				data, err := json.MarshalIndent(snap, "", "  ")
				if err != nil {
					return WorkspaceSnapshotOutput{}, fmt.Errorf("failed to marshal snapshot: %w", err)
				}
				if err := os.WriteFile(snapFile, data, 0644); err != nil {
					return WorkspaceSnapshotOutput{}, fmt.Errorf("failed to write snapshot: %w", err)
				}

				return WorkspaceSnapshotOutput{
					SnapshotID:   snap.SnapshotID,
					SavedTo:      snapFile,
					SessionCount: len(sessions),
					RepoCount:    len(repoSnaps),
					Size:         int64(len(data)),
				}, nil
			},
		),

		// ── claude_repo_roadmap_status ────────────────
		handler.TypedHandler[RepoRoadmapInput, RepoRoadmapOutput](
			"claude_repo_roadmap_status",
			"Parse ROADMAP.md checkbox items and compare completion against a session's task list. Returns progress percentage and unchecked items.",
			func(_ context.Context, input RepoRoadmapInput) (RepoRoadmapOutput, error) {
				if input.RepoPath == "" {
					return RepoRoadmapOutput{}, fmt.Errorf("[%s] repo_path is required", handler.ErrInvalidParam)
				}

				out := RepoRoadmapOutput{RepoPath: input.RepoPath, RepoName: repoNameFromPath(input.RepoPath)}

				// Read ROADMAP.md.
				roadmapPath := filepath.Join(input.RepoPath, "ROADMAP.md")
				data, err := os.ReadFile(roadmapPath)
				if err != nil {
					return RepoRoadmapOutput{}, fmt.Errorf("[%s] ROADMAP.md not found in %s", handler.ErrNotFound, input.RepoPath)
				}

				// Parse checkbox items.
				scanner := bufio.NewScanner(strings.NewReader(string(data)))
				lineNum := 0
				for scanner.Scan() {
					lineNum++
					line := strings.TrimSpace(scanner.Text())
					if strings.HasPrefix(line, "- [x]") || strings.HasPrefix(line, "- [X]") {
						text := strings.TrimSpace(line[5:])
						out.Items = append(out.Items, RoadmapItem{Text: text, IsChecked: true, LineNumber: lineNum})
						out.CompletedCount++
					} else if strings.HasPrefix(line, "- [ ]") {
						text := strings.TrimSpace(line[5:])
						out.Items = append(out.Items, RoadmapItem{Text: text, IsChecked: false, LineNumber: lineNum})
					}
				}
				out.TotalCount = len(out.Items)
				if out.TotalCount > 0 {
					out.ProgressPercent = out.CompletedCount * 100 / out.TotalCount
				}

				// If session provided, compare tasks.
				if input.SessionID != "" {
					tasks, _ := loadSessionTasks(input.SessionID)
					out.SessionTaskCount = len(tasks)
					sessionCompleted := 0
					for _, t := range tasks {
						if t.Status == "completed" {
							sessionCompleted++
						}
					}
					out.SessionCompletedCount = sessionCompleted
				}

				return out, nil
			},
		),

		// ── claude_recovery_history ───────────────────
		handler.TypedHandler[RecoveryHistoryInput, RecoveryHistoryOutput](
			"claude_recovery_history",
			"Audit trail of crash and recovery events extracted from history.jsonl. Shows crash timestamps, resume attempts, and session outcomes.",
			func(_ context.Context, input RecoveryHistoryInput) (RecoveryHistoryOutput, error) {
				days := input.Days
				if days <= 0 {
					days = 7
				}
				cutoff := time.Now().AddDate(0, 0, -days)

				histPath := filepath.Join(claudeDir(), "history.jsonl")
				entries, err := readJSONLAll(histPath)
				if err != nil {
					if os.IsNotExist(err) {
						return RecoveryHistoryOutput{LookbackDays: days}, nil
					}
					return RecoveryHistoryOutput{}, fmt.Errorf("failed to read history: %w", err)
				}

				var events []RecoveryEvent
				for _, entry := range entries {
					ts, _ := entry["timestamp"].(float64)
					if ts == 0 {
						continue
					}
					t := msToTime(ts)
					if t.Before(cutoff) {
						continue
					}

					display, _ := entry["display"].(string)
					sid, _ := entry["sessionId"].(string)
					project, _ := entry["project"].(string)
					lower := strings.ToLower(display)

					var eventType string
					if strings.Contains(lower, "session crashed") || strings.Contains(lower, "crash") {
						eventType = "crash"
					} else if strings.Contains(lower, "--resume") || strings.Contains(lower, "resume") {
						eventType = "resume"
					} else {
						continue
					}

					events = append(events, RecoveryEvent{
						Timestamp: t.Format(time.RFC3339),
						Type:      eventType,
						SessionID: sid,
						RepoName:  repoNameFromPath(project),
						Display:   truncate(display, 200),
					})
				}

				return RecoveryHistoryOutput{
					LookbackDays: days,
					Events:       events,
					Total:        len(events),
				}, nil
			},
		),

		// ── claude_fleet_recovery ─────────────────────
		handler.TypedHandler[FleetRecoveryInput, FleetRecoveryOutput](
			"claude_fleet_recovery",
			"Composed recovery workflow: detect dead sessions across all repos, analyze git state, generate recovery steps with resume commands. Dry-run by default — set execute=true to trigger resumes.",
			func(_ context.Context, input FleetRecoveryInput) (FleetRecoveryOutput, error) {
				window := input.Window
				if window == "" {
					window = "4h"
				}
				studioPath := input.StudioPath
				if studioPath == "" {
					studioPath = filepath.Join(homeDir(), "hairglasses-studio")
				}
				if strings.HasPrefix(studioPath, "~/") {
					studioPath = filepath.Join(homeDir(), studioPath[2:])
				}

				windowDur, err := parseDurationString(window)
				if err != nil {
					return FleetRecoveryOutput{}, fmt.Errorf("[%s] invalid window: %w", handler.ErrInvalidParam, err)
				}

				// Step 1: Scan sessions.
				sessions, err := scanSessions(windowDur)
				if err != nil {
					return FleetRecoveryOutput{}, err
				}

				out := FleetRecoveryOutput{DryRun: !input.Execute}
				var steps []RecoveryStep
				rank := 0

				// Collect dead sessions with enrichment.
				type enrichedSession struct {
					scan  SessionScanEntry
					tasks []SessionTask
					repo  RepoStatusOutput
				}
				var dead []enrichedSession
				for _, s := range sessions {
					if s.Status != "dead" {
						continue
					}
					tasks, _ := loadSessionTasks(s.SessionID)
					repoStatus, _ := getRepoStatus(s.Repo)
					dead = append(dead, enrichedSession{scan: s, tasks: tasks, repo: repoStatus})
				}

				// Sort by open tasks descending.
				sort.Slice(dead, func(i, j int) bool {
					openI, openJ := 0, 0
					for _, t := range dead[i].tasks {
						if t.Status != "completed" {
							openI++
						}
					}
					for _, t := range dead[j].tasks {
						if t.Status != "completed" {
							openJ++
						}
					}
					return openI > openJ
				})

				out.TotalDeadSessions = len(dead)

				for _, d := range dead {
					rank++
					openTasks := 0
					for _, t := range d.tasks {
						if t.Status != "completed" {
							openTasks++
						}
					}

					// Step: Stash if dirty.
					if d.repo.HasUncommitted {
						steps = append(steps, RecoveryStep{
							Rank:      rank,
							SessionID: d.scan.SessionID,
							RepoName:  d.scan.RepoName,
							OpenTasks: openTasks,
							Action:    "stash",
							Message:   fmt.Sprintf("Stash %d uncommitted changes", d.repo.StagedCount+d.repo.UnstagedCount),
						})
						out.Warnings = append(out.Warnings, fmt.Sprintf("%s: has uncommitted changes — stash before resume", d.scan.RepoName))
					}

					// Step: Resume.
					steps = append(steps, RecoveryStep{
						Rank:      rank,
						SessionID: d.scan.SessionID,
						RepoName:  d.scan.RepoName,
						OpenTasks: openTasks,
						Action:    "resume",
						Message:   fmt.Sprintf("Resume session with %d open tasks", openTasks),
						ResumeCmd: fmt.Sprintf("claude --resume %s", d.scan.SessionID),
					})
				}

				out.Steps = steps
				if len(dead) == 0 {
					out.Summary = "No dead sessions found — fleet is healthy."
				} else {
					out.Summary = fmt.Sprintf("%d dead sessions detected. %d recovery steps planned.", len(dead), len(steps))
				}

				return out, nil
			},
		),

		// ── claude_session_search ─────────────────────
		handler.TypedHandler[SessionSearchInput, SessionSearchOutput](
			"claude_session_search",
			"Full-text keyword search across all Claude Code session JSONL logs. Finds sessions by topic, tool names, or content. Returns ranked matches with context snippets and resume commands.",
			func(_ context.Context, input SessionSearchInput) (SessionSearchOutput, error) {
				if input.Query == "" {
					return SessionSearchOutput{}, fmt.Errorf("[%s] query is required", handler.ErrInvalidParam)
				}

				maxResults := input.MaxResults
				if maxResults <= 0 {
					maxResults = 10
				}

				var windowDur time.Duration
				if input.Window != "" {
					var err error
					windowDur, err = parseDurationString(input.Window)
					if err != nil {
						return SessionSearchOutput{}, fmt.Errorf("[%s] invalid window: %w", handler.ErrInvalidParam, err)
					}
				}

				start := time.Now()
				matches, total, err := searchSessions(input.Query, input.Repo, windowDur, input.Status, maxResults)
				if err != nil {
					return SessionSearchOutput{}, err
				}
				elapsed := time.Since(start)

				// Count scanned files for reporting.
				projectsDir := filepath.Join(claudeDir(), "projects")
				scanned := 0
				if pds, err := os.ReadDir(projectsDir); err == nil {
					for _, pd := range pds {
						if !pd.IsDir() {
							continue
						}
						if files, err := os.ReadDir(filepath.Join(projectsDir, pd.Name())); err == nil {
							for _, f := range files {
								if strings.HasSuffix(f.Name(), ".jsonl") && !f.IsDir() {
									scanned++
								}
							}
						}
					}
				}

				return SessionSearchOutput{
					Query:   input.Query,
					Results: matches,
					Total:   total,
					Scanned: scanned,
					Elapsed: elapsed.Round(time.Millisecond).String(),
				}, nil
			},
		),
	}
}

// ---------------------------------------------------------------------------
// Session Index for ccg (CLI mode: --session-index)
// ---------------------------------------------------------------------------

// SessionIndexEntry is the output schema matching ccg.sh's expected format.
type SessionIndexEntry struct {
	SessionID    string `json:"sessionId"`
	CWD          string `json:"cwd"`
	Repo         string `json:"repo"`
	Title        string `json:"title"`
	Branch       string `json:"branch"`
	Model        string `json:"model"`
	Version      string `json:"version"`
	Status       string `json:"status"`
	PID          int    `json:"pid"`
	LastActivity int64  `json:"lastActivity"`
	FileSize     int64  `json:"fileSize"`
	OpenTasks    int    `json:"openTasks"`
	TotalTasks   int    `json:"totalTasks"`
	Slug         string `json:"slug"`
	CustomTitle  string `json:"customTitle"`
}

// extractJSONLMeta reads the first ~200 lines of a session JSONL file to extract
// metadata without parsing the entire file.
func extractJSONLMeta(path string) (customTitle, cwd, branch, version, model, slug string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	linesRead := 0
	gotUser, gotAssistant := false, false

	for scanner.Scan() && linesRead < 200 {
		linesRead++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		lineStr := string(line)

		if customTitle == "" && strings.Contains(lineStr, `"custom-title"`) {
			var entry struct {
				Type        string `json:"type"`
				CustomTitle string `json:"customTitle"`
			}
			if json.Unmarshal(line, &entry) == nil && entry.Type == "custom-title" {
				customTitle = entry.CustomTitle
			}
		}
		if !gotUser && strings.Contains(lineStr, `"type":"user"`) {
			var entry struct {
				Type      string `json:"type"`
				CWD       string `json:"cwd"`
				GitBranch string `json:"gitBranch"`
				Version   string `json:"version"`
			}
			if json.Unmarshal(line, &entry) == nil && entry.Type == "user" {
				cwd = entry.CWD
				branch = entry.GitBranch
				version = entry.Version
				gotUser = true
			}
		}
		if !gotAssistant && strings.Contains(lineStr, `"type":"assistant"`) {
			var entry struct {
				Type    string `json:"type"`
				Slug    string `json:"slug"`
				Message struct {
					Model string `json:"model"`
				} `json:"message"`
			}
			if json.Unmarshal(line, &entry) == nil && entry.Type == "assistant" {
				model = entry.Message.Model
				slug = entry.Slug
				gotAssistant = true
			}
		}
		if gotUser && gotAssistant && customTitle != "" {
			break
		}
	}
	return
}

// buildSessionIndex produces the full session index for ccg.
func buildSessionIndex() ([]SessionIndexEntry, error) {
	sessions, err := loadAllSessionMeta()
	if err != nil {
		return nil, fmt.Errorf("load session meta: %w", err)
	}

	lastActivity, lastDisplay, err := loadHistoryIndex()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}

	type metaInfo struct {
		PID       int
		Name      string
		StartedAt int64
		CWD       string
	}
	metaByID := make(map[string]metaInfo, len(sessions))
	for _, s := range sessions {
		metaByID[s.SessionID] = metaInfo{PID: s.PID, Name: s.Name, StartedAt: s.StartedAt, CWD: s.CWD}
	}

	projectsDir := filepath.Join(claudeDir(), "projects")
	dirEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("read projects: %w", err)
	}

	type fileEntry struct {
		path, sessionID string
		size, mtime     int64
	}
	var files []fileEntry
	for _, de := range dirEntries {
		if !de.IsDir() || strings.HasPrefix(de.Name(), "-tmp-") {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(projectsDir, de.Name(), "*.jsonl"))
		for _, m := range matches {
			sid := strings.TrimSuffix(filepath.Base(m), ".jsonl")
			if len(sid) < 36 {
				continue
			}
			info, err := os.Stat(m)
			if err != nil {
				continue
			}
			files = append(files, fileEntry{path: m, sessionID: sid, size: info.Size(), mtime: info.ModTime().Unix()})
		}
	}

	type indexResult struct {
		entry SessionIndexEntry
		ok    bool
	}
	results := make([]indexResult, len(files))
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup

	for i, fi := range files {
		i, fi := i, fi
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			customTitle, cwdVal, branch, version, model, slug := extractJSONLMeta(fi.path)
			if cwdVal == "" {
				if m, ok := metaByID[fi.sessionID]; ok {
					cwdVal = m.CWD
				}
			}
			if cwdVal == "" {
				return
			}

			lastAct := fi.mtime
			if t, ok := lastActivity[fi.sessionID]; ok {
				lastAct = t.Unix()
			}

			title := customTitle
			if title == "" {
				if m, ok := metaByID[fi.sessionID]; ok {
					title = m.Name
				}
			}
			if title == "" {
				title = slug
			}
			if title == "" {
				if d, ok := lastDisplay[fi.sessionID]; ok && len(d) > 0 {
					if len(d) > 60 {
						d = d[:60]
					}
					title = d
				}
			}
			if title == "" {
				title = "(untitled)"
			}

			pid := 0
			status := "dead"
			if m, ok := metaByID[fi.sessionID]; ok {
				pid = m.PID
				if isProcessAlive(pid) {
					status = "alive"
				}
			}

			tasks, _ := loadSessionTasks(fi.sessionID)
			openTasks, totalTasks := 0, len(tasks)
			for _, t := range tasks {
				if t.Status != "completed" {
					openTasks++
				}
			}

			repo := repoNameFromPath(cwdVal)
			studio := filepath.Join(homeDir(), "hairglasses-studio")
			if strings.HasPrefix(cwdVal, studio+"/") {
				rel := strings.TrimPrefix(cwdVal, studio+"/")
				if idx := strings.IndexByte(rel, '/'); idx >= 0 {
					repo = rel[:idx]
				} else {
					repo = rel
				}
			} else if cwdVal == studio {
				repo = "hairglasses-studio"
			}

			results[i] = indexResult{ok: true, entry: SessionIndexEntry{
				SessionID: fi.sessionID, CWD: cwdVal, Repo: repo, Title: title,
				Branch: branch, Model: model, Version: version, Status: status,
				PID: pid, LastActivity: lastAct, FileSize: fi.size,
				OpenTasks: openTasks, TotalTasks: totalTasks, Slug: slug, CustomTitle: customTitle,
			}}
		}()
	}
	wg.Wait()

	var index []SessionIndexEntry
	for _, r := range results {
		if r.ok {
			index = append(index, r.entry)
		}
	}
	sort.Slice(index, func(i, j int) bool {
		return index[i].LastActivity > index[j].LastActivity
	})
	return index, nil
}

// outputSessionIndex writes the session index as JSONL to stdout.
func outputSessionIndex() error {
	index, err := buildSessionIndex()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	for _, entry := range index {
		if err := enc.Encode(entry); err != nil {
			return err
		}
	}
	return nil
}
