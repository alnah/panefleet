#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PANEFLEET_BIN="${REPO_ROOT}/bin/panefleet"
FAKE_TMUX_BIN="${REPO_ROOT}/tests/fake-tmux"
FAKE_FZF_BIN="${REPO_ROOT}/tests/fake-fzf"
TEST_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/panefleet-tests.XXXXXX")"
trap 'rm -rf "${TEST_TMPDIR}"' EXIT

export FZF_BIN="${FAKE_FZF_BIN}"

pass() {
  printf 'ok - %s\n' "$1"
}

fail() {
  printf 'not ok - %s\n' "$1" >&2
  exit 1
}

assert_eq() {
  local got="$1"
  local want="$2"
  local msg="$3"

  if [[ "$got" != "$want" ]]; then
    fail "${msg}: got '${got}', want '${want}'"
  fi
}

strip_ansi() {
  perl -pe 's/\e\[[0-9;]*m//g'
}

file_mode_octal() {
  local path="$1"

  if stat -c '%a' "$path" >/dev/null 2>&1; then
    stat -c '%a' "$path"
    return
  fi
  if stat -f '%Lp' "$path" >/dev/null 2>&1; then
    stat -f '%Lp' "$path"
    return
  fi

  fail "unable to read mode for $path"
}

setup_fake_tmux_fixture() {
  local root="$1"
  local now

  now="$(date +%s)"
  mkdir -p "$root/globals"
  mkdir -p "$root/panes/%101/options" "$root/panes/%102/options" "$root/panes/%103/options" "$root/panes/%104/options" "$root/panes/%105/options"

  printf '10' >"$root/globals/@panefleet-done-recent-minutes"
  printf '45' >"$root/globals/@panefleet-stale-minutes"
  printf '600' >"$root/globals/@panefleet-agent-status-max-age-seconds"
  printf 'dracula' >"$root/globals/@panefleet-theme"

  cat >"$root/panes/%101/meta" <<EOF
pane_id='%101'
session_name='workspace'
window_index='1'
window_name='codex'
pane_index='0'
pane_current_command='codex-aarch64-a'
pane_title='cdx'
pane_current_path='/tmp/workspace'
pane_dead='0'
pane_dead_status=''
window_activity='${now}'
history_size='10'
cursor_x='0'
cursor_y='0'
client_termfeatures='rgb'
EOF
  cat >"$root/panes/%101/capture" <<'EOF'
Enter to confirm
Esc to cancel
EOF

  cat >"$root/panes/%102/meta" <<EOF
pane_id='%102'
session_name='workspace'
window_index='2'
window_name='opencode'
pane_index='0'
pane_current_command='opencode'
pane_title='OC | Greeting'
pane_current_path='/tmp/workspace'
pane_dead='0'
pane_dead_status=''
window_activity='${now}'
history_size='10'
cursor_x='0'
cursor_y='0'
client_termfeatures='rgb'
EOF
  cat >"$root/panes/%102/capture" <<'EOF'
Ask anything...
ctrl+p commands
tab agents
EOF

  cat >"$root/panes/%103/meta" <<EOF
pane_id='%103'
session_name='workspace'
window_index='3'
window_name='shell'
pane_index='0'
pane_current_command='zsh'
pane_title='zsh'
pane_current_path='/tmp/workspace'
pane_dead='0'
pane_dead_status=''
window_activity='${now}'
history_size='10'
cursor_x='0'
cursor_y='0'
client_termfeatures='rgb'
EOF
  cat >"$root/panes/%103/capture" <<'EOF'
prompt
EOF

  cat >"$root/panes/%104/meta" <<EOF
pane_id='%104'
session_name='workspace'
window_index='4'
window_name='opencode'
pane_index='0'
pane_current_command='opencode'
pane_title='OC | Active'
pane_current_path='/tmp/workspace'
pane_dead='0'
pane_dead_status=''
window_activity='${now}'
history_size='1'
cursor_x='5'
cursor_y='35'
client_termfeatures='rgb'
EOF
  cat >"$root/panes/%104/capture" <<'EOF'
┃  Thinking: Setting up a script path
┃
┃  # Creates tmp directory in workspace
┃
┃  $ mkdir -p "/tmp/workspace"

   ~ Preparing patch...
   ▣  Build · gpt-5.4 · interrupted

┃  Build  model-x provider-y · high
╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
                                                OpenCode active
EOF
  printf 'stable:1:5:35:opencode:OC | Active:0:' >"$root/panes/%104/options/@panefleet_auto_signature"
  printf 'DONE' >"$root/panes/%104/options/@panefleet_auto_raw_status"
  printf 'opencode' >"$root/panes/%104/options/@panefleet_auto_tool"

  cat >"$root/panes/%105/meta" <<EOF
pane_id='%105'
session_name='workspace'
window_index='5'
window_name='claude code'
pane_index='0'
pane_current_command='2.1.76'
pane_title='✳ Claude Code'
pane_current_path='/tmp/workspace'
pane_dead='0'
pane_dead_status=''
window_activity='${now}'
history_size='50'
cursor_x='0'
cursor_y='39'
client_termfeatures='rgb'
EOF
  cat >"$root/panes/%105/capture" <<'EOF'
Some transcript above
────────────────────────────────────────────────────────────────────────────────
❯ 
────────────────────────────────────────────────────────────────────────────────
  ➜  workspace                                                    31413 tokens
                                                      current: 2.1.76 · latest: 2.1.76
EOF
}

run_list() {
  TMUX=1 \
    TMUX_BIN="${FAKE_TMUX_BIN}" \
    FZF_BIN="${FZF_BIN:-fzf}" \
    RG_BIN="${RG_BIN:-rg}" \
    PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" \
    "${PANEFLEET_BIN}" list | strip_ansi
}

reset_fake_tmux_fixture() {
  rm -rf "${TEST_TMPDIR}/fake-tmux"
  setup_fake_tmux_fixture "${TEST_TMPDIR}/fake-tmux"
}

run_install_target_in_fake_tmux() {
  local target="$1"
  local bridge_bin="$2"
  local plugin_dir="$3"
  local codex_config="$4"
  local claude_settings="$5"
  local bridge_mode="${6:-build}"

  TMUX=1 \
    TMUX_BIN="${FAKE_TMUX_BIN}" \
    FZF_BIN="${FAKE_FZF_BIN}" \
    PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" \
    PANEFLEET_AGENT_BRIDGE_BIN="$bridge_bin" \
    PANEFLEET_OPENCODE_PLUGIN_DIR="$plugin_dir" \
    PANEFLEET_CODEX_CONFIG="$codex_config" \
    PANEFLEET_CLAUDE_SETTINGS="$claude_settings" \
    PANEFLEET_BRIDGE_INSTALL_MODE="$bridge_mode" \
    "${PANEFLEET_BIN}" install "$target"
}

