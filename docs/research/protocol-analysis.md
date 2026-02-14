# Protocol & Standards Analysis for CodeForge

> Date: 2026-02-14
> Status: Research complete — recommendations for architecture integration

## Executive Summary

The agent protocol landscape has consolidated rapidly since late 2024. Three protocols now dominate under the Linux Foundation's Agentic AI Foundation (AAIF): **MCP** (agent-to-tool), **A2A** (agent-to-agent), and **AGENTS.md** (agent-to-codebase instructions). For CodeForge as an agent orchestrator, MCP is the single most important protocol to implement deeply. A2A becomes relevant when CodeForge's agents need to interoperate with external agent systems. The remaining protocols (LSP, OpenTelemetry, SARIF, DAP) provide significant value as supporting infrastructure.

This document evaluates 20+ protocols across 5 categories and provides prioritized recommendations aligned with CodeForge's phased development.

---

## 1. Agent Communication Protocols

### 1.1 MCP (Model Context Protocol) — Anthropic / AAIF

**What it is:** An open standard (November 2024) that provides a universal interface for AI models to access external tools, data sources, and services. Uses JSON-RPC 2.0 over stdio (local) or HTTP+SSE (remote). Donated to the Linux Foundation's AAIF in December 2025.

**Current status (February 2026):**
- Industry standard: adopted by OpenAI, Google, Microsoft, Amazon
- 8M+ server downloads, 5,800+ MCP servers, 300+ MCP clients, 97M+ monthly SDK downloads
- Spec version 2025-11-25 added: async operations, statelessness, server identity, official registry, OAuth 2.1 auth
- Governed by AAIF under Linux Foundation (Anthropic, OpenAI, Google, Microsoft, Amazon, Block, Cloudflare)

**Architecture: Host / Client / Server roles:**
- **Host**: The user-facing application (IDE, chat app) that contains the LLM and manages MCP clients
- **Client**: Component within the Host that maintains a 1:1 connection to a single MCP Server
- **Server**: External program exposing tools, resources, and prompts to AI models

**How CodeForge should use MCP:**

CodeForge should implement MCP in **three roles**:

1. **As an MCP Host** (primary role): CodeForge is the user-facing application that orchestrates agents. The Go Core acts as the Host, creating MCP Clients to connect to external MCP Servers. This gives CodeForge's agents access to the entire MCP ecosystem (5,800+ servers for databases, APIs, cloud services, etc.).

2. **As an MCP Client** (within the Host): Each Python Worker spawns MCP Client connections to relevant MCP Servers based on the task context. The `McpWorkbench` pattern (already planned in `workers/codeforge/execution/workbench.py`) is the right abstraction — it wraps MCP server connections as tool containers.

3. **As an MCP Server** (secondary role): CodeForge exposes its own capabilities as MCP tools so that external MCP clients (Claude Desktop, VS Code Copilot, Cursor, etc.) can invoke CodeForge tasks. This means users can trigger CodeForge agent workflows directly from their IDE.

**Implementation complexity:** Medium. SDKs exist for Python (workers) and TypeScript (frontend). The Go Core needs either a Go MCP SDK (emerging) or can proxy through the Python workers. The `mcp-agent` framework by lastmile-ai provides composable patterns (map-reduce, orchestrator, evaluator-optimizer) that align with CodeForge's existing workflow architecture.

**Key pattern — Code Execution with MCP (Anthropic):** Rather than loading all MCP tools into the context window, agents write code to interact with MCP servers, loading tools on demand. This aligns with CodeForge's YAML tool bundles and BM25 tool recommendation — only inject relevant MCP tools per task.

**Priority:** **Phase 1 (essential)** — MCP Client/Host in Python Workers for tool access. Phase 2 for MCP Server exposure.

