#!/usr/bin/env bash

# Integration install and bridge wiring helpers.
ensure_bridge_binary() {
  local root install_script output bridge_out

  root="$(self_root)"
  install_script="$(install_bridge_script_path)"
  bridge_out="$(bridge_bin_path)"
  if [[ ! -x "$install_script" ]]; then
    printf 'panefleet: missing bridge install script %s\n' "$install_script" >&2
    exit 1
  fi

  if ! output="$(PANEFLEET_ROOT="$root" PANEFLEET_AGENT_BRIDGE_BIN="$bridge_out" "$install_script" 2>&1)"; then
    printf '%s\n' "$output" >&2
    exit 1
  fi

  case "$output" in
  Bridge\ already\ installed*)
    PANEFLEET_BRIDGE_INSTALL_RESULT="already-installed"
    ;;
  Installed\ prebuilt\ bridge*)
    PANEFLEET_BRIDGE_INSTALL_RESULT="downloaded-release"
    ;;
  Built\ *)
    PANEFLEET_BRIDGE_INSTALL_RESULT="built-local"
    ;;
  *)
    PANEFLEET_BRIDGE_INSTALL_RESULT="installed"
    ;;
  esac
}

install_opencode_plugin() {
  local plugin_dir plugin_path bridge_path template_path escaped_bridge sed_bridge

  plugin_dir="$(opencode_plugin_dir)"
  plugin_path="$(opencode_plugin_path)"
  bridge_path="$(opencode_event_bridge_path)"
  template_path="$(opencode_plugin_template_path)"
  escaped_bridge="$(double_quote_literal_escape "$bridge_path")"
  sed_bridge="${escaped_bridge//&/\\&}"

  mkdir -p "$plugin_dir"
  if [[ ! -f "$template_path" ]]; then
    printf 'panefleet: missing opencode plugin template %s\n' "$template_path" >&2
    exit 1
  fi
  if [[ -L "$plugin_path" ]]; then
    rm -f "$plugin_path"
  fi
  sed "s|__PANEFLEET_BRIDGE_PATH__|$sed_bridge|g" "$template_path" >"$plugin_path"
}

install_codex_integration() {
  local config_path wrapper_path escaped_wrapper tmp_path

  config_path="$(codex_config_path)"
  wrapper_path="$(codex_notify_wrapper_path)"
  escaped_wrapper="$(double_quote_literal_escape "$wrapper_path")"
  mkdir -p "$(dirname "$config_path")"
  if [[ ! -f "$config_path" ]]; then
    : >"$config_path"
  fi

  tmp_path="$(mktemp "${TMPDIR:-/tmp}/panefleet-codex-config.XXXXXX")"

  if "${RG_BIN}" -Fq -- '# >>> panefleet codex notify >>>' "$config_path"; then
    awk -v wrapper="$escaped_wrapper" '
      BEGIN {
        start = "# >>> panefleet codex notify >>>"
        finish = "# <<< panefleet codex notify <<<"
        in_block = 0
      }
      $0 == start {
        print start
        print "notify = [\"" wrapper "\"]"
        print finish
        in_block = 1
        next
      }
      $0 == finish && in_block == 1 {
        in_block = 0
        next
      }
      in_block == 0 {
        print
      }
    ' "$config_path" >"$tmp_path"
  elif "${RG_BIN}" -q -e '^[[:space:]]*notify[[:space:]]*=' "$config_path"; then
    awk -v wrapper="$escaped_wrapper" '
      BEGIN {
        replaced = 0
      }
      {
        if (replaced == 0 && $0 ~ /^[[:space:]]*notify[[:space:]]*=/) {
          print "notify = [\"" wrapper "\"] # managed by panefleet"
          replaced = 1
          next
        }
        print
      }
      END {
        if (replaced == 0) {
          print ""
          print "# >>> panefleet codex notify >>>"
          print "notify = [\"" wrapper "\"]"
          print "# <<< panefleet codex notify <<<"
        }
      }
    ' "$config_path" >"$tmp_path"
  else
    cat "$config_path" >"$tmp_path"
    {
      if [[ -s "$config_path" ]]; then
        printf '\n'
      fi
      printf '# >>> panefleet codex notify >>>\n'
      printf 'notify = ["%s"]\n' "$escaped_wrapper"
      printf '# <<< panefleet codex notify <<<\n'
    } >>"$tmp_path"
  fi

  mv "$tmp_path" "$config_path"
  chmod 600 "$config_path" 2>/dev/null || true
}

