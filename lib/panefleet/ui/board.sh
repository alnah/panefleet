#!/usr/bin/env bash

# Board row assembly, preview rendering, and popup board UI.
pretty_age_into() {
  local outvar="$1"
  local epoch="$2"
  local now="$3"
  local diff

  if [[ -z "$epoch" || "$epoch" == "0" ]]; then
    printf -v "$outvar" '%s' '-'
    return
  fi

  diff=$((now - epoch))
  if ((diff < 60)); then
    printf -v "$outvar" '%ss' "$diff"
  elif ((diff < 3600)); then
    printf -v "$outvar" '%sm' $((diff / 60))
  elif ((diff < 86400)); then
    printf -v "$outvar" '%sh' $((diff / 3600))
  else
    printf -v "$outvar" '%sd' $((diff / 86400))
  fi
}

task_label_into() {
  local outvar="$1"
  local cmd="$2"
  local title="$3"
  local window_name="$4"
  local repo="$5"

  case "$title" in
  "" | "$cmd" | zsh | bash | fish | sh | tmux | cdx)
    if [[ -n "$window_name" && "$window_name" != "$repo" ]]; then
      printf -v "$outvar" '%s' "$window_name"
    elif [[ -n "$repo" ]]; then
      printf -v "$outvar" '%s' "$repo"
    else
      printf -v "$outvar" '%s' "$cmd"
    fi
    ;;
  *)
    printf -v "$outvar" '%s' "$title"
    ;;
  esac
}

status_rank_value() {
  case "$1" in
  RUN) REPLY='0' ;;
  WAIT) REPLY='1' ;;
  DONE) REPLY='2' ;;
  ERROR) REPLY='3' ;;
  IDLE) REPLY='4' ;;
  STALE) REPLY='5' ;;
  *) REPLY='9' ;;
  esac
}

list_runtime_defaults() {
  PANEFLEET_DONE_RECENT_MINUTES="$(tmux_global_option @panefleet-done-recent-minutes)"
  PANEFLEET_STALE_MINUTES="$(tmux_global_option @panefleet-stale-minutes)"
  PANEFLEET_AGENT_STATUS_MAX_AGE="$(agent_status_max_age_seconds)"
  if adapters_enabled; then
    PANEFLEET_ADAPTERS_ENABLED="1"
  else
    PANEFLEET_ADAPTERS_ENABLED="0"
  fi
  PANEFLEET_NOW="$(now_epoch)"
  PANEFLEET_REFRESH_QUEUE="${PANEFLEET_REFRESH_QUEUE:-}"
  PANEFLEET_SYNC_REFRESH_COUNT="${PANEFLEET_SYNC_REFRESH_COUNT:-0}"

  if [[ -z "$PANEFLEET_DONE_RECENT_MINUTES" ]]; then
    PANEFLEET_DONE_RECENT_MINUTES="10"
  fi
  if [[ -z "$PANEFLEET_STALE_MINUTES" ]]; then
    PANEFLEET_STALE_MINUTES="45"
  fi
}

list_mode_value() {
  printf '%s' "${PANEFLEET_LIST_MODE:-full}"
}

list_deferred_refresh_mode_enabled() {
  local mode
  mode="$(list_mode_value)"
  [[ "$mode" == "deferred-refresh" ]]
}

