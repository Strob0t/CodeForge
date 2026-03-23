# WT-3: Go Error Handling Sweep — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all ignored errors, panics-in-constructors, dead stores, and `interface{}` usage across the Go codebase.

**Architecture:** Systematic sweep through all adapter and service files. Each fix follows the same pattern: handle the error explicitly, wrap with context using `fmt.Errorf("...: %w", err)`, or use the existing `logBestEffort` pattern for best-effort operations.

**Tech Stack:** Go 1.25, `fmt.Errorf`, `slog`, existing `logBestEffort` pattern from `internal/service/log_best_effort.go`

**Best Practice:** Go proverb "Errors are values" — never discard them. For HTTP response body reading, always check the error even if the response will be used for error messages. For `json.Marshal`, the only types that can fail are channels, functions, and complex numbers — but wrapping the error is still correct practice for defense-in-depth.

---

### Task 1: Fix `io.ReadAll` Ignored Errors (14 occurrences)

**Files:**
- Modify: `internal/adapter/auth/anthropic.go:70,112,152`
- Modify: `internal/adapter/auth/github.go:76,116`
- Modify: `internal/adapter/copilot/client.go:106`
- Modify: `internal/adapter/discord/notifier.go:94`
- Modify: `internal/adapter/litellm/client.go:458`
- Modify: `internal/adapter/plane/provider.go:180,223,268,310`
- Modify: `internal/adapter/slack/notifier.go:95`
- Modify: `internal/service/project.go:776`

- [ ] **Step 1: Fix anthropic.go (3 occurrences)**

Replace each `respBody, _ := io.ReadAll(resp.Body)` with:
```go
respBody, err := io.ReadAll(resp.Body)
if err != nil {
    return nil, fmt.Errorf("reading response body: %w", err)
}
```
Apply at lines 70, 112, 152.

- [ ] **Step 2: Fix github.go (2 occurrences)**

Same pattern at lines 76, 116.

- [ ] **Step 3: Fix copilot/client.go (1 occurrence)**

Same pattern at line 106.

- [ ] **Step 4: Fix discord/notifier.go (1 occurrence)**

At line 94, the error is inside a conditional. Use `logBestEffort` or return:
```go
respBody, err := io.ReadAll(resp.Body)
if err != nil {
    return fmt.Errorf("reading discord response: %w", err)
}
```

- [ ] **Step 5: Fix litellm/client.go (1 occurrence)**

At line 458, same pattern.

- [ ] **Step 6: Fix plane/provider.go (4 occurrences)**

At lines 180, 223, 268, 310. Same pattern with appropriate error wrapping.

- [ ] **Step 7: Fix slack/notifier.go (1 occurrence)**

At line 95. Same pattern.

- [ ] **Step 8: Fix service/project.go (1 occurrence)**

At line 776, same pattern.

- [ ] **Step 9: Run tests and lint**

```bash
go vet ./internal/...
golangci-lint run ./internal/...
go test ./internal/adapter/auth/... ./internal/adapter/copilot/... ./internal/adapter/discord/... ./internal/adapter/litellm/... ./internal/adapter/plane/... ./internal/adapter/slack/... ./internal/service/... -count=1
```

- [ ] **Step 10: Commit**

```bash
git add internal/adapter/ internal/service/project.go
git commit -m "fix: handle io.ReadAll errors in 14 adapter/service locations"
```

---

### Task 2: Fix `json.Marshal` Ignored Errors (20+ occurrences without nolint)

**Files:**
- Modify: `internal/adapter/gitea/provider.go:152,183`
- Modify: `internal/adapter/gitlab/provider.go:106,134`
- Modify: `internal/adapter/http/handlers_a2a.go:241`
- Modify: `internal/service/a2a.go:92,119,179,181,293,385`
- Modify: `internal/service/backend_health.go:60`
- Modify: `internal/service/conversation.go:334`
- Modify: `internal/service/orchestrator_consensus.go:390`
- Modify: `internal/service/prompt_evolution.go:209,244`
- Modify: `internal/service/session.go:75,112,140,177`