run_doctor_install_in_fake_tmux() {
  local bridge_bin="$1"
  local plugin_dir="$2"
  local codex_config="$3"
  local claude_settings="$4"

  TMUX=1 \
    TMUX_BIN="${FAKE_TMUX_BIN}" \
    FZF_BIN="${FAKE_FZF_BIN}" \
    PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" \
    PANEFLEET_AGENT_BRIDGE_BIN="$bridge_bin" \
    PANEFLEET_OPENCODE_PLUGIN_DIR="$plugin_dir" \
    PANEFLEET_CODEX_CONFIG="$codex_config" \
    PANEFLEET_CLAUDE_SETTINGS="$claude_settings" \
    "${PANEFLEET_BIN}" doctor --install
}

run_board_with_stub() {
  local log_file="$1"
  local backend="${2:-go}"
  local command="${3:-board}"
  local stub_bin="${TEST_TMPDIR}/fake-go-board.sh"

  cat >"$stub_bin" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "\$*" >>"${log_file}"
EOF
  chmod +x "$stub_bin"

  TMUX=1 \
    TMUX_BIN="${FAKE_TMUX_BIN}" \
    FZF_BIN="${FZF_BIN:-fzf}" \
    PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" \
    PANEFLEET_BOARD_BACKEND="${backend}" \
    PANEFLEET_GO_BOARD_BIN="${stub_bin}" \
    "${PANEFLEET_BIN}" "${command}"
}

run_popup_with_stub() {
  local log_file="$1"
  local backend="${2:-go}"
  local command="${3:-popup}"
  local stub_bin="${TEST_TMPDIR}/fake-go-popup.sh"

  cat >"$stub_bin" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "\$*" >>"${log_file}"
EOF
  chmod +x "$stub_bin"

  TMUX=1 \
    TMUX_BIN="${FAKE_TMUX_BIN}" \
    FZF_BIN="${FZF_BIN:-fzf}" \
    PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" \
    PANEFLEET_BOARD_BACKEND="${backend}" \
    PANEFLEET_GO_BOARD_BIN="${stub_bin}" \
    "${PANEFLEET_BIN}" "${command}"
}

assert_opencode_readyish() {
  local doctor_output="$1"
  local msg="$2"

  if [[ "$doctor_output" == *"integration.opencode ready"* ]]; then
    return
  fi
  if [[ "$doctor_output" == *"integration.opencode plugin-ready bun-missing"* ]]; then
    return
  fi

  fail "$msg"
}

