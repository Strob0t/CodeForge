#!/usr/bin/env bash
# Starts the Vite dev server on 127.0.0.1:$LIVE_FRONTEND_PORT. It proxies
# /api, /health and /ws to the Go Core on localhost:8080 (vite.config.ts).
set -euo pipefail
# shellcheck source=scripts/live-e2e/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

[ -d "$LIVE_REPO_ROOT/frontend/node_modules" ] || live_die "no frontend/node_modules; run: cd frontend && npm ci"

live_start frontend "$LIVE_REPO_ROOT/frontend" \
  npm run dev -- --host 127.0.0.1 --port "$LIVE_FRONTEND_PORT" --strictPort
live_wait_http frontend "http://127.0.0.1:$LIVE_FRONTEND_PORT/" 120
