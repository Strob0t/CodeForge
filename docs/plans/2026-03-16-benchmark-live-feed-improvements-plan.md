# Benchmark Live Feed Improvements — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix live feed state loss on close/reopen/page-reload and increase information density with a stats line + mini score bars.

**Architecture:** Lift live feed state from component-local signals to a `Map<runId, LiveFeedState>` in the parent `BenchmarkPage`. On mount, hydrate running runs from the trajectory + results APIs. WebSocket events update the map incrementally. `BenchmarkLiveFeed` becomes a pure presentational component receiving state as props.

**Tech Stack:** SolidJS signals/stores, TypeScript, vitest, existing REST API (`/runs/{id}/trajectory`, `/benchmarks/runs/{id}/results`), existing WebSocket events.

**Spec:** `docs/specs/2026-03-16-benchmark-live-feed-density-design.md`

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `frontend/src/features/benchmarks/liveFeedState.ts` | **Create** | Types (`LiveFeedState`, `AggregateStats`, `FeatureEntry`), pure functions (mappers, helpers, stats computation). All testable logic lives here. |
| `frontend/src/features/benchmarks/liveFeedState.test.ts` | **Create** | Unit tests for all pure functions |
| `frontend/src/api/types.ts` | **Modify** | Extend `AgentEvent` + widen `BenchmarkLiveProgress.total_tasks` |
| `frontend/src/features/benchmarks/BenchmarkPage.tsx` | **Modify** | Add `liveFeedStates` Map, move WS subscription here, add hydration effect, pass state to `BenchmarkLiveFeed` |
| `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx` | **Modify** | Convert to presentational component (props-driven), add stats line + mini score bars |

---

## Chunk 1: Types, Pure Functions & Tests

### Task 1: Extend API types

**Files:**
- Modify: `frontend/src/api/types.ts:200-210` (AgentEvent)
- Modify: `frontend/src/api/types.ts:1687-1692` (BenchmarkLiveProgress)

- [ ] **Step 1: Add optional fields to `AgentEvent`**

In `frontend/src/api/types.ts`, find the `AgentEvent` interface (around line 200) and add the 5 optional fields after `created_at`:

```typescript
export interface AgentEvent {
  id: string;
  agent_id: string;
  task_id: string;
  project_id: string;
  type: AgentEventType;
  payload: Record<string, unknown>;
  request_id?: string;
  version: number;
  created_at: string;
  // Per-tool tracking (populated for tool_called/step_done events)
  tool_name?: string;
  model?: string;
  tokens_in?: number;
  tokens_out?: number;
  cost_usd?: number;
}
```

- [ ] **Step 2: Widen `BenchmarkLiveProgress.total_tasks`**

In `frontend/src/api/types.ts`, find `BenchmarkLiveProgress` (around line 1687) and change:

```typescript
// Before:
total_tasks: number;

// After:
total_tasks: number | null;
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -30`

Expected: Compilation errors in `BenchmarkLiveFeed.tsx` where `total_tasks` is used as `number` (e.g., division). These will be fixed in Task 5. For now, confirm the type change was applied by seeing the specific error locations.

- [ ] **Step 4: Commit**

```
feat(types): extend AgentEvent with tool tracking fields, widen total_tasks
```

---

### Task 2: Create `liveFeedState.ts` with types and helper functions

**Files:**
- Create: `frontend/src/features/benchmarks/liveFeedState.ts`
- Create: `frontend/src/features/benchmarks/liveFeedState.test.ts`

- [ ] **Step 1: Write failing tests for `formatTokens`**

Create `frontend/src/features/benchmarks/liveFeedState.test.ts`:

```typescript
import { describe, expect, it } from "vitest";

import { formatTokens } from "./liveFeedState";

describe("formatTokens", () => {
  it("returns raw number below 1000", () => {
    expect(formatTokens(0)).toBe("0");
    expect(formatTokens(999)).toBe("999");
  });
  it("formats thousands with k suffix", () => {
    expect(formatTokens(1000)).toBe("1.0k");
    expect(formatTokens(1500)).toBe("1.5k");
    expect(formatTokens(24300)).toBe("24.3k");
    expect(formatTokens(999_999)).toBe("1000.0k");
  });
  it("formats millions with M suffix", () => {
    expect(formatTokens(1_000_000)).toBe("1.0M");
    expect(formatTokens(2_500_000)).toBe("2.5M");
  });
});
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `cd frontend && npx vitest run src/features/benchmarks/liveFeedState.test.ts 2>&1 | tail -10`

Expected: FAIL — `formatTokens` not found.

- [ ] **Step 3: Implement `formatTokens` + types skeleton**

Create `frontend/src/features/benchmarks/liveFeedState.ts`:

```typescript
import type {
  AgentEvent,
  BenchmarkLiveProgress,
  LiveFeedEvent,
  TrajectorySummary,
} from "~/api/types";

// Re-export for use in BenchmarkPage/BenchmarkLiveFeed
export type { LiveFeedEvent };

