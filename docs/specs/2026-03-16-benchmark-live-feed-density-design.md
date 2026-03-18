# Benchmark Live Feed: State Persistence & Information Density

**Date:** 2026-03-16
**Status:** Draft
**Scope:** Frontend changes + minor `AgentEvent` type extension

## Problem

Two issues with the benchmark info card's live feed during a running benchmark:

1. **State loss on close/reopen and page reload:** `BenchmarkLiveFeed` stores all state (events, progress, features, elapsed timer) in component-local signals. When the card is collapsed via `Show when={selectedRun() === run.id}`, the component unmounts and all state is destroyed. On page reload, state is gone entirely.

2. **Low information density:** The live feed shows progress bar, task counter, cost, timer, and an event log — but omits several metrics that are already available via WebSocket: avg score, total tokens, tool success rate, cost per task, ETA.

## Existing Infrastructure

No backend changes needed. All required data is already persisted and accessible:

| Data Source | Endpoint / Signal | Provides |
|---|---|---|
| Trajectory API | `GET /runs/{id}/trajectory` | Paginated `AgentEvent[]` + `TrajectorySummary` (stats) |
| `TrajectorySummary` | Included in trajectory response | `total_events`, `tool_call_count`, `error_count`, `total_tokens_in/out`, `total_cost_usd` |
| Benchmark Results API | `GET /benchmarks/runs/{id}/results` | Completed task results (scores, cost, duration_ms, tool_calls) |
| `BenchmarkRun` record | Already fetched in `runs()` resource | `status`, `total_cost`, `total_tokens`, `metrics`, `created_at` |
| WebSocket events | Already subscribed | `trajectory.event`, `benchmark.run.progress`, `benchmark.task.started/completed`, `autoagent.status` |
| Frontend API client | `api.trajectory.get(runId)` | Already wired, returns `TrajectoryPage` with `stats` field |

## Design

### Part 1: State Persistence

**Approach:** Lift live feed state to `BenchmarkPage` level + hydrate from API on mount.

#### State Store

Create a `Map<string, LiveFeedState>` signal in `BenchmarkPage`, keyed by run ID:

```typescript
interface LiveFeedState {
  events: LiveFeedEvent[];
  progress: BenchmarkLiveProgress | null;
  features: Map<string, FeatureEntry>;
  stats: AggregateStats;
  hydratedFromApi: boolean;       // prevents duplicate hydration
  lastEventId: string | null;     // for deduplication on WS handoff
}

interface AggregateStats {
  totalTokensIn: number;
  totalTokensOut: number;
  toolCallCount: number;
  toolSuccessCount: number;
  avgScore: number;
  costPerTask: number;
}
```

#### Lifecycle

```
Page load / mount
  |
  v
For each run where status === "running":
  1. Fetch GET /runs/{id}/trajectory?limit=200  (last 200 events + stats)
  2. Fetch GET /benchmarks/runs/{id}/results     (already-completed tasks)
  3. Populate LiveFeedState from response:
     - events: map AgentEvent[] -> LiveFeedEvent[]
     - features: reconstruct from results (completed) + infer running task
     - stats: from TrajectorySummary
     - progress: { completed_tasks: results.length, total_tasks: null (unknown),
                   avg_score: mean of result scores, total_cost_usd: stats.total_cost_usd }
     - lastEventId: last event's ID (for dedup)
  4. Mark hydratedFromApi = true
  |
  v
WebSocket subscription (always active at BenchmarkPage level):
  - Filters by run_id, updates the matching LiveFeedState entry
  - DEDUP: skip events where event ID <= lastEventId (prevents duplicates
    during the race window between hydration fetch and WS subscription)
  - Appends new events, updates progress/features/stats incrementally
  - Same logic as current BenchmarkLiveFeed, but operating on the Map entry
  |
  v
Card expand (selectedRun changes):
  - BenchmarkLiveFeed receives LiveFeedState as props (read-only)
  - No local state, no local WS subscription
  - Component is purely presentational
  |
  v
Card collapse:
  - Component unmounts, but LiveFeedState persists in parent Map
  - WS subscription continues accumulating events
  |
  v
Page reload:
  - Step 1-4 repeats, trajectory API returns persisted events
  - No data loss (events are in PostgreSQL)
```

