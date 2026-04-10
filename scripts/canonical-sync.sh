#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="$repo_root/scripts/canonical-sync-manifest.txt"
mode="check"
json=0
canonical_override=""
canonical_ref="${CANONICAL_DOTFILES_MCP_REF:-origin/main}"
show_diff=0

usage() {
  cat <<'EOF'
Usage: canonical-sync.sh [--report|--diff] [--json] [--canonical PATH] [--ref REF]

Compare or sync the manifest-driven subset of files that this standalone
publish mirror intentionally carries forward from the canonical source at
dotfiles/mcp/dotfiles-mcp.

Options:
  --report           report manifest-driven drift against canonical (default)
  --diff             report drift and emit unified diffs for changed mappings
  --json             emit machine-readable output
  --canonical PATH   override the canonical dotfiles repo or module path
  --ref REF          git ref inside the canonical repo (default: origin/main)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --report|--check)
      mode="report"
      ;;
    --diff)
      mode="report"
      show_diff=1
      ;;
    --json)
      json=1
      ;;
    --canonical)
      shift
      [[ $# -gt 0 ]] || { echo "canonical-sync: missing value for --canonical" >&2; exit 2; }
      canonical_override="$1"
      ;;
    --ref)
      shift
      [[ $# -gt 0 ]] || { echo "canonical-sync: missing value for --ref" >&2; exit 2; }
      canonical_ref="$1"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "canonical-sync: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

resolve_canonical_repo() {
  local -a candidates=()

  if [[ -n "$canonical_override" ]]; then
    candidates+=("$canonical_override")
  fi

  candidates+=(
    "${CANONICAL_DOTFILES_REPO:-}"
    "$repo_root/../dotfiles"
    "$repo_root/../dotfiles/mcp/dotfiles-mcp"
    "$HOME/hairglasses-studio/dotfiles"
    "$HOME/hairglasses-studio/dotfiles/mcp/dotfiles-mcp"
    "/home/hg/hairglasses-studio/dotfiles"
    "/home/hg/hairglasses-studio/dotfiles/mcp/dotfiles-mcp"
  )

  local path
  for path in "${candidates[@]}"; do
    [[ -n "$path" ]] || continue
    if [[ -d "$path" ]]; then
      if git -C "$path" rev-parse --show-toplevel >/dev/null 2>&1; then
        git -C "$path" rev-parse --show-toplevel
        return 0
      fi
    fi
  done

  echo "canonical-sync: unable to locate canonical dotfiles git repo" >&2
  exit 1
}

canonical_repo="$(resolve_canonical_repo)"

resolve_effective_ref() {
  local repo="$1"
  local want="$2"
  if git -C "$repo" rev-parse --verify --quiet "$want^{commit}" >/dev/null; then
    printf '%s\n' "$want"
    return 0
  fi
  if git -C "$repo" rev-parse --verify --quiet "main^{commit}" >/dev/null; then
    printf '%s\n' "main"
    return 0
  fi
  printf '%s\n' "HEAD"
}

canonical_ref="$(resolve_effective_ref "$canonical_repo" "$canonical_ref")"

load_canonical_file() {
  local repo="$1"
  local ref="$2"
  local rel="$3"
  git -C "$repo" show "$ref:mcp/dotfiles-mcp/$rel"
}

if [[ ! -f "$manifest" ]]; then
  echo "canonical-sync: manifest not found: $manifest" >&2
  exit 1
fi

declare -a changed_paths=()
declare -a missing_paths=()

while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  [[ "${line:0:1}" == "#" ]] && continue

  src_rel="$line"
  dst_rel="$line"
  if [[ "$line" == *"=>"* ]]; then
    src_rel="${line%%=>*}"
    dst_rel="${line#*=>}"
    src_rel="$(printf '%s' "$src_rel" | sed 's/[[:space:]]*$//')"
    dst_rel="$(printf '%s' "$dst_rel" | sed 's/^[[:space:]]*//')"
  fi

  dst="$repo_root/$dst_rel"
  label="$src_rel"
  if [[ "$src_rel" != "$dst_rel" ]]; then
    label="$src_rel => $dst_rel"
  fi

  if ! canonical_blob="$(load_canonical_file "$canonical_repo" "$canonical_ref" "$src_rel" 2>/dev/null)"; then
    missing_paths+=("$label")
    continue
  fi

  current_blob=""
  if [[ -f "$dst" ]]; then
    current_blob="$(cat "$dst")"
  fi

  if [[ "$canonical_blob" != "$current_blob" ]]; then
    changed_paths+=("$label")
    if [[ $show_diff -eq 1 ]]; then
      diff -u \
        --label "$src_rel ($canonical_ref)" \
        --label "$dst_rel (mirror)" \
        <(printf '%s' "$canonical_blob") \
        <(printf '%s' "$current_blob") || true
    fi
  fi
done < "$manifest"

status="aligned"
if (( ${#missing_paths[@]} > 0 )); then
  status="missing"
elif (( ${#changed_paths[@]} > 0 )); then
  status="drift"
fi

if [[ $json -eq 1 ]]; then
  changed_file="$(mktemp)"
  missing_file="$(mktemp)"
  trap 'rm -f "$changed_file" "$missing_file"' EXIT
  printf '%s\n' "${changed_paths[@]}" >"$changed_file"
  printf '%s\n' "${missing_paths[@]}" >"$missing_file"
  python3 - "$status" "$mode" "$canonical_repo" "$canonical_ref" "$changed_file" "$missing_file" <<'PY'
import json
from pathlib import Path
import sys

status, mode, canonical_repo, canonical_ref, changed_path, missing_path = sys.argv[1:7]
changed = [line for line in Path(changed_path).read_text().splitlines() if line]
missing = [line for line in Path(missing_path).read_text().splitlines() if line]
print(json.dumps({
    "status": status,
    "mode": mode,
    "canonical_repo": canonical_repo,
    "canonical_ref": canonical_ref,
    "changed_paths": changed,
    "missing_paths": missing,
}, indent=2))
PY
else
  echo "Canonical repo: $canonical_repo"
  echo "Canonical ref: $canonical_ref"
  echo "Mode: $mode"
  echo "Status: $status"
  if (( ${#changed_paths[@]} > 0 )); then
    echo "Changed manifest paths:"
    printf '  - %s\n' "${changed_paths[@]}"
  else
    echo "Changed manifest paths: none"
  fi
  if (( ${#missing_paths[@]} > 0 )); then
    echo "Missing in canonical:"
    printf '  - %s\n' "${missing_paths[@]}"
  else
    echo "Missing in canonical: none"
  fi
fi

if [[ "$status" == "missing" ]]; then
  exit 1
fi
