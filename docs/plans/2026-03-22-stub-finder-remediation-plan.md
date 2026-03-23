# Stub Finder Remediation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address all actionable findings from the 2026-03-22 stub finder audit across 5 worktrees, grouped by codebase layer and scope.

**Architecture:** Three quick-win worktrees (A/B/C) target small, safe fixes in Go, Frontend, and Python respectively. Two larger worktrees (D/E) require breaking API changes or new subsystems and are documented as future work with scope definitions.

**Tech Stack:** Go 1.25, SolidJS/TypeScript, Python 3.12 (structlog), PostgreSQL, NATS JetStream

**Source:** `docs/audits/2026-03-22-stub-finder-report.md`

---

## Worktree Overview

| Worktree | Branch | Scope | Effort |
|----------|--------|-------|--------|
| **A** | `fix/go-core-hardening` | LSP wiring, secure cookies, NATS cleanup | ~1h |
| **B** | `fix/frontend-ux-polish` | BenchmarkPage errors, useCRUDForm, SVG icons | ~2h |
| **C** | `fix/python-worker-standardization` | structlog migration (43 files), FakeLLM consolidation | ~3h |
| **D** | `refactor/api-v2-handlers` | REST v2 (FIX-061/063/098/100) + Handlers decomposition | ~5d (separate plan) |
| **E** | `feat/extensibility-layer` | Hook System + Ed25519 signatures | ~5d (separate plan) |

---

# Worktree A: `fix/go-core-hardening`

**Files overview:**
- Modify: `internal/service/project.go` (LSP injection + auto-start + cleanup)
- Modify: `internal/config/config.go` (add `ForceSecureCookies` field)
- Modify: `internal/adapter/http/handlers_auth.go` (use new config flag)
- Modify: `internal/adapter/nats/nats.go` (clean up resolved TODO comments)
- Test: `internal/service/project_test.go`
- Test: `internal/adapter/http/handlers_auth_test.go`

---

### Task A1: Wire LSP into ProjectService

**Files:**
- Modify: `internal/service/project.go:32-57` (struct + setter + SetupProject + Delete)
- Test: `internal/service/project_test.go`

**Context:** LSP adapter is fully implemented (`internal/adapter/lsp/`, `internal/service/lsp.go`) with 8 HTTP routes registered and config ready (`config.LSP`). Only missing: ProjectService doesn't know about LSPService, so auto-start after stack detection and cleanup on delete are missing.

- [ ] **Step 1: Add LSPService field and setter to ProjectService**

Add to `internal/service/project.go` after line 37 (after `goalDiscovery` field):

```go
// In the struct (line 32-37):
type ProjectService struct {
	store         database.Store
	workspaceRoot string
	specDetector  SpecDetector
	goalDiscovery *GoalDiscoveryService
	lsp           *LSPService  // <-- add this
}
```

Add setter after `SetGoalDiscovery` (after line 57):

```go
// SetLSP sets the optional LSP service for auto-starting language servers.
func (s *ProjectService) SetLSP(svc *LSPService) {
	s.lsp = svc
}
```

- [ ] **Step 2: Add LSP auto-start to SetupProject after stack detection**

In `SetupProject()`, after persisting detected languages (after line 521, inside the `if len(stack.Languages) > 0` block), add:

```go
			// Auto-start LSP servers for detected languages.
			if s.lsp != nil {
				go s.lsp.StartServers(ctx, id, p.WorkspacePath, stack.Languages)
			}
```

Place this right after the `UpdateProject` call for detected languages (line 519) and before the closing braces.

- [ ] **Step 3: Add LSP cleanup to Delete**

In `Delete()` (line 132-155), add LSP server stop before workspace removal. After getting the project (line 133) and before deleting from store (line 140):

```go
	// Stop LSP servers for this project.
	if s.lsp != nil {
		s.lsp.StopServers(ctx, id)
	}
```

- [ ] **Step 4: Wire LSPService into ProjectService in main.go**

Search `cmd/codeforge/main.go` for where `projectSvc.SetGoalDiscovery(...)` is called. Add immediately after:

```go
	if lspSvc != nil {
		projectSvc.SetLSP(lspSvc)
	}
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./cmd/codeforge/`
Expected: Compiles without errors.

- [ ] **Step 6: Commit**

```bash
git add internal/service/project.go cmd/codeforge/main.go
git commit -m "feat: wire LSP auto-start into ProjectService (FIX-075)

StartServers called after stack detection in SetupProject.
StopServers called in Delete for cleanup."
```

---

### Task A2: Add ForceSecureCookies config flag

**Files:**
- Modify: `internal/config/config.go:68-80` (Auth struct)
- Modify: `internal/adapter/http/handlers_auth.go:17-32,50-58` (isSecureRequest + cookie setter)
- Modify: `internal/adapter/http/handlers.go` (Handlers struct — add AuthCfg field)
- Modify: `cmd/codeforge/main.go` (wire config)

**Context:** `isSecureRequest()` checks `r.TLS` or `X-Forwarded-Proto: https`. Behind non-standard proxies, neither may be set, causing the browser to silently discard the cookie. A config flag provides an escape hatch.

- [ ] **Step 1: Add ForceSecureCookies to Auth config**

Search `internal/config/config.go` for the `Auth` struct. Add field:

```go
type Auth struct {
	// ... existing fields ...
	ForceSecureCookies bool `yaml:"force_secure_cookies"` // Always set Secure=true on cookies (default: false)
}
```

No default change needed (false is zero value).

- [ ] **Step 2: Update isSecureRequest to accept config**

In `internal/adapter/http/handlers_auth.go`, change the function signature and add the flag check:

```go
// isSecureRequest returns true when the request arrived over TLS, behind
// a reverse proxy with X-Forwarded-Proto: https, or when force_secure_cookies
// is enabled in configuration.
func isSecureRequest(r *http.Request, forceSecure bool) bool {
	if forceSecure {
		return true
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
```

- [ ] **Step 3: Update all callers of isSecureRequest**

Search for `isSecureRequest(r)` in `handlers_auth.go` and update all call sites to pass the config flag:

```go
Secure: isSecureRequest(r, h.AuthCfg.ForceSecureCookies),
```

This requires the Handlers struct to have access to `AuthCfg`. Check if `h.Auth` already exposes config — if not, add `AuthCfg *config.Auth` field to Handlers struct and wire it in `main.go`.

- [ ] **Step 4: Remove FIX-093 TODO comment**

Remove lines 22-26 (the TODO comment about adding the config flag) since it's now implemented.

- [ ] **Step 5: Verify compilation**

Run: `go build ./cmd/codeforge/`
Expected: Compiles without errors.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/adapter/http/handlers_auth.go internal/adapter/http/handlers.go cmd/codeforge/main.go
git commit -m "feat: add force_secure_cookies config flag (FIX-093)

Allows operators to unconditionally set Secure=true on cookies
in hardened deployments behind non-standard proxies."
```

---

### Task A3: Clean up resolved NATS TODO comments

**Files:**
- Modify: `internal/adapter/nats/nats.go:271,316`

**Context:** FIX-049 and FIX-050 are already implemented. The comments describe what was done but still read as TODOs. Simplify them to descriptive comments.

- [ ] **Step 1: Update FIX-050 comment (line 271)**

Change:
```go
		// FIX-050: Use JetStream delivery metadata instead of custom Retry-Count
		// header (which was never incremented on NAK redelivery).
```
To:
```go
		// Prefer JetStream delivery metadata over custom Retry-Count header.
```

- [ ] **Step 2: Update FIX-049 comment (line 316)**

Change:
```go
		// FIX-049: Include message ID so operators can monitor DLQ accumulation.
```
To:
```go
		// Include message ID for DLQ monitoring.
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/nats/nats.go
git commit -m "chore: clean up resolved NATS FIX-049/050 TODO comments"
```

---

# Worktree B: `fix/frontend-ux-polish`

**Files overview:**
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx` (error feedback)
- Modify: `frontend/src/hooks/useCRUDForm.ts` (delete fallback warning)
- Create: `frontend/src/ui/icons/ActionIcons.tsx` (centralized SVG icons)
- Modify: `frontend/src/features/project/ChatPanel.tsx` (use centralized icons)
- Modify: `frontend/src/features/project/FilePanel.tsx` (use centralized icons)
- Modify: `frontend/src/features/project/ChatHeader.tsx` (use centralized icons)

