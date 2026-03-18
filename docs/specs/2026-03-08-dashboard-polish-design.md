# Dashboard Polish — Charts, Stats, Health Scores, Activity Feed

**Date:** 2026-03-08
**Status:** Approved
**Scope:** Frontend (SolidJS) + Backend (Go) additions

## Goal

Transform the dashboard from a plain project list into a hybrid command center:
project-centric layout enriched with real-time stats, health scores, charts, and
a smart activity timeline. The dashboard should answer two questions at a glance:
"What's happening right now?" and "How are my projects doing?"

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Layout philosophy | Project-centric with ambient stats | Projects stay the hero; stats and charts support |
| KPI count | 7 metrics in header strip | Covers financial, operational, and reliability |
| Project card richness | Full enhancement | Info-dense cards with sparklines, health, tasks |
| Health score display | Traffic light dot + hover tooltip (Option D) | Instant scan + detail on demand |
| Activity feed | Smart-prioritized + timeline + clickable | Errors first, navigable to source |
| Charts | 5 chart types in tabbed panel | Comprehensive without overwhelming |
| Charting library | @unovis/solid + @unovis/ts | Native SolidJS, CSS-variable theming, tree-shakable, 25KB |
| Project creation | Move to modal dialog | Frees dashboard space for data |

## Page Layout

```
+------------------------------------------------------------------+
|  Header: "Dashboard"                              [+ New Project] |
+------------------------------------------------------------------+
|  KPI Strip (7 stat cards, horizontal, scrollable on mobile)       |
|  [Cost Today] [Active Runs] [Success Rate] [Active Agents]       |
|  [Avg Cost/Run] [Token Usage] [Error Rate]                       |
+------------------------------------------------------------------+
|  Project Cards Grid (enhanced, 2-3 columns responsive)            |
|  Each card: health dot, stats, sparkline, agents, tasks, events   |
+---------------------------+--------------------------------------+
|  Activity Timeline (~40%) |  Charts Panel (~60%, tabbed)         |
|  Smart-prioritized        |  [Cost Trend] [Runs] [Agents]       |
|  Clickable entries        |  [Models] [Cost/Project]             |
+---------------------------+--------------------------------------+
```

## Section 1: KPI Strip

7 stat cards in a horizontal strip at the top of the page.

### Card Structure

```
+------------------+
|  ^ 12%           |   <- trend indicator vs previous period
|  $4.72           |   <- primary value, large font
|  Cost Today      |   <- label, muted text
+------------------+
```

- Trend arrow compares current period to previous (today vs yesterday, 7d vs prior 7d)
- Color: green = good direction, red = bad direction
- Inverted logic for cost/errors: going UP is red
- Responsive: horizontal scroll on small screens, wraps to 2 rows on medium

### KPI Definitions

| KPI | Calculation | Period |
|---|---|---|
| Cost Today | SUM(cost_usd) all runs today | today vs yesterday |
| Active Runs | COUNT runs with status=running | real-time |
| Success Rate | completed / (completed+failed+timeout) | 7d vs prior 7d |
| Active Agents | COUNT agents with status=running | real-time |
| Avg Cost/Run | total_cost_7d / total_runs_7d | 7d vs prior 7d |
| Token Usage | SUM(tokens_in + tokens_out) today | today vs yesterday |
| Error Rate | failed / total runs | 24h vs prior 24h |

### Backend Endpoint

`GET /api/v1/dashboard/stats`

Returns all 7 KPIs with current values and trend deltas in a single response:

```json
{
  "cost_today_usd": 4.72,
  "cost_today_delta_pct": 12.3,
  "active_runs": 3,
  "success_rate_7d_pct": 87.2,
  "success_rate_delta_pct": 4.1,
  "active_agents": 5,
  "avg_cost_per_run_usd": 2.94,
  "avg_cost_delta_pct": -8.2,
  "token_usage_today": 1200000,
  "token_usage_delta_pct": 15.0,
  "error_rate_24h_pct": 4.1,
  "error_rate_delta_pct": -2.0
}
```

## Section 2: Enhanced Project Cards

Each project card becomes a mini-dashboard with 5 data zones.

### Card Layout

```
+----------------------------------------+
|  * Project Alpha              GitHub   |  <- health dot + name + provider badge
|  E-commerce backend API                |  <- description (truncated 2 lines)
|  main - last commit 2h ago             |  <- branch + freshness
|----------------------------------------|
|  Success    Cost       Tasks           |
|  ======--   $12.40     3/7 ====--     |  <- inline progress bars
|  87%        ^ 14%      42%             |
|----------------------------------------|
|  _-~-^-v-_  cost trend (7d sparkline) |  <- mini SVG sparkline (120x24px)
|----------------------------------------|
|  2 agents active - 1 running task      |  <- agent/task summary
|  Last: "Task #3 completed" - 2h ago    |  <- most recent event
|----------------------------------------|
|  [Open Project]        [Edit] [Delete] |
+----------------------------------------+
```

