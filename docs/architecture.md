# CodeForge — Architecture

## Overview

CodeForge is a containerized service for orchestrating AI coding agents.
The architecture follows a three-layer model with strict language separation by responsibility.

## System Architecture

```
┌─────────────────────────────────────────────────────┐
│                  TypeScript Frontend                 │
│                     (SolidJS)                        │
│                                                     │
│  ┌─────────┐  ┌──────────┐  ┌────────┐  ┌────────┐ │
│  │ Project  │  │ Roadmap/ │  │  LLM   │  │ Agent  │ │
│  │Dashboard │  │FeatureMap│  │Provider│  │Monitor │ │
│  └─────────┘  └──────────┘  └────────┘  └────────┘ │
└────────────────────┬────────────────────────────────┘
                     │ REST / WebSocket
┌────────────────────▼────────────────────────────────┐
│                  Go Core Service                     │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐          │
│  │ HTTP/WS  │  │  Agent   │  │   Repo   │          │
│  │ Server   │  │Lifecycle │  │ Manager  │          │
│  └──────────┘  └──────────┘  └──────────┘          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐          │
│  │Scheduling│  │  Auth /  │  │  Queue   │          │
│  │ Engine   │  │ Sessions │  │ Producer │          │
│  └──────────┘  └──────────┘  └──────────┘          │
│  ┌──────────┐  ┌──────────┐                        │
│  │Auto-Detect│  │ PM Sync  │                        │
│  │ Engine   │  │ Service  │                        │
│  └──────────┘  └──────────┘                        │
└────────────┬────────────────────────┬───────────────┘
             │  Message Queue         │
             │  (NATS JetStream)      │
┌────────────▼──────┐  ┌─────────────▼───────────────┐
│  Python Worker 1  │  │  Python Worker N            │
│                   │  │                             │
│  ┌─────────────┐  │  │  ┌─────────────┐           │
│  │  LangGraph  │  │  │  │  LangGraph  │           │
│  │  (Agents)   │  │  │  │  (Agents)   │           │
│  └─────────────┘  │  │  └─────────────┘           │
│  ┌─────────────┐  │  │  ┌─────────────┐           │
│  │ Agent Exec  │  │  │  │ Agent Exec  │           │
│  │(Aider, etc.)│  │  │  │(OpenHands)  │           │
│  └─────────────┘  │  │  └─────────────┘           │
└────────┬──────────┘  └──────────┬──────────────────┘
         │  OpenAI-compatible API │
┌────────▼────────────────────────▼──────────────────┐
│            LiteLLM Proxy (Sidecar)                  │
│  127+ Provider │ Routing │ Budgets │ Cost-Tracking  │
└────────────────────────┬───────────────────────────┘
                         │ Provider APIs
┌────────────────────────▼───────────────────────────┐
│  OpenAI │ Anthropic │ Ollama │ Bedrock │ OpenRouter │
└────────────────────────────────────────────────────┘
```

## Layers in Detail

### Frontend (TypeScript)

- **Purpose:** Web GUI for all user interactions
- **Communication:** REST API for CRUD, WebSocket for real-time updates (agent logs, status)
- **Core Modules:**
  - Project Dashboard (manage repos, status overview)
  - Roadmap/Feature-Map Editor (visual, OpenSpec-compatible)
  - LLM Provider Management (configuration, cost tracking)
  - Agent Monitor (live logs, task status, results)

### Core Service (Go)

- **Purpose:** Performant backend for HTTP, WebSocket, scheduling, and coordination
- **Why Go:** Native concurrency (goroutines), minimal RAM (~10-20MB), fast startup times, excellent for thousands of simultaneous connections
- **Core Modules:**
  - HTTP/WebSocket Server
  - Agent Lifecycle Management (Start, Stop, Status, Restart)
  - Repo Manager (Git, GitHub, GitLab, SVN Integration)
  - Scheduling Engine (task queue, prioritization)
  - Auth / Sessions / Multi-Tenancy
  - Queue Producer (dispatch jobs to Python Workers)

### AI Workers (Python)

- **Purpose:** LLM interaction and agent execution
- **Why Python:** Native access to the AI ecosystem (LiteLLM, LangGraph, all LLM SDKs)
- **Scaling:** Horizontal via Message Queue — any number of worker instances
- **Core Modules:**
  - LiteLLM Integration (multi-provider routing: OpenAI, Claude, Ollama, etc.)
  - Agent Execution (Aider, OpenHands, SWE-agent, Goose, OpenCode, Plandex as swappable backends)
  - LangGraph Orchestration (for complex multi-agent workflows)

## Communication Between Layers

| From → To | Protocol | Purpose |
|---|---|---|
| Frontend → Go | REST (HTTP/2) | CRUD Operations |
| Frontend → Go | WebSocket | Real-time updates, logs |
| Go → Python Workers | NATS JetStream | Job dispatch (subject-based routing) |
| Python Workers → Go | NATS JetStream | Results, status updates |
| Go → LiteLLM Proxy | HTTP (OpenAI format) | Config management, health checks |
| Python Workers → LiteLLM Proxy | HTTP (OpenAI format) | LLM calls (`litellm.completion()`) |
| LiteLLM Proxy → LLM APIs | HTTPS | Provider-specific API calls |
| Go → SCM (Git/SVN) | CLI / REST API | Repo operations |
| Go → Ollama/LM Studio | HTTP | Local Model Auto-Discovery |
| Go → PM Platforms | REST API / Webhooks | Bidirectional PM sync (Plane, OpenProject, etc.) |
| Go → Repo Specs | Filesystem | Spec detection and sync (OpenSpec, Spec Kit, Autospec) |
| Go ↔ Tools/Agents | MCP (JSON-RPC) | Tool integration (server: expose tools, client: connect external) |
| Go → LSP Servers | LSP (JSON-RPC) | Code intelligence per project language |
| Go → OTEL Collector | OTLP (gRPC/HTTP) | Agent lifecycle traces, metrics |
| Python → OTEL Collector | OTLP (gRPC/HTTP) | LLM call traces, token metrics |
| Frontend ← Go | AG-UI events (Phase 2-3) | Standardized agent output streaming |
| External Agents ↔ Go | A2A (Phase 2-3) | Agent discovery via Agent Cards, task delegation |

## Protocol Support

CodeForge integrates with standardized protocols for tool integration, agent coordination,
frontend streaming, code intelligence, and observability.

### Tier 1: Essential (Phase 1-2)

| Protocol | Purpose | Standard | Integration Point |
|---|---|---|---|
| **MCP** (Model Context Protocol) | Agent ↔ Tool communication | JSON-RPC 2.0 over stdio/SSE/HTTP (Anthropic) | Go Core: MCP server (expose tools) + MCP client registry (connect external tools). Python Workers: MCP for agent tool access |
| **LSP** (Language Server Protocol) | Code intelligence for agents | JSON-RPC over stdio/TCP (Microsoft) | Go Core: manages LSP server lifecycle per project language. Agents receive go-to-definition, references, diagnostics, completions |
| **OpenTelemetry GenAI** | Standardized LLM/agent observability | OTEL Semantic Conventions (CNCF) | LiteLLM exports OTEL traces natively. Go Core adds spans for agent lifecycle. Feeds Cost Dashboard + audit trails |

### Tier 2: Important (Phase 2-3)

| Protocol | Purpose | Standard | Integration Point |
|---|---|---|---|
| **A2A** (Agent-to-Agent Protocol) | Peer-to-peer agent coordination | Agent Cards + Tasks over HTTP/SSE (Google → Linux Foundation AAIF) | Agent backends register as A2A agents with capability cards. External agents discover and delegate to CodeForge |
| **AG-UI** (Agent-User Interaction Protocol) | Bi-directional agent ↔ frontend streaming | JSON events over HTTP (CopilotKit) | Frontend WebSocket protocol follows AG-UI event format. Lifecycle events: TEXT_MESSAGE, TOOL_CALL, STATE_DELTA. Human-in-the-loop built in |

### Tier 3: Future / Watch

| Protocol | Purpose | Notes |
|---|---|---|
| **ANP** (Agent Network Protocol) | Decentralized agent communication over internet | Early stage, W3C DIDs. Relevant when agents talk to external agent networks |
| **LSAP** (Language Server Agent Protocol) | LSP extension for AI agents | Emerging proposal, extends LSP with AI-specific capabilities |

### Protocol Architecture

