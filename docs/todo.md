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
- [x] Goal system redesign: replaced `manage_goals` HTTP-callback tool with `propose_goal` AG-UI event tool (2026-03-09)
- [x] Rewritten goal-researcher mode with GSD questioning methodology (interview-first)
- [x] GoalProposalCard UI for approve/reject of agent-proposed goals
- [x] Context injection: docs/PROJECT.md, REQUIREMENTS.md, STATE.md passed to goal-researcher agent
- [x] Functional options pattern (`AgenticOption`/`WithContextEntries`) for extensible agentic dispatch

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

#### Benchmark Interactive Testing Guide (COMPLETED)
- [x] (2026-03-15) Added step-by-step interactive E2E testing guide to `docs/dev-setup.md` — covers infrastructure verification, API-level testing (all 3 benchmark types, evaluator combinations, comparison, cost analysis, export), frontend dashboard walkthrough, error scenarios, automated E2E suite, troubleshooting table, custom dataset creation

#### Benchmark Cross-Layer Bug Fixes (COMPLETED)
- [x] 7 bugs fixed: DB migration for rollout fields, cost population, NATS wiring, CSV export

#### Benchmark Validation E2E Bug Fixes (COMPLETED)
- [x] (2026-03-15) **Bug 1 — Score Key Mismatch (Medium):** Evaluator dimension names (`correctness`, `sparc_*`, `trajectory_*`) didn't match metric request names (`llm_judge`, `sparc`, `trajectory_verifier`). Added `_aggregate_metric_scores()` with `_DIMENSION_TO_METRIC` mapping (17 entries) in `workers/codeforge/consumer/_benchmark.py`. 16 tests in `workers/tests/test_score_key_normalization.py`.
- [x] (2026-03-15) **Bug 2 — Stuck "running" Runs (High):** Runs with invalid params stayed `"running"` forever. Fix 2A: `StartRun()` returns error when dataset resolution fails and no suite fallback. Fix 2B: Watchdog goroutine scans every 5 min for runs stuck >15 min. Added `ErrorMessage` field to `Run` struct + DB migration `072`. Files: `internal/service/benchmark.go`, `internal/domain/benchmark/benchmark.go`, `internal/adapter/postgres/store_benchmark.go`, `cmd/codeforge/main.go`. 5 tests in `internal/service/benchmark_test.go`.
- [x] (2026-03-15) **Bug 3 — Invalid Model Silently Succeeds (Medium):** LiteLLM fell back to default model. Added `_validate_model_exists()` checking `/v1/models` endpoint in `workers/codeforge/consumer/_benchmark.py`. 6 tests in `workers/tests/test_model_validation.py`.
- [x] (2026-03-15) **Bug 4 — `model=auto` Without Routing (Low):** `_resolve_effective_llm()` silently passed `"auto"` to LiteLLM. Now raises `ValueError` when router unavailable. 2 tests in `workers/tests/test_model_validation.py`.
- [x] (2026-03-15) **Bug 5 — LLM Judge Context Overflow (Low):** Evaluators exceeded local model context limits. Added `compress_for_context()` head+tail truncation in `workers/codeforge/evaluation/evaluators/prompt_compressor.py`. Enhanced error fallback distinguishes `context_overflow` from `evaluation_failed`. 18 tests in `workers/tests/test_prompt_compressor.py`.
- [x] (2026-03-15) Updated E2E test assertions in `block-3-agent.spec.ts`, `block-4-routing.spec.ts`, `block-5-errors.spec.ts` to verify all fixes
- Findings: `frontend/e2e/benchmark-validation/FINDINGS.md`
- Plan: `docs/superpowers/plans/2026-03-11-benchmark-findings-fixes.md`

#### Frontend UI Bug Fixes & i18n (COMPLETED)
- [x] (2026-03-15) **BUG-1 (High) — Broken "Go to Chat" Navigation:** `onNavigate("chat")` silently did nothing — `"chat"` was not a valid `LeftTab`. Created unified `handleNavigate()` in `ProjectDetailPage.tsx` that switches `mobileView` to `"chat"` on mobile. Replaced 8 duplicate inline handlers. Fixed in: `GoalsPanel.tsx:202`, `SessionPanel.tsx:116`, `WarRoom.tsx:90`, `OnboardingProgress.tsx:44`.
- [x] (2026-03-15) **BUG-2 (Medium) — Dead RunPanel Code:** `run.toolcall` WS event was a stub comment. `RunPanel.addToolCall`/`updateRunStatus` attached to component function object but never called. Removed dead code — tool calls are rendered via AG-UI events in `ChatPanel`. Files: `ProjectDetailPage.tsx`, `RunPanel.tsx`.
- [x] (2026-03-15) **BUG-3 (Low) — `window.prompt()` for Folder Creation:** Replaced `window.prompt("New folder name:")` with custom Modal dialog consistent with Create/Rename/Delete modals. New state: `showFolderModal`, `newFolderName`, `newFolderPrefix`. File: `FilePanel.tsx`.
- [x] (2026-03-15) **Monaco Theme Sync:** Editor now reactively follows dark/light theme toggle via `createEffect` + `monaco.editor.setTheme()`. File: `CodeEditor.tsx`.
- [x] (2026-03-15) **File Panel Icon Alignment:** Expand/Collapse-all SVG polyline points centered in 16x16 viewBox. File: `FilePanel.tsx`.
- [x] (2026-03-15) **i18n: ~40 hardcoded strings replaced** across `FilePanel.tsx`, `FileContextMenu.tsx`, `GoalProposalCard.tsx`, `KnowledgeBasesPage.tsx`. 28 new keys in `en.ts` + `de.ts` (`files.*`, `common.approve`, `common.reject`, `detail.tab.files`).
- Remaining: `TODO` in `PermissionRequestCard.tsx:50` — "Allow Always" does not persist policy rule to backend (requires new API endpoint).

#### Benchmark Live Feed (COMPLETED)
- [x] (2026-03-10) Go: `TrajectoryEventPayload` in `events.go` — enriched WS broadcast with cost, tokens, input, output, step fields
- [x] (2026-03-10) Go: Runtime trajectory subscription handler broadcasts enriched payload
- [x] (2026-03-10) TypeScript: `LiveFeedEvent` + `BenchmarkLiveProgress` types in `api/types.ts`
- [x] (2026-03-10) Frontend: `BenchmarkLiveFeed.tsx` — virtualized auto-scrolling feed with `@tanstack/solid-virtual`, feature accordions, progress header, elapsed timer
- [x] (2026-03-10) Integration: Wired into `BenchmarkPage.tsx` for selected running runs
- Design: `docs/plans/2026-03-10-benchmark-live-feed-design.md`
- Plan: `docs/plans/2026-03-10-benchmark-live-feed-plan.md`

