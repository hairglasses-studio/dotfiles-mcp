# Changelog

All notable changes to dotfiles-mcp will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- GitHub Stars workflow module for listing stars, managing GitHub star folders, auditing taxonomy drift, syncing list membership, and installing the global Codex MCP entries
- `cmd/github-starsctl` CLI wrapper for the GitHub Stars workflow
- Active standalone CI, release, publish-guard, and server-card validation workflows
- `make contract-diff` and `make canonical-drift` plus supporting scripts for mirror surface and canonical drift reporting
- Expanded standalone-only Hyprland IPC, Kitty runtime, and Arch Linux research-first surfaces
- Contract-diff publication in GitHub workflow step summaries for publish and release runs

### Changed

- Carried forward the remaining canonical notification history, fleet baseline refresh, fleet audit, and shader workflow definitions so canonical drift is now limited to intentional mirror-only extensions
- Removed the stale hard-coded tool-count assertion from CI and now treat the committed contract bundle plus `make publish-check` as the authoritative public-surface guard

## [v0.1.0] - 2026-04-03

### Added

- Initial public release with 86 tools across 10 modules
- Config management: symlink health, config validation, service reloads
- GitHub org lifecycle: transfers, fork squashing, bulk clone/pull/archive, fleet sync
- Fleet auditing: per-repo health dashboard, dependency skew, workflow sync
- Build pipeline: multi-language build+test, Go version sync, repo scaffolding
- Hyprland desktop: window/workspace management, screenshots, monitor config, input simulation
- Desktop services: cascade reload, rice check, eww bar management
- Shader pipeline: GLSL shader lifecycle for Ghostty and wallpapers
- Bluetooth: BLE-safe pairing, connect/disconnect, battery, trust
- Input devices: juhradial-mx config and battery, gamepad profiles (makima)
- MIDI: USB controller detection and mapping config
- Composed workflows: bt_discover_and_connect, input_auto_setup_controller
- Open-source readiness scoring (0-100) across 8 categories
- Built on mcpkit v0.1.0

[v0.1.0]: https://github.com/hairglasses-studio/dotfiles-mcp/releases/tag/v0.1.0
