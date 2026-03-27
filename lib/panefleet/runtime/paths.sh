#!/usr/bin/env bash

# Path helpers shared by shell entrypoints.
# The scope is intentionally narrow: repo-root discovery plus XDG/user paths.

panefleet_find_repo_root_from() {
  local entrypoint="$1"
  local dir=""

  if [[ -z "$entrypoint" ]]; then
    return 1
  fi

  dir="$(CDPATH='' cd -- "$(dirname -- "$entrypoint")" && pwd)"
  while [[ -n "$dir" ]]; do
    if [[ -x "$dir/bin/panefleet" && -d "$dir/lib/panefleet" ]]; then
      printf '%s' "$dir"
      return 0
    fi
    if [[ "$dir" == "/" ]]; then
      break
    fi
    dir="$(dirname -- "$dir")"
  done

  return 1
}

panefleet_user_config_home() {
  if [[ -n "${XDG_CONFIG_HOME:-}" ]]; then
    printf '%s' "$XDG_CONFIG_HOME"
  else
    printf '%s/.config' "$HOME"
  fi
}

panefleet_user_state_home() {
  if [[ -n "${XDG_STATE_HOME:-}" ]]; then
    printf '%s' "$XDG_STATE_HOME"
  else
    printf '%s/.local/state' "$HOME"
  fi
}

panefleet_default_bridge_bin_path() {
  printf '%s/panefleet/bin/panefleet-agent-bridge' "$(panefleet_user_state_home)"
}

panefleet_default_event_log_dir() {
  printf '%s/panefleet/events' "$(panefleet_user_state_home)"
}

panefleet_panefleet_cmd_fallback_from() {
  local repo_root="$1"

  printf '%s/bin/panefleet' "$repo_root"
}