export const MAX_EVENTS = 5000;

// ---- Types ----

export interface FeatureEntry {
  id: string;
  name: string;
  status: "pending" | "running" | "completed" | "failed";
  events: LiveFeedEvent[];
  startedAt?: number;
  cost: number;
  step: number;
  score?: number;
}

export interface AggregateStats {
  totalTokensIn: number;
  totalTokensOut: number;
  toolCallCount: number;
  toolSuccessCount: number;
  avgScore: number;
  costPerTask: number;
}

export interface LiveFeedState {
  events: LiveFeedEvent[];
  progress: BenchmarkLiveProgress | null;
  features: Map<string, FeatureEntry>;
  stats: AggregateStats;
  hydratedFromApi: boolean;
  lastEventId: string | null;
}

// ---- Helpers ----

export function emptyStats(): AggregateStats {
  return {
    totalTokensIn: 0,
    totalTokensOut: 0,
    toolCallCount: 0,
    toolSuccessCount: 0,
    avgScore: 0,
    costPerTask: 0,
  };
}

export function emptyLiveFeedState(): LiveFeedState {
  return {
    events: [],
    progress: null,
    features: new Map(),
    stats: emptyStats(),
    hydratedFromApi: false,
    lastEventId: null,
  };
}

export function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `cd frontend && npx vitest run src/features/benchmarks/liveFeedState.test.ts 2>&1 | tail -10`

Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```
feat(benchmark): add liveFeedState types and formatTokens helper
```

---

### Task 3: Add `computeEta` with tests

**Files:**
- Modify: `frontend/src/features/benchmarks/liveFeedState.ts`
- Modify: `frontend/src/features/benchmarks/liveFeedState.test.ts`

- [ ] **Step 1: Write failing tests for `computeEta`**

Append to `liveFeedState.test.ts`:

```typescript
import { computeEta, formatTokens } from "./liveFeedState";

// ... existing formatTokens tests ...

describe("computeEta", () => {
  it("returns null when total_tasks is null", () => {
    expect(computeEta(3, null, 120)).toBeNull();
  });
  it("returns null when 0 completed", () => {
    expect(computeEta(0, 5, 120)).toBeNull();
  });
  it("returns null when all completed", () => {
    expect(computeEta(5, 5, 120)).toBeNull();
  });
  it("calculates remaining seconds", () => {
    // 3/5 done in 120s => 40s/task => 2 remaining => 80s
    expect(computeEta(3, 5, 120)).toBe(80);
  });
  it("rounds to nearest second", () => {
    // 2/3 done in 100s => 50s/task => 1 remaining => 50s
    expect(computeEta(2, 3, 100)).toBe(50);
  });
});
```

Update the import at the top to include `computeEta`.

- [ ] **Step 2: Run test — verify FAIL**

Run: `cd frontend && npx vitest run src/features/benchmarks/liveFeedState.test.ts 2>&1 | tail -10`

Expected: FAIL — `computeEta` not exported.

- [ ] **Step 3: Implement `computeEta`**

Add to `liveFeedState.ts`:

```typescript
export function computeEta(
  completed: number,
  total: number | null,
  elapsedSec: number,
): number | null {
  if (total === null || completed === 0 || completed >= total) return null;
  const secPerTask = elapsedSec / completed;
  return Math.round(secPerTask * (total - completed));
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `cd frontend && npx vitest run src/features/benchmarks/liveFeedState.test.ts 2>&1 | tail -10`

Expected: PASS (all tests).

- [ ] **Step 5: Commit**

```
feat(benchmark): add computeEta helper with tests
```

---

### Task 4: Add `agentEventToLiveFeedEvent` and `statsFromSummary` with tests

**Files:**
- Modify: `frontend/src/features/benchmarks/liveFeedState.ts`
- Modify: `frontend/src/features/benchmarks/liveFeedState.test.ts`

- [ ] **Step 1: Write failing tests for `agentEventToLiveFeedEvent`**

Append to `liveFeedState.test.ts`:

```typescript
import type { AgentEvent, TrajectorySummary } from "~/api/types";
import type { BenchmarkResult } from "~/api/types";

import {
  agentEventToLiveFeedEvent,
  computeEta,
  formatTokens,
  statsFromSummary,
} from "./liveFeedState";

// ... existing tests ...

