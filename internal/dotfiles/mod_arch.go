package dotfiles

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

var (
	archHTTPClient            = &http.Client{Timeout: 20 * time.Second}
	archWikiAPIURL            = "https://wiki.archlinux.org/api.php"
	archPackagesSearchURL     = "https://archlinux.org/packages/search/json/"
	archAURSearchURL          = "https://aur.archlinux.org/rpc/v5/search/"
	archAURPKGBUILDURLPattern = "https://aur.archlinux.org/cgit/aur.git/plain/PKGBUILD?h=%s"
	archNewsFeedURL           = "https://archlinux.org/feeds/news/"
	archMirrorStatusURL       = "https://archlinux.org/mirrors/status/json/"
	archHTMLTagRe             = regexp.MustCompile(`<[^>]+>`)
	archWhitespaceRe          = regexp.MustCompile(`\s+`)
	archPkgbuildSignals       = []archPKGBuildSignal{
		{Pattern: regexp.MustCompile(`(?i)\b(?:curl|wget)\b.*\|\s*(?:bash|sh)\b`), Severity: "critical", Signal: "pipes remote content directly into a shell"},
		{Pattern: regexp.MustCompile(`(?i)\bbash\s*<\(`), Severity: "critical", Signal: "executes process substitution from a shell"},
		{Pattern: regexp.MustCompile(`(?i)\brm\s+-rf\s+/(?:\s|$)`), Severity: "critical", Signal: "contains a destructive root filesystem removal pattern"},
		{Pattern: regexp.MustCompile(`(?i)\b(?:sudo|doas)\b`), Severity: "high", Signal: "invokes privilege escalation inside the PKGBUILD"},
		{Pattern: regexp.MustCompile(`(?i)\b(?:systemctl|service)\b`), Severity: "high", Signal: "touches system services during package build or install"},
		{Pattern: regexp.MustCompile(`(?i)\b(?:useradd|groupadd|passwd|chsh)\b`), Severity: "high", Signal: "modifies local users or groups"},
		{Pattern: regexp.MustCompile(`(?i)\beval\b`), Severity: "high", Signal: "uses eval, which raises command-injection risk"},
		{Pattern: regexp.MustCompile(`(?i)^install=`), Severity: "medium", Signal: "uses a package install hook script"},
		{Pattern: regexp.MustCompile(`(?i)\bgit\s+clone\b`), Severity: "medium", Signal: "pulls live VCS content during the build path"},
		{Pattern: regexp.MustCompile(`(?i)\b(?:curl|wget)\b`), Severity: "medium", Signal: "downloads remote content during package preparation"},
		{Pattern: regexp.MustCompile(`(?i)\bchmod\s+777\b`), Severity: "medium", Signal: "uses world-writable permissions"},
	}
)

type ArchSearchInput struct {
	Query string `json:"query" jsonschema:"required,description=Search query"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Maximum results to return. Defaults to 10."`
}

type ArchPackageInfoInput struct {
	Name string `json:"name" jsonschema:"required,description=Exact package name"`
}

type ArchPkgbuildAuditInput struct {
	Package  string `json:"package,omitempty" jsonschema:"description=AUR package name to fetch and audit"`
	PKGBUILD string `json:"pkgbuild,omitempty" jsonschema:"description=Raw PKGBUILD content to audit directly"`
}

type ArchNewsInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"description=Maximum number of news items to return. Defaults to 10."`
}

type ArchCriticalNewsInput struct {
	Limit    int      `json:"limit,omitempty" jsonschema:"description=Maximum number of news items to return. Defaults to 10."`
	Packages []string `json:"packages,omitempty" jsonschema:"description=Optional package names to match against critical news items"`
}

type ArchMirrorStatusInput struct {
	Limit       int    `json:"limit,omitempty" jsonschema:"description=Maximum number of mirrors to return. Defaults to 10."`
	CountryCode string `json:"country_code,omitempty" jsonschema:"description=Optional country code filter such as US or DE"`
	Protocol    string `json:"protocol,omitempty" jsonschema:"description=Optional protocol filter such as https rsync or http"`
}

type ArchPacmanLogInput struct {
	Lines  int    `json:"lines,omitempty" jsonschema:"description=Number of log lines to return. Defaults to 50."`
	Filter string `json:"filter,omitempty" jsonschema:"description=Optional substring filter applied to log lines"`
}

type ArchFileOwnerInput struct {
	Path string `json:"path" jsonschema:"required,description=Filesystem path to inspect with pacman -Qo"`
}

type archWikiSearchOutput struct {
	Query   string                 `json:"query"`
	Results []archWikiSearchResult `json:"results"`
	Total   int                    `json:"total"`
}

type archWikiSearchResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet,omitempty"`
	PageID  int    `json:"page_id,omitempty"`
	URL     string `json:"url"`
}

type archWikiPageOutput struct {
	Title    string            `json:"title"`
	URL      string            `json:"url"`
	Sections []archWikiSection `json:"sections,omitempty"`
	Content  string            `json:"content"`
}

type archWikiSection struct {
	Index  string `json:"index,omitempty"`
	Line   string `json:"line"`
	Number string `json:"number,omitempty"`
}

type archPackageSearchOutput struct {
	Query   string               `json:"query"`
	Results []archPackageSummary `json:"results"`
	Total   int                  `json:"total"`
}

type archPackageInfoOutput struct {
	Name    string              `json:"name"`
	Found   bool                `json:"found"`
	Package *archPackageSummary `json:"package,omitempty"`
	Message string              `json:"message,omitempty"`
}

type archPackageSummary struct {
	Name        string   `json:"name"`
	PackageBase string   `json:"package_base,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Arch        string   `json:"arch,omitempty"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	URL         string   `json:"url,omitempty"`
	Maintainers []string `json:"maintainers,omitempty"`
	Licenses    []string `json:"licenses,omitempty"`
	Depends     []string `json:"depends,omitempty"`
	OptDepends  []string `json:"optdepends,omitempty"`
	LastUpdate  string   `json:"last_update,omitempty"`
}

type archAURSearchOutput struct {
	Query   string                 `json:"query"`
	Results []archAURPackageResult `json:"results"`
	Total   int                    `json:"total"`
}

type archAURPackageResult struct {
	Name        string   `json:"name"`
	PackageBase string   `json:"package_base,omitempty"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	URL         string   `json:"url,omitempty"`
	URLPath     string   `json:"url_path,omitempty"`
	Maintainer  string   `json:"maintainer,omitempty"`
	Licenses    []string `json:"licenses,omitempty"`
	Depends     []string `json:"depends,omitempty"`
	OptDepends  []string `json:"optdepends,omitempty"`
	Provides    []string `json:"provides,omitempty"`
	Conflicts   []string `json:"conflicts,omitempty"`
	Popularity  float64  `json:"popularity,omitempty"`
	Votes       int      `json:"votes,omitempty"`
	OutOfDate   bool     `json:"out_of_date"`
}

type archPKGBuildAuditOutput struct {
	Package   string                `json:"package,omitempty"`
	Source    string                `json:"source"`
	RiskLevel string                `json:"risk_level"`
	Findings  []archPKGBuildFinding `json:"findings,omitempty"`
	PKGBUILD  string                `json:"pkgbuild,omitempty"`
}

type archPKGBuildFinding struct {
	Severity string `json:"severity"`
	Signal   string `json:"signal"`
	Line     int    `json:"line"`
	Snippet  string `json:"snippet"`
}

type archNewsOutput struct {
	Items []archNewsItem `json:"items"`
	Total int            `json:"total"`
}

type archNewsItem struct {
	Title     string   `json:"title"`
	Link      string   `json:"link"`
	Published string   `json:"published"`
	Author    string   `json:"author,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	Reasons   []string `json:"reasons,omitempty"`
}

type archUpdatesDryRunOutput struct {
	Official      []string `json:"official,omitempty"`
	AUR           []string `json:"aur,omitempty"`
	OfficialCount int      `json:"official_count"`
	AURCount      int      `json:"aur_count"`
	NotAvailable  []string `json:"not_available,omitempty"`
}