- [ ] **Step 1: Fix gitea/provider.go and gitlab/provider.go**

Replace `payloadJSON, _ := json.Marshal(payload)` with:
```go
payloadJSON, err := json.Marshal(payload)
if err != nil {
    return fmt.Errorf("marshaling webhook payload: %w", err)
}
```

- [ ] **Step 2: Fix handlers_a2a.go**

At line 241:
```go
payload, err := json.Marshal(data)
if err != nil {
    http.Error(w, "internal error", http.StatusInternalServerError)
    return
}
```

- [ ] **Step 3: Fix service/a2a.go (6 occurrences)**

For lines 92, 119 (card marshaling): return error.
For lines 179, 181 (history/artifacts): return error.
For lines 293, 385 (map marshaling): return error.

- [ ] **Step 4: Fix backend_health.go, conversation.go, orchestrator_consensus.go**

Same pattern — handle error, return wrapped error.

- [ ] **Step 5: Fix prompt_evolution.go (2 occurrences)**

Lines 209, 244 — handle error.

- [ ] **Step 6: Fix session.go (4 occurrences)**

Lines 75, 112, 140, 177 — these marshal simple `map[string]string` which cannot fail in practice, but handle for consistency:
```go
meta, err := json.Marshal(map[string]string{...})
if err != nil {
    return fmt.Errorf("marshaling session metadata: %w", err)
}
```

- [ ] **Step 7: Run tests and lint**

```bash
golangci-lint run ./internal/...
go test ./internal/adapter/gitea/... ./internal/adapter/gitlab/... ./internal/adapter/http/... ./internal/service/... -count=1
```

- [ ] **Step 8: Commit**

```bash
git add internal/
git commit -m "fix: handle json.Marshal errors across adapter and service layers"
```

---

### Task 3: Replace Panics with Error Returns in MCP Server

**Files:**
- Modify: `internal/adapter/mcp/server.go:64-72`

- [ ] **Step 1: Change NewServer signature**

From:
```go
func NewServer(cfg ServerConfig, deps ServerDeps) *Server {
    if deps.ProjectLister == nil {
        panic("MCP ServerDeps.ProjectLister must not be nil")
    }
    ...
```

To:
```go
func NewServer(cfg ServerConfig, deps ServerDeps) (*Server, error) {
    if deps.ProjectLister == nil {
        return nil, errors.New("MCP ServerDeps.ProjectLister must not be nil")
    }
    if deps.RunReader == nil {
        return nil, errors.New("MCP ServerDeps.RunReader must not be nil")
    }
    if deps.CostReader == nil {
        return nil, errors.New("MCP ServerDeps.CostReader must not be nil")
    }
    // ... rest of constructor
    return &Server{...}, nil
}
```

- [ ] **Step 2: Update all callers of NewServer**

Search: `grep -rn "mcp.NewServer\|mcp\.NewServer" cmd/ internal/`
Update each caller to handle the error return.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/adapter/mcp/... ./cmd/... -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/mcp/ cmd/
git commit -m "fix: replace panics with error returns in MCP NewServer"
```

---

### Task 4: Remove Dead Store and Fix interface{}

**Files:**
- Modify: `internal/adapter/postgres/store_prompt_variant.go:109,122`

- [ ] **Step 1: Fix dead store**

At line 122, remove `_ = argIdx` and either use `argIdx` properly or remove the variable entirely if the query logic doesn't need it.

- [ ] **Step 2: Replace interface{} with any**

At line 109, change:
```go
args := []interface{}{tid}
```
to:
```go
args := []any{tid}
```

- [ ] **Step 3: Search for remaining interface{} usage**

```bash
grep -rn 'interface{}' internal/ --include="*.go" | grep -v _test.go | grep -v vendor | grep -v .worktrees
```
Fix any remaining occurrences.

- [ ] **Step 4: Run pre-commit**

```bash
pre-commit run --all-files
```

- [ ] **Step 5: Commit**

```bash
git add internal/
git commit -m "fix: remove dead store, replace interface{} with any"
```