describe("agentEventToLiveFeedEvent", () => {
  const base: AgentEvent = {
    id: "evt-1",
    agent_id: "agent-1",
    task_id: "task-1",
    project_id: "proj-1",
    type: "agent.tool_called",
    payload: { input: "hello", output: "world", success: true, step: 3 },
    version: 1,
    created_at: "2026-03-16T12:00:00Z",
    tool_name: "Read",
    model: "gpt-4",
    tokens_in: 100,
    tokens_out: 50,
    cost_usd: 0.005,
  };

  it("maps top-level fields", () => {
    const result = agentEventToLiveFeedEvent(base);
    expect(result.id).toBe("evt-1");
    expect(result.event_type).toBe("agent.tool_called");
    expect(result.tool_name).toBe("Read");
    expect(result.model).toBe("gpt-4");
    expect(result.tokens_in).toBe(100);
    expect(result.tokens_out).toBe(50);
    expect(result.cost_usd).toBe(0.005);
    expect(result.project_id).toBe("proj-1");
  });

  it("maps payload fields", () => {
    const result = agentEventToLiveFeedEvent(base);
    expect(result.input).toBe("hello");
    expect(result.output).toBe("world");
    expect(result.success).toBe(true);
    expect(result.step).toBe(3);
  });

  it("converts created_at to timestamp", () => {
    const result = agentEventToLiveFeedEvent(base);
    expect(result.timestamp).toBe(new Date("2026-03-16T12:00:00Z").getTime());
  });

  it("falls back to payload when top-level fields missing", () => {
    const ev: AgentEvent = {
      ...base,
      tool_name: undefined,
      model: undefined,
      tokens_in: undefined,
      tokens_out: undefined,
      cost_usd: undefined,
      payload: {
        tool_name: "Edit",
        model: "claude",
        tokens_in: 200,
        tokens_out: 80,
        cost_usd: 0.01,
        input: "x",
        output: "y",
        success: false,
        step: 1,
      },
    };
    const result = agentEventToLiveFeedEvent(ev);
    expect(result.tool_name).toBe("Edit");
    expect(result.model).toBe("claude");
    expect(result.tokens_in).toBe(200);
    expect(result.tokens_out).toBe(80);
    expect(result.cost_usd).toBe(0.01);
  });
});
```

- [ ] **Step 2: Write failing tests for `statsFromSummary`**

Append to `liveFeedState.test.ts`:

```typescript
describe("statsFromSummary", () => {
  const summary: TrajectorySummary = {
    total_events: 100,
    event_counts: {},
    duration_ms: 60000,
    tool_call_count: 47,
    error_count: 3,
    total_tokens_in: 24300,
    total_tokens_out: 8100,
    total_cost_usd: 0.42,
  };

  it("maps summary fields to AggregateStats", () => {
    const stats = statsFromSummary(summary, []);
    expect(stats.totalTokensIn).toBe(24300);
    expect(stats.totalTokensOut).toBe(8100);
    expect(stats.toolCallCount).toBe(47);
    expect(stats.toolSuccessCount).toBe(44); // 47 - 3
  });

  it("computes avgScore from results", () => {
    const results = [
      { scores: { correctness: 0.8 } },
      { scores: { correctness: 0.6 } },
    ] as BenchmarkResult[];
    const stats = statsFromSummary(summary, results);
    expect(stats.avgScore).toBeCloseTo(0.7);
  });

  it("computes costPerTask from results count", () => {
    const results = [
      { scores: { correctness: 0.8 } },
      { scores: { correctness: 0.6 } },
      { scores: { correctness: 0.7 } },
    ] as BenchmarkResult[];
    const stats = statsFromSummary(summary, results);
    expect(stats.costPerTask).toBeCloseTo(0.14);
  });

  it("handles zero results", () => {
    const stats = statsFromSummary(summary, []);
    expect(stats.avgScore).toBe(0);
    expect(stats.costPerTask).toBe(0);
  });
});
```

- [ ] **Step 3: Run tests — verify FAIL**

Run: `cd frontend && npx vitest run src/features/benchmarks/liveFeedState.test.ts 2>&1 | tail -15`

Expected: FAIL — `agentEventToLiveFeedEvent` and `statsFromSummary` not exported.

- [ ] **Step 4: Implement both functions**

Add to `liveFeedState.ts`:

```typescript
export function agentEventToLiveFeedEvent(ev: AgentEvent): LiveFeedEvent {
  return {
    id: ev.id,
    timestamp: new Date(ev.created_at).getTime(),
    run_id: "",
    project_id: ev.project_id,
    event_type: ev.type,
    tool_name: ev.tool_name ?? (ev.payload.tool_name as string | undefined),
    model: ev.model ?? (ev.payload.model as string | undefined),
    input: ev.payload.input as string | undefined,
    output: ev.payload.output as string | undefined,
    success: ev.payload.success as boolean | undefined,
    step: ev.payload.step as number | undefined,
    cost_usd: ev.cost_usd ?? (ev.payload.cost_usd as number | undefined),
    tokens_in: ev.tokens_in ?? (ev.payload.tokens_in as number | undefined),
    tokens_out: ev.tokens_out ?? (ev.payload.tokens_out as number | undefined),
  };
}

// Second generic parameter inferred — only `scores` used from BenchmarkResult
interface ResultWithScores {
  scores?: Record<string, number>;
}