```
┌─────────────────────────────────────────────────────┐
│              TypeScript Frontend                     │
│                                                     │
│   AG-UI Events ←→ Agent output streaming            │
│   (TEXT_MESSAGE, TOOL_CALL, STATE_DELTA, APPROVAL)  │
└────────────────────┬────────────────────────────────┘
                     │ WebSocket (AG-UI event format)
┌────────────────────▼────────────────────────────────┐
│              Go Core Service                         │
│                                                     │
│   MCP Server ←→ Expose CodeForge tools              │
│   MCP Client ←→ Connect to external MCP servers     │
│   LSP Client ←→ Code intelligence per language      │
│   A2A Server ←→ Agent Cards, task delegation        │
│   OTEL SDK   ←→ Traces, metrics → collector         │
└────────────────────┬────────────────────────────────┘
                     │ NATS JetStream
┌────────────────────▼────────────────────────────────┐
│              Python Workers                          │
│                                                     │
│   MCP Client ←→ Tool access for agents              │
│   OTEL SDK   ←→ LLM call traces, token metrics      │
└─────────────────────────────────────────────────────┘
```

## Design Decisions

### Why not everything in Python?
Go handles a fraction of the resources under the same load. A Go HTTP server scales effortlessly to tens of thousands of simultaneous connections — in Python you need significantly more tuning and instances for that.

### Why not everything in Go?
The entire AI/agent ecosystem (LiteLLM, LangGraph, Aider, OpenHands, SWE-agent, all LLM SDKs) is Python. Connecting everything via bridges would be more overhead than dedicated Python workers.

### Why Message Queue instead of direct calls?
- Decoupling: Go service does not have to wait for slow LLM calls
- Scaling: Workers are horizontally scalable
- Resilience: Jobs are not lost when a worker crashes
- Backpressure: Queue buffers during load spikes

### Why YAML as the uniform configuration format?

**All configuration files in CodeForge use YAML** — no exceptions:

- Agent modes and specializations
- Tool bundles and tool definitions
- Project settings and safety rules
- Autonomy configuration
- LiteLLM config (natively YAML)
- Prompt metadata (Jinja2 templates themselves remain `.jinja2`)

**Reason:** YAML supports comments. This is critical for:
- Documentation directly in the config (`# Why this budget limit?`)
- Temporarily disabling settings (`# tools: [terminal]`)
- Onboarding: Contributors understand configs without external documentation
- Versioning: Comments explain changes in the Git diff

JSON is **not** used for configuration files. JSON remains
for API responses, event serialization, and internal data exchange.

## Software Architecture: Hexagonal + Provider Registry

### Core Principle: Hexagonal Architecture (Ports & Adapters)

The core logic (domain + services) is completely isolated from external systems.
All dependencies point inward — never outward.

```
┌──────────────────────────────────────────────────────────┐
│                    ADAPTERS (outer)                        │
│  HTTP Handlers, GitHub, Postgres, NATS, Ollama, Aider     │
│                                                          │
│    ┌──────────────────────────────────────────────┐       │
│    │              PORTS (boundary)               │       │
│    │    Go Interfaces — define WHAT the           │       │
│    │    core logic needs, not HOW                 │       │
│    │                                              │       │
│    │    ┌──────────────────────────────┐          │       │
│    │    │        DOMAIN (core)        │          │       │
│    │    │   Business logic, entities  │          │       │
│    │    │   Rules, validation         │          │       │
│    │    │   Zero external imports     │          │       │
│    │    └──────────────────────────────┘          │       │
│    └──────────────────────────────────────────────┘       │
└──────────────────────────────────────────────────────────┘
```

### Provider Registry Pattern

For open-source extensibility, CodeForge uses a self-registering provider pattern.
New implementations (e.g., a Gitea adapter) require:

1. A Go package that satisfies the corresponding interface
2. A blank import in `cmd/codeforge/providers.go`
3. No changes to the core logic

#### Flow

```
1. Port defines interface + registry
   (Register, New, Available)

2. Adapter implements interface
   and registers itself via init()

3. Blank import in providers.go
   activates the adapter

4. Core logic only uses the interface —
   does not know which adapter is behind it
```

This pattern follows the Go standard pattern (`database/sql` + `_ "github.com/lib/pq"`).

#### Provider Types

| Port | Interface | Example Adapters |
|---|---|---|
| `gitprovider` | `Provider` | github, gitlab, gitlocal, svn, gitea |
| `agentbackend` | `Backend` | aider, openhands, sweagent, goose, opencode, plandex |
| `specprovider` | `SpecProvider` | openspec, speckit, autospec |
| `pmprovider` | `PMProvider` | plane, openproject, github_pm, gitlab_pm |
| `database` | `Store` | postgres |
| `messagequeue` | `Queue` | nats |

#### Capabilities

Not every provider supports all operations. Instead of empty implementations,
each provider declares its capabilities:

```go
type Capability string
const (
    CapClone    Capability = "clone"
    CapWebhooks Capability = "webhooks"
    CapPRs      Capability = "pull_requests"
    // ...
)
```

The core logic and the frontend check capabilities and adapt their behavior accordingly.
SVN does not support webhooks, for example — that is not an error but declared behavior.

#### Compliance Tests

Each provider type ships a reusable test suite (`RunComplianceTests`).
A new adapter calls this function and automatically receives all interface tests.
Contributors write minimal test code and get maximum coverage.

### Go Core Directory Structure

```
cmd/
  codeforge/
    main.go              # Entry point, dependency injection
    providers.go         # Blank imports of all active adapters
internal/
  domain/                # Core: Entities, business rules (zero external imports)
    project/
    agent/
    roadmap/
  port/                  # Interfaces + registries
    gitprovider/
      provider.go        # Interface + capability definitions
      registry.go        # Register(), New(), Available()
      compliance_test.go # Reusable test suite
    agentbackend/
    specprovider/
      provider.go        # SpecProvider Interface (Detect, ReadSpecs, WriteChange, Watch)
      registry.go        # Register(), New(), Available()
    pmprovider/
      provider.go        # PMProvider Interface (Detect, SyncItems, CreateItem, Webhooks)
      registry.go        # Register(), New(), Available()
    database/
    messagequeue/
  adapter/               # Concrete implementations
    github/
    gitlab/
    gitlocal/
    svn/
    litellm/             # LiteLLM config management adapter
    aider/
    openhands/
    sweagent/
    goose/               # Goose agent backend (Priority 1)
    opencode/            # OpenCode agent backend (Priority 1)
    plandex/             # Plandex agent backend (Priority 1)
    openspec/            # OpenSpec spec adapter
    speckit/             # GitHub Spec Kit adapter
    autospec/            # Autospec adapter
    plane/               # Plane.so PM adapter
    openproject/         # OpenProject PM adapter
    github_pm/           # GitHub Issues/Projects PM adapter
    gitlab_pm/           # GitLab Issues/Boards PM adapter
    postgres/
    nats/
  service/               # Use cases (connects domain with ports)
```

## LLM Capability Levels

Not every LLM brings the same capabilities. CodeForge must fill the gaps
so that even simple models can be used productively.

### The Problem

```
Claude Code / Aider       →  own tool usage, codebase search, agent loop
OpenAI API (direct)       →  function calling, but no codebase context
Ollama (local)            →  pure text completion, no tools, no context
```

A local Ollama model knows nothing about the repo, cannot read files,
and has no memory. CodeForge must provide these capabilities.

### Capability Stacking by Python Workers

The workers supplement missing capabilities depending on the LLM level:

