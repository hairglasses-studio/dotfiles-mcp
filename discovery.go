package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

type dotfilesToolSearchInput struct {
	Query string `json:"query" jsonschema:"required,description=Keywords to search across tool names descriptions tags and search terms"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Maximum results to return. Default 10."`
}

type dotfilesToolSearchResult struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	SearchTerms []string `json:"search_terms,omitempty"`
	MatchType   string   `json:"match_type"`
	Deferred    bool     `json:"deferred"`
}

type dotfilesToolSearchOutput struct {
	Results []dotfilesToolSearchResult `json:"results"`
	Total   int                        `json:"total"`
}

type dotfilesToolSchemaInput struct {
	Name string `json:"name" jsonschema:"required,description=Exact tool name to inspect"`
}

type dotfilesToolSchemaOutput struct {
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	Tags           []string       `json:"tags,omitempty"`
	SearchTerms    []string       `json:"search_terms,omitempty"`
	IsWrite        bool           `json:"is_write"`
	Deferred       bool           `json:"deferred"`
	InputSchema    map[string]any `json:"input_schema,omitempty"`
	OutputSchema   map[string]any `json:"output_schema,omitempty"`
	DescriptorMeta map[string]any `json:"descriptor_meta,omitempty"`
}

type dotfilesToolCatalogInput struct {
	Category string `json:"category,omitempty" jsonschema:"description=Optional category filter"`
}

type dotfilesToolCatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Deferred    bool   `json:"deferred"`
}

type dotfilesToolCatalogGroup struct {
	Category      string                     `json:"category"`
	ToolCount     int                        `json:"tool_count"`
	DeferredCount int                        `json:"deferred_count"`
	Tools         []dotfilesToolCatalogEntry `json:"tools"`
}

type dotfilesToolCatalogOutput struct {
	Groups []dotfilesToolCatalogGroup `json:"groups"`
}

type dotfilesToolStatsInput struct{}

type dotfilesToolStatsOutput struct {
	TotalTools      int            `json:"total_tools"`
	ModuleCount     int            `json:"module_count"`
	DeferredTools   int            `json:"deferred_tools"`
	ByCategory      map[string]int `json:"by_category"`
	ByRuntimeGroup  map[string]int `json:"by_runtime_group"`
	WriteToolsCount int            `json:"write_tools_count"`
	ReadOnlyCount   int            `json:"read_only_count"`
}

type DotfilesDiscoveryModule struct {
	reg *registry.ToolRegistry
}

func (m *DotfilesDiscoveryModule) Name() string { return "discovery" }
func (m *DotfilesDiscoveryModule) Description() string {
	return "Discovery tools for the dotfiles catalog"
}

