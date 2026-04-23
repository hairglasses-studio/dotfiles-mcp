package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/hairglasses-studio/dotfiles-mcp/internal/dotfiles"
	"github.com/hairglasses-studio/mcpkit/registry"
)

func main() {
	// Handle --session-index: output session index as JSONL for ccg.
	if len(os.Args) > 1 && os.Args[1] == "--session-index" {
		if err := dotfiles.OutputSessionIndex(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ctx := context.Background()
	_, s, shutdownTracing := dotfiles.Setup(ctx)
	defer shutdownTracing(ctx)

	if err := registry.ServeAuto(s); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
