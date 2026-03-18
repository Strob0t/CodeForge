# E2E Findings Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix all 5 findings (F1-F5) from the 2026-03-09 E2E Playwright test — workspace path bug, routing fallback, file upload UI, feature description UI, and Playwright healthcheck.

**Architecture:** F4 (critical) fixes the Go Core to send absolute workspace paths in NATS payloads so the Python worker resolves files correctly. F3 extends the rate tracker to classify billing/auth errors and block providers with longer cooldowns. F1/F2 are frontend-only changes adding missing UI elements. F5 is a Docker config change.

**Tech Stack:** Go 1.25, Python 3.12 (Pydantic, pytest), TypeScript (SolidJS), Docker Compose

---

## Task 1: F4 — Reproduce Workspace Path Bug with Test

**Files:**
- Create: `workers/tests/test_workspace_path_resolution.py`

**Step 1: Write the failing test**

```python
"""Test that workspace path resolution works with both absolute and relative paths."""

import os
import tempfile
from pathlib import Path

import pytest

from codeforge.tools._base import resolve_safe_path


class TestWorkspacePathResolution:
    """Verify resolve_safe_path handles relative workspace paths correctly."""

    def test_absolute_path_resolves_correctly(self, tmp_path: Path) -> None:
        """Absolute workspace path should resolve files inside it."""
        test_file = tmp_path / "hello.py"
        test_file.write_text("print('hello')")

        resolved, err = resolve_safe_path(str(tmp_path), "hello.py", must_be_file=True)
        assert err is None
        assert resolved == test_file

    def test_relative_path_resolves_to_wrong_location(self) -> None:
        """Relative workspace path resolves against CWD, not project root.

        This test documents the BUG: when workspace_path is relative
        (e.g. 'data/workspaces/tenant/project'), Path.resolve() uses CWD
        which is workers/ — NOT the CodeForge project root.
        """
        # Simulate the relative path the Go Core currently sends
        relative_ws = "data/workspaces/00000000/test-project"

        # resolve_safe_path will resolve this relative to CWD
        resolved, err = resolve_safe_path(relative_ws, "test.py")

        # The resolved workspace will be CWD/data/workspaces/... (WRONG)
        cwd = Path.cwd()
        expected_wrong = (cwd / relative_ws / "test.py").resolve()
        assert resolved == expected_wrong

        # But the file should be at PROJECT_ROOT/data/workspaces/... (CORRECT)
        # This proves the bug: relative paths resolve against CWD, not project root
```

**Step 2: Run test to verify it passes (documenting current buggy behavior)**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/test_workspace_path_resolution.py -v`
Expected: PASS (both tests pass — the second test documents the bug by asserting wrong behavior)

**Step 3: Commit**

```bash
git add workers/tests/test_workspace_path_resolution.py
git commit -m "test(F4): add workspace path resolution test documenting relative path bug"
```

---

## Task 2: F4 — Fix Go Core to Send Absolute Workspace Paths

**Files:**
- Modify: `internal/service/project.go:163,221`

The root cause is that `InitWorkspace` and `Clone` build paths with `filepath.Join(s.workspaceRoot, tenantID, p.ID)` where `s.workspaceRoot` is the config value `data/workspaces` (relative). The fix is to resolve `workspaceRoot` to an absolute path at service construction time.

**Step 1: Write the failing test**

```go
// File: internal/service/project_workspace_test.go
package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceRootIsAbsolute(t *testing.T) {
	// Simulate the relative default from config
	relativeRoot := "data/workspaces"

	// filepath.Join with a relative root produces a relative result
	result := filepath.Join(relativeRoot, "tenant", "project")
	if filepath.IsAbs(result) {
		t.Fatal("expected relative path from filepath.Join with relative root")
	}

	// After fix: workspaceRoot should be resolved to absolute at startup
	absRoot, err := filepath.Abs(relativeRoot)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	absResult := filepath.Join(absRoot, "tenant", "project")
	if !filepath.IsAbs(absResult) {
		t.Fatalf("expected absolute path, got: %s", absResult)
	}

	// Verify the absolute path points to the right place
	cwd, _ := os.Getwd()
	expected := filepath.Join(cwd, relativeRoot, "tenant", "project")
	if absResult != expected {
		t.Fatalf("expected %s, got %s", expected, absResult)
	}
}
```

**Step 2: Run test to verify behavior**

Run: `cd /workspaces/CodeForge && go test -run TestWorkspaceRootIsAbsolute ./internal/service/ -v`
Expected: PASS

**Step 3: Fix project service to resolve workspaceRoot to absolute at construction**

```go
// File: internal/service/project.go
// Find the NewProjectService constructor (or wherever s.workspaceRoot is set).
// Change:
//   s.workspaceRoot = cfg.Workspace.Root
// To:
//   absRoot, err := filepath.Abs(cfg.Workspace.Root)
//   if err != nil {
//       return nil, fmt.Errorf("resolve workspace root: %w", err)
//   }
//   s.workspaceRoot = absRoot
```

Find the constructor and apply:

```go
// In the function that creates ProjectService, after reading cfg.Workspace.Root:
absRoot, err := filepath.Abs(cfg.Workspace.Root)
if err != nil {
    return nil, fmt.Errorf("resolve workspace root: %w", err)
}
s.workspaceRoot = absRoot
```

**Step 4: Run all project service tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/... -run TestWorkspace -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/service/project.go internal/service/project_workspace_test.go
git commit -m "fix(F4): resolve workspace root to absolute path at service startup

Workspace paths were stored as relative paths (e.g. data/workspaces/tenant/project)
which caused the Python worker to resolve them against its own CWD (workers/)
instead of the project root. By making workspaceRoot absolute at construction,
all downstream paths (NATS payloads, tool execution) are automatically correct."
```

