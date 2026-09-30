# shellcheck shell=bash
# Resolve variables the way docker compose interpolates docker-compose.prod.yml:
# a non-empty shell variable wins over the .env file next to the compose file,
# which wins over the default. Sourced by generate-secrets.sh and
# validate-env.sh so both see the values the deployment really uses.

COMPOSE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_ENV_FILE="${COMPOSE_ENV_FILE:-$COMPOSE_ROOT/.env}"

# compose_var <name> <default>: print the value compose would use for ${name:-default}.
compose_var() {
    local name="$1" default="$2" value=""
    if [ -n "${!name:-}" ]; then
        value="${!name}"
    elif [ -f "$COMPOSE_ENV_FILE" ]; then
        value="$(sed -n -E "s/^[[:space:]]*(export[[:space:]]+)?${name}[[:space:]]*=[[:space:]]*(.*)$/\2/p" \
            "$COMPOSE_ENV_FILE" | tail -n 1)"
        case "$value" in
            \"*\") value="${value#\"}" && value="${value%\"}" ;;
            \'*\') value="${value#\'}" && value="${value%\'}" ;;
            *) value="${value%%[[:space:]]#*}" && value="${value%"${value##*[![:space:]]}"}" ;;
        esac
    fi
    printf '%s' "${value:-$default}"
}

# compose_path <path>: resolve a path relative to the compose project directory.
compose_path() {
    case "$1" in
        /*) printf '%s' "$1" ;;
        *) printf '%s/%s' "$COMPOSE_ROOT" "${1#./}" ;;
    esac
}
