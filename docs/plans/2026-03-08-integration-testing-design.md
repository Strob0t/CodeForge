# Integration Testing Strategy -- Design Document

> Date: 2026-03-08
> Status: Approved
> Scope: Smoke Tests (A) + Contract Tests (B) + Feature Verification Matrix (C)

## Problem Statement

CodeForge has ~30 major features across three layers (SolidJS, Go, Python) communicating via REST, WebSocket, and NATS JetStream. Individual layers have good unit test coverage (161 Go test files, 72 Python test files, 72 E2E Playwright specs), but **no tests verify that features work together across layer boundaries**.

### Current Test Gaps

1. **10 Go Postgres Store tests FAILING** (ProjectCRUD, TenantIsolation, UserCRUD)
2. **NATS message roundtrip never tested** (Go publish -> Python consume -> Go receive)
3. **Python consumer dispatch untested** (14 files, 0 tests)
4. **Agent tools untested** (13 files, 0 tests: Read, Write, Edit, Bash, Search, Glob, ListDir)
5. **CI runs no E2E tests** (only unit tests + lint + security scan)
6. **No feature verification tracking** (no matrix of "feature X verified on date Y")

### Feature Dependency Analysis

The Conversation feature alone touches 9 services:
- ModelRegistry, MCPService, PolicyService, ContextOptimizer, MicroagentService
- GoalService, MemoryService, NATS Publish, Python Agent Loop

25+ NATS subjects, 89 REST endpoints, and 8 AG-UI event types create a dense dependency graph where any single field mismatch can cause silent failures.

## Strategy Overview

Three complementary layers, ordered by execution priority:

```
Layer C: Feature Verification Matrix (tracking framework)
  |
  |-- Layer B: Contract Tests (fast, no Docker, CI-friendly)
  |     |-- B1: Fix broken foundation tests
  |     |-- B2: NATS payload contract tests
  |     |-- B3: Unit tests for untested modules
  |
  |-- Layer A: Smoke Tests (full stack, Docker Compose)
        |-- A1: Stack health verification
        |-- A2: Critical flow smoke tests
        |-- A3: CI integration with Docker Compose
```

---

## Layer B: Contract Tests (Priority 1 -- Foundation)

### B1: Fix Broken Foundation Tests

**Goal:** All existing tests pass green.

The 10 failing Postgres store tests indicate schema or query regressions in tenant isolation and CRUD operations. These are foundation-level -- nothing above them can be trusted while they fail.

**Scope:**
- `internal/adapter/postgres/store_test.go` -- TestStore_ProjectCRUD, TestStore_UserCRUD, TestStore_TokenRevocation
- `internal/adapter/postgres/store_a2a_test.go` -- TestStore_A2ATask_TenantIsolation, TestStore_ListA2ATasks_TenantIsolation, TestStore_ListA2ATasks_LimitParameterized, TestStore_RemoteAgent_TenantIsolation, TestStore_ListRemoteAgents_TenantIsolation
- `internal/adapter/postgres/store_test.go` -- TestStore_Conversation_TenantIsolation, TestStore_GetProjectByRepoName_TenantIsolation

### B2: NATS Payload Contract Tests

**Goal:** Prove that Go JSON serialization and Python Pydantic deserialization produce identical results for every NATS message type.

**Approach:** Generate JSON fixtures from Go structs, validate them against Python Pydantic models (and vice versa). No running NATS needed -- pure serialization tests.

**Scope -- 25+ NATS payload types:**

