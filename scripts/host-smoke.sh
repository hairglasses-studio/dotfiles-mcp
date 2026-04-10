#!/usr/bin/env bash
set -euo pipefail

json=0
strict_missing=0
strict_skip=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --json)
      json=1
      ;;
    --strict-missing)
      strict_missing=1
      ;;
    --strict-skip)
      strict_skip=1
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

sanitize_detail() {
  local path="$1"
  local text=""
  if [[ -f "$path" ]]; then
    text="$(tr '\n' ' ' <"$path" | sed 's/[[:space:]]\+/ /g' | sed 's/^ //; s/ $//')"
  fi
  printf '%.160s' "$text"
}

check_help_line() {
  local cmd="$1"
  local tmp="/tmp/dotfiles-mcp-help.$$"
  "$cmd" --help >"$tmp" 2>&1 || "$cmd" -h >"$tmp" 2>&1 || true
  head -n 1 "$tmp" || true
  rm -f "$tmp"
}

check_wtype_help() {
  local tmp="/tmp/dotfiles-mcp-wtype.$$"
  wtype --help >"$tmp" 2>&1 || true
  head -n 1 "$tmp" || true
  rm -f "$tmp"
}

check_juhradial_config() {
  local base
  for base in "${DOTFILES_DIR:-}" "$HOME/hairglasses-studio/dotfiles" "/home/hg/hairglasses-studio/dotfiles"; do
    if [[ -n "$base" && -f "$base/juhradial/config.json" ]]; then
      echo "present:$base/juhradial/config.json"
      return 0
    fi
  done
  echo "SKIP: juhradial config path not found"
}

check_dbus_session() {
  if [[ -n "${DBUS_SESSION_BUS_ADDRESS:-}" ]]; then
    echo "DBUS_SESSION_BUS_ADDRESS present"
    return 0
  fi
  echo "SKIP: DBUS_SESSION_BUS_ADDRESS not detected"
}

check_wayland_display() {
  if [[ -n "${WAYLAND_DISPLAY:-}" ]]; then
    echo "WAYLAND_DISPLAY=${WAYLAND_DISPLAY}"
    return 0
  fi
  echo "SKIP: WAYLAND_DISPLAY not detected"
}

check_hyprctl_session() {
  if [[ -z "${HYPRLAND_INSTANCE_SIGNATURE:-}" ]]; then
    echo "SKIP: hyprland session not active"
    return 0
  fi
  hyprctl version
}

check_pyatspi() {
  local tmp="/tmp/dotfiles-mcp-pyatspi.$$"
  if python3 -c "import pyatspi; print('pyatspi import ok')" >"$tmp" 2>&1; then
    head -n 1 "$tmp" || true
  else
    local detail
    detail="$(sanitize_detail "$tmp")"
    echo "SKIP: pyatspi import failed: ${detail:-unknown}"
  fi
  rm -f "$tmp"
}

run_check() {
  local group="$1"
  local name="$2"
  local probe="$3"
  local binary="${4:-}"
  local status detail
  local tmp="/tmp/dotfiles-mcp-host-smoke.$$"

  if [[ -n "$binary" ]] && ! command -v "$binary" >/dev/null 2>&1; then
    status="missing"
    detail="binary not found: $binary"
    ((missing+=1))
    if [[ $strict_missing -eq 1 ]]; then
      ((failures+=1))
    fi
  elif eval "$probe" >"$tmp" 2>&1; then
    detail="$(sanitize_detail "$tmp")"
    if [[ "$detail" == SKIP:* ]]; then
      status="skip"
      detail="${detail#SKIP: }"
      if [[ $strict_skip -eq 1 ]]; then
        ((failures+=1))
      fi
    else
      status="ok"
    fi
  else
    status="fail"
    detail="$(sanitize_detail "$tmp")"
    ((failures+=1))
  fi

  lines+=("$group|$name|$status|$detail")
  rm -f "$tmp" || true
}

run_check "hyprland" "hyprctl" "check_hyprctl_session" "hyprctl"
run_check "hyprland" "ydotool" "ydotool --help | head -n 1" "ydotool"
run_check "hyprland" "wtype" "check_wtype_help" "wtype"
run_check "semantic" "python3" "python3 --version | head -n 1" "python3"
run_check "semantic" "pyatspi" "check_pyatspi" "python3"
run_check "semantic" "session bus" "check_dbus_session"
run_check "session" "dbus-run-session" "check_help_line dbus-run-session" "dbus-run-session"
run_check "session" "wayland-info" "check_help_line wayland-info" "wayland-info"
run_check "session" "grim" "check_help_line grim" "grim"
run_check "session" "wl-copy" "check_help_line wl-copy" "wl-copy"
run_check "session" "wl-paste" "check_help_line wl-paste" "wl-paste"
run_check "session" "kwin_wayland" "check_help_line kwin_wayland" "kwin_wayland"
run_check "session" "wayland display" "check_wayland_display"
run_check "bluetooth" "bluetoothctl" "bluetoothctl --version" "bluetoothctl"
run_check "input" "busctl" "busctl --version | head -n 1" "busctl"
run_check "input" "juhradial config" "check_juhradial_config"
run_check "github" "gh" "gh --version | head -n 1" "gh"
run_check "github" "git" "git --version" "git"

if [[ $json -eq 1 ]]; then
  printf '{\n'
  printf '  "status": "%s",\n' "$([[ $failures -eq 0 ]] && echo ok || echo fail)"
  printf '  "strict_missing": %s,\n' "$([[ $strict_missing -eq 1 ]] && echo true || echo false)"
  printf '  "strict_skip": %s,\n' "$([[ $strict_skip -eq 1 ]] && echo true || echo false)"
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
