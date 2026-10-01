#!/usr/bin/env bash
# scripts/check-tool-isolation.sh -- check agent tool isolation (KI-71) in a container
#
# Usage: ./scripts/check-tool-isolation.sh [image]
#
# Runs workers/tests/tool_isolation_check.py through the worker image's
# entrypoint (scripts/worker-entrypoint.sh) with the worker's settings from
# docker-compose.prod.yml: started as root with only SETUID, SETGID and KILL,
# no-new-privileges, a read-only root, /run/secrets as a tmpfs only the worker
# user may enter with a bind-mounted 0644 secret inside (as Compose mounts
# them), and a setgid workspace directory of the workspace group. Prints the
# credentials of a tool process and what it could reach; exits non-zero when a
# check fails. The image needs Python 3.12 and setpriv (default
# python:3.12-slim; the worker image works too). The repository's workers/ and
# entrypoint are mounted, so no image build is needed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${1:-python:3.12-slim}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
printf 'internal-admin-key-for-the-check' > "$TMP/codeforge-internal-key"
chmod 644 "$TMP/codeforge-internal-key"

docker run --rm \
    --user 0:0 \
    --cap-drop ALL --cap-add SETUID --cap-add SETGID --cap-add KILL \
    --security-opt no-new-privileges:true \
    --read-only \
    --tmpfs /tmp \
    --tmpfs /run/secrets:uid=10001,gid=10001,mode=0700 \
    --tmpfs /data/workspaces:uid=10001,gid=10010,mode=2775 \
    -v "$TMP/codeforge-internal-key:/run/secrets/codeforge-internal-key:ro" \
    -v "$ROOT/workers:/app/workers:ro" \
    -v "$ROOT/scripts/worker-entrypoint.sh:/app/scripts/worker-entrypoint.sh:ro" \
    -e PYTHONPATH=/app/workers \
    -e PYTHONDONTWRITEBYTECODE=1 \
    -w /app/workers \
    --entrypoint /app/scripts/worker-entrypoint.sh \
    "$IMAGE" \
    python -m tests.tool_isolation_check /data/workspaces /run/secrets/codeforge-internal-key
