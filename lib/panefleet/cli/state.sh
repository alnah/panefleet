#!/usr/bin/env bash

# State, metrics, doctor, and inspection command handlers.
# It validates required fields so bad adapter payloads fail fast and loudly.
state_set_command() {
  local pane_id="" status="" tool="" source="" updated_at=""

  while (($# > 0)); do
    case "$1" in
    --pane)
      pane_id="${2:-}"
      shift 2
      ;;
    --status)
      status="${2:-}"
      shift 2
      ;;
    --tool)
      tool="${2:-}"
      shift 2
      ;;
    --source)
      source="${2:-}"
      shift 2
      ;;
    --updated-at)
      updated_at="${2:-}"
      shift 2
      ;;
    *)
      printf 'unknown state-set option: %s\n' "$1" >&2
      exit 1
      ;;
    esac
  done

  if [[ -z "$status" ]]; then
    printf 'state-set requires --status\n' >&2
    exit 1
  fi

  pane_id="$(resolve_target_pane "$pane_id")"
  set_agent_state "$pane_id" "$status" "$tool" "$source" "$updated_at"
}

state_clear_command() {
  local pane_id=""

  while (($# > 0)); do
    case "$1" in
    --pane)
      pane_id="${2:-}"
      shift 2
      ;;
    *)
      printf 'unknown state-clear option: %s\n' "$1" >&2
      exit 1
      ;;
    esac
  done

  pane_id="$(resolve_target_pane "$pane_id")"
  clear_manual_state "$pane_id"
  clear_agent_state "$pane_id"
}

usage_metrics_set_command() {
  local pane_id=""
  local tokens_used=""
  local context_left_pct=""
  local context_window=""
  local clear_tokens_used="0"
  local clear_context_left_pct="0"
  local clear_context_window="0"

  while (($# > 0)); do
    case "$1" in
    --pane)
      pane_id="${2:-}"
      shift 2
      ;;
    --tokens-used)
      tokens_used="${2:-}"
      shift 2
      ;;
    --context-left-pct)
      context_left_pct="${2:-}"
      shift 2
      ;;
    --context-window)
      context_window="${2:-}"
      shift 2
      ;;
    --clear-tokens-used)
      clear_tokens_used="1"
      shift
      ;;
    --clear-context-left-pct)
      clear_context_left_pct="1"
      shift
      ;;
    --clear-context-window)
      clear_context_window="1"
      shift
      ;;
    *)
      printf 'unknown metrics-set option: %s\n' "$1" >&2
      exit 1
      ;;
    esac
  done

  if [[ -z "$tokens_used" && -z "$context_left_pct" && -z "$context_window" && "$clear_tokens_used" != "1" && "$clear_context_left_pct" != "1" && "$clear_context_window" != "1" ]]; then
    printf 'metrics-set requires at least one metric flag\n' >&2
    exit 1
  fi
  if [[ -n "$tokens_used" && ! "$tokens_used" =~ ^[0-9]+$ ]]; then
    printf 'metrics-set requires --tokens-used as non-negative integer\n' >&2
    exit 1
  fi
  if [[ -n "$context_left_pct" ]]; then
    if [[ ! "$context_left_pct" =~ ^[0-9]+$ ]] || ((context_left_pct < 0 || context_left_pct > 100)); then
      printf 'metrics-set requires --context-left-pct in range 0..100\n' >&2
      exit 1
    fi
  fi
  if [[ -n "$context_window" ]]; then
    if [[ ! "$context_window" =~ ^[0-9]+$ ]] || ((context_window <= 0)); then
      printf 'metrics-set requires --context-window as positive integer\n' >&2
      exit 1
    fi
  fi

  pane_id="$(resolve_target_pane "$pane_id")"
  set_usage_metrics "$pane_id" "$tokens_used" "$context_left_pct" "$context_window" "$clear_tokens_used" "$clear_context_left_pct" "$clear_context_window"
}

