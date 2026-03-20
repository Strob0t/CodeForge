# Comprehensive Audit Overview

**Date:** 2026-03-20
**Scope:** 10 individual audits aggregated
**Total Files Reviewed:** ~1,100 files across Go, Python, TypeScript, Docker, and CI/CD
**Overall Grade: C (66.0/100 average)**

---

## 1. Score Summary Table

| # | System | Score | Grade | CRITICAL | HIGH | MEDIUM | LOW | Total |
|---|--------|------:|:-----:|:--------:|:----:|:------:|:---:|------:|
| 1 | NATS Integration | 68 | C | 1 | 3 | 5 | 3 | 12 |
| 2 | Go Core Service | 74 | C | 2 | 5 | 4 | 2 | 13 |
| 3 | Python Workers | 72 | C | 1 | 5 | 4 | 4 | 14 |
| 4 | Security (Code-Level) | 62 | C | 2 | 3 | 5 | 4 | 14 |
| 5 | API Contract | 72 | C | 1 | 4 | 5 | 4 | 14 |
| 6 | Test Coverage | 38 | F | 4 | 5 | 5 | 3 | 17 |
| 7 | Frontend Architecture | 78 | B | 0 | 3 | 4 | 3 | 10 |
| 8 | Protocol Compliance (MCP/A2A/LSP) | 72 | C | 1 | 2 | 4 | 3 | 10 |
| 9 | Hybrid Routing | 72 | C | 1 | 3 | 2 | 2 | 8 |
| 10 | Docker/Infra | 72 | C | 1 | 3 | 3 | 3 | 10 |
| | **TOTALS** | **660** | **C** | **14** | **36** | **41** | **31** | **122** |

**Average Score: 66.0/100**

---

## 2. Cross-System Findings

### 2.1 Tenant Isolation Gaps (5 systems affected)

Tenant isolation is the most pervasive cross-system issue, appearing in five separate audits. The following table maps every tenant isolation finding:

| Finding ID | System | File(s) | Description |
|------------|--------|---------|-------------|
| Go Core CRITICAL-001 | Go Core | `store_agent_identity.go` | 5 queries missing `AND tenant_id` |
| Go Core CRITICAL-002 | Go Core | `store_active_work.go` | `ReleaseStaleWork` cross-tenant |
| Go Core HIGH-001 | Go Core | `handlers.go:263` | `autoIndexProject` uses `context.Background()`, losing tenant context |
| Go Core HIGH-002 | Go Core | `store_user.go` | `GetUser`, `UpdateUser`, `DeleteUser` missing tenant filter |
| Go Core HIGH-003 | Go Core | `store_conversation.go` | `ListMessages` no tenant JOIN |
| Security CRITICAL-002 | Security | `store_agent_identity.go` | Confirmed cross-audit (same as Go Core CRITICAL-001) |
| Security HIGH-001 | Security | `store_active_work.go` | Confirmed cross-audit (same as Go Core CRITICAL-002) |
| Security HIGH-002 | Security | `store_api_key.go` | API key lookup globally scoped |
| Security HIGH-003 | Security | `schemas.go` | NATS payloads trust tenant_id without verification |
| Security MEDIUM-005 | Security | `store_user.go` | Confirmed cross-audit (same as Go Core HIGH-002) |
| NATS HIGH-003 | NATS | `schemas.go:44-60` | `RunStartPayload` missing `tenant_id` field |
| Python HIGH-001 | Python | `memory/storage.py:86` | `MemoryStore.recall()` missing `tenant_id` filter |
| Test Coverage CRITICAL-002 | Test Coverage | Multiple store files | No tenant isolation tests exist for any of the above |

**Root Cause:** The tenant isolation pattern (`AND tenant_id = $N` with `tenantFromCtx(ctx)`) is well-established in the primary store file (`store.go`) but was not consistently applied to secondary store files added later. There is no automated check (linter rule or CI gate) that enforces tenant scoping on new queries.

**Recommendation:** Create a SQL linter or code review checklist that flags any `WHERE` clause in `store_*.go` files lacking a `tenant_id` predicate. Add integration tests for all store files using the existing `ctxWithTenant` test pattern.