---

### Task B1: Add error feedback to BenchmarkPage hydration

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx:359-363,404-406`

**Context:** Two empty `catch()` blocks silently swallow errors. Users watching a running benchmark see incomplete data without any warning. Add `console.warn` for developer visibility and a toast for user visibility.

- [ ] **Step 1: Fix hydration catch block (line 359-363)**

Replace:
```typescript
        .catch(() => {
          // FIX-104: Silently mark as hydrated on failure — the WS live feed
          // will continue to provide updates. Hydration is best-effort.
          updateRunState(run.id, (prev) => ({ ...prev, hydratedFromApi: true }));
        });
```
With:
```typescript
        .catch((err) => {
          console.warn("Benchmark hydration failed, relying on WS feed", run.id, err);
          updateRunState(run.id, (prev) => ({ ...prev, hydratedFromApi: true }));
        });
```

- [ ] **Step 2: Fix gap-fill catch block (line 404-406)**

Replace:
```typescript
          .catch(() => {
            // FIX-104: Gap-fill is best-effort — WS will catch up on next event.
          });
```
With:
```typescript
          .catch((err) => {
            console.warn("Benchmark WS gap-fill failed", run.id, err);
          });
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/benchmarks/BenchmarkPage.tsx
git commit -m "fix: log benchmark hydration/gap-fill errors instead of swallowing (FIX-104)"
```

---

### Task B2: Add warning to useCRUDForm delete fallback

**Files:**
- Modify: `frontend/src/hooks/useCRUDForm.ts:51`

**Context:** The no-op `async () => {}` fallback is never triggered in practice (all callers implement their own delete), but if someone uses `crud.del` without providing `onDelete`, they'd get silent success. A warning makes this discoverable.

- [ ] **Step 1: Replace silent no-op with warning no-op**

Replace line 51:
```typescript
  // eslint-disable-next-line @typescript-eslint/no-empty-function
  const del = useConfirmAction(onDelete ?? (async () => {}));
```
With:
```typescript
  const del = useConfirmAction(
    onDelete ??
      (async () => {
        console.warn("useCRUDForm: onDelete not provided — delete action is a no-op");
      }),
  );
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/hooks/useCRUDForm.ts
git commit -m "fix: warn when useCRUDForm delete fallback is triggered"
```

---

### Task B3: Centralize duplicated SVG icons

**Files:**
- Create: `frontend/src/ui/icons/ActionIcons.tsx`
- Modify: `frontend/src/features/project/ChatPanel.tsx` (replace inline SVGs)
- Modify: `frontend/src/features/project/FilePanel.tsx` (replace inline SVGs)
- Modify: `frontend/src/features/project/ChatHeader.tsx` (replace inline SVGs)

**Context:** 40 inline `<svg>` definitions across 18 feature files. 3-5 icons (Plus, Attachment, Pencil) are duplicated. Existing `ui/icons/` has `CodeForgeLogo.tsx` and `EmptyStateIcons.tsx` but no action icons. `ui/layout/NavIcons.tsx` has 14 nav icons but is not reused.

- [ ] **Step 1: Identify duplicated icons**

Read `ChatPanel.tsx`, `FilePanel.tsx`, and `ChatHeader.tsx`. Extract all inline `<svg>` elements. Identify which share the same `d=` path data (duplicates).

- [ ] **Step 2: Create `ActionIcons.tsx`**

Create `frontend/src/ui/icons/ActionIcons.tsx` with shared icons extracted from the feature files. Follow the pattern in `EmptyStateIcons.tsx` — each icon as a named export function component:

```typescript
import type { JSX } from "solid-js";

interface IconProps {
  class?: string;
  size?: number;
}

