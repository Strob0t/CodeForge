#!/bin/sh
# Entrypoint of the worker image (KI-71, KI-96).
#
# docker-compose.prod.yml starts the worker container as root with only
# CAP_SETUID, CAP_SETGID and CAP_KILL. This runs the command as the worker
# user (uid 10001, supplementary group codeforge-ws 10010) and keeps those
# three capabilities as ambient capabilities: the worker needs them to start
# agent tool processes as their tenant's tool user (20000-29999, no group,
# under Landlock; codeforge.tool_process, ADR-018) and to stop them. The
# worker itself never runs as root, and per-tenant isolation needs no further
# capability.
#
# Started as another user, the command runs as that user; tool processes then
# cannot be isolated and, with CODEFORGE_TOOL_ISOLATION=required, every tool
# call fails (and /health/ready answers 503) instead of running as the
# worker user.
set -eu

WORKER_UID=10001
WORKER_GID=10001
WORKSPACE_GID=10010

if [ "$(id -u)" = 0 ]; then
    exec setpriv --reuid="$WORKER_UID" --regid="$WORKER_GID" --groups="$WORKSPACE_GID" \
        --inh-caps=-all,+setuid,+setgid,+kill --ambient-caps=-all,+setuid,+setgid,+kill \
        -- "$@"
fi
exec "$@"