### 2.2 Error Handling Inconsistencies (3 systems affected)

| Finding ID | System | Issue |
|------------|--------|-------|
| API Contract CRITICAL-001 | API | Search endpoints leak `err.Error()` to clients |
| API Contract MEDIUM-005 | API | `EvaluateStep` leaks internal error details |
| API Contract HIGH-002 | API | `http.Error()` returns plain text instead of JSON |
| Protocol MEDIUM-003 | Protocol | A2A executor silences `json.Marshal` errors |
| Python LOW-003 | Python | `_benchmark.py` uses `except Exception:` with no logging |
| NATS MEDIUM-005 | NATS | Validator accepts empty JSON objects as valid |

**Pattern:** Error handling is generally good (consistent `writeDomainError` mapping, `except Exception as exc:` with logging), but exceptions leak through at the edges -- search endpoints, benchmark handlers, and protocol adapters.

### 2.3 Input Validation Gaps (4 systems affected)

| Finding ID | System | Issue |
|------------|--------|-------|
| Python CRITICAL-001 | Python | Bash tool has zero command sanitization |
| Python HIGH-002 | Python | Glob tool missing path traversal protection |
| Python HIGH-003 | Python | Quality gate runs arbitrary shell commands |
| NATS HIGH-001 | NATS | Validator covers only 14 of 50+ subjects |
| NATS MEDIUM-005 | NATS | Empty JSON objects pass validation |
| Security CRITICAL-001 | Security | Password reset token logged in plaintext |
| Docker CRITICAL-001 | Docker | SQL injection in restore script |
| API Contract HIGH-004 | API | No pagination on most list endpoints (unbounded results) |

**Pattern:** Input validation is strongest at the Go HTTP layer (parameterized SQL, `MaxBytesReader`, domain validation) but weakest at the Python tool execution layer and the NATS message validation layer. The bash tool's lack of defense-in-depth command filtering is the highest-impact single finding.

### 2.4 Missing Test Coverage for Critical Findings (cross-cutting)

The Test Coverage audit (score: 38/100, the only F-grade system) reveals that 11 of 12 CRITICAL/HIGH findings from the other audits have no test that would have caught them:

| Critical Finding | Source Audit | Has Test? |
|-----------------|-------------|-----------|
| Tenant bypass in `store_agent_identity.go` | Go Core, Security | NO |
| Tenant bypass in `store_active_work.go` | Go Core, Security | NO |
| Bash tool command injection | Python Workers | NO |
| NATS no reconnect config | NATS Integration | NO |
| Password reset token in logs | Security | NO |
| Memory tenant isolation gap | Python Workers | NO |
| Internal error leakage in search | API Contract | NO |
| PathValue vs chi.URLParam mismatch | Go Core, API | Partial |

### 2.5 PathValue vs chi.URLParam Mismatch (2 systems affected)

Both the Go Core audit (HIGH-004) and API Contract audit (HIGH-001) independently identified the same bug: 7 benchmark handler functions use `r.PathValue("id")` instead of `chi.URLParam(r, "id")`. This is a concrete example of the cross-system pattern -- a bug that was found by two different auditors examining the same code from different angles (architecture vs contract compliance).

### 2.6 Configuration/Documentation Drift (3 systems affected)

| Finding ID | System | Issue |
|------------|--------|-------|
| Routing H1 | Routing | Cascade layer ordering contradicts documentation |
| Routing H3 | Routing | Default `enabled=True` contradicts docs saying `false` |
| Routing H2 | Routing | 9 config fields not loaded from YAML/env (dead features) |
| Protocol MEDIUM-001 | Protocol | LSP adapter is 700+ lines of dead code (never imported) |
| Docker H2 | Docker | Hardcoded private IP in litellm/config.yaml |

---

## 3. Prioritized Fix List

All 122 findings from all 10 audits, deduplicated and sorted by severity then system priority. Cross-audit duplicates are noted with "XREF" and counted once.

### CRITICAL (14 findings, 10 unique after dedup)

