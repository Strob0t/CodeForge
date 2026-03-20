# Comprehensive Codebase Audit Plan

**Date:** 2026-03-20
**Scope:** Full architecture + deep code review for all unaudited systems
**Approach:** Sequential (bottom-up), document-only, fixes after all audits complete

---

## Context

### Already Audited (4 existing audits)

| Audit | Date | File |
|---|---|---|
| Stub Tracker | 2026-03-17 | `docs/audits/stub-tracker.md` |
| UX/UI Audit | 2026-03-18 | `docs/audits/ux-ui-audit.md` |
| Schema Audit | 2026-03-18 | `docs/audits/2026-03-18-schema-audit.md` |
| Agent & Prompt System | 2026-03-20 | `docs/audits/agent-prompt-system-audit.md` |

### Unaudited Systems (10)

NATS Integration, Go Core Service, Python Workers, Security (Code-Level), API Contract, Test Coverage, Frontend Code Architecture, MCP/A2A/LSP Protocols, Hybrid Routing, Docker/Infra.

---

## Phase 1 — Audits (Document Only)

Sequential execution, bottom-up priority. Each system fully completed before moving to the next.

### Priority 1: NATS Integration

**Scope:** Subjects/Streams, Publisher (Go), Subscriber (Python `consumer/`), JSON Contracts (`schemas.go` <-> `models.py`), Error Handling, Dead Letters, Idempotency, AckWait/Redelivery.

**Key files:**
- `internal/port/messagequeue/queue.go` — Subject constants, Queue interface
- `internal/port/messagequeue/schemas.go` — Go-side JSON payload structs
- `internal/port/messagequeue/validator.go` — Payload validation
- `internal/port/messagequeue/contract_test.go` — Contract tests
- `internal/adapter/nats/nats.go` — JetStream StreamConfig & ConsumerConfig
- `workers/codeforge/consumer/_subjects.py` — Python-side subject constants
- `workers/codeforge/consumer/__main__.py` — Consumer entrypoint
- `workers/codeforge/models.py` — Python-side Pydantic payload models

**Why first:** NATS is the communication foundation. Issues here propagate to every other system. Understanding the message flow is prerequisite for auditing Go Core and Python Workers.

**Output:** `docs/audits/2026-03-20-nats-integration-audit.md`

---

### Priority 2: Go Core Service

**Scope:** HTTP Handlers (`handlers_*.go`), Middleware (auth, tenant, CORS), Services (`internal/service/*.go`), Domain Models (`internal/domain/`), CRUD/Store (`postgres/store*.go`), Prompt Assembly, Context Budget.

**Key directories:**
- `internal/adapter/http/` — ~25 handler files
- `internal/service/` — business logic
- `internal/domain/` — domain models
- `internal/adapter/postgres/` — data access (~39 store files)

**Critical stores (priority review):**
- `internal/adapter/postgres/store_conversation.go`
- `internal/adapter/postgres/store_project.go`
- `internal/adapter/postgres/store_agent.go`
- `internal/adapter/postgres/store_user.go`
- `internal/adapter/postgres/store_tenant.go`
- Remaining stores: pattern validation (spot-check 10+)

**Why second:** The heart of the system. Builds on NATS understanding. Required context for Security audit.

**Output:** `docs/audits/2026-03-20-go-core-service-audit.md`

---

### Priority 3: Python Workers

**Scope:** Agent Loop (`agent_loop.py`), Tools (10+ tools), Consumer Mixins, Memory/Scorer, GraphRAG, Evaluation Pipeline, Routing, Trust, Quality Gate, Plan/Act.