usage_metrics_clear_command() {
  local pane_id=""

  while (($# > 0)); do
    case "$1" in
    --pane)
      pane_id="${2:-}"
      shift 2
      ;;
    *)
      printf 'unknown metrics-clear option: %s\n' "$1" >&2
      exit 1
      ;;
    esac
  done

  pane_id="$(resolve_target_pane "$pane_id")"
  clear_usage_metrics "$pane_id"
}

# state_stale_command lets operators force STALE as a temporary override.
# The override is intentionally toggle-based to keep manual recovery quick.
state_stale_command() {
  local pane_id=""

  while (($# > 0)); do
    case "$1" in
    --pane)
      pane_id="${2:-}"
      shift 2
      ;;
    *)
      printf 'unknown state-stale option: %s\n' "$1" >&2
      exit 1
      ;;
    esac
  done

  pane_id="$(resolve_target_pane "$pane_id")"
  if [[ "$(manual_status "$pane_id")" == "STALE" ]]; then
    clear_manual_state "$pane_id"
  else
    set_manual_state "$pane_id" "STALE"
  fi
}

agent_freshness_label() {
  local updated_at="$1"
  local now="$2"
  local max_age="$3"

  if [[ -z "$updated_at" ]]; then
    printf 'none'
  elif agent_status_is_fresh "$updated_at" "$max_age" "$now"; then
    printf 'fresh'
  else
    printf 'stale'
  fi
}

state_inspect_values() {
  local pane_id="${1:?pane_id is required}"
  local session_name window_index window_name pane_index cmd title path dead dead_status activity signature local_status agent_status_value agent_tool_value agent_source_value agent_updated_at_value last_touch last_done cached_signature cached_raw_status cached_tool _tokens_used _context_left_pct

  require_runtime_support
  list_runtime_defaults
  IFS="$PANEFIELD_SEP" read -r pane_id session_name window_index window_name pane_index cmd title path dead dead_status activity signature local_status agent_status_value agent_tool_value agent_source_value agent_updated_at_value last_touch last_done cached_signature cached_raw_status cached_tool _tokens_used _context_left_pct <<<"$(full_pane_record "$pane_id")"
  resolve_list_row_values "$pane_id" "$cmd" "$title" "$dead" "$dead_status" "${activity:-0}" "$signature" "$local_status" "$agent_status_value" "$agent_tool_value" "$agent_updated_at_value" "$last_touch" "$last_done" "$cached_signature" "$cached_raw_status" "$cached_tool" "$agent_source_value"

  PANEFLEET_INSPECT_SESSION_NAME="$session_name"
  PANEFLEET_INSPECT_WINDOW_INDEX="$window_index"
  PANEFLEET_INSPECT_WINDOW_NAME="$window_name"
  PANEFLEET_INSPECT_PANE_INDEX="$pane_index"
  PANEFLEET_INSPECT_CMD="$cmd"
  PANEFLEET_INSPECT_TITLE="$title"
  PANEFLEET_INSPECT_PATH="$path"
  PANEFLEET_INSPECT_DEAD="$dead"
  PANEFLEET_INSPECT_DEAD_STATUS="$dead_status"
  PANEFLEET_INSPECT_ACTIVITY="$activity"
  PANEFLEET_INSPECT_SIGNATURE="$signature"
  PANEFLEET_INSPECT_MANUAL_STATUS="$local_status"
  PANEFLEET_INSPECT_AGENT_STATUS="$agent_status_value"
  PANEFLEET_INSPECT_AGENT_TOOL="$agent_tool_value"
  PANEFLEET_INSPECT_AGENT_SOURCE="$agent_source_value"
  PANEFLEET_INSPECT_AGENT_UPDATED_AT="$agent_updated_at_value"
  PANEFLEET_INSPECT_LAST_TOUCH="$last_touch"
  PANEFLEET_INSPECT_LAST_DONE="$last_done"
  PANEFLEET_INSPECT_CACHED_SIGNATURE="$cached_signature"
  PANEFLEET_INSPECT_CACHED_RAW_STATUS="$cached_raw_status"
  PANEFLEET_INSPECT_CACHED_TOOL="$cached_tool"
}