| FIX ID | Severity | System | Finding ID | Title | File(s) |
|--------|----------|--------|------------|-------|---------|
| FIX-001 | CRITICAL | Go Core | CRITICAL-001 | Missing tenant isolation in agent identity store (5 queries) | `internal/adapter/postgres/store_agent_identity.go` |
| FIX-002 | CRITICAL | Go Core | CRITICAL-002 | `ReleaseStaleWork` cross-tenant data modification | `internal/adapter/postgres/store_active_work.go` |
| FIX-003 | CRITICAL | Security | CRITICAL-001 | Password reset token logged in plaintext (all environments) | `internal/adapter/http/handlers_auth.go:386` |
| FIX-004 | CRITICAL | Python | CRITICAL-001 | Bash tool has zero command sanitization (defense-in-depth gap) | `workers/codeforge/tools/bash.py:74` |
| FIX-005 | CRITICAL | NATS | CRITICAL-001 | No NATS reconnect configuration (silent permanent disconnect) | `internal/adapter/nats/nats.go:39` |
| FIX-006 | CRITICAL | API Contract | CRITICAL-001 | Internal error message leakage in search endpoints | `internal/adapter/http/handlers_search.go:43,101` |
| FIX-007 | CRITICAL | Protocol | CRITICAL-001 | MCP server listens without authentication (port 3001) | `internal/adapter/mcp/server.go:70` |
| FIX-008 | CRITICAL | Routing | C1 | Reward computation uses hardcoded defaults instead of active config | `workers/codeforge/agent_loop.py:1179` |
| FIX-009 | CRITICAL | Docker | C1 | SQL injection in restore-postgres.sh | `scripts/restore-postgres.sh:37` |
| FIX-010 | CRITICAL | Test Coverage | CRITICAL-001 | Database store layer nearly untested (32 of 36 files) | `internal/adapter/postgres/store_*.go` |
| FIX-011 | CRITICAL | Test Coverage | CRITICAL-002 | No tenant isolation tests for critical store operations | Multiple store files |
| FIX-012 | CRITICAL | Test Coverage | CRITICAL-003 | No command injection tests for bash tool | `workers/tests/test_tool_bash.py` |
| FIX-013 | CRITICAL | Test Coverage | CRITICAL-004 | No NATS reconnect/resilience tests | `internal/adapter/nats/nats_test.go` |
| -- | CRITICAL | Security | CRITICAL-002 | XREF: Same as Go Core CRITICAL-001 (FIX-001) | -- |

### HIGH (36 findings, 30 unique after dedup)