export function PlusIcon(props: IconProps): JSX.Element {
  const s = props.size ?? 20;
  return (
    <svg class={props.class} width={s} height={s} viewBox="0 0 20 20" fill="currentColor">
      {/* path extracted from ChatHeader.tsx / FilePanel.tsx */}
    </svg>
  );
}

// AttachmentIcon, PencilIcon, UploadIcon, ChevronIcon, etc.
```

- [ ] **Step 3: Replace inline SVGs in ChatPanel.tsx**

Import from `~/ui/icons/ActionIcons` and replace inline `<svg>` blocks with `<AttachmentIcon />`, `<PencilIcon />` etc.

- [ ] **Step 4: Replace inline SVGs in FilePanel.tsx and ChatHeader.tsx**

Same pattern — import centralized icons and replace inline SVGs.

- [ ] **Step 5: Export from ui/icons/index.ts**

Add `ActionIcons` exports to `frontend/src/ui/icons/index.ts` (create if not exists).

- [ ] **Step 6: Verify build**

Run: `cd frontend && npm run build`
Expected: Build succeeds, no type errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/ui/icons/ frontend/src/features/project/ChatPanel.tsx frontend/src/features/project/FilePanel.tsx frontend/src/features/project/ChatHeader.tsx
git commit -m "refactor: centralize duplicated SVG icons into ui/icons/ActionIcons (FIX-106)"
```

---

# Worktree C: `fix/python-worker-standardization`

**Files overview:**
- Modify: 43 Python files with `import logging` → `import structlog`
- Modify: `workers/tests/conftest.py` (update FakeLLM import path)
- Move: `workers/tests/fake_llm.py` → `workers/tests/helpers/fake_llm.py`
- Create: `workers/tests/helpers/__init__.py`

---

### Task C1: Migrate stdlib logging to structlog

**Files:**
- Modify: All 43 files listed below

**Context:** `LOG_MIGRATION.md` documents the plan. 43 files use stdlib `logging`; the rest of the codebase uses `structlog`. Mixed logging produces inconsistent output (plain text vs structured JSON).

**Migration pattern per file:**

```python
# BEFORE:
import logging
logger = logging.getLogger(__name__)
logger.info("message %s", value)
logger.error("failed: %s", err)

# AFTER:
import structlog
logger = structlog.get_logger(__name__)
logger.info("message", value=value)
logger.error("failed", error=str(err))
```

**Key differences:**
- `structlog.get_logger()` instead of `logging.getLogger()`
- Keyword arguments instead of `%s` format strings
- `error=str(err)` instead of `"failed: %s", err`

**Complete file list (43 files):**

```
workers/codeforge/agent_loop.py
workers/codeforge/history.py
workers/codeforge/logger.py
workers/codeforge/executor.py
workers/codeforge/config.py
workers/codeforge/llm.py
workers/codeforge/claude_code_executor.py
workers/codeforge/mcp_workbench.py
workers/codeforge/model_resolver.py
workers/codeforge/subprocess_utils.py
workers/codeforge/context_reranker.py
workers/codeforge/schemas/parser.py
workers/codeforge/skills/selector.py
workers/codeforge/skills/safety.py
workers/codeforge/routing/router.py
workers/codeforge/routing/meta_router.py
workers/codeforge/routing/key_filter.py
workers/codeforge/routing/capabilities.py
workers/codeforge/routing/blocklist.py
workers/codeforge/evaluation/prompt_mutator.py
workers/codeforge/evaluation/prompt_optimizer.py
workers/codeforge/backends/aider.py
workers/codeforge/backends/goose.py
workers/codeforge/backends/opencode.py
workers/codeforge/backends/openhands.py
workers/codeforge/backends/plandex.py
workers/codeforge/backends/sweagent.py
workers/codeforge/backends/router.py
workers/codeforge/backends/_streaming.py
workers/codeforge/tools/__init__.py
workers/codeforge/tools/_lint.py
workers/codeforge/tools/read_file.py
workers/codeforge/tools/edit_file.py
workers/codeforge/tools/write_file.py
workers/codeforge/tools/bash.py
workers/codeforge/tools/glob_files.py
workers/codeforge/tools/list_directory.py
workers/codeforge/tools/search_files.py
workers/codeforge/tools/search_skills.py
workers/codeforge/tools/search_conversations.py
workers/codeforge/tools/propose_goal.py
workers/codeforge/tools/create_skill.py
workers/codeforge/tools/capability.py
```

