> **Consolidated** — This repo has been merged into [hairglasses-studio/dotfiles](https://github.com/hairglasses-studio/dotfiles) at `mcp/dotfiles-mcp/`. The dotfiles version has 99 tools (vs 82 here) and is actively maintained. For new development, use the consolidated version.

# dotfiles-mcp

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![MCP](https://img.shields.io/badge/MCP-2025--11--25-blue)](https://modelcontextprotocol.io/specification/2025-11-25)

MCP server for desktop environment management -- Hyprland, Ghostty shaders, Bluetooth, MIDI, input devices, GitHub org lifecycle, fleet auditing, and open-source readiness scoring. 90 tools across 15 modules.

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

Use `DOTFILES_MCP_PROFILE=full` if you explicitly want the full catalog treated as eager.

## Quick Start

After installing, try the discovery tools to explore what's available:

```bash
# Search tools by keyword
claude mcp call dotfiles dotfiles_tool_search '{"query": "bluetooth"}'

# Get full tool catalog
claude mcp call dotfiles dotfiles_tool_catalog '{}'

# Check desktop rice health
claude mcp call dotfiles dotfiles_rice_check '{}'
```

## Loading Profiles

Control how many tools load at startup via `DOTFILES_MCP_PROFILE`:

| Profile | Behavior | Context Cost |
|---------|----------|-------------|
| `default` | Discovery tools loaded, rest deferred on demand | ~2K tokens |
| `ops` | Operational subset (config, desktop, fleet) loaded eagerly | ~15K tokens |
| `full` | All 90 tools loaded immediately | ~40K tokens |

Set in your MCP config:

```json
{
  "mcpServers": {
    "dotfiles": {
      "command": "dotfiles-mcp",
      "env": { "DOTFILES_MCP_PROFILE": "ops" }
    }
  }
}
```

## Tool Categories

| Category | Tools | Description |
|----------|------:|-------------|
| Config Management | 4 | Dotfiles symlink health, config validation, service reloads |
| GitHub Org Lifecycle | 12 | Repo transfers, fork squashing, bulk clone/pull/archive, fleet sync |
| Fleet Auditing & CI | 4 | Per-repo health dashboard, dependency skew, workflow sync |
| Build & Sync | 5 | Multi-language build pipeline, Go version sync, repo scaffolding |
| Hyprland Desktop | 12 | Window/workspace management, screenshots, monitor config, input simulation |
| Desktop Services | 6 | Cascade reload, rice check, eww bar management |
| Shader Pipeline | 13 | GLSL shader lifecycle for Ghostty and wallpapers -- list, set, cycle, test, build |
| Bluetooth | 9 | Device discovery, pairing (BLE-safe), connect/disconnect, battery, trust |
| Input Devices | 13 | Logitech mouse config, gamepad profiles (makima), Solaar settings |
| MIDI | 4 | USB MIDI controller detection and mapping config |
| Composed Workflows | 2 | Multi-step automations: BT discover-and-connect, controller auto-setup |
| Open-Source Readiness | 2 | Score repos 0-100 across 8 categories with actionable suggestions |

## Key Patterns

- All batch tools use **dry-run by default** -- pass `execute: true` for live mode
- Composed "tool-of-tools" (`full_sync`, `fleet_audit`, `cascade_reload`, `rice_check`, `bulk_pipeline`) eliminate multi-step token waste
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
| Input / Mouse | `logiops` (logid), `solaar`, `makima` |
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
