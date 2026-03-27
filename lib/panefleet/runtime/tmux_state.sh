#!/usr/bin/env bash

# tmux pane state access, heuristics, cache refresh, and adapter resolution.
tmux_global_option() {
  local name="$1"
  "${TMUX_BIN}" show-options -gqv "$name" 2>/dev/null || true
}

pane_option() {
  local pane_id="$1"
  local name="$2"
  "${TMUX_BIN}" show-options -pt "$pane_id" -v "$name" 2>/dev/null || true
}

pane_records() {
  local format="$1"

  "${TMUX_BIN}" list-panes -a -F "$format" | awk -v OFS="$PANEFIELD_SEP" '{ gsub(/\t/, OFS); print }'
}

pane_record_format() {
  printf '%s' '#{pane_id}	#{session_name}	#{window_index}	#{window_name}	#{pane_index}	#{pane_current_command}	#{pane_title}	#{pane_current_path}	#{pane_dead}	#{pane_dead_status}	#{window_activity}'
}

list_pane_record_format() {
  printf '%s\t%s\t%s\t%s\n' \
    "$(pane_record_format)" \
    'stable:#{history_size}:#{cursor_x}:#{cursor_y}:#{pane_current_command}:#{pane_title}:#{pane_dead}:#{pane_dead_status}	#{@panefleet_status}	#{@panefleet_agent_status}	#{@panefleet_agent_tool}	#{@panefleet_agent_source}	#{@panefleet_agent_updated_at}	#{@panefleet_last_touch}	#{@panefleet_last_done}	#{@panefleet_auto_signature}	#{@panefleet_auto_raw_status}	#{@panefleet_auto_tool}' \
    '#{@panefleet_tokens_used}' \
    '#{@panefleet_context_left_pct}'
}

cache_refresh_record_format() {
  printf '%s\t%s\n' \
    "$(pane_record_format)" \
    'stable:#{history_size}:#{cursor_x}:#{cursor_y}:#{pane_current_command}:#{pane_title}:#{pane_dead}:#{pane_dead_status}	#{@panefleet_last_done}'
}

preview_pane_record() {
  local pane_id="$1"

  "${TMUX_BIN}" display-message -p -t "$pane_id" "$(pane_record_format)" | awk -v OFS="$PANEFIELD_SEP" '{ gsub(/\t/, OFS); print }'
}

full_pane_record() {
  local pane_id="$1"

  "${TMUX_BIN}" display-message -p -t "$pane_id" "$(list_pane_record_format)" | awk -v OFS="$PANEFIELD_SEP" '{ gsub(/\t/, OFS); print }'
}

runtime_log_dir() {
  printf '%s' "${PANEFLEET_RUNTIME_LOG_DIR:-}"
}

logfmt_value() {
  printf '%q' "${1:-}"
}

