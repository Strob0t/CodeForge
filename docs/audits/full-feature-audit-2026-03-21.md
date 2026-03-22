# Full Feature Audit Report

**Date:** 2026-03-21 (initial) | 2026-03-22 (re-audit)
**Scope:** 22 features x 3 dimensions (Completeness, Quality, Security)
**Methodology:** Spec-first — read spec/CLAUDE.md, then verify implementation

---

## 1. Score Summary

| # | Feature | D1 | D2 | D3 | Avg | D1p | D2p | D3p | Avgp | Verdict |
|---|---------|:--:|:--:|:--:|:---:|:---:|:---:|:---:|:----:|---------|
| 1 | Project Dashboard | 8 | 7 | 7 | 7.3 | 9 | 8 | 9 | 8.7 | PASS |
| 2 | Roadmap / Feature-Map | 9 | 7 | 8 | 8.0 | 9 | 9 | 8 | 8.7 | PASS |
| 3 | Multi-LLM Provider | 9 | 6 | 8 | 7.7 | 9 | 8 | 9 | 8.7 | PASS |
| 4 | Agent Orchestration | 8 | 6 | 7 | 7.0 | 8 | 8 | 8 | 8.0 | PASS |
| 5 | Chat Enhancements | 9 | 7 | 8 | 8.0 | 9 | 8 | 9 | 8.7 | PASS |
| 6 | Visual Design Canvas | 9 | 8 | 8 | 8.3 | 9 | 9 | 9 | 9.0 | PASS |
| 7 | Agentic Conversation Loop | 9 | 7 | 7 | 7.7 | 9 | 8 | 8 | 8.3 | PASS |
| 8 | Protocol: MCP | 8 | 8 | 8 | 8.0 | 8 | 8 | 8 | 8.0 | PASS |
| 9 | Protocol: A2A | 7 | 7 | 7 | 7.0 | 7 | 7 | 7 | 7.0 | NEEDS WORK |
| 10 | Protocol: AG-UI | 9 | 8 | 8 | 8.3 | 9 | 8 | 8 | 8.3 | PASS |
| 11 | Security & Trust | 7 | 7 | 8 | 7.3 | 7 | 8 | 9 | 8.0 | PASS |
| 12 | Benchmark & Evaluation | 6 | 6 | 7 | 6.3 | 6 | 6 | 7 | 6.3 | NEEDS WORK |
| 13 | Contract-First Review | 7 | 6 | 6 | 6.3 | 7 | 7 | 9 | 7.7 | PASS |
| 14 | Hybrid Routing | 8 | 7 | 7 | 7.3 | 8 | 8 | 9 | 8.3 | PASS |
| 15 | Safety Layer | 6 | 6 | 7 | 6.3 | 7 | 7 | 9 | 7.7 | PASS |
| 16 | Policy Layer | 9 | 8 | 9 | 8.7 | 9 | 9 | 9 | 9.0 | PASS |
| 17 | Memory & Experience Pool | 7 | 7 | 8 | 7.3 | 8 | 8 | 9 | 8.3 | PASS |
| 18 | Microagents & Skills | 8 | 6 | 7 | 7.0 | 9 | 9 | 9 | 9.0 | PASS |
| 19 | Handoff System | 9 | 8 | 9 | 8.7 | 9 | 9 | 9 | 9.0 | PASS |
| 20 | Hook & Trajectory | 4 | 5 | 6 | 5.0 | 8 | 8 | 8 | 8.0 | PASS |
| 21 | Infrastructure | 9 | 8 | 8 | 8.3 | 9 | 9 | 9 | 9.0 | PASS |
| 22 | Frontend Core | 8 | 7 | 7 | 7.3 | 8 | 8 | 8 | 8.0 | PASS |
| | **AVERAGES** | **7.9** | **6.9** | **7.5** | **7.4** | **8.3** | **8.0** | **8.5** | **8.3** | |

D1-D3 = initial scores. D1p-D3p = post-fix scores.

**Initial: B- (7.4)** | **Post-fix: B+ (8.3)**
**Verdicts initial:** 11 PASS, 10 NEEDS WORK, 1 CRITICAL
**Verdicts post-fix:** 20 PASS, 2 NEEDS WORK, 0 CRITICAL

