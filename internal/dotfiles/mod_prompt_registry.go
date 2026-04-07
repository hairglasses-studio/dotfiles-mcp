// mod_prompt_registry.go — Prompt capture, search, scoring, tagging, and improvement MCP tools.
// Stores prompts in ~/hairglasses-studio/docs/prompts/ with TOML frontmatter.
// Uses the prompt-improver enhancer for analysis and improvement.
package dotfiles

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/prompt-improver/pkg/enhancer"
)

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

// promptsBaseDir returns the base directory for prompt storage.
// It is a variable so tests can override it.
var promptsBaseDir = func() string {
	return filepath.Join(homeDir(), "hairglasses-studio", "docs", "prompts")
}

func promptIndexPath() string {
	return promptsBaseDir() + "/.prompt-index.jsonl"
}

// ---------------------------------------------------------------------------
// PromptRecord — stored metadata for a single prompt
// ---------------------------------------------------------------------------

type PromptRecord struct {
	Hash         string   `json:"hash"`
	ShortHash    string   `json:"short_hash"`
	Repo         string   `json:"repo"`
	Timestamp    string   `json:"timestamp"`
	SessionID    string   `json:"session_id,omitempty"`
	WordCount    int      `json:"word_count"`
	TaskType     string   `json:"task_type"`
	Score        int      `json:"score"`
	Grade        string   `json:"grade"`
	Tags         []string `json:"tags"`
	Status       string   `json:"status"` // unsorted, scored, improved, archived
	OriginalHash string   `json:"original_hash,omitempty"`
	Improvements []string `json:"improvements,omitempty"`
	StagesRun    []string `json:"stages_run,omitempty"`
	Prompt       string   `json:"prompt"` // full text in memory, truncated in JSONL
}

// ---------------------------------------------------------------------------
// PromptIndex — in-memory index loaded from JSONL
// ---------------------------------------------------------------------------

type PromptIndex struct {
	mu      sync.RWMutex
	records map[string]*PromptRecord // full hash -> record
	loaded  bool
}

var globalPromptIndex = &PromptIndex{
	records: make(map[string]*PromptRecord),
}

// load reads the JSONL index file into memory. Idempotent.
func (idx *PromptIndex) load() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.loaded {
		return nil
	}
	path := promptIndexPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			idx.loaded = true
			return nil
		}
		return err
	}
	var corruptCount int
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec PromptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			corruptCount++
			// Append corrupt line to recovery file
			corruptPath := path + ".corrupt"
			entry := fmt.Sprintf("[%s] line %d: %s\n%s\n", time.Now().UTC().Format(time.RFC3339), i+1, err, line)
			_ = os.WriteFile(corruptPath, append(readFileOrEmpty(corruptPath), []byte(entry)...), 0644)
			continue
		}
		idx.records[rec.Hash] = &rec
	}
	if corruptCount > 0 {
		slog.Warn("prompt index: skipped corrupt lines", "count", corruptCount, "path", path)
	}
	idx.loaded = true
	return nil
}

// add inserts a record into the in-memory index and appends to JSONL with file locking.
func (idx *PromptIndex) add(rec *PromptRecord) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.records[rec.Hash] = rec

	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(promptIndexPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	// Exclusive flock for cross-process safety (capture hook may write concurrently)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("flock: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

// update replaces a record in memory and rewrites the JSONL.
func (idx *PromptIndex) update(rec *PromptRecord) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.records[rec.Hash] = rec
	return idx.rewriteJSONL()
}

// rewriteJSONL writes all records back to the index file. Must hold mu.Lock().
func (idx *PromptIndex) rewriteJSONL() error {
	path := promptIndexPath()
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	for _, rec := range idx.records {
		line, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		fmt.Fprintf(f, "%s\n", line)
	}
	f.Close()
	return os.Rename(tmp, path)
}

// get returns a record by full or short hash.
func (idx *PromptIndex) get(hash string) *PromptRecord {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if rec, ok := idx.records[hash]; ok {
		return rec
	}
	// Try short hash prefix match
	for _, rec := range idx.records {
		if strings.HasPrefix(rec.Hash, hash) || rec.ShortHash == hash {
			return rec
		}
	}
	return nil
}

