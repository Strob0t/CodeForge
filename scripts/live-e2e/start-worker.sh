#!/usr/bin/env bash
# Starts the Python worker. The Go Core must run first: it creates the NATS
# stream the worker consumes from. LIVE_WORKER_PYTHON overrides the
# interpreter (default: the Poetry environment of workers/).
set -euo pipefail
# shellcheck source=scripts/live-e2e/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

curl -sf -o /dev/null "$CODEFORGE_CORE_URL/health" || live_die "the Go Core is not running; run start-core.sh first"

if [ -z "${LIVE_WORKER_PYTHON:-}" ]; then
  venv=$(cd "$LIVE_REPO_ROOT/workers" && poetry env info -p 2>/dev/null) ||
    live_die "no Poetry environment in workers/; run: cd workers && poetry install"
  LIVE_WORKER_PYTHON=$venv/bin/python
fi

# Tool processes get what the worker image gives them (Dockerfile.worker): a
# Python 3.12 interpreter with pytest and ruff, first on the tool PATH.
if [ ! -x "$LIVE_TOOL_VENV/bin/pytest" ]; then
  live_log "creating the tool environment $LIVE_TOOL_VENV"
  "$LIVE_TOOL_PYTHON" -m venv "$LIVE_TOOL_VENV"
  "$LIVE_TOOL_VENV/bin/python" -m pip install -q --disable-pip-version-check --require-hashes \
    -r "$LIVE_REPO_ROOT/workers/tool-requirements.txt"
fi
export CODEFORGE_TOOL_PATH=${CODEFORGE_TOOL_PATH:-$LIVE_TOOL_VENV/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin}
# Without isolation, tool processes inherit the worker's PATH instead.
if [ "$CODEFORGE_TOOL_ISOLATION" = off ]; then
  export PATH=$LIVE_TOOL_VENV/bin:$PATH
fi

export PYTHONPATH=$LIVE_REPO_ROOT/workers
live_start worker "$LIVE_REPO_ROOT/workers" "$LIVE_WORKER_PYTHON" -m codeforge.consumer
live_wait_http worker "http://127.0.0.1:$CODEFORGE_WORKER_HEALTH_PORT/health/ready" 120
