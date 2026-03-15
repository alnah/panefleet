#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEST_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/panefleet-bridge-tests.XXXXXX")"
trap 'rm -rf "${TEST_TMPDIR}"' EXIT

FAKE_BIN="${TEST_TMPDIR}/fake-bin"
GO_LOG="${TEST_TMPDIR}/go.log"
CURL_LOG="${TEST_TMPDIR}/curl.log"
TAR_LOG="${TEST_TMPDIR}/tar.log"
CURL_COUNT_FILE="${TEST_TMPDIR}/curl.count"
SYSTEM_PATH="${PATH}"

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

reset_logs() {
  : >"$GO_LOG"
  : >"$CURL_LOG"
  : >"$TAR_LOG"
  rm -f "$CURL_COUNT_FILE"
}

reset_behavior() {
  unset GIT_TAG CURL_FAIL_MODE GO_SHOULD_FAIL TAR_SHOULD_FAIL || true
  export UNAME_S="Linux"
  export UNAME_M="x86_64"
}

run_install_bridge() {
  local mode="$1"
  local output_bin="$2"
  local stdout_file="$3"
  local stderr_file="$4"

  PATH="${FAKE_BIN}:${SYSTEM_PATH}" \
  GO_LOG="$GO_LOG" \
  CURL_LOG="$CURL_LOG" \
  TAR_LOG="$TAR_LOG" \
  CURL_COUNT_FILE="$CURL_COUNT_FILE" \
  PANEFLEET_ROOT="$REPO_ROOT" \
  PANEFLEET_AGENT_BRIDGE_BIN="$output_bin" \
  PANEFLEET_BRIDGE_INSTALL_MODE="$mode" \
  "${REPO_ROOT}/scripts/install-bridge.sh" >"$stdout_file" 2>"$stderr_file"
}

mkdir -p "$FAKE_BIN"

cat >"${FAKE_BIN}/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

: "${GO_LOG:?GO_LOG is required}"
printf '%s\n' "$*" >>"$GO_LOG"

if [[ "${GO_SHOULD_FAIL:-0}" == "1" ]]; then
  exit 1
fi

if [[ "${1:-}" != "build" ]]; then
  exit 0
fi

out=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "-o" ]]; then
    out="$arg"
    break
  fi
  prev="$arg"
done

if [[ -z "$out" ]]; then
  printf 'fake go: missing -o target\n' >&2
  exit 2
fi

mkdir -p "$(dirname "$out")"
printf '#!/usr/bin/env bash\nexit 0\n' >"$out"
chmod 755 "$out"
EOF
chmod +x "${FAKE_BIN}/go"

cat >"${FAKE_BIN}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

: "${CURL_LOG:?CURL_LOG is required}"
printf '%s\n' "$*" >>"$CURL_LOG"

url=""
out=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "-o" ]]; then
    out="$arg"
  fi
  if [[ "$arg" == http*://* ]]; then
    url="$arg"
  fi
  prev="$arg"
done

case "${CURL_FAIL_MODE:-none}" in
  always)
    exit 1
    ;;
  exact-tag)
    if [[ "$url" == *"/releases/download/"* && "$url" != *"/releases/latest/download/"* ]]; then
      exit 1
    fi
    ;;
  first)
    : "${CURL_COUNT_FILE:?CURL_COUNT_FILE is required}"
    if [[ ! -f "$CURL_COUNT_FILE" ]]; then
      printf '1' >"$CURL_COUNT_FILE"
      exit 1
    fi
    ;;
esac

if [[ -n "$out" ]]; then
  mkdir -p "$(dirname "$out")"
  : >"$out"
fi
EOF
chmod +x "${FAKE_BIN}/curl"

cat >"${FAKE_BIN}/tar" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

: "${TAR_LOG:?TAR_LOG is required}"
printf '%s\n' "$*" >>"$TAR_LOG"

if [[ "${TAR_SHOULD_FAIL:-0}" == "1" ]]; then
  exit 1
fi

dest=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "-C" ]]; then
    dest="$arg"
    break
  fi
  prev="$arg"
done

if [[ -z "$dest" ]]; then
  printf 'fake tar: missing -C destination\n' >&2
  exit 2
fi

