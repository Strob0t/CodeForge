# QA Run 2 Bugfixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 4 minor bugs found during Full Service Interactive QA Run 2.

**Architecture:** All fixes are isolated, single-file changes in the frontend (3 SolidJS fixes) and backend (1 Go fix). No cross-boundary changes.

**Tech Stack:** TypeScript/SolidJS (frontend), Go (backend)

---

## File Structure

| Bug | File | Change |
|-----|------|--------|
| BUG-001 | `frontend/src/features/project/GoalsPanel.tsx:68` | Fix `result.imported` -> `result.goals_created` |
| BUG-002 | `frontend/src/features/project/FilePanel.tsx:208-217` | Add `invalidateCache()` after file creation |
| BUG-003 | `internal/service/project.go:319-333` | Catch "no remote" git error, return `ErrValidation` with message |
| BUG-004 | `frontend/src/features/project/GoalsPanel.tsx:194` | Add `relative z-10` to goals list container |

---

### Task 1: BUG-001 — Goals Detect toast shows "undefined" instead of count

**Root cause:** Backend returns `GoalDiscoveryResult` with field `goals_created` (JSON: `"goals_created"`). Frontend reads `result.imported` which doesn't exist — hence `undefined`.

**Files:**
- Modify: `frontend/src/features/project/GoalsPanel.tsx:68`

- [ ] **Step 1: Fix the field name**

In `GoalsPanel.tsx` line 68, change `result.imported` to `result.goals_created`:

```tsx
toast("success", t("goals.toast.detected", { count: String(result.goals_created) }));
```

- [ ] **Step 2: Verify in browser**

Navigate to project -> Goals panel -> click "Detect Goals". Toast should now show `"Detected and imported 1 goals"` (or correct count) instead of `"Detected and imported undefined goals"`.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/project/GoalsPanel.tsx
git commit -m "fix(goals): detect toast shows count instead of undefined

BUG-001: API returns goals_created, not imported."
```

---

### Task 2: BUG-002 — File tree doesn't refresh after New File creation

**Root cause:** `handleCreateFile` in `FilePanel.tsx` creates the file and opens it in a tab, but never invalidates the FileTree cache. The `FileTreeContext` has an `invalidateCache()` method that sets `fullTree` to `null`, forcing a re-fetch on next render.

**Files:**
- Modify: `frontend/src/features/project/FilePanel.tsx:208-217`

- [ ] **Step 1: Get the FileTree actions in handleCreateFile scope**

The `FilePanel` component already imports `useFileTree` (line 17) and uses it in sub-components. The `handleCreateFile` is defined inside the default export function, so we need to call `useFileTree()` at the component level and use `actions.invalidateCache()` after file creation.

Find the existing `useFileTree` usage at line 89 — it's inside the `FileToolbar` sub-component, not the main component. We need to add it to the main `FilePanel` component scope.

In `FilePanel.tsx`, inside the `export default function FilePanel` body (around line 200-207), add the tree context access and invalidation:

```tsx
// Around line 200, after other hooks:
const [, treeActions] = useFileTree();

// Then in handleCreateFile (line 208-218), add invalidateCache after openFile:
const { run: handleCreateFile, loading: creating } = useAsyncAction(
  async () => {
    const filePath = newFilePath().trim();
    if (!filePath) return;
    await api.files.write(props.projectId, filePath, newFileContent());
    toast("success", t("files.createSuccess"));
    setShowCreateModal(false);
    setNewFilePath("");
    setNewFileContent("");
    treeActions.invalidateCache();
    openFile(filePath);
  },
  {
```

- [ ] **Step 2: Verify in browser**

Navigate to project -> Files panel -> click "New File" -> fill path "test-refresh.txt" + content -> submit. The file should appear **immediately** in the file tree without requiring page navigation.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/project/FilePanel.tsx
git commit -m "fix(files): refresh file tree after creating new file

BUG-002: Call invalidateCache() after write so tree re-fetches."
```

---

### Task 3: BUG-003 — Git Pull returns "internal server error" for local projects

**Root cause:** `ProjectService.Pull()` calls `gp.Pull(ctx, workspace)` which runs `git pull` in a repo with no remote. Git returns an error, which propagates as a generic 500. Should be caught and returned as a 422 validation error with a helpful message.

**Files:**
- Modify: `internal/service/project.go:319-333`
- Test: `internal/service/project_test.go` (if existing pull test exists, otherwise skip)

- [ ] **Step 1: Add "no remote" error handling in Pull**

In `internal/service/project.go`, the `Pull` method (line 319-333), wrap the `gp.Pull` error to detect "no remote" situations:

```go
func (s *ProjectService) Pull(ctx context.Context, id string) error {
	p, err := s.store.GetProject(ctx, id)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if p.WorkspacePath == "" {
		return fmt.Errorf("%w: project %s has no workspace (not cloned)", domain.ErrValidation, id)
	}

	gp, err := resolveGitProvider(p)
	if err != nil {
		return fmt.Errorf("create git provider: %w", err)
	}

	if err := gp.Pull(ctx, p.WorkspacePath); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no remote") ||
			strings.Contains(errMsg, "does not have a default remote") ||
			strings.Contains(errMsg, "No remote repository specified") ||
			strings.Contains(errMsg, "no such remote") {
			return fmt.Errorf("%w: no remote configured for this project", domain.ErrValidation)
		}
		return fmt.Errorf("git pull: %w", err)
	}
	return nil
}
```

Ensure `"strings"` is in the imports if not already present.

- [ ] **Step 2: Verify the error response**

The `writeDomainError` handler in `handlers.go:482-483` already maps `domain.ErrValidation` to HTTP 422. So the frontend will now receive a 422 with message `"no remote configured for this project"` instead of a generic 500.

- [ ] **Step 3: Commit**

```bash
git add internal/service/project.go
git commit -m "fix(git): return helpful error when pulling without remote

BUG-003: Detect 'no remote' git errors and return 422 with message
instead of generic 500."
```

---

### Task 4: BUG-004 — Goals ON/OFF toggle button intercepted by chat header

**Root cause:** The goals list container at `GoalsPanel.tsx:194` scrolls behind the chat panel header due to z-index stacking. The GoalsPanel form was already fixed with `relative z-20` (line 162), but the scrollable list below it also needs a stacking context to prevent the chat header from intercepting pointer events.

**Files:**
- Modify: `frontend/src/features/project/GoalsPanel.tsx:194`

- [ ] **Step 1: Add z-index to the goals list container**

In `GoalsPanel.tsx` line 194, add `relative z-10` to the goals list scrollable container:

```tsx
<div class="flex-1 overflow-y-auto px-4 pb-4 space-y-4 relative z-10">
```

This creates a stacking context that keeps the goal toggle buttons above the chat panel header.

- [ ] **Step 2: Verify in browser**

Navigate to project -> Goals panel -> scroll to bottom goal -> click ON/OFF toggle. Button should be clickable without being intercepted by the chat header.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/project/GoalsPanel.tsx
git commit -m "fix(goals): z-index on list prevents chat header overlap

BUG-004: Add relative z-10 to goals list container so toggle buttons
are clickable when near the chat panel boundary."
```

---

## Final Step

- [ ] **Push all commits to staging**

```bash
git push origin staging
```
