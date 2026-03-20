# NATS Integration Audit Report

**Date:** 2026-03-20
**Scope:** Architecture + Code Review
**Files Reviewed:** 22 files
**Score: 68/100 -- Grade: C** (post-fix: 96/100 -- Grade: A)

> Warning: Original score was below 75. Most findings have been fixed.

---

## Executive Summary

| Severity | Count | Category Breakdown |
|----------|------:|---------------------|
| CRITICAL | 1     | Missing reconnect config |
| HIGH     | 3     | Validator coverage gap, tenant_id inconsistency, type mismatch |
| MEDIUM   | 5     | Idempotency guard non-thread-safe, DLQ routing, retry header not incremented, missing subjects in Python, validator passthrough |
| LOW      | 3     | Consumer naming, missing contract tests, compact subject asymmetry |
| **Total**| **12** |                     |

### Positive Findings

- **Excellent subject alignment:** 50+ Go subject constants match Python `_subjects.py` string-for-string with zero mismatches on all core subjects.
- **JetStream stream subjects perfectly synchronized:** Both Go `nats.go:54` and Python `_subjects.py:6-24` declare the same 17 wildcard prefixes in the same order.
- **Robust DLQ mechanism:** Both Go (`moveToDLQ`) and Python (`_move_to_dlq`) implement dead letter queue routing with ack-after-publish semantics.
- **Deduplication at both layers:** Go uses `PublishWithDedup` with `Nats-Msg-Id` header; Python uses an in-memory `_processed_ids` set plus `_active_runs` guard for conversation runs.
- **Contract test infrastructure:** 22 fixtures with round-trip tests and key-field verification cover the most critical payloads.
- **W3C trace context propagation:** Both Go (`injectTraceContext`/`extractTraceContext`) and Python (`extract_trace_context`) propagate `traceparent` headers through NATS for distributed tracing.
- **Circuit breaker support:** Go publish path wraps in `resilience.Breaker` when configured.
- **Trust annotations:** Both layers stamp outgoing messages with trust metadata (Phase 23A).
- **Graceful shutdown:** Both sides implement drain-with-timeout (Go: `Drain()`, Python: `asyncio.wait_for(drain, timeout=10)`).
- **Well-structured mixin architecture:** Python consumer uses 16 handler mixins, each owning a single subject group -- clean separation of concerns.
- **Generic handler pattern:** `_handle_request` in `_base.py` provides dedup, error handling, and ack/nak in a single reusable function.
- **Goroutine dispatch:** Go `Subscribe` dispatches handlers in goroutines (`go q.handleMessage`) so slow HITL approval waits do not block the consumer.

---

## Architecture Review

### Subject & Stream Design

**Stream configuration** is declared in two places:

1. **Go** (`internal/adapter/nats/nats.go:52-56`): `CreateOrUpdateStream` with 17 wildcard subjects.
2. **Python** (`workers/codeforge/consumer/_subjects.py:6-24`): `STREAM_SUBJECTS` list with the same 17 wildcards.

Both match exactly:
```
tasks.>, agents.>, runs.>, context.>, repomap.>, retrieval.>,
graph.>, conversation.>, evaluation.>, benchmark.>, mcp.>,
a2a.>, memory.>, handoff.>, backends.>, review.>, prompt.>
```

**Subject string comparison (Go vs Python):** All 50+ subject constants produce identical strings. The `sanitizeConsumerName` function in Go and `consumer_name` function in Python use the same replacement rules (`.` to `-`, `*` to `all`, `>` to `all`), though Go uses a `codeforge-go-` prefix and Python uses `codeforge-py-`, which is correct for consumer isolation.

**Single source of truth:** There is no single source of truth. Both sides independently define the same strings. This is a design trade-off (no code generation, no shared schema file). The contract tests in `contract_test.go` and testdata fixtures provide some safety net, but a new subject added to Go but forgotten in Python would not be caught until runtime.

### JSON Contract Alignment

**Field name matching:** Go JSON tags and Python Pydantic field names match across all reviewed payload types. Both use snake_case consistently. Examples verified:

| Payload | Go JSON tag | Python field | Match |
|---------|------------|-------------|-------|
| ConversationRunStart | `run_id`, `conversation_id`, `agentic` | `run_id`, `conversation_id`, `agentic` | Yes |
| BenchmarkRunRequest | `run_id`, `dataset_path`, `rollout_count` | `run_id`, `dataset_path`, `rollout_count` | Yes |
| ToolCallResultPayload | `tokens_in` (int64), `tokens_out` (int64) | `tokens_in` (int), `tokens_out` (int) | See HIGH-002 |

**Null coercion:** Python handles Go's nil-to-null serialization via `@field_validator` decorators (e.g., `_coerce_list_fields`, `_coerce_config_none`). This is a good defensive pattern.

