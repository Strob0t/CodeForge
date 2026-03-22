# Audit Remaining Fixes — Atomic Work Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** Fix all remaining actionable findings from the final audit (`docs/audits/full-feature-audit-2026-03-21.md`)

**Source:** Final audit 2026-03-22 (22 features, 11 NEEDS WORK)

---

## Task 1: Image Validation — 15MB Limit + Enforce Validate()

**Best Practice:** Server-side validation must be enforced at the HTTP handler boundary, not just defined in the domain. The domain defines the rule, the handler enforces it. Client-side limits are UX only — never trust the client.

**Why 15MB:** LLM vision APIs (Claude, GPT-4o) accept images up to 20MB. 15MB gives headroom for base64 encoding overhead (~33%) while staying under the 20MB wire limit.

**Files:**
- Modify: `internal/domain/conversation/conversation.go:29-30`
- Modify: `internal/service/conversation.go:222-227`
- Modify: `frontend/src/features/canvas/tools/ImageTool.ts:10`

- [ ] **Step 1:** Change `MaxImageSizeBytes` from `5 * 1024 * 1024` to `15 * 1024 * 1024` in `conversation.go:29-30`
- [ ] **Step 2:** Change `MAX_FILE_SIZE_BYTES` from `5 * 1024 * 1024` to `15 * 1024 * 1024` in `ImageTool.ts:10`
- [ ] **Step 3:** In `conversation.go` `SendMessage`, after creating `userMsg` (line 227), add image validation loop:
```go
for i := range req.Images {
    if err := req.Images[i].Validate(); err != nil {
        return nil, fmt.Errorf("image %d: %w", i, err)
    }
}
```
- [ ] **Step 4:** Write test: `TestSendMessage_ImageValidation` — message with oversized image returns error
- [ ] **Step 5:** Run tests, commit

---

## Task 2: Quarantine Docstring — Fix fail-open to fail-closed

**Best Practice:** Security components should be fail-closed (block on error). The code already does this correctly — only the docstring is wrong. Docstrings that contradict code are worse than no docstrings because they mislead reviewers.

**Files:**
- Modify: `internal/service/quarantine.go:31-33`

- [ ] **Step 1:** Replace docstring lines 31-33:
```
Before: "Follows a fail-open policy: if evaluation errors, the message is allowed through."
After:  "Follows a fail-closed policy: if evaluation or persistence errors, the message is blocked."
```
- [ ] **Step 2:** Commit

---

## Task 3: Quarantine Tenant Isolation

**Best Practice:** Every service method that operates on tenant-scoped data must extract the tenant from context and verify ownership before processing. Pattern used throughout the codebase: `tid := middleware.TenantIDFromContext(ctx)` followed by tenant-scoped store query.

**Files:**
- Modify: `internal/service/quarantine.go:39`

- [ ] **Step 1:** In `Evaluate()`, before scoring, add project ownership check:
```go
// Verify project belongs to caller's tenant.
if projectID != "" {
    if _, err := s.db.GetProject(ctx, projectID); err != nil {
        return true, fmt.Errorf("quarantine project access check: %w", err)
    }
}
```
The store's `GetProject` already filters by `tenant_id` from context. If the project doesn't belong to the tenant, it returns `ErrNotFound`, and quarantine blocks the message (fail-closed).
- [ ] **Step 2:** Write test: `TestQuarantineService_Evaluate_CrossTenantBlocked`
- [ ] **Step 3:** Remove the TODO(F11-D3) comment
- [ ] **Step 4:** Run tests, commit

---

## Task 4: AG-UI Event Emission — ToolCall and ToolResult

**Best Practice:** AG-UI (CopilotKit protocol) expects 8 event types for real-time streaming. Currently only RunStarted and RunFinished are emitted from Go. ToolCall/ToolResult events are emitted from the Python worker via NATS → Go → WebSocket. The Go service receives these as `conversation.run.complete` payloads containing the full message history — but individual tool calls are not streamed in real-time.

