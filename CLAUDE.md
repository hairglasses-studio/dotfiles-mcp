# dotfiles-mcp

MCP server for dotfiles configuration management, GitHub org lifecycle, fleet auditing, and desktop service orchestration. Built with [mcpkit](https://github.com/hairglasses-studio/mcpkit).

## Build & Test
```bash
go build ./...
go vet ./...
go test ./... -count=1
go install .
```

## Tools (30)

### Config Management (4)
- `dotfiles_list_configs` — List dotfiles config directories with symlink health and format
- `dotfiles_validate_config` — Validate TOML or JSON config syntax
- `dotfiles_reload_service` — Reload desktop service (hyprland, mako, eww, waybar, sway, tmux)
- `dotfiles_check_symlinks` — Check health of all expected dotfiles symlinks

### GitHub Org Lifecycle (12)
- `dotfiles_gh_list_personal_repos` — List personal repos with fork/visibility metadata
- `dotfiles_gh_list_org_repos` — List org repos with local clone sync status (supports language/archived/missing filters)
- `dotfiles_gh_transfer_repos` — Bulk transfer non-fork repos to org
- `dotfiles_gh_recreate_forks` — Squash forks into fresh org repos
- `dotfiles_gh_onboard_repos` — Fork public repos, squash history, clone locally (batch)
- `dotfiles_gh_local_sync_audit` — Audit local dirs vs org repos (orphaned/missing/mismatched)
- `dotfiles_gh_bulk_clone` — Clone all missing org repos locally
- `dotfiles_gh_pull_all` — Fetch/pull all local repos (detects dirty/detached)
- `dotfiles_gh_clean_stale` — Remove orphaned local clones (safety: checks uncommitted/unpushed)
- `dotfiles_gh_full_sync` — One-command fleet sync (pull + audit + clone missing)
- `dotfiles_gh_bulk_archive` — Batch archive repos
- `dotfiles_gh_bulk_settings` — Batch apply repo settings with before/after reporting

### Fleet Auditing & CI (4)
- `dotfiles_fleet_audit` — Per-repo language, Go version, CI status, test count, commit age, CLAUDE.md presence
- `dotfiles_health_check` — Org-wide health dashboard
- `dotfiles_dep_audit` — Go dependency version skew across fleet
- `dotfiles_workflow_sync` — Sync CI workflows from canonical sources

### Build & Sync (4)
- `dotfiles_pipeline_run` — Run build+test pipeline on a repo (Go/Node/Python)
- `dotfiles_bulk_pipeline` — Run pipeline across N repos with language filtering
- `dotfiles_go_sync` — Sync Go version across all repos
- `dotfiles_create_repo` — Scaffold new repo with standard files

### Desktop (4)
- `dotfiles_cascade_reload` — Ordered multi-service reload with health verification
- `dotfiles_rice_check` — Compositor/shader/wallpaper/service status + Snazzy palette compliance
- `dotfiles_eww_restart` — Kill and restart eww daemon with both bars
- `dotfiles_eww_status` — Show eww daemon health, windows, key variables

### Repository Onboarding (2)
- `dotfiles_onboard_repo` — Add standard files to any repo (.editorconfig, CI, LICENSE)
- `dotfiles_eww_get` — Query current eww variable value

## Key Patterns
- All batch tools use dry-run by default (`execute: true` for live mode)
- `bulk_settings` reports previous state before applying changes
- `clean_stale` checks for uncommitted/unpushed work before deletion
- `pull_all` detects dirty repos and detached HEAD, skips safely
- Composed "tool-of-tools" (full_sync, fleet_audit, cascade_reload, rice_check, bulk_pipeline) eliminate multi-step token waste