### Message Flow Patterns

Three distinct patterns are used:

1. **Request-Response:** Go publishes request, Python subscribes, processes, publishes result to paired subject (e.g., `retrieval.search.request` / `retrieval.search.result`). Go waits with a timeout.
2. **Fire-and-Forget:** Go publishes (e.g., `memory.store`), Python processes and acks. No result expected on NATS (result stored in DB).
3. **Event Stream:** Python publishes progress events (e.g., `benchmark.task.started`, `benchmark.task.progress`). Go subscribes and forwards to WebSocket.

All three patterns are well-implemented with appropriate ack semantics.

---

## Code Review Findings

### CRITICAL-001: No NATS Reconnect Configuration -- **FIXED**

- **File:** `internal/adapter/nats/nats.go:39`
- **Description:** `nats.Connect(url)` is called with zero options. The NATS Go client defaults to `MaxReconnects: 60` and `ReconnectWait: 2s`, but there are no disconnect/reconnect handlers, no custom reconnect buffer size, and no error handler. In a Docker environment with container restarts, the default 2-minute reconnect window may be insufficient. More critically, there is no logging or alerting when a disconnect occurs -- the system silently reconnects (or silently fails).
- **Impact:** After a NATS server restart or network partition exceeding 2 minutes, the Go service loses its connection permanently with no recovery mechanism and no operational visibility. Published messages during the disconnected period are silently dropped. The circuit breaker on the publish path does not trigger reconnection.
- **Recommendation:** Add explicit reconnect options:
  ```go
  nc, err := nats.Connect(url,
      nats.MaxReconnects(-1), // unlimited reconnects
      nats.ReconnectWait(2*time.Second),
      nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
          slog.Error("nats disconnected", "error", err)
      }),
      nats.ReconnectHandler(func(nc *nats.Conn) {
          slog.Info("nats reconnected", "url", nc.ConnectedUrl())
      }),
      nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
          slog.Error("nats async error", "error", err)
      }),
  )
  ```

### HIGH-001: Validator Covers Only 14 of 50+ Subjects -- **FIXED**

- **File:** `internal/port/messagequeue/validator.go:20-56`
- **Description:** The `Validate` function has explicit schema checks for only 14 subjects: `SubjectTaskResult`, `SubjectTaskCancel`, 4 retrieval subjects, 2 sub-agent subjects, 2 GEMMAS subjects, and 4 benchmark subjects. All other subjects (40+) fall through to the `default` case, which logs a warning and returns nil (passes validation). This means conversation run messages, run protocol messages, review messages, memory messages, A2A messages, graph messages, repomap messages, prompt evolution messages, and all other payloads are never structurally validated on the Go side.
- **Impact:** Malformed messages for the majority of subjects bypass validation and reach handlers, potentially causing panics, silent data corruption, or difficult-to-debug failures.
- **Recommendation:** Add all payload types to the validator switch statement. Since all payloads have corresponding Go structs, this is straightforward. Consider generating the switch from a subject-to-type registry to prevent future drift.

### HIGH-002: Type Mismatch -- int64 vs int for Token Counts -- **FIXED**

- **File:** `internal/port/messagequeue/schemas.go:98-99` vs `workers/codeforge/models.py:153-154`
- **Description:** Go `ToolCallResultPayload` uses `int64` for `TokensIn`/`TokensOut`, and Go `RunCompletePayload` also uses `int64` (line 114-115). However, Go `ConversationRunCompletePayload` uses `int` (line 464-465), while Go `TaskResultPayload` uses `int` (line 17-18). Python uses `int` for all of them (unbounded). The inconsistency within Go itself (`int` vs `int64` for the same semantic field) can cause silent truncation on 32-bit platforms or confusion when reading/maintaining the code.
- **Impact:** On 32-bit Go targets (unlikely but possible in embedded/CI), `int` is 32-bit and would truncate at ~2.1 billion tokens. More practically, the inconsistency is a maintenance hazard.
- **Recommendation:** Standardize all token count fields to `int64` in Go (or `int` if 64-bit is guaranteed). The JSON wire format handles both identically.

### HIGH-003: Missing tenant_id in RunStartPayload -- **FIXED**

- **File:** `internal/port/messagequeue/schemas.go:44-60`
- **Description:** `RunStartPayload` (the legacy run protocol, Phase 4B) has no `tenant_id` field, unlike `ConversationRunStartPayload` which added it. The Python `RunStartMessage` model also has a `tenant_id` field (line 102), creating a Go-Python contract mismatch. Go will never serialize `tenant_id` for run starts, but Python expects it (with an empty default).
- **Impact:** Legacy run protocol messages will always have an empty `tenant_id` on the Python side, which may bypass tenant isolation for background jobs using the older run protocol. If a multi-tenant deployment uses the legacy run path, tenant-scoped database queries may fail or return wrong data.
- **Recommendation:** Add `TenantID string json:"tenant_id,omitempty"` to `RunStartPayload` in Go, and ensure Go callers populate it from context.