- [ ] **Step 1: Migrate routing/ files (5 files)**

Migrate `workers/codeforge/routing/router.py`, `meta_router.py`, `key_filter.py`, `capabilities.py`, `blocklist.py`.

Pattern: `import logging` → `import structlog`, `logging.getLogger` → `structlog.get_logger`, `%s` format → keyword args.

- [ ] **Step 2: Verify routing tests**

Run: `cd workers && python -m pytest tests/test_routing*.py -v`
Expected: All pass.

- [ ] **Step 3: Commit routing migration**

```bash
git add workers/codeforge/routing/
git commit -m "refactor: migrate routing/ to structlog (FIX-092, 5 files)"
```

- [ ] **Step 4: Migrate backends/ files (8 files)**

Migrate `aider.py`, `goose.py`, `opencode.py`, `openhands.py`, `plandex.py`, `sweagent.py`, `router.py`, `_streaming.py`.

- [ ] **Step 5: Verify backend tests**

Run: `cd workers && python -m pytest tests/test_backend*.py tests/test_stub*.py -v`
Expected: All pass.

- [ ] **Step 6: Commit backends migration**

```bash
git add workers/codeforge/backends/
git commit -m "refactor: migrate backends/ to structlog (FIX-092, 8 files)"
```

- [ ] **Step 7: Migrate tools/ files (12 files)**

Migrate all files in `workers/codeforge/tools/`.

- [ ] **Step 8: Verify tool tests**

Run: `cd workers && python -m pytest tests/test_tools*.py -v`
Expected: All pass.

- [ ] **Step 9: Commit tools migration**

```bash
git add workers/codeforge/tools/
git commit -m "refactor: migrate tools/ to structlog (FIX-092, 12 files)"
```

- [ ] **Step 10: Migrate evaluation/ files (2 files)**

Migrate `prompt_mutator.py`, `prompt_optimizer.py`.

- [ ] **Step 11: Commit evaluation migration**

```bash
git add workers/codeforge/evaluation/
git commit -m "refactor: migrate evaluation/ to structlog (FIX-092, 2 files)"
```

- [ ] **Step 12: Migrate remaining root-level files (16 files)**

Migrate `agent_loop.py`, `history.py`, `logger.py`, `executor.py`, `config.py`, `llm.py`, `claude_code_executor.py`, `mcp_workbench.py`, `model_resolver.py`, `subprocess_utils.py`, `context_reranker.py`, `schemas/parser.py`, `skills/selector.py`, `skills/safety.py`.

- [ ] **Step 13: Run full test suite**

Run: `cd workers && python -m pytest tests/ -v --timeout=30`
Expected: All tests pass.

- [ ] **Step 14: Commit remaining migration**

```bash
git add workers/codeforge/
git commit -m "refactor: migrate remaining 16 files to structlog (FIX-092)"
```

- [ ] **Step 15: Remove LOG_MIGRATION.md**

The migration is complete — remove the tracking document.

```bash
git rm workers/codeforge/LOG_MIGRATION.md
git commit -m "chore: remove LOG_MIGRATION.md — migration complete (FIX-092)"
```

---

### Task C2: Consolidate FakeLLM into tests/helpers/

**Files:**
- Move: `workers/tests/fake_llm.py` → `workers/tests/helpers/fake_llm.py`
- Create: `workers/tests/helpers/__init__.py`
- Modify: `workers/tests/conftest.py` (update import)
- Modify: `workers/tests/test_context_reranker.py` (update import)
- Modify: `workers/tests/test_conversation_summarizer.py` (update import)
- Modify: `workers/tests/test_role_evaluation.py` (update import)

**Context:** FakeLLM is imported from 4 test files. Consolidating into `tests/helpers/` provides a clear location for all test doubles and matches the FIX-101 intent.

