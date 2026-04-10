# Roadmap

## Current State

dotfiles-mcp is now a discovery-first workstation MCP surface with committed contract artifacts. The canonical snapshot currently exposes `397` tools across `37` registered modules, plus `24` resources and `12` prompts. Public release metadata is regenerated into `.well-known/mcp.json`, and the JSON bundle in `snapshots/contract/` is treated as the checked-in contract for publish parity.

The server remains stdio-first, built on mcpkit, and defaults to deferred loading outside the discovery surface. Batch workflows still default to dry-run where live mutation would be risky.

Publish guard and release automation now emit contract-diff summaries into CI artifacts and step summaries, and the release flow appends the same diff into the GitHub release body so public surface changes stay visible at publish time.

Host smoke now covers Hyprland, semantic AT-SPI import readiness, session clipboard and screenshot prerequisites, and strict skip handling so prepared runners can turn missing runtime context into hard failures when needed.

Fixture-driven verification now covers `dotfiles_desktop_status`, Bluetooth and juhradial battery/service flows, semantic/session host preflights, and higher-signal `desktop`/`ops` defer-boundary checks without depending on a live workstation session.

Canonical-to-standalone carry-forward for the embedded `dotfiles/mcp/dotfiles-mcp` module now has a dedicated projection helper with apply mode, editable-worktree targeting for bare mirrors, imported internal-package dependency checks, and wrapper integration in `sync-standalone-mcp-repos.sh` so required mirror drift can be cleared mechanically instead of by hand.

The workstation diagnosis workflow now has a concrete front door in `dotfiles_workstation_diagnostics`, which composes machine health, desktop readiness, rice status, recommendations, and a publishable markdown report into a single read-first snapshot.

## Planned

### Phase 2 — Product Expansion
- Broader semantic desktop compatibility for Electron/Chromium-heavy apps, richer semantic form editing, and more resilient KWin virtual-session introspection
- Deeper workspace scene tooling around layout capture and window restoration

## Future Considerations
- Remove the remaining Solaar recovery-only bridge once juhradial can replay full MX wheel state durably
- Run host smoke on a logged-in self-hosted Hyprland publish runner rather than treating local workstation smoke as a partial proxy
- Consider richer runtime grouping metadata in contract snapshots so category counts stop collapsing into the current unassigned bucket
- Explore release-note automation that hyperlinks tool/resource/prompt deltas directly from `snapshots/contract`