**Analysis:** The Python worker (`agent_loop.py`) publishes AG-UI events directly via NATS. The Go `HandleConversationRunComplete` handler processes the final result. Real-time tool streaming happens via the NATS → WS bridge in the WebSocket handler, NOT via the conversation service. This means the events ARE being emitted — they just flow through a different path than the service layer.

**Finding re-evaluation:** The audit agent looked in `conversation_agent.go` for ToolCall emissions and didn't find them. But they flow through the WS adapter layer. This needs verification, not a code change.

**Files:**
- Read: `internal/adapter/ws/handler.go` or equivalent WS bridge

- [ ] **Step 1:** Verify the NATS → WS bridge forwards `text_message_content`, `tool_call_start`, `tool_call_result` events from Python to the frontend
- [ ] **Step 2:** If events ARE forwarded: document in `docs/features/05-chat-enhancements.md` that AG-UI tool events flow via NATS→WS bridge, not conversation service
- [ ] **Step 3:** If events are NOT forwarded: add forwarding in the WS handler for these event types
- [ ] **Step 4:** Commit

---

## Task 5: A2A Task Completion Callback — ALREADY IMPLEMENTED

**Finding re-evaluation:** The audit flagged "A2A task result callback to remote agents missing." Research shows this is **incorrect**:
- `internal/service/a2a.go:445-470`: `HandleTaskComplete` updates task state AND calls `DispatchPushNotifications`
- `internal/service/a2a.go:377`: `DispatchPushNotifications` sends webhook POST to all push configs
- `internal/service/a2a.go:486`: NATS handler wires `HandleTaskComplete` to incoming completion messages

**Action:** No code change needed. Mark as false positive in audit.

- [ ] **Step 1:** Update audit report to mark A2A callback as FIXED (was false positive)
- [ ] **Step 2:** Commit

---

## Task 6: AuthProvider Race Condition

**Best Practice:** In SPAs, token refresh should be coordinated across tabs using BroadcastChannel API (modern) or localStorage events (fallback). Only one tab should refresh at a time. The current code has jitter (added in prior fix) which reduces thundering herd, but doesn't prevent the race where concurrent `refreshTokens()` calls within the SAME tab overwrite each other's timer.

**Analysis after code review:** The actual race is minor — `scheduleRefresh` always clears the previous timer before setting a new one (line 51). Two concurrent `refreshTokens()` calls would both succeed, both call `scheduleRefresh`, and the last one wins. This is safe because:
1. Both calls set the same token (same refresh endpoint returns same access_token)
2. The timer is always reset correctly (clearTimeout + setTimeout)
3. Jitter prevents exact simultaneous calls

The bare `catch` (line 72) is the real issue — it logs out on ANY error, including transient network failures.

**Files:**
- Modify: `frontend/src/components/AuthProvider.tsx:65-77`

- [ ] **Step 1:** Distinguish auth errors from network errors:
```typescript
const refreshTokens = async (): Promise<boolean> => {
    try {
      const resp = await api.auth.refresh();
      setAccessToken(resp.access_token);
      setUser(resp.user);
      scheduleRefresh(resp.expires_in);
      return true;
    } catch (err: unknown) {
      // Only logout on auth failures (401/403). Retry on transient errors.
      if (err instanceof Error && "status" in err && (err as { status: number }).status >= 400 && (err as { status: number }).status < 500) {
        setAccessToken(null);
        setUser(null);
        return false;
      }
      // Network error — retry in 30s
      scheduleRefresh(90);
      return false;
    }
  };
```
- [ ] **Step 2:** Run frontend tests, commit

---

## Task 7: Config Loader — Log Parse Errors

**Best Practice:** Configuration parsing should never fail silently. Invalid values should produce a warning log so operators can diagnose misconfigurations. The current code skips invalid values without any feedback — an operator setting `CODEFORGE_RATE_RPS=abc` gets the default with no indication.

**Files:**
- Modify: `internal/config/loader.go:386-446`

