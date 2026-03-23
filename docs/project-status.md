# CodeForge -- Project Status

> Last update: 2026-03-22
> For granular task tracking, see [todo.md](todo.md).
> For phase implementation details, see git history.

### Phase 0: Project Setup (COMPLETED)

Market research (20+ tools analyzed), architecture decisions (Three-Layer Hybrid, Hexagonal, Provider Registry), devcontainer setup (Go 1.25, Python 3.12, Node.js 22), linting for all three languages, documentation structure, framework analysis (LangGraph, CrewAI, AutoGen, MetaGPT), protocol analysis (MCP, LSP, A2A, AG-UI, OTEL), library decisions (chi, coder/websocket, SolidJS, Tailwind), YAML config format, autonomy spectrum (5 levels), agent modes system designed.

### Phase 1: Foundation (COMPLETED)

Docker Compose infrastructure (PostgreSQL, NATS, LiteLLM), Go Core with domain entities, ports, registries, WebSocket hub, NATS adapter, REST API (9 endpoints), Python Workers with NATS consumer and LiteLLM client, SolidJS frontend with Dashboard page and CRUD, GitHub Actions CI.

### Phase 2: MVP Features (COMPLETED)

Git local provider (clone, status, pull, branches, checkout), agent lifecycle with Aider backend, WebSocket live agent output, LLM provider management via LiteLLM admin API, frontend project detail page with git operations and agent monitor.

### Phase 3: Reliability, Performance & Agent Foundation (COMPLETED)

Hierarchical config (defaults < YAML < ENV < CLI), structured JSON logging (Go slog + Python structlog), async logging with buffered channels, circuit breaker, graceful 4-phase shutdown, idempotency keys, optimistic locking, dead letter queue, event sourcing for agent trajectory, tiered cache (ristretto L1 + NATS KV L2), rate limiting, DB pool tuning, worker pools.

### Phase 4: Agent Execution Engine (COMPLETED)

Policy layer with first-match-wins evaluation (4 built-in presets), YAML-configurable custom policies, runtime API with step-by-step execution protocol (NATS Go<->Python), Docker sandbox execution, stall detection (FNV-64a hash ring), quality gate enforcement, 5 delivery modes (none/patch/commit/branch/PR), shadow Git checkpoints, resource limits, secrets vault with SIGHUP reload, multi-tenancy preparation (tenant_id on all tables).

### Phase 5: Multi-Agent Orchestration (COMPLETED)

DAG scheduling with 4 protocols (sequential, parallel, ping-pong, consensus), Meta-Agent with LLM-based feature decomposition, Agent Teams with role-based composition and pool management, Context Optimizer with token budget packing and shared team context, Modes System (24 built-in agent specialization modes).

### Phase 6: Code-RAG (COMPLETED)

tree-sitter-based Repo Map (16+ languages, PageRank file ranking), Hybrid Retrieval (BM25S + semantic embeddings via LiteLLM, RRF fusion), Retrieval Sub-Agent with LLM query expansion and parallel search, GraphRAG with PostgreSQL adjacency-list graph (BFS with hop-decay scoring).

### Phase 7: Cost & Token Transparency (COMPLETED)

Real cost extraction from LiteLLM responses, fallback pricing table, token persistence on runs, cost aggregation API (5 endpoints), WebSocket budget alerts, frontend cost dashboard with project breakdown and daily bars.

### Phase 8: Roadmap Foundation, Event Trajectory, Docker Production (COMPLETED)

Roadmap/Feature-Map domain model (Roadmap, Milestone, Feature), spec provider and PM provider port interfaces, 12 roadmap REST endpoints, trajectory API with cursor pagination, Docker production images (Go multi-stage, Python slim, nginx frontend), docker-compose.prod.yml, GitHub Actions Docker build CI.

### Phase 9A-9E: Advanced Integrations (COMPLETED)

**9A:** OpenSpec, Markdown, GitHub Issues adapters, enhanced AutoDetect, spec/PM import.
**9B:** SVN provider, Gitea/Forgejo PM adapter, VCS webhooks (GitHub + GitLab), bidirectional PM sync.
**9C:** PM webhook processing (GitHub/GitLab/Plane), Slack + Discord notification adapters.
**9D:** OpenTelemetry (TracerProvider + MeterProvider), A2A protocol stub, AG-UI event protocol, blue-green deployment infrastructure.
**9E:** Plane.so PM adapter (full CRUD), full auto-detection engine (three-tier), Feature-Map visual editor (Kanban drag-and-drop).

### Phase 10: Frontend Foundations (COMPLETED)

JWT authentication (HS256, access + refresh tokens), RBAC middleware (admin/editor/viewer), API key management, signal-based i18n (480+ keys, EN + DE), CSS design tokens with dark/light mode, command palette (Ctrl+K), toast notification system, WCAG 2.2 AA conformance with axe-core E2E audits, error boundaries and offline detection.

