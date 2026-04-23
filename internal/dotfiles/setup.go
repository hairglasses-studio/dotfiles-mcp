package dotfiles

import (
	"context"

	"github.com/hairglasses-studio/dotfiles-mcp/internal/tracing"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/mcpkit/resilience"
)

func Setup(ctx context.Context) (*registry.ToolRegistry, *registry.MCPServer, func(context.Context) error) {
	shutdownTracing := tracing.Init(ctx, dotfilesMCPVersion)

	cbRegistry := resilience.NewCircuitBreakerRegistry(nil)
	mw := []registry.Middleware{
		registry.AuditMiddleware(""),
		registry.SafetyTierMiddleware(),
		resilience.CircuitBreakerMiddleware(cbRegistry),
		tracing.Middleware(),
	}

	reg := registry.NewToolRegistry(registry.Config{
		Middleware: mw,
	})
	promptReg := buildDotfilesPromptRegistry()
	resReg := buildDotfilesResourceRegistry(reg, promptReg)
	registerDotfilesModules(reg, resReg, promptReg, dotfilesMCPVersion)

	s := registry.NewMCPServer("dotfiles-mcp", dotfilesMCPVersion)
	reg.RegisterWithServer(s)
	resReg.RegisterWithServer(s)
	promptReg.RegisterWithServer(s)

	return reg, s, shutdownTracing
}

func OutputSessionIndex() error {
	return outputSessionIndex()
}