runtime_log_event() {
  local event="${1:?event is required}"
  shift
  local log_dir path

  log_dir="$(runtime_log_dir)"
  if [[ -z "$log_dir" ]]; then
    return
  fi

  mkdir -p "$log_dir"
  chmod 700 "$log_dir" 2>/dev/null || true
  path="${log_dir%/}/runtime.log"

  {
    printf 'ts=%q event=%q' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "$event"
    while (($# > 1)); do
      printf ' %s=%s' "$1" "$(logfmt_value "$2")"
      shift 2
    done
    printf '\n'
  } >>"$path"

  chmod 600 "$path" 2>/dev/null || true
}

set_pane_option() {
  local pane_id="$1"
  local name="$2"
  local value="$3"
  "${TMUX_BIN}" set-option -pt "$pane_id" -q "$name" "$value"
}

unset_pane_option() {
  local pane_id="$1"
  local name="$2"
  "${TMUX_BIN}" set-option -pt "$pane_id" -u "$name"
}

set_cached_state() {
  local pane_id="$1"
  local signature="$2"
  local raw_status="$3"
  local tool="$4"

  set_pane_option "$pane_id" @panefleet_auto_signature "$signature"
  set_pane_option "$pane_id" @panefleet_auto_raw_status "$raw_status"
  set_pane_option "$pane_id" @panefleet_auto_tool "$tool"
}

cache_refresh_needed() {
  local local_status="$1"
  local cached_signature="$2"
  local signature="$3"
  local cached_raw_status="$4"

  if [[ -n "$local_status" ]]; then
    return 1
  fi

  if [[ -z "$cached_raw_status" ]]; then
    return 0
  fi

  [[ "$cached_signature" != "$signature" ]]
}

fallback_raw_status() {
  local cached_raw_status="$1"
  local tool="$2"
  local cmd="$3"
  local dead="$4"
  local dead_status="$5"

  if [[ -n "$cached_raw_status" ]]; then
    printf '%s' "$cached_raw_status"
  elif [[ "$dead" == "1" ]]; then
    inferred_status "$cmd" "$dead" "$dead_status"
  elif [[ "$tool" == "shell" ]]; then
    printf 'IDLE'
  else
    printf 'IDLE'
  fi
}

manual_status() {
  local pane_id="$1"
  pane_option "$pane_id" @panefleet_status
}

agent_status() {
  local pane_id="$1"
  pane_option "$pane_id" @panefleet_agent_status
}

agent_tool() {
  local pane_id="$1"
  pane_option "$pane_id" @panefleet_agent_tool
}

agent_source() {
  local pane_id="$1"
  pane_option "$pane_id" @panefleet_agent_source
}

agent_updated_at() {
  local pane_id="$1"
  pane_option "$pane_id" @panefleet_agent_updated_at
}

valid_epoch_seconds() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

agent_status_max_age_seconds() {
  local max_age

  max_age="$(tmux_global_option @panefleet-agent-status-max-age-seconds)"
  if ! valid_epoch_seconds "$max_age"; then
    max_age="600"
  fi
  printf '%s' "$max_age"
}

adapters_enabled() {
  local mode

  mode="$(tmux_global_option @panefleet-adapter-mode)"
  case "$mode" in
  auto | on | enabled)
    return 0
    ;;
  "" | heuristic-only | heuristic | off | disabled)
    return 1
    ;;
  *)
    return 0
    ;;
  esac
}

agent_state_matches_live_tool() {
  local live_tool="$1"
  local agent_tool_value="$2"

  if [[ -z "$agent_tool_value" ]]; then
    return 0
  fi

  case "$live_tool" in
  codex | claude | opencode)
    [[ "$agent_tool_value" == "$live_tool" ]]
    return
    ;;
  shell)
    [[ "$agent_tool_value" == "shell" ]]
    return
    ;;
  *)
    return 0
    ;;
  esac
}

agent_status_is_fresh() {
  local updated_at="$1"
  local max_age="$2"
  local now="$3"
  local age

  if ! valid_epoch_seconds "$updated_at"; then
    return 1
  fi
  if ! valid_epoch_seconds "$max_age"; then
    return 1
  fi
  if ((max_age <= 0)); then
    return 0
  fi

  age=$((now - updated_at))
  ((age >= 0 && age <= max_age))
}

resolve_target_pane() {
  local target="${1:-${TMUX_PANE:-}}"

  if [[ -n "$target" ]]; then
    printf '%s' "$target"
    return
  fi

  require_tmux
  "${TMUX_BIN}" display-message -p '#{pane_id}'
}

validate_status_name() {
  case "$1" in
  RUN | WAIT | DONE | IDLE | ERROR | STALE) ;;
  *)
    printf 'unsupported status: %s\n' "$1" >&2
    exit 1
    ;;
  esac
}

set_agent_state() {
  local pane_id="$1"
  local status="$2"
  local tool="${3:-}"
  local source="${4:-}"
  local updated_at="${5:-}"

  validate_status_name "$status"
  if [[ -z "$updated_at" ]]; then
    updated_at="$(now_epoch)"
  elif ! valid_epoch_seconds "$updated_at"; then
    printf 'invalid updated-at: %s\n' "$updated_at" >&2
    exit 1
  fi

  set_pane_option "$pane_id" @panefleet_agent_status "$status"
  set_pane_option "$pane_id" @panefleet_agent_updated_at "$updated_at"

  if [[ -n "$tool" ]]; then
    set_pane_option "$pane_id" @panefleet_agent_tool "$tool"
  else
    unset_pane_option "$pane_id" @panefleet_agent_tool 2>/dev/null || true
  fi

  if [[ -n "$source" ]]; then
    set_pane_option "$pane_id" @panefleet_agent_source "$source"
  else
    unset_pane_option "$pane_id" @panefleet_agent_source 2>/dev/null || true
  fi

  case "$status" in
  DONE)
    set_pane_option "$pane_id" @panefleet_last_done "$updated_at"
    ;;
  RUN | WAIT | ERROR)
    unset_pane_option "$pane_id" @panefleet_last_done 2>/dev/null || true
    ;;
  esac

  runtime_log_event "agent_state_set" \
    pane "$pane_id" \
    status "$status" \
    tool "$tool" \
    source "$source" \
    updated_at "$updated_at"
}