### Phase 11: GUI Enhancements (COMPLETED)

Tab-based ProjectDetailPage, settings page, mode selection UI, step-progress indicators, global activity stream, team management UI, trajectory replay inspector, diff-review code preview, split-screen feature planning, multi-terminal agent view, vector search simulator, architecture graph visualization, agent network visualization.

### Post-Phase 11: Security Hardening (COMPLETED)

18 audit findings fixed (5 P0, 8 P1, 5 P2): prompt injection defense, secret redaction, audit trail, fail-closed quality gates, post-execution budget enforcement, JWT standard claims + revocation, API key scopes, account lockout, password complexity, delivery error propagation.

### Phase 12A-12K: Architecture Evolution (COMPLETED)

**12A:** Mode system extensions (DeniedTools, DeniedActions, RequiredArtifact, modular prompt templates).
**12B:** LLM routing via LiteLLM tag-based scenario routing (6 scenarios).
**12C:** Role evaluation framework (FakeLLM harness, 9-role matrix, 15 scenario fixtures).
**12D:** RAG shared scope system (cross-project retrieval, incremental indexing with SHA-256 delta).
**12E:** Artifact-gated pipelines (6 artifact types with structural validators).
**12F:** Pipeline templates (3 built-in: standard-dev, security-audit, review-only).
**12G:** Project workspace management (tenant-isolated paths, adopt, health endpoint).
**12H:** Per-tool token tracking (cost-by-tool aggregation).
**12I:** Periodic reviews and audits (commit-count/pre-merge/cron triggers, pipeline integration).
**12J:** Project creation wizard with stack detection (18 manifest patterns, 25+ framework rules).
**12K:** Knowledge bases (8 built-in catalog entries, scope integration, retrieval pipeline).

### OWASP Audit Remediation (COMPLETED)

Two rounds of OWASP Top 10:2025 + WSTG v4.2 remediation (50+ findings across P0-P3): hardcoded credentials removed, Docker production hardening, tenant isolation in all store queries, request body size limits, path traversal prevention, RBAC on tenant routes, account lockout, WebSocket origin validation, SBOM generation, Content-Security-Policy headers.

### Phase 13: UI/UX Improvements & Chat Interface (COMPLETED)

Foundation fixes, CRUD completeness, settings and account management, spec/roadmap detection fix, chat interface with orchestrator conversation, automatic orchestration, AG-UI protocol integration.

### Phase 14: UX Simplification (COMPLETED)

New side-by-side project layout, simplified project creation with branch selection, roadmap structured parsing with drag-to-reorder, bidirectional sync (UI changes -> repo files), chat enhancements (streaming, Markdown, tool calls).

### Phase 15: Protocol Integrations -- MCP + LSP (COMPLETED)

MCP client in Python Workers (McpWorkbench with BM25 tool recommendation), MCP server in Go Core (mcp-go SDK, 4 tools, 2 resources), MCP server registry with PostgreSQL persistence and frontend UI, tool routing with `mcp:server:tool` policy integration, LSP code intelligence with per-language server lifecycle.

### Phase 16: Frontend Design System Rework (COMPLETED)

25 CSS design tokens, 11 primitive components, 8 composite components, 4 layout components, full page migration (42 files), WCAG contrast fix, CSS reduced 35%.

### Phase 17: Interactive Agent Loop (COMPLETED)

The core agentic loop: LLM tool-calling support, 7 built-in tools (Read, Write, Edit, Bash, Search, Glob, ListDir), MCP tool merge, NATS conversation protocol, AgentLoopExecutor with multi-turn tool-use, ConversationHistoryManager with token budget, HITL approval via WebSocket, AG-UI streaming events, ChatPanel with tool call display.

### Phase 18: Live E2E Testing & Blockers (COMPLETED)

NATS stream subjects bug fix, system prompt self-correction, model auto-discovery (LiteLLM + Ollama), runtime conversation policy fix, live testing with real LLM calls (file read, code change, bug analysis, multi-file), knowledge base system fixes.

### Phase 19: Frontend UX Refinements (COMPLETED)

Resizable roadmap/chat split, collapsible roadmap panel, chat auto-scroll, UI cleanup (removed mode/backend selectors from project settings), expanded mode prompts with composable prompt system and editor, MCP Streamable HTTP transport.

### Phase 20: Benchmark Mode (COMPLETED)

DeepEval integration (correctness, faithfulness, relevancy, tool correctness metrics), OpenTelemetry tracing (replacing AgentNeo), GEMMAS collaboration metrics (IDS, UPR), Go Core benchmark API (7 endpoints, migration 041), frontend benchmark dashboard, auto-evaluation hook.

### Phase 21: Intelligent Agent Orchestration (COMPLETED)

Confidence-based moderator router with structured output, typed agent module schemas (Pydantic per step type), SVG-based agent flow DAG visualization, moderator agent mode with debate protocol.

