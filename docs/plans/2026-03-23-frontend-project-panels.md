# Frontend Project Panels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire 14 orphaned project components into ProjectDetailPage as new tab panels.

**Architecture:** Extend the existing LeftTab type union and dropdown selector in ProjectDetailPage.tsx, add 6 new tab groups (agents, code, retrieval, plans, tasks, policy), wire WS events for LiveOutput/MultiTerminal/CostBreakdown data.

**Tech Stack:** SolidJS, TypeScript, Tailwind CSS

---

### Task 1: Capture resource signals and extend LeftTab type

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx`

- [ ] **Step 1: Capture tasks/agents resource signals**

Change destructuring (lines 128-144) from `[, { refetch }]` to `[tasks, { refetch }]` and `[agents, { refetch }]` to expose the data signals.

- [ ] **Step 2: Extend LeftTab type union**

```typescript
type LeftTab =
  | "roadmap" | "featuremap" | "files" | "warroom"
  | "goals" | "audit" | "sessions" | "trajectory" | "boundaries"
  | "agents" | "code" | "retrieval" | "plans" | "tasks" | "policy";
```

- [ ] **Step 3: Add WS state signals for LiveOutput, MultiTerminal, CostBreakdown**

```typescript
import type { OutputLine } from "./LiveOutput";
import type { AgentTerminal } from "./MultiTerminal";

const [liveOutputTaskId, setLiveOutputTaskId] = createSignal<string | null>(null);
const [liveOutputLines, setLiveOutputLines] = createSignal<OutputLine[]>([]);
const [agentTerminals, setAgentTerminals] = createSignal<AgentTerminal[]>([]);
const [activeRunCost, setActiveRunCost] = createSignal<{
  costUsd: number; tokensIn: number; tokensOut: number; steps: number; model?: string;
} | null>(null);
```

---

### Task 2: Add imports and dropdown options

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx`

- [ ] **Step 1: Add 14 component imports (alphabetical)**

```typescript
import AgentNetwork from "./AgentNetwork";
import AgentPanel from "./AgentPanel";
import ArchitectureGraph from "./ArchitectureGraph";
import CostBreakdown from "./CostBreakdown";
import LiveOutput from "./LiveOutput";
import LSPPanel from "./LSPPanel";
import MultiTerminal from "./MultiTerminal";
import PlanPanel from "./PlanPanel";
import PolicyPanel from "./PolicyPanel";
import RepoMapPanel from "./RepoMapPanel";
import RetrievalPanel from "./RetrievalPanel";
import RunPanel from "./RunPanel";
import SearchSimulator from "./SearchSimulator";
import TaskPanel from "./TaskPanel";
```

- [ ] **Step 2: Add 6 new options to dropdown**

```tsx
<option value="tasks">{t("detail.tab.tasks")}</option>
<option value="plans">{t("detail.tab.plans")}</option>
<option value="agents">{t("detail.tab.agents")}</option>
<option value="code">{t("detail.tab.code")}</option>
<option value="retrieval">{t("detail.tab.retrieval")}</option>
<option value="policy">{t("detail.tab.policy")}</option>
```

- [ ] **Step 3: Update overflow style guard**

Add new tab values to the no-scroll list:
```typescript
["featuremap", "files", "warroom", "goals", "audit", "sessions", "trajectory",
 "boundaries", "agents", "code", "retrieval", "plans", "tasks", "policy"].includes(leftTab())
```

---

### Task 3: Wire WS event handlers

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx`

- [ ] **Step 1: Wire task.output WS event for LiveOutput + MultiTerminal**

In the `case "task.output":` handler, feed liveOutputLines and agentTerminals signals.

- [ ] **Step 2: Wire run.status WS event for CostBreakdown**

In the `case "run.status":` handler, extract cost data into activeRunCost signal.

---

### Task 4: Add 6 Show blocks for new tabs

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx`