---

## Task 3: F4 — Add NATS Contract Assertion for Absolute workspace_path

**Files:**
- Modify: `workers/tests/test_nats_contracts.py`
- Modify: `internal/port/messagequeue/contract_test.go`

**Step 1: Add Python contract assertion**

In `workers/tests/test_nats_contracts.py`, find the `conversation_run_start` test and add:

```python
# Add to the existing conversation_run_start contract test:
def test_conversation_run_start_workspace_path_is_absolute(
    self, conversation_run_start_fixture: dict,
) -> None:
    """workspace_path must be absolute so Python tools resolve correctly."""
    ws = conversation_run_start_fixture.get("workspace_path", "")
    if ws:  # empty is allowed (non-agentic conversations)
        assert ws.startswith("/"), (
            f"workspace_path must be absolute, got: {ws}"
        )
```

**Step 2: Update Go fixture to use absolute path**

In `internal/port/messagequeue/contract_test.go`, find the `ConversationRunStartPayload` factory and ensure `WorkspacePath` uses an absolute path:

```go
// In the fixture factory for ConversationRunStartPayload, change:
//   WorkspacePath: "data/workspaces/...",
// To:
//   WorkspacePath: "/workspaces/CodeForge/data/workspaces/00000000-0000-0000-0000-000000000000/test-project",
```

**Step 3: Regenerate fixtures and run contract tests**

Run: `cd /workspaces/CodeForge && go test -run TestContract_GenerateFixtures ./internal/port/messagequeue/ -v`
Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/test_nats_contracts.py -v -k conversation_run_start`
Expected: PASS

**Step 4: Commit**

```bash
git add workers/tests/test_nats_contracts.py internal/port/messagequeue/contract_test.go internal/port/messagequeue/testdata/contracts/conversation_run_start.json
git commit -m "test(F4): add contract assertion that workspace_path is absolute"
```

---

## Task 4: F4 — Update Python Test to Verify Fix

**Files:**
- Modify: `workers/tests/test_workspace_path_resolution.py`

**Step 1: Update the test to assert correct behavior**

Replace the second test method:

```python
    def test_absolute_workspace_path_from_nats_resolves_correctly(self, tmp_path: Path) -> None:
        """After fix: Go Core sends absolute paths, so resolution is correct."""
        test_file = tmp_path / "hello.py"
        test_file.write_text("print('hello')")

        # Go Core now sends absolute path like /workspaces/CodeForge/data/workspaces/tenant/project
        resolved, err = resolve_safe_path(str(tmp_path), "hello.py", must_be_file=True)
        assert err is None
        assert resolved == test_file
        assert resolved.is_absolute()