```
┌──────────────────────────────────────────────────────┐
│                    CodeForge Worker                    │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Context Layer (for all LLMs)                  │  │
│  │  GraphRAG: Vector search + Graph DB +          │  │
│  │  Web fallback → find relevant code/docs        │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Quality Layer (optional, configurable)        │  │
│  │  Multi-Agent Debate: Pro/Con/Moderator →       │  │
│  │  Reduce hallucinations, verify solutions       │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Routing Layer                                 │  │
│  │  Task-based model routing via LiteLLM →        │  │
│  │  Right task to the right model                 │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │  Execution Layer                               │  │
│  │  Agent backends: Aider, OpenHands, SWE-agent,  │  │
│  │  Goose, OpenCode, Plandex, or direct LLM API   │  │
│  └────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

### Three LLM Integration Levels

| Level | Example | What CodeForge Provides |
|---|---|---|
| **Full-featured Agents** | Claude Code, Aider, OpenHands | Orchestration only — agent brings its own tools |
| **API with Tool Support** | OpenAI, Claude API, Gemini | Context Layer (GraphRAG) + Routing + Tool Definitions |
| **Pure Completion** | Ollama, LM Studio (local models) | Everything: Context, Tools, Prompt Engineering, Quality Layer |

The less an LLM can do, the more the CodeForge Worker takes over.

### Worker Modules in Detail

**Context Layer — GraphRAG**
- Vector Search (Qdrant or pgvector): Semantic search in the codebase index
- Graph DB (Neo4j/optional): Relationships between code elements (imports, calls, inheritance)
- Web Fallback (Tavily/SearXNG): Documentation and Stack Overflow when local info is missing
- Result: Relevant context is prepended to the LLM prompt

**Quality Layer — Multi-Stage Quality Assurance**

Four strategies, graduated by effort and criticality:

1. **Action Sampling** (lightweight)
   - Generate multiple independent LLM responses
   - AskColleagues: N proposals, LLM synthesizes the best solution
   - BinaryComparison: Pairwise comparison, winner is selected
   - For everyday tasks with moderate quality requirements

2. **RetryAgent + Reviewer** (medium)
   - Agent solves task multiple times (environment reset between attempts)
   - LLM-based reviewer evaluates each solution:
     - Score mode: Numerical evaluation, average across samples
     - Chooser mode: Direct comparison of all solutions
   - Best solution is selected
   - For important changes with measurable quality

3. **LLM Guardrail Agent** (medium)
   - A separate LLM evaluates the output of the working agent
   - Validates format compliance, safety, and correctness
   - Can reject and trigger retry before delivery
   - For automated pipelines where human review is unavailable

4. **Multi-Agent Debate** (heavyweight)
   - Pro agent argues for a solution
   - Con agent searches for weaknesses
   - Moderator synthesizes the result
   - For critical architecture decisions and security-relevant changes

All four strategies are optional and configurable per project/task.

**Routing Layer — Intelligent Model Routing**
- Task classification: Architecture, code generation, review, docs, tests
- Cost optimization: Simple tasks to cheap models, complex to powerful ones
- Latency routing: Fast responses for interactive usage
- Fallback chains: If a provider fails, automatically use the next one
- Routing rules configurable per project and per user
- **Cost Management:**
  - Budget limits per task, per project, per user
  - Automatic cost tracking via LiteLLM
  - Warning/stop when budget is exceeded
  - API call limits per agent run

## Agent Execution: Modes, Safety, Workflow

### Three Execution Modes

Not every use case needs a sandbox. CodeForge supports three modes:

```
┌─────────────────────────────────────────────────────────────────┐
│                      Execution Modes                             │
│                                                                 │
│  ┌──────────────┐  ┌──────────────────┐  ┌──────────────────┐  │
│  │   Sandbox    │  │     Mount        │  │     Hybrid       │  │
│  │              │  │                  │  │                  │  │
│  │  Isolated    │  │  Agent works     │  │  Sandbox with    │  │
│  │  container,  │  │  directly on     │  │  mounted         │  │
│  │  repo copy   │  │  mounted path    │  │  volumes         │  │
│  │  in container│  │  of the host     │  │  (read/write     │  │
│  │              │  │                  │  │   configurable)  │  │
│  └──────────────┘  └──────────────────┘  └──────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

| Mode | When | Security | Speed |
|---|---|---|---|
| **Sandbox** | Untrusted agents, foreign models, batch jobs | High — no access to host | Medium — container overhead, repo copy |
| **Mount** | Trusted agents (Claude Code, Aider), local development | Low — direct file access | High — no overhead |
| **Hybrid** | Review workflows, CI-like execution | Medium — controlled access | Medium |

**Mount Mode in Detail:**
- Agent receives path to the mounted repo (e.g., `/workspace/my-project`)
- Changes land directly in the host's filesystem
- Ideal for interactive use: user sees changes immediately in their IDE
- No container needed — agent runs in the worker process or native tool

**Sandbox Mode in Detail:**
- Docker container per task (Docker-in-Docker)
- Repo is copied into the container or mounted as a read-only volume
- Agent gets all necessary tools provisioned in the container
- Result is extracted as a patch/diff and applied to the original repo

**Hybrid Mode in Detail:**
- Container with mounted volume
- Mount permissions configurable: read-only source + write workspace copy
- Agent can read, but changes go into a copy
- User reviews and merges manually

### Tool Provisioning for Sandbox Agents

Agents in sandbox containers need the right tools. CodeForge provides
these automatically — depending on the agent type and execution mode:

```
┌─────────────────────────────────────────────────┐
│            Sandbox Container                     │
│                                                 │
│  ┌───────────────────────────────────────────┐  │
│  │  Base Image (Python/Node/Go)              │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │  CodeForge Tool Layer                     │  │
│  │  - Shell (with Safety Evaluator)          │  │
│  │  - File Read/Write/Patch                  │  │
│  │  - Grep/Search                            │  │
│  │  - Git Operations                         │  │
│  │  - Dependency Installation                │  │
│  │  - Test Runner                            │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │  Repo (copied or mounted)                 │  │
│  └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
```

Tools are defined as Pydantic schemas and passed to the LLM as function calls
or tool definitions. Full-featured agents (Aider, OpenHands)
bring their own tools and only need the repo.

### Command Safety Evaluator

Every shell command from an agent goes through a safety check:

- **Destructive operations** detection (`rm -rf`, `git push --force`, etc.)
- **Prompt injection** detection in commands
- **Risk level** assessment: low / medium / high
- **Tool blocklists:** Interactive programs (`vim`, `nano`), standalone
  interpreters (`python` without script), dangerous commands — configurable
  per project as YAML
- **Configurable** per project: What may an agent do, what not?
- When uncertain: Block command and ask user (human-in-the-loop)

Optional for trusted agents in mount mode. Mandatory for local models in the
sandbox.

### Agent Workflow: Plan → Execute → Review

Standardized workflow for all agents — with configurable autonomy level:

```
1. PLAN      Agent analyzes task + codebase, creates structured plan
                ↓
2. APPROVE   Plan is submitted for approval (depending on autonomy level)
                ↓  (User, safety rules, or auto-approve)
3. EXECUTE   Agent works through plan point by point
                ↓
4. REVIEW    Self-review, second agent, or guardrail agent
                ↓
5. DELIVER   Result as diff/patch, PR, or direct file change
```

- Each step is individually configurable (skip, auto-approve, etc.)
- Autonomy level determines who may approve (user vs. safety rules)
- At level 4-5, safety rules replace the human approver

### Autonomy Spectrum

CodeForge supports five autonomy levels — from fully supervised
operation to completely autonomous execution without user interaction:

```
Level 1   Level 2     Level 3     Level 4      Level 5
supervised  semi-auto   auto-edit   full-auto    headless
  │           │           │           │            │
  ▼           ▼           ▼           ▼            ▼
 User       User        User       Safety       Safety
 approves   approves    approves   Rules        Rules
 EVERYTHING destructive Terminal/  replace      replace
            actions     Deploy     User         User
                                                + no UI
```

| Level | Name | Who Approves | Use Case |
|---|---|---|---|
| 1 | `supervised` | User at every step | Learning, critical codebases, onboarding |
| 2 | `semi-auto` | User for destructive actions (delete, terminal, deploy) | Everyday development with safety net |
| 3 | `auto-edit` | User only for terminal/deploy, file changes auto-approved | Experienced users, trusted agents |
| 4 | `full-auto` | Safety rules (budget, blocklists, tests) | Batch jobs, trusted agents, delegated tasks |
| 5 | `headless` | Safety rules, no UI needed | CI/CD, cron jobs, API-driven pipelines |

#### Configuration (YAML)

```yaml
# Project level: codeforge-project.yaml
autonomy:
  default_level: semi-auto       # Default for new tasks

  # Safety rules — replace the user as guardrail at level 4-5
  safety:
    budget_hard_limit: 50.00     # USD — agent stops when exceeded
    max_steps: 100               # Max actions per task
    max_file_changes: 50         # Max changed files per task
    blocked_paths:               # Files that may never be changed
      - ".env"
      - "secrets/"
      - "**/credentials.*"
      - "production.yml"
    blocked_commands:             # Shell commands that may never be executed
      - "rm -rf /"
      - "DROP TABLE"
      - "git push --force"
      - "chmod 777"
    require_tests_pass: true     # Agent must have green tests before deliver
    require_lint_pass: true      # Linting must pass before deliver
    rollback_on_failure: true    # Auto-rollback on test/lint failure
    branch_isolation: true       # Autonomous agents never work on main/master
    max_cost_per_step: 2.00      # USD — single LLM call may cost max X
    stall_detection: true        # Detect and abort/re-plan on agent loops
```

