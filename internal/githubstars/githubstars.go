package githubstars

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	graphQLEndpoint = "https://api.github.com/graphql"

	CodexStartMarker = "# BEGIN MANAGED MCP SERVERS: github-stars"
	CodexEndMarker   = "# END MANAGED MCP SERVERS: github-stars"
)

type Client struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
}

type ViewerProfile struct {
	Login string `json:"login"`
}

type ListRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type StarredRepository struct {
	ID               string    `json:"id"`
	NameWithOwner    string    `json:"name_with_owner"`
	Description      string    `json:"description,omitempty"`
	URL              string    `json:"url,omitempty"`
	IsArchived       bool      `json:"is_archived"`
	IsFork           bool      `json:"is_fork"`
	ViewerHasStarred bool      `json:"viewer_has_starred"`
	StargazerCount   int       `json:"stargazer_count"`
	PrimaryLanguage  string    `json:"primary_language,omitempty"`
	Topics           []string  `json:"topics,omitempty"`
	UpdatedAt        string    `json:"updated_at,omitempty"`
	Lists            []ListRef `json:"lists,omitempty"`
}

type UserList struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	IsPrivate   bool              `json:"is_private"`
	Slug        string            `json:"slug,omitempty"`
	TotalCount  int               `json:"total_count"`
	Items       []StarredListItem `json:"items,omitempty"`
}

type StarredListItem struct {
	ID            string `json:"id"`
	NameWithOwner string `json:"name_with_owner"`
}

type EnsureListSpec struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsPrivate   bool   `json:"is_private,omitempty"`
}

type EnsureListResult struct {
	Name         string `json:"name"`
	ID           string `json:"id,omitempty"`
	Action       string `json:"action"`
	Status       string `json:"status"`
	Description  string `json:"description,omitempty"`
	IsPrivate    bool   `json:"is_private,omitempty"`
	CurrentState string `json:"current_state,omitempty"`
	DesiredState string `json:"desired_state,omitempty"`
	Message      string `json:"message,omitempty"`
}

