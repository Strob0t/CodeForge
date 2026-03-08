# CodeForge — TODO Tracker

> LLM Agents: This is your **primary** task reference.
> Always read this file before starting work to understand current priorities.

### How to Use This File

- Before starting work: Read this file to understand what needs to be done
- After completing a task: Mark it `[x]`, add completion date, move to "Recently Completed" if needed
- When discovering new work: Add items to the appropriate section with context
- Format: `- [ ]` for open/pending, `- [x]` for done (with date)
- Cross-reference: Link to feature docs, architecture.md sections, or issues where relevant

---

### Completed Phases (0 through 30+)

> All phases below are complete. For implementation details, see git history.
> For phase summaries, see [project-status.md](project-status.md).

#### Phase 0 -- Project Setup (COMPLETED)
- [x] Market research (20+ tools), architecture decisions, devcontainer, linting, documentation structure

#### Phase 1 -- Foundation (COMPLETED)
- [x] Docker Compose (PostgreSQL, NATS, LiteLLM), Go Core REST API (9 endpoints), Python Workers, SolidJS frontend, CI

#### Phase 2 -- MVP Features (COMPLETED)
- [x] Git local provider (clone, status, pull, branches), agent lifecycle with Aider backend, WebSocket live output, LLM provider management

#### Phase 3 -- Reliability & Performance (COMPLETED)
- [x] Hierarchical config, structured JSON logging, circuit breaker, graceful shutdown, idempotency keys, dead letter queue
- [x] Event sourcing, tiered cache (ristretto L1 + NATS KV L2), rate limiting, DB pool tuning, worker pools

#### Phase 4 -- Agent Execution Engine (COMPLETED)
- [x] Policy layer (first-match-wins, 4 presets, YAML custom policies), runtime step-by-step protocol
- [x] Docker sandbox execution, stall detection, quality gates, 5 delivery modes, shadow Git checkpoints
- [x] Resource limits, secrets vault with SIGHUP reload, multi-tenancy preparation

#### Phase 5 -- Multi-Agent Orchestration (COMPLETED)
- [x] DAG scheduling (sequential, parallel, ping-pong, consensus), Meta-Agent LLM decomposition
- [x] Agent Teams with role-based composition, Context Optimizer with token budget packing
- [x] Modes System: 21 built-in agent specialization modes

#### Phase 6 -- Code-RAG (COMPLETED)
- [x] tree-sitter Repo Map (16+ languages, PageRank), Hybrid Retrieval (BM25S + semantic, RRF fusion)
- [x] Retrieval Sub-Agent with LLM query expansion, GraphRAG with PostgreSQL adjacency-list graph

#### Phase 7 -- Cost & Token Transparency (COMPLETED)
- [x] Real cost extraction from LiteLLM, fallback pricing table, cost aggregation API (5 endpoints)
- [x] WebSocket budget alerts, frontend cost dashboard with project breakdown and daily bars

#### Phase 8 -- Roadmap Foundation, Trajectory, Docker Production (COMPLETED)
- [x] Roadmap/Feature-Map domain model, spec/PM provider ports, 12 REST endpoints
- [x] Trajectory API with cursor pagination, Docker production images, docker-compose.prod.yml

#### Phase 9A-9E -- Advanced Integrations (COMPLETED)
- [x] 9A: OpenSpec, Markdown, GitHub Issues adapters, spec/PM import
- [x] 9B: SVN provider, Gitea/Forgejo PM adapter, VCS webhooks (GitHub + GitLab), bidirectional PM sync
- [x] 9C: PM webhook processing, Slack + Discord notification adapters
- [x] 9D: OpenTelemetry stub, A2A protocol stub, AG-UI event protocol, blue-green deployment
- [x] 9E: Plane.so PM adapter (full CRUD), full auto-detection engine, Feature-Map visual editor

#### Phase 10 -- Frontend Foundations (COMPLETED)
- [x] JWT auth (HS256, access + refresh), RBAC middleware, API key management
- [x] Signal-based i18n (480+ keys, EN + DE), CSS design tokens, command palette, toast system
- [x] WCAG 2.2 AA conformance, error boundaries, offline detection

#### Phase 11 -- GUI Enhancements (COMPLETED)
- [x] Tab-based ProjectDetailPage, settings page, mode selection UI, step-progress indicators
- [x] Team management, trajectory replay inspector, diff-review, architecture graph visualization

#### Post-Phase 11 -- Security Hardening (COMPLETED)
- [x] 18 audit findings fixed (5 P0, 8 P1, 5 P2): prompt injection defense, secret redaction, audit trail
- [x] Fail-closed quality gates, JWT standard claims + revocation, API key scopes, account lockout

