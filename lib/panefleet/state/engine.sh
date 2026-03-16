#!/usr/bin/env bash

# State engine extracted from bin/panefleet.
# Why: keep lifecycle resolution logic isolated from UI/install concerns so it
# can evolve with lower regression risk.

inferred_status() {
  local cmd="$1"
  local dead="$2"
  local dead_status="$3"

  if [[ "$dead" == "1" ]]; then
    if [[ "$dead_status" == "0" ]]; then
      printf 'DONE'
    else
      printf 'ERROR'
    fi
    return
  fi

  case "$cmd" in
    sh|bash|zsh|fish|nu|tmux)
      printf 'IDLE'
      ;;
    *)
      printf 'RUN'
      ;;
  esac
}

idle_or_stale_status_values() {
  local touched_at="$1"
  local last_done="$2"
  local fallback_activity="$3"
  local stale_minutes="$4"
  local now="$5"
  local age

  if [[ -z "$touched_at" || "$touched_at" == "0" ]]; then
    if [[ -n "$last_done" && "$last_done" != "0" ]]; then
      touched_at="$last_done"
    else
      touched_at="$fallback_activity"
    fi
  fi

  if [[ -z "$touched_at" || "$touched_at" == "0" ]]; then
    printf 'IDLE'
    return
  fi

  age=$(( now - touched_at ))
  if (( age >= stale_minutes * 60 )); then
    printf 'STALE'
  else
    printf 'IDLE'
  fi
}

effective_status_values() {
  local raw_status="$1"
  local fallback_activity="$2"
  local last_done="$3"
  local last_touch="$4"
  local done_recent_minutes="$5"
  local stale_minutes="$6"
  local now="$7"
  local age

  case "$raw_status" in
    ERROR|WAIT|RUN)
      printf '%s' "$raw_status"
      return
      ;;
    DONE)
      if [[ -z "$last_done" || "$last_done" == "0" ]]; then
        if [[ -n "$fallback_activity" && "$fallback_activity" != "0" ]]; then
          last_done="$fallback_activity"
        else
          last_done="$now"
        fi
      elif [[ -n "$fallback_activity" && "$fallback_activity" != "0" && "$fallback_activity" -gt "$last_done" ]]; then
        last_done="$fallback_activity"
      fi

      age=$(( now - last_done ))
      if (( age < done_recent_minutes * 60 )); then
        printf 'DONE'
      else
        printf '%s' "$(idle_or_stale_status_values "$last_touch" "$last_done" "$fallback_activity" "$stale_minutes" "$now")"
      fi
      return
      ;;
    IDLE)
      printf '%s' "$(idle_or_stale_status_values "$last_touch" "$last_done" "$fallback_activity" "$stale_minutes" "$now")"
      return
      ;;
    *)
      printf '%s' "$raw_status"
      return
      ;;
  esac
}

normalized_last_done_value() {
  local last_done="$1"
  local fallback_activity="$2"
  local now="$3"

  if [[ -z "$last_done" || "$last_done" == "0" ]]; then
    if [[ -n "$fallback_activity" && "$fallback_activity" != "0" ]]; then
      printf '%s' "$fallback_activity"
    else
      printf '%s' "$now"
    fi
  elif [[ -n "$fallback_activity" && "$fallback_activity" != "0" && "$fallback_activity" -gt "$last_done" ]]; then
    printf '%s' "$fallback_activity"
  else
    printf '%s' "$last_done"
  fi
}

resolve_live_raw_state_values() {
  local pane_id="$1"
  local initial_tool="$2"
  local cmd="$3"
  local title="$4"
  local dead="$5"
  local dead_status="$6"
  local capture="${7:-}"
  local tool capture_tool raw_status source reason live_reason

  source="heuristic-live"
  reason=""
  PANEFLEET_RESOLVED_REASON=""
  tool="$initial_tool"
  if [[ -z "$tool" ]]; then
    tool="$(tool_kind "$cmd" "$title")"
  fi

  if [[ "$dead" == "1" || "$tool" == "shell" ]]; then
    raw_status="$(inferred_status "$cmd" "$dead" "$dead_status")"
    if [[ "$dead" == "1" ]]; then
      reason="dead pane exit=${dead_status:-unknown}"
    else
      reason="shell inferred from process"
    fi
  else
    if [[ -z "$capture" ]]; then
      if [[ "$tool" == "opencode" ]]; then
        capture="$(pane_visible_capture "$pane_id")"
      else
        capture="$(pane_recent_capture "$pane_id")"
      fi
    fi
    if [[ -n "$capture" && ( "$tool" == "$cmd" || "$tool" == "unknown" || "$tool" =~ ^[0-9] ) ]]; then
      capture_tool="$(tool_from_capture "$capture")"
      if [[ "$capture_tool" != "unknown" ]]; then
        tool="$capture_tool"
      fi
    fi
    PANEFLEET_RESOLVED_REASON=""
    raw_status="$(adapter_status "$pane_id" "$tool" "$cmd" "$dead" "$dead_status" "$capture")"
    live_reason="${PANEFLEET_RESOLVED_REASON:-}"
    if [[ -n "$live_reason" ]]; then
      reason="$live_reason"
    fi
  fi

  PANEFLEET_RESOLVED_TOOL="$tool"
  PANEFLEET_RESOLVED_RAW_STATUS="$raw_status"
  PANEFLEET_RESOLVED_SOURCE="$source"
  PANEFLEET_RESOLVED_REASON="$reason"
}