type archMirrorStatusOutput struct {
	LastCheck string              `json:"last_check,omitempty"`
	Cutoff    int                 `json:"cutoff,omitempty"`
	Mirrors   []archMirrorSummary `json:"mirrors"`
}

type archMirrorSummary struct {
	URL           string  `json:"url"`
	Protocol      string  `json:"protocol"`
	Country       string  `json:"country,omitempty"`
	CountryCode   string  `json:"country_code,omitempty"`
	CompletionPct float64 `json:"completion_pct,omitempty"`
	Delay         int     `json:"delay,omitempty"`
	Score         float64 `json:"score,omitempty"`
	LastSync      string  `json:"last_sync,omitempty"`
	Active        bool    `json:"active"`
}

type archPacmanLogOutput struct {
	Lines []string `json:"lines"`
	Total int      `json:"total"`
}

type archOrphansOutput struct {
	Packages     []string `json:"packages,omitempty"`
	Count        int      `json:"count"`
	NotAvailable string   `json:"not_available,omitempty"`
}

type archFileOwnerOutput struct {
	Path         string `json:"path"`
	Installed    bool   `json:"installed"`
	Owner        string `json:"owner,omitempty"`
	NotAvailable string `json:"not_available,omitempty"`
	Message      string `json:"message,omitempty"`
}

type archPKGBuildSignal struct {
	Pattern  *regexp.Regexp
	Severity string
	Signal   string
}

type archWikiSearchResponse struct {
	Query struct {
		Search []struct {
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
			PageID  int    `json:"pageid"`
		} `json:"search"`
	} `json:"query"`
}

type archWikiPageResponse struct {
	Parse struct {
		Title    string `json:"title"`
		Wikitext struct {
			Value string `json:"*"`
		} `json:"wikitext"`
		Text struct {
			Value string `json:"*"`
		} `json:"text"`
		Sections []struct {
			Index  string `json:"index"`
			Line   string `json:"line"`
			Number string `json:"number"`
		} `json:"sections"`
	} `json:"parse"`
}

type archPackagesSearchResponse struct {
	Results []struct {
		Name        string   `json:"pkgname"`
		PackageBase string   `json:"pkgbase"`
		Repo        string   `json:"repo"`
		Arch        string   `json:"arch"`
		PkgVer      string   `json:"pkgver"`
		PkgRel      string   `json:"pkgrel"`
		Description string   `json:"pkgdesc"`
		URL         string   `json:"url"`
		Maintainers []string `json:"maintainers"`
		Licenses    []string `json:"licenses"`
		Depends     []string `json:"depends"`
		OptDepends  []string `json:"optdepends"`
		LastUpdate  string   `json:"last_update"`
	} `json:"results"`
}

type archAURResponse struct {
	Results []struct {
		Name        string   `json:"Name"`
		PackageBase string   `json:"PackageBase"`
		Version     string   `json:"Version"`
		Description string   `json:"Description"`
		URL         string   `json:"URL"`
		URLPath     string   `json:"URLPath"`
		Maintainer  string   `json:"Maintainer"`
		License     []string `json:"License"`
		Depends     []string `json:"Depends"`
		OptDepends  []string `json:"OptDepends"`
		Provides    []string `json:"Provides"`
		Conflicts   []string `json:"Conflicts"`
		Popularity  float64  `json:"Popularity"`
		NumVotes    int      `json:"NumVotes"`
		OutOfDate   *int64   `json:"OutOfDate"`
	} `json:"results"`
}

type archNewsRSS struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			Author      string `xml:"creator"`
			PubDate     string `xml:"pubDate"`
		} `xml:"channel>item"`
	} `xml:"channel"`
}

type archMirrorStatusResponse struct {
	LastCheck string `json:"last_check"`
	Cutoff    int    `json:"cutoff"`
	URLs      []struct {
		URL           string  `json:"url"`
		Protocol      string  `json:"protocol"`
		Country       string  `json:"country"`
		CountryCode   string  `json:"country_code"`
		CompletionPct float64 `json:"completion_pct"`
		Delay         int     `json:"delay"`
		Score         float64 `json:"score"`
		LastSync      string  `json:"last_sync"`
		Active        bool    `json:"active"`
	} `json:"urls"`
}