test_sourced_helpers() {
  local got
  local original_codex_process_is_working
  local board_file

  # shellcheck disable=SC1090
  PANEFLEET_SOURCE_ONLY=1 source "${PANEFLEET_BIN}"

  got="$(XDG_STATE_HOME="${TEST_TMPDIR}/state" PANEFLEET_AGENT_BRIDGE_BIN='' bridge_bin_path)"
  assert_eq "$got" "${TEST_TMPDIR}/state/panefleet/bin/panefleet-agent-bridge" "bridge_bin_path should default to user state home"
  pass "bridge_bin_path defaults to user state home"

  board_file="${REPO_ROOT}/lib/panefleet/ui/board.sh"
  rg -Fq -- '--bind "up:up+execute-silent(${SELF} queue-refresh --pane {1})+' "$board_file" || fail "board up binding should queue refresh asynchronously"
  rg -Fq -- '--bind "down:down+execute-silent(${SELF} queue-refresh --pane {1})+' "$board_file" || fail "board down binding should queue refresh asynchronously"
  ! rg -Fq -- '--bind "up:up+execute-silent(${SELF} refresh-panes-cache {1})+reload(' "$board_file" || fail "board up binding should not run synchronous reloads"
  ! rg -Fq -- '--bind "down:down+execute-silent(${SELF} refresh-panes-cache {1})+reload(' "$board_file" || fail "board down binding should not run synchronous reloads"
  pass "board navigation stays decoupled from synchronous refresh"

  got="$(PANEFLEET_BOARD_COLUMNS=160 board_viewport_columns)"
  assert_eq "$got" "160" "board_viewport_columns should honor explicit override"
  pass "board_viewport_columns honors explicit override"

  PANEFLEET_BOARD_COLUMNS=160 board_layout_widths
  assert_eq "$PANEFLEET_BOARD_SESSION_WIDTH" "28" "board_layout_widths should expand session column"
  assert_eq "$PANEFLEET_BOARD_WINDOW_WIDTH" "38" "board_layout_widths should expand window column"
  assert_eq "$PANEFLEET_BOARD_REPO_WIDTH" "30" "board_layout_widths should expand repo column"
  pass "board layout widths scale to the viewport"

  got="$(board_padding_spec)"
  assert_eq "$got" "0,1,0,1" "board_padding_spec should keep the board nearly edge-to-edge"
  pass "board padding stays minimal"

  got="$(board_popup_width)"
  assert_eq "$got" "100%" "board popup width should default to full client width"
  got="$(board_popup_height)"
  assert_eq "$got" "100%" "board popup height should default to full client height"
  pass "board popup defaults to full client size"

  board_runner_log="${TEST_TMPDIR}/go-board.log"
  : >"$board_runner_log"
  run_board_with_stub "$board_runner_log"
  [[ "$(cat "$board_runner_log")" == "tui --refresh 1s" ]] || fail "board should default to the Go-backed board runtime"
  pass "board command defaults to the Go board runtime"

  popup_runner_log="${TEST_TMPDIR}/go-popup.log"
  : >"$popup_runner_log"
  run_popup_with_stub "$popup_runner_log"
  fake_tmux_log="${TEST_TMPDIR}/fake-tmux/tmux.log"
  rg -Fq -- "display-popup" "$fake_tmux_log" || fail "popup should still use tmux display-popup"
  rg -Fq -- "tui --refresh 1s" "$fake_tmux_log" || fail "popup should launch the Go-backed board runtime inside tmux popup"
  pass "popup command defaults to the Go board runtime"

  : >"$board_runner_log"
  run_board_with_stub "$board_runner_log" shell board-go
  [[ "$(cat "$board_runner_log")" == "tui --refresh 1s" ]] || fail "board-go should run the Go-backed board runtime"
  pass "board-go runs the Go-backed board runtime"

  : >"$fake_tmux_log"
  run_popup_with_stub "$popup_runner_log" shell popup-go
  rg -Fq -- "display-popup" "$fake_tmux_log" || fail "popup-go should use tmux display-popup"
  rg -Fq -- "tui --refresh 1s" "$fake_tmux_log" || fail "popup-go should launch the Go-backed board runtime inside tmux popup"
  pass "popup-go runs the Go-backed board runtime"

  got="$(FZF_BIN="$(command -v fzf)" fzf_supports_reload_sync && printf yes || printf no)"
  assert_eq "$got" "yes" "fzf_supports_reload_sync should probe runtime bind support"
  got="$(FZF_BIN="$(command -v fzf)" fzf_supports_result_event && printf yes || printf no)"
  assert_eq "$got" "yes" "fzf_supports_result_event should probe runtime bind support"
  got="$(FZF_BIN="$(command -v fzf)" fzf_supports_listen && printf yes || printf no)"
  assert_eq "$got" "yes" "fzf_supports_listen should detect listen support"
  pass "fzf capability probes detect live bind support"

  got="$(board_ticker_interval_seconds)"
  assert_eq "$got" "1" "board_ticker_interval_seconds should default to one second"
  got="$(board_reload_action_payload demo)"
  [[ "$got" == *"reload("*"rows-demo.tsv"* ]] || fail "board_reload_action_payload should reload the per-board cache file"
  pass "board ticker defaults to one-second async cache reloads"

  if agent_status_is_fresh "$(date +%s)" 600 "$(date +%s)"; then
    pass "agent_status_is_fresh accepts current timestamp"
  else
    fail "agent_status_is_fresh rejects current timestamp"
  fi

  got="$(effective_status_values DONE "$(date +%s)" "$(date +%s)" "" 10 45 "$(date +%s)")"
  assert_eq "$got" "DONE" "effective_status_values keeps recent DONE"
  pass "effective_status_values keeps recent done"

  got="$(effective_status_values DONE "$(date +%s)" "$(($(date +%s) - 3600))" "" 10 45 "$(date +%s)")"
  assert_eq "$got" "DONE" "effective_status_values refreshes done when recent activity is newer"
  pass "effective_status_values refreshes done from newer activity"

  resolve_uncached_state_values "%102" "opencode" "opencode" "OC | Greeting" 0 "" "$(date +%s)" $'Ask anything...\nctrl+p commands\ntab agents' "" "" "" "" "" "" 10 45 600 "$(date +%s)"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "DONE" "resolve_uncached_state_values infers opencode done"
  pass "resolve_uncached_state_values infers opencode done"

  got="$(adapter_status "%104" "opencode" "opencode" 0 "" $'Old transcript above\nThinking: earlier step\nfiller\nfiller\nfiller\nfiller\nBuild model-x provider-y · high\n╹▀▀▀▀▀▀\nctrl+t variants\ntab agents\nctrl+p commands\nOpenCode 1.2.26')"
  assert_eq "$got" "DONE" "adapter_status keeps opencode done when only ready footer remains in focus"
  pass "adapter_status keeps opencode done when only ready footer remains in focus"

  got="$(adapter_status "%101" "codex" "codex-aarch64-a" 0 "" $'/models\nSelect model\nEnter to confirm · Esc to cancel')"
  assert_eq "$got" "WAIT" "adapter_status infers codex wait from chooser text"
  pass "adapter_status infers codex wait from chooser text"

  original_codex_process_is_working="$(declare -f codex_process_is_working)"
  # shellcheck disable=SC2317,SC2329
  codex_process_is_working() { return 0; }
  got="$(adapter_status "%101" "codex" "codex-aarch64-a" 0 "" $'No chooser\nNo prompt yet')"
  assert_eq "$got" "RUN" "adapter_status infers codex run from process tree"
  pass "adapter_status infers codex run from process tree"
  eval "$original_codex_process_is_working"

  got="$(effective_status_values WAIT "$(date +%s)" "" "" 10 45 "$(date +%s)")"
  assert_eq "$got" "WAIT" "effective_status_values keeps wait visible"
  pass "effective_status_values keeps wait visible"

  got="$(adapter_status "%105" "claude" "2.1.76" 0 "" $'Some transcript\n❯\u00a0\ncurrent: 2.1.76 · latest: 2.1.76')"
  assert_eq "$got" "DONE" "adapter_status infers claude done from prompt"
  pass "adapter_status infers claude done from prompt"

  got="$(adapter_status "%105" "claude" "2.1.76" 0 "" $'❯ sleep 20s\n\n⏺ Bash(sleep 20)\n  ⎿  (No output)')"
  assert_eq "$got" "RUN" "adapter_status does not confuse a typed claude command with a done prompt"
  pass "adapter_status does not confuse a typed claude command with a done prompt"

  got="$(adapter_status "%105" "claude" "2.1.76" 0 "" $'Some transcript without prompt or tool activity')"
  assert_eq "$got" "IDLE" "adapter_status keeps claude idle when no clear signal exists"
  pass "adapter_status keeps claude idle when no clear signal exists"

  got="$(adapter_status "%105" "claude" "2.1.76" 0 "" $'Older transcript above\n\n⏺ Je vais créer le fichier maintenant.')"
  assert_eq "$got" "RUN" "adapter_status infers claude run from active assistant marker in the focus region"
  pass "adapter_status infers claude run from active assistant marker in the focus region"

  # shellcheck disable=SC2034
  PANEFLEET_ADAPTERS_ENABLED=1
  resolve_uncached_state_values "%105" "claude" "2.1.76" "✳ Claude Code" 0 "" "$(date +%s)" $'Some transcript above\n❯\n' "" "RUN" "claude" "$(date +%s)" "" "" 10 45 600 "$(date +%s)" "claude-hook"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "RUN" "resolve_uncached_state_values keeps a fresh claude adapter run"
  assert_eq "$PANEFLEET_RESOLVED_SOURCE" "agent" "resolve_uncached_state_values reports agent source for fresh claude adapter run"
  pass "resolve_uncached_state_values keeps fresh claude adapter run"

  resolve_uncached_state_values "%105" "claude" "2.1.76" "✳ Claude Code" 0 "" "$(date +%s)" $'Permissions: Allow Ask Deny\n❯ 1. Add a new rule…\nPress ↑↓ to navigate · Enter to select · Type to search · Esc to cancel' "" "RUN" "claude" "$(date +%s)" "" "" 10 45 600 "$(date +%s)" "claude-hook"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "WAIT" "resolve_uncached_state_values lets claude chooser override adapter run"
  assert_eq "$PANEFLEET_RESOLVED_SOURCE" "heuristic-live" "resolve_uncached_state_values reports heuristic source for claude chooser override"
  pass "resolve_uncached_state_values lets claude chooser override adapter run"

  resolve_uncached_state_values "%105" "claude" "2.1.76" "✳ Claude Code" 0 "" "$(date +%s)" $'Permissions: Allow Ask Deny\n❯ 1. Add a new rule…\nPress ↑↓ to navigate · Enter to select · Type to search · Esc to cancel' "" "DONE" "claude" "$(date +%s)" "" "" 10 45 600 "$(date +%s)" "claude-hook"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "WAIT" "resolve_uncached_state_values lets claude chooser override adapter done"
  pass "resolve_uncached_state_values lets claude chooser override adapter done"

  resolve_uncached_state_values "%105" "claude" "2.1.76" "✳ Claude Code" 0 "" "$(date +%s)" $'  - permissionDecision: fallback strategy notes\n  - activeFlags: []any → []string\n\n❯' "" "DONE" "claude" "$(date +%s)" "" "" 10 45 600 "$(date +%s)" "claude-hook"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "DONE" "resolve_uncached_state_values keeps claude done when generic permission text appears in prose"
  assert_eq "$PANEFLEET_RESOLVED_SOURCE" "agent" "resolve_uncached_state_values keeps adapter source when claude wait heuristic does not match chooser prompt"
  pass "resolve_uncached_state_values ignores claude wait false positive from prose text"

  resolve_uncached_state_values "%101" "codex" "codex-aarch64-a" "cdx" 0 "" "$(date +%s)" $'Working (2m)\nesc to interrupt' "" "DONE" "codex" "$(date +%s)" "" "" 10 45 600 "$(date +%s)" "codex-notify"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "RUN" "resolve_uncached_state_values lets codex live run override adapter done"
  assert_eq "$PANEFLEET_RESOLVED_SOURCE" "heuristic-live" "resolve_uncached_state_values reports heuristic source for codex live run override"
  pass "resolve_uncached_state_values lets codex live run override adapter done"

  resolve_uncached_state_values "%101" "codex" "codex-aarch64-a" "cdx" 0 "" "$(date +%s)" $'/permissions\nSelect permission\nEnter to confirm · Esc to cancel' "" "DONE" "codex" "$(date +%s)" "" "" 10 45 600 "$(date +%s)" "codex-notify"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "WAIT" "resolve_uncached_state_values lets codex live wait override adapter done"
  pass "resolve_uncached_state_values lets codex live wait override adapter done"

  resolve_uncached_state_values "%102" "opencode" "opencode" "OC | Greeting" 0 "" "$(date +%s)" $'filler\nfiller\nAsk anything...\nctrl+p commands\ntab agents' "" "RUN" "opencode" "$(date +%s)" "" "" 10 45 600 "$(date +%s)" "opencode-plugin"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "DONE" "resolve_uncached_state_values lets opencode live done override adapter run"
  assert_eq "$PANEFLEET_RESOLVED_SOURCE" "heuristic-live" "resolve_uncached_state_values reports heuristic source for opencode run override"
  pass "resolve_uncached_state_values lets opencode live done override adapter run"

  resolve_uncached_state_values "%102" "opencode" "opencode" "OC | Greeting" 0 "" "$(date +%s)" $'filler\nfiller\nAsk anything...\nctrl+p commands\ntab agents' "" "WAIT" "opencode" "$(date +%s)" "" "" 10 45 600 "$(date +%s)" "opencode-plugin"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "DONE" "resolve_uncached_state_values clears opencode wait when no visible wait prompt exists"
  assert_eq "$PANEFLEET_RESOLVED_SOURCE" "heuristic-live" "resolve_uncached_state_values reports heuristic source when opencode wait is cleared"
  pass "resolve_uncached_state_values clears stale opencode wait without visible chooser prompt"

  resolve_uncached_state_values "%103" "shell" "zsh" "$HOME/workspace" 0 "" "$(date +%s)" $'prompt' "" "RUN" "claude" "$(date +%s)" "" "" 10 45 600 "$(date +%s)" "claude-hook"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "IDLE" "resolve_uncached_state_values ignores fresh claude adapter state on shell pane"
  pass "resolve_uncached_state_values ignores stale claude adapter state on shell pane"
  # shellcheck disable=SC2034
  PANEFLEET_ADAPTERS_ENABLED=0

  got="$(adapter_status "%104" "opencode" "opencode" 0 "" $'┃  Thinking: Setting up a script path\n   ~ Preparing patch...\n   ▣  Build · model-x · interrupted\nctrl+t variants')"
  assert_eq "$got" "RUN" "adapter_status infers opencode run from active transcript"
  pass "adapter_status infers opencode run from active transcript"

}