#### Agent-Eval Benchmark Results (2026-03-10)
- [x] (2026-03-10) Ran `/agent-eval mistral/mistral-large-latest` — auto-agent pipeline end-to-end
- Result: 0/300 total score (Grade F) — Mistral model could not produce code within 43 min
- All skeleton files unchanged — agent stalled on spec reading without invoking Write tool
- Infrastructure verified: workspace paths absolute, project seeding works, test suites collect correctly

#### Test Suites (COMPLETED)
- [x] Browser E2E: 17 Playwright tests (health, navigation, projects, costs, models, a11y)
- [x] LLM E2E: 95 API-level tests across 12 spec files
- [x] Benchmark E2E: 132 browser Playwright tests across 12 spec files
- [x] Benchmark Validation E2E: 22 API-level tests across 6 blocks (`frontend/e2e/benchmark-validation/`)
- [x] Backend E2E: 88 pass / 0 fail / 3 skip (97% pass rate)
- [x] Python unit tests: 107 pass (includes 40 new tests from benchmark validation bug fixes)

#### Chat Enhancements (COMPLETED)
- [x] (2026-03-10) Phase 1: HITL permission UI + `supervised-ask-all` preset + autonomy-to-preset mapping
- [x] (2026-03-10) Phase 2: Inline diff review with DiffPreview component + file content endpoint
- [x] (2026-03-10) Phase 3: Action buttons (copy, retry, apply, view diff) on agent messages
- [x] (2026-03-10) Phase 4: Per-message cost tracking with MessageBadge + CostBreakdown
- [x] (2026-03-10) Phase 5: Smart references with @/#// autocomplete popover + frequency tracker
- [x] (2026-03-10) Phase 6: Slash commands (/compact, /rewind, /clear, /help, /mode, /model)
- [x] (2026-03-10) Phase 7: Conversation full-text search with PostgreSQL FTS (GIN index, ts_rank)
- [x] (2026-03-10) Phase 8: Notification center with browser push, sound, tab badge, AG-UI wiring
- [x] (2026-03-10) Phase 9: Real-time channels with threads, domain model, sidebar integration
- [x] (2026-03-10) Phase 10+11: Feature spec + documentation updates
- Feature spec: [docs/features/05-chat-enhancements.md](features/05-chat-enhancements.md)

#### Subscription Provider Integration (COMPLETED)
- [x] (2026-03-10) OAuth device flow adapters: Anthropic (Claude Max) + GitHub Copilot
- [x] (2026-03-10) Atomic EnvWriter service for .env file management
- [x] (2026-03-10) Subscription orchestration service (background polling, token exchange, .env persistence)
- [x] (2026-03-10) HTTP endpoints: list/connect/status/disconnect providers
- [x] (2026-03-10) Python routing: github_copilot in key_filter, router tiers, meta_router tiers
- [x] (2026-03-10) LiteLLM config: extra_headers for github_copilot
- [x] (2026-03-10) Frontend: Subscription Providers section in SettingsPage with device flow UI
- [x] (2026-03-10) Tests: 22 auth adapter tests, 9 envwriter tests, 8 subscription tests, 48 Python routing tests

---

### E2E Playwright Test Findings (2026-03-09)

> Report: [docs/plans/2026-03-09-e2e-playwright-test-report.md](plans/2026-03-09-e2e-playwright-test-report.md)
> 5 findings from interactive end-to-end Playwright MCP test of full agent evaluation workflow.

#### F4: Workspace Path Resolution Bug (Priority: CRITICAL)

> Python worker resolves workspace paths relative to its own CWD (`workers/`) instead of the
> project root. Agent tools write files to `workers/data/workspaces/.../data/workspaces/.../`
> (doubled path). This blocks ALL agent file operations end-to-end.

- [x] F4.1: Add integration test reproducing the bug (2026-03-09) — create a project, seed a file via API, then call `resolve_safe_path("data/workspaces/{tid}/{pid}", "lru_cache.py")` and assert the resolved path matches the actual file on disk
  - File: `workers/tests/test_workspace_path_resolution.py`
  - Run: `cd workers && poetry run pytest tests/test_workspace_path_resolution.py -v`
  - Expected: FAIL (confirms bug exists)

- [x] F4.2: Fix workspace path — Go `NewProjectService` resolves to absolute via `filepath.Abs()` (2026-03-09)
  - File: `workers/codeforge/tools/_base.py:64`
  - Current: `workspace = Path(workspace_path).resolve()` — resolves relative to CWD
  - Fix option A (preferred): Make Go Core send **absolute** paths in NATS payload — change `proj.WorkspacePath` to `filepath.Join(cfg.DataDir, proj.WorkspacePath)` before publishing
    - File: `internal/service/conversation_agent.go:255,433,698`
    - Also update: `internal/service/auto_agent.go` (wherever it publishes workspace_path)
  - Fix option B (fallback): In Python `_base.py`, detect relative paths and resolve against a known `CODEFORGE_ROOT` env var
    - File: `workers/codeforge/tools/_base.py:64`
    - Change: `workspace = Path(workspace_path) if Path(workspace_path).is_absolute() else (Path(os.environ.get("CODEFORGE_ROOT", "/workspaces/CodeForge")) / workspace_path)`

- [x] F4.3: Fix `bash.py` tool CWD — automatically fixed by F4.2 (absolute paths from Go) (2026-03-09)
  - File: `workers/codeforge/tools/bash.py:81`
  - Current: `cwd=workspace_path` (relative → wrong CWD)
  - Fix: Same as F4.2 — if Go sends absolute path, this is automatically fixed

- [x] F4.4: Add Go workspace path tests — 3 tests in `project_workspace_test.go` (2026-03-09)
  - File: `internal/port/messagequeue/contract_test.go` (add assertion)
  - File: `workers/tests/test_nats_contracts.py` (add assertion)
  - Rule: `workspace_path` must start with `/` in all `conversation.run.start` payloads

