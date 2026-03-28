#!/usr/bin/env bash

# Theme selector, theme preview, and popup UI commands.
board_popup_width() {
  printf '%s' "${PANEFLEET_BOARD_POPUP_WIDTH:-100%}"
}

board_popup_height() {
  printf '%s' "${PANEFLEET_BOARD_POPUP_HEIGHT:-100%}"
}

theme_popup_width() {
  printf '%s' "${PANEFLEET_THEME_POPUP_WIDTH:-$(board_popup_width)}"
}

theme_popup_height() {
  printf '%s' "${PANEFLEET_THEME_POPUP_HEIGHT:-$(board_popup_height)}"
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

    display="$(printf '%s %s' "$marker" "$theme_name")"
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

theme_preview_columns() {
  local columns="${FZF_PREVIEW_COLUMNS:-${COLUMNS:-}}"

  if [[ ! "$columns" =~ ^[0-9]+$ ]] || ((columns < 40)); then
    if command -v tput >/dev/null 2>&1; then
      columns="$(tput cols 2>/dev/null || true)"
    fi
  fi

  if [[ ! "$columns" =~ ^[0-9]+$ ]] || ((columns < 40)); then
    columns=80
  fi

  printf '%s' "$columns"
}

theme_preview_mode() {
  local columns="${1:?preview columns are required}"

  if ((columns < 74)); then
    printf 'narrow'
  elif ((columns < 110)); then
    printf 'medium'
  else
    printf 'full'
  fi
}

theme_preview_meta() {
  local resolved_theme_name="$1"
  local mode="$6"

  printf 'theme   %s\n' "$resolved_theme_name"
  theme_preview_palette "$mode"
}

theme_preview_bg_sequence() {
  resolve_color_mode

  case "$PANEFLEET_COLOR_MODE_RESOLVED" in
  truecolor)
    printf '\033[48;2;%sm' "$(hex_triplet "$THEME_BG")"
    ;;
  256)
    printf '\033[48;5;%sm' "$(hex_to_ansi256 "$THEME_BG")"
    ;;
  *)
    printf '\033[%sm' "$(ansi_bg_code "$THEME_BG")"
    ;;
  esac
}

theme_preview_strip_ansi() {
  perl -pe 's/\e\[[0-9;]*m//g'
}

