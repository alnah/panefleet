#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cd "$REPO_ROOT"

printf '==> go test\n'
go test ./...

printf '==> go test -race\n'
go test -race ./cmd/panefleet-agent-bridge

printf '==> shellcheck\n'
shellcheck bin/panefleet tests/fake-fzf tests/fake-tmux tests/test_panefleet.sh scripts/*.sh

printf '==> shell regression\n'
./tests/test_panefleet.sh