- [x] F4.5: Integration tests passing (2026-03-09)
  - Run: `cd workers && poetry run pytest tests/test_workspace_path_resolution.py -v`
  - Expected: PASS

- [x] (2026-03-10) F4.6: Re-run agent-eval to verify end-to-end fix — workspace paths are absolute (verified), but Mistral model scored 0/300 (agent stalled, never wrote code in 43 min)
  - Run: `/agent-eval mistral/mistral-large-latest`
  - Result: 0/3 features completed, 1 failed, agent timed out after 43 min
  - Root cause: Mistral free-tier rate limiting + model unable to invoke Write tool correctly
  - Workspace path fix (F4.2) confirmed working — paths are absolute in NATS payloads

#### F3: Routing Does Not Fallback on Provider Billing Errors (Priority: HIGH)

> When Anthropic credits are exhausted, the router selected `anthropic/claude-sonnet-4` and
> failed without trying another provider. Rate tracker only handles 429 (rate limit), not
> 401/402/billing errors.

- [x] F3.1: Add test for billing/auth error classification — 4 tests (2026-03-09)
  - File: `workers/tests/test_routing_rate_tracker.py` (new or extend existing)
  - Test: Call `rate_tracker.record_error("anthropic", error_type="billing")` → `is_exhausted("anthropic")` returns `True`
  - Test: Call `rate_tracker.record_error("anthropic", error_type="auth")` → `is_exhausted("anthropic")` returns `True`
  - Run: `cd workers && poetry run pytest tests/test_routing_rate_tracker.py -v`

- [x] F3.2: Add `record_error()` to `RateLimitTracker` with billing/auth cooldowns (2026-03-09)
  - File: `workers/codeforge/routing/rate_tracker.py`
  - Add: `record_error(provider, error_type)` method that marks provider as exhausted for longer duration (e.g. 1 hour for billing, 5 min for auth)
  - Billing/auth errors should mark provider exhausted with longer cooldown than rate limits

- [x] F3.3: Add `classify_error_type()` in `llm.py` wired into `_with_retry` (2026-03-09)
  - File: `workers/codeforge/llm.py`
  - Parse LiteLLM exceptions: `AuthenticationError` → "auth", `BudgetExceededError` / status 402 → "billing", `RateLimitError` → "rate_limit"
  - Feed classification to rate tracker on failure

- [x] (2026-03-09) F3.4: Add retry-with-fallback in agent loop LLM call path
  - File: `workers/codeforge/agent_loop.py`
  - Wired `classify_error_type()` + `get_tracker().record_error()` into `_try_model_fallback`
  - Provider extracted from model string (e.g., "anthropic" from "anthropic/claude-sonnet-4")

- [x] (2026-03-09) F3.5: Enable routing by default when multiple providers are configured
  - File: `internal/config/config.go` — `Defaults()` now sets `Routing.Enabled: true`
  - Override: `CODEFORGE_ROUTING_ENABLED=false` or YAML `routing.enabled: false`

- [x] (2026-03-10) F3.6: E2E test verifying full routing fallback chain — 6/6 tests pass
  - File: `workers/tests/test_routing_fallback_e2e.py`
  - Tests: billing error classification, fallback chain filtering, agent loop model fallback, all fallbacks exhausted, auth error trigger, rate limit short cooldown
  - Run: `cd workers && poetry run pytest tests/test_routing_fallback_e2e.py -v`

#### F6: Tool-Message Format Incompatibility on Mid-Conversation Model Switch (Priority: HIGH)

> When `_RoutingLLMWrapper` routes to different models per-iteration within the same agent loop
> (e.g. Gemini → Groq), the message history accumulates tool-result messages in one provider's
> format that the new provider rejects. Groq requires `content` on `role:tool` messages, but
> Gemini may omit it.
>
> Error: `'messages.3' : for 'role:tool' the following must be satisfied[('messages.3.content' : property 'content' is missing)]`

- [x] (2026-03-10) F6.1: Add test reproducing tool-message format rejection on model switch — 12 tests
  - File: `workers/tests/test_tool_message_compat.py`
  - Tests: empty content, missing content, None content, missing tool_call_id, mixed messages, routing wrapper sanitization
  - Run: `cd workers && poetry run pytest tests/test_tool_message_compat.py -v`

- [x] (2026-03-10) F6.2: Add `sanitize_tool_messages()` normalizer in `agent_loop.py`
  - File: `workers/codeforge/agent_loop.py`
  - Ensures all `role:tool` messages have `content` (defaults to `""`) and `tool_call_id`
  - Also fixed `_payload_to_dict()` to always include `content` for `role:tool` messages

- [x] (2026-03-10) F6.3: Wire sanitizer into `_RoutingLLMWrapper` and `AgentLoopExecutor`
  - File: `workers/codeforge/consumer/_benchmark.py` — `_RoutingLLMWrapper._sanitize_messages()` called before forwarding
  - File: `workers/codeforge/agent_loop.py` — `sanitize_tool_messages(messages)` called before `chat_completion_stream`

- [x] (2026-03-10) F6.4: Integration test — routing wrapper with tool message sanitization
  - File: `workers/tests/test_tool_message_compat.py` (`TestSanitizeWiredIntoLLMCall`)
  - Verifies `_RoutingLLMWrapper` sanitizes messages before forwarding to real LLM

#### F7: 429 Rate-Limit Should Trigger Immediate Model Fallback (Priority: HIGH)

> The `_with_retry` logic in `llm.py` retries the same rate-limited model 2 times with
> exponential backoff (20s + 58s waits) before failing, instead of immediately switching
> to a fallback model. This wastes ~78+ seconds per task on exhausted providers (e.g.
> Gemini free-tier 20 req/min limit).
>
> Expected: On first 429, immediately try the next fallback model from the routing plan.
> Current: Retries same exhausted model 2x, then fails the entire call.

- [x] (2026-03-10) F7.1: Add test for immediate-fallback-on-429 behavior — 8 tests
  - File: `workers/tests/test_llm_retry_fallback.py`
  - Tests: 429 raises immediately (no retry), 502/503/504 still retried, model fallback on 429, fallback chain exhausted, 429 total time <2s, rate tracker integration
  - Run: `cd workers && poetry run pytest tests/test_llm_retry_fallback.py -v`

