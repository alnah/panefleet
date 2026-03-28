#!/usr/bin/env bash

set -euo pipefail

# ops-healthcheck.sh runs fast operational checks that are safe in automation.
# It is intentionally read-only so operators can run it during incidents.

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/panefleet/runtime/paths.sh"

REPO_ROOT="${PANEFLEET_ROOT:-$(panefleet_find_repo_root_from "${BASH_SOURCE[0]}")}"
PANEFLEET_BIN="${PANEFLEET_BIN:-${REPO_ROOT}/bin/panefleet}"
PANEFLEET_GO_BIN="${PANEFLEET_GO_BIN:-${REPO_ROOT}/scripts/panefleet-go}"
require_bridge="${PANEFLEET_REQUIRE_BRIDGE:-0}"

if [[ ! -x "${PANEFLEET_BIN}" ]]; then
  printf 'healthcheck: panefleet binary not executable: %s\n' "${PANEFLEET_BIN}" >&2
  exit 1
fi
if [[ ! -x "${PANEFLEET_GO_BIN}" ]]; then
  printf 'healthcheck: panefleet go runtime not executable: %s\n' "${PANEFLEET_GO_BIN}" >&2
  exit 1
fi

printf 'healthcheck: preflight... '
if "${PANEFLEET_BIN}" preflight --quiet >/dev/null 2>&1; then
  printf 'ok\n'
else
  printf 'failed\n'
  printf "healthcheck: run \`%s preflight\` for details\n" "${PANEFLEET_BIN}" >&2
  exit 1
fi

printf 'healthcheck: go liveness... '
if "${PANEFLEET_GO_BIN}" health --check liveness >/dev/null; then
  printf 'ok\n'
else
  printf 'failed\n'
  exit 1
fi

if [[ -n "${TMUX:-}" ]]; then
  printf 'healthcheck: go readiness... '
  if "${PANEFLEET_GO_BIN}" health --check readiness >/dev/null; then
    printf 'ok\n'
  else
    printf 'failed\n'
    exit 1
  fi
else
  printf 'healthcheck: go readiness... skipped (outside tmux)\n'
fi

install_report="$("${PANEFLEET_BIN}" doctor --install)"
printf 'healthcheck: install diagnostics... ok\n'

bridge_bin="$(printf '%s\n' "${install_report}" | awk '/^bridge\.bin/{print $2; exit}')"
bridge_present="$(printf '%s\n' "${install_report}" | awk '/^bridge\.present/{print $2; exit}')"
adapter_default="$(printf '%s\n' "${install_report}" | awk '/^adapter\.default/{print $2; exit}')"

printf 'healthcheck: adapter.default=%s bridge.present=%s\n' "${adapter_default:-unknown}" "${bridge_present:-unknown}"

if [[ "${require_bridge}" == "1" && "${bridge_present}" != "yes" ]]; then
  printf 'healthcheck: bridge required but missing (bridge.bin=%s)\n' "${bridge_bin:-unknown}" >&2
  exit 1
fi

printf 'healthcheck: doctor summary\n'
"${PANEFLEET_BIN}" doctor
