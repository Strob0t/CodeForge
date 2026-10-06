#!/bin/bash
# SessionStart hook for Claude Code on the web.
# Installs the toolchains and dependencies CI uses, so tests, linters and the
# pre-commit hooks work in a fresh container, and starts PostgreSQL 18 + NATS
# JetStream for the Go database/integration tests (best effort).
# Keep versions in sync with .github/workflows/ci.yml and .devcontainer/setup.sh.
set -euo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

GO_TOOLCHAIN="go1.25.14"      # CI: setup-go "1.25" (latest patch)
GOLANGCI_LINT_VERSION="2.11.4"
# sha256 of golangci-lint-<version>-linux-amd64.tar.gz (the release's checksums file).
GOLANGCI_LINT_SHA256="200c5b7503f67b59a6743ccf32133026c174e272b930ee79aa2aa6f37aca7ef1"
GOIMPORTS_VERSION="v0.42.0"
GOPLS_VERSION="v0.21.1"       # gopls MCP server (.mcp.json); v0.22+ need Go 1.26 to build
PYTHON="python3.12"

cd "${CLAUDE_PROJECT_DIR:-$(pwd)}"
log() { echo "[session-start] $*" >&2; }
env_line() { [ -n "${CLAUDE_ENV_FILE:-}" ] && echo "$1" >> "$CLAUDE_ENV_FILE"; return 0; }

# -- Go ---------------------------------------------------------------------
export GOTOOLCHAIN="$GO_TOOLCHAIN"
GOBIN="$(go env GOPATH)/bin"
mkdir -p "$GOBIN"
log "go $(go version | awk '{print $3}'): downloading modules"
go mod download >&2

if ! "$GOBIN/golangci-lint" version 2>/dev/null | grep -q "version $GOLANGCI_LINT_VERSION "; then
  log "installing golangci-lint $GOLANGCI_LINT_VERSION"
  tmp="$(mktemp -d)"
  curl -sSfL -o "$tmp/golangci-lint.tar.gz" \
    "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz"
  echo "$GOLANGCI_LINT_SHA256  $tmp/golangci-lint.tar.gz" | sha256sum -c - >&2
  tar -xzf "$tmp/golangci-lint.tar.gz" -C "$tmp"
  install -m 0755 "$tmp/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint" "$GOBIN/golangci-lint"
  rm -rf "$tmp"
fi

if ! go version -m "$GOBIN/goimports" 2>/dev/null | grep -q "golang.org/x/tools[[:space:]]*$GOIMPORTS_VERSION"; then
  log "installing goimports $GOIMPORTS_VERSION"
  go install "golang.org/x/tools/cmd/goimports@$GOIMPORTS_VERSION" >&2
fi

# Only the MCP server needs gopls, so a failed install does not stop the hook.
if ! go version -m "$GOBIN/gopls" 2>/dev/null | grep -q "golang.org/x/tools/gopls[[:space:]]*$GOPLS_VERSION"; then
  log "installing gopls $GOPLS_VERSION"
  go install "golang.org/x/tools/gopls@$GOPLS_VERSION" >&2 \
    || log "gopls install failed; the gopls and serena MCP servers will not work"
fi

env_line "export GOTOOLCHAIN=$GO_TOOLCHAIN"
env_line "export PATH=\"$GOBIN:\$PATH\""

# -- Python (Poetry, Python 3.12 like CI) ----------------------------------
log "poetry install ($PYTHON)"
poetry env use "$PYTHON" >&2
poetry install --no-interaction >&2

# -- Frontend ----------------------------------------------------------------
log "npm install (frontend)"
npm install --prefix frontend --no-audit --no-fund >&2

# -- pre-commit ----------------------------------------------------------------
log "pre-commit hooks"
pre-commit install >&2
pre-commit install-hooks >&2

# -- MCP servers (.mcp.json, best effort) ------------------------------------
# Claude Code waits 30 s (MCP_TIMEOUT) for a stdio server to start. Fill the npx
# and uv caches with the packages .mcp.json pins, so a start does not download.
prewarm() {
  local pkg="$1"; shift
  [ -n "$pkg" ] || { log "MCP pre-warm: package not found in .mcp.json"; return 0; }
  log "MCP pre-warm: $pkg"
  "$@" >/dev/null 2>&1 || log "MCP pre-warm failed: $pkg (its first start may time out)"
}
playwright_mcp="$(grep -o '@playwright/mcp@[0-9.]*' .mcp.json || true)"
serena_agent="$(grep -o 'serena-agent==[0-9.]*' .mcp.json || true)"
prewarm "$playwright_mcp" npx -y "$playwright_mcp" --help
if command -v uvx >/dev/null 2>&1; then
  prewarm "$serena_agent" uvx --from "$serena_agent" serena --help
else
  log "uvx not found: the serena MCP server needs uv (https://docs.astral.sh/uv/)"
fi

# -- Test services (best effort) ---------------------------------------------
port_open() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

export_service_env() {
  env_line 'export DATABASE_URL="postgres://codeforge:codeforge_dev@localhost:5432/codeforge?sslmode=prefer"'
  env_line 'export NATS_URL="nats://localhost:4222"'
}

start_services() {
  if port_open 5432 && port_open 4222; then
    log "PostgreSQL and NATS already listen on 5432/4222, reusing them"
    export_service_env
    return 0
  fi
  command -v dockerd >/dev/null 2>&1 || { log "dockerd not available, skipping test services"; return 0; }
  if ! docker info >/dev/null 2>&1; then
    log "starting dockerd"
    nohup dockerd >/tmp/dockerd.log 2>&1 &
    for _ in $(seq 1 30); do docker info >/dev/null 2>&1 && break; sleep 1; done
    docker info >/dev/null 2>&1 || { log "dockerd did not start, skipping test services"; return 0; }
  fi
  ensure_container codeforge-test-postgres \
    -e POSTGRES_USER=codeforge -e POSTGRES_PASSWORD=codeforge_dev -e POSTGRES_DB=codeforge \
    -p 127.0.0.1:5432:5432 postgres:18-alpine || return 0
  ensure_container codeforge-test-nats \
    -p 127.0.0.1:4222:4222 -p 127.0.0.1:8222:8222 nats:2-alpine --jetstream --store_dir /data -m 8222 || return 0
  export_service_env
  log "test services: PostgreSQL 18 on 127.0.0.1:5432, NATS JetStream on 127.0.0.1:4222"
}

ensure_container() {
  local name="$1"; shift
  if docker container inspect "$name" >/dev/null 2>&1; then
    docker start "$name" >/dev/null 2>&1 || { log "could not start $name"; return 1; }
  else
    docker run -d --name "$name" "$@" >/dev/null 2>&1 || { log "could not run $name"; return 1; }
  fi
}

start_services
log "done"