#### Security for Fully Autonomous Execution

At level 4 (`full-auto`) and level 5 (`headless`), the following
mechanisms replace the human approver:

```
┌─────────────────────────────────────────────────────────────┐
│                  Safety Layer (replaces user)                  │
│                                                             │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Budget Limiter  │  │ Command Safety  │                  │
│  │  Hard stop when  │  │ Evaluator       │                  │
│  │  exceeded        │  │ Blocklist +     │                  │
│  │                  │  │ Regex matching  │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Branch Isolation│  │ Test/Lint Gate  │                  │
│  │  Never on main,  │  │ Deliver only    │                  │
│  │  always feature  │  │ when tests +    │                  │
│  │  branch          │  │ lint pass       │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Max Steps       │  │ Rollback        │                  │
│  │  Infinite loop   │  │ Automatic on    │                  │
│  │  detection       │  │ failure         │                  │
│  └─────────────────┘  └─────────────────┘                  │
│  ┌─────────────────┐  ┌─────────────────┐                  │
│  │  Path Blocklist  │  │ Stall Detection │                  │
│  │  Sensitive files  │  │ Re-planning     │                  │
│  │  protected       │  │ or abort        │                  │
│  └─────────────────┘  └─────────────────┘                  │
└─────────────────────────────────────────────────────────────┘
```

#### Headless Mode (Level 5) — Use Cases

```yaml
# Nightly code review (cron job)
# codeforge-schedules.yaml
schedules:
  - name: nightly-review
    cron: "0 2 * * *"                  # Every night at 2:00
    mode: reviewer
    autonomy: headless
    targets:
      - repo: "myorg/backend"
        branch: "develop"
    deliver: github-pr-comment         # Result as PR comment

  # Weekly dependency update
  - name: weekly-deps
    cron: "0 8 * * 1"                  # Mondays at 8:00
    mode: dependency-updater
    autonomy: headless
    targets:
      - repo: "myorg/backend"
      - repo: "myorg/frontend"
    deliver: pull-request               # Result as new PR
    safety:
      require_tests_pass: true
      max_file_changes: 5

  # Webhook-triggered: Lint fix on new PR
  - name: auto-lint-fix
    trigger: github-webhook             # On new PR
    event: pull_request.opened
    mode: lint-fixer
    autonomy: full-auto
    deliver: push-to-branch             # Push directly to the PR branch
    safety:
      max_file_changes: 20
      require_lint_pass: true
```

#### API-Driven Autonomous Execution

For CI/CD and external systems:

```
POST /api/v1/tasks
{
  "repo": "myorg/backend",
  "task": "Fix all lint errors in src/",
  "mode": "lint-fixer",
  "autonomy": "full-auto",
  "deliver": "pull-request",
  "safety": {
    "budget_hard_limit": 10.00,
    "require_lint_pass": true
  },
  "callback_url": "https://ci.example.com/webhook"
}
```

- No UI interaction needed
- Result is retrieved via callback or polling
- Ideal for GitHub Actions, GitLab CI, Jenkins, etc.

### Jinja2 Prompt Templates

All prompts for LLM calls are stored as Jinja2 templates in separate
files, not in Python code:

```
workers/codeforge/templates/
  planner.jinja2          # Planning prompt
  coder.jinja2            # Code generation prompt
  reviewer.jinja2         # Review prompt
  researcher.jinja2       # Research prompt
  safety_evaluator.jinja2 # Safety check prompt
```

Advantages:
- Prompts are adjustable without code changes
- Contributors can improve prompts without knowing Python
- Different prompt sets for different LLMs are possible
- Versionable and comparable (Git diff on prompt changes)

### Keyword Extraction (KeyBERT)

For the Context Layer: Semantic keyword extraction from tasks and code
using SentenceTransformers/BERT:

- Extracts relevant keywords from user requests and codebase
- Maximal Marginal Relevance (MMR) for diverse, non-redundant keywords
- Keywords improve retrieval quality in the GraphRAG layer
- Lightweight, runs locally without external API

### Real-time State via WebSocket

Every state mutation of an agent is immediately emitted to the
frontend via WebSocket:

- Agent status (active, waiting, finished)
- Internal monologue (what the agent is "thinking")
- Current step in the workflow
- Token usage and costs in real time
- Terminal/browser session data

The frontend can display live updates without polling.

### Agent Specialization: Modes System

Inspired by Roo Code's Modes and Cline's `.clinerules`. Instead of a
general-purpose agent, CodeForge defines specialized agent modes
as YAML configurations. Each mode has its own tools, LLM settings,
and autonomy level.

#### Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    Mode Registry                          │
│                                                          │
│  Built-in Modes        Custom Modes (user-defined)       │
│  ┌──────────────┐      ┌──────────────────────────┐     │
│  │ architect    │      │ my-react-reviewer        │     │
│  │ coder        │      │ security-auditor         │     │
│  │ reviewer     │      │ docs-writer              │     │
│  │ researcher   │      │ dependency-updater       │     │
│  │ tester       │      │ ...                      │     │
│  │ lint-fixer   │      │                          │     │
│  │ planner      │      │ (YAML in project or      │     │
│  │ debugger     │      │  global config)          │     │
│  └──────────────┘      └──────────────────────────┘     │
└──────────────────────────────────────────────────────────┘
```

#### Built-in Mode Definitions

```yaml
# modes/architect.yaml
name: architect
description: "Analyzes codebase structure, plans changes, creates design documents"
llm_scenario: think            # LiteLLM Tag → strong reasoning model
autonomy: supervised           # Architecture decisions always with user
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - plan                       # Create structured plan
  - web_search                 # Research documentation
# No write_file, no terminal — Architect may only read and plan
prompt_template: architect.jinja2
max_steps: 30
```

```yaml
# modes/coder.yaml
name: coder
description: "Implements features, fixes bugs, writes code"
llm_scenario: default          # LiteLLM Tag → standard coding model
autonomy: auto-edit            # File changes auto, terminal needs approval
tools:
  - read_file
  - write_file
  - search_file
  - search_dir
  - list_files
  - terminal                   # Shell commands (with Safety Evaluator)
  - git_diff
  - git_commit
  - lint
  - test
prompt_template: coder.jinja2
max_steps: 50
```

```yaml
# modes/reviewer.yaml
name: reviewer
description: "Reviews code changes for quality, bugs, security"
llm_scenario: review           # LiteLLM Tag → review-optimized model
autonomy: headless             # Can run completely autonomously (readonly)
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - git_diff
  - lint
  - test
# No write_file — Reviewer may not edit, only evaluate
prompt_template: reviewer.jinja2
max_steps: 30
deliver: comment               # Result as comment (PR, issue, web GUI)
```

```yaml
# modes/debugger.yaml
name: debugger
description: "Analyzes errors, reproduces bugs, finds root causes"
llm_scenario: think            # Complex reasoning for debugging
autonomy: semi-auto            # Terminal execution with approval
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - terminal                   # For reproduction and tests
  - git_log
  - git_diff
  - test
  - lint
prompt_template: debugger.jinja2
max_steps: 40
```

```yaml
# modes/nightly-reviewer.yaml
name: nightly-reviewer
description: "Automatic nightly code review"
llm_scenario: review
autonomy: headless             # Completely autonomous, no UI
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - git_diff
  - lint
  - test
prompt_template: reviewer.jinja2
schedule: "0 2 * * *"         # Every night at 2:00
deliver: github-pr-comment
safety:
  budget_hard_limit: 5.00
  max_steps: 30
```

#### Custom Modes (User-Defined)

Users can create their own modes as YAML files:

```yaml
# .codeforge/modes/security-auditor.yaml
name: security-auditor
description: "Reviews code for OWASP Top 10, injection, XSS, etc."
llm_scenario: think
autonomy: headless
tools:
  - read_file
  - search_file
  - search_dir
  - list_files
  - lint
  - terminal                   # For security scanners (npm audit, bandit, etc.)
prompt_template: security-auditor.jinja2
safety:
  blocked_commands:
    - "curl"                   # No network access
    - "wget"
  max_steps: 50
deliver: security-report       # Structured security report
```

#### Mode Selection and Composition

Modes can be used individually or as a pipeline:

```yaml
# Single mode
task:
  mode: coder
  prompt: "Implement feature X"

# Pipeline: Architect plans, Coder implements, Reviewer reviews
task:
  pipeline:
    - mode: architect
      prompt: "Analyze the codebase and create a plan for feature X"
    - mode: coder
      prompt: "Implement the plan from the previous step"
    - mode: reviewer
      prompt: "Review the coder's changes"
    - mode: tester
      prompt: "Write tests for the new changes"

