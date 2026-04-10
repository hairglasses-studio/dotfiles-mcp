package dotfiles

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hairglasses-studio/dotfiles-mcp/internal/githubstars"
	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type GitHubStarsModule struct{}

type githubStarListSpecInput struct {
	Name        string `json:"name" jsonschema:"required,description=GitHub star list name"`
	Description string `json:"description,omitempty" jsonschema:"description=Optional list description"`
	IsPrivate   bool   `json:"is_private,omitempty" jsonschema:"description=Create or update the list as private when true"`
}

type githubStarsListInput struct {
	Query        string   `json:"query,omitempty" jsonschema:"description=Filter starred repos by substring across name description and topics"`
	Language     string   `json:"language,omitempty" jsonschema:"description=Filter by primary language"`
	Topic        string   `json:"topic,omitempty" jsonschema:"description=Filter by GitHub topic"`
	ListNames    []string `json:"list_names,omitempty" jsonschema:"description=Only return repos that belong to one of these GitHub star lists"`
	Limit        int      `json:"limit,omitempty" jsonschema:"description=Maximum repos to return. Default 100."`
	SortBy       string   `json:"sort_by,omitempty" jsonschema:"description=Sort by starred updated stars or name,enum=starred,enum=updated,enum=stars,enum=name"`
	IncludeLists bool     `json:"include_lists,omitempty" jsonschema:"description=Include current GitHub list membership for each repo"`
}

type githubStarsListOutput struct {
	Viewer string                          `json:"viewer"`
	Total  int                             `json:"total"`
	Repos  []githubstars.StarredRepository `json:"repos"`
}

type githubStarsSummaryInput struct {
	TopN              int    `json:"top_n,omitempty" jsonschema:"description=Maximum buckets to return for list, language, and topic summaries. Default 10."`
	ManagedListPrefix string `json:"managed_list_prefix,omitempty" jsonschema:"description=Optional prefix to count repos missing a managed classification, for example MCP / "`
}

type githubStarsSummaryOutput struct {
	Viewer  string                   `json:"viewer"`
	Summary githubstars.StarsSummary `json:"summary"`
}

type githubStarListsListInput struct {
	IncludeItems bool `json:"include_items,omitempty" jsonschema:"description=Include repositories in each list"`
	ItemsPerList int  `json:"items_per_list,omitempty" jsonschema:"description=When include_items=true limit items returned per list. Zero means all items."`
}

type githubStarListsListOutput struct {
	Viewer string                 `json:"viewer"`
	Total  int                    `json:"total"`
	Lists  []githubstars.UserList `json:"lists"`
}

type githubStarListsEnsureInput struct {
	Lists   []githubStarListSpecInput `json:"lists" jsonschema:"required,description=Lists to create or update"`
	Execute bool                      `json:"execute,omitempty" jsonschema:"description=Apply mutations when true. Dry-run by default."`
}

type githubStarListsDeleteInput struct {
	Names   []string `json:"names" jsonschema:"required,description=List names to delete"`
	Execute bool     `json:"execute,omitempty" jsonschema:"description=Delete lists when true. Dry-run by default."`
}

type githubStarListsRenameInput struct {
	OldName string `json:"old_name" jsonschema:"required,description=Current GitHub list name"`
	NewName string `json:"new_name" jsonschema:"required,description=Desired GitHub list name"`
	Execute bool   `json:"execute,omitempty" jsonschema:"description=Rename the list when true. Dry-run by default."`
}

type githubStarsSetInput struct {
	Repos   []string `json:"repos" jsonschema:"required,description=Repository slugs in owner/name form"`
	State   string   `json:"state" jsonschema:"required,description=Desired star state,enum=star,enum=unstar"`
	Execute bool     `json:"execute,omitempty" jsonschema:"description=Apply star changes when true. Dry-run by default."`
}

