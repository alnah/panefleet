#!/usr/bin/env bash

# Shared shell runtime helpers for panefleet entrypoints.
now_epoch() {
  date +%s
}

self_root() {
  printf '%s' "$PANEFLEET_SCRIPT_ROOT"
}

user_config_home() {
  panefleet_user_config_home
}

user_state_home() {
  panefleet_user_state_home
}

default_bridge_bin_path() {
  panefleet_default_bridge_bin_path
}

install_script_path() {
  printf '%s/scripts/install-tmux-bindings.sh' "$(self_root)"
}

build_bridge_script_path() {
  printf '%s/scripts/build-agent-bridge.sh' "$(self_root)"
}

install_bridge_script_path() {
  printf '%s/scripts/install-bridge.sh' "$(self_root)"
}

bridge_bin_path() {
  if [[ -n "${PANEFLEET_AGENT_BRIDGE_BIN:-}" ]]; then
    printf '%s' "$PANEFLEET_AGENT_BRIDGE_BIN"
    return
  fi

  printf '%s' "$(default_bridge_bin_path)"
}

touch_hook_command_value() {
  printf 'run-shell -b "%s touch \\"#{pane_id}\\""' "$(self_root)/bin/panefleet"
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

bridge_present() {
  [[ -x "$(bridge_bin_path)" ]]
}

opencode_plugin_dir() {
  printf '%s' "${PANEFLEET_OPENCODE_PLUGIN_DIR:-$HOME/.config/opencode/plugins}"
}

opencode_plugin_path() {
  printf '%s/panefleet.ts' "$(opencode_plugin_dir)"
}

codex_notify_wrapper_path() {
  printf '%s/scripts/codex-notify-bridge' "$(self_root)"
}

codex_app_server_wrapper_path() {
  printf '%s/scripts/codex-app-server-bridge' "$(self_root)"
}

claude_hook_wrapper_path() {
  printf '%s/scripts/claude-code-hook' "$(self_root)"
}

opencode_event_bridge_path() {
  printf '%s/scripts/opencode-event-bridge' "$(self_root)"
}

opencode_plugin_template_path() {
  printf '%s/integrations/opencode/panefleet.ts.template' "$(self_root)"
}

codex_config_path() {
  local candidate

  if [[ -n "${PANEFLEET_CODEX_CONFIG:-}" ]]; then
    printf '%s' "$PANEFLEET_CODEX_CONFIG"
    return
  fi
  if [[ -n "${CODEX_CONFIG:-}" ]]; then
    printf '%s' "$CODEX_CONFIG"
    return
  fi

  for candidate in "$HOME/.codex/config.toml" "$HOME/.config/codex/config.toml"; do
    if [[ -f "$candidate" ]]; then
      printf '%s' "$candidate"
      return
    fi
  done

  printf '%s/.codex/config.toml' "$HOME"
}

claude_settings_path() {
  if [[ -n "${PANEFLEET_CLAUDE_SETTINGS:-}" ]]; then
    printf '%s' "$PANEFLEET_CLAUDE_SETTINGS"
    return
  fi

  printf '%s/.claude/settings.json' "$HOME"
}

toml_escape_string() {
  local value="$1"

  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s' "$value"
}

js_escape_string() {
  local value="$1"

  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s' "$value"
}
