#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

fail() {
  echo "release parity: $*" >&2
  exit 1
}

go run ./cmd/dotfiles-mcp-contract --check

jq -e '
  .publish_mirror == true and
  .canonical_source == "https://github.com/hairglasses-studio/dotfiles/tree/main/mcp/dotfiles-mcp" and
  .repository == "https://github.com/hairglasses-studio/dotfiles-mcp" and
  .homepage == "https://github.com/hairglasses-studio/dotfiles-mcp" and
  .capabilities.tools == true and
  .capabilities.resources == true and
  .capabilities.prompts == true and
  .tool_count > 0 and
  .resource_count > 0 and
  .prompt_count > 0
' .well-known/mcp.json >/dev/null || fail ".well-known/mcp.json is missing mirror parity fields"

for path in README.md ROADMAP.md CONTRIBUTING.md; do
  grep -q 'dotfiles/mcp/dotfiles-mcp' "$path" || fail "missing canonical source reference in $path"
  grep -q 'snapshots/contract' "$path" || fail "missing contract snapshot reference in $path"
done

echo "release parity: ok"
