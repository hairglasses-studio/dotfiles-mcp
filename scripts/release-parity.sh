#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

fail() {
  echo "release parity: $*" >&2
  exit 1
}

env GOWORK=off go run ./cmd/dotfiles-mcp-contract --check

jq -e '
  .publish_mirror == false and
  .canonical_source == "https://github.com/hairglasses-studio/dotfiles/tree/main/mcp/dotfiles-mcp" and
  .repository == "https://github.com/hairglasses-studio/dotfiles" and
  .capabilities.tools == true and
  .capabilities.resources == true and
  .capabilities.prompts == true and
  .tool_count > 0 and
  .resource_count > 0 and
  .prompt_count > 0
' .well-known/mcp.json >/dev/null || fail ".well-known/mcp.json is missing canonical parity fields"

for path in README.md ROADMAP.md CONTRIBUTING.md; do
  grep -q 'snapshots/contract' "$path" || fail "missing contract snapshot reference in $path"
done

echo "release parity: ok"
