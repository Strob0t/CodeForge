package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
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

// --- Projects ---

func (s *Store) ListProjects(ctx context.Context) ([]project.Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, repo_url, provider, workspace_path, config, version, created_at, updated_at
		 FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []project.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, id string) (*project.Project, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, description, repo_url, provider, workspace_path, config, version, created_at, updated_at
		 FROM projects WHERE id = $1`, id)

	p, err := scanProject(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get project %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get project %s: %w", id, err)
	}
	return &p, nil
}

func (s *Store) CreateProject(ctx context.Context, req project.CreateRequest) (*project.Project, error) {
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO projects (name, description, repo_url, provider, config)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, name, description, repo_url, provider, workspace_path, config, version, created_at, updated_at`,
		req.Name, req.Description, req.RepoURL, req.Provider, configJSON)

	p, err := scanProject(row)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *Store) UpdateProject(ctx context.Context, p *project.Project) error {
	configJSON, err := json.Marshal(p.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE projects SET name = $2, description = $3, repo_url = $4, provider = $5, workspace_path = $6, config = $7
		 WHERE id = $1 AND version = $8`,
		p.ID, p.Name, p.Description, p.RepoURL, p.Provider, p.WorkspacePath, configJSON, p.Version)
	if err != nil {
		return fmt.Errorf("update project %s: %w", p.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update project %s: %w", p.ID, domain.ErrConflict)
	}
	p.Version++
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete project %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete project %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// --- Agents ---

func (s *Store) ListAgents(ctx context.Context, projectID string) ([]agent.Agent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, name, backend, status, config, version, created_at, updated_at
		 FROM agents WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []agent.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *Store) GetAgent(ctx context.Context, id string) (*agent.Agent, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, backend, status, config, version, created_at, updated_at
		 FROM agents WHERE id = $1`, id)

	a, err := scanAgent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get agent %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get agent %s: %w", id, err)
	}
	return &a, nil
}

func (s *Store) CreateAgent(ctx context.Context, projectID, name, backend string, config map[string]string) (*agent.Agent, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO agents (project_id, name, backend, config)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, project_id, name, backend, status, config, version, created_at, updated_at`,
		projectID, name, backend, configJSON)

	a, err := scanAgent(row)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	return &a, nil
}

func (s *Store) UpdateAgentStatus(ctx context.Context, id string, status agent.Status) error {
	tag, err := s.pool.Exec(ctx, `UPDATE agents SET status = $2 WHERE id = $1`, id, string(status))
	if err != nil {
		return fmt.Errorf("update agent status %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update agent status %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete agent %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete agent %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// --- Tasks ---

func (s *Store) ListTasks(ctx context.Context, projectID string) ([]task.Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, agent_id, title, prompt, status, result, cost_usd, version, created_at, updated_at
		 FROM tasks WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []task.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (s *Store) GetTask(ctx context.Context, id string) (*task.Task, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, agent_id, title, prompt, status, result, cost_usd, version, created_at, updated_at
		 FROM tasks WHERE id = $1`, id)

	t, err := scanTask(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get task %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get task %s: %w", id, err)
	}
	return &t, nil
}

func (s *Store) CreateTask(ctx context.Context, req task.CreateRequest) (*task.Task, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO tasks (project_id, title, prompt)
		 VALUES ($1, $2, $3)
		 RETURNING id, project_id, agent_id, title, prompt, status, result, cost_usd, version, created_at, updated_at`,
		req.ProjectID, req.Title, req.Prompt)

	t, err := scanTask(row)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return &t, nil
}

func (s *Store) UpdateTaskStatus(ctx context.Context, id string, status task.Status) error {
	tag, err := s.pool.Exec(ctx, `UPDATE tasks SET status = $2 WHERE id = $1`, id, string(status))
	if err != nil {
		return fmt.Errorf("update task status %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update task status %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) UpdateTaskResult(ctx context.Context, id string, result task.Result, costUSD float64) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET result = $2, cost_usd = $3, status = $4 WHERE id = $1`,
		id, resultJSON, costUSD, string(task.StatusCompleted))
	if err != nil {
		return fmt.Errorf("update task result %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update task result %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// --- Runs ---

func (s *Store) CreateRun(ctx context.Context, r *run.Run) error {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO runs (task_id, agent_id, project_id, policy_profile, exec_mode, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, started_at, created_at, updated_at, version`,
		r.TaskID, r.AgentID, r.ProjectID, r.PolicyProfile, string(r.ExecMode), string(r.Status))

	return row.Scan(&r.ID, &r.StartedAt, &r.CreatedAt, &r.UpdatedAt, &r.Version)
}

func (s *Store) GetRun(ctx context.Context, id string) (*run.Run, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, task_id, agent_id, project_id, policy_profile, exec_mode, status,
		        step_count, cost_usd, error, version, started_at, completed_at, created_at, updated_at
		 FROM runs WHERE id = $1`, id)

	r, err := scanRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get run %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get run %s: %w", id, err)
	}
	return &r, nil
}

func (s *Store) UpdateRunStatus(ctx context.Context, id string, status run.Status, stepCount int, costUSD float64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, step_count = $3, cost_usd = $4, updated_at = now()
		 WHERE id = $1`,
		id, string(status), stepCount, costUSD)
	if err != nil {
		return fmt.Errorf("update run status %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update run status %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) CompleteRun(ctx context.Context, id string, status run.Status, errMsg string, costUSD float64, stepCount int) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, error = $3, cost_usd = $4, step_count = $5, completed_at = now(), updated_at = now()
		 WHERE id = $1`,
		id, string(status), errMsg, costUSD, stepCount)
	if err != nil {
		return fmt.Errorf("complete run %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete run %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) ListRunsByTask(ctx context.Context, taskID string) ([]run.Run, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, task_id, agent_id, project_id, policy_profile, exec_mode, status,
		        step_count, cost_usd, error, version, started_at, completed_at, created_at, updated_at
		 FROM runs WHERE task_id = $1 ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list runs by task: %w", err)
	}
	defer rows.Close()

	var runs []run.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// --- Scanners ---

type scannable interface {
	Scan(dest ...any) error
}

func scanAgent(row scannable) (agent.Agent, error) {
	var a agent.Agent
	var configJSON []byte
	err := row.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Backend, &a.Status, &configJSON, &a.Version, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	if configJSON != nil {
		if err := json.Unmarshal(configJSON, &a.Config); err != nil {
			return a, fmt.Errorf("unmarshal agent config: %w", err)
		}
	}
	return a, nil
}

func scanProject(row scannable) (project.Project, error) {
	var p project.Project
	var configJSON []byte
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.Provider, &p.WorkspacePath, &configJSON, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	if configJSON != nil {
		if err := json.Unmarshal(configJSON, &p.Config); err != nil {
			return p, fmt.Errorf("unmarshal config: %w", err)
		}
	}
	return p, nil
}

func scanRun(row scannable) (run.Run, error) {
	var r run.Run
	err := row.Scan(
		&r.ID, &r.TaskID, &r.AgentID, &r.ProjectID, &r.PolicyProfile,
		&r.ExecMode, &r.Status, &r.StepCount, &r.CostUSD, &r.Error,
		&r.Version, &r.StartedAt, &r.CompletedAt, &r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
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
	if resultJSON != nil {
		var r task.Result
		if err := json.Unmarshal(resultJSON, &r); err != nil {
			return t, fmt.Errorf("unmarshal result: %w", err)
		}
		t.Result = &r
	}
	return t, nil
}
