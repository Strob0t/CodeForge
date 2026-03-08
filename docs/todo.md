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

#### Mobile-Responsive Frontend (COMPLETED)
- [x] (2026-03-08) useBreakpoint hook, CSS foundation (safe-area, touch targets, scrollbar-none), viewport-fit
- [x] (2026-03-08) Primitives/composites: Button touch targets (36-48px), NavLink 44px, Modal/Table/Card/PageLayout responsive
- [x] (2026-03-08) 3-state sidebar (hidden+overlay on mobile, collapsed on tablet, expanded on desktop), hamburger menu
- [x] (2026-03-08) Responsive grids: CostDashboard, CostAnalysis, MultiCompare, PromptEditor, WarRoom, CompactSettings
- [x] (2026-03-08) ProjectDetailPage: mobile tab-switch (Panels/Chat), scrollable sub-tabs, responsive header
- [x] (2026-03-08) ChatPanel: responsive bubbles (90%/75%), flex-wrap header, text size fixes
- [x] (2026-03-08) FilePanel: mobile file tree drawer overlay with backdrop
- [x] (2026-03-08) Fix pre-existing i18n errors (featuremap.dragToMove, statusToggled, dropHere)

#### Codebase Optimization -- Full Overhaul (COMPLETED)
- [x] (2026-03-08) Go: Deleted duplicate `internal/crypto/crypto/aes.go` (byte-for-byte copy)
- [x] (2026-03-08) Go: Generic `scanRows[T]`, `writeJSONList[T]`, `queryParamInt` helpers in `internal/adapter/postgres/helpers.go`
- [x] (2026-03-08) Go: Migrated 27 store files from manual `for rows.Next()` to `scanRows()` (~350 lines removed)
- [x] (2026-03-08) Go: Migrated ~14 handler files from manual `strconv.Atoi` to `queryParamInt()` and `writeJSONList()`
- [x] (2026-03-08) Go: Removed duplicate `nilIfEmpty()` in `store_benchmark.go`, consolidated to `nullIfEmpty()` in helpers
- [x] (2026-03-08) Go: Externalized hardcoded server timeouts and stale-work thresholds to `config.go` with yaml/env tags
- [x] (2026-03-08) Python: Shared `coerce_none_to_list` validator in `_validators.py`, replaced duplicate Pydantic validators
- [x] (2026-03-08) Python: `@catch_os_error` decorator in `tools/_error_handler.py`, applied to read/write/edit tools
- [x] (2026-03-08) Python: `_extract_cost()` static method in `llm.py`, replaced 3 duplicate header-parsing blocks
- [x] (2026-03-08) Python: `BaseBenchmarkRunner` ABC in `evaluation/runners/_base.py`, 3 runners refactored
- [x] (2026-03-08) Python: `RoutingConfig` dataclass with complexity weights/tier thresholds/task type boosts
- [x] (2026-03-08) Python: OpenHands timeouts externalized to env vars via `_env_float()` helper
- [x] (2026-03-08) Python: Consumer `_handle_request()` generic handler -- 10 NATS handlers migrated, 6 skipped (too complex)
- [x] (2026-03-08) Python: Consumer backoff constants externalized to env vars
- [x] (2026-03-08) Frontend: `cx()` class-name utility in `utils/cx.ts`, adopted across UI components
- [x] (2026-03-08) Frontend: `getErrorMessage()` in `utils/getErrorMessage.ts`, adopted in 6+ pages
- [x] (2026-03-08) Frontend: `StatCard`, `ResourceView`, `GridLayout` shared components
- [x] (2026-03-08) Frontend: `useFocusTrap` hook extracted from Modal.tsx
- [x] (2026-03-08) Frontend: `useFormState` hook -- BenchmarkPage (5 signals) and DashboardPage (8 signals) consolidated
- [x] (2026-03-08) Frontend: `useAsyncAction` hook -- adopted in AuditTable, FilePanel, PolicyPanel, 4+ more pages
- [x] (2026-03-08) Frontend: `CHART_COLORS` and `RADAR_DEFAULTS` design constants extracted

#### Pillar 1: Project Dashboard

