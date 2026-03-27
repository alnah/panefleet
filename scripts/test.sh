#!/usr/bin/env bash

set -euo pipefail

# test.sh runs the project quality gates in CI/local with a fixed order:
# compile/tests first, then shell static checks, then contract regressions.

REPO_ROOT="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$REPO_ROOT"

printf '==> go test\n'
go test ./...

printf '==> go test -race\n'
go test -race ./cmd/panefleet-agent-bridge

./scripts/lint-shell.sh

printf '==> shell regression\n'
./tests/test_panefleet.sh

printf '==> make install contract\n'
./tests/test_make_install.sh

printf '==> bridge install contract\n'
./tests/test_install_bridge.sh