theme_preview_paint_bg() {
  local columns="${1:?preview columns are required}"
  local bg_seq reset line plain pad_width pad

  bg_seq="$(theme_preview_bg_sequence)"
  reset=$'\033[0m'

  while IFS= read -r line || [[ -n "$line" ]]; do
    plain="$(printf '%s' "$line" | theme_preview_strip_ansi)"
    pad_width=$((columns - ${#plain}))
    if ((pad_width < 0)); then
      pad_width=0
    fi
    printf -v pad '%*s' "$pad_width" ''
    line="${line//${reset}/${reset}${bg_seq}}"
    printf '%b%s%s%b\n' "$bg_seq" "$line" "$pad" "$reset"
  done
}

theme_preview_swatch() {
  local label="$1"
  local color="$2"

  printf '%s %s' \
    "$(paint_fg "$(fit_cell "$label" 7)" "$THEME_MUTED")" \
    "$(paint_fg "████" "$color")"
}

theme_preview_palette_narrow() {
  printf '%s\n' "$(paint_fg "PALETTE" "$THEME_HEADER" "1")"
  printf '%s  %s\n' \
    "$(theme_preview_swatch "bg" "$THEME_BG")" \
    "$(theme_preview_swatch "fg" "$THEME_FG")"
  printf '%s  %s\n' \
    "$(theme_preview_swatch "accent" "$THEME_ACCENT")" \
    "$(theme_preview_swatch "border" "$THEME_BORDER")"
  printf '%s  %s\n' \
    "$(theme_preview_swatch "run" "$THEME_RUN")" \
    "$(theme_preview_swatch "wait" "$THEME_WAIT")"
  printf '%s  %s\n' \
    "$(theme_preview_swatch "done" "$THEME_DONE")" \
    "$(theme_preview_swatch "error" "$THEME_ERROR")"
}

theme_preview_palette_medium() {
  printf '%s\n' "$(paint_fg "PALETTE" "$THEME_HEADER" "1")"
  printf '%s  %s  %s\n' \
    "$(theme_preview_swatch "bg" "$THEME_BG")" \
    "$(theme_preview_swatch "fg" "$THEME_FG")" \
    "$(theme_preview_swatch "accent" "$THEME_ACCENT")"
  printf '%s  %s  %s\n' \
    "$(theme_preview_swatch "border" "$THEME_BORDER")" \
    "$(theme_preview_swatch "run" "$THEME_RUN")" \
    "$(theme_preview_swatch "wait" "$THEME_WAIT")"
  printf '%s  %s  %s\n' \
    "$(theme_preview_swatch "done" "$THEME_DONE")" \
    "$(theme_preview_swatch "error" "$THEME_ERROR")" \
    "$(theme_preview_swatch "idle" "$THEME_IDLE")"
}

theme_preview_palette_full() {
  printf '%s\n' "$(paint_fg "PALETTE" "$THEME_HEADER" "1")"
  printf '%s  %s  %s  %s\n' \
    "$(theme_preview_swatch "bg" "$THEME_BG")" \
    "$(theme_preview_swatch "fg" "$THEME_FG")" \
    "$(theme_preview_swatch "border" "$THEME_BORDER")" \
    "$(theme_preview_swatch "accent" "$THEME_ACCENT")"
  printf '%s  %s  %s  %s\n' \
    "$(theme_preview_swatch "run" "$THEME_RUN")" \
    "$(theme_preview_swatch "wait" "$THEME_WAIT")" \
    "$(theme_preview_swatch "done" "$THEME_DONE")" \
    "$(theme_preview_swatch "error" "$THEME_ERROR")"
  printf '%s  %s  %s\n' \
    "$(theme_preview_swatch "idle" "$THEME_IDLE")" \
    "$(theme_preview_swatch "stale" "$THEME_STALE")" \
    "$(theme_preview_swatch "diff" "$THEME_DIFF_HUNK")"
}

theme_preview_palette() {
  local mode="$1"

  case "$mode" in
  narrow)
    theme_preview_palette_narrow
    ;;
  medium)
    theme_preview_palette_medium
    ;;
  *)
    theme_preview_palette_full
    ;;
  esac
}

theme_preview_board_full() {
  printf '%s\n' "$(paint_fg "BOARD" "$THEME_HEADER" "1")"
  printf '%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "  " \
    "$(header_cell "$(fit_cell "STATE" 5)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "TOOL" 7)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "TARGET" 8)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "SESSION" 12)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "WINDOW" 12)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "REPO" 12)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "TOKENS" 10)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "CTX%" 5)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "AGE" 4)")"
  printf '%s\n' "$(paint_fg "▌" "$THEME_ACCENT" "1") $(colored_status_cell "RUN")$(separator_cell)$(preview_value "$(fit_cell "codex" 7)")$(separator_cell)$(preview_value "$(fit_cell "2.0" 8)")$(separator_cell)$(preview_value "$(fit_cell "panefleet" 12)")$(separator_cell)$(preview_value "$(fit_cell "board" 12)")$(separator_cell)$(preview_value "$(fit_cell "panefleet" 12)")$(separator_cell)$(tokens_color "41226488" "$(fit_cell "41226488" 10)")$(separator_cell)$(context_left_color "41" "$(fit_cell "41%" 5)")$(separator_cell)$(preview_value "$(fit_cell "10s" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "WAIT")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "codex" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "3.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "panefleet" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "teach" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "panefleet" 12)")" "$(separator_cell)" \
    "$(tokens_color "153096" "$(fit_cell "153096" 10)")" "$(separator_cell)" \
    "$(context_left_color "12" "$(fit_cell "12%" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "24s" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "DONE")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "claude" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "4.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "panefleet" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "claude" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "fle" 12)")" "$(separator_cell)" \
    "$(tokens_color "29666586" "$(fit_cell "29666586" 10)")" "$(separator_cell)" \
    "$(context_left_color "0" "$(fit_cell "0%" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "1m" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "ERROR")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "opencode" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "5.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "panefleet" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "review" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "fle" 12)")" "$(separator_cell)" \
    "$(tokens_color "247366" "$(fit_cell "247366" 10)")" "$(separator_cell)" \
    "$(context_left_color "4" "$(fit_cell "4%" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "3m" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "IDLE")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "shell" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "0.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "teach" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "zsh" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "alnah.me" 12)")" "$(separator_cell)" \
    "$(tokens_color "-" "$(fit_cell "-" 10)")" "$(separator_cell)" \
    "$(context_left_color "-" "$(fit_cell "-" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "8m" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "STALE")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "shell" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "0.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "teach" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "zsh" 12)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "alnah.me" 12)")" "$(separator_cell)" \
    "$(tokens_color "-" "$(fit_cell "-" 10)")" "$(separator_cell)" \
    "$(context_left_color "-" "$(fit_cell "-" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "16m" 4)")"
}