test_fake_tmux_cli() {
  local output line101 line102 line103 line104 inspect_output doctor_output install_doctor_output stale_touch manual_103_path manual_102_path

  output="$(run_list)"
  line101="$(printf '%s\n' "$output" | rg '^%101')"
  line102="$(printf '%s\n' "$output" | rg '^%102')"
  line103="$(printf '%s\n' "$output" | rg '^%103')"
  line104="$(printf '%s\n' "$output" | rg '^%104')"

  [[ "$line101" == *"WAIT"* ]] || fail "codex pane should be WAIT"
  [[ "$line102" == *"DONE"* ]] || fail "opencode pane should be DONE"
  [[ "$line103" == *"IDLE"* ]] || fail "shell pane should be IDLE"
  [[ "$line104" == *"RUN"* ]] || fail "recent active opencode pane should bypass stale DONE cache"
  [[ "$line101" == *"workspace"* ]] || fail "list should expose the session column"
  [[ "$line101" == *"codex"* ]] || fail "list should expose the window column"
  [[ "$line101" != *"cdx resume panefleet"* ]] || fail "list should no longer expose the task column"
  pass "fake tmux list shows expected baseline statuses"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" metrics-set --pane %101 --tokens-used 12345 --context-left-pct 88 --context-window 128000 >/dev/null
  output="$(run_list)"
  line101="$(printf '%s\n' "$output" | rg '^%101')"
  [[ "$line101" == *"12345"* ]] || fail "metrics-set should expose TOKENS in list output"
  [[ "$line101" == *"88%"* ]] || fail "metrics-set should expose CTX% in list output"
  pass "metrics-set exposes tokens and context columns"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" metrics-set --pane %101 --tokens-used 13000 --clear-context-left-pct --clear-context-window >/dev/null
  output="$(run_list)"
  line101="$(printf '%s\n' "$output" | rg '^%101')"
  [[ "$line101" == *"13000"* ]] || fail "metrics-set should keep TOKENS when context fields are cleared"
  [[ "$line101" != *"88%"* ]] || fail "metrics-set clear flags should remove stale CTX%"
  pass "metrics-set clear flags remove stale context values"

  printf 'auto' >"${TEST_TMPDIR}/fake-tmux/globals/@panefleet-adapter-mode"
  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-set --pane %103 --status ERROR --tool shell --source test --updated-at "$(date +%s)" >/dev/null
  output="$(run_list)"
  line103="$(printf '%s\n' "$output" | rg '^%103')"
  [[ "$line103" == *"ERROR"* ]] || fail "fresh adapter state should override heuristic"
  pass "fresh adapter state overrides heuristic"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-clear --pane %103 >/dev/null
  output="$(run_list)"
  line103="$(printf '%s\n' "$output" | rg '^%103')"
  [[ "$line103" == *"IDLE"* ]] || fail "state-clear should restore heuristic state"
  pass "state-clear restores heuristic state"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-stale --pane %103 >/dev/null
  output="$(run_list)"
  line103="$(printf '%s\n' "$output" | rg '^%103')"
  [[ "$line103" == *"STALE"* ]] || fail "state-stale should force a manual stale override"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-stale --pane %103 >/dev/null
  output="$(run_list)"
  line103="$(printf '%s\n' "$output" | rg '^%103')"
  [[ "$line103" == *"IDLE"* ]] || fail "state-stale should toggle off the manual stale override"
  pass "state-stale toggles manual stale override"

  manual_103_path="${TEST_TMPDIR}/fake-tmux/panes/%103/options/@panefleet_status"
  manual_102_path="${TEST_TMPDIR}/fake-tmux/panes/%102/options/@panefleet_status"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-stale --pane %103 >/dev/null
  output="$(run_list)"
  line103="$(printf '%s\n' "$output" | rg '^%103')"
  [[ "$line103" == *"STALE"* ]] || fail "manual stale should apply on shell pane"
  [[ -f "$manual_103_path" ]] || fail "manual stale should persist pane override before activity resumes"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-set --pane %103 --status RUN --tool shell --source test --updated-at "$(date +%s)" >/dev/null
  output="$(run_list)"
  line103="$(printf '%s\n' "$output" | rg '^%103')"
  [[ "$line103" == *"RUN"* ]] || fail "manual stale should be overridden by RUN"
  [[ ! -f "$manual_103_path" ]] || fail "manual stale should clear once RUN overrides it"
  pass "manual stale yields to run"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-stale --pane %102 >/dev/null
  output="$(run_list)"
  line102="$(printf '%s\n' "$output" | rg '^%102')"
  [[ "$line102" == *"STALE"* ]] || fail "manual stale should apply on opencode pane"
  [[ -f "$manual_102_path" ]] || fail "manual stale should persist pane override before wait resumes"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-set --pane %102 --status WAIT --tool opencode --source test --updated-at "$(date +%s)" >/dev/null
  output="$(run_list)"
  line102="$(printf '%s\n' "$output" | rg '^%102')"
  [[ "$line102" == *"WAIT"* ]] || fail "manual stale should be overridden by WAIT"
  [[ ! -f "$manual_102_path" ]] || fail "manual stale should clear once WAIT overrides it"
  pass "manual stale yields to wait"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-clear --pane %103 >/dev/null
  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-clear --pane %102 >/dev/null

  printf '1' >"${TEST_TMPDIR}/fake-tmux/globals/@panefleet-stale-minutes"
  stale_touch="$(($(date +%s) - 120))"
  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${FAKE_TMUX_BIN}" set-option -pt %103 -q @panefleet_last_touch "$stale_touch" >/dev/null
  output="$(run_list)"
  line103="$(printf '%s\n' "$output" | rg '^%103')"
  [[ "$line103" == *"STALE"* ]] || fail "auto stale should still work from stale timing"
  printf '45' >"${TEST_TMPDIR}/fake-tmux/globals/@panefleet-stale-minutes"
  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${FAKE_TMUX_BIN}" set-option -pt %103 -u @panefleet_last_touch >/dev/null
  pass "auto stale still works"

  inspect_output="$(TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" state-show --pane %101)"
  [[ "$inspect_output" == *"final.status   WAIT"* ]] || fail "state-show should expose final WAIT status"
  [[ "$inspect_output" == *"final.source   heuristic-"* ]] || fail "state-show should expose heuristic source"
  [[ "$inspect_output" == *"final.reason   "* ]] || fail "state-show should expose resolution reason"
  pass "state-show exposes source and reason"

  doctor_output="$(TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" doctor --verbose)"
  [[ "$doctor_output" == *"state counts"* ]] || fail "doctor should print state counts"
  [[ "$doctor_output" == *"state list"* ]] || fail "doctor --verbose should print state list"
  [[ "$doctor_output" == *"heuristic-"* ]] || fail "doctor --verbose should expose state source"
  pass "doctor --verbose exposes runtime diagnostics"

  install_doctor_output="$(TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" doctor --install)"
  [[ "$install_doctor_output" == *"self.root"* ]] || fail "doctor --install should print install root"
  [[ "$install_doctor_output" == *"bridge.present"* ]] || fail "doctor --install should print bridge presence"
  pass "doctor --install exposes install diagnostics"
}