# DAG: Parallel execution + dependencies
task:
  dag:
    plan:
      mode: architect
    implement:
      mode: coder
      depends_on: [plan]
    test:
      mode: tester
      depends_on: [implement]
    review:
      mode: reviewer
      depends_on: [implement]     # Parallel to test
    deliver:
      mode: coder
      depends_on: [test, review]  # Only when both are done
```

#### Directory Structure

```
# Global (shipped with CodeForge)
modes/
  architect.yaml
  coder.yaml
  reviewer.yaml
  researcher.yaml
  tester.yaml
  lint-fixer.yaml
  planner.yaml
  debugger.yaml
  dependency-updater.yaml

# Project-specific (user-defined)
.codeforge/
  modes/
    security-auditor.yaml
    my-react-reviewer.yaml
  project.yaml                 # Project settings (autonomy, safety, etc.)
  schedules.yaml               # Cron jobs for autonomous tasks
```

### YAML-Based Tool Definitions

Tools for agents are defined declaratively in YAML, not hardcoded in code.
Contributors can add new tools without writing Python code:

```yaml
# tools/bundles/file_ops/config.yaml
tools:
  read_file:
    docstring: "Read contents of a file"
    arguments:
      - name: path
        type: string
        required: true
        description: "Absolute path to the file"
  write_file:
    docstring: "Write contents to a file"
    arguments:
      - name: path
        type: string
        required: true
      - name: content
        type: string
        required: true
```

- Tool bundles are directories with `config.yaml` + optional install script
- Automatic conversion to OpenAI function calling format
- Works with any LLM that supports function calling
- For LLMs without function calling: Backtick/JSON-based parsing as fallback

### History Processors (Context Window Management)

Long agent sessions exceed the context window. History Processors
optimize context as a configurable pipeline:

| Processor | Function |
|---|---|
| **LastNObservations** | Replace old tool outputs with summaries |
| **ClosedWindowProcessor** | Remove outdated file views, keep only the most recent |
| **CacheControlProcessor** | Set cache markers for prompt caching (Anthropic, etc.) |
| **RemoveRegex** | Remove specific patterns from the history |

Processors are applied as a pipeline one after another. Configurable
per agent type and LLM (small local models need more aggressive trimming).

### Hook System (Observer Pattern)

Extension points at agent and environment lifecycle, without core modification:

```
Agent Hooks:
  on_run_start       → Start monitoring, logging
  on_step_done       → Record step, update metrics
  on_model_query     → Track costs, rate limiting
  on_run_end         → Summary, cleanup

Environment Hooks:
  on_init            → Prepare container
  on_copy_repo       → Start repo indexing
  on_startup         → Install tools
  on_close           → Clean up container
```

Hooks enable monitoring, custom logging, metrics collection, and
integration with external systems — all without modifying the core logic.

### Trajectory Recording and Replay

Every agent run is recorded as a trajectory:

- Every step: Thought → Action → Observation → Timestamp → Cost
- Stored as JSON for analysis and reproducibility
- **Replay mode:** Deterministically repeat trajectory (debugging)
- **Inspector:** Web-based viewer integrated in the GUI
- **Batch statistics:** Success rates, costs, steps across many runs

Trajectories enable:
- Debugging of failed agent runs
- Comparison of different LLMs/configs on the same tasks
- Audit trail for code changes by agents

### Python Workers Directory Structure

```
workers/
  codeforge/
    consumer/            # Queue consumer (ingress)
    context/             # Context Layer
      graphrag.py        # Vector + Graph + Web retrieval
      indexer.py         # Codebase indexing
      keywords.py        # KeyBERT keyword extraction
    quality/             # Quality Layer
      debate.py          # Multi-Agent Debate (Pro/Con/Moderator)
      reviewer.py        # Score/Chooser-based solution reviewer
      sampler.py         # Action Sampling (AskColleagues, BinaryComparison)
      guardrail.py       # LLM Guardrail Agent (from CrewAI)
      action_node.py     # Structured Output / Schema validation (from MetaGPT)
    routing/             # Routing Layer
      router.py          # Task-based model routing
      cost.py            # Cost tracking and budgets
    safety/              # Safety Layer
      evaluator.py       # Command Safety Evaluator
      policies.py        # Project-specific security rules
      blocklists.py      # Tool blocklists (configurable)
    execution/           # Execution Layer
      sandbox.py         # Docker container management
      mount.py           # Mount mode logic
      tools.py           # Tool provisioning (Shell, File, Git, etc.)
      workbench.py       # Tool container with shared state (from AutoGen)
    memory/              # Memory Layer
      composite.py       # Composite Scoring (Semantic+Recency+Importance)
      context_window.py  # Context window strategies (Buffered/TokenLimited/HeadAndTail)
      experience.py      # Experience Pool (@exp_cache, from MetaGPT)
    history/             # History Management
      processors.py      # Context window optimization (pipeline)
    hooks/               # Hook System (Observer Pattern)
      agent_hooks.py     # Agent lifecycle hooks
      env_hooks.py       # Environment lifecycle hooks
    events/              # Event Bus (from CrewAI)
      bus.py             # Event emitter + subscriber
      types.py           # Agent/Task/System event definitions
    orchestration/       # Workflow Orchestration
      graph_flow.py      # DAG orchestration (from AutoGen)
      termination.py     # Composable Termination Conditions
      handoff.py         # HandoffMessage Pattern
      planning.py        # MagenticOne Planning Loop + Stall Detection
      pipeline.py        # Document pipeline PRD→Design→Tasks→Code (from MetaGPT)
    trajectory/          # Trajectory Recording
      recorder.py        # Step-by-step recording
      replay.py          # Deterministic replay
    agents/              # Agent backends (Aider, OpenHands, SWE-agent, Goose, OpenCode, Plandex)
    llm/                 # LLM client via LiteLLM
    models/              # Data models
      components.py      # Component System (JSON-serializable configs)
    tools/               # YAML-based tool bundles
      bundles/           # Tool bundle directories
      recommender.py     # BM25 Tool Recommendation (from MetaGPT)
    templates/           # Jinja2 prompt templates
    hitl/                # Human-in-the-Loop
      providers.py       # Human Feedback Provider Protocol (from CrewAI)
```

## Framework Insights: Adopted Patterns

From the analysis of LangGraph, CrewAI, AutoGen, and MetaGPT, the following
patterns were adopted for CodeForge. Detailed comparison: docs/research/market-analysis.md

### Composite Memory Scoring (from CrewAI)

Simple semantic similarity is not enough for memory recall. CodeForge
uses weighted scoring from three factors:

```
Score = (semantic_weight * cosine_similarity)
      + (recency_weight  * recency_decay)
      + (importance_weight * importance_score)
```

| Factor | Default Weight | Calculation |
|---|---|---|
| Semantic | 0.5 | Cosine similarity of embeddings |
| Recency | 0.3 | Exponential decay (half-life configurable) |
| Importance | 0.2 | LLM-based evaluation at storage time |

Two recall modes:
- **Shallow:** Direct vector search with composite scoring
- **Deep:** LLM distills sub-queries, searches in parallel, confidence-based routing

### Context Window Strategies (from AutoGen)

In addition to the History Processors, different strategies for
chat completion context management are supported:

| Strategy | Behavior |
|---|---|
| **Unbounded** | Keep all messages (only for short sessions) |
| **Buffered** | Keep last N messages |
| **TokenLimited** | Trim to token budget |
| **HeadAndTail** | Keep first N + last M messages (system prompt + current context) |

Configurable per agent type and LLM. Small local models get more aggressive
trimming, large API models keep more context.

### Experience Pool (from MetaGPT)

Successful agent runs are cached and reused for similar tasks:

```
@exp_cache(context_builder=build_task_context)
async def solve_task(task: Task) -> Result:
    # If similar task was already solved successfully:
    # → Return cached result
    # Otherwise: Execute normally and cache result
```

- Cache key based on task description + codebase context
- Similarity-based retrieval (not exact match)
- Configurable confidence threshold
- Saves LLM costs and improves consistency

### Tool Recommendation via BM25 (from MetaGPT)

Instead of passing all available tools to the LLM (token waste),
relevant tools are automatically selected:

- BM25-based ranking of tools against the current task context
- Top-K tools are offered to the LLM as function calls
- Reduces token usage and improves tool selection quality
- Fallback: All tools when confidence score is low

### Workbench — Tool Container (from AutoGen)

Related tools share state and lifecycle:

```python
class GitWorkbench(Workbench):
    """Git tools with shared repository state."""

    def __init__(self, repo_path: str):
        self.repo = git.Repo(repo_path)

    def get_tools(self) -> list[Tool]:
        return [
            Tool("git_status", self._status),
            Tool("git_diff", self._diff),
            Tool("git_commit", self._commit),
            # Tools share self.repo
        ]
