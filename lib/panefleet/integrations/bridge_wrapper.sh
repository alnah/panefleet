#!/usr/bin/env bash

# shellcheck disable=SC1091
source "$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/runtime/paths.sh"

# panefleet_bridge_wrapper_main keeps provider wrappers thin and consistent.
# It resolves repo-relative fallbacks, bridge location, event log directory,
# and optional pane forwarding in one place.
panefleet_bridge_wrapper_main() {
  local entrypoint="$1"
  local bridge_command="$2"
  local install_target="$3"
  local forward_pane="${4:-0}"
  local repo_root bridge_bin panefleet_cli panefleet_ingest_cmd pane
  shift 4

  if ! repo_root="$(panefleet_find_repo_root_from "$entrypoint")"; then
    printf 'panefleet: unable to resolve repository root from %s\n' "$entrypoint" >&2
    exit 1
  fi

  bridge_bin="${PANEFLEET_AGENT_BRIDGE_BIN:-$(panefleet_default_bridge_bin_path)}"
  panefleet_cli="$(command -v panefleet 2>/dev/null || panefleet_panefleet_cmd_fallback_from "$repo_root")"
  panefleet_ingest_cmd="${repo_root}/scripts/panefleet-go"
  export PANEFLEET_INGEST_BIN="${PANEFLEET_INGEST_BIN:-$panefleet_ingest_cmd}"
  export PANEFLEET_EVENT_LOG_DIR="${PANEFLEET_EVENT_LOG_DIR:-$(panefleet_default_event_log_dir)}"

  if [[ ! -x "$bridge_bin" ]]; then
    printf 'panefleet: missing compiled bridge %s\n' "$bridge_bin" >&2
    printf 'install it with %s install %s\n' "$panefleet_cli" "$install_target" >&2
    exit 1
  fi

  if [[ "$forward_pane" == "1" ]]; then
    pane="${PANEFLEET_PANE:-${TMUX_PANE:-}}"
    if [[ -n "$pane" ]]; then
      exec "$bridge_bin" "$bridge_command" --pane "$pane" "$@"
    fi
  fi

  exec "$bridge_bin" "$bridge_command" "$@"
}