test_wrapper_install_hints() {
  local fake_bin missing_bridge stderr_file fake_panefleet

  fake_bin="${TEST_TMPDIR}/fake-bin"
  missing_bridge="${TEST_TMPDIR}/missing/panefleet-agent-bridge"
  stderr_file="${TEST_TMPDIR}/wrapper.stderr"
  fake_panefleet="${fake_bin}/panefleet"
  mkdir -p "$fake_bin"
  cat >"$fake_panefleet" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "$fake_panefleet"

  PATH="$fake_bin:${PATH}" PANEFLEET_AGENT_BRIDGE_BIN="$missing_bridge" "${REPO_ROOT}/scripts/codex-notify-bridge" >/dev/null 2>"$stderr_file" || true
  [[ "$(cat "$stderr_file")" == *"install it with ${fake_panefleet} install codex"* ]] || fail "codex wrapper should suggest panefleet install codex"

  PATH="$fake_bin:${PATH}" PANEFLEET_AGENT_BRIDGE_BIN="$missing_bridge" "${REPO_ROOT}/scripts/claude-code-hook" >/dev/null 2>"$stderr_file" || true
  [[ "$(cat "$stderr_file")" == *"install it with ${fake_panefleet} install claude"* ]] || fail "claude wrapper should suggest panefleet install claude"

  PATH="$fake_bin:${PATH}" PANEFLEET_AGENT_BRIDGE_BIN="$missing_bridge" "${REPO_ROOT}/scripts/opencode-event-bridge" >/dev/null 2>"$stderr_file" || true
  [[ "$(cat "$stderr_file")" == *"install it with ${fake_panefleet} install opencode"* ]] || fail "opencode wrapper should suggest panefleet install opencode"
  pass "wrappers print consistent install hints"
}

test_codex_wrapper_forwards_pane_when_available() {
  local fake_bridge_root fake_bridge bridge_args expected_pane

  fake_bridge_root="${TEST_TMPDIR}/fake-bridge"
  fake_bridge="${fake_bridge_root}/panefleet-agent-bridge"
  bridge_args="${fake_bridge_root}/args.log"
  mkdir -p "$fake_bridge_root"
  cat >"$fake_bridge" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >"$bridge_args"
exit 0
EOF
  chmod +x "$fake_bridge"

  expected_pane="%777"
  PANEFLEET_AGENT_BRIDGE_BIN="$fake_bridge" TMUX_PANE="$expected_pane" "${REPO_ROOT}/scripts/codex-app-server-bridge" >/dev/null
  [[ "$(cat "$bridge_args")" == *"codex-app-server --pane ${expected_pane}"* ]] || fail "codex app-server wrapper should forward TMUX_PANE as --pane"

  PANEFLEET_AGENT_BRIDGE_BIN="$fake_bridge" PANEFLEET_PANE="%888" TMUX_PANE="$expected_pane" "${REPO_ROOT}/scripts/codex-notify-bridge" '{}' >/dev/null
  [[ "$(cat "$bridge_args")" == *"codex-notify --pane %888 {}"* ]] || fail "codex notify wrapper should prioritize PANEFLEET_PANE over TMUX_PANE"
  pass "codex wrappers forward pane when available"
}

