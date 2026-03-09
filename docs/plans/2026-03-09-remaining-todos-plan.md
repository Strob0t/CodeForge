# Remaining TODOs Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close all remaining open items from `docs/todo.md` — F3 routing fallback, F1 file upload, F2/F4 verification, and C3 CI improvements.

**Architecture:** Small, targeted fixes across Python (agent loop, rate tracker), Go (config default), and TypeScript (FilePanel). No new domain models or migrations needed.

**Tech Stack:** Python (workers/codeforge), Go (internal/config), TypeScript/SolidJS (frontend), Playwright (E2E)

---

## Dependency Graph

```
Task 1 (F3.4) ──> Task 2 (F3.5) ──> Task 3 (F3.6)
Task 4 (F1.3) ──> Task 5 (F1.4 + F2.4)
Task 6 (F4.6) — independent
Task 7 (C3) — independent
```

**Parallelizable:** [1] || [4] || [6] || [7], then [2] after [1], then [3] after [2], then [5] after [4].

---

### Task 1: F3.4 — Wire rate_tracker.record_error into agent loop fallback

> When the agent loop encounters a billing/auth error, mark the provider as exhausted
> in the RateLimitTracker so the HybridRouter skips it on subsequent calls.

**Files:**
- Modify: `workers/codeforge/agent_loop.py:123-148` (`_try_model_fallback`)
- Test: `workers/tests/test_agent_loop.py` (add new test case)

**Step 1: Write the failing test**

Add to `workers/tests/test_agent_loop.py` (or create if not exists):

```python
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from codeforge.agent_loop import AgentLoopExecutor, LoopConfig, _LoopState
from codeforge.llm import LLMError


@pytest.mark.asyncio
async def test_fallback_records_error_in_rate_tracker():
    """When a billing error triggers fallback, the provider should be marked exhausted."""
    llm = MagicMock()
    tools = MagicMock()
    tools.get_openai_tools.return_value = []
    runtime = AsyncMock()
    runtime.send_output = AsyncMock()
    runtime.is_cancelled = False

    executor = AgentLoopExecutor(llm, tools, runtime, "/tmp/ws")

    cfg = LoopConfig(model="anthropic/claude-sonnet-4", fallback_models=["mistral/mistral-large-latest"])
    state = _LoopState(model="anthropic/claude-sonnet-4")
    exc = LLMError(status_code=402, model="anthropic/claude-sonnet-4", body="credits exhausted")

    with patch("codeforge.agent_loop.get_tracker") as mock_get_tracker:
        tracker = MagicMock()
        mock_get_tracker.return_value = tracker

        result = await executor._try_model_fallback(cfg, state, exc)

    assert result is None  # None means "retry with new model"
    assert cfg.model == "mistral/mistral-large-latest"
    tracker.record_error.assert_called_once_with("anthropic", error_type="billing")
```

**Step 2: Run test to verify it fails**

Run: `cd workers && python -m pytest tests/test_agent_loop.py::test_fallback_records_error_in_rate_tracker -v`
Expected: FAIL — `record_error` not called (no such call in current code)

**Step 3: Implement — wire classify_error_type + record_error into _try_model_fallback**

In `workers/codeforge/agent_loop.py`, update the import at the top:

```python
from codeforge.llm import LLMError, classify_error_type, is_fallback_eligible
```

And at the top of `_try_model_fallback` (after adding the model to `failed_models`), add:

```python
    async def _try_model_fallback(
        self,
        cfg: LoopConfig,
        state: _LoopState,
        exc: LLMError,
    ) -> str | None:
        """Attempt to switch to a fallback model. Returns error string or None (retry)."""
        if not is_fallback_eligible(exc) or not cfg.fallback_models:
            return f"LLM call failed: {exc}"
        failed_model = cfg.model
        state.failed_models.add(failed_model)

        # Mark the provider as exhausted in the rate tracker so the HybridRouter
        # skips it on subsequent calls within the same conversation.
        error_type = classify_error_type(exc)
        if error_type:
            from codeforge.routing.rate_tracker import get_tracker
            get_tracker().record_error(_extract_provider(failed_model), error_type=error_type)

        if exc.status_code in (401, 403):
            get_blocklist().block_auth(failed_model, reason=f"HTTP {exc.status_code}")
        next_model = self._pick_next_fallback(cfg, state)
        if next_model is None:
            return f"LLM call failed: {exc}"
        cfg.model = next_model
        logger.warning(
            "model fallback: %s -> %s (status %d)",
            failed_model,
            next_model,
            exc.status_code,
        )
        notice = f"\n[Model {failed_model} unavailable ({exc.status_code}). Switching to {next_model}]\n"
        await self._runtime.send_output(notice)
        return None
```

Also add the import for `_extract_provider`:

```python
from codeforge.llm import LLMError, classify_error_type, is_fallback_eligible, _extract_provider
```