set_manual_state() {
  local pane_id="$1"
  local status="$2"

  validate_status_name "$status"
  set_pane_option "$pane_id" @panefleet_status "$status"
  runtime_log_event "manual_state_set" pane "$pane_id" status "$status"
}

clear_manual_state() {
  local pane_id="$1"

  unset_pane_option "$pane_id" @panefleet_status 2>/dev/null || true
  runtime_log_event "manual_state_cleared" pane "$pane_id"
}

clear_agent_state() {
  local pane_id="$1"

  unset_pane_option "$pane_id" @panefleet_agent_status 2>/dev/null || true
  unset_pane_option "$pane_id" @panefleet_agent_tool 2>/dev/null || true
  unset_pane_option "$pane_id" @panefleet_agent_source 2>/dev/null || true
  unset_pane_option "$pane_id" @panefleet_agent_updated_at 2>/dev/null || true
  runtime_log_event "agent_state_cleared" pane "$pane_id"
}

set_usage_metrics() {
  local pane_id="$1"
  local tokens_used="${2:-}"
  local context_left_pct="${3:-}"
  local context_window="${4:-}"
  local clear_tokens="${5:-0}"
  local clear_context_left="${6:-0}"
  local clear_context_window="${7:-0}"

  if [[ -n "$tokens_used" ]]; then
    set_pane_option "$pane_id" @panefleet_tokens_used "$tokens_used"
  elif [[ "$clear_tokens" == "1" ]]; then
    unset_pane_option "$pane_id" @panefleet_tokens_used 2>/dev/null || true
  fi
  if [[ -n "$context_left_pct" ]]; then
    set_pane_option "$pane_id" @panefleet_context_left_pct "$context_left_pct"
  elif [[ "$clear_context_left" == "1" ]]; then
    unset_pane_option "$pane_id" @panefleet_context_left_pct 2>/dev/null || true
  fi
  if [[ -n "$context_window" ]]; then
    set_pane_option "$pane_id" @panefleet_context_window "$context_window"
  elif [[ "$clear_context_window" == "1" ]]; then
    unset_pane_option "$pane_id" @panefleet_context_window 2>/dev/null || true
  fi
  advance_refresh_generation
  runtime_log_event "usage_metrics_set" pane "$pane_id" tokens "$tokens_used" ctx_left_pct "$context_left_pct" ctx_window "$context_window" clear_tokens "$clear_tokens" clear_ctx_left "$clear_context_left" clear_ctx_window "$clear_context_window"
}

clear_usage_metrics() {
  local pane_id="$1"

  unset_pane_option "$pane_id" @panefleet_tokens_used 2>/dev/null || true
  unset_pane_option "$pane_id" @panefleet_context_left_pct 2>/dev/null || true
  unset_pane_option "$pane_id" @panefleet_context_window 2>/dev/null || true
  advance_refresh_generation
  runtime_log_event "usage_metrics_cleared" pane "$pane_id"
}

pane_recent_capture() {
  local pane_id="$1"
  "${TMUX_BIN}" capture-pane -p -t "$pane_id" -S -30
}

pane_pid_value() {
  local pane_id="$1"
  "${TMUX_BIN}" display-message -p -t "$pane_id" '#{pane_pid}' 2>/dev/null || true
}

process_child_pids() {
  local pid="$1"

  if [[ -z "$pid" ]]; then
    return
  fi

  pgrep -P "$pid" 2>/dev/null || true
}

process_command_name() {
  local pid="$1"

  if [[ -z "$pid" ]]; then
    return
  fi

  ps -o comm= -p "$pid" 2>/dev/null | awk 'NR == 1 { print $1 }'
}