type ArchModule struct{}

func (m *ArchModule) Name() string { return "arch" }
func (m *ArchModule) Description() string {
	return "Arch Linux research and package management read-first tools"
}

func (m *ArchModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[ArchSearchInput, archWikiSearchOutput](
			"archwiki_search",
			"Search the ArchWiki and return matching pages with cleaned snippets.",
			func(ctx context.Context, input ArchSearchInput) (archWikiSearchOutput, error) {
				return archWikiSearch(ctx, input.Query, archNormalizeLimit(input.Limit, 10, 50))
			},
		),
		handler.TypedHandler[ArchPackageInfoInput, archWikiPageOutput](
			"archwiki_page",
			"Fetch an ArchWiki page with sections and cleaned page content.",
			func(ctx context.Context, input ArchPackageInfoInput) (archWikiPageOutput, error) {
				if strings.TrimSpace(input.Name) == "" {
					return archWikiPageOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
				}
				return archWikiPage(ctx, input.Name)
			},
		),
		handler.TypedHandler[ArchSearchInput, archPackageSearchOutput](
			"arch_package_search",
			"Search official Arch Linux packages by keyword.",
			func(ctx context.Context, input ArchSearchInput) (archPackageSearchOutput, error) {
				return archPackageSearch(ctx, input.Query, archNormalizeLimit(input.Limit, 10, 100))
			},
		),
		handler.TypedHandler[ArchPackageInfoInput, archPackageInfoOutput](
			"arch_package_info",
			"Fetch official Arch Linux package metadata for an exact package name.",
			func(ctx context.Context, input ArchPackageInfoInput) (archPackageInfoOutput, error) {
				return archPackageInfo(ctx, input.Name)
			},
		),
		handler.TypedHandler[ArchSearchInput, archAURSearchOutput](
			"arch_aur_search",
			"Search AUR packages and return key metadata for the best matches.",
			func(ctx context.Context, input ArchSearchInput) (archAURSearchOutput, error) {
				return archAURSearch(ctx, input.Query, archNormalizeLimit(input.Limit, 10, 100))
			},
		),
		handler.TypedHandler[ArchPkgbuildAuditInput, archPKGBuildAuditOutput](
			"arch_pkgbuild_audit",
			"Audit an AUR PKGBUILD or raw PKGBUILD content for risky patterns.",
			archAuditPKGBUILD,
		),
		handler.TypedHandler[ArchNewsInput, archNewsOutput](
			"arch_news_latest",
			"Return the latest Arch Linux news items from the official RSS feed.",
			func(ctx context.Context, input ArchNewsInput) (archNewsOutput, error) {
				items, err := archFetchNews(ctx)
				if err != nil {
					return archNewsOutput{}, err
				}
				limit := archNormalizeLimit(input.Limit, 10, 50)
				if len(items) > limit {
					items = items[:limit]
				}
				return archNewsOutput{Items: items, Total: len(items)}, nil
			},
		),
		handler.TypedHandler[ArchCriticalNewsInput, archNewsOutput](
			"arch_news_critical",
			"Return the latest Arch news items that look like manual-intervention or high-risk upgrade notices.",
			func(ctx context.Context, input ArchCriticalNewsInput) (archNewsOutput, error) {
				items, err := archFetchNews(ctx)
				if err != nil {
					return archNewsOutput{}, err
				}
				filtered := make([]archNewsItem, 0)
				for _, item := range items {
					if critical, reasons := archNewsCriticalReasons(item, input.Packages); critical {
						item.Reasons = reasons
						filtered = append(filtered, item)
					}
				}
				limit := archNormalizeLimit(input.Limit, 10, 50)
				if len(filtered) > limit {
					filtered = filtered[:limit]
				}
				return archNewsOutput{Items: filtered, Total: len(filtered)}, nil
			},
		),
		handler.TypedHandler[EmptyInput, archUpdatesDryRunOutput](
			"arch_updates_dry_run",
			"Show official and AUR updates without installing anything.",
			func(_ context.Context, _ EmptyInput) (archUpdatesDryRunOutput, error) {
				return archUpdatesDryRun()
			},
		),
		handler.TypedHandler[ArchMirrorStatusInput, archMirrorStatusOutput](
			"arch_mirror_status",
			"Return the healthiest official Arch Linux mirrors from the mirror-status JSON feed.",
			archMirrorStatus,
		),
		handler.TypedHandler[ArchPacmanLogInput, archPacmanLogOutput](
			"arch_pacman_log",
			"Read recent pacman log lines, optionally filtered by a substring.",
			func(_ context.Context, input ArchPacmanLogInput) (archPacmanLogOutput, error) {
				return archPacmanLog(input)
			},
		),
		handler.TypedHandler[EmptyInput, archOrphansOutput](
			"arch_orphans",
			"List orphaned packages from pacman -Qtdq.",
			func(_ context.Context, _ EmptyInput) (archOrphansOutput, error) {
				return archOrphans()
			},
		),
		handler.TypedHandler[ArchFileOwnerInput, archFileOwnerOutput](
			"arch_file_owner",
			"Find the pacman package that owns a given file path.",
			func(_ context.Context, input ArchFileOwnerInput) (archFileOwnerOutput, error) {
				return archFileOwner(input.Path)
			},
		),
	}
}

