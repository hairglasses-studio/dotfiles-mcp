# dotfiles-mcp

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![MCP](https://img.shields.io/badge/MCP-2025--11--25-blue)](https://modelcontextprotocol.io/specification/2025-11-25)

MCP server for desktop environment management, semantic desktop control, session tooling, repo ops, GitHub org lifecycle, fleet auditing, kitty runtime control, Arch Linux workflows, and open-source readiness scoring.

Canonical development lives in [`hairglasses-studio/dotfiles`](https://github.com/hairglasses-studio/dotfiles/tree/main/mcp/dotfiles-mcp) under `dotfiles/mcp/dotfiles-mcp`. The standalone [`dotfiles-mcp`](https://github.com/hairglasses-studio/dotfiles-mcp) repo is a publish mirror kept in parity for installation and discovery.

## Install

```bash
go install github.com/hairglasses-studio/dotfiles-mcp@latest
```

When developing from the monorepo mirror under `dotfiles/mcp/dotfiles-mcp`, use `GOWORK=off` for direct module commands so the shared `mcp/go.work` does not inherit sibling repo-local replaces from other MCP modules.

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
- `dotfiles_workstation_diagnostics`

Use `DOTFILES_MCP_PROFILE=desktop` for workstation desktop control, or `DOTFILES_MCP_PROFILE=full` if you explicitly want the full catalog treated as eager.

The server also exposes read-first workflow resources and prompt entrypoints for the common operator loops:

- Workflow catalog: `dotfiles://catalog/workflows`
- Skill catalog: `dotfiles://catalog/skills`
- Workflow priorities: `dotfiles://catalog/priorities`
- Prompt workflows: desktop control, fleet audit, config repair, desktop triage, workstation diagnosis, repo validation, repo hygiene, repo onboarding, and session recovery

The canonical module now commits public contract snapshots under [`snapshots/contract`](./snapshots/contract) and regenerates the public server card at [`.well-known/mcp.json`](./.well-known/mcp.json). Current canonical snapshot counts:

- `397` tools
- `37` registered modules
- `24` resources
- `12` prompts

## Quick Start

After installing, try the discovery tools to explore what's available:

```bash
GOWORK=off go build ./...
GOWORK=off go test ./... -count=1

# Search tools by keyword
claude mcp call dotfiles dotfiles_tool_search '{"query": "bluetooth"}'

# Get full tool catalog
claude mcp call dotfiles dotfiles_tool_catalog '{}'

# Read the canonical workflow catalog
claude mcp read dotfiles dotfiles://catalog/workflows

# Check desktop runtime readiness
claude mcp call dotfiles dotfiles_desktop_status '{}'

# Capture a publishable workstation diagnostics snapshot
claude mcp call dotfiles dotfiles_workstation_diagnostics '{"symptom":"desktop bar missing after login"}'

# Check desktop rice health
claude mcp call dotfiles dotfiles_rice_check '{}'
```

GitHub Stars workflow examples:

```bash
# Summarize managed GitHub Stars coverage
claude mcp call dotfiles dotfiles_gh_stars_summary '{"managed_list_prefix":"MCP / "}'

# List current GitHub star folders with items
claude mcp call dotfiles dotfiles_gh_star_lists_list '{"include_items":true}'

# Audit and bootstrap the MCP stars taxonomy
bash ./scripts/hg-github-stars.sh audit-taxonomy --managed-prefix 'MCP / ' --bootstrap-defaults
bash ./scripts/hg-github-stars.sh bootstrap --install-codex-mcp --execute
```

## Loading Profiles

Control how many tools load at startup via `DOTFILES_MCP_PROFILE`:

| Profile | Behavior | Approx. Prompt Footprint |
|---------|----------|--------------------------|
| `default` | Discovery tools loaded, rest deferred on demand | ~2-4K tokens |
| `desktop` | Desktop-control subset eager: Hyprland, screenshot/OCR, input, shader, clipboard, notifications, and rice status | ~12-18K tokens |
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

## Contract Snapshots

The canonical source treats the committed snapshot bundle as the public surface contract:

- `make contract-snapshot` regenerates [`.well-known/mcp.json`](./.well-known/mcp.json) and the JSON bundle in [`snapshots/contract`](./snapshots/contract)
- `make contract-check` verifies the checked-in artifacts match the live registry
- `make contract-diff` summarizes surface deltas against a base ref
- `make publish-check` runs vet, tests, contract validation, and release-parity checks together
- `make host-smoke` checks Hyprland, semantic AT-SPI, session clipboard/screenshot/runtime prerequisites, device basics, and GitHub CLI availability; `make host-smoke-strict` turns missing and skipped checks into failures for prepared runners
- The publish-guard and release workflows emit `make contract-diff` summaries into CI step summaries and uploaded artifacts; the release workflow also appends the diff into the GitHub release body

Exact per-tool counts should come from the snapshot bundle rather than prose. The current surface domains include:

| Domain | Description |
|--------|-------------|
| Discovery | Search, schema, catalog, stats, and health entrypoints for the deferred surface |
| Desktop Control | Hyprland, semantic AT-SPI targeting with refs/actions/multi-match queries plus window focus and value read-write helpers, kitty tab/window launch-focus-resize-text helpers, session-local accessibility and D-Bus control, screenshot/OCR, clipboard, notifications, shaders, audio, and Wayland input workflows |
| Workstation Ops | Systemd, process, tmux, sandbox, fleet audit, repo hygiene, and SDLC loops |
| GitHub Workflows | Org lifecycle, GitHub Stars, and repo sync helpers |
| Input & Devices | Bluetooth, juhradial-mx, controller mapping, MIDI, and mouse/controller diagnostics |
| Research & Recovery | Claude session recovery, Arch Linux knowledge, prompt registry, roadmap, and cross-repo forensic tooling |

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
| Screenshot / OCR | `wayshot`, `tesseract`, `magick` |
| Bluetooth | `bluetoothctl` |
| Shaders | `glslangValidator` (optional, for compile-testing) |
| Input / Mouse | `juhradial-mx`, `ydotool`, `makima` |
| Desktop | `eww`, `makoctl`, `pgrep` |
| GitHub Org | `gh` (GitHub CLI) |
| MIDI | ALSA (`aconnect`, `amidi`) |

## See Also

- [mcpkit](https://github.com/hairglasses-studio/mcpkit) -- production-grade Go MCP server toolkit
- [systemd-mcp](https://github.com/hairglasses-studio/systemd-mcp) -- systemd service management
- [tmux-mcp](https://github.com/hairglasses-studio/tmux-mcp) -- tmux multiplexer management
- [process-mcp](https://github.com/hairglasses-studio/process-mcp) -- process debugging with composed investigation

## License

MIT -- see [LICENSE](LICENSE).
