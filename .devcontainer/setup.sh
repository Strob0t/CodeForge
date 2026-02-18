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

# -- Go: golangci-lint ----------------------------
echo ""
echo "> Installing golangci-lint..."
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# -- Go: goimports --------------------------------
echo ""
echo "> Installing goimports..."
go install golang.org/x/tools/cmd/goimports@latest

# -- Claude Code (native CLI) --------------------
echo ""
echo "> Installing Claude Code CLI..."
if command -v claude &>/dev/null; then
    echo "  Claude Code already installed: $(claude --version)"
else
    npm install -g @anthropic-ai/claude-code
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

# -- Docker Compose Services ---------------------
echo ""
echo "> Starting docker-compose services..."
if [ -f docker-compose.yml ]; then
    docker compose up -d
    echo "  Services started:"
    docker compose ps --format "  - {{.Name}}: {{.Status}}"
else
    echo "  No docker-compose.yml found, skipping"
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