func (m *DotfilesDiscoveryModule) Tools() []registry.ToolDefinition {
	search := handler.TypedHandler[dotfilesToolSearchInput, dotfilesToolSearchOutput](
		"dotfiles_tool_search",
		"Search the dotfiles tool catalog by keyword before invoking a desktop or workflow tool.",
		func(_ context.Context, input dotfilesToolSearchInput) (dotfilesToolSearchOutput, error) {
			limit := input.Limit
			if limit <= 0 {
				limit = 10
			}
			results := m.reg.SearchTools(input.Query)
			if len(results) > limit {
				results = results[:limit]
			}
			out := make([]dotfilesToolSearchResult, 0, len(results))
			for _, result := range results {
				out = append(out, dotfilesToolSearchResult{
					Name:        result.Tool.Tool.Name,
					Description: result.Tool.Tool.Description,
					Category:    result.Tool.Category,
					Tags:        result.Tool.Tags,
					SearchTerms: result.Tool.SearchTerms,
					MatchType:   result.MatchType,
					Deferred:    m.reg.IsDeferred(result.Tool.Tool.Name),
				})
			}
			return dotfilesToolSearchOutput{Results: out, Total: len(out)}, nil
		},
	)
	search.Category = "discovery"
	search.SearchTerms = []string{"find tool", "which tool", "tool discovery", "catalog search"}

	schema := handler.TypedHandler[dotfilesToolSchemaInput, dotfilesToolSchemaOutput](
		"dotfiles_tool_schema",
		"Inspect one tool descriptor including schemas, search terms, and deferred-loading hints.",
		func(_ context.Context, input dotfilesToolSchemaInput) (dotfilesToolSchemaOutput, error) {
			td, ok := m.reg.GetTool(input.Name)
			if !ok {
				return dotfilesToolSchemaOutput{}, fmt.Errorf("tool not found: %s", input.Name)
			}
			annotated := registry.ApplyToolMetadata(td, "", m.reg.IsDeferred(td.Tool.Name))
			return dotfilesToolSchemaOutput{
				Name:           td.Tool.Name,
				Description:    td.Tool.Description,
				Category:       td.Category,
				Tags:           td.Tags,
				SearchTerms:    td.SearchTerms,
				IsWrite:        td.IsWrite,
				Deferred:       m.reg.IsDeferred(td.Tool.Name),
				InputSchema:    schemaMap(td.Tool.InputSchema),
				OutputSchema:   schemaMap(annotated.Tool.OutputSchema),
				DescriptorMeta: toolMetaMap(annotated.Tool),
			}, nil
		},
	)
	schema.Category = "discovery"
	schema.SearchTerms = []string{"tool schema", "tool descriptor", "input schema", "output schema"}

	catalog := handler.TypedHandler[dotfilesToolCatalogInput, dotfilesToolCatalogOutput](
		"dotfiles_tool_catalog",
		"Summarize the dotfiles tool catalog by category with deferred-loading hints.",
		func(_ context.Context, input dotfilesToolCatalogInput) (dotfilesToolCatalogOutput, error) {
			groups := make([]dotfilesToolCatalogGroup, 0)
			catalog := m.reg.GetToolCatalog()
			categories := make([]string, 0, len(catalog))
			for category := range catalog {
				if input.Category == "" || input.Category == category {
					categories = append(categories, category)
				}
			}
			sort.Strings(categories)

			for _, category := range categories {
				group := dotfilesToolCatalogGroup{Category: category}
				subcategories := catalog[category]
				subcategoryNames := make([]string, 0, len(subcategories))
				for subcategory := range subcategories {
					subcategoryNames = append(subcategoryNames, subcategory)
				}
				sort.Strings(subcategoryNames)
				for _, subcategory := range subcategoryNames {
					tools := subcategories[subcategory]
					sort.Slice(tools, func(i, j int) bool { return tools[i].Tool.Name < tools[j].Tool.Name })
					for _, td := range tools {
						deferred := m.reg.IsDeferred(td.Tool.Name)
						group.ToolCount++
						if deferred {
							group.DeferredCount++
						}
						group.Tools = append(group.Tools, dotfilesToolCatalogEntry{
							Name:        td.Tool.Name,
							Description: td.Tool.Description,
							Deferred:    deferred,
						})
					}
				}
				groups = append(groups, group)
			}

			return dotfilesToolCatalogOutput{Groups: groups}, nil
		},
	)
	catalog.Category = "discovery"
	catalog.SearchTerms = []string{"tool catalog", "tool list", "categories", "browse tools"}

	stats := handler.TypedHandler[dotfilesToolStatsInput, dotfilesToolStatsOutput](
		"dotfiles_tool_stats",
		"Show high-level catalog statistics, including how many tools are marked deferred in the active profile.",
		func(_ context.Context, _ dotfilesToolStatsInput) (dotfilesToolStatsOutput, error) {
			toolStats := m.reg.GetToolStats()
			return dotfilesToolStatsOutput{
				TotalTools:      toolStats.TotalTools,
				ModuleCount:     toolStats.ModuleCount,
				DeferredTools:   len(m.reg.ListDeferredTools()),
				ByCategory:      toolStats.ByCategory,
				ByRuntimeGroup:  toolStats.ByRuntimeGroup,
				WriteToolsCount: toolStats.WriteToolsCount,
				ReadOnlyCount:   toolStats.ReadOnlyCount,
			}, nil
		},
	)
	stats.Category = "discovery"
	stats.SearchTerms = []string{"tool stats", "catalog stats", "tool counts"}

	return []registry.ToolDefinition{search, schema, catalog, stats}
}

func registerDotfilesModules(reg *registry.ToolRegistry) {
	reg.RegisterModule(&DotfilesDiscoveryModule{reg: reg})

	profile := dotfilesProfile()
	for _, module := range dotfilesModules() {
		deferred := make(map[string]bool)
		for _, td := range module.Tools() {
			if shouldDeferDotfilesTool(profile, td) {
				deferred[td.Tool.Name] = true
			}
		}
		reg.RegisterDeferredModule(module, deferred)
	}
}

func dotfilesModules() []registry.ToolModule {
	return []registry.ToolModule{
		&DotfilesModule{},
		&HyprlandModule{},
		&ShaderModule{},
		&InputModule{},
		&BluetoothModule{},
		&ControllerModule{},
		&MidiModule{},
		&SolaarModule{},
		&WorkflowModule{},
		&OSSModule{},
		&MappingEngineModule{},
		&LearnModule{},
		&MappingStatusModule{},
		&ScreenModule{},
		&InputSimulateModule{},
		&ClaudeSessionModule{},
	}
}

func dotfilesProfile() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DOTFILES_MCP_PROFILE"))) {
	case "", "default":
		return "default"
	case "ops":
		return "ops"
	case "full":
		return "full"
	default:
		return "default"
	}
}

func shouldDeferDotfilesTool(profile string, td registry.ToolDefinition) bool {
	switch profile {
	case "full":
		return false
	case "ops":
		return !(strings.HasPrefix(td.Tool.Name, "dotfiles_") ||
			strings.HasPrefix(td.Tool.Name, "workflow_") ||
			strings.HasPrefix(td.Tool.Name, "oss_"))
	default:
		return true
	}
}

func schemaMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func toolMetaMap(tool registry.Tool) map[string]any {
	if tool.Meta == nil {
		return nil
	}
	data, err := json.Marshal(tool.Meta)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
