#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_ROOT="${PANEFLEET_ROOT:-$(CDPATH='' cd -- "${SCRIPT_DIR}/.." && pwd)}"
PANEFLEET_BIN="${PLUGIN_ROOT}/bin/panefleet"
ACTION="${1:-install}"

set_default_option() {
  local name="$1"
  local value="$2"

  if [[ -z "$(tmux show-options -gqv "$name")" ]]; then
    tmux set-option -gq "$name" "$value"
  fi
}

set_default_option @panefleet-done-recent-minutes "${PANEFLEET_DONE_RECENT_MINUTES:-10}"
set_default_option @panefleet-stale-minutes "${PANEFLEET_STALE_MINUTES:-45}"
set_default_option @panefleet-agent-status-max-age-seconds "${PANEFLEET_AGENT_STATUS_MAX_AGE_SECONDS:-600}"
set_default_option @panefleet-adapter-mode "${PANEFLEET_ADAPTER_MODE:-heuristic-only}"
set_default_option @panefleet-theme "${PANEFLEET_THEME:-panefleet-dark}"

ensure_hook() {
  local hook_name="$1"
  local hook_command="$2"
  local existing_hooks

  existing_hooks="$(tmux show-hooks -g "$hook_name" 2>/dev/null || true)"
  if printf '%s\n' "$existing_hooks" | rg -Fq -- "$hook_command"; then
    return
  fi

  tmux set-hook -ag "$hook_name" "$hook_command"
}

touch_hook_command() {
  printf 'run-shell -b "%s touch \\"#{pane_id}\\""' "$PANEFLEET_BIN"
}

remove_matching_hooks() {
  local hook_name="$1"
  local hook_command="$2"
  local hook_ref

  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    [[ "$line" != *"$hook_command"* ]] && continue
    hook_ref="${line%% *}"
    tmux set-hook -gu "$hook_ref"
  done < <(tmux show-hooks -g "$hook_name" 2>/dev/null || true)
}

if [[ ! -x "$PANEFLEET_BIN" ]]; then
  tmux display-message "panefleet: missing executable ${PANEFLEET_BIN}"
  exit 0
fi

if ! "${PANEFLEET_BIN}" preflight --quiet; then
  tmux display-message "panefleet: preflight failed, run ${PANEFLEET_BIN} preflight"
  exit 0
fi

case "$ACTION" in
  install|reconcile)
    tmux bind-key -T prefix P run-shell -b "${PANEFLEET_BIN} popup"
    tmux bind-key -T prefix T run-shell -b "${PANEFLEET_BIN} theme-popup"
    ensure_hook after-select-pane "$(touch_hook_command)"
    ensure_hook after-select-window "$(touch_hook_command)"
    ensure_hook client-session-changed "$(touch_hook_command)"
    ensure_hook client-active "$(touch_hook_command)"
    ;;
  uninstall)
    tmux unbind-key -q -T prefix P
    tmux unbind-key -q -T prefix T
    remove_matching_hooks after-select-pane "$(touch_hook_command)"
    remove_matching_hooks after-select-window "$(touch_hook_command)"
    remove_matching_hooks client-session-changed "$(touch_hook_command)"
    remove_matching_hooks client-active "$(touch_hook_command)"
    ;;
  *)
    printf 'unknown action: %s\n' "$ACTION" >&2
    exit 1
    ;;
esac