test_wrapper_uses_xdg_state_home_for_event_logs() {
  local fake_bridge_root fake_bridge env_log expected_log_dir repo_root env_bin_log expected_panefleet_bin

  fake_bridge_root="${TEST_TMPDIR}/fake-bridge-xdg"
  fake_bridge="${fake_bridge_root}/panefleet-agent-bridge"
  env_log="${fake_bridge_root}/env.log"
  env_bin_log="${fake_bridge_root}/bin.log"
  mkdir -p "$fake_bridge_root"
  cat >"$fake_bridge" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\${PANEFLEET_EVENT_LOG_DIR:-}" >"$env_log"
printf '%s\n' "\${PANEFLEET_INGEST_BIN:-}" >"$env_bin_log"
exit 0
EOF
  chmod +x "$fake_bridge"

  expected_log_dir="${TEST_TMPDIR}/xdg-state/panefleet/events"
  repo_root="$(cd -- "${REPO_ROOT}" && pwd)"
  expected_panefleet_bin="${repo_root}/scripts/panefleet-go"
  XDG_STATE_HOME="${TEST_TMPDIR}/xdg-state" PANEFLEET_AGENT_BRIDGE_BIN="$fake_bridge" "${REPO_ROOT}/scripts/claude-code-hook" >/dev/null
  [[ "$(cat "$env_log")" == "$expected_log_dir" ]] || fail "wrapper should default PANEFLEET_EVENT_LOG_DIR from XDG_STATE_HOME"
  [[ "$(cat "$env_bin_log")" == "$expected_panefleet_bin" ]] || fail "wrapper should export PANEFLEET_INGEST_BIN pointing to panefleet-go"
  pass "wrapper event logs honor XDG_STATE_HOME"
}

test_panefleet_go_runs_from_repo_root() {
  local fake_bin fake_go cwd_log args_log outside_dir

  fake_bin="${TEST_TMPDIR}/fake-go-bin"
  fake_go="${fake_bin}/go"
  cwd_log="${TEST_TMPDIR}/panefleet-go.cwd.log"
  args_log="${TEST_TMPDIR}/panefleet-go.args.log"
  outside_dir="${TEST_TMPDIR}/outside-repo"
  mkdir -p "$fake_bin" "$outside_dir"

  cat >"$fake_go" <<EOF
#!/usr/bin/env bash
set -euo pipefail
pwd >"$cwd_log"
printf '%s\n' "\$*" >"$args_log"
exit 0
EOF
  chmod +x "$fake_go"

  (
    cd "$outside_dir"
    PATH="$fake_bin:${PATH}" "${REPO_ROOT}/scripts/panefleet-go" ingest --pane %7 --kind start >/dev/null
  )

  [[ "$(cat "$cwd_log")" == "$REPO_ROOT" ]] || fail "panefleet-go should run go from the panefleet repo root"
  [[ "$(cat "$args_log")" == "run ./cmd/panefleet ingest --pane %7 --kind start" ]] || fail "panefleet-go should run the local panefleet package from repo root"
  pass "panefleet-go runs from repo root"
}

test_uninstall_bindings_works_without_panefleet_bin() {
  local missing_root bindings_root globals_root hook_root

  rm -rf "${TEST_TMPDIR}/fake-tmux"
  missing_root="${TEST_TMPDIR}/missing-plugin"
  bindings_root="${TEST_TMPDIR}/fake-tmux/bindings/prefix"
  globals_root="${TEST_TMPDIR}/fake-tmux/globals"
  hook_root="${TEST_TMPDIR}/fake-tmux/hooks/after-select-pane"
  mkdir -p "$missing_root" "${TEST_TMPDIR}/fake-tmux"

  PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${FAKE_TMUX_BIN}" bind-key -T prefix P run-shell -b "panefleet popup"
  PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${FAKE_TMUX_BIN}" bind-key -T prefix T run-shell -b "panefleet theme-popup"
  PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${FAKE_TMUX_BIN}" set-hook -ag after-select-pane 'run-shell -b "/tmp/panefleet touch \"#{pane_id}\""'

  TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_ROOT="${missing_root}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${REPO_ROOT}/scripts/install-tmux-bindings.sh" uninstall >/dev/null

  [[ ! -e "${bindings_root}/P" ]] || fail "uninstall should remove prefix P binding even when panefleet bin is missing"
  [[ ! -e "${bindings_root}/T" ]] || fail "uninstall should remove prefix T binding even when panefleet bin is missing"
  [[ ! -d "${hook_root}" || -z "$(find "${hook_root}" -mindepth 1 -maxdepth 1 -type f 2>/dev/null)" ]] || fail "uninstall should remove panefleet touch hooks even when panefleet bin is missing"
  [[ ! -e "${globals_root}/@panefleet-done-recent-minutes" ]] || fail "uninstall should not seed tmux defaults"
  [[ ! -e "${globals_root}/@panefleet-stale-minutes" ]] || fail "uninstall should not seed stale defaults"
  [[ ! -e "${globals_root}/@panefleet-adapter-mode" ]] || fail "uninstall should not seed adapter defaults"
  pass "install-tmux-bindings uninstall stays side-effect free"
}

