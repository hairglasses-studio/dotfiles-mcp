#!/usr/bin/env bash
# Shared helper: resolve the (canonical, mirror) directory pair for
# dotfiles-mcp drift/sync tooling.
#
#   canonical = dotfiles/mcp/dotfiles-mcp inside the ralphglasses monorepo
#               (source of truth)
#   mirror    = oss/dotfiles-mcp, the ralphglasses git submodule pinned to
#               the standalone published github.com/hairglasses-studio/dotfiles-mcp
#
# Both copies of this file (canonical + hand-copied mirror) source this
# helper, so it must work when invoked from either side. It never fails
# the caller's script for a missing sibling — a standalone clone of the
# published dotfiles-mcp repo legitimately has no ralphglasses superproject
# next to it, and that is an expected, non-error condition for these tools.
#
# Sets: CANONICAL_DIR, MIRROR_DIR (empty string if not resolvable).
# Respects env overrides: CANONICAL_DOTFILES_MCP_DIR, MIRROR_DOTFILES_MCP_DIR.

resolve_canonical_pair() {
  local self_dir
  self_dir="$(cd "$(dirname "${BASH_SOURCE[1]:-${BASH_SOURCE[0]}}")/.." && pwd)"

  CANONICAL_DIR="${CANONICAL_DOTFILES_MCP_DIR:-}"
  MIRROR_DIR="${MIRROR_DOTFILES_MCP_DIR:-}"

  if [[ -n "$CANONICAL_DIR" && -n "$MIRROR_DIR" ]]; then
    return 0
  fi

  case "$self_dir" in
    */dotfiles/mcp/dotfiles-mcp)
      # We ARE the canonical copy.
      [[ -z "$CANONICAL_DIR" ]] && CANONICAL_DIR="$self_dir"
      if [[ -z "$MIRROR_DIR" ]]; then
        local candidate
        candidate="$(cd "$self_dir/../../.." && pwd)/oss/dotfiles-mcp"
        [[ -d "$candidate" ]] && MIRROR_DIR="$candidate"
      fi
      ;;
    */oss/dotfiles-mcp)
      # We ARE the mirror copy (git submodule checkout).
      [[ -z "$MIRROR_DIR" ]] && MIRROR_DIR="$self_dir"
      if [[ -z "$CANONICAL_DIR" ]]; then
        local candidate
        candidate="$(cd "$self_dir/../.." && pwd)/dotfiles/mcp/dotfiles-mcp"
        [[ -d "$candidate" ]] && CANONICAL_DIR="$candidate"
      fi
      ;;
    *)
      # Unrecognized layout (e.g. a fully standalone clone of the
      # published repo with no ralphglasses superproject alongside it).
      # Leave whichever side is unresolved empty; callers report this
      # as a skip, not an error.
      ;;
  esac
}
