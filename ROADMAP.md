# Roadmap

## Current State

dotfiles-mcp is best treated as a standalone publish mirror for the canonical `dotfiles/mcp/dotfiles-mcp` module. The committed mirror contract currently exposes `278` tools across `32` modules, plus `8` resources and `4` prompts. Those counts are snapshot-backed by `.well-known/mcp.json` and `snapshots/contract/*`, and they may temporarily trail the canonical source between carry-forward tranches.

The repo now has explicit publish-mirror guards, but it should stay honest about the remaining job: reducing the still-real gap between this mirror and the richer canonical source in `dotfiles`.

## Planned

### Phase 1 — Canonical Carry-Forward
- Automate or semi-automate source carry-forward from `dotfiles/mcp/dotfiles-mcp` so the mirror stops lagging canonical resources, prompts, and newer workstation modules
- Reduce the current contract delta by porting the missing canonical workflow catalogs, prompts, and module registrations into the standalone layout
- Keep `make canonical-drift` green or at least bounded enough that drift is a conscious release decision rather than an accident

### Phase 2 — Publish Guard Hardening
- Keep active GitHub workflows for CI, release, server-card validation, and publish guard in this repo rather than assuming canonical-only checks are enough
- Publish contract diff summaries into release notes or docs whenever `snapshots/contract/` changes
- Run `host-smoke` on a logged-in self-hosted Hyprland runner before release-oriented pushes

### Phase 3 — Verification And Surface Quality
- Expand integration coverage for Bluetooth, juhradial-mx, and desktop-control paths that still rely on workstation state
- Add stronger tests around profile-specific eager/deferred loading behavior
- Improve contributor guidance so mirror maintainers know when to regenerate snapshots, compare against canonical, and update the publish workflows

## Future Considerations
- Full structural convergence with the canonical module, potentially by collapsing the mirror layout closer to the embedded source instead of preserving today’s dual architecture indefinitely
- Release automation that attaches contract drift reports and snapshot diffs as artifacts
- Removal of remaining recovery-only compatibility paths once the canonical source fully retires them

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
- `cmd/dotfiles-mcp-contract` now generates committed tool/resource/prompt snapshots plus `.well-known/mcp.json`.
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

<!-- whiteclaw-rollout:end -->
