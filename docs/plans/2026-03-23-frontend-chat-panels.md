# Frontend Chat Panels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire DiffSummaryModal and RewindTimeline into the chat UI, connecting them to /diff and /rewind slash commands.

**Architecture:** Add persistent sessionDiffs and stepHistory accumulators in useChatAGUI, wire the existing command executor modal actions to open the components, add discoverability buttons in ChatMessages and ChatHeader.

**Tech Stack:** SolidJS, TypeScript, Tailwind CSS

---

### Task 1: Extend ChatAGUIState interface

**Files:**
- Modify: `frontend/src/features/project/chatPanelTypes.ts`

- [ ] **Step 1: Add sessionDiffs and stepHistory accessors**

Add before closing `}` of ChatAGUIState:

```typescript
sessionDiffs: () => Array<{
  path: string;
  hunks: {
    old_start: number; old_lines: number;
    new_start: number; new_lines: number;
    old_content: string; new_content: string;
  }[];
}>;
stepHistory: () => Array<{
  stepId: string;
  name: string;
  timestamp: string;
  status: "running" | "completed" | "failed" | "cancelled" | "skipped";
}>;
```

---

### Task 2: Add accumulators in useChatAGUI

**Files:**
- Modify: `frontend/src/features/project/useChatAGUI.ts`

- [ ] **Step 1: Add sessionDiffs signal and accumulator**

```typescript
interface SessionDiffEntry {
  path: string;
  hunks: { old_start: number; old_lines: number; new_start: number; new_lines: number; old_content: string; new_content: string; }[];
}
const [sessionDiffs, setSessionDiffs] = createSignal<SessionDiffEntry[]>([]);
```

In `agui.tool_result` handler, when `diff` is present:
```typescript
if (diff) {
  setSessionDiffs((prev) => {
    const idx = prev.findIndex((d) => d.path === diff.path);
    if (idx >= 0) { const next = [...prev]; next[idx] = diff; return next; }
    return [...prev, diff];
  });
}
```

- [ ] **Step 2: Add stepHistory signal and accumulator**

```typescript
interface StepHistoryEntry {
  stepId: string; name: string; timestamp: string;
  status: "running" | "completed" | "failed" | "cancelled" | "skipped";
}
const [stepHistory, setStepHistory] = createSignal<StepHistoryEntry[]>([]);
```

In `step_started` handler:
```typescript
setStepHistory((prev) => [...prev, { stepId, name, status: "running", timestamp: new Date().toISOString() }]);
```

In `step_finished` handler:
```typescript
setStepHistory((prev) => prev.map((s) => (s.stepId === stepId ? { ...s, status } : s)));
```

- [ ] **Step 3: Export both from return object**

```typescript
sessionDiffs, stepHistory,
```

---

### Task 3: Wire components into ChatPanel

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx`

- [ ] **Step 1: Add imports**

```typescript
import DiffSummaryModal from "../chat/DiffSummaryModal";
import RewindTimeline from "../chat/RewindTimeline";
```

- [ ] **Step 2: Add visibility signals**

```typescript
const [showDiffSummary, setShowDiffSummary] = createSignal(false);
const [showRewindTimeline, setShowRewindTimeline] = createSignal(false);
```

- [ ] **Step 3: Replace modal case in slash command handler**

```typescript
case "modal":
  if (result.action === "show_diff") {
    setShowDiffSummary(true);
  } else if (result.action === "show_rewind") {
    setShowRewindTimeline(true);
  } else {
    toast("info", `Action: ${result.action ?? "modal"}`);
  }
  break;
```

- [ ] **Step 4: Render DiffSummaryModal**

```tsx
<DiffSummaryModal
  diffs={agui.sessionDiffs()}
  visible={showDiffSummary()}
  onClose={() => setShowDiffSummary(false)}
/>
```

- [ ] **Step 5: Render RewindTimeline**

```tsx
<RewindTimeline
  conversationId={activeConversation() ?? ""}
  steps={agui.stepHistory()}
  visible={showRewindTimeline()}
  onClose={() => setShowRewindTimeline(false)}
  onRewind={(stepId, mode) => {
    const convId = activeConversation();
    if (!convId) return;
    setShowRewindTimeline(false);
    void api.conversations.rewind(convId, { to_event_id: stepId })
      .then(() => { refetchMessages(); refetchSession(); toast("success", t("session.rewindSuccess")); })
      .catch(() => { toast("error", t("session.rewindFailed")); });
  }}
/>
```

---

### Task 4: Add "View Changes" button in ChatMessages

**Files:**
- Modify: `frontend/src/features/project/ChatMessages.tsx`

- [ ] **Step 1: Extend ChatMessagesProps**

Add `sessionDiffs` and `onShowDiffSummary` props.

- [ ] **Step 2: Add button after tool calls section**

```tsx
<Show when={!props.agentRunning() && props.sessionDiffs().length > 0}>
  <div class="flex justify-start ml-2 mt-1">
    <button type="button"
      class="inline-flex items-center gap-1.5 rounded-cf-sm border border-cf-border px-2.5 py-1 text-xs font-medium text-cf-text-secondary hover:bg-cf-bg-inset transition-colors"
      onClick={props.onShowDiffSummary}>
      View {props.sessionDiffs().length} file change{props.sessionDiffs().length !== 1 ? "s" : ""}
    </button>
  </div>
</Show>
```

- [ ] **Step 3: Pass props from ChatPanel**

```typescript
sessionDiffs={agui.sessionDiffs}
onShowDiffSummary={() => setShowDiffSummary(true)}
```

---

### Task 5: Add "Timeline" button in ChatHeader

**Files:**
- Modify: `frontend/src/features/project/ChatHeader.tsx`

- [ ] **Step 1: Extend ChatHeaderProps**

Add `onShowRewindTimeline` and `hasStepHistory` props.

- [ ] **Step 2: Add Timeline button next to existing Rewind button**

```tsx
<Show when={props.hasStepHistory}>
  <Button variant="secondary" size="sm" class="text-xs px-2 py-0.5"
    onClick={props.onShowRewindTimeline}>
    Timeline
  </Button>
</Show>
```

- [ ] **Step 3: Pass props from ChatPanel**

```typescript
onShowRewindTimeline={() => setShowRewindTimeline(true)}
hasStepHistory={agui.stepHistory().length > 0}
```

---

### Task 6: Add /diff to command autocomplete

**Files:**
- Modify: `frontend/src/features/chat/commandStore.ts`

- [ ] **Step 1: Add diff to FALLBACK_COMMANDS**

```typescript
{ id: "diff", label: "diff", category: "command" },
```

---

### Task 7: Verify and commit

- [ ] **Step 1: Build check**

```bash
cd frontend && npm run build 2>&1 | tail -20
```

- [ ] **Step 2: Type check**

```bash
cd frontend && npx tsc --noEmit 2>&1 | tail -20
```

- [ ] **Step 3: Commit**

```
feat: wire DiffSummaryModal and RewindTimeline into chat UI

Connect /diff and /rewind commands to their respective components.
Add sessionDiffs and stepHistory accumulators in useChatAGUI that
persist across runs. Add "View Changes" button in ChatMessages and
"Timeline" button in ChatHeader for discoverability.
```