state_show_command() {
  local pane_id=""
  local activity_age updated_age touch_age done_age agent_fresh

  while (($# > 0)); do
    case "$1" in
    --pane)
      pane_id="${2:-}"
      shift 2
      ;;
    *)
      printf 'unknown state-show option: %s\n' "$1" >&2
      exit 1
      ;;
    esac
  done

  pane_id="$(resolve_target_pane "$pane_id")"
  state_inspect_values "$pane_id"
  pretty_age_into activity_age "$PANEFLEET_INSPECT_ACTIVITY" "$PANEFLEET_NOW"
  pretty_age_into updated_age "$PANEFLEET_INSPECT_AGENT_UPDATED_AT" "$PANEFLEET_NOW"
  pretty_age_into touch_age "$PANEFLEET_INSPECT_LAST_TOUCH" "$PANEFLEET_NOW"
  pretty_age_into done_age "$PANEFLEET_INSPECT_LAST_DONE" "$PANEFLEET_NOW"
  agent_fresh="$(agent_freshness_label "$PANEFLEET_INSPECT_AGENT_UPDATED_AT" "$PANEFLEET_NOW" "$PANEFLEET_AGENT_STATUS_MAX_AGE")"

  printf 'pane    %s\n' "$pane_id"
  printf 'target  %s:%s.%s\n' "$PANEFLEET_INSPECT_SESSION_NAME" "$PANEFLEET_INSPECT_WINDOW_INDEX" "$PANEFLEET_INSPECT_PANE_INDEX"
  printf 'window  %s\n' "$PANEFLEET_INSPECT_WINDOW_NAME"
  printf 'cmd     %s\n' "$PANEFLEET_INSPECT_CMD"
  printf 'title   %s\n' "$PANEFLEET_INSPECT_TITLE"
  printf 'path    %s\n' "${PANEFLEET_INSPECT_PATH/#$HOME/\~}"
  printf 'dead    %s\n' "$PANEFLEET_INSPECT_DEAD"
  printf 'exit    %s\n' "$PANEFLEET_INSPECT_DEAD_STATUS"
  printf '\n'
  printf 'final.status   %s\n' "$PANEFLEET_RESOLVED_STATUS"
  printf 'final.raw      %s\n' "$PANEFLEET_RESOLVED_RAW_STATUS"
  printf 'final.tool     %s\n' "$PANEFLEET_RESOLVED_TOOL"
  printf 'final.source   %s\n' "$PANEFLEET_RESOLVED_SOURCE"
  printf 'final.reason   %s\n' "$PANEFLEET_RESOLVED_REASON"
  printf '\n'
  printf 'manual.status  %s\n' "$PANEFLEET_INSPECT_MANUAL_STATUS"
  printf 'agent.status   %s\n' "$PANEFLEET_INSPECT_AGENT_STATUS"
  printf 'agent.tool     %s\n' "$PANEFLEET_INSPECT_AGENT_TOOL"
  printf 'agent.source   %s\n' "$PANEFLEET_INSPECT_AGENT_SOURCE"
  printf 'agent.at       %s\n' "$PANEFLEET_INSPECT_AGENT_UPDATED_AT"
  printf 'agent.fresh    %s\n' "$agent_fresh"
  printf '\n'
  printf 'cache.raw      %s\n' "$PANEFLEET_INSPECT_CACHED_RAW_STATUS"
  printf 'cache.tool     %s\n' "$PANEFLEET_INSPECT_CACHED_TOOL"
  printf 'cache.sig      %s\n' "$PANEFLEET_INSPECT_CACHED_SIGNATURE"
  printf 'live.sig       %s\n' "$PANEFLEET_INSPECT_SIGNATURE"
  printf '\n'
  printf 'age.activity   %s\n' "$activity_age"
  printf 'age.agent      %s\n' "$updated_age"
  printf 'age.touch      %s\n' "$touch_age"
  printf 'age.done       %s\n' "$done_age"
}

