# ChatPanel Decomposition — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose `ChatPanel.tsx` (1141 lines, 27 signals) into focused sub-components and a custom hook, each with single responsibility.

**Architecture:** Extract AG-UI event handling into a `useChatAGUI` hook. Extract the message list rendering into `ChatMessages`. Extract the header bar into `ChatHeader`. Keep `ChatPanel` as a thin orchestrator that composes these pieces. All existing tests and E2E specs must continue to pass.

**Tech Stack:** SolidJS, TypeScript, Vitest. Run tests: `cd frontend && npm test`

---

## File Structure

| File | Responsibility | Lines (est.) |
|------|---------------|-------------|
| `ChatPanel.tsx` | Thin orchestrator: composes header, messages, input, footer | ~250 |
| `useChatAGUI.ts` (NEW) | AG-UI event subscriptions, streaming state, tool calls, plan steps, permissions, actions | ~250 |
| `ChatHeader.tsx` (NEW) | Header bar: conversation selector, new conversation, session badges, fork/rewind/resume buttons | ~200 |
| `ChatMessages.tsx` (NEW) | Message list rendering: messages, streaming, tool cards, plan steps, permissions, goals, errors | ~300 |
| `chatPanelTypes.ts` (NEW) | Shared types: `ToolCallState`, `PlanStepState`, `ChatAGUIState` | ~40 |

---

## Task 1: Extract Shared Types

**Files:**
- Create: `frontend/src/features/project/chatPanelTypes.ts`

- [ ] **Step 1: Create types file**

Extract `ToolCallState` and `PlanStepState` interfaces from `ChatPanel.tsx` (lines 61-84):

```typescript
import type { AGUIGoalProposal, AGUIPermissionRequest } from "~/api/websocket";
import type { ActionRule } from "./actionRules";

export interface ToolCallState {
  callId: string;
  name: string;
  args?: Record<string, unknown>;
  result?: string;
  status: "pending" | "running" | "completed" | "failed";
  diff?: {
    path: string;
    hunks: {
      old_start: number;
      old_lines: number;
      new_start: number;
      new_lines: number;
      old_content: string;
      new_content: string;
    }[];
  };
}

export interface PlanStepState {
  stepId: string;
  name: string;
  status: "running" | "completed" | "failed" | "cancelled" | "skipped";
}

/** All reactive state managed by the useChatAGUI hook. */
export interface ChatAGUIState {
  streamingContent: () => string;
  agentRunning: () => boolean;
  runError: () => string | null;
  toolCalls: () => ToolCallState[];
  planSteps: () => PlanStepState[];
  goalProposals: () => AGUIGoalProposal[];
  permissionRequests: () => AGUIPermissionRequest[];
  resolvedPermissions: () => Set<string>;
  actionSuggestions: () => ActionRule[];
  stepCount: () => number;
  runningCost: () => number;
  sessionModel: () => string;
  sessionCostUsd: () => number;
  sessionTokensIn: () => number;
  sessionTokensOut: () => number;
  sessionSteps: () => number;
  setResolvedPermissions: (v: Set<string>) => void;
  setPermissionRequests: (fn: (prev: AGUIPermissionRequest[]) => AGUIPermissionRequest[]) => void;
}
```

- [ ] **Step 2: Verify no circular imports**

```bash
cd frontend && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/project/chatPanelTypes.ts
git commit -m "refactor: extract ChatPanel shared types into chatPanelTypes.ts"
```

---

## Task 2: Extract useChatAGUI Hook

**Files:**
- Create: `frontend/src/features/project/useChatAGUI.ts`
- Modify: `frontend/src/features/project/ChatPanel.tsx`

- [ ] **Step 1: Create the hook file**

Extract lines 167-445 (all AG-UI event subscriptions + streaming/tool/plan/permission/action state) into `useChatAGUI.ts`:

```typescript
import { batch, createSignal, onCleanup } from "solid-js";
import type { AGUIGoalProposal, AGUIPermissionRequest } from "~/api/websocket";
import { useWebSocket } from "~/components/WebSocketProvider";
import type { ActionRule } from "./actionRules";
import { deriveActions } from "./actionRules";
import type { ChatAGUIState, PlanStepState, ToolCallState } from "./chatPanelTypes";

interface UseChatAGUIOptions {
  activeConversation: () => string | null;
  scrollToBottom: () => void;
  refetchMessages: () => Promise<void>;
  refetchSession: () => Promise<void>;
}

export function useChatAGUI(opts: UseChatAGUIOptions): ChatAGUIState {
  const { onAGUIEvent } = useWebSocket();

  // All signals from ChatPanel lines 167-201
  const [streamingContent, setStreamingContent] = createSignal("");
  const [agentRunning, setAgentRunning] = createSignal(false);
  const [runError, setRunError] = createSignal<string | null>(null);
  const [toolCalls, setToolCalls] = createSignal<ToolCallState[]>([]);
  const [planSteps, setPlanSteps] = createSignal<PlanStepState[]>([]);
  const [goalProposals, setGoalProposals] = createSignal<AGUIGoalProposal[]>([]);
  const [permissionRequests, setPermissionRequests] = createSignal<AGUIPermissionRequest[]>([]);
  const [resolvedPermissions, setResolvedPermissions] = createSignal<Set<string>>(new Set());
  const [actionSuggestions, setActionSuggestions] = createSignal<ActionRule[]>([]);
  const [stepCount, setStepCount] = createSignal(0);
  const [runningCost, setRunningCost] = createSignal(0);
  const [sessionModel, setSessionModel] = createSignal("");
  const [sessionCostUsd, setSessionCostUsd] = createSignal(0);
  const [sessionTokensIn, setSessionTokensIn] = createSignal(0);
  const [sessionTokensOut, setSessionTokensOut] = createSignal(0);
  const [sessionSteps, setSessionSteps] = createSignal(0);

  // Move ALL AG-UI event handlers (lines 261-445) here
  // ... (copy the exact event handler code from ChatPanel)

  return {
    streamingContent, agentRunning, runError,
    toolCalls, planSteps, goalProposals,
    permissionRequests, resolvedPermissions, actionSuggestions,
    stepCount, runningCost,
    sessionModel, sessionCostUsd, sessionTokensIn, sessionTokensOut, sessionSteps,
    setResolvedPermissions, setPermissionRequests,
  };
}
```