### Phase 22: Planned Pattern Implementation (COMPLETED)

All 8 patterns from CLAUDE.md implemented: RouterLLM scenario wiring, GitHub Copilot token exchange, composite memory scoring, experience pool (@exp_cache), HandoffMessage pattern, Microagents (YAML+Markdown triggers), Skills system (BM25-recommended snippets), Human Feedback Provider Protocol (Slack + Email adapters).

### Phase 23: Security & Identity Patterns (COMPLETED)

**23A:** Trust annotations (4 levels: untrusted/partial/verified/full) auto-stamped on NATS payloads.
**23B:** Message quarantine with risk scoring, admin review hold, evaluate/approve/reject.
**23C:** Persistent agent identity (fingerprint, stats accumulation, inbox, active work visibility).
**23D:** War Room -- live multi-agent collaboration view with swim lanes and handoff arrows.

### Phase 24: Active Work Visibility (COMPLETED)

Parallel task deduplication for agent claim/release lifecycle.

### Phase 25: Frontend Form Dropdowns (COMPLETED)

Dynamic dropdown population for agent, policy, and mode selectors.

### Phase 26: Benchmark System Redesign (COMPLETED)

Provider interface pattern, evaluator plugins (LLMJudge, FunctionalTest, SPARC), 3 runner types (simple/tool-use/agent), 8 external providers (HumanEval, MBPP, BigCodeBench, CRUXEval, LiveCodeBench, SWE-bench, SPARCBench, Aider Polyglot), multi-compare with radar chart, leaderboard, cost analysis, suites CRUD, NATS bridge, WebSocket live updates, 132 E2E tests.

### Phase 27: A2A Protocol Integration (COMPLETED)

Full A2A v0.3.0 implementation via a2a-go SDK. CodeForge as both A2A server (inbound tasks) and client (outbound federation). AgentCard builder, auth middleware, task lifecycle, remote agent registry, `a2a://` handoff routing prefix.

### Phase 28: R2E-Gym / EntroPO Integration (COMPLETED)

Hybrid verification pipeline (filter->rank), trajectory verifier evaluator (5-dimension LLM scoring), multi-rollout test-time scaling (best-of-N), diversity-aware MAB routing (entropy-enhanced UCB1), DPO/EntroPO trajectory export (JSONL chosen/rejected pairs), SWE-GEN synthetic task generation from Git history. Cross-layer bug fixes for DB fields, NATS wiring, and cost population.

### Phase 29: Hybrid Intelligent Model Routing (COMPLETED)

Three-layer routing cascade: ComplexityAnalyzer (rule-based, <1ms) -> MABModelSelector (UCB1 learning) -> LLMMetaRouter (cold-start fallback). Task-type complexity boost, model auto-discovery from LiteLLM (cached 60s), LiteLLM config simplified to 13 provider-level wildcard entries. Adaptive retry with exponential backoff, per-provider rate-limit tracking.

### Phase 30: Goal Discovery & Adaptive Retry (COMPLETED)

**Goal Discovery:** Auto-detection of project goals from workspace files (GSD, agent instructions, project docs), priority-based injection into agent system prompts, auto-detect on project setup, frontend GoalsPanel.
**Adaptive Retry:** LLMClientConfig with env-var-driven retry/timeout, per-provider rate-limit tracking from response headers, HybridRouter skips exhausted providers.

### Unified LLM Path & Global Run Tracking (COMPLETED)

Simple chat path unified with agentic path through NATS dispatch, ConversationRunProvider for global run state tracking across page navigation, sidebar run indicator, ChatPanel seamless resume.

### OTEL Tracing Rewrite (COMPLETED)

AgentNeo replaced with OpenTelemetry backend (OTLP gRPC exporter), 6 instrumented service methods, 3 conversation spans, run spans in sync.Map, all metrics nil-guarded.

### Test Suites (COMPLETED)

**Browser E2E:** 82 Playwright tests (health, navigation, projects, costs, models, a11y).
**LLM E2E:** 95 API-level Playwright tests across 11 spec files (prerequisites, models, conversations, streaming, multi-provider, routing, costs, MCP, benchmarks).
**Benchmark E2E:** 132 browser Playwright tests across 13 spec files.
**Backend E2E:** 88 pass / 0 fail / 3 skip (97% pass rate) across all 4 pillars with real LLM calls.

### Mobile-Responsive Frontend (COMPLETED)

Full mobile + tablet responsiveness for the CodeForge frontend (320px+). Bottom-up approach: `useBreakpoint` singleton hook with `matchMedia` signals, CSS foundation (safe-area insets, `@media (pointer: coarse)` touch targets, scrollbar-none), `viewport-fit=cover`. Primitives overhauled (Button min-heights 36-48px, NavLink 44px touch targets). Composites responsive (Modal, Table, Card, PageLayout). 3-state sidebar (hidden+Portal overlay on mobile, collapsed on tablet, expanded on desktop) with hamburger menu. All page grids responsive (1->2->4 columns). ProjectDetailPage redesigned with mobile tab-switch (bottom bar "Panels"/"Chat"), scrollable sub-tabs. ChatPanel responsive bubbles (90%/75%). FilePanel mobile file tree drawer overlay. 21 files changed (1 new, 20 modified).