resolve_list_row_values() {
  local pane_id="$1"
  local cmd="$2"
  local title="$3"
  local dead="$4"
  local dead_status="$5"
  local activity="$6"
  local signature="$7"
  local local_status="$8"
  local agent_status_value="$9"
  local agent_tool_value="${10}"
  local agent_updated_at_value="${11}"
  local last_touch="${12}"
  local last_done="${13}"
  local cached_signature="${14}"
  local cached_raw_status="${15}"
  local cached_tool="${16}"
  local agent_source_value="${17:-}"
  local tool raw_status status source reason refresh_record=""
  local uncached_record uncached_tool uncached_raw uncached_status uncached_source uncached_reason
  local force_live_refresh="0"

  source=""
  reason=""
  tool="$(tool_kind "$cmd" "$title")"
  if [[ -n "$cached_tool" && ("$tool" == "$cmd" || "$tool" == "unknown" || "$tool" =~ ^[0-9]) ]]; then
    tool="$cached_tool"
  fi

  if should_bypass_heuristic_cache "$tool" "${activity:-0}" "$PANEFLEET_NOW"; then
    force_live_refresh="1"
  fi

  if [[ -n "$local_status" || "$dead" == "1" || -n "$agent_status_value" ]]; then
    # When a fresh heuristic cache exists for the current pane signature,
    # prefer it over adapter-fast status to keep board rows aligned with
    # preview/live-derived state after queue-refresh.
    if [[ -z "$local_status" && "$dead" != "1" && -n "$cached_raw_status" && -n "$cached_signature" && "$cached_signature" == "$signature" ]]; then
      if [[ -n "$agent_status_value" ]]; then
        case "$cached_raw_status:$agent_status_value" in
        RUN:DONE | RUN:IDLE | RUN:STALE | WAIT:DONE | WAIT:IDLE | WAIT:STALE | ERROR:DONE | ERROR:IDLE | ERROR:STALE) ;;
        *)
          cached_raw_status=""
          ;;
        esac
      fi
    fi

    if [[ -z "$local_status" && "$dead" != "1" && -n "$cached_raw_status" && -n "$cached_signature" && "$cached_signature" == "$signature" ]]; then
      raw_status="$cached_raw_status"
      if [[ -n "$cached_tool" ]]; then
        tool="$cached_tool"
      fi
      status="$(effective_status_values "$raw_status" "${activity:-0}" "$last_done" "$last_touch" "$PANEFLEET_DONE_RECENT_MINUTES" "$PANEFLEET_STALE_MINUTES" "$PANEFLEET_NOW")"
      PANEFLEET_RESOLVED_TOOL="$tool"
      PANEFLEET_RESOLVED_RAW_STATUS="$raw_status"
      PANEFLEET_RESOLVED_STATUS="$status"
      PANEFLEET_RESOLVED_SOURCE="heuristic-cache"
      PANEFLEET_RESOLVED_REASON="cache signature match"
      PANEFLEET_RESOLVED_LAST_DONE="$last_done"
      return
    fi

    if list_deferred_refresh_mode_enabled && [[ -z "$local_status" ]] && [[ "$dead" != "1" ]] && [[ -n "$agent_status_value" ]] && agent_status_is_fresh "$agent_updated_at_value" "$PANEFLEET_AGENT_STATUS_MAX_AGE" "$PANEFLEET_NOW" && agent_state_matches_live_tool "$tool" "$agent_tool_value"; then
      raw_status="$agent_status_value"
      if [[ -n "$agent_tool_value" ]]; then
        tool="$agent_tool_value"
      fi
      status="$(effective_status_values "$raw_status" "${activity:-0}" "$last_done" "$last_touch" "$PANEFLEET_DONE_RECENT_MINUTES" "$PANEFLEET_STALE_MINUTES" "$PANEFLEET_NOW")"
      PANEFLEET_RESOLVED_TOOL="$tool"
      PANEFLEET_RESOLVED_RAW_STATUS="$raw_status"
      PANEFLEET_RESOLVED_STATUS="$status"
      PANEFLEET_RESOLVED_SOURCE="agent-fast"
      PANEFLEET_RESOLVED_REASON="fast list uses fresh adapter state without live capture"
      PANEFLEET_RESOLVED_LAST_DONE="$last_done"
      return
    fi

    uncached_record="$(resolve_uncached_state_record "$pane_id" "$tool" "$cmd" "$title" "$dead" "$dead_status" "${activity:-0}" "" "$local_status" "$agent_status_value" "$agent_tool_value" "$agent_updated_at_value" "$last_touch" "$last_done" "$PANEFLEET_DONE_RECENT_MINUTES" "$PANEFLEET_STALE_MINUTES" "$PANEFLEET_AGENT_STATUS_MAX_AGE" "$PANEFLEET_NOW" "$agent_source_value")"
    IFS="$PANEFIELD_SEP" read -r uncached_tool uncached_raw uncached_status uncached_source uncached_reason <<<"$uncached_record"
    PANEFLEET_RESOLVED_TOOL="$uncached_tool"
    PANEFLEET_RESOLVED_RAW_STATUS="$uncached_raw"
    PANEFLEET_RESOLVED_STATUS="$uncached_status"
    PANEFLEET_RESOLVED_SOURCE="$uncached_source"
    PANEFLEET_RESOLVED_REASON="$uncached_reason"
    PANEFLEET_RESOLVED_LAST_DONE="$last_done"
    return
  fi

  if [[ "$force_live_refresh" == "1" ]] || cache_refresh_needed "$local_status" "$cached_signature" "$signature" "$cached_raw_status"; then
    if list_deferred_refresh_mode_enabled; then
      PANEFLEET_REFRESH_QUEUE+="$pane_id"$'\n'
      raw_status="$(fallback_raw_status "$cached_raw_status" "$tool" "$cmd" "$dead" "$dead_status")"
      if [[ -n "$cached_raw_status" ]]; then
        source="heuristic-cache"
        reason="deferred refresh mode"
      else
        source="default"
        reason="deferred refresh without cache"
      fi
    elif should_sync_refresh_now "$PANEFLEET_NOW" "${activity:-0}" "$cached_raw_status" "$PANEFLEET_SYNC_REFRESH_COUNT"; then
      refresh_record="$(refresh_pane_cache_record "$pane_id" "${activity:-0}" "$signature" "$cmd" "$title" "$dead" "$dead_status" "$last_done")"
      IFS="$PANEFIELD_SEP" read -r tool raw_status last_done <<<"$refresh_record"
      PANEFLEET_SYNC_REFRESH_COUNT=$((PANEFLEET_SYNC_REFRESH_COUNT + 1))
      source="heuristic-live"
      if [[ -z "$reason" ]]; then
        if [[ "$force_live_refresh" == "1" ]]; then
          reason="recent pane activity forced live refresh"
        else
          reason="sync cache refresh"
        fi
      fi
    else
      PANEFLEET_REFRESH_QUEUE+="$pane_id"$'\n'
      raw_status="$(fallback_raw_status "$cached_raw_status" "$tool" "$cmd" "$dead" "$dead_status")"
      if [[ -n "$cached_raw_status" ]]; then
        source="heuristic-cache"
        reason="deferred refresh using cached state"
      else
        source="default"
        reason="deferred refresh without cache"
      fi
    fi
  else
    raw_status="$(fallback_raw_status "$cached_raw_status" "$tool" "$cmd" "$dead" "$dead_status")"
    if [[ -n "$cached_raw_status" ]]; then
      source="heuristic-cache"
      reason="cache signature match"
    else
      source="default"
      reason="no cache available"
    fi
  fi

  if [[ -z "$tool" ]]; then
    tool="$(tool_kind "$cmd" "$title")"
  fi
  if [[ -n "$cached_tool" && -z "$refresh_record" ]]; then
    tool="$cached_tool"
  fi
  status="$(effective_status_values "$raw_status" "${activity:-0}" "$last_done" "$last_touch" "$PANEFLEET_DONE_RECENT_MINUTES" "$PANEFLEET_STALE_MINUTES" "$PANEFLEET_NOW")"

  PANEFLEET_RESOLVED_TOOL="$tool"
  PANEFLEET_RESOLVED_RAW_STATUS="$raw_status"
  PANEFLEET_RESOLVED_STATUS="$status"
  PANEFLEET_RESOLVED_SOURCE="$source"
  PANEFLEET_RESOLVED_REASON="$reason"
  PANEFLEET_RESOLVED_LAST_DONE="$last_done"
}