### MEDIUM-001: Python _processed_ids Is Not Thread-Safe

- **File:** `workers/codeforge/consumer/_base.py:32-45`
- **Description:** `_processed_ids` is a `ClassVar[set[str]]` (shared across all instances) modified by `_is_duplicate` and `_clear_processed`. While Python's GIL protects against data corruption, the consumer uses `asyncio.gather(*loops)` with multiple concurrent message loops. The `_is_duplicate` method performs a check-then-act (`if msg_id in set` then `set.add`) that is not atomic across `await` boundaries. In the asyncio concurrency model, two coroutines handling the same redelivered message could both pass the `in` check before either adds to the set.
- **Impact:** Rare duplicate processing of the same message when two loops handle the same subject (unlikely in current architecture since each subject has its own loop, but the `_is_duplicate` is called with custom keys like `f"run-{run_id}"` that could collide across loops).
- **Recommendation:** Use an `asyncio.Lock` around the check-and-add operation, or at minimum document that the current design relies on the per-subject-single-loop invariant.

### MEDIUM-002: DLQ Messages May Not Be Routed to a Captured Stream -- **FIXED**

- **File:** `internal/adapter/nats/nats.go:212-238` and `workers/codeforge/consumer/_base.py:62-73`
- **Description:** DLQ messages are published to `{subject}.dlq` (e.g., `tasks.cancel.dlq`). Since the JetStream stream captures `tasks.>`, the wildcard does capture `.dlq` suffixed subjects. However, there is no dedicated DLQ consumer or monitoring. DLQ messages accumulate silently in the stream without alerting or reprocessing capability.
- **Impact:** Failed messages are durably stored but never acted upon. Operators have no visibility into DLQ accumulation without manually querying JetStream.
- **Recommendation:** Add a DLQ monitoring endpoint or periodic log that reports DLQ message counts. Consider a separate DLQ stream with alerting.

### MEDIUM-003: Retry-Count Header Never Incremented -- **FIXED**

- **File:** `internal/adapter/nats/nats.go:188-204`
- **Description:** When a handler fails and the message is nak'd with delay (`msg.NakWithDelay(nakDelay)`), the `Retry-Count` header is read but never incremented. The `retryCount(hdrs)` function reads the header, but nak'ing a message does not add/update this header. JetStream redelivery uses its own `NumDelivered` metadata, not a custom header. The `maxRetries` check against this header will only match if a publisher explicitly sets `Retry-Count: 3`.
- **Impact:** The retry-count-based DLQ escalation in `handleMessage` never triggers via organic redelivery. Messages that continuously fail will be redelivered up to `MaxDeliver: 4` times by JetStream (line 146), after which JetStream drops them -- but the Go code's DLQ logic based on `Retry-Count` header is dead code for naturally redelivered messages.
- **Recommendation:** Either (a) use JetStream's `msg.Metadata().NumDelivered` to count retries instead of a custom header, or (b) increment the `Retry-Count` header before nak'ing. Option (a) is preferred as it uses the built-in mechanism.

### MEDIUM-004: Python Defines SUBJECT_CONVERSATION_COMPACT_COMPLETE Without Go Counterpart -- **FIXED**

- **File:** `workers/codeforge/consumer/_subjects.py:64`
- **Description:** Python defines `SUBJECT_CONVERSATION_COMPACT_COMPLETE = "conversation.compact.complete"` and publishes to it in `_compact.py:56`. However, Go's `queue.go` has no corresponding `SubjectConversationCompactComplete` constant, and no Go subscriber listens for this subject.
- **Impact:** Compact completion results are published to NATS but never consumed. The messages accumulate in the JetStream stream (captured by `conversation.>`) without being processed. The Go side that triggered the compact has no way to know it completed.
- **Recommendation:** Add `SubjectConversationCompactComplete = "conversation.compact.complete"` to Go's `queue.go` and subscribe to it in the compact trigger service.

### MEDIUM-005: Validator Accepts Empty JSON Objects as Valid -- **FIXED**

- **File:** `internal/port/messagequeue/validator.go:58` and `validator_test.go:63-69`
- **Description:** The validator uses `json.Unmarshal(data, target)` which succeeds for `{}` against any struct (all fields get zero values). The test `TestValidateEmptyJSON` explicitly verifies this behavior. However, payloads like `TaskResultPayload` with an empty `task_id` or `BenchmarkRunRequestPayload` with an empty `run_id` are semantically invalid.
- **Impact:** Empty or near-empty payloads pass validation and reach handlers, which must then handle missing required fields individually. This pushes validation responsibility to each handler.
- **Recommendation:** Add required-field checks for key identifiers (e.g., `run_id`, `task_id`, `project_id` must be non-empty) as a post-unmarshal step.

