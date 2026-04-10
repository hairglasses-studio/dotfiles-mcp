#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

base_ref="${1:-HEAD~1}"

python3 - "$base_ref" <<'PY'
import json
import subprocess
import sys
from pathlib import Path

base_ref = sys.argv[1]
repo_root = Path.cwd()

def load_worktree(path):
    return json.loads((repo_root / path).read_text())

def load_ref(ref, path):
    try:
        data = subprocess.check_output(
            ["git", "show", f"{ref}:{path}"],
            text=True,
            stderr=subprocess.DEVNULL,
        )
    except subprocess.CalledProcessError as exc:
        if exc.returncode == 128:
            return None
        sys.stderr.write(f"contract diff: unable to read {path} from {ref}: {exc}\n")
        sys.exit(1)
    return json.loads(data)

base_overview = load_ref(base_ref, "snapshots/contract/overview.json")
head_overview = load_worktree("snapshots/contract/overview.json")
base_tools = load_ref(base_ref, "snapshots/contract/tools.json")
head_tools = load_worktree("snapshots/contract/tools.json")
base_resources = load_ref(base_ref, "snapshots/contract/resources.json")
head_resources = load_worktree("snapshots/contract/resources.json")
base_prompts = load_ref(base_ref, "snapshots/contract/prompts.json")
head_prompts = load_worktree("snapshots/contract/prompts.json")

if base_overview is None:
    base_overview = {
        "total_tools": 0,
        "resource_count": 0,
        "prompt_count": 0,
        "deferred_tools": 0,
    }
if base_tools is None:
    base_tools = []
if base_resources is None:
    base_resources = []
if base_prompts is None:
    base_prompts = []

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

tool_added, tool_removed, tool_changed = diff_names(base_tools, head_tools, "name")
res_added, res_removed, res_changed = diff_names(base_resources, head_resources, "uri")
prompt_added, prompt_removed, prompt_changed = diff_names(base_prompts, head_prompts, "name")

def delta(head, base):
    diff = head - base
    if diff > 0:
        return f"+{diff}"
    return str(diff)

def preview(items, limit=12):
    if not items:
        return "none"
    shown = items[:limit]
    extra = len(items) - len(shown)
    if extra > 0:
        return ", ".join(shown) + f", +{extra} more"
    return ", ".join(shown)

print(f"## Contract Diff vs {base_ref}")
print("")
if base_overview["total_tools"] == 0 and base_overview["resource_count"] == 0 and base_overview["prompt_count"] == 0:
    print("- Base ref did not contain a committed contract snapshot; treating this as initial publication.")
    print("")
print(f"- Tools: {base_overview['total_tools']} -> {head_overview['total_tools']} ({delta(head_overview['total_tools'], base_overview['total_tools'])})")
print(f"- Resources: {base_overview['resource_count']} -> {head_overview['resource_count']} ({delta(head_overview['resource_count'], base_overview['resource_count'])})")
print(f"- Prompts: {base_overview['prompt_count']} -> {head_overview['prompt_count']} ({delta(head_overview['prompt_count'], base_overview['prompt_count'])})")
print(f"- Deferred tools: {base_overview['deferred_tools']} -> {head_overview['deferred_tools']} ({delta(head_overview['deferred_tools'], base_overview['deferred_tools'])})")
print("")
print(f"- Added tools: {preview(tool_added)}")
print(f"- Removed tools: {preview(tool_removed)}")
print(f"- Changed tools: {preview(tool_changed)}")
print(f"- Added resources: {preview(res_added)}")
print(f"- Removed resources: {preview(res_removed)}")
print(f"- Changed resources: {preview(res_changed)}")
print(f"- Added prompts: {preview(prompt_added)}")
print(f"- Removed prompts: {preview(prompt_removed)}")
print(f"- Changed prompts: {preview(prompt_changed)}")
PY
