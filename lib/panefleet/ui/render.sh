#!/usr/bin/env bash

# Theme-aware ANSI rendering and preview formatting helpers.
hex_triplet() {
  local hex="${1#\#}"

  printf '%d;%d;%d' "0x${hex:0:2}" "0x${hex:2:2}" "0x${hex:4:2}"
}

hex_components() {
  local hex="${1#\#}"

  printf '%d %d %d' "0x${hex:0:2}" "0x${hex:2:2}" "0x${hex:4:2}"
}

resolve_color_mode() {
  local requested="${PANEFLEET_COLOR_MODE:-}"
  local client_features colors

  if [[ -n "$PANEFLEET_COLOR_MODE_RESOLVED" ]]; then
    return
  fi

  case "$requested" in
  truecolor | 24bit)
    PANEFLEET_COLOR_MODE_RESOLVED="truecolor"
    return
    ;;
  256 | 256color)
    PANEFLEET_COLOR_MODE_RESOLVED="256"
    return
    ;;
  ansi | 16)
    PANEFLEET_COLOR_MODE_RESOLVED="ansi"
    return
    ;;
  esac

  if [[ -n "${TMUX:-}" ]]; then
    client_features="$("${TMUX_BIN}" display-message -p '#{client_termfeatures}' 2>/dev/null || true)"
    if printf '%s\n' "$client_features" | grep -qi 'rgb'; then
      PANEFLEET_COLOR_MODE_RESOLVED="truecolor"
      return
    fi
  fi

  case "${COLORTERM:-}" in
  *truecolor* | *24bit*)
    PANEFLEET_COLOR_MODE_RESOLVED="truecolor"
    return
    ;;
  esac

  case "${TERM:-}" in
  *direct* | *truecolor*)
    PANEFLEET_COLOR_MODE_RESOLVED="truecolor"
    return
    ;;
  *256color*)
    PANEFLEET_COLOR_MODE_RESOLVED="256"
    return
    ;;
  esac

  colors="$(tput colors 2>/dev/null || true)"
  if [[ "$colors" =~ ^[0-9]+$ && "$colors" -ge 256 ]]; then
    PANEFLEET_COLOR_MODE_RESOLVED="256"
  else
    PANEFLEET_COLOR_MODE_RESOLVED="ansi"
  fi
}

xterm_cube_index() {
  local value="$1"

  if ((value < 48)); then
    printf '0'
  elif ((value < 115)); then
    printf '1'
  else
    printf '%d' $(((value - 35) / 40))
  fi
}

xterm_cube_value() {
  case "$1" in
  0) printf '0' ;;
  1) printf '95' ;;
  2) printf '135' ;;
  3) printf '175' ;;
  4) printf '215' ;;
  *) printf '255' ;;
  esac
}

theme_is_light() {
  local r g b

  read -r r g b <<<"$(hex_components "$THEME_BG")"
  (((r * 299 + g * 587 + b * 114) / 1000 >= 160))
}

semantic_ansi16_index() {
  local color="$1"
  local light_index dark_index

  if theme_is_light; then
    light_index=7
    dark_index=0
  else
    light_index=15
    dark_index=0
  fi

  case "$color" in
  "$THEME_RUN" | "$THEME_DIFF_ADD")
    printf '10'
    ;;
  "$THEME_WAIT")
    printf '11'
    ;;
  "$THEME_ERROR" | "$THEME_DIFF_REMOVE")
    printf '9'
    ;;
  "$THEME_IDLE" | "$THEME_DIFF_HEADER")
    printf '14'
    ;;
  "$THEME_ACCENT" | "$THEME_HEADER" | "$THEME_DIFF_HUNK")
    printf '13'
    ;;
  "$THEME_DONE" | "$THEME_STALE" | "$THEME_MUTED" | "$THEME_BORDER")
    printf '8'
    ;;
  "$THEME_BG" | "$THEME_BG_ALT")
    printf '%s' "$dark_index"
    ;;
  "$THEME_FG" | "$THEME_SELECTION_FG")
    printf '%s' "$light_index"
    ;;
  "$THEME_SELECTION_BG")
    printf '4'
    ;;
  *)
    return 1
    ;;
  esac
}

