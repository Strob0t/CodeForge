#!/usr/bin/env bash
# scripts/check-tool-isolation.sh -- check agent tool isolation (KI-71, KI-96) in a container
#
# Usage: ./scripts/check-tool-isolation.sh [image]
#
# Runs workers/tests/tool_isolation_check.py through the worker image's
# entrypoint (scripts/worker-entrypoint.sh) with the worker's settings from
# docker-compose.prod.yml: started as root with only SETUID, SETGID and KILL,
# no-new-privileges, a read-only root, /run/secrets as a tmpfs only the worker
# user may enter with a bind-mounted 0644 secret inside (as Compose mounts
# them), a setgid workspace root of the workspace group and the tool HOME
# base (both with POSIX ACLs; the HOME base not noexec). It sets up two
# tenants (tool uids 20000 and 20001), prints the credentials of their tool
# processes and what they could reach of the worker and of each other; exits
# non-zero when a check fails. The image needs Python 3.12 and setpriv (default
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
    --tmpfs /data/workspaces:uid=10001,gid=10010,mode=2771 \
    --tmpfs /home/codeforge-tools:uid=10001,gid=10001,mode=0711,exec \
    -v "$TMP/codeforge-internal-key:/run/secrets/codeforge-internal-key:ro" \
    -v "$ROOT/workers:/app/workers:ro" \
    -v "$ROOT/scripts/worker-entrypoint.sh:/app/scripts/worker-entrypoint.sh:ro" \
    -e PYTHONPATH=/app/workers \
    -e PYTHONDONTWRITEBYTECODE=1 \
    -w /app/workers \
    --entrypoint /app/scripts/worker-entrypoint.sh \
    "$IMAGE" \
    python -m tests.tool_isolation_check /data/workspaces /home/codeforge-tools /run/secrets/codeforge-internal-key