func archNormalizeLimit(limit, def, max int) int {
	if limit <= 0 {
		limit = def
	}
	if limit > max {
		limit = max
	}
	return limit
}

func archWikiSearch(ctx context.Context, query string, limit int) (archWikiSearchOutput, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return archWikiSearchOutput{}, fmt.Errorf("[%s] query is required", handler.ErrInvalidParam)
	}
	values := url.Values{}
	values.Set("action", "query")
	values.Set("list", "search")
	values.Set("srsearch", query)
	values.Set("srlimit", strconv.Itoa(limit))
	values.Set("format", "json")
	values.Set("utf8", "1")
	var response archWikiSearchResponse
	if err := archFetchJSON(ctx, archWikiAPIURL+"?"+values.Encode(), &response); err != nil {
		return archWikiSearchOutput{}, err
	}
	results := make([]archWikiSearchResult, 0, len(response.Query.Search))
	for _, item := range response.Query.Search {
		results = append(results, archWikiSearchResult{
			Title:   item.Title,
			Snippet: archCleanHTML(item.Snippet),
			PageID:  item.PageID,
			URL:     archWikiPageURL(item.Title),
		})
	}
	return archWikiSearchOutput{Query: query, Results: results, Total: len(results)}, nil
}

func archWikiPage(ctx context.Context, title string) (archWikiPageOutput, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return archWikiPageOutput{}, fmt.Errorf("[%s] title is required", handler.ErrInvalidParam)
	}
	values := url.Values{}
	values.Set("action", "parse")
	values.Set("page", title)
	values.Set("prop", "text|wikitext|sections")
	values.Set("redirects", "1")
	values.Set("format", "json")
	var response archWikiPageResponse
	if err := archFetchJSON(ctx, archWikiAPIURL+"?"+values.Encode(), &response); err != nil {
		return archWikiPageOutput{}, err
	}
	sections := make([]archWikiSection, 0, len(response.Parse.Sections))
	for _, section := range response.Parse.Sections {
		sections = append(sections, archWikiSection{Index: section.Index, Line: section.Line, Number: section.Number})
	}
	content := strings.TrimSpace(response.Parse.Wikitext.Value)
	if content == "" {
		content = archCleanHTML(response.Parse.Text.Value)
	}
	return archWikiPageOutput{
		Title:    response.Parse.Title,
		URL:      archWikiPageURL(response.Parse.Title),
		Sections: sections,
		Content:  content,
	}, nil
}