hex_to_ansi256() {
  local color="$1"
  local r g b
  local cube_r cube_g cube_b cube_index
  local cube_rv cube_gv cube_bv cube_distance
  local gray gray_index gray_value gray_distance

  read -r r g b <<<"$(hex_components "$color")"

  cube_r="$(xterm_cube_index "$r")"
  cube_g="$(xterm_cube_index "$g")"
  cube_b="$(xterm_cube_index "$b")"
  cube_index=$((16 + 36 * cube_r + 6 * cube_g + cube_b))

  cube_rv="$(xterm_cube_value "$cube_r")"
  cube_gv="$(xterm_cube_value "$cube_g")"
  cube_bv="$(xterm_cube_value "$cube_b")"
  cube_distance=$(((r - cube_rv) * (r - cube_rv) + (g - cube_gv) * (g - cube_gv) + (b - cube_bv) * (b - cube_bv)))

  gray=$(((r + g + b) / 3))
  if ((gray < 8)); then
    gray_index=232
    gray_value=8
  elif ((gray > 238)); then
    gray_index=255
    gray_value=238
  else
    gray_index=$((232 + (gray - 8) / 10))
    gray_value=$((8 + 10 * (gray_index - 232)))
  fi
  gray_distance=$(((r - gray_value) * (r - gray_value) + (g - gray_value) * (g - gray_value) + (b - gray_value) * (b - gray_value)))

  if ((gray_distance < cube_distance)); then
    printf '%s' "$gray_index"
  else
    printf '%s' "$cube_index"
  fi
}

hex_to_ansi16_index() {
  local color="$1"
  local r g b
  local best_index=0
  local best_distance=-1
  local idx palette_hex pr pg pb distance
  local -a palette=(
    "000000" "800000" "008000" "808000"
    "000080" "800080" "008080" "c0c0c0"
    "808080" "ff0000" "00ff00" "ffff00"
    "0000ff" "ff00ff" "00ffff" "ffffff"
  )

  if semantic_ansi16_index "$color" >/dev/null 2>&1; then
    semantic_ansi16_index "$color"
    return
  fi

  read -r r g b <<<"$(hex_components "$color")"
  for idx in "${!palette[@]}"; do
    palette_hex="${palette[$idx]}"
    pr=$((16#${palette_hex:0:2}))
    pg=$((16#${palette_hex:2:2}))
    pb=$((16#${palette_hex:4:2}))
    distance=$(((r - pr) * (r - pr) + (g - pg) * (g - pg) + (b - pb) * (b - pb)))
    if ((best_distance == -1 || distance < best_distance)); then
      best_distance="$distance"
      best_index="$idx"
    fi
  done

  printf '%s' "$best_index"
}

ansi_fg_code() {
  local index

  index="$(hex_to_ansi16_index "$1")"
  if ((index < 8)); then
    printf '%d' $((30 + index))
  else
    printf '%d' $((90 + index - 8))
  fi
}

ansi_bg_code() {
  local index

  index="$(hex_to_ansi16_index "$1")"
  if ((index < 8)); then
    printf '%d' $((40 + index))
  else
    printf '%d' $((100 + index - 8))
  fi
}

fzf_color_value() {
  local color="$1"

  resolve_color_mode
  case "$PANEFLEET_COLOR_MODE_RESOLVED" in
  truecolor)
    printf '%s' "$color"
    ;;
  256)
    printf '%s' "$(hex_to_ansi256 "$color")"
    ;;
  *)
    printf '%s' "$(hex_to_ansi16_index "$color")"
    ;;
  esac
}

tmux_style_color() {
  local color="$1"

  resolve_color_mode
  case "$PANEFLEET_COLOR_MODE_RESOLVED" in
  truecolor)
    printf '%s' "$color"
    ;;
  256)
    printf 'colour%s' "$(hex_to_ansi256 "$color")"
    ;;
  *)
    printf 'colour%s' "$(hex_to_ansi16_index "$color")"
    ;;
  esac
}

paint_fg() {
  local text="$1"
  local color="$2"
  local attrs="${3:-}"
  local prefix="\033["

  resolve_color_mode

  if [[ -n "$attrs" ]]; then
    prefix+="${attrs};"
  fi

  case "$PANEFLEET_COLOR_MODE_RESOLVED" in
  truecolor)
    prefix+="38;2;$(hex_triplet "$color")m"
    ;;
  256)
    prefix+="38;5;$(hex_to_ansi256 "$color")m"
    ;;
  *)
    prefix+="$(ansi_fg_code "$color")m"
    ;;
  esac

  printf '%b%s\033[0m' "$prefix" "$text"
}