build_list_row() {
  local pane_id="$1"
  local session_name="$2"
  local window_index="$3"
  local pane_index="$4"
  local path="$5"
  local activity="$6"
  local tool="$7"
  local status="$8"
  local window_name="$9"
  local tokens_used="${10:-}"
  local context_left_pct="${11:-}"
  local repo target_window target_pane short_path age
  local tool_cell target_cell session_cell window_cell repo_cell tokens_cell ctx_cell colored_status display rank sep
  local colored_tokens colored_ctx

  repo="${path##*/}"
  if [[ -z "$repo" ]]; then
    repo="$path"
  fi
  target_window="${session_name}:${window_index}"
  target_pane="${window_index}.${pane_index}"
  short_path="${path/#$HOME/\~}"
  pretty_age_into age "${activity:-0}" "$PANEFLEET_NOW"
  fit_cell_into tool_cell "$tool" 8
  fit_cell_into target_cell "$target_pane" 8
  fit_cell_into session_cell "$session_name" 14
  fit_cell_into window_cell "$window_name" 18
  fit_cell_into repo_cell "$repo" 14
  if [[ "$tokens_used" =~ ^[0-9]+$ ]]; then
    tokens_cell="$tokens_used"
  else
    tokens_cell="-"
  fi
  fit_cell_into tokens_cell "$tokens_cell" 8
  if [[ "$context_left_pct" =~ ^[0-9]+$ ]]; then
    ctx_cell="${context_left_pct}%"
  else
    ctx_cell="-"
  fi
  fit_cell_into ctx_cell "$ctx_cell" 4
  colored_tokens="$(tokens_color "$tokens_used" "$tokens_cell")"
  colored_ctx="$(context_left_color "$context_left_pct" "$ctx_cell")"
  colored_status="$(colored_status_cell "$status")"
  status_rank_value "$status"
  rank="$REPLY"
  sep="$(separator_cell)"
  display="$(printf '%b%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%4s' \
    "$colored_status" \
    "$sep" \
    "$tool_cell" \
    "$sep" \
    "$target_cell" \
    "$sep" \
    "$session_cell" \
    "$sep" \
    "$window_cell" \
    "$sep" \
    "$repo_cell" \
    "$sep" \
    "$colored_tokens" \
    "$sep" \
    "$colored_ctx" \
    "$sep" \
    "$age")"

  printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$pane_id" \
    "$target_window" \
    "$rank" \
    "${activity:-0}" \
    "$short_path" \
    "$display"
}