func archPackageSearch(ctx context.Context, query string, limit int) (archPackageSearchOutput, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return archPackageSearchOutput{}, fmt.Errorf("[%s] query is required", handler.ErrInvalidParam)
	}
	values := url.Values{}
	values.Set("q", query)
	var response archPackagesSearchResponse
	if err := archFetchJSON(ctx, archPackagesSearchURL+"?"+values.Encode(), &response); err != nil {
		return archPackageSearchOutput{}, err
	}
	results := make([]archPackageSummary, 0, len(response.Results))
	for _, item := range response.Results {
		results = append(results, archPackageSummary{
			Name:        item.Name,
			PackageBase: item.PackageBase,
			Repo:        item.Repo,
			Arch:        item.Arch,
			Version:     archJoinVersion(item.PkgVer, item.PkgRel),
			Description: item.Description,
			URL:         item.URL,
			Maintainers: item.Maintainers,
			Licenses:    item.Licenses,
			Depends:     item.Depends,
			OptDepends:  item.OptDepends,
			LastUpdate:  item.LastUpdate,
		})
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return archPackageSearchOutput{Query: query, Results: results, Total: len(results)}, nil
}

func archPackageInfo(ctx context.Context, name string) (archPackageInfoOutput, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return archPackageInfoOutput{}, fmt.Errorf("[%s] name is required", handler.ErrInvalidParam)
	}
	search, err := archPackageSearch(ctx, name, 100)
	if err != nil {
		return archPackageInfoOutput{}, err
	}
	for _, item := range search.Results {
		if item.Name == name {
			copy := item
			return archPackageInfoOutput{Name: name, Found: true, Package: &copy}, nil
		}
	}
	if len(search.Results) > 0 {
		copy := search.Results[0]
		return archPackageInfoOutput{
			Name:    name,
			Found:   false,
			Package: &copy,
			Message: "exact package not found; returning the closest search result",
		}, nil
	}
	return archPackageInfoOutput{Name: name, Found: false, Message: "package not found"}, nil
}

func archAURSearch(ctx context.Context, query string, limit int) (archAURSearchOutput, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return archAURSearchOutput{}, fmt.Errorf("[%s] query is required", handler.ErrInvalidParam)
	}
	var response archAURResponse
	if err := archFetchJSON(ctx, archAURSearchURL+"/"+url.QueryEscape(query), &response); err != nil {
		return archAURSearchOutput{}, err
	}
	results := make([]archAURPackageResult, 0, len(response.Results))
	for _, item := range response.Results {
		results = append(results, archAURPackageResult{
			Name:        item.Name,
			PackageBase: item.PackageBase,
			Version:     item.Version,
			Description: item.Description,
			URL:         item.URL,
			URLPath:     item.URLPath,
			Maintainer:  item.Maintainer,
			Licenses:    item.License,
			Depends:     item.Depends,
			OptDepends:  item.OptDepends,
			Provides:    item.Provides,
			Conflicts:   item.Conflicts,
			Popularity:  item.Popularity,
			Votes:       item.NumVotes,
			OutOfDate:   item.OutOfDate != nil,
		})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Popularity > results[j].Popularity })
	if len(results) > limit {
		results = results[:limit]
	}
	return archAURSearchOutput{Query: query, Results: results, Total: len(results)}, nil
}

func archAuditPKGBUILD(ctx context.Context, input ArchPkgbuildAuditInput) (archPKGBuildAuditOutput, error) {
	source := "inline"
	pkgbuild := strings.TrimSpace(input.PKGBUILD)
	pkg := strings.TrimSpace(input.Package)
	if pkgbuild == "" {
		if pkg == "" {
			return archPKGBuildAuditOutput{}, fmt.Errorf("[%s] package or pkgbuild is required", handler.ErrInvalidParam)
		}
		text, err := archFetchText(ctx, fmt.Sprintf(archAURPKGBUILDURLPattern, url.PathEscape(pkg)))
		if err != nil {
			return archPKGBuildAuditOutput{}, err
		}
		pkgbuild = text
		source = "aur"
	}
	findings := make([]archPKGBuildFinding, 0)
	for lineNo, line := range strings.Split(pkgbuild, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, signal := range archPkgbuildSignals {
			if signal.Pattern.MatchString(trimmed) {
				findings = append(findings, archPKGBuildFinding{
					Severity: signal.Severity,
					Signal:   signal.Signal,
					Line:     lineNo + 1,
					Snippet:  trimmed,
				})
			}
		}
	}
	risk := "low"
	for _, finding := range findings {
		switch finding.Severity {
		case "critical":
			risk = "critical"
		case "high":
			if risk != "critical" {
				risk = "high"
			}
		case "medium":
			if risk == "low" {
				risk = "medium"
			}
		}
	}
	return archPKGBuildAuditOutput{
		Package:   pkg,
		Source:    source,
		RiskLevel: risk,
		Findings:  findings,
		PKGBUILD:  pkgbuild,
	}, nil
}

