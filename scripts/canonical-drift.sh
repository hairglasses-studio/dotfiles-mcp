#!/usr/bin/env bash
# Report file-level drift between the canonical dotfiles-mcp checkout
# (dotfiles/mcp/dotfiles-mcp in the ralphglasses monorepo) and the
# oss/dotfiles-mcp publish mirror (a git submodule pinned to
# github.com/hairglasses-studio/dotfiles-mcp). Read-only — an
# `rsync --dry-run` diff report, never a sync.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/canonical-pair.sh
source "$script_dir/lib/canonical-pair.sh"
resolve_canonical_pair

if [[ -z "${CANONICAL_DIR:-}" || -z "${MIRROR_DIR:-}" ]]; then
  echo "canonical-drift: sibling checkout not found (canonical='${CANONICAL_DIR:-<none>}' mirror='${MIRROR_DIR:-<none>}')."
  echo "canonical-drift: this is expected for a standalone clone of the published repo without the ralphglasses monorepo alongside it. Skipping."
  exit 0
fi

if ! command -v rsync >/dev/null 2>&1; then
  echo "canonical-drift: rsync not found on PATH; cannot compute drift." >&2
  exit 1
fi

echo "## dotfiles-mcp canonical drift"
echo ""
echo "- canonical: $CANONICAL_DIR"
echo "- mirror:    $MIRROR_DIR"
echo ""

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

# -n dry-run, -i itemized changes, -rtc recurse + mtime + checksum compare
# (checksum, not size+mtime, since these are two independent git checkouts
# with unrelated commit history and mtimes).
report="$(rsync -rtcni --delete "${exclude_args[@]}" "$CANONICAL_DIR"/ "$MIRROR_DIR"/ || true)"

if [[ -z "$report" ]]; then
  echo "No drift: mirror content-matches canonical (excluding provider/config surfaces)."
  exit 0
fi

added=0
changed=0
removed=0
while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  case "$line" in
    '>f+++++++'*) added=$((added + 1)) ;;
    '>f'*) changed=$((changed + 1)) ;;
    '*deleting'*) removed=$((removed + 1)) ;;
  esac
done <<<"$report"

echo "Drift found: $added file(s) only in canonical, $changed changed, $removed only in mirror."
echo ""
echo "### rsync itemized report (canonical -> mirror direction; would-copy items)"
echo '```'
echo "$report"
echo '```'
echo ""
echo "To align the mirror, hand-copy the listed files from canonical into the"
echo "mirror checkout, commit inside oss/dotfiles-mcp, then bump the submodule"
echo "pin in the ralphglasses superproject. There is no automated canonical ->"
echo "mirror sync tool yet (see scripts/canonical-sync.sh for the read-only"
echo "report/diff precursor to one)."
