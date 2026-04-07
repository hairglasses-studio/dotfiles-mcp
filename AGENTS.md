# This repo uses [AGENTS.md](AGENTS.md) as the canonical instruction file. Treat this file as compatibility guidance for Claude-specific workflows. — Agent Instructions



> Canonical source: CLAUDE.md

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