- [ ] **Step 1: Create helpers directory**

```bash
mkdir -p workers/tests/helpers
touch workers/tests/helpers/__init__.py
```

- [ ] **Step 2: Move fake_llm.py**

```bash
git mv workers/tests/fake_llm.py workers/tests/helpers/fake_llm.py
```

- [ ] **Step 3: Add re-export in helpers/__init__.py**

```python
from tests.helpers.fake_llm import FakeLLM

__all__ = ["FakeLLM"]
```

- [ ] **Step 4: Update imports in 4 test files**

In each file, change:
```python
from tests.fake_llm import FakeLLM
```
To:
```python
from tests.helpers import FakeLLM
```

Files: `conftest.py`, `test_context_reranker.py`, `test_conversation_summarizer.py`, `test_role_evaluation.py`.

- [ ] **Step 5: Run tests to verify**

Run: `cd workers && python -m pytest tests/ -v --timeout=30`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add workers/tests/
git commit -m "refactor: consolidate FakeLLM into tests/helpers/ (FIX-101)"
```

---

# Worktree D: `refactor/api-v2-handlers` (Future — Separate Plan Required)

**Scope definition for future planning:**

### D1: REST API v2 Migration
- **FIX-061:** Rename 15+ verb-based endpoints to noun-based resources
- **FIX-063:** Add standard pagination envelope `{items, total, limit, offset, has_more}` to ~10 list endpoints
- **FIX-098:** Change 2 delete-via-POST endpoints to proper HTTP DELETE
- **FIX-100:** Change 15 PUT endpoints to PATCH for partial updates
- **FIX-095:** No action needed — SameSite + Bearer auth is sufficient for JSON API

**Approach:** Version prefix `/api/v2/` with v1 compatibility period. Frontend updated simultaneously.

### D2: Handlers Struct Decomposition
- Split 70-field monolith `Handlers` struct into ~12 domain-specific handler groups
- Each group gets its own struct with only the services it needs (4-8 fields)
- Wire via `HandlerRegistry` aggregate in `main.go`
- Extract config fields (`Limits`, `AgentConfig`, `AppEnv`) into middleware

**Prerequisite:** Best done alongside API v2 since both touch all 60 handler files.

**Estimated effort:** 5 days total (3d API v2 + 2d decomposition)

---

# Worktree E: `feat/extensibility-layer` (Future — Separate Plan Required)

**Scope definition for future planning:**

### E1: Hook System (Observer Pattern)
- Define `Hook` interface with `Before`/`After` callbacks and event context
- Build `HookRegistry` (sync.Map, register/unregister by event type)
- Build `HookDispatcher` service (iterate hooks, fail-open for logging, fail-fast for critical)
- Integrate at 4 points: runtime (run start/end), conversation (step done), LiteLLM (model query), container (init/close)
- YAML config for enabling/disabling hooks per mode

**Estimated effort:** 1-2 days, ~600 LOC

### E2: Ed25519 Signatures
- Key generation service (stdlib `crypto/ed25519`, encrypted storage)
- Signing middleware (NATS publisher-side, inject into `Annotation.Signature`)
- Verification service (check signature against agent public key registry)
- Agent key registry (DB table `agent_keys`, memory cache with TTL)
- A2A integration (require `LevelVerified` for external agent messages)
- Key rotation (timestamp-based expiry, key history)

**Prerequisite:** Hook System (E1) should be done first — Ed25519 verification can be implemented as a hook.

**Estimated effort:** 3-5 days, ~1200 LOC

---

# Documentation Updates

After all worktrees are merged, update:

- [ ] `docs/todo.md` — Mark FIX-075, FIX-093, FIX-104, FIX-106, FIX-092, FIX-101 as completed
- [ ] `docs/audits/2026-03-22-stub-finder-report.md` — Add "Remediated" status to each fixed finding
- [ ] `docs/project-status.md` — Note stub finder remediation milestone
- [ ] `CLAUDE.md` — Update LSP section from "NOT yet wired" to "auto-starts on project setup"