type DeleteListResult struct {
	Name    string `json:"name"`
	ID      string `json:"id,omitempty"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type RenameListResult struct {
	OldName string `json:"old_name"`
	NewName string `json:"new_name"`
	ID      string `json:"id,omitempty"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type StarMutationResult struct {
	Repo          string `json:"repo"`
	Action        string `json:"action"`
	Status        string `json:"status"`
	WasStarred    bool   `json:"was_starred"`
	WillBeStarred bool   `json:"will_be_starred"`
	Message       string `json:"message,omitempty"`
}

type MembershipOperation string

const (
	OperationMerge          MembershipOperation = "merge"
	OperationReplace        MembershipOperation = "replace"
	OperationRemove         MembershipOperation = "remove"
	OperationReplaceManaged MembershipOperation = "replace_managed"
)

type RepoListMutationRequest struct {
	Repo              string              `json:"repo"`
	TargetLists       []string            `json:"target_lists,omitempty"`
	Operation         MembershipOperation `json:"operation,omitempty"`
	ManagedListPrefix string              `json:"managed_list_prefix,omitempty"`
	CreateMissing     bool                `json:"create_missing,omitempty"`
	StarMissing       bool                `json:"star_missing,omitempty"`
}

type RepoListMutationResult struct {
	Repo         string   `json:"repo"`
	Action       string   `json:"action"`
	Status       string   `json:"status"`
	CurrentLists []string `json:"current_lists,omitempty"`
	DesiredLists []string `json:"desired_lists,omitempty"`
	Message      string   `json:"message,omitempty"`
}

type TaxonomyAssignment struct {
	Repo  string   `json:"repo"`
	Lists []string `json:"lists"`
}

type TaxonomySyncResult struct {
	ListResults []EnsureListResult       `json:"list_results,omitempty"`
	RepoResults []RepoListMutationResult `json:"repo_results,omitempty"`
}

type SuggestedList struct {
	Name         string   `json:"name"`
	Source       string   `json:"source"`
	MatchedRepos int      `json:"matched_repos"`
	Repos        []string `json:"repos,omitempty"`
	Rationale    string   `json:"rationale,omitempty"`
}

type CountBucket struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type StarsSummary struct {
	TotalStars        int           `json:"total_stars"`
	TotalLists        int           `json:"total_lists"`
	ArchivedCount     int           `json:"archived_count"`
	ForkCount         int           `json:"fork_count"`
	UnlistedCount     int           `json:"unlisted_count"`
	ManagedListPrefix string        `json:"managed_list_prefix,omitempty"`
	MissingManaged    int           `json:"missing_managed_count,omitempty"`
	Lists             []CountBucket `json:"lists,omitempty"`
	Languages         []CountBucket `json:"languages,omitempty"`
	Topics            []CountBucket `json:"topics,omitempty"`
}

type CleanupOptions struct {
	InactiveDays      int       `json:"inactive_days,omitempty"`
	IncludeArchived   bool      `json:"include_archived"`
	IncludeForks      bool      `json:"include_forks"`
	RequireUnlisted   bool      `json:"require_unlisted,omitempty"`
	ManagedListPrefix string    `json:"managed_list_prefix,omitempty"`
	Limit             int       `json:"limit,omitempty"`
	Now               time.Time `json:"-"`
}

type CleanupCandidate struct {
	Repo            string   `json:"repo"`
	CurrentLists    []string `json:"current_lists,omitempty"`
	Reasons         []string `json:"reasons"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
	UpdatedDaysAgo  int      `json:"updated_days_ago,omitempty"`
	StargazerCount  int      `json:"stargazer_count,omitempty"`
	PrimaryLanguage string   `json:"primary_language,omitempty"`
}

type RepoAuditGap struct {
	Repo         string   `json:"repo"`
	CurrentLists []string `json:"current_lists,omitempty"`
	Message      string   `json:"message"`
}

type TaxonomyDrift struct {
	Repo                string   `json:"repo"`
	CurrentLists        []string `json:"current_lists,omitempty"`
	CurrentManagedLists []string `json:"current_managed_lists,omitempty"`
	DesiredLists        []string `json:"desired_lists,omitempty"`
}

type TaxonomyAudit struct {
	ManagedListPrefix   string          `json:"managed_list_prefix,omitempty"`
	TotalStars          int             `json:"total_stars"`
	ManagedLists        []string        `json:"managed_lists,omitempty"`
	ReposMissingManaged []RepoAuditGap  `json:"repos_missing_managed,omitempty"`
	ReposOutsideProfile []RepoAuditGap  `json:"repos_outside_profile,omitempty"`
	ReposWithDrift      []TaxonomyDrift `json:"repos_with_drift,omitempty"`
	DesiredReposMissing []string        `json:"desired_repos_not_starred,omitempty"`
}

type CodexInstallResult struct {
	ConfigPath   string   `json:"config_path"`
	DotfilesDir  string   `json:"dotfiles_dir"`
	Status       string   `json:"status"`
	Changed      bool     `json:"changed"`
	Servers      []string `json:"servers"`
	BlockPreview string   `json:"block_preview"`
	Warning      string   `json:"warning,omitempty"`
}

type BootstrapPlan struct {
	Lists        []EnsureListSpec     `json:"lists"`
	Assignments  []TaxonomyAssignment `json:"assignments"`
	StarResults  []StarMutationResult `json:"star_results,omitempty"`
	Taxonomy     TaxonomySyncResult   `json:"taxonomy"`
	CodexInstall *CodexInstallResult  `json:"codex_install,omitempty"`
}

type StarsFilter struct {
	Query        string
	Language     string
	Topic        string
	ListNames    []string
	Limit        int
	IncludeLists bool
	SortBy       string
}

func NewClientFromEnv(ctx context.Context) (*Client, error) {
	token, err := ResolveToken(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{
		Token:   token,
		BaseURL: graphQLEndpoint,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func ResolveToken(ctx context.Context) (string, error) {
	if token := resolveTokenFromDefaultEnvFile(); token != "" {
		return token, nil
	}
	for _, key := range []string{"GITHUB_PAT", "GITHUB_TOKEN", "GH_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value, nil
		}
	}

	cmd := exec.CommandContext(ctx, "gh", "auth", "token")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		if token := strings.TrimSpace(stdout.String()); token != "" {
			return token, nil
		}
	}

	return "", fmt.Errorf("no GitHub token available in ~/.env GITHUB_PAT, GITHUB_PAT, GITHUB_TOKEN, GH_TOKEN, or gh auth")
}

func resolveTokenFromDefaultEnvFile() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return readEnvFileValue(filepath.Join(home, ".env"), "GITHUB_PAT")
}

func readEnvFileValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, raw, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		value := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2:
			return strings.Trim(value, "\"")
		case strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2:
			return strings.Trim(value, "'")
		default:
			if idx := strings.Index(value, " #"); idx >= 0 {
				value = value[:idx]
			}
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (c *Client) Viewer(ctx context.Context) (ViewerProfile, error) {
	const query = `query { viewer { login } }`
	var resp struct {
		Viewer ViewerProfile `json:"viewer"`
	}
	if err := c.graphQL(ctx, query, nil, &resp); err != nil {
		return ViewerProfile{}, err
	}
	return resp.Viewer, nil
}

func (c *Client) SuggestedListNames(ctx context.Context) ([]string, error) {
	const query = `query { viewer { suggestedListNames { name } } }`
	var resp struct {
		Viewer struct {
			SuggestedListNames []struct {
				Name string `json:"name"`
			} `json:"suggestedListNames"`
		} `json:"viewer"`
	}
	if err := c.graphQL(ctx, query, nil, &resp); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(resp.Viewer.SuggestedListNames))
	for _, item := range resp.Viewer.SuggestedListNames {
		if strings.TrimSpace(item.Name) != "" {
			names = append(names, item.Name)
		}
	}
	return dedupeStrings(names), nil
}

func (c *Client) ListLists(ctx context.Context, includeItems bool, itemsPerList int) ([]UserList, error) {
	type listNode struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		IsPrivate   bool   `json:"isPrivate"`
		Slug        string `json:"slug"`
		Items       struct {
			TotalCount int `json:"totalCount"`
		} `json:"items"`
	}

	const query = `
query($after: String) {
  viewer {
    lists(first: 100, after: $after) {
      nodes {
        id
        name
        description
        isPrivate
        slug
        items(first: 1) { totalCount }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}`

	var lists []UserList
	var after *string
	for {
		var resp struct {
			Viewer struct {
				Lists struct {
					Nodes    []listNode `json:"nodes"`
					PageInfo pageInfo   `json:"pageInfo"`
				} `json:"lists"`
			} `json:"viewer"`
		}
		vars := map[string]any{"after": after}
		if err := c.graphQL(ctx, query, vars, &resp); err != nil {
			return nil, err
		}
		for _, node := range resp.Viewer.Lists.Nodes {
			list := UserList{
				ID:          node.ID,
				Name:        node.Name,
				Description: node.Description,
				IsPrivate:   node.IsPrivate,
				Slug:        node.Slug,
				TotalCount:  node.Items.TotalCount,
			}
			if includeItems {
				items, totalCount, err := c.listItemsForList(ctx, node.ID, itemsPerList)
				if err != nil {
					return nil, err
				}
				list.TotalCount = totalCount
				list.Items = items
			}
			lists = append(lists, list)
		}
		if !resp.Viewer.Lists.PageInfo.HasNextPage {
			break
		}
		next := resp.Viewer.Lists.PageInfo.EndCursor
		after = &next
	}

	sort.Slice(lists, func(i, j int) bool {
		return strings.ToLower(lists[i].Name) < strings.ToLower(lists[j].Name)
	})
	return lists, nil
}

func (c *Client) ListStars(ctx context.Context, filter StarsFilter) ([]StarredRepository, error) {
	const query = `
query($after: String) {
  viewer {
    starredRepositories(first: 100, after: $after, orderBy: { field: STARRED_AT, direction: DESC }) {
      nodes {
        id
        nameWithOwner
        description
        url
        isArchived
        isFork
        viewerHasStarred
        stargazerCount
        updatedAt
        primaryLanguage { name }
        repositoryTopics(first: 20) { nodes { topic { name } } }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}`

	var stars []StarredRepository
	var after *string
	for {
		var resp struct {
			Viewer struct {
				StarredRepositories struct {
					Nodes []struct {
						ID               string `json:"id"`
						NameWithOwner    string `json:"nameWithOwner"`
						Description      string `json:"description"`
						URL              string `json:"url"`
						IsArchived       bool   `json:"isArchived"`
						IsFork           bool   `json:"isFork"`
						ViewerHasStarred bool   `json:"viewerHasStarred"`
						StargazerCount   int    `json:"stargazerCount"`
						UpdatedAt        string `json:"updatedAt"`
						PrimaryLanguage  *struct {
							Name string `json:"name"`
						} `json:"primaryLanguage"`
						RepositoryTopics struct {
							Nodes []struct {
								Topic struct {
									Name string `json:"name"`
								} `json:"topic"`
							} `json:"nodes"`
						} `json:"repositoryTopics"`
					} `json:"nodes"`
					PageInfo pageInfo `json:"pageInfo"`
				} `json:"starredRepositories"`
			} `json:"viewer"`
		}
		vars := map[string]any{"after": after}
		if err := c.graphQL(ctx, query, vars, &resp); err != nil {
			return nil, err
		}

		for _, node := range resp.Viewer.StarredRepositories.Nodes {
			repo := StarredRepository{
				ID:               node.ID,
				NameWithOwner:    node.NameWithOwner,
				Description:      node.Description,
				URL:              node.URL,
				IsArchived:       node.IsArchived,
				IsFork:           node.IsFork,
				ViewerHasStarred: node.ViewerHasStarred,
				StargazerCount:   node.StargazerCount,
				UpdatedAt:        node.UpdatedAt,
			}
			if node.PrimaryLanguage != nil {
				repo.PrimaryLanguage = node.PrimaryLanguage.Name
			}
			for _, topicNode := range node.RepositoryTopics.Nodes {
				if strings.TrimSpace(topicNode.Topic.Name) != "" {
					repo.Topics = append(repo.Topics, topicNode.Topic.Name)
				}
			}
			stars = append(stars, repo)
		}

		if filter.Limit > 0 && len(stars) >= filter.Limit && filter.Query == "" && filter.Language == "" && filter.Topic == "" && len(filter.ListNames) == 0 {
			stars = stars[:filter.Limit]
			break
		}
		if !resp.Viewer.StarredRepositories.PageInfo.HasNextPage {
			break
		}
		next := resp.Viewer.StarredRepositories.PageInfo.EndCursor
		after = &next
	}

	if filter.IncludeLists {
		membership, err := c.repoMembershipMap(ctx)
		if err != nil {
			return nil, err
		}
		for i := range stars {
			stars[i].Lists = membership[stars[i].NameWithOwner]
		}
	}

	stars = filterStars(stars, filter)
	sortStars(stars, filter.SortBy)
	if filter.Limit > 0 && len(stars) > filter.Limit {
		stars = stars[:filter.Limit]
	}
	return stars, nil
}

func (c *Client) ResolveRepositories(ctx context.Context, repos []string) ([]StarredRepository, error) {
	results := make([]StarredRepository, 0, len(repos))
	for _, repo := range repos {
		resolved, err := c.ResolveRepository(ctx, repo)
		if err != nil {
			return nil, err
		}
		results = append(results, resolved)
	}
	return results, nil
}

func (c *Client) ResolveRepository(ctx context.Context, repo string) (StarredRepository, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return StarredRepository{}, err
	}

	const query = `
query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    id
    nameWithOwner
    description
    url
    isArchived
    isFork
    viewerHasStarred
    stargazerCount
    updatedAt
    primaryLanguage { name }
    repositoryTopics(first: 20) { nodes { topic { name } } }
  }
}`

	var resp struct {
		Repository *struct {
			ID               string `json:"id"`
			NameWithOwner    string `json:"nameWithOwner"`
			Description      string `json:"description"`
			URL              string `json:"url"`
			IsArchived       bool   `json:"isArchived"`
			IsFork           bool   `json:"isFork"`
			ViewerHasStarred bool   `json:"viewerHasStarred"`
			StargazerCount   int    `json:"stargazerCount"`
			UpdatedAt        string `json:"updatedAt"`
			PrimaryLanguage  *struct {
				Name string `json:"name"`
			} `json:"primaryLanguage"`
			RepositoryTopics struct {
				Nodes []struct {
					Topic struct {
						Name string `json:"name"`
					} `json:"topic"`
				} `json:"nodes"`
			} `json:"repositoryTopics"`
		} `json:"repository"`
	}

	if err := c.graphQL(ctx, query, map[string]any{
		"owner": owner,
		"name":  name,
	}, &resp); err != nil {
		return StarredRepository{}, err
	}
	if resp.Repository == nil {
		return StarredRepository{}, fmt.Errorf("repository not found: %s", repo)
	}

	out := StarredRepository{
		ID:               resp.Repository.ID,
		NameWithOwner:    resp.Repository.NameWithOwner,
		Description:      resp.Repository.Description,
		URL:              resp.Repository.URL,
		IsArchived:       resp.Repository.IsArchived,
		IsFork:           resp.Repository.IsFork,
		ViewerHasStarred: resp.Repository.ViewerHasStarred,
		StargazerCount:   resp.Repository.StargazerCount,
		UpdatedAt:        resp.Repository.UpdatedAt,
	}
	if resp.Repository.PrimaryLanguage != nil {
		out.PrimaryLanguage = resp.Repository.PrimaryLanguage.Name
	}
	for _, topicNode := range resp.Repository.RepositoryTopics.Nodes {
		if strings.TrimSpace(topicNode.Topic.Name) != "" {
			out.Topics = append(out.Topics, topicNode.Topic.Name)
		}
	}
	return out, nil
}

func (c *Client) EnsureLists(ctx context.Context, specs []EnsureListSpec, execute bool) ([]EnsureListResult, error) {
	existing, err := c.ListLists(ctx, false, 0)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]UserList, len(existing))
	for _, list := range existing {
		byName[list.Name] = list
	}

	results := make([]EnsureListResult, 0, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec.Name) == "" {
			continue
		}
		current, ok := byName[spec.Name]
		if !ok {
			action := "dry-run-create"
			status := "planned"
			id := ""
			if execute {
				created, err := c.createList(ctx, spec)
				if err != nil {
					results = append(results, EnsureListResult{
						Name:    spec.Name,
						Action:  "create",
						Status:  "error",
						Message: err.Error(),
					})
					continue
				}
				action = "create"
				status = "ok"
				id = created.ID
				current = created
			}
			results = append(results, EnsureListResult{
				Name:         spec.Name,
				ID:           id,
				Action:       action,
				Status:       status,
				Description:  spec.Description,
				IsPrivate:    spec.IsPrivate,
				CurrentState: "missing",
				DesiredState: listStateString(spec.Description, spec.IsPrivate),
				Message:      "list does not exist yet",
			})
			continue
		}

		currentState := listStateString(current.Description, current.IsPrivate)
		desiredState := listStateString(spec.Description, spec.IsPrivate)
		if current.Description == spec.Description && current.IsPrivate == spec.IsPrivate {
			results = append(results, EnsureListResult{
				Name:         spec.Name,
				ID:           current.ID,
				Action:       "unchanged",
				Status:       "ok",
				Description:  current.Description,
				IsPrivate:    current.IsPrivate,
				CurrentState: currentState,
				DesiredState: desiredState,
				Message:      "list already matches desired state",
			})
			continue
		}

		action := "dry-run-update"
		status := "planned"
		if execute {
			updated, err := c.updateList(ctx, current.ID, spec)
			if err != nil {
				results = append(results, EnsureListResult{
					Name:         spec.Name,
					ID:           current.ID,
					Action:       "update",
					Status:       "error",
					CurrentState: currentState,
					DesiredState: desiredState,
					Message:      err.Error(),
				})
				continue
			}
			current = updated
			action = "update"
			status = "ok"
		}

		results = append(results, EnsureListResult{
			Name:         spec.Name,
			ID:           current.ID,
			Action:       action,
			Status:       status,
			Description:  spec.Description,
			IsPrivate:    spec.IsPrivate,
			CurrentState: currentState,
			DesiredState: desiredState,
			Message:      "list metadata differs",
		})
	}

	return results, nil
}

func (c *Client) DeleteLists(ctx context.Context, names []string, execute bool) ([]DeleteListResult, error) {
	existing, err := c.ListLists(ctx, false, 0)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]UserList, len(existing))
	for _, list := range existing {
		byName[list.Name] = list
	}

	results := make([]DeleteListResult, 0, len(names))
	for _, name := range dedupeStrings(names) {
		current, ok := byName[name]
		if !ok {
			results = append(results, DeleteListResult{
				Name:    name,
				Action:  "delete",
				Status:  "skipped",
				Message: "list not found",
			})
			continue
		}

		action := "dry-run-delete"
		status := "planned"
		if execute {
			if err := c.deleteList(ctx, current.ID); err != nil {
				results = append(results, DeleteListResult{
					Name:    current.Name,
					ID:      current.ID,
					Action:  "delete",
					Status:  "error",
					Message: err.Error(),
				})
				continue
			}
			action = "delete"
			status = "ok"
		}
		results = append(results, DeleteListResult{
			Name:    current.Name,
			ID:      current.ID,
			Action:  action,
			Status:  status,
			Message: "list scheduled for deletion",
		})
	}

	return results, nil
}

func (c *Client) RenameList(ctx context.Context, oldName, newName string, execute bool) (RenameListResult, error) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return RenameListResult{}, fmt.Errorf("old and new list names are required")
	}

	existing, err := c.ListLists(ctx, false, 0)
	if err != nil {
		return RenameListResult{}, err
	}

	var current UserList
	found := false
	targetTaken := false
	for _, list := range existing {
		if list.Name == oldName {
			current = list
			found = true
		}
		if list.Name == newName && oldName != newName {
			targetTaken = true
		}
	}

	if !found {
		return RenameListResult{
			OldName: oldName,
			NewName: newName,
			Action:  "rename",
			Status:  "error",
			Message: "source list not found",
		}, nil
	}
	if oldName == newName {
		return RenameListResult{
			OldName: oldName,
			NewName: newName,
			ID:      current.ID,
			Action:  "noop",
			Status:  "ok",
			Message: "list already has the desired name",
		}, nil
	}
	if targetTaken {
		return RenameListResult{
			OldName: oldName,
			NewName: newName,
			ID:      current.ID,
			Action:  "rename",
			Status:  "error",
			Message: "destination list name already exists",
		}, nil
	}

	action := "dry-run-rename"
	status := "planned"
	if execute {
		updated, err := c.updateList(ctx, current.ID, EnsureListSpec{
			Name:        newName,
			Description: current.Description,
			IsPrivate:   current.IsPrivate,
		})
		if err != nil {
			return RenameListResult{
				OldName: oldName,
				NewName: newName,
				ID:      current.ID,
				Action:  "rename",
				Status:  "error",
				Message: err.Error(),
			}, nil
		}
		current = updated
		action = "rename"
		status = "ok"
	}

	return RenameListResult{
		OldName: oldName,
		NewName: newName,
		ID:      current.ID,
		Action:  action,
		Status:  status,
		Message: "list name will change",
	}, nil
}

func (c *Client) SetStarState(ctx context.Context, repos []string, shouldStar bool, execute bool) ([]StarMutationResult, error) {
	results := make([]StarMutationResult, 0, len(repos))
	for _, repoSlug := range dedupeStrings(repos) {
		repo, err := c.ResolveRepository(ctx, repoSlug)
		if err != nil {
			results = append(results, StarMutationResult{
				Repo:    repoSlug,
				Action:  boolAction("star", "unstar", shouldStar),
				Status:  "error",
				Message: err.Error(),
			})
			continue
		}

		if repo.ViewerHasStarred == shouldStar {
			results = append(results, StarMutationResult{
				Repo:          repo.NameWithOwner,
				Action:        "noop",
				Status:        "ok",
				WasStarred:    repo.ViewerHasStarred,
				WillBeStarred: shouldStar,
				Message:       "repository already in desired starred state",
			})
			continue
		}

		action := boolAction("dry-run-star", "dry-run-unstar", shouldStar)
		status := "planned"
		if execute {
			if shouldStar {
				if err := c.addStar(ctx, repo.ID); err != nil {
					results = append(results, StarMutationResult{
						Repo:          repo.NameWithOwner,
						Action:        "star",
						Status:        "error",
						WasStarred:    repo.ViewerHasStarred,
						WillBeStarred: shouldStar,
						Message:       err.Error(),
					})
					continue
				}
				action = "star"
			} else {
				if err := c.removeStar(ctx, repo.ID); err != nil {
					results = append(results, StarMutationResult{
						Repo:          repo.NameWithOwner,
						Action:        "unstar",
						Status:        "error",
						WasStarred:    repo.ViewerHasStarred,
						WillBeStarred: shouldStar,
						Message:       err.Error(),
					})
					continue
				}
				action = "unstar"
			}
			status = "ok"
		}

		results = append(results, StarMutationResult{
			Repo:          repo.NameWithOwner,
			Action:        action,
			Status:        status,
			WasStarred:    repo.ViewerHasStarred,
			WillBeStarred: shouldStar,
			Message:       "repository state differs from desired starred state",
		})
	}
	return results, nil
}

func (c *Client) SetRepoMembership(ctx context.Context, requests []RepoListMutationRequest, execute bool) ([]RepoListMutationResult, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	lists, err := c.ListLists(ctx, true, 0)
	if err != nil {
		return nil, err
	}
	listByName := make(map[string]UserList, len(lists))
	membership := make(map[string][]string)
	for _, list := range lists {
		listByName[list.Name] = list
		for _, item := range list.Items {
			membership[item.NameWithOwner] = append(membership[item.NameWithOwner], list.Name)
		}
	}

	requiredLists := make(map[string]EnsureListSpec)
	for _, request := range requests {
		if request.CreateMissing {
			for _, listName := range request.TargetLists {
				if _, ok := listByName[listName]; !ok {
					requiredLists[listName] = EnsureListSpec{Name: listName}
				}
			}
		}
	}
	if len(requiredLists) > 0 {
		specs := make([]EnsureListSpec, 0, len(requiredLists))
		for _, spec := range requiredLists {
			specs = append(specs, spec)
		}
		_, err := c.EnsureLists(ctx, specs, execute)
		if err != nil {
			return nil, err
		}
		if execute {
			lists, err = c.ListLists(ctx, true, 0)
			if err != nil {
				return nil, err
			}
			listByName = make(map[string]UserList, len(lists))
			membership = make(map[string][]string)
			for _, list := range lists {
				listByName[list.Name] = list
				for _, item := range list.Items {
					membership[item.NameWithOwner] = append(membership[item.NameWithOwner], list.Name)
				}
			}
		} else {
			for name := range requiredLists {
				listByName[name] = UserList{
					ID:   "planned:" + name,
					Name: name,
				}
			}
		}
	}

	results := make([]RepoListMutationResult, 0, len(requests))
	for _, request := range requests {
		repo, err := c.ResolveRepository(ctx, request.Repo)
		if err != nil {
			results = append(results, RepoListMutationResult{
				Repo:    request.Repo,
				Action:  "membership",
				Status:  "error",
				Message: err.Error(),
			})
			continue
		}

		if !repo.ViewerHasStarred && request.StarMissing {
			starResults, err := c.SetStarState(ctx, []string{repo.NameWithOwner}, true, execute)
			if err != nil {
				return nil, err
			}
			if len(starResults) > 0 && starResults[0].Status == "error" {
				results = append(results, RepoListMutationResult{
					Repo:         repo.NameWithOwner,
					Action:       "membership",
					Status:       "error",
					CurrentLists: nil,
					Message:      "failed to star repo before list assignment: " + starResults[0].Message,
				})
				continue
			}
			repo.ViewerHasStarred = true
		}

		if !repo.ViewerHasStarred {
			results = append(results, RepoListMutationResult{
				Repo:         repo.NameWithOwner,
				Action:       "membership",
				Status:       "error",
				CurrentLists: membership[repo.NameWithOwner],
				Message:      "repository is not starred; set star_missing=true to auto-star before assignment",
			})
			continue
		}

		currentLists := dedupeStrings(membership[repo.NameWithOwner])
		desiredLists, err := calculateDesiredLists(currentLists, request.TargetLists, request.Operation, request.ManagedListPrefix)
		if err != nil {
			results = append(results, RepoListMutationResult{
				Repo:         repo.NameWithOwner,
				Action:       "membership",
				Status:       "error",
				CurrentLists: currentLists,
				Message:      err.Error(),
			})
			continue
		}

		if stringSlicesEqual(currentLists, desiredLists) {
			results = append(results, RepoListMutationResult{
				Repo:         repo.NameWithOwner,
				Action:       "noop",
				Status:       "ok",
				CurrentLists: currentLists,
				DesiredLists: desiredLists,
				Message:      "repository already matches desired list membership",
			})
			continue
		}

		listIDs := make([]string, 0, len(desiredLists))
		missingLists := make([]string, 0)
		for _, listName := range desiredLists {
			list, ok := listByName[listName]
			if !ok {
				missingLists = append(missingLists, listName)
				continue
			}
			listIDs = append(listIDs, list.ID)
		}
		if len(missingLists) > 0 {
			results = append(results, RepoListMutationResult{
				Repo:         repo.NameWithOwner,
				Action:       "membership",
				Status:       "error",
				CurrentLists: currentLists,
				DesiredLists: desiredLists,
				Message:      "missing destination lists: " + strings.Join(missingLists, ", "),
			})
			continue
		}

		action := "dry-run-membership"
		status := "planned"
		if execute {
			if err := c.updateItemLists(ctx, repo.ID, listIDs); err != nil {
				results = append(results, RepoListMutationResult{
					Repo:         repo.NameWithOwner,
					Action:       "membership",
					Status:       "error",
					CurrentLists: currentLists,
					DesiredLists: desiredLists,
					Message:      err.Error(),
				})
				continue
			}
			action = "membership"
			status = "ok"
			membership[repo.NameWithOwner] = desiredLists
		}

		results = append(results, RepoListMutationResult{
			Repo:         repo.NameWithOwner,
			Action:       action,
			Status:       status,
			CurrentLists: currentLists,
			DesiredLists: desiredLists,
			Message:      "list membership will change",
		})
	}

	return results, nil
}

func (c *Client) SyncTaxonomy(ctx context.Context, lists []EnsureListSpec, assignments []TaxonomyAssignment, operation MembershipOperation, managedPrefix string, starMissing bool, execute bool) (TaxonomySyncResult, error) {
	desiredLists := make(map[string]EnsureListSpec)
	for _, spec := range lists {
		if strings.TrimSpace(spec.Name) == "" {
			continue
		}
		desiredLists[spec.Name] = spec
	}
	for _, assignment := range assignments {
		for _, listName := range assignment.Lists {
			if _, ok := desiredLists[listName]; !ok {
				desiredLists[listName] = EnsureListSpec{Name: listName}
			}
		}
	}

	ensureSpecs := make([]EnsureListSpec, 0, len(desiredLists))
	for _, spec := range desiredLists {
		ensureSpecs = append(ensureSpecs, spec)
	}
	sort.Slice(ensureSpecs, func(i, j int) bool {
		return strings.ToLower(ensureSpecs[i].Name) < strings.ToLower(ensureSpecs[j].Name)
	})

	listResults, err := c.EnsureLists(ctx, ensureSpecs, execute)
	if err != nil {
		return TaxonomySyncResult{}, err
	}

	requests := make([]RepoListMutationRequest, 0, len(assignments))
	for _, assignment := range assignments {
		requests = append(requests, RepoListMutationRequest{
			Repo:              assignment.Repo,
			TargetLists:       assignment.Lists,
			Operation:         operation,
			ManagedListPrefix: managedPrefix,
			CreateMissing:     true,
			StarMissing:       starMissing,
		})
	}

	repoResults, err := c.SetRepoMembership(ctx, requests, execute)
	if err != nil {
		return TaxonomySyncResult{}, err
	}

	return TaxonomySyncResult{
		ListResults: listResults,
		RepoResults: repoResults,
	}, nil
}

func DefaultBootstrapSpecs() ([]EnsureListSpec, []TaxonomyAssignment) {
	lists := []EnsureListSpec{
		{Name: "MCP / Production", Description: "Stable MCP servers and production-ready GitHub automation."},
		{Name: "MCP / Experimental", Description: "Promising but still evolving MCP experiments."},
		{Name: "MCP / Alpha", Description: "Early-stage MCP projects worth watching but not production-ready."},
	}
	assignments := []TaxonomyAssignment{
		{Repo: "github/github-mcp-server", Lists: []string{"MCP / Production"}},
		{Repo: "microsoft/playwright-mcp", Lists: []string{"MCP / Production"}},
		{Repo: "SynapticSage/ganger", Lists: []string{"MCP / Alpha"}},
	}
	return lists, assignments
}

func (c *Client) Bootstrap(ctx context.Context, productionRepos, experimentalRepos, alphaRepos []string, installCodex bool, configPath, dotfilesDir string, execute bool) (BootstrapPlan, error) {
	defaultLists, defaultAssignments := DefaultBootstrapSpecs()
	if len(productionRepos) == 0 && len(experimentalRepos) == 0 && len(alphaRepos) == 0 {
		productionRepos = []string{"github/github-mcp-server", "microsoft/playwright-mcp"}
		alphaRepos = []string{"SynapticSage/ganger"}
	}

	lists := defaultLists
	assignments := defaultAssignments[:0]
	if len(productionRepos)+len(experimentalRepos)+len(alphaRepos) > 0 {
		assignments = make([]TaxonomyAssignment, 0, len(productionRepos)+len(experimentalRepos)+len(alphaRepos))
		for _, repo := range productionRepos {
			assignments = append(assignments, TaxonomyAssignment{Repo: repo, Lists: []string{"MCP / Production"}})
		}
		for _, repo := range experimentalRepos {
			assignments = append(assignments, TaxonomyAssignment{Repo: repo, Lists: []string{"MCP / Experimental"}})
		}
		for _, repo := range alphaRepos {
			assignments = append(assignments, TaxonomyAssignment{Repo: repo, Lists: []string{"MCP / Alpha"}})
		}
	}

	allRepos := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		allRepos = append(allRepos, assignment.Repo)
	}

	starResults, err := c.SetStarState(ctx, allRepos, true, execute)
	if err != nil {
		return BootstrapPlan{}, err
	}
	taxonomy, err := c.SyncTaxonomy(ctx, lists, assignments, OperationReplaceManaged, "MCP / ", true, execute)
	if err != nil {
		return BootstrapPlan{}, err
	}

	plan := BootstrapPlan{
		Lists:       lists,
		Assignments: assignments,
		StarResults: starResults,
		Taxonomy:    taxonomy,
	}
	if installCodex {
		result, err := InstallCodexConfig(configPath, dotfilesDir, execute)
		if err != nil {
			return BootstrapPlan{}, err
		}
		plan.CodexInstall = &result
	}
	return plan, nil
}

func (c *Client) Summary(ctx context.Context, topN int, managedPrefix string) (StarsSummary, error) {
	repos, err := c.ListStars(ctx, StarsFilter{IncludeLists: true})
	if err != nil {
		return StarsSummary{}, err
	}
	return SummarizeStars(repos, topN, managedPrefix), nil
}

func (c *Client) CleanupCandidates(ctx context.Context, options CleanupOptions) ([]CleanupCandidate, error) {
	repos, err := c.ListStars(ctx, StarsFilter{IncludeLists: true})
	if err != nil {
		return nil, err
	}
	return FindCleanupCandidates(repos, options), nil
}

func (c *Client) AuditTaxonomy(ctx context.Context, assignments []TaxonomyAssignment, managedPrefix string) (TaxonomyAudit, error) {
	repos, err := c.ListStars(ctx, StarsFilter{IncludeLists: true})
	if err != nil {
		return TaxonomyAudit{}, err
	}
	return BuildTaxonomyAudit(repos, assignments, managedPrefix), nil
}

func SuggestTaxonomy(repos []StarredRepository, githubSuggested []string, limit int) []SuggestedList {
	if limit <= 0 {
		limit = 8
	}

	type bucket struct {
		name      string
		source    string
		repos     []string
		rationale string
	}

	topics := map[string][]string{}
	languages := map[string][]string{}
	keywords := map[string][]string{
		"MCP":     {},
		"GitHub":  {},
		"Browser": {},
		"Agents":  {},
	}

	stopTopics := map[string]bool{
		"awesome":  true,
		"dotfiles": true,
		"cli":      true,
		"library":  true,
		"tools":    true,
	}

	for _, repo := range repos {
		for _, topic := range repo.Topics {
			key := strings.TrimSpace(strings.ToLower(topic))
			if key == "" || stopTopics[key] {
				continue
			}
			topics[key] = append(topics[key], repo.NameWithOwner)
		}
		if repo.PrimaryLanguage != "" {
			languages[repo.PrimaryLanguage] = append(languages[repo.PrimaryLanguage], repo.NameWithOwner)
		}
		lower := strings.ToLower(repo.NameWithOwner + " " + repo.Description + " " + strings.Join(repo.Topics, " "))
		if strings.Contains(lower, "mcp") || strings.Contains(lower, "model context protocol") {
			keywords["MCP"] = append(keywords["MCP"], repo.NameWithOwner)
		}
		if strings.Contains(lower, "github") {
			keywords["GitHub"] = append(keywords["GitHub"], repo.NameWithOwner)
		}
		if strings.Contains(lower, "browser") || strings.Contains(lower, "playwright") || strings.Contains(lower, "devtools") {
			keywords["Browser"] = append(keywords["Browser"], repo.NameWithOwner)
		}
		if strings.Contains(lower, "agent") || strings.Contains(lower, "orchestrat") || strings.Contains(lower, "swarm") {
			keywords["Agents"] = append(keywords["Agents"], repo.NameWithOwner)
		}
	}

	buckets := make([]bucket, 0)
	for name, matched := range keywords {
		matched = dedupeStrings(matched)
		if len(matched) >= 2 {
			buckets = append(buckets, bucket{
				name:      name,
				source:    "keyword",
				repos:     matched,
				rationale: "Name and topic heuristics found a repeated theme across your stars.",
			})
		}
	}
	for topic, matched := range topics {
		matched = dedupeStrings(matched)
		if len(matched) >= 2 {
			title := strings.ToUpper(topic[:1]) + topic[1:]
			buckets = append(buckets, bucket{
				name:      title,
				source:    "topic",
				repos:     matched,
				rationale: "GitHub topics show a repeated cluster worth its own list.",
			})
		}
	}
	for language, matched := range languages {
		matched = dedupeStrings(matched)
		if len(matched) >= 3 {
			buckets = append(buckets, bucket{
				name:      language,
				source:    "language",
				repos:     matched,
				rationale: "Primary language frequency suggests a language-focused list.",
			})
		}
	}
	for _, name := range githubSuggested {
		if strings.TrimSpace(name) == "" {
			continue
		}
		buckets = append(buckets, bucket{
			name:      name,
			source:    "github",
			rationale: "GitHub suggested this list name for your current stars.",
		})
	}

	sort.Slice(buckets, func(i, j int) bool {
		if len(buckets[i].repos) == len(buckets[j].repos) {
			return strings.ToLower(buckets[i].name) < strings.ToLower(buckets[j].name)
		}
		return len(buckets[i].repos) > len(buckets[j].repos)
	})

	seen := make(map[string]bool)
	out := make([]SuggestedList, 0, limit)
	for _, bucket := range buckets {
		key := strings.ToLower(strings.TrimSpace(bucket.name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, SuggestedList{
			Name:         bucket.name,
			Source:       bucket.source,
			MatchedRepos: len(bucket.repos),
			Repos:        bucket.repos,
			Rationale:    bucket.rationale,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func SummarizeStars(repos []StarredRepository, topN int, managedPrefix string) StarsSummary {
	if topN <= 0 {
		topN = 10
	}

	listCounts := make(map[string]int)
	languageCounts := make(map[string]int)
	topicCounts := make(map[string]int)

	summary := StarsSummary{
		TotalStars:        len(repos),
		ManagedListPrefix: managedPrefix,
	}
	for _, repo := range repos {
		if repo.IsArchived {
			summary.ArchivedCount++
		}
		if repo.IsFork {
			summary.ForkCount++
		}
		if len(repo.Lists) == 0 {
			summary.UnlistedCount++
		}
		if managedPrefix != "" && !hasListWithPrefix(repo.Lists, managedPrefix) {
			summary.MissingManaged++
		}
		for _, list := range repo.Lists {
			if strings.TrimSpace(list.Name) != "" {
				listCounts[list.Name]++
			}
		}
		if strings.TrimSpace(repo.PrimaryLanguage) != "" {
			languageCounts[repo.PrimaryLanguage]++
		}
		for _, topic := range repo.Topics {
			if strings.TrimSpace(topic) != "" {
				topicCounts[topic]++
			}
		}
	}

	summary.TotalLists = len(listCounts)
	summary.Lists = topBuckets(listCounts, topN)
	summary.Languages = topBuckets(languageCounts, topN)
	summary.Topics = topBuckets(topicCounts, topN)
	return summary
}

func FindCleanupCandidates(repos []StarredRepository, options CleanupOptions) []CleanupCandidate {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	if options.InactiveDays <= 0 {
		options.InactiveDays = 365
	}
	if options.Limit <= 0 {
		options.Limit = 100
	}

	candidates := make([]CleanupCandidate, 0)
	for _, repo := range repos {
		reasons := make([]string, 0, 4)
		if options.IncludeArchived && repo.IsArchived {
			reasons = append(reasons, "archived")
		}
		if options.IncludeForks && repo.IsFork {
			reasons = append(reasons, "fork")
		}

		updatedDaysAgo := 0
		if options.InactiveDays > 0 {
			if updatedAt, err := time.Parse(time.RFC3339, repo.UpdatedAt); err == nil {
				updatedDaysAgo = int(now.Sub(updatedAt).Hours() / 24)
				if updatedDaysAgo >= options.InactiveDays {
					reasons = append(reasons, fmt.Sprintf("inactive_%dd", options.InactiveDays))
				}
			}
		}

		listNames := listNames(repo.Lists)
		if options.RequireUnlisted && len(listNames) == 0 {
			reasons = append(reasons, "unlisted")
		}
		if options.ManagedListPrefix != "" && !hasListWithPrefix(repo.Lists, options.ManagedListPrefix) {
			reasons = append(reasons, "missing_managed_prefix")
		}

		if len(reasons) == 0 {
			continue
		}

		candidates = append(candidates, CleanupCandidate{
			Repo:            repo.NameWithOwner,
			CurrentLists:    listNames,
			Reasons:         reasons,
			UpdatedAt:       repo.UpdatedAt,
			UpdatedDaysAgo:  updatedDaysAgo,
			StargazerCount:  repo.StargazerCount,
			PrimaryLanguage: repo.PrimaryLanguage,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if len(candidates[i].Reasons) == len(candidates[j].Reasons) {
			if candidates[i].UpdatedDaysAgo == candidates[j].UpdatedDaysAgo {
				return strings.ToLower(candidates[i].Repo) < strings.ToLower(candidates[j].Repo)
			}
			return candidates[i].UpdatedDaysAgo > candidates[j].UpdatedDaysAgo
		}
		return len(candidates[i].Reasons) > len(candidates[j].Reasons)
	})
	if len(candidates) > options.Limit {
		return candidates[:options.Limit]
	}
	return candidates
}

func BuildTaxonomyAudit(repos []StarredRepository, assignments []TaxonomyAssignment, managedPrefix string) TaxonomyAudit {
	audit := TaxonomyAudit{
		ManagedListPrefix: managedPrefix,
		TotalStars:        len(repos),
	}

	currentByRepo := make(map[string]StarredRepository, len(repos))
	managedLists := make(map[string]bool)
	for _, repo := range repos {
		currentByRepo[repo.NameWithOwner] = repo
		for _, list := range repo.Lists {
			if strings.TrimSpace(list.Name) == "" {
				continue
			}
			if managedPrefix == "" || strings.HasPrefix(list.Name, managedPrefix) {
				managedLists[list.Name] = true
			}
		}
	}

	desiredByRepo := make(map[string][]string, len(assignments))
	for _, assignment := range assignments {
		if strings.TrimSpace(assignment.Repo) == "" {
			continue
		}
		desiredByRepo[assignment.Repo] = dedupeStrings(assignment.Lists)
		sort.Strings(desiredByRepo[assignment.Repo])
	}

	for _, repo := range repos {
		currentLists := listNames(repo.Lists)
		currentManaged := filterListsByPrefix(currentLists, managedPrefix)
		if len(currentManaged) == 0 {
			message := "repo has no managed classification"
			if managedPrefix == "" {
				message = "repo is not assigned to any list"
			} else if len(currentLists) == 0 {
				message = "repo is unlisted"
			}
			audit.ReposMissingManaged = append(audit.ReposMissingManaged, RepoAuditGap{
				Repo:         repo.NameWithOwner,
				CurrentLists: currentLists,
				Message:      message,
			})
		}

		desired, wanted := desiredByRepo[repo.NameWithOwner]
		if !wanted {
			if len(desiredByRepo) > 0 && len(currentManaged) > 0 {
				audit.ReposOutsideProfile = append(audit.ReposOutsideProfile, RepoAuditGap{
					Repo:         repo.NameWithOwner,
					CurrentLists: currentLists,
					Message:      "repo has managed lists but is not present in desired assignments",
				})
			}
			continue
		}
		if !stringSlicesEqual(currentManaged, desired) {
			audit.ReposWithDrift = append(audit.ReposWithDrift, TaxonomyDrift{
				Repo:                repo.NameWithOwner,
				CurrentLists:        currentLists,
				CurrentManagedLists: currentManaged,
				DesiredLists:        desired,
			})
		}
	}

	if len(desiredByRepo) > 0 {
		for repo := range desiredByRepo {
			if _, ok := currentByRepo[repo]; !ok {
				audit.DesiredReposMissing = append(audit.DesiredReposMissing, repo)
			}
		}
	}

	for name := range managedLists {
		audit.ManagedLists = append(audit.ManagedLists, name)
	}
	sort.Strings(audit.ManagedLists)
	sortRepoAudit(audit.ReposMissingManaged)
	sortRepoAudit(audit.ReposOutsideProfile)
	sort.Slice(audit.ReposWithDrift, func(i, j int) bool {
		return strings.ToLower(audit.ReposWithDrift[i].Repo) < strings.ToLower(audit.ReposWithDrift[j].Repo)
	})
	sort.Strings(audit.DesiredReposMissing)
	return audit
}

func InstallCodexConfig(configPath, dotfilesDir string, execute bool) (CodexInstallResult, error) {
	if strings.TrimSpace(configPath) == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".codex", "config.toml")
	}
	if strings.TrimSpace(dotfilesDir) == "" {
		dotfilesDir = detectDotfilesDir()
	}
	dotfilesDir = filepath.Clean(dotfilesDir)
	officialScript := filepath.Join(dotfilesDir, "scripts", "hg-github-official-mcp.sh")
	workflowScript := filepath.Join(dotfilesDir, "scripts", "run-dotfiles-mcp.sh")
	if !fileExists(officialScript) || !fileExists(workflowScript) {
		return CodexInstallResult{}, fmt.Errorf("dotfiles_dir does not look like a valid dotfiles checkout: expected %s and %s", officialScript, workflowScript)
	}

	block := RenderCodexBlock(dotfilesDir)
	warning := ""
	if strings.Contains(dotfilesDir, string(filepath.Separator)+".codex"+string(filepath.Separator)+"worktrees"+string(filepath.Separator)) {
		warning = "dotfiles_dir points at a Codex worktree; rerun install-codex-mcp from your stable dotfiles checkout after merging if you want a permanent global MCP path"
	}

	original := ""
	if data, err := os.ReadFile(configPath); err == nil {
		original = string(data)
	}
	updated := replaceManagedBlock(original, block)

	var parsed map[string]any
	if _, err := toml.Decode(updated, &parsed); err != nil {
		return CodexInstallResult{}, fmt.Errorf("refusing to write invalid Codex TOML: %w", err)
	}

	changed := updated != original
	status := "dry-run"
	if execute {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return CodexInstallResult{}, err
		}
		if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
			return CodexInstallResult{}, err
		}
		status = "updated"
		if !changed {
			status = "unchanged"
		}
	}

	return CodexInstallResult{
		ConfigPath:   configPath,
		DotfilesDir:  dotfilesDir,
		Status:       status,
		Changed:      changed,
		Servers:      []string{"github_stars_official", "github_stars_workflow"},
		BlockPreview: block,
		Warning:      warning,
	}, nil
}

func RenderCodexBlock(dotfilesDir string) string {
	officialScript := filepath.Join(dotfilesDir, "scripts", "hg-github-official-mcp.sh")
	dotfilesScript := filepath.Join(dotfilesDir, "scripts", "run-dotfiles-mcp.sh")

	var b strings.Builder
	b.WriteString(CodexStartMarker + "\n")
	b.WriteString("# Generated by github-starsctl / dotfiles GitHub Stars workflow helpers\n\n")
	b.WriteString("# Official GitHub MCP server scoped for star, repo, and stargazer operations.\n")
	b.WriteString("[mcp_servers.github_stars_official]\n")
	b.WriteString(fmt.Sprintf("command = %q\n", "bash"))
	b.WriteString(fmt.Sprintf("args = [%q]\n", officialScript))
	b.WriteString(fmt.Sprintf("cwd = %q\n\n", dotfilesDir))
	b.WriteString("[mcp_servers.github_stars_official.env]\n")
	b.WriteString(fmt.Sprintf("DOTFILES_DIR = %q\n", dotfilesDir))
	b.WriteString(fmt.Sprintf("GITHUB_TOOLSETS = %q\n", "default,stargazers"))
	b.WriteString(fmt.Sprintf("GITHUB_MCP_SERVER_NAME = %q\n", "github-stars-official"))
	b.WriteString(fmt.Sprintf("GITHUB_MCP_SERVER_TITLE = %q\n\n", "GitHub Stars Official"))
	b.WriteString("# Dotfiles GitHub Stars workflow surface for list management, taxonomy sync, and install helpers.\n")
	b.WriteString("[mcp_servers.github_stars_workflow]\n")
	b.WriteString(fmt.Sprintf("command = %q\n", "bash"))
	b.WriteString(fmt.Sprintf("args = [%q]\n", dotfilesScript))
	b.WriteString(fmt.Sprintf("cwd = %q\n", dotfilesDir))
	b.WriteString("enabled_tools = [\n")
	for _, tool := range []string{
		"dotfiles_tool_search",
		"dotfiles_tool_schema",
		"dotfiles_tool_catalog",
		"dotfiles_server_health",
		"dotfiles_gh_stars_list",
		"dotfiles_gh_stars_summary",
		"dotfiles_gh_star_lists_list",
		"dotfiles_gh_star_lists_ensure",
		"dotfiles_gh_star_lists_rename",
		"dotfiles_gh_star_lists_delete",
		"dotfiles_gh_stars_set",
		"dotfiles_gh_star_membership_set",
		"dotfiles_gh_stars_cleanup_candidates",
		"dotfiles_gh_stars_taxonomy_suggest",
		"dotfiles_gh_stars_taxonomy_audit",
		"dotfiles_gh_stars_taxonomy_sync",
		"dotfiles_gh_stars_bootstrap",
		"dotfiles_gh_stars_install_codex_mcp",
	} {
		b.WriteString(fmt.Sprintf("  %q,\n", tool))
	}
	b.WriteString("]\n\n")
	b.WriteString("[mcp_servers.github_stars_workflow.env]\n")
	b.WriteString(fmt.Sprintf("DOTFILES_DIR = %q\n", dotfilesDir))
	b.WriteString(fmt.Sprintf("DOTFILES_MCP_PROFILE = %q\n\n", "ops"))
	b.WriteString(CodexEndMarker + "\n")
	return b.String()
}

func detectDotfilesDir() string {
	for _, key := range []string{"DOTFILES_DIR", "HG_DOTFILES"} {
		if dir := strings.TrimSpace(os.Getenv(key)); dir != "" {
			return filepath.Clean(dir)
		}
	}

	if wd, err := os.Getwd(); err == nil {
		if root := findDotfilesRoot(wd); root != "" {
			return root
		}
	}
	if exe, err := os.Executable(); err == nil {
		if root := findDotfilesRoot(filepath.Dir(exe)); root != "" {
			return root
		}
	}
	home, _ := os.UserHomeDir()
	for _, candidate := range []string{
		filepath.Join(home, "hairglasses-studio", "dotfiles"),
		"/home/hg/hairglasses-studio/dotfiles",
	} {
		if root := findDotfilesRoot(candidate); root != "" {
			return root
		}
	}
	return filepath.Join(home, "hairglasses-studio", "dotfiles")
}

func findDotfilesRoot(start string) string {
	dir := filepath.Clean(strings.TrimSpace(start))
	if dir == "" {
		return ""
	}
	for {
		if looksLikeDotfilesRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func looksLikeDotfilesRoot(dir string) bool {
	for _, rel := range []string{
		"AGENTS.md",
		filepath.Join("scripts", "run-dotfiles-mcp.sh"),
		filepath.Join("scripts", "hg-github-stars.sh"),
	} {
		if !fileExists(filepath.Join(dir, rel)) {
			return false
		}
	}
	return true
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

func (c *Client) graphQL(ctx context.Context, query string, vars map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": vars,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dotfiles-mcp/github-stars")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github graphql returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var decoded graphQLResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	if len(decoded.Errors) > 0 {
		msgs := make([]string, 0, len(decoded.Errors))
		for _, item := range decoded.Errors {
			msgs = append(msgs, item.Message)
		}
		return fmt.Errorf("%s", strings.Join(msgs, "; "))
	}
	if out == nil || len(decoded.Data) == 0 {
		return nil
	}
	return json.Unmarshal(decoded.Data, out)
}

func (c *Client) listItemsForList(ctx context.Context, listID string, limit int) ([]StarredListItem, int, error) {
	const query = `
query($id: ID!, $after: String) {
  node(id: $id) {
    ... on UserList {
      items(first: 100, after: $after) {
        totalCount
        nodes {
          ... on Repository {
            id
            nameWithOwner
          }
        }
        pageInfo {
          hasNextPage
          endCursor
        }
      }
    }
  }
}`

	var items []StarredListItem
	var totalCount int
	var after *string
	for {
		var resp struct {
			Node *struct {
				Items struct {
					TotalCount int `json:"totalCount"`
					Nodes      []struct {
						ID            string `json:"id"`
						NameWithOwner string `json:"nameWithOwner"`
					} `json:"nodes"`
					PageInfo pageInfo `json:"pageInfo"`
				} `json:"items"`
			} `json:"node"`
		}
		if err := c.graphQL(ctx, query, map[string]any{
			"id":    listID,
			"after": after,
		}, &resp); err != nil {
			return nil, 0, err
		}
		if resp.Node == nil {
			return nil, 0, fmt.Errorf("user list not found: %s", listID)
		}

		totalCount = resp.Node.Items.TotalCount
		for _, node := range resp.Node.Items.Nodes {
			items = append(items, StarredListItem{
				ID:            node.ID,
				NameWithOwner: node.NameWithOwner,
			})
			if limit > 0 && len(items) >= limit {
				return items[:limit], totalCount, nil
			}
		}
		if !resp.Node.Items.PageInfo.HasNextPage {
			break
		}
		next := resp.Node.Items.PageInfo.EndCursor
		after = &next
	}
	return items, totalCount, nil
}

func (c *Client) repoMembershipMap(ctx context.Context) (map[string][]ListRef, error) {
	lists, err := c.ListLists(ctx, true, 0)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]ListRef)
	for _, list := range lists {
		for _, item := range list.Items {
			out[item.NameWithOwner] = append(out[item.NameWithOwner], ListRef{
				ID:   list.ID,
				Name: list.Name,
			})
		}
	}
	for repo := range out {
		sort.Slice(out[repo], func(i, j int) bool {
			return strings.ToLower(out[repo][i].Name) < strings.ToLower(out[repo][j].Name)
		})
	}
	return out, nil
}

func (c *Client) createList(ctx context.Context, spec EnsureListSpec) (UserList, error) {
	const query = `
mutation($name: String!, $description: String, $isPrivate: Boolean) {
  createUserList(input: {name: $name, description: $description, isPrivate: $isPrivate}) {
    list {
      id
      name
      description
      isPrivate
      slug
      items(first: 1) { totalCount }
    }
  }
}`

	var resp struct {
		CreateUserList struct {
			List struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				IsPrivate   bool   `json:"isPrivate"`
				Slug        string `json:"slug"`
				Items       struct {
					TotalCount int `json:"totalCount"`
				} `json:"items"`
			} `json:"list"`
		} `json:"createUserList"`
	}

	if err := c.graphQL(ctx, query, map[string]any{
		"name":        spec.Name,
		"description": spec.Description,
		"isPrivate":   spec.IsPrivate,
	}, &resp); err != nil {
		return UserList{}, err
	}
	return UserList{
		ID:          resp.CreateUserList.List.ID,
		Name:        resp.CreateUserList.List.Name,
		Description: resp.CreateUserList.List.Description,
		IsPrivate:   resp.CreateUserList.List.IsPrivate,
		Slug:        resp.CreateUserList.List.Slug,
		TotalCount:  resp.CreateUserList.List.Items.TotalCount,
	}, nil
}

func (c *Client) updateList(ctx context.Context, listID string, spec EnsureListSpec) (UserList, error) {
	const query = `
mutation($listId: ID!, $name: String, $description: String, $isPrivate: Boolean) {
  updateUserList(input: {listId: $listId, name: $name, description: $description, isPrivate: $isPrivate}) {
    list {
      id
      name
      description
      isPrivate
      slug
      items(first: 1) { totalCount }
    }
  }
}`

	var resp struct {
		UpdateUserList struct {
			List struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				IsPrivate   bool   `json:"isPrivate"`
				Slug        string `json:"slug"`
				Items       struct {
					TotalCount int `json:"totalCount"`
				} `json:"items"`
			} `json:"list"`
		} `json:"updateUserList"`
	}

	if err := c.graphQL(ctx, query, map[string]any{
		"listId":      listID,
		"name":        spec.Name,
		"description": spec.Description,
		"isPrivate":   spec.IsPrivate,
	}, &resp); err != nil {
		return UserList{}, err
	}
	return UserList{
		ID:          resp.UpdateUserList.List.ID,
		Name:        resp.UpdateUserList.List.Name,
		Description: resp.UpdateUserList.List.Description,
		IsPrivate:   resp.UpdateUserList.List.IsPrivate,
		Slug:        resp.UpdateUserList.List.Slug,
		TotalCount:  resp.UpdateUserList.List.Items.TotalCount,
	}, nil
}

func (c *Client) deleteList(ctx context.Context, listID string) error {
	const query = `
mutation($listId: ID!) {
  deleteUserList(input: {listId: $listId}) {
    clientMutationId
  }
}`
	return c.graphQL(ctx, query, map[string]any{"listId": listID}, &struct{}{})
}

func (c *Client) addStar(ctx context.Context, repoID string) error {
	const query = `
mutation($starrableId: ID!) {
  addStar(input: {starrableId: $starrableId}) {
    starrable {
      ... on Repository {
        id
      }
    }
  }
}`
	return c.graphQL(ctx, query, map[string]any{"starrableId": repoID}, &struct{}{})
}

func (c *Client) removeStar(ctx context.Context, repoID string) error {
	const query = `
mutation($starrableId: ID!) {
  removeStar(input: {starrableId: $starrableId}) {
    starrable {
      ... on Repository {
        id
      }
    }
  }
}`
	return c.graphQL(ctx, query, map[string]any{"starrableId": repoID}, &struct{}{})
}

func (c *Client) updateItemLists(ctx context.Context, itemID string, listIDs []string) error {
	const query = `
mutation($itemId: ID!, $listIds: [ID!]!) {
  updateUserListsForItem(input: {itemId: $itemId, listIds: $listIds}) {
    user { login }
  }
}`
	return c.graphQL(ctx, query, map[string]any{
		"itemId":  itemID,
		"listIds": listIDs,
	}, &struct{}{})
}

func filterStars(repos []StarredRepository, filter StarsFilter) []StarredRepository {
	if filter.Query == "" && filter.Language == "" && filter.Topic == "" && len(filter.ListNames) == 0 {
		return repos
	}

	listFilter := make(map[string]bool)
	for _, item := range filter.ListNames {
		listFilter[strings.ToLower(strings.TrimSpace(item))] = true
	}

	out := make([]StarredRepository, 0, len(repos))
	for _, repo := range repos {
		if filter.Query != "" {
			haystack := strings.ToLower(repo.NameWithOwner + " " + repo.Description + " " + strings.Join(repo.Topics, " "))
			if !strings.Contains(haystack, strings.ToLower(filter.Query)) {
				continue
			}
		}
		if filter.Language != "" && !strings.EqualFold(repo.PrimaryLanguage, filter.Language) {
			continue
		}
		if filter.Topic != "" && !hasStringFold(repo.Topics, filter.Topic) {
			continue
		}
		if len(listFilter) > 0 {
			matched := false
			for _, list := range repo.Lists {
				if listFilter[strings.ToLower(list.Name)] {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, repo)
	}
	return out
}

func sortStars(repos []StarredRepository, sortBy string) {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "", "starred":
		// Keep GraphQL order.
	case "updated":
		sort.SliceStable(repos, func(i, j int) bool { return repos[i].UpdatedAt > repos[j].UpdatedAt })
	case "stars":
		sort.SliceStable(repos, func(i, j int) bool {
			if repos[i].StargazerCount == repos[j].StargazerCount {
				return strings.ToLower(repos[i].NameWithOwner) < strings.ToLower(repos[j].NameWithOwner)
			}
			return repos[i].StargazerCount > repos[j].StargazerCount
		})
	case "name":
		sort.SliceStable(repos, func(i, j int) bool {
			return strings.ToLower(repos[i].NameWithOwner) < strings.ToLower(repos[j].NameWithOwner)
		})
	}
}

func topBuckets(counts map[string]int, limit int) []CountBucket {
	out := make([]CountBucket, 0, len(counts))
	for name, count := range counts {
		out = append(out, CountBucket{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].Count > out[j].Count
	})
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}

func listNames(lists []ListRef) []string {
	out := make([]string, 0, len(lists))
	for _, list := range lists {
		if strings.TrimSpace(list.Name) != "" {
			out = append(out, list.Name)
		}
	}
	out = dedupeStrings(out)
	sort.Strings(out)
	return out
}

func hasListWithPrefix(lists []ListRef, prefix string) bool {
	for _, list := range lists {
		if strings.HasPrefix(list.Name, prefix) {
			return true
		}
	}
	return false
}

func filterListsByPrefix(lists []string, prefix string) []string {
	if prefix == "" {
		out := dedupeStrings(lists)
		sort.Strings(out)
		return out
	}
	out := make([]string, 0, len(lists))
	for _, list := range lists {
		if strings.HasPrefix(list, prefix) {
			out = append(out, list)
		}
	}
	out = dedupeStrings(out)
	sort.Strings(out)
	return out
}

func sortRepoAudit(items []RepoAuditGap) {
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Repo) < strings.ToLower(items[j].Repo)
	})
}

func calculateDesiredLists(current, target []string, operation MembershipOperation, managedPrefix string) ([]string, error) {
	current = dedupeStrings(current)
	target = dedupeStrings(target)

	switch operation {
	case "", OperationMerge:
		return dedupeStrings(append(current, target...)), nil
	case OperationReplace:
		return target, nil
	case OperationRemove:
		var out []string
		for _, existing := range current {
			if !hasStringFold(target, existing) {
				out = append(out, existing)
			}
		}
		return dedupeStrings(out), nil
	case OperationReplaceManaged:
		if strings.TrimSpace(managedPrefix) == "" {
			return nil, fmt.Errorf("managed_list_prefix is required when operation=replace_managed")
		}
		var out []string
		for _, existing := range current {
			if !strings.HasPrefix(existing, managedPrefix) {
				out = append(out, existing)
			}
		}
		out = append(out, target...)
		return dedupeStrings(out), nil
	default:
		return nil, fmt.Errorf("unsupported list operation: %s", operation)
	}
}

func replaceManagedBlock(content, block string) string {
	if strings.Contains(content, CodexStartMarker) && strings.Contains(content, CodexEndMarker) {
		start := strings.Index(content, CodexStartMarker)
		end := strings.Index(content, CodexEndMarker)
		if start >= 0 && end >= start {
			end += len(CodexEndMarker)
			if end < len(content) && content[end] == '\n' {
				end++
			}
			return strings.TrimRight(content[:start], "\n") + "\n\n" + block + strings.TrimLeft(content[end:], "\n")
		}
	}

	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return block
	}
	return trimmed + "\n\n" + block
}

func listStateString(description string, isPrivate bool) string {
	visibility := "public"
	if isPrivate {
		visibility = "private"
	}
	return fmt.Sprintf("%s (%s)", strings.TrimSpace(description), visibility)
}

func boolAction(trueValue, falseValue string, value bool) string {
	if value {
		return trueValue
	}
	return falseValue
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(repo), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repo must be owner/name: %s", repo)
	}
	return parts[0], parts[1], nil
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func stringSlicesEqual(a, b []string) bool {
	a = dedupeStrings(a)
	b = dedupeStrings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

func hasStringFold(items []string, needle string) bool {
	for _, item := range items {
		if strings.EqualFold(item, needle) {
			return true
		}
	}
	return false
}