| NATS Subject | Go Struct (schemas.go) | Python Model (models.py) |
|---|---|---|
| conversation.run.start | ConversationRunStartPayload | ConversationRunStartMessage |
| conversation.run.complete | ConversationRunCompletePayload | ConversationRunCompleteMessage |
| benchmark.run.request | BenchmarkRunRequestPayload | BenchmarkRunRequestMessage |
| benchmark.run.result | BenchmarkRunResultPayload | BenchmarkRunResultMessage |
| evaluation.gemmas.request | GemmasEvalRequest | GemmasEvalRequestMessage |
| evaluation.gemmas.result | GemmasEvalResultPayload | GemmasEvalResultMessage |
| memory.store | MemoryStoreMessage | MemoryStoreMessage |
| memory.recall | MemoryRecallMessage | MemoryRecallMessage |
| memory.recall.result | MemoryRecallResultPayload | MemoryRecallResultMessage |
| repomap.generate.request | RepoMapRequestPayload | RepoMapRequestMessage |
| repomap.generate.result | RepoMapResultPayload | RepoMapResultMessage |
| retrieval.index.request | RetrievalIndexRequestPayload | RetrievalIndexRequestMessage |
| retrieval.index.result | RetrievalIndexResultPayload | RetrievalIndexResultMessage |
| retrieval.search.request | RetrievalSearchRequestPayload | RetrievalSearchRequestMessage |
| retrieval.search.result | RetrievalSearchResultPayload | RetrievalSearchResultMessage |
| retrieval.subagent.request | SubAgentSearchRequestPayload | SubAgentSearchRequestMessage |
| retrieval.subagent.result | SubAgentSearchResultPayload | SubAgentSearchResultMessage |
| graph.build.request | GraphBuildRequestPayload | GraphBuildRequestMessage |
| graph.build.result | GraphBuildResultPayload | GraphBuildResultMessage |
| graph.search.request | GraphSearchRequestPayload | GraphSearchRequestMessage |
| graph.search.result | GraphSearchResultPayload | GraphSearchResultMessage |
| a2a.task.created | A2ATaskCreatedPayload | A2ATaskCreatedMessage |
| a2a.task.complete | A2ATaskCompletePayload | A2ATaskCompleteMessage |
| handoff.request | HandoffRequestPayload | HandoffRequestMessage |

**Implementation:**
1. Go test (`internal/port/messagequeue/contract_test.go`): Marshal each struct to JSON, write to `testdata/contracts/*.json`
2. Python test (`workers/tests/test_nats_contracts.py`): Load each JSON fixture, validate against Pydantic model
3. Reverse: Python generates JSON from Pydantic, Go test unmarshals and validates
4. CI runs both directions on every commit

**Verification checklist per payload:**
- All required fields present
- Field names match exactly (Go json tags vs Python field names)
- Type compatibility (int64/int, float64/float, time.Time/datetime, UUID/str)
- Enum values match (status strings, trust levels)
- Nested structs deserialize correctly
- Empty/nil/null handling consistent
- tenant_id always present in tenant-scoped payloads

### B3: Unit Tests for Untested Critical Modules

**Goal:** Cover the modules with 0 tests that sit on critical paths.

**Python -- Agent Tools (13 files, `workers/codeforge/tools/`):**
- `tool_read.py` -- file reading with path validation, encoding handling
- `tool_write.py` -- file creation, overwrite protection, directory creation
- `tool_edit.py` -- diff-based editing, line range validation, conflict detection
- `tool_bash.py` -- command execution, timeout, output truncation, dangerous command detection
- `tool_search.py` -- content search with regex, file filtering
- `tool_glob.py` -- pattern matching, directory traversal
- `tool_listdir.py` -- directory listing, depth control
- Tool registry, tool base class, tool result formatting

**Python -- Consumer Dispatch (14 files, `workers/codeforge/consumer/`):**
- `_base.py` -- NATS connection, subscription setup, error handling
- `_conversation.py` -- agentic dispatch, simple chat dispatch, model resolution
- `_benchmark.py` -- benchmark type routing, LiteLLM readiness wait
- `_retrieval.py` -- index/search dispatch, result handling
- `_graph.py` -- graph build/search dispatch
- `_memory.py` -- store/recall dispatch
- `_a2a.py` -- A2A task handling
- `_handoff.py` -- handoff request processing
- Subject registration, duplicate detection, graceful shutdown

**Python -- Memory System (4 files, `workers/codeforge/memory/`):**
- `scorer.py` -- composite scoring (semantic + recency + importance)
- `experience.py` -- experience pool caching (@exp_cache decorator)
- Vector storage interface, recall ranking

**Go -- Critical Adapters (12 packages, 0 tests each):**
- `adapter/a2a/` -- A2A protocol HTTP handlers
- `adapter/lsp/` -- LSP server lifecycle management
- `adapter/otel/` -- OpenTelemetry provider setup
- `adapter/email/` -- Email feedback provider
- Agent backends: `openhands/`, `opencode/`, `plandex/`, `goose/`
- Infrastructure: `natskv/`, `ristretto/`

**Go -- Domain Models (untested, on critical paths):**
- `domain/conversation/` -- Conversation entity
- `domain/orchestration/` -- Handoff, orchestration models
- `domain/microagent/` -- Microagent matching logic
- `domain/memory/` -- Memory entity, scoring types
- `domain/skill/` -- Skill entity

