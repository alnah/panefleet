#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-check}"

cd "$REPO_ROOT"

SHELL_FILES=(
  bin/panefleet
  integrations/claude/claude-code-hook
  integrations/codex/codex-app-server-bridge
  integrations/codex/codex-notify-bridge
  integrations/opencode/opencode-event-bridge
  lib/panefleet/integrations/bridge_wrapper.sh
  lib/panefleet/ops/sqlite.sh
  lib/panefleet/runtime/paths.sh
  lib/panefleet/state/engine.sh
  scripts/*.sh
  tests/fake-fzf
  tests/fake-tmux
  tests/test_install_bridge.sh
  tests/test_make_install.sh
  tests/test_panefleet.sh
)

case "$MODE" in
check | --check)
  printf '==> bash -n\n'
  for file in "${SHELL_FILES[@]}"; do
    bash -n "$file"
  done

  printf '==> shfmt -d\n'
  shfmt -d "${SHELL_FILES[@]}"

  printf '==> shellcheck -x\n'
  shellcheck -x "${SHELL_FILES[@]}"
  ;;
fix | --fix)
  printf '==> shfmt -w\n'
  shfmt -w "${SHELL_FILES[@]}"
  ;;
*)
  printf 'usage: %s [check|fix]\n' "$0" >&2
  exit 1
  ;;
esac
