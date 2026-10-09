#!/usr/bin/env bash
# scripts/check-tool-isolation.sh -- check agent tool isolation (KI-71, KI-96) in the worker image
#
# Usage: ./scripts/check-tool-isolation.sh [image]
#
# Runs workers/tests/tool_isolation_check.py in the built worker image
# (default: $WORKER_IMAGE, else the image docker-compose.prod.yml runs)
# through its entrypoint, with the worker's settings from
# docker-compose.prod.yml: started as root with only SETUID, SETGID and KILL,
# no-new-privileges, a read-only root, /tmp 1771, /run/secrets as a tmpfs
# only the worker user may enter with a bind-mounted 0644 secret inside (as
# Compose mounts them), and fresh named volumes for the workspace root and
# the tool HOMEs (initialized by the image: 2771 and 0711, POSIX ACLs, not
# noexec). It sets up two tenants (tool uids 20000 and 20001), prints the
# credentials of their tool processes, what they could reach of the worker,
# of each other and of their own tenant's other project under Landlock, and
# whether a tool command line carries its environment; exits non-zero when a
# check fails. Only the checks (workers/tests) are mounted from this
# checkout; the worker code is the image's. The volumes are removed after.
# The full Docker suite is workers/tests/test_tenant_isolation_docker.py.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${1:-${WORKER_IMAGE:-ghcr.io/strob0t/codeforge-worker:$(tr -d '[:space:]' < "$ROOT/VERSION")}}"
RUN_ID="cf-isolation-check-$$-$(date +%s)"

TMP="$(mktemp -d)"
cleanup() {
    docker volume rm -f "$RUN_ID-workspaces" "$RUN_ID-tool-homes" >/dev/null 2>&1 || true
    rm -rf "$TMP"
}
trap cleanup EXIT
printf 'internal-admin-key-for-the-check' > "$TMP/codeforge-internal-key"
chmod 644 "$TMP/codeforge-internal-key"

docker run --rm \
    --user 0:0 \
    --cap-drop ALL --cap-add SETUID --cap-add SETGID --cap-add KILL \
    --security-opt no-new-privileges:true \
    --read-only \
    --tmpfs /tmp:uid=10001,gid=10010,mode=1771 \
    --tmpfs /run/secrets:uid=10001,gid=10001,mode=0700 \
    -v "$RUN_ID-workspaces:/data/workspaces" \
    -v "$RUN_ID-tool-homes:/home/codeforge-tools" \
    -v "$TMP/codeforge-internal-key:/run/secrets/codeforge-internal-key:ro" \
    -v "$ROOT/workers/tests:/app/workers/tests:ro" \
    -e APP_ENV=production \
    -e PYTHONPATH=/app/workers \
    -e PYTHONDONTWRITEBYTECODE=1 \
    -w /app/workers \
    "$IMAGE" \
    python -m tests.tool_isolation_check /data/workspaces /home/codeforge-tools /run/secrets/codeforge-internal-key