---

## Layer A: Smoke Tests (Priority 2 -- Full Stack Verification)

### A1: Stack Health Verification

**Goal:** Verify all services start, connect, and respond within Docker Compose.

**Test file:** `tests/integration/smoke_test.go` (build tag: `//go:build smoke`)

**Checks:**
1. Go backend `/health` returns 200 with `dev_mode: true`
2. PostgreSQL pool active (>0 connections)
3. NATS JetStream connected, CODEFORGE stream exists with all 14 subject wildcards
4. LiteLLM proxy `/health` returns 200
5. Frontend dev server returns 200 on `/`
6. WebSocket upgrade succeeds on `/ws`
7. All NATS subscribers registered (check via NATS monitoring API)

### A2: Critical Flow Smoke Tests

**Goal:** Verify the 6 most important end-to-end flows through the real stack.

**Test file:** `tests/integration/flows_test.go` (build tag: `//go:build smoke`)

**Flow 1: Project Lifecycle**
```
POST /api/v1/projects {name, repo_url} -> 201
GET /api/v1/projects/{id} -> 200, matches created data
DELETE /api/v1/projects/{id} -> 204
GET /api/v1/projects/{id} -> 404
```

**Flow 2: Conversation (Simple Chat)**
```
POST /api/v1/projects/{id}/conversations -> 201 {conversation_id}
POST /api/v1/conversations/{id}/messages {content: "Hello"} -> 200 {run_id}
Poll GET /api/v1/conversations/{id}/messages until assistant response appears
Verify: response contains text, cost_usd >= 0, status = "completed"
```
Note: Requires LiteLLM with at least one configured model. Use cheapest available or mock.

**Flow 3: Conversation (Agentic with Tool Use)**
```
POST /api/v1/conversations/{id}/messages {content: "List files in the project root"}
Poll until run completes
Verify: tool_calls contain "ListDir" or "Glob", assistant summarizes results
Verify: AG-UI events received via WebSocket (run_started, tool_call, tool_result, run_finished)
```

**Flow 4: Benchmark Run**
```
POST /api/v1/benchmarks/runs {dataset: "basic-coding", model: "...", benchmark_type: "simple"}
Poll GET /api/v1/benchmarks/runs/{id} until status = "completed"
GET /api/v1/benchmarks/runs/{id}/results -> verify results array non-empty
Verify: each result has scores, cost_usd, tokens_in, tokens_out
```

**Flow 5: Retrieval + GraphRAG**
```
POST /api/v1/projects/{id}/index -> 200 (trigger indexing)
Poll GET /api/v1/projects/{id}/index until status = "ready"
POST /api/v1/projects/{id}/search {query: "config"} -> results with scores
POST /api/v1/projects/{id}/graph/build -> 200
Poll until graph status = "ready"
POST /api/v1/projects/{id}/graph/search {query: "config"} -> symbol results
```

**Flow 6: Memory Store + Recall**
```
POST /api/v1/projects/{id}/memories {content: "The config uses YAML format", type: "fact"}
POST /api/v1/projects/{id}/memories/recall {query: "config format"} -> matches stored memory
Verify: recalled memory contains "YAML", score > 0
```

### A3: CI Integration

**Goal:** Run smoke tests in GitHub Actions on every push to staging/main.

**Approach:** New CI job `smoke` that depends on `go` + `python` jobs passing first.

```yaml
smoke:
  name: Smoke Tests
  needs: [go, python]
  runs-on: ubuntu-latest
  services:
    postgres: ...
    nats: ...
  steps:
    - Start LiteLLM (docker run or mock)
    - Start Go backend (APP_ENV=development)
    - Start Python worker
    - Run: go test -tags=smoke -count=1 -timeout=300s ./tests/integration/...
```

**LLM dependency:** Smoke tests requiring LLM calls use one of:
- A free-tier model (Gemini Flash) with rate-limit tolerance
- A mock LLM server that returns canned responses
- Skip LLM-dependent tests if no API key configured (graceful degradation)

---

## Layer C: Feature Verification Matrix (Priority 3 -- Tracking)

### C1: Verification Matrix Document

**Goal:** A living document that tracks verification status of every feature.

**File:** `docs/feature-verification-matrix.md`

**Format:**

