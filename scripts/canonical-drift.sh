#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

strict=0
json=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --strict)
      strict=1
      ;;
    --json)
      json=1
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
  shift
done

python3 - "$repo_root" "$strict" "$json" <<'PY'
import json
import subprocess
import sys
from pathlib import Path
from urllib.request import urlopen
from urllib.error import URLError

repo_root = Path(sys.argv[1])
strict = sys.argv[2] == "1"
as_json = sys.argv[3] == "1"

paths = [
    "snapshots/contract/overview.json",
    "snapshots/contract/tools.json",
    "snapshots/contract/resources.json",
    "snapshots/contract/prompts.json",
]

canonical_candidates = [
    repo_root.parent / "dotfiles" / "mcp" / "dotfiles-mcp",
    Path.home() / "hairglasses-studio" / "dotfiles" / "mcp" / "dotfiles-mcp",
    Path("/home/hg/hairglasses-studio/dotfiles/mcp/dotfiles-mcp"),
]

canonical_root = None
for candidate in canonical_candidates:
    if (candidate / "snapshots/contract/overview.json").exists():
        canonical_root = candidate
        break

def load_local_bundle(root: Path):
    return {
        path: json.loads((root / path).read_text())
        for path in paths
    }

def load_remote_bundle():
    base = "https://raw.githubusercontent.com/hairglasses-studio/dotfiles/main/mcp/dotfiles-mcp/"
    data = {}
    for path in paths:
        with urlopen(base + path) as resp:
            data[path] = json.loads(resp.read().decode("utf-8"))
    return data

mirror = load_local_bundle(repo_root)
source_label = None
try:
    if canonical_root is not None:
        canonical = load_local_bundle(canonical_root)
        source_label = str(canonical_root)
    else:
        canonical = load_remote_bundle()
        source_label = "https://github.com/hairglasses-studio/dotfiles/tree/main/mcp/dotfiles-mcp"
except (FileNotFoundError, URLError, OSError) as exc:
    print(f"canonical drift: unable to load canonical snapshots: {exc}", file=sys.stderr)
    sys.exit(1)

def keyed(items, key):
    return {item[key]: item for item in items}

def diff_names(base_items, head_items, key):
    base_map = keyed(base_items, key)
    head_map = keyed(head_items, key)
    added = sorted(set(head_map) - set(base_map))
    removed = sorted(set(base_map) - set(head_map))
    changed = sorted(
        name for name in (set(base_map) & set(head_map))
        if base_map[name] != head_map[name]
    )
    return added, removed, changed

canonical_overview = canonical["snapshots/contract/overview.json"]
mirror_overview = mirror["snapshots/contract/overview.json"]
tool_extra, tool_missing, tool_changed = diff_names(
    canonical["snapshots/contract/tools.json"],
    mirror["snapshots/contract/tools.json"],
    "name",
)
resource_extra, resource_missing, resource_changed = diff_names(
    canonical["snapshots/contract/resources.json"],
    mirror["snapshots/contract/resources.json"],
    "uri",
)
prompt_extra, prompt_missing, prompt_changed = diff_names(
    canonical["snapshots/contract/prompts.json"],
    mirror["snapshots/contract/prompts.json"],
    "name",
)

has_drift = any([
    canonical_overview["total_tools"] != mirror_overview["total_tools"],
    canonical_overview["resource_count"] != mirror_overview["resource_count"],
    canonical_overview["prompt_count"] != mirror_overview["prompt_count"],
    canonical_overview["module_count"] != mirror_overview["module_count"],
    tool_missing,
    tool_extra,
    tool_changed,
    resource_missing,
    resource_extra,
    resource_changed,
    prompt_missing,
    prompt_extra,
    prompt_changed,
])

payload = {
    "status": "drift" if has_drift else "aligned",
    "canonical_source": source_label,
    "canonical": {
        "tools": canonical_overview["total_tools"],
        "modules": canonical_overview["module_count"],
        "resources": canonical_overview["resource_count"],
        "prompts": canonical_overview["prompt_count"],
    },
    "mirror": {
        "tools": mirror_overview["total_tools"],
        "modules": mirror_overview["module_count"],
        "resources": mirror_overview["resource_count"],
        "prompts": mirror_overview["prompt_count"],
    },
    "missing_tools": tool_missing,
    "extra_tools": tool_extra,
    "changed_tools": tool_changed,
    "missing_resources": resource_missing,
    "extra_resources": resource_extra,
    "changed_resources": resource_changed,
    "missing_prompts": prompt_missing,
    "extra_prompts": prompt_extra,
    "changed_prompts": prompt_changed,
}

def preview(items, limit=10):
    if not items:
        return "none"
    shown = items[:limit]
    extra = len(items) - len(shown)
    if extra > 0:
        return ", ".join(shown) + f", +{extra} more"
    return ", ".join(shown)

if as_json:
    print(json.dumps(payload, indent=2, sort_keys=True))
else:
    print("## Canonical Drift")
    print("")
    print(f"- Canonical source: {source_label}")
    print(f"- Status: {'drift detected' if has_drift else 'aligned'}")
    print(f"- Tools: canonical {canonical_overview['total_tools']} vs mirror {mirror_overview['total_tools']}")
    print(f"- Modules: canonical {canonical_overview['module_count']} vs mirror {mirror_overview['module_count']}")
    print(f"- Resources: canonical {canonical_overview['resource_count']} vs mirror {mirror_overview['resource_count']}")
    print(f"- Prompts: canonical {canonical_overview['prompt_count']} vs mirror {mirror_overview['prompt_count']}")
    print("")
    print(f"- Missing tools in mirror: {preview(tool_missing)}")
    print(f"- Extra tools in mirror: {preview(tool_extra)}")
    print(f"- Changed tools: {preview(tool_changed)}")
    print(f"- Missing resources in mirror: {preview(resource_missing)}")
    print(f"- Extra resources in mirror: {preview(resource_extra)}")
    print(f"- Changed resources: {preview(resource_changed)}")
    print(f"- Missing prompts in mirror: {preview(prompt_missing)}")
    print(f"- Extra prompts in mirror: {preview(prompt_extra)}")
    print(f"- Changed prompts: {preview(prompt_changed)}")

if strict and has_drift:
    sys.exit(1)
PY