Note: `_extract_provider` is a private function. If it's not exported, inline it:

```python
provider = failed_model.split("/", 1)[0] if "/" in failed_model else failed_model
```

**Step 4: Run test to verify it passes**

Run: `cd workers && python -m pytest tests/test_agent_loop.py::test_fallback_records_error_in_rate_tracker -v`
Expected: PASS

**Step 5: Commit**

```bash
git add workers/codeforge/agent_loop.py workers/tests/test_agent_loop.py
git commit -m "fix(F3.4): wire rate_tracker.record_error into agent loop fallback"
```

---

### Task 2: F3.5 — Enable routing by default when multiple providers configured

> Change Go config so `routing.enabled` defaults to `true` instead of `false`.
> The Python side already defaults to `true` (llm.py:266). Only the Go side
> gates it via the NATS payload field `RoutingEnabled`.

**Files:**
- Modify: `internal/config/config.go:107` (Routing struct default)
- Modify: `internal/config/defaults.go` (or wherever defaults are applied)
- Test: `internal/config/config_test.go` (verify default)

**Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestRoutingEnabledByDefault(t *testing.T) {
    cfg := DefaultConfig()
    if !cfg.Routing.Enabled {
        t.Error("routing.enabled should default to true")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestRoutingEnabledByDefault -v`
Expected: FAIL — `Routing.Enabled` is `false` by default

**Step 3: Implement — change the default**

Find where `DefaultConfig()` or `Defaults()` sets the Routing struct defaults and change:

```go
Routing: Routing{
    Enabled: true,
},
```

Also update the struct doc comment:

```go
type Routing struct {
    Enabled bool `yaml:"enabled"` // Enable intelligent three-layer routing cascade (default: true)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestRoutingEnabledByDefault -v`
Expected: PASS

**Step 5: Run full config tests**

Run: `go test ./internal/config/... -v`
Expected: All pass

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/defaults.go internal/config/config_test.go
git commit -m "feat(F3.5): enable routing by default"
```

---

### Task 3: F3.6 — Manual verification of routing fallback

> This is a manual E2E verification step, not an automated test.
> Requires a running stack with Anthropic (exhausted) + Mistral (working).

**Step 1: Start the stack**

```bash
docker compose up -d postgres nats litellm
APP_ENV=development go run ./cmd/codeforge/
```

**Step 2: Verify routing is enabled**

```bash
curl -s http://localhost:8080/health | jq '.routing_enabled'
# Expected: true
```

**Step 3: Send a conversation without specifying model**

```bash
# Create a project and conversation first, then send a message.
# The HybridRouter should pick a model. If Anthropic is exhausted (402),
# the agent loop should fallback to the next provider.
```

**Step 4: Check logs for fallback**

```bash
docker compose logs codeforge-workers 2>&1 | grep "model fallback"
# Expected: "model fallback: anthropic/... -> mistral/... (status 402)"
```

**Step 5: Mark F3.6 complete in todo.md**

---

### Task 4: F1.3 — Upload File button with native file picker

> Add an "Upload File" button next to the existing "Create File" button in FilePanel.
> Uses native `<input type="file">` with FileReader to read content, then calls
> `api.files.write(projectId, fileName, fileContent)`.

**Files:**
- Modify: `frontend/src/features/project/FilePanel.tsx`
- Modify: `frontend/src/i18n/en.ts` (add `files.uploadFile` if not already present)
- Modify: `frontend/src/i18n/locales/de.ts`

**Step 1: Check existing i18n keys**

Read `frontend/src/i18n/en.ts` and search for `files.uploadFile`. It was added in F1.1, so it should exist. If not, add it.

**Step 2: Add Upload button and hidden file input to FilePanel**

In `frontend/src/features/project/FilePanel.tsx`, find the "Create File" button and add next to it:

```tsx
{/* Upload File button */}
<Button
  size="sm"
  variant="ghost"
  onClick={() => fileInputRef?.click()}
  title={t("files.uploadFile")}
>
  {UploadIcon()}
</Button>
<input
  ref={(el) => (fileInputRef = el)}
  type="file"
  class="hidden"
  onChange={handleFileUpload}
/>
```

Add the UploadIcon SVG (simple arrow-up icon):

```tsx
function UploadIcon(): JSX.Element {
  return (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="none"
      stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
      <path d="M8 12V3" />
      <path d="M4 7l4-4 4 4" />
      <path d="M2 14h12" />
    </svg>
  );
}
```

Add the handler:

```tsx
let fileInputRef: HTMLInputElement | undefined;

const handleFileUpload = async (e: Event) => {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;

  try {
    const content = await file.text();
    await api.files.write(props.projectId, file.name, content);
    toast.success(t("files.uploadSuccess"));
    // Refresh file tree
    ctx.refresh();
  } catch (err) {
    toast.error(getErrorMessage(err));
  }
  // Reset input so same file can be re-uploaded
  input.value = "";
};
```

**Step 3: Test manually**

- Open a project in the browser
- Click Upload File button
- Select a file
- Verify it appears in the file tree
- Verify content matches by clicking on it

**Step 4: Commit**

```bash
git add frontend/src/features/project/FilePanel.tsx
git commit -m "feat(F1.3): add upload file button with native file picker"
```

---

### Task 5: F1.4 + F2.4 — Playwright E2E tests for file CRUD and feature description

> Write Playwright tests to verify file create/upload and feature description persist.

**Files:**
- Create: `frontend/e2e/file-crud.spec.ts`
- Create: `frontend/e2e/feature-description.spec.ts`

**Step 1: Write file CRUD E2E test**

```typescript
import { test, expect } from "@playwright/test";

test.describe("File CRUD", () => {
  test("create file via modal and verify in file tree", async ({ page }) => {
    // Navigate to a project
    await page.goto("/projects");
    // ... login, select project, navigate to files tab
    // Click "Create File" button
    // Fill path and content
    // Submit
    // Verify file appears in tree
  });
});
```

**Step 2: Write feature description E2E test**

```typescript
test.describe("Feature Description", () => {
  test("create feature with description and verify it persists", async ({ page }) => {
    // Navigate to roadmap/feature-map
    // Create a feature with title + description
    // Verify description is visible
    // Edit the feature, change description
    // Verify new description persists
  });
});
```

**Step 3: Run tests**

Run: `cd frontend && npx playwright test e2e/file-crud.spec.ts e2e/feature-description.spec.ts`

**Step 4: Commit**

```bash
git add frontend/e2e/file-crud.spec.ts frontend/e2e/feature-description.spec.ts
git commit -m "test(F1.4+F2.4): Playwright E2E for file CRUD and feature description"
```

---

### Task 6: F4.6 — Re-run agent-eval to verify workspace path fix

> Run the agent evaluation benchmark with Mistral Large to verify that the workspace
> path fix (F4.2) works end-to-end. All 3 features should produce code in the correct
> workspace directory.

**Prerequisites:** Full stack running with `APP_ENV=development`, Mistral API key configured.

**Step 1: Run the evaluation**

```bash
/agent-eval mistral/mistral-large-latest
```

**Step 2: Verify results**

- Expected: 3/3 features produce test-passing implementations
- Check that files are written to the correct workspace path (not doubled)
- Check logs for any `workspace_path` resolution errors

**Step 3: Mark F4.6 complete in todo.md**

---

### Task 7: C3 — Non-critical feature verification improvements

> Two small CI improvements from the C3 section of todo.md.

**Files:**
- Modify: `scripts/verify-features.sh`
- Modify: `.github/workflows/ci.yml`

**Step 1: Add warn-only mode for non-critical features**

In `scripts/verify-features.sh`, after the critical feature check, add:

```bash
# Non-critical features: warn but don't fail
NON_CRITICAL_FEATURES="11,12,13,14,15,16,17,18,19,20,21,24,25,26,27,28,29,30"
NON_CRITICAL_FAILURES=0
for f in $(echo "$NON_CRITICAL_FEATURES" | tr ',' ' '); do
    if ! check_feature "$f"; then
        echo "::warning::Non-critical feature $f has regressions"
        NON_CRITICAL_FAILURES=$((NON_CRITICAL_FAILURES + 1))
    fi
done
echo "non_critical_failures=$NON_CRITICAL_FAILURES" >> "$GITHUB_OUTPUT"
```

**Step 2: Commit**

```bash
git add scripts/verify-features.sh .github/workflows/ci.yml
git commit -m "feat(C3): warn-only for non-critical feature regressions in CI"
```

---

## Documentation Updates (after all tasks)

After completing all tasks, update:

- `docs/todo.md` — mark all F3.4, F3.5, F3.6, F1.3, F1.4, F2.4, F4.6, C3 items as `[x]`
- `docs/project-status.md` — add "E2E Findings Fix — Completion" note if needed

---

## Out of Scope (Requires Separate Brainstorming)

The following items from `docs/todo.md` are **larger initiatives** that need their own brainstorming + design sessions:

- **Pillar 1:** GitHub adapter with full OAuth flow
- **Pillar 1:** Batch operations across selected repos
- **Pillar 1:** Cross-repo search (requires indexing infrastructure)
- **Pillar 4:** Enhanced CLI wrappers for Goose/OpenHands/OpenCode/Plandex
- **Pillar 4:** Trajectory replay UI (backend exists, frontend missing)
- **Pillar 4:** Session events as source of truth (domain model exists, integration TBD)
- **C3:** Historical verification results for trend tracking

These should be prioritized and tackled one at a time using the brainstorming skill.
