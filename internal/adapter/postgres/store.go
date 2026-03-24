package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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

func (s *Store) UpdateTaskStatus(ctx context.Context, id string, status task.Status) error {
	tag, err := s.pool.Exec(ctx, `UPDATE tasks SET status = $2 WHERE id = $1 AND tenant_id = $3`, id, string(status), tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update task status %s", id)
}

func (s *Store) UpdateTaskResult(ctx context.Context, id string, result task.Result, costUSD float64) error {
	resultJSON, err := marshalJSON(result, "result")
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET result = $2, cost_usd = $3, status = $4 WHERE id = $1 AND tenant_id = $5`,
		id, resultJSON, costUSD, string(task.StatusCompleted), tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update task result %s", id)
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
	if agentID != nil {
		t.AgentID = *agentID
	}
	if len(resultJSON) > 0 {
		var r task.Result
		if err := unmarshalJSONField(resultJSON, &r, "result"); err != nil {
			return t, err
		}
		t.Result = &r
	}
	return t, nil
}
