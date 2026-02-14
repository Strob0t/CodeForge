# CodeForge — Development Setup

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (with WSL2 backend on Windows)
- [VS Code](https://code.visualstudio.com/) with Extension "Dev Containers" (`ms-vscode-remote.remote-containers`)
- Git

## Quick Start

1. **Clone the repository:**
   ```bash
   git clone <repo-url> CodeForge
   cd CodeForge
   ```

2. **Configure environment:**
   ```bash
   cp .env.example .env
   # Edit .env (LM Studio / Ollama endpoint, API keys, etc.)
   ```

3. **Start devcontainer:**
   - Open VS Code: `code .`
   - `Ctrl+Shift+P` → "Dev Containers: Reopen in Container"
   - Wait until `setup.sh` has finished running

4. **Start infrastructure services:**
   ```bash
   docker compose up -d
   ```
   This starts PostgreSQL, NATS JetStream, LiteLLM Proxy, docs-mcp, and playwright-mcp.

5. **Done.** The container automatically installs:
   - Go 1.24, Python 3.12, Node.js 22
   - Poetry, golangci-lint v2, goimports, Claude Code CLI
   - Python dependencies (poetry install)
   - Node dependencies (npm install)
   - Pre-commit Hooks

## Project Structure (Planned)

> **Note:** This shows the target directory structure. Directories not yet created
> are marked with `(planned)` comments or will be scaffolded in Phase 1.

```
CodeForge/
├── .claude/                  # Claude Code Config (gitignored)
│   ├── commands/             # Custom Slash Commands
│   ├── hooks/                # Pre/Post Tool-Use Hooks
│   └── settings.local.json   # Local Settings
├── .devcontainer/
│   ├── devcontainer.json     # Container Definition
│   └── setup.sh              # Post-Create Setup Script
├── data/                     # Persistent data (gitignored, auto-created by docker compose)
│   ├── docs_mcp/             # Docs MCP Index
│   ├── litellm/              # LiteLLM Runtime Data
│   ├── nats/                 # NATS JetStream Data
│   ├── playwright/           # Playwright Config
│   └── postgres/             # PostgreSQL Data
├── cmd/
│   └── codeforge/
│       ├── main.go           # Entry point, Dependency Injection
│       └── providers.go      # Blank imports of all active adapters
├── internal/
│   ├── domain/               # Core: Entities, Business Rules
│   │   ├── project/
│   │   ├── agent/
│   │   └── roadmap/
│   ├── port/                 # Interfaces + Registries
│   │   ├── gitprovider/
│   │   ├── agentbackend/
│   │   ├── specprovider/    # Spec Detection (OpenSpec, Spec Kit, Autospec)
│   │   ├── pmprovider/      # PM Sync (Plane, OpenProject, GitHub/GitLab)
│   │   ├── database/
│   │   └── messagequeue/
│   ├── adapter/              # Concrete Implementations
│   │   ├── github/
│   │   ├── gitlab/
│   │   ├── svn/
│   │   ├── litellm/         # LiteLLM config management
│   │   ├── openspec/        # OpenSpec Adapter
│   │   ├── plane/           # Plane.so Adapter
│   │   ├── goose/           # Goose agent backend (Priority 1)
│   │   ├── opencode/        # OpenCode agent backend (Priority 1)
│   │   ├── plandex/         # Plandex agent backend (Priority 1)
│   │   ├── postgres/
│   │   ├── nats/
│   │   └── ...
│   └── service/              # Use Cases
├── workers/                  # Python AI Workers
│   └── codeforge/
│       ├── consumer/         # Queue Consumer
│       ├── agents/           # Agent Backends
│       ├── llm/              # LLM Client via LiteLLM
│       └── models/           # Data Models
├── frontend/                 # SolidJS Web GUI
│   └── src/
│       ├── features/         # Feature Modules
│       ├── shared/           # Shared Components, Primitives
│       └── api/              # API Client, WebSocket
├── docs/
│   ├── README.md             # Documentation index (start here)
│   ├── todo.md               # TODO tracker for LLM agents
│   ├── architecture.md       # System Architecture + Hexagonal + Provider Registry
│   ├── dev-setup.md          # This file
│   ├── project-status.md     # Project Status & Roadmap
│   ├── tech-stack.md         # Tech Stack Details
│   ├── features/             # Feature specifications
│   │   ├── 01-project-dashboard.md
│   │   ├── 02-roadmap-feature-map.md
│   │   ├── 03-multi-llm-provider.md
│   │   └── 04-agent-orchestration.md
│   ├── architecture/         # Detailed architecture docs
│   │   └── adr/              # Architecture Decision Records
│   └── research/
│       ├── market-analysis.md# Market Research & Competitors
│       └── aider-deep-analysis.md # Aider Architecture Deep-Dive
├── litellm/
│   └── config.yaml           # LiteLLM Proxy Configuration
├── .env.example              # Environment Template
├── .gitignore
├── .golangci.yml             # Go Linter Config
├── .mcp.json                 # MCP Server for Claude Code
├── .pre-commit-config.yaml   # Pre-commit Hooks (Python, Go, TS)
├── CLAUDE.md                 # Project Context for Claude Code
├── docker-compose.yml        # Dev Services
├── LICENSE                   # AGPL-3.0
└── pyproject.toml            # Python: Poetry + Ruff + Pytest
```

## Ports

| Port | Service              | Purpose                          |
|------|----------------------|----------------------------------|
| 3000 | Frontend Dev Server  | Web GUI                          |
| 4000 | LiteLLM Proxy        | LLM Routing (OpenAI-compatible)  |
| 5432 | PostgreSQL           | Primary Database (App + LiteLLM) |
| 4222 | NATS                 | Message Queue (client connections)|
| 5173 | Vite HMR             | Hot Module Replacement           |
| 6280 | docs-mcp-server      | Documentation Indexing           |
| 8001 | playwright-mcp       | Browser Automation               |
| 8080 | Go API               | Core Service REST/WebSocket      |
| 8222 | NATS Monitoring      | NATS HTTP monitoring dashboard   |

## Running Linting Manually

```bash
# All languages via pre-commit
pre-commit run --all-files

# Python only
ruff check workers/
ruff format workers/

# Go only
go build ./cmd/codeforge/
golangci-lint run ./...

# TypeScript only
npm run lint --prefix frontend
npm run format:check --prefix frontend
```

## Running the Project

```bash
# 1. Start infrastructure (PostgreSQL, NATS, LiteLLM)
docker compose up -d

# 2. Go Core Service (port 8080)
go run ./cmd/codeforge/

# 3. Python Worker (connects to NATS)
cd workers && poetry run python -m codeforge.consumer

# 4. Frontend Dev Server (port 3000, proxies /api and /ws to Go Core)
npm run dev --prefix frontend

# Run all tests
go test ./...                     # Go (11 tests)
cd workers && poetry run pytest -v  # Python (16 tests)
npm run build --prefix frontend   # Frontend (type check + build)
```

## Environment Variables

See `.env.example` for all configurable values.

| Variable                  | Default                                  | Description                     |
|---------------------------|------------------------------------------|---------------------------------|
| CODEFORGE_PORT            | 8080                                     | Go Core Service port            |
| CODEFORGE_CORS_ORIGIN     | http://localhost:3000                     | Allowed CORS origin             |
| DATABASE_URL              | postgres://codeforge:...@localhost:5432/codeforge | PostgreSQL connection string |
| NATS_URL                  | nats://localhost:4222                     | NATS server URL                 |
| LITELLM_URL               | http://localhost:4000                     | LiteLLM Proxy URL               |
| LITELLM_MASTER_KEY        | (required)                               | Master Key for LiteLLM Proxy    |
| DOCS_MCP_API_BASE         | http://host.docker.internal:1234/v1      | Embedding API Endpoint          |
| DOCS_MCP_API_KEY          | lmstudio                                 | API Key for Embeddings          |
| DOCS_MCP_EMBEDDING_MODEL  | text-embedding-qwen3-embedding-8b        | Embedding Model Name            |
| OPENAI_API_KEY            | (optional)                               | OpenAI API Key (via LiteLLM)    |
| ANTHROPIC_API_KEY         | (optional)                               | Anthropic API Key (via LiteLLM) |
| GEMINI_API_KEY            | (optional)                               | Google Gemini API Key           |
| OPENROUTER_API_KEY        | (optional)                               | OpenRouter API Key              |
| POSTGRES_PASSWORD         | (required)                               | PostgreSQL password              |
| OLLAMA_BASE_URL           | http://host.docker.internal:11434        | Ollama Endpoint (local)         |
