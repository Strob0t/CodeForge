package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// Store implements database.Store using PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// DefaultListLimit is the maximum number of rows returned by unbounded list queries.
// Callers can request fewer rows but never more than this hard cap.
const DefaultListLimit = 100

// --- Agents ---

func (s *Store) ListAgents(ctx context.Context, projectID string) ([]agent.Agent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, name, backend, mode_id, status, config, resource_limits, version, created_at, updated_at,
		        total_runs, total_cost, success_rate, state, capabilities, last_active_at
		 FROM agents WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC
		 LIMIT $3`, projectID, tenantFromCtx(ctx), DefaultListLimit)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (agent.Agent, error) {
		return scanAgent(r)
	})
}

func (s *Store) GetAgent(ctx context.Context, id string) (*agent.Agent, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, backend, mode_id, status, config, resource_limits, version, created_at, updated_at,
		        total_runs, total_cost, success_rate, state, capabilities, last_active_at
		 FROM agents WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	a, err := scanAgent(row)
	if err != nil {
		return nil, notFoundWrap(err, "get agent %s", id)
	}
	return &a, nil
}

func (s *Store) CreateAgent(ctx context.Context, projectID, name, backend string, config map[string]string, limits *resource.Limits) (*agent.Agent, error) {
	configJSON, err := marshalJSON(config, "config")
	if err != nil {
		return nil, err
	}

	var limitsJSON []byte
	if limits != nil {
		limitsJSON, err = marshalJSON(limits, "resource_limits")
		if err != nil {
			return nil, err
		}
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO agents (tenant_id, project_id, name, backend, mode_id, config, resource_limits)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, project_id, name, backend, mode_id, status, config, resource_limits, version, created_at, updated_at,
		          total_runs, total_cost, success_rate, state, capabilities, last_active_at`,
		tenantFromCtx(ctx), projectID, name, backend, "", configJSON, limitsJSON)

	a, err := scanAgent(row)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	return &a, nil
}

func (s *Store) UpdateAgentStatus(ctx context.Context, id string, status agent.Status) error {
	tag, err := s.pool.Exec(ctx, `UPDATE agents SET status = $2 WHERE id = $1 AND tenant_id = $3`, id, string(status), tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update agent status %s", id)
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete agent %s", id)
}

// --- Tasks ---

func (s *Store) ListTasks(ctx context.Context, projectID string) ([]task.Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, agent_id, title, prompt, status, result, cost_usd, version, created_at, updated_at
		 FROM tasks WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (task.Task, error) {
		return scanTask(r)
	})
}

func (s *Store) GetTask(ctx context.Context, id string) (*task.Task, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, agent_id, title, prompt, status, result, cost_usd, version, created_at, updated_at
		 FROM tasks WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	t, err := scanTask(row)
	if err != nil {
		return nil, notFoundWrap(err, "get task %s", id)
	}
	return &t, nil
}

