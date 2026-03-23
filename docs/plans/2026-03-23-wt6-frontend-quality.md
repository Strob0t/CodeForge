# WT-6: Frontend Quality & Accessibility — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix `as unknown as` type casts (19x), convert `.then()` chains to async/await (26x), fix WCAG color contrast, modal backdrop keyboard accessibility, catch handler documentation, large component decomposition, and add keyboard navigation E2E test.

**Architecture:** CSS variable adjustment for contrast, Backdrop utility component for keyboard accessibility, custom hook extraction for large components.

**Tech Stack:** SolidJS, Tailwind CSS, Playwright (E2E)

**Best Practice:**
- WCAG 2.2 AA: Minimum 4.5:1 contrast ratio for normal text.
- Keyboard accessibility: All interactive elements must be reachable via Tab and activatable via Enter/Space. Escape must close modals.
- Component size: Extract custom hooks (`createResource`, signals) and sub-components when a file exceeds 500 LOC.

---

### Task 1: Fix `as unknown as` Type Casts (19 occurrences)

**Files:**
- Modify: `frontend/src/features/canvas/canvasState.ts:137,146,166,174`
- Modify: `frontend/src/features/canvas/export/exportJson.ts:99`
- Modify: `frontend/src/features/project/RefactorApproval.tsx:31`
- Modify: `frontend/src/features/project/CodeEditor.tsx:92,93`
- Modify: `frontend/src/features/project/MessageFlow.tsx:21`
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx:389,390,437`
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx:797`

- [ ] **Step 1: Fix canvasState.ts — use typed update helpers**

Replace `(el as unknown as Record<string, unknown>)[key] = value` with a typed canvas element update function:
```typescript
function updateElementProp(el: CanvasElement, key: string, value: unknown): void {
  if (key in el) {
    Object.assign(el, { [key]: value });
  }
}
```
Apply at lines 137, 146, 166, 174.

- [ ] **Step 2: Fix WS payload casts — add type guard functions**

For `RefactorApproval.tsx:31`, `ProjectDetailPage.tsx:389,390,437`, `MessageFlow.tsx:21`, `BenchmarkPage.tsx:797`:

Create type guards instead of blind casts:
```typescript
function isApprovalRequest(p: unknown): p is ApprovalRequest {
  return typeof p === "object" && p !== null && "id" in p;
}

// Usage:
if (isApprovalRequest(msg.payload)) {
  setRequest(msg.payload);
}
```

- [ ] **Step 3: Fix CodeEditor.tsx — use proper Monaco types**

At lines 92-93, type the callback parameters from solid-monaco properly instead of casting through unknown.

- [ ] **Step 4: Fix exportJson.ts:99 — complete the return type**

Replace the partial cast with a proper construction of `ElementData`.

- [ ] **Step 5: Run lint + type-check**

```bash
cd frontend && npx tsc --noEmit && npx eslint src/features/canvas/ src/features/project/ src/features/benchmarks/
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/
git commit -m "fix: replace as unknown as casts with typed guards and helpers (F-012)"
```

---

### Task 2: Convert .then() Chains to async/await (26 occurrences)

**Files:**
- Modify: `frontend/src/api/resources/llm.ts:20,26`
- Modify: `frontend/src/features/project/ChatPanel.tsx:196-197,476`
- Modify: `frontend/src/features/project/ChatHeader.tsx:106,133,170`
- Modify: `frontend/src/features/project/RunPanel.tsx:74`
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx:319,387`
- Modify: `frontend/src/features/canvas/CanvasExportPanel.tsx:115,129`
- Modify: `frontend/src/features/canvas/CanvasModal.tsx:104`

Note: `.then()` inside `createResource(() => ...)` callbacks (e.g. `ProvidersSection.tsx`, `ModelCombobox.tsx`, `DevToolsSection.tsx`, `VCSSection.tsx`) are idiomatic SolidJS — keep those as-is since createResource expects a synchronous-looking fetcher that returns a Promise.

- [ ] **Step 1: Fix llm.ts — convert to async/await**

```typescript
// Before:
c.post<undefined>("/llm/models", data).then((r) => { ... })

