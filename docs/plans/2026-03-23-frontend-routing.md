# Frontend Routing Stats Page — Implementation Plan

**Date:** 2026-03-23
**Goal:** Add a new Routing Stats page that exposes the 5 routing endpoints from Phase 29.

---

## Context

The routing backend is fully implemented:
- `internal/adapter/http/handlers_routing.go` — 5 HTTP handlers
- `internal/adapter/http/routes.go` lines 574-580 — routes registered under `/routing`
- `internal/domain/routing/routing.go` — domain types (`ModelPerformanceStats`, `RoutingOutcome`)
- `workers/codeforge/routing/` — Python hybrid router (ComplexityAnalyzer, MABModelSelector, LLMMetaRouter)

Backend endpoints available:
| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/routing/stats` | `HandleListRoutingStats` | UCB scores, model stats (filter by `task_type`, `tier`) |
| POST | `/routing/stats/refresh` | `HandleRefreshRoutingStats` | Recompute aggregated stats from outcomes |
| GET | `/routing/outcomes` | `HandleListRoutingOutcomes` | List individual routing decisions (filter by `limit`) |
| POST | `/routing/outcomes` | `HandleCreateRoutingOutcome` | Manually record an outcome |
| POST | `/routing/seed-from-benchmarks` | `HandleSeedFromBenchmarks` | Seed outcome data from benchmark results |

**Note:** There is no `GET /routing/config` endpoint in the backend. The routing config lives in environment variables (`CODEFORGE_ROUTING_ENABLED`) and Python worker config, not in a REST-accessible endpoint. The plan omits a config display section; if desired later, a backend endpoint must be added first.

No frontend routing code exists yet. `frontend/src/features/benchmarks/RoutingReport.tsx` exists but only renders routing metadata from benchmark results -- it is not a standalone routing page.

---

## Task 1: Create API resource file for routing

**Files:**
- Create: `frontend/src/api/resources/routing.ts`
- Modify: `frontend/src/api/resources/index.ts` (add export)
- Modify: `frontend/src/api/client.ts` (register on `api` object)
- Modify: `frontend/src/api/types.ts` (add response types)

- [ ] **Step 1: Add TypeScript types**

In `frontend/src/api/types.ts`, matching `internal/domain/routing/routing.go`:

```typescript
export interface ModelPerformanceStats {
  id: string;
  model_name: string;
  task_type: string;
  complexity_tier: string;
  trial_count: number;
  total_reward: number;
  avg_reward: number;
  avg_cost_usd: number;
  avg_latency_ms: number;
  avg_quality: number;
  last_selected?: string;
  supports_tools: boolean;
  supports_vision: boolean;
  max_context: number;
  input_cost_per: number;
  output_cost_per: number;
  created_at: string;
  updated_at: string;
}

export interface RoutingOutcome {
  id: string;
  model_name: string;
  task_type: string;
  complexity_tier: string;
  success: boolean;
  quality_score: number;
  cost_usd: number;
  latency_ms: number;
  tokens_in: number;
  tokens_out: number;
  reward: number;
  routing_layer: string;
  run_id?: string;
  conversation_id?: string;
  prompt_hash?: string;
  created_at: string;
}
```

- [ ] **Step 2: Create resource file**

```typescript
// frontend/src/api/resources/routing.ts
import type { CoreClient } from "../core";
import type { ModelPerformanceStats, RoutingOutcome } from "../types";

export function createRoutingResource(c: CoreClient) {
  return {
    stats: (taskType?: string, tier?: string) => {
      const params = new URLSearchParams();
      if (taskType) params.set("task_type", taskType);
      if (tier) params.set("tier", tier);
      const qs = params.toString();
      return c.get<ModelPerformanceStats[]>(`/routing/stats${qs ? `?${qs}` : ""}`);
    },

    refreshStats: () =>
      c.post<{ status: string }>("/routing/stats/refresh"),

    outcomes: (limit?: number) =>
      c.get<RoutingOutcome[]>(`/routing/outcomes${limit ? `?limit=${limit}` : ""}`),

    recordOutcome: (data: Omit<RoutingOutcome, "id" | "created_at">) =>
      c.post<RoutingOutcome>("/routing/outcomes", data),

    seedFromBenchmarks: () =>
      c.post<{ status: string; outcomes_created: number }>("/routing/seed-from-benchmarks"),
  };
}
```

- [ ] **Step 3: Register in client.ts**

Add to `api` object:
```typescript
routing: createRoutingResource(core),
```

- [ ] **Step 4: Export from index.ts**

```typescript
export { createRoutingResource } from "./routing";
```

---

## Task 2: Create RoutingStatsPage component

**Files:**
- Create: `frontend/src/features/routing/RoutingStatsPage.tsx`

Page layout with three sections:

### Section 1: Stats Cards

```tsx
const [taskType, setTaskType] = createSignal("");
const [tier, setTier] = createSignal("");
const [stats, { refetch: refetchStats }] = createResource(
  () => ({ taskType: taskType(), tier: tier() }),
  (opts) => api.routing.stats(opts.taskType || undefined, opts.tier || undefined),
);
```

Display as a table (not cards, since the data is tabular):

| Model | Task Type | Tier | Trials | Avg Reward | Avg Quality | Avg Cost | Avg Latency | Tools | Vision | Max Context |
|-------|-----------|------|--------|------------|-------------|----------|-------------|-------|--------|-------------|

Filters above the table:
- Task Type select: code, review, plan, qa, chat, debug, refactor (or empty for all)
- Complexity Tier select: simple, medium, complex, reasoning (or empty for all)

Action buttons:
- "Refresh Stats" — calls `api.routing.refreshStats()`, then `refetchStats()`
- "Seed from Benchmarks" — calls `api.routing.seedFromBenchmarks()`, toast with count

- [ ] **Step 1: Build stats table with filters**
- [ ] **Step 2: Add Refresh and Seed action buttons**

### Section 2: Outcomes Table

```tsx
const [outcomes, { refetch: refetchOutcomes }] = createResource(
  () => api.routing.outcomes(50),
);
```

Display recent routing decisions:

| Model | Task | Tier | Success | Quality | Cost | Latency | Layer | Time |
|-------|------|------|---------|---------|------|---------|-------|------|

Use `Badge` for success (success=green, fail=red) and routing layer.

- [ ] **Step 3: Build outcomes table**

### Section 3: Record Outcome Form (collapsed by default)

Allow manual outcome recording for testing/seeding:

Fields: model_name, task_type (select), complexity_tier (select), success (checkbox), quality_score, cost_usd, latency_ms, tokens_in, tokens_out, routing_layer.

- [ ] **Step 4: Build record outcome form with validation**

---

## Task 3: Register route and navigation

**Files:**
- Modify: `frontend/src/index.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Add route in index.tsx**