test_install_command() {
  local out_bin
  local plugin_dir
  local output
  local mode

  out_bin="${TEST_TMPDIR}/bin/install-bridge"
  plugin_dir="${TEST_TMPDIR}/install-opencode-plugins"

  output="$(TMUX='' TMUX_BIN="${FAKE_TMUX_BIN}" FZF_BIN="${FAKE_FZF_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" PANEFLEET_AGENT_BRIDGE_BIN="$out_bin" "${PANEFLEET_BIN}" install core)"
  [[ "$output" == *'Load core in tmux with: tmux source-file "'* ]] || fail "install core outside tmux should print the tmux load hint"

  output="$(TMUX='' TMUX_BIN="${FAKE_TMUX_BIN}" FZF_BIN="${FAKE_FZF_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" PANEFLEET_AGENT_BRIDGE_BIN="$out_bin" PANEFLEET_OPENCODE_PLUGIN_DIR="$plugin_dir" PANEFLEET_BRIDGE_INSTALL_MODE=build "${PANEFLEET_BIN}" install opencode)"
  [[ "$output" == *'Bridge: built locally with Go'* ]] || fail "install opencode should report how the bridge was installed"
  [[ "$output" == *'Adapter mode will switch to auto the next time you run install inside tmux.'* ]] || fail "install opencode outside tmux should explain deferred adapter mode"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" FZF_BIN="${FAKE_FZF_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" PANEFLEET_AGENT_BRIDGE_BIN="$out_bin" PANEFLEET_OPENCODE_PLUGIN_DIR="$plugin_dir" PANEFLEET_BRIDGE_INSTALL_MODE=build "${PANEFLEET_BIN}" install opencode >/dev/null
  [[ -x "$out_bin" ]] || fail "install opencode should ensure the bridge binary"
  [[ -f "${plugin_dir}/panefleet.ts" ]] || fail "install opencode should install the opencode plugin file"
  rg -Fq -- 'stderr: "ignore"' "${plugin_dir}/panefleet.ts" || fail "install opencode should silence bridge stderr inside the pane"
  ! rg -Fq -- 'stderr: "inherit"' "${plugin_dir}/panefleet.ts" || fail "install opencode should not inherit bridge stderr into the pane"
  ! rg -Fq -- 'await proc.exited' "${plugin_dir}/panefleet.ts" || fail "install opencode should not block on bridge exit"
  mode="$(cat "${TEST_TMPDIR}/fake-tmux/globals/@panefleet-adapter-mode")"
  [[ "$mode" == "auto" ]] || fail "install opencode should enable adapter mode in tmux"
  pass "install provides the public core and provider entrypoints"
}

test_cli_surface_contract() {
  local stderr_file

  stderr_file="${TEST_TMPDIR}/cli-surface.stderr"
  if "${PANEFLEET_BIN}" reconcile >/dev/null 2>"$stderr_file"; then
    fail "reconcile should not be part of the public CLI anymore"
  fi
  [[ "$(cat "$stderr_file")" == *"unknown command: reconcile"* ]] || fail "reconcile should fail as an unknown command"

  if "${PANEFLEET_BIN}" setup >/dev/null 2>"$stderr_file"; then
    fail "setup should not be part of the public CLI anymore"
  fi
  [[ "$(cat "$stderr_file")" == *"unknown command: setup"* ]] || fail "setup should fail as an unknown command"

  if "${PANEFLEET_BIN}" install-integrations >/dev/null 2>"$stderr_file"; then
    fail "install-integrations should not be part of the public CLI anymore"
  fi
  [[ "$(cat "$stderr_file")" == *"unknown command: install-integrations"* ]] || fail "install-integrations should fail as an unknown command"

  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" "${PANEFLEET_BIN}" uninstall >/dev/null
  pass "public CLI surface is install doctor uninstall"
}

test_runtime_install_contract() {
  local contract_root
  local case_root bridge_bin plugin_dir codex_config claude_settings
  local old_codex_wrapper old_claude_wrapper_a old_claude_wrapper_b custom_claude_hook
  local old_opencode_bridge
  local output doctor_output mode
  local codex_wrapper claude_wrapper

  contract_root="${TEST_TMPDIR}/runtime-contract"
  codex_wrapper="${REPO_ROOT}/scripts/codex-notify-bridge"
  claude_wrapper="${REPO_ROOT}/scripts/claude-code-hook"
  old_codex_wrapper="/opt/homebrew/Cellar/panefleet/0.3.0/libexec/scripts/codex-notify-bridge"
  old_claude_wrapper_a="/opt/homebrew/Cellar/panefleet/0.3.0/libexec/scripts/claude-code-hook"
  old_claude_wrapper_b="/opt/homebrew/Cellar/panefleet/0.3.2/libexec/scripts/claude-code-hook"
  custom_claude_hook="${TEST_TMPDIR}/custom-hooks/claude-custom-hook"
  old_opencode_bridge="/opt/homebrew/Cellar/panefleet/0.3.0/libexec/scripts/opencode-event-bridge"

  # core
  reset_fake_tmux_fixture
  case_root="${contract_root}/core"
  bridge_bin="${case_root}/bin/panefleet-agent-bridge"
  plugin_dir="${case_root}/opencode/plugins"
  codex_config="${case_root}/codex/config.toml"
  claude_settings="${case_root}/claude/settings.json"
  output="$(run_install_target_in_fake_tmux core "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$output" == *"Core installed"* ]] || fail "install core should confirm core install"
  [[ "$output" != *"Bridge:"* ]] || fail "install core should not install bridge"
  mode="$(cat "${TEST_TMPDIR}/fake-tmux/globals/@panefleet-adapter-mode")"
  [[ "$mode" == "heuristic-only" ]] || fail "install core should keep adapter mode heuristic-only"
  doctor_output="$(run_doctor_install_in_fake_tmux "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$doctor_output" == *"integration.codex   bridge-missing"* ]] || fail "install core should keep codex integration missing"
  [[ "$doctor_output" == *"integration.claude  bridge-missing"* ]] || fail "install core should keep claude integration missing"
  [[ "$doctor_output" == *"integration.opencode bridge-missing"* ]] || fail "install core should keep opencode integration missing"
  pass "install core runtime contract"

  # codex
  reset_fake_tmux_fixture
  case_root="${contract_root}/codex"
  bridge_bin="${case_root}/bin/panefleet-agent-bridge"
  plugin_dir="${case_root}/opencode/plugins"
  codex_config="${case_root}/codex/config.toml"
  claude_settings="${case_root}/claude/settings.json"
  mkdir -p "$(dirname "$codex_config")"
  cat >"$codex_config" <<EOF
# >>> panefleet codex notify >>>
notify = ["${old_codex_wrapper}"]
# <<< panefleet codex notify <<<
EOF
  output="$(run_install_target_in_fake_tmux codex "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$output" == *"Integration installed: codex"* ]] || fail "install codex should report codex integration"
  [[ "$output" == *"Bridge: built locally with Go"* ]] || fail "install codex should report local bridge build in tests"
  [[ "$output" == *"Adapter mode: auto"* ]] || fail "install codex should enable adapter mode"
  rg -Fq -- "$codex_wrapper" "$codex_config" || fail "install codex should wire codex config"
  ! rg -Fq -- "$old_codex_wrapper" "$codex_config" || fail "install codex should remove stale codex wrapper path"
  mode="$(cat "${TEST_TMPDIR}/fake-tmux/globals/@panefleet-adapter-mode")"
  [[ "$mode" == "auto" ]] || fail "install codex should set adapter mode auto"
  doctor_output="$(run_doctor_install_in_fake_tmux "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$doctor_output" == *"integration.codex   ready"* ]] || fail "install codex should be ready in doctor output"
  [[ "$doctor_output" == *"integration.claude  config-missing"* ]] || fail "install codex should leave claude config missing"
  [[ "$doctor_output" == *"integration.opencode plugin-missing"* ]] || fail "install codex should leave opencode plugin missing"
  pass "install codex runtime contract"

  # claude
  reset_fake_tmux_fixture
  case_root="${contract_root}/claude"
  bridge_bin="${case_root}/bin/panefleet-agent-bridge"
  plugin_dir="${case_root}/opencode/plugins"
  codex_config="${case_root}/codex/config.toml"
  claude_settings="${case_root}/claude/settings.json"
  mkdir -p "$(dirname "$claude_settings")" "$(dirname "$custom_claude_hook")"
  cat >"$custom_claude_hook" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "$custom_claude_hook"
  cat >"$claude_settings" <<EOF
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          { "type": "command", "command": "${old_claude_wrapper_a}" },
          { "type": "command", "command": "${custom_claude_hook}" }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "${old_claude_wrapper_b}" }
        ]
      }
    ],
    "PermissionRequest": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "${old_claude_wrapper_a}" }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          { "type": "command", "command": "${old_claude_wrapper_b}" }
        ]
      }
    ]
  }
}
EOF
  output="$(run_install_target_in_fake_tmux claude "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$output" == *"Integration installed: claude"* ]] || fail "install claude should report claude integration"
  [[ "$output" == *"Bridge: built locally with Go"* ]] || fail "install claude should report local bridge build in tests"
  [[ "$output" == *"Adapter mode: auto"* ]] || fail "install claude should enable adapter mode"
  rg -Fq -- "$claude_wrapper" "$claude_settings" || fail "install claude should wire claude settings"
  ! rg -Fq -- "$old_claude_wrapper_a" "$claude_settings" || fail "install claude should remove stale wrapper path 0.3.0"
  ! rg -Fq -- "$old_claude_wrapper_b" "$claude_settings" || fail "install claude should remove stale wrapper path 0.3.2"
  [[ "$(rg -F --count "$claude_wrapper" "$claude_settings")" == "4" ]] || fail "install claude should keep one hook command per claude event"
  [[ "$(rg -F --count "$custom_claude_hook" "$claude_settings")" == "1" ]] || fail "install claude should preserve non-panefleet custom hooks"
  mode="$(cat "${TEST_TMPDIR}/fake-tmux/globals/@panefleet-adapter-mode")"
  [[ "$mode" == "auto" ]] || fail "install claude should set adapter mode auto"
  doctor_output="$(run_doctor_install_in_fake_tmux "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$doctor_output" == *"integration.codex   config-missing"* ]] || fail "install claude should leave codex config missing"
  [[ "$doctor_output" == *"integration.claude  ready"* ]] || fail "install claude should be ready in doctor output"
  [[ "$doctor_output" == *"integration.opencode plugin-missing"* ]] || fail "install claude should leave opencode plugin missing"
  pass "install claude runtime contract"

  # opencode
  reset_fake_tmux_fixture
  case_root="${contract_root}/opencode"
  bridge_bin="${case_root}/bin/panefleet-agent-bridge"
  plugin_dir="${case_root}/opencode/plugins"
  codex_config="${case_root}/codex/config.toml"
  claude_settings="${case_root}/claude/settings.json"
  mkdir -p "$plugin_dir"
  cat >"${plugin_dir}/panefleet.ts" <<EOF