```

**Step 2: Run test**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/test_workspace_path_resolution.py -v`
Expected: PASS

**Step 3: Commit**

```bash
git add workers/tests/test_workspace_path_resolution.py
git commit -m "test(F4): update workspace path test to verify absolute path fix"
```

---

## Task 5: F3 — Extend Rate Tracker with Error Classification

**Files:**
- Modify: `workers/codeforge/routing/rate_tracker.py`
- Create: `workers/tests/test_routing_error_classification.py`

**Step 1: Write the failing test**

```python
"""Test rate tracker error classification for billing/auth errors."""

import time

import pytest

from codeforge.routing.rate_tracker import RateLimitTracker


class TestRateLimitTrackerErrorClassification:
    """Verify that billing/auth errors mark providers as exhausted."""

    def test_billing_error_marks_provider_exhausted(self) -> None:
        tracker = RateLimitTracker()
        assert not tracker.is_exhausted("anthropic")

        tracker.record_error("anthropic", error_type="billing")
        assert tracker.is_exhausted("anthropic")

    def test_auth_error_marks_provider_exhausted(self) -> None:
        tracker = RateLimitTracker()
        tracker.record_error("anthropic", error_type="auth")
        assert tracker.is_exhausted("anthropic")

    def test_billing_exhaustion_lasts_one_hour(self) -> None:
        tracker = RateLimitTracker()
        tracker.record_error("anthropic", error_type="billing")
        assert tracker.is_exhausted("anthropic")

        # Simulate time passing (1 hour + 1 second)
        with pytest.MonkeyPatch.context() as mp:
            mp.setattr(time, "monotonic", lambda: time.monotonic() + 3601)
            assert not tracker.is_exhausted("anthropic")

    def test_auth_exhaustion_lasts_five_minutes(self) -> None:
        tracker = RateLimitTracker()
        tracker.record_error("anthropic", error_type="auth")
        assert tracker.is_exhausted("anthropic")

        # After 5 min + 1s, should be cleared
        with pytest.MonkeyPatch.context() as mp:
            mp.setattr(time, "monotonic", lambda: time.monotonic() + 301)
            assert not tracker.is_exhausted("anthropic")

    def test_error_appears_in_exhausted_providers(self) -> None:
        tracker = RateLimitTracker()
        tracker.record_error("anthropic", error_type="billing")
        tracker.record_error("openai", error_type="auth")
        exhausted = tracker.get_exhausted_providers()
        assert "anthropic" in exhausted
        assert "openai" in exhausted

    def test_unknown_error_type_ignored(self) -> None:
        tracker = RateLimitTracker()
        tracker.record_error("anthropic", error_type="unknown")
        assert not tracker.is_exhausted("anthropic")
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/test_routing_error_classification.py -v`
Expected: FAIL — `RateLimitTracker` has no `record_error` method

**Step 3: Implement `record_error()` in RateLimitTracker**

Add to `workers/codeforge/routing/rate_tracker.py`:

```python
# Add near top of file, after existing imports:
_ERROR_COOLDOWNS: dict[str, float] = {
    "billing": 3600.0,   # 1 hour
    "auth": 300.0,        # 5 minutes
}

# Add to RateLimitTracker class, after the existing update() method:

    def record_error(self, provider: str, *, error_type: str) -> None:
        """Mark a provider as exhausted due to a non-rate-limit error.

        Supported error_type values: "billing", "auth".
        Unknown types are silently ignored.
        """
        cooldown = _ERROR_COOLDOWNS.get(error_type)
        if cooldown is None:
            return
        with self._lock:
            self._state[provider] = _ProviderState(
                remaining_requests=0,
                reset_at=time.monotonic() + cooldown,
                last_update=time.monotonic(),
            )
```

Also update `is_exhausted()` to check `reset_at` against `time.monotonic()`:

```python
    def is_exhausted(self, provider: str) -> bool:
        """Return True when *provider* has exhausted its request budget."""
        with self._lock:
            state = self._state.get(provider)
            if state is None:
                return False
            if state.remaining_requests is not None and state.remaining_requests > 0:
                return False
            if self._is_stale(state):
                del self._state[provider]
                return False
            return True
```