type githubStarMembershipSetInput struct {
	Repos              []string `json:"repos" jsonschema:"required,description=Repository slugs in owner/name form"`
	ListNames          []string `json:"list_names,omitempty" jsonschema:"description=Target GitHub list names"`
	Operation          string   `json:"operation,omitempty" jsonschema:"description=How to apply membership changes,enum=merge,enum=replace,enum=remove,enum=replace_managed"`
	ManagedListPrefix  string   `json:"managed_list_prefix,omitempty" jsonschema:"description=Required when operation=replace_managed; only lists with this prefix are replaced"`
	CreateMissingLists bool     `json:"create_missing_lists,omitempty" jsonschema:"description=Create missing target lists before assignment"`
	StarMissing        bool     `json:"star_missing,omitempty" jsonschema:"description=Star repositories first if needed before list assignment"`
	Execute            bool     `json:"execute,omitempty" jsonschema:"description=Apply list membership changes when true. Dry-run by default."`
}

type githubStarsTaxonomySuggestInput struct {
	Query string `json:"query,omitempty" jsonschema:"description=Optional substring filter before generating suggestions"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Maximum suggestions to return. Default 8."`
}

type githubStarsTaxonomySuggestOutput struct {
	Viewer            string                      `json:"viewer"`
	GitHubSuggestions []string                    `json:"github_suggestions"`
	Suggestions       []githubstars.SuggestedList `json:"suggestions"`
}

type githubStarsCleanupCandidatesInput struct {
	InactiveDays      int    `json:"inactive_days,omitempty" jsonschema:"description=Flag repos inactive for at least this many days. Default 365."`
	IncludeArchived   *bool  `json:"include_archived,omitempty" jsonschema:"description=Include archived repos as cleanup candidates. Default true."`
	IncludeForks      *bool  `json:"include_forks,omitempty" jsonschema:"description=Include forked repos as cleanup candidates. Default true."`
	RequireUnlisted   bool   `json:"require_unlisted,omitempty" jsonschema:"description=Also flag repos that are not currently assigned to any list"`
	ManagedListPrefix string `json:"managed_list_prefix,omitempty" jsonschema:"description=Also flag repos missing a managed list prefix, for example MCP / "`
	Limit             int    `json:"limit,omitempty" jsonschema:"description=Maximum cleanup candidates to return. Default 100."`
}

type githubStarsCleanupCandidatesOutput struct {
	Viewer     string                         `json:"viewer"`
	Total      int                            `json:"total"`
	Candidates []githubstars.CleanupCandidate `json:"candidates"`
}

type githubRepoAssignmentInput struct {
	Repo  string   `json:"repo" jsonschema:"required,description=Repository slug in owner/name form"`
	Lists []string `json:"lists" jsonschema:"required,description=Desired GitHub star lists for the repo"`
}

type githubStarsTaxonomyAuditInput struct {
	RepoAssignments      []githubRepoAssignmentInput `json:"repo_assignments,omitempty" jsonschema:"description=Optional desired repo-to-list mapping to audit against"`
	ManagedListPrefix    string                      `json:"managed_list_prefix,omitempty" jsonschema:"description=Managed list prefix to audit, for example MCP / "`
	MaxItemsPerBucket    int                         `json:"max_items_per_bucket,omitempty" jsonschema:"description=Maximum items to return in each audit bucket. Default 100."`
	UseBootstrapDefaults bool                        `json:"use_bootstrap_defaults,omitempty" jsonschema:"description=Audit against the built-in MCP bootstrap defaults when true"`
}

type githubStarsTaxonomyAuditOutput struct {
	Viewer string                    `json:"viewer"`
	Audit  githubstars.TaxonomyAudit `json:"audit"`
}

type githubStarsTaxonomySyncInput struct {
	Lists             []githubStarListSpecInput   `json:"lists,omitempty" jsonschema:"description=Lists to ensure before syncing assignments"`
	RepoAssignments   []githubRepoAssignmentInput `json:"repo_assignments" jsonschema:"required,description=Desired repo to list mapping"`
	Operation         string                      `json:"operation,omitempty" jsonschema:"description=How to apply repo memberships,enum=merge,enum=replace,enum=replace_managed"`
	ManagedListPrefix string                      `json:"managed_list_prefix,omitempty" jsonschema:"description=Prefix used when operation=replace_managed"`
	StarMissing       bool                        `json:"star_missing,omitempty" jsonschema:"description=Star repositories before assigning them to lists"`
	Execute           bool                        `json:"execute,omitempty" jsonschema:"description=Apply list and membership mutations when true. Dry-run by default."`
}