### Codebase Optimization -- Full Overhaul (COMPLETED)

Systematic cleanup of code duplicates, stubs, hardcoded constants, and code smells across all 3 language layers. 25 tasks executed via subagent-driven development.

**Go (13 tasks):** Deleted duplicate `internal/crypto/crypto/aes.go`. Added generic helpers (`scanRows[T]`, `writeJSONList[T]`, `queryParamInt`) and migrated 27 store files + ~14 handler files. Removed duplicate `nilIfEmpty()`. Externalized server timeouts and stale-work thresholds to config struct with yaml/env tags. Net ~350 lines removed from store boilerplate alone.

**Python (8 tasks):** Shared `coerce_none_to_list` Pydantic validator. `@catch_os_error` decorator for file tools. `_extract_cost()` in LLM client. `BaseBenchmarkRunner` ABC for 3 runner types. `RoutingConfig` dataclass with configurable weights/thresholds. OpenHands timeouts externalized to env vars. Consumer `_handle_request()` generic NATS handler (10 migrated, 6 skipped for complexity). Consumer backoff constants externalized.

**TypeScript (4 tasks):** `cx()` class-name utility, `getErrorMessage()` error helper, `StatCard`/`ResourceView`/`GridLayout` shared components, `useFocusTrap`/`useFormState`/`useAsyncAction` hooks, `CHART_COLORS`/`RADAR_DEFAULTS` design constants. Adopted across 10+ pages.

### E2E Findings Fix — F1-F5 (COMPLETED)

F4: Workspace path resolution (`filepath.Abs` in `NewProjectService`). F3: Routing fallback (`record_error` + `classify_error_type`). F2: Feature description textarea in `FeatureCardForm`. F1: Create File modal in `FilePanel`. F5: Playwright Docker healthcheck + session recovery docs.

### Dashboard Polish (COMPLETED)

KPI strip with 7 stat cards (cost, runs, success rate, agents, avg cost, tokens, error rate) with trend deltas and inverted-delta logic. HealthDot traffic-light indicator per project (weighted composite: success_rate 0.30, error_rate 0.25, activity_freshness 0.20, task_velocity 0.15, cost_stability 0.10) with hover tooltip showing factor breakdown bars. ChartsPanel tabbed container with 5 Unovis charts (CostTrend area, RunOutcomes donut, AgentPerformance grouped bars, ModelUsage pie, CostByProject stacked bars) and 7d/30d period toggle. ActivityTimeline WebSocket-fed with 5-tier priority system, auto-sorted, max 100 events. ProjectCard enhanced with HealthDot, stats row, compact footer. CreateProjectModal extracted from inline form. Full Go backend: dashboard service, store queries, HTTP handlers (7 endpoints), domain types. Unovis CSS variable integration for theme support. i18n keys for EN + DE.

### Benchmark External Providers, Auto-Routing & Prompt Optimization (COMPLETED)

Suite-based unified benchmark architecture: all benchmark sources (local datasets + 8 external providers) unified under `benchmark_suites` DB entity with `provider_name` + JSONB config. Universal task filter (`apply_task_filters()`) with difficulty, shuffle, seed, max_tasks, task_percentage. Full-auto model routing (`model: "auto"`) via `_RoutingLLMWrapper` that transparently intercepts LLM calls through the HybridRouter, with per-result routing metadata (selected_model, routing_reason, fallback_chain, fallback_count, provider_errors) and aggregated routing report (model distribution, fallback events, provider availability). ModelFamily utility classifying model names into provider families. Per-mode model-family prompt adaptations via `ModelAdaptations` map on Mode struct. Prompt optimization with LLM-as-Critic failure analysis (SICA+SCOPE+MIPROv2 hybrid) producing tactical fixes and strategic principles. Frontend: suite dropdown with optgroup Local/External replacing dataset dropdown, TaskSettings component with universal + provider-specific settings, auto-model checkbox, RoutingReport with model distribution bar and fallback timeline, PromptOptimizationPanel with analyze/accept/reject. Go: migration 068 (5 routing columns), suite seeding (11 defaults), `POST /runs/{id}/analyze` endpoint. Python: task filter (9 tests), prompt optimizer (9 tests), provider config params on all 8 external providers. i18n EN + DE (27+ keys).

### Project Workflow Redesign (COMPLETED)

UX improvements to project detail page workflow. Tab reorder to match natural project lifecycle: Files, Goals, Roadmap, Feature Map, War Room, Sessions, Trajectory, Audit (was Roadmap-first). Empty states with navigation action links added to all 8 tab panels -- each guides the user to the logical next step (e.g., Goals empty state links to Chat, Roadmap links to Goals). Proactive agent greeting on first chat open per project, stored in localStorage to fire only once. i18n keys for all new UI strings. 9 files modified across 4 commits.

