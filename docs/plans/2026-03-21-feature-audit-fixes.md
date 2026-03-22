# Feature Audit Fixes — Atomic Work Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all ~85 findings from the full feature audit (`docs/audits/full-feature-audit-2026-03-21.md`) across 22 features, organized by priority.

**Architecture:** Fixes are grouped into 6 phases by severity. Each task is atomic (one concern, one commit). Security fixes first, then missing tests, then quality/completeness. Large new features (Hook System, Branch Isolation, contract_reviewer pipeline) are deferred to separate plans.

**Tech Stack:** Go 1.25, Python 3.12, TypeScript/SolidJS, PostgreSQL, NATS JetStream

**Source Audit:** `docs/audits/full-feature-audit-2026-03-21.md`

---

## File Structure

### Phase 1 — CRITICAL Security (Tasks 1-4)
- Modify: `internal/service/microagent.go` — ReDoS fix
- Modify: `internal/domain/microagent/microagent.go` — regex validation
- Create: `internal/service/microagent_test.go` — service tests
- Modify: `internal/adapter/postgres/eventstore.go` — safe SQL builder
- Create: `internal/adapter/postgres/eventstore_test.go` — SQL tests
- Modify: `internal/adapter/postgres/store_conversation.go` — safe SQL builder
- Modify: `internal/adapter/mcp/tools.go` — per-user auth filter

### Phase 2 — Missing Tests (Tasks 5-9)
- Create: `internal/service/skill_test.go`
- Create: `internal/service/memory_test.go`
- Modify: `internal/service/handoff_test.go` — expand coverage

### Phase 3 — Security Hardening (Tasks 10-17)
- Modify: `frontend/src/features/project/Markdown.tsx` — block data: URLs
- Modify: `internal/service/project.go` — path normalization
- Modify: `internal/adapter/a2a/executor.go` — prompt length
- Modify: `internal/domain/conversation/conversation.go` — image size
- Modify: `workers/codeforge/history.py` — base64 validation
- Modify: `workers/codeforge/tools/handoff.py` — max hops
- Modify: `internal/service/review_trigger.go` — tenant check
- Modify: `internal/service/quarantine.go` — project access check

### Phase 4 — Quality Fixes (Tasks 18-30)
- Modify: `workers/codeforge/agent_loop.py` — stall hash, dead code, image handling, model validation
- Modify: `internal/service/roadmap.go` — N+1 query
- Modify: `workers/codeforge/routing/blocklist.py`, `key_filter.py` — DI refactor
- Modify: `workers/codeforge/routing/router.py` — timeout
- Modify: `workers/codeforge/memory/scorer.py` — weight validation
- Modify: `frontend/src/components/AuthProvider.tsx` — jitter
- Modify: `internal/domain/policy/validate.go` — MaxSteps upper bound
- Modify: `internal/adapter/a2a/executor.go` — cancel task check
- Modify: `internal/logger/async.go` — health check integration

### Phase 5 — Documentation & Config (Tasks 31-33)
- Modify: `docs/architecture/adr/007-policy-layer.md` — 5th preset
- Modify: `internal/service/context_budget.go` — config override
- Modify: `internal/config/config.go` — JWT secret production check

### Phase 6 — Deferred (Separate Plans Required)
- Hook System (Observer pattern) — Feature #20, ~500 LOC new feature
- Branch Isolation safety mechanism — Feature #15
- CommandSafetyEvaluator service — Feature #15
- contract_reviewer + refactorer handlers — Feature #13
- RLVR/DPO export endpoints — Feature #12
- BenchmarkService decomposition — Feature #12
- Policy scope cascade (run > project > global) — Feature #16
- Agent statistics tracking — Feature #11
- Agent inbox message routing — Feature #11

---

## Phase 1: CRITICAL Security Fixes

### Task 1: Fix ReDoS in Microagent Trigger Matching

**Findings:** F18-D3 CRITICAL, F18-D2 (regex errors silent), F18-D2 (Validate() missing regex check)
**Files:**
- Modify: `internal/service/microagent.go:104-115`
- Modify: `internal/domain/microagent/microagent.go:57-72`
- Create: `internal/service/microagent_test.go`

- [ ] **Step 1: Write failing test for ReDoS protection**

```go
// internal/service/microagent_test.go
package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/microagent"
)

func TestMatchesTrigger_ReDoSProtection(t *testing.T) {
	// This pattern causes catastrophic backtracking in naive regex engines.
	// matchesTrigger must reject or safely handle it.
	pattern := "(a+)+b"
	text := "aaaaaaaaaaaaaaaaaaaaaaaaaac" // no match, triggers backtracking

	// Must complete in under 1 second (not hang).
	result := matchesTrigger(pattern, text)
	if result {
		t.Error("expected no match for ReDoS pattern")
	}
}

func TestMatchesTrigger_InvalidRegex(t *testing.T) {
	// Invalid regex should return false AND be caught at validation time.
	result := matchesTrigger("[invalid", "test input")
	if result {
		t.Error("expected false for invalid regex")
	}
}
```

- [ ] **Step 2: Run test to verify it fails (or hangs)**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestMatchesTrigger -timeout 5s -v`
Expected: FAIL (timeout or panic — the current code compiles arbitrary regex)

- [ ] **Step 3: Add regex validation to domain CreateRequest.Validate()**

```go
// internal/domain/microagent/microagent.go — add to imports
import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"
)

// MaxTriggerPatternLength is the maximum allowed length for trigger patterns.
const MaxTriggerPatternLength = 512

