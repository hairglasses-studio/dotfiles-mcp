#!/usr/bin/env bash
# validate-server-json.sh -- local guard for Official MCP Registry metadata.

set -euo pipefail

ROOT_DIR="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"

python3 - "$ROOT_DIR" <<'PY'
import json
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
server_path = root / "server.json"
overview_path = root / "snapshots/contract/overview.json"
dockerfile_path = root / "Dockerfile"

server = json.loads(server_path.read_text(encoding="utf-8"))
overview = json.loads(overview_path.read_text(encoding="utf-8"))
dockerfile = dockerfile_path.read_text(encoding="utf-8")

version = str(overview.get("version", ""))
expected_name = "io.github.hairglasses-studio/dotfiles-mcp"
expected_identifier = f"ghcr.io/hairglasses-studio/dotfiles-mcp:{version}"
failures: list[str] = []

def require(condition: bool, message: str) -> None:
    if not condition:
        failures.append(message)

require(server.get("$schema") == "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json", "server.json schema URL drifted")
require(server.get("name") == expected_name, f"server.json name must be {expected_name}")
require(bool(re.match(r"^[a-zA-Z0-9.-]+/[a-zA-Z0-9._-]+$", server.get("name", ""))), "server.json name must match registry slash namespace format")
require(server.get("version") == version, f"server.json version must match contract overview version {version}")
require(1 <= len(server.get("description", "")) <= 100, "server.json description must be 1..100 chars")

repo = server.get("repository") or {}
require(repo.get("url") == "https://github.com/hairglasses-studio/dotfiles-mcp", "server.json repository URL drifted")
require(repo.get("source") == "github", "server.json repository source must be github")

packages = server.get("packages") or []
oci = next((pkg for pkg in packages if pkg.get("registryType") == "oci"), None)
require(oci is not None, "server.json must include an OCI package")
if oci:
    require(oci.get("identifier") == expected_identifier, f"OCI identifier must be {expected_identifier}")
    require(oci.get("version") == version, f"OCI package version must match {version}")
    require((oci.get("transport") or {}).get("type") == "stdio", "OCI package transport must be stdio")

label = f'io.modelcontextprotocol.server.name="{expected_name}"'
require(label in dockerfile, "Dockerfile MCP registry label must match server.json name")
require("CGO_ENABLED=0" in dockerfile, "Dockerfile should build a static binary for the runtime image")

if failures:
    for failure in failures:
        print(f"server_json=fail {failure}", file=sys.stderr)
    sys.exit(1)

print(f"server_json=ok name={expected_name} version={version} package={expected_identifier}")
PY