**Key directories & files:**
- `workers/codeforge/agent_loop.py` — main agent loop
- `workers/codeforge/plan_act.py` — Plan/Act mode
- `workers/codeforge/tools/` — 10+ agent tools
- `workers/codeforge/memory/` — composite memory (scorer, storage, experience)
- `workers/codeforge/evaluation/` — benchmark pipeline
- `workers/codeforge/routing/` — hybrid routing (also covered in Prio 9)
- `workers/codeforge/trust/` — trust scoring
- `workers/codeforge/graphrag.py` — GraphRAG integration
- `workers/codeforge/qualitygate.py` — quality gate
- `workers/codeforge/consumer/` — 17 NATS consumer mixins:
  - `_base.py`, `_conversation.py`, `_memory.py`, `_quality_gate.py`
  - `_repomap.py`, `_runs.py`, `_tasks.py`, `_handoff.py`
  - `_backend_health.py`, `_compact.py`, `_retrieval.py`, `_graph.py`
  - `_prompt_evolution.py`, `_a2a.py`, `_context.py`, `_subject.py`
  - `_benchmark.py`, `_review.py`

**Why third:** Builds on both NATS and Go Core understanding. The LLM execution layer.

**Output:** `docs/audits/2026-03-20-python-workers-audit.md`

---

### Priority 4: Security (Code-Level)

**Scope:** Auth Flow (login, JWT, refresh), RBAC, Tenant Isolation in Go queries + Python handlers, Policy Layer, API Key handling, NATS payload trust, Input Validation.

**Cross-cutting files:**
- `internal/adapter/http/handlers_auth.go` — authentication
- `internal/adapter/http/middleware.go` — auth/tenant middleware
- `internal/adapter/postgres/store*.go` — tenant isolation in queries
- `workers/codeforge/trust/` — trust scoring
- `internal/domain/policy/` — policy layer
- NATS payloads — `tenant_id` propagation

**Why fourth:** Cross-cutting concern. Benefits from full understanding of Go Core, Python Workers, and NATS acquired in Prio 1-3.

**Output:** `docs/audits/2026-03-20-security-code-level-audit.md`

---

### Priority 5: API Contract

**Scope:** REST API consistency (naming, error responses, HTTP status codes), endpoint documentation vs. reality, request/response schemas, pagination patterns.

**Key files:**
- `internal/adapter/http/handlers_*.go` — all endpoint handlers
- `internal/adapter/http/crud.go` — generic CRUD
- `internal/adapter/http/helpers.go` — response helpers

**Why fifth:** REST layer over Go Core, already audited. Focus on consumer-facing consistency.

**Output:** `docs/audits/2026-03-20-api-contract-audit.md`

---

### Priority 6: Test Coverage

**Scope:** Inventory all existing tests, identify coverage gaps, assess test quality (mocks vs. integration), cross-reference edge cases from Audits 1-5.

**Key directories:**
- `internal/**/*_test.go` — Go unit/integration tests
- `workers/tests/` — Python unit tests
- `frontend/e2e/` — E2E tests (60+ `.spec.ts` files, primary test layer)
- `frontend/e2e/llm/` — LLM API-level tests (95 tests, 12 specs)
- `frontend/src/**/*.test.ts` — Frontend unit tests (limited, ~4 files)

**Why sixth:** All systems known by now. Can identify blind spots informed by findings from Prio 1-5.

**Output:** `docs/audits/2026-03-20-test-coverage-audit.md`

---

### Priority 7: Frontend Code Architecture

**Scope:** Component architecture, state management patterns, API layer, error handling, TypeScript strictness.

**Key directories:**
- `frontend/src/features/` — feature components
- `frontend/src/stores/` — state management
- `frontend/src/ui/` — shared UI components
- `frontend/src/lib/` — utilities, API layer

**Scope details:**
- Component patterns: size, composition, reusability
- State management: store hygiene, signal/effect lifecycle
- API layer: fetch wrapper, error handling, type safety
- Representative routes for deep review: dashboard, project detail, conversation (3 of 13)
- Remaining routes: pattern validation (spot-check)

**Why seventh:** Frontend is consumer-facing but lower risk than backend systems.

**Output:** `docs/audits/2026-03-20-frontend-architecture-audit.md`

---

### Priority 8: MCP/A2A/LSP Protocols