process_has_children() {
  local pid="$1"

  [[ -n "$pid" ]] && pgrep -P "$pid" >/dev/null 2>&1
}

deepest_codex_pid_for_pane() {
  local pane_id="$1"
  local current next_pid pid comm

  current="$(pane_pid_value "$pane_id")"
  if [[ -z "$current" ]]; then
    return
  fi

  while :; do
    next_pid=""
    while IFS= read -r pid; do
      [[ -z "$pid" ]] && continue
      comm="$(process_command_name "$pid")"
      case "$comm" in
      codex | codex-aarch64-a)
        next_pid="$pid"
        ;;
      esac
    done < <(process_child_pids "$current")

    if [[ -n "$next_pid" ]]; then
      current="$next_pid"
    else
      break
    fi
  done

  comm="$(process_command_name "$current")"
  case "$comm" in
  codex | codex-aarch64-a)
    printf '%s' "$current"
    ;;
  esac
}

codex_process_is_working() {
  local pane_id="$1"
  local codex_pid

  codex_pid="$(deepest_codex_pid_for_pane "$pane_id")"
  process_has_children "$codex_pid"
}

pane_visible_capture() {
  local pane_id="$1"
  "${TMUX_BIN}" capture-pane -p -t "$pane_id"
}

preview_body_capture() {
  local pane_id="$1"
  local visible_capture body_lines

  visible_capture="$(pane_visible_capture "$pane_id")"
  body_lines="${FZF_PREVIEW_LINES:-0}"
  if [[ "$body_lines" =~ ^[0-9]+$ ]]; then
    body_lines=$((body_lines - 9))
  else
    body_lines=0
  fi

  if ((body_lines < 1)); then
    printf '%s\n' "$visible_capture"
  else
    printf '%s\n' "$visible_capture" | awk -v body_lines="$body_lines" '
      { lines[++n] = $0 }
      function visually_empty(line, tmp) {
        tmp = line
        gsub(/[[:space:]┃╹▀█▌▐▄▁▔▕▏]/, "", tmp)
        return tmp == ""
      }
      END {
        end = n
        start = n - body_lines + 1
        if (start < 1) {
          start = 1
        }
        while (start > 1 && end > start && visually_empty(lines[start])) {
          start--
          end--
        }
        for (i = start; i <= end; i++) {
          print lines[i]
        }
      }
    '
  fi
}

contains_line() {
  local haystack="$1"
  local pattern="$2"
  printf '%s\n' "$haystack" | "${RG_BIN}" -qi -- "$pattern"
}

codex_is_error() {
  local capture="$1"

  contains_line "$capture" 'permission denied|access denied|fatal:|traceback|exception|command failed|request failed'
}

codex_is_working() {
  local capture="$1"

  contains_line "$capture" 'Working \([0-9]+[smhd]|esc to interrupt|background terminal[s]? running'
}

codex_is_waiting() {
  local capture="$1"

  contains_line "$capture" 'Enter to confirm|Esc to cancel|waiting on approval|approval required|select model|choose model|model to change|permission mode|approval mode|/permissions|/models'
}

claude_is_error() {
  local capture="$1"

  contains_line "$capture" 'permission denied|access denied|fatal:|traceback|exception|command failed|tool failed'
}

claude_is_working() {
  local capture="$1"

  contains_line "$capture" '^\s*⏺ |Bash\(|Read\(|Write\(|Edit\(|MultiEdit\(|Glob\(|Grep\(|LS\(|Task\(|Running|Processing'
}

claude_is_waiting() {
  local capture="$1"

  contains_line "$capture" 'Enter to confirm|Esc to cancel|Do you want to|approval required|waiting on approval|choose an option|select an option|Yes, proceed|No, cancel|/permissions|Allow Ask Deny|Press .*navigate|allow all edits|allow once|deny once|deny all'
}

claude_is_done() {
  local capture="$1"

  printf '%s\n' "$capture" | "${RG_BIN}" -q '^\s*❯\s*$'
}

opencode_is_working() {
  local capture="$1"

  contains_line "$capture" 'esc interrupt|Thinking:|Preparing patch|Applying patch|Running|Generating|Writing|Tool execution'
}

opencode_is_waiting() {
  local capture="$1"

  contains_line "$capture" 'Enter to confirm|Esc to cancel|approval|required|permission|confirm|cancel|select model|select variant'
}