- [ ] Implement GitHub adapter with OAuth flow -- only Copilot token exchange exists (`internal/adapter/copilot/`), no full GitHub OAuth integration
- [ ] Verify GitHub adapter compatibility with Forgejo/Codeberg -- base URL override, API differences untested
- [ ] Batch operations across selected repos -- UI and service layer
- [ ] Cross-repo search (code, issues) -- requires indexing infrastructure

#### Pillar 4: Agent Orchestration

- [ ] Enhance CLI wrappers for Goose, OpenHands, OpenCode, Plandex -- basic wrappers exist in `workers/codeforge/backends/`, needs advanced features (streaming, interactive mode, config passthrough)
- [ ] Trajectory replay UI and audit trail -- event store + service exist (`internal/port/eventstore/`, `internal/service/agent.go`), frontend UI missing
- [ ] Session events as source of truth (Resume/Fork/Rewind) -- domain model + service exist (`internal/service/session.go`), full integration TBD

---

### Integration Testing Strategy (A + B + C)

> Design: [docs/plans/2026-03-08-integration-testing-design.md](plans/2026-03-08-integration-testing-design.md)
> Goal: Verify that all 30 major features work together across Go, Python, and Frontend layers.
> Tracking: [docs/feature-verification-matrix.md](feature-verification-matrix.md)

#### B1: Fix Broken Foundation Tests (Priority: CRITICAL)

> 10 Go Postgres Store tests are FAILING. These test Project CRUD, User CRUD, and Tenant Isolation --
> foundation operations that everything else depends on. Nothing above can be trusted while these fail.

- [ ] Fix `TestStore_ProjectCRUD` -- `internal/adapter/postgres/store_test.go`
- [ ] Fix `TestStore_UserCRUD` -- `internal/adapter/postgres/store_test.go`
- [ ] Fix `TestStore_TokenRevocation` -- `internal/adapter/postgres/store_test.go`
- [ ] Fix `TestStore_Conversation_TenantIsolation` -- `internal/adapter/postgres/store_test.go`
- [ ] Fix `TestStore_GetProjectByRepoName_TenantIsolation` -- `internal/adapter/postgres/store_test.go`
- [ ] Fix `TestStore_A2ATask_TenantIsolation` -- `internal/adapter/postgres/store_a2a_test.go`
- [ ] Fix `TestStore_ListA2ATasks_TenantIsolation` -- `internal/adapter/postgres/store_a2a_test.go`
- [ ] Fix `TestStore_ListA2ATasks_LimitParameterized` -- `internal/adapter/postgres/store_a2a_test.go`
- [ ] Fix `TestStore_RemoteAgent_TenantIsolation` -- `internal/adapter/postgres/store_a2a_test.go`
- [ ] Fix `TestStore_ListRemoteAgents_TenantIsolation` -- `internal/adapter/postgres/store_a2a_test.go`
- [ ] Verify `go test ./internal/...` passes 100% green after all fixes

#### B2: NATS Payload Contract Tests (Priority: HIGH)

> Prove that Go JSON serialization and Python Pydantic deserialization match for every NATS message type.
> No running NATS needed -- pure serialization tests. Catches the #1 cross-layer bug class: field mismatches.
>
> Go side: `internal/port/messagequeue/contract_test.go`
> Python side: `workers/tests/test_nats_contracts.py`
> Fixtures: `internal/port/messagequeue/testdata/contracts/*.json` (Go-generated)

**Infrastructure:**

- [ ] Create Go contract test generator -- marshals each payload struct to JSON, writes to `testdata/contracts/{subject_name}.json`
- [ ] Create Python contract validator -- loads each JSON fixture, validates against corresponding Pydantic model
- [ ] Create reverse contract test -- Python generates JSON from Pydantic defaults, Go test unmarshals and validates
- [ ] Add contract test verification checklist: required fields, type compat, enum values, nested structs, nil/null handling, tenant_id presence

**Conversation Payloads (2 subjects):**

- [ ] Contract test: `conversation.run.start` -- Go `ConversationRunStartPayload` vs Python `ConversationRunStartMessage`
  - Verify `agentic` field present (was missing before 2026-03-08 fix)
  - Verify `mcp_servers` nested array serialization
  - Verify `microagent_prompts` string array
  - Verify `context` entries with `source`, `content`, `token_count`