paint_bgfg() {
  local text="$1"
  local fg="$2"
  local bg="$3"
  local attrs="${4:-}"
  local prefix="\033["

  resolve_color_mode

  if [[ -n "$attrs" ]]; then
    prefix+="${attrs};"
  fi

  case "$PANEFLEET_COLOR_MODE_RESOLVED" in
  truecolor)
    prefix+="38;2;$(hex_triplet "$fg");48;2;$(hex_triplet "$bg")m"
    ;;
  256)
    prefix+="38;5;$(hex_to_ansi256 "$fg");48;5;$(hex_to_ansi256 "$bg")m"
    ;;
  *)
    prefix+="$(ansi_fg_code "$fg");$(ansi_bg_code "$bg")m"
    ;;
  esac

  printf '%b%s\033[0m' "$prefix" "$text"
}

status_color() {
  local status="$1"
  local text="${2:-$status}"

  case "$status" in
  RUN) paint_fg "$text" "$THEME_RUN" "1" ;;
  WAIT) paint_fg "$text" "$THEME_WAIT" "1" ;;
  DONE) paint_fg "$text" "$THEME_DONE" ;;
  ERROR) paint_fg "$text" "$THEME_ERROR" "1" ;;
  IDLE) paint_fg "$text" "$THEME_IDLE" ;;
  STALE) paint_fg "$text" "$THEME_STALE" "2" ;;
  *) printf '%s' "$text" ;;
  esac
}

fit_cell() {
  local value="$1"
  local width="$2"

  printf "%-${width}.${width}s" "$value"
}

fit_cell_into() {
  local outvar="$1"
  local value="$2"
  local width="$3"

  printf -v "$outvar" "%-${width}.${width}s" "$value"
}

colored_status_cell() {
  local status="$1"

  status_color "$status" "$(fit_cell "$status" 5)"
}

header_cell() {
  local text="$1"

  paint_fg "$text" "$THEME_HEADER" "1"
}

separator_cell() {
  paint_fg " │ " "$THEME_BORDER_SOFT"
}

preview_label() {
  paint_fg "$(fit_cell "$1" 7)" "$THEME_FG_SUBTLE" "1"
}

preview_value() {
  local text="$1"
  local color="${2:-$THEME_FG}"

  paint_fg "$text" "$color"
}

tokens_color() {
  local tokens="$1"
  local text="${2:-$tokens}"

  if [[ ! "$tokens" =~ ^[0-9]+$ ]]; then
    paint_fg "$text" "$THEME_FG_SUBTLE"
    return
  fi

  if ((tokens >= 100000)); then
    paint_fg "$text" "$THEME_WAIT" "1"
  else
    paint_fg "$text" "$THEME_FG"
  fi
}

context_left_color() {
  local pct="$1"
  local text="${2:-$pct%}"

  if [[ ! "$pct" =~ ^[0-9]+$ ]]; then
    paint_fg "$text" "$THEME_FG_SUBTLE"
    return
  fi

  if ((pct >= 60)); then
    paint_fg "$text" "$THEME_RUN" "1"
  elif ((pct >= 30)); then
    paint_fg "$text" "$THEME_WAIT" "1"
  else
    paint_fg "$text" "$THEME_ERROR" "1"
  fi
}

preview_pad_bg() {
  local text="$1"
  local fg="${2:-$THEME_FG}"
  local attrs="${4:-}"

  # Avoid per-line background blocks to keep the preview panel visually flat.
  paint_fg " $text " "$fg" "$attrs"
}

is_indented_code_like() {
  local line="$1"

  case "$line" in
  '    $ '* | '    > '* | '    +'* | '    -'* | '    @'* | '    {'* | '    }'*)
    return 0
    ;;
  '    '*';'* | '    '*'.'* | '    '*'='* | '    '*'() {'* | '    '*': '*)
    return 0
    ;;
  '    '*if\ * | '    '*for\ * | '    '*while\ * | '    '*case\ * | '    '*return\ * | '    '*func\ * | '    '*package\ * | '    '*import\ * | '    '*const\ * | '    '*let\ * | '    '*var\ * | '    '*class\ * | '    '*def\ *)
    return 0
    ;;
  *)
    return 1
    ;;
  esac
}