opencode_is_done() {
  local capture="$1"

  contains_line "$capture" 'Ask anything\.\.\.|ctrl\+p.*commands|tab.*agents'
}

claude_focus_capture() {
  local capture="$1"

  printf '%s\n' "$capture" | tail -n 12
}

opencode_is_error() {
  local capture="$1"

  contains_line "$capture" 'permission denied|access denied|fatal:|traceback|exception|command failed|request failed'
}

tool_kind() {
  local cmd="$1"
  local title="$2"
  local cmd_lower title_lower

  cmd_lower="$(printf '%s' "$cmd" | tr '[:upper:]' '[:lower:]')"
  title_lower="$(printf '%s' "$title" | tr '[:upper:]' '[:lower:]')"

  case "${cmd_lower}:${title_lower}" in
  codex*:*)
    printf 'codex'
    ;;
  claude*:* | *:*claude*)
    printf 'claude'
    ;;
  opencode:* | open-code:* | *:*opencode*)
    printf 'opencode'
    ;;
  sh:* | bash:* | zsh:* | fish:* | nu:* | tmux:*)
    printf 'shell'
    ;;
  *)
    printf '%s' "$cmd"
    ;;
  esac
}

tool_from_capture() {
  local capture="$1"

  if contains_line "$capture" 'OpenAI Codex|/model to change|gpt-5\.[0-9].*left'; then
    printf 'codex'
  elif contains_line "$capture" 'ANTHROPIC_API_KEY|Claude Code|Do you want to use this API key'; then
    printf 'claude'
  elif contains_line "$capture" 'Ask anything\.\.\.|OpenCode|ctrl\+p.*commands|tab.*agents'; then
    printf 'opencode'
  else
    printf 'unknown'
  fi
}

refresh_pane_cache_metadata() {
  local pane_id="$1"
  local activity="$2"
  local signature="$3"
  local cmd="$4"
  local title="$5"
  local dead="$6"
  local dead_status="$7"
  local last_done="${8:-}"

  refresh_pane_cache_core "$pane_id" "$activity" "$signature" "$cmd" "$title" "$dead" "$dead_status" "$last_done"
}

refresh_pane_cache_record() {
  local pane_id="$1"
  local activity="$2"
  local signature="$3"
  local cmd="$4"
  local title="$5"
  local dead="$6"
  local dead_status="$7"
  local last_done="${8:-}"

  refresh_pane_cache_core "$pane_id" "$activity" "$signature" "$cmd" "$title" "$dead" "$dead_status" "$last_done"
  printf '%s%s%s%s%s\n' "$PANEFLEET_RESOLVED_TOOL" "$PANEFIELD_SEP" "$PANEFLEET_RESOLVED_RAW_STATUS" "$PANEFIELD_SEP" "$PANEFLEET_RESOLVED_LAST_DONE"
}

refresh_pane_cache_core() {
  local pane_id="$1"
  local activity="$2"
  local signature="$3"
  local cmd="$4"
  local title="$5"
  local dead="$6"
  local dead_status="$7"
  local last_done="${8:-}"
  local now tool capture raw_status normalized_last_done
  local live_record live_tool live_raw _live_source _live_reason

  tool="$(tool_kind "$cmd" "$title")"
  capture="$(pane_recent_capture "$pane_id")"
  live_record="$(resolve_live_raw_state_record "$pane_id" "$tool" "$cmd" "$title" "$dead" "$dead_status" "$capture")"
  IFS="$PANEFIELD_SEP" read -r live_tool live_raw _live_source _live_reason <<<"$live_record"
  tool="$live_tool"
  raw_status="$live_raw"
  normalized_last_done="$last_done"

  set_cached_state "$pane_id" "$signature" "$raw_status" "$tool"

  case "$raw_status" in
  DONE)
    now="$(now_epoch)"
    normalized_last_done="$(normalized_last_done_value "$last_done" "${activity:-0}" "$now")"
    if [[ "$normalized_last_done" != "$last_done" ]]; then
      set_pane_option "$pane_id" @panefleet_last_done "$normalized_last_done"
    fi
    ;;
  RUN | WAIT | ERROR)
    if [[ -n "$last_done" && "$last_done" != "0" ]]; then
      unset_pane_option "$pane_id" @panefleet_last_done 2>/dev/null || true
    fi
    normalized_last_done=""
    ;;
  esac

  PANEFLEET_RESOLVED_TOOL="$tool"
  PANEFLEET_RESOLVED_RAW_STATUS="$raw_status"
  PANEFLEET_RESOLVED_LAST_DONE="$normalized_last_done"
  runtime_log_event "heuristic_cache_refresh" \
    pane "$pane_id" \
    tool "$tool" \
    raw_status "$raw_status" \
    signature "$signature"
}