// search returns records matching the given filters.
func (idx *PromptIndex) search(query, repo, status string, tags []string, minScore, limit int) []*PromptRecord {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []*PromptRecord
	queryLower := strings.ToLower(query)

	for _, rec := range idx.records {
		// Repo filter
		if repo != "" && rec.Repo != repo {
			continue
		}
		// Status filter
		if status != "" && rec.Status != status {
			continue
		}
		// Score filter
		if minScore > 0 && rec.Score < minScore {
			continue
		}
		// Tag filter (AND)
		if len(tags) > 0 {
			tagSet := make(map[string]bool, len(rec.Tags))
			for _, t := range rec.Tags {
				tagSet[t] = true
			}
			allMatch := true
			for _, t := range tags {
				if !tagSet[t] {
					allMatch = false
					break
				}
			}
			if !allMatch {
				continue
			}
		}
		// Full-text query
		if query != "" && !strings.Contains(strings.ToLower(rec.Prompt), queryLower) {
			continue
		}
		results = append(results, rec)
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp > results[j].Timestamp
	})

	if limit <= 0 {
		limit = 20
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// stats returns aggregate statistics.
func (idx *PromptIndex) stats(repoFilter string) promptStatsResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	out := promptStatsResult{
		ByRepo:     make(map[string]int),
		ByStatus:   make(map[string]int),
		ByTaskType: make(map[string]int),
		ByGrade:    make(map[string]int),
	}

	var totalScore int
	cutoff24h := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	tagCounts := make(map[string]int)

	for _, rec := range idx.records {
		if repoFilter != "" && rec.Repo != repoFilter {
			continue
		}
		out.TotalPrompts++
		out.ByRepo[rec.Repo]++
		out.ByStatus[rec.Status]++
		if rec.TaskType != "" {
			out.ByTaskType[rec.TaskType]++
		}
		if rec.Grade != "" {
			out.ByGrade[rec.Grade]++
		}
		totalScore += rec.Score
		for _, t := range rec.Tags {
			tagCounts[t]++
		}
		if rec.Timestamp >= cutoff24h {
			out.RecentCaptures24h++
		}
	}

	if out.TotalPrompts > 0 {
		out.AverageScore = float64(totalScore) / float64(out.TotalPrompts)
	}

	// Top tags (sorted by count, top 15)
	type tc struct {
		Tag   string
		Count int
	}
	var tcs []tc
	for t, c := range tagCounts {
		tcs = append(tcs, tc{t, c})
	}
	sort.Slice(tcs, func(i, j int) bool { return tcs[i].Count > tcs[j].Count })
	for i, v := range tcs {
		if i >= 15 {
			break
		}
		out.TopTags = append(out.TopTags, TagCount{Tag: v.Tag, Count: v.Count})
	}

	return out
}

// ---------------------------------------------------------------------------
// File I/O helpers
// ---------------------------------------------------------------------------

func computePromptHash(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", h)
}

func readFileOrEmpty(path string) []byte {
	data, _ := os.ReadFile(path)
	return data
}

func wordCount(s string) int {
	return len(strings.Fields(s))
}

// readPromptFile reads the full prompt text from a .md file (after TOML frontmatter).
func readPromptFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	// Skip TOML frontmatter (between +++ markers)
	parts := strings.SplitN(content, "+++", 3)
	if len(parts) >= 3 {
		return strings.TrimSpace(parts[2]), nil
	}
	return strings.TrimSpace(content), nil
}