theme_preview_board_medium() {
  printf '%s\n' "$(paint_fg "BOARD" "$THEME_HEADER" "1")"
  printf '%s%s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "  " \
    "$(header_cell "$(fit_cell "STATE" 5)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "TOOL" 7)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "TARGET" 8)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "WINDOW" 12)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "TOKENS" 10)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "CTX%" 5)")" "$(separator_cell)" \
    "$(header_cell "$(fit_cell "AGE" 4)")"
  printf '%s\n' "$(paint_fg "▌" "$THEME_ACCENT" "1") $(colored_status_cell "RUN")$(separator_cell)$(preview_value "$(fit_cell "codex" 7)")$(separator_cell)$(preview_value "$(fit_cell "2.0" 8)")$(separator_cell)$(preview_value "$(fit_cell "board" 12)")$(separator_cell)$(tokens_color "41226488" "$(fit_cell "41226488" 10)")$(separator_cell)$(context_left_color "41" "$(fit_cell "41%" 5)")$(separator_cell)$(preview_value "$(fit_cell "10s" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "WAIT")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "codex" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "3.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "teach" 12)")" "$(separator_cell)" \
    "$(tokens_color "153096" "$(fit_cell "153096" 10)")" "$(separator_cell)" \
    "$(context_left_color "12" "$(fit_cell "12%" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "24s" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "DONE")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "claude" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "4.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "claude" 12)")" "$(separator_cell)" \
    "$(tokens_color "29666586" "$(fit_cell "29666586" 10)")" "$(separator_cell)" \
    "$(context_left_color "0" "$(fit_cell "0%" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "1m" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "ERROR")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "opencode" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "5.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "review" 12)")" "$(separator_cell)" \
    "$(tokens_color "247366" "$(fit_cell "247366" 10)")" "$(separator_cell)" \
    "$(context_left_color "4" "$(fit_cell "4%" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "3m" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "IDLE")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "shell" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "0.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "zsh" 12)")" "$(separator_cell)" \
    "$(tokens_color "-" "$(fit_cell "-" 10)")" "$(separator_cell)" \
    "$(context_left_color "-" "$(fit_cell "-" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "8m" 4)")"
  printf '  %s%s%s%s%s%s%s%s%s%s%s%s%s\n' \
    "$(colored_status_cell "STALE")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "shell" 7)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "0.0" 8)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "zsh" 12)")" "$(separator_cell)" \
    "$(tokens_color "-" "$(fit_cell "-" 10)")" "$(separator_cell)" \
    "$(context_left_color "-" "$(fit_cell "-" 5)")" "$(separator_cell)" \
    "$(preview_value "$(fit_cell "16m" 4)")"
}

