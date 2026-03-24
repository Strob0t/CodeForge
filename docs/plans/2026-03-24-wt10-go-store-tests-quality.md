# WT-10: Go Store Tests + Code Quality — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add integration tests for the most-called untested store methods, extract helper functions from large service functions (>200 LOC), and fix the ignored filepath.Rel error.

**Architecture:** Store tests use real PostgreSQL via `DATABASE_URL` env var (skip if absent). Large functions are decomposed into focused helpers. All changes are backward-compatible.

**Tech Stack:** Go 1.25, pgx v5, table-driven tests, existing `setupStore(t)` pattern

**Best Practice:**
- Go database testing: Real DB > testcontainers > mocks for store layer. Use `t.Skip()` for CI without DB.
- Function size: <50 LOC ideal. Extract when a function has >3 responsibilities.
- Error handling: Never ignore — log or propagate with context.

---

### Task 1: Store Integration Tests — Message Operations

**Files:**
- Modify: `internal/adapter/postgres/store_test.go`

- [ ] **Step 1: Write tests for CreateMessage + ListMessages**

```go
func TestStore_MessageCRUD(t *testing.T) {
    store, ctx := setupStore(t)

    // Setup: create tenant, project, conversation
    tid := createTestTenant(t, store)
    tctx := ctxWithTenant(t, tid)
    // ... create project + conversation

    tests := []struct {
        name string
        fn   func(t *testing.T)
    }{
        {"create and list messages", func(t *testing.T) { /* ... */ }},
        {"list empty conversation", func(t *testing.T) { /* ... */ }},
        {"wrong tenant returns empty", func(t *testing.T) { /* ... */ }},
        {"search messages by content", func(t *testing.T) { /* ... */ }},
    }
    for _, tt := range tests {
        t.Run(tt.name, tt.fn)
    }
}
```

- [ ] **Step 2: Run tests**

```bash
DATABASE_URL="..." go test ./internal/adapter/postgres/... -run TestStore_MessageCRUD -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/postgres/store_test.go
git commit -m "test: add store integration tests for message CRUD + search"
```

---

### Task 2: Store Integration Tests — Roadmap + Goals + MCP

**Files:**
- Modify: `internal/adapter/postgres/store_test.go`

- [ ] **Step 1: Write tests for Roadmap operations**

```go
func TestStore_RoadmapCRUD(t *testing.T) {
    // CreateRoadmap, GetRoadmapByProject, UpdateRoadmap, DeleteRoadmap
    // CreateMilestone, ListMilestones, UpdateMilestone, DeleteMilestone
    // CreateFeature, ListFeatures, UpdateFeature, DeleteFeature
}
```

- [ ] **Step 2: Write tests for Goal operations**

```go
func TestStore_GoalCRUD(t *testing.T) {
    // CreateProjectGoal, ListProjectGoals, GetProjectGoal, UpdateProjectGoal, DeleteProjectGoal
}
```

- [ ] **Step 3: Write tests for MCP server operations**

```go
func TestStore_MCPServerCRUD(t *testing.T) {
    // CreateMCPServer, GetMCPServer, ListMCPServersByProject
    // AssignMCPServerToProject, UnassignMCPServerFromProject
}
```

- [ ] **Step 4: Run all new tests**

```bash
DATABASE_URL="..." go test ./internal/adapter/postgres/... -run "TestStore_Roadmap|TestStore_Goal|TestStore_MCP" -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/postgres/store_test.go
git commit -m "test: add store integration tests for roadmap, goals, MCP servers"
```

---

### Task 3: Extract Helper Functions from SendMessageAgentic (254 LOC)

**Files:**
- Modify: `internal/service/conversation_agent.go`

- [ ] **Step 1: Extract resolveFullAutoGate() (~30 LOC)**

Move lines ~292-320 (full-auto check + goal_researcher redirect) into:
```go
func (cs *ConversationService) resolveFullAutoGate(ctx context.Context, proj *project.Project, content string, convID string) (redirected bool, err error)
```

- [ ] **Step 2: Extract resolveModelAndMode() (~38 LOC)**

Move lines ~351-388 (model resolution + mode lookup + ModePayload assembly):
```go
func (cs *ConversationService) resolveModelAndMode(ctx context.Context, convID, modeID string) (model string, mode *messagequeue.ModePayload, autonomy int, err error)
```

- [ ] **Step 3: Extract buildMCPDefinitions() (~17 LOC)**

Move lines ~412-428:
```go
func (cs *ConversationService) buildMCPDefinitions(ctx context.Context, projectID string) []messagequeue.MCPServerDefPayload
```

- [ ] **Step 4: Extract matchMicroagentsAndReminders() (~22 LOC)**

Move lines ~435-456:
```go
func (cs *ConversationService) matchMicroagentsAndReminders(ctx context.Context, projectID, content, convID string, history []messagequeue.ConversationMessagePayload) (microagents []messagequeue.MicroagentPayload, reminders []string)
```

- [ ] **Step 5: Verify SendMessageAgentic is now ~120 LOC**

The function should now be a clear orchestration flow:
1. Validate → 2. Load conversation/project → 3. Full-auto gate → 4. Store message → 5. Resolve model/mode → 6. Build MCP/microagents → 7. Assemble payload → 8. Publish

- [ ] **Step 6: Run tests + lint**

```bash
go test ./internal/service/... -count=1
golangci-lint run ./internal/service/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/service/conversation_agent.go
git commit -m "refactor: extract 4 helpers from SendMessageAgentic (254->~120 LOC)"
```

---

### Task 4: Extract Helper Functions from StartRun (280 LOC)

**Files:**
- Modify: `internal/service/runtime.go`

- [ ] **Step 1: Extract validateRunPolicy() (~40 LOC)**

Policy validation + mode resolution logic.

- [ ] **Step 2: Extract prepareSandbox() (~30 LOC)**

Sandbox setup + container configuration.

- [ ] **Step 3: Extract buildRunPayload() (~35 LOC)**

NATS payload assembly.

- [ ] **Step 4: Verify StartRun is now ~150 LOC**

- [ ] **Step 5: Run tests + commit**

```bash
go test ./internal/service/... -count=1
git add internal/service/runtime.go
git commit -m "refactor: extract 3 helpers from StartRun (280->~150 LOC)"
```

---

### Task 5: Fix filepath.Rel Ignored Error

**Files:**
- Modify: `internal/adapter/autospec/provider.go:59`

- [ ] **Step 1: Handle the error with fallback**

```go
rel, err := filepath.Rel(workspacePath, path)
if err != nil {
    slog.Warn("failed to compute relative path", "workspace", workspacePath, "path", path, "error", err)
    rel = filepath.Base(path)
}
```

- [ ] **Step 2: Run tests + commit**

```bash
go test ./internal/adapter/autospec/... -count=1
git add internal/adapter/autospec/provider.go
git commit -m "fix: handle filepath.Rel error with fallback to basename (F-024)"
```
