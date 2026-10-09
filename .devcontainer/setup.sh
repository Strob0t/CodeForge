#!/usr/bin/env bash
set -euo pipefail

echo "=============================================="
echo "  CodeForge Dev Container Setup"
echo "=============================================="

# -- Python: Poetry -------------------------------
echo ""
echo "> Installing Poetry..."
pipx install poetry
poetry config virtualenvs.in-project true

# -- Go: golangci-lint v2 --------------------------
echo ""
echo "> Installing golangci-lint..."
# Keep in sync with .github/workflows/ci.yml and .claude/hooks/session-start.sh.
# The release tarball, checked against the checksums of
# golangci-lint-<version>-checksums.txt (KI-214).
GOLANGCI_LINT_VERSION="2.11.4"
case "$(uname -m)" in
    x86_64) golangci_arch=amd64; golangci_sha256=200c5b7503f67b59a6743ccf32133026c174e272b930ee79aa2aa6f37aca7ef1 ;;
    aarch64 | arm64) golangci_arch=arm64; golangci_sha256=3bcfa2e6f3d32b2bf5cd75eaa876447507025e0303698633f722a05331988db4 ;;
    *) echo "  unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac
golangci_tmp="$(mktemp -d)"
golangci_name="golangci-lint-${GOLANGCI_LINT_VERSION}-linux-${golangci_arch}"
curl -sSfL -o "$golangci_tmp/$golangci_name.tar.gz" \
    "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/$golangci_name.tar.gz"
echo "$golangci_sha256  $golangci_tmp/$golangci_name.tar.gz" | sha256sum -c -
tar -xzf "$golangci_tmp/$golangci_name.tar.gz" -C "$golangci_tmp"
install -m 0755 "$golangci_tmp/$golangci_name/golangci-lint" "$(go env GOPATH)/bin/golangci-lint"
rm -rf "$golangci_tmp"

# -- Go: goimports --------------------------------
echo ""
echo "> Installing goimports..."
GOIMPORTS_VERSION="v0.42.0"  # keep in sync with .claude/hooks/session-start.sh
go install "golang.org/x/tools/cmd/goimports@$GOIMPORTS_VERSION"

# -- Claude Code (native installer) ---------------
echo ""
echo "> Installing Claude Code CLI..."
if command -v claude &>/dev/null; then
    echo "  Claude Code already installed: $(claude --version)"
else
    curl -fsSL https://claude.ai/install.sh | bash
fi

# -- Python Dependencies -------------------------
echo ""
echo "> Installing Python dependencies..."
if [ -f pyproject.toml ]; then
    poetry install --no-root
else
    echo "  No pyproject.toml found, skipping"
fi

# -- Node Dependencies ---------------------------
echo ""
echo "> Installing Node dependencies..."
if [ -f frontend/package.json ]; then
    npm install --prefix frontend
else
    echo "  No frontend/package.json found, skipping"
fi

# -- Pre-commit Hooks ----------------------------
echo ""
echo "> Setting up pre-commit hooks..."
if command -v pre-commit &>/dev/null; then
    pre-commit install -c .pre-commit-config.yaml
else
    pipx install pre-commit
    pre-commit install -c .pre-commit-config.yaml
fi

