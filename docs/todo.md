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
- [x] Modes System: 24 built-in agent specialization modes

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
- [x] (2026-03-16) Evaluation improvements: logprob verifier, categorical trajectory scoring, longest/shortest selection strategies

#### Phase 29 -- Hybrid Intelligent Model Routing (COMPLETED)
- [x] Three-layer cascade: ComplexityAnalyzer (<1ms) -> MABModelSelector (UCB1) -> LLMMetaRouter
- [x] Task-type complexity boost, model auto-discovery from LiteLLM, wildcard config
- [x] Adaptive retry with exponential backoff, per-provider rate-limit tracking

#### Phase 30 -- Goal Discovery & Adaptive Retry (COMPLETED)
- [x] Auto-detection of project goals from workspace files, priority-based context injection
- [x] LLMClientConfig with env-var-driven retry/timeout, HybridRouter skips exhausted providers
- [x] Goal system redesign: replaced `manage_goals` HTTP-callback tool with `propose_goal` AG-UI event tool (2026-03-09)
- [x] Rewritten goal_researcher mode with GSD questioning methodology (interview-first)
- [x] GoalProposalCard UI for approve/reject of agent-proposed goals
- [x] Context injection: docs/PROJECT.md, REQUIREMENTS.md, STATE.md passed to goal_researcher agent
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
- Plan: `docs/plans/2026-03-11-benchmark-findings-fixes-plan.md`

#### Benchmark Validation E2E Round 2 — Bugs 6-10 + External Suite Fixes (COMPLETED)
- [x] (2026-03-15) **Bug 6 — Agent Provider Wrong Kwarg (High):** `datasets_dir=` → `dataset_path=` in `_benchmark.py:405`
- [x] (2026-03-15) **Bug 7 — Watchdog Timeout Too Short (High):** 15min → 2h default, configurable via `BENCHMARK_WATCHDOG_TIMEOUT` env var in `cmd/codeforge/main.go`
- [x] (2026-03-15) **Bug 8 — RolloutOutcome Missing eval_score (High):** Added `eval_score` field to `RolloutOutcome` dataclass in `multi_rollout.py`
- [x] (2026-03-15) **Bug 9 — Wrong Attribute Name in _convert_rollout_outcome (High):** `outcome.execution.*` → `outcome.result.*` in `_benchmark.py:518-527`
- [x] (2026-03-15) **Bug 10 — Hybrid Pipeline Passed as Regular Pipeline (Medium):** Separated pipeline construction, added `hybrid_pipeline` parameter
- [x] (2026-03-16) **Issue D — External Suite HF API Failures:** Fixed BigCodeBench (config/split swap), CRUXEval (dataset moved to `cruxeval-org/cruxeval` + HF_TOKEN auth), LiveCodeBench (correct dataset + adaptive page size fallback 100→10→1 with timeout handling and broken-row skipping)
- [x] (2026-03-16) Early NATS ack in benchmark handler to prevent stale message redelivery
- [x] (2026-03-16) Documented `HF_TOKEN` and `BENCHMARK_WATCHDOG_TIMEOUT` env vars in `docs/dev-setup.md`
- Results: Phase 3b external suites 4/5 PASS (LiveCodeBench partial due to HF server limitations), Phase 5 API 12/12 PASS, Phase 6 errors 2/5 PASS
- Findings: `frontend/e2e/benchmark-validation/FINDINGS.md`

#### Benchmark E2E Full Run (2026-03-19) — Findings & Recommendations (OPEN)

> Report: `docs/testing/benchmark-e2e-report.md`
> Full API + Playwright-MCP UI test. 86/90 passed, 4 deferred (queue timing).

##### REC-1: Parallel Benchmark Run Processing with Dependency Awareness (Critical)

> **Problem:** Python worker processes runs sequentially (`consumer/__init__.py:271` blocks on `await handler(msg)`).
> A single agent run (15-30 min) blocks ALL subsequent runs. During E2E test, Phase 4+6 runs waited >30 min.
> **Root cause:** `_message_loop` fetches `batch=1`, awaits handler inline. Go side supports parallelism
> (`MaxAckPending: 100`) but Python serializes everything. Tasks within a run are also sequential
> (`runners/_base.py:run_tasks()` for-loop).

- [ ] REC-1.1: Add `asyncio.Semaphore` to benchmark handler, spawn runs via `asyncio.create_task()` instead of inline `await`
  - File: `workers/codeforge/consumer/_benchmark.py`
  - Config: `BENCHMARK_MAX_PARALLEL` env var (default 3)
  - Constraint: Agent `mount` mode runs sharing the same project workspace MUST NOT run in parallel (file corruption risk). Guard with per-project workspace lock or reject parallel mount runs to same project.
  - Constraint: LLM rate limits are the real parallelism bottleneck — size semaphore based on provider capacity
  - Note: Each run already has its own `RunResult` — no shared mutable state between runs, safe to parallelize
- [ ] REC-1.2: Add structured error handling for concurrent task failures
  - If a spawned task raises an exception, it must still publish `benchmark.run.result` with `status: "failed"` to NATS
  - Use `asyncio.create_task()` with an `add_done_callback` that catches and publishes errors
- [ ] REC-1.3: Update `_message_loop` to support concurrent handlers
  - File: `workers/codeforge/consumer/__init__.py:271`
  - Current: `await handler(msg)` — blocks loop
  - Change: `asyncio.create_task(handler(msg))` — only for benchmark subject, other subjects (conversation, toolcall) remain sequential for ordering guarantees
- [ ] REC-1.4: Add integration test verifying parallel execution
  - Create 3 simple runs with different datasets simultaneously
  - Assert all 3 complete within ~1x single-run duration (not 3x)
  - Assert results are correct and don't interfere

##### REC-2: Trajectory Endpoint Returns 500 for Running Runs (High)

> **Problem:** `GET /runs/{id}/trajectory` returns HTTP 500 when run has no events yet.
> Frontend LiveFeed logs errors + skips hydration for all visible running runs.
> **Location:** `handlers_roadmap.go:444-455`

- [ ] REC-2.1: Return empty result instead of 500 when `LoadTrajectory()` or `TrajectoryStats()` errors
  - File: `internal/adapter/http/handlers_roadmap.go:444-455`
  - Fix: On error, set `page = &eventstore.TrajectoryPage{Events: []event.Event{}}` and `stats = &eventstore.TrajectoryStats{}`
  - Distinguish "run not found" (404) from "no events yet" (200 empty) if needed by checking run existence first

##### REC-3: Training Export Returns Empty Body Instead of Empty Array (Medium)

> **Problem:** JSONL export writes nothing when no pairs exist (for-loop iterates zero times).
> Client gets headers but zero-byte body. JSON format correctly returns `[]`.
> **Location:** `handlers_benchmark.go:336-339`

- [ ] REC-3.1: Add empty-check before JSONL loop, fall back to `[]` JSON response when no pairs exist
  - File: `internal/adapter/http/handlers_benchmark.go:336-339`

##### REC-4: Suite Creation Should Auto-Derive Type from Provider (Low)

> **Problem:** `POST /suites` requires explicit `type` field. Provider already implies type.
> **Location:** `benchmark.go:86` — `r.Type.IsValid()` rejects empty type

- [ ] REC-4.1: Add provider-to-type mapping, auto-derive in `RegisterSuite()` before `Validate()`
  - File: `internal/service/benchmark.go` (service layer), `internal/domain/benchmark/benchmark.go` (mapping)
  - Frontend: auto-populate type field in `SuiteManagement.tsx` on provider selection (nice-to-have)

##### REC-5: Configurable Watchdog Timeout per Suite/Type (Low)

> **Problem:** Single global 2h watchdog. Simple runs stuck 2h before cleanup, agent runs on slow models killed prematurely.
> **Location:** `cmd/codeforge/main.go` — watchdog goroutine, `internal/service/benchmark.go`

- [ ] REC-5.1: Use benchmark type as heuristic timeout (no DB change needed)
  - `simple` → 30 min, `tool_use` → 1h, `agent` → 4h
  - Or: add optional `timeout` field to `Suite` domain model + DB migration

---

#### Benchmark E2E — Remaining Bugs (OPEN)

> Discovered during E2E validation Round 2 (Phase 6 error scenarios).
> Reference: `frontend/e2e/benchmark-validation/FINDINGS.md` → "Known Issues (Not Yet Fixed)"
> These bugs affect input validation and error handling — the happy path works, but malformed
> requests don't fail cleanly.

##### Issue A: Invalid Model Name Silently Succeeds (Regression from Bug 3)