| FIX ID | Severity | System | Finding ID | Title | File(s) |
|--------|----------|--------|------------|-------|---------|
| FIX-014 | HIGH | Go Core | HIGH-001 | `autoIndexProject` loses tenant context via `context.Background()` | `internal/adapter/http/handlers.go:263` |
| FIX-015 | HIGH | Go Core | HIGH-002 | Missing tenant isolation in `GetUser`, `UpdateUser`, `DeleteUser` | `internal/adapter/postgres/store_user.go` |
| FIX-016 | HIGH | Go Core | HIGH-003 | Missing tenant isolation in `ListMessages` | `internal/adapter/postgres/store_conversation.go:97` |
| FIX-017 | HIGH | Go Core | HIGH-004 | `r.PathValue` vs `chi.URLParam` mismatch (7 occurrences) | `internal/adapter/http/handlers_benchmark.go` |
| FIX-018 | HIGH | NATS | HIGH-001 | Validator covers only 14 of 50+ subjects | `internal/port/messagequeue/validator.go` |
| FIX-019 | HIGH | NATS | HIGH-002 | Type mismatch int64 vs int for token counts | `internal/port/messagequeue/schemas.go:98` |
| FIX-020 | HIGH | NATS | HIGH-003 | Missing `tenant_id` in `RunStartPayload` | `internal/port/messagequeue/schemas.go:44` |
| FIX-021 | HIGH | Python | HIGH-001 | Memory storage `recall()` missing tenant_id filter | `workers/codeforge/memory/storage.py:86` |
| FIX-022 | HIGH | Python | HIGH-002 | Glob tool missing path traversal protection | `workers/codeforge/tools/glob_files.py:53` |
| FIX-023 | HIGH | Python | HIGH-003 | Quality gate runs arbitrary shell commands | `workers/codeforge/qualitygate.py:60` |
| FIX-024 | HIGH | Python | HIGH-004 | `object` type annotations instead of proper types | `workers/codeforge/agent_loop.py:135,151` |
| FIX-025 | HIGH | Python | HIGH-005 | Unbounded growth of `_processed_ids` with random eviction | `workers/codeforge/consumer/_base.py:32` |
| FIX-026 | HIGH | Security | HIGH-002 | API key store -- no tenant isolation | `internal/adapter/postgres/store_api_key.go` |
| FIX-027 | HIGH | Security | HIGH-003 | NATS payloads trust tenant_id without verification | `internal/service/benchmark.go:736` |
| FIX-028 | HIGH | API Contract | HIGH-002 | `http.Error()` returns plain text instead of JSON in LLM keys | `internal/adapter/http/handlers_llm_keys.go` |
| FIX-029 | HIGH | API Contract | HIGH-003 | `DeleteBranchProtectionRule` returns 200+body instead of 204 | `internal/adapter/http/handlers_session.go:80` |
| FIX-030 | HIGH | API Contract | HIGH-004 | No pagination on ~40+ list endpoints | Multiple handler files |
| FIX-031 | HIGH | Test Coverage | HIGH-001 | 32 of 36 postgres store files untested | `internal/adapter/postgres/store_*.go` |
| FIX-032 | HIGH | Test Coverage | HIGH-002 | 25 service files have no test file | `internal/service/*.go` |
| FIX-033 | HIGH | Test Coverage | HIGH-003 | 6 consumer mixins have no tests | `workers/codeforge/consumer/_*.py` |
| FIX-034 | HIGH | Test Coverage | HIGH-004 | Frontend core features have zero unit tests | `frontend/src/features/` |
| FIX-035 | HIGH | Test Coverage | HIGH-005 | Memory tenant isolation gap has no test | `workers/tests/test_memory_system.py` |
| FIX-036 | HIGH | Protocol | HIGH-001 | MCP `Start()` silently swallows listen errors | `internal/adapter/mcp/server.go:79` |
| FIX-037 | HIGH | Protocol | HIGH-002 | A2A cancel payload uses untyped inline map | `internal/adapter/a2a/executor.go:96` |
| FIX-038 | HIGH | Routing | H1 | Cascade layer ordering contradicts documentation | `workers/codeforge/routing/router.py:98` |
| FIX-039 | HIGH | Routing | H2 | 9 config fields not loaded from YAML/env (dead features) | `workers/codeforge/llm.py:276` |
| FIX-040 | HIGH | Routing | H3 | Routing defaults to enabled=True, contradicting docs | `workers/codeforge/llm.py:283` |
| FIX-041 | HIGH | Docker | H1 | No resource limits on any Docker service | `docker-compose.prod.yml` |
| FIX-042 | HIGH | Docker | H2 | Hardcoded private IP in litellm config | `litellm/config.yaml:29` |
| FIX-043 | HIGH | Docker | H3 | No health check for Python worker in production | `docker-compose.prod.yml:133` |
| -- | HIGH | Security | HIGH-001 | XREF: Same as Go Core CRITICAL-002 (FIX-002) | -- |
| -- | HIGH | Security | MEDIUM-005 | XREF: Same as Go Core HIGH-002 (FIX-015) | -- |
| -- | HIGH | Go Core | HIGH-004 | XREF: Same as API Contract HIGH-001 (FIX-017) | -- |
| -- | HIGH | API Contract | HIGH-001 | XREF: Same as Go Core HIGH-004 (FIX-017) | -- |

### MEDIUM (41 findings)

