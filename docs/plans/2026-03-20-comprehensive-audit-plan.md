# Comprehensive Codebase Audit — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Audit all 10 unaudited CodeForge subsystems (architecture + deep code review), produce scored reports, then fix all findings by severity.

**Architecture:** Sequential bottom-up audits — NATS (foundation) first, then layers that build on it. Each audit produces an independent report in `docs/audits/`. After all 10 audits, an overview aggregates findings, then fixes are applied via TDD.

**Tech Stack:** Go 1.25, Python 3.12, TypeScript/SolidJS, NATS JetStream, PostgreSQL 18, Docker Compose

**Spec:** `docs/specs/2026-03-20-comprehensive-audit-plan-design.md`

**Reference audits (format template):**
- `docs/audits/2026-03-18-schema-audit.md` — best example of severity table + findings format
- `docs/audits/agent-prompt-system-audit.md` — best example of inventory tables

---

## Report Template

Every audit report MUST follow this structure:

```markdown
# [System Name] Audit Report

**Date:** YYYY-MM-DD
**Scope:** Architecture + Code Review
**Files Reviewed:** N files
**Score: XX/100 — Grade: X**

> [Warning if score < 60]

---

## Executive Summary

| Severity | Count | Category Breakdown |
|----------|------:|---------------------|
| CRITICAL | N     | ...                 |
| HIGH     | N     | ...                 |
| MEDIUM   | N     | ...                 |
| LOW      | N     | ...                 |
| **Total**| **N** |                     |

### Positive Findings
- [What is done well]

---

## Architecture Review
### [Pattern/Topic]
[Analysis]

---

## Code Review Findings

### [SEVERITY]-NNN: [Title]
- **File:** `path/to/file.go:42`
- **Description:** [What is wrong]
- **Impact:** [What could happen]
- **Recommendation:** [How to fix]

---

## Summary & Recommendations
[Prioritized action items]
```

**Scoring:** Start at 100. Deductions: CRITICAL -15, HIGH -5, MEDIUM -2, LOW -1. Minimum 0. Grading: A (90+), B (75-89), C (60-74), D (45-59), F (<45).

**Severity criteria:**
- CRITICAL: Security vulnerability, data loss, production crash
- HIGH: Functional bug, missing validation, inconsistent state
- MEDIUM: Code smell, performance issue, pattern violation
- LOW: Naming, style, minor inconsistency

---

## Task 1: NATS Integration Audit

**Files to review:**
- `internal/port/messagequeue/queue.go` — Subject constants, Queue interface
- `internal/port/messagequeue/schemas.go` — Go-side JSON payload structs
- `internal/port/messagequeue/validator.go` — Payload validation
- `internal/port/messagequeue/contract_test.go` — Contract tests
- `internal/port/messagequeue/schemas_test.go` — Schema tests
- `internal/adapter/nats/nats.go` — JetStream StreamConfig & ConsumerConfig
- `workers/codeforge/consumer/_subjects.py` — Python-side subject constants
- `workers/codeforge/consumer/__main__.py` — Consumer entrypoint
- `workers/codeforge/models.py` — Python-side Pydantic payload models
- Output: `docs/audits/2026-03-20-nats-integration-audit.md`

- [ ] **Step 1: Inventory NATS files**

Use Glob to find all NATS-related files:
```
internal/port/messagequeue/**
internal/adapter/nats/**
workers/codeforge/consumer/_subjects.py
workers/codeforge/models.py
```
List every file with line count and purpose. Record in scratch notes.

- [ ] **Step 2: Architecture Review — Subject & Stream Design**

Read `internal/port/messagequeue/queue.go` and `workers/codeforge/consumer/_subjects.py`.

Check for:
- Do Go subjects match Python subjects EXACTLY? (CLAUDE.md: "Subjects must match EXACTLY")
- Are all subjects covered by JetStream wildcard filters in `internal/adapter/nats/nats.go`?
- Are new subject prefixes (e.g. `benchmark.>`, `review.>`) included in stream config?
- Is there a single source of truth for subject names?

- [ ] **Step 3: Architecture Review — JSON Contract Alignment**

Read `internal/port/messagequeue/schemas.go` and `workers/codeforge/models.py`.