Ensure `_ProviderState` supports `reset_at` field (check if it already has one — it tracks `reset_requests` from headers).

**Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/test_routing_error_classification.py -v`
Expected: PASS

**Step 5: Commit**

```bash
git add workers/codeforge/routing/rate_tracker.py workers/tests/test_routing_error_classification.py
git commit -m "feat(F3): extend rate tracker with billing/auth error classification

Adds record_error() to RateLimitTracker. Billing errors mark provider
exhausted for 1 hour, auth errors for 5 minutes. This prevents the
router from selecting providers with known payment/credential issues."
```

---

## Task 6: F3 — Wire Error Classification into LLM Client

**Files:**
- Modify: `workers/codeforge/llm.py`

**Step 1: Find `_with_retry()` method (around line 355)**

After a non-retryable `LLMError` is caught, classify the error and record it:

```python
# In _with_retry(), after the retryable check fails (the `else` branch of
# `if self._is_retryable(exc)`), add before re-raising:

from codeforge.routing.rate_tracker import get_tracker

# Classify error for rate tracker
provider = model.split("/")[0] if "/" in model else ""
if provider:
    tracker = get_tracker()
    if exc.status_code in (401, 403):
        tracker.record_error(provider, error_type="auth")
    elif exc.status_code == 402 or (
        exc.status_code == 400
        and any(kw in exc.body.lower() for kw in ("credit", "billing", "balance", "quota", "budget"))
    ):
        tracker.record_error(provider, error_type="billing")
```

**Step 2: Run existing LLM tests**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/ -k "llm" -v`
Expected: PASS

**Step 3: Commit**

```bash
git add workers/codeforge/llm.py
git commit -m "feat(F3): classify LLM errors and feed into rate tracker

Auth errors (401/403) and billing errors (402, credit/billing keywords)
are now recorded in the rate tracker so the router skips broken providers."
```

---

## Task 7: F2 — Add Description Textarea to Feature Form

**Files:**
- Modify: `frontend/src/features/project/featuremap/FeatureCardForm.tsx`
- Modify: `frontend/src/i18n/en.ts`
- Modify: `frontend/src/i18n/locales/de.ts`

**Step 1: Add i18n keys**

In `frontend/src/i18n/en.ts`, find the `featuremap` section (around line 571) and add after `"featuremap.featurePlaceholder"`:

```typescript
"featuremap.descriptionPlaceholder": "Feature description (optional)...",
```

In `frontend/src/i18n/locales/de.ts`, add the corresponding key:

```typescript
"featuremap.descriptionPlaceholder": "Feature-Beschreibung (optional)...",
```

**Step 2: Add description signal and textarea to FeatureCardForm**

In `frontend/src/features/project/featuremap/FeatureCardForm.tsx`:

Add import for `Textarea`:

```typescript
import { Button, Input, Select, Textarea } from "~/ui";
```

Add description signal after the title signal (after line 31):

```typescript
  // eslint-disable-next-line solid/reactivity -- intentional one-time initialization
  const [description, setDescription] = createSignal(props.feature?.description ?? "");
```

Pass description in `createFeature` call (line 48-49):

```typescript
        await api.roadmap.createFeature(props.milestoneId, {
          title: trimmed,
          description: description().trim() || undefined,
        });
```

Pass description in `updateFeature` call (line 41-44):

```typescript
        await api.roadmap.updateFeature(props.feature.id, {
          title: trimmed,
          description: description().trim(),
          status: status(),
          version: props.feature.version,
        });
```

Add textarea in JSX after the `<Input>` (after line 80):

```tsx
      <Textarea
        placeholder={t("featuremap.descriptionPlaceholder")}
        value={description()}
        onInput={(e) => setDescription(e.currentTarget.value)}
        rows={3}
      />
```

**Step 3: Run frontend type check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/features/project/featuremap/FeatureCardForm.tsx frontend/src/i18n/en.ts frontend/src/i18n/locales/de.ts
git commit -m "feat(F2): add description textarea to feature create/edit form