| FIX ID | Severity | System | Finding ID | Title | File(s) |
|--------|----------|--------|------------|-------|---------|
| FIX-044 | MEDIUM | Go Core | MEDIUM-001 | `Handlers` God struct with 67 fields | `internal/adapter/http/handlers.go:38` |
| FIX-045 | MEDIUM | Go Core | MEDIUM-002 | `BenchmarkService` at 1064 LOC | `internal/service/benchmark.go` |
| FIX-046 | MEDIUM | Go Core | MEDIUM-003 | Residual `interface{}` usage (7 occurrences) | Multiple Go files |
| FIX-047 | MEDIUM | Go Core | MEDIUM-004 | Duplicated default tenant ID comment | `internal/middleware/tenant.go:12` |
| FIX-048 | MEDIUM | NATS | MEDIUM-001 | `_processed_ids` not thread-safe (asyncio) | `workers/codeforge/consumer/_base.py:32` |
| FIX-049 | MEDIUM | NATS | MEDIUM-002 | DLQ messages accumulate without monitoring | `internal/adapter/nats/nats.go:212` |
| FIX-050 | MEDIUM | NATS | MEDIUM-003 | Retry-Count header never incremented | `internal/adapter/nats/nats.go:188` |
| FIX-051 | MEDIUM | NATS | MEDIUM-004 | Python compact complete subject has no Go counterpart | `workers/codeforge/consumer/_subjects.py:64` |
| FIX-052 | MEDIUM | NATS | MEDIUM-005 | Validator accepts empty JSON objects | `internal/port/messagequeue/validator.go:58` |
| FIX-053 | MEDIUM | Python | MEDIUM-001 | Rollout scoring always returns 1.0 | `workers/codeforge/agent_loop.py:1105` |
| FIX-054 | MEDIUM | Python | MEDIUM-002 | Idempotency set not safe under concurrent access | `workers/codeforge/consumer/_base.py:36` |
| FIX-055 | MEDIUM | Python | MEDIUM-003 | Synchronous HTTP in async context (routing) | `workers/codeforge/consumer/_conversation.py:688` |
| FIX-056 | MEDIUM | Python | MEDIUM-004 | Duplicate embedding computation code | `workers/codeforge/memory/storage.py:141` |
| FIX-057 | MEDIUM | Security | MEDIUM-001 | WebSocket auth token in URL query parameter | `internal/middleware/auth.go:92` |
| FIX-058 | MEDIUM | Security | MEDIUM-002 | Authentication disabled by default | `internal/config/config.go:157` |
| FIX-059 | MEDIUM | Security | MEDIUM-003 | Internal service key grants unrestricted admin | `internal/middleware/auth.go:119` |
| FIX-060 | MEDIUM | Security | MEDIUM-004 | CORS wildcard allowed without hard block | `internal/adapter/http/middleware.go:30` |
| FIX-061 | MEDIUM | API Contract | MEDIUM-001 | Verb-in-URL violations (6 endpoints) | `internal/adapter/http/routes.go` |
| FIX-062 | MEDIUM | API Contract | MEDIUM-002 | Duplicate query parameter parser functions | `internal/adapter/http/helpers.go:42` |
| FIX-063 | MEDIUM | API Contract | MEDIUM-003 | Missing pagination envelope consistency | Multiple handler files |
| FIX-064 | MEDIUM | API Contract | MEDIUM-004 | No 422 usage for validation errors | `internal/adapter/http/helpers.go:104` |
| FIX-065 | MEDIUM | API Contract | MEDIUM-005 | Error detail leakage in `EvaluateStep` | `internal/adapter/http/handlers_orchestration.go:119` |
| FIX-066 | MEDIUM | Test Coverage | MEDIUM-001 | Port/interface layer largely untested | `internal/port/` |
| FIX-067 | MEDIUM | Test Coverage | MEDIUM-002 | 7 frontend features have zero tests | `frontend/src/features/` |
| FIX-068 | MEDIUM | Test Coverage | MEDIUM-003 | Password reset token logging untested | `internal/service/auth_test.go` |
| FIX-069 | MEDIUM | Test Coverage | MEDIUM-004 | PathValue mismatch not caught by tests | `internal/adapter/http/handlers_benchmark_*_test.go` |
| FIX-070 | MEDIUM | Test Coverage | MEDIUM-005 | GraphRAG module untested | `workers/codeforge/graphrag.py` |
| FIX-071 | MEDIUM | Frontend | MEDIUM-001 | WebSocket payload uses `as unknown as` casting (19 occurrences) | `frontend/src/api/websocket.ts:211` |
| FIX-072 | MEDIUM | Frontend | MEDIUM-002 | Hardcoded magic number for token budget (120000) | `frontend/src/features/project/ChatPanel.tsx:1087` |
| FIX-073 | MEDIUM | Frontend | MEDIUM-003 | Module-level singleton stores without disposal | `frontend/src/features/notifications/notificationStore.ts` |
| FIX-074 | MEDIUM | Frontend | MEDIUM-004 | Sparse unit test coverage for components (6.2%) | `frontend/src/` |
| FIX-075 | MEDIUM | Protocol | MEDIUM-001 | LSP adapter is 700+ lines of dead code (never imported) | `internal/adapter/lsp/` |
| FIX-076 | MEDIUM | Protocol | MEDIUM-002 | A2A `TaskStoreAdapter.List()` ignores filter parameter | `internal/adapter/a2a/taskstore.go:51` |
| FIX-077 | MEDIUM | Protocol | MEDIUM-003 | A2A executor silences `json.Marshal` errors | `internal/adapter/a2a/executor.go:60` |
| FIX-078 | MEDIUM | Protocol | MEDIUM-004 | MCP resources are static only -- no parameterized templates | `internal/adapter/mcp/resources.go` |
| FIX-079 | MEDIUM | Routing | M1 | Blocklist `is_blocked()` has TOCTOU race | `workers/codeforge/routing/blocklist.py:69` |
| FIX-080 | MEDIUM | Routing | M2 | `_effective_models` re-fetches blocklist every call | `workers/codeforge/routing/router.py:86` |
| FIX-081 | MEDIUM | Docker | M1 | NATS monitoring port 8222 exposed in dev compose | `docker-compose.yml:90` |
| FIX-082 | MEDIUM | Docker | M2 | No restart policy in dev compose | `docker-compose.yml` |
| FIX-083 | MEDIUM | Docker | M3 | Blue-green references missing traefik.yaml | `docker-compose.blue-green.yml:13` |