Check for:
- Do Go JSON tags match Python Pydantic field names exactly?
- Type mapping: Go `int64`/`float64`/`time.Time` <-> Python `int`/`float`/`datetime`
- Are there any fields present in one side but not the other?
- Are there contract tests that verify round-trip serialization?

- [ ] **Step 4: Code Review — Go Publisher Side**

Read `internal/adapter/nats/nats.go` fully.

Check for:
- JetStream config: AckWait, MaxAckPending, MaxDeliver settings
- Dead letter queue configuration
- Error handling on publish failures
- Connection resilience (reconnect, circuit breaker)
- Idempotency keys on publish

- [ ] **Step 5: Code Review — Python Consumer Side**

Read `workers/codeforge/consumer/__main__.py` fully.

Check for:
- Message acknowledgment patterns (always ack, even on error?)
- Idempotency guards (skip if already completed?)
- Error handling (`except Exception as exc:` not bare `except:`)
- Graceful shutdown
- Consumer group / durable name configuration

- [ ] **Step 6: Code Review — Payload Validation**

Read `internal/port/messagequeue/validator.go` and `contract_test.go`.

Check for:
- Are all payload types validated before publish?
- Do contract tests cover all subject/payload combinations?
- Edge cases: empty payloads, missing required fields, invalid tenant_id

- [ ] **Step 7: Write audit report**

Create `docs/audits/2026-03-20-nats-integration-audit.md` using the Report Template above. Include all findings with file:line references. Calculate score.

- [ ] **Step 8: Commit**

```bash
git add docs/audits/2026-03-20-nats-integration-audit.md
git commit -m "audit: NATS integration — architecture + code review"
git push
```

---

## Task 2: Go Core Service Audit

**Files to review:**
- `internal/adapter/http/` — ~25 handler files, middleware, helpers, CRUD
- `internal/service/` — all service files (conversation, orchestration, benchmark, etc.)
- `internal/domain/` — all domain model files
- `internal/adapter/postgres/` — ~39 store files (5 critical + spot-check)
- Output: `docs/audits/2026-03-20-go-core-service-audit.md`

- [ ] **Step 1: Inventory Go Core files**

Use Glob to find all files:
```
internal/adapter/http/*.go
internal/service/*.go
internal/domain/**/*.go
internal/adapter/postgres/store*.go
```
List every file with line count, group by layer (handler/service/domain/store).

- [ ] **Step 2: Architecture Review — Hexagonal Architecture Compliance**

Read representative files from each layer. Check:
- Do handlers only call services (not stores directly)?
- Do services depend on ports/interfaces (not concrete adapters)?
- Are domain models free of infrastructure concerns?
- Is the dependency direction correct: handler -> service -> port <- adapter?

- [ ] **Step 3: Architecture Review — Service Layer Patterns**

Read key service files:
- `internal/service/conversation.go`
- `internal/service/orchestrator.go`
- `internal/service/benchmark.go`
- `internal/service/context_budget.go`

Check for:
- Service size (god services?)
- Error handling patterns (custom error types vs. generic)
- Context propagation (tenant, cancellation)
- Consistent patterns across services

- [ ] **Step 4: Code Review — HTTP Handlers**

Read all `internal/adapter/http/handlers_*.go` files.

Check for:
- Input validation on request bodies
- Consistent error response format
- Proper HTTP status codes
- Tenant isolation (`tenantFromCtx`)
- Missing authorization checks
- SQL injection via string interpolation (must use `$N` placeholders)

- [ ] **Step 5: Code Review — Middleware**

Read `internal/adapter/http/middleware.go`.

Check for:
- Auth middleware coverage (all routes protected?)
- Tenant context injection
- CORS configuration
- Request logging
- Rate limiting

- [ ] **Step 6: Code Review — Critical Stores**

Read these 5 stores in full:
- `internal/adapter/postgres/store_conversation.go`
- `internal/adapter/postgres/store_project.go`
- `internal/adapter/postgres/store_agent.go`
- `internal/adapter/postgres/store_user.go`
- `internal/adapter/postgres/store_tenant.go`

Check for:
- `AND tenant_id = $N` on ALL tenant-scoped queries
- Parameterized queries (no `fmt.Sprintf` for SQL)
- Proper error handling (no swallowed errors)
- Connection/transaction management
- Spot-check 10+ remaining stores for same patterns