### Chat Enhancements (COMPLETED)

10 features transforming the chat into a full-featured development workspace. HITL permission UI with approve/deny cards and countdown timer, `supervised-ask-all` policy preset, autonomy-to-preset auto-mapping, "Allow Always" button with persistent policy rule creation (`POST /policies/allow-always` clones preset to custom profile, prepends allow rule, idempotent). Inline diff review (DiffPreview component). Action buttons (copy, retry, apply, view diff). Per-message cost tracking (MessageBadge + CostBreakdown). Smart references with @/#// autocomplete popover and frequency tracker. Slash commands (/compact, /rewind, /clear, /help, /mode, /model) with rewind timeline picker. Conversation full-text search (PostgreSQL GIN index, ts_rank, SearchPage tabs). Notification center (browser push, Web Audio sounds, tab badge, AG-UI event wiring, notificationStore). Real-time channels with threads (3 tables, 9 endpoints, WebSocket events, sidebar ChannelList, ChannelView, ThreadPanel). Feature spec: `docs/features/05-chat-enhancements.md`.

### Subscription Provider Integration (COMPLETED)

OAuth device flow integration for subscription-based LLM providers (Claude Code Max, GitHub Copilot). Go adapter layer (`internal/adapter/auth/`) with `SubscriptionProvider` interface and implementations for Anthropic (device code -> OAuth token -> `create_api_key` endpoint) and GitHub (device code -> PAT). Atomic `.env` writer (`internal/service/envwriter.go`) with temp-file-and-rename. Subscription orchestration service with background goroutine polling, context cancellation, and configurable intervals. 4 HTTP endpoints (`/api/v1/auth/providers/*`) for list/connect/status/disconnect. Python routing updates: `github_copilot` added to `PROVIDER_KEY_MAP`, prepended in all 4 complexity tiers in both `router.py` and `meta_router.py`. LiteLLM config updated with Copilot extra_headers. Frontend SettingsPage extended with Subscription Providers section showing device code + "Open in browser" link, connect/disconnect buttons, and available models per provider. 87 tests total (22 auth adapter, 9 envwriter, 8 subscription service, 48 Python routing).

### Benchmark Live Feed (COMPLETED)

Real-time structured event feed for running benchmark runs. Go `TrajectoryEventPayload` enriched with cost, tokens, input, output, step fields. Frontend `BenchmarkLiveFeed.tsx` with `@tanstack/solid-virtual` virtualized auto-scrolling, feature accordions (when 2+ features), progress header with bar/cost/elapsed timer, event row rendering by type (tool_called, step_done, finished). Integrated into `BenchmarkPage.tsx` for selected running runs. TypeScript `LiveFeedEvent` + `BenchmarkLiveProgress` types.

### Benchmark Live Feed — State Persistence & Density Improvements (COMPLETED)

Live feed state lifted from child `BenchmarkLiveFeed` component to parent `BenchmarkPage` as `Map<runId, LiveFeedState>` signal — closing/reopening or page-reloading a running benchmark's info card no longer loses event history. API hydration on mount fetches trajectory + results for running runs. `BenchmarkLiveFeed` converted to presentational component receiving `LiveFeedState` props. Pure functions extracted to `liveFeedState.ts` with 18 unit tests. Information density improvements: inline stats line (avg score, tokens, tool calls, $/task), mini score bars on feature rows with green/yellow/red color coding, ETA display, and indeterminate progress bar for unknown total_tasks.

### Frontend UI Bug Fixes & i18n (COMPLETED)

10 UI issues fixed across 8 files. (1) **Broken "Go to Chat" navigation** — `onNavigate("chat")` silently did nothing because `"chat"` wasn't a valid `LeftTab`; unified `handleNavigate()` now switches mobile view to chat. (2) **Dead RunPanel code** — `run.toolcall` WS stub and unused method-attachment hack removed; tool calls render via AG-UI in ChatPanel. (3) **`window.prompt()` for folders** — replaced with custom Modal dialog matching Create/Rename/Delete UX. (4) **Monaco theme sync** — editor reactively follows dark/light toggle. (5) **File panel icon alignment** — SVG polyline points centered. (6) **i18n coverage** — ~40 hardcoded strings replaced with `t()` calls across FilePanel, FileContextMenu, GoalProposalCard, KnowledgeBasesPage; 28 new keys in en.ts + de.ts.

### Benchmark Validation E2E Bug Fixes (COMPLETED)

5 backend bugs discovered by the 22-test benchmark validation E2E suite (`frontend/e2e/benchmark-validation/`) and fixed. Full pipeline: Go Core -> NATS JetStream -> Python Worker -> LiteLLM -> LM Studio -> Evaluators -> DB -> API.