type githubStarsMarkdownSyncInput struct {
	SourceDir           string   `json:"source_dir,omitempty" jsonschema:"description=Directory containing markdown files to map into GitHub star lists. Defaults to /home/hg/github-reference-repos/index-files when present."`
	SourcePaths         []string `json:"source_paths,omitempty" jsonschema:"description=Optional explicit markdown file paths to include alongside or instead of source_dir"`
	DescriptionTemplate string   `json:"description_template,omitempty" jsonschema:"description=Optional list description template. Use %s for the markdown stem. Defaults to Managed by docs-mcp github-reference-repos import for %s"`
	IsPrivate           *bool    `json:"is_private,omitempty" jsonschema:"description=Create or update target lists as private. Defaults true."`
	StarMissing         bool     `json:"star_missing,omitempty" jsonschema:"description=Star repositories before assigning them to lists"`
	Execute             bool     `json:"execute,omitempty" jsonschema:"description=Apply star and list mutations when true. Dry-run by default."`
}

type githubStarsMarkdownSyncOutput struct {
	Viewer      string                         `json:"viewer"`
	Sources     []githubstars.MarkdownSource   `json:"sources,omitempty"`
	UniqueRepos int                            `json:"unique_repos"`
	Overlaps    []githubstars.RepoOverlap      `json:"overlaps,omitempty"`
	Taxonomy    githubstars.TaxonomySyncResult `json:"taxonomy"`
}

type githubStarsMarkdownAuditInput struct {
	SourceDir       string   `json:"source_dir,omitempty" jsonschema:"description=Directory containing markdown files to audit against GitHub star lists. Defaults to /home/hg/github-reference-repos/index-files when present."`
	SourcePaths     []string `json:"source_paths,omitempty" jsonschema:"description=Optional explicit markdown file paths to include alongside or instead of source_dir"`
	MaxItemsPerList int      `json:"max_items_per_list,omitempty" jsonschema:"description=Maximum missing or extra repos returned for each list. Default 100."`
}

type githubStarsMarkdownAuditOutput struct {
	Viewer string                    `json:"viewer"`
	Audit  githubstars.MarkdownAudit `json:"audit"`
}

type githubStarsBootstrapInput struct {
	ProductionRepos   []string `json:"production_repos,omitempty" jsonschema:"description=Repos to place in MCP / Production. Defaults to researched recommendations when empty."`
	ExperimentalRepos []string `json:"experimental_repos,omitempty" jsonschema:"description=Repos to place in MCP / Experimental"`
	AlphaRepos        []string `json:"alpha_repos,omitempty" jsonschema:"description=Repos to place in MCP / Alpha"`
	InstallCodexMCP   bool     `json:"install_codex_mcp,omitempty" jsonschema:"description=Also install the personal Codex MCP entries for GitHub Stars"`
	ConfigPath        string   `json:"config_path,omitempty" jsonschema:"description=Optional Codex config path override for install_codex_mcp"`
	DotfilesDir       string   `json:"dotfiles_dir,omitempty" jsonschema:"description=Optional dotfiles root override for install_codex_mcp"`
	Execute           bool     `json:"execute,omitempty" jsonschema:"description=Apply star and list mutations when true. Dry-run by default."`
}

type githubStarsInstallCodexInput struct {
	ConfigPath  string `json:"config_path,omitempty" jsonschema:"description=Optional Codex config path override. Defaults to ~/.codex/config.toml"`
	DotfilesDir string `json:"dotfiles_dir,omitempty" jsonschema:"description=Optional dotfiles root override. Defaults to the current repo root"`
	Execute     bool   `json:"execute,omitempty" jsonschema:"description=Write the Codex config when true. Dry-run by default."`
}

func (m *GitHubStarsModule) Name() string { return "github_stars" }
func (m *GitHubStarsModule) Description() string {
	return "GitHub stars, list folders, taxonomy sync, and Codex MCP install workflows"
}