list_header() {
  local status_text tool_text target_text session_text window_text repo_text tokens_text ctx_text
  local status_cell tool_cell target_cell session_cell window_cell repo_cell tokens_cell ctx_cell display sep

  fit_cell_into status_text "STATE" 5
  fit_cell_into tool_text "TOOL" 8
  fit_cell_into target_text "TARGET" 8
  fit_cell_into session_text "SESSION" 14
  fit_cell_into window_text "WINDOW" 18
  fit_cell_into repo_text "REPO" 14
  fit_cell_into tokens_text "TOKENS" 8
  fit_cell_into ctx_text "CTX%" 4
  status_cell="$(header_cell "$status_text")"
  tool_cell="$(header_cell "$tool_text")"
  target_cell="$(header_cell "$target_text")"
  session_cell="$(header_cell "$session_text")"
  window_cell="$(header_cell "$window_text")"
  repo_cell="$(header_cell "$repo_text")"
  tokens_cell="$(header_cell "$tokens_text")"
  ctx_cell="$(header_cell "$ctx_text")"
  sep="$(separator_cell)"
  display="$(printf '%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%4s' \
    "$status_cell" \
    "$sep" \
    "$tool_cell" \
    "$sep" \
    "$target_cell" \
    "$sep" \
    "$session_cell" \
    "$sep" \
    "$window_cell" \
    "$sep" \
    "$repo_cell" \
    "$sep" \
    "$tokens_cell" \
    "$sep" \
    "$ctx_cell" \
    "$sep" \
    "$(header_cell "AGE")")"

  printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
    "__header__" \
    "__header__" \
    "-1" \
    "0" \
    "__header__" \
    "$display"
}

