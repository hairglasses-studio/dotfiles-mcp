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
```

Or use the pipeline script directly:

```bash
~/hairglasses-studio/dotfiles/scripts/hg-pipeline.sh
```

## Architecture

dotfiles-mcp is a single-binary MCP server with 87 tools registered across module files:

- `main.go` -- Server setup, tool registration
- `mod_hyprland.go` -- Hyprland compositor tools
- `mod_shader.go` -- Ghostty shader pipeline
- `mod_input.go` -- Input device management (juhradial-mx, makima, MIDI)
- `oss.go` -- Open-source readiness scoring

All tools are built on [mcpkit](https://github.com/hairglasses-studio/mcpkit) using `handler.TypedHandler` generics and `registry.ToolDefinition`.

## Making Changes

1. Create a branch: `git checkout -b feat/my-change`
2. Make your changes
3. Run the pipeline: `go build ./... && go vet ./... && go test ./... -count=1`
4. Commit with a descriptive message
5. Push and open a PR

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

## Questions?

Open an issue or tag `@hairglasses` in your PR.
