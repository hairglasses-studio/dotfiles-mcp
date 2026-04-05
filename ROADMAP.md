# Roadmap

## Current State

dotfiles-mcp is a consolidated desktop environment MCP server with 86 tools across 10 modules: config management, GitHub org lifecycle, fleet auditing, build pipelines, Hyprland desktop, shader pipeline, Bluetooth, input devices (Logitech/gamepad/MIDI), composed workflows, and open-source readiness scoring. Discovery-first design with deferred tool loading. Built on mcpkit with stdio transport.

All modules functional and tested. MIT licensed, README and CLAUDE.md in place. Batch tools default to dry-run mode.

## Planned

### Phase 1 — Test Coverage & Documentation
- Increase unit test coverage for GitHub org lifecycle tools (currently lowest coverage)
- Add integration tests for Bluetooth and input device modules
- Per-module documentation with usage examples
- Improve error messages for missing system dependencies (ydotool, wtype, bluetoothctl)

### Phase 2 — Desktop Automation
- `hypr_record_screen` — screen recording via wf-recorder with configurable region
- `hypr_layout_save` / `hypr_layout_restore` — snapshot and restore window arrangements
- Eww widget data tools — expose eww variable state and widget tree to MCP
- Notification management tools (mako: list, dismiss, toggle DND)

### Phase 3 — Fleet & CI Improvements
- `dotfiles_pipeline_status` — aggregate CI status across all repos in one view
- `dotfiles_changelog_gen` — generate changelogs from conventional commits
- `dotfiles_release` — orchestrate go-releaser across repos
- Webhook support for fleet audit notifications

## Future Considerations
- Wayland-native screenshot/recording (replace ydotool with libinput where possible)
- Audio device management tools (PipeWire/PulseAudio)
- Multi-monitor layout presets (save/restore per-workspace monitor configurations)
- Plugin architecture for community-contributed modules
