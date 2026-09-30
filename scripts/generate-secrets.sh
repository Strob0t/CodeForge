#!/usr/bin/env bash
# scripts/generate-secrets.sh -- Generate the secret files for docker-compose.prod.yml
#
# Usage: ./scripts/generate-secrets.sh [secrets_dir]
# Default dir: SECRETS_DIR from the environment or .env (as compose reads it),
# else ./secrets next to docker-compose.prod.yml.
#
# Missing secrets are created, existing files are never overwritten. Values
# are hex, so they are safe inside connection URLs. database-url, nats-url and
# nats-auth.conf are derived from the other files: they are written when
# missing or when their inputs were just generated, otherwise kept (so edits
# such as sslmode=verify-full survive); delete one to rebuild it.
#
# POSTGRES_USER and POSTGRES_DB are read like compose reads them (environment,
# then .env, then "codeforge").
#
# Rotation (then recreate the services: docker compose up -d --force-recreate):
#   codeforge-internal-key, nats-user, nats-pass: delete the file and re-run;
#     nats-url and nats-auth.conf follow automatically.
#   postgres-password: do not delete it. The postgres image applies it only when
#     it initializes an empty volume. Change the password in the database
#     (ALTER USER ... PASSWORD '...'), then write it into postgres-password and
#     database-url yourself.
#   codeforge-auth-jwt-secret: rotating logs out every user and makes stored VCS
#     account tokens unreadable (they are encrypted with a key derived from it).
#   codeforge-auth-llm-key-encryption-secret: rotating makes stored LLM keys
#     unreadable.
#   litellm-master-key: LiteLLM also encrypts the model credentials it stores
#     with it (unless LITELLM_SALT_KEY is set).
#   This script never replaces these four once the secrets directory is in use
#   (any file only this script creates exists: a derived file, the JWT or LLM key
#   secret, the TLS pair); it stops and explains instead. A directory holding
#   only files you pre-seeded (e.g. your own postgres-password) counts as new.
set -euo pipefail

# shellcheck source=scripts/lib/compose-env.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/compose-env.sh"