- [ ] **Step 2: Update ChatPanel.tsx to use the hook**

Replace the 27 `createSignal` calls and event handlers with:

```typescript
import { useChatAGUI } from "./useChatAGUI";

// Inside ChatPanel:
const agui = useChatAGUI({
  activeConversation,
  scrollToBottom,
  refetchMessages: () => refetchMessages(),
  refetchSession: () => refetchSession(),
});
```

Replace all `streamingContent()` with `agui.streamingContent()` etc.

- [ ] **Step 3: Run tests**

```bash
cd frontend && npm test
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/project/useChatAGUI.ts frontend/src/features/project/ChatPanel.tsx
git commit -m "refactor: extract useChatAGUI hook from ChatPanel (AG-UI events + state)"
```

---

## Task 3: Extract ChatHeader

**Files:**
- Create: `frontend/src/features/project/ChatHeader.tsx`
- Modify: `frontend/src/features/project/ChatPanel.tsx`

- [ ] **Step 1: Create ChatHeader.tsx**

Extract the header JSX (lines ~574-740) — conversation selector, new conversation button, session badges, fork/rewind/resume buttons.

Props:
```typescript
interface ChatHeaderProps {
  projectId: string;
  activeConversation: () => string | null;
  setActiveConversation: (id: string | null) => void;
  conversations: () => Conversation[] | undefined;
  refetchConversations: () => Promise<void>;
  session: () => Session | null | undefined;
  refetchSession: () => Promise<void>;
  agentRunning: () => boolean;
  showSessionHistory: () => boolean;
  setShowSessionHistory: (v: boolean) => void;
}
```

- [ ] **Step 2: Update ChatPanel.tsx to use ChatHeader**

Replace the header JSX block with `<ChatHeader {...headerProps} />`.

- [ ] **Step 3: Run tests**

```bash
cd frontend && npm test
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/project/ChatHeader.tsx frontend/src/features/project/ChatPanel.tsx
git commit -m "refactor: extract ChatHeader from ChatPanel (session controls, conversation selector)"
```

---

## Task 4: Extract ChatMessages

**Files:**
- Create: `frontend/src/features/project/ChatMessages.tsx`
- Modify: `frontend/src/features/project/ChatPanel.tsx`

- [ ] **Step 1: Create ChatMessages.tsx**

Extract the message list rendering (lines ~740-end of JSX minus the input area). This includes:
- Message loop (`<For>` over messages)
- Streaming content display
- ToolCallCard rendering
- PlanStep badges
- PermissionRequestCard
- GoalProposalCard
- Error/command output display

Props: messages, tool state from agui hook, handlers.

- [ ] **Step 2: Update ChatPanel.tsx**

Replace message list JSX with `<ChatMessages {...messageProps} />`.

- [ ] **Step 3: Run tests**

```bash
cd frontend && npm test
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/project/ChatMessages.tsx frontend/src/features/project/ChatPanel.tsx
git commit -m "refactor: extract ChatMessages from ChatPanel (message list, tool cards, streaming)"
```

---

## Task 5: Add Tests for New Components

**Files:**
- Create: `frontend/src/features/project/useChatAGUI.test.ts`
- Create: `frontend/src/features/project/ChatHeader.test.ts`
- Create: `frontend/src/features/project/ChatMessages.test.ts`

- [ ] **Step 1: Write useChatAGUI test**

```typescript
import { describe, it, expect } from "vitest";

describe("useChatAGUI", () => {
  it("should export useChatAGUI function", async () => {
    const mod = await import("./useChatAGUI");
    expect(typeof mod.useChatAGUI).toBe("function");
  });
});
```

- [ ] **Step 2: Write ChatHeader test**

```typescript
import { describe, it, expect } from "vitest";

describe("ChatHeader", () => {
  it("should export default component", async () => {
    const mod = await import("./ChatHeader");
    expect(mod.default || mod.ChatHeader).toBeDefined();
  });
});
```

- [ ] **Step 3: Write ChatMessages test**

Same pattern.

- [ ] **Step 4: Run all tests**

```bash
cd frontend && npm test
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/project/*.test.ts
git commit -m "test: unit tests for extracted ChatPanel components"
```

---

## Task 6: Verify & Cleanup

- [ ] **Step 1: Verify ChatPanel.tsx is under 300 lines**

```bash
wc -l frontend/src/features/project/ChatPanel.tsx
```
Expected: ~250 lines (down from 1141).

- [ ] **Step 2: Run full test suite**

```bash
cd frontend && npm test
```

- [ ] **Step 3: Verify no regressions in E2E (compile check)**

```bash
cd frontend && npx tsc --noEmit
```

- [ ] **Step 4: Remove old TODOs**

Remove the FIX-105 TODO comment from ChatPanel.tsx (the refactoring is done).

- [ ] **Step 5: Final commit**

```bash
git add frontend/src/features/project/
git commit -m "refactor: ChatPanel decomposition complete — 1141 LOC → 5 focused modules"
```
