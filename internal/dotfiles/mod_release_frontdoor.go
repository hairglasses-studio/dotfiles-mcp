package dotfiles

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
)

type PipelineStatusInput struct {
	LocalDir        string   `json:"local_dir,omitempty" jsonschema:"description=Local directory (default: ~/hairglasses-studio)"`
	Repos           []string `json:"repos,omitempty" jsonschema:"description=Optional repo names to include; defaults to all workspace repos"`
	RefreshBaseline bool     `json:"refresh_baseline,omitempty" jsonschema:"description=Refresh cached local baseline results before aggregating statuses"`
	IncludePassing  bool     `json:"include_passing,omitempty" jsonschema:"description=Include green repos in the returned list"`
	MaxRepos        int      `json:"max_repos,omitempty" jsonschema:"description=Maximum repos to return after ranking"`
}

type PipelineStatusRepo struct {
	Name                string `json:"name"`
	Language            string `json:"language"`
	RemoteCIStatus      string `json:"remote_ci_status"`
	RemoteCIUpdatedAt   string `json:"remote_ci_updated_at,omitempty"`
	LocalBaselineStatus string `json:"local_baseline_status"`
	WorkflowStatus      string `json:"workflow_status,omitempty"`
	SignalVerdict       string `json:"signal_verdict"`
	SignalFreshnessDays int    `json:"signal_freshness_days,omitempty"`
	LastCommitDays      int    `json:"last_commit_days"`
}

type PipelineStatusOutput struct {
	Total             int                  `json:"total"`
	Passing           int                  `json:"passing"`
	Failing           int                  `json:"failing"`
	Stale             int                  `json:"stale"`
	Governance        int                  `json:"governance"`
	Unknown           int                  `json:"unknown"`
	BaselineRefreshed bool                 `json:"baseline_refreshed"`
	Repos             []PipelineStatusRepo `json:"repos"`
}

type DotfilesChangelogGenInput struct {
	LocalDir string   `json:"local_dir,omitempty" jsonschema:"description=Local directory (default: ~/hairglasses-studio)"`
	Repos    []string `json:"repos,omitempty" jsonschema:"description=Optional repo names to process; defaults to all workspace repos"`
	Write    bool     `json:"write,omitempty" jsonschema:"description=Write generated markdown into each repo CHANGELOG.md"`
	MaxRepos int      `json:"max_repos,omitempty" jsonschema:"description=Maximum repos to process"`
}

type RepoChangelogResult struct {
	Repo        string         `json:"repo"`
	Status      string         `json:"status"`
	LastTag     string         `json:"last_tag,omitempty"`
	CommitCount int            `json:"commit_count"`
	Groups      map[string]int `json:"groups,omitempty"`
	Written     bool           `json:"written"`
	Markdown    string         `json:"markdown,omitempty"`
	Error       string         `json:"error,omitempty"`
}

type DotfilesChangelogGenOutput struct {
	Total     int                   `json:"total"`
	Generated int                   `json:"generated"`
	Failed    int                   `json:"failed"`
	Results   []RepoChangelogResult `json:"results"`
}

type DotfilesReleaseTarget struct {
	Repo    string `json:"repo" jsonschema:"required,description=Repo name or absolute path"`
	Version string `json:"version" jsonschema:"required,description=Version to release (e.g. 1.2.3 or v1.2.3)"`
}

type DotfilesReleaseInput struct {
	LocalDir      string                  `json:"local_dir,omitempty" jsonschema:"description=Local directory (default: ~/hairglasses-studio)"`
	Targets       []DotfilesReleaseTarget `json:"targets" jsonschema:"required,description=Repo/version pairs to release"`
	AutoChangelog bool                    `json:"auto_changelog,omitempty" jsonschema:"description=Generate and prepend CHANGELOG.md entries"`
	Push          bool                    `json:"push,omitempty" jsonschema:"description=Push release commits and tags"`
	Execute       bool                    `json:"execute,omitempty" jsonschema:"description=Execute the release instead of dry-run preview"`
}

