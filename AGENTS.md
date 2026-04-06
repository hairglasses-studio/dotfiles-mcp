# dotfiles-mcp — Agent Instructions
> Canonical instructions: AGENTS.md


## Project Overview
MCP server for dotfiles configuration management. Provides validated config editing, symlink health checks, and service reloading over the Model Context Protocol (stdio transport).

## Tech Stack
- Go (single-binary MCP server)
- mcp-go SDK (github.com/mark3labs/mcp-go)
- BurntSushi/toml for config parsing

## Build & Run
```bash
go build -o dotfiles-mcp .
DOTFILES_DIR=$HOME/hairglasses-studio/dotfiles ./dotfiles-mcp
```

## Test
```bash
go test ./...
```

## Architecture
- `main.go` — Single-file MCP server with all tool handlers
- Stdio transport, designed to run as a Claude Code MCP subprocess
- Reads/writes dotfiles from `DOTFILES_DIR` (default: `~/hairglasses-studio/dotfiles`)
- Tools: config editing, symlink management, service reload

## Code Standards
- Go standard formatting (gofmt)
- Error wrapping with context
- All MCP tool handlers follow mcp-go patterns

## Shared Research Repository

Cross-project research lives at `~/hairglasses-studio/docs/` (git: hairglasses-studio/docs). When launching research agents, check existing docs first and write reusable research outputs back to the shared repo rather than local docs/.