- [ ] **Step 1:** Add `slog` import if not present
- [ ] **Step 2:** Add warning log to each `set*` helper. Example for `setInt`:
```go
func setInt(dst *int, key string) {
    if v := os.Getenv(key); v != "" {
        n, err := strconv.Atoi(v)
        if err != nil {
            slog.Warn("ignoring invalid config value", "key", key, "value", v, "error", err)
            return
        }
        *dst = n
    }
}
```
Apply same pattern to `setInt32`, `setFloat64`, `setInt64`, `setBool`, `setDuration`.
- [ ] **Step 3:** Run config tests, commit

---

## Task 8: MCP Tenant Context Injection

**Best Practice:** Multi-tenant APIs must inject tenant identity at the middleware layer, not at the handler level. The pattern used by the main HTTP router: `TenantMiddleware` extracts tenant from JWT and calls `tenantctx.WithTenant(ctx, tid)`. MCP uses API keys instead of JWT, so the middleware needs a key→tenant lookup.

**Files:**
- Modify: `internal/adapter/mcp/auth.go`
- Modify: `internal/adapter/mcp/server.go` (add store dependency for key→tenant lookup)

- [ ] **Step 1:** Add tenant context injection to AuthMiddleware. For now, use a simple approach — map the configured API key to the default tenant:
```go
func AuthMiddleware(apiKey string, tenantID string, next http.Handler) http.Handler {
    // ... existing auth check ...
    ctx := tenantctx.WithTenant(r.Context(), tenantID)
    next.ServeHTTP(w, r.WithContext(ctx))
}
```
- [ ] **Step 2:** Update `NewServer` to pass `cfg.TenantID` (or default tenant) to AuthMiddleware
- [ ] **Step 3:** Update server_test.go to verify tenant context is set
- [ ] **Step 4:** Run tests, commit

---

## Task 9: FIX-032 — Missing Orchestration Tests

**Best Practice:** Test TODOs should be tracked as backlog items and executed systematically. The codebase has 8+ TODO(FIX-032) markers identifying specific missing test cases. These are not bugs — they're coverage gaps.

**Files:**
- Modify: `internal/service/files_source_test.go`
- Modify: `internal/service/orchestrator_consensus_test.go`
- Modify: `internal/service/runtime_execution_source_test.go`
- Modify: `internal/service/runtime_lifecycle_source_test.go`

- [ ] **Step 1:** Read all TODO(FIX-032) markers across the 4 test files
- [ ] **Step 2:** Implement each listed test case (8+ cases total)
- [ ] **Step 3:** Remove TODO markers as cases are implemented
- [ ] **Step 4:** Run tests, commit per file

---

## Task 10: AG-UI Event Tests

**Best Practice:** Protocol event types should have serialization round-trip tests. Currently only GoalProposal has tests. Add tests for all 8 AG-UI event types.

**Files:**
- Modify: `internal/adapter/ws/agui_events_test.go`

- [ ] **Step 1:** Add table-driven tests for JSON marshaling of RunStarted, RunFinished, TextMessage, ToolCall, ToolResult, StateDelta, StepStarted, StepFinished
- [ ] **Step 2:** Verify all required fields serialize correctly
- [ ] **Step 3:** Run tests, commit

---

## Summary

| Task | Problem | Effort | Priority |
|------|---------|:------:|:--------:|
| 1 | Image 15MB limit + enforce Validate() | 30 min | P0 |
| 2 | Quarantine docstring fix | 5 min | P0 |
| 3 | Quarantine tenant isolation | 30 min | P0 |
| 4 | AG-UI event emission verification | 1-2h | P1 |
| 5 | A2A callback — false positive | 5 min | P0 |
| 6 | AuthProvider error classification | 30 min | P1 |
| 7 | Config loader parse warnings | 30 min | P1 |
| 8 | MCP tenant context injection | 1-2h | P1 |
| 9 | FIX-032 orchestration tests | 4-8h | P2 |
| 10 | AG-UI event tests | 1-2h | P2 |

**Total: 10 tasks, ~10-16h effort**