- [ ] Contract test: `conversation.run.complete` -- Go `ConversationRunCompletePayload` vs Python `ConversationRunCompleteMessage`
  - Verify `tool_messages` nested array with `call_id`, `name`, `result`
  - Verify `cost_usd` float precision
  - Verify `tokens_in`, `tokens_out` int types

**Benchmark Payloads (2 subjects):**

- [ ] Contract test: `benchmark.run.request` -- Go `BenchmarkRunRequestPayload` vs Python `BenchmarkRunRequestMessage`
  - Verify `dataset_path` is absolute path (Go resolves before publish)
  - Verify `benchmark_type` enum values match ("simple", "tool_use", "agent")
  - Verify `evaluators` string array
  - Verify `rollout_count`, `hybrid_verification` fields
- [ ] Contract test: `benchmark.run.result` -- Go `BenchmarkRunResultPayload` vs Python `BenchmarkRunResultMessage`
  - Verify `results` array with nested `scores` map[string]float64
  - Verify `summary` struct fields
  - Verify `rollout_id`, `is_best_rollout`, `diversity_score` (Phase 28 fields)

**Evaluation Payloads (2 subjects):**

- [ ] Contract test: `evaluation.gemmas.request` -- Go `GemmasEvalRequest` vs Python `GemmasEvalRequestMessage`
- [ ] Contract test: `evaluation.gemmas.result` -- Go `GemmasEvalResultPayload` vs Python `GemmasEvalResultMessage`

**Memory Payloads (3 subjects):**

- [ ] Contract test: `memory.store` -- Go `MemoryStoreMessage` vs Python
  - Verify vector dimension handling, timestamp format (RFC3339)
- [ ] Contract test: `memory.recall` -- Go `MemoryRecallMessage` vs Python
- [ ] Contract test: `memory.recall.result` -- Go `MemoryRecallResultPayload` vs Python

**RepoMap Payloads (2 subjects):**

- [ ] Contract test: `repomap.generate.request` -- Go `RepoMapRequestPayload` vs Python
- [ ] Contract test: `repomap.generate.result` -- Go `RepoMapResultPayload` vs Python

**Retrieval Payloads (6 subjects):**

- [ ] Contract test: `retrieval.index.request` -- Go `RetrievalIndexRequestPayload` vs Python
- [ ] Contract test: `retrieval.index.result` -- Go `RetrievalIndexResultPayload` vs Python
- [ ] Contract test: `retrieval.search.request` -- Go `RetrievalSearchRequestPayload` vs Python
  - Verify `bm25_weight`, `semantic_weight` float fields
- [ ] Contract test: `retrieval.search.result` -- Go `RetrievalSearchResultPayload` vs Python
- [ ] Contract test: `retrieval.subagent.request` -- Go `SubAgentSearchRequestPayload` vs Python
- [ ] Contract test: `retrieval.subagent.result` -- Go `SubAgentSearchResultPayload` vs Python

**Graph Payloads (4 subjects):**

- [ ] Contract test: `graph.build.request` -- Go `GraphBuildRequestPayload` vs Python
- [ ] Contract test: `graph.build.result` -- Go `GraphBuildResultPayload` vs Python
- [ ] Contract test: `graph.search.request` -- Go `GraphSearchRequestPayload` vs Python
- [ ] Contract test: `graph.search.result` -- Go `GraphSearchResultPayload` vs Python

**A2A + Handoff Payloads (3 subjects):**

- [ ] Contract test: `a2a.task.created` -- Go `A2ATaskCreatedPayload` vs Python
- [ ] Contract test: `a2a.task.complete` -- Go `A2ATaskCompletePayload` vs Python
- [ ] Contract test: `handoff.request` -- Go `HandoffRequestPayload` vs Python

#### B3: Unit Tests for Untested Critical Modules (Priority: HIGH)

> These modules have 0 tests but sit on critical execution paths.
> Tests use mocks/fakes -- no Docker or external services needed.