if [ $# -ge 1 ]; then
    SECRETS_DIR="$1"
else
    SECRETS_DIR="$(compose_path "$(compose_var SECRETS_DIR ./secrets)")"
fi
PG_USER="$(compose_var POSTGRES_USER codeforge)"
PG_DB="$(compose_var POSTGRES_DB codeforge)"

# URL userinfo and path characters that need no escaping.
URL_SAFE='^[A-Za-z0-9._~-]+$'

if [[ ! "$PG_USER" =~ $URL_SAFE ]] || [[ ! "$PG_DB" =~ $URL_SAFE ]]; then
    echo "ERROR: POSTGRES_USER and POSTGRES_DB must match $URL_SAFE" >&2
    exit 1
fi

# Compose bind-mounts file secrets with their host owner and mode (uid/gid/mode
# only work in Swarm), and core, worker and nats do not run as the host user.
# The files are therefore world-readable (0644) inside a private (0700)
# directory, which keeps other host users out.
umask 077
mkdir -p "$SECRETS_DIR"
chmod 700 "$SECRETS_DIR"

# A secrets directory is in use once any file exists that only this script
# creates. Deleting one of them (e.g. database-url to rebuild it) must not make
# the directory look new: that would regenerate secrets data depends on.
IN_USE=false
for marker in database-url nats-url nats-auth.conf codeforge-auth-jwt-secret \
    codeforge-auth-llm-key-encryption-secret postgres-tls.crt postgres-tls.key; do
    [ -f "$SECRETS_DIR/$marker" ] && IN_USE=true
done

write_file() {
    local file="$SECRETS_DIR/$1"
    printf '%s' "$2" > "$file"
    chmod 644 "$file"
}

# generate <name> <value>: write a random secret unless the file exists.
# Returns 0 when it wrote the file, 1 when it kept an existing one.
generate() {
    local name="$1"
    if [ -f "$SECRETS_DIR/$name" ]; then
        chmod 644 "$SECRETS_DIR/$name"
        echo "Secret $name already exists, skipping"
        return 1
    fi
    write_file "$name" "$2"
    echo "Generated $name"
}

# generate_kept <name> <value> <why>: like generate, but for secrets that must
# not change once data depends on them.
generate_kept() {
    local name="$1"
    if [ ! -f "$SECRETS_DIR/$name" ] && [ "$IN_USE" = true ]; then
        echo "ERROR: $SECRETS_DIR/$name is missing but this secrets directory is in use." >&2
        echo "  $3" >&2
        echo "  Restore the file from your backup. To replace it deliberately, write the" >&2
        echo "  new value into the file yourself; this script will not." >&2
        exit 1
    fi
    generate "$1" "$2" || true
}

# read_url_safe <name>: print a secret that is embedded in a URL or config.
read_url_safe() {
    local value
    value="$(< "$SECRETS_DIR/$1")"
    if [[ ! "$value" =~ $URL_SAFE ]]; then
        echo "ERROR: $SECRETS_DIR/$1 contains characters that are not URL-safe" \
            "(older versions of this script wrote base64); delete it and re-run" >&2
        exit 1
    fi
    printf '%s' "$value"
}

generate_kept postgres-password "$(openssl rand -hex 32)" \
    "The postgres image applies it only when it initializes an empty volume; the database keeps the old password."

NATS_GENERATED=false
generate nats-user "codeforge-$(openssl rand -hex 4)" && NATS_GENERATED=true
generate nats-pass "$(openssl rand -hex 32)" && NATS_GENERATED=true

generate_kept litellm-master-key "sk-$(openssl rand -hex 32)" \
    "LiteLLM encrypts the model credentials it stores with it (unless LITELLM_SALT_KEY is set)."
generate_kept codeforge-auth-jwt-secret "$(openssl rand -hex 32)" \
    "A new JWT secret logs out every user and makes stored VCS account tokens unreadable."
generate codeforge-internal-key "$(openssl rand -hex 32)" || true

LLM_KEY=codeforge-auth-llm-key-encryption-secret
if [ ! -f "$SECRETS_DIR/$LLM_KEY" ] && [ "$IN_USE" = true ]; then
    # Existing installations encrypted stored LLM keys with a key derived from
    # the JWT secret (the core's fallback); the same value keeps them readable.
    write_file "$LLM_KEY" "$(< "$SECRETS_DIR/codeforge-auth-jwt-secret")"
    echo "Created $LLM_KEY from codeforge-auth-jwt-secret (keeps stored LLM keys readable)"
else
    generate "$LLM_KEY" "$(openssl rand -hex 32)" || true
fi

# Self-signed server certificate for PostgreSQL TLS. The clients use
# sslmode=require (encrypted, not verified); replace both files with a
# CA-signed pair and switch the DSNs to sslmode=verify-full to verify it.
if [ -f "$SECRETS_DIR/postgres-tls.crt" ] && [ -f "$SECRETS_DIR/postgres-tls.key" ]; then
    chmod 644 "$SECRETS_DIR/postgres-tls.crt" "$SECRETS_DIR/postgres-tls.key"
    echo "Secret postgres-tls.crt/.key already exist, skipping"
else
    openssl req -x509 -newkey rsa:3072 -nodes -days 3650 -subj "/CN=postgres" \
        -addext "subjectAltName=DNS:postgres" \
        -keyout "$SECRETS_DIR/postgres-tls.key" -out "$SECRETS_DIR/postgres-tls.crt" 2> /dev/null
    chmod 644 "$SECRETS_DIR/postgres-tls.crt" "$SECRETS_DIR/postgres-tls.key"
    echo "Generated postgres-tls.crt/.key (self-signed, CN=postgres)"
fi

# keep_note <name> <inputs>: explain why a derived file was left alone.
keep_note() {
    echo "Kept existing $1 (delete it to rebuild it from $2)"
}

if [ ! -f "$SECRETS_DIR/database-url" ]; then
    PG_PASS="$(read_url_safe postgres-password)"
    write_file database-url "postgresql://${PG_USER}:${PG_PASS}@postgres:5432/${PG_DB}?sslmode=require"
    echo "Wrote database-url"
else
    keep_note database-url "postgres-password, POSTGRES_USER and POSTGRES_DB"
fi

if [ "$NATS_GENERATED" = true ] || [ ! -f "$SECRETS_DIR/nats-url" ] || [ ! -f "$SECRETS_DIR/nats-auth.conf" ]; then
    NATS_USER="$(read_url_safe nats-user)"
    NATS_PASS="$(read_url_safe nats-pass)"
    write_file nats-url "nats://${NATS_USER}:${NATS_PASS}@nats:4222"
    write_file nats-auth.conf "# Generated by scripts/generate-secrets.sh from nats-user and nats-pass
authorization {
  user: \"${NATS_USER}\"
  password: \"${NATS_PASS}\"
}
"
    echo "Wrote nats-url and nats-auth.conf"
else
    keep_note "nats-url and nats-auth.conf" "nats-user and nats-pass"
fi

echo ""
echo "Secrets generated in $SECRETS_DIR"
echo "Keep $SECRETS_DIR out of git and out of image build contexts."
