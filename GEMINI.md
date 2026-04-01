# dotfiles-mcp — Gemini CLI Instructions

## Overview
Go MCP server for dotfiles management — config editing, symlink checks, service reloading via stdio transport.

## Build & Test
```bash
go build -o dotfiles-mcp .
go test ./...
```

## Key Details
- Single-file server: `main.go`
- SDK: mcp-go (github.com/mark3labs/mcp-go)
- Env: `DOTFILES_DIR` sets dotfiles path
