# CodeForge — Development Setup

## Voraussetzungen

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (mit WSL2 Backend unter Windows)
- [VS Code](https://code.visualstudio.com/) mit Extension "Dev Containers" (`ms-vscode-remote.remote-containers`)
- Git

## Schnellstart

1. **Repository klonen:**
   ```bash
   git clone <repo-url> CodeForge
   cd CodeForge
   ```

2. **Environment konfigurieren:**
   ```bash
   cp .env.example .env
   # .env anpassen (LM Studio / Ollama Endpoint, API Keys, etc.)
   ```

3. **Devcontainer starten:**
   - VS Code oeffnen: `code .`
   - `Ctrl+Shift+P` → "Dev Containers: Reopen in Container"
   - Warten bis `setup.sh` durchgelaufen ist

4. **Fertig.** Der Container installiert automatisch:
   - Go 1.23, Python 3.12, Node.js 22
   - Poetry, golangci-lint, goimports, Claude Code CLI
   - Python Dependencies (poetry install)
   - Node Dependencies (npm install, wenn package.json vorhanden)
   - Pre-commit Hooks
   - Docker Compose Services (docs-mcp, playwright-mcp)

## Projektstruktur

```
CodeForge/
├── .claude/                  # Claude Code Config (gitignored)
│   ├── commands/             # Custom Slash-Commands
│   ├── hooks/                # Pre/Post Tool-Use Hooks
│   └── settings.local.json   # Lokale Einstellungen
├── .devcontainer/
│   ├── devcontainer.json     # Container-Definition
│   └── setup.sh              # Post-Create Setup Script
├── .devdata/                 # Docker Volumes (gitignored)
│   ├── docs_mcp_data/        # Docs MCP Index
│   └── playwright-mcp/       # Playwright Config
├── cmd/
│   └── codeforge/
│       ├── main.go           # Einstiegspunkt, Dependency Injection
│       └── providers.go      # Blank-Imports aller aktiven Adapter
├── internal/
│   ├── domain/               # Kern: Entities, Business Rules
│   │   ├── project/
│   │   ├── agent/
│   │   └── roadmap/
│   ├── port/                 # Interfaces + Registries
│   │   ├── gitprovider/
│   │   ├── llmprovider/
│   │   ├── agentbackend/
│   │   ├── database/
│   │   └── messagequeue/
│   ├── adapter/              # Konkrete Implementierungen
│   │   ├── github/
│   │   ├── gitlab/
│   │   ├── svn/
│   │   ├── postgres/
│   │   ├── nats/
│   │   └── ...
│   └── service/              # Use Cases
├── workers/                  # Python AI Workers
│   └── codeforge/
│       ├── consumer/         # Queue-Consumer
│       ├── agents/           # Agent-Backends
│       ├── llm/              # LLM-Client via LiteLLM
│       └── models/           # Datenmodelle
├── frontend/                 # TypeScript Web-GUI
│   └── src/
│       ├── features/         # Feature-Module
│       ├── shared/           # Gemeinsame Komponenten
│       └── api/              # API-Client, WebSocket
├── docs/
│   ├── architecture.md       # Systemarchitektur + Hexagonal + Provider Registry
│   ├── dev-setup.md          # Diese Datei
│   ├── project-status.md     # Projektstatus & Roadmap
│   ├── tech-stack.md         # Tech Stack Details
│   └── research/
│       └── market-analysis.md# Marktrecherche & Wettbewerber
├── .env.example              # Environment Template
├── .gitignore
├── .golangci.yml             # Go Linter Config
├── .mcp.json                 # MCP Server fuer Claude Code
├── .pre-commit-config.yaml   # Pre-commit Hooks (Python, Go, TS)
├── CLAUDE.md                 # Projektkontext fuer Claude Code
├── docker-compose.yml        # Dev Services (MCP Server)
├── LICENSE                   # AGPL-3.0
└── pyproject.toml            # Python: Poetry + Ruff + Pytest
```

## Ports

| Port | Service              | Zweck                        |
|------|----------------------|------------------------------|
| 3000 | Frontend Dev-Server  | Web-GUI                      |
| 5173 | Vite HMR             | Hot Module Replacement       |
| 6280 | docs-mcp-server      | Dokumentations-Indexierung   |
| 8001 | playwright-mcp       | Browser-Automatisierung      |
| 8080 | Go API               | Core Service REST/WebSocket  |

## Linting manuell ausfuehren

```bash
# Alle Sprachen via pre-commit
pre-commit run --all-files

# Nur Python
ruff check workers/
ruff format workers/

# Nur Go
golangci-lint run ./...

# Nur TypeScript (wenn Frontend existiert)
npx eslint .
npx prettier --check .
```

## Environment-Variablen

Siehe `.env.example` fuer alle konfigurierbaren Werte.

| Variable                  | Default                                  | Beschreibung                    |
|---------------------------|------------------------------------------|---------------------------------|
| DOCS_MCP_API_BASE         | http://host.docker.internal:1234/v1      | Embedding API Endpoint          |
| DOCS_MCP_API_KEY          | lmstudio                                 | API Key fuer Embeddings         |
| DOCS_MCP_EMBEDDING_MODEL  | text-embedding-qwen3-embedding-8b        | Embedding Model Name            |
