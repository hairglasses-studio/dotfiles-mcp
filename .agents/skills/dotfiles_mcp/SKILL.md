---
name: dotfiles_mcp
description: 'Thin MCP server workflow for dotfiles-mcp. Use this when changing the dedicated workstation and fleet-operation server in this repo; prefer the canonical dotfiles and fleet skills for broader workstation changes outside this codebase.'
---

# dotfiles-mcp

This repo is the focused MCP surface for workstation and fleet operations. Keep it thin and interoperable with the broader dotfiles control plane instead of duplicating repo policy or operator guidance here.

Focus paths:
- `cmd/`
- `internal/`
- `AGENTS.md`
- `README.md`

Prefer additive tool or contract improvements over bespoke one-off scripts.