The feature card form now includes a multi-line description field.
Descriptions are passed to createFeature and updateFeature API calls.
This allows users to provide full feature specs through the UI."
```

---

## Task 8: F1 — Add Create File Modal to FilePanel

**Files:**
- Modify: `frontend/src/features/project/FilePanel.tsx`
- Modify: `frontend/src/i18n/en.ts`
- Modify: `frontend/src/i18n/locales/de.ts`

**Step 1: Add i18n keys**

In `frontend/src/i18n/en.ts`, find a suitable location (near `files` or `common` section) and add:

```typescript
"files.createFile": "New File",
"files.fileName": "File path",
"files.fileNamePlaceholder": "e.g. src/main.py",
"files.fileContent": "Content",
"files.fileContentPlaceholder": "File content (optional)...",
"files.createSuccess": "File created",
"files.createFailed": "Failed to create file",
```

In `frontend/src/i18n/locales/de.ts`:

```typescript
"files.createFile": "Neue Datei",
"files.fileName": "Dateipfad",
"files.fileNamePlaceholder": "z.B. src/main.py",
"files.fileContent": "Inhalt",
"files.fileContentPlaceholder": "Dateiinhalt (optional)...",
"files.createSuccess": "Datei erstellt",
"files.createFailed": "Datei konnte nicht erstellt werden",
```

**Step 2: Add Create File modal to FilePanel**

In `frontend/src/features/project/FilePanel.tsx`:

Add imports (merge with existing):

```typescript
import { Modal, Button, Input, Textarea } from "~/ui";
import { FormField } from "~/ui";
```

Add state signals near other signals:

```typescript
const [createFileOpen, setCreateFileOpen] = createSignal(false);
const [newFilePath, setNewFilePath] = createSignal("");
const [newFileContent, setNewFileContent] = createSignal("");
```

Add the create file handler using `useAsyncAction`:

```typescript
const { run: handleCreateFile, loading: creating } = useAsyncAction(
  async () => {
    const path = newFilePath().trim();
    if (!path) return;
    await api.files.write(projectId(), path, newFileContent());
    toast("success", t("files.createSuccess"));
    setCreateFileOpen(false);
    setNewFilePath("");
    setNewFileContent("");
    // Refresh file tree
    refetchTree();
  },
  {
    onError: (err) => {
      toast("error", getErrorMessage(err, t("files.createFailed")));
    },
  },
);
```

Add a "+" button in the sidebar header (find the header area with expand/collapse buttons, around line 85-95):

```tsx
<button
  class="rounded p-1 text-cf-text-secondary hover:bg-cf-bg-hover"
  onClick={() => setCreateFileOpen(true)}
  title={t("files.createFile")}
>
  +
</button>
```

Add the Modal JSX before the closing `</>` of the component:

```tsx
<Modal
  open={createFileOpen()}
  onClose={() => setCreateFileOpen(false)}
  title={t("files.createFile")}
>
  <div class="flex flex-col gap-3">
    <FormField id="new-file-path" label={t("files.fileName")} required>
      <Input
        id="new-file-path"
        type="text"
        placeholder={t("files.fileNamePlaceholder")}
        value={newFilePath()}
        onInput={(e) => setNewFilePath(e.currentTarget.value)}
        autofocus
      />
    </FormField>
    <FormField id="new-file-content" label={t("files.fileContent")}>
      <Textarea
        id="new-file-content"
        placeholder={t("files.fileContentPlaceholder")}
        value={newFileContent()}
        onInput={(e) => setNewFileContent(e.currentTarget.value)}
        rows={8}
        mono
      />
    </FormField>
    <div class="flex justify-end gap-2">
      <Button variant="ghost" onClick={() => setCreateFileOpen(false)}>
        {t("featuremap.cancel")}
      </Button>
      <Button
        variant="primary"
        onClick={() => void handleCreateFile()}
        disabled={creating() || !newFilePath().trim()}
        loading={creating()}
      >
        {t("files.createFile")}
      </Button>
    </div>
  </div>
</Modal>
```

**Step 3: Run frontend type check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/features/project/FilePanel.tsx frontend/src/i18n/en.ts frontend/src/i18n/locales/de.ts
git commit -m "feat(F1): add Create File modal to FilePanel

Users can now create new files in the project workspace directly from
the UI via a '+' button in the file tree header. The modal accepts
a file path and optional content."
```

