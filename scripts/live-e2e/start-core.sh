#!/usr/bin/env bash
# Builds and starts the Go Core (HTTP on $CODEFORGE_PORT). Set
# LIVE_SKIP_BUILD=1 to reuse the binary in $LIVE_DIR/bin.
set -euo pipefail
# shellcheck source=scripts/live-e2e/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

live_ensure_db

if [ "${LIVE_SKIP_BUILD:-0}" != 1 ]; then
  pkg=github.com/Strob0t/CodeForge/internal/version
  ldflags="-X $pkg.Version=$(cat "$LIVE_REPO_ROOT/VERSION") -X $pkg.GitSHA=$(git -C "$LIVE_REPO_ROOT" rev-parse --short HEAD)"
  live_log "building the Go Core"
  (cd "$LIVE_REPO_ROOT" && go build -ldflags "$ldflags" -o "$LIVE_DIR/bin/codeforge" ./cmd/codeforge)
fi

live_start core "$LIVE_REPO_ROOT" "$LIVE_DIR/bin/codeforge"
live_wait_http core "$CODEFORGE_CORE_URL/health" 120