const bridgePath = process.env.PANEFLEET_OPENCODE_BRIDGE || "${old_opencode_bridge}"
EOF
  output="$(run_install_target_in_fake_tmux opencode "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$output" == *"Integration installed: opencode"* ]] || fail "install opencode should report opencode integration"
  [[ "$output" == *"Bridge: built locally with Go"* ]] || fail "install opencode should report local bridge build in tests"
  [[ "$output" == *"Adapter mode: auto"* ]] || fail "install opencode should enable adapter mode"
  [[ -f "${plugin_dir}/panefleet.ts" ]] || fail "install opencode should install plugin file"
  ! rg -Fq -- "$old_opencode_bridge" "${plugin_dir}/panefleet.ts" || fail "install opencode should remove stale bridge path in plugin file"
  rg -Fq -- 'process.env.PANEFLEET_OPENCODE_BRIDGE ||' "${plugin_dir}/panefleet.ts" || fail "install opencode should keep bridge env override in plugin file"
  rg -Fq -- 'stderr: "ignore"' "${plugin_dir}/panefleet.ts" || fail "install opencode should silence bridge stderr in the installed plugin"
  ! rg -Fq -- 'await proc.exited' "${plugin_dir}/panefleet.ts" || fail "install opencode should not await bridge exit"
  mode="$(cat "${TEST_TMPDIR}/fake-tmux/globals/@panefleet-adapter-mode")"
  [[ "$mode" == "auto" ]] || fail "install opencode should set adapter mode auto"
  doctor_output="$(run_doctor_install_in_fake_tmux "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$doctor_output" == *"integration.codex   config-missing"* ]] || fail "install opencode should leave codex config missing"
  [[ "$doctor_output" == *"integration.claude  config-missing"* ]] || fail "install opencode should leave claude config missing"
  assert_opencode_readyish "$doctor_output" "install opencode should be plugin-ready in doctor output"
  pass "install opencode runtime contract"

  # all + idempotence
  reset_fake_tmux_fixture
  case_root="${contract_root}/all"
  bridge_bin="${case_root}/bin/panefleet-agent-bridge"
  plugin_dir="${case_root}/opencode/plugins"
  codex_config="${case_root}/codex/config.toml"
  claude_settings="${case_root}/claude/settings.json"
  output="$(run_install_target_in_fake_tmux all "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$output" == *"Integrations installed: codex, claude, opencode"* ]] || fail "install all should report all integrations"
  [[ "$output" == *"Bridge: built locally with Go"* ]] || fail "install all should report local bridge build on first run"
  [[ "$output" == *"Adapter mode: auto"* ]] || fail "install all should enable adapter mode"

  output="$(run_install_target_in_fake_tmux all "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings" auto)"
  [[ "$output" == *"Integrations installed: codex, claude, opencode"* ]] || fail "install all second run should still report all integrations"
  [[ "$output" == *"Bridge: already installed"* ]] || fail "install all second run should report already installed bridge"

  output="$(run_install_target_in_fake_tmux all "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings" force-build)"
  [[ "$output" == *"Bridge: built locally with Go"* ]] || fail "install all force-build should rebuild the bridge"
  [[ "$(rg -F --count '# >>> panefleet codex notify >>>' "$codex_config")" == "1" ]] || fail "install all should keep a single managed codex block after rerun"
  [[ "$(rg -F --count 'notify = [' "$codex_config")" == "1" ]] || fail "install all should keep one codex notify entry after rerun"
  [[ "$(rg -F --count "$claude_wrapper" "$claude_settings")" == "4" ]] || fail "install all should keep one claude hook command per event after rerun"
  [[ -f "${plugin_dir}/panefleet.ts" ]] || fail "install all should keep opencode plugin installed"
  mode="$(cat "${TEST_TMPDIR}/fake-tmux/globals/@panefleet-adapter-mode")"
  [[ "$mode" == "auto" ]] || fail "install all should keep adapter mode auto"
  doctor_output="$(run_doctor_install_in_fake_tmux "$bridge_bin" "$plugin_dir" "$codex_config" "$claude_settings")"
  [[ "$doctor_output" == *"integration.codex   ready"* ]] || fail "install all should make codex ready"
  [[ "$doctor_output" == *"integration.claude  ready"* ]] || fail "install all should make claude ready"
  assert_opencode_readyish "$doctor_output" "install all should make opencode plugin-ready"
  pass "install all runtime contract and idempotence"
}

setup_fake_tmux_fixture "${TEST_TMPDIR}/fake-tmux"
test_sourced_helpers
test_fake_tmux_cli
test_install_command
test_codex_wrapper_forwards_pane_when_available
test_wrapper_uses_xdg_state_home_for_event_logs
test_panefleet_go_runs_from_repo_root
test_wrapper_install_hints
test_uninstall_bindings_works_without_panefleet_bin
test_cli_surface_contract
test_runtime_install_contract
