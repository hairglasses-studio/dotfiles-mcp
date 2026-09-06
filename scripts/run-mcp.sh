#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_PATH="${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles-mcp/bin/dotfiles-mcp"

export GOWORK="${GOWORK:-off}"

needs_build=false
if [[ ! -x "$BIN_PATH" ]]; then
  needs_build=true
elif find "$ROOT/cmd/dotfiles-mcp" "$ROOT/internal" "$ROOT/go.mod" "$ROOT/go.sum" \
        -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) \
        -newer "$BIN_PATH" -print -quit 2>/dev/null | grep -q .; then
  needs_build=true
fi

cd "$ROOT"
if [[ "$needs_build" == true ]]; then
  mkdir -p "$(dirname "$BIN_PATH")"
  GOWORK="$GOWORK" go build -o "$BIN_PATH" ./cmd/dotfiles-mcp
fi

exec "$BIN_PATH" "$@"
