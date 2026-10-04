#!/usr/bin/env bash
# scripts/cleanup-wal-archives.sh
# Remove PostgreSQL WAL archives older than RETENTION_DAYS (default: 7)
set -euo pipefail

RETENTION_DAYS="${1:-7}"
ARCHIVE_DIR="${ARCHIVE_DIR:-/archive}"

if [ ! -d "$ARCHIVE_DIR" ]; then
    echo "Archive directory $ARCHIVE_DIR not found"
    exit 0
fi

COUNT=$(find "$ARCHIVE_DIR" -name "*.backup" -o -name "0000*" -mtime +"$RETENTION_DAYS" | wc -l)
if [ "$COUNT" -gt 0 ]; then
    find "$ARCHIVE_DIR" -name "*.backup" -o -name "0000*" -mtime +"$RETENTION_DAYS" -delete
    echo "Cleaned up $COUNT WAL archive files older than $RETENTION_DAYS days"
else
    echo "No WAL archives older than $RETENTION_DAYS days"
fi