**Sources:**
- [MCP Specification (2025-11-25)](https://modelcontextprotocol.io/specification/2025-11-25)
- [MCP Architecture](https://modelcontextprotocol.io/specification/2025-06-18/architecture)
- [Anthropic: Code Execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp)
- [IBM: MCP Architecture Patterns for Multi-Agent AI](https://developer.ibm.com/articles/mcp-architecture-patterns-ai-systems/)
- [mcp-agent framework](https://github.com/lastmile-ai/mcp-agent)
- [AAIF Announcement](https://www.anthropic.com/news/donating-the-model-context-protocol-and-establishing-of-the-agentic-ai-foundation)

---

### 1.2 A2A (Agent-to-Agent Protocol) — Google / Linux Foundation

**What it is:** An open protocol for communication between opaque AI agents from different providers and frameworks. Announced April 2025 at Google Cloud Next. Uses JSON-RPC 2.0 over HTTP(S). Now governed by the Linux Foundation (v0.3, July 2025).

**Key concepts:**
- **Agent Card**: JSON file at `/.well-known/agent.json` advertising name, endpoint, skills, and supported auth flows
- **Task lifecycle**: Tasks can complete immediately or long-running with status updates
- **Artifacts**: Output of completed tasks
- **Flexible interaction**: Supports synchronous request/response, streaming (SSE), and async push notifications
- **Security**: OAuth 2.0, OpenID Connect, API keys (aligned with OpenAPI spec)

**A2A vs MCP — complementary, not competing:**
- MCP = agent-to-tool (how an agent connects to tools, APIs, and data sources)
- A2A = agent-to-agent (how agents from different systems collaborate)
- A2A is the "public internet" for agent interoperability; MCP is the "local network" for tool access

**Relevance for CodeForge:**

A2A is relevant in two scenarios:

1. **CodeForge agents as A2A servers**: External agents can discover and delegate tasks to CodeForge's specialized agents (e.g., an enterprise workflow agent asks CodeForge to "review this PR" or "implement this feature"). CodeForge publishes an Agent Card describing its capabilities.

2. **CodeForge consuming external A2A agents**: CodeForge's orchestrator can discover and delegate to external specialized agents (e.g., a security scanning agent, a documentation agent) that expose A2A endpoints.

**Implementation complexity:** Low-Medium. A2A is HTTP-based (no special transport). Publishing an Agent Card is trivial. The task lifecycle model maps well to CodeForge's existing task workflow (Plan/Approve/Execute/Review/Deliver). Python SDK available.

**Note on ACP merger:** IBM's Agent Communication Protocol (ACP) has officially merged with A2A under the Linux Foundation. ACP is being wound down and users are advised to migrate to A2A. Kate Blair (IBM) joins the A2A Technical Steering Committee.

**Priority:** **Phase 3 (nice-to-have)** — A2A becomes important when CodeForge needs to interoperate with external agent ecosystems. For internal agent-to-agent communication (between CodeForge's own agents), NATS JetStream + the HandoffMessage pattern is simpler and faster.

**Sources:**
- [A2A Protocol Documentation](https://a2a-protocol.org/latest/)
- [A2A GitHub](https://github.com/a2aproject/A2A)
- [Google Developers Blog: A2A Announcement](https://developers.googleblog.com/en/a2a-a-new-era-of-agent-interoperability/)
- [ACP Merges with A2A](https://lfaidata.foundation/communityblog/2025/08/29/acp-joins-forces-with-a2a-under-the-linux-foundations-lf-ai-data/)
- [IBM: What is ACP?](https://www.ibm.com/think/topics/agent-communication-protocol)

---

### 1.3 ACP (Agent Communication Protocol) — IBM/BeeAI (MERGED INTO A2A)

**What it was:** REST-based agent-to-agent protocol launched March 2025 by IBM Research for the BeeAI platform. Distinguished by requiring no SDK (plain HTTP), offline discovery, and OTLP-instrumented observability.

**Current status:** **Merged into A2A** under the Linux Foundation (August 2025). ACP development is winding down. BeeAI platform now uses A2A.

**Recommendation for CodeForge:** **Skip ACP entirely.** Use A2A when agent-to-agent interoperability is needed. The best ideas from ACP (REST simplicity, offline discovery, observability) are being incorporated into A2A.

**Sources:**
- [ACP Joins Forces with A2A](https://lfaidata.foundation/communityblog/2025/08/29/acp-joins-forces-with-a2a-under-the-linux-foundations-lf-ai-data/)
- [IBM: ACP Technical Overview (WorkOS)](https://workos.com/blog/ibm-agent-communication-protocol-acp)

---

### 1.4 AG-UI (Agent-User Interaction Protocol) — CopilotKit

**What it is:** An open protocol (May 2025) for bi-directional communication between AI agents and user-facing applications. Streams JSON events over HTTP: messages, tool calls, state patches, and lifecycle signals. 9,000+ GitHub stars, 120,000+ weekly installs.

**Key features:**
- Shared state synchronization (agent and application state, read/write or read-only)
- Guardrails at the boundary (prompt injection, data leaks, compliance)
- AI product analytics (every agent-user interaction)
- Self-improving agents via implicit human feedback (RLiHF)

**How AG-UI differs from MCP and A2A:**
- MCP = agent-to-tool
- A2A = agent-to-agent
- AG-UI = agent-to-user (the frontend interaction layer)

**Relevance for CodeForge:**

AG-UI addresses the exact layer CodeForge needs for its SolidJS frontend: streaming agent state, tool call results, plan approvals, and live logs to the user interface. However, CodeForge already plans a custom WebSocket-based event system (Event Bus from CrewAI pattern) that covers similar ground.

**Assessment:** AG-UI is designed for React-based applications (the `useAgent` hook). CodeForge uses SolidJS, which would require either a SolidJS adapter or manual event stream handling. The WebSocket + Event Bus architecture already planned in `workers/codeforge/events/bus.py` achieves the same goal with less external dependency.

**Priority:** **Phase 3+ (monitor)** — If AG-UI gains a SolidJS adapter or becomes framework-agnostic, reconsider. For now, the custom WebSocket event system is the right choice. AG-UI's guardrail patterns (prompt injection at the boundary) are worth studying for the Safety Layer regardless.

**Sources:**
- [AG-UI Documentation](https://docs.ag-ui.com/)
- [AG-UI Protocol (CopilotKit)](https://www.copilotkit.ai/ag-ui)
- [CopilotKit v1.50 useAgent Hook](https://www.marktechpost.com/2025/12/11/copilotkit-v1-50-brings-ag-ui-agents-directly-into-your-app-with-the-new-useagent-hook/)
- [AG-UI GitHub](https://github.com/ag-ui-protocol/ag-ui)

---

### 1.5 ANP (Agent Network Protocol)

**What it is:** A decentralized protocol for agent discovery, identity, and encrypted communication across organizations. Uses W3C Decentralized Identifiers (DIDs), JSON-LD metadata, and end-to-end encryption. Aims to be the "HTTP of the Agentic Web."

**Three-layer architecture:**
1. Identity and encrypted communication (DIDs, cryptographic signatures)
2. Meta-protocol negotiation (agents negotiate how to communicate)
3. Application protocol (structured capability discovery via JSON-LD)

**Relevance for CodeForge:** ANP solves the problem of agents from different organizations discovering and trusting each other across the open internet. This is relevant for enterprise scenarios where CodeForge instances at different companies need to collaborate, or for an agent marketplace.

**Priority:** **Skip for now** — ANP addresses internet-scale agent federation, which is far beyond CodeForge's current scope. A2A covers the agent interoperability needs CodeForge will have in Phase 3. Revisit only if CodeForge enters a multi-organization federation scenario.

**Sources:**
- [ANP Official Website](https://agent-network-protocol.com/)
- [ANP GitHub](https://github.com/agent-network-protocol/AgentNetworkProtocol)
- [Survey: MCP, ACP, A2A, ANP (arXiv)](https://arxiv.org/html/2505.02279v1)

---

### 1.6 AGENTS.md — OpenAI / AAIF

**What it is:** A simple, open Markdown format for providing AI coding agents with project-specific instructions. Think of it as a README for agents. Adopted by 60,000+ open-source projects since August 2025. Now governed by the AAIF under the Linux Foundation.

**How it works:** An `AGENTS.md` file at the repo root (or in subdirectories) contains build steps, coding conventions, test commands, and other context that coding agents need. Agents automatically read the nearest file in the directory tree.

**Relevance for CodeForge:**

CodeForge should both **read** and **write** AGENTS.md:

1. **Read**: When CodeForge clones a repo, the Context Layer should automatically parse any `AGENTS.md` files and inject their content into agent prompts. This provides project-specific instructions without user configuration.

2. **Write**: CodeForge could generate or update `AGENTS.md` files based on learned project conventions, similar to how `.codeforge/microagents/` work but in a standardized format other tools understand.

3. **Interoperability**: Since AGENTS.md is supported by Amp, Codex, Cursor, Devin, Gemini CLI, GitHub Copilot, Jules, and VS Code, CodeForge agents benefit from instructions already written for other tools.

**Implementation complexity:** Trivial. It is just Markdown parsing. No SDK, no protocol, no transport layer.

**Priority:** **Phase 1 (essential)** — Reading AGENTS.md during repo analysis is near-zero effort and immediately valuable. Writing/updating can come in Phase 2.

**Sources:**
- [AGENTS.md Specification](https://agents.md/)
- [AAIF Announcement (Linux Foundation)](https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation)
- [OpenAI: Agentic AI Foundation](https://openai.com/index/agentic-ai-foundation/)

---

### 1.7 OpenAI Tool Calling / Function Calling

**What it is:** OpenAI's standard for structured tool/function calling from LLMs. JSON Schema-based function definitions, strict mode for guaranteed schema adherence. The de facto standard that LiteLLM normalizes all providers to.

**Relevance for CodeForge:** Already fully covered by the LiteLLM integration. LiteLLM handles the conversion of tool calling across 127+ providers. CodeForge's YAML tool bundles are converted to OpenAI function calling format. No additional work needed.

**Priority:** **Already handled** — via LiteLLM. The only action item is ensuring YAML tool bundles produce valid JSON Schema for strict mode compliance.

**Sources:**
- [OpenAI Function Calling Guide](https://platform.openai.com/docs/guides/function-calling)
- [OpenAI Agents SDK](https://openai.github.io/openai-agents-python/)

---

## 2. Code Intelligence Protocols

### 2.1 LSP (Language Server Protocol)

**What it is:** A protocol (by Microsoft, 2016) that standardizes how IDEs communicate with language-specific analysis servers. Provides go-to-definition, find references, diagnostics, completions, hover documentation, and symbol navigation.

**2025 landscape: LSP + AI coding agents:**
- Claude Code added native LSP integration (December 2025) supporting 11 languages
- Performance: finding all call sites takes ~50ms with LSP vs ~45 seconds with text search (900x improvement)
- OpenCode had LSP integration earlier, demonstrating per-language loading strategies
- Multiple MCP servers now bridge LSP to AI agents (cclsp, LSP-MCP, tree-sitter-analyzer)
- Emerging "Agent Client Protocol" (ACP, by Zed Editor, unrelated to IBM ACP) proposes LSP-like standardization for AI agents

**Relevance for CodeForge:**

LSP integration would give CodeForge's agents precise, semantic code understanding:
- **Go-to-definition**: Find where a function/class is defined across the entire codebase
- **Find references**: Locate all call sites of a function (critical for refactoring)
- **Diagnostics**: Real-time error/warning detection without running the compiler
- **Symbol navigation**: Structured understanding of code hierarchy

This directly improves the Context Layer (GraphRAG). Instead of relying solely on tree-sitter for repo maps and vector search for code retrieval, LSP provides deterministic, precise answers that save tokens and reduce hallucination.

**Implementation approach:** Run language servers inside agent sandbox containers or as sidecar services. Expose LSP capabilities as MCP tools (using existing LSP-MCP bridge servers). This avoids building a custom LSP client in Go — instead, the Python workers connect to LSP servers via the standard protocol and expose results as tool calls.

**Key consideration:** LSP servers are per-language (TypeScript needs `typescript-language-server`, Python needs `pyright` or `pylsp`, Go needs `gopls`, etc.). CodeForge must auto-detect the repo's primary languages and start the right servers. This adds container complexity but is worth the precision improvement.

**Implementation complexity:** Medium-High. Requires managing multiple language server processes, handling LSP's stateful connection model, and auto-detecting which servers to start. The MCP-bridge approach reduces this significantly.

**Priority:** **Phase 2 (important)** — LSP integration via MCP bridge servers provides a massive quality improvement for code understanding. Start with Python + TypeScript + Go (the three languages CodeForge itself uses), expand from there.

**Sources:**
- [Claude Code LSP Setup Guide](https://www.aifreeapi.com/en/posts/claude-code-lsp)
- [Using Coding Agents with LSP on Large Codebases](https://medium.com/@dconsonni/using-coding-agents-with-language-server-protocols-on-large-codebases-24334bfff834)
- [cclsp: Claude Code LSP via MCP](https://github.com/ktnyt/cclsp)
- [LSP-AI: Open Source Language Server with AI](https://github.com/SilasMarvin/lsp-ai)
- [OpenCode: LSP Integration](https://deepwiki.com/opencode-ai/opencode/8-language-server-integration)

---

### 2.2 DAP (Debug Adapter Protocol)

**What it is:** A protocol (by Microsoft) that abstracts debugger communication, similar to how LSP abstracts language analysis. Supports breakpoints, stepping, variable inspection, and expression evaluation across languages.

**2025 landscape: DAP + AI agents:**
- Multiple MCP servers now bridge DAP to AI agents: debugger-mcp (Rust), mcp-debugger (TypeScript), dap-mcp (Python), AIDB
- Proven pattern: AI agent communicates via MCP, MCP server translates to DAP, DAP controls language-specific debug adapters (debugpy, delve, CodeLLDB, etc.)
- Claude Code and Codex can autonomously debug programs via MCP-DAP bridges across Python, Ruby, Node.js, Go, and Rust

**Relevance for CodeForge:**

DAP gives CodeForge's `debugger` mode actual debugging capability beyond "read the error message and guess":
- Set breakpoints, step through code, inspect variables
- Reproduce bugs programmatically
- Investigate state at failure points
- CI/CD integration for automated root cause analysis

**Implementation approach:** Same as LSP — use existing MCP-DAP bridge servers rather than building a custom DAP client. The `debugger` agent mode (already defined in architecture) would gain access to DAP tools via MCP.

**Implementation complexity:** Low-Medium. The MCP-DAP bridges already exist and have 90%+ test coverage. The main work is integrating the bridge servers into CodeForge's sandbox containers.

**Priority:** **Phase 3 (nice-to-have)** — Debugging is powerful but less critical than core code editing and analysis. The `debugger` mode can start with log analysis and test reproduction before adding full DAP integration.

**Sources:**
- [DAP Official Specification](https://microsoft.github.io/debug-adapter-protocol/)
- [debugger-mcp: DAP for AI Agents (Rust)](https://github.com/Govinda-Fichtner/debugger-mcp)
- [AIDB: AI Debugger Backend](https://github.com/ai-debugger-inc/aidb)

---

### 2.3 Tree-sitter

**What it is:** A parser generator and incremental parsing library (not a protocol). Written in C, dependency-free, with bindings for Rust, Python, Go, JavaScript, and more. Parses source code into concrete syntax trees (CSTs) for structural analysis.

**Relevance for CodeForge:** Already planned for the tree-sitter repo map (from Aider's pattern). Tree-sitter is a library, not a protocol — it runs locally within the Python workers. Key ecosystem developments:

- `go-repomap`: Go implementation of Aider's repo map using tree-sitter (could be used by Go Core for fast indexing)
- `tree-sitter-analyzer`: Python framework with MCP server support, supporting multi-language analysis
- Aider's approach: tree-sitter extracts definitions + references, PageRank ranks by relevance, generates compact repo map

**Implementation complexity:** Low. Python `tree-sitter` bindings are mature. Go bindings exist via `go-tree-sitter`. The main work is parsing 40+ language grammars and building the repo map ranking algorithm.

**Priority:** **Phase 1-2 (essential)** — Already planned as part of the Context Layer. No protocol integration needed — it is a library. Implement in Python workers for the repo map, optionally in Go Core for fast initial indexing.

**Sources:**
- [Aider: Building a Repository Map with Tree-sitter](https://aider.chat/2023/10/22/repomap.html)
- [go-repomap: Go Tree-sitter Repo Map](https://github.com/entrepeneur4lyf/go-repomap)
- [tree-sitter-analyzer with MCP](https://pypi.org/project/tree-sitter-analyzer/1.7.1/)
- [tree-sitter Ecosystem Map](https://dcreager.net/tree-sitter/map/)

---

### 2.4 SARIF (Static Analysis Results Interchange Format)

**What it is:** An OASIS standard (JSON-based) for representing the output of static analysis tools. Version 2.1.0. Provides a rich schema for findings including source location, severity, remediation guidance, CWE/CVE identifiers.

**2025 developments:**
- "AI-Native SARIF" concept: embedding prompts and code context directly in SARIF files for AI triage
- SARIF enrichment pipelines at LinkedIn (metadata + remediation info)
- SDKs available for .NET, JavaScript, Java, Python
- GitHub Code Scanning natively consumes SARIF

**Relevance for CodeForge:**

SARIF is valuable in two ways:

1. **Input**: CodeForge's `security-auditor` and `reviewer` modes can consume SARIF output from existing static analysis tools (bandit, semgrep, ESLint, SonarQube). This gives agents structured, machine-readable analysis results to reason about instead of parsing tool-specific text output.

2. **Output**: CodeForge's agent review results could be emitted as SARIF, making them consumable by GitHub Code Scanning, VS Code, and other tools that understand SARIF. This provides interoperability with existing security workflows.

**Implementation complexity:** Low. SARIF is just a JSON schema. Python and Go libraries exist for reading/writing. No runtime dependency.

**Priority:** **Phase 2-3 (nice-to-have)** — Consuming SARIF from external tools is easy and immediately useful for the `reviewer` and `security-auditor` modes. Emitting SARIF is a later optimization for CI/CD integration.

**Sources:**
- [SARIF Standard (OASIS)](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
- [AI-Native SARIF](https://parsiya.net/blog/ai-native-sarif/)
- [SARIF Complete Guide (Sonar)](https://www.sonarsource.com/resources/library/sarif/)

---

## 3. Observability Protocols

### 3.1 OpenTelemetry (OTEL)

**What it is:** The CNCF standard for distributed tracing, metrics, and logs. The GenAI Semantic Conventions SIG (started April 2024) defines standard attribute names for LLM calls, agent steps, sessions, token counts, costs, and quality metrics.

**Current status of GenAI conventions (February 2026):**
- Status: **Development/Experimental** (not yet stable)
- Defines schemas for: LLM client spans, agent spans, tool call events, token usage metrics
- Provider-specific conventions for Azure AI, OpenAI, AWS Bedrock
- Agentic systems proposal: attributes for tasks, actions, agents, teams, artifacts, memory
- Vendor adoption: Datadog (native), Langfuse (v3 SDK), VictoriaMetrics, AG2 (built-in tracing)

**Key instrumentation libraries:**
- **OpenLLMetry** (Traceloop): Open-source OTel extensions for LLM apps, Apache 2.0
- **OpenLIT**: One-line-of-code monitoring for AI stack (LLMs, vector DBs, GPUs)
- LiteLLM already has 42+ observability integrations including Prometheus and can emit OTEL-compatible data

**Relevance for CodeForge:**

OpenTelemetry is highly relevant for three reasons:

1. **Agent run tracing**: Each agent workflow (Plan/Execute/Review/Deliver) maps naturally to OTel spans. Sub-spans for individual LLM calls, tool executions, and agent steps. This provides the detailed observability needed for debugging failed runs, optimizing costs, and comparing agents.

2. **Cross-service correlation**: CodeForge has three services (Go Core, Python Workers, LiteLLM). OTel trace context propagation through NATS messages connects the full request lifecycle from frontend click to LLM response.

3. **Vendor-agnostic export**: OTel data can be sent to any backend (Jaeger, Grafana Tempo, Datadog, Langfuse) without code changes. This aligns with CodeForge's provider-agnostic philosophy.

**Implementation approach:**
- Python Workers: Use OpenLLMetry or the official OTel GenAI instrumentation for automatic LLM call tracing
- Go Core: Use the official Go OTel SDK for HTTP/gRPC span creation
- NATS: Propagate trace context in message headers
- Export: OTel Collector sidecar or direct export to configurable backends

**Relationship to existing plans:** The Event Bus (from CrewAI pattern) emits events for the frontend. OTel traces are the backend-side equivalent for operational observability. They complement each other — events are for users, traces are for operators.

**Implementation complexity:** Medium. OTel SDKs exist for Go, Python, and TypeScript. The GenAI semantic conventions are still experimental, so attribute names may change. LiteLLM's existing Prometheus/Langfuse integration covers LLM-layer observability immediately.

**Priority:** **Phase 2 (important)** — Basic OTel tracing for request lifecycle. Phase 3 for full GenAI semantic conventions once they stabilize.

**Sources:**
- [OTel: AI Agent Observability Blog](https://opentelemetry.io/blog/2025/ai-agent-observability/)
- [OTel GenAI Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/)
- [OTel GenAI Agent Spans](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/)
- [OpenLLMetry](https://github.com/traceloop/openllmetry)
- [AG2 OpenTelemetry Tracing](https://docs.ag2.ai/latest/docs/blog/2026/02/08/AG2-OpenTelemetry-Tracing/)
- [Datadog OTel GenAI Support](https://www.datadoghq.com/blog/llm-otel-semantic-convention/)

---

### 3.2 Prometheus Metrics

**What it is:** The CNCF standard for time-series metrics collection via a pull-based scrape model. LiteLLM already exposes comprehensive Prometheus metrics.

**LiteLLM Prometheus integration (v1.80.0+, open-source):**
- Token consumption: `litellm_input_tokens_metric`, `litellm_output_tokens_metric`, `litellm_total_tokens_metric`
- Request tracking: `litellm_proxy_total_requests_metric`
- Deployment health: `litellm_deployment_success_responses`, `litellm_deployment_failure_responses`
- Budget tracking: `litellm_remaining_team_budget_metric`
- Custom tags as Prometheus labels (e.g., User-Agent tracking per agent)
- Selective metric groups to control cardinality
- Authentication for `/metrics` endpoint

**Relevance for CodeForge:**

Prometheus metrics are already available through LiteLLM. CodeForge should:

1. **Expose LiteLLM's metrics**: Configure LiteLLM's Prometheus metrics and expose them to the host for scraping
2. **Add CodeForge-specific metrics**: Agent task counts, queue depth, execution times, sandbox container lifecycle
3. **Build Grafana dashboards**: Cost per model over time, P50/P90/P99 latencies, error rates, agent success rates

**Implementation complexity:** Low. LiteLLM handles the LLM metrics. Adding Go metrics uses the `prometheus/client_golang` library. Python metrics use `prometheus_client`.

**Priority:** **Phase 2 (important)** — LiteLLM metrics are available out-of-the-box. CodeForge-specific metrics require minimal additional work. This directly powers the Cost Dashboard (already planned in the architecture).

**Sources:**
- [LiteLLM Prometheus Metrics](https://docs.litellm.ai/docs/proxy/prometheus)
- [LiteLLM v1.80.0 Release Notes](https://docs.litellm.ai/release_notes/v1-80-0)

---

## 4. Authentication & Authorization

### 4.1 OAuth 2.0 / OAuth 2.1 / OIDC

**What they are:**
- **OAuth 2.0**: Authorization framework for delegated access (RFC 6749)
- **OAuth 2.1**: Simplified, security-hardened update (consolidates best practices, deprecates implicit grant)
- **OIDC (OpenID Connect)**: Identity layer on top of OAuth 2.0 for authentication

**Relevance for CodeForge:**

OAuth/OIDC is required for multiple integration points:

1. **GitHub/GitLab authentication**: Clone private repos, create PRs, access issues. Both platforms use OAuth 2.0 with OIDC support.
2. **MCP remote server auth**: The MCP spec (2025-11-25) mandates OAuth 2.1 for remote server authorization. CodeForge as an MCP host must implement the OAuth flow.
3. **PM platform auth**: Plane.so, OpenProject, and other PM tools use OAuth 2.0 for API access.
4. **User login**: CodeForge's own authentication can use OIDC to federate with GitHub, GitLab, Google, etc.
5. **CI/CD OIDC tokens**: GitHub Actions and GitLab CI emit OIDC tokens for keyless authentication.

**Implementation complexity:** Medium. The Go ecosystem has mature OAuth/OIDC libraries (`golang.org/x/oauth2`, `coreos/go-oidc`). The key complexity is managing multiple OAuth flows for different providers simultaneously.

**Priority:** **Phase 1-2 (essential)** — GitHub OAuth is needed for basic repo access. Full OIDC for user login in Phase 2. MCP OAuth 2.1 when implementing remote MCP servers.

---

### 4.2 SCIM (System for Cross-domain Identity Management)

**What it is:** An open standard (RFC 7643/7644) for automated user provisioning and deprovisioning across systems. REST-based with JSON payloads.

**2025 developments:**
- Emerging SCIM Agent extension: `Agents` and `AgenticApplications` as first-class SCIM objects
- OpenAI ChatGPT Enterprise supports SCIM for automated user provisioning
- Grafana added SCIM integration in 2025

**Relevance for CodeForge:** SCIM is relevant only for enterprise multi-tenancy: automated onboarding/offboarding of users from corporate identity providers (Okta, Azure AD, etc.). Not needed for single-instance or small-team deployments.

**Priority:** **Phase 3+ (future)** — Only when multi-tenancy is implemented. Single-team instances can use simple user management or OIDC federation.

---

## 5. API Standards

### 5.1 OpenAPI 3.1 / 3.2

**What it is:** The standard for describing REST APIs. Version 3.1 (February 2021) aligned with JSON Schema 2020-12. Version 3.2.0 (September 2025) adds structured tag navigation, streaming media types, and new OAuth flows.

**Relevance for CodeForge:**

1. **API documentation**: CodeForge's REST API should be documented with OpenAPI. This enables auto-generated API docs, client SDKs, and validation.
2. **MCP tool discovery**: MCP tools can be generated from OpenAPI specs. This means any service with an OpenAPI spec can become an MCP server automatically (Apigee already does this).
3. **A2A Agent Card**: The A2A Agent Card references security schemes from the OpenAPI specification.

**Implementation complexity:** Low. Go has `swaggo/swag` for auto-generation from annotations, or write the spec manually and validate handlers against it.

**Priority:** **Phase 1 (essential)** — Define the API spec early. It is the contract between frontend and backend.

---

### 5.2 JSON-RPC 2.0

**What it is:** A lightweight remote procedure call protocol encoded in JSON. Stateless, transport-agnostic.

**Relevance for CodeForge:** JSON-RPC is the transport for both MCP and A2A. CodeForge does not need to implement JSON-RPC independently — the MCP and A2A SDKs handle it. However, understanding JSON-RPC is important for debugging MCP connections and implementing custom MCP servers.

**Priority:** **Already handled** — by MCP/A2A SDK usage. No independent implementation needed.

---

### 5.3 GraphQL

**What it is:** A query language for APIs that lets clients request exactly the data they need.

**Architecture decision (already made):** GraphQL was explicitly rejected in `docs/architecture.md` ("Explicitly NOT adopted: HAL+JSON/HATEOAS, GraphQL, custom PM tool"). The GitHub and GitLab PM adapters use their GraphQL APIs as consumers but CodeForge does not expose GraphQL.

**Confirmation:** This decision remains correct. GraphQL adds complexity (schema management, resolver layer, N+1 query problems) that is not justified for CodeForge's use case. REST + WebSocket covers all needs. The only GraphQL interaction is as a client to GitHub/GitLab APIs, handled by their respective adapter packages.

**Priority:** **Skip** — Decision already made and still valid.

---

## 6. Prioritized Recommendation Matrix

### Phase 1 — Foundation (Essential)

| Protocol | Role | Implementation | Effort |
|---|---|---|---|
| **MCP** (Client/Host) | Agent tool access | Python SDK in workers, MCP client connections to external servers | Medium |
| **AGENTS.md** | Repo context | Parse during repo analysis, inject into agent prompts | Trivial |
| **OpenAPI 3.1** | API documentation | Go Core REST API spec | Low |
| **OAuth 2.0** | GitHub/GitLab auth | `golang.org/x/oauth2` for SCM access | Low-Medium |
| **Tree-sitter** | Repo map | Python bindings in Context Layer | Low |
| **WebSocket** | Real-time frontend | Already planned (Event Bus pattern) | Already planned |
| **NATS JetStream** | Internal messaging | Already planned | Already planned |

### Phase 2 — MVP Features (Important)

| Protocol | Role | Implementation | Effort |
|---|---|---|---|
| **LSP** (via MCP bridge) | Semantic code analysis | LSP MCP servers in sandbox containers | Medium-High |
| **OpenTelemetry** | Request tracing | Go + Python OTel SDKs, trace context in NATS | Medium |
| **Prometheus** | Operational metrics | LiteLLM metrics + CodeForge-specific metrics | Low |
| **MCP** (Server) | IDE integration | Expose CodeForge tasks as MCP tools | Medium |
| **OIDC** | User authentication | `coreos/go-oidc` for federated login | Medium |
| **SARIF** (consumer) | Static analysis results | Parse SARIF from external tools | Low |

### Phase 3 — Advanced Features (Nice-to-Have)

| Protocol | Role | Implementation | Effort |
|---|---|---|---|
| **A2A** | External agent interop | Agent Card + task lifecycle | Medium |
| **DAP** (via MCP bridge) | Agent debugging | DAP MCP servers in sandbox | Low-Medium |
| **SCIM** | Enterprise user provisioning | Only for multi-tenancy | Medium |
| **SARIF** (producer) | CI/CD integration | Emit analysis results as SARIF | Low |

### Skip / Monitor

| Protocol | Reason |
|---|---|
| **ACP** (IBM) | Merged into A2A. Dead protocol. |
| **AG-UI** | React-focused. Custom WebSocket event system covers the need. Monitor for SolidJS adapter. |
| **ANP** | Internet-scale agent federation. Far beyond current scope. |
| **GraphQL** (as server) | Already rejected. Decision still correct. |

---

## 7. Architecture Integration Map

How the recommended protocols map to CodeForge's three-layer architecture:

```
┌────────────────────────────────────────────────────────────────┐
│                   TypeScript Frontend (SolidJS)                  │
│                                                                │
│  WebSocket (Event Bus)    REST (OpenAPI 3.1)    OIDC Login     │
└────────────────────┬──────────────┬──────────────┬─────────────┘
                     │              │              │
┌────────────────────▼──────────────▼──────────────▼─────────────┐
│                      Go Core Service                             │
│                                                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            │
│  │ OAuth 2.0   │  │ OpenAPI 3.1 │  │ OTel Traces │            │
│  │ (SCM Auth)  │  │ (API Spec)  │  │ (Go SDK)    │            │
│  └─────────────┘  └─────────────┘  └─────────────┘            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            │
│  │ OIDC        │  │ Prometheus  │  │ MCP Server  │            │
│  │ (User Auth) │  │ (Metrics)   │  │ (IDE integ.)│            │
│  └─────────────┘  └─────────────┘  └─────────────┘            │
│  ┌─────────────┐                                               │
│  │ A2A Agent   │  (Phase 3)                                    │
│  │ Card        │                                               │
│  └─────────────┘                                               │
└───────────┬─────────────────────────────────┬──────────────────┘
            │  NATS JetStream                 │
            │  (OTel trace context in headers)│
┌───────────▼─────────────────────────────────▼──────────────────┐
│                     Python Workers                               │
│                                                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            │
│  │ MCP Client  │  │ OTel Traces │  │ AGENTS.md   │            │
│  │ (Tool       │  │ (GenAI      │  │ Parser      │            │
│  │  access)    │  │  semconv)   │  │             │            │
│  └─────────────┘  └─────────────┘  └─────────────┘            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            │
│  │ Tree-sitter │  │ LSP (via    │  │ SARIF       │            │
│  │ (Repo Map)  │  │ MCP bridge) │  │ (Consumer)  │            │
│  └─────────────┘  └─────────────┘  └─────────────┘            │
│  ┌─────────────┐                                               │
│  │ DAP (via    │  (Phase 3)                                    │
│  │ MCP bridge) │                                               │
│  └─────────────┘                                               │
└────────────────────────────────────────────────────────────────┘
```

---

## 8. Key Insight: MCP as the Universal Integration Layer

The single most important architectural insight from this research is that **MCP serves as CodeForge's universal integration layer**. Rather than building custom integrations for LSP, DAP, databases, cloud services, and external tools, CodeForge can leverage the MCP ecosystem:

- **LSP integration** → via existing LSP-MCP bridge servers
- **DAP debugging** → via existing DAP-MCP bridge servers
- **Database access** → via MCP servers for PostgreSQL, MongoDB, etc.
- **Cloud services** → via MCP servers for AWS, GCP, Azure
- **File operations** → via MCP servers (or native tools)
- **Web search** → via MCP servers for search engines

This means CodeForge's Python workers need a robust MCP client implementation as their primary extension mechanism. The `McpWorkbench` pattern (already planned) is the right abstraction — it wraps any MCP server as a tool container that can be dynamically loaded based on task context.

The MCP ecosystem effectively solves the M*N integration problem (M tools * N agents) by standardizing both sides. CodeForge agents speak MCP; tools speak MCP; the connection is automatic.

---

## 9. The AAIF Protocol Stack

The three AAIF-governed protocols form a complete layered standard for AI coding agents:

```
Layer 3: AGENTS.md  — What the agent should know about THIS codebase
Layer 2: A2A        — How agents from different systems collaborate
Layer 1: MCP        — How an agent accesses tools, data, and services
```

CodeForge should implement all three layers, with MCP and AGENTS.md in Phase 1, and A2A in Phase 3. This positions CodeForge as a first-class citizen in the emerging agent standards ecosystem.

---

## 10. Risk Assessment

| Risk | Mitigation |
|---|---|
| MCP spec evolving (last update Nov 2025) | Pin to a spec version per release; AAIF governance reduces breaking changes |
| OTel GenAI conventions not yet stable | Use OpenLLMetry for now; migration path to stable conventions is documented |
| A2A still early (v0.3) | Defer to Phase 3; spec will mature by then |
| LSP server management complexity | Use MCP bridge servers to avoid managing LSP directly |
| AG-UI has no SolidJS support | Custom WebSocket event system covers the need |
| Protocol proliferation | Focus on AAIF-governed protocols (MCP, A2A, AGENTS.md) as the safe bets |
