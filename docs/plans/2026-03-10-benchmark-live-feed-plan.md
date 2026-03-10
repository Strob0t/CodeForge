# Benchmark Live Feed Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a real-time structured event feed to expanded benchmark run cards so users can see the agent is alive and working.

**Architecture:** The Python agent loop already publishes trajectory events (`agent.step_done`, `agent.tool_called`, `agent.finished`) to NATS (`runs.trajectory.event`). The Go runtime service subscribes and broadcasts them as `trajectory.event` via WebSocket. Currently the broadcast payload is minimal (just `run_id`, `event_type`, `tool_name`, `model`). We need to: (1) enrich the broadcast with more fields (cost, output snippet, input snippet, success, step count), (2) build a new frontend component that collects these events per run and displays them in a virtualized, auto-scrolling feed with feature-level accordions.

**Tech Stack:** SolidJS, @tanstack/solid-virtual, Tailwind CSS, existing WebSocket infrastructure

---

### Task 1: Install @tanstack/solid-virtual

**Files:**
- Modify: `frontend/package.json`

**Step 1: Install the dependency**

Run: `cd /workspaces/CodeForge/frontend && npm install @tanstack/solid-virtual`

**Step 2: Verify installation**

Run: `cd /workspaces/CodeForge/frontend && node -e "require('@tanstack/solid-virtual')"`
Expected: No error

**Step 3: Commit**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "chore(frontend): add @tanstack/solid-virtual for benchmark live feed"
```

---

### Task 2: Enrich trajectory event broadcast in Go

The current broadcast at `internal/service/runtime.go:647` only sends `run_id`, `event_type`, `tool_name`, `model`. We need to include `cost_usd`, `input`, `output`, `success`, `step`, `tokens_in`, `tokens_out`, and `project_id` so the frontend has enough data to render a useful feed.

**Files:**
- Modify: `internal/service/runtime.go:615-652` (trajectory subscription handler)
- Modify: `internal/adapter/ws/events.go` (add TrajectoryEventPayload struct)

**Step 1: Add TrajectoryEventPayload struct to events.go**

In `internal/adapter/ws/events.go`, add after the existing event structs:

```go
// TrajectoryEventPayload is broadcast with enriched trajectory data for live feeds.
type TrajectoryEventPayload struct {
	RunID     string  `json:"run_id"`
	ProjectID string  `json:"project_id"`
	EventType string  `json:"event_type"`
	ToolName  string  `json:"tool_name,omitempty"`
	Model     string  `json:"model,omitempty"`
	Input     string  `json:"input,omitempty"`
	Output    string  `json:"output,omitempty"`
	Success   *bool   `json:"success,omitempty"`
	Step      int     `json:"step,omitempty"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
	TokensIn  int64   `json:"tokens_in,omitempty"`
	TokensOut int64   `json:"tokens_out,omitempty"`
}
```

**Step 2: Update the trajectory subscription handler in runtime.go**

Replace the payload struct and broadcast at `runtime.go:615-652` to parse and forward the enriched fields:

```go
// In the Subscribe callback, expand the parsed struct:
var payload struct {
	EventType string  `json:"event_type"`
	RunID     string  `json:"run_id"`
	ProjectID string  `json:"project_id"`
	ToolName  string  `json:"tool_name,omitempty"`
	Model     string  `json:"model,omitempty"`
	Input     string  `json:"input,omitempty"`
	Output    string  `json:"output,omitempty"`
	Success   *bool   `json:"success,omitempty"`
	Step      int     `json:"step,omitempty"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
	TokensIn  int64   `json:"tokens_in,omitempty"`
	TokensOut int64   `json:"tokens_out,omitempty"`
}

// Replace the BroadcastEvent call:
s.hub.BroadcastEvent(msgCtx, ws.EventTrajectoryEvent, ws.TrajectoryEventPayload{
	RunID:     payload.RunID,
	ProjectID: payload.ProjectID,
	EventType: payload.EventType,
	ToolName:  payload.ToolName,
	Model:     payload.Model,
	Input:     payload.Input,
	Output:    payload.Output,
	Success:   payload.Success,
	Step:      payload.Step,
	CostUSD:   payload.CostUSD,
	TokensIn:  payload.TokensIn,
	TokensOut: payload.TokensOut,
})
```

**Step 3: Run Go tests**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: Compiles clean

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/ws/... -v -count=1`
Expected: All pass

