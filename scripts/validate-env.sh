#!/usr/bin/env bash
# Validates the production secrets before a deployment.
#
# Every secret is checked under the name the services read. Its value comes
# from, in this order:
#   1. the environment variable (e.g. CODEFORGE_AUTH_JWT_SECRET)
#   2. <VAR>_FILE, the path of a file holding it (read by the Go core)
#   3. the Docker secret file in SECRETS_DIR (default ./secrets) that
#      docker-compose.prod.yml mounts (e.g. codeforge-auth-jwt-secret)
# Values are never printed.
#
# Usage: ./scripts/validate-env.sh
set -euo pipefail

SECRETS_DIR="${SECRETS_DIR:-./secrets}"
INSECURE_PATTERN='codeforge_dev|sk-codeforge-dev|codeforge-internal-dev|codeforge-dev-jwt-secret|e2e-test-secret'

# variable|secret file|minimum length
SECRETS=(
    "POSTGRES_PASSWORD|postgres-password|16"
    "DATABASE_URL|database-url|1"
    "NATS_URL|nats-url|1"
    "LITELLM_MASTER_KEY|litellm-master-key|16"
    "CODEFORGE_AUTH_JWT_SECRET|codeforge-auth-jwt-secret|32"
    "CODEFORGE_INTERNAL_KEY|codeforge-internal-key|16"
)

errors=0
fail() {
    echo "ERROR: $*" >&2
    errors=$((errors + 1))
}

# resolve <var> <file>: print the value and set SOURCE, or return 1.
resolve() {
    local var="$1" file="$2" file_var="${1}_FILE"
    local env_value="${!var:-}" file_path="${!file_var:-}"
    if [ -n "$env_value" ] && [ -n "$file_path" ]; then
        fail "both $var and $file_var are set, set only one"
        return 1
    fi
    if [ -n "$env_value" ]; then
        SOURCE="$var"
        VALUE="$env_value"
        return 0
    fi
    if [ -z "$file_path" ] && [ -f "$SECRETS_DIR/$file" ]; then
        file_path="$SECRETS_DIR/$file"
        # Compose mounts the host file as is; non-root containers need o+r.
        local mode
        if mode="$(stat -c '%a' "$file_path" 2> /dev/null)" && (((8#$mode & 4) == 0)); then
            fail "$file_path is not readable by the container users (mode $mode), run chmod 644"
        fi
    fi
    if [ -z "$file_path" ]; then
        fail "$var is not set (no $var, $file_var or $SECRETS_DIR/$file)"
        return 1
    fi
    if [ ! -r "$file_path" ]; then
        fail "$var: cannot read $file_path"
        return 1
    fi
    SOURCE="$file_path"
    VALUE="$(< "$file_path")"
    VALUE="${VALUE%$'\n'}"
    VALUE="${VALUE%$'\r'}"
}

for entry in "${SECRETS[@]}"; do
    IFS='|' read -r var file min_len <<< "$entry"
    SOURCE=""
    VALUE=""
    resolve "$var" "$file" || continue
    if [ -z "$VALUE" ]; then
        fail "$var is empty ($SOURCE)"
    elif [ "${#VALUE}" -lt "$min_len" ]; then
        fail "$var is shorter than $min_len characters ($SOURCE)"
    elif echo "$VALUE" | grep -qE "$INSECURE_PATTERN"; then
        fail "$var contains a default/insecure value ($SOURCE)"
    elif [ "$var" = "DATABASE_URL" ] && [[ "$VALUE" == *sslmode=disable* ]]; then
        fail "$var uses sslmode=disable, which the core rejects outside development ($SOURCE)"
    fi
done

if [ "$errors" -gt 0 ]; then
    echo "$errors problem(s) found." >&2
    exit 1
fi
echo "All required secrets validated."
