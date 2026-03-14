#!/usr/bin/env bash

set -euo pipefail

PLUGIN_ROOT="${PANEFLEET_ROOT:-${HOME}/.tmux/plugins/panefleet}"
PANEFLEET_BIN="${PLUGIN_ROOT}/bin/panefleet"

set_default_option() {
  local name="$1"
  local value="$2"

  if [[ -z "$(tmux show-options -gqv "$name")" ]]; then
    tmux set-option -gq "$name" "$value"
  fi
}

set_default_option @panefleet-done-recent-minutes "${PANEFLEET_DONE_RECENT_MINUTES:-10}"
set_default_option @panefleet-stale-minutes "${PANEFLEET_STALE_MINUTES:-45}"
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

if [[ ! -x "$PANEFLEET_BIN" ]]; then
  tmux display-message "panefleet: missing executable ${PANEFLEET_BIN}"
  exit 0
fi

if ! "${PANEFLEET_BIN}" preflight --quiet; then
  tmux display-message "panefleet: preflight failed, run ${PANEFLEET_BIN} preflight"
  exit 0
fi

tmux bind-key -T prefix P run-shell -b "${PANEFLEET_BIN} popup"
tmux bind-key -T prefix T run-shell -b "${PANEFLEET_BIN} theme-popup"
ensure_hook after-select-pane "$(touch_hook_command)"
ensure_hook after-select-window "$(touch_hook_command)"
ensure_hook client-session-changed "$(touch_hook_command)"
ensure_hook client-active "$(touch_hook_command)"
