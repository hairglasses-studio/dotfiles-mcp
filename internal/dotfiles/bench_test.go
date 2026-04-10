package dotfiles

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/mcpkit/registry"
)

func benchmarkRegistry() *registry.ToolRegistry {
	reg := registry.NewToolRegistry()
	registerDotfilesModules(reg, nil, nil, dotfilesMCPVersion)
	return reg
}

// BenchmarkToolSearch measures the discovery tool search handler, which is the
// most-called tool in dotfiles-mcp (agents call it to find tools before use).
func BenchmarkToolSearch(b *testing.B) {
	b.Setenv("DOTFILES_MCP_PROFILE", "full")

	reg := benchmarkRegistry()

	disc := &DotfilesDiscoveryModule{reg: reg}
	tools := disc.Tools()

	var searchTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_tool_search" {
			searchTool = td
			break
		}
	}

	ctx := context.Background()
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{"query": "shader", "limit": float64(10)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		searchTool.Handler(ctx, req)
	}
}

// BenchmarkToolCatalog measures the catalog handler which builds a grouped
// view of all registered tools — used by agents for initial orientation.
func BenchmarkToolCatalog(b *testing.B) {
	b.Setenv("DOTFILES_MCP_PROFILE", "full")

	reg := benchmarkRegistry()

	disc := &DotfilesDiscoveryModule{reg: reg}
	tools := disc.Tools()

	var catalogTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_tool_catalog" {
			catalogTool = td
			break
		}
	}

	ctx := context.Background()
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		catalogTool.Handler(ctx, req)
	}
}

// BenchmarkToolStats measures the stats handler — lightweight aggregation
// over the full registry.
func BenchmarkToolStats(b *testing.B) {
	b.Setenv("DOTFILES_MCP_PROFILE", "full")

	reg := benchmarkRegistry()

	disc := &DotfilesDiscoveryModule{reg: reg}
	tools := disc.Tools()

	var statsTool registry.ToolDefinition
	for _, td := range tools {
		if td.Tool.Name == "dotfiles_tool_stats" {
			statsTool = td
			break
		}
	}

	ctx := context.Background()
	req := registry.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		statsTool.Handler(ctx, req)
	}
}

// BenchmarkRegistrySetup measures the cost of registering all dotfiles modules
// with the registry — this runs once at server startup.
func BenchmarkRegistrySetup(b *testing.B) {
	b.Setenv("DOTFILES_MCP_PROFILE", "full")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = benchmarkRegistry()
	}
}

// BenchmarkShouldDeferDotfilesTool measures the per-tool deferred loading
// decision, called once per tool during registration.
func BenchmarkShouldDeferDotfilesTool(b *testing.B) {
	td := registry.ToolDefinition{}
	td.Tool.Name = "hypr_list_windows"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shouldDeferDotfilesTool("default", td)
	}
}