export function statsFromSummary(
  summary: TrajectorySummary,
  results: ResultWithScores[],
): AggregateStats {
  const completedCount = results.length;
  const avgScore =
    completedCount > 0
      ? results.reduce((sum, r) => {
          const firstScore = r.scores ? (Object.values(r.scores)[0] ?? 0) : 0;
          return sum + firstScore;
        }, 0) / completedCount
      : 0;

  return {
    totalTokensIn: summary.total_tokens_in,
    totalTokensOut: summary.total_tokens_out,
    toolCallCount: summary.tool_call_count,
    toolSuccessCount: summary.tool_call_count - summary.error_count,
    avgScore,
    costPerTask: completedCount > 0 ? summary.total_cost_usd / completedCount : 0,
  };
}
```

- [ ] **Step 5: Run tests — verify PASS**

Run: `cd frontend && npx vitest run src/features/benchmarks/liveFeedState.test.ts 2>&1 | tail -15`

Expected: PASS (all tests).

- [ ] **Step 6: Commit**

```
feat(benchmark): add agentEventToLiveFeedEvent mapper and statsFromSummary
```

---

### Task 5: Add `resultToFeatureEntry` with tests

**Files:**
- Modify: `frontend/src/features/benchmarks/liveFeedState.ts`
- Modify: `frontend/src/features/benchmarks/liveFeedState.test.ts`

- [ ] **Step 1: Write failing tests**

Append to `liveFeedState.test.ts`:

```typescript
import { resultToFeatureEntry } from "./liveFeedState";

// ... existing tests ...

