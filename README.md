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

MCP server for desktop environment management, semantic AT-SPI control, isolated desktop sessions, repo ops, GitHub org lifecycle, fleet auditing, open-source readiness scoring, kitty runtime control, and Arch Linux research-first package workflows.

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

| Profile | Behavior | Context Cost |
|---------|----------|-------------|
| `default` | Discovery tools loaded, rest deferred on demand | ~2K tokens |
| `desktop` | Desktop/operator subset loaded eagerly | ~8K tokens |
| `ops` | Operational subset (config, desktop, fleet) loaded eagerly | ~15K tokens |
| `full` | All tools loaded immediately | ~40K tokens |

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

# Run bounded host checks for Hyprland, Bluetooth, input, and GitHub CLI surfaces
make host-smoke

# Verify mirror docs + manifest parity for publish
make release-parity

# Summarize the committed public-surface delta vs the previous ref
make contract-diff

# Compare the committed mirror bundle against canonical dotfiles/mcp/dotfiles-mcp
make canonical-drift

# Report or diff the manifest-driven canonical carry-forward subset
make canonical-sync-report
make canonical-sync-diff

# Full publish guard: vet + test + contract + manifest parity
make publish-check
```

Generated artifacts live under `snapshots/contract/` and `.well-known/mcp.json`.

## Current Surface

The authoritative publish-mirror contract is generated into `snapshots/contract/` and `.well-known/mcp.json`. Treat those committed artifacts as the source of truth for the live tool, resource, and prompt counts. The current exported surface is:

- a canonical-superset mirror with zero missing canonical tools
- 370 tools across 37 modules
- 24 resources, including 5 resource templates
- 12 prompt entrypoints
- a small set of standalone-only Arch, Hyprland, and Kitty extensions
- discovery-first profiles: `default`, `desktop`, `ops`, `full`
- a manifest-driven canonical carry-forward path for the files that must stay byte-for-byte aligned with the canonical source

High-value additions in the current surface include:

- Canonical carry-forward fixes for notification history entry/clear tools, fleet baseline refresh parity, updated fleet audit semantics, and Kitty-aligned shader helpers
- Expanded Hyprland IPC coverage: active window/workspace, binds, devices, layers, layouts, config errors, cursor position, option reads, keyword writes, dispatch, notify, property control, and socket2 event capture/wait helpers
- Semantic desktop targeting: AT-SPI backed desktop snapshots, stable semantic refs, explicit action invocation, state-aware waits, and typed keyboard actions with OCR remaining available as a fallback path
- Desktop session control: live-session handles plus KWin virtual-session startup under `dbus-run-session`, session-local accessibility trees/find/click/action flows, D-Bus calls, screenshots, clipboard reads/writes, app launches, and per-session log access
- Kitty runtime control: status, tab/window inventory, config reload, font size, opacity, themes, layouts, titles, send-text, image overlays, and generic remote subcommands
- Arch Linux research-first operations: ArchWiki search/page reads, official package search/info, AUR search, PKGBUILD auditing, Arch news review, mirror status, update dry runs, pacman log reads, orphan detection, and file-owner inspection

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
| Semantic Desktop | `python3`, `pyatspi` |
| Session Tools | `dbus-run-session`, `wayland-info`, `grim`, `wl-copy`, `wl-paste`, `wtype`; `kwin_wayland` for virtual-session startup |
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