- [ ] **Step 7: Code Review — Domain Models**

Read `internal/domain/` files.

Check for:
- Proper encapsulation (exported vs unexported fields)
- Validation on construction
- No infrastructure leaks (no SQL, no HTTP in domain)
- Consistent ID types

- [ ] **Step 8: Write audit report**

Create `docs/audits/2026-03-20-go-core-service-audit.md` using the Report Template. Include all findings with file:line references. Calculate score.

- [ ] **Step 9: Commit**

```bash
git add docs/audits/2026-03-20-go-core-service-audit.md
git commit -m "audit: Go Core Service — architecture + code review"
git push
```

---

## Task 3: Python Workers Audit

**Files to review:**
- `workers/codeforge/agent_loop.py` — main agent loop
- `workers/codeforge/plan_act.py` — Plan/Act mode
- `workers/codeforge/tools/` — 10+ agent tools
- `workers/codeforge/memory/` — scorer, storage, experience
- `workers/codeforge/evaluation/` — benchmark pipeline
- `workers/codeforge/trust/` — trust scoring
- `workers/codeforge/graphrag.py` — GraphRAG
- `workers/codeforge/qualitygate.py` — quality gate
- `workers/codeforge/consumer/` — 17 NATS consumer mixins
- Output: `docs/audits/2026-03-20-python-workers-audit.md`

- [ ] **Step 1: Inventory Python Worker files**

Use Glob:
```
workers/codeforge/**/*.py
```
List every file with line count. Group by subsystem (consumer, tools, memory, evaluation, routing, trust).

- [ ] **Step 2: Architecture Review — Module Structure**

Check for:
- Clear module boundaries (consumer vs tools vs memory vs evaluation)
- Circular imports
- Dependency direction (tools depend on nothing, consumer depends on tools)
- Consistent patterns across modules

- [ ] **Step 3: Code Review — Agent Loop**

Read `workers/codeforge/agent_loop.py` and `workers/codeforge/plan_act.py` fully.

Check for:
- Loop termination conditions (MaxLoopIterations, budget, timeout)
- Error handling in LLM calls
- Streaming correctness
- Token counting accuracy
- Stall detection logic

- [ ] **Step 4: Code Review — Tools**

Read all files in `workers/codeforge/tools/`.

Check for:
- Input validation/sanitization (command injection in bash tool?)
- Path traversal protection (read_file, edit_file)
- Consistent error handling (`_error_handler.py` usage)
- Tool result size limits
- Permission checks before execution

- [ ] **Step 5: Code Review — Consumer Mixins**

Read all 18 mixin files in `workers/codeforge/consumer/` (plus `__init__.py`, `__main__.py`).

Check for:
- Message acknowledgment (always `msg.ack()`)
- Idempotency guards
- Error handling (`except Exception as exc:`, not bare)
- Tenant ID propagation from NATS payload
- Consistent patterns across mixins

- [ ] **Step 6: Code Review — Memory, GraphRAG, Quality Gate**

Read:
- `workers/codeforge/memory/scorer.py`, `storage.py`, `experience.py`
- `workers/codeforge/graphrag.py`
- `workers/codeforge/qualitygate.py`

Check for:
- Memory scoring correctness (semantic + recency + importance)
- GraphRAG query safety
- Quality gate bypass conditions
- Error handling on external calls

- [ ] **Step 7: Code Review — Evaluation Pipeline**

Read key files in `workers/codeforge/evaluation/`.

Check for:
- Evaluator plugin interface consistency
- Runner isolation (do runs interfere with each other?)
- Result persistence
- Timeout handling

- [ ] **Step 8: Write audit report**

Create `docs/audits/2026-03-20-python-workers-audit.md` using the Report Template. Calculate score.

- [ ] **Step 9: Commit**

```bash
git add docs/audits/2026-03-20-python-workers-audit.md
git commit -m "audit: Python Workers — architecture + code review"
git push
```

---

## Task 4: Security (Code-Level) Audit

