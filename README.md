> **Consolidated** — This repo has been merged into [hairglasses-studio/dotfiles](https://github.com/hairglasses-studio/dotfiles) at `mcp/dotfiles-mcp/`. The dotfiles version remains the canonical source of truth and is actively maintained. For new development, use the consolidated version.

[![Go Reference](https://pkg.go.dev/badge/github.com/hairglasses-studio/dotfiles-mcp.svg)](https://pkg.go.dev/github.com/hairglasses-studio/dotfiles-mcp)
[![Go Report Card](https://goreportcard.com/badge/github.com/hairglasses-studio/dotfiles-mcp)](https://goreportcard.com/report/github.com/hairglasses-studio/dotfiles-mcp)
[![CI](https://github.com/hairglasses-studio/dotfiles-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/hairglasses-studio/dotfiles-mcp/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Glama](https://glama.ai/mcp/servers/hairglasses-studio/dotfiles-mcp/badges/score.svg)](https://glama.ai/mcp/servers/hairglasses-studio/dotfiles-mcp)

# dotfiles-mcp

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![MCP](https://img.shields.io/badge/MCP-2025--11--25-blue)](https://modelcontextprotocol.io/specification/2025-11-25)

MCP server for desktop environment management, repo ops, GitHub org lifecycle, fleet auditing, and open-source readiness scoring.

Canonical development lives in [`hairglasses-studio/dotfiles`](https://github.com/hairglasses-studio/dotfiles/tree/main/mcp/dotfiles-mcp) under `dotfiles/mcp/dotfiles-mcp`. The standalone [`dotfiles-mcp`](https://github.com/hairglasses-studio/dotfiles-mcp) repo is a publish mirror kept in parity for installation and discovery.

## Install

```bash
go install github.com/hairglasses-studio/dotfiles-mcp@latest
```

## Configure

Add to your MCP client config (e.g. Claude Code `.mcp.json`):

```json
{
  "mcpServers": {
    "dotfiles": {
      "command": "dotfiles-mcp"
    }
  }
}
```

By default, `dotfiles-mcp` marks its non-discovery tools as `defer_loading` and exposes discovery helpers first:

- `dotfiles_tool_search`
- `dotfiles_tool_schema`
- `dotfiles_tool_catalog`
- `dotfiles_tool_stats`
- `dotfiles_server_health`

It also ships discovery-adjacent resources and prompts for workflow entrypoints:

- `dotfiles://catalog/workflows`
- `dotfiles://catalog/prompts`
- prompt entrypoints for fleet, repo, and desktop flows

Use `DOTFILES_MCP_PROFILE=desktop` for a practical workstation-first eager set, or `full` if you explicitly want the full catalog treated as eager.

The committed mirror contract currently exposes:

- `278` tools
- `32` registered modules
- `8` resources
- `4` prompts

Those counts come from the checked-in bundle under [`snapshots/contract`](./snapshots/contract). The canonical source in `dotfiles/mcp/dotfiles-mcp` may lead this mirror between carry-forward tranches, so the mirror tracks its own published contract explicitly instead of implying perfect real-time parity.

## Quick Start

After installing, try the discovery tools to explore what's available:

```bash
# Search tools by keyword
claude mcp call dotfiles dotfiles_tool_search '{"query": "bluetooth"}'

# Get full tool catalog
claude mcp call dotfiles dotfiles_tool_catalog '{}'

# Browse workflow resources
claude mcp resources read dotfiles dotfiles://catalog/workflows

# Check desktop rice health
claude mcp call dotfiles dotfiles_rice_check '{}'

# Inspect desktop service health
claude mcp call dotfiles dotfiles_server_health '{}'
```

GitHub Stars workflow examples:

```bash
# Summarize managed GitHub Stars coverage
claude mcp call dotfiles dotfiles_gh_stars_summary '{"managed_list_prefix":"MCP / "}'

# List current GitHub star folders with items
claude mcp call dotfiles dotfiles_gh_star_lists_list '{"include_items":true}'

# Bootstrap Codex MCP config from managed GitHub Stars lists
~/hairglasses-studio/dotfiles/scripts/hg-gh-stars-codex-mcp-bootstrap.sh --dry-run

# Bootstrap Claude MCP config from managed GitHub Stars lists
~/hairglasses-studio/dotfiles/scripts/hg-gh-stars-claude-mcp-bootstrap.sh --dry-run
```

## Loading Profiles

Control how many tools load at startup via `DOTFILES_MCP_PROFILE`:

| Profile | Behavior | Approx. Prompt Footprint |
|---------|----------|--------------------------|
| `default` | Discovery tools loaded, rest deferred on demand | ~2-4K tokens |
| `desktop` | Desktop/operator subset loaded eagerly | ~8-12K tokens |
| `ops` | Operational subset (config, desktop, fleet) loaded eagerly | ~15-22K tokens |
| `full` | All tools loaded immediately | ~40K+ tokens |

Set in your MCP config:

```json
{
  "mcpServers": {
    "dotfiles": {
      "command": "dotfiles-mcp",
      "env": { "DOTFILES_MCP_PROFILE": "desktop" }
    }
  }
}
```

## Mirror Guards

The standalone repo now keeps its publish-mirror contract explicit and checkable:

```bash
# Regenerate committed contract snapshots and .well-known manifest
make contract-snapshot

# Fail if snapshots or .well-known/mcp.json drift from the live registry
make contract-check

# Summarize surface deltas against a base ref
make contract-diff

# Compare the mirror snapshot against the canonical dotfiles source
make canonical-drift

# Run bounded host checks for Hyprland, Bluetooth, input, and GitHub CLI surfaces
make host-smoke

# Verify mirror docs + manifest parity for publish
make release-parity

# Full publish guard: vet + test + contract + manifest parity
make publish-check
```

Generated artifacts live under `snapshots/contract/` and `.well-known/mcp.json`.

## Surface Domains

Exact counts should come from the committed contract bundle, not prose. The mirror currently publishes these major domains:

| Domain | Description |
|--------|-------------|
| Discovery | Search, schema, catalog, stats, and health entrypoints for the deferred surface |
| Desktop Control | Hyprland, screenshot/OCR, clipboard, notifications, shaders, and Wayland input workflows |
| Workstation Ops | Systemd, process, tmux, sandbox, fleet audit, repo hygiene, and SDLC loops |
| GitHub Workflows | Org lifecycle, GitHub Stars, and repo sync helpers |
| Input & Devices | Bluetooth, juhradial-mx, controller mapping, MIDI, and mouse/controller diagnostics |
| Research & Recovery | Claude session recovery, prompt registry, roadmap, and cross-repo operator tooling |

## Key Patterns

- All batch tools use **dry-run by default** -- pass `execute: true` for live mode
- Composed "tool-of-tools" (`full_sync`, `fleet_audit`, `cascade_reload`, `rice_check`, `bulk_pipeline`) eliminate multi-step token waste
- GitHub Stars helpers prefer `GITHUB_PAT` from `~/.env`, then existing shell env, then `gh auth token`
- `clean_stale` checks for uncommitted/unpushed work before deletion
- `pull_all` detects dirty repos and detached HEAD, skips safely

## Requirements

- Go 1.26+
- Linux (Hyprland/Wayland for desktop tools)

Runtime tools vary by category. Missing tools are detected gracefully -- unused categories won't error:

| Category | Runtime Dependencies |
|----------|---------------------|
| Hyprland | `hyprctl`, `ydotool`, `wtype` |
| Bluetooth | `bluetoothctl` |
| Shaders | `glslangValidator` (optional, for compile-testing) |
| Input / Mouse | `juhradial-mx`, `ydotool`, `makima` |
| Desktop | `eww`, `makoctl`, `pgrep` |
| Screenshot / OCR | `grim`, `slurp`, `tesseract` (optional, for capture/vision flows) |
| GitHub Org | `gh` (GitHub CLI) |
| MIDI | ALSA (`aconnect`, `amidi`) |

## See Also

- [mcpkit](https://github.com/hairglasses-studio/mcpkit) -- production-grade Go MCP server toolkit
- [systemd-mcp](https://github.com/hairglasses-studio/systemd-mcp) -- systemd service management
- [tmux-mcp](https://github.com/hairglasses-studio/tmux-mcp) -- tmux multiplexer management
- [process-mcp](https://github.com/hairglasses-studio/process-mcp) -- process debugging with composed investigation

## License

MIT -- see [LICENSE](LICENSE).
