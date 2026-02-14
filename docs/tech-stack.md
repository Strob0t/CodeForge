# CodeForge — Tech Stack

## Sprachen & Versionen

| Sprache    | Version | Einsatzbereich        |
|------------|---------|----------------------|
| Go         | 1.23    | Core Service         |
| Python     | 3.12    | AI Workers           |
| TypeScript | 5.x     | Frontend             |
| Node.js    | 22 LTS  | Frontend Build/Dev   |

## Linting & Formatting

### Python
- **Linter/Formatter:** [Ruff](https://docs.astral.sh/ruff/) (ersetzt flake8, isort, black)
- **Konfiguration:** `pyproject.toml` unter `[tool.ruff]`
- **Regeln:** pyflakes, pycodestyle, isort, pep8-naming, pyupgrade, bugbear, simplify
- **Line Length:** 120

### Go
- **Linter:** [golangci-lint](https://golangci-lint.run/) (Aggregator)
- **Konfiguration:** `.golangci.yml`
- **Aktive Linter:** errcheck, govet, staticcheck, unused, gosimple, gocritic, gofmt, goimports, misspell, prealloc, unconvert, unparam
- **Formatter:** gofmt + goimports

### TypeScript
- **Linter:** ESLint
- **Formatter:** Prettier
- **Konfiguration:** (wird mit Frontend-Setup erstellt)

### Pre-commit Hooks
- **Konfiguration:** `.pre-commit.yaml`
- **Aufruf:** `pre-commit run -c .pre-commit.yaml --all-files`
- Laeuft automatisch bei jedem `git commit`

## Paketmanagement

| Sprache    | Tool       | Lockfile         | Config           |
|------------|------------|------------------|------------------|
| Python     | Poetry     | poetry.lock      | pyproject.toml   |
| Go         | Go Modules | go.sum           | go.mod           |
| TypeScript | npm        | package-lock.json| package.json     |

## Infrastructure

### Devcontainer
- **Base Image:** `mcr.microsoft.com/devcontainers/base:bookworm` (Debian 12)
- **Features:** Go, Python, Node.js, Docker-in-Docker, Git
- **Setup:** `.devcontainer/setup.sh` (automatisch via postCreateCommand)

### Docker Compose (Dev Services)
- **docs-mcp-server** (Port 6280) — Dokumentations-Indexierung fuer LLM-Kontext
- **playwright-mcp** (Port 8001) — Browser-Automatisierung / Web-Scraping

### MCP Server
- Konfiguration: `.mcp.json`
- Automatisch geladen via `enableAllProjectMcpServers: true`

## VS Code Extensions (im Devcontainer)

| Extension | Zweck |
|---|---|
| anthropic.claude-code | Claude Code CLI Integration |
| golang.go | Go Language Support |
| ms-python.python | Python Language Support |
| ms-python.vscode-pylance | Python Type Checking |
| dbaeumer.vscode-eslint | ESLint Integration |
| esbenp.prettier-vscode | Prettier Integration |
| bradlc.vscode-tailwindcss | Tailwind CSS IntelliSense |

## Geplante Dependencies (noch nicht installiert)

### Go Core Service
- HTTP Router (chi, fiber, oder echo)
- WebSocket Library
- NATS/Redis Client
- Git/SVN Bindings

### Python Workers
- LiteLLM (Multi-Provider LLM Routing)
- LangGraph (Agent Orchestrierung)
- Redis/NATS Client

### TypeScript Frontend
- Framework (React/Next.js oder Svelte/SvelteKit)
- Tailwind CSS
- WebSocket Client
