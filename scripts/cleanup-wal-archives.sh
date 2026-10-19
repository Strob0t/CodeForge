#!/bin/sh
# scripts/cleanup-wal-archives.sh
# Remove PostgreSQL WAL archive files older than RETENTION_DAYS (default: 7).
#
# Only needed with WAL archiving on (POSTGRES_ARCHIVE_MODE=on in
# docker-compose.prod.yml). The archive is the postgres_archive volume, mounted
# at /archive in the postgres container only; the script is POSIX sh, so it
# runs there (the image has no bash), e.g. daily from the host's cron:
#   docker compose -f docker-compose.prod.yml exec -T postgres \
#     sh -s -- 7 < scripts/cleanup-wal-archives.sh
# Archived WAL is replayed on top of a base backup: keep it at least back to
# the oldest base backup you would restore.
set -eu

RETENTION_DAYS="${1:-7}"
ARCHIVE_DIR="${ARCHIVE_DIR:-/archive}"

case "$RETENTION_DAYS" in
    '' | *[!0-9]*)
        echo "RETENTION_DAYS must be a whole number of days (got '$RETENTION_DAYS')" >&2
        exit 2
        ;;
esac

if [ ! -d "$ARCHIVE_DIR" ]; then
    echo "Archive directory $ARCHIVE_DIR not found"
    exit 0
fi

# The name tests are grouped: without \( \) the age test and -delete would
# apply to the last -name only.
DELETED=$(find "$ARCHIVE_DIR" -type f \( -name '*.backup' -o -name '0000*' \) \
    -mtime +"$RETENTION_DAYS" -print -delete)
COUNT=$(printf '%s' "$DELETED" | grep -c '' || true)
if [ "$COUNT" -gt 0 ]; then
    echo "Cleaned up $COUNT WAL archive files older than $RETENTION_DAYS days"
else
    echo "No WAL archives older than $RETENTION_DAYS days"
fi
