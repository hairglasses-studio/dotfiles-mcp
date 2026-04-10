# Roadmap

## Current State

dotfiles-mcp is now best treated as a standalone publish mirror for the canonical `dotfiles/mcp/dotfiles-mcp` module. The current `main` target surface exposes 340 tools across 35 modules, plus 24 resources and 12 prompts, when published from the canonical source. The mirror keeps discovery-first loading, workflow resources, prompt entrypoints, and a juhradial-first MX input contract while staying explicit about its relationship to the canonical source of record.

All modules functional and tested. MIT licensed, README and CLAUDE.md in place. Batch tools default to dry-run mode.

## Planned

### Phase 1 — Test Coverage & Documentation
- Increase unit test coverage for GitHub org lifecycle tools (currently lowest coverage)
- Add integration tests for Bluetooth and input device modules
- Per-module documentation with usage examples
- Improve error messages for missing system dependencies (ydotool, wtype, bluetoothctl)

### Phase 2 — Desktop Automation
- Semantic desktop control — AT-SPI backed desktop snapshot/find/click/type/key flows with OCR fallback
- Desktop session tooling — isolated session bootstrap, screenshots, focus/window inventory, and clipboard access
- KWin virtual-session support where the host provides `kwin_wayland --virtual`
- Additional Hyprland eventing and compositor-adjacent helpers as new stable IPC hooks appear upstream

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
- [x] [Mirror contract] Keep the canonical-source mapping to `dotfiles/mcp/dotfiles-mcp` explicit in roadmap, README, and release notes.
- [x] [Count reconciliation] Make the standalone mirror state explicit whenever its public docs lag the canonical tool count or module inventory.
- [x] [Contract snapshots] Snapshot the exported tool/resource/prompt contracts for the major module groups so mirror drift is visible.
- [x] [Host smoke tests] Add smoke tests for the host-dependent Hyprland, Bluetooth, input, and org-lifecycle surfaces before publish.
- [x] [Release parity] Verify that release tags and manifests still reflect the canonical source-of-truth module.

### Landed Safeguards (2026-04-09)
- `cmd/dotfiles-mcp-contract` now generates committed tool/resource/prompt snapshots under `snapshots/contract/` plus `.well-known/mcp.json`.
- `scripts/host-smoke.sh` provides bounded workstation checks for Hyprland, Bluetooth, input, and GitHub CLI dependencies.
- `scripts/release-parity.sh` verifies canonical-source references and manifest parity before publish.
- `make contract-snapshot`, `make contract-check`, `make host-smoke`, `make release-parity`, and `make publish-check` make the mirror guard path explicit.

### Rationale Snapshot
- Tier / lifecycle: `standalone` / `publish-mirror`
- Language profile: `Go`
- Visibility / sensitivity: `PUBLIC` / `public`
- Surface baseline: AGENTS=yes, skills=yes, codex=yes, mcp_manifest=missing, ralph=no, roadmap=yes
- Whiteclaw transfers in scope: mirror contract, contract snapshots, host-dependent smoke tests, release parity
- Live repo notes: AGENTS, skills, Codex config, 1 workflow(s)

### Capability Expansion Tranche (2026-04-10)
- [x] Expanded Hyprland IPC tooling with active-window/workspace, binds, devices, layers, layouts, config diagnostics, property control, and socket2 event helpers
- [x] Added Kitty runtime control for tab/window inventory, config reloads, theme/layout/title changes, send-text, image overlays, and generic remote commands
- [x] Added Arch Linux research-first surfaces for ArchWiki, official repos, AUR, PKGBUILD auditing, Arch news review, mirror status, update dry runs, pacman logs, orphan detection, and file ownership
- [x] Refreshed `snapshots/contract/` and `.well-known/mcp.json` to publish the expanded tool/resource/prompt surface

<!-- whiteclaw-rollout:end -->
