#!/usr/bin/env bash
# Stops the stack in reverse startup order: stop.sh [frontend] [worker] [core] [litellm]
# (default: all four). PostgreSQL, NATS and Ollama keep running.
set -euo pipefail
# shellcheck source=scripts/live-e2e/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

[ "$#" -gt 0 ] || set -- frontend worker core litellm
for name in "$@"; do
  case $name in
    frontend | worker | core) live_stop "$name" ;;
    litellm)
      docker rm -f "$LIVE_LITELLM_CONTAINER" >/dev/null 2>&1 || true
      live_log "litellm stopped"
      ;;
    *) live_die "unknown component: $name (frontend, worker, core, litellm)" ;;
  esac
done