mkdir -p "$dest"
printf '#!/usr/bin/env bash\nexit 0\n' >"${dest}/panefleet-agent-bridge"
chmod 755 "${dest}/panefleet-agent-bridge"
EOF
chmod +x "${FAKE_BIN}/tar"

cat >"${FAKE_BIN}/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "-C" ]]; then
  shift 2
fi

if [[ "${1:-}" != "describe" ]]; then
  exit 1
fi

if [[ -z "${GIT_TAG:-}" ]]; then
  exit 1
fi

printf '%s\n' "$GIT_TAG"
EOF
chmod +x "${FAKE_BIN}/git"

cat >"${FAKE_BIN}/uname" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  -s)
    printf '%s\n' "${UNAME_S:-Linux}"
    ;;
  -m)
    printf '%s\n' "${UNAME_M:-x86_64}"
    ;;
  *)
    real_uname="/usr/bin/uname"
    if [[ ! -x "$real_uname" ]]; then
      real_uname="/bin/uname"
    fi
    exec "$real_uname" "$@"
    ;;
esac
EOF
chmod +x "${FAKE_BIN}/uname"

case_dir="${TEST_TMPDIR}/cases"
mkdir -p "$case_dir"

test_unknown_mode_rejected() {
  local out_bin stdout_file stderr_file

  reset_behavior
  reset_logs
  out_bin="${case_dir}/unknown/bridge"
  stdout_file="${case_dir}/unknown/stdout"
  stderr_file="${case_dir}/unknown/stderr"
  mkdir -p "$(dirname "$out_bin")" "$(dirname "$stdout_file")"

  if run_install_bridge "bad-mode" "$out_bin" "$stdout_file" "$stderr_file"; then
    fail "install-bridge should reject unknown mode"
  fi
  assert_contains "$stderr_file" "unknown PANEFLEET_BRIDGE_INSTALL_MODE: bad-mode" "unknown mode should show explicit error"
  pass "install-bridge rejects unknown mode"
}

test_auto_skips_when_bridge_already_exists() {
  local out_bin stdout_file stderr_file

  reset_behavior
  reset_logs
  out_bin="${case_dir}/auto-skip/bridge"
  stdout_file="${case_dir}/auto-skip/stdout"
  stderr_file="${case_dir}/auto-skip/stderr"
  mkdir -p "$(dirname "$out_bin")" "$(dirname "$stdout_file")"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$out_bin"
  chmod 755 "$out_bin"

  run_install_bridge "auto" "$out_bin" "$stdout_file" "$stderr_file"
  assert_contains "$stdout_file" "Bridge already installed $out_bin" "auto mode should short-circuit on existing bridge"
  [[ ! -s "$GO_LOG" ]] || fail "auto skip should not invoke go"
  [[ ! -s "$CURL_LOG" ]] || fail "auto skip should not invoke curl"
  pass "auto mode skips install when bridge already exists"
}

test_auto_prefers_exact_tag_download() {
  local out_bin stdout_file stderr_file

  reset_behavior
  reset_logs
  export GIT_TAG="v9.9.9"
  out_bin="${case_dir}/auto-tag/bridge"
  stdout_file="${case_dir}/auto-tag/stdout"
  stderr_file="${case_dir}/auto-tag/stderr"
  mkdir -p "$(dirname "$out_bin")" "$(dirname "$stdout_file")"

  run_install_bridge "auto" "$out_bin" "$stdout_file" "$stderr_file"
  assert_contains "$stdout_file" "Installed prebuilt bridge $out_bin" "auto mode should install prebuilt bridge when exact tag download works"
  assert_contains "$CURL_LOG" "/releases/download/v9.9.9/" "auto mode should try exact tagged release first"
  [[ ! -s "$GO_LOG" ]] || fail "exact tag download success should not invoke go build"
  [[ -x "$out_bin" ]] || fail "downloaded bridge should be executable"
  pass "auto mode prefers exact tag release download"
}