#### Phase 12A-12K -- Architecture Evolution (COMPLETED)
- [x] 12A: Mode extensions (DeniedTools, DeniedActions, RequiredArtifact, modular prompt templates)
- [x] 12B: LLM routing via LiteLLM tag-based scenario routing (6 scenarios)
- [x] 12C: Role evaluation framework (FakeLLM harness, 9-role matrix, 15 fixtures)
- [x] 12D-12F: RAG shared scopes, artifact-gated pipelines, pipeline templates (3 built-in)
- [x] 12G-12K: Workspace management, per-tool token tracking, periodic reviews, project wizard, knowledge bases

#### OWASP Audit Remediation (COMPLETED)
- [x] Two rounds of OWASP Top 10:2025 + WSTG v4.2 (50+ findings across P0-P3)
- [x] Docker hardening, tenant isolation, request body limits, path traversal prevention, CSP headers

#### Phase 13 -- UI/UX Improvements & Chat Interface (COMPLETED)
- [x] Foundation fixes, CRUD completeness (projects, modes editable), settings + account management
- [x] Spec/roadmap detection fix, chat interface with conversation API and AG-UI integration
- [x] Automatic orchestration, Goose/OpenCode/Plandex/OpenHands agent backends

#### Phase 14 -- UX Simplification (COMPLETED)
- [x] Side-by-side project layout, simplified project creation with branch selection
- [x] Roadmap structured parsing with drag-to-reorder, bidirectional sync (UI -> repo files)
- [x] Chat enhancements (streaming, Markdown rendering, tool call cards)

#### Phase 15 -- Protocol Integrations (MCP + LSP) (COMPLETED)
- [x] MCP client in Python Workers (McpWorkbench with BM25 tool recommendation)
- [x] MCP server in Go Core (mcp-go SDK, 4 tools, 2 resources), server registry with DB persistence
- [x] LSP code intelligence with per-language server lifecycle, tool routing with policy integration

#### Phase 16 -- Frontend Design System Rework (COMPLETED)
- [x] 25 CSS design tokens, 11 primitives, 8 composites, 4 layout components, full page migration (42 files)

#### Phase 17 -- Interactive Agent Loop (COMPLETED)
- [x] LLM tool-calling support, 7 built-in tools (Read, Write, Edit, Bash, Search, Glob, ListDir)
- [x] AgentLoopExecutor with multi-turn tool-use, ConversationHistoryManager with token budget
- [x] HITL approval via WebSocket, AG-UI streaming events, ChatPanel with tool call display

#### Phase 18 -- Live E2E Testing & Blockers (COMPLETED)
- [x] NATS stream subjects bug fix, system prompt self-correction, model auto-discovery
- [x] Runtime conversation policy fix, live testing with real LLM calls, knowledge base system fixes

#### Phase 19 -- Frontend UX Refinements (COMPLETED)
- [x] Resizable roadmap/chat split, collapsible roadmap panel, chat auto-scroll
- [x] Expanded mode prompts with composable prompt system and editor, MCP Streamable HTTP transport

#### Phase 20 -- Benchmark Mode (COMPLETED)
- [x] DeepEval integration (correctness, faithfulness, relevancy, tool correctness metrics)
- [x] OpenTelemetry tracing, GEMMAS collaboration metrics (IDS, UPR)
- [x] Go Core benchmark API (7 endpoints, migration 041), frontend benchmark dashboard

#### Phase 21 -- Intelligent Agent Orchestration (COMPLETED)
- [x] Confidence-based moderator router with structured output, typed agent module schemas
- [x] SVG-based agent flow DAG visualization, moderator agent mode with debate protocol

#### Phase 22 -- Planned Pattern Implementation (COMPLETED)
- [x] All 8 patterns from CLAUDE.md: RouterLLM wiring, Copilot token exchange, composite memory scoring
- [x] Experience pool (@exp_cache), HandoffMessage, Microagents, Skills system, Human Feedback Protocol

#### Phase 23 -- Security & Identity Patterns (COMPLETED)
- [x] 23A: Trust annotations (4 levels), auto-stamped on NATS payloads
- [x] 23B: Message quarantine with risk scoring, admin review hold
- [x] 23C: Persistent agent identity (fingerprint, stats accumulation, inbox)
- [x] 23D: War Room -- live multi-agent collaboration view with swim lanes

#### Phase 24 -- Active Work Visibility (COMPLETED)
- [x] Parallel task deduplication, atomic claim/release with optimistic locking, stale recovery

#### Phase 25 -- Frontend Form Dropdowns (COMPLETED)
- [x] Dynamic dropdown population for agent, policy, and mode selectors, TagInput component