theme_preview_board_narrow() {
  printf '%s\n' "$(paint_fg "BOARD" "$THEME_HEADER" "1")"
  printf '%s\n' "$(paint_fg "▌" "$THEME_ACCENT" "1") $(colored_status_cell "RUN") $(preview_value "codex 2.0") $(context_left_color "41" "41%") $(preview_value "10s")"
  printf '%s\n' "  $(colored_status_cell "WAIT") $(preview_value "codex 3.0") $(context_left_color "12" "12%") $(preview_value "24s")"
  printf '%s\n' "  $(colored_status_cell "DONE") $(preview_value "claude 4.0") $(context_left_color "0" "0%") $(preview_value "1m")"
  printf '%s\n' "  $(colored_status_cell "ERROR") $(preview_value "opencode 5.0") $(context_left_color "4" "4%") $(preview_value "3m")"
  printf '%s\n' "  $(colored_status_cell "IDLE") $(preview_value "shell 0.0") $(preview_value "8m")"
  printf '%s\n' "  $(colored_status_cell "STALE") $(preview_value "shell 0.0") $(preview_value "16m")"
  printf '  %s\n' "$(preview_value "board · panefleet")"
  printf '  %s %s\n' "$(preview_label "tokens")" "$(tokens_color "41226488" "41226488")"
  printf '  %s\n' "$(preview_value "paths, links, warnings, diffs" "$THEME_FG_SUBTLE")"
}

theme_preview_capture() {
  local mode="$1"

  printf '%s\n' "$(paint_fg "PREVIEW" "$THEME_HEADER" "1")"

  case "$mode" in
  narrow)
    render_preview_capture <<'EOF'
# preview
You can preview a coding-agent chat here.

› open /Users/alexis/workspace/panefleet/internal/tui/board_theme.go
+ clearer links and file paths
- washed out selected row
EOF
    ;;
  medium)
    render_preview_capture <<'EOF'
• You can preview a coding-agent session here
› open /Users/alexis/workspace/panefleet/internal/tui/board_theme.go

# theme preview
Commands, file paths, links, warnings, and diffs should stay readable.

+ improve link and file emphasis
- washed out selected row
EOF
    ;;
  *)
    render_preview_capture <<'EOF'
• You can preview the chat with your coding agent here
› open /Users/alexis/workspace/panefleet/internal/tui/board_theme.go
› open /Users/alexis/workspace/panefleet/README.md

# theme preview
Commands, file paths, links, warnings, and diffs should stay readable.

diff --git a/internal/tui/board_theme.go b/internal/tui/board_theme.go
@@ preview
+ improve link and file emphasis
- washed out selected row
EOF
    ;;
  esac
}

theme_preview() {
  local theme_name="${1:?theme name is required}"
  local text_ratio ui_ratio text_pass ui_pass resolved_theme_name columns mode

  PANEFLEET_THEME="$theme_name"
  resolve_theme
  resolved_theme_name="${THEME_NAME:-$theme_name}"
  columns="$(theme_preview_columns)"
  mode="$(theme_preview_mode "$columns")"
  text_ratio="$(contrast_ratio_x100 "$THEME_FG" "$THEME_BG")"
  ui_ratio="$(contrast_ratio_x100 "$THEME_BORDER_STRONG" "$THEME_BG")"
  text_pass="$(contrast_badge_text "$text_ratio" 450)"
  ui_pass="$(contrast_badge_text "$ui_ratio" 300)"

  {
    theme_preview_meta "$resolved_theme_name" "$text_ratio" "$ui_ratio" "$text_pass" "$ui_pass" "$mode"
    printf '\n'
    case "$mode" in
    narrow)
      theme_preview_board_narrow
      ;;
    medium)
      theme_preview_board_medium
      ;;
    *)
      theme_preview_board_full
      ;;
    esac
    printf '\n'
    theme_preview_capture "$mode"
  } | theme_preview_paint_bg "$columns"
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
    --header='[⏎] apply theme' \
    --separator='─' \
    --pointer='▌' \
    --marker='•' \
    --preview "${SELF} theme-preview {1}" \
    --preview-window='right,78%,border-left,wrap' \
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
    -w "$(theme_popup_width)" \
    -h "$(theme_popup_height)" \
    -T "panefleet themes" \
    -s "$(theme_popup_style)" \
    -S "$(theme_popup_border_style)" \
    "${SELF} theme-select"
}
