#!/usr/bin/env bash
set -euo pipefail

json=0
strict_missing=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --json)
      json=1
      ;;
    --strict-missing)
      strict_missing=1
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
  shift
done

declare -a lines=()
failures=0
missing=0

run_check() {
  local group="$1"
  local name="$2"
  local probe="$3"
  local binary="${4:-}"
  local status detail

  if [[ -n "$binary" ]] && ! command -v "$binary" >/dev/null 2>&1; then
    status="missing"
    detail="binary not found: $binary"
    ((missing+=1))
    if [[ $strict_missing -eq 1 ]]; then
      ((failures+=1))
    fi
  elif eval "$probe" >/tmp/dotfiles-mcp-host-smoke.$$ 2>&1; then
    detail="$(tr '\n' ' ' </tmp/dotfiles-mcp-host-smoke.$$ | sed 's/[[:space:]]\\+/ /g' | sed 's/^ //; s/ $//')"
    detail="${detail:0:160}"
    if [[ "$detail" == SKIP:* ]]; then
      status="skip"
      detail="${detail#SKIP: }"
    else
      status="ok"
    fi
  else
    status="fail"
    detail="$(tr '\n' ' ' </tmp/dotfiles-mcp-host-smoke.$$ | sed 's/[[:space:]]\\+/ /g' | sed 's/^ //; s/ $//')"
    detail="${detail:0:160}"
    ((failures+=1))
  fi

  lines+=("$group|$name|$status|$detail")
  rm -f /tmp/dotfiles-mcp-host-smoke.$$ || true
}

run_check "hyprland" "hyprctl" "[ -n \"${HYPRLAND_INSTANCE_SIGNATURE:-}\" ] || { echo 'SKIP: hyprland session not active'; exit 0; }; hyprctl version" "hyprctl"
run_check "hyprland" "ydotool" "ydotool --help | head -n 1" "ydotool"
run_check "hyprland" "wtype" "wtype --help >/tmp/dotfiles-mcp-wtype.$$ 2>&1 || true; head -n 1 /tmp/dotfiles-mcp-wtype.$$; rm -f /tmp/dotfiles-mcp-wtype.$$; exit 0" "wtype"
run_check "bluetooth" "bluetoothctl" "bluetoothctl --version" "bluetoothctl"
run_check "input" "busctl" "busctl --version | head -n 1" "busctl"
run_check "input" "juhradial config" "for base in \"${DOTFILES_DIR:-}\" \"$HOME/hairglasses-studio/dotfiles\" \"/home/hg/hairglasses-studio/dotfiles\"; do if [[ -n \"$base\" && -f \"$base/juhradial/config.json\" ]]; then echo present:\"$base/juhradial/config.json\"; exit 0; fi; done; echo 'SKIP: juhradial config path not found'; exit 0"
run_check "github" "gh" "gh --version | head -n 1" "gh"
run_check "github" "git" "git --version" "git"

if [[ $json -eq 1 ]]; then
  printf '{\n'
  printf '  "status": "%s",\n' "$([[ $failures -eq 0 ]] && echo ok || echo fail)"
  printf '  "strict_missing": %s,\n' "$([[ $strict_missing -eq 1 ]] && echo true || echo false)"
  printf '  "checks": [\n'
  for i in "${!lines[@]}"; do
    IFS='|' read -r group name status detail <<<"${lines[$i]}"
    printf '    {"group":"%s","name":"%s","status":"%s","detail":"%s"}' \
      "$(printf '%s' "$group" | sed 's/"/\\"/g')" \
      "$(printf '%s' "$name" | sed 's/"/\\"/g')" \
      "$(printf '%s' "$status" | sed 's/"/\\"/g')" \
      "$(printf '%s' "$detail" | sed 's/"/\\"/g')"
    if (( i < ${#lines[@]} - 1 )); then
      printf ','
    fi
    printf '\n'
  done
  printf '  ]\n'
  printf '}\n'
else
  printf '%-12s %-18s %-8s %s\n' "GROUP" "CHECK" "STATUS" "DETAIL"
  for line in "${lines[@]}"; do
    IFS='|' read -r group name status detail <<<"$line"
    printf '%-12s %-18s %-8s %s\n' "$group" "$name" "$status" "$detail"
  done
fi

if [[ $failures -ne 0 ]]; then
  exit 1
fi
