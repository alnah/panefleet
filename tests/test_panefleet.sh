#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PANEFLEET_BIN="${REPO_ROOT}/bin/panefleet"
FAKE_TMUX_BIN="${REPO_ROOT}/tests/fake-tmux"
TEST_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/panefleet-tests.XXXXXX")"
trap 'rm -rf "${TEST_TMPDIR}"' EXIT

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

test_sourced_helpers() {
  local got

  # shellcheck disable=SC1090
  PANEFLEET_SOURCE_ONLY=1 source "${PANEFLEET_BIN}"

  if agent_status_is_fresh "$(date +%s)" 600 "$(date +%s)"; then
    pass "agent_status_is_fresh accepts current timestamp"
  else
    fail "agent_status_is_fresh rejects current timestamp"
  fi

  got="$(effective_status_values DONE "$(date +%s)" "$(date +%s)" "" 10 45 "$(date +%s)")"
  assert_eq "$got" "DONE" "effective_status_values keeps recent DONE"
  pass "effective_status_values keeps recent done"

  got="$(effective_status_values DONE "$(date +%s)" "$(( $(date +%s) - 3600 ))" "" 10 45 "$(date +%s)")"
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

  # shellcheck disable=SC2329
  codex_process_is_working() { return 0; }
  got="$(adapter_status "%101" "codex" "codex-aarch64-a" 0 "" $'No chooser\nNo prompt yet')"
  assert_eq "$got" "RUN" "adapter_status infers codex run from process tree"
  pass "adapter_status infers codex run from process tree"
  unset -f codex_process_is_working

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
  local output line101 line102 line103 line104 inspect_output doctor_output install_doctor_output

  output="$(run_list)"
  line101="$(printf '%s\n' "$output" | rg '^%101')"
  line102="$(printf '%s\n' "$output" | rg '^%102')"
  line103="$(printf '%s\n' "$output" | rg '^%103')"
  line104="$(printf '%s\n' "$output" | rg '^%104')"

  [[ "$line101" == *"WAIT"* ]] || fail "codex pane should be WAIT"
  [[ "$line102" == *"DONE"* ]] || fail "opencode pane should be DONE"
  [[ "$line103" == *"IDLE"* ]] || fail "shell pane should be IDLE"
  [[ "$line104" == *"RUN"* ]] || fail "recent active opencode pane should bypass stale DONE cache"
  pass "fake tmux list shows expected baseline statuses"

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

test_install_integrations_command() {
  local out_bin
  local mode
  local plugin_dir

  out_bin="${TEST_TMPDIR}/bin/panefleet-agent-bridge"
  plugin_dir="${TEST_TMPDIR}/opencode-plugins"
  mkdir -p "$(dirname "$out_bin")"
  TMUX=1 TMUX_BIN="${FAKE_TMUX_BIN}" PANEFLEET_FAKE_TMUX_DIR="${TEST_TMPDIR}/fake-tmux" PANEFLEET_AGENT_BRIDGE_BIN="$out_bin" PANEFLEET_OPENCODE_PLUGIN_DIR="$plugin_dir" PANEFLEET_BRIDGE_INSTALL_MODE=build "${PANEFLEET_BIN}" install-integrations >/dev/null
  [[ -x "$out_bin" ]] || fail "install-integrations should build the bridge binary"
  [[ -f "${plugin_dir}/panefleet.ts" ]] || fail "install-integrations should install the opencode plugin file"
  mode="$(cat "${TEST_TMPDIR}/fake-tmux/globals/@panefleet-adapter-mode")"
  [[ "$mode" == "auto" ]] || fail "install-integrations should enable adapter mode in tmux"
  pass "install-integrations builds the bridge binary"
}

setup_fake_tmux_fixture "${TEST_TMPDIR}/fake-tmux"
test_sourced_helpers
test_fake_tmux_cli
test_install_integrations_command