> **Severity:** Medium
> **E2E Test:** Phase 6.2 — FAIL (run completes with score=0 instead of failing)
> **Root cause:** `_validate_model_exists()` does exact match against `/v1/models` list, but
> LiteLLM accepts `provider/model-name` format models (e.g. `nonexistent/model-xyz-404`) and
> silently falls back or passes them through. The validation only catches bare model names that
> aren't in the list — prefixed names bypass it.

- [x] (2026-03-16) A.1: Write failing test — model with `provider/name` format not in LiteLLM models list
  - File: `workers/tests/test_model_validation.py`
  - Test: `_validate_model_exists("nonexistent/model-xyz-404", available_models=["lm_studio/qwen3-30b"])` should raise `ValueError`
  - Test: `_validate_model_exists("openai/gpt-4", available_models=["openai/gpt-4"])` should pass (exact match still works)
  - Run: `cd workers && poetry run pytest tests/test_model_validation.py -v`

- [x] (2026-03-16) A.2: Fix `_validate_model_exists()` — add /model/info fallback for provider-prefixed models
  - File: `workers/codeforge/consumer/_benchmark.py` (line 57-71)
  - Current logic: `if model not in available_models: raise ValueError`
  - Problem: LiteLLM `/v1/models` might return `lm_studio/qwen3-30b-a3b` while user sends `lm_studio/nonexistent-model` — both have `lm_studio/` prefix but only one is valid
  - Fix option A (preferred): Keep exact match but also attempt a LiteLLM model info call (`/model/info` endpoint) to confirm the model is actually routable. If the model doesn't resolve, raise `ValueError`.
  - Fix option B (simpler): Extract provider prefix from model name (`model.split("/")[0]`), check that the prefix matches at least one available model's prefix AND the full model name matches. If no exact match, raise `ValueError` with helpful message listing similar models.
  - Key constraint: Must not break `model=auto` (already guarded by early return)

- [x] (2026-03-16) A.3: Write test for edge cases
  - File: `workers/tests/test_model_validation.py`
  - Test: model with valid prefix but invalid name (`lm_studio/nonexistent`) → fails
  - Test: model that is a substring of a valid model (`lm_studio/qwen3`) → fails (no partial match)
  - Test: empty model list (LiteLLM unreachable) → passes (existing skip behavior)
  - Test: model `auto` → passes (existing early return)

##### Issue B: HTTP 500 Instead of 400 for Invalid Requests

> **Severity:** High
> **E2E Tests:** Phase 6.1 (invalid dataset) — WEAK PASS (HTTP 500, should be 400),
>                Phase 6.3 (missing required field) — FAIL (HTTP 500, should be 400)
> **Root cause:** `CreateRunRequest.Validate()` returns plain `fmt.Errorf()` errors, but
> `writeDomainError()` only maps `domain.ErrValidation`-wrapped errors to HTTP 400. Plain
> errors fall through to the `default` case → HTTP 500.