preview_line_kind() {
  local line="$1"

  if is_indented_code_like "$line"; then
    printf 'code'
    return
  fi

  if [[ "$line" =~ ^(diff[[:space:]]--git|index[[:space:]][0-9a-f]|@@|---[[:space:]]|\\+\\+\\+[[:space:]]) ]]; then
    printf 'code'
    return
  fi

  case "$line" in
  "")
    printf 'blank'
    ;;
  '```'*)
    printf 'fence'
    ;;
  '# '* | '## '* | '### '* | '#### '*)
    printf 'heading'
    ;;
  '> '*)
    printf 'quote'
    ;;
  '- '* | '* '* | '+ '*)
    printf 'list'
    ;;
  [0-9]*'. '*)
    printf 'list'
    ;;
  *"────────────────"* | *"────────────"* | *"━━━━"* | *"----"*)
    printf 'separator'
    ;;
  "• "* | "  └ "* | "  ├ "* | "  │ "* | "│ "* | "└ "* | "├ "* | "╰"* | "╭"* | "╹"* | "▣ "* | "⬝"* | '$ '* | '› '*)
    printf 'shell'
    ;;
  *error:* | *Error:* | *ERROR*)
    printf 'error'
    ;;
  *warning:* | *Warning:* | *WARNING*)
    printf 'warning'
    ;;
  *)
    printf 'prose'
    ;;
  esac
}

render_separator_line() {
  preview_value "────────────────────────────────────────" "$THEME_BORDER"
  printf '\n'
}

render_heading_line() {
  local line="$1"
  paint_fg "$line" "$THEME_HEADER" "1"
  printf '\n'
}

render_quote_line() {
  local line="$1"
  preview_value "$line" "$THEME_MUTED"
  printf '\n'
}

render_list_line() {
  local line="$1"
  local marker body

  if [[ "$line" =~ ^([[:space:]]*([0-9]+\.)|[[:space:]]*[-*+])[[:space:]]+(.*)$ ]]; then
    marker="${BASH_REMATCH[1]}"
    body="${BASH_REMATCH[3]}"
    printf '%s %s\n' "$(paint_fg "$marker" "$THEME_ACCENT" "1")" "$(preview_value "$body")"
  else
    printf '%s\n' "$line"
  fi
}

render_shell_line() {
  local line="$1"

  case "$line" in
  "• Working"*)
    preview_pad_bg "$line" "$THEME_RUN" "$THEME_BG_ALT" "1"
    ;;
  "• Ran "* | "• Explored"* | "• Edited"* | "• Read "*)
    preview_pad_bg "$line" "$THEME_ACCENT" "$THEME_BG_ALT" "1"
    ;;
  *error:* | *Error:* | *ERROR*)
    preview_pad_bg "$line" "$THEME_ERROR" "$THEME_BG_ALT" "1"
    ;;
  *warning:* | *Warning:* | *WARNING*)
    preview_pad_bg "$line" "$THEME_WAIT" "$THEME_BG_ALT" "1"
    ;;
  "› "* | '$ '*)
    preview_pad_bg "$line" "$THEME_ACCENT" "$THEME_BG_ALT"
    ;;
  "  └ "* | "  ├ "* | "  │ "* | "│ "* | "└ "* | "├ "* | "╰"* | "╭"* | "╹"* | "▣ "* | "⬝"*)
    preview_pad_bg "$line" "$THEME_MUTED" "$THEME_BG_ALT"
    ;;
  *)
    preview_pad_bg "$line" "$THEME_FG" "$THEME_BG_ALT"
    ;;
  esac
  printf '\n'
}

render_code_line() {
  local line="$1"

  case "$line" in
  "diff --git"* | "index "* | "@@ "* | "--- "* | "+++ "*)
    if [[ "$line" == "@@ "* ]]; then
      preview_pad_bg "$line" "$THEME_DIFF_HUNK" "$THEME_BG_ALT" "1"
    else
      preview_pad_bg "$line" "$THEME_DIFF_HEADER" "$THEME_BG_ALT" "1"
    fi
    ;;
  "    +"* | "+"*)
    preview_pad_bg "$line" "$THEME_DIFF_ADD" "$THEME_BG_ALT"
    ;;
  "    -"* | "-"*)
    preview_pad_bg "$line" "$THEME_DIFF_REMOVE" "$THEME_BG_ALT"
    ;;
  *error:* | *Error:* | *ERROR*)
    preview_pad_bg "$line" "$THEME_ERROR" "$THEME_BG_ALT" "1"
    ;;
  *warning:* | *Warning:* | *WARNING*)
    preview_pad_bg "$line" "$THEME_WAIT" "$THEME_BG_ALT" "1"
    ;;
  *)
    preview_pad_bg "$line" "$THEME_FG" "$THEME_BG_ALT"
    ;;
  esac
  printf '\n'
}