**Bug 1 (Medium) — Score Key Mismatch:** Evaluator dimension names (`correctness`, `sparc_*`) didn't match requested metric names (`llm_judge`, `sparc`). Fixed with `_aggregate_metric_scores()` + `_DIMENSION_TO_METRIC` mapping (17 entries) in `workers/codeforge/consumer/_benchmark.py`. 16 tests.

**Bug 2 (High) — Stuck "running" Runs:** Two-tier fix. (2A) `StartRun()` validates dataset path and fails fast when no suite fallback exists. Added `ErrorMessage` field to `Run` struct with migration `072`. (2B) Watchdog goroutine in `internal/service/benchmark.go` scans every 5 min for runs stuck >15 min. 5 tests.

**Bug 3 (Medium) — Invalid Model Silently Succeeds:** LiteLLM fell back to default model. Added `_validate_model_exists()` checking LiteLLM `/v1/models` before execution. 6 tests.

**Bug 4 (Low) — `model=auto` Without Routing:** `_resolve_effective_llm()` now raises `ValueError` instead of passing `"auto"` literally to LiteLLM. 2 tests.

**Bug 5 (Low) — LLM Judge Context Overflow:** Added `compress_for_context()` head+tail truncation utility (`workers/codeforge/evaluation/evaluators/prompt_compressor.py`). LLM Judge and Trajectory Verifier compress inputs to conservative budgets (4K/2K chars). Error fallback distinguishes `context_overflow` from `evaluation_failed`. 18 tests.

18 files changed, 2478 insertions, 109 deletions. Findings: `frontend/e2e/benchmark-validation/FINDINGS.md`. Plan: `docs/plans/2026-03-11-benchmark-findings-fixes-plan.md`.

### Benchmark Validation Round 2 — Bugs 6-10 + External Suite Fixes (COMPLETED)

5 more bugs found and fixed during the 33-run E2E test plan (Round 2), plus 3 external suite HuggingFace API fixes.

**Bug 6 (High) — Agent Provider Wrong Kwarg:** `datasets_dir=` → `dataset_path=` in consumer. **Bug 7 (High) — Watchdog Timeout Too Short:** 15min → 2h default, configurable via `BENCHMARK_WATCHDOG_TIMEOUT`. **Bug 8 (High) — RolloutOutcome Missing eval_score:** Added field + pass-through in `multi_rollout.py`. **Bug 9 (High) — Wrong Attribute in _convert_rollout_outcome:** `outcome.execution.*` → `outcome.result.*`. **Bug 10 (Medium) — Hybrid Pipeline Passed as Regular:** Separated pipeline construction with dedicated `hybrid_pipeline` parameter.

**Issue D — External Suite HF API Failures (3 fixes):** BigCodeBench config/split swap, CRUXEval dataset moved to `cruxeval-org/cruxeval` + HF_TOKEN auth, LiveCodeBench correct dataset + adaptive page size fallback (100→10→1) with timeout handling and broken-row skipping in `cache.py:download_hf_dataset()`. Early NATS ack prevents stale message redelivery.

**E2E Results:** Phase 3b external suites 4/5 PASS (LiveCodeBench partial — HF server 502/504 for large rows). Phase 5 API comparison/analysis 12/12 PASS. Phase 6 error scenarios 2/5 PASS (3 known issues: invalid model validation regression, HTTP 500 vs 400, unknown evaluator silently ignored).

### Allow Always Policy Persistence (COMPLETED)

"Allow Always" button in HITL PermissionRequestCard now persists permanent policy rules. `POST /api/v1/policies/allow-always` endpoint: resolves project's effective profile, clones immutable presets to `{preset}-custom-{projectId}`, constructs `PermissionRule` from tool name + command glob pattern (e.g., `Bash/git*`), prepends to profile (first-match-wins), updates project association. Idempotent via `HasRuleForSpecifier` duplicate detection. Future identical tool calls auto-approve without HITL prompt. 12 files changed, 26 new tests across Go domain/service/HTTP layers + frontend wiring.

### Phase 31: Contract-First Review/Refactor (COMPLETED)

Contract-first review and refactoring pipeline: boundary domain model (ProjectBoundaryConfig), 2 new agent modes (boundary-analyzer, contract-reviewer), review-refactor pipeline template (4-step sequential: boundary analysis -> contract review -> intra-layer review -> refactoring), DiffImpactScorer with 3-tier threshold HITL (auto-apply/notify/approve), ReviewTriggerService with cascade dedup (pipeline-completion, branch-merge, manual), phase-aware context budget per pipeline step, waiting_approval step status with approve/reject flow, HTTP endpoints for boundaries CRUD and run approval, Python NATS consumer for review triggers, SolidJS RefactorApproval overlay and BoundariesPanel.

### Sidebar Restructure (COMPLETED)

Section grouping, page merges, top bar navigation improvements.

