#!/usr/bin/env bash
# CodeForge PostgreSQL Backup Script
# Usage:
#   ./scripts/backup-postgres.sh              Run backup now
#   ./scripts/backup-postgres.sh --cleanup    Run backup + enforce retention
#
# Environment:
#   PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE (standard libpq vars)
#   BACKUP_DIR (default: ./backups/postgres)
#   BACKUP_RETAIN_DAYS (default: 7)
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-./backups/postgres}"
RETAIN_DAYS="${BACKUP_RETAIN_DAYS:-7}"
TIMESTAMP="$(date -u +%Y%m%d_%H%M%S)"
FILENAME="codeforge_${TIMESTAMP}.sql.gz"

mkdir -p "$BACKUP_DIR"

echo "Starting backup of ${PGDATABASE:-codeforge}..."

# pg_dump with custom format (includes compression)
pg_dump \
  --format=custom \
  --compress=6 \
  --file="$BACKUP_DIR/$FILENAME" \
  "${PGDATABASE:-codeforge}"

# Verify dump is valid by listing its TOC
pg_restore --list "$BACKUP_DIR/$FILENAME" > /dev/null

SIZE="$(du -h "$BACKUP_DIR/$FILENAME" | cut -f1)"
echo "Backup complete: $BACKUP_DIR/$FILENAME ($SIZE)"

# Retention cleanup
if [[ "${1:-}" == "--cleanup" ]]; then
  DELETED=$(find "$BACKUP_DIR" -name "codeforge_*.sql.gz" -mtime +"$RETAIN_DAYS" -print -delete | wc -l)
  echo "Retention: removed $DELETED backups older than $RETAIN_DAYS days"
fi