#### Phase 26 -- Benchmark System Redesign (COMPLETED)
- [x] Provider interface pattern, evaluator plugins (LLMJudge, FunctionalTest, SPARC), 3 runner types
- [x] 8 external providers (HumanEval, MBPP, SWE-bench, etc.), multi-compare with radar chart
- [x] NATS bridge, WebSocket live updates, suites CRUD, 132 E2E tests

#### Phase 27 -- A2A Protocol Integration (COMPLETED)
- [x] Full A2A v0.3.0 via a2a-go SDK -- server (inbound tasks) and client (outbound federation)
- [x] AgentCard builder, auth middleware, task lifecycle, remote agent registry, `a2a://` handoff routing

#### Phase 28 -- R2E-Gym / EntroPO Integration (COMPLETED)
- [x] Hybrid verification pipeline (filter->rank), trajectory verifier (5-dimension LLM scoring)
- [x] Multi-rollout test-time scaling (best-of-N), diversity-aware MAB routing (entropy-UCB1)
- [x] DPO/EntroPO trajectory export (JSONL), SWE-GEN synthetic task generation from Git history

#### Phase 29 -- Hybrid Intelligent Model Routing (COMPLETED)
- [x] Three-layer cascade: ComplexityAnalyzer (<1ms) -> MABModelSelector (UCB1) -> LLMMetaRouter
- [x] Task-type complexity boost, model auto-discovery from LiteLLM, wildcard config
- [x] Adaptive retry with exponential backoff, per-provider rate-limit tracking

#### Phase 30 -- Goal Discovery & Adaptive Retry (COMPLETED)
- [x] Auto-detection of project goals from workspace files, priority-based context injection
- [x] LLMClientConfig with env-var-driven retry/timeout, HybridRouter skips exhausted providers

#### Unified LLM Path & Global Run Tracking (COMPLETED)
- [x] Simple chat unified with agentic path through NATS dispatch
- [x] ConversationRunProvider for global run state, sidebar indicator, ChatPanel seamless resume

#### OTEL Tracing Rewrite (COMPLETED)
- [x] AgentNeo replaced with OpenTelemetry backend (OTLP gRPC exporter), 6 instrumented services

#### QA Audit (COMPLETED)
- [x] ~90 new handler tests across P0-P3 tiers, 33 duplicate test names renamed
- [x] P0: Auth (23 tests) + Orchestration (18 tests), P1: Auto-Agent, Files, Roadmap, Agent Features
- [x] P2: Conversation, Cost, Settings, Session, MCP, KB, LLM, P3: Service-layer gaps

#### Comprehensive Code Review (COMPLETED)
- [x] 46 issues found across 10 areas (18 critical, 24 important, 4 medium) -- all fixed
- [x] NATS contract fixes, backend executor implementations, runtime state leak fixes
- [x] Security hardening, benchmark fixes, memory/multi-tenancy, PM sync, orchestration fixes

#### Documentation-Code Reconciliation (COMPLETED)
- [x] Python trust/quarantine layer, A2A protocol expansion, handoff enrichment

#### Benchmark Cross-Layer Bug Fixes (COMPLETED)
- [x] 7 bugs fixed: DB migration for rollout fields, cost population, NATS wiring, CSV export

#### Test Suites (COMPLETED)
- [x] Browser E2E: 17 Playwright tests (health, navigation, projects, costs, models, a11y)
- [x] LLM E2E: 95 API-level tests across 12 spec files
- [x] Benchmark E2E: 132 browser Playwright tests across 12 spec files
- [x] Backend E2E: 88 pass / 0 fail / 3 skip (97% pass rate)

---

### Feature Roadmap -- Consolidated Open Items

> Extracted from `docs/features/*.md` and centralized here per documentation policy.
> Feature docs now reference this file instead of maintaining their own TODO lists.

#### Pillar 1: Project Dashboard

- [ ] Implement GitHub adapter with OAuth flow -- only Copilot token exchange exists (`internal/adapter/copilot/`), no full GitHub OAuth integration
- [ ] Verify GitHub adapter compatibility with Forgejo/Codeberg -- base URL override, API differences untested
- [ ] Batch operations across selected repos -- UI and service layer
- [ ] Cross-repo search (code, issues) -- requires indexing infrastructure

#### Pillar 4: Agent Orchestration

- [ ] Enhance CLI wrappers for Goose, OpenHands, OpenCode, Plandex -- basic wrappers exist in `workers/codeforge/backends/`, needs advanced features (streaming, interactive mode, config passthrough)
- [ ] Trajectory replay UI and audit trail -- event store + service exist (`internal/port/eventstore/`, `internal/service/agent.go`), frontend UI missing
- [ ] Session events as source of truth (Resume/Fork/Rewind) -- domain model + service exist (`internal/service/session.go`), full integration TBD