**Step 4: Commit**

```bash
git add internal/service/runtime.go internal/adapter/ws/events.go
git commit -m "feat(ws): enrich trajectory event broadcast with cost, input, output fields"
```

---

### Task 3: Add TypeScript types for live feed events

**Files:**
- Modify: `frontend/src/api/types.ts` (add LiveFeedEvent type)

**Step 1: Add types at the end of the benchmark section**

After the existing benchmark types, add:

```typescript
/** A single event in the benchmark live feed, collected from WebSocket. */
export interface LiveFeedEvent {
  id: string;
  timestamp: number;
  run_id: string;
  project_id: string;
  event_type: "agent.step_done" | "agent.tool_called" | "agent.finished" | string;
  tool_name?: string;
  model?: string;
  input?: string;
  output?: string;
  success?: boolean;
  step?: number;
  cost_usd?: number;
  tokens_in?: number;
  tokens_out?: number;
}

/** Benchmark-specific progress tracked from WS events. */
export interface BenchmarkLiveProgress {
  completed_tasks: number;
  total_tasks: number;
  avg_score: number;
  total_cost_usd: number;
}
```

**Step 2: Verify types compile**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend/src/api/types.ts
git commit -m "feat(frontend): add LiveFeedEvent and BenchmarkLiveProgress types"
```

---

### Task 4: Build BenchmarkLiveFeed component

This is the main new component. It subscribes to WebSocket events, collects them per run, and renders a virtualized auto-scrolling feed with feature accordions.

**Files:**
- Create: `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx`

**Step 1: Create the component**

The component receives the `run_id` as a prop, subscribes to `trajectory.event` and `benchmark.task.completed` / `benchmark.run.progress` WebSocket messages, and renders a virtualized list.

Key implementation details:
- `createSignal<LiveFeedEvent[]>([])` to collect events, filtered by `run_id`
- `createSignal<BenchmarkLiveProgress | null>(null)` for aggregate progress
- `createVirtualizer` from `@tanstack/solid-virtual` for the event list
- Auto-scroll logic: track `userScrolledUp` state, show "scroll to bottom" button
- Each event rendered as a compact row with icon (based on `event_type`), tool name/model, output snippet, cost
- Elapsed timer using `setInterval` ticking every second
- Progress bar header: `completed_tasks / total_tasks (XX%)`
- `onCleanup` to unsubscribe from WebSocket

```typescript
import { createEffect, createMemo, createSignal, For, on, onCleanup, onMount, Show } from "solid-js";
import { createVirtualizer } from "@tanstack/solid-virtual";
import type { BenchmarkLiveProgress, LiveFeedEvent } from "~/api/types";
import { useWebSocket } from "~/components/WebSocketProvider";
import { Badge, CostDisplay } from "~/ui";

interface BenchmarkLiveFeedProps {
  runId: string;
  startedAt: string;
}
```

The component structure:

1. **Progress header**: progress bar + `completed/total (%)` + elapsed timer + cost
2. **Event feed**: virtualized list of `LiveFeedEvent` items
3. **Event row renderer**: icon + timestamp + description + cost (if any)
4. **Scroll-to-bottom button**: appears when user scrolls up

Event row rendering by `event_type`:
- `agent.tool_called`: wrench icon + `tool_name` + truncated output (first 120 chars) + success/fail badge
- `agent.step_done`: chart icon + `model` + tokens + cost
- `agent.finished`: checkmark icon + "Agent finished" + total cost
- Other: info icon + raw `event_type`

Auto-scroll implementation:
- Track container `scrollTop` vs `scrollHeight - clientHeight`
- If difference < 50px, consider "at bottom" -> auto-scroll enabled
- On new events, if auto-scroll enabled, call `scrollToIndex(events.length - 1)`
- "Scroll to bottom" button appears when not at bottom, clicking it re-enables auto-scroll

**Step 2: Verify it compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx
git commit -m "feat(frontend): add BenchmarkLiveFeed component with virtualized auto-scroll"
```

---

