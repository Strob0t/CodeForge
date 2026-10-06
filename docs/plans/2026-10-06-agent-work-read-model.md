# Plan: one "agent work" read model (KI-129, S9-D)

**Status:** planned (2026-10-06). **Owner decisions:** 2026-10-04 (one read model for the dashboard, the cost page and a new page), 2026-10-06 (store each chat turn's result with its completion claim; keep a turn's cost when its conversation is deleted).

## Problem

The dashboard, the cost page, the onboarding step "First agent run" and the activity timeline read only the `runs` table. Conversation turns, the most common kind of agent work, never create a `runs` row (`runID := conversationID`, `internal/service/conversation.go`, `conversation_dispatch.go`), and their cost and status are stored nowhere:

- `HandleToolCallResult` returns early for conversations (`runtime_execution.go`).
- `completeConversationRun` (`conversation_agent.go`) sends the turn's cost only to metrics and the `agui.run_finished` broadcast.

Other defects in the same area:

- The onboarding step reads `api.sessions.list`, which holds only resume, fork and conversation sessions, and is never refetched.
- The activity timeline listens for `run.completed`, `run.failed`, `run.started` and `plan.completed`. These are `agent_events` types, never WebSocket broadcasts, so nothing arrives.
- Dashboard and cost pages load once and never refetch.

## Inventory

| Work | Storage | Status | Model, cost, tokens |
|---|---|---|---|
| API run | `runs` | pending, running, quality_gate, completed, failed, timeout, cancelled | `model`, `cost_usd`, `tokens_in/out`, `step_count` |
| Conversation turn | `conversations.active_turn_id` (running), `conversation_turn_completions` (finished, exactly-once claim, migration 105) | none stored | none stored |
| Plan | `execution_plans`, `plan_steps.run_id` (latest run of a step) | plan: pending, running, completed, failed, cancelled; step adds `waiting_approval`, skipped | derived from the steps' runs |

Pending tool-call approvals live in memory per replica (`RunStateManager.pendingRequests`, key `runID:callID@tenant`; for conversations the run ID is the conversation ID). Plan-step approvals are `plan_steps.status = 'waiting_approval'`.

## Design

### Storage (migrations 128 and 129; 127 belongs to the KI-94 fix)

**Migration 128 (transactional):**

- `conversation_turn_completions` gains `project_id`, `status`, `model`, `cost_usd NUMERIC(12,6)`, `tokens_in`, `tokens_out`, `step_count`, `error`, `started_at` and `by_worker BOOLEAN NOT NULL DEFAULT true`.
- The foreign key to `conversations` no longer cascades: a deleted conversation's turns keep their cost. They are purged with the cost retention (as `runs`), and with their tenant.
- `conversations.active_turn_started_at` is set by `BeginConversationTurn`.
- `ClaimConversationTurnCompletion` takes a `conversation.TurnResult` and writes it in the same claim INSERT:
  - The row comes from `INSERT ... SELECT ... FROM conversations WHERE id = $1 AND tenant_id = $N`.
  - It uses `ON CONFLICT (conversation_id, turn_id) DO UPDATE ... WHERE NOT by_worker`.
  - "First worker completion" keeps meaning `RowsAffected() == 1`.
- The Core's own completions (lost worker, dead-lettered start) insert `by_worker = false ... ON CONFLICT DO NOTHING`. Failures are counted, and a late worker completion still wins.
- **View `agent_work`:** a `UNION ALL` of four branches:
  1. runs, with the task title and the parent plan via `plan_steps.run_id`;
  2. finished turns, with the conversation title, or "deleted conversation" when the conversation is gone;
  3. active turns;
  4. plans, with the summed cost and tokens of their step runs and the count of `waiting_approval` steps.
- Status mapping happens in the view, in one place:
  - run: pending becomes queued, quality_gate becomes running, timeout becomes failed;
  - turn: active is running, legacy rows are unknown;
  - plan: running becomes waiting_approval when a step waits.
- A view, not query-side SQL: about 15 queries need the same union and mapping. PostgreSQL inlines simple views and pushes the tenant, project and time filters into each branch.
- A materialized view would be stale on a live dashboard.

**Migration 129 (`NO TRANSACTION`, `CREATE INDEX CONCURRENTLY`, like 096/123):** indexes on

- `runs(tenant_id, created_at DESC)`
- `execution_plans(tenant_id, created_at DESC)`
- `conversation_turn_completions(tenant_id, completed_at DESC)`
- `conversation_turn_completions(project_id, completed_at)`
- `conversations(tenant_id) WHERE active_turn_id IS NOT NULL`

### Domain, port, store

