#!/usr/bin/env bash

# Runtime capability checks for tmux, fzf, and ripgrep.
require_tmux() {
  if [[ -z "${TMUX:-}" ]]; then
    printf 'panefleet must run inside tmux\n' >&2
    exit 1
  fi
}

tmux_supports_popup() {
  command_exists "${TMUX_BIN}" && "${TMUX_BIN}" list-commands 2>/dev/null | grep -q '^display-popup'
}

fzf_supports_header_border() {
  command_exists "${FZF_BIN}" && "${FZF_BIN}" --help 2>/dev/null | grep -Fq -- '--header-lines-border'
}

fzf_supports_bind_action() {
  local bind_expression="${1:?bind expression is required}"

  if ! command_exists "${FZF_BIN}"; then
    return 1
  fi

  printf 'x\n' | "${FZF_BIN}" --filter 'x' --bind "$bind_expression" >/dev/null 2>&1
}

fzf_supports_reload_sync() {
  fzf_supports_bind_action 'start:reload-sync(printf "x\n")'
}

fzf_supports_padding() {
  command_exists "${FZF_BIN}" && "${FZF_BIN}" --help 2>/dev/null | grep -Fq -- '--padding'
}

fzf_supports_result_event() {
  fzf_supports_bind_action 'result:abort'
}

preflight() {
  local quiet="${1:-}"
  local ok=0
  local issues=()

  if ! command_exists "${TMUX_BIN}"; then
    issues+=("tmux introuvable")
  elif ! tmux_supports_popup; then
    issues+=("tmux ne supporte pas display-popup")
  fi

  if ! command_exists "${FZF_BIN}"; then
    issues+=("fzf introuvable")
  elif ! fzf_supports_header_border; then
    issues+=("fzf est trop ancien pour --header-lines-border")
  fi

  if ! command_exists "${RG_BIN}"; then
    issues+=("ripgrep (rg) introuvable")
  fi

  if ((${#issues[@]} > 0)); then
    ok=1
    if [[ "$quiet" != "--quiet" ]]; then
      printf 'panefleet preflight: échec\n' >&2
      printf ' - %s\n' "${issues[@]}" >&2
    fi
    return "$ok"
  fi

  if [[ "$quiet" != "--quiet" ]]; then
    printf 'panefleet preflight: ok\n'
  fi
}

# require_runtime_support is used by interactive commands so users get one clear
# actionable error instead of partial rendering failures later in the flow.
require_runtime_support() {
  if ! preflight --quiet; then
    printf 'panefleet preflight failed; run %s preflight\n' "$SELF" >&2
    exit 1
  fi
}