### LOW (31 findings)

| FIX ID | Severity | System | Finding ID | Title | File(s) |
|--------|----------|--------|------------|-------|---------|
| FIX-084 | LOW | Go Core | LOW-001 | Missing per-route rate limiting for auth endpoints | `internal/adapter/http/routes.go` |
| FIX-085 | LOW | Go Core | LOW-002 | `context.Background()` loses request-scoped values | `internal/adapter/http/handlers.go:266` |
| FIX-086 | LOW | NATS | LOW-001 | Contract tests do not cover all subjects | `internal/port/messagequeue/contract_test.go` |
| FIX-087 | LOW | NATS | LOW-002 | Inconsistent consumer name prefixes undocumented | `internal/adapter/nats/nats.go:138` |
| FIX-088 | LOW | NATS | LOW-003 | Bare `except Exception:` with no logging in benchmark model fetch | `workers/codeforge/consumer/_benchmark.py:96` |
| FIX-089 | LOW | Python | LOW-001 | Broad `Any` type usage in tool framework | `workers/codeforge/tools/_error_handler.py:11` |
| FIX-090 | LOW | Python | LOW-002 | Plan/Act transition has side effect in check method | `workers/codeforge/plan_act.py:60` |
| FIX-091 | LOW | Python | LOW-003 | Signal handler closure captures stale reference | `workers/codeforge/consumer/__init__.py:309` |
| FIX-092 | LOW | Python | LOW-004 | Inconsistent logging libraries (logging vs structlog) | Multiple Python files |
| FIX-093 | LOW | Security | LOW-001 | Refresh cookie Secure flag conditional on TLS detection | `internal/adapter/http/handlers_auth.go:49` |
| FIX-094 | LOW | Security | LOW-002 | Password hash returned in ListUsers response | `internal/adapter/postgres/store_user.go:56` |
| FIX-095 | LOW | Security | LOW-003 | No CSRF protection beyond SameSite cookie | `internal/adapter/http/middleware.go` |
| FIX-096 | LOW | Security | LOW-004 | Rate limiter uses only RemoteAddr IP | `internal/middleware/ratelimit.go:144` |
| FIX-097 | LOW | API Contract | LOW-001 | Quarantine handlers use lowercase method names | `internal/adapter/http/handlers_quarantine.go` |
| FIX-098 | LOW | API Contract | LOW-002 | Batch operations use POST for DELETE semantics | `internal/adapter/http/routes.go:51` |
| FIX-099 | LOW | API Contract | LOW-003 | Inconsistent status message spelling (cancelled/canceled) | Multiple handler files |
| FIX-100 | LOW | API Contract | LOW-004 | Missing PATCH usage for partial updates | `internal/adapter/http/routes.go` |
| FIX-101 | LOW | Test Coverage | LOW-001 | Test helper duplication (FakeLLM) | Multiple test files |
| FIX-102 | LOW | Test Coverage | LOW-002 | No integration test CI pipeline | `.github/workflows/ci.yml` |
| FIX-103 | LOW | Test Coverage | LOW-003 | E2E hardcoded localhost URLs | `frontend/e2e/*.spec.ts` |
| FIX-104 | LOW | Frontend | LOW-001 | `console.warn` in production code | `frontend/src/features/benchmarks/BenchmarkPage.tsx:360` |
| FIX-105 | LOW | Frontend | LOW-002 | ESLint disable comment density in ChatPanel | `frontend/src/features/project/ChatPanel.tsx` |
| FIX-106 | LOW | Frontend | LOW-003 | Inline SVG duplication across features | Multiple frontend files |
| FIX-107 | LOW | Protocol | LOW-001 | MCP AuthMiddleware defined but never used | `internal/adapter/mcp/auth.go:11` |
| FIX-108 | LOW | Protocol | LOW-002 | LSP `parseLocations` does not handle `LocationLink` | `internal/adapter/lsp/client.go:468` |
| FIX-109 | LOW | Protocol | LOW-003 | A2A AgentCard hardcodes `Streaming=false` | `internal/adapter/a2a/agentcard.go:48` |
| FIX-110 | LOW | Routing | L1 | Module-level singletons reduce testability | `workers/codeforge/routing/blocklist.py:92` |
| FIX-111 | LOW | Routing | L2 | `_warned_providers` global mutable set without lock | `workers/codeforge/routing/key_filter.py:28` |
| FIX-112 | LOW | Docker | L1 | No HEALTHCHECK instruction in Dockerfiles | `Dockerfile`, `Dockerfile.worker`, `Dockerfile.frontend` |
| FIX-113 | LOW | Docker | L2 | CI NATS service missing JetStream flag | `.github/workflows/ci.yml:33` |
| FIX-114 | LOW | Docker | L3 | .dockerignore missing test/docs/scripts exclusions | `.dockerignore` |