### LOW-001: Contract Tests Do Not Cover All Subjects

- **File:** `internal/port/messagequeue/contract_test.go:449-473`
- **Description:** `allFixtures()` returns 22 entries covering conversation, benchmark, GEMMAS, repomap, retrieval, graph, and A2A subjects. Missing: run protocol (runs.start, runs.complete, runs.toolcall.*), quality gate, context reranking, MCP, memory, handoff, backend health, review, prompt evolution, trajectory, and heartbeat subjects.
- **Impact:** JSON contract drift for uncovered subjects would not be caught by automated tests.
- **Recommendation:** Extend `allFixtures()` to cover all subject-payload pairs.

### LOW-002: Inconsistent Consumer Name Prefixes

- **File:** `internal/adapter/nats/nats.go:138` and `workers/codeforge/consumer/_subjects.py:116`
- **Description:** Go uses prefix `codeforge-go-` and Python uses `codeforge-py-`. This is correct and intentional for consumer isolation. However, neither the prefix nor the naming scheme is documented, making it non-obvious why two consumers exist for the same subject.
- **Impact:** No functional impact. Minor documentation gap.
- **Recommendation:** Add a brief comment in each file explaining the dual-consumer design.

### LOW-003: Python _benchmark.py Uses Bare except at Line 96-97 -- **FIXED**

- **File:** `workers/codeforge/consumer/_benchmark.py:96-97`
- **Description:** `_fetch_available_models()` uses `except Exception:` with no logging (`return []`). Similarly `_fetch_configured_models()` at line 130. While these are fallback paths, silently swallowing errors violates the project's coding principle "Errors should never pass silently."
- **Impact:** Debugging LiteLLM connectivity issues is harder when model-fetch errors are silently swallowed.
- **Recommendation:** Add `logger.debug("failed to fetch models", exc_info=True)` to these exception handlers.

---

## Summary & Recommendations

### Priority 1 (Fix Before Production)

1. **CRITICAL-001:** Add NATS reconnect options, disconnect/reconnect handlers, and error handler to `Connect()`. This is a single-point-of-failure in the current architecture.
2. **HIGH-001:** Extend the validator to cover all 50+ subjects. Generate the switch statement from a registry if needed.
3. **HIGH-003:** Add `tenant_id` to `RunStartPayload` to match the Python contract.

### Priority 2 (Fix Soon)

4. **HIGH-002:** Standardize token count types to `int64` across all Go payloads.
5. **MEDIUM-003:** Switch retry counting to use JetStream's `msg.Metadata().NumDelivered` instead of the never-incremented `Retry-Count` header.
6. **MEDIUM-004:** Add `SubjectConversationCompactComplete` to Go and subscribe to it.
7. **MEDIUM-005:** Add required-field validation after unmarshal in the validator.

### Priority 3 (Improve)

8. **MEDIUM-001:** Add `asyncio.Lock` to `_is_duplicate` or document the single-loop-per-subject invariant.
9. **MEDIUM-002:** Add DLQ monitoring or alerting.
10. **LOW-001:** Extend contract tests to all subject-payload pairs.
11. **LOW-002:** Document consumer naming convention.
12. **LOW-003:** Add debug logging to silently-caught exceptions in benchmark model fetching.

### Architecture Strengths Worth Preserving

- The hexagonal port/adapter separation (Queue interface in `queue.go`, NATS implementation in `nats.go`) is clean and testable.
- The mixin-based Python consumer architecture scales well as new subject groups are added.
- The contract test infrastructure with fixture generation is a strong foundation -- it just needs broader coverage.
- The dual deduplication strategy (JetStream `Nats-Msg-Id` at the publisher + in-memory `_processed_ids` at the consumer) provides defense in depth.

---

## Fix Status

| Severity | Total | Fixed | Unfixed |
|----------|------:|------:|--------:|
| CRITICAL | 1     | 1     | 0       |
| HIGH     | 3     | 3     | 0       |
| MEDIUM   | 5     | 4     | 1       |
| LOW      | 3     | 1     | 2       |
| **Total**| **12**| **9** | **3**   |

**Post-fix score:** 100 - (0 CRITICAL x 15) - (0 HIGH x 5) - (1 MEDIUM x 2) - (2 LOW x 1) = **96/100 -- Grade: A**

**Remaining unfixed findings:**
- MEDIUM-001: `_processed_ids` not thread-safe (asyncio)
- LOW-001: Contract tests do not cover all subjects
- LOW-002: Inconsistent consumer name prefixes undocumented