**Python Agent Tools -- `workers/codeforge/tools/` (13 files, 0 tests):**

> These are the "hands" of the agent -- every agentic conversation uses them.
> Test file: `workers/tests/test_tools_unit.py` (or split per tool)

- [ ] Test `tool_read.py` -- file reading: valid path, non-existent path, binary file handling, encoding errors, line range (offset+limit), path traversal prevention, permission denied
- [ ] Test `tool_write.py` -- file creation: new file, overwrite existing, create parent dirs, empty content, path traversal blocked, permission denied, large file handling
- [ ] Test `tool_edit.py` -- diff editing: exact match replacement, unique match enforcement, no-match error, multi-line edits, indentation preservation, replace_all mode, empty old_string rejection
- [ ] Test `tool_bash.py` -- command execution: simple command, timeout enforcement, output truncation (long output), dangerous command detection (rm -rf, etc.), exit code handling, stderr capture, working directory persistence
- [ ] Test `tool_search.py` -- content search: regex patterns, file type filtering, result ranking, empty results, case sensitivity, binary file skip
- [ ] Test `tool_glob.py` -- pattern matching: `**/*.py`, `src/*.ts`, no matches, directory traversal depth, symlink handling
- [ ] Test `tool_listdir.py` -- directory listing: valid dir, non-existent dir, empty dir, hidden files, depth control, permission denied
- [ ] Test tool registry -- tool registration, lookup by name, duplicate prevention, MCP tool merge
- [ ] Test tool result formatting -- truncation, error formatting, cost annotation

**Python Consumer Dispatch -- `workers/codeforge/consumer/` (14 files, 0 tests):**

> This is the NATS message router -- receives messages and dispatches to the right handler.
> Test file: `workers/tests/test_consumer_dispatch.py`

- [ ] Test `_base.py` -- NATS connection setup, subscription registration, graceful shutdown, reconnect behavior
- [ ] Test `_conversation.py` -- dispatch to agentic vs simple chat based on `agentic` flag, model resolution, duplicate run detection (`_active_runs`), error handling with NATS publish back
- [ ] Test `_benchmark.py` -- benchmark type routing (simple/tool_use/agent), LiteLLM readiness wait, duplicate detection, dataset path validation
- [ ] Test `_retrieval.py` -- index vs search dispatch, result payload construction, error propagation
- [ ] Test `_graph.py` -- build vs search dispatch, result forwarding
- [ ] Test `_memory.py` -- store vs recall dispatch, vector dimension validation
- [ ] Test `_a2a.py` -- A2A task creation handling, completion publishing
- [ ] Test `_handoff.py` -- handoff request processing, agent routing
- [ ] Test `_repomap.py` -- repomap generation dispatch
- [ ] Test duplicate detection -- `_is_duplicate()` with `Nats-Msg-Id` headers, `_active_runs` set
- [ ] Test error handling -- exception capture, error result publish, `msg.ack()` always called
- [ ] Test subject registration -- all 14 subject wildcards registered with JetStream

**Python Memory System -- `workers/codeforge/memory/` (4 files, 0 tests):**

> Test file: `workers/tests/test_memory_system.py`

- [ ] Test `scorer.py` -- composite scoring: semantic similarity weight, recency decay, importance boost, edge cases (zero scores, all equal, very old memories)
- [ ] Test `experience.py` -- `@exp_cache` decorator: cache hit, cache miss, cache invalidation, key generation from inputs, TTL expiration
- [ ] Test vector storage interface -- store operation, recall with top_k, empty results, dimension mismatch error
- [ ] Test recall ranking -- score ordering, tie-breaking, filtering by type/project

**Go Adapters -- 0 tests, critical paths:**

> Test files: `internal/adapter/{name}/{name}_test.go`

- [ ] Test `adapter/a2a/` -- A2A HTTP handler: AgentCard serving, task creation, task status, SSE streaming, auth middleware, tenant isolation
- [ ] Test `adapter/lsp/` -- LSP lifecycle: server start per language, capability detection, shutdown, timeout handling
- [ ] Test `adapter/otel/` -- OTEL setup: TracerProvider creation, MeterProvider creation, shutdown, disabled mode (no-op)
- [ ] Test `adapter/email/` -- Email feedback: send, template rendering, connection error handling
- [ ] Test `adapter/natskv/` -- NATS KV: get, put, delete, TTL expiration, key not found
- [ ] Test `adapter/ristretto/` -- Cache adapter: get, set, delete, eviction, TTL