- [x] (2026-03-10) F7.2: Remove 429 from `retryable_codes` in `LLMClientConfig`
  - File: `workers/codeforge/llm.py` (line 184)
  - Changed: `retryable_codes` from `(429, 502, 503, 504)` to `(502, 503, 504)`
  - 429 now propagates immediately to agent loop's `_try_model_fallback` which switches models
  - No refactoring of `_with_retry` needed — existing agent loop fallback logic handles everything

- [x] (2026-03-10) F7.3: No additional wiring needed
  - Agent loop's `_handle_llm_error` → `_try_model_fallback` → `_pick_next_fallback` already handles model switching via `LoopConfig.fallback_models`
  - The fix in F7.2 (removing 429 from retryable) is sufficient to trigger this existing path

- [x] (2026-03-10) F7.4: Integration tests verify full fallback chain
  - File: `workers/tests/test_llm_retry_fallback.py` (`TestFallbackChain`, `TestRateLimitTrackerIntegration`)
  - 429 resolves to fallback model in <2s, rate tracker records exhausted providers

#### F1: No File Upload/Create in Project UI (Priority: MEDIUM)

> FilePanel only displays files — no buttons to create, upload, or edit files.
> Users must use the REST API as workaround.

- [x] F1.1: Add i18n keys for file management actions (2026-03-09)
  - File: `frontend/src/i18n/en.ts`
  - Keys: `files.createFile`, `files.uploadFile`, `files.fileName`, `files.fileContent`, `files.createSuccess`, `files.uploadSuccess`, `files.createFailed`
  - File: `frontend/src/i18n/locales/de.ts` (German translations)

- [x] F1.2: Add "Create File" button and modal to FilePanel (2026-03-09)
  - File: `frontend/src/features/project/FilePanel.tsx`
  - Add: Button in the file tree header (+ icon or "New File" text)
  - Add: Modal with `path` (text input) and `content` (textarea) fields
  - Call: `api.files.write(projectId, path, content)` on submit
  - Show: toast on success/error

- [x] (2026-03-09) F1.3: Add "Upload File" button with native file picker
  - File: `frontend/src/features/project/FilePanel.tsx`
  - Upload button with SVG icon in SidebarHeader, hidden `<input type="file">`, FileReader handler
  - Calls `api.files.write()`, shows toast, opens uploaded file in editor

- [x] (2026-03-10) F1.4: Playwright E2E tests for file creation — 4/4 pass
  - File: `frontend/e2e/file-crud.spec.ts`
  - Tests: create file via API + verify in file tree, read back content, overwrite replaces content, special characters in name
  - Run: `cd frontend && npx playwright test file-crud.spec.ts`

#### F2: No Feature Description Field in Create/Edit Modal (Priority: MEDIUM)

> FeatureCardForm only has a title input — no description/body textarea.
> Feature descriptions are critical for agent consumption (contain full problem specs).

- [x] F2.1: Add i18n key `featuremap.descriptionPlaceholder` (2026-03-09)
  - File: `frontend/src/i18n/en.ts`
  - Keys: `featuremap.description`, `featuremap.descriptionPlaceholder`
  - File: `frontend/src/i18n/locales/de.ts`

- [x] F2.2: Description wired into create/update API calls (2026-03-09)
  - File: `frontend/src/api/client.ts` (around line 1170)
  - Check: Does `api.roadmap.createFeature()` accept a `description` field? If not, add it.
  - Check: Does `api.roadmap.updateFeature()` accept a `description` field? If not, add it.

- [x] F2.3: Add description textarea to FeatureCardForm (2026-03-09)
  - File: `frontend/src/features/project/featuremap/FeatureCardForm.tsx`
  - Add: `const [description, setDescription] = createSignal(props.feature?.description ?? "");`
  - Add: `<textarea>` between the title input and status selector
  - Pass: `description` to `createFeature()` and `updateFeature()` calls (lines 48, 41)
  - Style: Match existing form patterns (rounded-cf-sm, border-cf-border, p-2)

- [x] (2026-03-10) F2.4: Playwright E2E tests for feature descriptions — 4/4 pass
  - File: `frontend/e2e/feature-description.spec.ts`
  - Tests: create with description, update persists changes, visible in project UI, empty description allowed
  - Run: `cd frontend && npx playwright test feature-description.spec.ts`

#### F5: Playwright MCP Session Not Recoverable After Container Restart (Priority: LOW)

> After `docker restart codeforge-playwright`, the MCP session ID becomes stale.
> All subsequent browser_* calls return "Session not found".

- [x] F5.1: Document Playwright MCP session limitation (2026-03-09, commit 197557c)
  - File: `docs/dev-setup.md`
  - Add section: "Playwright MCP Container" with note that session is lost on restart
  - Workaround: Restart the Claude Code session (or MCP client) after container restart

- [x] F5.2: Add health check to Playwright Docker service (2026-03-09, commit 197557c)
  - File: `docker-compose.yml`
  - Add: `healthcheck` to `codeforge-playwright` service (test: HTTP GET to :8001/mcp)
  - Ensures container is only "healthy" when MCP server is accepting connections

---

### Auto-Agent Skills System (Phase 31)

> Design: [docs/plans/2026-03-09-auto-agent-skills-design.md](plans/2026-03-09-auto-agent-skills-design.md)
> Plan: [docs/plans/2026-03-09-auto-agent-skills-plan.md](plans/2026-03-09-auto-agent-skills-plan.md)
> Goal: Auto-agent automatically selects and uses relevant skills via LLM, with multi-format import, agent-generated skills, and prompt injection protection.

#### Task 1: DB Migration — Extend skills table (Priority: CRITICAL)
- [x] T1.1: Write migration 067 — add type, source, source_url, format_origin, status, usage_count, content columns (2026-03-09)
  - File: `internal/adapter/postgres/migrations/067_extend_skills.sql`
  - Includes: check constraints, status index, data migration (code → content)
- [x] T1.2: Verify migration applies cleanly (2026-03-09)
- [x] T1.3: Commit (2026-03-09)

#### Task 2: Go Domain Model — Extend Skill struct (Priority: CRITICAL)
- [x] T2.1: Write failing tests for new fields and validation (content required, invalid type, valid workflow, status/source constants) (2026-03-09)
  - File: `internal/domain/skill/skill_test.go`
