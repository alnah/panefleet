#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_BIN="${PANEFLEET_AGENT_BRIDGE_BIN:-$REPO_ROOT/bin/panefleet-agent-bridge}"

if ! command -v go >/dev/null 2>&1; then
  printf 'Go est requis pour compiler panefleet-agent-bridge depuis les sources.\n' >&2
  exit 1
fi

mkdir -p "$(dirname "$OUTPUT_BIN")"
go build -o "$OUTPUT_BIN" "$REPO_ROOT/cmd/panefleet-agent-bridge"

printf 'Built %s\n' "$OUTPUT_BIN"