// writePromptFile writes a prompt .md file with TOML frontmatter atomically.
func writePromptFile(path string, rec *PromptRecord) error {
	tagsStr := "[]"
	if len(rec.Tags) > 0 {
		quoted := make([]string, len(rec.Tags))
		for i, t := range rec.Tags {
			quoted[i] = fmt.Sprintf("%q", t)
		}
		tagsStr = "[" + strings.Join(quoted, ", ") + "]"
	}

	content := fmt.Sprintf(`+++
hash = %q
short_hash = %q
repo = %q
timestamp = %q
session_id = %q
word_count = %d
task_type = %q
score = %d
grade = %q
tags = %s
status = %q
original_hash = %q
+++

%s
`, rec.Hash, rec.ShortHash, rec.Repo, rec.Timestamp, rec.SessionID,
		rec.WordCount, rec.TaskType, rec.Score, rec.Grade, tagsStr,
		rec.Status, rec.OriginalHash, rec.Prompt)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// promptFilePath returns the file path for a prompt record.
func promptFilePath(rec *PromptRecord) string {
	subdir := "unsorted"
	if rec.Status == "improved" || rec.Status == "scored" {
		subdir = "sorted"
	}
	return filepath.Join(promptsBaseDir(), rec.Repo, subdir, rec.ShortHash+".md")
}

// ---------------------------------------------------------------------------
// Enhancer helpers
// ---------------------------------------------------------------------------

func scorePrompt(prompt string) (int, string, string, *enhancer.ScoreReport) {
	taskType := enhancer.Classify(prompt)
	ar := enhancer.Analyze(prompt)
	lints := enhancer.Lint(prompt)
	report := enhancer.Score(prompt, taskType, lints, &ar)
	return report.Overall, report.Grade, string(taskType), report
}

// ---------------------------------------------------------------------------
// Input/Output types for MCP tools
// ---------------------------------------------------------------------------

// prompt_capture
type promptCaptureInput struct {
	Prompt    string   `json:"prompt" jsonschema:"required,description=The prompt text to capture"`
	Repo      string   `json:"repo,omitempty" jsonschema:"description=Repository name. Defaults to 'unknown'."`
	SessionID string   `json:"session_id,omitempty" jsonschema:"description=Claude Code session ID"`
	Tags      []string `json:"tags,omitempty" jsonschema:"description=Initial tags to apply"`
}

type promptCaptureOutput struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
	Score     int    `json:"score"`
	Grade     string `json:"grade"`
	TaskType  string `json:"task_type"`
	SavedTo   string `json:"saved_to"`
	Status    string `json:"status"`
}

// prompt_search
type promptSearchInput struct {
	Query    string   `json:"query,omitempty" jsonschema:"description=Full-text search query across prompt content"`
	Tags     []string `json:"tags,omitempty" jsonschema:"description=Filter by tags (AND logic)"`
	Repo     string   `json:"repo,omitempty" jsonschema:"description=Filter by repository name"`
	Status   string   `json:"status,omitempty" jsonschema:"description=Filter by status: unsorted scored improved archived"`
	MinScore int      `json:"min_score,omitempty" jsonschema:"description=Minimum quality score 0-100"`
	Limit    int      `json:"limit,omitempty" jsonschema:"description=Max results. Default 20."`
}

type promptSearchResult struct {
	Hash      string   `json:"hash"`
	ShortHash string   `json:"short_hash"`
	Preview   string   `json:"preview"`
	Repo      string   `json:"repo"`
	Score     int      `json:"score"`
	Grade     string   `json:"grade"`
	TaskType  string   `json:"task_type"`
	Tags      []string `json:"tags"`
	Status    string   `json:"status"`
	Timestamp string   `json:"timestamp"`
}

type promptSearchOutput struct {
	Results []promptSearchResult `json:"results"`
	Total   int                  `json:"total"`
}

// prompt_get
type promptGetInput struct {
	Hash string `json:"hash" jsonschema:"required,description=Full hash or short hash (first 12 chars) of the prompt"`
}

// prompt_tag
type promptTagInput struct {
	Hash   string   `json:"hash" jsonschema:"required,description=Prompt hash"`
	Add    []string `json:"add,omitempty" jsonschema:"description=Tags to add"`
	Remove []string `json:"remove,omitempty" jsonschema:"description=Tags to remove"`
}

type promptTagOutput struct {
	Hash string   `json:"hash"`
	Tags []string `json:"tags"`
}

// prompt_score
type promptScoreInput struct {
	Hash   string `json:"hash,omitempty" jsonschema:"description=Hash of stored prompt to score"`
	Prompt string `json:"prompt,omitempty" jsonschema:"description=Raw prompt text to score (if not using hash)"`
}

type promptScoreOutput struct {
	Score      int                       `json:"score"`
	Grade      string                    `json:"grade"`
	TaskType   string                    `json:"task_type"`
	Dimensions []enhancer.DimensionScore `json:"dimensions"`
	LintCount  int                       `json:"lint_count"`
}