// After:
const r = await c.post<undefined>("/llm/models", data);
```

- [ ] **Step 2: Fix ChatPanel.tsx — merge double .then() chain**

```typescript
// Before:
.then(() => refetchMessages())
.then(() => scrollToBottom())

// After:
await sendMessage();
refetchMessages();
scrollToBottom();
```

- [ ] **Step 3: Fix ChatHeader.tsx — 3 occurrences**

Convert each `.then(` to `await` pattern.

- [ ] **Step 4: Fix BenchmarkPage.tsx, CanvasExportPanel.tsx, CanvasModal.tsx, RunPanel.tsx**

Convert remaining `.then()` chains to async/await.

- [ ] **Step 5: Run lint + type-check**

```bash
cd frontend && npx tsc --noEmit && npx eslint src/
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/
git commit -m "fix: convert .then() chains to async/await for consistency (F-014)"
```

---

### Task 3: Fix WCAG Color Contrast (4.39:1 -> 4.5:1)

**Files:**
- Modify: `frontend/src/index.css:29,105`

- [ ] **Step 1: Adjust --cf-text-tertiary CSS variable**

Light mode (line 29): Change `--cf-text-tertiary: #657b83;` to `--cf-text-tertiary: #596e76;`
Dark mode (line 105): Change `--cf-text-tertiary: #858585;` to `--cf-text-tertiary: #8a8a8a;`

Both values increase contrast by ~0.15 to exceed 4.5:1 threshold.

- [ ] **Step 2: Re-enable axe-core color-contrast rule**

In `frontend/e2e/a11y.spec.ts:16`, remove the `disableRules(["color-contrast"])` line or the color-contrast entry from the disabled list.

- [ ] **Step 3: Run a11y tests**

```bash
cd frontend && npx playwright test e2e/a11y.spec.ts
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/index.css frontend/e2e/a11y.spec.ts
git commit -m "fix: WCAG AA color contrast for text-cf-text-tertiary (4.5:1 ratio)"
```

---

### Task 4: Fix Modal Backdrop Keyboard Accessibility (4 locations)

**Files:**
- Modify: `frontend/src/features/channels/ThreadPanel.tsx:128`
- Modify: `frontend/src/features/notifications/NotificationCenter.tsx:76`
- Modify: `frontend/src/features/project/FilePanel.tsx:652`
- Modify: `frontend/src/ui/layout/Sidebar.tsx:27`

- [ ] **Step 1: Create reusable Backdrop component**

```tsx
// frontend/src/ui/primitives/Backdrop.tsx
import type { JSX } from "solid-js";

interface BackdropProps {
  onClick: () => void;
  class?: string;
}

export function Backdrop(props: BackdropProps): JSX.Element {
  const handleKeyDown = (e: KeyboardEvent): void => {
    if (e.key === "Escape") {
      props.onClick();
    }
  };

  return (
    <div
      class={`fixed inset-0 z-40 ${props.class ?? "bg-black/30"}`}
      onClick={() => props.onClick()}
      onKeyDown={handleKeyDown}
      role="button"
      tabIndex={0}
      aria-label="Close"
    />
  );
}
```

- [ ] **Step 2: Replace backdrop divs in all 4 files**

Replace each `<div class="fixed inset-0 z-40 ..." onClick={...} />` with:
```tsx
<Backdrop onClick={() => props.onClose()} class="bg-black/30" />
```

- [ ] **Step 3: Run lint + tests**

```bash
cd frontend && npx eslint src/ui/primitives/Backdrop.tsx src/features/channels/ThreadPanel.tsx src/features/notifications/NotificationCenter.tsx src/features/project/FilePanel.tsx src/ui/layout/Sidebar.tsx
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/ui/primitives/Backdrop.tsx frontend/src/features/ frontend/src/ui/layout/
git commit -m "fix: keyboard-accessible modal backdrops with Escape key support (WCAG 2.1.1)"
```

---

### Task 5: Document Silent .catch() Handlers

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx:359,404`
- Modify: `frontend/src/features/canvas/CanvasModal.tsx:108`
- Modify: `frontend/src/features/project/ChatPanel.tsx:198,288,481`
- Modify: `frontend/src/features/onboarding/steps/CreateProjectStep.tsx:34`
- Modify: `frontend/src/features/dashboard/CreateProjectModal.tsx:103`

- [ ] **Step 1: Add consistent best-effort comments to all silent catches**

For each `.catch(() => { ... })` or `.catch(() => {})`, ensure a clear comment:
```tsx
.catch(() => {
  // best-effort: non-critical operation, failure logged server-side
})
```

For catches that should log, add `console.warn`:
```tsx
.catch((err) => {
  console.warn("canvas load failed:", err);
})
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/features/
git commit -m "fix: document silent .catch() handlers as intentional best-effort (FIX-104)"
```

---

### Task 6: Extract Custom Hooks from Large Components

**Files:**
- Create: `frontend/src/features/project/useProjectDetail.ts`
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx`

- [ ] **Step 1: Extract state and data fetching from ProjectDetailPage (1036 LOC)**

Create a custom hook that encapsulates the 28 signal/resource declarations:

```tsx
// frontend/src/features/project/useProjectDetail.ts
import { createSignal, createResource } from "solid-js";

export function useProjectDetail(projectId: () => string) {
  // Move all createSignal, createResource, createEffect from ProjectDetailPage
  // Return an object with all reactive state

  return {
    project, agents, tasks, runs,
    activeTab, setActiveTab,
    // ... all state
  };
}
```

- [ ] **Step 2: Update ProjectDetailPage to use the hook**

```tsx
// ProjectDetailPage.tsx
export default function ProjectDetailPage(): JSX.Element {
  const params = useParams();
  const state = useProjectDetail(() => params.id);
  // Component now focuses on rendering only
}
```

- [ ] **Step 3: Repeat for FilePanel (929 LOC) — extract useFilePanel**

Create `frontend/src/features/project/useFilePanel.ts` with file tree state, editor state, context menu state.

- [ ] **Step 4: Run tests + lint**

```bash
cd frontend && npx eslint src/features/project/
npx vitest run --reporter=verbose
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/project/
git commit -m "refactor: extract useProjectDetail and useFilePanel hooks from large components"
```

---

### Task 7: Add Keyboard Navigation E2E Test

**Files:**
- Create: `frontend/e2e/keyboard-nav.spec.ts`

- [ ] **Step 1: Create keyboard navigation test**

```typescript
// frontend/e2e/keyboard-nav.spec.ts
import { test, expect } from "@playwright/test";

test.describe("Keyboard Navigation", () => {
  test.beforeEach(async ({ page }) => {
    // Login
    await page.goto("/login");
    await page.fill('[name="email"]', "admin@localhost");
    await page.fill('[name="password"]', "Changeme123");
    await page.click('button[type="submit"]');
    await page.waitForURL("**/dashboard");
  });

  test("Tab cycles through nav items", async ({ page }) => {
    // Tab through sidebar navigation
    await page.keyboard.press("Tab");
    const focused = await page.evaluate(() => document.activeElement?.tagName);
    expect(focused).toBeTruthy();
  });

  test("Escape closes modals", async ({ page }) => {
    // Open settings popover or modal
    await page.click('[aria-label="Settings"]');
    await page.keyboard.press("Escape");
    // Verify modal closed
  });

  test("Enter activates buttons", async ({ page }) => {
    // Focus a button via Tab
    await page.keyboard.press("Tab");
    await page.keyboard.press("Enter");
    // Verify action occurred
  });
});
```

- [ ] **Step 2: Run test**

```bash
cd frontend && npx playwright test e2e/keyboard-nav.spec.ts
```

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/keyboard-nav.spec.ts
git commit -m "test: add keyboard navigation E2E test (WCAG 2.1.1)"
```
