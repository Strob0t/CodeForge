# Changelog

All notable changes to CodeForge are documented in this file.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

## [Unreleased]

### Added
- Self-service GDPR endpoints: `/me/export` (GET) and `/me/data` (DELETE) for user data portability and erasure without admin role (Art. 15, 17, 20)
- Entropy-based JWT secret validation in production (rejects secrets with < 3.0 bits/char Shannon entropy)
- Auto-generation of cryptographically random JWT secret on first boot
- Docker secrets file support via `_FILE` env var suffix pattern
- `queryParamIntClamped` helper for bounded API parameter validation
- WebSocket connection limiting in nginx (limit_conn 50)
- Canvas keyboard navigation: Tab cycling, Delete/Backspace removal, Escape deselect (WCAG 2.1.1)
- `extractErrorMessage()` frontend utility replacing 52 inline error extractions

### Changed
- RBAC middleware added to ~30 previously unprotected write endpoints
- Audit logging added to password change, API key CRUD, branch protection CRUD
- Grype severity cutoff lowered from `critical` to `high`
- CI tool versions pinned: poetry==2.1.1, govulncheck@v1.1.4, pip-audit==2.9.0
- Worker HEALTHCHECK start-period increased from 15s to 30s
- nginx client_max_body_size reduced from 100M to 20M
- WebSocket proxy_read_timeout reduced from 86400 to 300
- PII (email) removed from INFO-level production logs in auth service
- Retention service now iterates all tenants for cleanup
- Agent loop methods annotated with typed IterationOutcome union

### Fixed
- Path traversal bypass in Python tool resolution (startswith -> is_relative_to)
- SSRF in A2A webhook, project info, VCS account HTTP clients (added SafeTransport)
- Channel webhook key validated with constant-time comparison (was presence-only check)
- Workspace adoption restricted to workspace root (was unrestricted absolute paths)
- DetectStackByPath restricted to workspace root
- Hardcoded JWT secret in active config bypassing loader blocklist
- 9 unbounded SQL queries now have LIMIT clauses
- Silenced JSON unmarshal errors in store layer replaced with slog.Warn
- Duplicate buildSystemPrompt (~125 LOC) removed
- Dead code (writeValidationError) removed
- Custom min64 replaced with Go 1.21+ built-in min

### Security
- F-ARC-011/012: bypass-approvals and branch protection CRUD now require admin role
- F-SEC-001: Python tool path traversal fix (CWE-22)
- F-SEC-002: Webhook HMAC validation (CWE-287)
- F-SEC-005/007/012: SSRF protection via SafeTransport (CWE-918)
- F-INF-001: Hardcoded secrets removed from config, entropy validation added
- F-COM-003: PII removed from production logs (GDPR Art. 5(1)(c))

## [0.8.0] - 2026-03-23

### Added
- Visual Design Canvas with SVG tools and multimodal pipeline (Phase 32)
- Contract-First Review/Refactor pipeline (Phase 31)
- Benchmark and Evaluation system (Phase 26+28)
- Security and Trust annotations (Phase 23)
- Real-Time Channels for team collaboration
- Chat enhancements: HITL permission UI, inline diff review, action buttons
- Hybrid routing with ComplexityAnalyzer and MAB model selection
- Agentic conversation loop with 7 built-in tools
- A2A protocol support (v0.3.0)
- MCP server with project/cost resources

### Security
- HSTS header on all responses
- Docker images pinned to specific versions
- cap_drop ALL on production containers
- Pre-deploy environment validation script
- Trust annotations with 4 levels on NATS messages

### Fixed
- Python exception handlers now log error details
- Bare Go error returns wrapped with context
- WebSocket endpoint rate-limited
- PostgreSQL archive command error handling

## [0.7.0] - 2026-03-10

### Added
- Hybrid intelligent model routing with three-layer cascade: ComplexityAnalyzer, MAB (UCB1), LLMMetaRouter (Phase 29)
- Model auto-discovery from LiteLLM with 60s cache
- LiteLLM config simplified to 13 provider-level wildcard entries
- Adaptive retry with exponential backoff and per-provider rate-limit tracking
- Goal discovery: auto-detection of project goals from workspace files (Phase 30)
- Priority-based goal injection into agent system prompts
- Frontend GoalsPanel for goal management
- Unified LLM path: simple chat dispatched through NATS like agentic conversations
- ConversationRunProvider for global run state tracking across page navigation
- Sidebar run indicator with ChatPanel seamless resume
- OTEL tracing rewrite: AgentNeo replaced with OpenTelemetry (OTLP gRPC exporter)

### Changed
- LiteLLM provider wildcards (`openai/*`, `anthropic/*`) replace per-model entries
- Scenario tags (default/background/think/longContext/review/plan) used as fallback when routing disabled
- HybridRouter skips exhausted providers automatically