### Loading Animation System (COMPLETED)

Complete loading animation system for the SolidJS frontend. 5 primitives (Skeleton, TypingIndicator, StreamingCursor, ProgressBar, PacmanSpinner), 4 composites (SkeletonText, SkeletonCard, SkeletonTable, SkeletonChat), 7 CSS keyframes (cf-shimmer, cf-blink, cf-bounce-dot, cf-progress-slide, cf-pacman-chomp, cf-dot-orbit, cf-fade-in), skeleton design tokens for light/dark themes. ChatPanel integration: TypingIndicator replaces animate-pulse thinking indicator, StreamingCursor replaces static "Streaming..." label. ResourceGuard gains optional `skeleton` prop for custom loading states. No new dependencies.

### Benchmark Metric Validation & Detail Card Fix (COMPLETED)

Go `ValidMetrics` allowlist was missing 3 of 5 frontend-offered metrics (`tool_correctness`, `answer_relevancy`, `contextual_precision`), causing benchmark runs with all metrics to fail HTTP 400. Added missing entries. Fixed SolidJS event delegation bug where clicking task rows in the benchmark detail table collapsed the parent card — parent `onClick` now guards against clicks originating from table/button/a elements. Verified with Playwright MCP against `lm_studio/qwen/qwen3-30b-a3b`: detail card shows summary scores, task results table, and expandable rows with Actual Output and Evaluator Scores.

### Phase 32: Visual Design Canvas (COMPLETED)

SVG-based design canvas with 7 tools (select, rect, ellipse, freehand, text, annotate, image), triple-output export pipeline (PNG via offscreen canvas, ASCII art character grid, structured JSON), multimodal message pipeline (Frontend MessageImage -> Go JSONB images column -> NATS MessageImagePayload -> Python content-array -> LiteLLM), smart output selection based on model vision capability (vision: PNG+JSON, text-only: ASCII+JSON, basic: JSON only), canvas-to-chat integration with buildCanvasPrompt() utility, supports_vision model detection via LiteLLM metadata + name-pattern fallback. 46 files, +6370 lines, 159 frontend tests, 32 Python multimodal tests, full Go coverage.

### E2E Test Expansion & Verification Tooling (COMPLETED)

Routing fallback E2E test (`workers/tests/test_routing_fallback_e2e.py`, 6 tests verifying full billing error -> classify -> mark exhausted -> model switch chain). File CRUD Playwright E2E (`frontend/e2e/file-crud.spec.ts`, 4 tests). Feature description Playwright E2E (`frontend/e2e/feature-description.spec.ts`, 4 tests). Verification trend tracking in `scripts/verify-features.sh` (`--trend` flag, JSON history with git SHA/branch/timestamp in `data/verification-history/`). Agent-eval benchmark run with `mistral/mistral-large-latest` (0/300 -- model could not produce code, infrastructure verified working).

### Phase 5 Ecosystem: Benchmark Providers + RLVR Export (COMPLETED)

**C3: DPAI Arena + Terminal-Bench Providers.** Two new benchmark providers: `DPAIArenaProvider` (BenchmarkType.SIMPLE, HuggingFace `DPAI/arena` dataset, functional_tests + llm_judge) and `TerminalBenchProvider` (BenchmarkType.AGENT, filesystem state verification). New `FilesystemStateEvaluator` compares expected vs actual filesystem state (file existence, content match, expected-missing checks). Go `defaultSuites` updated. 56 Python tests.

**C4: RLVR Training Pipeline Export.** New export pipeline for Reinforcement Learning from Verifiable Rewards. Python `RLVRExporter` with `compute_rlvr_reward()` (weighted average, functional_test 2x weight, clamped [0,1]) and `format_rlvr_entry()`. Go `ExportRLVRDataset()` service method + `ComputeRLVRReward()`. HTTP endpoint `GET /api/v1/benchmarks/runs/{id}/export/rlvr` supporting JSONL (default) and JSON formats. 36 tests (19 Python + 13 Go service + 4 Go handler).

### Quality & Performance — Phase 1 Quick Wins (COMPLETED)

**A1: Stall Detection + Escape.** `StallDetector` class with deque-based sliding window and FNV-64a hash ring for detecting repeated tool calls. Integrated into `AgentLoopExecutor.run()`. Publishes `trajectory.stall_detected` events. 22 tests.

**B3: Adaptive Context Budget.** `ClassifyComplexity()` with 7 heuristics + task-type boost. `ComplexityBudget()` composes with PhaseAware + Adaptive decay. 17 tests.

**C2: Confidence-Based Early Stopping.** `EarlyStopChecker` for multi-rollout runs with quorum + threshold. Integrated into `MultiRolloutRunner.run()`. Configurable via `CODEFORGE_EARLY_STOP_THRESHOLD` / `CODEFORGE_EARLY_STOP_QUORUM`. 15 tests.

