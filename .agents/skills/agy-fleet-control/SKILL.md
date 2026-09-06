---
name: agy-fleet-control
description: Standardized AGY fleet management skill for managing multi-agent subagents, sync engine, and control plane workflows.
---

# AGY Fleet Control Skill

Use this skill when managing, auditing, or expanding AGY surfaces across the hairglasses-studio fleet.

## Capabilities

1. **Bidirectional Sync Engine**:
   - Preview changes across fleet nodes: `bin/hg-agy-sync --dry-run`
   - Apply synchronized state across nodes: `bin/hg-agy-sync --apply`
   - Resolve conflicts with `--resolve-local` or `--resolve-remote`

2. **Teamwork Multi-Agent Orchestration**:
   - Delegate specialized subagent tasks using `teamwork_preview` protocol.
   - Enforce non-overlapping file write ownership per worker.
   - Maintain artifact lifecycle tracking via `BRIEFING.md`, `progress.md`, and `handoff.md`.

3. **Convention & Ownership Auditing**:
   - Run convention ownership checks: `bin/convention-ownership-check`
   - Execute hermetic verification suites: `make test`
   - Enforce secret exclusion policies (`.env`, certificates, tokens).

## Fleet Architecture Rules

- Maintain provider-neutral guidelines in `AGENTS.md`.
- Keep AGY-specific configurations, skills, rules, and hooks in `.gemini/` and `.agents/`.
- Ensure all custom tools in `bin/` provide self-test capabilities (`*-self-test`).
