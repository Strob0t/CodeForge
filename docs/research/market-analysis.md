# CodeForge -- Market Analysis & Research

> As of: 2026-02-16

### Project Vision

Containerized service with web GUI for orchestrating AI coding agents. Core features:

- Project Dashboard (SVN/Git/GitHub/GitLab/local)
- Roadmap/Feature Map Management (in repo or in service)
- Multi-LLM Provider Management (OpenAI, Claude, local models, etc.)
- Agent orchestration for code work

---

### 1. Direct Competitors

#### BjornMelin/CodeForge AI

- URL: [https://github.com/BjornMelin/codeforge](https://github.com/BjornMelin/codeforge)
- Description: Multi-agent orchestration via LangGraph with Dynamic Model Routing (Grok, Claude, Gemini), GraphRAG+ Retrieval (Qdrant + Neo4j), Debate Framework for architecture decisions.
- Stack: LangGraph 0.5.3+, Qdrant, Neo4j, Redis, Docker Compose, Python 3.12+
- Status: Phase 1/MVP, 28 commits
- **Gaps:** No web GUI for project management, no SCM integration, no roadmap feature

#### OpenHands (formerly OpenDevin) -- Deep Analysis

- URL: [https://github.com/OpenHands/OpenHands](https://github.com/OpenHands/OpenHands)
- Website: [https://openhands.dev/](https://openhands.dev/)
- Paper (ICLR 2025): [https://arxiv.org/abs/2407.16741](https://arxiv.org/abs/2407.16741)
- V1 SDK Paper: [https://arxiv.org/html/2511.03690v1](https://arxiv.org/html/2511.03690v1)
- Stars: 65,000+ | License: MIT (Core), Source-available (Enterprise) | Language: Python (Backend), TypeScript/React (Frontend)
- Description: Open-source AI-driven development platform. Closest direct competitor to CodeForge. Web GUI, CLI, REST+WebSocket API. Docker/Kubernetes deployment. GitHub/GitLab/Bitbucket/Forgejo/Azure DevOps integration. Model-agnostic via LiteLLM (100+ providers). ICLR 2025 paper. V0 to V1 architecture migration ongoing.

**1. Architecture & Tech Stack:**

```text
React Frontend (Remix SPA + Vite + TypeScript + Tailwind CSS + Redux + TanStack Query)
        |
        v  REST / WebSocket (FastAPI)
Python Backend (FastAPI, EventStream, AgentController, Session Management)
        |
        v  Docker API / SSH / HTTP
Docker Sandbox (Agent Runtime: Bash, IPython, Browser, File Editor)
```

| Layer | Technology | Purpose |
|---|---|---|
| Frontend | React + Remix SPA + Vite + TypeScript | Web GUI with Redux + TanStack Query state |
| Backend | Python + FastAPI | HTTP/WS server, session management, agent lifecycle |
| Agent Runtime | Python (openhands.sdk) | Agent loop, LLM calls, action/observation processing |
| Sandbox | Docker container (per session) | Isolated code execution (Bash, IPython, Browser) |
| LLM | LiteLLM | Multi-provider abstraction (100+ models) |
| Storage | FileStore (Local/S3/GCS/InMemory) | Conversation persistence, state, events |

V0 vs. V1 Architecture Evolution: V0 (Legacy) was monolithic, sandbox-centric, tight coupling between agent and sandbox, SSH-based communication, 140+ config fields in 15 classes (2,800 lines). V1 (Current) is a modular SDK with clear package boundaries, opt-in sandboxing, workspace abstraction, event sourcing, immutable config via Pydantic, REST+WebSocket server built-in.

**V1 Package Structure:**

| Package | Purpose |
|---|---|
| `openhands.sdk` | Core abstractions: Agent, Conversation, LLM, Tool, Event System |
| `openhands.tools` | Concrete tool implementations |
| `openhands.workspace` | Execution environments: Local, Docker, API-Remote |
| `openhands.agent_server` | REST/WebSocket API server for remote execution |

**2. Core Concepts:**

| Concept | Description |
|---|---|
| Agent | Examines current state, produces actions to make progress. Various implementations in AgentHub. |
| AgentController | Initializes agent, manages state, drives the agent loop incrementally. |
| State | Data structure with task info, step count, event history, planning data, LLM costs, delegation metadata. |
| EventStream | Central communication hub. Publish/subscribe for actions and observations. Backbone of all interactions. |
| Action | Agent request: shell command, Python code, browser navigation, file edit, agent delegation, message. |
| Observation | Environment feedback: command output, file content, browser state, error messages. |
| Runtime | Executes actions, produces observations. Sandbox handles commands in Docker containers. |
| Session | Holds exactly one EventStream, one AgentController, one Runtime. Represents a task. |
| ConversationManager | Manages active sessions, routes requests to the correct session. |
| Workspace | V1 abstraction: `LocalWorkspace` (in-process), `RemoteWorkspace` (HTTP), `DockerWorkspace` (container). |
| Conversation | Factory pattern: `LocalConversation` or `RemoteConversation` depending on workspace type. |

Agent Loop (Pseudocode):

```python
while True:
    prompt = agent.generate_prompt(state)
    response = llm.completion(prompt)
    action = agent.parse_response(response)
    observation = runtime.run(action)
    state = state.update(action, observation)
```

Data Flow:

```text
Agent -> Actions -> AgentController -> EventStream -> Runtime
Runtime -> Observations -> EventStream -> AgentController -> State -> Agent
```

**3. Agent System:**

Agent Types (AgentHub):

| Agent | Type | Description |
|---|---|---|
| CodeActAgent | Generalist (Default) | Write code, debug, Bash/Python/Browser/File-Edit. Multi-turn CodeAct framework. |
| BrowsingAgent | Specialist | Web navigation, forms, buttons, complex browser interaction. |
| Delegator Agent | Coordinator | Delegates tasks to sub-agents (RepoStudyAgent, VerifierAgent, etc.). |
| GPTSwarm Agent | Graph-based | Optimizable graphs for agent systems, modular nodes and edges. |
| Micro Agents / Skills | Specialized | Lightweight agents for specific tasks, configured via Markdown + YAML. |

Multi-Agent Delegation: `AgentDelegateAction` enables hierarchical agent structures. CodeActAgent can delegate web tasks to BrowsingAgent. Sub-agents operate as independent conversations with inherited config and workspace.

Skills / Microagents: Specialized prompts with domain knowledge, stored as Markdown + YAML frontmatter. Three trigger types: `always` (always active), `keyword` (on keyword), `manual` (user-controlled). Storage location: `.openhands/skills/`, repo root (AGENTS.md, .cursorrules), or global registry. MCP integration: Skills can reference MCP servers for additional tools. Interoperability: Reads `.cursorrules`, `CLAUDE.md`, `copilot-instructions.md`, `AGENTS.md`.

**4. Runtime & Sandbox:**

Docker Sandbox (Default): Each session runs in its own Docker container. Full OS capabilities, isolated from host. SSH-mediated interface (V0) / HTTP-based (V1). Container is destroyed after session (filesystem integrity).

Workspace mounting for project-specific files. Resource access policies: Only task-relevant files exposed.

V1 Workspace Abstraction:

| Workspace Type | Execution | Use Case |
|---|---|---|
| LocalWorkspace | In-process, direct host access | Quick prototyping, development |
| DockerWorkspace | Container with resource isolation | Production, multi-tenancy |
| APIRemoteWorkspace | HTTP delegation to agent server | Cloud deployment, SaaS |

Factory Pattern: `Workspace(working_dir="/path")` creates a LocalWorkspace, `Workspace(host="...", runtime="...")` creates a RemoteWorkspace.

E2B Integration: Legacy V0 supported cloud sandbox via E2B (open-source secure environments). V1 supports E2B via workspace abstraction.

Production Observability: VNC Desktop for real-time GUI access to agent filesystem and processes. VSCode Web for embedded editor in workspace. Chromium Browser for non-headless browser access (agent sees what user sees).

**5. LLM Integration:**

Architecture: LiteLLM as backbone for 100+ providers (OpenAI, Anthropic, Gemini, Bedrock, Ollama, vLLM, etc.). Unified `LLM` class in SDK, encapsulates chat and completion APIs. Support for reasoning models (ThinkingBlock for Anthropic, ReasoningItemModel for OpenAI).

Multi-LLM Routing (RouterLLM): `RouterLLM` subclass with custom `select_llm()` method. Per-invocation LLM selection (e.g., text to cheaper model, images to multimodal model). Fallback/ensemble logic configurable.

Non-Native Tool Calling: `NonNativeToolCallingMixin` uses text-based prompts + regex parsing for models without function calling. This expands the usable model universe beyond peer SDKs.

Configuration: Settings via UI or `config.toml` (under `[llm]`). Retry behavior configurable: `num_retries`, `retry_min_wait`, `retry_max_wait`, `retry_multiplier`. Automatic rate-limit retries (HTTP 429). Custom tokenizers for specialized models.

**Verified Models (SWE-bench Verified, as of 2025):**

| Model | Resolution Rate |
|---|---|
| Claude Sonnet 4.5 | 72.8% |
| GPT-5 (reasoning=high) | 68.8% |
| Claude Sonnet 4 | 68.0% |
| Qwen3 Coder 480B | 65.2% |

**6. Plugin/Extension System:**

Plugin 1.0 Architecture:

```text
plugin/
  ├── skills/          # Skill directories with SKILL.md
  ├── runtime.json     # Custom container extensions (planned)
  └── .mcp.json        # MCP server definitions
```

Metadata for marketplace GUI (name, version, description, author). Plugin loading via API: Frontend calls `app-conversations` API with plugin spec. Plugins are loaded and executed in the agent server (sandbox).

MCP (Model Context Protocol) Integration: SSE, SHTTP, and stdio protocols supported. External tool servers register tools with the agent. Configuration via UI (Settings > MCP) or `config.toml` under `[mcp]`. Skills can reference MCP servers for extended capabilities.

**Tool System (Action-Execution-Observation):**

| Component | Description |
|---|---|
| Action | Input schema validated against Pydantic model before execution |
| Execution | `ToolExecutor` executes validated action |
| Observation | Structured result with LLM-compatible format |
| Tool Registry | Decouples specs from implementation, lazy instantiation, distributed execution |

**Risk Management:** Each tool action gets a risk level: LOW / MEDIUM / HIGH / UNKNOWN. `LLMSecurityAnalyzer` has the LLM evaluate security risk of each action. `ConfirmationPolicy` requires confirmation for actions above user threshold. `SecretRegistry` masks credentials in logs and LLM context. Auto-detection of secrets in bash commands and outputs.

**7. API (REST + WebSocket):**

REST Endpoints:

| Endpoint | Method | Description |
|---|---|---|
| `/workspaces` | POST | Create workspace |
| `/workspaces/{id}` | GET/DELETE | Workspace info / delete |
| `/workspaces/{id}/execute` | POST | Execute command |
| `/conversations` | POST | Create conversation |
| `/conversations/{id}` | GET | Retrieve conversation |
| `/conversations/{id}/messages` | POST | Send message |
| `/conversations/{id}/stream` | GET (WS) | Response streaming |
| `/api/user/repositories` | GET | List user repos |
| `/api/user/search/repositories` | GET | Search repos |
| `/api/user/repository/branches` | GET | List branches |
| `/api/user/repository/{name}/microagents` | GET | Microagent metadata |
| `/api/user/suggested-tasks` | GET | PRs and assigned issues |

WebSocket: Bidirectional real-time communication during active session. Messages: Agent actions, observations, errors, state updates. Streaming of agent events to frontend.

Authentication: JWT-based for session identification. FastAPI dependency injection for route protection. OAuth 2.0 for Git provider tokens.

**8. State Management & Persistence:**

Event-Sourcing Pattern: Immutable event hierarchy: `Event` -> `LLMConvertibleEvent` -> `ActionEvent` / `ObservationBaseEvent`. `ConversationState` as single source of truth (mutable metadata + append-only EventLog). Thread-safe updates via FIFO locking. Deterministic replay possible.

Persistence (Dual-Path): Metadata stored in `base_state.json` (serialized). Events stored as individual JSON files (incremental, efficient).

FileStore Backends:

| Backend | Description |
|---|---|
| `InMemoryFileStore` | Ephemeral storage (tests, prototyping) |
| `LocalFileStore` | Local filesystem |
| `S3FileStore` | Amazon S3 cloud storage |
| `GoogleCloudFileStore` | Google Cloud Storage |

Conversation State includes: Message history (complete event log), agent configuration (LLM settings, tools, MCP servers), execution state (agent status, iteration count), tool outputs, statistics (LLM usage metrics), workspace context, activated skills.

**9. GitHub/GitLab Integration:**

Supported Providers:

| Provider | Features | Auth URL Format |
|---|---|---|
| GitHub | Repos, Issues, PRs, Installations, Microagent Discovery | `https://{token}@{domain}/{repo}.git` |
| GitLab | Repos, Issues, MRs, Self-Hosted | `https://oauth2:{token}@{domain}/{repo}.git` |
| Bitbucket | Repos, Issues, Workspaces | `https://{username}:{password}@{domain}/{repo}.git` |
| Forgejo | Repos, Issues (GitHub-compatible API) | `https://{token}@{domain}/{repo}.git` |
| Azure DevOps | Repos, Issues, Organizations | URL-encoded org/project as username |

ProviderHandler Architecture: Central orchestration of all Git provider interactions. `GitService` protocol: All providers implement common interface. Mixin-based architecture for provider-specific operations. Fallback pattern: Operations are tried sequentially across all providers. Custom service classes via environment variables (`OPENHANDS_GITHUB_SERVICE_CLS`, etc.).

GitHub-specific: GitHub App Installation (OpenHands as GitHub App). Issue labeling with `@openhands` tag triggers the agent to work autonomously on an issue. Automatic PR creation on resolved issue. Comment with summary and PR link.

Microagent Discovery: Checks `.cursorrules` in repo root. Checks `.openhands/microagents/` directory. Loads and parses microagent files via `BaseMicroagent.load()`.

MCP Integration for SCM: PR/MR creation via MCP tools. Automatic links back to OpenHands conversations in PR descriptions.

**10. Strengths (what CodeForge should learn from):**

| Strength | Detail | Relevance for CodeForge |
|---|---|---|
| Event-Sourcing + Deterministic Replay | Immutable events, crash recovery, time-travel debugging | Adopt for agent trajectory recording and audit trail |
| Workspace Abstraction | Same agent code executable locally and remotely | Model for agent execution modes (Sandbox/Mount/Hybrid) |
| Multi-Provider Git Integration | 5 providers with common interface + custom classes | Provider Registry Pattern confirmed, add Forgejo/Azure DevOps |
| MCP as Extension Standard | Native MCP integration for tools and skills | Adopt MCP servers as part of the tool system |
| LLM Security Analyzer | LLM evaluates risk of each agent action | Additional analysis layer for Command Safety Evaluator |
| Skills/Microagent System | YAML+Markdown, trigger-based, repo-specific | For agent specialization and task-specific prompts |
| Non-Native Tool Calling | Models without function calling still usable | Directly relevant for LLM Capability Levels (pure completion models) |
| RouterLLM | Per-invocation model selection | Confirms scenario-based routing approach |
| Plugin Marketplace (planned) | Skills + MCP + Runtime Extensions as package | Inspiration for CodeForge community extensions |
| Stuck Detection | Detection of infinite loops and redundant calls | Adopt for agent workflow quality layer |
| Benchmark Evaluation | 15+ benchmarks, SWE-bench state-of-the-art | Evaluation framework for own agent quality measurement |
| 3-Tier Testing | Programmatic (commit) + LLM integration (daily) + benchmark (on-demand) | Adopt testing strategy for Python Workers |
| Context Condensation | Automatic context window optimization | For History Processors / context window management |
| Pause/Resume | State persistence with event sourcing | For long-running agent tasks and Plan->Approve workflow |

**11. Weaknesses / Gaps that CodeForge fills:**

| Weakness | Detail | CodeForge Solution |
|---|---|---|
| No Roadmap/Feature Map Management | No visual roadmap, no spec tracking, no feature planning | Roadmap/Feature Map as core pillar with auto-detection + multi-format support |
| No Multi-Project Dashboard | One project per session, no cross-project management | Project dashboard for multiple repos simultaneously |
| No SVN Support | Only Git-based SCMs (GitHub, GitLab, Bitbucket, Forgejo, Azure DevOps) | SVN as first-class SCM provider |
| Single-Task Focus | One session = one task, no orchestration of multiple parallel tasks | Multi-agent orchestration across multiple tasks/repos |
| Python-only Backend | Entire core in Python (performance limitation at high concurrency) | Go Core for HTTP/WS/scheduling, Python only for AI work |
| No Scenario-based Routing | RouterLLM exists, but no task-type-based model selection | Scenario routing via LiteLLM tags (default/background/think/longContext/review/plan) |
| No PM Tool Integration | No sync with Plane, OpenProject, Jira, Linear (only planned) | Bidirectional sync with PM tools as core pillar |
| No Spec-Driven Development | No auto-detection of OpenSpec, Spec Kit, Autospec | Three-tier auto-detection + multi-format SDD support |
| High LLM Costs | Frontier models required, looping behavior drives costs | Budget enforcement per task/project/user, Experience Pool for caching |
| Ambiguity Problem | Poor performance with vague requirements without clear specs | Document pipeline PRD->Design->Tasks->Code reduces ambiguity |
| No ADR/RFC Support | No detection/integration of architecture decisions | Auto-detection of docs/adr/, docs/rfcs/ |
| No Feature Flag Integration | No knowledge about feature rollout status | Integration with Unleash, OpenFeature, Flagsmith |
| Limited Agent Variety | Only own agents (CodeAct, Browsing, Micro), no external agents | Integration of Aider, OpenHands, SWE-agent as interchangeable backends |
| No Gitea/Forgejo as SCM Adapter | Forgejo only as Git provider, not as PM tool adapter | Gitea/Forgejo Issues/Boards as PM sync target |
| No Desktop IDE Integration | Only web GUI, no VS Code extension, no JetBrains plugin | Connectable via MCP Server (planned) |

**12. Adopted Patterns for CodeForge:**

| Pattern | Source in OpenHands | Implementation in CodeForge |
|---|---|---|
| Event-Sourcing State Model | ConversationState + EventLog | Trajectory recording with immutable events and replay |
| Workspace Factory Pattern | `Workspace()` -> Local/Docker/Remote | Agent execution modes: Sandbox/Mount/Hybrid with factory |
| Action-Execution-Observation | Tool system with Pydantic validation | Tool bundles with schema validation in YAML |
| GitService Protocol + ProviderHandler | Mixin-based multi-provider Git integration | Provider Registry Pattern (same architecture, extended with SVN) |
| Skills with YAML Frontmatter + Triggers | Microagents with always/keyword/manual trigger | Agent specialization as YAML with trigger configuration |
| LLM Security Analyzer | Risk level per action + ConfirmationPolicy | Command Safety Evaluator with risk levels + approval flow |
| SecretRegistry + Auto-Masking | Credentials masked in logs and LLM context | Secret masking in agent execution and trajectory logs |
| Stuck Detection | Detection of redundant calls and loops | Quality layer with loop detection and re-planning (MagenticOne) |
| Context Condensation | Automatic context window optimization | History Processors Pipeline (Buffered/TokenLimited/HeadAndTail) |
| Multi-Format Prompt Interop | Reads .cursorrules, CLAUDE.md, AGENTS.md | Context file interoperability for all supported formats |
| Non-Native Tool Calling | Text prompts + regex for models without function calling | LLM Capability Levels: Pure completion -> everything (Context, Tools, Quality) |

**13. Explicitly NOT Adopted:**

| Concept | Reason |
|---|---|
| Python-only Backend | Go Core for performance, Python only for AI-specific work |
| React Frontend | SolidJS chosen (lighter, more performant, no VDOM) |
| Redux + TanStack Query | SolidJS has its own reactive primitives |
| FastAPI as HTTP Server | Go net/http for Core (performance, concurrency) |
| SSH-based Sandbox Communication | Message queue (NATS/Redis) between Go Core and Python Workers |
| Single-Session Architecture | Multi-project dashboard with parallel sessions |
| Event-Sourcing as only persistence mechanism | Additionally PostgreSQL for structured data (projects, users, config) |
| Plugin Marketplace Approach | Focus on Provider Registry Pattern + YAML-based extensibility |

#### Open SWE (LangChain)

- URL: [https://github.com/langchain-ai/open-swe](https://github.com/langchain-ai/open-swe)
- Description: Cloud-based async coding agent. Understands codebases, plans solutions, creates PRs automatically.
- Strengths: GitHub integration, async workflows
- **Gaps:** No multi-provider LLM management, no roadmap feature, no self-hosting focus

#### Codel

- URL: [https://github.com/semanser/codel](https://github.com/semanser/codel)
- Stars: ~2,400 | License: AGPL-3.0 | Language: Go (Backend), React (Frontend)
- Description: Fully autonomous AI agent with terminal, browser, and editor in sandboxed Docker environment. Modern self-hosted web UI, persistent history in PostgreSQL, automatic Docker image selection based on user tasks.
- Core Features: Built-in browser for web research during tasks, built-in text editor with file change visualization, Smart Docker Image Picker (task-based selection), self-contained sandboxed execution.
- Relevance for CodeForge: High as architecture reference. Architecturally very close to CodeForge: Go backend, web GUI, Docker sandbox. Missing: Multi-project, roadmap, multi-agent orchestration.

#### AutoForge

- URL: [https://github.com/AutoForgeAI/autoforge](https://github.com/AutoForgeAI/autoforge)
- Stars: ~1,600 | Language: Python (Agent), React (UI)
- Description: Long-running autonomous coding agent based on Claude Agent SDK. Builds complete applications across multiple sessions via two-agent pattern (Initializer + Coding Agent). React-based UI for real-time monitoring.
- Core Features: Two-agent architecture (Initializer generates feature test cases, Coding Agent implements), multi-session design (tasks over hours/multiple sessions), Claude, GLM (Zhipu AI), Ollama, Kimi (Moonshot), Custom Providers.
- **Relevance for CodeForge:** Medium. Multi-session, test-first approach as pattern reference for CodeForge's Plan->Approve->Execute->Review->Deliver workflow.

#### bolt.diy (Community Fork of Bolt.new)

- URL: [https://github.com/stackblitz-labs/bolt.diy](https://github.com/stackblitz-labs/bolt.diy)
- Stars: ~19,000 | License: MIT (WebContainers API requires commercial license for production) | Language: TypeScript (Remix)
- Description: Official open-source fork of Bolt.new. Prompt, run, edit, and deploy full-stack web apps with any LLM. 19+ AI provider integrations (OpenAI, Anthropic, Ollama, OpenRouter, Gemini, LM Studio, Mistral, DeepSeek, etc.).
- Core Features: In-browser full dev environment (filesystem, Node server, terminal, package manager via WebContainers), 19+ LLM providers, MCP integration, Git integration, Diff View, Expo App Creation for React Native, Electron Desktop App option.
- Relevance for CodeForge: Medium. Targets "vibe coding" and app creation, not multi-project management or agent orchestration. Multi-LLM provider architecture as reference.

#### Dyad

- URL: [https://github.com/dyad-sh/dyad](https://github.com/dyad-sh/dyad)
- Stars: ~16,800 | License: Apache 2.0 (with `src/pro` directory excluded) | Language: TypeScript (Electron)
- Description: Local, open-source AI app builder (v0/Lovable/Replit/Bolt alternative). Everything runs locally on the user's machine. Real-time previews, instant undo.
- Core Features: Fully local execution (nothing leaves the machine), multi-model support (OpenAI, Google, Anthropic, free models), real-time previews, instant undo, responsive workflows.
- Relevance for CodeForge: Low-Medium. Focus on app creation from scratch, not multi-project or agent orchestration. Local-first philosophy fits CodeForge's self-hosted approach.

#### CLI Agent Orchestrator (CAO) -- AWS

- URL: [https://github.com/awslabs/cli-agent-orchestrator](https://github.com/awslabs/cli-agent-orchestrator)
- Stars: ~210 | License: Apache 2.0 | Language: Python
- Description: AWS-backed open-source multi-agent orchestration framework. Transforms developer CLI tools (Amazon Q CLI, Claude Code, etc.) into a hierarchical multi-agent system. Supervisor agent coordinates specialized worker agents in isolated tmux sessions.
- Core Features: Hierarchical Supervisor/Worker Agent Pattern, isolated tmux sessions per agent with MCP communication, three orchestration patterns (Handoff synchronous, Assign async parallel, Send Message direct), Flow Scheduling (cron-like) for unattended automatic execution, Claude Code, Amazon Q CLI (planned: Codex CLI, Gemini CLI, Aider), fully local execution.
- **Relevance for CodeForge:** Very high -- competitor AND architecture reference. Hierarchical multi-agent orchestration via tmux/MCP is directly comparable to CodeForge's agent orchestration layer. Supervisor/Worker pattern, isolated sessions, support for multiple CLI agents. Cron-based flow scheduling relevant. Missing: Web GUI, project dashboard, roadmap features.

---

### 2. AI Coding Agents (Partial Overlap)

#### SWE-agent -- Deep Analysis

- URL: [https://github.com/SWE-agent/SWE-agent](https://github.com/SWE-agent/SWE-agent)
- Paper (NeurIPS 2024): [https://arxiv.org/abs/2405.15793](https://arxiv.org/abs/2405.15793)
- Mini-SWE-Agent: [https://github.com/SWE-agent/mini-swe-agent](https://github.com/SWE-agent/mini-swe-agent)
- Stars: ~15,000+ | License: MIT | Language: Python
- Authors: John Yang, Carlos E. Jimenez, Alexander Wettig, Kilian Lieret, Shunyu Yao, Karthik Narasimhan, Ofir Press (Princeton / Stanford)
- Status: Active, v1.0 release, NeurIPS 2024 paper

**1. Architecture:**

```text
GitHub Issue / User Task
        |
        v
  SWE-agent Runner (sweagent/run/)
        |
        ├── Agent (ReAct Loop)
        │     ├── Thought -> Action -> Observation
        │     ├── LLM Call (via LiteLLM)
        │     └── History Processors (Context-Window-Optimization)
        |
        ├── Tool System (sweagent/tools/)
        │     ├── ToolConfig (Bundle-Loading, Registration)
        │     ├── ToolHandler (Execution, Filtering, Security)
        │     └── Tool Bundles (YAML-based Tool Definitions)
        |
        └── Environment (SWE-ReX)
              ├── Docker Container (isolated)
              ├── Bash Execution (Actions -> Shell)
              └── State Management (/root/state.json)
```

Core Pattern: Agent-Computer Interface (ACI) -- shell commands specifically designed for LLMs. Runtime: SWE-ReX (Remote Execution) -- Docker-based sandbox. LLM Integration: LiteLLM for 100+ provider support.

**2. Core Concepts:**

Agent-Computer Interface (ACI): Central innovation of the paper. Traditional Unix shells are unsuitable for LLMs. Special commands (`find_file`, `search_file`, `search_dir`, `edit`, `scroll_up/down`) are optimized for LLM comprehension: compact output, clear error messages. Inspired by HCI research (Human-Computer Interaction -> Agent-Computer Interaction).

ReAct Loop: At each step, the LLM generates Thought + Action. Action is executed in the environment producing an Observation. Observation flows back into the next LLM call. Typical pattern: Localization (Turns 1-5) then Edit+Execute loops (Turns 5+). Submission via `submit` command produces Patch (git diff).

Tool System: ToolConfig loads tools from bundles, detects duplicates, converts to function-calling format. ToolHandler provides security via `ToolFilterConfig` -- Blocklist (vim, nano, gdb), Standalone Blocklist (python, bash, su), Conditional Blocking (regex-based). Tool Bundles are YAML-defined tool collections, installed in `/root/tools/{bundle_name}`, with optional `install.sh` and PATH update. 15+ predefined bundles for various tasks. Multiline Support uses heredoc-style (`<< '{end_name}'`) for multiline commands.

History Processors: Pipeline for context window optimization. Older observations are shortened/summarized. Only current working context remains complete. Prevents token limit overflow during long sessions.

State Management: Environment state in `/root/state.json`. State commands after each action (working directory, variables). Enables introspection of environment state.

**3. SWE-ReX (Remote Execution):**

Separate module for sandboxed code execution. Docker container per task (isolated). Asynchronous bundle installation. PATH extension per bundle. `which` checks for tool availability verification. Supports local and remote execution.

**4. Mini-SWE-Agent:**

100 lines of Python -- radically minimalist agent. >74% on SWE-bench Verified (current, with Gemini 3 Pro). 65% on SWE-bench Verified (initial, with Claude Sonnet). No tool-calling interface -- only Bash as single tool. Linear history -- each step is appended to the messages.

`subprocess.run` -- each action independent (no stateful shell). Key insight: Modern LLMs need less scaffolding than assumed. Users: Meta, NVIDIA, Essential AI, Anyscale. From Princeton/Stanford team (same authors as SWE-agent).

**5. SWE-bench Performance:**

| Configuration | SWE-bench Verified | Year |
|---|---|---|
| SWE-agent + GPT-4 (1106) | 12.5% (SWE-bench full) | 04/2024 |
| SWE-agent + Claude 3 Opus | 12.5% (SWE-bench full) | 04/2024 |
| SWE-agent + Claude 3.5 Sonnet | ~33% | 06/2024 |
| SWE-agent 1.0 + Claude 3.7 Sonnet | ~66% | 02/2025 |
| Mini-SWE-Agent + Gemini 3 Pro | >74% | 2025 |
| Claude Opus 4.5 + Live-SWE-agent | 79.2% | 2025 |

SWE-bench Pro (harder benchmark): Best models only ~23% (GPT-5, Claude Opus 4.1). Contamination Concerns: Increasing evidence that frontier models have seen SWE-bench data in training leads to new benchmarks (SWE-bench Pro, SWE-rebench, SWE-bench-Live).

**6. Hook System:**

Observer pattern for agent lifecycle events. Hooks for: Pre/Post-Action, Error-Handling, Submission, State-Changes. Extensible for logging, observability, custom logic. Similar to CodeForge's planned hook system.

**7. Strengths:**

| Strength | Detail |
|---|---|
| ACI Innovation | Shell commands specifically designed for LLMs instead of generic Unix tools |
| Academically Grounded | NeurIPS 2024 paper, Princeton/Stanford research |
| SWE-bench Benchmark | De-facto standard for coding agent evaluation |
| Tool Bundles | YAML-declarative, extensible, interchangeable |
| Mini-SWE-Agent | Proves that 100 lines are sufficient for 74% SWE-bench |
| History Processors | Robust context window management |
| Cybersecurity Mode | Can also be used for offensive security and CTF |
| SWE-ReX Sandbox | Docker-based isolation, remote execution |
| LiteLLM Integration | 100+ providers out-of-the-box |
| Open Source (MIT) | Full freedom for integration and customization |

**8. Weaknesses:**

| Weakness | Detail |
|---|---|
| Edit Error Rate | 51.7% of trajectories have 1+ failed edits |
| No Web GUI | Pure CLI tool, no dashboard |
| Single-Agent | No multi-agent pattern, no delegation |
| No Multi-Project | One issue per run, no project management |
| No Approval Flow | Purely autonomous, no human-in-the-loop |
| Contamination Risk | SWE-bench scores possibly influenced by training contamination |
| Context-Window Dependent | Performance correlates strongly with available context (8k vs 128k) |
| No State Persistence | No cross-session memory |

**9. Relevance for CodeForge:**

```text
Go Core -> Task Queue -> Python AI Worker -> SWE-agent (CLI / Python API)
   ├── ACI Tools: find_file, search_file, edit (Code-Navigation)
   ├── SWE-ReX Sandbox: Docker-Container (Isolation)
   ├── Tool Bundles: YAML-based (extensible)
   ├── History Processors: Context-Window-Optimization
   ├── ReAct Loop: Thought -> Action -> Observation
   └── LLM Call: via LiteLLM (same stack as CodeForge)
```

Backend Candidate Priority 2 (after Aider due to lower API maturity). ACI concept directly adoptable for own tool definitions. Tool bundles concept identical to CodeForge's planned YAML tool system. History Processors adoptable for context window strategies.

SWE-bench serves as evaluation framework for CodeForge's agent quality. Mini-SWE-Agent provides a reference for minimal viable agent scaffolding.

**10. Adopted Patterns:**

| Pattern | Application in CodeForge |
|---|---|
| ACI (Agent-Computer Interface) | Tool definitions optimized for LLM comprehension |
| Tool Bundles (YAML) | Declarative tool definitions, interchangeable bundles |
| History Processors | Context window pipeline (Buffered, TokenLimited, HeadAndTail) |
| ReAct Loop | Thought->Action->Observation as fundamental agent pattern |
| ToolFilterConfig | Command Safety Evaluator (Blocklist, Conditional Blocking) |
| State Management (/root/state.json) | Worker state tracking in agent containers |
| SWE-ReX Sandbox | Docker-based agent execution isolation |
| Mini-SWE-Agent Pattern | Minimal scaffolding as fallback for simple tasks |

**11. Explicitly NOT Adopted:**

| Concept | Reason |
|---|---|
| Pure CLI Architecture | CodeForge is web-GUI-based |
| Single-Agent Pattern | CodeForge: Multi-agent orchestration |
| Missing Approval Flow | Human-in-the-loop is a core principle |
| Stateless Sessions | CodeForge needs persistent project contexts |
| SWE-bench as only metric | CodeForge also evaluates cost, speed, user satisfaction |

#### Devika

- URL: [https://github.com/stitionai/devika](https://github.com/stitionai/devika)
- Stars: ~19,500 | License: MIT | Status: Experimental/Stagnated (Rebranding to "Opcode" announced, but little activity since mid-2025)
- Description: First open-source implementation of an agentic software engineer (Devin alternative). Python backend (Flask + SocketIO), Svelte frontend, multi-LLM (Claude 3, GPT-4, Gemini, Mistral, Groq, Ollama), AI planning with specialized sub-agents, web browsing via Playwright, multi-language code generation. SQLite database for persistence. Jinja2 prompt templates.

Core Concepts:

- Agent Core (Orchestrator): Central `Agent` class drives the planning/execution loop. Manages conversation history, agent state, context keywords. Delegates to specialized sub-agents.
- Specialized Sub-Agents (9 total): Planner (analyzes user prompt, generates step-by-step plan with focus areas), Researcher (extracts search queries from the plan, prioritizes for efficiency), Coder (transforms plan + research into code, multi-file, multi-language), Action (maps follow-up prompts to action keywords: run, test, deploy, fix, implement, report), Runner (executes generated code in sandbox environment, multi-OS), Feature (implements new features in existing code), Patcher (debugs and fixes issues with root-cause analysis), Reporter (generates project documentation as PDF), Decision (handles special commands: Git ops, browser sessions).
- Agent Loop (Two Phases): Initial Execute (Prompt -> Planner -> Researcher -> Web Search via Bing/Google/DuckDuckGo -> Crawler -> Formatter -> Coder -> Code to disk) and Subsequent Execute (Follow-up -> Action Agent -> Specialist such as Runner/Feature/Patcher/Reporter -> Update).
- Browser Interaction: Playwright-based. `Browser` class for high-level primitives (navigate, query DOM, extract text/markdown/PDF, screenshots). `Crawler` class for LLM-driven webpage interaction (reasoning loop: Page Content + Objective -> LLM -> Action like CLICK/TYPE/SCROLL).
- Knowledge Base: SentenceBERT for semantic keyword extraction. Domain-specific experts (WebDev, Physics, Chemistry, Mathematics) as knowledge modules.
- State Management: `AgentStateModel` in SQLite -- sequential state logs with Step, Internal Monologue, Browser Session (Screenshot + URL), Terminal Session (Command + Output), Token Usage, Timestamp. Enables real-time visualization of agent thought process.
- Architecture Stack: Backend uses Python 3.10-3.11, Flask, Flask-SocketIO (Port 1337). Frontend uses SvelteKit + Bun (Port 3001). Communication via Socket.IO (WebSocket) for real-time + REST API (/api/*). Database: SQLite (Projects + AgentState tables). Browser: Playwright. LLM: Direct API calls per provider (no unified proxy).
- Jinja2 Prompt Templates: Each sub-agent has its own `prompt.jinja2` -- prompts as separate files, not in code.
- Stateless Agents: Agents are stateless/idempotent -- state is managed by the Agent Core and passed as needed.
- External Integrations: GitHub (Clone, File-List, Commits), Netlify (Deploy with URL generation).
- Config: `config.toml` for API keys, paths, search engine selection.

Strengths: Conceptually clean agent separation -- each sub-agent has clearly defined responsibility. Jinja2 prompt templates (identical pattern to what CodeForge plans). SentenceBERT keyword extraction for context-aware research. Real-time agent state visualization (Internal Monologue, Step, Browser, Terminal). Multi-LLM including Ollama for local models.

Open source (MIT), community-driven. Modular, extensible architecture. Agent loop pattern (Plan->Research->Code->Execute) serves as reference architecture.

Weaknesses: Project de facto stagnated/abandoned -- Issue #685 "is this project abandoned?" without answer, barely any commits since mid-2025. Many features unimplemented or broken (officially documented in README). No human-in-the-loop -- agent runs without approval flow. No checkpoint/rollback mechanisms. Single-process Flask server, no scaling, no message queue.

No diff-based file editing -- code is written directly. Security vulnerabilities (API key exposure risk in config). No context window management -- full context is sent to LLM. Direct provider API calls instead of unified LLM proxy.

No Git integration for code changes (only GitHub clone). No project management or roadmap.

**Relevance for CodeForge:** Sub-Agent Architecture (Planner/Researcher/Coder/Patcher separation as pattern for CodeForge worker modules, not 1:1, but conceptually). Jinja2 Prompt Templates confirms CodeForge decision to manage prompts as separate template files. SentenceBERT Keywords directly adopted -- KeyBERT for semantic keyword extraction in research module. Agent State Visualization (real-time display of Internal Monologue, Steps, Browser, Terminal as pattern for CodeForge Dashboard).

Browser Crawling Pattern (LLM-driven crawler as inspiration for web research worker). Anti-Patterns (what CodeForge avoids): No approval flow, no checkpointing, single-process, no context management, direct provider calls.

#### Aider

- URL: [https://aider.chat](https://aider.chat) / [https://github.com/Aider-AI/aider](https://github.com/Aider-AI/aider)
- Stars: ~40,000+ | License: Apache 2.0 | Language: Python
- Description: Terminal-based AI pair programmer. Git-native, multi-model support (127+ providers via LiteLLM). tree-sitter + PageRank Repo Map for codebase context. 7+ edit formats (model-specifically optimized). Architect/Editor two-model pattern. Auto-lint, auto-test, feedback loop with reflection cycles. CLI scripting and unofficial Python API.
- Strengths: Most mature codebase context system (Repo Map), deepest Git integration of all tools, empirically optimized edit formats, Architect/Editor reasoning separation
- **Gaps:** No web GUI (only experimental browser UI), no project management, no REST API, no agent orchestration, no sandbox isolation, single-user/single-session
- Relevance: Potential agent backend for CodeForge (via `--message` CLI or Python API). Repo Map concept as inspiration for GraphRAG Context Layer.
- Detailed Analysis: [docs/research/aider-deep-analysis.md](aider-deep-analysis.md)

#### Cline

- URL: [https://cline.bot](https://cline.bot) / [https://github.com/cline/cline](https://github.com/cline/cline)
- Stars: 4M+ users | License: Apache 2.0 | Version: 3.17+ (actively developed)
- Description: Autonomous AI coding agent as VS Code extension (+ CLI + JetBrains). Creates/edits files, executes commands, controls browser, uses MCP tools -- with human-in-the-loop approval at every step. Zero server-side components -- everything runs locally. React Webview frontend, TypeScript extension backend, gRPC communication via Protocol Buffers.

Core Concepts:

- Three-Tier Runtime Architecture: VS Code Extension Host (Node.js) for core orchestration, task management, state persistence. Webview UI (React) for sandboxed browser interface in VS Code panel. CLI Tool (Go) for standalone terminal interface with shared protocols.
- Controller (Central Orchestrator): Singleton, manages task lifecycle, StateManager, AuthService, McpHub. Registers 60+ VS Code commands.
- Task Execution Engine (Recursive Loop): User Input -> Controller -> Task Instance -> ContextManager builds system prompt. ApiHandler streams AI response -> Task parses tool invocations. Ask/Say Pattern: Request user approval -> ToolExecutor executes. Result is appended to conversation history -> Loop until completion.
- Tool System (5 Categories): File Operations (read_file, write_to_file, replace_in_file, list_files, search_files -- with diff-based approval), Terminal Commands (execute_command with output monitoring, `requires_approval` flag per LLM), Browser Automation (launch browser, screenshots, page interaction), MCP Tools (tools from connected Model Context Protocol servers), Context Management (embed workspace context in prompts, load .clinerules).
- Plan/Act Mode System: Plan Mode for read-only codebase analysis and architecture planning (can use cheaper model). Act Mode to execute real code changes (separate model configurable). Explicit user toggle -- agent cannot switch to Act Mode on its own. Cost optimization: DeepSeek-R1 for Plan, Claude Sonnet for Act -> up to 97% cost reduction.
- Human-in-the-Loop Approval (Layered Permission System): Default requires every action needs user approval (File Read, Write, Command, Browser, MCP). Auto-Approve Menu provides granular autonomy per tool category (Read Files, Edit Files, Safe Commands, Browser, MCP). `.clinerules` provide project-specific rules as text file or directory with Markdown files -- control approval behavior, coding standards, architecture constraints. Workflows offer on-demand automation (`/workflow.md`), injected as `<explicit_instructions>` -- consume tokens only when invoked. YOLO Mode bypasses all approvals (with OS notifications as safety net).
- Checkpoint System (Shadow Git): Separate Git repository (invisible to user's normal Git workflow). Automatic commits after each AI operation. Two diff modes: Incremental (last->current checkpoint) and Full Task (Baseline->Final). Restore Files Only (reset code, keep chat) or Restore Files & Task (reset both). Nested-Git-Handling: Temporary renaming (`_disabled` suffix) during operations. `.gitignore` and `.gitattributes` (LFS) are respected.
- MCP (Model Context Protocol) Integration: McpHub manages server connections via `mcp_config.json`. Transport: stdio, SSE, HTTP Streaming. Cline can create its own MCP servers ("add a tool" -> builds server + installs in extension). MCP Rules: Auto-selection based on conversation context and keywords. Global Workflows (v3.17+): Share workflows across all workspaces. MCP Marketplace for community servers.
- API Handler Architecture (40+ Providers): Factory Pattern with `buildApiHandler()` creates provider-specific handlers. Unified `ApiHandler` interface for all providers. Streaming responses with token counting and format conversion. Provider-specific features: Anthropic Prompt Caching, OpenAI Tool Calling, Gemini Extended Thinking. Supported providers: Anthropic, OpenAI, OpenRouter, Google Gemini, AWS Bedrock, Azure OpenAI, GCP Vertex, DeepSeek, Ollama, LM Studio, Cerebras, Groq + any OpenAI-compatible API.
- Context Management & Token Optimization: Context Window Progress Bar with input/output token tracking. Auto Compact at ~80% context utilization (conversation summary). Redundant File Read Removal (keep only latest version in context). `load_mcp_documentation` tool instead of static MCP instructions in system prompt (~8,000 tokens saved). Task sorting by cost/token usage.
- State Management (Three-Tier): In-memory cache -> 500ms debounced writes -> VS Code APIs + filesystem. Settings precedence: Remote Config (Org) > Task Settings > Global Settings. VS Code `secrets` API for API keys (encrypted). Mutex-protected concurrent access.
- gRPC Communication (Extension<->Webview): Protocol Buffers for type-safe bidirectional messaging. Services: StateService, TaskService, ModelsService, FileService, UiService, McpService. `window.postMessage()` with type "grpc_request" as transport.
- Diff-based File Editing: Search/Replace Blocks (SEARCH + REPLACE pattern). Lenient Matching (whitespace-tolerant, close-match instead of fail). VscodeDiffViewProvider with custom URI scheme (`cline-diff`). Side-by-side comparison before approval. Atomic replacements via `vscode.WorkspaceEdit`.
- Agent Client Protocol (ACP): JSON-RPC over stdin/stdout for cross-editor support (Zed-compatible).
- Enterprise Features: SSO, RBAC, Audit Logs, VPC Deployments, OpenTelemetry, `.clinerules`-based directory permissions.
- Cost/Token Tracking: Real-time token/cache/context usage display. Task sorting by cost or token consumption. OpenRouter `usage_details` for more precise cost tracking. Provider routing optimization: Throughput, Price, or Latency.
- Build System: esbuild (Extension) + Vite (Webview UI), Protocol Buffer codegen via `ts-proto`.

Strengths: Human-in-the-loop as architecture principle -- not bolted on after the fact. Checkpoint System (Shadow Git) is innovative and prevents data loss. MCP extensibility enables unlimited tool extension without plugin system. Plan/Act separation with separate models saves costs significantly. 40+ LLM providers natively -- more than any other coding agent.

`.clinerules` + Workflows = declarative project configuration. Auto-approve with granular control per tool category. Diff-based editing with side-by-side review. Zero server-side -- no data leaves the machine. Context management with auto-compact at ~80% window utilization.

Active community (4M+ users), regular releases. ACP for cross-editor portability (VS Code, Zed, JetBrains). Enterprise-ready (SSO, RBAC, Audit).

Weaknesses: High token consumption -- users report $50/day with intensive use. Bound to VS Code (CLI exists, but VS Code is primary). Not a standalone service -- not deployable as Docker container. Learning curve for .clinerules + Workflows + MCP setup. No multi-project dashboard.

No roadmap/feature map management. No built-in LLM routing/load balancing (user selects manually). Context window limits on very long tasks. No agent-to-agent orchestration (single-agent architecture).

Code quality not always optimal -- post-review needed. No budget enforcement (only tracking, no hard limit).

**Relevance for CodeForge:** Plan/Act Mode Pattern as direct inspiration for CodeForge's Plan->Approve->Execute workflow with separate LLM configurations per phase. Checkpoint System (Shadow Git concept as model for CodeForge's rollback mechanism, but in Docker containers instead of VS Code). Approval Flow (Ask/Say Pattern as granular permission system reference for CodeForge's human-in-the-loop design, web GUI instead of VS Code panel). MCP Integration (CodeForge can use MCP servers as tool extension -- standard protocol, no custom plugin system needed).

`.clinerules` Pattern (declarative project configuration as model for CodeForge's YAML-based project settings). Context Management (auto-compact and redundant file read removal as strategies for CodeForge's History Processors). Diff-based Editing (Search/Replace with lenient matching as pattern for CodeForge's file change approval in the web GUI). Tool Categorization (5 tool categories as taxonomy for CodeForge's tool system).

Anti-Patterns (what CodeForge does differently): Single-agent (CodeForge: Multi-agent), VS Code bound (CodeForge: Standalone service), no LLM routing (CodeForge: LiteLLM), no budget enforcement (CodeForge: hard limits).

#### Goose (Block / Square)

- URL: [https://github.com/block/goose](https://github.com/block/goose)
- Stars: ~30,400 | License: Apache 2.0 | Language: Rust (Core), Python/JavaScript Bindings, Electron (Desktop)
- Description: Block's (formerly Square) extensible AI agent. Goes beyond code suggestions: installs, executes, edits, and tests with any LLM. Rewritten in Rust for portability. Deep MCP integration (1,700+ servers).
- Core Features: Rust core for cross-platform binary distribution and embeddability, MCP-native design (1,700+ extensions), multi-model configuration for cost/performance optimization, CLI + Desktop App, 350+ contributors, 100+ releases in one year, backed by Block (Square/Cash App/Afterpay).
- **Relevance for CodeForge:** High as backend agent. MCP-native design, multi-model support, Apache 2.0. Rust core with Python/JS bindings enables programmatic control.

#### OpenCode (SST)

- URL: [https://github.com/anomalyco/opencode](https://github.com/anomalyco/opencode)
- Stars: ~100,000+ | License: MIT | Language: Go (Backend/TUI), SQLite
- Description: Open-source AI coding agent for the terminal, from the SST team (Serverless Stack). Interactive TUI with Bubble Tea, client/server architecture, LSP integration, session management.
- Core Features: Client/server architecture (TUI is just a client; can be remotely controlled, e.g., from mobile app), Vim-like editor in terminal, LSP integration for language-aware completions, provider-agnostic (Claude, OpenAI, Gemini, local models), persistent SQLite sessions.
- **Relevance for CodeForge:** High as backend agent. Client/server architecture ideal for programmatic control. MIT-licensed, written in Go (same language as CodeForge Core). LSP integration valuable feature.

#### Plandex

- URL: [https://github.com/plandex-ai/plandex](https://github.com/plandex-ai/plandex)
- Stars: ~14,700 | License: MIT | Language: Go (CLI + Server)
- Description: Terminal-based AI coding agent specifically for large projects and realistic tasks. Up to 2M token context, tree-sitter project maps for 20M+ token directories. Cumulative Diff Review Sandbox, full version control for AI-generated changes, Dockerized server.
- Core Features: Designed for large multi-file tasks (2M token context window), Cumulative Diff Review Sandbox (changes remain separate until approval), built-in version control for AI plans (branching, rollback), multi-model support (mix Anthropic, OpenAI, Google, Open Source), Dockerized Server Mode for self-hosting, REPL mode with fuzzy auto-complete.
- **Relevance for CodeForge:** High as backend agent. Planning-first approach, diff sandbox, and version control for AI changes fit perfectly with CodeForge's Plan->Approve->Execute->Review->Deliver workflow. Written in Go (same stack), MIT-licensed, Dockerized.

#### AutoCodeRover (NUS)

- URL: [https://github.com/nus-apr/auto-code-rover](https://github.com/nus-apr/auto-code-rover)
- Paper (ISSTA 2024): [https://github.com/nus-apr/auto-code-rover/blob/main/preprint.pdf](https://github.com/nus-apr/auto-code-rover/blob/main/preprint.pdf)
- Stars: ~2,800 | License: GPL-3.0 | Language: Python
- Description: Academically grounded autonomous software engineer (National University of Singapore). Uses AST-based code search for bug fixing and feature addition. 46.2% on SWE-bench Verified at under $0.70 per task.
- Core Features: Program Structure Aware (code search via Abstract Syntax Tree, not plain text), Statistical Fault Localization via test suites, extremely cost-efficient ($0.70/task, 7 minutes/task), supports GPT-4, Gemini, Claude, Llama (via Ollama).
- Relevance for CodeForge: Medium as backend agent. AST-aware search and fault localization are unique capabilities. Very low costs attractive for automated bug fixing. GPL-3.0 restricts integration.

#### Roo Code (formerly Roo Cline)

- URL: [https://github.com/RooCodeInc/Roo-Code](https://github.com/RooCodeInc/Roo-Code)
- Stars: ~22,200 | License: Apache 2.0 | Language: TypeScript (VS Code Extension)
- Description: AI-powered autonomous coding agent as VS Code extension. Offers a "whole dev team" of AI agents via the Modes system (QA Engineer, Product Manager, Code Reviewer, etc.). Cloud agents reachable via Web, Slack, or GitHub.
- Core Features: Modes System (specialized agent roles such as QA, PM, Architect, Reviewer with tool restrictions per mode), Custom Modes (create custom specialized agents with custom prompts), Flexible Approval (manual, autonomous, or hybrid), MCP Integration for unlimited custom tools, Cloud Agents (delegate work via Web, Slack, or GitHub), Roomote Control (remote control of local VS Code tasks).
- **Relevance for CodeForge:** High as pattern reference AND potential backend (Headless Mode). Modes system (specialized agent roles with restricted tool access) directly relevant for CodeForge's agent specialization. Cloud Agents as model for multi-interface vision.

#### Codex CLI (OpenAI)

- URL: [https://github.com/openai/codex](https://github.com/openai/codex)
- Stars: ~55,000 | License: Apache 2.0 | Language: TypeScript (Node.js CLI)
- Description: OpenAI's official open-source coding agent for the terminal. Reads, modifies, and executes code locally with o3/o4-mini models. Multimodal inputs (text, screenshots, diagrams), rich approval workflow, zero setup.
- Core Features: Official OpenAI product with native model integration, multimodal (text, screenshots, diagrams as input), three approval modes (suggest, auto-edit, full-auto), GitHub Action available for CI/CD integration, local execution (code never leaves the environment), community fork "open-codex" for Gemini, OpenRouter, Ollama.
- **Relevance for CodeForge:** High as backend agent. Apache 2.0, terminal-based, approval workflow. GitHub Action as CI/CD integration pattern. Community fork (open-codex) with multi-provider support.

#### SERA (Allen Institute for AI)

- URL: [https://github.com/allenai/sera-cli](https://github.com/allenai/sera-cli) | Models: [https://huggingface.co/allenai/SERA-32B-GA](https://huggingface.co/allenai/SERA-32B-GA)
- License: Apache 2.0 (CLI + Open Model Weights) | Language: Python
- Description: Family of open coding agent models from Ai2 (Allen Institute for AI). SERA-32B achieves 54.2% on SWE-bench Verified, comparable to proprietary models. Trainable for ~$400, customizable for private codebases for ~$1,300.
- Core Features: Open model weights (8B and 32B variants), extremely low training costs ($400 for reproduction, $1,300 for private codebase customization), "Student surpasses teacher" -- smaller open model outperforms larger proprietary teacher model, designed for customization on private codebases, uses Claude Code as execution harness.
- **Relevance for CodeForge:** High as backend model. SERA models can be deployed via Ollama/vLLM behind LiteLLM as self-hosted alternative to proprietary APIs. Particularly relevant for CodeForge's "Pure Completion" tier (needs Context + Tools + Quality Layer).

---

### 3. Orchestration Frameworks

#### LangGraph

- URL: [https://github.com/langchain-ai/langgraph](https://github.com/langchain-ai/langgraph)
- Stars: ~24,700 | License: MIT | Version: 1.0.8 (stable)
- Description: Graph-based agent orchestration from LangChain. StateGraph with Pregel Runtime (Bulk Synchronous Parallel). Channels/Reducers for state management. 6 streaming modes. Production-grade checkpointing (Postgres, SQLite).
- Core Concepts: StateGraph, Nodes (functions), Edges (fixed/conditional), Channels (LastValue/Topic/BinaryOperator), Pregel Runtime, Checkpointing, interrupt() for HITL

Strengths: Durable agent execution with crash recovery and time-travel. `interrupt()` for dynamic human-in-the-loop at any point. 6 streaming modes (values, updates, messages, custom, tasks, debug). PostgresSaver/PostgresStore for production. Multi-agent patterns: Supervisor, Swarm, Scatter-Gather. Functional API (`@entrypoint`/`@task`) for simple workflows.

Weaknesses: `langchain-core` as hard dependency (~20 transitive packages). Pregel model with Channels/Supersteps has steep learning curve. Node restart on interrupt-resume (entire node is re-executed). Distributed runtime only via LangGraph Platform (commercial). No built-in context window management.

**Relevance for CodeForge:** StateGraph as orchestration layer in Python Workers. Checkpointing via PostgresSaver. interrupt() for Plan->Approve workflow. Streaming for UI.

#### CrewAI

- URL: [https://github.com/crewAIInc/crewAI](https://github.com/crewAIInc/crewAI)
- Stars: ~27,000 | License: MIT | Version: 0.114+
- Description: Role-based multi-agent framework. Agents with Role/Goal/Backstory. Tasks with expected_output and guardrails. Two orchestration systems: Crew (Tasks) and Flow (DAG).
- Core Concepts: Agent (Role/Goal/Backstory), Task (Description/ExpectedOutput), Crew (Process: sequential/hierarchical), Flow (@start/@listen/@router DAG), Unified Memory (LanceDB), YAML Config

Strengths: Intuitive agent definition with persona system (Role/Goal/Backstory). YAML-based agent/task configuration + Python decorators. Unified Memory with Composite Scoring (Semantic + Recency + Importance). LLM Guardrail Agent -- one agent validates another's output. Flow system with @start/@listen/@router for DAG workflows.

Event bus with 60+ event types for observability. Human Feedback Provider Protocol (extensible: Web, Slack, Email). @tool decorator for clean tool definition. MCP integration native.

Weaknesses: No true parallelism in Crew (only via Flow). Two overlapping orchestration systems (Crew + Flow). ChromaDB + LanceDB both as dependencies (redundant). Memory needs LLM (gpt-4o-mini) + Embedder for basic operations.

Single-process, no message queue, no REST API. Consensual Process never implemented.

**Relevance for CodeForge:** YAML config pattern, Composite Memory Scoring, LLM Guardrail, Event Bus, Human Feedback Provider Protocol.

#### AutoGen (Microsoft)

- URL: [https://github.com/microsoft/autogen](https://github.com/microsoft/autogen)
- Stars: ~42,000 | License: MIT | Version: 0.7.5 (v0.4+ architecture)
- Description: Actor-model-based multi-agent framework. Clean layering: autogen-core (Runtime) -> autogen-agentchat (Teams) -> autogen-ext (Extensions). Distributed runtime via gRPC. Python + .NET.
- Core Concepts: Agent (Protocol), AgentId (type/key), AgentRuntime (Message Routing), Teams (RoundRobin/Selector/Swarm/GraphFlow/MagenticOne), ChatCompletionClient, Workbench (Tool-Container), Component System

Strengths: Clean package structure (Core -> AgentChat -> Extensions, a-la-carte dependencies). GraphFlow with DiGraphBuilder (DAG + Conditional Edges + Parallel Nodes). Workbench (tool container with shared state and dynamic tool discovery). Termination Conditions composable with & / | operators (12+ types). Context window strategies: Buffered, TokenLimited, HeadAndTail.

Component System (Agents/Tools/Teams as JSON serializable). MagenticOne Orchestrator (Planning Loop + Stall Detection + Re-Planning). HandoffMessage Pattern for agent handoff. SocietyOfMindAgent (wrap team as agent for nested orchestration).

Distributed runtime via gRPC (cross-language: Python <-> .NET). Minimal core dependencies (Pydantic, Protobuf, OpenTelemetry).

Weaknesses: No LLM routing/load balancing (each provider its own client). SingleThreadedAgentRuntime not suited for high concurrency. UserProxyAgent blocks entire team. No built-in persistent storage (state as Dict, caller must persist).

Complex abstraction layers (Core vs AgentChat). Memory system still young (ListMemory in Core).

**Relevance for CodeForge:** Layered Package Structure, GraphFlow, Workbench, Termination Conditions, Component System, MagenticOne Orchestrator, HandoffMessage Pattern.

#### MetaGPT

- URL: [https://github.com/geekan/MetaGPT](https://github.com/geekan/MetaGPT)
- Stars: ~50,000 | License: MIT
- Description: "Code = SOP(Team)". Simulates software development teams with specialized roles (ProductManager, Architect, Engineer, QA). Document-driven pipeline: PRD -> Design -> Tasks -> Code. Structured intermediate artifacts reduce hallucination.
- Core Concepts: Role (Profile/Goal/Actions/Watch), Action (LLM Call + Processing), Message (Pub-Sub with cause_by routing), Environment (Shared Space), Team (Hire + Run), ActionNode (schema-enforced outputs)

Strengths: Document-driven SOP Pipeline (PRD -> Design -> Tasks -> Code -> Test). ActionNode (schema validation + Review/Revise cycles on LLM output). Experience Pool (@exp_cache, cache successful runs and reuse). BM25 Tool Recommendation (automatically select relevant tools). Budget Enforcement (NoMoneyException, hard cost limits).

Mermaid diagram generation as design artifact. Incremental Development Mode (consider existing code). Multi-Environment (Software, Minecraft, Android, Stanford Town). Per-Action LLM Override (different models for different tasks). Message Compression strategies (pre-cut, post-cut by token/message).

Weaknesses: ~90 direct dependencies (massive footprint). Single-process asyncio, no distributed runtime. Tension between rigid SOPs and dynamic RoleZero. Memory simplistic (message list, optional vector search).

Cost management only global, not per Role/Action. Python 3.9-3.11 only (no 3.12+). No web GUI (only CLI, MGX commercial).

**Relevance for CodeForge:** Document Pipeline, ActionNode/Structured Output, Experience Pool, BM25 Tool Recommendation, Budget Enforcement, Incremental Development.

---

### 4. LLM Routing & Multi-Provider

#### LiteLLM

- URL: [https://github.com/BerriAI/litellm](https://github.com/BerriAI/litellm)
- Stars: ~22,000 | License: MIT | Version: 1.81+
- Description: Universal LLM proxy (Python). Unified OpenAI-compatible API (`litellm.completion()`) for 127+ providers. Production-grade Proxy Server (FastAPI + Postgres + Redis). Router with 6 routing strategies (latency/cost/usage/least-busy/shuffle/tag-based). Fallback chains with cooldown. Budget management per key/team/user. 42+ observability integrations (Langfuse, Prometheus, Datadog, etc.). Caching (Redis, Semantic, In-Memory).
- Core Concepts: `litellm.completion()` (unified entry point), `Router` (Load Balancing + Fallbacks), Proxy Server (FastAPI, Port 4000), `model_list` (YAML Config), `BaseConfig` (Provider Abstraction), `CustomStreamWrapper` (Streaming), Callbacks/Hooks

Strengths: 127+ providers natively (OpenAI, Anthropic, Gemini, Bedrock, Ollama, vLLM, LM Studio, etc.). OpenAI-compatible REST API -- any client that speaks OpenAI automatically speaks LiteLLM. Router: 6 routing strategies + fallback chains + cooldown on provider outages. Budget Management: Per-key, per-team, per-user, per-provider limits. Docker image available (`docker.litellm.ai/berriai/litellm:main-stable`).

Structured Output cross-provider (schema as tool call for providers without native support). 42+ observability integrations (Prometheus, Langfuse, Datadog, etc.). Caching: In-Memory, Redis, Semantic (Qdrant), S3, GCS. Model aliases: Map logical names to real provider models. Per-call cost tracking with comprehensive pricing database (36,000+ lines JSON).

Weaknesses: Monolithic codebase (6,500+ files, `main.py` 7,400 lines with if/elif chain). Python-only -- must run as separate service, not embeddable in Go. Proxy needs Postgres for persistent spend tracking and key management. Memory footprint: 200-500MB+ RAM in proxy mode.

High change rate (frequent releases, occasional breaking changes). Error mapping across 127 providers not always perfect. No built-in prompt management.

**Relevance for CodeForge:** Central architecture decision -- LiteLLM Proxy as Docker sidecar. No custom LLM provider interface needed. Go Core speaks OpenAI format against LiteLLM. Python Workers use `litellm.completion()` directly. Routing, fallbacks, budgets, cost tracking delegated to LiteLLM.

#### OpenRouter

- URL: [https://openrouter.ai](https://openrouter.ai)
- Stars: n/a (Cloud SaaS) | Models: 300+ | Providers: 60+
- Description: Cloud-hosted unified API gateway for LLMs. Single endpoint (`/api/v1/chat/completions`) routes to 300+ models across 60+ providers. ~30 trillion tokens/month, 5M+ users. OpenAI-compatible API. Auto-Router (NotDiamond AI) for intelligent model selection. BYOK (Bring Your Own Keys) support.
- Core Concepts: Provider Routing (price/latency/throughput sorting), Model Fallbacks (cross-model), Auto Router (AI-based model selection), Model Variants (:free, :nitro, :thinking, :online), Credits System, BYOK

Strengths: 300+ models, 60+ providers via one endpoint. OpenAI-compatible API (1-line integration via base URL change). Auto-Router: AI selects optimal model per prompt. Provider Routing: Sorting by price/latency/throughput, whitelist/blacklist. Model Fallbacks: Cross-model fallback chains.

Zero Data Retention (ZDR) option. Rankings/Leaderboard based on real usage data. Message Transforms: Intelligent prompt compression on context overflow.

Weaknesses: Cloud-only -- no self-hosting (core problem for CodeForge). ~5.5% platform fee on all spending. No local models (Ollama, LM Studio not supported). Privacy dependency: All prompts transit OpenRouter infrastructure. Credits expire after 1 year. No volume discount.

**Relevance for CodeForge:** As optional provider behind LiteLLM. LiteLLM has native OpenRouter support (`openrouter/<model-id>`). Users who prefer OpenRouter configure it as a LiteLLM deployment. CodeForge does not build its own OpenRouter integration.

#### Claude Code Router

- URL: [https://github.com/musistudio/claude-code-router](https://github.com/musistudio/claude-code-router)
- Stars: ~27,800 | License: MIT | Version: 2.0.0 (npm)
- Description: Local proxy specifically for Claude Code CLI. Sets `ANTHROPIC_BASE_URL` to localhost, intercepts all requests, routes to configured providers (OpenAI, Gemini, DeepSeek, Groq, etc.). Transformer chain architecture for request/response transformation. Scenario-based routing (default/background/think/longContext/webSearch).
- Core Concepts: Transformer Chain (composable request/response transformers), Scenario-based Routing, Provider Config (JSON5), Preset System (Export/Import/Share), Custom Router Functions (JS modules), Token-Threshold Routing, Subagent Routing

Strengths: Scenario-based Routing -- different models for different task types: `default` (general coding tasks), `background` (non-interactive tasks, cheaper models), `think` (reasoning-intensive operations, thinking models), `longContext` (automatically when tokens > threshold, large context windows), `webSearch` (web-search-capable models).

Transformer Chain: Composable, ordered transformers for provider normalization. 22 transformer adapters (provider-specific + feature adapters). Preset System: Export/share routing configurations. Token-based Routing: Auto-switch to long-context models above threshold. Custom Router Functions: User-defined routing logic as JS modules. React-based Config UI (`ccr ui`).

Weaknesses: Claude-Code-specific (only works as proxy for Anthropic CLI). 714 open issues (stability problems). No formal GitHub releases. No load balancing, no fallback chains.

No cost tracking, no budget management. Single-user, no multi-tenancy. Node.js-only, fragile streaming transformation.

**Relevance for CodeForge:** Scenario-based routing is the core concept. CodeForge adopts the idea (default/background/think/longContext/review/plan), but implements it via LiteLLM's tag-based routing instead of as a separate proxy. Token threshold routing and preset system also planned as features.

#### OpenCode CLI

- URL: [https://github.com/opencode-ai/opencode](https://github.com/opencode-ai/opencode) (archived) -> [https://opencode.ai](https://opencode.ai) (TypeScript rewrite)
- Stars: n/a (archived) | License: MIT
- Description: Open-source terminal AI agent. Original in Go (archived, successors: Crush by Charm + OpenCode by Anomaly/SST). 7 Go clients cover 12 providers via OpenAI-compatible base URL pattern. GitHub Copilot token exchange. Local model auto-discovery. TypeScript rewrite (opencode.ai) uses Vercel AI SDK + Models.dev for 75+ providers.
- Core Concepts: OpenAI-compatible Base URL Pattern (1 SDK for many providers), GitHub Copilot Token Exchange, Local Model Auto-Discovery (/v1/models), Provider Priority Chain, Context File Interoperability (CLAUDE.md, .cursorrules, copilot-instructions.md), Per-Model Pricing Data

Strengths: Shows that most providers are OpenAI-compatible -- base URL is sufficient. GitHub Copilot as free provider (token from `~/.config/github-copilot/hosts.json`). Auto-Discovery: Detect local models via `/v1/models` endpoint. Provider Priority Chain (Copilot > Anthropic > OpenAI > Gemini > ...).

Context File Interoperability (reads CLAUDE.md, .cursorrules etc.). Per-session cost tracking with hardcoded pricing.

Weaknesses: Go codebase archived (split into Crush + OpenCode TypeScript). Hardcoded model catalog (every new model requires code change). No multi-provider routing (one provider per agent). No load balancing, no fallbacks.

Single-agent architecture. No web GUI.

**Relevance for CodeForge:** Three patterns adopted: (1) GitHub Copilot token exchange as provider in Go Core, (2) Auto-discovery for local models (query Ollama/LM Studio `/v1/models`), (3) Provider Priority Chain for intelligent defaults without configuration.

#### Architecture Decision: No Custom LLM Interface

The analysis of all four tools leads to a clear decision.

CodeForge builds NO custom LLM provider interface. LiteLLM covers 127+ providers, including routing, fallbacks, cost tracking, budgets, streaming, and tool calling. Building this ourselves would take months and would permanently lag behind LiteLLM's provider coverage.

**What CodeForge does NOT build:**

- No custom LLM provider proxy
- No custom provider abstraction in Go or Python
- No custom cost tracking at token level (LiteLLM does this)
- No custom fallback/retry logic for LLM calls

**What CodeForge BUILDS:**

| Component | Layer | Description |
|---|---|---|
| LiteLLM Config Manager | Go Core | Generates/updates LiteLLM Proxy YAML config |
| User-Key-Mapping | Go Core | Maps CodeForge users to LiteLLM Virtual Keys |
| Scenario Routing | Go Core | Maps task types to LiteLLM tags (default/background/think/longContext/review/plan) |
| Cost Dashboard | Frontend | Pulls spend data from LiteLLM API (`/spend/logs`, `/global/spend/per_team`) |
| Local Model Discovery | Go Core | Auto-discovery via Ollama/LM Studio `/v1/models` endpoint |
| Copilot Token Exchange | Go Core | Exchange GitHub Copilot token from local config |

**Integration Architecture:**

```text
TypeScript Frontend (SolidJS)
        |
        v  REST / WebSocket
Go Core Service
        |
        v  HTTP (OpenAI-compatible API)
LiteLLM Proxy (Docker Sidecar, Port 4000)
        |
        v  Provider APIs
OpenAI / Anthropic / Ollama / Bedrock / OpenRouter / etc.
```

Go Core and Python Workers both communicate with LiteLLM via the standard OpenAI API. Go Core uses the OpenAI Go SDK or raw HTTP. Python Workers use `litellm.completion()` directly.

**Scenario-based Routing (inspired by Claude Code Router):**

| Scenario | Description | Example Routing |
|---|---|---|
| `default` | General coding tasks | Claude Sonnet / GPT-4o |
| `background` | Non-interactive tasks, batch | GPT-4o-mini / DeepSeek |
| `think` | Reasoning-intensive tasks | Claude Opus / o3 |
| `longContext` | Input > token threshold | Gemini Pro (1M Context) |
| `review` | Code review, quality check | Claude Sonnet |
| `plan` | Architecture, design | Claude Opus |

Implemented via LiteLLM's tag-based routing: Go Core sets `metadata.tags` in the request, LiteLLM routes to the matching model deployment.

---

### 5. Spec-Driven Development, Roadmap Tools & Project Management

#### 5.1 Spec-Driven Development (SDD) Tools

**OpenSpec:**

- URL: [https://github.com/Fission-AI/OpenSpec](https://github.com/Fission-AI/OpenSpec) | Website: [https://openspec.dev/](https://openspec.dev/)
- Stars: ~24,000 | License: MIT | CLI: `openspec` (npm)
- Description: Brownfield SDD framework. Specs live in the repo (`openspec/specs/` as source of truth, `openspec/changes/` for delta proposals). CLI-based, no web GUI. Integration with 22+ AI tools (Claude Code, Cursor, Windsurf, Cline, Aider, etc.).
- Core Concepts: Spec Directory (`openspec/specs/` -- YAML/Markdown requirements, API specs, data models), Change Proposals (`openspec/changes/` -- Delta format: ADDED/MODIFIED/REMOVED requirements), CLI Commands (`openspec init`, `openspec review`, `openspec apply`, `openspec status`), JSON Output (`--json` flag for machine-readable CLI output), Detection (`openspec/` directory in repo root).
- Strengths: Brownfield-capable (existing projects without major restructuring), delta spec format elegant for change management, agent-agnostic (works with any AI tool), CLI with `--json` for programmatic integration, growing community (~24k stars).
- Weaknesses: No web GUI, no bidirectional sync to PM tools, no task tracking (only specs, no issues/tasks).
- Relevance for CodeForge: Primary SDD format. Auto-detection via `openspec/` directory. Delta spec format adopted for change proposals. CLI integration for `openspec review`/`apply` as agent tool.

**GitHub Spec Kit:**

- URL: [https://github.com/spec-kit/spec-kit](https://github.com/spec-kit/spec-kit)
- Stars: ~16,000+ | License: MIT
- Description: Greenfield SDD framework. `.specify/` folder with `spec.md`/`plan.md`/`tasks/`. Feature numbering. Agent-agnostic.
- Core Concepts: Spec Directory (`.specify/` with `spec.md`, `plan.md`, `tasks/*.md`), Pipeline (Spec -> Plan -> Tasks as structured decomposition), Feature Numbering (each feature gets a unique number), Detection (`.specify/` directory in repo root).
- Strengths: Intuitive Markdown-based structure, clear pipeline (Spec -> Plan -> Tasks), agent-agnostic, good community adoption.
- Weaknesses: No brownfield support (only for new features), no delta format (complete spec rewrites), no CLI for machine processing.
- Relevance for CodeForge: Second SDD format. Auto-detection via `.specify/`. Spec->Plan->Tasks pipeline as inspiration for document pipeline.

**Autospec:**

- URL: [https://github.com/Autospec-AI/autospec](https://github.com/Autospec-AI/autospec)
- Description: YAML-first SDD. `specs/spec.yaml`, `specs/plan.yaml`, `specs/tasks.yaml`. Ideal for programmatic integration.
- Core Concepts: Spec Directory (`specs/` with YAML files), YAML Format (structured, machine-readable, versionable), Detection (`specs/spec.yaml` in repo).
- Strengths: YAML format ideal for Go/Python parsing (no Markdown ambiguity), programmatically generatable and validatable, clear schema definition.
- Weaknesses: Smaller community than OpenSpec/Spec Kit, YAML less human-readable than Markdown for long text.
- Relevance for CodeForge: Third SDD format. YAML ideal for machine processing. Auto-detection via `specs/spec.yaml`.

**Additional SDD Tools:**

| Tool | Approach | Detection Marker | Relevance |
|---|---|---|---|
| BMAD-METHOD | Multi-Agent Design with specialized personas | `bmad/` directory | Persona pattern interesting for agent specialization |
| Amazon Kiro | Spec-based IDE (Commercial) | `.kiro/` directory | Shows commercial trend toward SDD |
| cc-sdd | Claude Code SDD Extension | `.sdd/` directory | Lightweight SDD variant |

#### 5.2 Project Management Tools (Open Source)

**Plane.so:**

- URL: [https://plane.so](https://plane.so) / [https://github.com/makeplane/plane](https://github.com/makeplane/plane)
- Stars: ~45,600 | License: AGPL-3.0
- Description: Modern open-source PM with AI features, roadmaps, wiki, GitHub/GitLab sync. REST API v1 with 180+ endpoints. Python SDK available.
- Core Concepts: Hierarchy (Workspace -> Initiative -> Project -> Epic -> Work Item/Issue + Cycles + Modules), API (REST `/api/v1/`, cursor-based pagination, field selection with `expand` and `fields`, CRUD for all entities), SDK (`plane-sdk` Python -- typed clients for all resources), Webhooks (Events for Issues, Cycles, Modules with HMAC-SHA256 signing), MCP Server (official MCP integration for AI assistants), Labels (label-triggered automation and sync).
- Strengths: Strongest open-source PM tool (45.6k stars, active development), comprehensive REST API with 180+ endpoints (well documented), Python SDK for easy integration, bidirectional GitHub/GitLab sync (Issues, Labels, Comments), Initiative/Epic/WorkItem hierarchy maps roadmap, cursor-based pagination (scales well), field selection and expansion (reduces API traffic), webhooks with HMAC-SHA256 for secure integration, MCP Server for AI integration, AGPL-3.0 (self-hosted possible).
- Weaknesses: No SVN support, no AI coding agent integration, API REST only (no GraphQL), AGPL-3.0 requires caution with integration.
- **Relevance for CodeForge:** Primary PM platform adapter. REST API as sync target. Adopted patterns: Initiative/Epic/WorkItem hierarchy, cursor-based pagination, field selection, Webhook HMAC-SHA256, label-triggered sync.

**OpenProject:**

- URL: [https://www.openproject.org/](https://www.openproject.org/) / [https://github.com/opf/openproject](https://github.com/opf/openproject)
- Stars: ~14,400 | License: GPL v3
- Description: Enterprise PM with Gantt charts, version boards, roadmaps. HAL+JSON API v3 with HATEOAS. OAuth 2.0. GitHub/GitLab webhook integration. Ruby on Rails.
- Core Concepts: API (HAL+JSON API v3 with HATEOAS, self-describing links, 50+ endpoint families), Auth (OAuth 2.0 PKCE + API Keys), Work Packages (central entity with 20+ types: Task, Bug, Feature, Epic, etc.), Versions (version-based roadmaps as releases/milestones), Gantt (interactive Gantt charts with dependencies), SCM Integration (GitHub/GitLab webhooks linking Pull Requests to Work Packages), Schema Endpoints (`/api/v3/work_packages/schema` for dynamic forms), Form Endpoints (`/api/v3/work_packages/form` for validation before submit), Notification Reasons (granular: mentioned, assigned, responsible, watched).
- Strengths: Enterprise-ready (14+ years of development, ISO 27001), Gantt charts and version boards (real roadmap features), HATEOAS API (self-describing, discovery via links), Optimistic Locking via `lockVersion` (conflict detection), Schema/Form endpoints for dynamic UIs, GitHub/GitLab webhook integration (PR -> Work Package link), Notification Reasons (granular notifications).
- Weaknesses: HAL+JSON too complex for Go Core (heavy parsing), Ruby on Rails monolith (difficult to embed), GPL v3 (more restrictive than MIT/AGPL), API documentation partially outdated.
- Relevance for CodeForge: Second PM platform adapter. Adopted patterns: Optimistic Locking (lockVersion), Schema Endpoints for dynamic forms, Notification Reasons. HAL+JSON explicitly NOT adopted (too complex for Go Core, normal JSON REST is sufficient).

**Ploi Roadmap:**

- URL: [https://github.com/ploi/roadmap](https://github.com/ploi/roadmap)
- License: MIT | Stack: Laravel (PHP)
- Description: Simple open-source roadmap tool with innovative `/ai` endpoint for machine-readable data.
- Core Concepts: `/ai` Endpoint (provides roadmap data in JSON/YAML/Markdown -- specifically for LLM consumption), Roadmap Board (Kanban-style display: Under Review -> Planned -> In Progress -> Live), Webhooks (simple event notifications), Voting (user voting on roadmap items).
- Strengths: `/ai` Endpoint is an innovative pattern for machine-readable roadmap data, simple clear architecture, MIT license (maximally permissive), voting feature for community feedback.
- Weaknesses: Minimal feature set (no Gantt, no Epic/Story, no hierarchy), Laravel/PHP stack (not directly integrable), no API for CRUD (read only via `/ai`).
- **Relevance for CodeForge:** `/ai` endpoint pattern adopted. CodeForge provides its own `/api/v1/roadmap/ai` endpoint that formats roadmap data for LLM consumption in JSON/YAML/Markdown.

**Additional PM Tools:**

| Tool | Stars | License | Core Feature | Relevance for CodeForge |
|---|---|---|---|---|
| Huly | ~22,000 | EPL-2.0 | All-in-one PM, bidirectional GitHub sync | Sync architecture as reference |
| Linear | Closed Source | Commercial | GraphQL API, MCP Server, best DX | GraphQL pattern, MCP as integration |
| Leantime | ~5,000 | AGPL-3.0 | Lean PM, Strategy -> Portfolio -> Project | Strategy layer concept |
| Roadmapper | ~500 | MIT | Golang-based roadmap tool | Go implementation as reference |

#### 5.3 SCM-based Project Management

Many teams use the built-in PM features of their SCM platform (GitHub Issues, GitLab Issues/Boards). CodeForge must detect and integrate these.

| Platform | PM Features | API | Detection |
|---|---|---|---|
| GitHub | Issues, Projects (v2), Milestones, Labels | REST v3 + GraphQL v4 | Remote URL `github.com` |
| GitLab | Issues, Boards, Milestones, Epics, Roadmaps | REST v4 + GraphQL | Remote URL `gitlab.com` or self-hosted |
| Gitea/Forgejo | Issues, Labels, Milestones, Projects | REST (GitHub-compatible) | Remote URL + `/api/v1/version` |

Gitea/Forgejo Insight: The GitHub-compatible API means that CodeForge's GitHub adapter works with minimal changes for Gitea/Forgejo as well. Recommendation: Implement Gitea as third SCM adapter, based on the GitHub adapter.

#### 5.4 Repo-based Project Management

Some tools store PM artifacts directly in the repository. CodeForge should detect and integrate these.

| Tool | Detection Marker | Format | Description |
|---|---|---|---|
| Markdown Projects (mdp) | `.mdp/` directory | Markdown | Projects as Markdown files in the repo |
| Backlog.md | `backlog/` directory | Markdown | Backlog as Markdown files |
| git-bug | Embedded in Git objects | Git Objects | Bug tracking directly in Git (no external service) |
| Tasks.md | `TASKS.md` file | Markdown | Simple task list as Markdown |
| markdown-plan | `PLAN.md` / `ROADMAP.md` | Markdown | Roadmap as Markdown |

#### 5.5 ADR/RFC Tools

Architectural Decision Records and RFCs are present in many projects and relevant for CodeForge's planning features.

| Detection Marker | Tool/Convention | Description |
|---|---|---|
| `docs/adr/` | ADR Tools (adr-tools, log4brains) | Architectural Decision Records |
| `docs/decisions/` | Alternative ADR convention | Decision documentation |
| `docs/rfcs/` or `rfcs/` | RFC Process | Request for Comments |

CodeForge detects these directories and can display and reference ADRs/RFCs in the roadmap view.

#### 5.6 Feature Flag Tools

Feature flags influence the roadmap view (which features are active, in rollout, etc.).

| Tool | Stars | License | API | Relevance |
|---|---|---|---|---|
| Unleash | ~12,000 | Apache-2.0 | REST API | Integrate feature flag state into roadmap |
| OpenFeature | Standard | Apache-2.0 | SDK Standard | Vendor-neutral feature flag interface |
| Flagsmith | ~5,000 | BSD-3 | REST + SDK | Feature flags + remote config |
| FeatBit | ~2,000 | MIT | REST API | Self-hosted feature flags |
| GrowthBook | ~7,000 | MIT | REST + SDK | Feature flags + A/B testing |

Feature flag integration is a Phase 3 feature. CodeForge can query the current feature status via the tools' REST APIs and display it in the roadmap view.

#### 5.7 Auto-Detection Architecture

CodeForge automatically detects which spec, PM, and roadmap tools are used in a project and offers matching integration.

**Three-Tier Detection:**

```text
Tier 1: Spec-Driven Detectors (Scan repo files)
  ├── openspec/           -> OpenSpec
  ├── .specify/           -> GitHub Spec Kit
  ├── specs/spec.yaml     -> Autospec
  ├── .bmad/              -> BMAD-METHOD
  ├── .kiro/              -> Amazon Kiro
  ├── .sdd/               -> cc-sdd
  ├── docs/adr/           -> ADR Tools
  ├── docs/rfcs/          -> RFC Process
  ├── .mdp/               -> Markdown Projects
  ├── backlog/            -> Backlog.md
  ├── TASKS.md            -> Tasks.md
  └── ROADMAP.md          -> markdown-plan

Tier 2: Platform Detectors (API-based detection)
  ├── Remote URL Analysis  -> GitHub / GitLab / Gitea / Forgejo
  ├── API Probe            -> Plane.so / OpenProject / Huly / Linear
  └── Webhook Config       -> Detect existing webhook setups

Tier 3: File-Based Detectors (simple markers)
  ├── .github/            -> GitHub Actions, Issue Templates
  ├── .gitlab-ci.yml      -> GitLab CI
  ├── CHANGELOG.md        -> Changelog management
  └── .env / .env.example -> Environment configuration
```

**Detection Flow:**

```text
1. Repo is added to CodeForge
     |
2. Go Core scans repo root for detection markers (Tier 1 + 3)
     |
3. Go Core analyzes remote URL and probes platform APIs (Tier 2)
     |
4. Detected tools are shown to the user:
   "Detected: OpenSpec, GitHub Issues, ADRs"
     |
5. User configures integration:
   - Which tools are actively tracked
   - Sync direction (Import / Export / Bidirectional)
   - Sync frequency (Webhook / Poll / Manual)
     |
6. Go Core sets up sync (register webhooks, schedule poll jobs)
```

#### 5.8 Architecture Decisions: Roadmap/PM Integration

| Decision | Rationale |
|---|---|
| No custom PM tool | Plane, OpenProject, GitHub Issues exist. CodeForge synchronizes instead of reinventing. |
| Repo-based specs as first-class | OpenSpec, Spec Kit, Autospec live in the repo -- CodeForge treats them as primary roadmap source. |
| Bidirectional sync | Changes in CodeForge -> PM tool and vice versa. Conflict resolution via timestamps + user decision. |
| Provider Registry Pattern | Same architecture as `gitprovider` and `llmprovider` -- new PM adapters only require a new package + blank import. |
| Cursor-based pagination (from Plane) | Scales better than offset-based for large datasets. For CodeForge's own API and PM sync. |
| HAL+JSON NOT adopted (from OpenProject) | Too complex for Go Core. Normal JSON REST with clear endpoints is sufficient. |
| Label-triggered sync (from Plane) | Labels as trigger for automatic sync -- e.g., label "codeforge-sync" activates bidirectional synchronization. |
| `/ai` Endpoint (from Ploi Roadmap) | Dedicated endpoint that prepares roadmap data for LLM consumption (JSON/YAML/Markdown). |

#### 5.9 Adopted Patterns

**From Plane.so:**

| Pattern | Implementation in CodeForge |
|---|---|
| Initiative/Epic/WorkItem Hierarchy | CodeForge Roadmap Model: Milestone -> Feature -> Task |
| Cursor-based Pagination | Standard pagination for CodeForge API and PM sync |
| Field Selection (`expand`, `fields`) | API responses configurable -- only required fields |
| Webhook HMAC-SHA256 | Secure webhook verification for incoming events |
| Label-triggered Sync | Label "codeforge-sync" activates bidirectional sync |
| MCP Server | CodeForge provides its own MCP Server for AI integration |

**From OpenProject:**

| Pattern | Implementation in CodeForge |
|---|---|
| Optimistic Locking (lockVersion) | Conflict detection for concurrent changes |
| Schema Endpoints | `/api/v1/{resource}/schema` for dynamic form generation in GUI |
| Form Endpoints | `/api/v1/{resource}/form` for validation before submit |
| Notification Reasons | Granular notifications (mentioned, assigned, responsible, watching) |

**From OpenSpec:**

| Pattern | Implementation in CodeForge |
|---|---|
| Delta Spec Format | Change proposals as ADDED/MODIFIED/REMOVED deltas |
| Change Proposal Workflow | Spec changes go through Review -> Apply pipeline |
| `--json` CLI Output | Agent tools receive machine-readable outputs |

**From GitHub Spec Kit:**

| Pattern | Implementation in CodeForge |
|---|---|
| Spec -> Plan -> Tasks Pipeline | Structured decomposition in document pipeline |
| Feature Numbering | Unique feature IDs for referencing |

**From Autospec:**

| Pattern | Implementation in CodeForge |
|---|---|
| YAML-first Artifacts | Specs/Plans/Tasks in YAML (machine-readable, validatable) |

**From Ploi Roadmap:**

| Pattern | Implementation in CodeForge |
|---|---|
| `/ai` Endpoint | `/api/v1/roadmap/ai` -- Roadmap data for LLM consumption (JSON/YAML/Markdown) |

**Explicitly NOT Adopted:**

| Concept | Reason |
|---|---|
| HAL+JSON / HATEOAS (OpenProject) | Too complex for Go Core, normal JSON REST is sufficient |
| GraphQL API (Linear) | REST as primary API, GraphQL possibly later |
| Custom PM Tool | Sync with existing tools instead of new development |
| Ruby on Rails Patterns (OpenProject) | Go Core has its own architecture (Hexagonal) |
| Plane's AGPL Code | No code adoption, only API integration and pattern inspiration |

---

### 6. Self-Hosted LLM Platforms

#### Dify

- URL: [https://github.com/langgenius/dify](https://github.com/langgenius/dify) | Website: [https://dify.ai](https://dify.ai)
- Description: Open-source LLM App Development. Visual Workflow Builder, RAG, Agent Capabilities, LLMOps. Docker Compose deployment.
- Stars: ~129,000
- Relevance: Best example of self-hosted LLM platform with UI. UI/UX inspiration.

#### AnythingLLM

- URL: [https://github.com/Mintplex-Labs/anything-llm](https://github.com/Mintplex-Labs/anything-llm)
- Description: All-in-one Desktop & Docker AI Application. RAG, AI Agents, No-code Agent Builder, MCP.
- Relevance: Shows what all-in-one Docker AI can look like.

#### Open WebUI

- URL: [https://github.com/open-webui/open-webui](https://github.com/open-webui/open-webui)
- Description: Self-hosted AI Interface. Ollama + OpenAI-compatible. Docker/Kubernetes.
- Relevance: UI patterns for LLM interaction.

---

### 7. Market Assessment

| Area | Market Status | Our Opportunity |
|---|---|---|
| AI Coding Agents | Overcrowded (>20) | Don't reinvent, integrate |
| Multi-LLM Routing | Solved | Use LiteLLM/OpenRouter |
| Self-hosted Web GUI Agent | 1-2 players | OpenHands dominates |
| Spec-Driven Development | Fragmented (6+) | Auto-detection + multi-format support |
| PM Tool Integration | Many silos | Bidirectional sync as aggregator |
| Roadmap + Agent + Multi-Project | No solution | Primary differentiation |
| SVN Support for AI Agents | None | Unique selling point |
| Auto-Detection + Adaptive Integration | Does not exist | Technical unique selling point |
| Integrated Platform (all 4 pillars) | Does not exist | Core offering of CodeForge |

---

### 8. Strategic Recommendations

**Build on existing building blocks:**

- LLM Routing: LiteLLM as proxy layer (instead of custom routing)
- Agent Backends: Integration of Aider, OpenHands, SWE-agent as interchangeable backends
- Spec Formats: Multi-format support (OpenSpec, Spec Kit, Autospec) with auto-detection
- PM Integration: Sync with Plane, OpenProject, GitHub/GitLab Issues (instead of custom PM tool)

**Differentiate through integration:**

- Central dashboard for multiple projects (Git, GitHub, GitLab, SVN, Gitea/Forgejo)
- Visual roadmap management with bidirectional sync to repo specs AND PM tools
- Auto-Detection: Automatic detection of all Spec/PM/Roadmap tools in the repo
- Adaptive Integration: Offer matching sync strategy based on detected tools
- LLM provider management with task-based routing
- Agent orchestration coordinating different coding agents

**Avoid:**

- Building custom LLM proxy from scratch (LiteLLM exists)
- Building custom coding agent from scratch (integrate existing ones)
- Building custom PM tool from scratch (sync with Plane/OpenProject/GitHub Issues)
- Feature war with OpenHands on their core territory (solving individual issues)
- HAL+JSON/HATEOAS -- too complex for Go, normal JSON REST is sufficient

---

### 9. Framework Comparison: LangGraph vs CrewAI vs AutoGen vs MetaGPT

#### Architecture Comparison

| Dimension | LangGraph | CrewAI | AutoGen | MetaGPT |
|---|---|---|---|---|
| Metaphor | State Machine / Graph | Crew with Tasks | Actor Model / Pub-Sub | Software Company with SOPs |
| State Model | Central (Shared State Dict) | In Crew context | Distributed (each agent own state) | Environment + Memory + Documents |
| Communication | State Mutation (Dict Updates) | Tool-based Delegation | Message Passing (typed) | Pub-Sub with cause_by routing |
| Agent Identity | None (Nodes = Functions) | Role/Goal/Backstory | First Class (AgentId, Lifecycle) | Role/Profile/Actions/Watch |
| Orchestration | Graph Topology (Edges) | Process (seq/hierarchical) | Teams (5 types) | SOP Pipeline + TeamLeader |
| Persistence | Built-in Checkpointing | Flow Persistence | State Save/Load (manual) | Serialization + Git Repo |
| Distributed | Only Platform ($) | No | Yes (gRPC native) | No |
| LangChain Coupling | Yes (langchain-core) | No (removed) | No (optional) | No |
| Dependencies | Medium (~20 transitive) | Heavy (ChromaDB+LanceDB+OTel) | Minimal (Core), modular (Ext) | Very heavy (~90 direct) |

#### Feature Comparison

| Feature | LangGraph | CrewAI | AutoGen | MetaGPT |
|---|---|---|---|---|
| Sequential | Edges | Process.sequential | RoundRobin | SOP Pipeline |
| Hierarchical | Subgraphs | Process.hierarchical | SelectorGroupChat | TeamLeader Hub |
| DAG/Graph | StateGraph | Flow (@start/@listen) | GraphFlow (DiGraph) | No |
| Parallel | Send API | Flow (and_/or_) | GraphFlow (activation) | asyncio.gather |
| Handoff/Swarm | langgraph-swarm | DelegateWorkTool | Swarm + HandoffMessage | publish_message |
| Nested Teams | Subgraph as Node | Crew in Flow | SocietyOfMindAgent | No |
| Planning Loop | Custom Nodes | planning=True | MagenticOne | Plan-and-Act Mode |
| Human-in-Loop | interrupt() | human_input + Provider | UserProxyAgent | HumanProvider + AskReview |
| Streaming | 6 Modes | Token + Events | 3 Levels (Token/Agent/Team) | LLM-Level |
| Structured Output | No (LLM-native) | output_json/pydantic | StructuredMessage[T] | ActionNode + Review/Revise |
| Memory (Short) | Checkpointer | In Crew context | ChatCompletionContext | Message list |
| Memory (Long) | BaseStore (KV+Vector) | Unified (LanceDB) | ChromaDB/Redis/Mem0 | Vector (optional) |
| Tool System | ToolNode (LangChain) | BaseTool + @tool + MCP | Workbench + MCP | ToolRegistry + BM25 |
| Guardrails | RetryPolicy | LLM Guardrail Agent | Termination Conditions | Budget Enforcement |
| YAML Config | No | Agents + Tasks | Component System (JSON) | No |
| Event System | Debug Stream | Event Bus (60+ Types) | No | No |
| Experience Cache | No | No | No | Experience Pool |
| Document Pipeline | No | No | No | PRD->Design->Code |
| Cost Management | No | No | Token-based | Budget + NoMoneyException |

#### Synthesis: What CodeForge Adopts

**From LangGraph:**

| Concept | Implementation in CodeForge |
|---|---|
| StateGraph + Checkpointing | Orchestration layer in Python Workers, PostgresSaver |
| interrupt() for HITL | Plan->Approve->Execute workflow |
| 6 Streaming Modes | UI feedback (Token, State-Updates, Custom Events, Debug) |
| PostgresStore | Long-term memory backend |
| Functional API | Simpler workflows via @entrypoint/@task |

**From CrewAI:**

| Concept | Implementation in CodeForge |
|---|---|
| YAML Agent/Task Config | Agent specialization as YAML, GUI-configurable |
| Unified Memory (Composite Scoring) | Recall with Semantic + Recency + Importance weighting |
| LLM Guardrail Agent | Quality Layer: Agent validates agent output |
| Event Bus (60+ Events) | Observability for dashboard, monitoring, WebSocket |
| Flow DAG (@start/@listen/@router) | Inspiration for workflow editor in GUI |
| Human Feedback Provider Protocol | Extensible HITL channels (Web GUI, Slack, Email) |
| @tool Decorator | Clean tool definition for custom tools |

**From AutoGen:**

| Concept | Implementation in CodeForge |
|---|---|
| Layered Package Structure | Core -> AgentChat -> Extensions, clean separation |
| GraphFlow (DiGraphBuilder) | DAG + Conditional Edges + Parallel Nodes for agent coordination |
| Workbench (Tool Container) | Shared state for related tools, MCP integration |
| Termination Conditions (composable) | Flexible stop conditions with & / | operators |
| Context Window Strategies | Buffered, TokenLimited, HeadAndTail (no context overflow) |
| Component System (declarative) | Agents/Tools/Workflows as JSON -- essential for GUI editor |
| MagenticOne Orchestrator | Planning Loop + Stall Detection + Re-Planning |
| HandoffMessage Pattern | Agent handoff between specialists (Aider->OpenHands->SWE-agent) |
| SocietyOfMindAgent | Wrap team as agent for nested orchestration |

**From MetaGPT:**

| Concept | Implementation in CodeForge |
|---|---|
| Document-driven Pipeline | PRD->Design->Tasks->Code reduces hallucination |
| ActionNode (Schema Validation) | Enforced structures + Review/Revise cycles |
| Experience Pool (@exp_cache) | Cache successful runs, save costs |
| BM25 Tool Recommendation | Automatically select relevant tools |
| Budget Enforcement | Hard cost limits per task/project/user |
| Mermaid Generation | Automatic architecture visualization |
| Incremental Development | Consider existing code during generation |
| Per-Action LLM Override | Different models for different steps |

**Explicitly NOT Adopted:**

| Concept | Reason |
|---|---|
| LangChain Message Types | Own message format, LLM via LiteLLM |
| CrewAI's ChromaDB + LanceDB | Too heavy, PostgresStore + pgvector is sufficient |
| AutoGen's per-provider LLM Clients | LiteLLM routes everything uniformly |
| MetaGPT's 90 Dependencies | Lean workers, only what's needed |
| All Single-Process Runtimes | Go Core orchestrates, Python Workers execute |
| LangGraph Platform / CrewAI Enterprise | Self-hosted by design |
| AutoGen's gRPC Runtime | Go Core manages agent lifecycle via NATS/Redis |
| MetaGPT's Pydantic Inheritance Chains | Composition over deep inheritance |

---

### 10. Coding Agent Comparison: Cline vs Devika

#### Architecture Comparison

| Dimension | Cline | Devika |
|---|---|---|
| Type | VS Code Extension (+ CLI + JetBrains) | Standalone Web App |
| Backend | TypeScript (Node.js Extension Host) | Python (Flask + SocketIO) |
| Frontend | React (Webview in VS Code) | SvelteKit (Port 3001) |
| Communication | gRPC / Protocol Buffers | Socket.IO (WebSocket) + REST |
| Database | VS Code Storage + Filesystem + Secrets API | SQLite |
| LLM Integration | 40+ providers via ApiHandler Factory | Direct API calls per provider |
| Agent Model | Single agent with tool invocations | Multi-agent (9 specialized sub-agents) |
| Execution Model | Recursive Conversation Loop with HITL | Sequential Pipeline (Plan->Research->Code) |
| File Editing | Diff-based (Search/Replace + Side-by-Side Review) | Direct writing to disk |
| Approval Flow | Granular (per tool category, .clinerules) | None |
| Checkpointing | Shadow Git (isolated Git repo) | None |
| Browser | Integrated (Screenshot + Interaction) | Playwright + LLM-driven Crawler |
| MCP Support | Complete (Hub, Marketplace, Self-Build) | None |
| Context Management | Auto-Compact, Redundant-Read-Removal | None (full context) |
| Prompt Templates | In code (System-Prompt-Builder) | Jinja2 files per agent |
| Status | Active (4M+ users, regular releases) | Stagnated/abandoned (since mid-2025) |
| License | Apache 2.0 | MIT |

#### Feature Comparison

| Feature | Cline | Devika |
|---|---|---|
| Plan/Act Modes | Yes (separate models configurable) | Implicit (Planner agent, no user toggle) |
| Multi-LLM | 40+ Providers | 6 providers (Claude, GPT, Gemini, Mistral, Groq, Ollama) |
| Local Models | Ollama + LM Studio | Ollama |
| Cost Tracking | Real-time token/cost display | Basic token usage in agent state |
| Budget Limits | No (only tracking) | No |
| Web Research | Via MCP servers | Built-in (Bing/Google/DuckDuckGo + Crawler) |
| Code Execution | Terminal commands with approval | Runner agent (Multi-OS Sandbox) |
| Git Integration | Shadow Git + Workspace Git | GitHub Clone only |
| Deployment | No | Netlify integration |
| Project Management | No | Basic (project-based organization) |
| Keyword Extraction | No | SentenceBERT |
| Report Generation | No | Reporter agent (PDF) |
| Enterprise | SSO, RBAC, Audit, VPC | No |
| Cross-Editor | ACP (VS Code, Zed, JetBrains) | Standalone (browser-based) |

#### Synthesis: What CodeForge Adopts from Cline

| Concept | Implementation in CodeForge |
|---|---|
| Plan/Act Mode | Plan->Approve->Execute workflow with separate LLM configs per phase |
| Shadow Git Checkpoints | Rollback mechanism in agent containers (Git-based) |
| Ask/Say Approval Pattern | Web-GUI-based approval flow with granular permissions |
| MCP as Extensibility | MCP servers as tool extension (standard protocol) |
| .clinerules Pattern | YAML-based project configuration (agent behavior, permissions) |
| Auto-Compact Context | History Processors with automatic summarization at ~80% |
| Diff-based File Review | Side-by-side diff display in web GUI before approval |
| Tool Categorization (5) | Files, Terminal, Browser, MCP, Context as tool taxonomy |
| Auto-Approve Granularity | Per-tool-category autonomy level in project settings |
| Provider Routing (Plan vs Act) | LiteLLM Tags (plan/default) for different models |

#### Synthesis: What CodeForge Adopts from Devika

| Concept | Implementation in CodeForge |
|---|---|
| Sub-Agent Architecture | Worker modules with Planner/Researcher/Coder separation |
| Jinja2 Prompt Templates | Prompts as separate template files, not in code |
| SentenceBERT/KeyBERT Keywords | Semantic keyword extraction for better retrieval |
| Agent State Visualization | Real-time dashboard (Internal Monologue, Steps, Browser, Terminal) |
| LLM-driven Web Crawler | Web research worker (Page -> LLM -> Action loop) |
| Stateless Agent Design | Worker modules stateless, state in Go Core / Message Queue |
| Sequential Pipeline | Plan->Research->Code as base workflow (extensible via DAG) |
| Domain Experts | Specialized knowledge modules per domain |
| Socket.IO Real-Time | WebSocket-based live updates (CodeForge: native WS) |

#### Explicitly NOT Adopted (Cline + Devika)

| Concept | Reason |
|---|---|
| Cline's VS Code Binding | CodeForge is standalone service (Docker), not IDE extension |
| Cline's gRPC/Protobuf UI Communication | Web GUI via REST + WebSocket (simpler, sufficient) |
| Cline's Shadow Git in Extension | CodeForge uses Git in agent containers (Docker-native) |
| Cline's Single-Agent Architecture | CodeForge: Multi-agent orchestration via Go Core |
| Cline's Provider-specific ApiHandler | LiteLLM as unified proxy (no custom provider interface) |
| Devika's Flask Single-Process | Go Core + Python Workers via NATS/Redis |
| Devika's SQLite | PostgreSQL (production-grade) |
| Devika's Missing Approval Flow | Human-in-the-loop is a core principle of CodeForge |
| Devika's Missing Checkpoints | Git-based checkpointing in agent containers |
| Devika's Direct Provider Calls | LiteLLM routes everything uniformly |
| Devika's Browser as Required Dependency | Browser optional, only for web research tasks |

---

### 11. Extended Competitor Analysis: 12 New Tools (Overview)

#### Complete Overview

| # | Name | Stars | License | Type | Stack | CodeForge Relevance |
|---|------|-------|--------|-----|-------|-------------------|
| 1 | Codel | ~2,400 | AGPL-3.0 | Competitor | Go + React | High (similar architecture) |
| 2 | AutoForge | ~1,600 | TBD | Competitor | Python + React | Medium (Multi-Session Pattern) |
| 3 | bolt.diy | ~19,000 | MIT* | Competitor | TypeScript/Remix | Medium (Multi-LLM App Builder) |
| 4 | Dyad | ~16,800 | Apache 2.0 | Competitor | TypeScript/Electron | Low-Medium (Local App Builder) |
| 5 | CAO (AWS) | ~210 | Apache 2.0 | Competitor | Python/tmux/MCP | Very high (Multi-Agent Orchestrator) |
| 6 | Goose | ~30,400 | Apache 2.0 | Backend | Rust | High (MCP-native Agent) |
| 7 | OpenCode | ~100,000 | MIT | Backend | Go | High (Client/Server Go Agent) |
| 8 | Plandex | ~14,700 | MIT | Backend | Go | High (Planning-First Go Agent) |
| 9 | AutoCodeRover | ~2,800 | GPL-3.0 | Backend | Python | Medium (AST-aware, GPL) |
| 10 | Roo Code | ~22,200 | Apache 2.0 | Both | TypeScript | High (Modes System, Cloud Agents) |
| 11 | Codex CLI | ~55,000 | Apache 2.0 | Backend | TypeScript | High (OpenAI official, GH Action) |
| 12 | SERA | New | Apache 2.0 | Backend Model | Python | High (Self-Hosted Open Model) |

#### Backend Integration Priorities

Priority 1 -- Go-native, MIT/Apache 2.0, high community support:

- Goose -- MCP-native, Rust with bindings, 30k+ stars, Apache 2.0
- OpenCode -- Go-based (same stack), Client/Server, MIT, 100k+ stars
- Plandex -- Go-based, Planning-First with Diff-Sandbox, MIT, Dockerized

Priority 2 -- Strong features, good license:

- Codex CLI -- OpenAI official, Multimodal, GitHub Action, Apache 2.0
- Roo Code -- Modes system, Cloud Agents, Headless potential, Apache 2.0

Priority 3 -- Niche candidates:

- SERA -- Open Model Weights for self-hosting (Ollama/vLLM behind LiteLLM)
- AutoCodeRover -- AST-aware, $0.70/task, but GPL-3.0

#### Closest Competitor to Watch

**CLI Agent Orchestrator (AWS):** Although currently small (~210 stars), it is AWS-backed and uses the same multi-agent orchestration pattern (Supervisor/Worker, tmux/MCP, support for Claude Code + Aider). Could grow quickly. Missing: Web GUI, project dashboard, roadmap -- exactly CodeForge's differentiation.

#### Synthesis: New Patterns from Extended Analysis

| Pattern | Source | Application in CodeForge |
|---|---|---|
| Supervisor/Worker via tmux/MCP | CAO (AWS) | Reference for agent session isolation |
| Client/Server Agent Architecture | OpenCode | Agent as server, CodeForge Core as client |
| Modes System (Agent Roles) | Roo Code | YAML-configurable agent specialization |
| Cumulative Diff Sandbox | Plandex | Changes remain separate until approval |
| Two-Agent Pattern (Initializer+Coder) | AutoForge | Test-first agent workflow |
| MCP-native Tool Extensibility | Goose | Standard protocol for tool integration |
| Open Model Weights Deployment | SERA | Self-hosted LLM via Ollama/vLLM behind LiteLLM |
| GitHub Action CI/CD Pattern | Codex CLI | Agent execution in CI/CD pipelines |
| AST-based Code Search | AutoCodeRover | Complement to tree-sitter Repo Map |
| Smart Docker Image Picker | Codel | Automatic container image selection per task |
| Flow Scheduling (Cron) | CAO (AWS) | Unattended automatic agent execution |
| Cloud Agent Delegation | Roo Code | Delegate work via Web/Slack/GitHub |