### Fixed
- 6 instrumented Go service methods with OTEL spans
- 3 conversation-level spans added for end-to-end tracing
- All metrics nil-guarded to prevent panics

## [0.6.0] - 2026-02-28

### Added
- A2A v0.3.0 protocol integration via a2a-go SDK (Phase 27)
- CodeForge as both A2A server (inbound tasks) and client (outbound federation)
- AgentCard builder, auth middleware, task lifecycle management
- Remote agent registry with `a2a://` handoff routing prefix
- R2E-Gym / EntroPO integration (Phase 28): hybrid verification pipeline, trajectory verifier
- Multi-rollout test-time scaling (best-of-N)
- Diversity-aware MAB routing with entropy-enhanced UCB1
- DPO/EntroPO trajectory export (JSONL chosen/rejected pairs)
- SWE-GEN synthetic task generation from Git history
- Benchmark system redesign: provider interface, evaluator plugins (Phase 26)
- 3 runner types (simple/tool-use/agent) for benchmarks
- 8 external benchmark providers (HumanEval, MBPP, BigCodeBench, CRUXEval, LiveCodeBench, SWE-bench, SPARCBench, Aider Polyglot)
- Multi-compare radar chart, leaderboard, cost analysis for benchmarks
- 132 benchmark E2E tests

### Fixed
- Cross-layer bug fixes for DB fields, NATS wiring, and cost population in benchmarks

## [0.5.0] - 2026-02-18

### Added
- Trust annotations with 4 levels (untrusted/partial/verified/full) auto-stamped on NATS payloads (Phase 23A)
- Message quarantine with risk scoring and admin review hold (Phase 23B)
- Persistent agent identity: fingerprint, stats accumulation, inbox (Phase 23C)
- War Room: live multi-agent collaboration view with swim lanes and handoff arrows (Phase 23D)
- Parallel task deduplication for agent claim/release lifecycle (Phase 24)
- Dynamic dropdown population for agent, policy, and mode selectors (Phase 25)
- Benchmark Mode with DeepEval (correctness, faithfulness, relevancy, tool correctness) (Phase 20)
- OpenTelemetry tracing (TracerProvider + MeterProvider) replacing AgentNeo
- GEMMAS collaboration metrics (IDS, UPR) for multi-agent evaluation
- Go Core benchmark API (7 endpoints) with frontend benchmark dashboard

### Security
- Trust annotations prevent untrusted messages from triggering privileged operations
- Quarantine system holds suspicious messages for admin review before processing
- Agent identity fingerprinting enables accountability and audit trails

## [0.4.0] - 2026-02-08