refresh_panes_cache() {
  local filter_set=""
  local pane_id session_name window_index window_name pane_index cmd title path dead dead_status activity signature last_done

  require_runtime_support

  if (($# > 0)); then
    for pane_id in "$@"; do
      filter_set+=$'\n'"$pane_id"
    done
    filter_set+=$'\n'
  fi

  while IFS="$PANEFIELD_SEP" read -r pane_id session_name window_index window_name pane_index cmd title path dead dead_status activity signature last_done; do
    if [[ -n "$filter_set" && "$filter_set" != *$'\n'"$pane_id"$'\n'* ]]; then
      continue
    fi

    refresh_pane_cache_metadata "$pane_id" "${activity:-0}" "$signature" "$cmd" "$title" "$dead" "$dead_status" "$last_done"
  done < <(pane_records "$(cache_refresh_record_format)")
}

refresh_generation_file() {
  printf '%s/panefleet/refresh-cache.generation' "$(user_state_home)"
}

current_refresh_generation() {
  local generation_file value

  generation_file="$(refresh_generation_file)"
  value="$(cat "$generation_file" 2>/dev/null || printf '0')"
  if ! [[ "$value" =~ ^[0-9]+$ ]]; then
    value="0"
  fi
  printf '%s' "$value"
}

advance_refresh_generation() {
  local generation_file generation_dir current next tmp

  generation_file="$(refresh_generation_file)"
  generation_dir="$(dirname "$generation_file")"
  mkdir -p "$generation_dir" 2>/dev/null || return
  current="$(current_refresh_generation)"
  next=$((current + 1))
  tmp="$(mktemp "${TMPDIR:-/tmp}/panefleet-refresh-generation.XXXXXX")"
  printf '%s\n' "$next" >"$tmp"
  mv "$tmp" "$generation_file"
}

run_refresh_request() {
  local scope="$1"
  shift
  local -a pane_ids=("$@")

  if [[ "$scope" == "panes" && ${#pane_ids[@]} -gt 0 ]]; then
    "$SELF" refresh-panes-cache "${pane_ids[@]}" >/dev/null 2>&1
    advance_refresh_generation
    return
  fi

  "$SELF" refresh-panes-cache >/dev/null 2>&1
  advance_refresh_generation
}

write_pending_refresh_request() {
  local pending_file="$1"
  local scope="$2"
  shift 2
  local pending_tmp pane_id

  pending_tmp="$(mktemp "${TMPDIR:-/tmp}/panefleet-refresh-pending.XXXXXX")"
  {
    printf '%s\n' "$scope"
    for pane_id in "$@"; do
      printf '%s\n' "$pane_id"
    done
  } >"$pending_tmp"
  mv "$pending_tmp" "$pending_file"
}

load_pending_refresh_request() {
  local pending_file="$1"
  local line
  local first_line="1"

  PANEFLEET_PENDING_SCOPE="all"
  PANEFLEET_PENDING_ARGS=()

  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$first_line" == "1" ]]; then
      if [[ "$line" == "panes" ]]; then
        PANEFLEET_PENDING_SCOPE="panes"
      else
        PANEFLEET_PENDING_SCOPE="all"
      fi
      first_line="0"
      continue
    fi

    if [[ -n "$line" ]]; then
      PANEFLEET_PENDING_ARGS+=("$line")
    fi
  done <"$pending_file"
}

queue_refresh_request() {
  local scope="$1"
  shift
  local queue_root lock_dir lock_parent pending_file
  local -a pane_ids=("$@")
  local pending_scope="all"
  local -a pending_args=()

  queue_root="$(user_state_home)/panefleet"
  lock_dir="${queue_root}/refresh-cache.lock"
  pending_file="${queue_root}/refresh-cache.pending"
  lock_parent="$(dirname "$lock_dir")"

  if ! mkdir -p "$lock_parent" 2>/dev/null; then
    (run_refresh_request "$scope" "${pane_ids[@]}") &
    return
  fi

  if ! mkdir "$lock_dir" 2>/dev/null; then
    write_pending_refresh_request "$pending_file" "$scope" "${pane_ids[@]}"
    return
  fi

  (
    trap 'rmdir "$lock_dir" 2>/dev/null || true' EXIT

    run_refresh_request "$scope" "${pane_ids[@]}"
    while [[ -s "$pending_file" ]]; do
      load_pending_refresh_request "$pending_file"
      pending_scope="$PANEFLEET_PENDING_SCOPE"
      pending_args=("${PANEFLEET_PENDING_ARGS[@]}")
      rm -f "$pending_file"
      run_refresh_request "$pending_scope" "${pending_args[@]}"
    done
  ) &
}

queue_refresh_command() {
  local scope="all"
  local pane_id
  local -a pane_ids=()

  while (($# > 0)); do
    case "$1" in
    --all)
      scope="all"
      pane_ids=()
      shift
      ;;
    --pane)
      if (($# < 2)); then
        printf 'queue-refresh requires value after --pane\n' >&2
        exit 1
      fi
      pane_id="${2:-}"
      if [[ -z "$pane_id" || "$pane_id" == "__header__" ]]; then
        shift 2
        continue
      fi
      scope="panes"
      pane_ids+=("$pane_id")
      shift 2
      ;;
    *)
      printf 'unknown queue-refresh option: %s\n' "$1" >&2
      exit 1
      ;;
    esac
  done

  runtime_log_event "heuristic_refresh_queued" scope "$scope" count "${#pane_ids[@]}"
  queue_refresh_request "$scope" "${pane_ids[@]}"
}

board_repaint_cache_command() {
  local cache_file=""
  local state_file=""
  local warm_cache_file=""
  local current_generation last_generation tmp

  while (($# > 0)); do
    case "$1" in
    --cache-file)
      cache_file="${2:-}"
      shift 2
      ;;
    --state-file)
      state_file="${2:-}"
      shift 2
      ;;
    *)
      printf 'unknown board-repaint-cache option: %s\n' "$1" >&2
      exit 1
      ;;
    esac
  done

  if [[ -z "$cache_file" || -z "$state_file" ]]; then
    printf 'board-repaint-cache requires --cache-file and --state-file\n' >&2
    exit 1
  fi
  warm_cache_file="$(user_state_home)/panefleet/board-cache/rows-last.tsv"

  current_generation="$(current_refresh_generation)"
  last_generation="$(cat "$state_file" 2>/dev/null || printf '')"
  if [[ ! "$last_generation" =~ ^[0-9]+$ ]]; then
    last_generation=""
  fi

  if [[ -s "$cache_file" ]]; then
    if ! head -n 1 "$cache_file" | "${RG_BIN}" -Fq -- 'TOKENS' || ! head -n 1 "$cache_file" | "${RG_BIN}" -Fq -- 'CTX%'; then
      rm -f "$cache_file"
    fi
  fi

  if [[ ! -s "$cache_file" || "$current_generation" != "$last_generation" ]]; then
    tmp="$(mktemp "${TMPDIR:-/tmp}/panefleet-board-cache.XXXXXX")"
    if ! PANEFLEET_LIST_MODE=deferred-refresh list_rows >"$tmp"; then
      rm -f "$tmp"
      if [[ -s "$cache_file" ]]; then
        cat "$cache_file"
        return
      fi
      PANEFLEET_LIST_MODE=deferred-refresh list_rows
      return
    fi
    mv "$tmp" "$cache_file"
    cp "$cache_file" "$warm_cache_file" 2>/dev/null || true
    printf '%s\n' "$current_generation" >"$state_file"
  fi

  cat "$cache_file"
}

schedule_refresh_panes() {
  local panes="$1"
  local pane_id
  local -a args=()

  if [[ -z "$panes" ]]; then
    return
  fi

  while IFS= read -r pane_id; do
    if [[ -n "$pane_id" ]]; then
      args+=("$pane_id")
    fi
  done <<<"$panes"

  if ((${#args[@]} == 0)); then
    return
  fi

  runtime_log_event "heuristic_refresh_scheduled" count "${#args[@]}"
  queue_refresh_request "panes" "${args[@]}"
}

should_sync_refresh_now() {
  local now="$1"
  local activity="$2"
  local cached_raw_status="$3"
  local sync_refresh_count="$4"
  local age

  if [[ "${PANEFLEET_FORCE_SYNC_REFRESH:-0}" == "1" ]]; then
    return 0
  fi

  if ((sync_refresh_count >= 6)); then
    return 1
  fi

  case "$cached_raw_status" in
  RUN | WAIT | DONE | "") ;;
  *)
    return 1
    ;;
  esac

  if [[ -z "$activity" || "$activity" == "0" ]]; then
    return 1
  fi

  age=$((now - activity))
  ((age <= 1800))
}

should_bypass_heuristic_cache() {
  local tool="$1"
  local activity="$2"
  local now="$3"
  local age

  case "$tool" in
  codex | claude | opencode) ;;
  *)
    return 1
    ;;
  esac

  if [[ -z "$activity" || "$activity" == "0" ]]; then
    return 1
  fi

  age=$((now - activity))
  ((age >= 0 && age <= 180))
}

adapter_status() {
  local pane_id="$1"
  local tool="$2"
  local cmd="$3"
  local dead="$4"
  local dead_status="$5"
  local capture="$6"
  local recent focus

  if [[ "$dead" == "1" ]]; then
    inferred_status "$cmd" "$dead" "$dead_status"
    return
  fi

  recent="$(printf '%s\n' "$capture" | tail -n 20)"
  focus="$(printf '%s\n' "$recent" | tail -n 8)"

  case "$tool" in
  codex)
    if codex_is_waiting "$recent"; then
      PANEFLEET_RESOLVED_REASON="codex approval prompt"
      printf 'WAIT'
    elif codex_is_error "$recent"; then
      PANEFLEET_RESOLVED_REASON="codex error text"
      printf 'ERROR'
    elif codex_process_is_working "$pane_id"; then
      PANEFLEET_RESOLVED_REASON="codex process tree has active children"
      printf 'RUN'
    elif codex_is_working "$recent"; then
      PANEFLEET_RESOLVED_REASON="codex working footer"
      printf 'RUN'
    elif printf '%s\n' "$recent" | "${RG_BIN}" -q '^[[:space:]]*› '; then
      PANEFLEET_RESOLVED_REASON="codex prompt"
      printf 'DONE'
    else
      PANEFLEET_RESOLVED_REASON="codex fallback idle session"
      printf 'IDLE'
    fi
    ;;
  claude)
    focus="$(claude_focus_capture "$recent")"
    if claude_is_waiting "$focus"; then
      PANEFLEET_RESOLVED_REASON="claude approval or chooser prompt"
      printf 'WAIT'
    elif claude_is_error "$focus"; then
      PANEFLEET_RESOLVED_REASON="claude error text"
      printf 'ERROR'
    elif claude_is_done "$focus"; then
      PANEFLEET_RESOLVED_REASON="claude prompt"
      printf 'DONE'
    elif claude_is_working "$focus"; then
      PANEFLEET_RESOLVED_REASON="claude tool activity"
      printf 'RUN'
    else
      PANEFLEET_RESOLVED_REASON="claude fallback idle session"
      printf 'IDLE'
    fi
    ;;
  opencode)
    if opencode_is_waiting "$focus"; then
      PANEFLEET_RESOLVED_REASON="opencode approval prompt"
      printf 'WAIT'
    elif opencode_is_done "$focus" && ! opencode_is_working "$focus"; then
      PANEFLEET_RESOLVED_REASON="opencode ready prompt"
      printf 'DONE'
    elif opencode_is_error "$focus"; then
      PANEFLEET_RESOLVED_REASON="opencode error text"
      printf 'ERROR'
    elif opencode_is_working "$recent"; then
      PANEFLEET_RESOLVED_REASON="opencode activity text"
      printf 'RUN'
    else
      PANEFLEET_RESOLVED_REASON="opencode fallback idle session"
      printf 'IDLE'
    fi
    ;;
  *)
    PANEFLEET_RESOLVED_REASON="generic inferred process state"
    inferred_status "$cmd" "$dead" "$dead_status"
    ;;
  esac
}