- [x] T2.2: Implement extended Skill struct with Type, Source, SourceURL, FormatOrigin, Status, UsageCount, Content fields (2026-03-09)
  - File: `internal/domain/skill/skill.go`
- [x] T2.3: Run tests — all pass (2026-03-09)
- [x] T2.4: Commit (2026-03-09)

#### Task 3: Go Postgres Store — Update SQL queries (Priority: CRITICAL)
- [x] T3.1: Add `IncrementSkillUsage` and `ListActiveSkills` to store interface (2026-03-09)
  - File: `internal/port/database/store.go`
- [x] T3.2: Update all SQL queries in store for new columns, status-based filtering (2026-03-09)
  - File: `internal/adapter/postgres/store_skill.go`
- [x] T3.3: Run existing store tests — backwards compat passes (2026-03-09)
- [x] T3.4: Commit (2026-03-09)

#### Task 4: Go Service — Update SkillService (Priority: HIGH)
- [x] T4.1: Update Create (defaults: type=pattern, source=user, status=active), Update (handle status), List (active-only) (2026-03-09)
  - File: `internal/service/skill.go`
- [x] T4.2: Run tests — pass (2026-03-09)
- [x] T4.3: Commit (2026-03-09)

#### Task 5: Python Model — Extend Pydantic Skill (Priority: CRITICAL)
- [x] T5.1: Write tests for new fields and defaults (2026-03-09)
  - File: `workers/tests/test_skill_models.py`
- [x] T5.2: Update Pydantic Skill model with type, source, status, format_origin, usage_count, content, source_url (2026-03-09)
  - File: `workers/codeforge/skills/models.py`
- [x] T5.3: Run tests — all pass (2026-03-09)
- [x] T5.4: Commit (2026-03-09)

#### Task 6: Quarantine Scorer — Add prompt injection patterns (Priority: HIGH)
- [x] T6.1: Write failing tests for prompt override, role hijack, exfiltration detection (2026-03-09)
  - File: `internal/domain/quarantine/scorer_test.go`
- [x] T6.2: Add 3 new regex patterns (promptOverridePattern, roleHijackPattern, exfilPattern) and scoring blocks (2026-03-09)
  - File: `internal/domain/quarantine/scorer.go`
- [x] T6.3: Run all quarantine tests — all pass (2026-03-09)
- [x] T6.4: Commit (2026-03-09)

#### Task 7: Python Format Parsers — Multi-format skill import (Priority: HIGH)
- [x] T7.1: Write tests for CodeForge YAML, Claude Skills, Cursor Rules, plain Markdown, .mdc, unknown format (2026-03-09)
  - File: `workers/tests/test_skill_parsers.py`
- [x] T7.2: Implement `parse_skill_file()` with format detection and 4 parsers (2026-03-09)
  - File: `workers/codeforge/skills/parsers.py`
- [x] T7.3: Run tests — all pass (2026-03-09)
- [x] T7.4: Commit (2026-03-09)

#### Task 8: Python Skill Selector — LLM-based pre-loop selection (Priority: CRITICAL)
- [x] T8.1: Write tests for `resolve_skill_selection_model()` (cheapest, fallback) and `select_skills_for_task()` (LLM match, BM25 fallback) (2026-03-09)
  - File: `workers/tests/test_skill_selector.py`
- [x] T8.2: Implement selector with LLM selection + BM25 fallback + design decision docs (2026-03-09)
  - File: `workers/codeforge/skills/selector.py`
- [x] T8.3: Run tests — all pass (2026-03-09)
- [x] T8.4: Commit (2026-03-09)

#### Task 9: Python `search_skills` Tool (Priority: HIGH)
- [x] T9.1: Write tests for BM25 search, empty results, type filtering (2026-03-09)
  - File: `workers/tests/test_tool_search_skills.py`
- [x] T9.2: Implement SearchSkillsTool (ToolDefinition + ToolExecutor) (2026-03-09)
  - File: `workers/codeforge/tools/search_skills.py`
- [x] T9.3: Register in `build_default_registry()` in `workers/codeforge/tools/__init__.py` (2026-03-09)
- [x] T9.4: Run tests — all pass (2026-03-09)
- [x] T9.5: Commit (2026-03-09)

#### Task 10: Python `create_skill` Tool (Priority: HIGH)
- [x] T10.1: Write tests for validation, draft save, injection rejection, content length limit (2026-03-09)
  - File: `workers/tests/test_tool_create_skill.py`
- [x] T10.2: Implement CreateSkillTool with validation, regex safety check, DB save as draft (2026-03-09)
  - File: `workers/codeforge/tools/create_skill.py`
- [x] T10.3: Register in `build_default_registry()` (2026-03-09)
- [x] T10.4: Run tests — all pass (2026-03-09)
- [x] T10.5: Commit (2026-03-09)

#### Task 11: Python Safety Check — LLM-based injection detection (Priority: MEDIUM)
- [x] T11.1: Write tests for safe content, unsafe content, LLM error fallback (2026-03-09)
  - File: `workers/tests/test_skill_safety.py`
- [x] T11.2: Implement `check_skill_safety()` with LLM call using cheapest model (2026-03-09)
  - File: `workers/codeforge/skills/safety.py`
- [x] T11.3: Run tests — all pass (2026-03-09)
- [x] T11.4: Commit (2026-03-09)

#### Task 12: Update Conversation Consumer — LLM skill selection (Priority: CRITICAL)
- [x] T12.1: Write test for new injection flow (LLM selection, sandboxed `<skill>` tags, workflow/pattern separation) (2026-03-09)
- [x] T12.2: Replace `_inject_skill_recommendations()` with `_inject_skills()` in `_build_system_prompt()` (2026-03-09)
  - File: `workers/codeforge/consumer/_conversation.py`
- [x] T12.3: Add sandboxing instruction to system prompt (2026-03-09)
- [x] T12.4: Run full conversation consumer tests — all pass (2026-03-09)
- [x] T12.5: Commit (2026-03-09)

#### Task 13: Meta-Skill — Built-in skill creator (Priority: MEDIUM)
- [x] T13.1: Write test that meta-skill YAML parses correctly (2026-03-09)
  - File: `workers/tests/test_builtin_skills.py`
- [x] T13.2: Create meta-skill YAML with schema docs, examples, quality criteria (2026-03-09)
  - File: `workers/codeforge/skills/builtins/codeforge-skill-creator.yaml`
