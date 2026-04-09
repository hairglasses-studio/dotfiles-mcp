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

<!-- whiteclaw-rollout:start -->
## Whiteclaw-Derived Overhaul (2026-04-08)

This tranche applies the highest-value whiteclaw findings that fit this repo's real surface: engineer briefs, bounded skills/runbooks, searchable provenance, scoped MCP packaging, and explicit verification ladders.

### Strategic Focus
- Treat this repo as a public mirror of the canonical `dotfiles/mcp/dotfiles-mcp` source, with extra attention on surface drift and host-dependent verification.
- The whiteclaw backport should strengthen contract snapshots and smoke tests for the shipped module surfaces, not invent new local autonomy layers.
- Keep the roadmap explicit about mirror ownership, host dependencies, and publish parity.

### Recommended Work
- [ ] [Mirror contract] Keep the canonical-source mapping to `dotfiles/mcp/dotfiles-mcp` explicit in roadmap, README, and release notes.
- [ ] [Contract snapshots] Snapshot the exported tool/resource/prompt contracts for the major module groups so mirror drift is visible.
- [ ] [Host smoke tests] Add smoke tests for the host-dependent Hyprland, Bluetooth, input, and org-lifecycle surfaces before publish.
- [ ] [Release parity] Verify that release tags and manifests still reflect the canonical source-of-truth module.

### Rationale Snapshot
- Tier / lifecycle: `standalone` / `publish-mirror`
- Language profile: `Go`
- Visibility / sensitivity: `PUBLIC` / `public`
- Surface baseline: AGENTS=yes, skills=yes, codex=yes, mcp_manifest=missing, ralph=no, roadmap=yes
- Whiteclaw transfers in scope: mirror contract, contract snapshots, host-dependent smoke tests, release parity
- Live repo notes: AGENTS, skills, Codex config, 1 workflow(s)

<!-- whiteclaw-rollout:end -->