**Files to review (cross-cutting):**
- `internal/adapter/http/handlers_auth.go` — authentication
- `internal/adapter/http/handlers_auth_test.go` — auth tests
- `internal/adapter/http/middleware.go` — auth/tenant middleware
- `internal/adapter/http/middleware_test.go` — middleware tests
- `internal/adapter/postgres/store*.go` — tenant isolation in ALL queries
- `workers/codeforge/trust/` — trust scoring (levels, middleware, scorer)
- `internal/domain/policy/` — policy layer
- `internal/adapter/http/handlers_llm_keys.go` — API key management
- `internal/adapter/copilot/client.go` — Copilot token exchange
- NATS payloads — `tenant_id` in all messages
- Output: `docs/audits/2026-03-20-security-code-level-audit.md`

- [ ] **Step 1: Inventory security-relevant files**

Use Grep to find all auth/security/tenant references:
```
grep -r "tenant" internal/ --include="*.go" -l
grep -r "auth\|jwt\|token\|password" internal/ --include="*.go" -l
grep -r "tenant_id\|trust" workers/ --include="*.py" -l
```
Cross-reference with findings from Audits 1-3.

- [ ] **Step 2: Auth Flow Review**

Read `internal/adapter/http/handlers_auth.go` fully.

Check for:
- Password hashing (bcrypt/argon2, not plain/MD5/SHA)
- JWT token generation (signing algorithm, expiry, claims)
- Refresh token rotation
- Brute force protection (rate limiting, account lockout)
- Session invalidation on password change

- [ ] **Step 3: Middleware Security Review**

Read `internal/adapter/http/middleware.go` fully.

Check for:
- Are ALL routes behind auth middleware? (list unprotected routes)
- Tenant extraction from JWT — can it be spoofed?
- CORS: is `Access-Control-Allow-Origin: *` used? (should be restricted)
- CSRF protection
- Security headers (X-Content-Type-Options, X-Frame-Options, etc.)

- [ ] **Step 4: Tenant Isolation Audit**

Read ALL `internal/adapter/postgres/store*.go` files.

For EACH query, verify:
- `AND tenant_id = $N` present (with `tenantFromCtx(ctx)`)
- No `fmt.Sprintf` for tenant_id (must be parameterized)
- No queries that cross tenant boundaries
- Exceptions documented (user/token/tenant management)

- [ ] **Step 5: NATS Payload Trust**

Review NATS message handling for:
- Does every NATS payload carry `tenant_id`?
- Is `tenant_id` from NATS payload used to set context? (`tenantctx.WithTenant`)
- Can a malicious worker inject a different `tenant_id`?
- Are payloads validated before processing?

- [ ] **Step 6: Input Validation & Injection**

Check all HTTP handlers for:
- SQL injection (string interpolation in queries)
- Command injection (user input in exec calls)
- Path traversal (user-supplied file paths)
- XSS (user content reflected in responses)
- SSRF (user-supplied URLs fetched server-side)

- [ ] **Step 7: API Key & Secret Management**

Read `internal/adapter/http/handlers_llm_keys.go` and `internal/adapter/copilot/client.go`.

Check for:
- Are API keys stored encrypted at rest?
- Are keys logged or exposed in error messages?
- Key rotation support
- LITELLM_MASTER_KEY handling

- [ ] **Step 8: Python Trust Layer**

Read `workers/codeforge/trust/levels.py`, `middleware.py`, `scorer.py`.

Check for:
- Trust level enforcement consistency
- Can trust be escalated without authorization?
- Quarantine bypass conditions

- [ ] **Step 9: Write audit report**

Create `docs/audits/2026-03-20-security-code-level-audit.md` using the Report Template. Calculate score.

- [ ] **Step 10: Commit**

```bash
git add docs/audits/2026-03-20-security-code-level-audit.md
git commit -m "audit: Security (Code-Level) — architecture + code review"
git push
```

---

## Task 5: API Contract Audit

**Files to review:**
- `internal/adapter/http/handlers_*.go` — all endpoint handlers (~25 files)
- `internal/adapter/http/crud.go` — generic CRUD
- `internal/adapter/http/helpers.go` — response helpers
- Output: `docs/audits/2026-03-20-api-contract-audit.md`

- [ ] **Step 1: Inventory all API endpoints**

Read all handler files. For each, extract:
- HTTP method + path
- Request body schema
- Response body schema
- HTTP status codes used
- Authentication required?

Create a complete endpoint inventory table.

- [ ] **Step 2: Consistency Review — Naming**