list_rows() {
  require_runtime_support
  resolve_theme
  local rows_file
  local pane_id session_name window_index window_name pane_index cmd title path dead dead_status activity signature local_status agent_status_value agent_tool_value agent_source_value agent_updated_at_value last_touch last_done cached_signature cached_raw_status cached_tool tokens_used context_left_pct
  local tool status

  list_runtime_defaults
  PANEFLEET_REFRESH_QUEUE=""
  PANEFLEET_SYNC_REFRESH_COUNT=0

  rows_file="$(mktemp "${TMPDIR:-/tmp}/panefleet-list.XXXXXX")"
  list_header >"$rows_file"
  while IFS="$PANEFIELD_SEP" read -r pane_id session_name window_index window_name pane_index cmd title path dead dead_status activity signature local_status agent_status_value agent_tool_value agent_source_value agent_updated_at_value last_touch last_done cached_signature cached_raw_status cached_tool tokens_used context_left_pct; do
    : "${agent_source_value:=}" "${agent_updated_at_value:=}"
    resolve_list_row_values "$pane_id" "$cmd" "$title" "$dead" "$dead_status" "${activity:-0}" "$signature" "$local_status" "$agent_status_value" "$agent_tool_value" "$agent_updated_at_value" "$last_touch" "$last_done" "$cached_signature" "$cached_raw_status" "$cached_tool" "$agent_source_value"
    tool="$PANEFLEET_RESOLVED_TOOL"
    status="$PANEFLEET_RESOLVED_STATUS"
    build_list_row "$pane_id" "$session_name" "$window_index" "$pane_index" "$path" "${activity:-0}" "$tool" "$status" "$window_name" "$tokens_used" "$context_left_pct" >>"$rows_file"
  done < <(pane_records "$(list_pane_record_format)")

  sort -t $'\t' -k3,3n -k4,4nr "$rows_file"
  rm -f "$rows_file"
  if ! list_deferred_refresh_mode_enabled; then
    schedule_refresh_panes "$PANEFLEET_REFRESH_QUEUE"
  fi
}

preview_row() {
  local pane_id="${1:?pane_id is required}"
  local status cmd title path short_path session_name window_index pane_index window_name dead dead_status activity
  local agent_status_value agent_tool_value agent_updated_at_value
  local uncached_record uncached_tool uncached_raw uncached_status _uncached_source _uncached_reason

  require_runtime_support
  resolve_theme
  list_runtime_defaults
  status="$(manual_status "$pane_id")"
  IFS="$PANEFIELD_SEP" read -r _pane_id session_name window_index window_name pane_index cmd title path dead dead_status activity <<<"$(preview_pane_record "$pane_id")"
  tool="$(tool_kind "$cmd" "$title")"
  if [[ "$tool" == "opencode" ]]; then
    capture="$(pane_visible_capture "$pane_id")"
  else
    capture="$(pane_recent_capture "$pane_id")"
  fi
  agent_status_value="$(agent_status "$pane_id")"
  agent_tool_value="$(agent_tool "$pane_id")"
  agent_updated_at_value="$(agent_updated_at "$pane_id")"
  if [[ -z "$status" ]]; then
    uncached_record="$(resolve_uncached_state_record "$pane_id" "$tool" "$cmd" "$title" "$dead" "$dead_status" "${activity:-0}" "$capture" "" "$agent_status_value" "$agent_tool_value" "$agent_updated_at_value" "$(pane_option "$pane_id" @panefleet_last_touch)" "$(pane_option "$pane_id" @panefleet_last_done)" "$PANEFLEET_DONE_RECENT_MINUTES" "$PANEFLEET_STALE_MINUTES" "$PANEFLEET_AGENT_STATUS_MAX_AGE" "$PANEFLEET_NOW" "$(agent_source "$pane_id")")"
    IFS="$PANEFIELD_SEP" read -r uncached_tool uncached_raw uncached_status _uncached_source _uncached_reason <<<"$uncached_record"
    tool="$uncached_tool"
    status="$uncached_status"
  fi

  short_path="${path/#$HOME/\~}"
  capture="$(preview_body_capture "$pane_id")"

  printf '%s\n' "$(paint_fg "pane meta" "$THEME_HEADER" "1")"
  printf '%s %s\n' "$(preview_label "status")" "$(status_color "$status")"
  printf '%s %s\n' "$(preview_label "target")" "$(preview_value "${session_name}:${window_index}.${pane_index}" "$THEME_ACCENT")"
  printf '%s %s\n' "$(preview_label "window")" "$(preview_value "$window_name")"
  printf '%s %s\n' "$(preview_label "cmd")" "$(preview_value "$cmd" "$THEME_IDLE")"
  printf '%s %s\n' "$(preview_label "tool")" "$(preview_value "$tool" "$THEME_IDLE")"
  printf '%s %s\n' "$(preview_label "title")" "$(preview_value "$title")"
  printf '%s %s\n' "$(preview_label "path")" "$(preview_value "$short_path" "$THEME_ACCENT")"
  printf '\n'
  printf '%s\n' "$(paint_fg "pane output ────────────────────────────" "$THEME_BORDER_STRONG")"
  printf '%s\n' "$capture" | render_preview_capture
}