render_state_snapshot() {
  local pane_id session_name window_index window_name pane_index cmd title path dead dead_status activity signature local_status agent_status_value agent_tool_value agent_source_value agent_updated_at_value last_touch last_done cached_signature cached_raw_status cached_tool _tokens_used _context_left_pct
  local rows_file tool status fresh updated_age target task

  rows_file="$(mktemp "${TMPDIR:-/tmp}/panefleet-state-snapshot.XXXXXX")"

  while IFS="$PANEFIELD_SEP" read -r pane_id session_name window_index window_name pane_index cmd title path dead dead_status activity signature local_status agent_status_value agent_tool_value agent_source_value agent_updated_at_value last_touch last_done cached_signature cached_raw_status cached_tool _tokens_used _context_left_pct; do
    resolve_list_row_values "$pane_id" "$cmd" "$title" "$dead" "$dead_status" "${activity:-0}" "$signature" "$local_status" "$agent_status_value" "$agent_tool_value" "$agent_updated_at_value" "$last_touch" "$last_done" "$cached_signature" "$cached_raw_status" "$cached_tool" "$agent_source_value"
    tool="$PANEFLEET_RESOLVED_TOOL"
    status="$PANEFLEET_RESOLVED_STATUS"
    fresh="$(agent_freshness_label "$agent_updated_at_value" "$PANEFLEET_NOW" "$PANEFLEET_AGENT_STATUS_MAX_AGE")"
    pretty_age_into updated_age "$agent_updated_at_value" "$PANEFLEET_NOW"
    target="${session_name}:${window_index}.${pane_index}"
    task_label_into task "$cmd" "$title" "$window_name" "${path##*/}"
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$pane_id" \
      "$status" \
      "$PANEFLEET_RESOLVED_RAW_STATUS" \
      "$PANEFLEET_RESOLVED_SOURCE" \
      "$tool" \
      "$fresh" \
      "$updated_age" \
      "$target" \
      "$task" \
      "$PANEFLEET_RESOLVED_REASON" >>"$rows_file"
  done < <(pane_records "$(list_pane_record_format)")

  printf '%s\n' "$rows_file"
}

print_state_list_snapshot() {
  local rows_file="$1"

  {
    printf 'PANE\tSTATE\tRAW\tSOURCE\tTOOL\tAGENT\tUPDATED\tTARGET\tTASK\tREASON\n'
    cat "$rows_file"
  } | awk -F '\t' '
    NR == 1 {
      printf "%-7s %-6s %-6s %-16s %-10s %-7s %-8s %-18s %-18s %s\n", $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
      next
    }
    {
      printf "%-7s %-6s %-6s %-16s %-10s %-7s %-8s %-18s %-18s %s\n", $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
    }
  '
}

state_list_command() {
  local rows_file

  require_runtime_support
  list_runtime_defaults
  rows_file="$(render_state_snapshot)"
  print_state_list_snapshot "$rows_file"
  rm -f "$rows_file"
}

print_state_counts_snapshot() {
  local rows_file="$1"

  cut -f2 "$rows_file" | sort | uniq -c | awk '{ printf "  %-5s %s\n", $2, $1 }'
}

