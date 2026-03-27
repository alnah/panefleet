#!/usr/bin/env bash

# Theme selector, theme preview, and popup UI commands.
board_popup_width() {
  printf '%s' "${PANEFLEET_BOARD_POPUP_WIDTH:-100%}"
}

board_popup_height() {
  printf '%s' "${PANEFLEET_BOARD_POPUP_HEIGHT:-100%}"
}

open_popup() {
  require_tmux
  require_runtime_support
  resolve_theme

  "${TMUX_BIN}" display-popup \
    -E \
    -w "$(board_popup_width)" \
    -h "$(board_popup_height)" \
    -s "$(theme_popup_style)" \
    -S "$(theme_popup_border_style)" \
    "${SELF} board"
}

theme_sample() {
  local theme_name="$1"
  local previous_theme="${PANEFLEET_THEME:-}"
  local sample

  PANEFLEET_THEME="$theme_name"
  resolve_theme
  sample="$(printf '%s  %s  %s  %s  %s  %s' \
    "$(status_color "RUN")" \
    "$(status_color "WAIT")" \
    "$(status_color "DONE")" \
    "$(status_color "ERROR")" \
    "$(status_color "IDLE")" \
    "$(status_color "STALE")")"
  PANEFLEET_THEME="$previous_theme"

  printf '%s' "$sample"
}

theme_rows() {
  local current_theme theme_name marker display

  current_theme="$(tmux_global_option @panefleet-theme)"
  if [[ -z "$current_theme" ]]; then
    current_theme="panefleet-dark"
  fi

  while IFS= read -r theme_name; do
    if [[ "$theme_name" == "$current_theme" ]]; then
      marker="*"
    else
      marker=" "
    fi

    display="$(printf '%s %-18s %s' "$marker" "$theme_name" "$(theme_sample "$theme_name")")"
    printf '%s\t%s\n' "$theme_name" "$display"
  done < <(available_themes)
}

contrast_ratio_x100() {
  local c1="$1"
  local c2="$2"
  local r1 g1 b1 r2 g2 b2

  read -r r1 g1 b1 <<<"$(hex_components "$c1")"
  read -r r2 g2 b2 <<<"$(hex_components "$c2")"
  awk -v r1="$r1" -v g1="$g1" -v b1="$b1" -v r2="$r2" -v g2="$g2" -v b2="$b2" '
    function channel(c) {
      c = c / 255.0
      if (c <= 0.03928) {
        return c / 12.92
      }
      return ((c + 0.055) / 1.055) ^ 2.4
    }
    BEGIN {
      l1 = 0.2126 * channel(r1) + 0.7152 * channel(g1) + 0.0722 * channel(b1)
      l2 = 0.2126 * channel(r2) + 0.7152 * channel(g2) + 0.0722 * channel(b2)
      if (l1 > l2) {
        hi = l1
        lo = l2
      } else {
        hi = l2
        lo = l1
      }
      ratio = (hi + 0.05) / (lo + 0.05)
      printf "%d", int(ratio * 100 + 0.5)
    }
  '
}

contrast_badge_text() {
  local ratio_x100="$1"
  local threshold_x100="$2"

  if ((ratio_x100 >= threshold_x100)); then
    printf '%s' "$(paint_fg "pass" "$THEME_RUN" "1")"
  else
    printf '%s' "$(paint_fg "warn" "$THEME_ERROR" "1")"
  fi
}

theme_preview() {
  local theme_name="${1:?theme name is required}"
  local text_ratio ui_ratio text_pass ui_pass resolved_theme_name

  PANEFLEET_THEME="$theme_name"
  resolve_theme
  resolved_theme_name="${THEME_NAME:-$theme_name}"
  text_ratio="$(contrast_ratio_x100 "$THEME_FG" "$THEME_BG")"
  ui_ratio="$(contrast_ratio_x100 "$THEME_BORDER_STRONG" "$THEME_BG")"
  text_pass="$(contrast_badge_text "$text_ratio" 450)"
  ui_pass="$(contrast_badge_text "$ui_ratio" 300)"

  printf 'theme   %s\n' "$resolved_theme_name"
  printf 'bg      %s\n' "$THEME_BG"
  printf 'fg      %s\n' "$THEME_FG"
  printf 'border  %s\n' "$THEME_BORDER"
  printf 'accent  %s\n' "$THEME_ACCENT"
  printf 'wcag    text %s (%d.%02d:1)  ui %s (%d.%02d:1)\n' \
    "$text_pass" \
    "$((text_ratio / 100))" "$((text_ratio % 100))" \
    "$ui_pass" \
    "$((ui_ratio / 100))" "$((ui_ratio % 100))"
  printf '\n'
  printf 'states  %s  %s  %s  %s  %s  %s\n' \
    "$(status_color "RUN")" \
    "$(status_color "WAIT")" \
    "$(status_color "DONE")" \
    "$(status_color "ERROR")" \
    "$(status_color "IDLE")" \
    "$(status_color "STALE")"
  printf 'diffs   %s  %s  %s  %s\n' \
    "$(paint_fg "header" "$THEME_DIFF_HEADER" "1")" \
    "$(paint_fg "hunk" "$THEME_DIFF_HUNK" "1")" \
    "$(paint_fg "add" "$THEME_DIFF_ADD" "1")" \
    "$(paint_fg "remove" "$THEME_DIFF_REMOVE" "1")"
}

apply_theme() {
  local theme_name="${1:?theme name is required}"

  if ! available_themes | "${RG_BIN}" -qx -- "$theme_name"; then
    printf 'unsupported theme: %s\n' "$theme_name" >&2
    exit 1
  fi

  "${TMUX_BIN}" set-option -gq @panefleet-theme "$theme_name"
  "${TMUX_BIN}" display-message "panefleet theme: ${theme_name}"
}

open_theme_selector() {
  require_tmux
  require_runtime_support
  resolve_theme
  local rc

  set +e
  theme_rows | "${FZF_BIN}" \
    --ansi \
    --color="$(fzf_color_spec)" \
    --delimiter=$'\t' \
    --with-nth=2 \
    --layout=reverse \
    --height=100% \
    --border=none \
    --info=inline-right \
    --prompt='theme> ' \
    --header='enter: apply theme · preview includes quick wcag check' \
    --separator='─' \
    --pointer='▌' \
    --marker='•' \
    --preview "${SELF} theme-preview {1}" \
    --preview-window='bottom,45%,border-top,wrap' \
    --bind "enter:execute-silent(${SELF} theme-apply {1})+abort"
  rc=$?
  set -e

  case "$rc" in
  0 | 1 | 130)
    return 0
    ;;
  *)
    return "$rc"
    ;;
  esac
}

open_theme_popup() {
  require_tmux
  require_runtime_support
  resolve_theme

  "${TMUX_BIN}" display-popup \
    -E \
    -w 70% \
    -h 70% \
    -T "panefleet themes" \
    -s "$(theme_popup_style)" \
    -S "$(theme_popup_border_style)" \
    "${SELF} theme-select"
}