- [x] T13.3: Add builtin loader to SkillRegistry (2026-03-09)
  - File: `workers/codeforge/skills/registry.py`
- [x] T13.4: Run tests — all pass (2026-03-09)
- [x] T13.5: Commit (2026-03-09)

#### Task 14: Go Import Handler — HTTP endpoint (Priority: MEDIUM)
- [x] T14.1: Implement `POST /api/v1/skills/import` handler (URL fetch, format detect, safety score, save) (2026-03-09)
  - File: `internal/adapter/http/handlers_skill_import.go`
- [x] T14.2: Add route to `routes.go` (2026-03-09)
- [x] T14.3: Write handler tests (2026-03-09)
- [x] T14.4: Commit (2026-03-09)

#### Task 15: WebSocket Skill Draft Notification (Priority: LOW)
- [x] T15.1: Add `SkillDraftEvent` struct to `internal/adapter/ws/events.go` (2026-03-09)
- [x] T15.2: Emit WebSocket event when agent creates a skill draft (via NATS → Go → WS broadcast) (2026-03-09)
- [x] T15.3: Commit (2026-03-09)

#### Task 16: Documentation and Exports (Priority: LOW)
- [x] T16.1: Update `workers/codeforge/skills/__init__.py` exports (2026-03-09)
- [x] T16.2: Update `CLAUDE.md` with skills system references (2026-03-09)
- [x] T16.3: Update `docs/features/04-agent-orchestration.md` (2026-03-09)
- [x] T16.4: Mark completed tasks in `docs/todo.md` (2026-03-09)
- [x] T16.5: Final commit (2026-03-09)

#### Adaptive Context Injection (COMPLETED)
- [x] (2026-03-09) AdaptiveContextBudget function: linear decay from base budget to 0 over 60 messages
- [x] (2026-03-09) Wire adaptive budget into conversation dispatch (buildConversationContextEntries)
- [x] (2026-03-09) Auto-trigger all indexes (RepoMap, Retrieval, GraphRAG) after clone/adopt/setup
- [x] (2026-03-09) Update documentation (CLAUDE.md, feature spec, todo)

#### Feature Activation Sweep (COMPLETED)
- [x] (2026-03-09) Activate Context Optimizer by default (ContextEnabled=true in Go config)
- [x] (2026-03-09) Add 16 missing env-var bindings in loader.go (agent context, quarantine, LSP, review router, copilot, routing, experience)
- [x] (2026-03-09) Add tenant_id to ConversationRunStartPayload (Go NATS + Python Pydantic) for Experience Pool isolation
- [x] (2026-03-09) Add max_entries eviction logic to Experience Pool store()
- [x] (2026-03-09) Integrate Experience Pool into AgentLoopExecutor (pre-loop cache check + post-loop store)
- [x] (2026-03-09) Enable OpenTelemetry tracing with Jaeger collector in codeforge.yaml
- [x] (2026-03-09) Fix routing default inconsistency (config.py default aligned to True)
- [x] (2026-03-09) Documentation updates (env vars in dev-setup.md, experience pool in agent-orchestration.md)

---

### Benchmark External Providers, Auto-Routing & Prompt Optimization (COMPLETED)

> Design: [docs/plans/2026-03-09-benchmark-external-providers-design.md](plans/2026-03-09-benchmark-external-providers-design.md)
> Plan: [docs/plans/2026-03-09-benchmark-external-providers-plan.md](plans/2026-03-09-benchmark-external-providers-plan.md)

- [x] (2026-03-09) Migration 068: routing tracking columns (selected_model, routing_reason, fallback_chain, fallback_count, provider_errors)
- [x] (2026-03-09) Domain: routing fields on Result, ProviderConfig on CreateRunRequest, ModelFamily utility
- [x] (2026-03-09) Store: updated INSERT/scan for 5 new routing columns
- [x] (2026-03-09) NATS payloads: provider_name/config on BenchmarkRunRequestPayload, routing fields on BenchmarkTaskResult, ModelAdaptations on ModePayload
- [x] (2026-03-09) Service: suite-based StartRun with provider_name resolution, mergeProviderConfig, SeedDefaultSuites (11 suites)
- [x] (2026-03-09) Python: universal task filter with difficulty/shuffle/seed/max_tasks/task_percentage (9 tests)
- [x] (2026-03-09) Python: config parameter on all 8 external provider constructors, auto-import in __init__.py
- [x] (2026-03-09) Python consumer: provider-based task loading with legacy fallback, _RoutingLLMWrapper for auto-model, routing report
- [x] (2026-03-09) Python: prompt optimizer with LLM-as-Critic failure analysis (9 tests)
- [x] (2026-03-09) Go: POST /runs/{id}/analyze endpoint for prompt optimization
- [x] (2026-03-09) Mode: ModelAdaptations map on Mode struct, appendModelAdaptation in conversation_agent.go
- [x] (2026-03-09) Frontend types: ProviderConfig, RoutingReport, PromptAnalysisReport, TacticalFix interfaces
- [x] (2026-03-09) Frontend: suite dropdown with optgroup Local/External, TaskSettings component, auto-model checkbox
- [x] (2026-03-09) Frontend: RoutingReport (model distribution, fallback timeline, provider status), SuiteManagement provider dropdown
- [x] (2026-03-09) Frontend: PromptOptimizationPanel with analyze/accept/reject
- [x] (2026-03-09) Frontend: analyzeRun API client method
- [x] (2026-03-09) i18n: 27+ benchmark keys in EN + DE

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
- [x] (2026-03-09) Dashboard Polish: KPI strip (7 stat cards with trend deltas), HealthDot (traffic-light per-project), ChartsPanel (5 Unovis tabs: cost trend, run outcomes, agents, models, cost/project), ActivityTimeline (WebSocket-fed 5-tier priority), CreateProjectModal extracted, ProjectCard enhanced with health + stats row

#### Pillar 1: Project Dashboard

- [x] (2026-03-09) Implement GitHub adapter with OAuth flow -- domain model, state store, service, HTTP handlers, `github-api` git provider, frontend OAuth connect button
- [x] (2026-03-09) Verify GitHub adapter compatibility with Forgejo/Codeberg -- provider aliases, variant config, detection, tests
- [x] (2026-03-09) Batch operations across selected repos -- batch API endpoints, store methods, frontend multi-select UI
- [x] (2026-03-09) Cross-repo search (code, issues) -- Go aggregation endpoint, frontend SearchPage with debounced input + project filter