```tsx
import RoutingStatsPage from "./features/routing/RoutingStatsPage.tsx";
// ...
<Route path="/routing" component={RoutingStatsPage} />
```

- [ ] **Step 2: Add to KNOWN_ROUTES in App.tsx**

```typescript
const KNOWN_ROUTES = new Set([
  // ... existing routes
  "/routing",
]);
```

- [ ] **Step 3: Add navigation link in App.tsx sidebar**

Add after the Benchmarks NavLink in the "Intelligence" NavSection:
```tsx
<NavLink href="/routing" icon={<RoutingIcon />} label={t("app.nav.routing")}>
  {t("app.nav.routing")}
</NavLink>
```

Create `RoutingIcon` in `frontend/src/ui/layout/NavIcons.tsx` (simple SVG, e.g., branching arrows icon).

---

## Task 4: Add i18n keys

**Files:**
- Modify: `frontend/src/i18n/en.ts`
- Modify: `frontend/src/i18n/locales/de.ts`

- [ ] **Step 1: Add English keys**

```typescript
"app.nav.routing": "Routing",
"routing.title": "Model Routing",
"routing.subtitle": "Intelligent model selection statistics and outcomes.",
"routing.stats": "Performance Stats",
"routing.stats.empty": "No routing stats available",
"routing.stats.emptyDescription": "Seed data from benchmarks or wait for routing decisions to accumulate.",
"routing.stats.refresh": "Refresh Stats",
"routing.stats.seed": "Seed from Benchmarks",
"routing.stats.seeded": "Seeded {count} outcomes from benchmark data.",
"routing.stats.refreshed": "Stats refreshed.",
"routing.filter.taskType": "Task Type",
"routing.filter.tier": "Complexity Tier",
"routing.filter.all": "All",
"routing.outcomes": "Recent Outcomes",
"routing.outcomes.empty": "No routing outcomes recorded",
"routing.outcomes.record": "Record Outcome",
"routing.field.modelName": "Model",
"routing.field.taskType": "Task Type",
"routing.field.tier": "Tier",
"routing.field.success": "Success",
"routing.field.qualityScore": "Quality",
"routing.field.costUsd": "Cost (USD)",
"routing.field.latencyMs": "Latency (ms)",
"routing.field.tokensIn": "Tokens In",
"routing.field.tokensOut": "Tokens Out",
"routing.field.routingLayer": "Layer",
"routing.field.avgReward": "Avg Reward",
"routing.field.trials": "Trials",
"routing.recorded": "Outcome recorded.",
"routing.error.fetchFailed": "Failed to load routing data.",
"routing.error.refreshFailed": "Failed to refresh stats.",
"routing.error.seedFailed": "Failed to seed from benchmarks.",
"routing.error.recordFailed": "Failed to record outcome.",
```

- [ ] **Step 2: Add German translations (de.ts)**

---

## Task 5: Tests

**Files:**
- Create: `frontend/src/features/routing/routing.test.ts`

- [ ] **Step 1: Unit tests**

Test: renders stats table, filters change resource params, refresh button calls API, seed button calls API, outcomes table renders, record form validates required fields.

- [ ] **Step 2: Verify**

```bash
cd frontend && npm test -- --filter routing
```

---

## Task 6: Final commit

```
feat: add Routing Stats page with model performance and outcomes

- Create routing API resource (stats/outcomes/refresh/seed/record)
- Build RoutingStatsPage with stats table, outcomes table, record form
- Add /routing route, nav link, and i18n keys
- Add TypeScript types for ModelPerformanceStats and RoutingOutcome
```

---

## File Reference

| File | Action |
|------|--------|
| `frontend/src/api/resources/routing.ts` | Create |
| `frontend/src/api/resources/index.ts` | Modify |
| `frontend/src/api/client.ts` | Modify |
| `frontend/src/api/types.ts` | Modify |
| `frontend/src/features/routing/RoutingStatsPage.tsx` | Create |
| `frontend/src/index.tsx` | Modify |
| `frontend/src/App.tsx` | Modify (KNOWN_ROUTES + NavLink) |
| `frontend/src/ui/layout/NavIcons.tsx` | Modify (add RoutingIcon) |
| `frontend/src/i18n/en.ts` | Modify |
| `frontend/src/i18n/locales/de.ts` | Modify |
| `frontend/src/features/routing/routing.test.ts` | Create |