### Task 5: Integrate BenchmarkLiveFeed into BenchmarkPage

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx:423-434` (replace pulse bar with live feed)

**Step 1: Import and add the live feed**

Add import at top of BenchmarkPage.tsx:
```typescript
import { BenchmarkLiveFeed } from "./BenchmarkLiveFeed";
```

**Step 2: Replace the progress bar section**

Replace lines 423-434 (the `Show when={run.status === "running"}` block with the pulse bar) with:

```tsx
<Show when={run.status === "running"}>
  <BenchmarkLiveFeed runId={run.id} startedAt={run.created_at} />
</Show>
```

Also add it to the expanded results section (lines 453-475). When the run is selected AND running, show the live feed above the detail table:

```tsx
<Show when={selectedRun() === run.id}>
  <Show when={run.status === "running"}>
    <BenchmarkLiveFeed runId={run.id} startedAt={run.created_at} />
  </Show>
  <BenchmarkRunDetail ... />
  ...
</Show>
```

Remove the duplicate progress bar since the live feed now contains its own progress indicator.

**Step 3: Verify it compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/features/benchmarks/BenchmarkPage.tsx
git commit -m "feat(frontend): integrate BenchmarkLiveFeed into expanded run cards"
```

---

### Task 6: Add auto-agent feature accordion support

The auto-agent tracks features (via `autoagent.status` events with `current_feature_id`). When a benchmark is run via auto-agent, the live feed should show feature-level grouping.

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx`

**Step 1: Subscribe to autoagent.status events**

Add a WebSocket listener for `autoagent.status` messages. Track `current_feature_id` and feature names from `benchmark.task.started` / `benchmark.task.completed` events (which carry `task_name` and `index`/`total`).

Build a feature list from task events:
```typescript
interface FeatureEntry {
  id: string;
  name: string;
  status: "pending" | "running" | "completed" | "failed";
  events: LiveFeedEvent[];
  startedAt?: number;
  cost: number;
  step: number;
}
```

**Step 2: Render feature accordions**

When features are detected (more than 1 task name), render collapsible accordion sections. Each accordion:
- Header: feature name + status icon (spinner/check/X) + elapsed time + cost + step counter
- Body: virtualized event feed (same as before, but filtered to this feature's events)

When only 1 feature or no feature grouping, fall back to flat feed (same as Task 4).

**Step 3: Verify it compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx
git commit -m "feat(frontend): add feature accordion grouping to benchmark live feed"
```

---

### Task 7: Style and polish

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx`

**Step 1: Theme-consistent styling**

Ensure all colors use the existing Tailwind theme classes:
- Background: `bg-cf-bg-secondary` / `dark:bg-cf-bg-secondary`
- Text: `text-cf-text-primary` / `text-cf-text-secondary`
- Borders: `border-cf-border`
- Accent: `text-cf-accent` for active items
- Use existing `Badge`, `CostDisplay` components from `~/ui`

Check against existing components (ChatPanel, WarRoom, TrajectoryPanel) for consistent patterns.

**Step 2: Event row icons**

Use inline SVG or Unicode symbols consistent with the project's "no icon library" principle:
- Tool call: Unicode wrench or `>_` monospace
- Step done: chart bar Unicode
- Error/retry: warning triangle Unicode
- Agent finished: checkmark Unicode
- Spinner for running items: CSS `animate-spin` on a small circle

**Step 3: Scroll-to-bottom button styling**

Position as `sticky bottom-2` inside the feed container, centered, with subtle bg + shadow.

**Step 4: Verify visual consistency**

Run dev server and visually inspect:
```bash
cd /workspaces/CodeForge/frontend && npm run dev
```

**Step 5: Commit**

```bash
git add frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx
git commit -m "style(frontend): polish benchmark live feed theme and icons"
```

---

### Task 8: Run pre-commit and final verification

**Step 1: Run pre-commit**

Run: `cd /workspaces/CodeForge && pre-commit run --all-files`
Expected: All pass

**Step 2: Run TypeScript check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 3: Run Go tests**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/ws/... ./internal/service/... -count=1 -timeout 60s`
Expected: All pass

**Step 4: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: pre-commit and lint fixes for benchmark live feed"
```

**Step 5: Push**

```bash
git push
```