```markdown
| # | Feature | Phase | Go Unit | Py Unit | E2E | Contract | Smoke | Verified | Date |
|---|---------|-------|---------|---------|-----|----------|-------|----------|------|
| 1 | Project CRUD | 1 | PASS | -- | PASS | -- | PASS | YES | 2026-03-XX |
| 2 | Auth/JWT/RBAC | 10 | PASS | -- | PASS | -- | PASS | YES | 2026-03-XX |
| ... | ... | ... | ... | ... | ... | ... | ... | ... | ... |
```

**Feature list (30 features to verify):**

1. Project CRUD + Git Operations
2. Auth/JWT/RBAC + API Keys
3. Multi-Tenancy (tenant isolation on all queries)
4. LLM Model Registry + Auto-Discovery
5. LLM Key Management + Provider Config
6. Conversation (Simple Chat via NATS)
7. Conversation (Agentic Multi-Turn with Tools)
8. Agent Tools (Read, Write, Edit, Bash, Search, Glob, ListDir)
9. HITL Approval Flow (tool call -> WS permission_request -> approve/deny)
10. Policy Layer (first-match-wins, tool blocking, presets)
11. Modes System (21 built-in + custom YAML modes)
12. MCP Server Integration (registry, tool discovery, tool calls)
13. LSP Code Intelligence (per-language server lifecycle)
14. Retrieval (BM25 + Semantic Hybrid Search)
15. GraphRAG (dependency graph build + search)
16. RepoMap (tree-sitter code map generation)
17. Context Optimizer (token budget packing)
18. Memory System (vector store + semantic recall)
19. Experience Pool (caching successful runs)
20. Microagent System (trigger-based prompt injection)
21. Skills System (reusable Python snippets)
22. Cost Tracking (real-time cost extraction + aggregation)
23. Benchmark System (simple + tool_use + agent runners)
24. Evaluation Pipeline (GEMMAS, hybrid verification, multi-rollout)
25. Routing (ComplexityAnalyzer -> MAB -> LLMMetaRouter)
26. Orchestration (Plan -> Execute -> Review -> Deliver)
27. Trust Annotations (4 levels, auto-stamped on NATS)
28. Quarantine (risk scoring + admin review)
29. A2A Protocol (external agent federation)
30. Handoff (agent-to-agent task transfer)

### C2: Automated Verification Reporter

**Goal:** A script that runs all test suites and generates the matrix automatically.

**File:** `scripts/verify-features.sh`

**Logic:**
1. Run `go test ./internal/...` -- parse results per package
2. Run `pytest workers/tests/` -- parse results per module
3. Run `npx playwright test --config=playwright.config.ts` -- parse results per spec (if stack running)
4. Run contract tests -- parse pass/fail per payload type
5. Run smoke tests -- parse pass/fail per flow (if stack running)
6. Map test results to feature matrix
7. Output: markdown table + JSON summary + exit code (0 = all verified, 1 = gaps)

### C3: CI Gate

**Goal:** Block merges to main if critical features regress.

**Implementation:**
- Add verification reporter as CI step after smoke tests
- Define "critical features" (features 1-10, 22-23) that must be PASS
- Non-critical features log warnings but don't block
- Matrix output uploaded as CI artifact for review

---

## Execution Order

```
Week 1 (Foundation):
  B1: Fix 10 broken Postgres tests
  B2: NATS contract test infrastructure + first 10 payload types
  C1: Create feature verification matrix document

Week 2 (Coverage):
  B2: Remaining 15+ payload contract tests
  B3: Python agent tools tests (13 files)
  B3: Python consumer dispatch tests (14 files)
  B3: Python memory system tests (4 files)

Week 3 (Integration):
  A1: Stack health smoke tests
  A2: 6 critical flow smoke tests
  B3: Go adapter tests (A2A, LSP, OTEL)
  B3: Go domain model tests (conversation, orchestration, microagent)

Week 4 (Automation):
  A3: CI smoke test job (GitHub Actions)
  C2: Automated verification reporter script
  C3: CI gate for critical features
  C1: Update matrix with all results, mark verified features
```

## Success Criteria

- All 10 broken Go tests pass green
- 25+ NATS payload contract tests pass bidirectionally
- Agent tools have >80% line coverage
- Consumer dispatch has >70% line coverage
- 6 smoke test flows pass against real stack
- Feature verification matrix shows >80% features verified
- CI runs contract + smoke tests on every push
