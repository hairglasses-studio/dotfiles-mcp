package dotfiles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hairglasses-studio/mcpkit/prompts"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/mcpkit/resources"
)

const canonicalSourceURL = "https://github.com/hairglasses-studio/dotfiles/tree/main/mcp/dotfiles-mcp"

type ContractToolSnapshot struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Category     string   `json:"category"`
	Subcategory  string   `json:"subcategory,omitempty"`
	RuntimeGroup string   `json:"runtime_group,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	SearchTerms  []string `json:"search_terms,omitempty"`
	IsWrite      bool     `json:"is_write"`
	Deferred     bool     `json:"deferred"`
}

type ContractResourceSnapshot struct {
	URI         string   `json:"uri"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	MIMEType    string   `json:"mime_type,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type ContractTemplateSnapshot struct {
	URITemplate string   `json:"uri_template"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	MIMEType    string   `json:"mime_type,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type ContractPromptSnapshot struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Version     string   `json:"version,omitempty"`
}

type ContractOverviewSnapshot struct {
	Version         string         `json:"version"`
	Profile         string         `json:"profile"`
	CanonicalSource string         `json:"canonical_source"`
	PublishMirror   bool           `json:"publish_mirror"`
	TotalTools      int            `json:"total_tools"`
	ModuleCount     int            `json:"module_count"`
	DeferredTools   int            `json:"deferred_tools"`
	ResourceCount   int            `json:"resource_count"`
	TemplateCount   int            `json:"template_count"`
	PromptCount     int            `json:"prompt_count"`
	ByCategory      map[string]int `json:"by_category"`
	ByRuntimeGroup  map[string]int `json:"by_runtime_group"`
	DiscoveryTools  []string       `json:"discovery_tools"`
}

type WellKnownCapabilities struct {
	Tools     bool `json:"tools"`
	Resources bool `json:"resources"`
	Prompts   bool `json:"prompts"`
}

type WellKnownTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type WellKnownManifest struct {
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	Version         string                `json:"version"`
	Homepage        string                `json:"homepage"`
	License         string                `json:"license"`
	Organization    string                `json:"organization"`
	Repository      string                `json:"repository"`
	CanonicalSource string                `json:"canonical_source"`
	PublishMirror   bool                  `json:"publish_mirror"`
	Transport       []string              `json:"transport"`
	Profiles        []string              `json:"profiles"`
	DefaultProfile  string                `json:"default_profile"`
	Capabilities    WellKnownCapabilities `json:"capabilities"`
	Tools           []WellKnownTool       `json:"tools"`
	Tags            []string              `json:"tags"`
	ToolCount       int                   `json:"tool_count"`
	ResourceCount   int                   `json:"resource_count"`
	PromptCount     int                   `json:"prompt_count"`
	Categories      []string              `json:"categories"`
}

type ContractSnapshotBundle struct {
	Overview  ContractOverviewSnapshot   `json:"overview"`
	Tools     []ContractToolSnapshot     `json:"tools"`
	Resources []ContractResourceSnapshot `json:"resources"`
	Templates []ContractTemplateSnapshot `json:"templates"`
	Prompts   []ContractPromptSnapshot   `json:"prompts"`
	Manifest  WellKnownManifest          `json:"manifest"`
}

func withDotfilesProfile(profile string, fn func() error) error {
	prev, had := os.LookupEnv("DOTFILES_MCP_PROFILE")
	if profile == "" {
		profile = "default"
	}
	if err := os.Setenv("DOTFILES_MCP_PROFILE", profile); err != nil {
		return err
	}
	defer func() {
		if had {
			_ = os.Setenv("DOTFILES_MCP_PROFILE", prev)
			return
		}
		_ = os.Unsetenv("DOTFILES_MCP_PROFILE")
	}()
	return fn()
}

func BuildContractSnapshotBundle(profile string) (ContractSnapshotBundle, error) {
	var bundle ContractSnapshotBundle
	err := withDotfilesProfile(profile, func() error {
		reg := registry.NewToolRegistry()
		promptReg := buildDotfilesPromptRegistry()
		resReg := buildDotfilesResourceRegistry(reg, promptReg)
		registerDotfilesModules(reg, resReg, promptReg, dotfilesMCPVersion)

		activeProfile := dotfilesProfile()
		stats := reg.GetToolStats()

		toolDefs := reg.GetAllToolDefinitions()
		sort.Slice(toolDefs, func(i, j int) bool { return toolDefs[i].Tool.Name < toolDefs[j].Tool.Name })
		totalTools := len(toolDefs)
		resourceDefs := resReg.GetAllResourceDefinitions()
		sort.Slice(resourceDefs, func(i, j int) bool { return resourceDefs[i].Resource.URI < resourceDefs[j].Resource.URI })
		templateDefs := resReg.GetAllTemplateDefinitions()
		sort.Slice(templateDefs, func(i, j int) bool {
			return templateDefs[i].Template.URITemplate.Raw() < templateDefs[j].Template.URITemplate.Raw()
		})
		promptDefs := promptReg.GetAllPromptDefinitions()
		sort.Slice(promptDefs, func(i, j int) bool { return promptDefs[i].Prompt.Name < promptDefs[j].Prompt.Name })

		toolsOut := make([]ContractToolSnapshot, 0, len(toolDefs))
		manifestTools := make([]WellKnownTool, 0, len(toolDefs))
		for _, td := range toolDefs {
			toolsOut = append(toolsOut, ContractToolSnapshot{
				Name:         td.Tool.Name,
				Description:  td.Tool.Description,
				Category:     td.Category,
				Subcategory:  td.Subcategory,
				RuntimeGroup: td.RuntimeGroup,
				Tags:         append([]string(nil), td.Tags...),
				SearchTerms:  append([]string(nil), td.SearchTerms...),
				IsWrite:      td.IsWrite,
				Deferred:     reg.IsDeferred(td.Tool.Name),
			})
			manifestTools = append(manifestTools, WellKnownTool{
				Name:        td.Tool.Name,
				Description: td.Tool.Description,
			})
		}

		resourcesOut := make([]ContractResourceSnapshot, 0, len(resourceDefs))
		for _, rd := range resourceDefs {
			resourcesOut = append(resourcesOut, ContractResourceSnapshot{
				URI:         rd.Resource.URI,
				Name:        rd.Resource.Name,
				Description: rd.Resource.Description,
				MIMEType:    rd.Resource.MIMEType,
				Category:    rd.Category,
				Tags:        append([]string(nil), rd.Tags...),
			})
		}

		templatesOut := make([]ContractTemplateSnapshot, 0, len(templateDefs))
		for _, td := range templateDefs {
			templatesOut = append(templatesOut, ContractTemplateSnapshot{
				URITemplate: td.Template.URITemplate.Raw(),
				Name:        td.Template.Name,
				Description: td.Template.Description,
				MIMEType:    td.Template.MIMEType,
				Category:    td.Category,
				Tags:        append([]string(nil), td.Tags...),
			})
		}

		promptsOut := make([]ContractPromptSnapshot, 0, len(promptDefs))
		for _, pd := range promptDefs {
			promptsOut = append(promptsOut, ContractPromptSnapshot{
				Name:        pd.Prompt.Name,
				Description: pd.Prompt.Description,
				Category:    pd.Category,
				Tags:        append([]string(nil), pd.Tags...),
				Version:     pd.Version,
			})
		}

		categories := make([]string, 0, len(stats.ByCategory))
		for category := range stats.ByCategory {
			categories = append(categories, category)
		}
		sort.Strings(categories)

		resourceCount := resReg.ResourceCount() + resReg.TemplateCount()
		bundle.Overview = ContractOverviewSnapshot{
			Version:         dotfilesMCPVersion,
			Profile:         activeProfile,
			CanonicalSource: canonicalSourceURL,
			PublishMirror:   true,
			TotalTools:      totalTools,
			ModuleCount:     stats.ModuleCount,
			DeferredTools:   len(reg.ListDeferredTools()),
			ResourceCount:   resourceCount,
			TemplateCount:   resReg.TemplateCount(),
			PromptCount:     promptReg.PromptCount(),
			ByCategory:      stats.ByCategory,
			ByRuntimeGroup:  stats.ByRuntimeGroup,
			DiscoveryTools:  dotfilesDiscoveryToolNames(),
		}
		bundle.Tools = toolsOut
		bundle.Resources = resourcesOut
		bundle.Templates = templatesOut
		bundle.Prompts = promptsOut
		bundle.Manifest = WellKnownManifest{
			Name:            "io.github.hairglasses-studio.dotfiles-mcp",
			Description:     "Publish mirror of the canonical dotfiles-mcp workstation MCP server with discovery-first tools, resources, and prompts.",
			Version:         dotfilesMCPVersion,
			Homepage:        "https://github.com/hairglasses-studio/dotfiles-mcp",
			License:         "MIT",
			Organization:    "hairglasses-studio",
			Repository:      "https://github.com/hairglasses-studio/dotfiles-mcp",
			CanonicalSource: canonicalSourceURL,
			PublishMirror:   true,
			Transport:       []string{"stdio"},
			Profiles:        []string{"default", "desktop", "ops", "full"},
			DefaultProfile:  "default",
			Capabilities: WellKnownCapabilities{
				Tools:     true,
				Resources: true,
				Prompts:   true,
			},
			Tools:         manifestTools,
			Tags:          []string{"linux", "desktop", "hyprland", "wayland", "bluetooth", "input", "github-org", "fleet-management", "publish-mirror"},
			ToolCount:     totalTools,
			ResourceCount: resourceCount,
			PromptCount:   promptReg.PromptCount(),
			Categories:    categories,
		}
		return nil
	})
	return bundle, err
}

func SnapshotJSON(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func contractSnapshotFiles(bundle ContractSnapshotBundle) (map[string][]byte, error) {
	files := map[string]any{
		".well-known/mcp.json":              bundle.Manifest,
		"snapshots/contract/overview.json":  bundle.Overview,
		"snapshots/contract/tools.json":     bundle.Tools,
		"snapshots/contract/resources.json": bundle.Resources,
		"snapshots/contract/templates.json": bundle.Templates,
		"snapshots/contract/prompts.json":   bundle.Prompts,
	}

	out := make(map[string][]byte, len(files))
	for path, payload := range files {
		content, err := SnapshotJSON(payload)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out[path] = append(content, '\n')
	}
	return out, nil
}

func writeContractSnapshotFiles(files map[string][]byte) error {
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
		if err != nil {
			return err
		}
		tmpPath := tmp.Name()
		if _, err := tmp.Write(content); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return err
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		if err := os.Rename(tmpPath, path); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
	}
	return nil
}

func checkContractSnapshotFiles(files map[string][]byte) error {
	var drift []string
	for path, content := range files {
		current, err := os.ReadFile(path)
		if err != nil {
			drift = append(drift, fmt.Sprintf("%s (missing: %v)", path, err))
			continue
		}
		if !bytes.Equal(current, content) {
			drift = append(drift, path)
		}
	}
	if len(drift) == 0 {
		return nil
	}
	return fmt.Errorf("contract drift detected:\n%s", stringsJoinBullets(drift))
}

func stringsJoinBullets(items []string) string {
	var buf bytes.Buffer
	for _, item := range items {
		buf.WriteString(" - ")
		buf.WriteString(item)
		buf.WriteByte('\n')
	}
	return string(bytes.TrimRight(buf.Bytes(), "\n"))
}

var _ = resources.NewResourceRegistry
var _ = prompts.NewPromptRegistry