Check for:
- Consistent URL patterns (plural nouns, kebab-case?)
- Consistent parameter naming (camelCase vs snake_case in JSON)
- RESTful resource naming (no verb-in-URL anti-patterns)
- API prefix consistency (`/api/v1/` everywhere?)

- [ ] **Step 3: Consistency Review — Error Responses**

Check for:
- Consistent error response format (same JSON structure for all errors?)
- Correct HTTP status codes (400 vs 422, 401 vs 403, 404 vs 410)
- Error messages: informative without leaking internals
- Are validation errors structured (field-level errors)?

- [ ] **Step 4: Consistency Review — Pagination & Query Patterns**

Check for:
- Consistent pagination (cursor vs offset, parameter names)
- Filtering patterns (query params consistency)
- Sorting patterns
- Response envelope consistency (data/meta/pagination fields)

- [ ] **Step 5: Documentation vs Reality**

Compare actual endpoints against any API documentation:
- `docs/features/*.md` — documented endpoints
- Swagger/OpenAPI if exists
- Are there undocumented endpoints?
- Are there documented endpoints that don't exist?

- [ ] **Step 6: Write audit report**

Create `docs/audits/2026-03-20-api-contract-audit.md` using the Report Template. Calculate score.

- [ ] **Step 7: Commit**

```bash
git add docs/audits/2026-03-20-api-contract-audit.md
git commit -m "audit: API Contract — consistency review"
git push
```

---

## Task 6: Test Coverage Audit

**Files to review:**
- `internal/**/*_test.go` — Go unit/integration tests
- `workers/tests/` — Python unit tests
- `frontend/e2e/` — E2E tests (90+ .spec.ts files)
- `frontend/e2e/llm/` — LLM API-level tests (95 tests, 12 specs)
- `frontend/src/**/*.test.ts` — Frontend unit tests (~4 files)
- Output: `docs/audits/2026-03-20-test-coverage-audit.md`

- [ ] **Step 1: Inventory all test files**

Use Glob:
```
internal/**/*_test.go
workers/tests/**/*.py
frontend/e2e/**/*.spec.ts
frontend/src/**/*.test.ts
```
Count: total files, total test functions/cases per language.

- [ ] **Step 2: Coverage Gap Analysis — Go**

For each Go package with source files, check:
- Does a corresponding `_test.go` exist?
- What is tested: happy path only? Error cases? Edge cases?
- Are critical services (conversation, orchestrator, benchmark) well-tested?
- List packages with NO tests.

- [ ] **Step 3: Coverage Gap Analysis — Python**

For each Python module, check:
- Does a corresponding test file exist in `workers/tests/`?
- Are consumer mixins tested? Tools? Agent loop?
- Critical gap: is `agent_loop.py` tested?
- List modules with NO tests.

- [ ] **Step 4: Coverage Gap Analysis — Frontend**

Check:
- E2E coverage: which pages/flows are covered?
- Unit test coverage: which components/utilities are tested?
- Are stores tested?
- Are critical user flows (login, project creation, conversation) covered?

- [ ] **Step 5: Test Quality Assessment**

Sample 10-15 test files across all languages. For each:
- Table-driven tests? (Go)
- Mocks vs real dependencies?
- Assertion quality (specific vs vague)
- Edge case coverage (nil, empty, boundary values)
- Are tests testing behavior or implementation?

- [ ] **Step 6: Cross-reference with Audit 1-5 findings**

For each CRITICAL/HIGH finding from Audits 1-5:
- Is there a test that would have caught it?
- If not, flag as a test gap.

- [ ] **Step 7: Write audit report**

Create `docs/audits/2026-03-20-test-coverage-audit.md` using the Report Template. Calculate score.

- [ ] **Step 8: Commit**

```bash
git add docs/audits/2026-03-20-test-coverage-audit.md
git commit -m "audit: Test Coverage — gap analysis"
git push
```

---

## Task 7: Frontend Code Architecture Audit

**Files to review:**
- `frontend/src/features/` — feature components
- `frontend/src/features/*/` — co-located stores (commandStore.ts, notificationStore.ts, contextFilesStore.ts)
- `frontend/src/ui/` — shared UI components
- `frontend/src/lib/` — utilities, API layer
- Deep review: dashboard, project detail, conversation routes
- Output: `docs/audits/2026-03-20-frontend-architecture-audit.md`