#### Pillar 4: Agent Orchestration

- [x] (2026-03-09) Enhance CLI wrappers for Goose, OpenHands, OpenCode, Plandex -- streaming via NATS bridge, config passthrough, health check endpoints
- [x] (2026-03-09) Trajectory replay UI and audit trail -- TrajectoryPanel with event timeline, rewind with confirmation, export button, trajectory tab in ProjectDetailPage
- [x] (2026-03-09) Session events as source of truth (Resume/Fork/Rewind) -- session controls in ChatPanel, session indicators in conversation list, rewind with event picker UX

---

### Integration Testing Strategy (A + B + C)

> Design: [docs/plans/2026-03-08-integration-testing-design.md](plans/2026-03-08-integration-testing-design.md)
> Goal: Verify that all 30 major features work together across Go, Python, and Frontend layers.
> Tracking: [docs/feature-verification-matrix.md](feature-verification-matrix.md)

#### B1: Fix Broken Foundation Tests (Priority: CRITICAL) -- DONE 2026-03-08

> 10 Go Postgres Store tests were FAILING due to non-idempotent migration 065.
> Root cause: bare `ALTER TABLE ADD COLUMN` without `IF NOT EXISTS` guards.
> Fix: `internal/adapter/postgres/migrations/065_benchmark_result_rollout_fields.sql`

- [x] Fix `TestStore_ProjectCRUD` -- migration 065 idempotency fix
- [x] Fix `TestStore_UserCRUD` -- migration 065 idempotency fix
- [x] Fix `TestStore_TokenRevocation` -- migration 065 idempotency fix
- [x] Fix `TestStore_Conversation_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_GetProjectByRepoName_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_A2ATask_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_ListA2ATasks_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_ListA2ATasks_LimitParameterized` -- migration 065 idempotency fix
- [x] Fix `TestStore_RemoteAgent_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_ListRemoteAgents_TenantIsolation` -- migration 065 idempotency fix
- [x] Verify `go test ./internal/...` passes 100% green after all fixes

#### B2: NATS Payload Contract Tests (Priority: HIGH) -- DONE 2026-03-08

> Go side: `internal/port/messagequeue/contract_test.go` (20 sample factories, roundtrip + fixture generation)
> Python side: `workers/tests/test_nats_contracts.py` (80 parametrized tests, fixture validation + field coverage)
> Fixtures: `internal/port/messagequeue/testdata/contracts/*.json` (20 files, Go-generated)
> Contract violation found & fixed: `BenchmarkRunResult` Python model was missing `tenant_id`

**Infrastructure:**

- [x] Create Go contract test generator -- `TestContract_GenerateFixtures` writes 20 JSON fixtures
- [x] Create Python contract validator -- 80 parametrized tests: fixture parse, roundtrip, field coverage, required fields
- [x] Create reverse contract test -- Python roundtrip (Pydantic parse → dump → re-parse)
- [x] Add contract test verification checklist -- field coverage, required fields, tenant_id presence

**All 20 NATS payload types covered:**

- [x] Contract test: `conversation.run.start` -- PASS
- [x] Contract test: `conversation.run.complete` -- PASS
- [x] Contract test: `benchmark.run.request` -- PASS
- [x] Contract test: `benchmark.run.result` -- PASS (fixed missing `tenant_id` in Python model)
- [x] Contract test: `evaluation.gemmas.request` -- PASS
- [x] Contract test: `evaluation.gemmas.result` -- PASS
- [x] Contract test: `memory.store` -- PASS
- [x] Contract test: `memory.recall` -- PASS
- [x] Contract test: `memory.recall.result` -- PASS
- [x] Contract test: `repomap.generate.request` -- PASS
- [x] Contract test: `repomap.generate.result` -- PASS
- [x] Contract test: `retrieval.index.request` -- PASS
- [x] Contract test: `retrieval.index.result` -- PASS
- [x] Contract test: `retrieval.search.request` -- PASS
- [x] Contract test: `retrieval.search.result` -- PASS
- [x] Contract test: `retrieval.subagent.request` -- PASS
- [x] Contract test: `retrieval.subagent.result` -- PASS
- [x] Contract test: `graph.build.request` -- PASS
- [x] Contract test: `graph.build.result` -- PASS
- [x] Contract test: `graph.search.request` -- PASS
- [x] Contract test: `graph.search.result` -- PASS
- [x] Contract test: `a2a.task.created` -- PASS
- [x] Contract test: `a2a.task.complete` -- PASS
- [x] Contract test: `handoff.request` -- PASS

#### B3: Unit Tests for Untested Critical Modules (Priority: HIGH) -- DONE 2026-03-08

> 405+ tests created across Python and Go. All use mocks/fakes -- no Docker or external services needed.

**Python Agent Tools (134 tests across 6 test files):**

- [x] Test `tool_read.py` -- `workers/tests/test_tool_read_file.py` (22 tests)
- [x] Test `tool_write.py` -- `workers/tests/test_tool_write_file.py` (17 tests)
- [x] Test `tool_edit.py` -- `workers/tests/test_tool_edit_file.py` (15 tests)
- [x] Test `tool_bash.py` -- `workers/tests/test_tool_bash.py` (20 tests)
- [x] Test `tool_search.py`, `tool_glob.py`, `tool_listdir.py` -- `workers/tests/test_tool_search_glob_listdir.py` (35 tests)
- [x] Test tool registry -- `workers/tests/test_tool_registry.py` (25 tests)

**Python Consumer Dispatch (34 tests):**

- [x] Test `_base.py` -- duplicate detection, mixin helpers, DLQ -- `workers/tests/test_consumer_dispatch.py`
- [x] Test `_conversation.py` -- agentic vs simple routing, model resolution
- [x] Test duplicate detection -- `_is_duplicate()`, eviction behavior
- [x] Test error handling -- exception capture, error result publish
- [x] Test subject registration -- subject constants match Go side

**Python Memory System (26 tests):**

- [x] Test `scorer.py` -- composite scoring, recency decay, edge cases -- `workers/tests/test_memory_system.py`
- [x] Test `experience.py` -- `@exp_cache` decorator: hit, miss, key generation
- [x] Test vector storage interface -- store, recall, empty results

**Go Adapters (58 tests across 5 files):**

