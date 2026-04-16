This repo uses [AGENTS.md](AGENTS.md) as the canonical instruction file.

## Summary

dotfiles-mcp: MCP server for dotfiles config management, GitHub org lifecycle, fleet auditing, and desktop orchestration. 99 tools across 14 categories. Built with mcpkit.

## Build & Test
```bash
go build ./...
go vet ./...
go test ./... -count=1
go install .
```