func (s *Store) CreateTask(ctx context.Context, req task.CreateRequest) (*task.Task, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO tasks (tenant_id, project_id, title, prompt)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, project_id, agent_id, title, prompt, status, result, cost_usd, version, created_at, updated_at`,
		tenantFromCtx(ctx), req.ProjectID, req.Title, req.Prompt)

	t, err := scanTask(row)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return &t, nil
}

// UpdateTaskStatus sets a task's status. Its callers set a status outside a
// dispatch (a run takes the task, a dispatch could not be published, the
// task was stopped), so the task's dispatch ends with it (see
// endingDispatch).
func (s *Store) UpdateTaskStatus(ctx context.Context, id string, status task.Status) error {
	n, err := s.updateEndingDispatch(ctx,
		`UPDATE tasks SET status = $2, dispatch_id = NULL WHERE id = $1 AND tenant_id = $3`,
		id, string(status), tenantFromCtx(ctx))
	return expectOneUpdated(n, err, "update task status %s", id)
}

// endingDispatch wraps an UPDATE of tasks that clears dispatch_id, so the
// updated task's dispatch ends: its heartbeat row is deleted in the same
// statement, and the statement returns how many tasks it updated. A task
// whose dispatch ended (a result, a stop, the watchdog) or that a run took
// must not keep the dispatch and heartbeat of its last dispatch: the
// lost-task watchdog would count that stale heartbeat for whatever runs the
// task next and fail it (S2-G fix, 1).
func endingDispatch(update string) string {
	return `WITH updated AS (` + update + ` RETURNING id),
	 ended_beat AS (DELETE FROM task_heartbeats WHERE task_id IN (SELECT id FROM updated))
	 SELECT count(*) FROM updated`
}

// updateEndingDispatch runs update through endingDispatch and returns how
// many tasks it updated.
func (s *Store) updateEndingDispatch(ctx context.Context, update string, args ...any) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, endingDispatch(update), args...).Scan(&n)
	return n, err
}

// isCurrentDispatch is the predicate on tasks that the dispatch named by
// the parameter param is the task's current one. A dispatch without ID ("",
// from before dispatches had IDs) is current only for a task never
// dispatched with an ID: a task whose dispatch ended, or that a run took,
// has no current dispatch.
func isCurrentDispatch(param string) string {
	return `(dispatch_id = ` + param + ` OR (` + param + ` = '' AND dispatch_id IS NULL AND dispatched_at IS NULL))`
}

const taskExistsSQL = `SELECT EXISTS (SELECT 1 FROM tasks WHERE id = $1 AND tenant_id = $2)`

// QueueTask queues a task for a dispatch to agentID unless it is already
// queued or running: domain.ErrConflict then, domain.ErrNotFound for an
// unknown task or one of another tenant. The status predicate decides
// between concurrent dispatches. The task records its agent, so its result
// can set the agent idle again, and the dispatch (ID and time), which its
// heartbeats name.
func (s *Store) QueueTask(ctx context.Context, id, agentID, dispatchID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET status = 'queued', agent_id = $3, dispatch_id = $4, dispatched_at = now()
		 WHERE id = $1 AND tenant_id = $2 AND status NOT IN ('queued', 'running')`,
		id, tenantFromCtx(ctx), agentID, dispatchID)
	return s.guardedUpdateResult(ctx, tag, err, taskExistsSQL, "queue task", id)
}

// UpdateTaskResult stores a task's result and cost and sets its status in one
// statement; the task's dispatch ends with it (see endingDispatch).
func (s *Store) UpdateTaskResult(ctx context.Context, id string, status task.Status, result task.Result, costUSD float64) error {
	resultJSON, err := marshalJSON(result, "result")
	if err != nil {
		return err
	}
	n, err := s.updateEndingDispatch(ctx,
		`UPDATE tasks SET result = $2, cost_usd = $3, status = $4, dispatch_id = NULL WHERE id = $1 AND tenant_id = $5`,
		id, resultJSON, costUSD, string(status), tenantFromCtx(ctx))
	return expectOneUpdated(n, err, "update task result %s", id)
}

// EndTaskDispatch ends a task's dispatch for the control plane (a lost
// worker, a dead-lettered or never accepted dispatch) only while it is the
// task's current dispatch and the task is queued or running: a result that
// arrived meanwhile and a later dispatch are never overwritten. A task
// dispatched before dispatches had IDs matches dispatchID "" (see
// isCurrentDispatch). The dispatch ends (see endingDispatch).
func (s *Store) EndTaskDispatch(ctx context.Context, id, dispatchID string, status task.Status, result task.Result) error {
	resultJSON, err := marshalJSON(result, "result")
	if err != nil {
		return err
	}
	n, err := s.updateEndingDispatch(ctx,
		`UPDATE tasks SET result = $3, status = $4, dispatch_id = NULL
		 WHERE id = $1 AND tenant_id = $2 AND status IN ('queued', 'running')
		   AND `+isCurrentDispatch("$5"),
		id, tenantFromCtx(ctx), resultJSON, string(status), dispatchID)
	if err != nil {
		return fmt.Errorf("end task dispatch %s: %w", id, err)
	}
	if n > 0 {
		return nil
	}
	return s.refusedUpdate(ctx, taskExistsSQL, "end task dispatch", id)
}