describe("resultToFeatureEntry", () => {
  it("maps BenchmarkResult to FeatureEntry", () => {
    const r = {
      task_id: "t1",
      task_name: "parse-json",
      cost_usd: 0.12,
      duration_ms: 72000,
      scores: { correctness: 0.95 },
    } as BenchmarkResult;
    const entry = resultToFeatureEntry(r);
    expect(entry.id).toBe("t1");
    expect(entry.name).toBe("parse-json");
    expect(entry.status).toBe("completed");
    expect(entry.cost).toBe(0.12);
    expect(entry.score).toBe(0.95);
    expect(entry.events).toEqual([]);
    expect(entry.startedAt).toBeDefined();
  });

  it("handles missing scores", () => {
    const r = {
      task_id: "t2",
      task_name: "empty",
      cost_usd: 0,
      duration_ms: 0,
    } as BenchmarkResult;
    const entry = resultToFeatureEntry(r);
    expect(entry.score).toBeUndefined();
    expect(entry.startedAt).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `cd frontend && npx vitest run src/features/benchmarks/liveFeedState.test.ts 2>&1 | tail -10`

- [ ] **Step 3: Implement `resultToFeatureEntry`**

Add to `liveFeedState.ts`:

```typescript
interface ResultForFeature {
  task_id: string;
  task_name: string;
  cost_usd: number;
  duration_ms: number;
  scores?: Record<string, number>;
}

export function resultToFeatureEntry(r: ResultForFeature): FeatureEntry {
  return {
    id: r.task_id,
    name: r.task_name,
    status: "completed",
    events: [],
    cost: r.cost_usd,
    step: 0,
    score: r.scores ? (Object.values(r.scores)[0] as number | undefined) : undefined,
    startedAt: r.duration_ms > 0 ? Date.now() - r.duration_ms : undefined,
  };
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `cd frontend && npx vitest run src/features/benchmarks/liveFeedState.test.ts 2>&1 | tail -10`

- [ ] **Step 5: Run full test suite to verify nothing broken**

Run: `cd frontend && npx vitest run 2>&1 | tail -15`

Expected: All tests PASS.

- [ ] **Step 6: Commit**

```
feat(benchmark): add resultToFeatureEntry mapper with tests
```

---

## Chunk 2: State Lift — BenchmarkPage

### Task 6: Add `liveFeedStates` Map and WS subscription to `BenchmarkPage`

This is the core refactor: move WebSocket subscription logic from `BenchmarkLiveFeed` into `BenchmarkPage`, operating on a `Map<runId, LiveFeedState>`.

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx`

- [ ] **Step 1: Add imports from `liveFeedState.ts`**

At the top of `BenchmarkPage.tsx`, add:

```typescript
import { createEffect, onCleanup } from "solid-js"; // ensure createEffect imported
import type { LiveFeedEvent } from "~/api/types";
import {
  emptyLiveFeedState,
  MAX_EVENTS,
  type AggregateStats,
  type FeatureEntry,
  type LiveFeedState,
} from "./liveFeedState";
```

- [ ] **Step 2: Add `liveFeedStates` signal after `selectedRun`**

After line 120 (`const [selectedRun, setSelectedRun] = ...`), add:

```typescript
// Live feed state per running benchmark — persists across card close/reopen
const [liveFeedStates, setLiveFeedStates] = createSignal<Map<string, LiveFeedState>>(new Map());

// Helper: get or create LiveFeedState for a run
const getOrCreateState = (runId: string): LiveFeedState => {
  const map = liveFeedStates();
  return map.get(runId) ?? emptyLiveFeedState();
};

// Helper: update a single run's LiveFeedState
const updateRunState = (runId: string, updater: (prev: LiveFeedState) => LiveFeedState) => {
  setLiveFeedStates((prev) => {
    const next = new Map(prev);
    const current = next.get(runId) ?? emptyLiveFeedState();
    next.set(runId, updater(current));
    return next;
  });
};
```

- [ ] **Step 3: Move WS event handling into the existing subscription**

Replace the existing WS subscription (lines 100-104) with a combined handler that does both run-list refresh AND live feed state updates:

```typescript
const cleanupWS = onMessage((msg) => {
  // Auto-refresh runs list on progress/completion
  if (msg.type === "benchmark.run.progress" || msg.type === "benchmark.task.completed") {
    refetch();
  }

  // ---- Live feed state updates ----
  if (msg.type === "trajectory.event") {
    const p = msg.payload as {
      run_id: string; project_id: string; event_type: string;
      tool_name?: string; model?: string; input?: string; output?: string;
      success?: boolean; step?: number; cost_usd?: number;
      tokens_in?: number; tokens_out?: number;
    };
    updateRunState(p.run_id, (state) => {
      const evt: LiveFeedEvent = {
        id: crypto.randomUUID(),
        timestamp: Date.now(),
        run_id: p.run_id,
        project_id: p.project_id ?? "",
        event_type: p.event_type,
        tool_name: p.tool_name,
        model: p.model,
        input: p.input,
        output: p.output,
        success: p.success,
        step: p.step,
        cost_usd: p.cost_usd,
        tokens_in: p.tokens_in,
        tokens_out: p.tokens_out,
      };

      // Event buffer with MAX_EVENTS cap
      const events = [...state.events, evt];
      const trimmed = events.length > MAX_EVENTS ? events.slice(events.length - MAX_EVENTS) : events;

      // Update stats
      const stats = { ...state.stats };
      if (p.event_type === "agent.tool_called") {
        stats.toolCallCount++;
        if (p.success !== false) stats.toolSuccessCount++;
      }
      stats.totalTokensIn += p.tokens_in ?? 0;
      stats.totalTokensOut += p.tokens_out ?? 0;

      // Track event under current feature
      const features = new Map(state.features);
      // Find the currently running feature
      let currentFeatureId: string | null = null;
      for (const [id, f] of features) {
        if (f.status === "running") { currentFeatureId = id; break; }
      }
      if (currentFeatureId) {
        const f = features.get(currentFeatureId)!;
        features.set(currentFeatureId, {
          ...f,
          events: [...f.events, evt],
          cost: f.cost + (p.cost_usd ?? 0),
          step: p.step ?? f.step,
        });
      }

      return { ...state, events: trimmed, stats, features, lastEventId: evt.id };
    });
  }

  if (msg.type === "benchmark.run.progress") {
    const p = msg.payload as {
      run_id: string; completed_tasks: number; total_tasks: number;
      avg_score: number; total_cost_usd: number;
    };
    updateRunState(p.run_id, (state) => {
      const progress = {
        completed_tasks: p.completed_tasks,
        total_tasks: p.total_tasks,
        avg_score: p.avg_score,
        total_cost_usd: p.total_cost_usd,
      };
      const stats = { ...state.stats };
      stats.avgScore = p.avg_score;
      stats.costPerTask = p.completed_tasks > 0
        ? p.total_cost_usd / p.completed_tasks : 0;
      return { ...state, progress, stats };
    });
  }

  if (msg.type === "benchmark.task.started") {
    const p = msg.payload as {
      run_id: string; task_id: string; task_name: string;
      index: number; total: number;
    };
    updateRunState(p.run_id, (state) => {
      const features = new Map(state.features);
      if (!features.has(p.task_id)) {
        features.set(p.task_id, {
          id: p.task_id,
          name: p.task_name,
          status: "running",
          events: [],
          startedAt: Date.now(),
          cost: 0,
          step: 0,
        });
      }
      return { ...state, features };
    });
  }

  if (msg.type === "benchmark.task.completed") {
    const p = msg.payload as {
      run_id: string; task_id: string; task_name: string;
      score: number; cost_usd: number;
    };
    updateRunState(p.run_id, (state) => {
      const features = new Map(state.features);
      const existing = features.get(p.task_id);
      features.set(p.task_id, {
        id: p.task_id,
        name: p.task_name,
        status: "completed",
        events: existing?.events ?? [],
        startedAt: existing?.startedAt,
        cost: p.cost_usd,
        step: existing?.step ?? 0,
        score: p.score,
      });
      return { ...state, features };
    });
  }

  if (msg.type === "autoagent.status") {
    const p = msg.payload as { run_id?: string; current_feature_id?: string };
    if (p.run_id && p.current_feature_id) {
      updateRunState(p.run_id, (state) => {
        const features = new Map(state.features);
        // Mark all running features as pending, set the new one as running
        for (const [id, f] of features) {
          if (f.status === "running" && id !== p.current_feature_id) {
            features.set(id, { ...f, status: "pending" });
          }
        }
        const target = features.get(p.current_feature_id!);
        if (target && target.status !== "completed") {
          features.set(p.current_feature_id!, { ...target, status: "running" });
        }
        return { ...state, features };
      });
    }
  }
});
onCleanup(cleanupWS);
```

- [ ] **Step 4: Update `BenchmarkLiveFeed` usage — pass state as prop**

Find the `BenchmarkLiveFeed` usage (around line 465) and change:

```typescript
// Before:
<BenchmarkLiveFeed runId={run.id} startedAt={run.created_at} />

// After:
<BenchmarkLiveFeed
  state={liveFeedStates().get(run.id) ?? emptyLiveFeedState()}
  startedAt={run.created_at}
/>
```

- [ ] **Step 5: Verify TypeScript compiles (with expected errors in BenchmarkLiveFeed)**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

Expected: Errors in `BenchmarkLiveFeed.tsx` because its props interface hasn't been updated yet. That's Task 8.

- [ ] **Step 6: Commit**

```
feat(benchmark): lift live feed state to BenchmarkPage with WS subscription
```

---

### Task 7: Add hydration effect to `BenchmarkPage`

Hydrate running runs from API on page mount/when runs list updates.

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkPage.tsx`

- [ ] **Step 1: Add imports for hydration functions**

Add to the imports:

```typescript
import {
  agentEventToLiveFeedEvent,
  emptyLiveFeedState,
  emptyStats,
  MAX_EVENTS,
  resultToFeatureEntry,
  statsFromSummary,
  type AggregateStats,
  type FeatureEntry,
  type LiveFeedState,
} from "./liveFeedState";
```

- [ ] **Step 2: Add hydration effect**

After the WS subscription block, add:

```typescript
// Hydrate live feed state for running runs from API
createEffect(() => {
  const runList = runs();
  if (!runList) return;
  const runningRuns = runList.filter((r) => r.status === "running");

  for (const run of runningRuns) {
    const existing = liveFeedStates().get(run.id);
    if (existing?.hydratedFromApi) continue; // already hydrated

    // Fetch trajectory + results in parallel
    Promise.all([
      api.trajectory.get(run.id, { limit: 200 }),
      api.benchmarks.listResults(run.id),
    ]).then(([trajectory, resultsList]) => {
      const events = trajectory.events.map(agentEventToLiveFeedEvent);
      const stats = statsFromSummary(trajectory.stats, resultsList);

      const features = new Map<string, FeatureEntry>();
      for (const r of resultsList) {
        features.set(r.task_id, resultToFeatureEntry(r));
      }

      const completedCount = resultsList.length;
      const avgScore = completedCount > 0
        ? resultsList.reduce((sum, r) => {
            const first = r.scores ? (Object.values(r.scores)[0] ?? 0) : 0;
            return sum + (first as number);
          }, 0) / completedCount
        : 0;

      const progress = {
        completed_tasks: completedCount,
        total_tasks: null as number | null,
        avg_score: avgScore,
        total_cost_usd: trajectory.stats.total_cost_usd,
      };

      const lastEvent = events.length > 0 ? events[events.length - 1] : null;

      updateRunState(run.id, (prev) => ({
        events: prev.events.length > events.length ? prev.events : events,
        progress: prev.progress ?? progress,
        features: prev.features.size > features.size ? prev.features : features,
        stats: prev.hydratedFromApi ? prev.stats : stats,
        hydratedFromApi: true,
        lastEventId: prev.lastEventId ?? lastEvent?.id ?? null,
      }));
    }).catch((err) => {
      console.warn(`[LiveFeed] hydration failed for run ${run.id}:`, err);
      // Fall back to WS-only mode
      updateRunState(run.id, (prev) => ({ ...prev, hydratedFromApi: true }));
    });
  }
});
```

- [ ] **Step 3: Commit**

```
feat(benchmark): add API hydration for running benchmarks on page load
```

---

## Chunk 3: Presentational BenchmarkLiveFeed

### Task 8: Convert `BenchmarkLiveFeed` to presentational component

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx`

- [ ] **Step 1: Update imports and props interface**

Replace the top section (lines 1-12) with:

```typescript
import { createVirtualizer } from "@tanstack/solid-virtual";
import { createEffect, createMemo, createSignal, For, on, onCleanup, Show } from "solid-js";

import type { BenchmarkLiveProgress, LiveFeedEvent } from "~/api/types";

import {
  computeEta,
  formatTokens,
  type AggregateStats,
  type FeatureEntry,
  type LiveFeedState,
} from "./liveFeedState";

interface BenchmarkLiveFeedProps {
  state: LiveFeedState;
  startedAt: string;
}
```

- [ ] **Step 2: Remove all local state signals, WS subscription, and FeatureEntry/payload interfaces**

Delete:
- The local `FeatureEntry` interface (lines 14-23) — now imported from `liveFeedState.ts`
- All WS payload interfaces (lines 25-71) — no longer needed, WS is in BenchmarkPage
- The local state signals inside the component (lines 146-152)
- The `useWebSocket` import and the entire WS subscription block (lines 162-265)

- [ ] **Step 3: Derive all data from `props.state`**

Replace the component body (from the function signature through the derived data section) with:

```typescript
export function BenchmarkLiveFeed(props: BenchmarkLiveFeedProps) {
  // ---- Derived from props ----
  const events = () => props.state.events;
  const progress = () => props.state.progress;
  const features = () => props.state.features;
  const stats = () => props.state.stats;

  // ---- Local UI state (survives only within mounted component) ----
  const [elapsed, setElapsed] = createSignal(0);
  const [autoScroll, setAutoScroll] = createSignal(true);
  const [expandedFeature, setExpandedFeature] = createSignal<string | null>(null);

  // ---- Elapsed timer ----
  createEffect(() => {
    const startTime = new Date(props.startedAt).getTime();
    const id = setInterval(() => setElapsed(Math.floor((Date.now() - startTime) / 1000)), 1000);
    onCleanup(() => clearInterval(id));
  });

  // ---- Derived data ----
  const featureList = createMemo(() => Array.from(features().values()));
  const useAccordion = createMemo(() => featureList().length > 1);

  const displayedEvents = createMemo(() => {
    const ef = expandedFeature();
    if (useAccordion() && ef) {
      const f = features().get(ef);
      return f?.events ?? [];
    }
    return events();
  });

  const pct = createMemo(() => {
    const p = progress();
    if (!p || p.total_tasks === null || p.total_tasks === 0) return 0;
    return Math.round((p.completed_tasks / p.total_tasks) * 100);
  });

  const totalTasksKnown = createMemo(() => {
    const p = progress();
    return p !== null && p.total_tasks !== null;
  });

  const currentTaskRunning = createMemo(() => {
    const p = progress();
    return p && p.total_tasks !== null && p.completed_tasks < p.total_tasks ? p : undefined;
  });

  const eta = createMemo(() => {
    const p = progress();
    if (!p) return null;
    return computeEta(p.completed_tasks, p.total_tasks, elapsed());
  });
```

Keep the existing virtualizer, scrollToEnd, handleScroll, toggleFeature code unchanged (lines 291-332).

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

Expected: Clean or very few warnings. No errors.

- [ ] **Step 5: Commit**

```
refactor(benchmark): convert BenchmarkLiveFeed to presentational component
```

---

### Task 9: Add stats line to `BenchmarkLiveFeed` render

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx`

- [ ] **Step 1: Update progress header to handle null `total_tasks`**

Replace the progress header text sections. The task counter should handle the `total_tasks: null` case:

```tsx
<Show when={progress()}>
  {(p) => (
    <span class="whitespace-nowrap text-cf-text-secondary">
      <Show
        when={totalTasksKnown()}
        fallback={<>{p().completed_tasks} tasks completed</>}
      >
        {p().completed_tasks}/{p().total_tasks} tasks ({pct()}%)
      </Show>
    </span>
  )}
</Show>
```

Also add ETA to the header row, after the elapsed timer:

```tsx
<span class="whitespace-nowrap font-mono text-cf-text-muted">
  {formatElapsed(elapsed())}
</span>
<Show when={eta()}>
  {(e) => (
    <span class="whitespace-nowrap text-cf-text-muted">
      ETA ~{formatElapsed(e())}
    </span>
  )}
</Show>
```

- [ ] **Step 2: Add stats line between progress header and feature list**

Insert after the progress header `</div>` and before the feature accordion `<Show when={useAccordion()}>`:

```tsx
{/* ---- Inline Stats ---- */}
<Show when={stats().toolCallCount > 0 || (progress()?.completed_tasks ?? 0) > 0}>
  <div class="flex items-center gap-2 text-xs font-mono text-cf-text-muted"
       style={{ "border-top": "1px solid var(--cf-border)", "padding-top": "6px" }}>
    <Show when={stats().avgScore > 0}>
      <span>
        avg{" "}
        <span class={
          stats().avgScore >= 0.7
            ? "text-cf-success-fg font-semibold"
            : stats().avgScore >= 0.4
              ? "text-yellow-400 font-semibold"
              : "text-cf-danger-fg font-semibold"
        }>
          {stats().avgScore.toFixed(2)}
        </span>
      </span>
      <span class="text-cf-border">|</span>
    </Show>
    <Show when={stats().totalTokensIn > 0}>
      <span>
        tok{" "}
        <span class="text-cf-text-secondary">
          {formatTokens(stats().totalTokensIn)}/{formatTokens(stats().totalTokensOut)}
        </span>
      </span>
      <span class="text-cf-border">|</span>
    </Show>
    <Show when={stats().toolCallCount > 0}>
      <span>
        tools{" "}
        <span class="text-cf-text-secondary">{stats().toolCallCount}</span>
        {" "}
        <span class={
          stats().toolCallCount > 0 &&
          (stats().toolSuccessCount / stats().toolCallCount) >= 0.9
            ? "text-cf-success-fg"
            : "text-yellow-400"
        }>
          ({Math.round((stats().toolSuccessCount / stats().toolCallCount) * 100)}%)
        </span>
      </span>
      <span class="text-cf-border">|</span>
    </Show>
    <Show when={stats().costPerTask > 0}>
      <span>
        $/task{" "}
        <span class="text-cf-text-secondary">${stats().costPerTask.toFixed(2)}</span>
      </span>
    </Show>
  </div>
</Show>
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -10`

Expected: Clean compile.

- [ ] **Step 4: Commit**

```
feat(benchmark): add inline stats line to live feed
```

---

### Task 10: Add mini score bars to feature rows

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx`

- [ ] **Step 1: Create a `MiniScoreBar` helper**

Add before the `BenchmarkLiveFeed` function:

```typescript
function scoreBarColor(score: number): string {
  if (score >= 0.7) return "bg-cf-success-fg";
  if (score >= 0.4) return "bg-yellow-400";
  return "bg-cf-danger-fg";
}
```

- [ ] **Step 2: Add mini bar to accordion feature rows**

In the accordion `<For each={featureList()}>` block, find the score display section (where `feature.score` is shown). Insert a mini bar before the score text:

```tsx
<Show when={feature.score !== undefined}>
  {/* Mini score bar */}
  <span class="inline-block w-10 h-1 rounded-full bg-cf-bg-secondary overflow-hidden shrink-0">
    <span
      class={`block h-full rounded-full ${scoreBarColor(feature.score!)}`}
      style={{ width: `${Math.round((feature.score!) * 100)}%` }}
    />
  </span>
  <span class="font-mono">{feature.score!.toFixed(2)}</span>
</Show>
```

Replace the existing score rendering (the `<Show when={feature.score}>` block that only shows `score().toFixed(1)`) with the above in both the accordion section AND the flat task list section.

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -10`

Expected: Clean compile.

- [ ] **Step 4: Run full test suite**

Run: `cd frontend && npx vitest run 2>&1 | tail -15`

Expected: All tests PASS.

- [ ] **Step 5: Commit**

```
feat(benchmark): add mini score bars to feature rows in live feed
```

---

## Chunk 4: Final Cleanup & Verification

### Task 11: Remove dead code from `BenchmarkLiveFeed`

**Files:**
- Modify: `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx`

- [ ] **Step 1: Clean up unused imports**

Remove `useWebSocket` import if still present. Remove any unused type imports.

- [ ] **Step 2: Verify no dead local state code remains**

Grep for `createSignal` in the file — only `elapsed`, `autoScroll`, `expandedFeature` should remain. No `events`, `progress`, `features`, `currentFeatureId` signals.

- [ ] **Step 3: Verify build**

Run: `cd frontend && npx tsc --noEmit && echo "OK"`

Expected: `OK`

- [ ] **Step 4: Run full test suite**

Run: `cd frontend && npx vitest run 2>&1 | tail -15`

Expected: All tests PASS.

- [ ] **Step 5: Commit**

```
refactor(benchmark): remove dead code from BenchmarkLiveFeed
```

---

### Task 12: Manual E2E verification

No automated E2E test for this — manual verification with a running benchmark.

- [ ] **Step 1: Start backend and frontend**

```bash
# Terminal 1: Docker services
docker compose up -d postgres nats litellm

# Terminal 2: Go backend
APP_ENV=development go run ./cmd/codeforge/

# Terminal 3: Frontend
cd frontend && npm run dev
```

- [ ] **Step 2: Start a benchmark run**

Log in as `admin@localhost` / `Changeme123`, navigate to Benchmarks, start a run against any available suite/model.

- [ ] **Step 3: Verify stats line appears**

While the benchmark is running, verify the expanded info card shows:
- Progress bar with task count
- **NEW:** Stats line with avg score, tokens, tools, $/task (fields appear as data becomes available)
- Feature list with **NEW:** mini score bars on completed features
- Event log

- [ ] **Step 4: Verify close/reopen persistence**

Click the card to collapse it. Click again to expand. Verify:
- Events are still there (not reset to empty)
- Progress, stats, feature list are preserved
- Event log scrolls to the correct position

- [ ] **Step 5: Verify page reload persistence**

Press F5 / Ctrl+R to reload the page. Expand the running benchmark card. Verify:
- Events are loaded from the trajectory API (may be fewer than before reload, but present)
- Stats line shows aggregate data from TrajectorySummary
- Completed features show scores and mini bars
- New WS events continue appending

- [ ] **Step 6: Final commit with any fixes**

If any fixes were needed during manual testing, commit them.

```
fix(benchmark): adjustments from manual E2E testing
```

---

### Task 13: Update documentation

**Files:**
- Modify: `docs/todo.md`
- Modify: `docs/project-status.md`

- [ ] **Step 1: Mark task complete in `docs/todo.md`**

Add/check off the benchmark live feed improvements task.

- [ ] **Step 2: Update `docs/project-status.md`**

Add a note about live feed persistence and density improvements.

- [ ] **Step 3: Commit**

```
docs: update todo and project-status for benchmark live feed improvements
```