- **Domain:** `internal/domain/agentwork`.
  - `Kind`: run, conversation_turn, plan; `IsLeaf` is true for run and turn.
  - `Status`: queued, running, waiting_approval, completed, failed, cancelled, unknown.
  - `Item`, `Filter`, an opaque keyset `Cursor{At, Kind, ID}` and `Summary`.
- **Port:** `AgentWorkStore { ListAgentWork; AgentWorkSummary }` in `internal/port/database`, embedded in `Store`. All full-store mocks are updated.
- **Store:** `store_agent_work.go`.
  - `tenant_id = $1` always.
  - The WHERE clause is built with appended `$N` arguments, with no catch-all `$n IS NULL OR` predicates.
  - Keyset paging `ORDER BY at DESC, kind DESC, id DESC LIMIT $N`.
  - Totals count only leaf kinds, so a plan's cost is never counted twice.

### Service and HTTP

- `AgentWorkService` validates filters and clamps the limit (default 50, max 200).
- It merges pending tool-call approval counts per run and per conversation from a new `RunStateManager.PendingApprovalCounts(tenantID)`. These counts are per replica, the same limit as `ConversationRunState`.
- Read-only routes, readable by every role like `/costs`:
  - `GET /api/v1/agent-work?project_id&kind&status&from&to&top_level&limit&cursor` returns `{items, next_cursor}`.
  - `GET /api/v1/projects/{id}/agent-work`: a foreign project gives 404.
  - `GET /api/v1/agent-work/summary`.
- Invalid filters give 400.
- OpenAPI gets the new schemas, and the existing `/costs` drift is fixed (`total_runs` vs `run_count`).

### Consumers

- **Cost and dashboard queries** (`store_cost.go`, `store_dashboard.go`, except agent performance) read `agent_work` leaf kinds instead of `runs`.
  - Response shapes stay, so the MCP cost resources are fixed too.
  - `cost.Summary` gains `conversation_run_count`.
  - "Active runs" counts running chats.
- **Health score:** the no-data case is fixed in the S9-D UI round (KI-129 item A1). This work only changes the inputs.
- **Live updates:** no new event type. A `useAgentWorkLive` hook refetches with a debounce of about 750 ms on these events:
  - `run.status`, `plan.status`, `plan.step.status`, `run.toolcall`;
  - `agui.run_started`, `agui.run_finished`, `agui.permission_request`.
- **Pages that switch to the live hook:** onboarding ("First agent run" from `agent-work?limit=1`), the activity timeline (seeded from `agent-work?limit=15`), the dashboard, and the cost page ("Recent agent work").
- **New page `/agent-work`:**
  - nav entry after Activity, plus a command palette entry;
  - a summary strip;
  - columns: kind, title, project, status, model, cost, tokens, steps, start, duration, approvals, parent plan;
  - filters in URL params, and "load more" by cursor;
  - deep links for conversations, plus new `?run=` and `?plan=` deep links on the project page;
  - EN and DE strings.

## Tests (TDD)

- **Domain:** enums, cursor round trip and invalid input.
- **Store:**
  - status mapping, each filter, keyset paging with equal timestamps;
  - plan cost = sum of its step runs, and the summary does not count the plan again;
  - a deleted conversation keeps its cost;
  - **tenant isolation**: tenant B sees nothing of tenant A, even when filtering by A's project ID.
- **Turn completion claim:**
  - the result is stored;
  - a repeat changes nothing;
  - a Core-recorded failure is overwritten once by the worker.
- **Service:** approval counts merged per tenant only.
- **HTTP:** envelope, 400 cases, limit clamp, 404 for a foreign project, viewer access.
- **Integration:** two tenants end to end; migrations 128 and 129 up and down.
- **Frontend:** page, live hook debounce, onboarding step, activity timeline.

## Commits

1. `feat(conversation)`: store each turn's result with its completion claim (migration 128 columns, claim and Core-recorded paths).
2. `feat(agentwork)`: view and indexes (128 view, 129), domain, port, store, tenant-isolation tests.
3. `feat(agentwork)`: service, approval counts, routes, OpenAPI, handler and integration tests.
4. `fix(dashboard,costs)`: dashboard and cost queries read `agent_work` (KI-129).
5. `feat(frontend)`: agent-work page, live hook, onboarding, activity, dashboard and cost refresh (KI-129).

## Risks

- **Performance:** the view must keep predicate pushdown. Check with EXPLAIN on a large seed; a top-N should be a Merge Append over the new indexes.
- **Plan cost understated:** a step links only its latest run, so earlier runs of a re-planned step show as top-level runs.
- **History gap:** turns finished before migration 128 have status unknown and cost 0.
- **Numbers change meaning:** success rate and cost per run now include chats.
- **Run ID collision:** the frontend must not match `agui.*` run IDs (conversation IDs) against `runs`.