**Go Domain Models -- 0 tests, used across services:**

> Test files: `internal/domain/{name}/{name}_test.go`

- [ ] Test `domain/conversation/` -- Conversation entity: creation, validation, status transitions, message append
- [ ] Test `domain/orchestration/` -- Handoff model: request validation, agent routing, status tracking
- [ ] Test `domain/microagent/` -- Microagent matching: trigger keyword detection, prompt injection, priority ordering
- [ ] Test `domain/memory/` -- Memory entity: creation, scoring types, vector dimension validation
- [ ] Test `domain/skill/` -- Skill entity: creation, validation, content parsing

#### A1: Stack Health Smoke Tests (Priority: HIGH)

> Full stack must be running (Docker Compose + Go backend + Python worker).
> Test file: `tests/integration/smoke_test.go` (build tag: `//go:build smoke`)
> Run: `go test -tags=smoke -count=1 -timeout=300s ./tests/integration/...`

- [ ] Smoke test: Go backend `/health` returns 200 with expected fields (`status`, `dev_mode`, `version`)
- [ ] Smoke test: PostgreSQL pool active -- query `SELECT 1` succeeds
- [ ] Smoke test: NATS JetStream connected -- CODEFORGE stream exists
- [ ] Smoke test: All 14 NATS subject wildcards registered in stream config
- [ ] Smoke test: LiteLLM proxy `/health` returns 200
- [ ] Smoke test: WebSocket upgrade on `/ws` succeeds (with valid JWT)
- [ ] Smoke test: At least 1 LLM model available via `GET /api/v1/llm/available`
- [ ] Smoke test: All NATS subscribers active (Python worker consuming)

#### A2: Critical Flow Smoke Tests (Priority: HIGH)

> End-to-end flows through the real stack. Each flow creates test data, verifies, and cleans up.
> Test file: `tests/integration/flows_test.go` (build tag: `//go:build smoke`)

**Flow 1 -- Project Lifecycle:**

- [ ] Smoke flow: Create project via POST -> verify 201 + valid UUID
- [ ] Smoke flow: Get project via GET -> verify all fields match creation request
- [ ] Smoke flow: List projects -> verify created project appears in list
- [ ] Smoke flow: Delete project via DELETE -> verify 204
- [ ] Smoke flow: Get deleted project -> verify 404
- [ ] Smoke flow: Tenant isolation -- project created by tenant A not visible to tenant B

**Flow 2 -- Simple Conversation (NATS roundtrip):**

- [ ] Smoke flow: Create conversation for project -> verify 201
- [ ] Smoke flow: Send message (non-agentic) -> verify 200 with `run_id`
- [ ] Smoke flow: Poll messages until assistant response appears (timeout: 60s)
- [ ] Smoke flow: Verify response: non-empty content, status "completed", cost_usd >= 0
- [ ] Smoke flow: Verify NATS roundtrip: Go published `conversation.run.start` -> Python consumed -> Python published `conversation.run.complete` -> Go received
- [ ] Smoke flow: Cleanup -- delete conversation

**Flow 3 -- Agentic Conversation with Tool Use:**

- [ ] Smoke flow: Send agentic message ("List files in this directory") -> verify `run_id`
- [ ] Smoke flow: Verify AG-UI events via WebSocket: `run_started`, `tool_call`, `tool_result`, `run_finished`
- [ ] Smoke flow: Verify at least 1 tool call executed (ListDir or Glob)
- [ ] Smoke flow: Verify assistant response references tool output
- [ ] Smoke flow: Verify cost tracking: `cost_usd > 0`, `tokens_in > 0`, `tokens_out > 0`
- [ ] Smoke flow: Verify step count > 1 (multi-turn)

**Flow 4 -- Benchmark Run:**

