#!/usr/bin/env bash

# Install and uninstall command handlers.
# It keeps wiring idempotent and ensures provider setup always includes core setup.
install_command() {
  local root target="${1:-core}"

  if ! is_provider_target "$target"; then
    printf 'unknown install target: %s\n' "$target" >&2
    printf 'usage: %s install core|codex|claude|opencode|all\n' "$SELF" >&2
    exit 1
  fi

  if (($# > 1)); then
    printf 'unexpected extra arguments for install: %s\n' "${*:2}" >&2
    printf 'usage: %s install core|codex|claude|opencode|all\n' "$SELF" >&2
    exit 1
  fi

  preflight
  root="$(self_root)"

  case "$target" in
  core)
    if [[ -n "${TMUX:-}" ]]; then
      PANEFLEET_ROOT="$root" "$(install_script_path)" install >/dev/null
    else
      :
    fi
    print_core_installed_message
    ;;
  codex | claude | opencode | all)
    if [[ -n "${TMUX:-}" ]]; then
      PANEFLEET_ROOT="$root" "$(install_script_path)" install >/dev/null
    else
      :
    fi
    print_core_installed_message
    install_integrations_targets "$target"
    ;;
  esac
}

uninstall_command() {
  local root

  require_tmux
  root="$(self_root)"
  PANEFLEET_ROOT="$root" "$(install_script_path)" uninstall
}

print_integration_wiring_message() {
  local target="$1"

  case "$target" in
  codex)
    printf '  codex config: %s\n' "$(codex_config_path)"
    ;;
  claude)
    printf '  claude settings: %s\n' "$(claude_settings_path)"
    ;;
  opencode)
    printf '  opencode plugin: %s\n' "$(opencode_plugin_path)"
    if ! command_exists bun; then
      printf '  warning: bun is missing; OpenCode plugin loading stays unavailable until bun is installed\n'
    fi
    ;;
  esac
}

print_installed_targets_message() {
  if ((${#PANEFLEET_INSTALL_TARGETS[@]} == 1)); then
    printf 'Integration installed: %s\n' "${PANEFLEET_INSTALL_TARGETS[0]}"
  else
    printf 'Integrations installed: %s\n' "$(join_by_comma_space "${PANEFLEET_INSTALL_TARGETS[@]}")"
  fi
}

print_bridge_install_message() {
  case "$PANEFLEET_BRIDGE_INSTALL_RESULT" in
  already-installed)
    printf 'Bridge: already installed\n'
    ;;
  downloaded-release)
    printf 'Bridge: downloaded from GitHub Releases\n'
    ;;
  built-local)
    printf 'Bridge: built locally with Go\n'
    ;;
  *)
    printf 'Bridge: installed\n'
    ;;
  esac
}

set_adapter_mode_auto_with_message() {
  if [[ -n "${TMUX:-}" ]]; then
    "${TMUX_BIN}" set-option -gq @panefleet-adapter-mode auto
    printf 'Adapter mode: auto\n'
  else
    printf 'Adapter mode will switch to auto the next time you run install inside tmux.\n'
  fi
}

# install_integrations_targets wires provider adapters and keeps bridge setup
# centralized, so each provider install path shares the same reliability checks.
install_integrations_targets() {
  local target

  resolve_integration_targets "$@"
  ensure_bridge_binary

  for target in "${PANEFLEET_INSTALL_TARGETS[@]}"; do
    case "$target" in
    codex)
      install_codex_integration
      print_integration_wiring_message codex
      ;;
    claude)
      install_claude_integration
      print_integration_wiring_message claude
      ;;
    opencode)
      install_opencode_plugin
      print_integration_wiring_message opencode
      ;;
    esac
  done

  print_installed_targets_message
  print_bridge_install_message
  set_adapter_mode_auto_with_message
}