### Quality & Performance — Phase 2 Core Quality (COMPLETED)

**A3: Plan/Act Mode Toggle.** Two-phase agent execution: plan phase (read-only tools: read_file, search_files, glob_files, list_directory) then act phase (all tools). `PlanActController` in Python with auto-transition after configurable iterations (`CODEFORGE_PLAN_ACT_MAX_ITERATIONS`, default 10). Enabled automatically for modes with autonomy >= 4 via `plan_act_enabled` NATS field. Routing tag `"plan"` for LLM scenario routing in plan phase. 37 tests (29 Python + 8 Go).

**B2: Semantic Deduplication of Context Candidates.** SimHash-based near-duplicate detection for context candidates from multiple retrieval sources (BM25, semantic search, GraphRAG). `simhash64()` (64-bit fingerprint via trigram shingles + FNV-64a), `hammingDistance()` (XOR + popcount), `deduplicateCandidates()` (greedy dedup, threshold 3 bits, keeps highest-priority). Integrated into `assembleAndPack()` before token budget packing. 26 tests.

### Database Schema Audit & Remediation (COMPLETED)

**Full audit** of 78 migrations, 60+ tables, 40+ store files using automated `db_schema_audit.py` + 4 parallel deep-analysis agents. Pre-remediation score: 48/100 (Grade D). Post-remediation: Critical 3->1, High 42->25, Total 120->104.

**7 remediation migrations (079-085):** tenant_id columns on channel_messages/channel_members, 7 missing FK indexes, 5 GIN indexes on JSONB columns, 6 redundant index drops, tenant-prefixed composite index fix, ON DELETE CASCADE on channels, REAL->NUMERIC(12,6) for precision columns.

**14 tenant isolation query fixes** across 5 store files: store_channel.go (3 queries), store_conversation.go (4 queries), store_a2a.go (4 queries), store_vcsaccount.go (2 queries), store_mcp.go (1 query). All cross-tenant data access paths closed using JOIN, subquery, and direct WHERE patterns with `tenantFromCtx(ctx)`.

**Connection pool tuning:** MaxConns 15->50, MinConns 2->10, MaxConnLifetime 1h->30m, MaxConnIdleTime 10m->5m, HealthCheck 1m->30s. Audit report: `docs/audits/2026-03-18-schema-audit.md`.

### UX/UI Audit Implementation (COMPLETED)

17 atomic tasks across 3 layers. **Layer 1 Quick Wins (6):** Anvil SVG favicon, per-page document titles (17 pages), Prompts Preview button variant fix, debounced WebSocket reconnect banner (2s delay + 3s initial suppress), abbreviated KPI labels for mobile, hover effects + click-to-navigate on project cards. **Layer 2 Medium-Term (7):** SVG empty state illustrations (6 pages), page transition fade-in animations (PageTransition component), skeleton loaders on AI Config/Costs/Settings, sticky section navigation on Settings page (9 sections, IntersectionObserver), per-panel ErrorBoundary for graceful degradation, anvil brand mark in sidebar header (CodeForgeLogo), collapsible model cards on AI Config page. **Layer 3 Strategic (4):** Typography system (Outfit display + Source Sans 3 body, self-hosted woff2), micro-interactions (button press, card hover lift, tab animation, KPI count-up, toast slide-in, modal fade+scale), living design system page at `/design-system` (dev-mode only) with `DESIGN-SYSTEM.md`, 3-step onboarding wizard (Connect Code -> Configure AI -> Create Project).

### Frontend Feature Pages & Prompt Evolution (COMPLETED 2026-03-23)

Five features merged on 2026-03-23:

**MicroagentsPage** (`frontend/src/features/microagents/MicroagentsPage.tsx`): UI for managing YAML+Markdown trigger-driven microagents. Lists, creates, and edits microagent definitions backed by the existing microagent domain and service layer.

**QuarantinePage** (`frontend/src/features/quarantine/QuarantinePage.tsx`): Admin review UI for the Phase 23B message quarantine system. Displays quarantined messages with risk scores, allows evaluate/approve/reject actions.

**A2APage** (`frontend/src/features/a2a/A2APage.tsx`): Frontend for the A2A v0.3.0 agent federation system (Phase 27). Shows registered remote agents, task history, and AgentCard details.

**RoutingStatsPage** (`frontend/src/features/routing/RoutingStatsPage.tsx`): Live statistics for the Phase 29 hybrid routing system. Displays model selection distribution, fallback events, provider availability, and MAB UCB1 scores.

**Prompt Evolution** (`frontend/src/features/prompts/EvolutionTab.tsx`, `internal/service/prompt_evolution.go`, `workers/codeforge/consumer/_prompt_evolution.py`, migration 078): LLM-driven prompt improvement pipeline. Go service orchestrates reflect/mutate cycles via NATS; Python worker executes LLM calls for reflection and mutation; frontend EvolutionTab shows evolution history and allows triggering new cycles.
