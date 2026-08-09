#!/usr/bin/env bash
# Read-only precursor to a canonical -> mirror sync for dotfiles-mcp.
# Never mutates either checkout — this is reporting only, matching the
# "canonical-sync-report" / "canonical-sync-diff" Makefile targets that
# call it. There is no `--apply` mode: hand-copy the reported files and
# commit inside oss/dotfiles-mcp, per canonical-drift.sh's guidance.
#
# Usage:
#   canonical-sync.sh --report   # categorized file lists (added/changed/removed)
#   canonical-sync.sh --diff     # unified diff per changed file (capped)
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/canonical-pair.sh
source "$script_dir/lib/canonical-pair.sh"
resolve_canonical_pair

mode=""
case "${1:-}" in
  --report) mode="report" ;;
  --diff) mode="diff" ;;
  *)
    echo "usage: $(basename "$0") --report|--diff" >&2
    exit 2
    ;;
esac

if [[ -z "${CANONICAL_DIR:-}" || -z "${MIRROR_DIR:-}" ]]; then
  echo "canonical-sync: sibling checkout not found (canonical='${CANONICAL_DIR:-<none>}' mirror='${MIRROR_DIR:-<none>}')."
  echo "canonical-sync: this is expected for a standalone clone of the published repo without the ralphglasses monorepo alongside it. Skipping."
  exit 0
fi

if ! command -v rsync >/dev/null 2>&1; then
  echo "canonical-sync: rsync not found on PATH." >&2
  exit 1
fi

exclude_args=(
  --exclude .git
  --exclude .agents
  --exclude .claude
  --exclude .codex
  --exclude .gemini
  --exclude .github
  --exclude coverage.out
  --exclude '*.log'
  --exclude dotfiles-mcp
  --exclude '.golangci*.yml'
  --exclude '.pre-commit-config.yaml'
)

report="$(rsync -rtcni --delete "${exclude_args[@]}" "$CANONICAL_DIR"/ "$MIRROR_DIR"/ || true)"

added=()
changed=()
removed=()
while read -r code path; do
  [[ -z "${code:-}" ]] && continue
  [[ "$path" == */ ]] && continue # directory entries only, skip
  case "$code" in
    '>f+++++++'*) added+=("$path") ;;
    '>f'*) changed+=("$path") ;;
    '*deleting') removed+=("$path") ;;
  esac
done <<<"$report"

echo "## dotfiles-mcp canonical sync ($mode)"
echo ""
echo "- canonical: $CANONICAL_DIR"
echo "- mirror:    $MIRROR_DIR"
echo ""

if [[ "$mode" == "report" ]]; then
  echo "### Only in canonical (would be added to mirror): ${#added[@]}"
  printf '  %s\n' "${added[@]}"
  echo ""
  echo "### Changed (canonical differs from mirror): ${#changed[@]}"
  printf '  %s\n' "${changed[@]}"
  echo ""
  echo "### Only in mirror (not in canonical): ${#removed[@]}"
  printf '  %s\n' "${removed[@]}"
  exit 0
fi

# --diff mode
if [[ ${#changed[@]} -eq 0 ]]; then
  echo "No changed files to diff."
  exit 0
fi

cap=20
shown=0
for path in "${changed[@]}"; do
  if [[ $shown -ge $cap ]]; then
    remaining=$((${#changed[@]} - shown))
    echo "... $remaining more changed file(s) omitted (cap=$cap); run --report for the full list."
    break
  fi
  echo "### $path"
  echo '```diff'
  diff -u "$MIRROR_DIR/$path" "$CANONICAL_DIR/$path" || true
  echo '```'
  echo ""
  shown=$((shown + 1))
done
