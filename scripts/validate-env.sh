#!/usr/bin/env bash
# Validates the production secrets before a deployment of docker-compose.prod.yml.
#
# Checks the secret files the compose file mounts (SECRETS_DIR from the
# environment or .env, default ./secrets next to the compose file) and the
# compose variables that must agree with them (POSTGRES_USER, POSTGRES_DB).
# The services never read secret environment variables, so none are checked.
# Values are never printed.
#
# Usage: ./scripts/validate-env.sh
set -euo pipefail

# shellcheck source=scripts/lib/compose-env.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/compose-env.sh"

SECRETS_DIR="$(compose_path "$(compose_var SECRETS_DIR ./secrets)")"
PG_USER="$(compose_var POSTGRES_USER codeforge)"
PG_DB="$(compose_var POSTGRES_DB codeforge)"
INSECURE_PATTERN='codeforge_dev|sk-codeforge-dev|codeforge-internal-dev|codeforge-dev-jwt-secret|e2e-test-secret'

# secret file|minimum length
SECRET_FILES=(
    "postgres-password|16"
    "database-url|1"
    "nats-core-url|1"
    "nats-worker-url|1"
    "nats-passwords.conf|1"
    "litellm-master-key|16"
    "codeforge-auth-jwt-secret|32"
    "codeforge-auth-llm-key-encryption-secret|32"
    "codeforge-internal-key|16"
    "postgres-tls.crt|1"
    "postgres-tls.key|1"
)

errors=0
fail() {
    echo "ERROR: $*" >&2
    errors=$((errors + 1))
}
warn() {
    echo "WARNING: $*" >&2
}

# read_secret <name>: print the trimmed content of a secret file.
read_secret() {
    local value
    value="$(< "$SECRETS_DIR/$1")"
    printf '%s' "${value%$'\r'}"
}

for entry in "${SECRET_FILES[@]}"; do
    IFS='|' read -r name min_len <<< "$entry"
    file="$SECRETS_DIR/$name"
    if [ ! -f "$file" ]; then
        fail "$file is missing (run ./scripts/generate-secrets.sh)"
        continue
    fi
    if [ ! -r "$file" ]; then
        fail "cannot read $file"
        continue
    fi
    # Compose mounts the host file as is; the non-root containers need o+r.
    if mode="$(stat -c '%a' "$file" 2> /dev/null)" && (((8#$mode & 4) == 0)); then
        fail "$file is not readable by the container users (mode $mode), run chmod 644"
    fi
    value="$(read_secret "$name")"
    if [ -z "$value" ]; then
        fail "$file is empty"
    elif [ "${#value}" -lt "$min_len" ]; then
        fail "$file is shorter than $min_len characters"
    elif echo "$value" | grep -qE "$INSECURE_PATTERN"; then
        fail "$file contains a default/insecure value"
    fi
done

# database-url must match the user and database the postgres service creates.
if [ -f "$SECRETS_DIR/database-url" ]; then
    url="$(read_secret database-url)"
    if [[ "$url" =~ ^postgres(ql)?://([^:@/]+):([^@/]*)@([^/]+)/([^?]+)(\?(.*))?$ ]]; then
        url_user="${BASH_REMATCH[2]}"
        url_pass="${BASH_REMATCH[3]}"
        url_db="${BASH_REMATCH[5]}"
        url_query="${BASH_REMATCH[7]}"
        if [ "$url_user" != "$PG_USER" ]; then
            fail "database-url connects as '$url_user' but POSTGRES_USER is '$PG_USER'"
        fi
        if [ "$url_db" != "$PG_DB" ]; then
            fail "database-url uses database '$url_db' but POSTGRES_DB is '$PG_DB'"
        fi
        if [[ "&$url_query&" == *"&sslmode=disable&"* ]]; then
            fail "database-url uses sslmode=disable, which the core rejects outside development"
        fi
        if [ -f "$SECRETS_DIR/postgres-password" ] && [ "$url_pass" != "$(read_secret postgres-password)" ]; then
            warn "database-url and postgres-password hold different passwords" \
                "(fine only if you changed the password in the database after the first start)"
        fi
    else
        fail "database-url is not a postgresql://user:password@host/database URL"
    fi
fi

# The NATS URLs must carry the users and passwords the NATS server accepts
# (configs/nats/nats-server.conf: users "core" and "worker", passwords from
# nats-passwords.conf), and the two services must not share a password.
if [ -f "$SECRETS_DIR/nats-passwords.conf" ]; then
    conf="$(< "$SECRETS_DIR/nats-passwords.conf")"
    declare -A nats_passwords=()
    for entry in "nats-core-url|core|CORE_PASSWORD" "nats-worker-url|worker|WORKER_PASSWORD"; do
        IFS='|' read -r name user variable <<< "$entry"
        [ -f "$SECRETS_DIR/$name" ] || continue
        url="$(read_secret "$name")"
        if [[ "$url" =~ ^nats://([^:@/]+):([^@/]*)@ ]]; then
            if [ "${BASH_REMATCH[1]}" != "$user" ]; then
                fail "$name connects as '${BASH_REMATCH[1]}', expected the NATS user '$user'"
            fi
            nats_passwords[$user]="${BASH_REMATCH[2]}"
            if [[ "$conf" != *"${variable}: \"${BASH_REMATCH[2]}\""* ]]; then
                fail "$name and nats-passwords.conf hold different passwords for '$user'"
            fi
        else
            fail "$name is not a nats://user:password@host URL"
        fi
    done
    if [ -n "${nats_passwords[core]:-}" ] && [ "${nats_passwords[core]:-}" = "${nats_passwords[worker]:-}" ]; then
        fail "nats-core-url and nats-worker-url use the same password"
    fi
fi

if [ "$errors" -gt 0 ]; then
    echo "$errors problem(s) found in $SECRETS_DIR." >&2
    exit 1
fi
echo "All secrets in $SECRETS_DIR validated."
