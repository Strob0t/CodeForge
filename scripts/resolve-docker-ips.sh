#!/usr/bin/env bash
# resolve-docker-ips.sh — Resolve Docker container IPs for WSL2 environments.
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

set -euo pipefail

resolve_ip() {
    local name="$1"
    local ip
    ip=$(docker inspect "$name" 2>/dev/null | grep -m1 '"IPAddress"' | grep -oP '[\d.]+' || true)
    if [ -z "$ip" ]; then
        echo "ERROR: Container '$name' not found or not running" >&2
        return 1
    fi
    echo "$ip"
}

NATS_IP=$(resolve_ip "codeforge-nats")
LITELLM_IP=$(resolve_ip "codeforge-litellm")
POSTGRES_IP=$(resolve_ip "codeforge-postgres")

export NATS_URL="nats://${NATS_IP}:4222"
export LITELLM_BASE_URL="http://${LITELLM_IP}:4000"
export LITELLM_MASTER_KEY="${LITELLM_MASTER_KEY:-sk-codeforge-dev}"
export DATABASE_URL="postgresql://codeforge:codeforge_dev@${POSTGRES_IP}:5432/codeforge"
export PYTHONPATH="${PYTHONPATH:-/workspaces/CodeForge/workers}"
export APP_ENV="${APP_ENV:-development}"

echo "Docker container IPs resolved:"
echo "  NATS:     ${NATS_IP}:4222"
echo "  LiteLLM:  ${LITELLM_IP}:4000"
echo "  Postgres:  ${POSTGRES_IP}:5432"
echo ""
echo "Environment variables exported:"
echo "  NATS_URL=$NATS_URL"
echo "  LITELLM_BASE_URL=$LITELLM_BASE_URL"
echo "  DATABASE_URL=postgresql://codeforge:***@${POSTGRES_IP}:5432/codeforge"
echo "  PYTHONPATH=$PYTHONPATH"
echo "  APP_ENV=$APP_ENV"