```

- Shared state between related tools
- Lifecycle management (start/stop/restart)
- Dynamic tool discovery (tools can change)
- Ideal for MCP integration (McpWorkbench)

### LLM Guardrail Agent (from CrewAI)

A dedicated agent validates the output of another agent:

```
Agent A (Coder) → Output → Guardrail Agent → Validates → Accept / Reject + Feedback
                                                              ↓ (on reject)
                                                         Agent A retries with feedback
```

Integrated into the Quality Layer as a fourth strategy alongside
Action Sampling, RetryAgent+Reviewer, and Multi-Agent Debate:

| Level | Effort | Mechanism |
|---|---|---|
| 1. Action Sampling | Light | N responses, select the best |
| 2. RetryAgent + Reviewer | Medium | Retry + score/chooser evaluation |
| 3. LLM Guardrail Agent | Medium | Dedicated agent checks output |
| 4. Multi-Agent Debate | Heavy | Pro/Con/Moderator |

### Structured Output / ActionNode (from MetaGPT)

LLM outputs are validated against a schema and automatically corrected if needed:

```python
class CodeReviewOutput(ActionNode):
    issues: list[Issue]       # Found problems
    severity: str             # critical / warning / info
    suggestion: str           # Improvement suggestion
    approved: bool            # Review passed?
```

- Schema definition as Pydantic model
- LLM fills the fields via constrained generation
- Automatic review/revise cycle on schema violation
- Retry with error feedback to the LLM

### Event Bus for Observability (from CrewAI)

All relevant events in the system are emitted via an event bus:

```
Agent Events:          Task Events:           System Events:
  agent_started          task_assigned          budget_warning
  agent_step_done        task_completed         budget_exceeded
  agent_tool_called      task_failed            provider_error
  agent_tool_result      task_retrying          provider_fallback
  agent_thinking         task_guardrail_fail    queue_backpressure
  agent_finished         task_human_input       worker_started
  agent_error            task_delegated         worker_stopped
```

- Events are streamed to the frontend via WebSocket
- Dashboard can filter, aggregate, and visualize events
- Monitoring/alerting based on events (e.g., budget_exceeded → notification)
- Audit trail: All events persisted for traceability

### GraphFlow / DAG Orchestration (from AutoGen)

For complex multi-agent workflows with conditional paths:

```
                    ┌─── success ──→ [Test Agent]
[Plan Agent] ──→ [Code Agent] ──┤
                    └─── failure ──→ [Debug Agent] ──→ [Code Agent]
                                                          (Cycle)
```

- Conditional edges based on agent output
- Parallel nodes (activation="any" for race, activation="all" for join)
- Cycle support with exit conditions (max_iterations, success_condition)
- DiGraphBuilder API for fluent graph construction
- Visualizable in the frontend as an interactive DAG editor

### Termination Conditions (from AutoGen)

Flexible, composable stop conditions for agent workflows:

```python
# Composable with & (AND) and | (OR)
stop = (MaxSteps(50)
        | BudgetExceeded(max_cost=5.0)
        | TextMention("TASK_COMPLETE")
        | Timeout(minutes=30))
        & NotCondition(StallDetected())
```

Available conditions:
- MaxSteps, MaxMessages, MaxTokens
- BudgetExceeded (cost limit)
- TextMention (specific text in output)
- Timeout (wall-clock-based)
- StallDetected (no progress)
- FunctionCallResult (specific tool result)
- Custom (arbitrary predicate function)

### Component System / Declarative Configuration (from AutoGen)

Agents, tools, and workflows are JSON/YAML serializable and
reconstructable without code changes:

```json
{
  "provider": "codeforge.agents.CodeReviewAgent",
  "version": 1,
  "config": {
    "llm": "claude-sonnet-4-20250514",
    "tools": ["git_diff", "file_read", "lint"],
    "guardrail": "code_quality",
    "max_iterations": 10,
    "budget_limit": 2.0
  }
}
```

- Essential for the GUI workflow editor
- Agents/workflows can be saved, shared, and versioned
- Schema versioning with migration support
- Import/export of agent configurations

### Document Pipeline PRD→Design→Tasks→Code (from MetaGPT)

For complex features: Structured intermediate artifacts instead of direct code generation:

```
1. Requirement → Structured PRD (JSON)
     User stories, acceptance criteria, scope
2. PRD → System Design (JSON + Mermaid)
     Data structures, API specification, class diagram
3. Design → Task List (JSON)
     Ordered list of files to create with dependencies
4. Tasks → Code (per file)
     Context: Design + other already created files
5. Code → Review + Tests
     Automatic validation against design specification
```

- Each intermediate document is schema-validated (ActionNode)
- Reduces hallucination through structured constraints
- Incremental development: Existing code is taken into account
- Intermediate documents are visible and editable in the GUI

### MagenticOne Planning Loop (from AutoGen)

For complex, long-lived tasks: Adaptive planning with stall detection:

```
1. PLAN    → Orchestrator creates initial plan
2. EXECUTE → Agent works through next step
3. CHECK   → Evaluate progress:
               - Progress? → Continue with 2
               - Stall?    → Re-planning (back to 1)
               - Done?     → Deliver result
               - Failed?   → Fact gathering, then re-planning
```

- Stall detection recognizes when agents are going in circles
- Re-planning adjusts the plan based on previous results
- Fact gathering collects missing information before a new plan
- Progress tracking via a ledger (progress protocol)

### HandoffMessage Pattern (from AutoGen)

Agents explicitly hand off tasks to specialists:

```
[Planner Agent]
    → HandoffMessage(target="coder", context="Implement feature X per plan")
        → [Code Agent]
            → HandoffMessage(target="reviewer", context="Review changes in src/")
                → [Review Agent]
                    → HandoffMessage(target="tester", context="Run test suite")
                        → [Test Agent]
```

- Explicit handoff with context (not blind forwarding)
- Agent decides itself who to hand off to
- Fits CodeForge's agent specialization (Planner, Coder, Reviewer, etc.)
- Works with different agent backends (Aider→OpenHands→SWE-agent)

### Human Feedback Provider Protocol (from CrewAI)

Extensible HITL channels via a provider interface:

```python
class HumanFeedbackProvider(Protocol):
    async def request_feedback(
        self, context: dict, options: list[str]
    ) -> FeedbackResult:
        ...
```

Implementations:
- **WebGuiProvider** — Feedback via the SolidJS web GUI (default)
- **SlackProvider** — Approval requests as Slack messages
- **EmailProvider** — Approval via email link
- **CliProvider** — Terminal input for development/debugging

## Coding Agent Insights: Adopted Patterns

From the deep analysis of Cline, Devika, OpenHands, SWE-agent, and Aider, the following
patterns were adopted for CodeForge. Detailed analysis: docs/research/market-analysis.md
and docs/research/aider-deep-analysis.md

### Shadow Git Checkpoints (from Cline)

Isolated git repository for safe rollback during agent execution:

- Before each agent action, a checkpoint is created in a shadow git repo
- On failure or user rejection: instant rollback to last good state
- Separate from the project's actual git history (no polluting commits)
- Complementary to the Sandbox mode's container isolation
- Integrated into the Safety Layer as the Rollback component

### Event-Sourcing Architecture (from OpenHands)

All agent activities are recorded as an append-only event stream:

```
EventStream: Agent actions, observations, thoughts, tool results
     │
     ├── Replay: Reconstruct any point in time
     ├── Audit: Complete traceability of all actions
     ├── Debug: Step through failed runs
     └── Persist: Events stored for trajectory recording
```

- Central abstraction for agent execution
- All components communicate through events (not direct calls)
- Enables the Trajectory Recording system
- Frontend receives events via WebSocket for live visualization

### Microagents (from OpenHands)

Small, trigger-driven agents defined in YAML+Markdown:

```yaml
# .codeforge/microagents/fix-imports.yaml
name: fix-imports
trigger: "import error"          # Triggered when pattern appears in output
type: knowledge                  # knowledge | repo | task
prompt: |
  When you see import errors in Python, check:
  1. Is the package in pyproject.toml?
  2. Is the import path correct?
  3. Run: poetry install