# -- Resolve Host Project Path for Docker bind mounts --
# In docker-outside-of-docker, the Docker daemon runs on the host.
# Bind mounts must use host paths, not devcontainer paths.
echo ""
echo "> Resolving host project path for Docker bind mounts..."
_host_path=$(docker inspect "$(hostname)" 2>/dev/null \
  | python3 -c "
import sys, json
c = json.load(sys.stdin)
for m in c[0]['Mounts']:
    if m.get('Destination') == '/workspaces/CodeForge':
        print(m['Source']); break
" 2>/dev/null || true)
if [ -n "$_host_path" ]; then
    export HOST_PROJECT_PATH="$_host_path"
    echo "  HOST_PROJECT_PATH=$HOST_PROJECT_PATH"
else
    echo "  Could not detect host path, using default (./)"
fi

# -- Dev environment ----------------------------
# The Core and the worker share a CODEFORGE_INTERNAL_KEY generated once per
# container (dev-env.sh, also sourced by ~/.bashrc). POSTGRES_PASSWORD and
# LITELLM_MASTER_KEY come from devcontainer.json (the host's values or the dev
# defaults); docker compose below takes them from this environment, before
# .env, so the services get the values DATABASE_URL and the Core use.
echo ""
echo "> Preparing the dev environment..."
. .devcontainer/dev-env.sh
echo "  APP_ENV=${APP_ENV:-<unset>}; CODEFORGE_INTERNAL_KEY set (kept in ~/.config/codeforge/internal_key)"

# -- Docker Compose Services ---------------------
echo ""
echo "> Starting docker-compose services..."
if [ -f docker-compose.yml ]; then
    docker compose up -d
    echo "  Services started:"
    docker compose ps --format "  - {{.Name}}: {{.Status}}"

    # Connect devcontainer to the codeforge network so services
    # are reachable by container name (codeforge-postgres, etc.)
    echo ""
    echo "> Connecting devcontainer to codeforge network..."
    if docker network connect codeforge "$(hostname)" 2>/dev/null; then
        echo "  Connected to codeforge network"
    else
        echo "  Already connected (or network not available)"
    fi
else
    echo "  No docker-compose.yml found, skipping"
fi

# -- Shell: CodeForge dev environment ------------
echo ""
echo "> Configuring the CodeForge dev environment for new shells..."
if ! grep -q 'CodeForge dev environment' ~/.bashrc 2>/dev/null; then
    cat >> ~/.bashrc << 'BASHRC_EOF'

# CodeForge dev environment: the Core and the worker share CODEFORGE_INTERNAL_KEY
if [ -f /workspaces/CodeForge/.devcontainer/dev-env.sh ]; then
    . /workspaces/CodeForge/.devcontainer/dev-env.sh
fi
BASHRC_EOF
    echo "  Added dev-env.sh to ~/.bashrc"
else
    echo "  dev-env.sh already in ~/.bashrc"
fi

# -- Shell: auto-activate .venv -------------------
echo ""
echo "> Configuring automatic .venv activation..."
if ! grep -q 'CodeForge .venv' ~/.bashrc 2>/dev/null; then
    cat >> ~/.bashrc << 'BASHRC_EOF'

# Activate CodeForge virtual environment
if [ -f /workspaces/CodeForge/.venv/bin/activate ]; then
    source /workspaces/CodeForge/.venv/bin/activate
fi

# Resolve HOST_PROJECT_PATH for Docker bind mounts (docker-outside-of-docker)
if [ -z "$HOST_PROJECT_PATH" ] || [ "$HOST_PROJECT_PATH" = '${localWorkspaceFolder}' ]; then
    _hp=$(docker inspect "$(hostname)" 2>/dev/null \
      | python3 -c "import sys,json;c=json.load(sys.stdin);[print(m['Source']) for m in c[0]['Mounts'] if m.get('Destination')=='/workspaces/CodeForge']" 2>/dev/null || true)
    [ -n "$_hp" ] && export HOST_PROJECT_PATH="$_hp"
fi
BASHRC_EOF
    echo "  Added .venv activation to ~/.bashrc"
else
    echo "  .venv activation already in ~/.bashrc"
fi

# -- Verify ---------------------------------------
echo ""
echo "=============================================="
echo "  Installed versions:"
echo "=============================================="
echo "  Go:             $(go version | awk '{print $3}')"
echo "  Python:         $(python --version 2>&1 | awk '{print $2}')"
echo "  Node:           $(node --version)"
echo "  Poetry:         $(poetry --version 2>&1 | awk '{print $NF}')"
echo "  golangci-lint:  $(golangci-lint --version 2>&1 | awk '{print $4}')"
echo "  Claude Code:    $(claude --version 2>&1 || echo 'not available')"
echo "  Pre-commit:     $(pre-commit --version 2>&1 | awk '{print $NF}')"
echo "  Docker:         $(docker --version 2>&1 | awk '{print $3}' | tr -d ',')"
echo ""
echo "  CodeForge devcontainer ready."
echo "=============================================="