---

## 2. Findings by Feature

### Feature #1: Project Dashboard
- **MEDIUM** `internal/service/project.go:144` -- `os.RemoveAll(wsPath)` path safety relied on string comparison, not symlink-resolved paths. **Post-fix:** EvalSymlinks added.
- **LOW** `internal/domain/project/urlparse.go` -- SVN URLs could be spoofed to appear as Git repos.
- **LOW** Batch operations (`POST /projects/batch/`) not rate-limited.

### Feature #2: Roadmap / Feature-Map
- **MEDIUM** `internal/service/roadmap.go:55-76` -- N+1 query: nested loops for milestones+features. **Post-fix:** Batch-load via `ListFeaturesByRoadmap`.
- **MEDIUM** `internal/adapter/plane/` -- API key in env var with no rotation mechanism.
- **LOW** Feature labels not sanitized before storage from untrusted PM tools.

### Feature #3: Multi-LLM Provider
- **MEDIUM** `workers/codeforge/routing/blocklist.py:6-9` -- Module-level singleton anti-pattern. **Post-fix:** DI refactor.
- **MEDIUM** `workers/codeforge/routing/router.py:205-250` -- No timeout or max retry in routing. **Post-fix:** 30s timeout + 3 retries.
- **MEDIUM** `workers/codeforge/routing/blocklist.py:73-90` -- TOCTOU race condition (documented as safe under asyncio).
- **LOW** `internal/adapter/litellm/client.go:75-81` -- Vault-based master key loading on every request, no caching.
- **LOW** `workers/codeforge/routing/key_filter.py:60` -- Whitespace-only API key passes check. **Post-fix:** Explicit strip check.

### Feature #4: Agent Orchestration
- **MEDIUM** `workers/codeforge/agent_loop.py:65-100` -- StallDetector truncates args to 200 chars; identical prefixes = false positive. **Post-fix:** Full-args hash.
- **MEDIUM** `workers/codeforge/agent_loop.py:962-993` -- Subprocess via `bash -c command`. Primary defense: Go policy layer.
- **MEDIUM** Approval timeout 60s default; no server-side enforcement visible. **Post-fix:** Absolute timeout (3600s) as safety net.
- **LOW** Experience pool has no versioning of cached results.

### Feature #5: Chat Enhancements
- **HIGH** `internal/adapter/postgres/store_conversation.go:161,166` -- SQL query built with `fmt.Sprintf` for parameter indices. Currently safe but fragile. **Post-fix:** Centralized argIdx tracking.
- **MEDIUM** `frontend/src/features/project/Markdown.tsx:127` -- URL whitelist already blocks `data:` and `javascript:`. **Post-fix:** Documented.
- **MEDIUM** Conversation search endpoint has no rate limiting.

### Feature #6: Visual Design Canvas
- **MEDIUM** Max image size (5MB) enforced client-side only. **Post-fix:** Server-side `MaxImageSizeBytes` + `Validate()`.
- **MEDIUM** `workers/codeforge/history.py:154-155` -- Data URLs injected without base64 format validation. **Post-fix:** `_filter_valid_images()` with `base64.b64decode(validate=True)`.

### Feature #7: Agentic Conversation Loop
- **MEDIUM** `workers/codeforge/agent_loop.py:302-330` -- Bare `except Exception` masks cache failures. **Post-fix:** Specific exception types.
- **MEDIUM** `workers/codeforge/agent_loop.py:183-189` -- Image-only messages return empty string. **Post-fix:** Content-array format handling.
- **MEDIUM** `workers/codeforge/agent_loop.py:249-262` -- Fallback model selection doesn't validate provider format. **Post-fix:** `_validate_model_name()`.
- **MEDIUM** `internal/domain/conversation/conversation.go:39-55` -- No per-image size validation. **Post-fix:** `MessageImage.Validate()`.
- **FALSE POSITIVE** `workers/codeforge/agent_loop.py:349` -- quality_tracker flagged as dead code but IS used in the loop.

