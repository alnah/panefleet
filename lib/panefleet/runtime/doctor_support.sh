#!/usr/bin/env bash

# tmux install-state inspection helpers for doctor and install flows.
binding_state() {
  local key="$1"
  local line

  if [[ -z "${TMUX:-}" ]]; then
    printf 'outside-tmux'
    return
  fi

  line="$("${TMUX_BIN}" list-keys -T prefix 2>/dev/null | awk -v key="$key" '$1 == "bind-key" && $4 == key { print; exit }')"
  if [[ -n "$line" ]]; then
    printf '%s' "$line"
  else
    printf 'missing'
  fi
}

matching_hook_count() {
  local hook_name="$1"
  local hook_command="$2"
  local count

  if [[ -z "${TMUX:-}" ]]; then
    printf 'n/a'
    return
  fi

  count="$("${TMUX_BIN}" show-hooks -g "$hook_name" 2>/dev/null | "${RG_BIN}" -F --count -- "$hook_command" || true)"
  printf '%s' "${count:-0}"
}

# install_command is the single user-facing install entrypoint.
