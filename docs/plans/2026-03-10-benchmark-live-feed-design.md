# Benchmark Live Feed Design

**Date:** 2026-03-10
**Status:** Approved

## Problem

When a benchmark run is in progress and the user expands the run card on the Benchmark page, they only see a pulsing progress bar and a status badge. There is no visibility into what the agent is actually doing, which tools it is calling, or whether it is stuck. The user has no way to tell if the benchmark is making progress or has hung.

## Goal

Give the user real-time structured feedback inside the expanded run card so they can see the benchmark is alive and working. The primary purpose is **liveness confidence**, not full debugging.

## Design

### Run-Card Header (always visible, collapsed)

- Animated progress bar with absolute + percentage display (e.g., "47/100 tasks (47%)")
- Real-time elapsed timer
- Cumulative cost so far
- Status icon: spinner (running), checkmark (done), X (failed)

### Expanded: Feature Accordion

Each feature/task gets its own collapsible accordion entry.

**Accordion header (always visible):**
- Feature name
- Status icon (spinner / checkmark / X)
- Elapsed time since feature started
- Cost for this feature
- Current step counter (e.g., "Step 7/50")

**Accordion body (expanded):**
A virtualized, auto-scrolling event feed showing four event types:

| Icon | Type | WebSocket Source |
|------|------|------------------|
| Robot | Agent message (what the agent thinks/plans) | `agui.text_message` |
| Wrench | Tool call + result (e.g., "pytest -> 20/25 passed") | `agui.tool_call` + `agui.tool_result` |
| Warning | Error / retry (e.g., "Rate limited, retry in 30s") | Error payloads, `agui.run_finished(failed)` |
| Chart | Model / token info | `agui.step_finished` (cost_usd, tokens) |

### Feed Behavior

- **Auto-scroll:** New events appear at the bottom, feed scrolls automatically (CI/CD log pattern).
- **Scroll-lock:** When the user manually scrolls up, auto-scroll pauses. A "Scroll to bottom" button appears.
- **Smooth scrolling:** `@tanstack/solid-virtual` provides GPU-accelerated virtualization with `transform: translateY()`.
- **Memory model:** All events stored in a SolidJS signal array. Only the ~20-30 visible entries are rendered in the DOM.
- **Event routing:** Events are matched to the correct feature accordion via `run_id` from the WebSocket payload.

### Pending Features

Pending features (not yet started) show as collapsed accordion entries with a "Pending" badge and no feed content.

## Data Flow

```
Python Worker (agent loop)
    | NATS: conversation.run.*, benchmark.*
    v
Go Core Service
    | Hub.BroadcastEvent()
    v
WebSocket Hub
    | ws://
    v
Frontend BenchmarkPage
    | onMessage() / onAGUIEvent()
    v
BenchmarkLiveFeed component
    | routes events by run_id to feature accordions
    v
Virtualized event list (auto-scroll)
```

No new backend endpoints or events are needed. All required AG-UI events (`agui.text_message`, `agui.tool_call`, `agui.tool_result`, `agui.step_started`, `agui.step_finished`) and benchmark events (`benchmark.task.started`, `benchmark.task.completed`, `benchmark.run.progress`) are already emitted by the backend.

## Technical Decisions

| Decision | Rationale |
|----------|-----------|
| `@tanstack/solid-virtual` | Headless virtualization library. Zero styling opinions, 100% compatible with Tailwind theme. Smooth scrolling via GPU-accelerated transforms. |
| No new backend endpoints | All data arrives via existing WebSocket events. No API-based lazy loading needed since events are collected in memory as they stream in. |
| Signal array as event store | Events accumulate in a SolidJS signal array (~50KB for 500 events). Virtualization ensures DOM stays light regardless of total count. |
| Auto-scroll with scroll-lock | Standard CI/CD log pattern. Users expect newest entries at the bottom with auto-follow. Manual scroll-up pauses auto-scroll. |

## Scope

### In scope
- New `BenchmarkLiveFeed` component with feature accordion + virtualized event feed
- Integration into existing expanded run card in `BenchmarkPage.tsx`
- Enhanced run-card header with progress bar, timer, cost
- `@tanstack/solid-virtual` dependency

### Out of scope
- Auto-Agent button enhancements (separate feature)
- Historical event replay from API (no new endpoints)
- Persistent event storage beyond current session
- War Room integration

## Files to Create/Modify

| File | Action |
|------|--------|
| `frontend/src/features/benchmarks/BenchmarkLiveFeed.tsx` | Create: main live feed component |
| `frontend/src/features/benchmarks/BenchmarkPage.tsx` | Modify: integrate live feed into expanded run card |
| `frontend/src/api/types.ts` | Modify: add event feed item types if needed |
| `frontend/package.json` | Modify: add `@tanstack/solid-virtual` |
