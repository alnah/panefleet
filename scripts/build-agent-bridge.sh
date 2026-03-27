#!/usr/bin/env bash

set -euo pipefail

# build-agent-bridge.sh exists for explicit local builds when release binaries
# are unavailable. It is intentionally tiny so build behavior stays transparent.

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/panefleet/runtime/paths.sh"

REPO_ROOT="${PANEFLEET_ROOT:-$(panefleet_find_repo_root_from "${BASH_SOURCE[0]}")}"
OUTPUT_BIN="${PANEFLEET_AGENT_BRIDGE_BIN:-$REPO_ROOT/bin/panefleet-agent-bridge}"

if ! command -v go >/dev/null 2>&1; then
  printf 'Go est requis pour compiler panefleet-agent-bridge depuis les sources.\n' >&2
  exit 1
fi

mkdir -p "$(dirname "$OUTPUT_BIN")"
chmod 700 "$(dirname "$OUTPUT_BIN")" 2>/dev/null || true
go build -o "$OUTPUT_BIN" "$REPO_ROOT/cmd/panefleet-agent-bridge"

printf 'Built %s\n' "$OUTPUT_BIN"