// prompt_improve
type promptImproveInput struct {
	Hash     string `json:"hash,omitempty" jsonschema:"description=Hash of stored prompt to improve"`
	Prompt   string `json:"prompt,omitempty" jsonschema:"description=Raw prompt text to improve (if not using hash)"`
	TaskType string `json:"task_type,omitempty" jsonschema:"description=Override task type classification"`
}

type promptImproveOutput struct {
	OriginalHash  string   `json:"original_hash,omitempty"`
	Enhanced      string   `json:"enhanced"`
	TaskType      string   `json:"task_type"`
	StagesRun     []string `json:"stages_run"`
	Improvements  []string `json:"improvements"`
	OriginalScore int      `json:"original_score"`
	EnhancedScore int      `json:"enhanced_score"`
	SavedTo       string   `json:"saved_to,omitempty"`
	Hash          string   `json:"hash,omitempty"`
}

// prompt_stats
type promptStatsInput struct {
	Repo string `json:"repo,omitempty" jsonschema:"description=Filter stats by repo"`
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

type promptStatsResult struct {
	TotalPrompts      int            `json:"total_prompts"`
	ByRepo            map[string]int `json:"by_repo"`
	ByStatus          map[string]int `json:"by_status"`
	ByTaskType        map[string]int `json:"by_task_type"`
	ByGrade           map[string]int `json:"by_grade"`
	AverageScore      float64        `json:"average_score"`
	TopTags           []TagCount     `json:"top_tags"`
	RecentCaptures24h int            `json:"recent_captures_24h"`
}

// prompt_export
type promptExportInput struct {
	Repo     string   `json:"repo,omitempty" jsonschema:"description=Filter by repo"`
	Tags     []string `json:"tags,omitempty" jsonschema:"description=Filter by tags"`
	MinScore int      `json:"min_score,omitempty" jsonschema:"description=Minimum quality score"`
	Status   string   `json:"status,omitempty" jsonschema:"description=Filter by status"`
	Format   string   `json:"format,omitempty" jsonschema:"description=Output format: jsonl (default) or markdown"`
	Limit    int      `json:"limit,omitempty" jsonschema:"description=Max records. Default 100."`
}

type promptExportOutput struct {
	Exported int    `json:"exported"`
	Format   string `json:"format"`
	Data     string `json:"data"`
}

// ---------------------------------------------------------------------------
// PromptRegistryModule
// ---------------------------------------------------------------------------

type PromptRegistryModule struct{}

func (m *PromptRegistryModule) Name() string { return "prompt_registry" }
func (m *PromptRegistryModule) Description() string {
	return "Prompt capture, search, scoring, tagging, and improvement"
}

func (m *PromptRegistryModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		// ── prompt_capture ──────────────────────────────
		handler.TypedHandler[promptCaptureInput, promptCaptureOutput](
			"prompt_capture",
			"Capture a prompt with auto-scoring. Computes SHA-256 hash for dedup, classifies task type, scores against 10 quality dimensions, and saves to docs/prompts/{repo}/unsorted/. Use when you want to store a prompt for later retrieval and analysis.",
			func(_ context.Context, input promptCaptureInput) (promptCaptureOutput, error) {
				if strings.TrimSpace(input.Prompt) == "" {
					return promptCaptureOutput{}, fmt.Errorf("[%s] prompt is required", handler.ErrInvalidParam)
				}

				if err := globalPromptIndex.load(); err != nil {
					return promptCaptureOutput{}, err
				}

				hash := computePromptHash(input.Prompt)
				shortHash := hash[:12]

				// Dedup check
				if existing := globalPromptIndex.get(hash); existing != nil {
					return promptCaptureOutput{
						Hash:      existing.Hash,
						ShortHash: existing.ShortHash,
						Score:     existing.Score,
						Grade:     existing.Grade,
						TaskType:  existing.TaskType,
						SavedTo:   promptFilePath(existing),
						Status:    existing.Status,
					}, nil
				}

				// Score the prompt
				score, grade, taskType, _ := scorePrompt(input.Prompt)

				repo := input.Repo
				if repo == "" {
					repo = "unknown"
				}

				rec := &PromptRecord{
					Hash:      hash,
					ShortHash: shortHash,
					Repo:      repo,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					SessionID: input.SessionID,
					WordCount: wordCount(input.Prompt),
					TaskType:  taskType,
					Score:     score,
					Grade:     grade,
					Tags:      input.Tags,
					Status:    "unsorted",
					Prompt:    input.Prompt,
				}
				if rec.Tags == nil {
					rec.Tags = []string{}
				}

				path := promptFilePath(rec)
				if err := writePromptFile(path, rec); err != nil {
					return promptCaptureOutput{}, err
				}
				if err := globalPromptIndex.add(rec); err != nil {
					return promptCaptureOutput{}, err
				}

				return promptCaptureOutput{
					Hash:      hash,
					ShortHash: shortHash,
					Score:     score,
					Grade:     grade,
					TaskType:  taskType,
					SavedTo:   path,
					Status:    "unsorted",
				}, nil
			},
		),

		// ── prompt_search ──────────────────────────────
		handler.TypedHandler[promptSearchInput, promptSearchOutput](
			"prompt_search",
			"Search the prompt registry with full-text query and/or tag/repo/status/score filters. Returns previews sorted by recency. Use when finding previously captured prompts or exploring prompt patterns.",
			func(_ context.Context, input promptSearchInput) (promptSearchOutput, error) {
				if err := globalPromptIndex.load(); err != nil {
					return promptSearchOutput{}, err
				}

				results := globalPromptIndex.search(
					input.Query, input.Repo, input.Status,
					input.Tags, input.MinScore, input.Limit,
				)

				out := promptSearchOutput{Total: len(results)}
				for _, rec := range results {
					preview := rec.Prompt
					if len(preview) > 200 {
						preview = preview[:200] + "..."
					}
					out.Results = append(out.Results, promptSearchResult{
						Hash:      rec.Hash,
						ShortHash: rec.ShortHash,
						Preview:   preview,
						Repo:      rec.Repo,
						Score:     rec.Score,
						Grade:     rec.Grade,
						TaskType:  rec.TaskType,
						Tags:      rec.Tags,
						Status:    rec.Status,
						Timestamp: rec.Timestamp,
					})
				}
				return out, nil
			},
		),

		// ── prompt_get ──────────────────────────────
		handler.TypedHandler[promptGetInput, PromptRecord](
			"prompt_get",
			"Retrieve a single prompt by full hash or short hash (first 12 chars). Returns the complete prompt text and all metadata. Use when you need the full content of a specific prompt.",
			func(_ context.Context, input promptGetInput) (PromptRecord, error) {
				if strings.TrimSpace(input.Hash) == "" {
					return PromptRecord{}, fmt.Errorf("[%s] hash is required", handler.ErrInvalidParam)
				}

				if err := globalPromptIndex.load(); err != nil {
					return PromptRecord{}, err
				}

				rec := globalPromptIndex.get(input.Hash)
				if rec == nil {
					return PromptRecord{}, fmt.Errorf("[%s] prompt not found: %s", handler.ErrNotFound, input.Hash)
				}

				// If the in-memory prompt is truncated, read full text from file
				path := promptFilePath(rec)
				if fullText, err := readPromptFile(path); err == nil && len(fullText) > len(rec.Prompt) {
					rec.Prompt = fullText
				}

				return *rec, nil
			},
		),

		// ── prompt_tag ──────────────────────────────
		handler.TypedHandler[promptTagInput, promptTagOutput](
			"prompt_tag",
			"Add or remove tags on a stored prompt. Tags are validated against the docs taxonomy (mcp, agents, go, terminal, etc). Use to organize and categorize prompts for easier retrieval.",
			func(_ context.Context, input promptTagInput) (promptTagOutput, error) {
				if strings.TrimSpace(input.Hash) == "" {
					return promptTagOutput{}, fmt.Errorf("[%s] hash is required", handler.ErrInvalidParam)
				}

				if err := globalPromptIndex.load(); err != nil {
					return promptTagOutput{}, err
				}

				rec := globalPromptIndex.get(input.Hash)
				if rec == nil {
					return promptTagOutput{}, fmt.Errorf("[%s] prompt not found: %s", handler.ErrNotFound, input.Hash)
				}

				// Build tag set
				tagSet := make(map[string]bool, len(rec.Tags))
				for _, t := range rec.Tags {
					tagSet[t] = true
				}
				for _, t := range input.Add {
					tagSet[strings.TrimSpace(t)] = true
				}
				for _, t := range input.Remove {
					delete(tagSet, strings.TrimSpace(t))
				}

				newTags := make([]string, 0, len(tagSet))
				for t := range tagSet {
					if t != "" {
						newTags = append(newTags, t)
					}
				}
				sort.Strings(newTags)
				rec.Tags = newTags

				// Update file and index
				path := promptFilePath(rec)
				if err := writePromptFile(path, rec); err != nil {
					return promptTagOutput{}, err
				}
				if err := globalPromptIndex.update(rec); err != nil {
					return promptTagOutput{}, err
				}

				return promptTagOutput{Hash: rec.Hash, Tags: rec.Tags}, nil
			},
		),

		// ── prompt_score ──────────────────────────────
		handler.TypedHandler[promptScoreInput, promptScoreOutput](
			"prompt_score",
			"Score a prompt against 10 quality dimensions (Clarity, Specificity, Context, Structure, Examples, DocPlacement, Role, TaskFocus, Format, Tone). Returns 0-100 overall score with per-dimension breakdown. Works on stored prompts (by hash) or raw text.",
			func(_ context.Context, input promptScoreInput) (promptScoreOutput, error) {
				prompt := input.Prompt

				if input.Hash != "" {
					if err := globalPromptIndex.load(); err != nil {
						return promptScoreOutput{}, err
					}
					rec := globalPromptIndex.get(input.Hash)
					if rec == nil {
						return promptScoreOutput{}, fmt.Errorf("[%s] prompt not found: %s", handler.ErrNotFound, input.Hash)
					}
					prompt = rec.Prompt
					// Try full text from file
					if fullText, err := readPromptFile(promptFilePath(rec)); err == nil && len(fullText) > len(prompt) {
						prompt = fullText
					}
				}

				if strings.TrimSpace(prompt) == "" {
					return promptScoreOutput{}, fmt.Errorf("[%s] either hash or prompt is required", handler.ErrInvalidParam)
				}

				taskType := enhancer.Classify(prompt)
				ar := enhancer.Analyze(prompt)
				lints := enhancer.Lint(prompt)
				report := enhancer.Score(prompt, taskType, lints, &ar)

				// Update stored record if scoring by hash
				if input.Hash != "" {
					if rec := globalPromptIndex.get(input.Hash); rec != nil {
						rec.Score = report.Overall
						rec.Grade = report.Grade
						rec.TaskType = string(taskType)
						if rec.Status == "unsorted" {
							rec.Status = "scored"
						}
						_ = globalPromptIndex.update(rec)
						_ = writePromptFile(promptFilePath(rec), rec)
					}
				}

				return promptScoreOutput{
					Score:      report.Overall,
					Grade:      report.Grade,
					TaskType:   string(taskType),
					Dimensions: report.Dimensions,
					LintCount:  len(lints),
				}, nil
			},
		),

		// ── prompt_improve ──────────────────────────────
		handler.TypedHandler[promptImproveInput, promptImproveOutput](
			"prompt_improve",
			"Improve a prompt via the 13-stage enhancement pipeline (specificity, positive reframing, tone, XML structure, context reordering, format enforcement, etc). Saves the improved version to sorted/. Works on stored prompts (by hash) or raw text.",
			func(_ context.Context, input promptImproveInput) (promptImproveOutput, error) {
				prompt := input.Prompt
				var originalHash string
				var repo string

				if input.Hash != "" {
					if err := globalPromptIndex.load(); err != nil {
						return promptImproveOutput{}, err
					}
					rec := globalPromptIndex.get(input.Hash)
					if rec == nil {
						return promptImproveOutput{}, fmt.Errorf("[%s] prompt not found: %s", handler.ErrNotFound, input.Hash)
					}
					prompt = rec.Prompt
					originalHash = rec.Hash
					repo = rec.Repo
					// Try full text from file
					if fullText, err := readPromptFile(promptFilePath(rec)); err == nil && len(fullText) > len(prompt) {
						prompt = fullText
					}
				}

				if strings.TrimSpace(prompt) == "" {
					return promptImproveOutput{}, fmt.Errorf("[%s] either hash or prompt is required", handler.ErrInvalidParam)
				}

				// Classify
				taskType := enhancer.TaskType(input.TaskType)
				if taskType == "" {
					taskType = enhancer.Classify(prompt)
				}

				// Score original
				origScore, _, _, _ := scorePrompt(prompt)

				// Enhance
				cfg := enhancer.ResolveConfig(".")
				result := enhancer.EnhanceWithConfig(prompt, taskType, cfg)

				// Score enhanced
				enhScore, enhGrade, _, _ := scorePrompt(result.Enhanced)

				out := promptImproveOutput{
					OriginalHash:  originalHash,
					Enhanced:      result.Enhanced,
					TaskType:      string(result.TaskType),
					StagesRun:     result.StagesRun,
					Improvements:  result.Improvements,
					OriginalScore: origScore,
					EnhancedScore: enhScore,
				}

				// Save improved version
				if repo == "" {
					repo = "unknown"
				}
				newHash := computePromptHash(result.Enhanced)
				newRec := &PromptRecord{
					Hash:         newHash,
					ShortHash:    newHash[:12],
					Repo:         repo,
					Timestamp:    time.Now().UTC().Format(time.RFC3339),
					WordCount:    wordCount(result.Enhanced),
					TaskType:     string(result.TaskType),
					Score:        enhScore,
					Grade:        enhGrade,
					Tags:         []string{"improved"},
					Status:       "improved",
					OriginalHash: originalHash,
					Improvements: result.Improvements,
					StagesRun:    result.StagesRun,
					Prompt:       result.Enhanced,
				}

				path := promptFilePath(newRec)
				if err := writePromptFile(path, newRec); err == nil {
					_ = globalPromptIndex.load()
					_ = globalPromptIndex.add(newRec)
					out.SavedTo = path
					out.Hash = newHash
				}

				return out, nil
			},
		),

		// ── prompt_stats ──────────────────────────────
		handler.TypedHandler[promptStatsInput, promptStatsResult](
			"prompt_stats",
			"Aggregate statistics for the prompt registry: counts by repo, status, task type, grade, average score, top tags, and recent capture rate. Use to understand the prompt database composition.",
			func(_ context.Context, input promptStatsInput) (promptStatsResult, error) {
				if err := globalPromptIndex.load(); err != nil {
					return promptStatsResult{}, err
				}
				return globalPromptIndex.stats(input.Repo), nil
			},
		),

		// ── prompt_export ──────────────────────────────
		handler.TypedHandler[promptExportInput, promptExportOutput](
			"prompt_export",
			"Export prompts matching filters as JSONL or markdown. Use for batch analysis, training data export, or prompt collection sharing.",
			func(_ context.Context, input promptExportInput) (promptExportOutput, error) {
				if err := globalPromptIndex.load(); err != nil {
					return promptExportOutput{}, err
				}

				limit := input.Limit
				if limit <= 0 {
					limit = 100
				}

				results := globalPromptIndex.search(
					"", input.Repo, input.Status,
					input.Tags, input.MinScore, limit,
				)

				format := input.Format
				if format == "" {
					format = "jsonl"
				}

				var buf strings.Builder
				for _, rec := range results {
					switch format {
					case "jsonl":
						line, _ := json.Marshal(rec)
						buf.Write(line)
						buf.WriteByte('\n')
					case "markdown":
						fmt.Fprintf(&buf, "## %s (Score: %d/%s)\n", rec.ShortHash, rec.Score, rec.Grade)
						fmt.Fprintf(&buf, "**Repo**: %s | **Type**: %s | **Tags**: %s\n\n", rec.Repo, rec.TaskType, strings.Join(rec.Tags, ", "))
						fmt.Fprintf(&buf, "```\n%s\n```\n\n---\n\n", rec.Prompt)
					}
				}

				return promptExportOutput{
					Exported: len(results),
					Format:   format,
					Data:     buf.String(),
				}, nil
			},
		),
	}
}

// ensure PromptRegistryModule satisfies the interface at compile time
var _ registry.ToolModule = (*PromptRegistryModule)(nil)