- [ ] **Step 1: Agents tab** (AgentPanel + RunPanel + AgentNetwork + LiveOutput + MultiTerminal + CostBreakdown)

```tsx
<Show when={leftTab() === "agents"}>
  <ErrorBoundary fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}>
    <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4 space-y-4">
      <AgentPanel projectId={params.id} tasks={tasks() ?? []} onError={setError} />
      <RunPanel projectId={params.id} tasks={tasks() ?? []} agents={agents() ?? []} onError={setError} />
      <AgentNetwork projectId={params.id} />
      <Show when={liveOutputLines().length > 0}>
        <LiveOutput taskId={liveOutputTaskId()} lines={liveOutputLines()} />
      </Show>
      <Show when={agentTerminals().length > 0}>
        <MultiTerminal terminals={agentTerminals()} />
      </Show>
      <Show when={activeRunCost()}>
        {(cost) => (
          <CostBreakdown costUsd={cost().costUsd} tokensIn={cost().tokensIn}
            tokensOut={cost().tokensOut} steps={cost().steps} model={cost().model} />
        )}
      </Show>
    </div>
  </ErrorBoundary>
</Show>
```

- [ ] **Step 2: Code tab** (RepoMapPanel + ArchitectureGraph + LSPPanel)

```tsx
<Show when={leftTab() === "code"}>
  <ErrorBoundary fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}>
    <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4 space-y-4">
      <RepoMapPanel projectId={params.id} />
      <ArchitectureGraph projectId={params.id} />
      <LSPPanel projectId={params.id} />
    </div>
  </ErrorBoundary>
</Show>
```

- [ ] **Step 3: Retrieval tab** (RetrievalPanel + SearchSimulator)

```tsx
<Show when={leftTab() === "retrieval"}>
  <ErrorBoundary fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}>
    <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4 space-y-4">
      <RetrievalPanel projectId={params.id} />
      <SearchSimulator projectId={params.id} />
    </div>
  </ErrorBoundary>
</Show>
```

- [ ] **Step 4: Plans tab**

```tsx
<Show when={leftTab() === "plans"}>
  <ErrorBoundary fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}>
    <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
      <PlanPanel projectId={params.id} tasks={tasks() ?? []} agents={agents() ?? []} onError={setError} />
    </div>
  </ErrorBoundary>
</Show>
```

- [ ] **Step 5: Tasks tab**

```tsx
<Show when={leftTab() === "tasks"}>
  <ErrorBoundary fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}>
    <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
      <TaskPanel projectId={params.id} tasks={tasks() ?? []} onRefetch={refetchTasks} onError={setError} />
    </div>
  </ErrorBoundary>
</Show>
```

- [ ] **Step 6: Policy tab**

```tsx
<Show when={leftTab() === "policy"}>
  <ErrorBoundary fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}>
    <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
      <PolicyPanel projectId={params.id} onError={setError} />
    </div>
  </ErrorBoundary>
</Show>
```

---

### Task 5: Add i18n keys

**Files:**
- Modify: `frontend/src/i18n/en.ts`
- Modify: `frontend/src/i18n/locales/de.ts`

- [ ] **Step 1: Add English keys**

```typescript
"detail.tab.plans": "Plans",
"detail.tab.code": "Code Intelligence",
"detail.tab.retrieval": "Retrieval",
"detail.tab.policy": "Policy",
```

- [ ] **Step 2: Add German keys**

```typescript
"detail.tab.plans": "Plaene",
"detail.tab.code": "Code-Intelligenz",
"detail.tab.retrieval": "Retrieval",
"detail.tab.policy": "Richtlinien",
```

---

### Task 6: Verify and commit

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
feat: wire 14 orphaned project panels into ProjectDetailPage

Add 6 new tab groups (agents, code, retrieval, plans, tasks, policy)
to the ProjectDetailPage left panel. Wire LiveOutput, MultiTerminal,
and CostBreakdown to WebSocket events for real-time data. All 14
previously orphaned components are now accessible via the tab dropdown.
```
