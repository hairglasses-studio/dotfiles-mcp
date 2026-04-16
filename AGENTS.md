# dotfiles-mcp

MCP server for dotfiles configuration management, GitHub org lifecycle, fleet auditing, and desktop service orchestration. Built with [mcpkit](https://github.com/hairglasses-studio/mcpkit).

## Build & Test
```bash
go build ./...
go vet ./...
go test ./... -count=1
go install .
```

## Key Conventions
- All batch/write tools use dry-run by default (`execute: true` for live mode)
- `bulk_settings` reports previous state before applying changes
- `clean_stale` checks for uncommitted/unpushed work before deletion
- `pull_all` detects dirty repos and detached HEAD, skips safely
- Composed "tool-of-tools" (full_sync, fleet_audit, cascade_reload, rice_check, bulk_pipeline) eliminate multi-step token waste

## Tools (99)

99 tools across 14 categories. Use `dotfiles_server_health` for the current contract shape.

Key composed tools: `full_sync`, `fleet_audit`, `cascade_reload`, `rice_check`, `bulk_pipeline`, `bt_discover_and_connect`, `input_auto_setup_controller`, `ops_iterate`, `ops_ship`, `sandbox_validate`.