---

## Task 9: F5 — Add Playwright Docker Healthcheck

**Files:**
- Modify: `docker-compose.yml`
- Modify: `docs/dev-setup.md`

**Step 1: Add healthcheck to Playwright service**

In `docker-compose.yml`, find the `playwright` service and add:

```yaml
    healthcheck:
      test: ["CMD", "node", "-e", "fetch('http://localhost:8001/mcp').catch(() => process.exit(1))"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 5s
```

**Step 2: Add documentation note**

In `docs/dev-setup.md`, find the section about Docker services or Playwright and add:

```markdown
### Playwright MCP Container

The `codeforge-playwright` container provides browser automation via Model Context Protocol.

**Important:** The MCP session is ephemeral — if the container restarts, all active MCP
sessions become invalid ("Session not found"). You must reconnect from the MCP client
(e.g., restart Claude Code or the MCP client process) after a container restart.
```

**Step 3: Test healthcheck**

Run: `docker compose up -d playwright && sleep 5 && docker inspect --format='{{.State.Health.Status}}' codeforge-playwright`
Expected: `healthy`

**Step 4: Commit**

```bash
git add docker-compose.yml docs/dev-setup.md
git commit -m "fix(F5): add healthcheck to Playwright MCP container and document session limitation"
```

---

## Task 10: Integration Verification — Run Full Test Suite

**Files:** None (verification only)

**Step 1: Run Go tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/... -v -count=1 2>&1 | tail -20`
Expected: All PASS

**Step 2: Run Python tests**

Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/ -v --tb=short 2>&1 | tail -30`
Expected: All PASS

**Step 3: Run NATS contract tests**

Run: `cd /workspaces/CodeForge && go test -run TestContract_GenerateFixtures ./internal/port/messagequeue/ -v`
Run: `cd /workspaces/CodeForge/workers && python -m pytest tests/test_nats_contracts.py -v`
Expected: All PASS

**Step 4: Run frontend type check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 5: Run pre-commit**

Run: `cd /workspaces/CodeForge && pre-commit run --all-files`
Expected: All PASS

**Step 6: Commit any lint fixes if needed**

---

## Task 11: Update Documentation

**Files:**
- Modify: `docs/todo.md`
- Modify: `docs/project-status.md`

**Step 1: Mark all F1-F5 tasks as done in todo.md**

In `docs/todo.md`, change all `- [ ] F*.N:` to `- [x] F*.N: (2026-03-09)` under the "E2E Playwright Test Findings" section.

**Step 2: Add entry to project-status.md**

Add under the latest section:

```markdown
#### E2E Findings Fix (2026-03-09)
- [x] F4: Workspace path resolution — absolute paths in NATS payloads
- [x] F3: Routing fallback — billing/auth error classification in rate tracker
- [x] F2: Feature description textarea in roadmap UI
- [x] F1: Create File modal in FilePanel
- [x] F5: Playwright Docker healthcheck + documentation
```

**Step 3: Commit**

```bash
git add docs/todo.md docs/project-status.md
git commit -m "docs: mark E2E findings F1-F5 as resolved"
```

---

## Dependency Graph

```
Task 1 (F4 test)
  └── Task 2 (F4 Go fix) ──── depends on Task 1
       └── Task 3 (F4 contract) ── depends on Task 2
            └── Task 4 (F4 verify) ── depends on Task 3

Task 5 (F3 rate tracker) ──── independent
  └── Task 6 (F3 LLM wiring) ── depends on Task 5

Task 7 (F2 description) ──── independent
Task 8 (F1 file modal) ───── independent
Task 9 (F5 healthcheck) ──── independent

Task 10 (integration verify) ── depends on ALL above
  └── Task 11 (docs) ────────── depends on Task 10
```

**Parallelizable groups:**
- Group A: Tasks 1→2→3→4 (F4 chain)
- Group B: Tasks 5→6 (F3 chain)
- Group C: Task 7 (F2)
- Group D: Task 8 (F1)
- Group E: Task 9 (F5)

Groups A-E can all run in parallel. Tasks 10-11 run after all groups complete.
