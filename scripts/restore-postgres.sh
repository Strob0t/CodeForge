#!/usr/bin/env bash
# CodeForge PostgreSQL Restore Script
# Usage:
#   ./scripts/restore-postgres.sh <backup-file>
#   ./scripts/restore-postgres.sh latest
#
# Environment:
#   PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE (standard libpq vars)
#   BACKUP_DIR (default: ./backups/postgres)
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-./backups/postgres}"
TARGET="${1:?Usage: $0 <backup-file|latest>}"
DB="${PGDATABASE:-codeforge}"

if [[ "$TARGET" == "latest" ]]; then
  TARGET="$(find "$BACKUP_DIR" -name "codeforge_*.sql.gz" -print0 | xargs -0 ls -t 2>/dev/null | head -1)"
  if [[ -z "$TARGET" ]]; then
    echo "No backups found in $BACKUP_DIR"
    exit 1
  fi
fi

if [[ ! -f "$TARGET" ]]; then
  echo "Backup file not found: $TARGET"
  exit 1
fi

echo "Restoring from: $TARGET"
echo "Target database: $DB"
echo "WARNING: This will DROP and recreate the database."
read -r -p "Continue? [y/N] " confirm
[[ "$confirm" =~ ^[Yy]$ ]] || exit 0

# Terminate active connections
psql -d postgres -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$DB' AND pid <> pg_backend_pid();" \
  > /dev/null 2>&1 || true

dropdb --if-exists "$DB"
createdb "$DB"

pg_restore \
  --dbname="$DB" \
  --no-owner \
  --no-privileges \
  "$TARGET"

echo "Restore complete."