**Scope:** MCP Server (`adapter/mcp/`), A2A (`adapter/a2a/`), LSP Client (`adapter/lsp/`), protocol compliance, edge cases.

**Key files:**
- `internal/adapter/mcp/server.go`, `tools.go`, `resources.go`, `auth.go`, `server_test.go`
- `internal/adapter/a2a/agentcard.go`, `executor.go`, `taskstore.go`, `agentcard_test.go`
- `internal/adapter/lsp/client.go`, `jsonrpc.go`, `client_test.go`
- `workers/codeforge/mcp_workbench.py`, `mcp_models.py`
- `workers/codeforge/a2a_protocol.py`

**Why eighth:** Specialized protocol implementations, medium risk.

**Output:** `docs/audits/2026-03-20-protocol-compliance-audit.md`

---

### Priority 9: Hybrid Routing

**Scope:** ComplexityAnalyzer, MAB/UCB1 selector, LLMMetaRouter, Reward Tracker, Blocklist, Capabilities.

**Key files:**
- `workers/codeforge/routing/complexity.py`
- `workers/codeforge/routing/reward.py`
- `workers/codeforge/routing/blocklist.py`
- `workers/codeforge/routing/capabilities.py`
- `workers/codeforge/routing/rate_tracker.py`

**Why ninth:** Specialized subsystem, isolated scope.

**Output:** `docs/audits/2026-03-20-hybrid-routing-audit.md`

---

### Priority 10: Docker/Infra

**Scope:** Compose config, networking, health checks, startup order, volume mounts, secrets handling.

**Key files:**
- `docker-compose.yml`
- `Dockerfile` / `Dockerfile.*`
- `litellm-config.yaml` (runtime volume-mounted via Docker Compose, not in repo)
- `scripts/` — helper scripts

**Why tenth:** Lowest risk, mostly configuration.

**Output:** `docs/audits/2026-03-20-docker-infra-audit.md`

---

## Phase 2 — Fix Phase (After All Audits Complete)

### Step 1: Overview Document

Create `docs/audits/2026-03-20-audit-overview.md`:
- All scores at a glance
- All findings aggregated by severity
- Cross-system findings marked
- Prioritized fix list

### Step 2: Fix Order

Strictly by severity, system priority breaks ties:

1. All CRITICAL findings across all systems
2. All HIGH findings across all systems
3. All MEDIUM findings across all systems
4. All LOW findings across all systems

### Step 3: Fix Execution

Per fix:
1. **TDD** — Write test that reproduces the problem
2. **Fix** — Minimum code to pass the test
3. **Commit** — Reference finding ID (`fix: NATS-CRITICAL-001 — ...`)
4. **Update report** — Mark finding as FIXED in audit report

### Step 4: Completion

- All reports updated with fix status
- Scores recalculated
- `docs/todo.md` updated

---

## Audit Methodology

### Process (per system)

1. **Inventory** — List all relevant files, map dependencies
2. **Architecture Review** — Patterns, interfaces, layer separation, CLAUDE.md compliance
3. **Code Review** — File by file, categorize findings (Security, Bug, Code Smell, Performance, Consistency)
4. **Report** — Write findings + score + recommendations to `docs/audits/`

### Severity Criteria

| Severity | Definition |
|---|---|
| CRITICAL | Security vulnerability, data loss, production crash |
| HIGH | Functional bug, missing validation, inconsistent state |
| MEDIUM | Code smell, performance issue, pattern violation |
| LOW | Naming, style, minor inconsistency |

### Scoring

- Start at 100 points
- Deductions: CRITICAL -15, HIGH -5, MEDIUM -2, LOW -1
- Minimum: 0
- Grading: A (90+), B (75-89), C (60-74), D (45-59), F (<45)

### Output Format

Each audit report follows the established format:
- Executive Summary with severity table
- Positive Findings section
- Architecture Review section
- Code Review Findings (with file:line references)
- Summary & Recommendations

Consistent with existing audits in `docs/audits/`.