doctor_command() {
  local verbose="0"
  local install_view="0"
  local pane_count rows_file
  local tmux_bin_path fzf_bin_path rg_bin_path root bridge_bin touch_hook adapter_default bun_bin_path

  while (($# > 0)); do
    case "$1" in
    --verbose)
      verbose="1"
      shift
      ;;
    --install)
      install_view="1"
      shift
      ;;
    *)
      printf 'unknown doctor option: %s\n' "$1" >&2
      exit 1
      ;;
    esac
  done

  preflight
  require_tmux
  resolve_theme
  resolve_color_mode
  list_runtime_defaults
  root="$(self_root)"
  bridge_bin="$(bridge_bin_path)"
  touch_hook="$(touch_hook_command_value)"

  tmux_bin_path="$(command -v "${TMUX_BIN}" 2>/dev/null || printf '%s' "${TMUX_BIN}")"
  fzf_bin_path="$(command -v "${FZF_BIN}" 2>/dev/null || printf '%s' "${FZF_BIN}")"
  rg_bin_path="$(command -v "${RG_BIN}" 2>/dev/null || printf '%s' "${RG_BIN}")"
  bun_bin_path="$(command -v bun 2>/dev/null || printf 'missing')"
  if [[ "$install_view" == "1" ]]; then
    adapter_default="heuristic-only"
    if adapters_enabled; then
      adapter_default="auto"
    fi
    printf 'self.root        %s\n' "$root"
    printf 'self.bin         %s\n' "$SELF"
    printf 'install.script   %s\n' "$(install_script_path)"
    printf 'bridge.bin       %s\n' "$bridge_bin"
    printf 'bridge.present   %s\n' "$([[ -x "$bridge_bin" ]] && printf 'yes' || printf 'no')"
    printf 'bun.bin          %s\n' "$bun_bin_path"
    printf 'integration.codex   %s\n' "$(integration_status_codex)"
    printf 'integration.claude  %s\n' "$(integration_status_claude)"
    printf 'integration.opencode %s\n' "$(integration_status_opencode)"
    printf 'codex.config    %s\n' "$(codex_config_path)"
    printf 'claude.settings %s\n' "$(claude_settings_path)"
    printf 'opencode.plugin  %s\n' "$(opencode_plugin_path)"
    printf 'adapter.default  %s\n' "$adapter_default"
    printf 'key.prefix.P     %s\n' "$(binding_state P)"
    printf 'key.prefix.T     %s\n' "$(binding_state T)"
    printf 'hook.select-pane %s\n' "$(matching_hook_count after-select-pane "$touch_hook")"
    printf 'hook.select-win  %s\n' "$(matching_hook_count after-select-window "$touch_hook")"
    printf 'hook.session     %s\n' "$(matching_hook_count client-session-changed "$touch_hook")"
    printf 'hook.client      %s\n' "$(matching_hook_count client-active "$touch_hook")"
    return
  fi

  pane_count="$(pane_records "$(pane_record_format)" | wc -l | tr -d ' ')"

  printf 'tmux.bin        %s\n' "$tmux_bin_path"
  printf 'fzf.bin         %s\n' "$fzf_bin_path"
  printf 'rg.bin          %s\n' "$rg_bin_path"
  printf 'theme           %s\n' "$THEME_NAME"
  printf 'color.mode      %s\n' "$PANEFLEET_COLOR_MODE_RESOLVED"
  printf 'adapter.mode    %s\n' "$([[ "$PANEFLEET_ADAPTERS_ENABLED" == "1" ]] && printf 'enabled' || printf 'heuristic-only')"
  printf 'pane.count      %s\n' "$pane_count"
  printf 'done.minutes    %s\n' "$PANEFLEET_DONE_RECENT_MINUTES"
  printf 'stale.minutes   %s\n' "$PANEFLEET_STALE_MINUTES"
  printf 'agent.max_age   %s\n' "$PANEFLEET_AGENT_STATUS_MAX_AGE"
  printf 'runtime.logs    %s\n' "$(runtime_log_dir)"
  printf 'event.logs      %s\n' "${PANEFLEET_EVENT_LOG_DIR:-}"
  rows_file="$(render_state_snapshot)"

  printf '\nstate counts\n'
  print_state_counts_snapshot "$rows_file"

  if [[ "$verbose" == "1" ]]; then
    printf '\nstate list\n'
    print_state_list_snapshot "$rows_file"
  fi

  rm -f "$rows_file"
}

# main keeps CLI routing explicit to preserve a stable command contract for
# wrappers, tests, and tmux hook invocations.