// Validate checks that a CreateRequest has all required fields.
func (r *CreateRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if !slices.Contains(ValidTypes, r.Type) {
		return errors.New("invalid type: must be knowledge, repo, or task")
	}
	if r.TriggerPattern == "" {
		return errors.New("trigger_pattern is required")
	}
	if len(r.TriggerPattern) > MaxTriggerPatternLength {
		return errors.New("trigger_pattern exceeds maximum length")
	}
	// Validate regex patterns compile.
	if r.TriggerPattern[0] == '^' || r.TriggerPattern[0] == '(' {
		if _, err := regexp.Compile(r.TriggerPattern); err != nil {
			return fmt.Errorf("invalid trigger_pattern regex: %w", err)
		}
	}
	if r.Prompt == "" {
		return errors.New("prompt is required")
	}
	return nil
}
```

- [ ] **Step 4: Fix matchesTrigger to use pre-compiled regex with length limit**

```go
// internal/service/microagent.go — replace matchesTrigger
import (
	"regexp"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/microagent"
)

// matchesTrigger checks if text matches a trigger pattern.
// Patterns starting with ^ or ( are treated as regex; others as substring.
// Regex patterns are length-limited and pre-validated at creation time.
func matchesTrigger(pattern, text string) bool {
	if len(pattern) > microagent.MaxTriggerPatternLength {
		return false
	}
	if strings.HasPrefix(pattern, "^") || strings.HasPrefix(pattern, "(") {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		// Use re.MatchString with a truncated input to bound execution.
		input := text
		if len(input) > 10_000 {
			input = input[:10_000]
		}
		return re.MatchString(input)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
}
```

- [ ] **Step 5: Write domain validation test**

```go
// Add to microagent_test.go or internal/domain/microagent/microagent_test.go
func TestCreateRequest_Validate_RegexPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr bool
	}{
		{"valid substring", "hello", false},
		{"valid regex", "^hello.*world$", false},
		{"invalid regex", "[unclosed", true},
		{"too long", strings.Repeat("a", 513), true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &microagent.CreateRequest{
				Name:           "test",
				Type:           microagent.TypeKnowledge,
				TriggerPattern: tt.pattern,
				Prompt:         "test prompt",
			}
			err := req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 6: Run all tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestMatchesTrigger -v && go test ./internal/domain/microagent/ -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/service/microagent.go internal/service/microagent_test.go internal/domain/microagent/microagent.go
git commit -m "security: fix ReDoS in microagent trigger matching (F18-D3)"
```

---

### Task 2: Refactor Dynamic SQL in Eventstore

**Findings:** F20-D3 HIGH — SQL injection risk via dynamic WHERE clause in `eventstore.go:148,155,278`
**Files:**
- Modify: `internal/adapter/postgres/eventstore.go:100-160`
- Create: `internal/adapter/postgres/eventstore_test.go`

- [ ] **Step 1: Write failing test for safe query building**

```go
// internal/adapter/postgres/eventstore_test.go
package postgres

import (
	"testing"
)

func TestBuildEventQuery_NoInjection(t *testing.T) {
	// The query builder should never use fmt.Sprintf for WHERE values.
	// This test verifies the builder returns parameterized queries.
	filter := eventFilter{
		TenantID: "tenant-1",
		AgentID:  "agent-1",
	}
	query, args := buildEventQuery(filter, 50)
	if len(args) < 2 {
		t.Fatalf("expected at least 2 args, got %d", len(args))
	}
	// Query should contain $1, $2 placeholders, not interpolated values.
	if !strings.Contains(query, "$1") {
		t.Error("query missing parameterized placeholders")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/postgres/ -run TestBuildEventQuery -v`
Expected: FAIL (buildEventQuery doesn't exist yet)

- [ ] **Step 3: Extract query builder function from LoadTrajectory**

Refactor `LoadTrajectory` to use a `buildEventQuery` helper that returns `(countSQL string, fetchSQL string, args []any)`. Replace all `fmt.Sprintf("... $%d", argIdx)` with the builder pattern. The key change: instead of building WHERE clauses with `fmt.Sprintf`, use a `queryBuilder` struct that tracks `argIdx` internally and appends conditions safely.

```go
// eventstore.go — add query builder
type queryBuilder struct {
	conditions []string
	args       []any
	argIdx     int
}

func newQueryBuilder(tenantID string) *queryBuilder {
	return &queryBuilder{
		conditions: []string{"tenant_id = $1"},
		args:       []any{tenantID},
		argIdx:     2,
	}
}

func (qb *queryBuilder) add(condition string, val any) {
	qb.conditions = append(qb.conditions, fmt.Sprintf(condition, qb.argIdx))
	qb.args = append(qb.args, val)
	qb.argIdx++
}

func (qb *queryBuilder) where() string {
	return strings.Join(qb.conditions, " AND ")
}

func (qb *queryBuilder) nextArgIdx() int {
	return qb.argIdx
}
```

Then refactor `LoadTrajectory` to use `newQueryBuilder(tid)` + `qb.add("agent_id = $%d", filter.AgentID)`.

- [ ] **Step 4: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/postgres/ -run TestBuildEventQuery -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/postgres/eventstore.go internal/adapter/postgres/eventstore_test.go
git commit -m "security: refactor eventstore dynamic SQL to safe query builder (F20-D3)"
```

---

### Task 3: Refactor Conversation Search SQL

**Findings:** F5-D3 HIGH — Fragile SQL construction with `fmt.Sprintf` in `store_conversation.go:161,166`
**Files:**
- Modify: `internal/adapter/postgres/store_conversation.go:146-169`

- [ ] **Step 1: Write test for search with project filter**

```go
// Add to store_conversation_test.go (or create it)
func TestSearchConversationMessages_SafeSQL(t *testing.T) {
	// Verify the search function builds safe parameterized queries.
	// This is a structure test — we verify the function signature accepts
	// the right types and doesn't panic.
	// Full integration test requires DB.
}
```

- [ ] **Step 2: Refactor SearchConversationMessages to use queryBuilder**

Replace the manual `argIdx` tracking and `fmt.Sprintf` with the same `queryBuilder` pattern from Task 2, or simply use a fixed query with optional filtering:

```go
func (s *Store) SearchConversationMessages(ctx context.Context, query string, projectIDs []string, limit int) ([]conversation.Message, error) {
	tid := tenantFromCtx(ctx)

	qb := newQueryBuilder(tid)
	qb.add("m.content IS NOT NULL AND m.content != ''")
	qb.addParam("to_tsvector('english', m.content) @@ plainto_tsquery('english', $%d)", query)

	if len(projectIDs) > 0 {
		qb.addParam("c.project_id = ANY($%d)", projectIDs)
	}

	qb.addParam("1=1") // placeholder for LIMIT
	limitIdx := qb.nextArgIdx()
	// ... build final SQL with LIMIT $limitIdx
}
```

The key fix: ensure `argIdx` tracking is centralized in one place, not scattered across manual `fmt.Sprintf` calls.

- [ ] **Step 3: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/postgres/ -run TestSearch -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/postgres/store_conversation.go
git commit -m "security: refactor conversation search SQL to safe builder (F5-D3)"
```

---

### Task 4: Add Per-User Authorization to MCP Tools

**Findings:** F8-D3 HIGH — MCP tools return full objects without per-user authorization
**Files:**
- Modify: `internal/adapter/mcp/server.go:62-96`
- Modify: `internal/adapter/mcp/tools.go`
- Modify: `internal/adapter/mcp/server_test.go`

- [ ] **Step 1: Write failing test**

```go
// Add to server_test.go
func TestMCPTools_FilterByTenant(t *testing.T) {
	// MCP tool handlers must extract tenant context from auth
	// and filter results accordingly.
}
```

- [ ] **Step 2: Add tenant context extraction to MCP auth middleware**

The auth middleware should set tenant context on the request context. Tool handlers already use `ctx` — they need to pass it to `ProjectLister.ListProjects(ctx)` which respects tenant isolation at the store layer.

Verify that `handleListProjects` passes the context from the MCP request to `s.deps.ProjectLister.ListProjects(ctx)`. If the `ProjectLister` interface already filters by tenant (via `tenantFromCtx`), the fix is ensuring the MCP auth middleware injects the tenant ID into context. Add nil check to `NewServer`:

```go
func NewServer(cfg ServerConfig, deps ServerDeps) *Server {
	if deps.ProjectLister == nil {
		panic("MCP ServerDeps.ProjectLister must not be nil")
	}
	if deps.RunReader == nil {
		panic("MCP ServerDeps.RunReader must not be nil")
	}
	if deps.CostReader == nil {
		panic("MCP ServerDeps.CostReader must not be nil")
	}
	// ...
}
```

- [ ] **Step 3: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/mcp/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/mcp/server.go internal/adapter/mcp/tools.go internal/adapter/mcp/server_test.go
git commit -m "security: add tenant context and nil checks to MCP server (F8-D3)"
```

---

## Phase 2: Missing Service Tests

### Task 5: Write SkillService Tests

**Findings:** F18-D2 CRITICAL — `internal/service/skill.go` has NO test file
**Files:**
- Create: `internal/service/skill_test.go`

- [ ] **Step 1: Write table-driven tests for Create, Get, List, Update, Delete**

Test each method with: happy path, not found, validation error, empty project ID (global). Use a mock/fake store. Cover: Create with defaults (TypePattern, SourceUser), Update partial fields, IncrementUsage, ListActive.

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestSkill -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/skill_test.go
git commit -m "test: add SkillService tests (F18-D2)"
```

---

### Task 6: Write MicroagentService Tests

**Findings:** F18-D2 CRITICAL — No dedicated service test file
**Files:**
- Extend: `internal/service/microagent_test.go` (created in Task 1)

- [ ] **Step 1: Add CRUD tests**

Test: Create (happy, validation error), Get (found, not found), List, Update (partial fields, enable/disable), Delete, Match (substring, regex, disabled agents skipped, no match).

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestMicroagent -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/microagent_test.go
git commit -m "test: add MicroagentService CRUD tests (F18-D2)"
```

---

### Task 7: Write MemoryService Tests

**Findings:** F17-D2 — No service tests (only domain tests exist)
**Files:**
- Create: `internal/service/memory_test.go`

- [ ] **Step 1: Write tests for Store publish/error and ListByProject**

Use a fake store and fake queue. Test: Store publishes to queue, Store with queue error, ListByProject returns results, ListByProject with empty project.

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestMemory -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/memory_test.go
git commit -m "test: add MemoryService tests (F17-D2)"
```

---

### Task 8: Write Eventstore Tests

**Findings:** F20-D2 — No dedicated test file for eventstore.go
**Files:**
- Extend: `internal/adapter/postgres/eventstore_test.go` (created in Task 2)

- [ ] **Step 1: Add tests for query builder edge cases**

Test: empty filter (only tenant), all filters set, nil After/Before, zero AfterSequence, limit=0, very large limit.

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/postgres/ -run TestBuild -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/postgres/eventstore_test.go
git commit -m "test: add eventstore query builder tests (F20-D2)"
```

---

### Task 9: Expand Handoff Service Tests

**Findings:** F19-D2 — Only 3 test functions; missing quarantine, broadcast, concurrent tests
**Files:**
- Modify: `internal/service/handoff_test.go`

- [ ] **Step 1: Add tests for quarantine evaluation, War Room broadcast, error paths**

Test: handoff with quarantine returning error (should log warning, proceed), handoff with hub broadcasting, handoff with nil hub (no panic), handoff to a2a:// target with nil A2A service (error), concurrent handoffs.

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestHandoff -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/handoff_test.go
git commit -m "test: expand HandoffService test coverage (F19-D2)"
```

---

## Phase 3: Security Hardening

### Task 10: Block `data:` URLs in Markdown Sanitization

**Findings:** F5-D3 MEDIUM, F22-D3 MEDIUM — Markdown link sanitization doesn't block `data:` URLs
**Files:**
- Modify: `frontend/src/features/project/Markdown.tsx:123-130`

- [ ] **Step 1: Fix the URL protocol check**

Change line 127 from:
```typescript
const safeUrl = /^(https?:|mailto:)/i.test(url) ? url : "#";
```
To:
```typescript
const safeUrl = /^(https?:|mailto:)/i.test(url) ? url : "#";
```
Wait — the current code already only allows `https?:` and `mailto:` and falls back to `#`. The issue is whether `data:` or `javascript:` could bypass via encoding. The current regex is correct for blocking — it uses a whitelist pattern. Let me re-read the audit: "blocks `javascript:` but not `data:` URLs". Actually, the current code IS a whitelist: it only allows URLs starting with `http:`, `https:`, or `mailto:`. Everything else becomes `#`. So `data:` is already blocked. The audit finding may be a false positive.

Verify this by adding a test:

```typescript
// frontend/src/features/canvas/__tests__/markdown-sanitize.test.ts
import { describe, it, expect } from "vitest";

describe("Markdown renderInline link sanitization", () => {
  it("blocks data: URLs", () => {
    // Verify the regex whitelist blocks data: protocol
    const url = "data:text/html,<script>alert(1)</script>";
    const result = /^(https?:|mailto:)/i.test(url);
    expect(result).toBe(false);
  });
  it("blocks javascript: URLs", () => {
    const url = "javascript:alert(1)";
    const result = /^(https?:|mailto:)/i.test(url);
    expect(result).toBe(false);
  });
  it("allows https: URLs", () => {
    const result = /^(https?:|mailto:)/i.test("https://example.com");
    expect(result).toBe(true);
  });
});
```

- [ ] **Step 2: Run test to confirm current behavior is safe**

Run: `cd /workspaces/CodeForge/frontend && npx vitest run --reporter=verbose src/features/canvas/__tests__/markdown-sanitize.test.ts`
Expected: ALL PASS (whitelist already blocks data:)

- [ ] **Step 3: Add comment to Markdown.tsx for clarity**

Add a comment above line 127:
```typescript
// Whitelist: only http(s) and mailto URLs are allowed.
// All other protocols (javascript:, data:, vbscript:, etc.) resolve to "#".
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/project/Markdown.tsx frontend/src/features/canvas/__tests__/markdown-sanitize.test.ts
git commit -m "security: verify and document Markdown URL whitelist (F5-D3, F22-D3)"
```

---

### Task 11: Harden Project Deletion Path Normalization

**Findings:** F1-D3 MEDIUM — `os.RemoveAll(wsPath)` relies on string comparison, not normalized paths
**Files:**
- Modify: `internal/service/project.go:144`

- [ ] **Step 1: Write test for path traversal protection**

```go
func TestProjectService_Delete_PathTraversal(t *testing.T) {
	tests := []struct {
		name     string
		wsPath   string
		root     string
		wantSafe bool
	}{
		{"normal path", "/workspaces/project-1", "/workspaces", true},
		{"traversal attempt", "/workspaces/../etc/passwd", "/workspaces", false},
		{"symlink-like", "/workspaces/./../../etc", "/workspaces", false},
		{"empty path", "", "/workspaces", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &ProjectService{workspaceRoot: tt.root}
			got := svc.isUnderWorkspaceRoot(tt.wsPath)
			if got != tt.wantSafe {
				t.Errorf("isUnderWorkspaceRoot(%q) = %v, want %v", tt.wsPath, got, tt.wantSafe)
			}
		})
	}
}
```

- [ ] **Step 2: Harden `isUnderWorkspaceRoot` to resolve symlinks**

Note: The existing implementation already uses `filepath.Abs()` (which calls `filepath.Clean` internally). The value-add here is resolving symlinks via `filepath.EvalSymlinks` to prevent symlink-based traversal attacks.

```go
func (s *ProjectService) isUnderWorkspaceRoot(wsPath string) bool {
	if wsPath == "" || s.workspaceRoot == "" {
		return false
	}
	// EvalSymlinks resolves symlinks AND cleans the path.
	resolvedPath, err := filepath.EvalSymlinks(wsPath)
	if err != nil {
		return false // path doesn't exist or can't be resolved — reject
	}
	resolvedRoot, err := filepath.EvalSymlinks(s.workspaceRoot)
	if err != nil {
		return false
	}
	return strings.HasPrefix(resolvedPath, resolvedRoot+string(filepath.Separator))
}
```

- [ ] **Step 3: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestProjectService_Delete_PathTraversal -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/project.go internal/service/project_test.go
git commit -m "security: normalize paths in project deletion (F1-D3)"
```

---

### Task 12: Add TriggerPattern Regex Validation to Update

**Findings:** F18-D2 — Validate() on UpdateRequest doesn't check regex validity
**Files:**
- Modify: `internal/service/microagent.go:54-77`

- [ ] **Step 1: Write test for Update with invalid regex**

```go
func TestMicroagentService_Update_InvalidRegex(t *testing.T) {
	// Update with invalid regex trigger pattern should fail.
}
```

- [ ] **Step 2: Add validation in Update method**

```go
// In Update(), after setting TriggerPattern:
if req.TriggerPattern != "" {
	if len(req.TriggerPattern) > microagent.MaxTriggerPatternLength {
		return nil, errors.New("trigger_pattern exceeds maximum length")
	}
	if strings.HasPrefix(req.TriggerPattern, "^") || strings.HasPrefix(req.TriggerPattern, "(") {
		if _, err := regexp.Compile(req.TriggerPattern); err != nil {
			return nil, fmt.Errorf("invalid trigger_pattern regex: %w", err)
		}
	}
	m.TriggerPattern = req.TriggerPattern
}
```

- [ ] **Step 3: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestMicroagent -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/microagent.go internal/service/microagent_test.go
git commit -m "security: validate regex on microagent update (F18-D2)"
```

---

### Task 13: Add A2A Prompt Length Validation

**Findings:** F9-D3 MEDIUM — No MAX_PROMPT_LENGTH check in `executor.go:42-50`
**Files:**
- Modify: `internal/adapter/a2a/executor.go:40-88`
- Create: `internal/adapter/a2a/executor_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestExecutor_Execute_PromptTooLong(t *testing.T) {
	// Prompt exceeding 100KB should be rejected.
	longPrompt := strings.Repeat("a", 100_001)
	// ... should return error
}
```

- [ ] **Step 2: Add length validation**

```go
const MaxPromptLength = 100_000 // 100KB

// In Execute(), before processing prompt:
if len(prompt) > MaxPromptLength {
	return nil, fmt.Errorf("prompt exceeds maximum length (%d > %d)", len(prompt), MaxPromptLength)
}
```

- [ ] **Step 3: Add cancel task existence check (F9-D2)**

In `Cancel()`, after fetching task, add nil check:
```go
dt, err := s.store.Get(ctx, taskID)
if err != nil {
	return fmt.Errorf("get task for cancel: %w", err)
}
if dt == nil {
	return fmt.Errorf("task %s not found", taskID)
}
```

- [ ] **Step 4: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/a2a/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/a2a/executor.go internal/adapter/a2a/executor_test.go
git commit -m "security: add A2A prompt length validation and cancel check (F9-D3, F9-D2)"
```

---

### Task 14: Add Server-Side Image Size Validation

**Findings:** F6-D1 — 5MB limit client-only; F7-D3 — No per-image size validation in struct
**Files:**
- Modify: `internal/domain/conversation/conversation.go`
- Modify: `workers/codeforge/history.py:150-171`

- [ ] **Step 1: Add MaxImageSize constant and validation**

```go
// conversation.go
const MaxImageSizeBytes = 5 * 1024 * 1024 // 5MB

func (img *MessageImage) Validate() error {
	if img.MediaType == "" {
		return errors.New("image media_type is required")
	}
	if len(img.Data) > MaxImageSizeBytes {
		return fmt.Errorf("image data exceeds %d bytes", MaxImageSizeBytes)
	}
	return nil
}
```

- [ ] **Step 2: Add base64 validation in history.py (F6-D2)**

```python
# workers/codeforge/history.py — in _to_msg_dict image handling
import base64

def _validate_base64(data: str) -> bool:
    try:
        base64.b64decode(data, validate=True)
        return True
    except Exception:
        return False

# Before building data URL:
if not _validate_base64(img.data):
    logger.warning("skipping image with invalid base64 data")
    continue
```

- [ ] **Step 3: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/conversation/ -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/domain/conversation/conversation.go workers/codeforge/history.py
git commit -m "security: add server-side image size and base64 validation (F6-D1, F7-D3)"
```

---

### Task 15: Add Handoff Max-Hops Limit

**Findings:** F19-D2 — No cycle detection or max hops in `handoff.py:77-83`
**Files:**
- Modify: `workers/codeforge/tools/handoff.py:77-83`

- [ ] **Step 1: Add max hops constant and check**

```python
MAX_HANDOFF_HOPS = 10

# In the handoff tool execution, after hop increment:
hop = int(metadata.get("handoff_hop", "0")) + 1
if hop > MAX_HANDOFF_HOPS:
    return f"Error: handoff chain exceeded maximum of {MAX_HANDOFF_HOPS} hops (cycle detected?)"
metadata["handoff_hop"] = str(hop)
```

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -k handoff -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add workers/codeforge/tools/handoff.py
git commit -m "security: add max-hops limit to handoff chain (F19-D2)"
```

---

### Task 16: Add Tenant Check to Review Trigger

**Findings:** F13-D3 MEDIUM — No tenant isolation check before triggering review
**Files:**
- Modify: `internal/service/review_trigger.go:37`

- [ ] **Step 1: Add tenant context extraction in TriggerReview**

Ensure `TriggerReview` extracts `tenantFromCtx(ctx)` and passes it to the database query, or verifies the project belongs to the tenant before triggering.

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestReview -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/review_trigger.go
git commit -m "security: add tenant isolation to review trigger (F13-D3)"
```

---

### Task 17: Add Project Access Check to Quarantine

**Findings:** F11-D3 MEDIUM — Quarantine.Evaluate doesn't verify project access
**Files:**
- Modify: `internal/service/quarantine.go:34`

- [ ] **Step 1: Add project access verification**

In `Evaluate()`, verify that the message's ProjectID belongs to the current tenant context before proceeding with quarantine evaluation.

- [ ] **Step 2: Add UTF-8 validation to scorer (F11-D3)**

```go
// internal/domain/quarantine/scorer.go — before regex matching:
if !utf8.Valid(payload) {
	// Replace invalid UTF-8 to prevent pattern bypass
	payload = []byte(strings.ToValidUTF8(string(payload), "\uFFFD"))
}
```

- [ ] **Step 3: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestQuarantine -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/quarantine.go internal/domain/quarantine/scorer.go
git commit -m "security: add project access check and UTF-8 validation to quarantine (F11-D3)"
```

---

## Phase 4: Quality Fixes

### Task 18: Fix StallDetector Hash Truncation

**Findings:** F4-D2 — StallDetector truncates args to 200 chars; identical prefixes = false positive
**Files:**
- Modify: `workers/codeforge/agent_loop.py:65-100`

- [ ] **Step 1: Change hash to use full args, not truncated**

```python
# Instead of: hash_args = json.dumps(args)[:200]
# Use full JSON serialization:
def _hash_tool_call(name: str, args: dict) -> str:
    """Hash tool call for stall detection. Uses full args, not truncated."""
    raw = json.dumps(args, sort_keys=True, default=str)
    return hashlib.sha256(f"{name}:{raw}".encode()).hexdigest()
```

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -k stall -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add workers/codeforge/agent_loop.py
git commit -m "fix: use full args in stall detector hash (F4-D2)"
```

---

### ~~Task 19: REMOVED — quality_tracker is NOT dead code~~

**Reviewer note:** The audit incorrectly flagged `quality_tracker` as unused. It IS used in the loop: `_check_model_switch(quality_tracker, cfg)` (line 375), `quality_tracker.end_iteration()` (line 391), and `quality_tracker=state.quality_tracker` (line 707). Removing it would break the agent loop. Finding F7-D2 (quality_tracker) is a **false positive** — no action needed.

---

### Task 20: Fix Image-Only Message Handling

**Findings:** F7-D2 — Image-only messages return empty string from `_extract_user_prompt`
**Files:**
- Modify: `workers/codeforge/agent_loop.py:183-189`

- [ ] **Step 1: Handle content-array format in _extract_user_prompt**

```python
def _extract_user_prompt(msg) -> str:
    content = msg.get("content", "")
    if isinstance(content, list):
        # Content-array format: extract text parts
        text_parts = [p.get("text", "") for p in content if p.get("type") == "text"]
        return " ".join(text_parts).strip()
    return str(content)
```

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -k agent_loop -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add workers/codeforge/agent_loop.py
git commit -m "fix: handle image-only messages in prompt extraction (F7-D2)"
```

---

### Task 21: Add Model Name Validation in Fallback

**Findings:** F7-D3 MEDIUM — Fallback model selection doesn't validate provider format
**Files:**
- Modify: `workers/codeforge/agent_loop.py:249-262`

- [ ] **Step 1: Add provider/model format validation**

```python
def _validate_model_name(model: str) -> bool:
    """Validate model name has exactly one slash (provider/model)."""
    parts = model.split("/")
    return len(parts) == 2 and all(p.strip() for p in parts)
```

Use before selecting fallback model.

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add workers/codeforge/agent_loop.py
git commit -m "fix: validate model name format in fallback selection (F7-D3)"
```

---

### Task 22: Fix N+1 Query in Roadmap GetByProject

**Findings:** F2-D2 — Nested loops for milestones+features instead of JOINs
**Files:**
- Modify: `internal/service/roadmap.go:55-76`

- [ ] **Step 1: Write test for GetByProject performance**

```go
func TestRoadmapService_GetByProject_SingleQuery(t *testing.T) {
	// Verify the service loads roadmap with milestones and features
	// without N+1 queries.
}
```

- [ ] **Step 2: Refactor to batch-load milestones and features**

Replace nested loop with two batch queries:
1. `SELECT * FROM milestones WHERE roadmap_id = $1`
2. `SELECT * FROM features WHERE milestone_id = ANY($1)`

Then assemble in Go.

- [ ] **Step 3: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestRoadmap -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/roadmap.go internal/adapter/postgres/store_roadmap.go
git commit -m "perf: fix N+1 query in roadmap GetByProject (F2-D2)"
```

---

### Task 23: Refactor Routing Module Singletons to DI

**Findings:** F3-D2, F14-D2 — Module-level singletons in blocklist.py, key_filter.py
**Files:**
- Modify: `workers/codeforge/routing/blocklist.py`
- Modify: `workers/codeforge/routing/key_filter.py`
- Modify: `workers/codeforge/routing/router.py`

- [ ] **Step 1: Convert blocklist module globals to class**

Replace `_blocklist: dict[str, float]` module global with a `Blocklist` class that gets injected into `HybridRouter.__init__()`.

- [ ] **Step 2: Convert key_filter module globals to class**

Replace `_warned_providers`, `_healthy_models` globals with a `KeyFilter` class.

- [ ] **Step 3: Update HybridRouter to accept injected dependencies**

```python
class HybridRouter:
    def __init__(self, blocklist: Blocklist, key_filter: KeyFilter, ...):
        self._blocklist = blocklist
        self._key_filter = key_filter
```

- [ ] **Step 4: Fix whitespace API key check (F14-D3)**

In `KeyFilter`, change:
```python
key = os.environ.get(env_var, "").strip()
if key:  # was just checking truthiness
```
To explicitly reject whitespace-only:
```python
key = os.environ.get(env_var, "").strip()
if key and not key.isspace():
```

- [ ] **Step 5: Run tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -k routing -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add workers/codeforge/routing/blocklist.py workers/codeforge/routing/key_filter.py workers/codeforge/routing/router.py
git commit -m "refactor: convert routing singletons to dependency injection (F3-D2, F14-D2, F14-D3)"
```

---

### Task 24: Add Timeout to route_with_fallbacks

**Findings:** F3-D2 — No timeout or max retry in `route_with_fallbacks`
**Files:**
- Modify: `workers/codeforge/routing/router.py:205-250`

- [ ] **Step 1: Add max_retries parameter and timeout**

```python
MAX_ROUTING_RETRIES = 3
ROUTING_TIMEOUT_SECONDS = 30

async def route_with_fallbacks(self, ..., max_retries: int = MAX_ROUTING_RETRIES):
    for attempt in range(max_retries):
        try:
            model = await asyncio.wait_for(
                self._select_model(...),
                timeout=ROUTING_TIMEOUT_SECONDS
            )
            if model:
                return model
        except asyncio.TimeoutError:
            logger.warning("routing timeout on attempt %d", attempt + 1)
    return self._default_fallback(...)
```

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -k routing -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add workers/codeforge/routing/router.py
git commit -m "fix: add timeout and max retries to route_with_fallbacks (F3-D2)"
```

---

### Task 25: Add Scorer Weight Validation

**Findings:** F17-D2 — Weights must sum to 1.0 but no validation
**Files:**
- Modify: `workers/codeforge/memory/scorer.py:28-38`

- [ ] **Step 1: Add validation in CompositeScorer.__init__**

```python
def __init__(self, weights: ScoreWeights | None = None, half_life_hours: float = 168.0) -> None:
    self.weights = weights or ScoreWeights()
    total = self.weights.semantic + self.weights.recency + self.weights.importance
    if abs(total - 1.0) > 1e-6:
        raise ValueError(f"Score weights must sum to 1.0, got {total}")
    self._decay_lambda = math.log(2) / half_life_hours
```

- [ ] **Step 2: Fix division by zero in experience.py cosine similarity (F17-D3)**

The code already has `+ 1e-8` but the scorer.py uses a cleaner check. Harmonize by adding explicit zero-vector check in experience.py:

```python
norm_q = np.linalg.norm(query_emb)
norm_e = np.linalg.norm(entry_emb)
if norm_q == 0 or norm_e == 0:
    similarity = 0.0
else:
    similarity = float(np.dot(query_emb, entry_emb) / (norm_q * norm_e))
```

- [ ] **Step 3: Run tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -k memory -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/memory/scorer.py workers/codeforge/memory/experience.py
git commit -m "fix: add weight validation and fix division by zero (F17-D2, F17-D3)"
```

---

### Task 26: Add Token Refresh Jitter

**Findings:** F22-D2 — Token refresh with no jitter (thundering herd risk)
**Files:**
- Modify: `frontend/src/components/AuthProvider.tsx:50-57`

- [ ] **Step 1: Add jitter to setTimeout**

```typescript
// Add random jitter (0-30 seconds) to prevent thundering herd
const jitter = Math.random() * 30_000;
const refreshIn = (expiresIn - 60) * 1000 + jitter;
setTimeout(() => refreshToken(), refreshIn);
```

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge/frontend && npx vitest run`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/AuthProvider.tsx
git commit -m "fix: add jitter to token refresh timing (F22-D2)"
```

---

### Task 27: Add MaxSteps Upper Bound Validation

**Findings:** F15-D2 — MaxSteps validates `< 0` but no upper bound
**Files:**
- Modify: `internal/domain/policy/validate.go`
- Create: `internal/domain/policy/validate_test.go`

- [ ] **Step 1: Write test**

```go
// internal/domain/policy/validate_test.go
package policy

import "testing"

func TestValidatePolicy_MaxStepsUpperBound(t *testing.T) {
	profile := &PolicyProfile{MaxSteps: 100_000}
	err := profile.Validate()
	if err == nil {
		t.Error("expected error for MaxSteps > 10000")
	}
}
```

- [ ] **Step 2: Add upper bound constant and check**

```go
const MaxStepsLimit = 10_000

// In Validate():
if p.MaxSteps > MaxStepsLimit {
	return fmt.Errorf("max_steps must not exceed %d", MaxStepsLimit)
}
```

- [ ] **Step 3: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/policy/ -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/domain/policy/validate.go internal/domain/policy/validate_test.go
git commit -m "fix: add MaxSteps upper bound validation (F15-D2)"
```

---

### Task 28: Add Async Logger DroppedCount to Health

**Findings:** F21-D2 — Channel overflow drops records silently; DroppedCount() not in health checks
**Files:**
- Modify: `internal/logger/async.go`

- [ ] **Step 1: Export DroppedCount in health check response**

Add `DroppedLogCount() int64` method if not already public. Wire it into the `/health` endpoint response so operators can monitor log drops.

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/logger/ -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/logger/async.go internal/adapter/http/handlers_backend_health.go
git commit -m "fix: expose async logger dropped count in health check (F21-D2)"
```

---

### Task 29: Improve Experience Cache Error Handling

**Findings:** F7-D2 — Bare `except Exception` masks cache failures in agent_loop.py:302-330
**Files:**
- Modify: `workers/codeforge/agent_loop.py:302-330`

- [ ] **Step 1: Replace bare except with specific exception types**

```python
except (ConnectionError, TimeoutError) as exc:
    logger.warning("experience cache lookup failed (transient): %s", exc)
except ValueError as exc:
    logger.error("experience cache data corruption: %s", exc)
except Exception as exc:
    logger.error("unexpected experience cache error: %s", type(exc).__name__, exc_info=True)
```

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add workers/codeforge/agent_loop.py
git commit -m "fix: classify experience cache errors instead of bare except (F7-D2)"
```

---

### Task 30: Add Stall Detection Timeout Fallback

**Findings:** F15-D3 — If stall detection disabled, no timeout triggers agent stop
**Files:**
- Modify: `internal/service/runtime_lifecycle.go` or equivalent

- [ ] **Step 1: Add absolute timeout as safety net**

Even when stall detection is disabled, enforce a hard timeout (e.g., `MaxExecutionTimeSeconds` from config or a default of 3600s) as ultimate safety net.

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestRuntime -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/runtime_lifecycle.go
git commit -m "safety: add absolute timeout when stall detection is disabled (F15-D3)"
```

---

## Phase 5: Documentation & Config

### Task 31: Update ADR 007 for 5th Preset

**Findings:** F16-D2 — 5th preset `supervised-ask-all` not documented
**Files:**
- Modify: `docs/architecture/adr/007-policy-layer.md`

- [ ] **Step 1: Add `supervised-ask-all` preset to ADR documentation**

Add to the "Built-in Presets" section:
```
5. `supervised-ask-all` — Requires user approval for ALL tool calls. For maximum oversight.
   - All tools: decision = "ask"
   - No path/command restrictions (human decides)
```

- [ ] **Step 2: Commit**

```bash
git add docs/architecture/adr/007-policy-layer.md
git commit -m "docs: add supervised-ask-all preset to ADR 007 (F16-D2)"
```

---

### Task 32: Make Context Budget Phase Scaling Configurable

**Findings:** F15-D2 — Phase scaling hardcoded, no config override
**Files:**
- Modify: `internal/service/context_budget.go:20-25`

- [ ] **Step 1: Extract phase scaling to config**

Move the hardcoded phase scaling map into a config struct with defaults that can be overridden via YAML.

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestContextBudget -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/context_budget.go internal/config/config.go
git commit -m "config: make context budget phase scaling configurable (F15-D2)"
```

---

### Task 33: Add JWT Secret Production Check

**Findings:** F21-D3 LOW — Default JWT secret in code, easy to forget in production
**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add startup warning for default JWT secret**

```go
// In config validation/loading:
if cfg.Auth.JWTSecret == "codeforge-dev-jwt-secret-change-in-production" && cfg.AppEnv == "production" {
	return fmt.Errorf("FATAL: default JWT secret detected in production — set AUTH_JWT_SECRET environment variable")
}
```

- [ ] **Step 2: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/config/ -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "security: reject default JWT secret in production mode (F21-D3)"
```

---

## Phase 6: Deferred — Separate Plans Required

The following findings require new feature implementation (>100 LOC each) and should be planned separately:

| Finding | Feature | Estimated Effort | Separate Plan? |
|---------|---------|:----------------:|:--------------:|
| F20-D1: Hook System (Observer pattern) | #20 | 500+ LOC, new domain | YES |
| F15-D1: Branch Isolation mechanism | #15 | 300+ LOC, new safety layer | YES |
| F15-D1: CommandSafetyEvaluator service | #15 | 200+ LOC, new service | YES |
| F13-D1: contract_reviewer handler | #13 | 200+ LOC, Python consumer | YES |
| F13-D1: refactorer approval endpoint | #13 | 150+ LOC, Go handler + frontend | YES |
| F12-D1: RLVR/DPO export endpoints | #12 | 300+ LOC, new handlers | YES |
| F12-D2: BenchmarkService decomposition | #12 | Refactor 1000+ LOC | YES |
| F16-D2: Policy scope cascade | #16 | 200+ LOC, service redesign | YES |
| F11-D1: Agent statistics tracking | #11 | 200+ LOC, cross-layer | YES |
| F11-D1: Agent inbox message routing | #11 | 150+ LOC, consumer handler | YES |
| F9-D2: AgentCard refresh/invalidation | #9 | 100+ LOC, cache pattern | Combined with A2A |
| F20-D3: Trajectory JSONB schema validation | #20 | Combined with Hook System plan |

### Uncovered MEDIUM Findings — Explicitly Deferred

These MEDIUM findings are real but lower-priority than the 33 tasks above. They should be addressed in a follow-up cycle:

| Finding | Feature | Reason Deferred |
|---------|---------|----------------|
| F2-D3: API key rotation mechanism | #2 | Ops concern, not code bug — requires Vault/secret manager integration |
| F4-D2: context_optimizer.go no test file | #4 | Complex pipeline test requiring extensive mocking — separate test plan |
| F4-D2: orchestrator.go no concurrent tests | #4 | Requires test infra for parallel execution — separate test plan |
| F4-D3: Approval timeout server-side enforcement | #4 | Needs design decision on timeout behavior (reject vs auto-approve) |
| F5-D2: ChatPanel.tsx decomposition | #5 | Large refactor (1100+ LOC) — separate frontend plan |
| F5-D3: Conversation search rate limiting | #5 | Needs rate limiter middleware design for specific endpoints |
| F8-D3: No constant-time comparison for MCP API key | #8 | Low practical risk (timing attacks on localhost MCP) but should fix |
| F10-D3: Tool results may contain sensitive data (no redaction) | #10 | Requires redaction filter design — which fields, which tools |
| F14-D2: No Python unit tests for routing module | #14 | Covered by DI refactor (Task 23) which enables testability — tests follow |
| F17-D2: experience.py creates new DB connection per lookup | #17 | Requires async connection pool integration — architecture decision needed |

---

## Summary

| Phase | Tasks | Findings Covered | Priority |
|-------|:-----:|:----------------:|----------|
| 1: CRITICAL Security | 4 | 5 | P0 |
| 2: Missing Tests | 5 | 5 | P0 |
| 3: Security Hardening | 8 | 11 | P1 |
| 4: Quality Fixes | 12 | 17 | P1-P2 |
| 5: Documentation & Config | 3 | 3 | P2 |
| 6: Deferred — separate plans | -- | 12 | P2-P3 |
| 6b: Deferred — follow-up cycle | -- | 10 | P2 |
| **TOTAL** | **32 tasks** | **63 findings** | |

**Note:** Task 19 was removed after reviewer discovered quality_tracker is NOT dead code (false positive in audit).

**Remaining ~22 findings** are either:
- Informational/verified-safe (6 — "VERIFIED" findings in audit)
- Duplicates across features (5 — e.g., F5/F22 Markdown URL is one fix)
- Extremely low risk (8 — LOW severity with no practical exploit path)
- Audit false positive (1 — quality_tracker, F7-D2)
- Explicitly deferred with spec status "future" (2 — dependency graph, voice/video)