render_prose_line() {
  local line="$1"
  printf '%s\n' "$(preview_value "$line")"
}

render_block() {
  local kind="$1"
  shift
  local lines=("$@")
  local line

  case "$kind" in
  shell)
    for line in "${lines[@]}"; do
      render_shell_line "$line"
    done
    ;;
  code)
    for line in "${lines[@]}"; do
      render_code_line "$line"
    done
    ;;
  heading)
    for line in "${lines[@]}"; do
      render_heading_line "$line"
    done
    ;;
  list)
    for line in "${lines[@]}"; do
      render_list_line "$line"
    done
    ;;
  quote)
    for line in "${lines[@]}"; do
      render_quote_line "$line"
    done
    ;;
  error)
    for line in "${lines[@]}"; do
      preview_pad_bg "$line" "$THEME_ERROR" "$THEME_BG_ALT" "1"
      printf '\n'
    done
    ;;
  warning)
    for line in "${lines[@]}"; do
      preview_pad_bg "$line" "$THEME_WAIT" "$THEME_BG_ALT" "1"
      printf '\n'
    done
    ;;
  prose | *)
    for line in "${lines[@]}"; do
      render_prose_line "$line"
    done
    ;;
  esac
}

render_preview_capture() {
  local in_code=0
  local current_kind=""
  local line_kind
  local line
  local block_lines=()

  flush_preview_block() {
    if [[ -n "$current_kind" && "${#block_lines[@]}" -gt 0 ]]; then
      render_block "$current_kind" "${block_lines[@]}"
      block_lines=()
    fi
    current_kind=""
  }

  while IFS= read -r line; do
    if ((in_code)); then
      if [[ "$line" == '```'* ]]; then
        flush_preview_block
        in_code=0
      else
        if [[ "$current_kind" != "code" ]]; then
          flush_preview_block
          current_kind="code"
        fi
        block_lines+=("$line")
      fi
      continue
    fi

    line_kind="$(preview_line_kind "$line")"
    case "$line_kind" in
    blank)
      flush_preview_block
      printf '\n'
      ;;
    fence)
      flush_preview_block
      in_code=1
      current_kind="code"
      ;;
    separator)
      flush_preview_block
      render_separator_line
      ;;
    *)
      if [[ "$line_kind" != "$current_kind" ]]; then
        flush_preview_block
        current_kind="$line_kind"
      fi
      block_lines+=("$line")
      ;;
    esac
  done

  flush_preview_block
}

fzf_color_spec() {
  resolve_color_mode
  printf 'bg:%s,fg:%s,preview-bg:%s,preview-fg:%s,bg+:%s,fg+:%s,hl:%s,hl+:%s,info:%s,border:%s,list-border:%s,preview-border:%s,header-border:%s,input-border:%s,separator:%s,prompt:%s,pointer:%s,marker:%s,spinner:%s,scrollbar:%s,header:%s,label:%s,gutter:%s' \
    "$(fzf_color_value "$THEME_BG")" \
    "$(fzf_color_value "$THEME_FG")" \
    "$(fzf_color_value "$THEME_BG")" \
    "$(fzf_color_value "$THEME_FG")" \
    "$(fzf_color_value "$THEME_FOCUS_BG")" \
    "$(fzf_color_value "$THEME_FOCUS_FG")" \
    "$(fzf_color_value "$THEME_ACCENT")" \
    "$(fzf_color_value "$THEME_ACCENT")" \
    "$(fzf_color_value "$THEME_FG_SUBTLE")" \
    "$(fzf_color_value "$THEME_BORDER_SOFT")" \
    "$(fzf_color_value "$THEME_BORDER_SOFT")" \
    "$(fzf_color_value "$THEME_BORDER_SOFT")" \
    "$(fzf_color_value "$THEME_BORDER_STRONG")" \
    "$(fzf_color_value "$THEME_BORDER_SOFT")" \
    "$(fzf_color_value "$THEME_BORDER_SOFT")" \
    "$(fzf_color_value "$THEME_ACCENT")" \
    "$(fzf_color_value "$THEME_FOCUS_MARKER")" \
    "$(fzf_color_value "$THEME_ACCENT")" \
    "$(fzf_color_value "$THEME_FOCUS_MARKER")" \
    "$(fzf_color_value "$THEME_FG_SUBTLE")" \
    "$(fzf_color_value "$THEME_HEADER")" \
    "$(fzf_color_value "$THEME_HEADER")" \
    "$(fzf_color_value "$THEME_BG")"
}
