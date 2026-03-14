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
  mkdir -p "$root/panes/%101/options" "$root/panes/%102/options" "$root/panes/%103/options"

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

  resolve_uncached_state_values "%102" "opencode" "opencode" "OC | Greeting" 0 "" "$(date +%s)" $'Ask anything...\nctrl+p commands\ntab agents' "" "" "" "" "" "" 10 45 600 "$(date +%s)"
  assert_eq "$PANEFLEET_RESOLVED_STATUS" "DONE" "resolve_uncached_state_values infers opencode done"
  pass "resolve_uncached_state_values infers opencode done"
}

test_fake_tmux_cli() {
  local output line101 line102 line103

  output="$(run_list)"
  line101="$(printf '%s\n' "$output" | rg '^%101')"
  line102="$(printf '%s\n' "$output" | rg '^%102')"
  line103="$(printf '%s\n' "$output" | rg '^%103')"

  [[ "$line101" == *"WAIT"* ]] || fail "codex pane should be WAIT"
  [[ "$line102" == *"DONE"* ]] || fail "opencode pane should be DONE"
  [[ "$line103" == *"IDLE"* ]] || fail "shell pane should be IDLE"
  pass "fake tmux list shows expected baseline statuses"

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
}

setup_fake_tmux_fixture "${TEST_TMPDIR}/fake-tmux"
test_sourced_helpers
test_fake_tmux_cli
