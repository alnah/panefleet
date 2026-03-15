#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEST_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/panefleet-make-tests.XXXXXX")"
trap 'rm -rf "${TEST_TMPDIR}"' EXIT

pass() {
  printf 'ok - %s\n' "$1"
}

fail() {
  printf 'not ok - %s\n' "$1" >&2
  exit 1
}

assert_contains() {
  local file="$1"
  local expected="$2"
  local msg="$3"

  if ! rg -Fq -- "$expected" "$file"; then
    fail "$msg"
  fi
}

make_with_stubs() {
  local output_file="$1"
  local error_file="$2"
  shift 2

  make -C "$REPO_ROOT" \
    PANEFLEET_INSTALL_DEPS_CMD="${FAKE_DEPS_BIN}" \
    PANEFLEET_BIN="${FAKE_PANEFLEET_BIN}" \
    "$@" >"$output_file" 2>"$error_file"
}

FAKE_DEPS_BIN="${TEST_TMPDIR}/fake-install-deps.sh"
FAKE_PANEFLEET_BIN="${TEST_TMPDIR}/fake-panefleet.sh"
DEPS_LOG="${TEST_TMPDIR}/deps.log"
PF_LOG="${TEST_TMPDIR}/panefleet.log"

cat >"$FAKE_DEPS_BIN" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf 'deps\n' >>"${DEPS_LOG}"
EOF
chmod +x "$FAKE_DEPS_BIN"

cat >"$FAKE_PANEFLEET_BIN" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "\$*" >>"${PF_LOG}"
EOF
chmod +x "$FAKE_PANEFLEET_BIN"

run_success_case() {
  local cmd_label="$1"
  shift
  local out_file="${TEST_TMPDIR}/${cmd_label}.out"
  local err_file="${TEST_TMPDIR}/${cmd_label}.err"

  : >"$DEPS_LOG"
  : >"$PF_LOG"
  make_with_stubs "$out_file" "$err_file" "$@"
  [[ ! -s "$err_file" ]] || fail "${cmd_label} should not write stderr"
}

run_success_case "install-core" install core
assert_contains "$DEPS_LOG" "deps" "make install core should run deps installer"
assert_contains "$PF_LOG" "install core" "make install core should call panefleet install core"
pass "make install core uses canonical install path"

run_success_case "install-codex" install codex
assert_contains "$PF_LOG" "install codex" "make install codex should call panefleet install codex"
pass "make install codex uses canonical install path"

run_success_case "install-claude" install claude
assert_contains "$PF_LOG" "install claude" "make install claude should call panefleet install claude"
pass "make install claude uses canonical install path"

run_success_case "install-opencode" install opencode
assert_contains "$PF_LOG" "install opencode" "make install opencode should call panefleet install opencode"
pass "make install opencode uses canonical install path"

run_success_case "install-all" install all
assert_contains "$PF_LOG" "install all" "make install all should call panefleet install all"
pass "make install all uses canonical install path"

run_success_case "shortcut-core" core
assert_contains "$PF_LOG" "install core" "make core should call panefleet install core"
pass "make core shortcut stays aligned with install core"

run_success_case "shortcut-all" all
assert_contains "$PF_LOG" "install all" "make all should call panefleet install all"
pass "make all shortcut stays aligned with install all"

missing_out="${TEST_TMPDIR}/install-missing.out"
missing_err="${TEST_TMPDIR}/install-missing.err"
: >"$DEPS_LOG"
: >"$PF_LOG"
if make_with_stubs "$missing_out" "$missing_err" install; then
  fail "make install without target should fail"
fi
assert_contains "$missing_err" "usage: make install core|codex|claude|opencode|all" "make install without target should print usage"
[[ ! -s "$DEPS_LOG" ]] || fail "make install without target should not run deps installer"
[[ ! -s "$PF_LOG" ]] || fail "make install without target should not call panefleet"
pass "make install rejects missing target"

bad_out="${TEST_TMPDIR}/install-bad.out"
bad_err="${TEST_TMPDIR}/install-bad.err"
: >"$DEPS_LOG"
: >"$PF_LOG"
if make_with_stubs "$bad_out" "$bad_err" install invalid; then
  fail "make install invalid should fail"
fi
assert_contains "$bad_err" "unknown install target: invalid" "make install invalid should explain target error"
[[ ! -s "$DEPS_LOG" ]] || fail "make install invalid should not run deps installer"
[[ ! -s "$PF_LOG" ]] || fail "make install invalid should not call panefleet"
pass "make install rejects unknown target"
