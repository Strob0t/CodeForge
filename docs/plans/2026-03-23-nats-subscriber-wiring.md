# NATS Subscriber Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Go subscribers for 5 broken NATS subjects where Python publishes results that Go ignores, and remove 4 dead subject constants.

**Architecture:** Add payload structs in schemas.go, implement handler methods in conversation/review/prompt_evolution services, wire subscriptions in main.go, remove unused constants.

**Tech Stack:** Go, NATS JetStream, PostgreSQL

---

### Task 1: Add payload structs for compact-complete and review-trigger-complete

**Files:**
- Modify: `internal/port/messagequeue/schemas.go`
- Modify: `internal/port/messagequeue/validator.go`
- Modify: `internal/port/messagequeue/contract_test.go`

- [ ] **Step 1: Add ConversationCompactCompletePayload struct**

Match Python _compact.py publish payload:
```go
type ConversationCompactCompletePayload struct {
    ConversationID string `json:"conversation_id"`
    TenantID       string `json:"tenant_id"`
    Summary        string `json:"summary"`
    OriginalCount  int    `json:"original_count"`
    Status         string `json:"status"`
}
```

- [ ] **Step 2: Add ReviewTriggerCompletePayload struct**

Match Python _review.py publish payload:
```go
type ReviewTriggerCompletePayload struct {
    ProjectID string `json:"project_id"`
    TenantID  string `json:"tenant_id"`
    CommitSHA string `json:"commit_sha"`
    Status    string `json:"status"`
    RunID     string `json:"run_id"`
}
```

- [ ] **Step 3: Add validator cases and contract test fixtures**

- [ ] **Step 4: Run tests**

```bash
go test ./internal/port/messagequeue/... -run TestContract -v
```

---

### Task 2: Implement HandleCompactComplete

**Files:**
- Modify: `internal/service/conversation.go`
- Create: `internal/service/conversation_compact_test.go`

- [ ] **Step 1: Write failing tests**

Test cases: completed summary, missing conversation_id, malformed JSON, non-completed status noop, empty summary.

- [ ] **Step 2: Implement handler**

```go
func (s *ConversationService) HandleCompactComplete(ctx context.Context, _ string, data []byte) error {
    var p messagequeue.ConversationCompactCompletePayload
    if err := json.Unmarshal(data, &p); err != nil { return fmt.Errorf("unmarshal: %w", err) }
    if p.ConversationID == "" { return errors.New("missing conversation_id") }
    if p.Status != "completed" { slog.Warn("compact not completed", "status", p.Status); return nil }
    // Update conversation state, log success
    slog.Info("compact complete", "conversation_id", p.ConversationID, "original_count", p.OriginalCount)
    return nil
}
```

- [ ] **Step 3: Add StartCompactSubscriber method**

- [ ] **Step 4: Run tests**

```bash
go test ./internal/service/... -run TestHandleCompactComplete -v
```

---

### Task 3: Implement HandleReflectComplete

**Files:**
- Modify: `internal/service/prompt_evolution.go`
- Modify: `internal/service/prompt_evolution_test.go`

- [ ] **Step 1: Write failing tests**

Test cases: happy path with fixes, error payload, malformed JSON.

- [ ] **Step 2: Implement handler**

Unmarshal PromptEvolutionReflectCompletePayload, log tactical fixes and strategic principles.

- [ ] **Step 3: Add StartSubscribers method**

Subscribe to both reflect.complete and mutate.complete subjects.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/service/... -run TestHandleReflectComplete -v
```

---

### Task 4: Implement HandleReviewTriggerComplete

**Files:**
- Modify: `internal/service/review_trigger.go`
- Create: `internal/service/review_trigger_complete_test.go`

- [ ] **Step 1: Write failing tests**

Test cases: dispatched status success, malformed JSON, missing project_id.

- [ ] **Step 2: Implement handler**

Log review trigger completion with project_id, commit_sha, status, run_id.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/service/... -run TestHandleReviewTriggerComplete -v
```

---

### Task 5: Wire subscribers in main.go

**Files:**
- Modify: `cmd/codeforge/main.go`

- [ ] **Step 1: Add subscription calls**

```go
convCompactCancel, err := conversationSvc.StartCompactSubscriber(ctx)
evoCancels, err := evoSvc.StartSubscribers(ctx)
reviewTriggerCancel, err := queue.Subscribe(ctx, mq.SubjectReviewTriggerComplete, reviewTriggerSvc.HandleReviewTriggerComplete)
```

- [ ] **Step 2: Add shutdown cancels**

- [ ] **Step 3: Build verification**

```bash
go build ./cmd/codeforge/
```

---

### Task 6: Remove 4 dead subject constants

**Files:**
- Modify: `internal/port/messagequeue/queue.go` — remove 4 constants
- Modify: `internal/port/messagequeue/validator.go` — remove 2 switch cases
- Modify: `internal/port/messagequeue/queue_test.go` — remove 4 map entries
- Modify: `internal/port/messagequeue/contract_test.go` — remove fixtures
- Modify: `workers/codeforge/consumer/_subjects.py` — remove 2 constants

Constants to remove:
- `SubjectReviewBoundaryAnalyzed`
- `SubjectReviewApprovalResponse`
- `SubjectMCPServerStatus`
- `SubjectMCPToolsDiscovered`

- [ ] **Step 1: Remove from Go**
- [ ] **Step 2: Remove from Python**
- [ ] **Step 3: Run tests**

```bash
go test ./internal/port/messagequeue/... -v && go build ./cmd/codeforge/
```

---

### Task 7: Final verification and commit

- [ ] **Step 1: Run full Go test suite**

```bash
go test ./internal/... -v 2>&1 | tail -30
```

- [ ] **Step 2: Run golangci-lint**

```bash
golangci-lint run ./internal/... ./cmd/...
```

- [ ] **Step 3: Commit**

```
feat: wire Go subscribers for 5 broken NATS subjects

Add handlers for conversation.compact.complete,
review.trigger.complete, prompt.evolution.reflect.complete,
and prompt.evolution.mutate.complete. Remove 4 dead subject
constants (review.boundary.analyzed, review.approval.response,
mcp.server.status, mcp.tools.discovered).
```
