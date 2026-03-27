#!/usr/bin/env bash

# shellcheck disable=SC1091
source "$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/runtime/paths.sh"

panefleet_db_path() {
  if [[ -n "${PANEFLEET_DB_PATH:-}" ]]; then
    printf '%s' "$PANEFLEET_DB_PATH"
  else
    printf '%s/panefleet/panefleet.db' "$(panefleet_user_state_home)"
  fi
}

panefleet_default_backup_dir() {
  printf '%s/panefleet/backups' "$(panefleet_user_state_home)"
}

panefleet_require_file_db_path() {
  local db_path="$1"

  if [[ "$db_path" == ":memory:" || "$db_path" == file:* || "$db_path" == *'?'* ]]; then
    printf 'unsupported PANEFLEET_DB_PATH for file-backed sqlite ops: %s\n' "$db_path" >&2
    return 1
  fi
}

panefleet_sqlite3_bin() {
  local sqlite_bin="${SQLITE3_BIN:-sqlite3}"

  if ! command -v "$sqlite_bin" >/dev/null 2>&1; then
    printf 'sqlite3 is required for safe SQLite backup/restore operations.\n' >&2
    return 1
  fi

  printf '%s' "$sqlite_bin"
}

panefleet_sqlite_cli_arg() {
  local value="$1"

  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '"%s"' "$value"
}

panefleet_sqlite_backup() {
  local db_path="$1"
  local output_path="$2"
  local sqlite_bin dot_arg

  sqlite_bin="$(panefleet_sqlite3_bin)" || return 1
  dot_arg="$(panefleet_sqlite_cli_arg "$output_path")"
  "$sqlite_bin" "$db_path" ".timeout 5000" ".backup ${dot_arg}"
}

panefleet_sqlite_restore() {
  local db_path="$1"
  local backup_path="$2"
  local sqlite_bin dot_arg

  sqlite_bin="$(panefleet_sqlite3_bin)" || return 1
  dot_arg="$(panefleet_sqlite_cli_arg "$backup_path")"
  "$sqlite_bin" "$db_path" ".timeout 5000" ".restore ${dot_arg}"
}
