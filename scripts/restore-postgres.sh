#!/usr/bin/env bash
# CodeForge PostgreSQL Restore Script
# Usage:
#   ./scripts/restore-postgres.sh <backup-file>
#   ./scripts/restore-postgres.sh latest
#
# Stop the services that use the database first and start them again after
# the restore; the script refuses to run while other sessions are connected:
#   docker compose -f docker-compose.prod.yml stop core worker litellm
#
# Encrypted backups (*.sql.gz.gpg, see backup-postgres.sh) are decrypted with
# BACKUP_ENCRYPTION_KEY_FILE into a private temporary file (TMPDIR), which is
# removed afterwards. Nothing is dropped unless pg_restore can read the
# whole backup, data included.
#
# Environment:
#   PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE (standard libpq vars)
#   BACKUP_DIR (default: ./backups/postgres)
#   BACKUP_ENCRYPTION_KEY_FILE (the backup script's passphrase file; for .gpg)
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-./backups/postgres}"
TARGET="${1:?Usage: $0 <backup-file|latest>}"
DB="${PGDATABASE:-codeforge}"

# Validate DB name: only allow alphanumeric, underscore, hyphen.
if [[ ! "$DB" =~ ^[a-zA-Z0-9_-]+$ ]]; then
  echo "ERROR: Invalid database name: $DB (must be alphanumeric/underscore/hyphen only)"
  exit 1
fi

if [[ "$TARGET" == "latest" ]]; then
  # -r: without backups, ls must not run (it would list the current directory).
  TARGET="$(find "$BACKUP_DIR" -type f \( -name 'codeforge_*.sql.gz' -o -name 'codeforge_*.sql.gz.gpg' \) -print0 \
    | xargs -0 -r ls -t | sed -n 1p)"
  if [[ -z "$TARGET" ]]; then
    echo "No backups found in $BACKUP_DIR"
    exit 1
  fi
fi

if [[ ! -f "$TARGET" ]]; then
  echo "Backup file not found: $TARGET"
  exit 1
fi

DUMP="$TARGET"
if [[ "$TARGET" == *.gpg ]]; then
  if [[ -z "${BACKUP_ENCRYPTION_KEY_FILE:-}" || ! -r "$BACKUP_ENCRYPTION_KEY_FILE" ]]; then
    echo "ERROR: $TARGET is encrypted: set BACKUP_ENCRYPTION_KEY_FILE to the backup's passphrase file." >&2
    exit 1
  fi
  DUMP="$(mktemp)"
  trap 'rm -f "$DUMP"' EXIT
  if ! gpg --batch --yes --quiet --passphrase-file "$BACKUP_ENCRYPTION_KEY_FILE" \
      --output "$DUMP" --decrypt "$TARGET"; then
    echo "ERROR: cannot decrypt $TARGET with $BACKUP_ENCRYPTION_KEY_FILE; nothing was changed." >&2
    exit 1
  fi
fi

# The backup must be a pg_dump archive (backup-postgres.sh writes the custom
# format) that pg_restore reads to the end before anything is dropped:
# --list reads only the table of contents and passes a truncated archive.
if ! pg_restore --file=/dev/null "$DUMP"; then
  echo "ERROR: pg_restore cannot read all of $TARGET (not a pg_dump archive, or damaged); nothing was changed." >&2
  exit 1
fi

# other_sessions: print "<count>|<clients>" for the sessions on $DB other
# than this one. psql substitutes :'dbname' (quoted) only in SQL from stdin.
other_sessions() {
  psql -X -q -At -d postgres -v ON_ERROR_STOP=1 -v dbname="$DB" <<'SQL'
SELECT count(*) || '|' || coalesce(string_agg(DISTINCT coalesce(nullif(application_name, ''), host(client_addr), 'local'), ', '), '')
FROM pg_stat_activity
WHERE datname = :'dbname' AND pid <> pg_backend_pid();
SQL
}

# A running core, worker or LiteLLM reconnects at once and would work on (or
# migrate) the empty database while pg_restore fills it.
sessions="$(other_sessions)"
if [[ "${sessions%%|*}" != "0" ]]; then
  echo "ERROR: ${sessions%%|*} session(s) are connected to $DB (${sessions#*|})." >&2
  echo "Stop the services that use it and start them after the restore, e.g.:" >&2
  echo "  docker compose -f docker-compose.prod.yml stop core worker litellm" >&2
  exit 1
fi

echo "Restoring from: $TARGET"
echo "Target database: $DB"
echo "WARNING: This will DROP and recreate the database."
read -r -p "Continue? [y/N] " confirm
[[ "$confirm" =~ ^[Yy]$ ]] || exit 0

# --force (PostgreSQL 13+) also ends sessions opened since the check above;
# errors abort the restore (set -e).
dropdb --if-exists --force "$DB"
createdb "$DB"

sessions="$(other_sessions)"
if [[ "${sessions%%|*}" != "0" ]]; then
  echo "ERROR: a client connected to the new, empty $DB (${sessions#*|}); nothing was restored." >&2
  echo "Stop it and run the restore again." >&2
  exit 1
fi

pg_restore \
  --dbname="$DB" \
  --no-owner \
  --no-privileges \
  "$DUMP"

echo "Restore complete."
