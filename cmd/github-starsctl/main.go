package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hairglasses-studio/dotfiles-mcp/internal/githubstars"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	ctx := context.Background()
	switch os.Args[1] {
	case "list-stars":
		runListStars(ctx, os.Args[2:])
	case "summary":
		runSummary(ctx, os.Args[2:])
	case "list-lists":
		runListLists(ctx, os.Args[2:])
	case "rename-list":
		runRenameList(ctx, os.Args[2:])
	case "set-membership":
		runSetMembership(ctx, os.Args[2:])
	case "suggest-taxonomy":
		runSuggestTaxonomy(ctx, os.Args[2:])
	case "cleanup-candidates":
		runCleanupCandidates(ctx, os.Args[2:])
	case "audit-taxonomy":
		runAuditTaxonomy(ctx, os.Args[2:])
	case "audit-markdown":
		runAuditMarkdown(ctx, os.Args[2:])
	case "sync-markdown":
		runSyncMarkdown(ctx, os.Args[2:])
	case "install-codex-mcp":
		runInstallCodex(os.Args[2:])
	case "bootstrap":
		runBootstrap(ctx, os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	_, _ = os.Stderr.WriteString(`github-starsctl - shell entrypoint for GitHub Stars workflows

Usage:
  github-starsctl list-stars [--query text] [--language lang] [--topic topic] [--list names] [--limit n] [--sort starred|updated|stars|name] [--include-lists]
  github-starsctl summary [--top n] [--managed-prefix prefix]
  github-starsctl list-lists [--include-items] [--items-per-list n]
  github-starsctl rename-list --from old --to new [--execute]
  github-starsctl set-membership --repos owner/name[,owner/name...] [--lists names] [--operation merge|replace|remove|replace_managed] [--managed-prefix prefix] [--create-missing-lists] [--star-missing] [--execute]
  github-starsctl suggest-taxonomy [--query text] [--limit n]
  github-starsctl cleanup-candidates [--inactive-days n] [--include-archived=true|false] [--include-forks=true|false] [--require-unlisted] [--managed-prefix prefix] [--limit n]
  github-starsctl audit-taxonomy [--managed-prefix prefix] [--max-items n] [--bootstrap-defaults] [--production repos] [--experimental repos] [--alpha repos]
  github-starsctl audit-markdown [--source-dir dir] [--sources path1,path2] [--max-items n]
  github-starsctl sync-markdown [--source-dir dir] [--sources path1,path2] [--description-template 'text %s'] [--private=true|false] [--star-missing] [--execute]
  github-starsctl install-codex-mcp [--config path] [--dotfiles-dir path] [--execute]
  github-starsctl bootstrap [--production repos] [--experimental repos] [--alpha repos] [--install-codex-mcp] [--config path] [--dotfiles-dir path] [--execute]
`)
}

func runListStars(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("list-stars", flag.ExitOnError)
	query := fs.String("query", "", "substring filter")
	language := fs.String("language", "", "primary language filter")
	topic := fs.String("topic", "", "topic filter")
	listNames := fs.String("list", "", "comma-separated list names")
	limit := fs.Int("limit", 100, "result limit")
	sortBy := fs.String("sort", "starred", "starred|updated|stars|name")
	includeLists := fs.Bool("include-lists", false, "include GitHub list membership")
	fs.Parse(args)

	client := mustClient(ctx)
	viewer, err := client.Viewer(ctx)
	fatalIf(err)
	repos, err := client.ListStars(ctx, githubstars.StarsFilter{
		Query:        *query,
		Language:     *language,
		Topic:        *topic,
		ListNames:    splitCSV(*listNames),
		Limit:        *limit,
		SortBy:       *sortBy,
		IncludeLists: *includeLists,
	})
	fatalIf(err)
	printJSON(map[string]any{
		"viewer": viewer.Login,
		"total":  len(repos),
		"repos":  repos,
	})
}

func runSummary(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("summary", flag.ExitOnError)
	topN := fs.Int("top", 10, "max buckets per section")
	managedPrefix := fs.String("managed-prefix", "", "optional managed list prefix")
	fs.Parse(args)

	client := mustClient(ctx)
	viewer, err := client.Viewer(ctx)
	fatalIf(err)
	summary, err := client.Summary(ctx, *topN, *managedPrefix)
	fatalIf(err)
	printJSON(map[string]any{
		"viewer":  viewer.Login,
		"summary": summary,
	})
}

func runListLists(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("list-lists", flag.ExitOnError)
	includeItems := fs.Bool("include-items", false, "include repositories in each list")
	itemsPerList := fs.Int("items-per-list", 0, "item cap per list when include-items is set")
	fs.Parse(args)

	client := mustClient(ctx)
	viewer, err := client.Viewer(ctx)
	fatalIf(err)
	lists, err := client.ListLists(ctx, *includeItems, *itemsPerList)
	fatalIf(err)
	printJSON(map[string]any{
		"viewer": viewer.Login,
		"total":  len(lists),
		"lists":  lists,
	})
}

func runRenameList(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("rename-list", flag.ExitOnError)
	oldName := fs.String("from", "", "current list name")
	newName := fs.String("to", "", "desired list name")
	execute := fs.Bool("execute", false, "rename the list")
	fs.Parse(args)

	client := mustClient(ctx)
	result, err := client.RenameList(ctx, *oldName, *newName, *execute)
	fatalIf(err)
	printJSON(result)
}

func runSetMembership(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("set-membership", flag.ExitOnError)
	repos := fs.String("repos", "", "comma-separated repository slugs")
	listNames := fs.String("lists", "", "comma-separated list names")
	operation := fs.String("operation", "merge", "merge|replace|remove|replace_managed")
	managedPrefix := fs.String("managed-prefix", "", "managed prefix for replace_managed")
	createMissingLists := fs.Bool("create-missing-lists", false, "create missing target lists")
	starMissing := fs.Bool("star-missing", false, "star repositories before assigning them to lists")
	execute := fs.Bool("execute", false, "apply membership changes")
	fs.Parse(args)

	repoList := splitCSV(*repos)
	if len(repoList) == 0 {
		fatalIf(fmt.Errorf("repos is required"))
	}

	client := mustClient(ctx)
	requests := make([]githubstars.RepoListMutationRequest, 0, len(repoList))
	for _, repo := range repoList {
		requests = append(requests, githubstars.RepoListMutationRequest{
			Repo:              repo,
			TargetLists:       splitCSV(*listNames),
			Operation:         githubstars.MembershipOperation(strings.TrimSpace(*operation)),
			ManagedListPrefix: *managedPrefix,
			CreateMissing:     *createMissingLists,
			StarMissing:       *starMissing,
		})
	}
	results, err := client.SetRepoMembership(ctx, requests, *execute)
	fatalIf(err)
	printJSON(results)
}

func runSuggestTaxonomy(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("suggest-taxonomy", flag.ExitOnError)
	query := fs.String("query", "", "substring filter")
	limit := fs.Int("limit", 8, "max suggestions")
	fs.Parse(args)

	client := mustClient(ctx)
	viewer, err := client.Viewer(ctx)
	fatalIf(err)
	repos, err := client.ListStars(ctx, githubstars.StarsFilter{
		Query: *query,
		Limit: 500,
	})
	fatalIf(err)
	githubSuggested, err := client.SuggestedListNames(ctx)
	fatalIf(err)
	printJSON(map[string]any{
		"viewer":             viewer.Login,
		"github_suggestions": githubSuggested,
		"suggestions":        githubstars.SuggestTaxonomy(repos, githubSuggested, *limit),
	})
}

func runCleanupCandidates(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("cleanup-candidates", flag.ExitOnError)
	inactiveDays := fs.Int("inactive-days", 365, "minimum days since last update")
	includeArchived := fs.Bool("include-archived", true, "include archived repositories")
	includeForks := fs.Bool("include-forks", true, "include forked repositories")
	requireUnlisted := fs.Bool("require-unlisted", false, "flag repositories that are not in any list")
	managedPrefix := fs.String("managed-prefix", "", "flag repositories missing this managed list prefix")
	limit := fs.Int("limit", 100, "max candidates to return")
	fs.Parse(args)

	client := mustClient(ctx)
	viewer, err := client.Viewer(ctx)
	fatalIf(err)
	candidates, err := client.CleanupCandidates(ctx, githubstars.CleanupOptions{
		InactiveDays:      *inactiveDays,
		IncludeArchived:   *includeArchived,
		IncludeForks:      *includeForks,
		RequireUnlisted:   *requireUnlisted,
		ManagedListPrefix: *managedPrefix,
		Limit:             *limit,
	})
	fatalIf(err)
	printJSON(map[string]any{
		"viewer":     viewer.Login,
		"total":      len(candidates),
		"candidates": candidates,
	})
}

func runAuditTaxonomy(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("audit-taxonomy", flag.ExitOnError)
	managedPrefix := fs.String("managed-prefix", "", "managed list prefix to audit")
	maxItems := fs.Int("max-items", 100, "max items to return per audit bucket")
	bootstrapDefaults := fs.Bool("bootstrap-defaults", false, "audit against the built-in MCP bootstrap assignments")
	production := fs.String("production", "", "comma-separated production repo slugs")
	experimental := fs.String("experimental", "", "comma-separated experimental repo slugs")
	alpha := fs.String("alpha", "", "comma-separated alpha repo slugs")
	fs.Parse(args)

	assignments := taxonomyAssignmentsFromBands(splitCSV(*production), splitCSV(*experimental), splitCSV(*alpha))
	if *bootstrapDefaults && len(assignments) == 0 {
		_, assignments = githubstars.DefaultBootstrapSpecs()
	}

	client := mustClient(ctx)
	viewer, err := client.Viewer(ctx)
	fatalIf(err)
	audit, err := client.AuditTaxonomy(ctx, assignments, *managedPrefix)
	fatalIf(err)
	audit = trimAuditForCLI(audit, *maxItems)
	printJSON(map[string]any{
		"viewer": viewer.Login,
		"audit":  audit,
	})
}

func runAuditMarkdown(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("audit-markdown", flag.ExitOnError)
	sourceDir := fs.String("source-dir", "", "directory containing markdown source files")
	sources := fs.String("sources", "", "comma-separated markdown file paths")
	maxItems := fs.Int("max-items", 100, "max missing or extra repos per list")
	fs.Parse(args)

	if strings.TrimSpace(*sourceDir) == "" && strings.TrimSpace(*sources) == "" {
		if path := defaultMarkdownSourceDir(); path != "" {
			*sourceDir = path
		}
	}

	loadedSources, err := githubstars.ParseMarkdownSources(*sourceDir, splitCSV(*sources))
	fatalIf(err)

	client := mustClient(ctx)
	viewer, err := client.Viewer(ctx)
	fatalIf(err)
	currentLists, err := client.ListLists(ctx, true, 0)
	fatalIf(err)
	audit := githubstars.TrimMarkdownAudit(githubstars.BuildMarkdownAudit(loadedSources, currentLists), *maxItems)
	printJSON(map[string]any{
		"viewer": viewer.Login,
		"audit":  audit,
	})
}

func runSyncMarkdown(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("sync-markdown", flag.ExitOnError)
	sourceDir := fs.String("source-dir", "", "directory containing markdown source files")
	sources := fs.String("sources", "", "comma-separated markdown file paths")
	descriptionTemplate := fs.String("description-template", "Managed by docs-mcp github-reference-repos import for %s", "list description template")
	isPrivate := fs.Bool("private", true, "create or update target lists as private")
	starMissing := fs.Bool("star-missing", true, "star repositories before assigning them to lists")
	execute := fs.Bool("execute", false, "apply star and list changes")
	fs.Parse(args)

	if strings.TrimSpace(*sourceDir) == "" && strings.TrimSpace(*sources) == "" {
		if path := defaultMarkdownSourceDir(); path != "" {
			*sourceDir = path
		}
	}

	loadedSources, err := githubstars.ParseMarkdownSources(*sourceDir, splitCSV(*sources))
	fatalIf(err)
	lists, assignments, overlaps := githubstars.BuildMarkdownTaxonomy(loadedSources, *descriptionTemplate, *isPrivate)

	client := mustClient(ctx)
	viewer, err := client.Viewer(ctx)
	fatalIf(err)
	taxonomy, err := client.SyncExactTaxonomy(ctx, lists, assignments, *starMissing, *execute)
	fatalIf(err)

	uniqueRepos := 0
	for _, assignment := range assignments {
		if strings.TrimSpace(assignment.Repo) != "" {
			uniqueRepos++
		}
	}
	printJSON(map[string]any{
		"viewer":       viewer.Login,
		"sources":      loadedSources,
		"unique_repos": uniqueRepos,
		"overlaps":     overlaps,
		"taxonomy":     taxonomy,
	})
}

func runInstallCodex(args []string) {
	fs := flag.NewFlagSet("install-codex-mcp", flag.ExitOnError)
	configPath := fs.String("config", "", "Codex config path override")
	dotfilesDir := fs.String("dotfiles-dir", "", "dotfiles root override")
	execute := fs.Bool("execute", false, "write the config instead of dry-run")
	fs.Parse(args)

	result, err := githubstars.InstallCodexConfig(*configPath, *dotfilesDir, *execute)
	fatalIf(err)
	printJSON(result)
}

func runBootstrap(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	production := fs.String("production", "", "comma-separated production repo slugs")
	experimental := fs.String("experimental", "", "comma-separated experimental repo slugs")
	alpha := fs.String("alpha", "", "comma-separated alpha repo slugs")
	installCodex := fs.Bool("install-codex-mcp", false, "also install global Codex MCP entries")
	configPath := fs.String("config", "", "Codex config path override")
	dotfilesDir := fs.String("dotfiles-dir", "", "dotfiles root override")
	execute := fs.Bool("execute", false, "apply star and list mutations")
	fs.Parse(args)

	client := mustClient(ctx)
	plan, err := client.Bootstrap(ctx, splitCSV(*production), splitCSV(*experimental), splitCSV(*alpha), *installCodex, *configPath, *dotfilesDir, *execute)
	fatalIf(err)
	printJSON(plan)
}

func mustClient(ctx context.Context) *githubstars.Client {
	client, err := githubstars.NewClientFromEnv(ctx)
	fatalIf(err)
	return client
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func taxonomyAssignmentsFromBands(production, experimental, alpha []string) []githubstars.TaxonomyAssignment {
	assignments := make([]githubstars.TaxonomyAssignment, 0, len(production)+len(experimental)+len(alpha))
	for _, repo := range production {
		assignments = append(assignments, githubstars.TaxonomyAssignment{Repo: repo, Lists: []string{"MCP / Production"}})
	}
	for _, repo := range experimental {
		assignments = append(assignments, githubstars.TaxonomyAssignment{Repo: repo, Lists: []string{"MCP / Experimental"}})
	}
	for _, repo := range alpha {
		assignments = append(assignments, githubstars.TaxonomyAssignment{Repo: repo, Lists: []string{"MCP / Alpha"}})
	}
	return assignments
}

func trimAuditForCLI(audit githubstars.TaxonomyAudit, limit int) githubstars.TaxonomyAudit {
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

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	fatalIf(enc.Encode(v))
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func defaultMarkdownSourceDir() string {
	candidate := "/home/hg/github-reference-repos/index-files"
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return ""
}