// RecordTaskResult records a worker's task result for the dispatch
// dispatchID (see database.TaskStore), in one transaction. The first result
// of a dispatch records its cost (task_result_costs, one row per task and
// dispatch) and adds it to the task's cost, the sum of its dispatches'
// costs; a repeated result of a dispatch adds nothing (S2-G fix 2, 4). The
// result of the task's current dispatch also ends the dispatch with its
// status and result.
func (s *Store) RecordTaskResult(ctx context.Context, id, dispatchID string, status task.Status, result task.Result, costUSD float64) (bool, error) {
	resultJSON, err := marshalJSON(result, "result")
	if err != nil {
		return false, err
	}
	tenantID := tenantFromCtx(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("record task result %s: begin tx: %w", id, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	tag, err := tx.Exec(ctx,
		`INSERT INTO task_result_costs (task_id, dispatch_id, tenant_id, cost_usd)
		 SELECT id, $3, tenant_id, $4 FROM tasks WHERE id = $1 AND tenant_id = $2
		 ON CONFLICT (task_id, dispatch_id) DO NOTHING`,
		id, tenantID, dispatchID, costUSD)
	if err != nil {
		return false, fmt.Errorf("record task cost %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, taskExistsSQL, id, tenantID).Scan(&exists); err != nil {
			return false, fmt.Errorf("record task result %s: %w", id, err)
		}
		if !exists {
			return false, fmt.Errorf("record task result %s: %w", id, domain.ErrNotFound)
		}
		return false, nil // a repeated result of the dispatch
	}

	var ended int64
	err = tx.QueryRow(ctx, endingDispatch(
		`UPDATE tasks SET result = $3, status = $4, cost_usd = cost_usd + $5, dispatch_id = NULL
		 WHERE id = $1 AND tenant_id = $2 AND status IN ('queued', 'running')
		   AND `+isCurrentDispatch("$6")),
		id, tenantID, resultJSON, string(status), costUSD, dispatchID).Scan(&ended)
	if err != nil {
		return false, fmt.Errorf("record task result %s: %w", id, err)
	}
	if ended == 0 {
		// Not the task's current dispatch: only its cost counts.
		if _, err := tx.Exec(ctx, `UPDATE tasks SET cost_usd = cost_usd + $3 WHERE id = $1 AND tenant_id = $2`,
			id, tenantID, costUSD); err != nil {
			return false, fmt.Errorf("record task cost %s: %w", id, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("record task result %s: commit: %w", id, err)
	}
	return ended > 0, nil
}

// --- Scanners ---

func scanAgent(row scannable) (agent.Agent, error) {
	var a agent.Agent
	var configJSON, limitsJSON, stateJSON []byte
	var caps []string
	err := row.Scan(
		&a.ID, &a.ProjectID, &a.Name, &a.Backend, &a.ModeID, &a.Status,
		&configJSON, &limitsJSON, &a.Version, &a.CreatedAt, &a.UpdatedAt,
		&a.TotalRuns, &a.TotalCost, &a.SuccessRate, &stateJSON, &caps, &a.LastActiveAt,
	)
	if err != nil {
		return a, err
	}
	if err := unmarshalJSONField(configJSON, &a.Config, "agent config"); err != nil {
		return a, err
	}
	if len(limitsJSON) > 0 {
		var limits resource.Limits
		if err := unmarshalJSONField(limitsJSON, &limits, "agent resource_limits"); err != nil {
			return a, err
		}
		a.ResourceLimits = &limits
	}
	if err := unmarshalJSONField(stateJSON, &a.State, "agent state"); err != nil {
		return a, err
	}
	a.Capabilities = caps
	return a, nil
}

func scanTask(row scannable) (task.Task, error) {
	var t task.Task
	var agentID *string
	var resultJSON []byte
	err := row.Scan(&t.ID, &t.ProjectID, &agentID, &t.Title, &t.Prompt, &t.Status, &resultJSON, &t.CostUSD, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, err
	}
	err = finishTaskScan(&t, agentID, resultJSON)
	return t, err
}

// scanDispatchedTask scans the scanTask columns followed by tenant_id and
// the dispatch ID, for the watchdog's cross-tenant queries, whose callers
// must know each task's tenant and dispatch.
func scanDispatchedTask(row scannable) (task.Task, error) {
	var t task.Task
	var agentID *string
	var resultJSON []byte
	err := row.Scan(&t.ID, &t.ProjectID, &agentID, &t.Title, &t.Prompt, &t.Status, &resultJSON, &t.CostUSD, &t.Version, &t.CreatedAt, &t.UpdatedAt, &t.TenantID, &t.DispatchID)
	if err != nil {
		return t, err
	}
	err = finishTaskScan(&t, agentID, resultJSON)
	return t, err
}

// finishTaskScan fills the nullable agent and the JSON result of a scanned task.
func finishTaskScan(t *task.Task, agentID *string, resultJSON []byte) error {
	if agentID != nil {
		t.AgentID = *agentID
	}
	if len(resultJSON) > 0 {
		var r task.Result
		if err := unmarshalJSONField(resultJSON, &r, "result"); err != nil {
			return err
		}
		t.Result = &r
	}
	return nil
}
