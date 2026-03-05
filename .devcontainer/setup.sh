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
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "$(go env GOPATH)/bin" latest

# -- Go: goimports --------------------------------
echo ""
echo "> Installing goimports..."
go install golang.org/x/tools/cmd/goimports@latest

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