test_auto_falls_back_to_build_after_download_failure() {
  local out_bin stdout_file stderr_file

  reset_behavior
  reset_logs
  export GIT_TAG="v1.2.3"
  export CURL_FAIL_MODE="exact-tag"
  out_bin="${case_dir}/auto-build-fallback/bridge"
  stdout_file="${case_dir}/auto-build-fallback/stdout"
  stderr_file="${case_dir}/auto-build-fallback/stderr"
  mkdir -p "$(dirname "$out_bin")" "$(dirname "$stdout_file")"

  run_install_bridge "auto" "$out_bin" "$stdout_file" "$stderr_file"
  assert_contains "$stdout_file" "Built $out_bin" "auto mode should build when exact download fails and go build works"
  assert_contains "$GO_LOG" "build -o $out_bin" "auto build fallback should invoke go build"
  [[ -x "$out_bin" ]] || fail "built bridge should be executable"
  pass "auto mode falls back to build when exact download fails"
}

test_auto_falls_back_to_latest_download_after_build_failure() {
  local out_bin stdout_file stderr_file

  reset_behavior
  reset_logs
  export GIT_TAG="v1.2.3"
  export CURL_FAIL_MODE="exact-tag"
  export GO_SHOULD_FAIL="1"
  out_bin="${case_dir}/auto-latest-fallback/bridge"
  stdout_file="${case_dir}/auto-latest-fallback/stdout"
  stderr_file="${case_dir}/auto-latest-fallback/stderr"
  mkdir -p "$(dirname "$out_bin")" "$(dirname "$stdout_file")"

  run_install_bridge "auto" "$out_bin" "$stdout_file" "$stderr_file"
  assert_contains "$stdout_file" "Installed prebuilt bridge $out_bin" "auto mode should fall back to latest download after build failure"
  assert_contains "$CURL_LOG" "/releases/latest/download/" "auto mode should attempt latest download after build failure"
  assert_contains "$GO_LOG" "build -o $out_bin" "auto latest fallback should still attempt go build before latest"
  [[ -x "$out_bin" ]] || fail "latest downloaded bridge should be executable"
  pass "auto mode falls back to latest download after build failure"
}

test_force_modes_override_short_circuit() {
  local out_bin stdout_file stderr_file

  reset_behavior
  reset_logs
  out_bin="${case_dir}/force/bridge"
  stdout_file="${case_dir}/force/stdout"
  stderr_file="${case_dir}/force/stderr"
  mkdir -p "$(dirname "$out_bin")" "$(dirname "$stdout_file")"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$out_bin"
  chmod 755 "$out_bin"

  run_install_bridge "force-build" "$out_bin" "$stdout_file" "$stderr_file"
  assert_contains "$stdout_file" "Built $out_bin" "force-build should rebuild even when bridge exists"
  assert_contains "$GO_LOG" "build -o $out_bin" "force-build should invoke go build"

  reset_logs
  run_install_bridge "force-download" "$out_bin" "$stdout_file" "$stderr_file"
  assert_contains "$stdout_file" "Installed prebuilt bridge $out_bin" "force-download should redownload even when bridge exists"
  assert_contains "$CURL_LOG" "/releases/latest/download/" "force-download should use download path"
  pass "force modes bypass idempotent short-circuit"
}

test_auto_errors_when_all_fallbacks_fail() {
  local out_bin stdout_file stderr_file

  reset_behavior
  reset_logs
  export GIT_TAG="v1.2.3"
  export CURL_FAIL_MODE="always"
  export GO_SHOULD_FAIL="1"
  out_bin="${case_dir}/auto-fail/bridge"
  stdout_file="${case_dir}/auto-fail/stdout"
  stderr_file="${case_dir}/auto-fail/stderr"
  mkdir -p "$(dirname "$out_bin")" "$(dirname "$stdout_file")"

  if run_install_bridge "auto" "$out_bin" "$stdout_file" "$stderr_file"; then
    fail "auto mode should fail when download and build fallbacks fail"
  fi
  assert_contains "$stderr_file" "panefleet: could not install the bridge automatically." "auto failure should show consolidated error"
  pass "auto mode reports clear failure when all fallbacks fail"
}

test_unknown_mode_rejected
test_auto_skips_when_bridge_already_exists
test_auto_prefers_exact_tag_download
test_auto_falls_back_to_build_after_download_failure
test_auto_falls_back_to_latest_download_after_build_failure
test_force_modes_override_short_circuit
test_auto_errors_when_all_fallbacks_fail
