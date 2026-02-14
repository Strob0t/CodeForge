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

4. **Done.** The container automatically installs:
   - Go 1.23, Python 3.12, Node.js 22
   - Poetry, golangci-lint, goimports, Claude Code CLI
   - Python dependencies (poetry install)
   - Node dependencies (npm install, if package.json exists)
   - Pre-commit Hooks
   - Docker Compose Services (docs-mcp, playwright-mcp, litellm-proxy planned)

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
├── .devdata/                 # Docker Volumes (gitignored)
│   ├── docs_mcp_data/        # Docs MCP Index
│   └── playwright-mcp/       # Playwright Config
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
├── .env.example              # Environment Template
├── .gitignore
├── .golangci.yml             # Go Linter Config
├── .mcp.json                 # MCP Server for Claude Code
├── .pre-commit-config.yaml   # Pre-commit Hooks (Python, Go, TS)
├── CLAUDE.md                 # Project Context for Claude Code
├── docker-compose.yml        # Dev Services (MCP Server)
├── LICENSE                   # AGPL-3.0
└── pyproject.toml            # Python: Poetry + Ruff + Pytest
```

## Ports

| Port | Service              | Purpose                          |
|------|----------------------|----------------------------------|
| 3000 | Frontend Dev Server  | Web GUI                          |
| 4000 | LiteLLM Proxy        | LLM Routing (OpenAI-compatible)  |
| 5173 | Vite HMR             | Hot Module Replacement           |
| 6280 | docs-mcp-server      | Documentation Indexing           |
| 8001 | playwright-mcp       | Browser Automation               |
| 8080 | Go API               | Core Service REST/WebSocket      |

## Running Linting Manually

```bash
# All languages via pre-commit
pre-commit run --all-files

# Python only
ruff check workers/
ruff format workers/

# Go only
golangci-lint run ./...

# TypeScript only (if frontend exists)
npx eslint .
npx prettier --check .
```

## Environment Variables

See `.env.example` for all configurable values.

| Variable                  | Default                                  | Description                     |
|---------------------------|------------------------------------------|---------------------------------|
| DOCS_MCP_API_BASE         | http://host.docker.internal:1234/v1      | Embedding API Endpoint          |
| DOCS_MCP_API_KEY          | lmstudio                                 | API Key for Embeddings          |
| DOCS_MCP_EMBEDDING_MODEL  | text-embedding-qwen3-embedding-8b        | Embedding Model Name            |
| LITELLM_MASTER_KEY        | (required)                               | Master Key for LiteLLM Proxy    |
| OPENAI_API_KEY            | (optional)                               | OpenAI API Key (via LiteLLM)    |
| ANTHROPIC_API_KEY         | (optional)                               | Anthropic API Key (via LiteLLM) |
| GEMINI_API_KEY            | (optional)                               | Google Gemini API Key           |
| OPENROUTER_API_KEY        | (optional)                               | OpenRouter API Key              |
| OLLAMA_BASE_URL           | http://host.docker.internal:11434        | Ollama Endpoint (local)         |
