#!/usr/bin/env bash

# Go-backed board launcher and popup integration.

board_backend() {
  printf '%s' "${PANEFLEET_BOARD_BACKEND:-shell}"
}

go_board_refresh_interval() {
  printf '%s' "${PANEFLEET_GO_BOARD_REFRESH:-1s}"
}

go_board_entrypoint_path() {
  if [[ -n "${PANEFLEET_GO_BOARD_BIN:-}" ]]; then
    printf '%s' "$PANEFLEET_GO_BOARD_BIN"
    return
  fi

  printf '%s/scripts/panefleet-go' "$PANEFLEET_SCRIPT_ROOT"
}

run_go_board() {
  require_tmux
  require_runtime_support

  local entrypoint
  entrypoint="$(go_board_entrypoint_path)"
  "${entrypoint}" tui --refresh "$(go_board_refresh_interval)"
}

go_board_popup_command() {
  local entrypoint command

  entrypoint="$(go_board_entrypoint_path)"
  printf -v command '%q %q --refresh %q' "$entrypoint" "tui" "$(go_board_refresh_interval)"
  printf '%s' "$command"
}

open_go_popup() {
  require_tmux
  require_runtime_support
  resolve_theme

  "${TMUX_BIN}" display-popup \
    -E \
    -w "$(board_popup_width)" \
    -h "$(board_popup_height)" \
    -s "$(theme_popup_style)" \
    -S "$(theme_popup_border_style)" \
    "$(go_board_popup_command)"
}