func (m *GitHubStarsModule) Tools() []registry.ToolDefinition {
	listStars := handler.TypedHandler[githubStarsListInput, githubStarsListOutput](
		"gh_stars_list",
		"List starred GitHub repositories with optional filters and current GitHub star-list membership.",
		func(ctx context.Context, input githubStarsListInput) (githubStarsListOutput, error) {
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubStarsListOutput{}, err
			}
			viewer, err := client.Viewer(ctx)
			if err != nil {
				return githubStarsListOutput{}, err
			}
			limit := input.Limit
			if limit <= 0 {
				limit = 100
			}
			repos, err := client.ListStars(ctx, githubstars.StarsFilter{
				Query:        input.Query,
				Language:     input.Language,
				Topic:        input.Topic,
				ListNames:    input.ListNames,
				Limit:        limit,
				SortBy:       input.SortBy,
				IncludeLists: input.IncludeLists || len(input.ListNames) > 0,
			})
			if err != nil {
				return githubStarsListOutput{}, err
			}
			return githubStarsListOutput{
				Viewer: viewer.Login,
				Total:  len(repos),
				Repos:  repos,
			}, nil
		},
	)
	listStars.Category = "github"
	listStars.SearchTerms = []string{"github stars", "starred repos", "list stars", "star folders"}

	summary := handler.TypedHandler[githubStarsSummaryInput, githubStarsSummaryOutput](
		"gh_stars_summary",
		"Summarize current GitHub stars with counts by list, language, topic, archived/fork totals, and optional managed-prefix coverage.",
		func(ctx context.Context, input githubStarsSummaryInput) (githubStarsSummaryOutput, error) {
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubStarsSummaryOutput{}, err
			}
			viewer, err := client.Viewer(ctx)
			if err != nil {
				return githubStarsSummaryOutput{}, err
			}
			summary, err := client.Summary(ctx, input.TopN, input.ManagedListPrefix)
			if err != nil {
				return githubStarsSummaryOutput{}, err
			}
			return githubStarsSummaryOutput{
				Viewer:  viewer.Login,
				Summary: summary,
			}, nil
		},
	)
	summary.Category = "github"
	summary.SearchTerms = []string{"github stars summary", "star overview", "list coverage", "star statistics"}

	listLists := handler.TypedHandler[githubStarListsListInput, githubStarListsListOutput](
		"gh_star_lists_list",
		"List the current GitHub star folders (user lists), optionally including list items.",
		func(ctx context.Context, input githubStarListsListInput) (githubStarListsListOutput, error) {
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubStarListsListOutput{}, err
			}
			viewer, err := client.Viewer(ctx)
			if err != nil {
				return githubStarListsListOutput{}, err
			}
			lists, err := client.ListLists(ctx, input.IncludeItems, input.ItemsPerList)
			if err != nil {
				return githubStarListsListOutput{}, err
			}
			return githubStarListsListOutput{
				Viewer: viewer.Login,
				Total:  len(lists),
				Lists:  lists,
			}, nil
		},
	)
	listLists.Category = "github"
	listLists.SearchTerms = []string{"github star lists", "folders", "lists", "star categories"}

	ensureLists := handler.TypedHandler[githubStarListsEnsureInput, []githubstars.EnsureListResult](
		"gh_star_lists_ensure",
		"Create or update GitHub star folders (user lists). Dry-run by default.",
		func(ctx context.Context, input githubStarListsEnsureInput) ([]githubstars.EnsureListResult, error) {
			if len(input.Lists) == 0 {
				return nil, fmt.Errorf("[%s] lists is required", handler.ErrInvalidParam)
			}
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return nil, err
			}
			specs := make([]githubstars.EnsureListSpec, 0, len(input.Lists))
			for _, spec := range input.Lists {
				specs = append(specs, githubstars.EnsureListSpec{
					Name:        spec.Name,
					Description: spec.Description,
					IsPrivate:   spec.IsPrivate,
				})
			}
			return client.EnsureLists(ctx, specs, input.Execute)
		},
	)
	ensureLists.Category = "github"
	ensureLists.SearchTerms = []string{"create github list", "ensure star lists", "update star folders"}
	ensureLists.IsWrite = true

	deleteLists := handler.TypedHandler[githubStarListsDeleteInput, []githubstars.DeleteListResult](
		"gh_star_lists_delete",
		"Delete GitHub star folders (user lists) by name. Dry-run by default.",
		func(ctx context.Context, input githubStarListsDeleteInput) ([]githubstars.DeleteListResult, error) {
			if len(input.Names) == 0 {
				return nil, fmt.Errorf("[%s] names is required", handler.ErrInvalidParam)
			}
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return nil, err
			}
			return client.DeleteLists(ctx, input.Names, input.Execute)
		},
	)
	deleteLists.Category = "github"
	deleteLists.SearchTerms = []string{"delete github list", "remove star folders", "delete user list"}
	deleteLists.IsWrite = true

	renameList := handler.TypedHandler[githubStarListsRenameInput, githubstars.RenameListResult](
		"gh_star_lists_rename",
		"Rename an existing GitHub star folder while preserving its description and privacy settings. Dry-run by default.",
		func(ctx context.Context, input githubStarListsRenameInput) (githubstars.RenameListResult, error) {
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubstars.RenameListResult{}, err
			}
			return client.RenameList(ctx, input.OldName, input.NewName, input.Execute)
		},
	)
	renameList.Category = "github"
	renameList.SearchTerms = []string{"rename github list", "rename star folder", "rename star list"}
	renameList.IsWrite = true

	setStars := handler.TypedHandler[githubStarsSetInput, []githubstars.StarMutationResult](
		"gh_stars_set",
		"Star or unstar GitHub repositories in batch. Dry-run by default.",
		func(ctx context.Context, input githubStarsSetInput) ([]githubstars.StarMutationResult, error) {
			if len(input.Repos) == 0 {
				return nil, fmt.Errorf("[%s] repos is required", handler.ErrInvalidParam)
			}
			state := strings.ToLower(strings.TrimSpace(input.State))
			if state != "star" && state != "unstar" {
				return nil, fmt.Errorf("[%s] state must be star or unstar", handler.ErrInvalidParam)
			}
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return nil, err
			}
			return client.SetStarState(ctx, input.Repos, state == "star", input.Execute)
		},
	)
	setStars.Category = "github"
	setStars.SearchTerms = []string{"star repo", "unstar repo", "batch star", "batch unstar"}
	setStars.IsWrite = true

	setMembership := handler.TypedHandler[githubStarMembershipSetInput, []githubstars.RepoListMutationResult](
		"gh_star_membership_set",
		"Update which GitHub star folders a repo belongs to. Supports merge, replace, remove, and replace_managed. Dry-run by default.",
		func(ctx context.Context, input githubStarMembershipSetInput) ([]githubstars.RepoListMutationResult, error) {
			if len(input.Repos) == 0 {
				return nil, fmt.Errorf("[%s] repos is required", handler.ErrInvalidParam)
			}
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return nil, err
			}
			requests := make([]githubstars.RepoListMutationRequest, 0, len(input.Repos))
			for _, repo := range input.Repos {
				requests = append(requests, githubstars.RepoListMutationRequest{
					Repo:              repo,
					TargetLists:       input.ListNames,
					Operation:         githubstars.MembershipOperation(strings.TrimSpace(input.Operation)),
					ManagedListPrefix: input.ManagedListPrefix,
					CreateMissing:     input.CreateMissingLists,
					StarMissing:       input.StarMissing,
				})
			}
			return client.SetRepoMembership(ctx, requests, input.Execute)
		},
	)
	setMembership.Category = "github"
	setMembership.SearchTerms = []string{"assign star folder", "move star to list", "repo list membership", "github folders"}
	setMembership.IsWrite = true

	suggestTaxonomy := handler.TypedHandler[githubStarsTaxonomySuggestInput, githubStarsTaxonomySuggestOutput](
		"gh_stars_taxonomy_suggest",
		"Suggest useful GitHub star folders from your current stars using GitHub suggestions plus topic, language, and keyword clustering.",
		func(ctx context.Context, input githubStarsTaxonomySuggestInput) (githubStarsTaxonomySuggestOutput, error) {
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubStarsTaxonomySuggestOutput{}, err
			}
			viewer, err := client.Viewer(ctx)
			if err != nil {
				return githubStarsTaxonomySuggestOutput{}, err
			}
			repos, err := client.ListStars(ctx, githubstars.StarsFilter{
				Query: input.Query,
				Limit: 500,
			})
			if err != nil {
				return githubStarsTaxonomySuggestOutput{}, err
			}
			githubSuggested, err := client.SuggestedListNames(ctx)
			if err != nil {
				return githubStarsTaxonomySuggestOutput{}, err
			}
			return githubStarsTaxonomySuggestOutput{
				Viewer:            viewer.Login,
				GitHubSuggestions: githubSuggested,
				Suggestions:       githubstars.SuggestTaxonomy(repos, githubSuggested, input.Limit),
			}, nil
		},
	)
	suggestTaxonomy.Category = "github"
	suggestTaxonomy.SearchTerms = []string{"suggest github folders", "taxonomy suggestion", "organize stars"}

	cleanupCandidates := handler.TypedHandler[githubStarsCleanupCandidatesInput, githubStarsCleanupCandidatesOutput](
		"gh_stars_cleanup_candidates",
		"Find starred repositories that look like cleanup candidates, such as archived repos, forks, stale repos, unlisted repos, or repos missing a managed prefix.",
		func(ctx context.Context, input githubStarsCleanupCandidatesInput) (githubStarsCleanupCandidatesOutput, error) {
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubStarsCleanupCandidatesOutput{}, err
			}
			viewer, err := client.Viewer(ctx)
			if err != nil {
				return githubStarsCleanupCandidatesOutput{}, err
			}
			includeArchived := true
			if input.IncludeArchived != nil {
				includeArchived = *input.IncludeArchived
			}
			includeForks := true
			if input.IncludeForks != nil {
				includeForks = *input.IncludeForks
			}
			candidates, err := client.CleanupCandidates(ctx, githubstars.CleanupOptions{
				InactiveDays:      input.InactiveDays,
				IncludeArchived:   includeArchived,
				IncludeForks:      includeForks,
				RequireUnlisted:   input.RequireUnlisted,
				ManagedListPrefix: input.ManagedListPrefix,
				Limit:             input.Limit,
			})
			if err != nil {
				return githubStarsCleanupCandidatesOutput{}, err
			}
			return githubStarsCleanupCandidatesOutput{
				Viewer:     viewer.Login,
				Total:      len(candidates),
				Candidates: candidates,
			}, nil
		},
	)
	cleanupCandidates.Category = "github"
	cleanupCandidates.SearchTerms = []string{"cleanup starred repos", "find stale stars", "archived fork stars", "star pruning"}

	auditTaxonomy := handler.TypedHandler[githubStarsTaxonomyAuditInput, githubStarsTaxonomyAuditOutput](
		"gh_stars_taxonomy_audit",
		"Audit managed GitHub star-list coverage and compare current stars against an optional desired taxonomy profile.",
		func(ctx context.Context, input githubStarsTaxonomyAuditInput) (githubStarsTaxonomyAuditOutput, error) {
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubStarsTaxonomyAuditOutput{}, err
			}
			viewer, err := client.Viewer(ctx)
			if err != nil {
				return githubStarsTaxonomyAuditOutput{}, err
			}
			assignments := make([]githubstars.TaxonomyAssignment, 0, len(input.RepoAssignments))
			for _, assignment := range input.RepoAssignments {
				assignments = append(assignments, githubstars.TaxonomyAssignment{
					Repo:  assignment.Repo,
					Lists: assignment.Lists,
				})
			}
			if input.UseBootstrapDefaults && len(assignments) == 0 {
				_, assignments = githubstars.DefaultBootstrapSpecs()
			}
			audit, err := client.AuditTaxonomy(ctx, assignments, input.ManagedListPrefix)
			if err != nil {
				return githubStarsTaxonomyAuditOutput{}, err
			}
			audit = trimTaxonomyAudit(audit, input.MaxItemsPerBucket)
			return githubStarsTaxonomyAuditOutput{
				Viewer: viewer.Login,
				Audit:  audit,
			}, nil
		},
	)
	auditTaxonomy.Category = "github"
	auditTaxonomy.SearchTerms = []string{"audit github taxonomy", "audit star folders", "managed list drift", "star list coverage"}

	syncTaxonomy := handler.TypedHandler[githubStarsTaxonomySyncInput, githubstars.TaxonomySyncResult](
		"gh_stars_taxonomy_sync",
		"Ensure GitHub star folders and reconcile repo membership from an explicit repo-to-list taxonomy. Dry-run by default.",
		func(ctx context.Context, input githubStarsTaxonomySyncInput) (githubstars.TaxonomySyncResult, error) {
			if len(input.RepoAssignments) == 0 {
				return githubstars.TaxonomySyncResult{}, fmt.Errorf("[%s] repo_assignments is required", handler.ErrInvalidParam)
			}
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubstars.TaxonomySyncResult{}, err
			}
			specs := make([]githubstars.EnsureListSpec, 0, len(input.Lists))
			for _, spec := range input.Lists {
				specs = append(specs, githubstars.EnsureListSpec{
					Name:        spec.Name,
					Description: spec.Description,
					IsPrivate:   spec.IsPrivate,
				})
			}
			assignments := make([]githubstars.TaxonomyAssignment, 0, len(input.RepoAssignments))
			for _, assignment := range input.RepoAssignments {
				assignments = append(assignments, githubstars.TaxonomyAssignment{
					Repo:  assignment.Repo,
					Lists: assignment.Lists,
				})
			}
			operation := githubstars.MembershipOperation(strings.TrimSpace(input.Operation))
			if operation == "" {
				operation = githubstars.OperationReplaceManaged
			}
			return client.SyncTaxonomy(ctx, specs, assignments, operation, input.ManagedListPrefix, input.StarMissing, input.Execute)
		},
	)
	syncTaxonomy.Category = "github"
	syncTaxonomy.SearchTerms = []string{"sync github folders", "taxonomy sync", "reconcile star lists"}
	syncTaxonomy.IsWrite = true

	auditMarkdown := handler.TypedHandler[githubStarsMarkdownAuditInput, githubStarsMarkdownAuditOutput](
		"gh_stars_audit_markdown",
		"Audit markdown-derived GitHub repo lists against the matching live GitHub star folders without mutating anything.",
		func(ctx context.Context, input githubStarsMarkdownAuditInput) (githubStarsMarkdownAuditOutput, error) {
			sourceDir := strings.TrimSpace(input.SourceDir)
			if sourceDir == "" {
				sourceDir = defaultGitHubReferenceSourceDir()
			}
			sources, err := githubstars.ParseMarkdownSources(sourceDir, input.SourcePaths)
			if err != nil {
				return githubStarsMarkdownAuditOutput{}, err
			}

			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubStarsMarkdownAuditOutput{}, err
			}
			viewer, err := client.Viewer(ctx)
			if err != nil {
				return githubStarsMarkdownAuditOutput{}, err
			}
			lists, err := client.ListLists(ctx, true, 0)
			if err != nil {
				return githubStarsMarkdownAuditOutput{}, err
			}
			return githubStarsMarkdownAuditOutput{
				Viewer: viewer.Login,
				Audit:  githubstars.TrimMarkdownAudit(githubstars.BuildMarkdownAudit(sources, lists), input.MaxItemsPerList),
			}, nil
		},
	)
	auditMarkdown.Category = "workflow"
	auditMarkdown.SearchTerms = []string{"audit markdown stars", "github-reference-repos audit", "star list drift from markdown", "check markdown star lists"}

	syncMarkdown := handler.TypedHandler[githubStarsMarkdownSyncInput, githubStarsMarkdownSyncOutput](
		"gh_stars_sync_markdown",
		"Parse markdown GitHub links into per-file star lists, then exactly reconcile those target lists while preserving unrelated list memberships. Dry-run by default.",
		func(ctx context.Context, input githubStarsMarkdownSyncInput) (githubStarsMarkdownSyncOutput, error) {
			sourceDir := strings.TrimSpace(input.SourceDir)
			if sourceDir == "" {
				sourceDir = defaultGitHubReferenceSourceDir()
			}
			sources, err := githubstars.ParseMarkdownSources(sourceDir, input.SourcePaths)
			if err != nil {
				return githubStarsMarkdownSyncOutput{}, err
			}
			isPrivate := true
			if input.IsPrivate != nil {
				isPrivate = *input.IsPrivate
			}
			lists, assignments, overlaps := githubstars.BuildMarkdownTaxonomy(sources, input.DescriptionTemplate, isPrivate)

			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubStarsMarkdownSyncOutput{}, err
			}
			viewer, err := client.Viewer(ctx)
			if err != nil {
				return githubStarsMarkdownSyncOutput{}, err
			}
			taxonomy, err := client.SyncExactTaxonomy(ctx, lists, assignments, input.StarMissing, input.Execute)
			if err != nil {
				return githubStarsMarkdownSyncOutput{}, err
			}
			return githubStarsMarkdownSyncOutput{
				Viewer:      viewer.Login,
				Sources:     sources,
				UniqueRepos: len(assignments),
				Overlaps:    overlaps,
				Taxonomy:    taxonomy,
			}, nil
		},
	)
	syncMarkdown.Category = "workflow"
	syncMarkdown.SearchTerms = []string{"sync markdown stars", "github-reference-repos", "markdown star taxonomy", "reconcile star lists from markdown"}
	syncMarkdown.IsWrite = true

	bootstrap := handler.TypedHandler[githubStarsBootstrapInput, githubstars.BootstrapPlan](
		"gh_stars_bootstrap",
		"Bootstrap the GitHub Stars workflow: star the researched MCP repos, create the MCP / Production|Experimental|Alpha folders, assign repos, and optionally install the Codex MCP entries. Dry-run by default.",
		func(ctx context.Context, input githubStarsBootstrapInput) (githubstars.BootstrapPlan, error) {
			client, err := githubstars.NewClientFromEnv(ctx)
			if err != nil {
				return githubstars.BootstrapPlan{}, err
			}
			return client.Bootstrap(ctx, input.ProductionRepos, input.ExperimentalRepos, input.AlphaRepos, input.InstallCodexMCP, input.ConfigPath, input.DotfilesDir, input.Execute)
		},
	)
	bootstrap.Category = "workflow"
	bootstrap.SearchTerms = []string{"github stars bootstrap", "install github stars workflow", "mcp stars bootstrap"}
	bootstrap.IsWrite = true

	installCodex := handler.TypedHandler[githubStarsInstallCodexInput, githubstars.CodexInstallResult](
		"gh_stars_install_codex_mcp",
		"Install the global Codex MCP entries for the official GitHub server and the dotfiles GitHub Stars workflow surface. Dry-run by default.",
		func(_ context.Context, input githubStarsInstallCodexInput) (githubstars.CodexInstallResult, error) {
			return githubstars.InstallCodexConfig(input.ConfigPath, input.DotfilesDir, input.Execute)
		},
	)
	installCodex.Category = "workflow"
	installCodex.SearchTerms = []string{"install codex mcp", "global github stars mcp", "codex config github stars"}
	installCodex.IsWrite = true

	return []registry.ToolDefinition{
		listStars,
		summary,
		listLists,
		ensureLists,
		renameList,
		deleteLists,
		setStars,
		setMembership,
		suggestTaxonomy,
		cleanupCandidates,
		auditTaxonomy,
		syncTaxonomy,
		auditMarkdown,
		syncMarkdown,
		bootstrap,
		installCodex,
	}
}

func trimTaxonomyAudit(audit githubstars.TaxonomyAudit, limit int) githubstars.TaxonomyAudit {
	if limit <= 0 {
		limit = 100
	}
	if len(audit.ReposMissingManaged) > limit {
		audit.ReposMissingManaged = audit.ReposMissingManaged[:limit]
	}
	if len(audit.ReposOutsideProfile) > limit {
		audit.ReposOutsideProfile = audit.ReposOutsideProfile[:limit]
	}
	if len(audit.ReposWithDrift) > limit {
		audit.ReposWithDrift = audit.ReposWithDrift[:limit]
	}
	if len(audit.DesiredReposMissing) > limit {
		audit.DesiredReposMissing = audit.DesiredReposMissing[:limit]
	}
	return audit
}

func defaultGitHubReferenceSourceDir() string {
	const candidate = "/home/hg/github-reference-repos/index-files"
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return ""
}