func archFetchNews(ctx context.Context) ([]archNewsItem, error) {
	body, err := archFetchText(ctx, archNewsFeedURL)
	if err != nil {
		return nil, err
	}
	var rss archNewsRSS
	if err := xml.Unmarshal([]byte(body), &rss); err != nil {
		return nil, fmt.Errorf("parse Arch news RSS: %w", err)
	}
	items := make([]archNewsItem, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		items = append(items, archNewsItem{
			Title:     strings.TrimSpace(item.Title),
			Link:      strings.TrimSpace(item.Link),
			Published: strings.TrimSpace(item.PubDate),
			Author:    strings.TrimSpace(item.Author),
			Summary:   archCleanHTML(item.Description),
		})
	}
	return items, nil
}

func archNewsCriticalReasons(item archNewsItem, packages []string) (bool, []string) {
	text := strings.ToLower(strings.Join([]string{item.Title, item.Summary}, " "))
	reasons := make([]string, 0)
	phrases := []string{
		"manual intervention",
		"requires manual intervention",
		"may require manual intervention",
		"requires intervention",
		"broken graphical environment",
		"failed to commit transaction",
		"overwrite",
		"drops",
		"breaking changes",
		"incompatible",
	}
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			reasons = append(reasons, phrase)
		}
	}
	for _, pkg := range packages {
		pkg = strings.TrimSpace(pkg)
		if pkg != "" && strings.Contains(text, strings.ToLower(pkg)) {
			reasons = append(reasons, "mentions package "+pkg)
		}
	}
	if len(reasons) == 0 {
		return false, nil
	}
	return true, reasons
}

func archUpdatesDryRun() (archUpdatesDryRunOutput, error) {
	out := archUpdatesDryRunOutput{}
	if official, err := archRunOptionalCommand("checkupdates"); err == nil {
		out.Official = archSplitNonEmptyLines(official)
		out.OfficialCount = len(out.Official)
	} else {
		out.NotAvailable = append(out.NotAvailable, err.Error())
	}
	var aurRaw string
	var aurErr error
	if _, err := exec.LookPath("yay"); err == nil {
		aurRaw, aurErr = archRunOptionalCommand("yay", "-Qua")
	} else if _, err := exec.LookPath("paru"); err == nil {
		aurRaw, aurErr = archRunOptionalCommand("paru", "-Qua")
	} else {
		aurErr = fmt.Errorf("yay or paru not found on PATH")
	}
	if aurErr == nil {
		out.AUR = archSplitNonEmptyLines(aurRaw)
		out.AURCount = len(out.AUR)
	} else {
		out.NotAvailable = append(out.NotAvailable, aurErr.Error())
	}
	return out, nil
}