### Added
- Agentic conversation loop: multi-turn LLM tool-calling with 7 built-in tools (Phase 17)
- Tools: Read, Write, Edit, Bash, Search, Glob, ListDir with MCP tool merge
- AgentLoopExecutor (Python) with streaming LLM, per-tool policy, cost tracking
- ConversationHistoryManager with head-and-tail token budget
- HITL approval via WebSocket: permission requests, approve/deny, allow-always
- AG-UI streaming events (8 event types) across Go and TypeScript
- ChatPanel with tool call display and inline diff review
- HITL Permission UI with countdown and preset mapping
- Per-message cost display via AG-UI state_delta
- Smart references (@/#// autocomplete) and slash commands (/compact, /rewind, /clear)
- Conversation search with PostgreSQL FTS and GIN index
- Notification center with browser push, Web Audio, tab badge
- Real-Time Channels (3 tables, 9 endpoints, WS events)
- Confidence-based moderator router with structured output (Phase 21)
- SVG-based agent flow DAG visualization
- RouterLLM scenario wiring, GitHub Copilot token exchange (Phase 22)
- Composite memory scoring, experience pool, HandoffMessage pattern
- Microagents (YAML+Markdown triggers), Skills system (BM25 snippets)
- Human Feedback Provider Protocol (Slack + Email adapters)
- Live E2E testing with real LLM calls (Phase 18)
- Resizable roadmap/chat split, collapsible roadmap panel (Phase 19)
- MCP Streamable HTTP transport

### Changed
- System prompt self-correction for improved agent accuracy
- Model auto-discovery (LiteLLM + Ollama) for provider management
- Composable prompt system with editor for mode prompts

### Fixed
- NATS stream subjects bug causing message routing failures
- Runtime conversation policy evaluation
- Knowledge base system integration issues

## [0.3.0] - 2026-01-28

### Added
- JWT authentication (HS256, access + refresh tokens) with RBAC middleware (Phase 10)
- API key management with scoped permissions
- Signal-based i18n (480+ keys, EN + DE) for frontend
- CSS design tokens with dark/light mode support
- Command palette (Ctrl+K) and toast notification system
- WCAG 2.2 AA conformance with axe-core E2E audits
- Error boundaries and offline detection
- Tab-based ProjectDetailPage, settings page, mode selection UI (Phase 11)
- Trajectory replay inspector and diff-review code preview
- Agent network visualization and architecture graph
- MCP client in Python Workers with BM25 tool recommendation (Phase 15)
- MCP server in Go Core (mcp-go SDK, 4 tools, 2 resources)
- MCP server registry with PostgreSQL persistence and frontend UI
- LSP code intelligence with per-language server lifecycle (Phase 15D)
- 25 CSS design tokens, 11 primitive components, 8 composite components (Phase 16)
- Full page migration (42 files) to design system
- Mode system extensions: DeniedTools, DeniedActions, RequiredArtifact (Phase 12A-12K)
- LLM routing via LiteLLM tag-based scenario routing (6 scenarios)
- Role evaluation framework (FakeLLM harness, 9-role matrix)
- RAG shared scope system with cross-project retrieval
- Artifact-gated pipelines (6 artifact types with structural validators)
- Project workspace management (tenant-isolated paths, adopt, health endpoint)
- Knowledge bases (8 built-in catalog entries, scope integration)
- OpenSpec, Markdown, GitHub Issues adapters (Phase 9A-9E)
- SVN provider, Gitea/Forgejo PM adapter, VCS webhooks
- Bidirectional PM sync (GitHub/GitLab/Plane)
- Plane.so PM adapter (full CRUD) and Feature-Map visual editor

### Security
- 18 audit findings fixed (5 P0, 8 P1, 5 P2): prompt injection defense, secret redaction
- JWT standard claims + revocation, API key scopes, account lockout
- Password complexity enforcement, audit trail, fail-closed quality gates

### Changed
- CSS reduced 35% through design system consolidation
- Frontend migrated to design system primitives

## [0.2.0] - 2026-01-15

### Added
- Policy layer with first-match-wins evaluation and 4 built-in presets (Phase 4)
- YAML-configurable custom policies with runtime API
- Step-by-step execution protocol via NATS (Go<->Python)
- Docker sandbox execution with resource limits
- Stall detection (FNV-64a hash ring) and quality gate enforcement
- 5 delivery modes: none, patch, commit, branch, PR
- Shadow Git checkpoints for safe rollback
- Secrets vault with SIGHUP reload
- Multi-tenancy preparation (tenant_id on all tables)
- DAG scheduling with 4 protocols: sequential, parallel, ping-pong, consensus (Phase 5)
- Meta-Agent with LLM-based feature decomposition
- Agent Teams with role-based composition and pool management
- Context Optimizer with token budget packing and shared team context
- Modes System (24 built-in agent specialization modes)
- tree-sitter-based Repo Map (16+ languages, PageRank ranking) (Phase 6)
- Hybrid Retrieval (BM25S + semantic embeddings via LiteLLM, RRF fusion)
- Retrieval Sub-Agent with LLM query expansion and parallel search
- GraphRAG with PostgreSQL adjacency-list graph (BFS with hop-decay scoring)
- Real cost extraction from LiteLLM responses with fallback pricing (Phase 7)
- Cost aggregation API (5 endpoints) with WebSocket budget alerts
- Frontend cost dashboard with project breakdown and daily bars
- Roadmap/Feature-Map domain model with 12 REST endpoints (Phase 8)
- Spec provider and PM provider port interfaces
- Trajectory API with cursor pagination
- Docker production images (Go multi-stage, Python slim, nginx frontend)
- GitHub Actions Docker build CI

### Changed
- Agent lifecycle extended with execution engine and safety layer
- NATS communication evolved to step-by-step protocol

## [0.1.0] - 2026-01-01

### Added
- Three-layer hybrid architecture: Go Core + Python Workers + SolidJS Frontend (Phase 0)
- Market research covering 20+ AI coding tools
- Hexagonal Architecture (Ports & Adapters) for Go Core
- Provider Registry Pattern with self-registering `init()` functions
- Docker Compose infrastructure: PostgreSQL, NATS JetStream, LiteLLM (Phase 1)
- Go Core with domain entities, ports, registries, WebSocket hub
- NATS adapter for Go<->Python communication
- REST API (9 initial endpoints)
- Python Workers with NATS consumer and LiteLLM client
- SolidJS frontend with Dashboard page and project CRUD
- GitHub Actions CI pipeline
- Git local provider: clone, status, pull, branches, checkout (Phase 2)
- Agent lifecycle with Aider backend
- WebSocket live agent output streaming
- LLM provider management via LiteLLM admin API
- Frontend project detail page with git operations and agent monitor
- Hierarchical config: defaults < YAML < ENV < CLI flags (Phase 3)
- Structured JSON logging (Go slog + Python structlog)
- Async logging with buffered channels
- Circuit breaker, graceful 4-phase shutdown
- Idempotency keys, optimistic locking, dead letter queue
- Event sourcing for agent trajectory
- Tiered cache (ristretto L1 + NATS KV L2)
- Rate limiting and DB pool tuning

### Security
- Request body size limits
- WebSocket origin validation
- Tenant isolation in store queries
