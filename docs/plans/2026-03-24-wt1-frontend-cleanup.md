# WT1: Frontend Cleanup — Icon Extraction & Async Fallback Fix

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract duplicated inline SVGs from ChatPanel into the shared icon library and fix the empty async fallback in useCRUDForm.

**Architecture:** Extract ChatPanel's 2 inline SVGs into `frontend/src/ui/icons/ChatIcons.tsx` following the established icon pattern (PascalCase, `size?`/`class?` props, `currentColor`). Fix useCRUDForm's empty async handler to log a dev warning instead of silently no-op'ing.

**Tech Stack:** SolidJS, TypeScript, Tailwind CSS

---

### Task 1: Fix useCRUDForm empty async fallback

**Files:**
- Modify: `frontend/src/hooks/useCRUDForm.ts:51`

**Context:** `useConfirmAction` requires a non-optional `action` parameter. When `onDelete` is undefined, the current code passes `async () => {}` which silently swallows any accidental delete call. The fix: pass a function that logs a dev warning.

- [ ] **Step 1: Fix the empty async fallback**

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
      if (import.meta.env.DEV) {
        console.warn("useCRUDForm: onDelete called but no handler was provided");
      }
    }),
);
```

- [ ] **Step 2: Verify no type errors**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors related to useCRUDForm.ts

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/useCRUDForm.ts
git commit -m "fix: replace silent no-op with dev warning in useCRUDForm onDelete fallback"
```

---

### Task 2: Extract ChatPanel inline SVGs into icon components

**Files:**
- Create: `frontend/src/ui/icons/ChatIcons.tsx`
- Modify: `frontend/src/features/project/ChatPanel.tsx:390-420`

**Context:** ChatPanel has 2 inline SVGs: an attach/paperclip icon (line 390) and a canvas/pencil icon (line 412). The existing icon pattern in `NavIcons.tsx` and `EmptyStateIcons.tsx` uses: `function IconName(props: { size?: number; class?: string }): JSX.Element` with `currentColor` fill and reactive `size()` accessor.

- [ ] **Step 1: Create ChatIcons.tsx with extracted icons**

```typescript
// frontend/src/ui/icons/ChatIcons.tsx
import type { JSX } from "solid-js";

interface IconProps {
  size?: number;
  class?: string;
}

/** Paperclip/attach icon (Heroicons 20x20 solid). */
export function AttachIcon(props: IconProps): JSX.Element {
  const size = () => props.size ?? 16;
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 20 20"
      fill="currentColor"
      width={size()}
      height={size()}
      class={props.class}
      aria-hidden="true"
    >
      <path
        fill-rule="evenodd"
        d="M15.621 4.379a3 3 0 0 0-4.242 0l-7 7a3 3 0 0 0 4.241 4.243l7-7a1.5 1.5 0 0 0-2.121-2.122l-7 7a.5.5 0 1 1-.707-.707l7-7a3 3 0 0 1 4.242 4.243l-7 7a5 5 0 0 1-7.071-7.071l7-7a1 1 0 0 1 1.414 1.414l-7 7a3 3 0 1 0 4.243 4.243l7-7a1.5 1.5 0 0 0 0-2.122Z"
        clip-rule="evenodd"
      />
    </svg>
  );
}

/** Pencil/canvas icon (Heroicons 20x20 solid). */
export function CanvasIcon(props: IconProps): JSX.Element {
  const size = () => props.size ?? 16;
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 20 20"
      fill="currentColor"
      width={size()}
      height={size()}
      class={props.class}
      aria-hidden="true"
    >
      <path d="M15.993 1.385a1.87 1.87 0 0 1 2.623 2.622l-4.03 4.031-2.622-2.623 4.03-4.03ZM3.74 12.104l7.217-7.216 2.623 2.622-7.217 7.217H3.74v-2.623Z" />
      <path
        fill-rule="evenodd"
        d="M.837 17.374a.75.75 0 0 1 .545-.916l4.5-1.07a.75.75 0 1 1 .346 1.46l-4.5 1.07a.75.75 0 0 1-.891-.544Z"
        clip-rule="evenodd"
      />
    </svg>
  );
}
```

- [ ] **Step 2: Read ChatPanel.tsx canvas icon path (line 412-420) to verify second path element**

Read `frontend/src/features/project/ChatPanel.tsx` lines 412-425 to capture the exact second `<path>` of the canvas icon. Update `ChatIcons.tsx` if the path differs from above.

- [ ] **Step 3: Replace inline SVGs in ChatPanel with icon components**

In `ChatPanel.tsx`, add import at the top (near other imports):
```typescript
import { AttachIcon, CanvasIcon } from "~/ui/icons/ChatIcons";
```

Replace the attach button SVG (~lines 390-401) with:
```tsx
<AttachIcon class="w-4 h-4" />
```

Replace the canvas button SVG (~lines 412-425) with:
```tsx
<CanvasIcon class="w-4 h-4" />
```

- [ ] **Step 4: Remove the FIX-106 TODO comment from ChatPanel.tsx**

Delete lines 1-3 (the TODO comment about icon duplication):
```typescript
// TODO: FIX-106: Inline SVG icons are duplicated across ChatPanel and other
// components. Extract shared SVG icons into a reusable icon component library
// (e.g., frontend/src/ui/icons/).
```

- [ ] **Step 5: Verify no type errors**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add frontend/src/ui/icons/ChatIcons.tsx frontend/src/features/project/ChatPanel.tsx
git commit -m "refactor(FIX-106): extract ChatPanel inline SVGs into ChatIcons component library"
```

---

### Task 3: Update docs/todo.md

**Files:**
- Modify: `docs/todo.md`

- [ ] **Step 1: Mark FIX-106 as completed in docs/todo.md**

Find the FIX-106 entry and mark it `[x]` with date `2026-03-24`.

- [ ] **Step 2: Commit**

```bash
git add docs/todo.md
git commit -m "docs: mark FIX-106 icon extraction as completed"
```