- [ ] **Step 1: Inventory frontend files**

Use Glob:
```
frontend/src/features/**/*.tsx
frontend/src/features/**/*.ts
frontend/src/stores/**/*.ts
frontend/src/ui/**/*.tsx
frontend/src/lib/**/*.ts
```
Count files per directory, note largest files (>300 lines).

- [ ] **Step 2: Architecture Review — Component Patterns**

Read 3 representative feature components (dashboard, project, conversation).

Check for:
- Component size (>500 lines = flag)
- Single responsibility
- Prop drilling vs context/store usage
- Consistent component structure

- [ ] **Step 3: Architecture Review — State Management**

Read state management files (no central `stores/` dir — stores are co-located with features):
- `frontend/src/features/chat/commandStore.ts`
- `frontend/src/features/notifications/notificationStore.ts`
- `frontend/src/features/project/contextFilesStore.ts`
- `frontend/src/hooks/` — shared hooks
- Use Grep for `createStore\|createSignal` to find all state definitions.

Check for:
- SolidJS signal/store patterns (correct createSignal/createStore usage)
- Effect cleanup (onCleanup in createEffect)
- Derived state (createMemo where appropriate)
- Store granularity (god store vs focused stores)
- Are WebSocket connections properly managed?

- [ ] **Step 4: Code Review — API Layer**

Read `frontend/src/lib/` API-related files.

Check for:
- Consistent fetch wrapper usage (not raw fetch scattered)
- Error handling on API calls
- Type safety (response types match backend)
- Auth token injection
- Base URL configuration

- [ ] **Step 5: Code Review — TypeScript Strictness**

Check for:
- `any` usage (should be zero per CLAUDE.md)
- Missing type annotations on public interfaces
- Proper union types vs loose string types
- Null safety patterns

- [ ] **Step 6: Write audit report**

Create `docs/audits/2026-03-20-frontend-architecture-audit.md` using the Report Template. Calculate score.

- [ ] **Step 7: Commit**

```bash
git add docs/audits/2026-03-20-frontend-architecture-audit.md
git commit -m "audit: Frontend Code Architecture — architecture + code review"
git push
```

---

## Task 8: MCP/A2A/LSP Protocol Compliance Audit

**Files to review:**
- `internal/adapter/mcp/server.go`, `tools.go`, `resources.go`, `auth.go`, `server_test.go`
- `internal/adapter/a2a/agentcard.go`, `executor.go`, `taskstore.go`, `agentcard_test.go`
- `internal/adapter/lsp/client.go`, `jsonrpc.go`, `client_test.go`
- `workers/codeforge/mcp_workbench.py`, `mcp_models.py`
- `workers/codeforge/a2a_protocol.py`
- Output: `docs/audits/2026-03-20-protocol-compliance-audit.md`

- [ ] **Step 1: Inventory protocol files**

Use Glob to find all MCP, A2A, LSP files across Go and Python.

- [ ] **Step 2: MCP Server Review**

Read all `internal/adapter/mcp/` files.

Check for:
- JSON-RPC 2.0 compliance (proper request/response format)
- Tool registration completeness (are all advertised tools functional?)
- Resource registration (do `codeforge://` URIs resolve?)
- Auth: API key validation, policy glob matching
- Error handling per MCP spec

- [ ] **Step 3: MCP Python Client Review**

Read `workers/codeforge/mcp_workbench.py` and `mcp_models.py`.

Check for:
- Multi-server management
- BM25 tool recommendation correctness
- Discovery and bridging
- Error handling on server communication failures

- [ ] **Step 4: A2A Protocol Review**

Read all `internal/adapter/a2a/` files.

Check for:
- Agent Card builder: dynamic skills from modes
- Task lifecycle state machine (8 states per spec)
- Executor: task execution flow
- Task store: PostgreSQL persistence
- Test coverage in `agentcard_test.go`

- [ ] **Step 5: LSP Client Review**

Read `internal/adapter/lsp/client.go` and `jsonrpc.go`.

Check for:
- JSON-RPC 2.0 compliance
- LSP lifecycle (initialize, initialized, shutdown)
- Language server process management
- Error handling on server crashes

- [ ] **Step 6: Write audit report**

Create `docs/audits/2026-03-20-protocol-compliance-audit.md` using the Report Template. Calculate score.