---

## 4. Statistics

### 4.1 Totals

| Metric | Value |
|--------|------:|
| Total findings | 122 |
| Unique findings (after dedup) | 114 |
| CRITICAL findings | 14 (10 unique) |
| HIGH findings | 36 (30 unique) |
| MEDIUM findings | 41 |
| LOW findings | 31 |
| Average score | 66.0/100 |
| Median score | 72/100 |
| Highest score | 78 (Frontend Architecture) |
| Lowest score | 38 (Test Coverage) |

### 4.2 Systems Below 60 (Needs Immediate Attention)

| System | Score | Grade | Key Issue |
|--------|------:|:-----:|-----------|
| Test Coverage | 38 | F | 32 of 36 store files untested, no tenant isolation tests, no command injection tests, no NATS reconnect tests |

The Test Coverage audit is the only system below 60. Its 4 CRITICAL findings all relate to the absence of tests that would have caught bugs identified by other audits. This is the single most impactful area for improvement -- adding these tests would provide regression protection for the tenant isolation, command injection, and NATS resilience fixes.

### 4.3 Systems Below 75 (Significant Attention Required)

| System | Score | Grade | Top Issue |
|--------|------:|:-----:|-----------|
| Test Coverage | 38 | F | Massive testing gaps |
| Security | 62 | C | Password reset token leak, tenant isolation bypasses |
| NATS Integration | 68 | C | No reconnect config, validator coverage gap |
| API Contract | 72 | C | Error leakage, PathValue bug, no pagination |
| Protocol Compliance | 72 | C | MCP unauthenticated, LSP dead code |
| Hybrid Routing | 72 | C | Reward config mismatch, dead feature config |
| Docker/Infra | 72 | C | SQL injection in script, no resource limits |
| Python Workers | 72 | C | Bash tool no sanitization, memory tenant gap |
| Go Core Service | 74 | C | Tenant isolation bypasses |