```

- Three types: knowledge (factual), repo (project-specific), task (action)
- Auto-injected into agent context when trigger matches
- Lightweight alternative to full agent modes for simple patterns
- User-definable in `.codeforge/microagents/`

### Diff-based File Review (from Cline)

Before applying changes, the user sees a side-by-side diff:

- Agent proposes changes as a unified diff
- Frontend renders before/after with syntax highlighting
- User can accept, reject, or edit individual hunks
- Integrated into the Plan → Approve → Execute workflow
- At autonomy level 3+, diffs are auto-approved for file edits

### Stateless Agent Design (from Devika)

Agent processes are stateless — all state lives in the Go Core:

- Agent receives full context (task, repo info, history) per invocation
- No persistent agent processes between tasks
- State transitions tracked in the core service via database
- Enables horizontal scaling: any worker can pick up any task
- Agent State Visualization in frontend reads from core, not from agents

### ACI — Agent-Computer Interface (from SWE-agent)

Shell commands optimized for LLM agents (not for humans):

- `open <file> [line]` instead of complex `vim`/`cat` invocations
- `edit <start>:<end> <content>` instead of sed/awk
- `search_dir <pattern> [dir]` instead of `grep -r`
- `find_file <name> [dir]` instead of `find`
- Reduces error rate by providing LLM-friendly abstractions
- Implemented as YAML tool bundles (see Tool Definitions section)

### tree-sitter Repo Map (from Aider)

Semantic code map generated via tree-sitter parsing:

- Extracts class/function/method definitions from all files
- Ranked by relevance to current task (PageRank on call graph)
- Provides codebase overview without sending all file contents
- Reduces token usage while maintaining context quality
- Part of the Context Layer (GraphRAG): complements vector search

### Architect/Editor Pattern (from Aider)

Separate LLM roles for planning and implementation:

- **Architect model** (strong reasoning, e.g., Claude Opus): analyzes codebase, creates plan
- **Editor model** (fast coding, e.g., Claude Sonnet): implements the plan
- Maps directly to CodeForge's Modes System: `architect` → `coder` pipeline
- Cost optimization: expensive model only for planning, cheaper model for execution
- Configurable via mode pipelines in task YAML

### Edit Formats (from Aider)

Multiple output formats for different LLM capabilities:

| Format | When | How |
|---|---|---|
| **whole-file** | Small files, local models | LLM outputs complete file |
| **diff** | Standard edits, capable models | Unified diff format |
| **search/replace** | Precise edits | Search block → Replace block |
| **udiff** | Complex multi-file edits | Universal diff with context |

- Format selection based on LLM capability level and file size
- Automatic retry with simpler format on parse failure
- Integrated into the Execution Layer's tool provisioning

### Skills System (from OpenHands)

Reusable Python snippets automatically injected into agent context:

- Pre-built skills for common operations (file manipulation, git, testing)
- Skills are Python functions available in the agent's execution environment
- Automatically included in the prompt based on task context
- User-extensible: custom skills in `.codeforge/skills/`
- Complementary to YAML Tool Bundles (skills are code, bundles are declarations)

### Risk Management (from OpenHands)

LLM-based security analysis of agent actions:

- **InvariantAnalyzer**: Validates agent actions against security policies
- Checks for: path traversal, command injection, credential exposure
- Runs as a pre-execution filter in the Safety Layer
- Complements the Command Safety Evaluator with LLM-based reasoning
- Can be disabled for trusted agents to reduce latency

## Roadmap/Feature-Map: Auto-Detection & Adaptive Integration

### Core Principle

CodeForge automatically detects which spec-driven development tools, PM platforms, and
roadmap artifacts are used in a project, and offers appropriate integration.
**No proprietary PM tool** — instead, bidirectional sync with existing tools.

### Provider Registry for Specs and PM

Same architecture as `gitprovider` — new adapters only require a new package and
a blank import:

```
port/
  specprovider/
    provider.go        # Interface: Detect(), ReadSpecs(), WriteChange(), Watch()
    registry.go        # Register(), New(), Available()
  pmprovider/
    provider.go        # Interface: Detect(), SyncItems(), CreateItem(), Webhooks()
    registry.go        # Register(), New(), Available()

adapter/
  openspec/            # OpenSpec (openspec/ directory)
  speckit/             # GitHub Spec Kit (.specify/ directory)
  autospec/            # Autospec (specs/spec.yaml)
  plane/               # Plane.so (REST API v1)
  openproject/         # OpenProject (REST API v3)
  github_pm/           # GitHub Issues/Projects (REST + GraphQL)
  gitlab_pm/           # GitLab Issues/Boards (REST + GraphQL)
