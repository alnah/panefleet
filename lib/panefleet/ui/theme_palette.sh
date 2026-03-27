#!/usr/bin/env bash

# Theme palette definitions and derived theme tokens.
set_theme_palette() {
  THEME_NAME="$1"
  THEME_BG="$2"
  THEME_BG_ALT="$3"
  THEME_FG="$4"
  THEME_MUTED="$5"
  THEME_BORDER="$6"
  THEME_ACCENT="$7"
  THEME_HEADER="$8"
  THEME_SELECTION_BG="$9"
  THEME_SELECTION_FG="${10}"
  THEME_RUN="${11}"
  THEME_WAIT="${12}"
  THEME_DONE="${13}"
  THEME_ERROR="${14}"
  THEME_IDLE="${15}"
  THEME_STALE="${16}"
  THEME_DIFF_HEADER="${17}"
  THEME_DIFF_ADD="${18}"
  THEME_DIFF_REMOVE="${19}"
  THEME_DIFF_HUNK="${20}"
}

available_themes() {
  cat <<'EOF'
panefleet-dark
panefleet-light
dracula
catppuccin-mocha
tokyo-night
gruvbox-dark
nord
solarized-dark
rose-pine
monokai
github-dark
EOF
}

resolve_theme() {
  local requested="${PANEFLEET_THEME:-}"

  if [[ -z "$requested" ]]; then
    requested="$(tmux_global_option @panefleet-theme)"
  fi

  case "$requested" in
  "" | panefleet-dark)
    set_theme_palette \
      "panefleet-dark" \
      "#11131a" "#181c25" "#e6e9ef" "#8b93a7" "#2b3241" "#7cc7ff" "#9fd6ff" \
      "#263248" "#eff3ff" "#72e39b" "#ffd166" "#93a1b7" "#ff7a90" "#7fb0ff" "#768096" \
      "#7cc7ff" "#72e39b" "#ff8da1" "#9fd6ff"
    ;;
  panefleet-light)
    set_theme_palette \
      "panefleet-light" \
      "#f8fafc" "#eef2f7" "#1f2937" "#6b7280" "#d2d8e2" "#2563eb" "#1d4ed8" \
      "#dbeafe" "#0f172a" "#15803d" "#b45309" "#64748b" "#dc2626" "#2563eb" "#94a3b8" \
      "#2563eb" "#15803d" "#dc2626" "#1d4ed8"
    ;;
  dracula)
    set_theme_palette \
      "dracula" \
      "#282a36" "#343746" "#f8f8f2" "#6272a4" "#44475a" "#bd93f9" "#ff79c6" \
      "#44475a" "#f8f8f2" "#50fa7b" "#f1fa8c" "#6272a4" "#ff5555" "#8be9fd" "#6272a4" \
      "#ff79c6" "#50fa7b" "#ff5555" "#bd93f9"
    ;;
  catppuccin-mocha)
    set_theme_palette \
      "catppuccin-mocha" \
      "#1e1e2e" "#313244" "#cdd6f4" "#9399b2" "#45475a" "#89b4fa" "#f5c2e7" \
      "#45475a" "#cdd6f4" "#a6e3a1" "#f9e2af" "#9399b2" "#f38ba8" "#89b4fa" "#6c7086" \
      "#89b4fa" "#a6e3a1" "#f38ba8" "#f5c2e7"
    ;;
  tokyo-night)
    set_theme_palette \
      "tokyo-night" \
      "#1a1b26" "#24283b" "#c0caf5" "#565f89" "#414868" "#7aa2f7" "#bb9af7" \
      "#283457" "#c0caf5" "#9ece6a" "#e0af68" "#565f89" "#f7768e" "#7dcfff" "#414868" \
      "#7aa2f7" "#9ece6a" "#f7768e" "#bb9af7"
    ;;
  gruvbox-dark)
    set_theme_palette \
      "gruvbox-dark" \
      "#282828" "#3c3836" "#ebdbb2" "#928374" "#504945" "#83a598" "#fabd2f" \
      "#3c3836" "#fbf1c7" "#b8bb26" "#fabd2f" "#928374" "#fb4934" "#83a598" "#7c6f64" \
      "#83a598" "#b8bb26" "#fb4934" "#fabd2f"
    ;;
  nord)
    set_theme_palette \
      "nord" \
      "#2e3440" "#3b4252" "#e5e9f0" "#81a1c1" "#4c566a" "#88c0d0" "#81a1c1" \
      "#434c5e" "#eceff4" "#a3be8c" "#ebcb8b" "#4c566a" "#bf616a" "#81a1c1" "#616e88" \
      "#88c0d0" "#a3be8c" "#bf616a" "#81a1c1"
    ;;
  solarized-dark)
    set_theme_palette \
      "solarized-dark" \
      "#002b36" "#073642" "#93a1a1" "#657b83" "#586e75" "#268bd2" "#2aa198" \
      "#073642" "#eee8d5" "#859900" "#b58900" "#657b83" "#dc322f" "#268bd2" "#586e75" \
      "#268bd2" "#859900" "#dc322f" "#2aa198"
    ;;
  rose-pine)
    set_theme_palette \
      "rose-pine" \
      "#191724" "#26233a" "#e0def4" "#908caa" "#403d52" "#9ccfd8" "#c4a7e7" \
      "#403d52" "#e0def4" "#9ccfd8" "#f6c177" "#908caa" "#eb6f92" "#c4a7e7" "#6e6a86" \
      "#9ccfd8" "#31748f" "#eb6f92" "#c4a7e7"
    ;;
  monokai)
    set_theme_palette \
      "monokai" \
      "#272822" "#3e3d32" "#f8f8f2" "#75715e" "#49483e" "#66d9ef" "#a6e22e" \
      "#49483e" "#f8f8f2" "#a6e22e" "#e6db74" "#75715e" "#f92672" "#66d9ef" "#75715e" \
      "#66d9ef" "#a6e22e" "#f92672" "#e6db74"
    ;;
  github-dark)
    set_theme_palette \
      "github-dark" \
      "#0d1117" "#161b22" "#c9d1d9" "#8b949e" "#30363d" "#58a6ff" "#79c0ff" \
      "#1f2a38" "#e6edf3" "#3fb950" "#d29922" "#8b949e" "#f85149" "#58a6ff" "#6e7681" \
      "#79c0ff" "#3fb950" "#f85149" "#d29922"
    ;;
  *)
    set_theme_palette \
      "panefleet-dark" \
      "#11131a" "#181c25" "#e6e9ef" "#8b93a7" "#2b3241" "#7cc7ff" "#9fd6ff" \
      "#263248" "#eff3ff" "#72e39b" "#ffd166" "#93a1b7" "#ff7a90" "#7fb0ff" "#768096" \
      "#7cc7ff" "#72e39b" "#ff8da1" "#9fd6ff"
    ;;
  esac

  derive_theme_tokens
}

derive_theme_tokens() {
  # Advanced visual tokens are derived from the base palette so all themes
  # receive the same UI semantics without expanding every theme definition.
  THEME_FG_SUBTLE="${THEME_MUTED}"
  THEME_BORDER_SOFT="${THEME_BORDER}"
  THEME_BORDER_STRONG="${THEME_HEADER}"
  THEME_FOCUS_BG="${THEME_SELECTION_BG}"
  THEME_FOCUS_FG="${THEME_SELECTION_FG}"
  THEME_FOCUS_MARKER="${THEME_ACCENT}"
}
