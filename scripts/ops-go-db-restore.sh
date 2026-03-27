#!/usr/bin/env bash

set -euo pipefail

# ops-go-db-restore.sh restores a backup into the Go runtime sqlite DB path.
# It keeps a safety copy of the current DB first to make rollback reversible.

# shellcheck disable=SC1091
source "$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/lib/panefleet/ops/sqlite.sh"

if (($# != 1)); then
  printf 'usage: %s <backup-file>\n' "$0" >&2
  exit 1
fi

backup_file="$1"
if [[ ! -f "${backup_file}" ]]; then
  printf 'restore: backup file not found: %s\n' "${backup_file}" >&2
  exit 1
fi

db_path="$(panefleet_db_path)"
rollback_copy="${db_path}.pre-restore.$(date -u +%Y%m%dT%H%M%SZ)"

panefleet_require_file_db_path "${db_path}" || exit 1

mkdir -p "$(dirname "${db_path}")"
chmod 700 "$(dirname "${db_path}")" 2>/dev/null || true

if [[ -f "${db_path}" ]]; then
  if ! panefleet_sqlite_backup "${db_path}" "${rollback_copy}"; then
    printf 'restore: failed to create rollback copy for %s\n' "${db_path}" >&2
    rm -f "${rollback_copy}"
    exit 1
  fi
  chmod 600 "${rollback_copy}" 2>/dev/null || true
  printf 'restore: saved rollback copy %s\n' "${rollback_copy}"
fi

if ! panefleet_sqlite_restore "${db_path}" "${backup_file}"; then
  printf 'restore: sqlite restore failed from %s\n' "${backup_file}" >&2
  exit 1
fi

chmod 600 "${db_path}" 2>/dev/null || true

printf 'restore: restored %s -> %s\n' "${backup_file}" "${db_path}"
