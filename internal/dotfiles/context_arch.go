package dotfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/hairglasses-studio/mcpkit/prompts"
	"github.com/hairglasses-studio/mcpkit/resources"
	"github.com/mark3labs/mcp-go/mcp"
)

type archResourceModule struct{}

func (m *archResourceModule) Name() string { return "arch_resources" }
func (m *archResourceModule) Description() string {
	return "Arch Linux package, news, wiki, and AUR resources"
}

func (m *archResourceModule) Resources() []resources.ResourceDefinition {
	return []resources.ResourceDefinition{
		{
			Resource: mcp.NewResource(
				"archnews://latest",
				"Latest Arch News",
				mcp.WithResourceDescription("Latest Arch Linux news items from the official RSS feed"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				items, err := archFetchNews(ctx)
				if err != nil {
					return nil, err
				}
				if len(items) > 10 {
					items = items[:10]
				}
				data, _ := json.MarshalIndent(archNewsOutput{Items: items, Total: len(items)}, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: "archnews://latest", MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "arch",
			Tags:     []string{"arch", "news", "updates"},
		},
		{
			Resource: mcp.NewResource(
				"archnews://critical",
				"Critical Arch News",
				mcp.WithResourceDescription("Critical or manual-intervention Arch news items"),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				items, err := archFetchNews(ctx)
				if err != nil {
					return nil, err
				}
				filtered := make([]archNewsItem, 0)
				for _, item := range items {
					if critical, reasons := archNewsCriticalReasons(item, nil); critical {
						item.Reasons = reasons
						filtered = append(filtered, item)
					}
				}
				if len(filtered) > 10 {
					filtered = filtered[:10]
				}
				data, _ := json.MarshalIndent(archNewsOutput{Items: filtered, Total: len(filtered)}, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: "archnews://critical", MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "arch",
			Tags:     []string{"arch", "news", "critical"},
		},
	}
}

func (m *archResourceModule) Templates() []resources.TemplateDefinition {
	return []resources.TemplateDefinition{
		{
			Template: mcp.NewResourceTemplate(
				"archwiki://search/{query}",
				"ArchWiki Search",
				mcp.WithTemplateDescription("Search the ArchWiki for a query string"),
				mcp.WithTemplateMIMEType("application/json"),
			),
			Handler: func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				query := archTemplateValue(req.Params.URI, "archwiki://search/")
				out, err := archWikiSearch(ctx, query, 10)
				if err != nil {
					return nil, err
				}
				data, _ := json.MarshalIndent(out, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "arch",
			Tags:     []string{"arch", "wiki", "search"},
		},
		{
			Template: mcp.NewResourceTemplate(
				"archwiki://page/{title}",
				"ArchWiki Page",
				mcp.WithTemplateDescription("Read an ArchWiki page by title"),
				mcp.WithTemplateMIMEType("application/json"),
			),
			Handler: func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				title := archTemplateValue(req.Params.URI, "archwiki://page/")
				out, err := archWikiPage(ctx, title)
				if err != nil {
					return nil, err
				}
				data, _ := json.MarshalIndent(out, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "arch",
			Tags:     []string{"arch", "wiki", "page"},
		},
		{
			Template: mcp.NewResourceTemplate(
				"archrepo://package/{name}",
				"Arch Package Metadata",
				mcp.WithTemplateDescription("Read exact official Arch package metadata by package name"),
				mcp.WithTemplateMIMEType("application/json"),
			),
			Handler: func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				name := archTemplateValue(req.Params.URI, "archrepo://package/")
				out, err := archPackageInfo(ctx, name)
				if err != nil {
					return nil, err
				}
				data, _ := json.MarshalIndent(out, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "arch",
			Tags:     []string{"arch", "packages", "repo"},
		},
		{
			Template: mcp.NewResourceTemplate(
				"aur://package/{name}/info",
				"AUR Package Metadata",
				mcp.WithTemplateDescription("Read AUR package metadata by package name"),
				mcp.WithTemplateMIMEType("application/json"),
			),
			Handler: func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				name := archTemplateTrimSuffix(archTemplateValue(req.Params.URI, "aur://package/"), "/info")
				out, err := archAURSearch(ctx, name, 25)
				if err != nil {
					return nil, err
				}
				filtered := out.Results[:0]
				for _, item := range out.Results {
					if item.Name == name {
						filtered = append(filtered, item)
					}
				}
				out.Results = filtered
				out.Total = len(filtered)
				data, _ := json.MarshalIndent(out, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
				}, nil
			},
			Category: "arch",
			Tags:     []string{"arch", "aur", "package"},
		},
		{
			Template: mcp.NewResourceTemplate(
				"aur://package/{name}/pkgbuild",
				"AUR PKGBUILD",
				mcp.WithTemplateDescription("Read the raw PKGBUILD for an AUR package"),
				mcp.WithTemplateMIMEType("text/plain"),
			),
			Handler: func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				name := archTemplateTrimSuffix(archTemplateValue(req.Params.URI, "aur://package/"), "/pkgbuild")
				text, err := archFetchText(ctx, fmt.Sprintf(archAURPKGBUILDURLPattern, url.PathEscape(name)))
				if err != nil {
					return nil, err
				}
				return []mcp.ResourceContents{
					mcp.TextResourceContents{URI: req.Params.URI, MIMEType: "text/plain", Text: text},
				}, nil
			},
			Category: "arch",
			Tags:     []string{"arch", "aur", "pkgbuild"},
		},
	}
}

type archPromptModule struct{}

func (m *archPromptModule) Name() string { return "arch_prompts" }
func (m *archPromptModule) Description() string {
	return "Prompt workflows for safe Arch Linux investigation, updates, and AUR auditing"
}

func (m *archPromptModule) Prompts() []prompts.PromptDefinition {
	return []prompts.PromptDefinition{
		{
			Prompt: mcp.NewPrompt(
				"safe_system_update",
				mcp.WithPromptDescription("Review critical Arch news and pending updates before deciding on a package upgrade path"),
				mcp.WithArgument("focus_packages", mcp.ArgumentDescription("Optional comma-separated package names to emphasize")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				focus := strings.TrimSpace(req.Params.Arguments["focus_packages"])
				extra := ""
				if focus != "" {
					extra = fmt.Sprintf(" Treat %q as the package focus list when reviewing critical news and package metadata.", focus)
				}
				return mcp.NewGetPromptResult("Plan a safe Arch system update", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(
						"Plan a safe Arch Linux update. Start with `arch_news_critical` and `arch_news_latest` to identify manual-intervention notices, then use `arch_updates_dry_run` to inventory pending official and AUR updates. If package-specific risk matters, use `arch_package_info`, `arch_aur_search`, and `arch_pkgbuild_audit` before suggesting any upgrade path. Do not recommend a blind `pacman -Syu` until the critical-news review is complete."+extra,
					)),
				}), nil
			},
			Category: "arch",
			Tags:     []string{"arch", "updates", "safety"},
		},
		{
			Prompt: mcp.NewPrompt(
				"audit_aur_package",
				mcp.WithPromptDescription("Audit one AUR package before installation or upgrade"),
				mcp.WithArgument("package", mcp.RequiredArgument(), mcp.ArgumentDescription("AUR package name")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				pkg := strings.TrimSpace(req.Params.Arguments["package"])
				return mcp.NewGetPromptResult("Audit an AUR package", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Audit the AUR package %q before installation. Use `arch_aur_search` to confirm the package metadata, `arch_pkgbuild_audit` to inspect risky PKGBUILD patterns, and the `aur://package/%s/pkgbuild` resource if you need the full raw PKGBUILD. Summarize the risk level, suspicious lines, maintainer status, and whether installation looks reasonable.",
						pkg, pkg,
					))),
				}), nil
			},
			Category: "arch",
			Tags:     []string{"arch", "aur", "audit"},
		},
		{
			Prompt: mcp.NewPrompt(
				"troubleshoot_issue",
				mcp.WithPromptDescription("Troubleshoot an Arch Linux issue with wiki, package, and log evidence"),
				mcp.WithArgument("symptom", mcp.RequiredArgument(), mcp.ArgumentDescription("Short description of the issue")),
			),
			Handler: func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				symptom := strings.TrimSpace(req.Params.Arguments["symptom"])
				return mcp.NewGetPromptResult("Troubleshoot an Arch issue", []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(fmt.Sprintf(
						"Troubleshoot this Arch Linux issue: %q. Start with `archwiki_search` and `archwiki_page` to find the canonical guidance, then use `arch_news_critical` to check for known breakage or manual-intervention news. If packages or filesystem ownership are involved, use `arch_package_info`, `arch_file_owner`, and `arch_pacman_log` to collect local evidence before suggesting remediation.",
						symptom,
					))),
				}), nil
			},
			Category: "arch",
			Tags:     []string{"arch", "troubleshooting", "wiki"},
		},
	}
}

func archTemplateValue(uri, prefix string) string {
	return strings.TrimSpace(archTemplateTrimSuffix(strings.TrimPrefix(uri, prefix), ""))
}

func archTemplateTrimSuffix(value, suffix string) string {
	value = strings.TrimSpace(value)
	if suffix != "" && strings.HasSuffix(value, suffix) {
		value = strings.TrimSuffix(value, suffix)
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}