install_claude_integration() {
  local settings_path wrapper_path

  settings_path="$(claude_settings_path)"
  wrapper_path="$(claude_hook_wrapper_path)"
  mkdir -p "$(dirname "$settings_path")"

  perl - "$settings_path" "$wrapper_path" <<'PERL'
use strict;
use warnings;
use JSON::PP;

my ($path, $command) = @ARGV;
my $raw = q{};

if (-e $path) {
  local $/ = undef;
  open my $fh, '<', $path or die "failed to read $path: $!";
  $raw = <$fh> // q{};
  close $fh;
}

my $doc = {};
if ($raw =~ /\S/) {
  eval { $doc = decode_json($raw); 1 } or die "invalid JSON in $path\n";
}
$doc = {} if ref($doc) ne 'HASH';

my $hooks = $doc->{hooks};
$hooks = {} if ref($hooks) ne 'HASH';
$doc->{hooks} = $hooks;

sub ensure_event {
  my ($hooks_ref, $event_name, $command_path, $with_matcher) = @_;
  my $entries = $hooks_ref->{$event_name};
  $entries = [] if ref($entries) ne 'ARRAY';

  my @clean_entries = ();
  ENTRY: for my $entry (@{$entries}) {
    next ENTRY if ref($entry) ne 'HASH';
    my $hook_items = $entry->{hooks};
    next ENTRY if ref($hook_items) ne 'ARRAY';

    my @kept_hooks = ();
    for my $hook (@{$hook_items}) {
      if (ref($hook) ne 'HASH') {
        push @kept_hooks, $hook;
        next;
      }

      my $hook_type = $hook->{type} // q{};
      my $hook_cmd  = $hook->{command} // q{};
      if ($hook_type eq 'command' && $hook_cmd ne q{} && $hook_cmd =~ m{claude-code-hook$} && $hook_cmd =~ /panefleet/) {
        next;
      }

      push @kept_hooks, $hook;
    }

    next ENTRY if @kept_hooks == 0;
    my %clean_entry = %{$entry};
    $clean_entry{hooks} = \@kept_hooks;
    push @clean_entries, \%clean_entry;
  }

  my %new_entry = (
    hooks => [ { type => 'command', command => $command_path } ],
  );
  if ($with_matcher) {
    $new_entry{matcher} = q{};
  }
  push @clean_entries, \%new_entry;
  $hooks_ref->{$event_name} = \@clean_entries;
}

ensure_event($hooks, 'UserPromptSubmit', $command, 0);
ensure_event($hooks, 'PreToolUse',      $command, 1);
ensure_event($hooks, 'PermissionRequest', $command, 1);
ensure_event($hooks, 'Stop',            $command, 0);

my $json = JSON::PP->new->ascii->pretty->canonical->encode($doc);
open my $out, '>', $path or die "failed to write $path: $!";
print {$out} $json;
close $out;
PERL
  chmod 600 "$settings_path" 2>/dev/null || true
}

codex_notify_is_wired() {
  local config_path wrapper_path

  config_path="$(codex_config_path)"
  wrapper_path="$(codex_notify_wrapper_path)"
  if [[ ! -f "$config_path" ]]; then
    return 1
  fi

  "${RG_BIN}" -Fq -- "$wrapper_path" "$config_path"
}

claude_hook_is_wired() {
  local settings_path wrapper_path

  settings_path="$(claude_settings_path)"
  wrapper_path="$(claude_hook_wrapper_path)"
  if [[ ! -f "$settings_path" ]]; then
    return 1
  fi

  "${RG_BIN}" -Fq -- "$wrapper_path" "$settings_path"
}

append_unique_target() {
  local value="$1"
  local existing

  for existing in "${PANEFLEET_INSTALL_TARGETS[@]:-}"; do
    if [[ "$existing" == "$value" ]]; then
      return
    fi
  done

  PANEFLEET_INSTALL_TARGETS+=("$value")
}

resolve_integration_targets() {
  local arg

  PANEFLEET_INSTALL_TARGETS=()
  if (($# == 0)); then
    printf 'missing install target\n' >&2
    printf 'usage: %s install codex|claude|opencode|all\n' "$SELF" >&2
    exit 1
  fi

  for arg in "$@"; do
    case "$arg" in
    all)
      append_unique_target codex
      append_unique_target claude
      append_unique_target opencode
      ;;
    codex | claude | opencode)
      append_unique_target "$arg"
      ;;
    *)
      printf 'unknown install integration target: %s\n' "$arg" >&2
      printf 'expected one of: codex, claude, opencode, all\n' >&2
      exit 1
      ;;
    esac
  done
}

is_provider_target() {
  case "${1:-}" in
  core | codex | claude | opencode | all) return 0 ;;
  *) return 1 ;;
  esac
}

print_core_load_hint() {
  printf 'Load core in tmux with: tmux source-file "%s/panefleet.tmux"\n' "$(self_root)"
}

print_core_installed_message() {
  if [[ -n "${TMUX:-}" ]]; then
    printf 'Core installed\n'
    printf 'Bindings: prefix+P, prefix+T\n'
  else
    print_core_load_hint
  fi
}

join_by_comma_space() {
  local joined=""
  local item

  for item in "$@"; do
    if [[ -n "$joined" ]]; then
      joined+=", "
    fi
    joined+="$item"
  done

  printf '%s' "$joined"
}

integration_status_codex() {
  if ! bridge_present; then
    printf 'bridge-missing'
  elif ! codex_notify_is_wired; then
    printf 'config-missing'
  else
    printf 'ready'
  fi
}

integration_status_claude() {
  if ! bridge_present; then
    printf 'bridge-missing'
  elif ! claude_hook_is_wired; then
    printf 'config-missing'
  else
    printf 'ready'
  fi
}

integration_status_opencode() {
  if ! bridge_present; then
    printf 'bridge-missing'
  elif [[ ! -f "$(opencode_plugin_path)" ]]; then
    printf 'plugin-missing'
  elif ! command_exists bun; then
    printf 'plugin-ready bun-missing'
  else
    printf 'ready'
  fi
}
