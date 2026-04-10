# Roadmap

## Current State

dotfiles-mcp is now a discovery-first workstation MCP surface with committed contract artifacts. The canonical snapshot currently exposes `382` tools across `37` registered modules, plus `24` resources and `12` prompts. Public release metadata is regenerated into `.well-known/mcp.json`, and the JSON bundle in `snapshots/contract/` is treated as the checked-in contract for publish parity.

The server remains stdio-first, built on mcpkit, and defaults to deferred loading outside the discovery surface. Batch workflows still default to dry-run where live mutation would be risky.

## Planned

### Phase 1 — Publish And Mirror Hygiene
- Automate canonical-to-standalone carry-forward for the embedded `dotfiles/mcp/dotfiles-mcp` module so publish-mirror updates stop depending on manual drift cleanup
- Publish contract diff summaries into release notes and docs whenever `snapshots/contract/` changes
- Promote the committed `publish-check`, `host-smoke`, and release-parity workflows into the standalone mirror so GitHub releases enforce the same gate set

### Phase 2 — Surface Quality And Verification
- Add targeted integration tests for Bluetooth, juhradial-mx, and desktop-control readiness paths that currently depend on workstation state
- Expand resource and prompt coverage tests so the contract bundle fails loudly when workflow catalogs drift
- Add higher-signal validation for profile-specific eager/deferred loading behavior, especially `desktop` and `ops`
- Add AT-SPI host smoke and richer semantic desktop fixtures so `desktop_snapshot`, `desktop_find`, `desktop_focus`, `desktop_read_value`, and `desktop_set_text` can be exercised outside the main workstation
- Add stronger session-fixture coverage for live handles and KWin virtual-session startup, semantic inspection/action/value flows, screenshots, and clipboard flows

### Phase 3 — Product Expansion
- `dotfiles_pipeline_status` — aggregate CI status across all repos in one view
- `dotfiles_changelog_gen` — generate changelogs from conventional commits
- `dotfiles_release` — orchestrate go-releaser across repos
- Broader semantic desktop compatibility for Electron/Chromium-heavy apps, richer semantic form editing, and more resilient KWin virtual-session introspection
- Deeper workspace scene tooling around layout capture, window restoration, and publishable workstation diagnostics

## Future Considerations
- Remove the remaining Solaar recovery-only bridge once juhradial can replay full MX wheel state durably
- Run host smoke on a logged-in self-hosted Hyprland publish runner rather than treating local workstation smoke as a partial proxy
- Consider richer runtime grouping metadata in contract snapshots so category counts stop collapsing into the current unassigned bucket
- Explore release-note automation that hyperlinks tool/resource/prompt deltas directly from `snapshots/contract`