#### `AgentEvent` Type Extension

The Go `AgentEvent` struct includes top-level fields (`tool_name`, `model`, `tokens_in`, `tokens_out`, `cost_usd`) that are not in the frontend TypeScript type. The mapper needs these fields.

Add to `frontend/src/api/types.ts` `AgentEvent` interface:

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
  // Per-tool tracking fields (populated for tool_called/step_done events):
  tool_name?: string;
  model?: string;
  tokens_in?: number;
  tokens_out?: number;
  cost_usd?: number;
}
```

These fields are already serialized by Go (`json:"tool_name,omitempty"` etc.) — the frontend type just needs to declare them.

#### Event Mapping: `AgentEvent` -> `LiveFeedEvent`

The trajectory API returns `AgentEvent` (DB schema), the live feed renders `LiveFeedEvent` (WS schema). Mapper reads from top-level fields, falling back to `payload` for backwards compatibility:

```typescript
function agentEventToLiveFeedEvent(ev: AgentEvent): LiveFeedEvent {
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
```

#### Feature Reconstruction from Results

On hydration, completed tasks come from `ListBenchmarkResults`. Each `Result` maps to a `FeatureEntry`:

```typescript
function resultToFeatureEntry(r: BenchmarkResult): FeatureEntry {
  return {
    id: r.task_id,
    name: r.task_name,
    status: "completed",
    events: [],       // not loaded per-feature on hydration (too expensive)
    cost: r.cost_usd,
    step: 0,
    score: r.scores ? Object.values(r.scores)[0] as number : undefined,
    // duration_ms available on BenchmarkResult — convert to elapsed display
    startedAt: r.duration_ms > 0 ? Date.now() - r.duration_ms : undefined,
  };
}
```

Note: `startedAt` for hydrated features is approximate (back-calculated from `duration_ms`). This is sufficient for displaying elapsed time in the feature row. For features observed live via WS, the actual `startedAt` timestamp is used.

#### `total_tasks` on Hydration

`total_tasks` is only broadcast via WS `benchmark.run.progress` — it is not persisted on the `BenchmarkRun` record. On hydration (page reload):

- **Before first WS progress message:** `total_tasks` is unknown. Show indeterminate progress bar (pulsing, same as current "no progress yet" state) with only the completed count: `"3 tasks completed"`.
- **After first WS progress message arrives:** Switch to percentage-based progress bar: `"3/5 tasks (60%)"`.

This is a graceful degradation — the indeterminate state lasts only until the next progress event from the running benchmark (typically seconds).

### Part 2: Information Density (Option C — Dense Inline Stats)

Add a compact stats line and enhanced feature rows to the live feed. All data comes from `AggregateStats` (computed from `TrajectorySummary` on hydration, then updated incrementally from WS events).

#### New Stats Line

Inserted between the progress header and the feature list. Single line, monospace, pipe-separated:

```
avg 0.82 | tok 24.3k/8.1k | tools 47 (93%) | $/task $0.14 | ETA ~02:55
```

Fields:
- **avg** — `progress.avg_score`, colored green >= 0.7, yellow >= 0.4, red < 0.4
- **tok** — `stats.totalTokensIn` / `stats.totalTokensOut`, formatted with `k` suffix for thousands
- **tools** — `stats.toolCallCount` (success rate = `toolSuccessCount / toolCallCount * 100`)
- **$/task** — `progress.total_cost_usd / progress.completed_tasks` (hidden if 0 completed)
- **ETA** — `elapsed / completed_tasks * remaining_tasks`, hidden until at least 1 task completes and `total_tasks` is known

Each field is individually hidden when its data is unavailable (e.g., no avg score until first task completes, no ETA until total_tasks known).

#### Enhanced Feature Rows

Add mini score bars to each completed feature in the accordion/flat list:

```
[check] parse-json     [====95%=   ] 0.95  $0.12  01:12
[check] validate-schema [===78%    ] 0.78  $0.14  01:34
[spin]  error-recovery  [ACTIVE]     step 4 $0.07  00:39
[dot]   edge-cases                   pending
```

The mini bar is a 40px wide inline element with a fill proportional to the score (0.0-1.0). Color: green >= 0.7, yellow >= 0.4, red < 0.4.

#### Aggregate Stats Computation

On hydration (from `TrajectorySummary`):

```typescript
function statsFromSummary(
  summary: TrajectorySummary,
  results: BenchmarkResult[],
): AggregateStats {
  const completedCount = results.length;
  const avgScore = completedCount > 0
    ? results.reduce((sum, r) => {
        const firstScore = r.scores ? Object.values(r.scores)[0] as number : 0;
        return sum + firstScore;
      }, 0) / completedCount
    : 0;

  return {
    totalTokensIn: summary.total_tokens_in,
    totalTokensOut: summary.total_tokens_out,
    toolCallCount: summary.tool_call_count,
    // Approximation: error_count includes all errors, not just tool failures.
    // This slightly undercounts success rate but is acceptable for a live indicator.
    toolSuccessCount: summary.tool_call_count - summary.error_count,
    avgScore,
    costPerTask: completedCount > 0 ? summary.total_cost_usd / completedCount : 0,
  };
}
```

Incremental update (from WS `trajectory.event`):

```typescript
// On each trajectory event:
if (evt.event_type === "agent.tool_called") {
  stats.toolCallCount++;
  if (evt.success !== false) stats.toolSuccessCount++;
}
stats.totalTokensIn += evt.tokens_in ?? 0;
stats.totalTokensOut += evt.tokens_out ?? 0;

// On benchmark.run.progress:
stats.avgScore = progress.avg_score;
stats.costPerTask = progress.completed_tasks > 0
  ? progress.total_cost_usd / progress.completed_tasks
  : 0;
```

### Component Changes

#### `BenchmarkPage.tsx`

- Add `liveFeedStates` signal: `Map<string, LiveFeedState>`
- Move WebSocket subscription from `BenchmarkLiveFeed` to `BenchmarkPage`
- Add hydration effect: for each running run, fetch trajectory + results on mount
- Pass `LiveFeedState` as prop to `BenchmarkLiveFeed`
- Handle run cancellation: update `LiveFeedState` entry on cancel WS event

#### `BenchmarkLiveFeed.tsx`

- Remove all local state signals (`events`, `progress`, `features`, etc.)
- Remove WebSocket subscription and `onCleanup`
- Accept `state: LiveFeedState` as prop (read-only, reactive)
- Add stats line rendering (new section between progress header and feature list)
- Add mini score bars to feature rows
- Keep: virtualizer, auto-scroll, elapsed timer (derived from `props.startedAt`)

#### `frontend/src/api/types.ts`

- Extend `AgentEvent` interface with optional fields: `tool_name?`, `model?`, `tokens_in?`, `tokens_out?`, `cost_usd?`
- Widen `BenchmarkLiveProgress.total_tasks` from `number` to `number | null` (unknown on hydration until first WS progress message)

#### No changes to:

- Backend (Go / Python)
- API client (`client.ts`)
- WebSocket events or NATS subjects
- Other benchmark components (`BenchmarkRunDetail`, `RoutingReport`, etc.)

### Helpers

```typescript
function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

function computeEta(completed: number, total: number | null, elapsedSec: number): number | null {
  if (total === null || completed === 0 || completed >= total) return null;
  const secPerTask = elapsedSec / completed;
  return Math.round(secPerTask * (total - completed));
}
```

### Edge Cases

| Scenario | Behavior |
|---|---|
| Page reload mid-run | Hydrate from trajectory API + results API, WS picks up new events |
| Close/reopen card | State in parent Map, no data loss, no re-fetch |
| Run completes while card closed | WS updates state in Map, card shows final state on reopen |
| Run cancelled while card open | WS delivers cancel event, state updated, progress stops |
| Run cancelled during hydration | Hydration completes, WS cancel event updates status; no conflict |
| 0 completed tasks | Hide avg score, $/task, ETA; show indeterminate progress |
| `total_tasks` unknown (post-reload) | Show indeterminate progress bar + completed count, no percentage or ETA until first WS progress message |
| Trajectory API returns 0 events | Show "Waiting for events..." empty state, rely on WS |
| Very long runs (>5000 events) | Trajectory API: fetch last 200; WS buffer: MAX_EVENTS=5000. Event log shows recent history, stats cover full run (via TrajectorySummary) |
| Multiple running runs simultaneously | Each has own `LiveFeedState` entry in Map, independent WS handling |
| Run not found in trajectory API | Graceful fallback: empty state, WS-only mode (current behavior) |
| Hydration API call fails | Log warning, fall back to WS-only mode (current behavior) |
| Race: WS events arrive during hydration | Events buffered; after hydration, dedup by comparing event ID against `lastEventId` |
| WS reconnects after disconnect | On reconnect, re-fetch trajectory for events after `lastEventId` timestamp to fill the gap |

### Visual Layout (Expanded Info Card, Running)

```
+------------------------------------------------------------------+
| basic-coding  lm_studio/qwen3-30b  [agent]     [running] 04:23  |
| [correctness] [tool_correctness]                $0.42  [Cancel]  |
+------------------------------------------------------------------+
| [========================================-----------]  60%       |
| 3/5 tasks (60%)              $0.42           04:23   ETA ~02:55  |
| avg 0.82 | tok 24.3k/8.1k | tools 47 (93%) | $/task $0.14      |  <-- NEW
|                                                                  |
| [check] parse-json      [====] 0.95  $0.12  01:12               |  <-- mini bar NEW
| [check] validate-schema [===]  0.78  $0.14  01:34               |
| [check] transform-ast   [===]  0.73  $0.09  00:58               |
| [spin]  error-recovery   ACTIVE  step 4  $0.07  00:39           |
| [dot]   edge-cases       pending                                 |
|                                                                  |
| +--------------------------------------------------------------+ |
| | >_  13:42:01  Read: src/utils/parser.ts (245 lines)      OK  | |
| | >_  13:42:03  Edit: src/utils/parser.ts L42-58            OK  | |
| | [] 13:42:05  Step 7 | qwen3-30b | 1.2k+340 tok | $0.008     | |
| | >_  13:42:08  Bash: npm test -- --filter parser         FAIL  | |
| +--------------------------------------------------------------+ |
+------------------------------------------------------------------+
```

### Files Modified

| File | Change |
|---|---|
| `frontend/src/features/benchmarks/BenchmarkPage.tsx` | Add `liveFeedStates` Map, move WS subscription, add hydration effect, handle cancel |
| `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx` | Convert to presentational component, add stats line, add mini score bars |
| `frontend/src/api/types.ts` | Extend `AgentEvent` with optional `tool_name`, `model`, `tokens_in`, `tokens_out`, `cost_usd` fields |

### Testing

- **Unit:** `formatTokens()`, `computeEta()`, `agentEventToLiveFeedEvent()`, `statsFromSummary()`, incremental stats update, event deduplication logic
- **E2E:** Start benchmark run, verify stats line visible, close/reopen card (state preserved), reload page (state hydrated from API), cancel running run (state updates)
- **Edge:** 0 completed tasks (no avg/ETA), total_tasks unknown (indeterminate bar), very fast completion (ETA vanishes), multiple concurrent runs, WS reconnect catch-up
