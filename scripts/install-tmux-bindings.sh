#!/usr/bin/env bash

set -euo pipefail

PLUGIN_ROOT="${PANEFLEET_ROOT:-${HOME}/.tmux/plugins/panefleet}"
PANEFLEET_BIN="${PLUGIN_ROOT}/bin/panefleet"

tmux set-option -gq @panefleet-done-recent-minutes "${PANEFLEET_DONE_RECENT_MINUTES:-10}"
tmux set-option -gq @panefleet-stale-minutes "${PANEFLEET_STALE_MINUTES:-45}"
tmux bind-key -T prefix P display-popup -E -w 90% -h 85% -T "panefleet" "${PANEFLEET_BIN} board"
tmux set-hook -g after-select-pane "run-shell -b '${PANEFLEET_BIN} touch \"#{pane_id}\"'"
tmux set-hook -g after-select-window "run-shell -b '${PANEFLEET_BIN} touch \"#{pane_id}\"'"
tmux set-hook -g client-session-changed "run-shell -b '${PANEFLEET_BIN} touch \"#{pane_id}\"'"
tmux set-hook -g client-active "run-shell -b '${PANEFLEET_BIN} touch \"#{pane_id}\"'"
