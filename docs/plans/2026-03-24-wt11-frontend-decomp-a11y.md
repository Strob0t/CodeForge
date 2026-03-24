# WT-11: Frontend Decomposition Phase 2 + Accessibility — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract hooks from 3 large components (PolicyPanel, MicroagentsPage, BenchmarkPage), fix 9 accessibility gaps (missing form labels), and add aria-labels to all unlabeled inputs.

**Architecture:** Follow the existing `useProjectDetail.ts` pattern for hook extraction. Use `FormField` wrapper or `aria-label` prop for accessibility. All changes are backward-compatible.

**Tech Stack:** SolidJS, TypeScript, FormField component, axe-core

**Best Practice:**
- WCAG 2.2 SC 1.3.1: All form controls MUST have an accessible name (label, aria-label, or aria-labelledby).
- WCAG 2.2 SC 4.1.2: Interactive elements MUST have a programmatically determinable name and role.
- Placeholder text is NOT a substitute for a label (WCAG advisory: placeholder disappears on input).
- Component hooks: Extract signals + resources + handlers into `use*.ts` files when component exceeds 500 LOC.

---

### Task 1: Fix Accessibility — Missing Form Labels (9 locations)

**Files:**
- Modify: `frontend/src/features/search/SearchPage.tsx:78-84`
- Modify: `frontend/src/features/project/GoalsPanel.tsx:164,171,179`
- Modify: `frontend/src/features/project/PolicyPanel.tsx:517-533,787-829`
- Modify: `frontend/src/features/chat/ChatInput.tsx:293-302`
- Modify: `frontend/src/features/channels/ChannelInput.tsx:38-45`
- Modify: `frontend/src/features/canvas/DesignCanvas.tsx:373-394`

- [ ] **Step 1: Fix SearchPage.tsx — add aria-label to search input**

```tsx
<input
  type="text"
  aria-label={t("search.placeholder")}
  value={query()}
  ...
/>
```

- [ ] **Step 2: Fix GoalsPanel.tsx — add aria-labels to inline form (3 inputs)**

```tsx
<select aria-label={t("goals.kindLabel")} ...>
<input type="text" aria-label={t("goals.titleLabel")} .../>
<textarea aria-label={t("goals.descriptionLabel")} .../>
```

- [ ] **Step 3: Fix PolicyPanel.tsx eval section — add aria-labels (3 inputs)**

```tsx
<Input aria-label={t("policy.evalTool")} placeholder={...} .../>
<Input aria-label={t("policy.evalCommand")} placeholder={...} .../>
<Input aria-label={t("policy.evalPath")} placeholder={...} .../>
```

- [ ] **Step 4: Fix PolicyPanel.tsx rule editor — add aria-labels (7 inputs + 1 select)**

For the inline rule editor table row, add aria-labels to each field:
```tsx
<Input aria-label="Tool pattern" .../>
<Input aria-label="Sub-pattern" .../>
<select aria-label="Action" ...>
<Input aria-label="Path allow pattern" .../>
<Input aria-label="Path deny pattern" .../>
<Input aria-label="Command allow pattern" .../>
<Input aria-label="Command deny pattern" .../>
```

- [ ] **Step 5: Fix ChatInput.tsx — accept aria-label prop**

Add `ariaLabel?: string` to ChatInput props:
```tsx
interface ChatInputProps {
  // ... existing
  ariaLabel?: string;
}

<textarea
  aria-label={props.ariaLabel ?? "Chat message"}
  ...
/>
```

- [ ] **Step 6: Fix ChannelInput.tsx — add aria-label**

```tsx
<textarea aria-label="Channel message" .../>
```

- [ ] **Step 7: Fix DesignCanvas.tsx — add aria-label to SVG textarea**

```tsx
<textarea aria-label="Canvas text editing" .../>
```

- [ ] **Step 8: Run lint + type check**

```bash
cd frontend && npx tsc --noEmit && npx eslint src/
```

- [ ] **Step 9: Commit**

```bash
git add frontend/src/
git commit -m "fix: add missing aria-labels to 9 form controls (WCAG SC 1.3.1, 4.1.2)"
```

---

### Task 2: Extract usePolicyManagement Hook

**Files:**
- Create: `frontend/src/features/project/usePolicyPanel.ts`
- Modify: `frontend/src/features/project/PolicyPanel.tsx`

- [ ] **Step 1: Create usePolicyPanel hook**

Extract from PolicyPanel.tsx:
- All `createSignal`/`createResource` declarations (~17 signals)
- All handler functions (handleSelect, handleSave, handleDelete, handleEvaluate, handlePreview)
- Return object with all reactive state + handlers

```typescript
export function usePolicyPanel() {
  const [profiles, { refetch }] = createResource(() => api.policies.list());
  const [view, setView] = createSignal<"list" | "detail" | "edit">("list");
  const [selectedName, setSelectedName] = createSignal<string | null>(null);
  // ... all other signals

  const handleSave = async () => { /* ... */ };
  const handleDelete = async (name: string) => { /* ... */ };
  const handleEvaluate = async () => { /* ... */ };

  return { profiles, view, setView, selectedName, /* ... */ handleSave, handleDelete, handleEvaluate };
}
```

- [ ] **Step 2: Update PolicyPanel.tsx to use the hook**

```tsx
export default function PolicyPanel(props: { projectId: string }): JSX.Element {
  const state = usePolicyPanel();
  // Component now focuses on rendering
}
```

- [ ] **Step 3: Verify + commit**

```bash
cd frontend && npx tsc --noEmit
git add frontend/src/features/project/
git commit -m "refactor: extract usePolicyPanel hook (866 LOC -> ~600 LOC + hook)"
```

---

### Task 3: Extract useBenchmarkPage Hook

**Files:**
- Create: `frontend/src/features/benchmarks/useBenchmarkPage.ts`
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx`

- [ ] **Step 1: Create useBenchmarkPage hook**

Extract:
- Run management signals (runs, selectedRun, results)
- Live feed state (liveFeedStates map, updateRunState)
- WebSocket handler setup
- Form state (showForm, form fields)
- All handler functions

- [ ] **Step 2: Update BenchmarkPage to use the hook**

- [ ] **Step 3: Verify + commit**

```bash
cd frontend && npx tsc --noEmit
git add frontend/src/features/benchmarks/
git commit -m "refactor: extract useBenchmarkPage hook (839 LOC -> ~550 LOC + hook)"
```

---

### Task 4: Extract useMicroagentsPage Hook

**Files:**
- Create: `frontend/src/features/microagents/useMicroagentsPage.ts`
- Modify: `frontend/src/features/microagents/MicroagentsPage.tsx`

- [ ] **Step 1: Create useMicroagentsPage hook**

Extract:
- Microagent resource + CRUD state
- Skills resource + import state
- All handlers (submit, edit, delete, import)

- [ ] **Step 2: Update MicroagentsPage to use the hook**

- [ ] **Step 3: Verify + commit**

```bash
cd frontend && npx tsc --noEmit
git add frontend/src/features/microagents/
git commit -m "refactor: extract useMicroagentsPage hook (850 LOC -> ~550 LOC + hook)"
```
