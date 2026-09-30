#!/usr/bin/env bash
# resolve-docker-ips.sh - Resolve Docker container IPs for WSL2 environments.
#
# In WSL2, Docker port mappings (0.0.0.0:4000 -> container:4000) are NOT
# reachable via localhost from inside the WSL2 instance. This script resolves
# the actual container IPs and exports the correct environment variables for
# the Python worker and other host-side services.
#
# Usage:
#   source scripts/resolve-docker-ips.sh
#   # Then start the worker:
#   .venv/bin/python -m codeforge.consumer
#
# It is sourced into your interactive shell, so it sets no shell options
# (set -e/-u/-o pipefail would stay active there) and returns instead of
# exiting on errors. Nothing is exported unless all three containers are found.

if ! (return 0 2>/dev/null); then
    echo "ERROR: run it with 'source ${BASH_SOURCE[0]}'; a child process cannot export to your shell" >&2
    exit 1
fi

_codeforge_container_ip() {
    docker inspect "$1" 2>/dev/null | grep -m1 '"IPAddress"' | grep -oP '[\d.]+'
}

_codeforge_resolve_docker_ips() {
    local nats_ip litellm_ip postgres_ip missing=""
    nats_ip=$(_codeforge_container_ip codeforge-nats) || missing="$missing codeforge-nats"
    litellm_ip=$(_codeforge_container_ip codeforge-litellm) || missing="$missing codeforge-litellm"
    postgres_ip=$(_codeforge_container_ip codeforge-postgres) || missing="$missing codeforge-postgres"
    unset -f _codeforge_container_ip _codeforge_resolve_docker_ips

    if [ -n "$missing" ]; then
        echo "ERROR: container(s) not found or not running:${missing}; nothing exported" >&2
        return 1
    fi

    export NATS_URL="nats://${nats_ip}:4222"
    export LITELLM_BASE_URL="http://${litellm_ip}:4000"
    export LITELLM_MASTER_KEY="${LITELLM_MASTER_KEY:-sk-codeforge-dev}"
    export DATABASE_URL="postgresql://codeforge:codeforge_dev@${postgres_ip}:5432/codeforge"
    export PYTHONPATH="${PYTHONPATH:-/workspaces/CodeForge/workers}"
    export APP_ENV="${APP_ENV:-development}"

    echo "Docker container IPs resolved:"
    echo "  NATS:     ${nats_ip}:4222"
    echo "  LiteLLM:  ${litellm_ip}:4000"
    echo "  Postgres: ${postgres_ip}:5432"
    echo ""
    echo "Environment variables exported:"
    echo "  NATS_URL=$NATS_URL"
    echo "  LITELLM_BASE_URL=$LITELLM_BASE_URL"
    echo "  DATABASE_URL=postgresql://codeforge:***@${postgres_ip}:5432/codeforge"
    echo "  PYTHONPATH=$PYTHONPATH"
    echo "  APP_ENV=$APP_ENV"
}

_codeforge_resolve_docker_ips
