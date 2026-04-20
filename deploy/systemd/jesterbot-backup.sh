#!/usr/bin/env bash
# Creates a point-in-time SQLite backup and removes old backups.
# Entry point is this script called by jesterbot-backup.service.
# Coupled with deploy/systemd/jesterbot-backup.service and .timer.
# Keep retention behavior simple: delete files older than N days.
#
set -euo pipefail

DB_PATH="${JESTERBOT_DB_PATH:-/opt/jesterbot/data/jesterbot.db}"
BACKUP_DIR="${JESTERBOT_BACKUP_DIR:-/opt/jesterbot/backups}"
RETENTION_DAYS="${JESTERBOT_BACKUP_RETENTION_DAYS:-3}"

if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "sqlite3 is required for backups" >&2
  exit 1
fi

if [[ ! -f "${DB_PATH}" ]]; then
  echo "database file not found: ${DB_PATH}" >&2
  exit 1
fi

mkdir -p "${BACKUP_DIR}"

db_file_name="$(basename "${DB_PATH}")"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_path="${BACKUP_DIR}/${db_file_name}.${timestamp}.sqlite3"

sqlite3 "${DB_PATH}" ".timeout 5000" ".backup '${backup_path}'"

find "${BACKUP_DIR}" -maxdepth 1 -type f -name "${db_file_name}.*.sqlite3" -mtime +"${RETENTION_DAYS}" -delete
