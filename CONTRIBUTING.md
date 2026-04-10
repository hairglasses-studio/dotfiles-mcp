# Contributing

This project supports development with **Claude Code**, **Gemini CLI**, and **OpenAI Codex CLI**. Any provider can lead development.

## Development Setup

### 1. Clone and build

```bash
git clone https://github.com/hairglasses-studio/dotfiles-mcp
cd dotfiles-mcp
go build ./...
go vet ./...
go test ./... -count=1
```

### 2. Verify

```bash
make pipeline-check   # build + vet + test (via shared pipeline)
make publish-check    # mirror contract + manifest parity
make host-smoke       # host-dependent surfaces on the publish machine
make canonical-drift  # compare this mirror against dotfiles/mcp/dotfiles-mcp
```

Or use the pipeline script directly:

```bash
~/hairglasses-studio/dotfiles/scripts/hg-pipeline.sh
```

## Architecture

dotfiles-mcp is a single-binary MCP server with a discovery-first contract. The canonical source of truth lives at `dotfiles/mcp/dotfiles-mcp` inside the shared `hairglasses-studio/dotfiles` repo. This standalone repo remains a publish mirror for installation and discovery, with its exact public surface committed under `snapshots/contract/` and `.well-known/mcp.json`.

Current mirror counts should always come from the checked-in contract bundle, not hand-maintained prose.

Focus paths:

- `cmd/`
- `internal/dotfiles/`
- `internal/githubstars/`
- `internal/ops/`
- `internal/common/`

All tools are built on [mcpkit](https://github.com/hairglasses-studio/mcpkit) using `handler.TypedHandler` generics and `registry.ToolDefinition`.

## Making Changes

1. Create a branch: `git checkout -b feat/my-change`
2. Make your changes
3. Run the pipeline: `go build ./... && go vet ./... && go test ./... -count=1`
4. If the change affects the public surface, refresh the committed mirror artifacts: `make contract-snapshot`
5. Run mirror parity checks: `make publish-check`
6. Compare against the canonical source when applicable: `make canonical-drift`
7. Commit with a descriptive message
8. Push and open a PR

## Code Style

- **Go**: `gofmt` formatting, `go vet` clean
- Error handling: `handler.CodedErrorResult(handler.ErrInvalidParam, err)` -- never naked panics
- Thread safety: `sync.RWMutex` with `RLock` for reads, `Lock` for writes
- Param extraction: `handler.GetStringParam`, `handler.GetIntParam`, `handler.GetBoolParam`

Editor settings are in `.editorconfig` -- most editors pick this up automatically.

## Pre-commit Hooks

Install with:

```bash
make install-hooks
```

This runs vet + fast tests before each commit.

## CI

All PRs trigger CI automatically. The pipeline runs lint, test, and build checks.

For publish-mirror work, also keep these repo-owned guards green:

- `make contract-check` for snapshot and `.well-known/mcp.json` drift
- `make release-parity` for canonical-source and manifest parity
- `make contract-diff` for human-readable public surface deltas
- `make canonical-drift` to compare the committed mirror bundle against canonical `dotfiles/mcp/dotfiles-mcp`
- `make host-smoke` on a real workstation host before release-oriented pushes

## Questions?

Open an issue or tag `@hairglasses` in your PR.
