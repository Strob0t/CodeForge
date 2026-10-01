#!/bin/sh
# Entrypoint of the worker image (KI-71).
#
# docker-compose.prod.yml starts the worker container as root with only
# CAP_SETUID, CAP_SETGID and CAP_KILL. This runs the command as the worker
# user (uid 10001, supplementary group codeforge-ws 10010) and keeps those
# three capabilities as ambient capabilities: the worker needs them to start
# agent tool processes as the tool user (uid 10002, codeforge.tool_process)
# and to stop them. The worker itself never runs as root.
#
# Started as another user, the command runs as that user; tool processes then
# cannot be isolated and, with CODEFORGE_TOOL_ISOLATION=required, every tool
# call fails instead of running as the worker user.
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