- [ ] **Step 7: Commit**

```bash
git add docs/audits/2026-03-20-protocol-compliance-audit.md
git commit -m "audit: MCP/A2A/LSP Protocol Compliance — architecture + code review"
git push
```

---

## Task 9: Hybrid Routing Audit

**Files to review:**
- `workers/codeforge/routing/complexity.py` — ComplexityAnalyzer
- `workers/codeforge/routing/reward.py` — Reward tracker
- `workers/codeforge/routing/blocklist.py` — Model blocklist
- `workers/codeforge/routing/capabilities.py` — Model capabilities
- `workers/codeforge/routing/rate_tracker.py` — Rate limiting
- Output: `docs/audits/2026-03-20-hybrid-routing-audit.md`

- [ ] **Step 1: Inventory routing files**

Use Glob: `workers/codeforge/routing/**/*.py`. List all files.

- [ ] **Step 2: Architecture Review — 3-Layer Cascade**

Check the routing cascade design:
- Layer 1: ComplexityAnalyzer (rule-based, <1ms) — is it actually fast?
- Layer 2: MABModelSelector (UCB1) — correct UCB1 formula?
- Layer 3: LLMMetaRouter (cold-start) — when does it trigger?
- Fallback behavior when routing is disabled (`CODEFORGE_ROUTING_ENABLED=false`)

- [ ] **Step 3: Code Review — All Routing Files**

Read each file fully.

Check for:
- UCB1 exploration/exploitation balance
- Reward tracking accuracy (does it reflect actual model quality?)
- Blocklist: can a blocked model still be selected?
- Rate tracker: does it respect provider rate limits?
- Edge cases: no healthy models, all models blocked, cold start with no data

- [ ] **Step 4: Write audit report**

Create `docs/audits/2026-03-20-hybrid-routing-audit.md` using the Report Template. Calculate score.

- [ ] **Step 5: Commit**

```bash
git add docs/audits/2026-03-20-hybrid-routing-audit.md
git commit -m "audit: Hybrid Routing — architecture + code review"
git push
```

---

## Task 10: Docker/Infra Audit

**Files to review:**
- `docker-compose.yml` — Service definitions
- `Dockerfile` / `Dockerfile.*` — Build definitions
- `scripts/` — Helper scripts
- `.pre-commit-config.yaml` — Pre-commit hooks
- `.github/workflows/` — CI/CD (if exists)
- Output: `docs/audits/2026-03-20-docker-infra-audit.md`

- [ ] **Step 1: Inventory infra files**

Use Glob:
```
docker-compose*.yml
Dockerfile*
scripts/**
.pre-commit-config.yaml
.github/workflows/**
```

- [ ] **Step 2: Docker Compose Review**

Read `docker-compose.yml` fully.

Check for:
- Service dependency order (depends_on + healthcheck)
- Health check definitions for all services
- Volume mounts: are secrets exposed?
- Network configuration
- Resource limits (memory, CPU)
- Port exposure (which ports are exposed to host?)

- [ ] **Step 3: Dockerfile Review**

Read all Dockerfiles.

Check for:
- Multi-stage builds (minimize image size)
- Non-root user
- .dockerignore effectiveness
- Layer caching optimization
- Security: no secrets in build args/layers

- [ ] **Step 4: Scripts & CI Review**

Read all files in `scripts/`.

Check for:
- Script safety (set -euo pipefail)
- Hardcoded paths or secrets
- Are scripts documented (--help)?
- CI pipeline coverage (build, test, lint, deploy)

- [ ] **Step 5: Write audit report**

Create `docs/audits/2026-03-20-docker-infra-audit.md` using the Report Template. Calculate score.

- [ ] **Step 6: Commit**

```bash
git add docs/audits/2026-03-20-docker-infra-audit.md
git commit -m "audit: Docker/Infra — architecture + code review"
git push
```

---

## Task 11: Audit Overview Document

**Depends on:** Tasks 1-10 complete.
**Output:** `docs/audits/2026-03-20-audit-overview.md`

- [ ] **Step 1: Aggregate all scores**

Read all 10 audit reports. Create summary table:

```markdown
| System | Score | Grade | CRITICAL | HIGH | MEDIUM | LOW | Total |
|--------|-------|-------|----------|------|--------|-----|-------|
| NATS   | XX    | X     | N        | N    | N      | N   | N     |
| ...    |       |       |          |      |        |     |       |
```

- [ ] **Step 2: Identify cross-system findings**

Look for patterns that appear across multiple audits:
- Tenant isolation gaps (Go + Python + NATS)
- Error handling patterns (all layers)
- Input validation gaps (HTTP + Tools + NATS)
- Consistency issues (naming, patterns)

- [ ] **Step 3: Create prioritized fix list**

Sort ALL findings by:
1. Severity (CRITICAL first)
2. System priority (1-10) as tiebreaker

Assign fix IDs: `FIX-001`, `FIX-002`, etc.

- [ ] **Step 4: Write overview document**

Create `docs/audits/2026-03-20-audit-overview.md` with:
- Score summary table
- Cross-system findings
- Prioritized fix list with references to original findings

- [ ] **Step 5: Update docs/todo.md**

Add all findings as TODO items, grouped by severity.

- [ ] **Step 6: Commit**

```bash
git add docs/audits/2026-03-20-audit-overview.md docs/todo.md
git commit -m "audit: overview document — all 10 audits aggregated"
git push
```

---

## Task 12: Fix Phase — CRITICAL Findings

**Depends on:** Task 11 complete.
**Process:** TDD per fix, commit per fix.

- [ ] **Step 1: Read the prioritized fix list**

Read `docs/audits/2026-03-20-audit-overview.md`. Extract all CRITICAL findings.

- [ ] **Step 2: For each CRITICAL finding, apply TDD fix**

Per finding:

```
a) Read the original audit finding (file:line reference)
b) Write a failing test that reproduces the issue
c) Run the test — verify it fails
d) Write the minimal fix
e) Run the test — verify it passes
f) Run full test suite — verify no regressions
g) Commit: git commit -m "fix: [SYSTEM]-CRITICAL-NNN — [description]"
h) Update audit report: mark finding as FIXED
```

- [ ] **Step 3: Commit all report updates**

```bash
git add docs/audits/*.md
git commit -m "audit: mark all CRITICAL findings as FIXED"
git push
```

---

## Task 13: Fix Phase — HIGH Findings

**Depends on:** Task 12 complete.
**Process:** Same TDD process as Task 12 but for all HIGH findings.

- [ ] **Step 1: Extract all HIGH findings from overview**
- [ ] **Step 2: For each HIGH finding, apply TDD fix** (same a-h process as Task 12)
- [ ] **Step 3: Commit all report updates**

```bash
git add docs/audits/*.md
git commit -m "audit: mark all HIGH findings as FIXED"
git push
```

---

## Task 14: Fix Phase — MEDIUM Findings

**Depends on:** Task 13 complete.
**Process:** Same TDD process for all MEDIUM findings.

- [ ] **Step 1: Extract all MEDIUM findings from overview**
- [ ] **Step 2: For each MEDIUM finding, apply TDD fix**
- [ ] **Step 3: Commit all report updates**

```bash
git add docs/audits/*.md
git commit -m "audit: mark all MEDIUM findings as FIXED"
git push
```

---

## Task 15: Fix Phase — LOW Findings

**Depends on:** Task 14 complete.
**Process:** Same TDD process for all LOW findings.

- [ ] **Step 1: Extract all LOW findings from overview**
- [ ] **Step 2: For each LOW finding, apply TDD fix**
- [ ] **Step 3: Commit all report updates**

```bash
git add docs/audits/*.md
git commit -m "audit: mark all LOW findings as FIXED"
git push
```

---

## Task 16: Final Completion

**Depends on:** Tasks 12-15 complete.

- [ ] **Step 1: Recalculate all scores**

Re-read each audit report. Recalculate scores based on remaining (unfixed) findings. Update score/grade in each report header.

- [ ] **Step 2: Update overview document**

Update `docs/audits/2026-03-20-audit-overview.md` with:
- New scores (post-fix)
- Fix completion stats (N/N fixed per system)
- Remaining items (if any)

- [ ] **Step 3: Update docs/todo.md**

Mark all completed fix items as `[x]` with date.

- [ ] **Step 4: Final commit**

```bash
git add docs/
git commit -m "audit: all findings fixed, scores recalculated, audit complete"
git push
```