- [ ] Smoke flow: Create benchmark run (type: "simple", cheapest available model)
- [ ] Smoke flow: Poll run status until "completed" (timeout: 120s)
- [ ] Smoke flow: Get results -> verify non-empty results array
- [ ] Smoke flow: Verify each result: `task_id`, `scores` map non-empty, `cost_usd >= 0`
- [ ] Smoke flow: Verify run summary: `task_count > 0`, `avg_score > 0`
- [ ] Smoke flow: Cleanup -- delete benchmark run

**Flow 5 -- Retrieval + GraphRAG Pipeline:**

- [ ] Smoke flow: Trigger project indexing via POST
- [ ] Smoke flow: Poll index status until "ready" (timeout: 120s)
- [ ] Smoke flow: Search project with query -> verify results array with scores
- [ ] Smoke flow: Trigger graph build via POST
- [ ] Smoke flow: Poll graph status until "ready" (timeout: 120s)
- [ ] Smoke flow: Search graph with symbol query -> verify results

**Flow 6 -- Memory Store + Recall:**

- [ ] Smoke flow: Store memory entry via POST (`content`, `type: "fact"`)
- [ ] Smoke flow: Recall memory via POST with matching query
- [ ] Smoke flow: Verify recalled memory: content matches, score > 0
- [ ] Smoke flow: Verify recall with non-matching query: empty or low-score results

#### A3: CI Integration (Priority: MEDIUM)

> Run contract tests and smoke tests in GitHub Actions.

- [ ] Add `contract` CI job: runs Go contract test generator + Python contract validator
  - No external services needed (pure serialization)
  - Runs on every push to staging/main
- [ ] Add `smoke` CI job: depends on `go` + `python` + `contract` jobs
  - Services: postgres, nats (same as existing `go` job)
  - Start Go backend in background (`APP_ENV=development`)
  - Start Python worker in background
  - Start LiteLLM with mock config or skip LLM-dependent tests
  - Run `go test -tags=smoke -timeout=300s ./tests/integration/...`
- [ ] Configure smoke test LLM strategy: env var `SMOKE_LLM_MODE` (mock / free-tier / skip)
  - `mock`: canned LLM responses (fastest, most reliable)
  - `free-tier`: use Gemini Flash or similar (real but rate-limited)
  - `skip`: skip LLM-dependent flows, only test infrastructure flows
- [ ] Upload verification matrix as CI artifact after smoke tests
- [ ] Add status badge to README: "Integration Tests: passing/failing"

#### C1: Feature Verification Matrix (Priority: MEDIUM)

> Living document tracking verification status of all 30 major features.
> File: `docs/feature-verification-matrix.md`

- [ ] Create initial matrix with all 30 features listed (see design doc for full list)
- [ ] Define verification criteria per feature: which test types must pass
- [ ] Mark currently-passing features based on existing test results
- [ ] Add "Last Verified" date column -- updated on each full test run
- [ ] Cross-reference: each feature row links to relevant test files

#### C2: Automated Verification Reporter (Priority: MEDIUM)

> Script that runs all test suites and generates the feature matrix automatically.
> File: `scripts/verify-features.sh`

- [ ] Parse Go test output (`go test -json`) and map packages to features
- [ ] Parse Python test output (`pytest --json-report`) and map modules to features
- [ ] Parse Playwright results (if stack running) and map specs to features
- [ ] Parse contract test results and map payload types to features
- [ ] Parse smoke test results and map flows to features
- [ ] Generate markdown table output (feature-verification-matrix.md)
- [ ] Generate JSON summary for CI consumption
- [ ] Exit code: 0 if all critical features pass, 1 if any critical feature fails
- [ ] Add `--critical-only` flag to only check features 1-10 + 22-23

#### C3: CI Verification Gate (Priority: LOW)

> Block merges to main if critical features regress.

- [ ] Define critical feature set: Project CRUD, Auth, Tenancy, Model Registry, Conversations (simple + agentic), Agent Tools, Policies, Cost Tracking, Benchmarks
- [ ] Add verification reporter as required CI check on PRs to main
- [ ] Non-critical features (A2A, LSP, Handoff, etc.): warn but don't block
- [ ] Store historical verification results for trend tracking
