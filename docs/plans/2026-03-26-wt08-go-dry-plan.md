# Worktree 8: refactor/go-dry — Go DRY-Violations + Code Cleanup

**Branch:** `refactor/go-dry`
**Priority:** Mittel
**Scope:** 6 findings (F-QUA-001, F-QUA-007, F-QUA-008, F-QUA-009, F-QUA-010, F-QUA-011)
**Estimated effort:** Medium (3-5 days)

## Steps

### 1. F-QUA-001: Extract dispatchAgenticRun() — eliminate 80% duplication

**File:** `internal/service/conversation_agent.go:386-709`

Extract shared logic from `SendMessageAgentic` and `SendMessageAgenticWithMode`:
```go
type agenticDispatchOpts struct {
    dedupKey       string
    providerAPIKey string
    rolloutCount   int
    fullAutoGate   bool
    recordMetrics  bool
}

func (s *ConversationAgentService) dispatchAgenticRun(
    ctx context.Context, conv *conversation.Conversation, content string,
    modeID string, opts agenticDispatchOpts,
) error {
    // shared: fetch project, ensure session, build system prompt,
    // resolve model/mode/policy, build context, build payload, publish
}
```

Both public methods become thin wrappers (~20 lines each).

### 2. F-QUA-007: Extract broadcastRunStatus() — eliminate 8x duplication

**Files:** `runtime_execution.go`, `runtime.go`, `runtime_lifecycle.go`

```go
func (s *RuntimeService) broadcastRunStatus(ctx context.Context, r *run.Run, status run.Status) {
    s.hub.BroadcastEvent(ctx, event.EventRunStatus, event.RunStatusEvent{
        RunID: r.ID, TaskID: r.TaskID, ProjectID: r.ProjectID,
        Status: string(status), StepCount: r.StepCount, CostUSD: r.CostUSD,
        Model: r.Model, TokensIn: r.TokensIn, TokensOut: r.TokensOut,
    })
}
```

### 3. F-QUA-008: Extract parseScores() — eliminate 3x duplication

**Files:** `benchmark.go`, `benchmark_result.go`, `handlers_benchmark.go`

```go
func parseScores(raw json.RawMessage) map[string]float64 {
    scores := make(map[string]float64)
    _ = json.Unmarshal(raw, &scores)
    return scores
}
```

### 4. F-QUA-009: Fix swallowed error — use logBestEffort

**File:** `internal/service/autoagent.go:217`

Replace `_ = s.db.UpdateAutoAgentStatus(...)` with:
```go
logBestEffort(ctx, s.db.UpdateAutoAgentStatus(ctx, projectID, finalStatus, errMsg),
    "UpdateAutoAgentStatus", slog.String("project_id", projectID))
```

### 5. F-QUA-010: Replace map[string]any with typed structs (9 locations)

Prioritize `a2a.go:400` (push dispatch) and template data maps. For `text/template` data, define typed structs:
```go
type reminderTemplateData struct {
    TurnCount     int
    BudgetPercent float64
    // ...
}
```

Accept `map[string]any` only where `text/template` requires it, with a comment explaining why.

### 6. F-QUA-011: Fix AuthUserCtxKeyForTest return type

**File:** `internal/middleware/auth.go:237`

```go
// Before:
func AuthUserCtxKeyForTest() any { return authUserCtxKey{} }
// After:
func ContextWithTestUser(ctx context.Context, u *user.User) context.Context {
    return context.WithValue(ctx, authUserCtxKey{}, u)
}
```

## Verification

- All existing tests pass
- `golangci-lint run` passes
- No new `map[string]any` in non-template Go code
- `SendMessageAgentic` and `SendMessageAgenticWithMode` both work identically