- [x] (2026-03-16) B.1: Write failing Go test — validation errors should return HTTP 400
  - File: `internal/adapter/http/handlers_test.go`
  - Test: `POST /api/v1/benchmarks/runs` with `{"model": "gpt-4", "metrics": ["llm_judge"]}` (missing dataset AND suite_id) → expect HTTP 400 with `"dataset or suite_id is required"`
  - Test: `POST /api/v1/benchmarks/runs` with `{"dataset": "foo", "metrics": ["llm_judge"]}` (missing model) → expect HTTP 400 with `"model is required"`
  - Test: `POST /api/v1/benchmarks/runs` with `{"dataset": "foo", "model": "gpt-4"}` (missing metrics) → expect HTTP 400 with `"at least one metric is required"`
  - Test: `POST /api/v1/benchmarks/runs` with `{"dataset": "foo", "model": "gpt-4", "metrics": ["llm_judge"], "benchmark_type": "invalid"}` → expect HTTP 400 with `"invalid benchmark type"`
  - Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -run TestCreateBenchmarkRun -v`

- [x] (2026-03-16) B.2: Wrap `Validate()` errors with `domain.ErrValidation`
  - File: `internal/domain/benchmark/benchmark.go` (line 175-190, `Validate()` method)
  - Current: `return fmt.Errorf("model is required")`
  - Fix: `return fmt.Errorf("%w: model is required", domain.ErrValidation)`
  - Apply to ALL 5 error returns in `Validate()`:
    1. `dataset or suite_id is required`
    2. `model is required`
    3. `at least one metric is required`
    4. `invalid benchmark type: %q`
    5. `invalid exec mode: %q`
  - Import: `"github.com/CodeForge/internal/domain"` (or wherever `ErrValidation` is defined)
  - This ensures `writeDomainError()` in `internal/adapter/http/helpers.go:114` matches the `errors.Is(err, domain.ErrValidation)` case → HTTP 400

- [x] (2026-03-16) B.3: Verify `StartRun()` dataset-not-found also returns 400 (not 500)
  - File: `internal/service/benchmark.go` (line 192)
  - Current: `return nil, fmt.Errorf("dataset %q not found: %w", run.Dataset, statErr)`
  - This wraps `statErr` (an `os.PathError`) — `writeDomainError()` won't match it → HTTP 500
  - Fix: `return nil, fmt.Errorf("%w: dataset %q not found", domain.ErrValidation, run.Dataset)`
  - Test: `POST /api/v1/benchmarks/runs` with `{"dataset": "nonexistent-xyz", "model": "gpt-4", "metrics": ["llm_judge"]}` → expect HTTP 400 (not 500)

- [x] (2026-03-16) B.4: Run full test suite to verify no regressions
  - Run: `cd /workspaces/CodeForge && go test ./internal/... -count=1`
  - All existing benchmark tests must still pass

##### Issue C: Unknown Evaluator Names Silently Ignored

> **Severity:** Medium
> **E2E Test:** Phase 6.4 — FAIL (run completes with empty scores instead of failing)
> **Root cause:** `_build_evaluators()` in `workers/codeforge/consumer/_benchmark.py:652-697`
> uses a `logger.warning("unknown evaluator, skipping")` for unrecognized names (line 681),
> then falls through to the "no evaluators" fallback which creates a default LLMJudgeEvaluator.
> So requesting `metrics: ["nonexistent_evaluator"]` silently succeeds with a default evaluator.

- [x] (2026-03-16) C.1: Decide on validation strategy (two options):
  - **Option 1 — Go-side validation (preferred):** Add a `ValidMetrics` set in `internal/domain/benchmark/benchmark.go` containing the 4 valid top-level metrics (`llm_judge`, `functional_test`, `sparc`, `trajectory_verifier`) plus the 5 LLM judge sub-metrics (`correctness`, `faithfulness`, `relevance`, `coherence`, `fluency`). Check each element of `req.Metrics` in `Validate()` — reject with HTTP 400 if unknown.
  - **Option 2 — Python-side validation:** Change `_build_evaluators()` to raise `ValueError` instead of logging a warning for unknown names. The existing error handler would then publish `status=failed` with a descriptive error message.
  - **Recommendation:** Option 1 (Go-side) catches errors earlier and returns proper HTTP 400. Option 2 is a safety net. Implement both.

- [x] (2026-03-16) C.2: Write failing Go test — unknown metric names rejected
  - File: `internal/domain/benchmark/benchmark_test.go`
  - Test: `CreateRunRequest{Metrics: []string{"nonexistent_evaluator"}}` → `Validate()` returns error containing `"unknown metric"` and `"nonexistent_evaluator"`
  - Test: `CreateRunRequest{Metrics: []string{"llm_judge", "functional_test"}}` → `Validate()` returns nil (valid)
  - Test: `CreateRunRequest{Metrics: []string{"llm_judge", "invalid"}}` → `Validate()` returns error (one invalid is enough to reject)
  - Run: `cd /workspaces/CodeForge && go test ./internal/domain/benchmark/ -v`

- [x] (2026-03-16) C.3: Add `ValidMetrics` set and check in `Validate()`
  - File: `internal/domain/benchmark/benchmark.go`
  - Add: `var ValidMetrics = map[string]bool{"llm_judge": true, "functional_test": true, "sparc": true, "trajectory_verifier": true, "correctness": true, "faithfulness": true, "relevance": true, "coherence": true, "fluency": true}`
  - In `Validate()`, after the `len(r.Metrics) == 0` check, add:
    ```go
    for _, m := range r.Metrics {
        if !ValidMetrics[m] {
            return fmt.Errorf("%w: unknown metric %q; valid metrics: llm_judge, functional_test, sparc, trajectory_verifier", domain.ErrValidation, m)
        }
    }
    ```

- [x] (2026-03-16) C.4: Python-side safety net — raise instead of warn for unknown evaluators
  - File: `workers/codeforge/consumer/_benchmark.py` (line 681)
  - Current: `logger.warning("unknown evaluator, skipping", evaluator=name)`
  - Fix: `raise ValueError(f"unknown evaluator/metric: {name!r}. Valid: llm_judge, functional_test, sparc, trajectory_verifier, correctness, faithfulness, relevance, coherence, fluency")`
  - This ensures that even if Go-side validation is bypassed (e.g. direct NATS message), the worker fails cleanly
  - Write test in `workers/tests/test_score_key_normalization.py` or new file:
    `_build_evaluators(["nonexistent_evaluator"], "gpt-4")` → raises `ValueError`

- [x] (2026-03-16) C.5: Remove the "no evaluators" fallback default
  - File: `workers/codeforge/consumer/_benchmark.py` (lines 691-697)
  - Current: `if not evaluators:` → creates a default `LLMJudgeEvaluator`
  - This fallback masks validation failures. After C.3+C.4, it should never be reached.
  - Replace with: `if not evaluators: raise ValueError("no valid evaluators after processing metrics list")`
  - Or remove the fallback entirely (the `raise ValueError` in C.4 will already prevent reaching this code)

##### Issue E: LiveCodeBench — Replace HF HTTP API with `datasets` Library

> **Severity:** Low (workaround exists: `max_tasks: 3`)
> **Root cause:** HuggingFace Datasets Server HTTP API (`datasets-server.huggingface.co/rows`)
> can't serve `livecodebench/code_generation` rows reliably — returns 502/504 for large rows
> even at page_size=10. Current adaptive page_size fallback (100→10→1) works but is extremely
> slow (~12h for 880 rows). Some rows return 500 and are skipped entirely.

- [x] (2026-03-16) E.1: Add `datasets` library to Poetry dependencies
  - File: `workers/pyproject.toml`
  - Add: `datasets = "^3.0"` (HuggingFace datasets library)
  - Run: `cd workers && poetry add datasets`
  - Note: This is a large dependency (~100MB with Apache Arrow). Consider making it optional via extras: `[tool.poetry.extras] hf = ["datasets"]`

- [x] (2026-03-16) E.2: Add `download_hf_dataset_parquet()` alternative in cache module
  - File: `workers/codeforge/evaluation/cache.py`
  - New function: `async def download_hf_dataset_parquet(dataset, split, provider_name, filename, base_dir, config)` that:
    1. Checks cache first (same `get_cached_path()` logic)
    2. Uses `datasets.load_dataset(dataset, config, split=split)` for direct Parquet download
    3. Converts to JSONL and saves to cache directory
    4. Handles `HF_TOKEN` authentication via `datasets.login(token=hf_token)` or `HfFolder.save_token()`
  - This bypasses the HTTP rows API entirely — uses HuggingFace Hub direct download

- [x] (2026-03-16) E.3: Update LiveCodeBench provider to use Parquet download
  - File: `workers/codeforge/evaluation/providers/livecodebench.py`
  - Change `_fetch_tasks()` to call `download_hf_dataset_parquet()` instead of `download_hf_dataset()`
  - Keep `download_hf_dataset()` as fallback for providers that work fine with HTTP API (humaneval, mbpp, bigcodebench, cruxeval)

- [x] (2026-03-16) E.4: Write tests for Parquet download path
  - File: `workers/tests/test_cache_parquet.py`
  - Test: mock `datasets.load_dataset()` → verify JSONL file created with correct records
  - Test: cached file exists → skips download
  - Test: `HF_TOKEN` env var propagated to datasets library
  - Run: `cd workers && poetry run pytest tests/test_cache_parquet.py -v`

#### Evaluation System Improvements — R2E-Gym Cherry-Picks + Categorical Verifier (COMPLETED)

> Three targeted improvements to the Phase 26+28 evaluation pipeline. Python-only, no Go/NATS/frontend changes.

- [x] (2026-03-16) **LogprobVerifierEvaluator (new):** Calibrated ranking via P(YES) logprobs with `max_tokens=1`. Softmax normalization `P(YES) = exp(yes_lp) / (exp(yes_lp) + exp(no_lp))`. Falls back to text parsing when provider doesn't support logprobs. Registered in `_build_evaluators()` + `_DIMENSION_TO_METRIC`. Files: `workers/codeforge/evaluation/evaluators/logprob_verifier.py` (new), `workers/tests/test_logprob_verifier.py` (new, 13 tests), `workers/codeforge/consumer/_benchmark.py`.
- [x] (2026-03-16) **Categorical TrajectoryVerifier:** Replaced unreliable float-based scoring (0.0-1.0) with ACHIEVED/PARTIALLY_ACHIEVED/NOT_ACHIEVED categories (based on RocketEval ICLR 2025, Prometheus ICLR 2024). Same 5 dimensions, same interface, case-insensitive parsing with backward compat for floats. `max_tokens` reduced 256->128. Files: `workers/codeforge/evaluation/evaluators/trajectory_verifier.py`, `workers/tests/test_trajectory_verifier.py` (5 new + 2 updated tests).
- [x] (2026-03-16) **Selection strategies (longest/shortest):** Added trajectory-length-based selection for `MultiRolloutRunner` (from R2E-Gym). `_trajectory_length()` helper with 3-tier fallback (trajectory -> step_count -> actual_output). Zero-cost heuristic when hybrid pipeline unavailable. Files: `workers/codeforge/evaluation/runners/multi_rollout.py`, `workers/tests/test_multi_rollout_runner.py` (7 new tests).
- Total: 48 tests (27 new + 21 existing), 7 files changed (+718/-30 lines)

#### Sidebar Restructure (COMPLETED)
- [x] (2026-03-16) Section grouping, page merges, top bar navigation

#### Loading Animation System (COMPLETED)
- [x] (2026-03-16) 5 primitives: Skeleton (text/rect/circle), TypingIndicator (bouncing dots), StreamingCursor (blink cursor), ProgressBar (determinate/indeterminate), PacmanSpinner (branded SVG)
- [x] (2026-03-16) 4 composites: SkeletonText, SkeletonCard (stat/project), SkeletonTable, SkeletonChat
- [x] (2026-03-16) 7 CSS keyframes: cf-shimmer, cf-blink, cf-bounce-dot, cf-progress-slide, cf-pacman-chomp, cf-dot-orbit, cf-fade-in
- [x] (2026-03-16) Skeleton design tokens (--cf-skeleton-base, --cf-skeleton-shine) for light/dark themes, registered in @theme
- [x] (2026-03-16) ChatPanel: TypingIndicator replaces animate-pulse, StreamingCursor replaces static "Streaming..." label
- [x] (2026-03-16) ResourceGuard: optional `skeleton` prop for custom loading states (backward-compatible)

#### Benchmark Metric Validation & Detail Card Fix (COMPLETED)
- [x] (2026-03-16) **Go ValidMetrics allowlist gap:** Frontend offers 5 metrics (`correctness`, `tool_correctness`, `faithfulness`, `answer_relevancy`, `contextual_precision`) but Go `ValidMetrics` only had 9 entries — missing `tool_correctness`, `answer_relevancy`, `contextual_precision`. Runs with all metrics failed HTTP 400. Added 3 missing metrics to `internal/domain/benchmark/benchmark.go:180`. 29 Go tests pass.
- [x] (2026-03-16) **SolidJS event delegation bug in benchmark detail card:** Clicking task rows in `BenchmarkRunDetail` collapsed the parent card because SolidJS delegates all `onClick` to `document` — `stopPropagation()` alone doesn't prevent parent SolidJS handlers. Fixed parent card `onClick` in `BenchmarkPage.tsx:346` with `target.closest("table"|"button"|"a")` guard. Added `stopPropagation()` on task row as defense-in-depth.
- [x] (2026-03-16) **Verified via Playwright MCP** with `lm_studio/qwen/qwen3-30b-a3b`: detail card shows summary scores + task results table, all 3 task rows expand/collapse correctly showing Actual Output and Evaluator Scores.

#### Frontend UI Bug Fixes & i18n (COMPLETED)
- [x] (2026-03-15) **BUG-1 (High) — Broken "Go to Chat" Navigation:** `onNavigate("chat")` silently did nothing — `"chat"` was not a valid `LeftTab`. Created unified `handleNavigate()` in `ProjectDetailPage.tsx` that switches `mobileView` to `"chat"` on mobile. Replaced 8 duplicate inline handlers. Fixed in: `GoalsPanel.tsx:202`, `SessionPanel.tsx:116`, `WarRoom.tsx:90`, `OnboardingProgress.tsx:44`.
- [x] (2026-03-15) **BUG-2 (Medium) — Dead RunPanel Code:** `run.toolcall` WS event was a stub comment. `RunPanel.addToolCall`/`updateRunStatus` attached to component function object but never called. Removed dead code — tool calls are rendered via AG-UI events in `ChatPanel`. Files: `ProjectDetailPage.tsx`, `RunPanel.tsx`.
- [x] (2026-03-15) **BUG-3 (Low) — `window.prompt()` for Folder Creation:** Replaced `window.prompt("New folder name:")` with custom Modal dialog consistent with Create/Rename/Delete modals. New state: `showFolderModal`, `newFolderName`, `newFolderPrefix`. File: `FilePanel.tsx`.
- [x] (2026-03-15) **Monaco Theme Sync:** Editor now reactively follows dark/light theme toggle via `createEffect` + `monaco.editor.setTheme()`. File: `CodeEditor.tsx`.
- [x] (2026-03-15) **File Panel Icon Alignment:** Expand/Collapse-all SVG polyline points centered in 16x16 viewBox. File: `FilePanel.tsx`.
- [x] (2026-03-15) **i18n: ~40 hardcoded strings replaced** across `FilePanel.tsx`, `FileContextMenu.tsx`, `GoalProposalCard.tsx`, `KnowledgeBasesPage.tsx`. 28 new keys in `en.ts` + `de.ts` (`files.*`, `common.approve`, `common.reject`, `detail.tab.files`).
- [x] (2026-03-15) **"Allow Always" Policy Persistence:** `PermissionRequestCard.tsx` TODO resolved. Clicking "Allow Always" now approves the current tool call AND persists a permanent `allow` rule to the project's policy profile via `POST /api/v1/policies/allow-always`. Preset profiles are cloned to `{preset}-custom-{projectId}` on first use. Rule construction: tool name + first word of command as glob pattern (e.g., `Bash/git*`). Idempotent (duplicate rules detected via `HasRuleForSpecifier`). 26 new tests across domain, service, and HTTP layers. Files: `internal/domain/policy/policy.go`, `internal/service/policy.go`, `internal/service/project.go`, `internal/adapter/http/handlers.go`, `internal/adapter/http/routes.go`, `frontend/src/api/client.ts`, `frontend/src/features/project/PermissionRequestCard.tsx`, `frontend/src/features/project/ChatPanel.tsx`.

#### Benchmark Live Feed (COMPLETED)
- [x] (2026-03-10) Go: `TrajectoryEventPayload` in `events.go` — enriched WS broadcast with cost, tokens, input, output, step fields
- [x] (2026-03-10) Go: Runtime trajectory subscription handler broadcasts enriched payload
- [x] (2026-03-10) TypeScript: `LiveFeedEvent` + `BenchmarkLiveProgress` types in `api/types.ts`
- [x] (2026-03-10) Frontend: `BenchmarkLiveFeed.tsx` — virtualized auto-scrolling feed with `@tanstack/solid-virtual`, feature accordions, progress header, elapsed timer
- [x] (2026-03-10) Integration: Wired into `BenchmarkPage.tsx` for selected running runs
- Design: `docs/specs/2026-03-10-benchmark-live-feed-design.md`
- Plan: `docs/plans/2026-03-10-benchmark-live-feed-plan.md`

#### Benchmark Live Feed — State Persistence & Density Improvements (COMPLETED)
- [x] (2026-03-16) State persistence: Live feed state lifted from `BenchmarkLiveFeed` to `BenchmarkPage` as `Map<runId, LiveFeedState>` — closing/reopening info card no longer loses state
- [x] (2026-03-16) API hydration: Running runs rehydrate from `GET /runs/{id}/trajectory` + `GET /benchmarks/runs/{id}/results` on page load
- [x] (2026-03-16) `BenchmarkLiveFeed` converted to presentational component (receives `LiveFeedState` props)
- [x] (2026-03-16) Pure functions extracted to `liveFeedState.ts` with 18 unit tests: `formatTokens`, `computeEta`, `agentEventToLiveFeedEvent`, `statsFromSummary`, `resultToFeatureEntry`
- [x] (2026-03-16) Inline stats line: avg score, tokens in/out, tool calls, $/task
- [x] (2026-03-16) Mini score bars on feature rows (green/yellow/red color coding)
- [x] (2026-03-16) ETA display when total_tasks known
- [x] (2026-03-16) Indeterminate progress bar fix for unknown total_tasks
- [x] (2026-03-18) Event dedup: backend `sequence_number` on trajectory events (migration 077, Go eventstore, frontend dedup)
- [x] (2026-03-18) WS reconnect gap: `after_sequence` REST param, frontend gap-fill on reconnect
- Spec: `docs/specs/2026-03-16-benchmark-live-feed-density-design.md`
- Plan: `docs/plans/2026-03-16-benchmark-live-feed-improvements-plan.md`

#### Agent-Eval Benchmark Results (2026-03-10)
- [x] (2026-03-10) Ran `/agent-eval mistral/mistral-large-latest` — auto-agent pipeline end-to-end
- Result: 0/300 total score (Grade F) — Mistral model could not produce code within 43 min
- All skeleton files unchanged — agent stalled on spec reading without invoking Write tool
- Infrastructure verified: workspace paths absolute, project seeding works, test suites collect correctly

#### Test Suites (COMPLETED)
- [x] Browser E2E: 17 Playwright tests (health, navigation, projects, costs, models, a11y)
- [x] LLM E2E: 95 API-level tests across 11 spec files
- [x] Benchmark E2E: 132 browser Playwright tests across 12 spec files
- [x] Benchmark Validation E2E: 22 API-level tests across 7 blocks (`frontend/e2e/benchmark-validation/`)
- [x] Backend E2E: 88 pass / 0 fail / 3 skip (97% pass rate)
- [x] Python unit tests: 134 pass (107 prior + 27 new from evaluation improvements)

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

- [ ] F3.1: Add test for billing/auth error classification — 4 tests (2026-03-09)
  - File: `workers/tests/test_routing_rate_tracker.py` (File absorbed into other test files — not yet created as standalone)
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

> Design: [docs/specs/2026-03-09-auto-agent-skills-design.md](specs/2026-03-09-auto-agent-skills-design.md)
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

> Design: [docs/specs/2026-03-09-benchmark-external-providers-design.md](specs/2026-03-09-benchmark-external-providers-design.md)
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

> Design: [docs/specs/2026-03-08-integration-testing-design.md](specs/2026-03-08-integration-testing-design.md)
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

> Go side: `internal/port/messagequeue/contract_test.go` (39 sample factories, roundtrip + fixture generation)
> Python side: `workers/tests/test_nats_contracts.py` (80 parametrized tests, fixture validation + field coverage)
> Fixtures: `internal/port/messagequeue/testdata/contracts/*.json` (31 files, Go-generated)
> Contract violation found & fixed: `BenchmarkRunResult` Python model was missing `tenant_id`

**Infrastructure:**

- [x] Create Go contract test generator -- `TestContract_GenerateFixtures` writes 31 JSON fixtures
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
- [ ] Contract test: `memory.store` -- fixture not yet generated
- [ ] Contract test: `memory.recall` -- fixture not yet generated
- [ ] Contract test: `memory.recall.result` -- fixture not yet generated
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
- [ ] Contract test: `handoff.request` -- fixture not yet generated

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

> File: `docs/feature-verification-matrix.md` (File not yet created)

- [ ] Create initial matrix with all 30 features listed
- [ ] Define verification criteria per feature: 5 test layers (Go Unit, Py Unit, E2E, Contract, Smoke)
- [ ] Mark currently-passing features based on test results (24 partial, 0 blocked)
- [ ] Add "Last Verified" date column
- [ ] Cross-reference: each feature row links to relevant test files

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

#### Phase 31 -- Contract-First Review/Refactor (COMPLETED)
- [x] Boundary domain model (ProjectBoundaryConfig, BoundaryFile) -- 2026-03-15
- [x] Plan domain: waiting_approval step status -- 2026-03-15
- [x] DB migrations (073 project_boundaries, 074 review_triggers) -- 2026-03-15
- [x] NATS subjects (review.>) for Go + Python -- 2026-03-15
- [x] Mode presets: boundary_analyzer, contract_reviewer (24 total) -- 2026-03-15
- [x] Pipeline template: review-refactor (4-step sequential) -- 2026-03-15
- [x] DiffImpactScorer (3-tier threshold HITL) -- 2026-03-15
- [x] Phase-aware context budget (boundary/contract/review/refactor phases) -- 2026-03-15
- [x] Store interface + PostgreSQL implementation (boundaries + review triggers) -- 2026-03-15
- [x] BoundaryService with CRUD and validation -- 2026-03-15
- [x] ReviewTriggerService with cascade dedup -- 2026-03-15
- [x] Orchestrator: waiting_approval status handling (approve/reject) -- 2026-03-15
- [x] HTTP endpoints: boundaries CRUD, review trigger, run approval -- 2026-03-15
- [x] Python NATS consumer: review trigger handler -- 2026-03-15
- [x] Frontend: RefactorApproval HITL UI + BoundariesPanel -- 2026-03-15
- [x] Integration wiring: services in main.go + autoIndex trigger -- 2026-03-15

#### Project Workflow Redesign (COMPLETED -- 2026-03-09)

> Plan: `docs/plans/2026-03-09-project-workflow-plan.md`
> Design: `docs/specs/2026-03-09-project-workflow-redesign-design.md`

- [x] Reorder project tabs: Files, Goals, Roadmap, Feature Map, War Room, Sessions, Trajectory, Audit (2026-03-09)
- [x] Add i18n keys for onboarding, empty states, and chat suggestions (2026-03-09)
- [x] Add empty states with navigation links to all 8 tab panels (2026-03-09)
- [x] Proactive agent greeting on first chat open per project (localStorage-gated) (2026-03-09)
- [x] Lint, TypeScript verify, pre-commit pass (2026-03-09)

#### Phase 32 -- Visual Design Canvas

> Feature spec: `docs/features/06-visual-design-canvas.md`

**Phase 32A -- Canvas Types & State (Frontend)**
- [x] 32A.1: Create `canvasTypes.ts` -- all type definitions (CanvasElement, ElementStyle, tool types) (2026-03-16)
- [x] 32A.2: Write tests for canvas state store (add/remove/update/undo/redo/select) (2026-03-16)
- [x] 32A.3: Implement `canvasState.ts` -- SolidJS createStore with undo/redo (2026-03-16)

**Phase 32B -- SVG Viewport & Select Tool (Frontend)**
- [x] 32B.1: Write tests for `screenToSvg` coordinate transform (2026-03-16)
- [x] 32B.2: Implement `DesignCanvas.tsx` -- main SVG component with viewport (2026-03-16)
- [x] 32B.3: Implement `SelectTool.ts` -- select, move, resize via pointer capture (2026-03-16)

**Phase 32C -- Shape Tools (Frontend)**
- [x] 32C.1: Implement `RectTool.ts` -- rectangle creation via drag (2026-03-16)
- [x] 32C.2: Implement `EllipseTool.ts` -- ellipse/circle creation (Shift constraint) (2026-03-16)
- [x] 32C.3: Write tests for Catmull-Rom path smoothing (2026-03-16)
- [x] 32C.4: Implement `FreehandTool.ts` -- freehand SVG path with smoothing (2026-03-16)
- [x] 32C.5: Implement `TextTool.ts` -- click-to-place text via foreignObject (2026-03-16)

**Phase 32D -- Image Upload & Annotation (Frontend)**
- [x] 32D.1: Implement `ImageTool.ts` -- file upload, base64, 5MB limit (2026-03-16)
- [x] 32D.2: Implement `AnnotateTool.ts` -- arrow + callout annotation (2026-03-16)

**Phase 32E -- Toolbar & Modal (Frontend)**
- [x] 32E.1: Implement `CanvasToolbar.tsx` -- tool selector with keyboard shortcuts (2026-03-16)
- [x] 32E.2: Implement `CanvasModal.tsx` -- fullscreen modal wrapper (2026-03-16)

**Phase 32F -- Export Pipeline (Frontend)**
- [x] 32F.1: Write tests for PNG export (2026-03-16)
- [x] 32F.2: Implement `exportPng.ts` -- SVG to PNG via offscreen canvas (2026-03-16)
- [x] 32F.3: Write tests for ASCII art export (2026-03-16)
- [x] 32F.4: Implement `exportAscii.ts` -- element tree to character grid (2026-03-16)
- [x] 32F.5: Write tests for JSON export (2026-03-16)
- [x] 32F.6: Implement `exportJson.ts` -- structured JSON description (2026-03-16)
- [x] 32F.7: Implement `CanvasExportPanel.tsx` -- export preview sidebar (2026-03-16)

**Phase 32L -- Canvas Tool Improvements (Frontend)**
- [x] 32L.1: 8-point resize handles on SelectTool with Shift aspect-ratio lock (2026-03-17)
- [x] 32L.2: Inline text/annotation editing via double-click (foreignObject textarea) (2026-03-17)
- [x] 32L.3: Fix freehand movement + apply Catmull-Rom smoothing in renderer (2026-03-17)
- [x] 32L.4: PolygonTool -- multi-click polygon, close on first-vertex or double-click (2026-03-17)
- [x] 32L.5: NodeTool -- drag individual vertices on polygon/freehand/annotation (2026-03-17)
- [x] 32L.6: ImageTool drag-to-size with preview rect (2026-03-17)
- [x] 32L.7: Delete/Backspace removes selected elements (2026-03-17)
- [x] 32L.8: Collapsible + resizable export panel with drag handle (2026-03-17)
- [x] 32L.9: Polygon ASCII export via Bresenham line rasterization (2026-03-17)

**Phase 32G -- Phase 1 Integration & E2E Tests**
- [x] 32G.1: E2E test -- canvas basic interactions (2026-03-18)
- [x] 32G.2: E2E test -- export pipeline (2026-03-18)
- [x] 32G.3: Add canvas entry point to ProjectDetailPage (2026-03-18)

**Phase 32H -- Go Backend Multimodal Pipeline (COMPLETED 2026-03-18)**
- [x] 32H.1: Write failing Go test -- MessageImage serialization (2026-03-18)
- [x] 32H.2: Add MessageImage to Go domain types (2026-03-18) — `internal/domain/conversation/conversation.go`
- [x] 32H.3: Write database migration 075_add_message_images.sql (2026-03-18)
- [x] 32H.4: Update message store to read/write images JSONB (2026-03-18) — `store_conversation.go`
- [x] 32H.5: Write failing Go test -- NATS payload with images (2026-03-18)
- [x] 32H.6: Add MessageImagePayload to NATS schema (2026-03-18) — `schemas.go:453-468`
- [x] 32H.7: Update historyToPayload to propagate images (2026-03-18) — `conversation_agent.go:863-869`
- [x] 32H.8: Run full Go test suite -- no regressions (2026-03-18)

**Phase 32I -- Python Workers Multimodal Pipeline (COMPLETED 2026-03-18)**
- [x] 32I.1: Write failing Python test -- MessageImagePayload model (2026-03-18)
- [x] 32I.2: Add MessageImagePayload to Python models (2026-03-18) — `models.py:433-449`
- [x] 32I.3: Write failing Python test -- history builder multimodal output (2026-03-18)
- [x] 32I.4: Update _to_msg_dict for multimodal content-array (2026-03-18) — `history.py:135-171`
- [x] 32I.5: Contract test -- Go marshal to Python unmarshal (2026-03-18) — standalone tests in `schemas_test.go` + `test_multimodal.py`
- [x] 32I.6: Run full Python test suite -- no regressions (2026-03-18)

**Phase 32J -- Frontend Multimodal Types (COMPLETED 2026-03-18)**
- [x] 32J.1: Add MessageImage to frontend types.ts (2026-03-18) — `types.ts:1392-1395`
- [x] 32J.2: Update ChatPanel to render image thumbnails (2026-03-18) — `ChatPanel.tsx:849-865`

**Phase 32K -- Canvas-to-Chat Integration**
- [x] 32K.1: Add supports_vision to Go model discovery (2026-03-16)
- [x] 32K.2: Add supports_vision to frontend LLMModel type (done in 32J)
- [x] 32K.3: Implement buildCanvasPrompt() utility (2026-03-16)
- [x] 32K.4: Write tests for buildCanvasPrompt() (2026-03-16)
- [x] 32K.5: Wire canvas export to ChatPanel send (2026-03-16)
- [x] 32K.6: E2E test -- canvas to chat flow (2026-03-18)

#### Stub & Placeholder Cleanup (2026-03-17)

> Tracker: `docs/audits/stub-tracker.md` (full details, file:line references, effort estimates)

**Quick Wins (small effort, high impact):**
- [x] STUB-002: Convert `StubBackendExecutor` to ABC with `@abstractmethod` on `info` property (2026-03-17)
- [x] STUB-005: Add `mode` + `model` columns to conversations table, implement UPDATE queries (2026-03-17)
- [x] STUB-011: Refactor `detectGoalFiles()` to use intermediate `DetectedGoal` type (2026-03-17)

**Medium Effort:**
- [x] STUB-004: Wire stall detection (`countStallIterations`) into conversation agent template data (2026-03-18) — BudgetPercent still 0.0, needs Python worker cost reporting
- [x] STUB-003: Implement review trigger dispatch to boundary_analyzer agent loop (`workers/codeforge/consumer/_review.py:35`) (2026-03-18)
- [x] STUB-009: Add trajectory event `sequence_number` (Go) + frontend dedup + WS reconnect re-hydration (`frontend/src/features/benchmarks/BenchmarkPage.tsx:131-135`) (2026-03-18)
- [x] STUB-006: A2A agent card skills already built dynamically in SDK-based `CardBuilder` (`internal/adapter/a2a/agentcard.go`) (2026-03-18)
- [x] STUB-012: Pipeline `Instantiate()` auto-generates TaskID/AgentID UUIDs when nil (2026-03-18)

**Large Effort (phase-level):**
- [x] STUB-001: A2A Python consumer mixin (`workers/codeforge/consumer/_a2a.py`) + PostgreSQL persistence already in SDK implementation (2026-03-18) — Cleanup: delete dead code `internal/port/a2a/`
- [x] STUB-010: SWE-agent backend adapter implementation (`workers/codeforge/backends/sweagent.py`) (2026-03-17)

**Small Effort (new from 2026-03-18 scan):**
- [x] STUB-024: Wire re-run benchmark button onClick in PromptOptimizationPanel (2026-03-18)
- [x] STUB-025: Remove deprecated `activeTool` prop from CanvasModal interface (2026-03-18)
- [x] Cleanup: Delete dead code `internal/port/a2a/` (zero imports, merge artifact) (2026-03-18)
- [x] STUB-004 (remaining): Wire BudgetPercent from event store accumulated cost (2026-03-18)

**No Action Needed (intentional designs):**
- STUB-007/008: GitHub OAuth + Subscription 501 gates (feature-gated by design)
- STUB-013/014/015: Intentional no-ops (_BenchmarkRuntime, StubBackendExecutor.cancel, useCRUDForm fallback)
- STUB-016-023, STUB-026: Documentation TODOs and intentional config exclusions

**Modular Prompt System (Phases A-F)**
- [x] Phase A: Domain types (PromptEntry, AssemblyContext, Category, Conditions) + YAML loader + tests (2026-03-17)
- [x] Phase B: PromptAssembler + PromptLibraryService + wiring into buildSystemPrompt + tests (2026-03-17)
- [x] Phase C: Write 56 embedded YAML prompt library files across 12 categories (2026-03-17)
- [x] Phase D: Migrate 24 mode PromptPrefix strings to YAML library files (2026-03-17)
- [x] Phase E: System reminders in NATS payload and prompt pipeline (2026-03-17)
- [x] Phase F: Wire assembler at app startup, add PromptsFS() accessor, integration tests (2026-03-17)

#### Codebase-Wide Lint Cleanup (2026-03-18)

- [x] Go: Fix all 50 golangci-lint issues (errcheck, gocritic hugeParam/rangeValCopy/appendCombine, gosec G118 nolint) (2026-03-18)
- [x] TypeScript: Fix all 21 ESLint issues (18 SolidJS reactivity warnings, 3 config parsing errors) (2026-03-18)
- [x] Config: golangci-lint exclusions for frontend/node_modules + G117/G118 test rules (2026-03-18)
- [x] Audit: Reviewed ~220+ linter suppression comments across Go/Python/TypeScript -- all justified (2026-03-18)

---

### Quality & Performance Improvements (2026-03-18)

> Spec: `docs/specs/2026-03-18-quality-performance-improvements-design.md`
> Based on: Current LLM-for-coding research (2025/26), codebase analysis, identified gaps
> 12 measures, 90+ atomic TODOs, 5 implementation phases

#### Phase 1 — Quick Wins (COMPLETED 2026-03-18)

**A1: Stall Detection + Escape** -- COMPLETED (2026-03-18)
- [x] A1.1-A1.4: Write 22 stall detection tests (identical calls, args hash, escape injection, double-stall abort, edge cases)
- [x] A1.5: Implement `StallDetector` class in `workers/codeforge/agent_loop.py` (~50 lines, deque-based sliding window)
- [x] A1.6: Integrate into `AgentLoopExecutor.run()` with `_check_stall()` helper (cyclomatic complexity managed)
- [x] A1.7: Publish `trajectory.stall_detected` event via `_runtime.publish_trajectory_event()`
- [x] A1.8: 60/60 tests pass (22 new + 33 existing agent loop + 5 related)

**B3: Adaptive Context Budget Based on Task Complexity** -- COMPLETED (2026-03-18)
- [x] B3.1-B3.3: Write 17 tests (9 budget mapping + 8 complexity classifier including edge cases)
- [x] B3.4: Implement `ClassifyComplexity()` in `internal/service/complexity.go` (240 lines, 7 heuristics + task-type boost)
- [x] B3.5: Implement `ComplexityBudget()` in `internal/service/context_budget.go` (composes with PhaseAware + Adaptive)
- [x] B3.6: NATS payload integration deferred to Phase 2 (Go-side functions ready)
- [x] B3.7: All Go service tests pass, golangci-lint clean

**C2: Confidence-Based Early Stopping for Multi-Rollout** -- COMPLETED (2026-03-18)
- [x] C2.1-C2.5: Write 15 early stopping tests (quorum, threshold, exit_code, rollout_count<=3, clusters, best selection)
- [x] C2.6: Implement `EarlyStopChecker` class in `workers/codeforge/evaluation/runners/early_stopping.py`
- [x] C2.7: Integrate into `MultiRolloutRunner.run()` with `MultiRolloutMetadata` dataclass
- [x] C2.8: Add `CODEFORGE_EARLY_STOP_THRESHOLD` / `CODEFORGE_EARLY_STOP_QUORUM` env vars
- [x] C2.9: 35/35 tests pass (15 new + 20 existing runner tests)

#### Phase 2 — Core Quality (A1 helps validate, ~8h total)

**A3: Plan/Act Mode Toggle (~5h)** -- DONE 2026-03-18
- [x] A3.1-A3.5: Write plan/act tests (tool restriction, phase transition, max iterations, autonomy, routing tags) -- 29 Python + 8 Go tests
- [x] A3.6: Add `plan_act_enabled` to NATS payload (`schemas.go` + `models.py`)
- [x] A3.7: Set `plan_act_enabled` based on `modeAutonomy >= 4` in dispatcher (`conversation_agent.go`)
- [x] A3.8: Implement `PlanActController` class in `workers/codeforge/plan_act.py`
- [x] A3.9: Integrate into `run()` and `_do_llm_iteration()` in `agent_loop.py`
- [x] A3.10-A3.11: Add `CODEFORGE_PLAN_ACT_MAX_ITERATIONS` env var (default 10) + all tests passing

**B2: Semantic Deduplication of Context Candidates (~3h)** ✅ 2026-03-18
- [x] B2.1-B2.3b: Write dedup tests (overlapping lines, cross-file, no-dupes, simhash edge cases) — 26 tests in `dedup_test.go`
- [x] B2.4: Implement `simhash64()` + `hammingDistance()` in `internal/service/dedup.go`
- [x] B2.5: Implement `deduplicateCandidates()` in `dedup.go`
- [x] B2.6: Integrate into `assembleAndPack()` in `context_optimizer.go`
- [x] B2.7: Run regression tests — all 11 context optimizer tests pass

#### Phase 3 — Context Intelligence (COMPLETED)

**B1: LLM-Based Re-Ranking of Retrieval Results**
- [x] (2026-03-18) B1.1-B1.4: Write reranker tests (reorder, prompt format, fallback, routing tags)
- [x] (2026-03-18) B1.5: Add NATS subjects in `queue.go` + `_subjects.py`
- [x] (2026-03-18) B1.6: Implement `ContextReranker` in `workers/codeforge/context_reranker.py`
- [x] (2026-03-18) B1.7-B1.7b: Add NATS handler (`ContextHandlerMixin`) + Go integration test
- [x] (2026-03-18) B1.8: Integrate into Go `ContextOptimizerService` (syncWaiter + `assembleAndPack()`)
- [x] (2026-03-18) B1.9-B1.11: Add config (`CODEFORGE_CONTEXT_RERANK_ENABLED/MODEL`) + verify JetStream + run tests

**A2: Conversation Summarization at Context Exhaustion**
- [x] (2026-03-18) A2.1-A2.4: Write summarization tests (threshold, tail preservation, prompt, routing tags)
- [x] (2026-03-18) A2.5: Implement `ConversationSummarizer` class in `workers/codeforge/history.py`
- [x] (2026-03-18) A2.6: Implement `_summarize_history()` async method
- [x] (2026-03-18) A2.7: Add `async summarize_if_needed()` pre-processing step in `_conversation.py`
- [x] (2026-03-18) A2.8: Add `CODEFORGE_SUMMARIZE_THRESHOLD` env var + wire into NATS payload
- [x] (2026-03-18) A2.9: Run regression tests (16 new tests pass, no regressions)

#### Phase 4 — Advanced Features (COMPLETED 2026-03-18)

**C1: Routing Transparency + Mid-Loop Model Switching (COMPLETED 2026-03-18)**
- [x] (2026-03-18) C1.1-C1.4: Write routing transparency + quality signal + model switch tests — 7 new tests in `test_routing_transparency.py`
- [x] (2026-03-18) C1.5: `route_with_metadata()` on `HybridRouter` — `router.py:133`, returns `RoutingMetadata`
- [x] (2026-03-18) C1.6: `IterationQualityTracker` class — `agent_loop.py:857`, integrated into `run()` at line 310
- [x] (2026-03-18) C1.7: Wired `route_with_metadata()` into agent loop via `RoutingResult.routing_metadata` → `LoopConfig` → `_publish_routing_decision()` trajectory event
- [x] (2026-03-18) C1.8: 132/132 tests pass (23 routing + 33 agent loop + 67 routing/fallback + 34 consumer dispatch), zero regressions

**A4: Inference-Time Scaling for Conversations (COMPLETED 2026-03-18)**
- [x] (2026-03-18) A4.1-A4.5b: 27 rollout tests in `test_conversation_rollout.py` (single, multi, selection, early stopping, cost, non-git fallback, snapshot/restore, clamping, trajectory metadata)
- [x] (2026-03-18) A4.6: `rollout_count` in Go NATS payload (`schemas.go:495`) + Python `ConversationRunStartMessage` (`models.py:488`)
- [x] (2026-03-18) A4.7: `Agent.ConversationRolloutCount` config (`config.go:151`), env var `CODEFORGE_AGENT_CONVERSATION_ROLLOUT_COUNT`, default 1
- [x] (2026-03-18) A4.8: `ConversationRolloutExecutor` (`agent_loop.py:1003`) with `EarlyStopChecker`, non-git fallback
- [x] (2026-03-18) A4.9: `_snapshot_workspace()` (`agent_loop.py:919`) + `_restore_workspace()` (`:935`) via git stash
- [x] (2026-03-18) A4.10: NATS consumer dispatch wired — `rollout_count > 1` triggers `ConversationRolloutExecutor` in `_conversation.py:279-290`, clamped `max(1, min(count, 8))`
- [x] (2026-03-18) A4.11-A4.12: `trajectory.rollout_complete` event with rollout_count, selected_index, scores, early_stopped; NATS contract fixture updated; 128/128 tests pass

#### Phase 5 — Ecosystem (COMPLETED)

**C3: New Benchmark Providers — DPAI Arena + Terminal-Bench (~6h)**
- [x] (2026-03-18) C3.1-C3.2: Research dataset formats and access methods
- [x] (2026-03-18) C3.3-C3.4: Write provider load tests (20 DPAI Arena + 21 Terminal-Bench + 15 Filesystem State = 56 tests)
- [x] (2026-03-18) C3.5: Implement `DPAIArenaProvider` (BenchmarkType.SIMPLE, HuggingFace DPAI/arena dataset)
- [x] (2026-03-18) C3.6: Implement `TerminalBenchProvider` (BenchmarkType.AGENT, filesystem state verification)
- [x] (2026-03-18) C3.7: Add `FilesystemStateEvaluator` for Terminal-Bench (expected files, content match, missing files)
- [x] (2026-03-18) C3.8-C3.9: Update Go `defaultSuites` + all tests pass

**C4: RLVR Training Pipeline Export (~8h)**
- [x] (2026-03-18) C4.1-C4.4: Write RLVR export tests (19 Python + 13 Go service + 4 Go handler = 36 tests)
- [x] (2026-03-18) C4.5: Implement `compute_rlvr_reward()` (weighted avg, functional_test 2x, clamped [0,1])
- [x] (2026-03-18) C4.6: Implement `format_rlvr_entry()` formatter + `RLVRExporter` class
- [x] (2026-03-18) C4.7: Implement `ExportRLVRDataset()` in Go service + `ComputeRLVRReward()`
- [x] (2026-03-18) C4.8-C4.10: Add `GET /api/v1/benchmarks/runs/{id}/export/rlvr` endpoint (JSONL + JSON)
- [x] (2026-03-18) C4.11: Full test suite passes (87 Python, all Go packages green)

#### Claude Code Integration

- [x] Claude Code as routing target with execution branch (2026-03-18)
  - ClaudeCodeExecutor with SDK + CLI fallback
  - Policy enforcement via can_use_tool callback
  - COMPLEXITY_DEFAULTS: claudecode/default in COMPLEX + REASONING
  - Availability detection with caching
- [ ] Claude Code: E2E manual test with live CLI
- [x] (2026-03-19) Claude Code: model selection override (claudecode/claude-sonnet-4) <!-- audit: implemented in claude_code_executor.py + conversation routing -->
- [x] Claude Code: read CODEFORGE_CLAUDECODE_MAX_TURNS, _TIMEOUT, _TIERS from env (2026-03-18)
- [x] Claude Code: update docs/dev-setup.md with new env vars (2026-03-18)

#### UX/UI Audit Implementation (COMPLETED 2026-03-18)

> 17 atomic tasks across 3 layers implementing UX/UI improvements.

**Layer 1 — Quick Wins (6 tasks):**
- [x] (2026-03-18) Q1: Added anvil SVG favicon (`frontend/public/favicon.svg`)
- [x] (2026-03-18) Q2: Per-page document titles (17 pages)
- [x] (2026-03-18) Q3: Fixed Prompts Preview button variant (ghost -> secondary)
- [x] (2026-03-18) Q4: Debounced WebSocket reconnect banner (2s delay + 3s initial suppress)
- [x] (2026-03-18) Q5: Abbreviated KPI labels for mobile viewport
- [x] (2026-03-18) Q6: Hover effects + click-to-navigate on project cards

**Layer 2 — Medium-Term (7 tasks):**
- [x] (2026-03-18) M1: SVG empty state illustrations for 6 pages (MCP, Knowledge, Benchmarks, Prompts, Activity, Costs)
- [x] (2026-03-18) M2: Page transition fade-in animations (PageTransition component)
- [x] (2026-03-18) M3: Skeleton loaders replacing "Loading..." text on AI Config, Costs, Settings
- [x] (2026-03-18) M4: Sticky section navigation on Settings page (9 sections, IntersectionObserver)
- [x] (2026-03-18) M5: Project Detail graceful degradation (per-panel ErrorBoundary)
- [x] (2026-03-18) M6: Anvil brand mark in sidebar header (CodeForgeLogo component)
- [x] (2026-03-18) M7: Collapsible model cards on AI Config page (expand/collapse all)

**Layer 3 — Strategic (4 tasks):**
- [x] (2026-03-18) S1: Typography system -- Outfit (display) + Source Sans 3 (body), self-hosted woff2 in `frontend/public/fonts/`
- [x] (2026-03-18) S2: Micro-interactions -- button press, card hover lift, tab animation, KPI count-up, toast slide-in, modal fade+scale
- [x] (2026-03-18) S3: Living design system page at `/design-system` (dev-mode only) + `frontend/src/ui/DESIGN-SYSTEM.md`
- [x] (2026-03-18) S4: 3-step onboarding wizard for first-time users (Connect Code -> Configure AI -> Create Project)

---

#### Bugs Found During Autonomous Goal-to-Program Test (2026-03-19)

> Discovered during S1 testplan execution (`docs/testing/2026-03-19-autonomous-goal-to-program-testplan.md`).
> These are product bugs blocking autonomous agent execution end-to-end.

**Bug 1 — Model Router ignores LiteLLM health status (Priority: HIGH) — FIXED 2026-03-19**
- [x] (2026-03-19) `model_resolver.py` now queries `/health` endpoint and prefers healthy models over alphabetically first
- [x] (2026-03-19) `key_filter.py` now accepts models known healthy from LiteLLM `/health` even without API key (covers local LM Studio via `openai/container`)
- [x] (2026-03-19) `_conversation.py:144` explicit `model` from NATS payload now takes precedence over routing; routing only applies when no explicit model set
- [x] (2026-03-19) Health-aware model selection added via `_fetch_healthy_models()` in `model_resolver.py`

**Bug 2 — NATS JetStream backlog from cancelled conversations blocks new runs (Priority: CRITICAL) — FIXED 2026-03-19**
- [x] (2026-03-19) `handleConversationToolCall` now fast-rejects tool calls for cancelled conversation runs via `cancelledConvRuns` sync.Map
- [x] (2026-03-19) `StopConversation` HTTP handler calls `Runtime.MarkConversationRunCancelled()` which also cleans up HITL approval channels
- [ ] Consider per-conversation NATS subjects or consumer groups to prevent cross-conversation blocking (future improvement)

**Bug 3 — Worker env var naming inconsistency (Priority: LOW) — FIXED 2026-03-19**
- [x] (2026-03-19) `LITELLM_URL` vs `LITELLM_BASE_URL` — code reads `LITELLM_BASE_URL`, documented in testplan
- [x] (2026-03-19) `_conversation.py` inline env reads replaced with `self._litellm_url` from constructor
- [x] (2026-03-19) `_benchmark.py` inline env reads replaced with `WorkerSettings().litellm_url`

**Bug 4 — Onboarding agent blocks project setup (Priority: MEDIUM) — FIXED 2026-03-19**
- [x] (2026-03-19) `goal_researcher` mode autonomy changed from 2 (semi-auto) to 4 (full-auto) — tool calls auto-approve
- [ ] HITL approval cards for `list_directory`/`glob_files` tools are not visible in chat UI (tool calls time out silently after 60s) — separate UI bug
- [x] (2026-03-19) Zombie NATS messages from cancelled conversations now fast-rejected (see Bug 2 fix)

**Bug 5 — WSL2 Docker port mapping unreachable from host (Priority: LOW, environment-specific) — FIXED 2026-03-19**
- [x] (2026-03-19) Documented in testplan Phase 0 — container IPs must be used instead of localhost
- [ ] Add `dev-setup.md` section for WSL2-specific Docker networking workarounds
- [x] (2026-03-19) Created `scripts/resolve-docker-ips.sh` helper that exports correct env vars

#### Multi-Language Autonomous Testplan (2026-03-22)

> Design spec: `docs/specs/2026-03-22-autonomous-multi-language-testplan-design.md`
> Testplan: `docs/testing/autonomous-multi-language-testplan.md`
> Implementation plan: `docs/plans/2026-03-22-autonomous-multi-language-testplan.md`

- [x] (2026-03-22) Research: AI coding agent benchmarks + showcase demos (SWE-bench, Commit0, DevBench, Devin, MetaGPT, Codex, etc.)
- [x] (2026-03-22) Design spec: 10 phases, 2 modes (A: Weather Dashboard, D: Free Choice), 4-tier verification
- [x] (2026-03-22) Implementation plan: 11 tasks, 28 steps
- [x] (2026-03-22) Executable runbook: 11 phases with Playwright-MCP commands, decision trees, report template
- [ ] First test run: Mode A (Weather Dashboard) with local model
- [ ] First test run: Mode A with cloud model (Claude/GPT)
- [ ] First test run: Mode D (Free Choice)

#### Universal Audit Remediation (2026-03-23)

> Audit report: `docs/audits/2026-03-23-universal-audit-report.md`
> 11 commits, 57 files changed, +3287 / -863 lines

**Resolved Findings:**
- [x] (2026-03-23) **F-002 (CRITICAL):** Hexagonal architecture violation -- event types moved from `adapter/ws/events.go` to `internal/domain/event/` (broadcast.go, broadcast_payloads.go, agui.go). OTEL span helpers moved from `adapter/otel/spans.go` to `internal/telemetry/spans.go`. LSP service decoupled via `port/codeintel/provider.go` interface + `adapter/lsp/noop.go` fallback
- [x] (2026-03-23) **F-008 (HIGH):** Silenced database errors -- 38 `_ = s.store.*` calls in service layer replaced with `logBestEffort` helper (`internal/service/log_best_effort.go`) that logs non-fatal errors with structured context
- [x] (2026-03-23) **F-010 (HIGH):** Test coverage gaps -- 2384 LOC new tests: `runtime_execution_test.go` (903 LOC), `runtime_lifecycle_test.go` (904 LOC), `test_conversation_handler.py` (577 LOC)
- [x] (2026-03-23) **F-033 (MEDIUM):** Handler struct concrete types -- `Handlers.LiteLLM *litellm.Client` replaced with `Handlers.LLM llm.Provider`, `Handlers.Copilot *copilot.Client` replaced with `Handlers.TokenExchanger tokenexchange.Exchanger`
- [x] (2026-03-23) **F-034 (MEDIUM):** Event types in adapter layer -- moved to `internal/domain/event/` (55 event constants + 49 payload structs)

**Remaining Findings (not yet addressed):**
- [ ] **F-001 (CRITICAL):** Rotate all exposed API keys in `.env` and `data/.env`
- [ ] **F-003 (CRITICAL):** Remove hardcoded JWT secret from `codeforge.yaml`, generate via `openssl rand -hex 32`
- [ ] **F-004 (HIGH):** Configure TLS in production stack (Traefik ACME/Let's Encrypt, PostgreSQL `sslmode=require`)
- [ ] **F-006 (HIGH):** Decompose `database.Store` god interface (~290 methods) into domain-specific repositories
- [ ] **F-007 (HIGH):** Decompose `Handlers` struct (69 fields, 353 methods) into domain-specific handler groups
- [ ] **F-009 (HIGH):** Fix safety check fail-open pattern in `workers/codeforge/skills/safety.py` -- change to fail-closed
- [ ] **F-011 (HIGH):** Refactor `RuntimeService.StartRun` (282 lines, 12 responsibilities) into helper methods
- [ ] **F-012 (HIGH):** Split `RuntimeService` god object (41 methods, 2051 LOC) into focused services
- [ ] **F-013 (HIGH):** Wrap A2A SDK types in port interfaces (anti-corruption layer)
- [ ] **F-014 (HIGH):** Move direct HTTP calls from service layer to adapter layer via `port/httpclient/` interface
- [ ] **F-015 (HIGH):** Redact PII (user email) from INFO-level production logs (GDPR Art. 5(1)(c))
- [ ] **F-016 (HIGH):** Implement GDPR data deletion (`DELETE /users/me`) and export (`GET /users/me/export`) endpoints
- [ ] **F-017 to F-055:** 24 MEDIUM + 10 LOW findings -- see audit report for full details

---

#### Frontend Feature Pages & Prompt Evolution (COMPLETED 2026-03-23)

- [x] (2026-03-23) **MicroagentsPage:** UI for managing YAML+Markdown trigger-driven microagents -- `frontend/src/features/microagents/MicroagentsPage.tsx`
- [x] (2026-03-23) **QuarantinePage:** Admin review UI for Phase 23B message quarantine (risk scores, evaluate/approve/reject) -- `frontend/src/features/quarantine/QuarantinePage.tsx`
- [x] (2026-03-23) **A2APage:** Frontend for Phase 27 A2A v0.3.0 agent federation (remote agents, task history, AgentCard details) -- `frontend/src/features/a2a/A2APage.tsx`
- [x] (2026-03-23) **RoutingStatsPage:** Live statistics for Phase 29 hybrid routing (model distribution, fallback events, MAB UCB1 scores) -- `frontend/src/features/routing/RoutingStatsPage.tsx`
- [x] (2026-03-23) **Prompt Evolution:** LLM-driven prompt improvement pipeline -- reflect/mutate cycles via NATS. Files: `frontend/src/features/prompts/EvolutionTab.tsx`, `internal/service/prompt_evolution.go`, `workers/codeforge/consumer/_prompt_evolution.py`, migration 078

#### Security Hardening -- Per-User Rate Limiting & Cookie Config (2026-03-24)

- [x] (2026-03-24) **FIX-093:** Add `force_secure_cookies` config flag to `Server` struct -- unconditionally set `Secure=true` on cookies for TLS-terminating proxy deployments. Refactored `isSecureRequest` into `isSecureRequestWithConfig` + `isSecureCookie` method on Handlers. Files: `internal/config/config.go`, `internal/adapter/http/handlers_auth.go`, `internal/adapter/http/handlers.go`
- [x] (2026-03-24) **FIX-096:** Add per-user rate limiting keyed on JWT user ID -- composite key `userID:IP` for authenticated requests, IP-only fallback for unauthenticated. Auth middleware injects user ID into context at all 5 auth paths. Files: `internal/middleware/ratelimit.go`, `internal/middleware/auth.go`