func archMirrorStatus(ctx context.Context, input ArchMirrorStatusInput) (archMirrorStatusOutput, error) {
	var response archMirrorStatusResponse
	if err := archFetchJSON(ctx, archMirrorStatusURL, &response); err != nil {
		return archMirrorStatusOutput{}, err
	}
	limit := archNormalizeLimit(input.Limit, 10, 100)
	countryCode := strings.ToUpper(strings.TrimSpace(input.CountryCode))
	protocol := strings.ToLower(strings.TrimSpace(input.Protocol))
	mirrors := make([]archMirrorSummary, 0, len(response.URLs))
	for _, item := range response.URLs {
		if !item.Active {
			continue
		}
		if countryCode != "" && !strings.EqualFold(item.CountryCode, countryCode) {
			continue
		}
		if protocol != "" && !strings.EqualFold(item.Protocol, protocol) {
			continue
		}
		mirrors = append(mirrors, archMirrorSummary{
			URL:           item.URL,
			Protocol:      item.Protocol,
			Country:       item.Country,
			CountryCode:   item.CountryCode,
			CompletionPct: item.CompletionPct,
			Delay:         item.Delay,
			Score:         item.Score,
			LastSync:      item.LastSync,
			Active:        item.Active,
		})
	}
	sort.Slice(mirrors, func(i, j int) bool {
		if mirrors[i].Score == mirrors[j].Score {
			return mirrors[i].Delay < mirrors[j].Delay
		}
		return mirrors[i].Score < mirrors[j].Score
	})
	if len(mirrors) > limit {
		mirrors = mirrors[:limit]
	}
	return archMirrorStatusOutput{LastCheck: response.LastCheck, Cutoff: response.Cutoff, Mirrors: mirrors}, nil
}

func archPacmanLog(input ArchPacmanLogInput) (archPacmanLogOutput, error) {
	logPath := "/var/log/pacman.log"
	data, err := os.ReadFile(logPath)
	if err != nil {
		return archPacmanLogOutput{}, fmt.Errorf("read %s: %w", logPath, err)
	}
	lines := archSplitNonEmptyLines(string(data))
	filter := strings.ToLower(strings.TrimSpace(input.Filter))
	if filter != "" {
		filtered := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), filter) {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}
	limit := input.Lines
	if limit <= 0 {
		limit = 50
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return archPacmanLogOutput{Lines: lines, Total: len(lines)}, nil
}

func archOrphans() (archOrphansOutput, error) {
	raw, err := archRunOptionalCommand("pacman", "-Qtdq")
	if err != nil {
		return archOrphansOutput{NotAvailable: err.Error()}, nil
	}
	packages := archSplitNonEmptyLines(raw)
	return archOrphansOutput{Packages: packages, Count: len(packages)}, nil
}

func archFileOwner(path string) (archFileOwnerOutput, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return archFileOwnerOutput{}, fmt.Errorf("[%s] path is required", handler.ErrInvalidParam)
	}
	raw, err := archRunOptionalCommand("pacman", "-Qo", path)
	if err != nil {
		return archFileOwnerOutput{Path: path, Installed: false, Message: err.Error()}, nil
	}
	return archFileOwnerOutput{Path: path, Installed: true, Owner: strings.TrimSpace(raw)}, nil
}

func archFetchJSON(ctx context.Context, target string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := archHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("fetch %s: status %d: %s", target, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode %s: %w", target, err)
	}
	return nil
}

func archFetchText(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := archHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("fetch %s: status %d: %s", target, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", target, err)
	}
	return string(body), nil
}

func archCleanHTML(raw string) string {
	raw = html.UnescapeString(raw)
	raw = archHTMLTagRe.ReplaceAllString(raw, " ")
	raw = archWhitespaceRe.ReplaceAllString(raw, " ")
	return strings.TrimSpace(raw)
}

func archWikiPageURL(title string) string {
	title = strings.ReplaceAll(strings.TrimSpace(title), " ", "_")
	return "https://wiki.archlinux.org/title/" + url.PathEscape(title)
}

func archJoinVersion(ver, rel string) string {
	ver = strings.TrimSpace(ver)
	rel = strings.TrimSpace(rel)
	if ver == "" {
		return ""
	}
	if rel == "" {
		return ver
	}
	return ver + "-" + rel
}

func archSplitNonEmptyLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func archRunOptionalCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			trimmed := strings.TrimSpace(string(out))
			if trimmed == "" {
				return "", nil
			}
			return string(out), nil
		}
		return string(out), fmt.Errorf("%s failed: %w: %s", name, err, string(out))
	}
	return string(out), nil
}