### 4.4 Findings by Category

| Category | Count | Percentage |
|----------|------:|----------:|
| Tenant Isolation | 13 | 10.7% |
| Testing Gaps | 17 | 13.9% |
| Input Validation / Sanitization | 8 | 6.6% |
| Error Handling | 6 | 4.9% |
| Configuration / Defaults | 7 | 5.7% |
| Type Safety | 5 | 4.1% |
| Code Size / Structure | 7 | 5.7% |
| Security Hardening | 10 | 8.2% |
| API Consistency | 8 | 6.6% |
| Infrastructure | 10 | 8.2% |
| Protocol Compliance | 10 | 8.2% |
| Other | 21 | 17.2% |

### 4.5 Top 10 Priority Fixes

The following fixes should be addressed first, ordered by impact and effort:

1. **FIX-001** -- Add `AND tenant_id = $N` to all 5 queries in `store_agent_identity.go` (30 min)
2. **FIX-003** -- Remove password reset token from log output in `handlers_auth.go` (5 min)
3. **FIX-005** -- Add NATS reconnect options with disconnect/reconnect handlers (30 min)
4. **FIX-006** -- Replace `err.Error()` with generic message in search endpoints (10 min)
5. **FIX-007** -- Wire existing `AuthMiddleware` to MCP server (15 min)
6. **FIX-017** -- Replace `r.PathValue("id")` with `chi.URLParam(r, "id")` in 7 locations (15 min)
7. **FIX-009** -- Use psql variable binding in restore script (10 min)
8. **FIX-004** -- Add command blocklist to bash tool as defense-in-depth (1 hr)
9. **FIX-002** -- Add tenant filter or document system-wide scope for `ReleaseStaleWork` (15 min)
10. **FIX-011** -- Add tenant isolation integration tests using existing test patterns (2 hr)

**Estimated total effort for top 10:** ~5 hours

---

## 5. Architecture Strengths (Preserve)

Despite the findings, the audit identified significant architectural strengths that should be preserved:

1. **Hexagonal Architecture** -- Clean dependency direction (handler -> service -> port <- adapter) consistently enforced across 165 Go files
2. **Zero `any` / zero `@ts-ignore`** in the entire frontend codebase (260 files)
3. **Parameterized SQL throughout** -- No SQL injection vectors in application code (script-only issue)
4. **Strong service-layer testing** -- 79 Go test files with ~1,200 test functions, 138 Python test files with ~2,085 functions
5. **Contract test infrastructure** -- 22 NATS fixtures with round-trip verification
6. **Defense in depth** -- Dual deduplication (JetStream + in-memory), circuit breakers, trust annotations, 8 safety layers
7. **Well-typed API** -- 150+ TypeScript interfaces mirroring Go domain types with `strict: true`
8. **Clean protocol implementations** -- MCP, A2A, LSP all use official SDKs with correct error handling patterns

---

## Appendix: Cross-Reference Matrix

Findings that appear in multiple audits (counted once in prioritized list):

| Finding | Audits | Canonical FIX ID |
|---------|--------|-----------------|
| Tenant bypass in `store_agent_identity.go` | Go Core (CRITICAL-001), Security (CRITICAL-002) | FIX-001 |
| `ReleaseStaleWork` cross-tenant | Go Core (CRITICAL-002), Security (HIGH-001) | FIX-002 |
| User store missing tenant filter | Go Core (HIGH-002), Security (MEDIUM-005) | FIX-015 |
| PathValue vs chi.URLParam | Go Core (HIGH-004), API Contract (HIGH-001) | FIX-017 |
| `RunStartPayload` missing tenant_id / NATS tenant trust | NATS (HIGH-003), Security (HIGH-003) | FIX-020, FIX-027 |
| `_processed_ids` concurrency | NATS (MEDIUM-001), Python (MEDIUM-002) | FIX-048 |
