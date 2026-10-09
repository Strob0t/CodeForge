#!/usr/bin/env bash
# scripts/check-host.sh -- preflight of per-tenant tool isolation (KI-96, ADR-018)
#
# Usage: WORKER_IMAGE=<new worker image> ./scripts/check-host.sh [-f <extra compose file>]...
#
# Step 2 of the upgrade to per-tenant tool users, before any worker is
# stopped: runs codeforge.host_check in the worker image with the production
# service definition (`docker compose -f docker-compose.prod.yml run --rm
# --no-deps worker`: the worker's user, capabilities, security options,
# read-only root, volumes and tmpfs mounts) against the deployment's real
# workspaces and tool_homes volumes. It prints the host's kernel, LSM list
# and Docker version, then what the worker sees:
#
#   - the Landlock ABI (ENOSYS: the kernel has none or a seccomp profile
#     blocks it, Docker before 23.0; EOPNOTSUPP: not in the lsm= list),
#   - POSIX ACLs on both volumes, their file systems and mount options
#     (tool_homes must not be noexec),
#   - the mode of /tmp (1771) and the worker's own isolation check.
#
# Exits non-zero when anything fails; the worker would then answer
# /health/ready with 503 and run no tool. The check prepares the volumes as
# the new worker's start does (the root's mode 2771, the state directory
# <root>/.codeforge); a worker of the older image keeps working with them.
#
# Environment: WORKER_IMAGE (the image docker-compose.prod.yml runs, default
# ghcr.io/strob0t/codeforge-worker:<VERSION>), SECRETS_DIR and
# COMPOSE_PROJECT_NAME as for the deployment. Extra arguments are compose
# options placed before `run` (for example -f docker-compose.blue-green.yml).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "host kernel: $(uname -r)"
if [ -r /sys/kernel/security/lsm ]; then
    echo "host LSMs: $(cat /sys/kernel/security/lsm)"
else
    echo "host LSMs: unknown (/sys/kernel/security/lsm is not readable here)"
fi
echo "docker engine: $(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)"
echo "worker image: ${WORKER_IMAGE:-ghcr.io/strob0t/codeforge-worker:$(tr -d '[:space:]' < VERSION)}"

exec docker compose -f docker-compose.prod.yml "$@" \
    run --rm --no-deps -T worker python -m codeforge.host_check