### Health Dot (Option D: Traffic Light + Tooltip)

Display:
- Green dot: score 75-100 (healthy)
- Yellow dot: score 40-74 (needs attention)
- Red dot: score 0-39 (problems)

Hover tooltip shows factor breakdown:

```
+-------------------------------+
|  Health Score: 87              |
|  ----------------------------- |
|  Success rate (7d)  ====== 92% |
|  Error rate (24h)   ====== 95% |
|  Recent activity    =====- 80% |
|  Task velocity      ====-- 72% |
|  Cost stability     =====- 85% |
+-------------------------------+
```

### Health Score Formula

Computed backend-side per project:

```
score = (
    success_rate_7d     * 0.30   // completed / (completed+failed+timeout)
  + error_rate_inv_24h  * 0.25   // 1 - (failed/total), spikes hurt fast
  + activity_freshness  * 0.20   // decay: 100 if <1h, 80 if <6h, 50 if <24h, 20 if <3d, 0 if >7d
  + task_velocity       * 0.15   // completed_tasks / total_tasks (0 if no tasks)
  + cost_stability      * 0.10   // 1 - min(abs(cost_delta_pct) / 100, 1)
)
```

All factors normalized to 0-100 before weighting.

### Sparkline

7-point SVG line rendered with @unovis/solid VisLine in a tiny 120x24px container.
Shows last 7 days of daily cost. No axes, no labels -- pure trend visualization.

### Backend Endpoint

`GET /api/v1/projects/{id}/health`

```json
{
  "score": 87,
  "level": "healthy",
  "factors": {
    "success_rate": { "value": 92, "weight": 0.30 },
    "error_rate_inv": { "value": 95, "weight": 0.25 },
    "activity_freshness": { "value": 80, "weight": 0.20 },
    "task_velocity": { "value": 72, "weight": 0.15 },
    "cost_stability": { "value": 85, "weight": 0.10 }
  },
  "sparkline_7d": [1.20, 2.10, 1.80, 3.40, 2.90, 4.72, 3.10],
  "stats": {
    "success_rate_pct": 87.2,
    "total_runs_7d": 12,
    "total_cost_usd": 12.40,
    "cost_delta_pct": 14.2,
    "active_agents": 2,
    "running_tasks": 1,
    "tasks_completed": 3,
    "tasks_total": 7,
    "last_activity": "Task #3 completed",
    "last_activity_at": "2026-03-08T14:30:00Z"
  }
}
```

## Section 3: Activity Timeline

Left column of the bottom section (~40% width).

### Smart Prioritization (tier-based)

Events are sorted by priority tier first, then by recency within each tier:

| Tier | Event Types | Visual |
|---|---|---|
| 1 (top) | agent.error, run.failed, run.stall_detected | Red dot, bold text |
| 2 | run.budget_alert, run.qualitygate.failed | Yellow dot |
| 3 | run.completed, run.delivery.completed, plan.completed | Green dot |
| 4 | agent.started, run.started, task.status | Blue dot |
| 5 (bottom) | agent.step_done, agent.tool_called, info-level | Gray dot, dimmed |

### Timeline Entry Structure

```
  *---- [severity dot + colored connector line]
  |  Project Alpha - agent-coder          <- project + agent context
  |  "Tool bash failed: permission denied" <- event summary
  |  2 min ago                        [->] <- relative time + navigate button
  |
```

### Behavior

- Max 15 entries visible, "Show more..." loads next page
- WebSocket-fed: new events slide in at top with subtle CSS animation
- Click [->] navigates to relevant project detail page
- No filters on dashboard -- full filtering lives on /activity page
- Events come from existing WebSocket subscription with client-side priority sorting

## Section 4: Charts Panel

Right column of the bottom section (~60% width). Tabbed interface with 5 views.

### Tab 1: Cost Trend Line (default)

- Component: VisXYContainer + VisLine + VisAxis
- Data: daily cost aggregated across all projects
- Period toggle: 7d / 30d buttons
- Style: area fill under line, gradient from accent color to transparent
- Data source: `GET /api/v1/dashboard/charts?type=cost_trend&days=30`

### Tab 2: Run Outcomes Donut

- Component: VisSingleContainer + VisDonut
- Segments: Completed (green), Failed (red), Timeout (orange), Cancelled (gray)
- Center label: total run count
- Period: last 7 days
- Data source: `GET /api/v1/dashboard/charts?type=run_outcomes&days=7`