```

### Three-Tier Auto-Detection

```
┌─────────────────────────────────────────────────────────────┐
│                    Auto-Detection Engine                      │
│                                                             │
│  Tier 1: Spec-Driven Detectors (repo files)                 │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ OpenSpec  │ │ Spec Kit │ │ Autospec │ │ ADR/RFC  │      │
│  │openspec/ │ │.specify/ │ │specs/*.y │ │docs/adr/ │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
│                                                             │
│  Tier 2: Platform Detectors (API-based)                     │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ GitHub   │ │ GitLab   │ │ Plane.so │ │OpenProj. │      │
│  │Issues/PR │ │Issues/MR │ │REST API  │ │REST API  │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
│                                                             │
│  Tier 3: File-Based Detectors (simple markers)              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ROADMAP.md│ │TASKS.md  │ │backlog/  │ │CHANGELOG │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
└─────────────────────────────────────────────────────────────┘
```

Each detector implements the `specprovider.SpecProvider` or `pmprovider.PMProvider`
interface and registers itself via `init()`. The detection engine iterates over all
registered detectors and returns a list of detected tools.

### Spec Provider Interface

```go
type SpecProvider interface {
    // Detect checks if this spec format is present in the repo
    Detect(repoPath string) (bool, error)

    // ReadSpecs reads all specs from the repo
    ReadSpecs(repoPath string) ([]Spec, error)

    // WriteChange writes a change (delta format)
    WriteChange(repoPath string, change Change) error

    // Watch observes spec changes (for bidirectional sync)
    Watch(repoPath string, callback func(event SpecEvent)) error

    // Capabilities declares supported operations
    Capabilities() []Capability
}
```

### PM Provider Interface

```go
type PMProvider interface {
    // Detect checks if this PM platform is configured for the project
    Detect(projectConfig ProjectConfig) (bool, error)

    // SyncItems synchronizes items bidirectionally
    SyncItems(ctx context.Context, direction SyncDirection) (SyncResult, error)

    // CreateItem creates a new item on the platform
    CreateItem(ctx context.Context, item Item) (string, error)

    // RegisterWebhook registers a webhook for real-time sync
    RegisterWebhook(ctx context.Context, callbackURL string) error

    // Capabilities declares supported operations
    Capabilities() []Capability
}
```

### Bidirectional Sync

```
┌─────────────────┐              ┌─────────────────┐
│  CodeForge       │  ◄── Sync ──►  │  External PM     │
│  Roadmap Model   │              │  (Plane/GitHub/  │
│                  │              │   OpenProject)   │
│  Milestone       │  ←──────→   │  Initiative/     │
│  Feature         │  ←──────→   │  Epic/Issue      │
│  Task            │  ←──────→   │  Work Item       │
└────────┬────────┘              └─────────────────┘
         │
         │  ◄── Sync ──►
         │
┌────────▼────────┐
│  Repo Specs      │
│  (OpenSpec/       │
│   Spec Kit/       │
│   Autospec)       │
└─────────────────┘
```

- **Import:** PM tool → CodeForge roadmap model (issues/epics become features/tasks)
- **Export:** CodeForge → PM tool (new features are created as issues)
- **Bidirectional:** Changes synchronized in both directions
- **Conflict resolution:** Timestamp-based + user decision on conflicts
- **Sync triggers:** Webhook (real-time), poll (periodic), manual

### Roadmap Data Model

```go
// Internal roadmap model — PM adapters map to this format
type Milestone struct {
    ID          string
    Title       string
    Description string
    DueDate     time.Time
    Features    []Feature
    Status      MilestoneStatus  // planned, active, completed
    LockVersion int              // Optimistic Locking (from OpenProject)
}

type Feature struct {
    ID          string
    Title       string
    Description string
    Priority    Priority
    Tasks       []Task
    Labels      []string         // Label-triggered sync (from Plane)
    SpecRef     string           // Reference to spec file (openspec/specs/feature.md)
    ExternalIDs map[string]string // {"plane": "abc", "github": "123"}
}
```

### `/ai` Endpoint for LLM Consumption (from Ploi Roadmap)

```
GET /api/v1/projects/{id}/roadmap/ai?format=json
GET /api/v1/projects/{id}/roadmap/ai?format=yaml
GET /api/v1/projects/{id}/roadmap/ai?format=markdown
```

Provides the roadmap in an LLM-optimized format:
- Compact summary of all milestones, features, tasks
- Status information and dependencies
- Usable for AI agents that need to understand project context

### Directory Structure (Extension)

```
internal/
  port/
    specprovider/          # Spec detection interface
      provider.go          # SpecProvider Interface + Capabilities
      registry.go          # Register(), New(), Available()
    pmprovider/            # PM platform interface
      provider.go          # PMProvider Interface + Capabilities
      registry.go          # Register(), New(), Available()
  adapter/
    openspec/              # OpenSpec adapter (openspec/ directory)
    speckit/               # GitHub Spec Kit adapter (.specify/)
    autospec/              # Autospec adapter (specs/spec.yaml)
    plane/                 # Plane.so REST API v1 adapter
    openproject/           # OpenProject REST API v3 adapter
    github_pm/             # GitHub Issues/Projects adapter
    gitlab_pm/             # GitLab Issues/Boards adapter
  domain/
    roadmap/               # Roadmap domain (Milestone, Feature, Task)
  service/
    detection.go           # Auto-Detection Engine
    sync.go                # Bidirectional Sync Service
```

## LLM Integration: LiteLLM Proxy as Sidecar

### Architecture Decision

After analysis of LiteLLM, OpenRouter, Claude Code Router, and OpenCode CLI:
**CodeForge does not build its own LLM provider interface.** LiteLLM Proxy runs as a Docker sidecar
and provides a unified OpenAI-compatible API. Detailed analysis: docs/research/market-analysis.md

### Integration Architecture

```
┌─────────────────────────────────────────────────────┐
│                  TypeScript Frontend                 │
│                                                     │
│  ┌──────────────────────────────────────────────┐   │
│  │  Cost Dashboard  │  Provider Config UI       │   │
│  └──────────────────────────────────────────────┘   │
└────────────────────┬────────────────────────────────┘
                     │ REST / WebSocket
┌────────────────────▼────────────────────────────────┐
│                  Go Core Service                     │
│                                                     │
│  ┌──────────────┐  ┌──────────────┐                 │
│  │ LiteLLM      │  │ Scenario     │                 │
│  │ Config Mgr   │  │ Router       │                 │
│  └──────────────┘  └──────────────┘                 │
│  ┌──────────────┐  ┌──────────────┐                 │
│  │ User-Key     │  │ Local Model  │                 │
│  │ Mapping      │  │ Discovery    │                 │
│  └──────────────┘  └──────────────┘                 │
│  ┌──────────────┐                                   │
│  │ Copilot      │                                   │
│  │ Token Exch.  │                                   │
│  └──────────────┘                                   │
└────────────┬────────────────────────┬───────────────┘
             │ OpenAI-compatible API  │
             │ (Port 4000)            │
┌────────────▼────────────────────────┤
│      LiteLLM Proxy (Sidecar)       │
│                                     │
│  ┌──────────────┐  ┌────────────┐  │
│  │  Router      │  │  Budget    │  │
│  │  (6 Strat.)  │  │  Manager   │  │
│  └──────────────┘  └────────────┘  │
│  ┌──────────────┐  ┌────────────┐  │
│  │  Caching     │  │  Callbacks │  │
│  │  (Redis)     │  │(Prometheus)│  │
│  └──────────────┘  └────────────┘  │
└────────────┬────────────────────────┘
             │ Provider APIs
┌────────────▼────────────────────────────────────────┐
│  OpenAI │ Anthropic │ Ollama │ Bedrock │ OpenRouter  │
└─────────────────────────────────────────────────────┘
```

### What LiteLLM Provides (not built by us)

| Feature | LiteLLM Mechanism |
|---|---|
| Provider abstraction | 127+ providers, unified API |
| Routing | 6 strategies: latency, cost, usage, least-busy, shuffle, tag-based |
| Fallbacks | Fallback chains with cooldown (60s default) |
| Cost tracking | Per call, per model, per key via pricing DB (36,000+ entries) |
| Budgets | Per key, per team, per user, per provider limits |
| Streaming | `CustomStreamWrapper` normalizes all providers to OpenAI SSE |
| Tool calling | Unified via `tools` parameter, provider conversion automatic |
| Structured output | `response_format` cross-provider (native or via tool-call fallback) |
| Caching | In-memory, Redis, semantic (Qdrant), S3, GCS |
| Observability | 42+ integrations (Prometheus, Langfuse, Datadog, etc.) |
| Rate limiting | Per-key TPM/RPM, per-team, per-model |

### What CodeForge Builds (Custom Development)

| Component | Layer | Description |
|---|---|---|
| **LiteLLM Config Manager** | Go Core | Generates `litellm_config.yaml` from CodeForge DB. CRUD for models, deployments, keys. |
| **User-Key Mapping** | Go Core | CodeForge user → LiteLLM Virtual Keys. API keys stored securely in CodeForge DB, forwarded to LiteLLM. |
| **Scenario Router** | Go Core | Task type → LiteLLM tag. `metadata.tags: ["think"]` in request → LiteLLM routes to matching deployment. |
| **Cost Dashboard** | Frontend | Query LiteLLM Spend API (`/spend/logs`, `/global/spend/per_team`). Visualization per project/user/agent. |
| **Local Model Discovery** | Go Core | Query Ollama (`/api/tags`) and LM Studio (`/v1/models`) endpoints. Discovered models automatically added to LiteLLM config. |
| **Copilot Token Exchange** | Go Core | Read GitHub OAuth token from `~/.config/github-copilot/hosts.json`, exchange for bearer token via `api.github.com/copilot_internal/v2/token`. |

### Scenario-Based Routing

Inspired by Claude Code Router. Different task types are automatically routed to
matching models via LiteLLM's tag-based routing:

```yaml
# litellm_config.yaml (generated by Go Core)
model_list:
  - model_name: default
    litellm_params:
      model: anthropic/claude-sonnet-4-20250514
      api_key: os.environ/ANTHROPIC_API_KEY
      tags: ["default", "review"]

  - model_name: background
    litellm_params:
      model: openai/gpt-4o-mini
      api_key: os.environ/OPENAI_API_KEY
      tags: ["background"]

  - model_name: think
    litellm_params:
      model: anthropic/claude-opus-4-20250514
      api_key: os.environ/ANTHROPIC_API_KEY
      tags: ["think", "plan"]

  - model_name: longcontext
    litellm_params:
      model: google/gemini-2.0-pro
      api_key: os.environ/GEMINI_API_KEY
      tags: ["longContext"]

  - model_name: local
    litellm_params:
      model: ollama/llama3
      api_base: http://ollama:11434
      tags: ["background", "default"]

router_settings:
  routing_strategy: "tag-based-routing"
  num_retries: 3
  fallbacks:
    - default: ["local"]
    - think: ["default"]
```

| Scenario | When | Typical Models |
|---|---|---|
| `default` | General coding tasks | Claude Sonnet, GPT-4o |
| `background` | Batch, index, embedding | GPT-4o-mini, DeepSeek, local |
| `think` | Architecture, debugging, complex logic | Claude Opus, o3 |
| `longContext` | Input > 60K tokens | Gemini Pro (1M context) |
| `review` | Code review, quality check | Claude Sonnet |
| `plan` | Feature planning, design documents | Claude Opus |

### LiteLLM Proxy Configuration

```yaml
# docker-compose.yml (excerpt)
services:
  litellm:
    image: docker.litellm.ai/berriai/litellm:main-stable
    ports:
      - "4000:4000"
    volumes:
      - ./litellm_config.yaml:/app/config.yaml
    command: ["--config", "/app/config.yaml", "--port", "4000"]
    environment:
      - LITELLM_MASTER_KEY=${LITELLM_MASTER_KEY}
      - DATABASE_URL=postgresql://codeforge:${POSTGRES_PASSWORD}@postgres:5432/codeforge?schema=litellm
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:4000/health/liveliness"]
```

### Frontend Directory Structure (SolidJS)

```
frontend/
  src/
    features/            # Feature modules (dashboard, roadmap, agents, llm)
    shared/              # Shared components, primitives, utils
    api/                 # API client, WebSocket handler
```