jump_to_pane() {
  local pane_id="${1:?pane_id is required}"
  local target_window="${2:?target_window is required}"

  "${TMUX_BIN}" switch-client -t "$target_window"
  "${TMUX_BIN}" select-pane -t "$pane_id"
}

board_header_navigation() {
  local enter_hint list_hint stale_hint sep

  enter_hint="$(paint_fg "[↵]" "$THEME_ACCENT" "1") $(paint_fg "jump" "$THEME_MUTED")"
  list_hint="$(paint_fg "[↑↓]" "$THEME_ACCENT" "1") $(paint_fg "list" "$THEME_MUTED")"
  stale_hint="$(paint_fg "[ctrl-s]" "$THEME_ACCENT" "1") $(paint_fg "stale" "$THEME_MUTED")"
  sep="$(paint_fg " · " "$THEME_BORDER")"

  # Keep one explicit spacer line between controls and table headers.
  printf '%s%s%s%s%s\n%s' "$enter_hint" "$sep" "$list_hint" "$sep" "$stale_hint" " "
}

board_left_padding() {
  printf '%s' ''
}

board_prompt_text() {
  printf '%spanefleet ~ ' "$(board_left_padding)"
}

open_board() {
  require_tmux
  require_runtime_support
  resolve_theme
  local rc
  local cache_dir cache_file state_file warm_cache_file board_token
  local initial_generation repaint_command repaint_delay load_reload_action reload_event poll_interval
  local -a fzf_args

  cache_dir="$(user_state_home)/panefleet/board-cache"
  mkdir -p "$cache_dir" 2>/dev/null || true
  board_token="${$}-$(now_epoch)"
  cache_file="${cache_dir}/rows-${board_token}.tsv"
  state_file="${cache_dir}/state-${board_token}.txt"
  warm_cache_file="${cache_dir}/rows-last.tsv"
  initial_generation="$(current_refresh_generation)"
  if [[ -s "$warm_cache_file" ]]; then
    if ! head -n 1 "$warm_cache_file" | "${RG_BIN}" -Fq -- 'TOKENS' || ! head -n 1 "$warm_cache_file" | "${RG_BIN}" -Fq -- 'CTX%'; then
      rm -f "$warm_cache_file"
    fi
  fi
  if [[ -s "$warm_cache_file" ]]; then
    cp "$warm_cache_file" "$cache_file" 2>/dev/null || PANEFLEET_LIST_MODE=deferred-refresh list_rows >"$cache_file"
  else
    PANEFLEET_LIST_MODE=deferred-refresh list_rows >"$cache_file"
    cp "$cache_file" "$warm_cache_file" 2>/dev/null || true
  fi
  printf '%s\n' "$initial_generation" >"$state_file"
  # Immediate by default; optional override for slower environments.
  repaint_delay="${PANEFLEET_BOARD_REPAINT_DELAY:-0}"
  repaint_command="\"${SELF}\" board-repaint-cache --cache-file \"${cache_file}\" --state-file \"${state_file}\""
  if [ "$repaint_delay" != "0" ] && [ "$repaint_delay" != "0.0" ]; then
    repaint_command="sleep \"${repaint_delay}\"; ${repaint_command}"
  fi
  load_reload_action="reload"
  reload_event="load"
  if fzf_supports_reload_sync; then
    load_reload_action="reload-sync"
  else
    # With async reload fallback, start event reduces initial empty-list flashes.
    reload_event="start"
  fi

  fzf_args=(
    --ansi
    --color="$(fzf_color_spec)"
    --delimiter=$'\t'
    --with-nth=6
    --header-lines=1
    --header-lines-border=line
    --no-sort
    --layout=reverse
    --height=100%
    --border=none
    --info=inline-right
    --prompt="$(board_prompt_text)"
    --header="$(board_header_navigation)"
    --separator='─'
    --pointer='▌'
    --marker='•'
    --preview "${SELF} preview {1}"
    --preview-window='bottom,55%,border-top,wrap,follow,~9'
    --bind "enter:execute-silent(${SELF} jump {1} {2})+abort"
    --bind "up:up+execute-silent(${SELF} refresh-panes-cache {1})+reload(PANEFLEET_LIST_MODE=deferred-refresh ${SELF} list-deferred)"
    --bind "down:down+execute-silent(${SELF} refresh-panes-cache {1})+reload(PANEFLEET_LIST_MODE=deferred-refresh ${SELF} list-deferred)"
    --bind "change:execute-silent(${SELF} queue-refresh --all)"
    --bind "ctrl-s:execute-silent(${SELF} state-stale --pane {1})+execute-silent(${SELF} refresh-panes-cache {1})+reload(PANEFLEET_LIST_MODE=deferred-refresh ${SELF} list-deferred)"
    --bind "${reload_event}:${load_reload_action}(${repaint_command})"
  )
  poll_interval="${PANEFLEET_BOARD_POLL_INTERVAL_SECONDS:-1}"
  if [[ ! "$poll_interval" =~ ^[0-9]+$ ]] || ((poll_interval <= 0)); then
    poll_interval="1"
  fi
  if fzf_supports_reload_sync && fzf_supports_result_event; then
    # Keep board data fresh even when the user does not interact.
    fzf_args+=(--bind "result:reload-sync(sleep \"${poll_interval}\"; ${repaint_command})")
  fi
  if fzf_supports_padding; then
    # Use symmetric padding to keep prompt/controls/table visually aligned.
    fzf_args+=(--padding='1,3,1,3')
  fi
  "$SELF" queue-refresh --all >/dev/null 2>&1 || true

  set +e
  "${FZF_BIN}" "${fzf_args[@]}" <"$cache_file"
  rc=$?
  set -e
  rm -f "$cache_file" "$state_file"

  case "$rc" in
  0 | 1 | 130)
    return 0
    ;;
  *)
    return "$rc"
    ;;
  esac
}

theme_popup_style() {
  resolve_theme
  resolve_color_mode
  printf 'fg=%s,bg=%s' "$(tmux_style_color "$THEME_FG")" "$(tmux_style_color "$THEME_BG")"
}

theme_popup_border_style() {
  resolve_theme
  resolve_color_mode
  printf 'fg=%s,bg=%s' "$(tmux_style_color "$THEME_BORDER_STRONG")" "$(tmux_style_color "$THEME_BG")"
}