### Tab 3: Agent Performance Bars

- Component: VisXYContainer + VisGroupedBar + VisAxis
- Horizontal bars, one per agent, sorted by success rate descending
- Bar color: green gradient based on success percentage
- Label: agent name + percentage
- Data source: `GET /api/v1/dashboard/charts?type=agent_performance`

### Tab 4: Model Usage Pie

- Component: VisSingleContainer + VisDonut (arcWidth=0 for pie)
- Segments: one per LLM model, sized by cost
- Legend: model name + cost amount
- Data source: existing `GET /api/v1/costs` by-model aggregation

### Tab 5: Cost by Project (Horizontal Bars)

- Component: VisXYContainer + VisStackedBar + VisAxis
- Horizontal bars, one per project, sorted by total cost descending
- Sequential color palette
- Data source: existing `GET /api/v1/costs` global endpoint

### Backend Endpoint

`GET /api/v1/dashboard/charts?type={chart_type}&days={period}`

Single endpoint with `type` parameter to avoid multiple round-trips.
Returns pre-aggregated data tailored to each chart type.

## Section 5: Project Creation Modal

The inline project creation form moves out of the dashboard page into a modal dialog.

- Triggered by [+ New Project] button in the page header
- Same form content (3 tabs: remote, local, empty), same validation
- Modal overlay with backdrop blur
- Closes on success, Escape, or backdrop click
- Dashboard refreshes project list on successful creation

## Technical Specification

### New Dependency

```
npm install @unovis/ts @unovis/solid
```

- @unovis/ts: core charting engine (~25KB tree-shaken)
- @unovis/solid: SolidJS component wrappers
- CSS-variable theming integrates with existing --cf-* design tokens
- SVG rendering, no canvas -- styleable with Tailwind

### New Backend Endpoints (Go)

| Endpoint | Handler | Purpose |
|---|---|---|
| `GET /api/v1/dashboard/stats` | `handleDashboardStats` | 7 KPIs with trend deltas |
| `GET /api/v1/projects/{id}/health` | `handleProjectHealth` | Health score + breakdown |
| `GET /api/v1/dashboard/charts` | `handleDashboardCharts` | Aggregated chart data |

All endpoints are tenant-scoped (tenantFromCtx) and query existing tables
(runs, agents, tasks, costs). No new database tables or migrations needed.

### New/Modified Frontend Files

```
frontend/src/features/dashboard/
  DashboardPage.tsx              -- restructured layout (major rewrite)
  ProjectCard.tsx                -- enhanced with health, sparkline, stats
  KpiStrip.tsx                   -- new: 7 KPI cards
  HealthDot.tsx                  -- new: traffic light dot + hover tooltip
  ActivityTimeline.tsx           -- new: smart-prioritized clickable timeline
  ChartsPanel.tsx                -- new: tabbed chart container
  CreateProjectModal.tsx         -- new: extracted from DashboardPage
  charts/
    CostTrendChart.tsx           -- new: VisLine area chart
    RunOutcomesDonut.tsx         -- new: VisDonut
    AgentPerformanceBars.tsx     -- new: VisGroupedBar horizontal
    ModelUsagePie.tsx            -- new: VisDonut (arcWidth=0)
    CostByProjectBars.tsx        -- new: VisStackedBar horizontal
```

### API Client Additions

```typescript
// frontend/src/api/client.ts additions
getDashboardStats(): Promise<DashboardStats>
getProjectHealth(projectId: string): Promise<ProjectHealth>
getDashboardCharts(type: string, days?: number): Promise<ChartData>
```

### CSS Theme Integration

Unovis charts use CSS variables for colors. Map to existing design tokens:

```css
/* Chart colors via --cf-* variables */
--vis-color0: var(--cf-accent);        /* primary series */
--vis-color1: var(--cf-success);       /* success/completed */
--vis-color2: var(--cf-danger);        /* failed/errors */
--vis-color3: var(--cf-warning);       /* warnings/timeout */
--vis-color4: var(--cf-text-muted);    /* cancelled/neutral */
```

Dark/light theme switching happens instantly via CSS -- no chart re-renders.

## Responsive Behavior

| Breakpoint | Layout |
|---|---|
| Desktop (>1280px) | 3-column project grid, side-by-side timeline + charts |
| Tablet (768-1280px) | 2-column project grid, stacked timeline then charts |
| Mobile (<768px) | 1-column project grid, KPI strip scrolls horizontally, timeline + charts stacked |

## Out of Scope

- Historical health score trends (future: health over time chart)
- Customizable KPI selection (all 7 are fixed for now)
- Dashboard layout customization / drag-and-drop widgets
- Export/share dashboard as image or PDF
- Per-project cost budgets with visual burn-down