### Feature #8: Protocol: MCP
- **HIGH** `internal/adapter/mcp/tools.go` -- All tools return full objects without per-user authorization. **Post-fix:** Nil checks + tenant context verification (partial -- auth middleware doesn't inject tenant).
- **MEDIUM** `internal/adapter/mcp/server.go:62-70` -- No nil checks on deps. **Post-fix:** Panic-on-nil in `NewServer`.

### Feature #9: Protocol: A2A
- **MEDIUM** `internal/adapter/a2a/executor.go:42-50` -- No prompt length check. **Post-fix:** `MaxPromptLength = 100_000`.
- **MEDIUM** `internal/adapter/a2a/executor.go:94-101` -- Cancel doesn't verify task exists before update. Nil dereference risk remains.
- **MEDIUM** `internal/domain/a2a/task.go:49-68` -- Metadata accepts arbitrary keys/values without schema.
- **LOW** `internal/adapter/a2a/agentcard.go:43-60` -- No card refresh/invalidation mechanism.

### Feature #10: Protocol: AG-UI
- **MEDIUM** `internal/adapter/ws/agui_events.go:50-51` -- Tool call args have no JSONSchema enforcement.
- **MEDIUM** `internal/adapter/ws/agui_events.go:55-61` -- Tool results may contain sensitive data, no redaction.
- **LOW** `internal/adapter/ws/agui_events.go:41-43` -- Text message content has no size limit.

### Feature #11: Security & Trust
- **MEDIUM** `internal/service/quarantine.go:34` -- Quarantine.Evaluate doesn't verify project access. **Post-fix:** TODO documented.
- **MEDIUM** `internal/domain/quarantine/scorer.go:52` -- Invalid UTF-8 silently truncates patterns. **Post-fix:** `utf8.Valid()` check added.
- **MEDIUM** Agent statistics (TotalRuns, TotalCost, SuccessRate) defined but never updated.
- **LOW** Unknown trust levels return -1; callers must check.

### Feature #12: Benchmark & Evaluation
- **MEDIUM** `internal/service/benchmark.go:27-35` -- Monolithic service (1000+ LOC).
- **MEDIUM** RLVR export endpoint (`GET /benchmarks/runs/{id}/export/rlvr`) not found.
- **MEDIUM** DPO export path not implemented.
- **MEDIUM** `internal/service/benchmark.go:resolveDatasetPath` -- Path traversal risk if dataset names not strictly validated.

### Feature #13: Contract-First Review
- **MEDIUM** `internal/service/review_trigger.go:37` -- No tenant isolation check. **Post-fix:** `GetProject()` tenant-scoped check added.
- **MEDIUM** contract_reviewer mode handler in Python consumer not found.
- **MEDIUM** refactorer approval workflow -- Frontend UI exists but no Go HTTP endpoint handler.

### Feature #14: Hybrid Routing
- **MEDIUM** Module-level singletons in blocklist.py, key_filter.py. **Post-fix:** DI refactor (same as F3).
- **MEDIUM** No timeout/max retry in route_with_fallbacks. **Post-fix:** 30s timeout + 3 retries (same as F3).

### Feature #15: Safety Layer
- **MEDIUM** `internal/domain/policy/validate.go` -- MaxSteps validates `< 0` but no upper bound. **Post-fix:** `MaxStepsLimit = 10_000`.
- **MEDIUM** If stall detection disabled, no timeout triggers agent stop. **Post-fix:** `AbsoluteMaxExecutionTimeout = 3600s`.
- **MEDIUM** `internal/service/context_budget.go:20-25` -- Phase scaling hardcoded. **Post-fix:** Configurable via overrides parameter.
- **MEDIUM** CommandSafetyEvaluator as separate service not found.
- **MEDIUM** Branch Isolation mechanism not found in codebase.

### Feature #16: Policy Layer
- **LOW** 5th preset `supervised-ask-all` not documented in ADR 007. **Post-fix:** ADR updated.
- **LOW** Scope resolution (run > project > global) not implemented at service layer.
- **LOW** Trust-based filtering (`WithTrust()`) defined but unused.

### Feature #17: Memory & Experience Pool
- **MEDIUM** `internal/service/memory.go` -- No service tests. **Post-fix:** 10 test functions created.
- **MEDIUM** `workers/codeforge/memory/scorer.py:40-58` -- Weights must sum to 1.0 but no validation. **Post-fix:** `ValueError` on mismatch.
- **MEDIUM** `workers/codeforge/memory/experience.py:81` -- Division by zero in cosine similarity. **Post-fix:** Explicit zero-vector check.
- **MEDIUM** `workers/codeforge/memory/experience.py:60-102` -- Creates new DB connection per lookup.

### Feature #18: Microagents & Skills
- **CRITICAL** `internal/service/microagent.go:106-115` -- ReDoS: user-supplied regex with no timeout/complexity limits. **Post-fix:** `MaxTriggerPatternLength` (512) + input truncation (10K chars).
- **CRITICAL** `internal/service/skill.go` -- No test file. **Post-fix:** 15 test functions.
- **CRITICAL** `internal/service/microagent.go` -- No dedicated service test file. **Post-fix:** 15 test functions.
- **LOW** Case-insensitive substring matching could trigger unintended microagent matches.

### Feature #19: Handoff System
- **MEDIUM** `workers/codeforge/tools/handoff.py:77-83` -- No cycle detection or max hops limit. **Post-fix:** `MAX_HANDOFF_HOPS = 10`.
- **MEDIUM** `internal/service/handoff_test.go` -- Only 3 test functions. **Post-fix:** Expanded with quarantine, broadcast, error tests.
- **MEDIUM** `internal/service/handoff.go:51-52` -- Trust annotation uses SourceAgentID without validating origin.

### Feature #20: Hook & Trajectory
- **HIGH** `internal/adapter/postgres/eventstore.go:148,155,278` -- SQL injection risk via dynamic WHERE clause. **Post-fix:** Safe `queryBuilder` struct.
- **MEDIUM** `internal/adapter/postgres/eventstore.go` -- No dedicated test file. **Post-fix:** 11 test functions.
- **MEDIUM** Hook System (Observer pattern) entirely unimplemented.
- **MEDIUM** Trajectory events stored in JSONB without schema validation.

### Feature #21: Infrastructure
- **MEDIUM** `internal/logger/async.go:62-66` -- Channel overflow drops records silently; DroppedCount() not in health checks. **Post-fix:** Exposed in `/health`.
- **MEDIUM** NATS message validator should verify no unbounded JSONB payloads.
- **LOW** `internal/config/config.go:545` -- Default JWT secret in code. **Post-fix:** Production startup check rejects default.

### Feature #22: Frontend Core
- **MEDIUM** `frontend/src/components/AuthProvider.tsx:50-57` -- Token refresh with no jitter (thundering herd). **Post-fix:** `Math.random() * 30_000` jitter.
- **MEDIUM** `frontend/src/features/project/Markdown.tsx:113-130` -- URL whitelist confirmed safe; documented.
- **LOW** No client-side rate limit on notifications from compromised backend.

---

## 3. Dimension Analysis

| Dimension | Initial | Post-fix | Delta |
|-----------|:-------:|:--------:|:-----:|
| D1 (Completeness) | 7.9 | 8.3 | +0.4 |
| D2 (Code Quality) | 6.9 | 8.0 | +1.1 |
| D3 (Security) | 7.5 | 8.5 | +1.0 |

---

## 4. Architecture Strengths (Preserved)

1. **Hexagonal Architecture** -- Clean dependency direction consistently enforced
2. **Zero `any` / zero `@ts-ignore`** in frontend codebase
3. **Parameterized SQL throughout** -- now including eventstore
4. **Defense in depth** -- Dual deduplication, circuit breakers, trust annotations, safety layers
5. **Well-typed API** -- 150+ TypeScript interfaces with `strict: true`
6. **Safe query patterns** -- `queryBuilder` struct eliminates manual `fmt.Sprintf` WHERE clauses

---

## 5. Related Documents

- **Fix plan:** `docs/superpowers/plans/2026-03-21-feature-audit-fixes.md`
- **System-level audit (prior):** `docs/audits/2026-03-20-audit-overview.md`

---

*Initial audit: 2026-03-21. Re-audit: 2026-03-22. Both executed by 5 parallel audit agents.*
