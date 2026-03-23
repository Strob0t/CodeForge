# Worktree J: Test Coverage Gaps — Atomic Plan

> **Branch:** `fix/test-coverage-gaps`
> **Effort:** ~1d | **Findings:** 9 | **Risk:** Low (test-only, no production code changes except Q-006)

---

## Task J1: Create branchprotection_test.go (Q-015)

**File:** Create `internal/service/branchprotection_test.go`

8 exported functions to test: `NewBranchProtectionService`, `CreateRule`, `GetRule`, `ListRules`, `UpdateRule`, `DeleteRule`, `CheckBranch`, `CheckMerge`

- [ ] Create mock store implementing required `database.Store` methods
- [ ] Table-driven tests for:
  - CreateRule: valid request, empty pattern, duplicate name
  - GetRule: found, not found
  - ListRules: empty project, with rules
  - UpdateRule: valid, not found
  - DeleteRule: valid, not found
  - CheckBranch: allowed, blocked by pattern match
  - CheckMerge: allowed, blocked
- [ ] Verify: `go test ./internal/service/ -run TestBranchProtection -v -count=1`

**Commit:** `test: add branchprotection service tests (Q-015)`

---

## Task J2: Create channel_test.go (Q-015)

**File:** Create `internal/service/channel_test.go`

10 exported functions to test.

- [ ] Create mock store implementing required `database.Store` methods
- [ ] Table-driven tests for:
  - Create: valid, empty name, nil channel
  - Get: found, not found
  - List: empty project, with channels
  - Delete: valid, not found
  - SendMessage: valid, empty body
  - ListMessages: empty, with messages, pagination cursor
  - AddMember: valid, duplicate
  - UpdateMemberNotify: valid
  - GenerateWebhookKey: returns non-empty, unique
- [ ] Verify: `go test ./internal/service/ -run TestChannel -v -count=1`

**Commit:** `test: add channel service tests (Q-015)`

---

## Task J3: Fix Type-unsafe Test Pattern (Q-003, Q-020)

**File:** `internal/adapter/ws/agui_events_test.go:25-42`

- [ ] Replace `map[string]any` with strongly-typed struct:
```go
var got struct {
    RunID      string `json:"run_id"`
    ProposalID string `json:"proposal_id"`
    Kind       string `json:"kind"`
    Action     string `json:"action"`
    Title      string `json:"title"`
}
if err := json.Unmarshal(data, &got); err != nil {
    t.Fatalf("unmarshal: %v", err)
}
if got.RunID != "run-123" {
    t.Errorf("run_id = %v, want run-123", got.RunID)
}
// ... same for other fields
```
- [ ] Verify: `go test ./internal/adapter/ws/ -v -count=1`

**Commit:** `test: replace map[string]any with typed struct in agui_events_test (Q-003, Q-020)`

---

## Task J4: Add Registry Panic Tests (Q-017)

**File:** Create `internal/port/specprovider/registry_test.go`

- [ ] Test that duplicate registration panics:
```go
func TestRegister_DuplicatePanics(t *testing.T) {
    defer func() {
        if r := recover(); r == nil {
            t.Error("expected panic on duplicate registration")
        }
    }()
    Register("test-dup", func(_ map[string]string) (Provider, error) { return nil, nil })
    Register("test-dup", func(_ map[string]string) (Provider, error) { return nil, nil })
}
```
- [ ] Test `New` with unknown provider returns error
- [ ] Test `Available` returns sorted names
- [ ] Same for `internal/port/pmprovider/registry_test.go`
- [ ] Verify: `go test ./internal/port/specprovider/ ./internal/port/pmprovider/ -v -count=1`

**Commit:** `test: add registry panic and error tests (Q-017)`

---

## Task J5: Fix Uninitialized Ref in ChatPanel (Q-006)

**File:** `frontend/src/features/project/ChatPanel.tsx:92`

- [ ] Change from:
```tsx
let chatFileInputRef: HTMLInputElement | undefined;
```
To:
```tsx
let chatFileInputRef: HTMLInputElement | undefined = undefined;
```
- [ ] Verify: `cd frontend && npx tsc --noEmit`

**Commit:** `fix: explicitly initialize chatFileInputRef (Q-006)`

---

## Verification

- [ ] `go test ./... -count=1 -timeout=120s`
- [ ] `cd frontend && npx tsc --noEmit`
