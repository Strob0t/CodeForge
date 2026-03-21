<div align="center">

# CodeForge

**The only self-hosted platform combining project management, visual roadmapping, intelligent LLM routing, and AI agent orchestration -- across all your repositories.**

<!-- TODO: Add demo GIF/screenshot (15-20s: Dashboard -> Chat -> Agent working -> Result) -->
<!-- ![CodeForge Demo](docs/assets/demo.gif) -->

[![CI](https://github.com/Strob0t/CodeForge/actions/workflows/ci.yml/badge.svg?branch=staging)](https://github.com/Strob0t/CodeForge/actions/workflows/ci.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.8.0-green.svg)](VERSION)
[![GitHub Stars](https://img.shields.io/github/stars/Strob0t/CodeForge?style=social)](https://github.com/Strob0t/CodeForge)
<!-- TODO: Add Discord badge when server exists -->
<!-- [![Discord](https://img.shields.io/discord/SERVERID?color=7289da&label=Discord&logo=discord&logoColor=white)](https://discord.gg/INVITE) -->

[![127+ LLM Models](https://img.shields.io/badge/LLM_Models-127+-8A2BE2)](#-intelligent-llm-routing)
[![21 Agent Modes](https://img.shields.io/badge/Agent_Modes-21-FF6B6B)](#-agent-orchestration)
[![5 Protocols](https://img.shields.io/badge/Protocols-MCP_|_A2A_|_AG--UI_|_LSP_|_OTEL-00B4D8)](#architecture)

[**Quick Start**](#quick-start) | [**Documentation**](docs/README.md) | [**Architecture**](docs/architecture.md) | [**Roadmap**](docs/project-status.md)

</div>

---

## What is CodeForge?

Most AI coding tools do one thing well -- Aider for pair programming, Cline for IDE automation, OpenHands for agent orchestration. **CodeForge is the only platform that combines all four pillars** into a single self-hosted Docker stack:

<table>
<tr>
<td width="50%" valign="top">

### :file_folder: Multi-Repo Dashboard
Manage Git, GitHub, GitLab, SVN, Gitea/Forgejo, and local repos from one place. Stack detection, workspace health monitoring, and project-level cost tracking.

</td>
<td width="50%" valign="top">

### :world_map: Visual Roadmap
Drag-and-drop feature planning with OpenSpec, Spec Kit, and Autospec support. Bidirectional sync with GitHub Issues, GitLab, and Plane.

</td>
</tr>
<tr>
<td width="50%" valign="top">

### :brain: Intelligent LLM Routing
127+ models through LiteLLM. 3-layer routing cascade: rule-based complexity analysis (<1ms), UCB1 multi-armed bandit, LLM meta-router fallback. Auto-discovery from Ollama and LM Studio.

</td>
<td width="50%" valign="top">

### :robot: Agent Orchestration
Coordinate Aider, Goose, OpenHands, OpenCode, and Plandex with 5 autonomy levels, 21 built-in modes, DAG scheduling, and multi-agent teams. Built-in agentic loop with 7 tools + MCP.

</td>
</tr>
</table>

<details>
<summary><strong>More capabilities</strong></summary>

- **Real-Time Chat** -- Streaming, inline diff review, slash commands, full-text search, notification center, channels with threads
- **Visual Design Canvas** -- SVG canvas with 7 tools, triple export (PNG/ASCII/JSON), multimodal LLM pipeline with vision support
- **War Room** -- Live multi-agent collaboration view with swim lanes and handoff arrows
- **Code-RAG** -- BM25 + semantic search, sub-agent search, PostgreSQL GraphRAG, SimHash dedup
- **Benchmark System** -- LLM Judge, Functional Test, SPARC, Trajectory Verifier; 8 external providers (HumanEval, SWE-bench, etc.); DPO/RLVR export
- **Safety Layer** -- 8 controls: budget limits, command policies, branch isolation, test/lint gates, stall detection, rollback, path blocklist, max steps
- **HITL Approval** -- Permission cards with approve/deny/allow-always, countdown timer, persistent policy rules
- **Plan/Act Mode** -- Two-phase execution: read-only planning, then full tool access
- **Trust & Quarantine** -- 4-level trust annotations, risk-scored quarantine, persistent agent identity
- **Contract-First Review** -- Boundary detection, review-refactor pipeline, diff impact scoring
- **Goal Discovery** -- Auto-detection of project goals from workspace files
- **Cost Tracking** -- Per-run and per-project monitoring with budget alerts and per-tool token breakdown
- **Audit Trail** -- Event sourcing, trajectory recording, replay, and inspection

</details>

## Why CodeForge?

| | Aider | Cline | OpenHands | **CodeForge** |
|---|:---:|:---:|:---:|:---:|
| Multi-repo dashboard | - | - | - | **Yes** |
| Visual roadmap & PM sync | - | - | - | **Yes** |
| Multi-LLM routing (127+ models) | Partial | Partial | Partial | **Yes** |
| Multi-agent orchestration | - | - | Yes | **Yes** |
| Built-in benchmarking | - | - | Yes | **Yes** |
| Self-hosted, single Docker stack | - | - | Yes | **Yes** |
| MCP + A2A + AG-UI protocols | MCP | MCP | - | **All three** |

No competitor combines all four pillars. [Full market analysis](docs/research/market-analysis.md)

## Quick Start

### Option A: GitHub Codespaces (zero install)

[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://codespaces.new/Strob0t/CodeForge)

The Dev Container auto-installs Go 1.25, Python 3.12, Node.js 22, and starts infrastructure. Then run:

```bash
go run ./cmd/codeforge/ &                                    # API on :8080
cd workers && poetry run python -m codeforge.consumer &      # AI workers
cd frontend && npm run dev                                   # UI on :3000
```

### Option B: Run locally

```bash
git clone https://github.com/Strob0t/CodeForge.git && cd CodeForge
cp .env.example .env                                         # Add your API keys
docker compose up -d                                         # PostgreSQL, NATS, LiteLLM
go run ./cmd/codeforge/ &                                    # API on :8080
cd workers && poetry run python -m codeforge.consumer &      # AI workers
cd frontend && npm run dev                                   # UI on :3000
```

Open [http://localhost:3000](http://localhost:3000) -- default login: `admin@localhost` / `Changeme123`

### Production

```bash
docker compose -f docker-compose.prod.yml up -d              # All 6 services
```

## Architecture

```
SolidJS Frontend (:3000)  --REST/WS/AG-UI-->  Go Core (:8080)  --NATS JetStream-->  Python Workers
```

| Layer | Stack | Purpose |
|-------|-------|---------|
| Frontend | TypeScript 5.x, SolidJS, Tailwind CSS | Web GUI with real-time updates |
| Core | Go 1.25, chi v5, pgx v5 | HTTP/WS server, policies, state management |
| Workers | Python 3.12, LiteLLM, tree-sitter | LLM calls, agent execution, RAG |
| Infra | Docker, PostgreSQL 18, NATS, LiteLLM | Storage, messaging, LLM proxy |

**Protocols:** [MCP](https://modelcontextprotocol.io/) (agent-to-tool) | [A2A](https://github.com/google/a2a-spec) (agent-to-agent) | [AG-UI](https://docs.ag-ui.com/) (agent-to-frontend) | [LSP](https://microsoft.github.io/language-server-protocol/) (code intelligence) | [OpenTelemetry](https://opentelemetry.io/) (tracing)

**Design:** Hexagonal architecture, provider registry pattern, zero-config defaults. [Full architecture docs](docs/architecture.md) | [ADRs](docs/architecture/adr/)

## Documentation

| | |
|---|---|
| **[Architecture](docs/architecture.md)** | System design, protocols, patterns |
| **[Dev Setup](docs/dev-setup.md)** | Ports, config, testing, linting, scripts |
| **[Tech Stack](docs/tech-stack.md)** | Languages, libraries, versions |
| **[Project Status](docs/project-status.md)** | 32 phases completed |
| **[Feature Specs](docs/features/)** | Per-pillar design docs (6 specs) |

## Contributing

1. Fork the repository
2. Create a feature branch from `staging`
3. Run `pre-commit run --all-files` and `./scripts/test.sh all`
4. Submit a pull request to `staging`

All code, comments, and commits in English. See [Dev Setup](docs/dev-setup.md) for the full development guide.

## License

[GNU Affero General Public License v3.0](LICENSE)