type RepoReleaseResult struct {
	Repo           string   `json:"repo"`
	Status         string   `json:"status"`
	CurrentVersion string   `json:"current_version,omitempty"`
	NewVersion     string   `json:"new_version,omitempty"`
	Tag            string   `json:"tag,omitempty"`
	FilesModified  []string `json:"files_modified,omitempty"`
	Committed      bool     `json:"committed"`
	Pushed         bool     `json:"pushed"`
	DryRun         bool     `json:"dry_run"`
	ChangelogEntry string   `json:"changelog_entry,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type DotfilesReleaseOutput struct {
	Total    int                 `json:"total"`
	Released int                 `json:"released"`
	Failed   int                 `json:"failed"`
	Results  []RepoReleaseResult `json:"results"`
}

func resolveFleetRepoPath(localDir, repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", fmt.Errorf("[%s] repo is required", handler.ErrInvalidParam)
	}
	if filepath.IsAbs(repo) {
		return filepath.Clean(repo), nil
	}
	return filepath.Join(fleetRoot(localDir), repo), nil
}

func collectFleetRepoPaths(localDir string, repos []string, maxRepos int) ([]string, error) {
	if len(repos) == 0 {
		paths, err := listWorkspaceRepos(localDir)
		if err != nil {
			return nil, err
		}
		if maxRepos > 0 && len(paths) > maxRepos {
			paths = paths[:maxRepos]
		}
		return paths, nil
	}

	paths := make([]string, 0, len(repos))
	for _, repo := range repos {
		path, err := resolveFleetRepoPath(localDir, repo)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	if maxRepos > 0 && len(paths) > maxRepos {
		paths = paths[:maxRepos]
	}
	return paths, nil
}

func pipelineVerdictRank(verdict string) int {
	switch verdict {
	case "red":
		return 0
	case "governance":
		return 1
	case "stale_remote":
		return 2
	case "unknown":
		return 3
	case "green":
		return 4
	default:
		return 5
	}
}

func dotfilesPipelineStatus(_ context.Context, input PipelineStatusInput) (PipelineStatusOutput, error) {
	localDir := fleetRoot(input.LocalDir)
	if input.RefreshBaseline {
		if _, err := runFleetBaselineRefresh(FleetBaselineRefreshInput{
			LocalDir: localDir,
			Repos:    input.Repos,
		}); err != nil {
			return PipelineStatusOutput{}, err
		}
	}

	audit, err := runFleetAudit(localDir)
	if err != nil {
		return PipelineStatusOutput{}, err
	}

	selected := make(map[string]struct{}, len(input.Repos))
	for _, repo := range input.Repos {
		selected[strings.TrimSpace(repo)] = struct{}{}
	}

	results := make([]PipelineStatusRepo, 0, len(audit.Repos))
	for _, repo := range audit.Repos {
		if len(selected) > 0 {
			if _, ok := selected[repo.Name]; !ok {
				continue
			}
		}
		if !input.IncludePassing && repo.SignalVerdict == "green" {
			continue
		}
		results = append(results, PipelineStatusRepo{
			Name:                repo.Name,
			Language:            repo.Language,
			RemoteCIStatus:      repo.RemoteCIStatus,
			RemoteCIUpdatedAt:   repo.RemoteCIUpdatedAt,
			LocalBaselineStatus: repo.LocalBaselineStatus,
			WorkflowStatus:      repo.WorkflowStatus,
			SignalVerdict:       repo.SignalVerdict,
			SignalFreshnessDays: repo.SignalFreshnessDays,
			LastCommitDays:      repo.LastCommitDays,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		ri := pipelineVerdictRank(results[i].SignalVerdict)
		rj := pipelineVerdictRank(results[j].SignalVerdict)
		if ri != rj {
			return ri < rj
		}
		if results[i].LastCommitDays != results[j].LastCommitDays {
			return results[i].LastCommitDays < results[j].LastCommitDays
		}
		return results[i].Name < results[j].Name
	})

	if input.MaxRepos > 0 && len(results) > input.MaxRepos {
		results = results[:input.MaxRepos]
	}

	out := PipelineStatusOutput{
		Total:             len(results),
		BaselineRefreshed: input.RefreshBaseline,
		Repos:             results,
	}
	for _, repo := range results {
		switch repo.SignalVerdict {
		case "green":
			out.Passing++
		case "red":
			out.Failing++
		case "stale_remote":
			out.Stale++
		case "governance":
			out.Governance++
		default:
			out.Unknown++
		}
	}
	return out, nil
}

func dotfilesChangelogGen(_ context.Context, input DotfilesChangelogGenInput) (DotfilesChangelogGenOutput, error) {
	localDir := fleetRoot(input.LocalDir)
	repoPaths, err := collectFleetRepoPaths(localDir, input.Repos, input.MaxRepos)
	if err != nil {
		return DotfilesChangelogGenOutput{}, err
	}

	out := DotfilesChangelogGenOutput{
		Total:   len(repoPaths),
		Results: make([]RepoChangelogResult, 0, len(repoPaths)),
	}
	for _, repoPath := range repoPaths {
		repoName := filepath.Base(repoPath)
		result, err := opsChangelogGenerate(context.Background(), OpsChangelogGenerateInput{
			Repo:  repoPath,
			Write: input.Write,
		})
		if err != nil {
			out.Failed++
			out.Results = append(out.Results, RepoChangelogResult{
				Repo:   repoName,
				Status: "failed",
				Error:  err.Error(),
			})
			continue
		}

		status := "generated"
		if strings.Contains(result.Markdown, "No new commits since last tag.") {
			status = "no_changes"
		} else {
			out.Generated++
		}

		out.Results = append(out.Results, RepoChangelogResult{
			Repo:        repoName,
			Status:      status,
			LastTag:     result.LastTag,
			CommitCount: result.CommitCount,
			Groups:      result.Groups,
			Written:     result.Written,
			Markdown:    result.Markdown,
		})
	}
	return out, nil
}

func dotfilesRelease(_ context.Context, input DotfilesReleaseInput) (DotfilesReleaseOutput, error) {
	if len(input.Targets) == 0 {
		return DotfilesReleaseOutput{}, fmt.Errorf("[%s] targets is required", handler.ErrInvalidParam)
	}

	localDir := fleetRoot(input.LocalDir)
	out := DotfilesReleaseOutput{
		Total:   len(input.Targets),
		Results: make([]RepoReleaseResult, 0, len(input.Targets)),
	}
	for _, target := range input.Targets {
		repoPath, err := resolveFleetRepoPath(localDir, target.Repo)
		if err != nil {
			out.Failed++
			out.Results = append(out.Results, RepoReleaseResult{
				Repo:   strings.TrimSpace(target.Repo),
				Status: "failed",
				Error:  err.Error(),
			})
			continue
		}
		if strings.TrimSpace(target.Version) == "" {
			out.Failed++
			out.Results = append(out.Results, RepoReleaseResult{
				Repo:   filepath.Base(repoPath),
				Status: "failed",
				Error:  fmt.Sprintf("[%s] version is required", handler.ErrInvalidParam),
			})
			continue
		}

		result, err := opsRelease(context.Background(), OpsReleaseInput{
			Repo:          repoPath,
			Version:       target.Version,
			AutoChangelog: input.AutoChangelog,
			Push:          input.Push,
			Execute:       input.Execute,
		})
		if err != nil {
			out.Failed++
			out.Results = append(out.Results, RepoReleaseResult{
				Repo:   filepath.Base(repoPath),
				Status: "failed",
				Error:  err.Error(),
			})
			continue
		}

		status := "dry-run"
		if input.Execute {
			status = "released"
			out.Released++
		}
		out.Results = append(out.Results, RepoReleaseResult{
			Repo:           filepath.Base(repoPath),
			Status:         status,
			CurrentVersion: result.CurrentVersion,
			NewVersion:     result.NewVersion,
			Tag:            result.Tag,
			FilesModified:  result.FilesModified,
			Committed:      result.Committed,
			Pushed:         result.Pushed,
			DryRun:         result.DryRun,
			ChangelogEntry: result.ChangelogEntry,
		})
	}
	return out, nil
}
