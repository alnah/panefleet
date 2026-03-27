#!/usr/bin/env bash

set -euo pipefail

# ops-go-db-backup.sh creates a timestamped copy of the Go runtime sqlite DB.
# This enables a fast rollback point before migrations or risky maintenance.

# shellcheck disable=SC1091
source "$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/lib/panefleet/ops/sqlite.sh"

db_path="$(panefleet_db_path)"
backup_dir="${PANEFLEET_DB_BACKUP_DIR:-$(panefleet_default_backup_dir)}"
panefleet_require_file_db_path "${db_path}" || exit 1

if [[ ! -f "${db_path}" ]]; then
  printf 'backup: database not found: %s\n' "${db_path}" >&2
  exit 1
fi

mkdir -p "${backup_dir}"
chmod 700 "${backup_dir}" 2>/dev/null || true

ts="$(date -u +%Y%m%dT%H%M%SZ)"
backup_file="${backup_dir}/panefleet-${ts}.db"

if ! panefleet_sqlite_backup "${db_path}" "${backup_file}"; then
  printf 'backup: sqlite backup failed for %s\n' "${db_path}" >&2
  rm -f "${backup_file}"
  exit 1
fi

chmod 600 "${backup_file}" 2>/dev/null || true

printf 'backup: created %s\n' "${backup_file}"
