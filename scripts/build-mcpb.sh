#!/usr/bin/env bash
# build-mcpb.sh -- build a Linux/amd64 MCPB bundle for registry publication.

set -euo pipefail

ROOT_DIR="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/dist}"
VERSION="$(python3 - "$ROOT_DIR" <<'PY'
import json
import sys
from pathlib import Path

root = Path(sys.argv[1])
overview = json.loads((root / "snapshots/contract/overview.json").read_text(encoding="utf-8"))
print(overview["version"])
PY
)"

artifact="dotfiles-mcp_${VERSION}_linux_amd64.mcpb"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

mkdir -p "$tmpdir/server" "$OUT_DIR"

(
    cd "$ROOT_DIR"
    GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -trimpath -buildvcs=false \
        -ldflags="-s -w -X main.version=${VERSION} -X main.commit=mcpb" \
        -o "$tmpdir/server/dotfiles-mcp" ./cmd/dotfiles-mcp
)

cat > "$tmpdir/manifest.json" <<EOF
{
  "manifest_version": "0.3",
  "name": "dotfiles-mcp",
  "display_name": "dotfiles-mcp",
  "version": "${VERSION}",
  "description": "Hyprland, Wayland desktop, GitHub org, and fleet automation for Linux workstations.",
  "long_description": "Discovery-first MCP server for Linux workstation automation with Hyprland IPC, Wayland desktop control, GitHub org workflows, fleet auditing, systemd operations, input devices, and kitty runtime control.",
  "author": {
    "name": "hairglasses-studio",
    "url": "https://github.com/hairglasses-studio"
  },
  "repository": {
    "type": "git",
    "url": "https://github.com/hairglasses-studio/dotfiles-mcp.git"
  },
  "homepage": "https://github.com/hairglasses-studio/dotfiles-mcp",
  "documentation": "https://github.com/hairglasses-studio/dotfiles-mcp#readme",
  "support": "https://github.com/hairglasses-studio/dotfiles-mcp/issues",
  "license": "MIT",
  "keywords": ["mcp", "hyprland", "wayland", "linux", "desktop-automation", "github", "systemd"],
  "server": {
    "type": "binary",
    "entry_point": "server/dotfiles-mcp",
    "mcp_config": {
      "command": "\${__dirname}/server/dotfiles-mcp",
      "env": {
        "DOTFILES_MCP_PROFILE": "\${user_config.profile}"
      }
    }
  },
  "tools_generated": true,
  "prompts_generated": true,
  "compatibility": {
    "platforms": ["linux"]
  },
  "user_config": {
    "profile": {
      "type": "string",
      "title": "Startup profile",
      "description": "Tool loading profile: default, desktop, ops, or full.",
      "default": "default",
      "required": false
    }
  }
}
EOF

find "$tmpdir" -exec touch -t 202604230000 {} +

(
    cd "$tmpdir"
    zip -X -q -r "$OUT_DIR/$artifact" manifest.json server
)

openssl dgst -sha256 "$OUT_DIR/$artifact" | awk '{print $2}' > "$OUT_DIR/$artifact.sha256"
printf 'mcpb=%s sha256=%s\n' "$OUT_DIR/$artifact" "$(cat "$OUT_DIR/$artifact.sha256")"