resolve_live_raw_state_record() {
  resolve_live_raw_state_values "$@"
  printf '%s%s%s%s%s%s%s\n' \
    "$PANEFLEET_RESOLVED_TOOL" \
    "$PANEFIELD_SEP" \
    "$PANEFLEET_RESOLVED_RAW_STATUS" \
    "$PANEFIELD_SEP" \
    "$PANEFLEET_RESOLVED_SOURCE" \
    "$PANEFIELD_SEP" \
    "$PANEFLEET_RESOLVED_REASON"
}

resolve_uncached_state_values() {
  local pane_id="$1"
  local initial_tool="$2"
  local cmd="$3"
  local title="$4"
  local dead="$5"
  local dead_status="$6"
  local activity="$7"
  local capture="$8"
  local local_status="$9"
  local agent_status_value="${10}"
  local agent_tool_value="${11}"
  local agent_updated_at_value="${12}"
  local last_touch="${13}"
  local last_done="${14}"
  local done_recent_minutes="${15}"
  local stale_minutes="${16}"
  local agent_status_max_age="${17}"
  local now="${18}"
  local agent_source_value="${19:-}"
  local tool raw_status status live_override source reason
  local live_record live_tool live_raw live_source live_reason

  source=""
  reason=""
  tool="$initial_tool"
  raw_status=""
  status=""

  if [[ -n "$local_status" ]]; then
    if [[ "$local_status" == "STALE" ]]; then
      live_override=""
      if [[ "${PANEFLEET_ADAPTERS_ENABLED:-1}" == "1" ]] && [[ -n "$agent_status_value" ]] && agent_status_is_fresh "$agent_updated_at_value" "$agent_status_max_age" "$now" && agent_state_matches_live_tool "$tool" "$agent_tool_value"; then
        live_override="$agent_status_value"
        if [[ -n "$agent_tool_value" ]]; then
          tool="$agent_tool_value"
        fi
      fi

      if [[ "$live_override" != "RUN" && "$live_override" != "WAIT" ]]; then
        live_record="$(resolve_live_raw_state_record "$pane_id" "$tool" "$cmd" "$title" "$dead" "$dead_status" "$capture")"
        IFS="$PANEFIELD_SEP" read -r live_tool live_raw live_source live_reason <<<"$live_record"
        tool="$live_tool"
        live_override="$live_raw"
        source="$live_source"
        reason="$live_reason"
      fi

      if [[ "$live_override" == "RUN" || "$live_override" == "WAIT" ]]; then
        clear_manual_state "$pane_id"
        raw_status="$live_override"
        status="$(effective_status_values "$raw_status" "${activity:-0}" "$last_done" "$last_touch" "$done_recent_minutes" "$stale_minutes" "$now")"
        if [[ -z "$source" ]]; then
          source="agent"
        fi
        reason="manual stale override cleared by active ${raw_status}"
      else
        raw_status="$local_status"
        status="$local_status"
        source="manual"
        reason="manual status override"
      fi
    else
      raw_status="$local_status"
      status="$local_status"
      source="manual"
      reason="manual status override"
    fi
  elif [[ "$dead" == "1" ]]; then
    raw_status="$(inferred_status "$cmd" "$dead" "$dead_status")"
    status="$(effective_status_values "$raw_status" "${activity:-0}" "$last_done" "$last_touch" "$done_recent_minutes" "$stale_minutes" "$now")"
    source="process"
    reason="dead pane exit=${dead_status:-unknown}"
  elif [[ "${PANEFLEET_ADAPTERS_ENABLED:-1}" == "1" ]] && [[ -n "$agent_status_value" ]] && agent_status_is_fresh "$agent_updated_at_value" "$agent_status_max_age" "$now" && agent_state_matches_live_tool "$tool" "$agent_tool_value"; then
    local live_override=""

    raw_status="$agent_status_value"
    if [[ -n "$agent_tool_value" ]]; then
      tool="$agent_tool_value"
    fi
    if [[ "$tool" == "codex" && "$raw_status" != "WAIT" ]]; then
      if [[ -z "$capture" ]]; then
        capture="$(pane_recent_capture "$pane_id")"
      fi
      if [[ -n "$capture" ]]; then
        live_override="$(adapter_status "$pane_id" "$tool" "$cmd" "$dead" "$dead_status" "$capture")"
        if [[ "$live_override" == "RUN" || "$live_override" == "WAIT" ]]; then
          raw_status="$live_override"
          source="heuristic-live"
          reason="codex live state overrides adapter ${agent_status_value}"
        fi
      fi
    fi
    if [[ "$tool" == "claude" && "$raw_status" != "WAIT" ]]; then
      if [[ -z "$capture" ]]; then
        capture="$(pane_visible_capture "$pane_id")"
      fi
      if [[ -n "$capture" ]]; then
        capture="$(claude_focus_capture "$capture")"
        if claude_is_waiting "$capture"; then
          raw_status="WAIT"
          source="heuristic-live"
          reason="claude chooser overrides adapter ${agent_status_value}"
        fi
      fi
    fi
    if [[ "$tool" == "opencode" ]]; then
      if [[ -z "$capture" ]]; then
        capture="$(pane_visible_capture "$pane_id")"
      fi
      if [[ -n "$capture" ]]; then
        live_override="$(adapter_status "$pane_id" "$tool" "$cmd" "$dead" "$dead_status" "$capture")"
        if [[ "$raw_status" == "RUN" ]]; then
          if [[ "$live_override" == "WAIT" || "$live_override" == "DONE" || "$live_override" == "IDLE" || "$live_override" == "ERROR" ]]; then
            raw_status="$live_override"
            source="heuristic-live"
            reason="opencode live state overrides adapter RUN"
          fi
        elif [[ "$raw_status" == "WAIT" ]]; then
          if [[ "$live_override" == "RUN" || "$live_override" == "DONE" || "$live_override" == "IDLE" || "$live_override" == "ERROR" ]]; then
            raw_status="$live_override"
            source="heuristic-live"
            reason="opencode live state overrides adapter WAIT"
          fi
        fi
      fi
    fi
    status="$(effective_status_values "$raw_status" "${activity:-0}" "$last_done" "$last_touch" "$done_recent_minutes" "$stale_minutes" "$now")"
    if [[ -z "$source" ]]; then
      source="agent"
      if [[ -n "$agent_source_value" ]]; then
        reason="fresh adapter state from ${agent_source_value}"
      else
        reason="fresh adapter state"
      fi
    fi
  else
    live_record="$(resolve_live_raw_state_record "$pane_id" "$tool" "$cmd" "$title" "$dead" "$dead_status" "$capture")"
    IFS="$PANEFIELD_SEP" read -r live_tool live_raw live_source live_reason <<<"$live_record"
    tool="$live_tool"
    raw_status="$live_raw"
    source="$live_source"
    reason="$live_reason"
    status="$(effective_status_values "$raw_status" "${activity:-0}" "$last_done" "$last_touch" "$done_recent_minutes" "$stale_minutes" "$now")"
  fi

  PANEFLEET_RESOLVED_TOOL="$tool"
  PANEFLEET_RESOLVED_RAW_STATUS="$raw_status"
  PANEFLEET_RESOLVED_STATUS="$status"
  PANEFLEET_RESOLVED_SOURCE="$source"
  PANEFLEET_RESOLVED_REASON="$reason"
}

resolve_uncached_state_record() {
  resolve_uncached_state_values "$@"
  printf '%s%s%s%s%s%s%s%s%s\n' \
    "$PANEFLEET_RESOLVED_TOOL" \
    "$PANEFIELD_SEP" \
    "$PANEFLEET_RESOLVED_RAW_STATUS" \
    "$PANEFIELD_SEP" \
    "$PANEFLEET_RESOLVED_STATUS" \
    "$PANEFIELD_SEP" \
    "$PANEFLEET_RESOLVED_SOURCE" \
    "$PANEFIELD_SEP" \
    "$PANEFLEET_RESOLVED_REASON"
}