- [x] Test `adapter/a2a/` -- AgentCard, security schemes, skills -- `internal/adapter/a2a/agentcard_test.go` (16 tests)
- [x] Test `adapter/lsp/` -- JSON-RPC, notification, capability -- `internal/adapter/lsp/client_test.go` (15 tests)
- [x] Test `adapter/otel/` -- tracer, metrics, middleware, spans -- `internal/adapter/otel/setup_test.go` (9 tests)
- [x] Test `adapter/natskv/` -- KV get/set/delete, missing key -- `internal/adapter/natskv/cache_test.go` (8 tests)
- [x] Test `adapter/ristretto/` -- cache get/set/delete, TTL, eviction -- `internal/adapter/ristretto/cache_test.go` (10 tests)

**Go Domain Models (39 tests across 5 files):**

- [x] Test `domain/conversation/` -- creation, validation, status -- `internal/domain/conversation/conversation_test.go` (9 tests)
- [x] Test `domain/orchestration/` -- handoff model, validation -- `internal/domain/orchestration/orchestration_test.go` (4 tests)
- [x] Test `domain/microagent/` -- trigger matching, priority -- `internal/domain/microagent/microagent_test.go` (11 tests)
- [x] Test `domain/memory/` -- entity, kinds, scoring types -- `internal/domain/memory/memory_test.go` (9 tests)
- [x] Test `domain/skill/` -- creation, validation, parsing -- `internal/domain/skill/skill_test.go` (6 tests)

#### A1: Stack Health Smoke Tests (Priority: HIGH) -- DONE 2026-03-08

> Test file: `tests/integration/smoke_test.go` (build tag: `//go:build smoke`, 6 tests)
> Run: `go test -tags=smoke -count=1 -timeout=300s ./tests/integration/...`

- [x] Smoke test: Go backend `/health` returns 200 with expected fields
- [x] Smoke test: Dev mode enabled when `APP_ENV=development`
- [x] Smoke test: All API routes under `/api/v1/` prefix
- [x] Smoke test: Auth required on protected endpoints (401 without JWT)
- [x] Smoke test: LiteLLM proxy `/health` returns 200
- [x] Smoke test: NATS JetStream connected -- CODEFORGE stream exists

#### A2: Critical Flow Smoke Tests (Priority: HIGH) -- DONE 2026-03-08

> Test file: `tests/integration/flows_test.go` (build tag: `//go:build smoke`, 6 flow tests)
> Skip env vars: `SMOKE_SKIP_LLM`, `SMOKE_SKIP_LITELLM`, `SMOKE_SKIP_NATS`

- [x] Smoke flow: Project CRUD lifecycle (create, get, list, delete, verify 404)
- [x] Smoke flow: Simple conversation (create, send message, poll response, verify cost)
- [x] Smoke flow: Cost tracking (verify cost_usd, tokens_in, tokens_out after conversation)
- [x] Smoke flow: Modes list (GET /api/v1/modes returns non-empty array)
- [x] Smoke flow: Models list (GET /api/v1/llm/available returns model data)
- [x] Smoke flow: Policies (GET /api/v1/policies returns policy presets)

#### A3: CI Integration (Priority: MEDIUM) -- DONE 2026-03-08

> Added to `.github/workflows/ci.yml`

- [x] Add `contract` CI job: Go fixture generation + Python Pydantic validation (every push)
- [x] Add `smoke` CI job: full stack with Postgres+NATS services (staging/main branches only)
- [x] Configure smoke test skip env vars: `SMOKE_SKIP_LLM`, `SMOKE_SKIP_LITELLM`, `SMOKE_SKIP_NATS`
- [x] (2026-03-09) Upload verification matrix as CI artifact after smoke tests
- [x] (2026-03-09) Add CI status badge to README

#### C1: Feature Verification Matrix (Priority: MEDIUM) -- DONE 2026-03-08

> File: `docs/feature-verification-matrix.md`

- [x] Create initial matrix with all 30 features listed
- [x] Define verification criteria per feature: 5 test layers (Go Unit, Py Unit, E2E, Contract, Smoke)
- [x] Mark currently-passing features based on test results (24 partial, 0 blocked)
- [x] Add "Last Verified" date column
- [x] Cross-reference: each feature row links to relevant test files

#### C2: Automated Verification Reporter (Priority: MEDIUM) -- DONE 2026-03-08

> File: `scripts/verify-features.sh`

- [x] Parse Go test output (`go test -json`) and map packages to features
- [x] Parse Python test output (`pytest --json-report`) and map modules to features
- [x] Map contract test results to features
- [x] Generate markdown table output
- [x] Generate JSON summary to `/tmp/verification-summary.json`
- [x] Exit code: 0 if critical features (1-10, 22-23) pass, 1 otherwise

#### C3: CI Verification Gate (Priority: LOW)

> Block merges to main if critical features regress. Partially covered by A3 CI jobs.

- [x] Define critical feature set: features 1-10 + 22-23 (in `scripts/verify-features.sh`)
- [x] (2026-03-09) Add verification gate as CI job (can be set as required check)
- [x] (2026-03-09) Non-critical features (A2A, LSP, Handoff, etc.): warn but don't block
  - Added `warn_non_critical()` to `scripts/verify-features.sh` — emits `::warning::` GitHub Actions annotations
- [x] (2026-03-10) Store historical verification results for trend tracking
  - Added `store_history()` and `show_trend()` functions to `scripts/verify-features.sh`
  - History stored as JSON in `data/verification-history/` with git SHA, branch, timestamp
  - `--trend` flag displays last 20 runs summary + per-feature trend across last 5 runs

#### Project Workflow Redesign (COMPLETED -- 2026-03-09)

> Plan: `docs/plans/2026-03-09-project-workflow-impl-plan.md`
> Design: `docs/plans/2026-03-09-project-workflow-redesign.md`

- [x] Reorder project tabs: Files, Goals, Roadmap, Feature Map, War Room, Sessions, Trajectory, Audit (2026-03-09)
- [x] Add i18n keys for onboarding, empty states, and chat suggestions (2026-03-09)
- [x] Add empty states with navigation links to all 8 tab panels (2026-03-09)
- [x] Proactive agent greeting on first chat open per project (localStorage-gated) (2026-03-09)
- [x] Lint, TypeScript verify, pre-commit pass (2026-03-09)
