package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/domain/project"
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
		`SELECT id, name, description, repo_url, provider, config, created_at, updated_at
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
		`SELECT id, name, description, repo_url, provider, config, created_at, updated_at
		 FROM projects WHERE id = $1`, id)

	p, err := scanProject(row)
	if err != nil {
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
		 RETURNING id, name, description, repo_url, provider, config, created_at, updated_at`,
		req.Name, req.Description, req.RepoURL, req.Provider, configJSON)

	p, err := scanProject(row)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete project %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("project %s not found", id)
	}
	return nil
}

// --- Tasks ---

func (s *Store) ListTasks(ctx context.Context, projectID string) ([]task.Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, agent_id, title, prompt, status, result, cost_usd, created_at, updated_at
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
		`SELECT id, project_id, agent_id, title, prompt, status, result, cost_usd, created_at, updated_at
		 FROM tasks WHERE id = $1`, id)

	t, err := scanTask(row)
	if err != nil {
		return nil, fmt.Errorf("get task %s: %w", id, err)
	}
	return &t, nil
}

func (s *Store) CreateTask(ctx context.Context, req task.CreateRequest) (*task.Task, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO tasks (project_id, title, prompt)
		 VALUES ($1, $2, $3)
		 RETURNING id, project_id, agent_id, title, prompt, status, result, cost_usd, created_at, updated_at`,
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
		return fmt.Errorf("task %s not found", id)
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
		return fmt.Errorf("task %s not found", id)
	}
	return nil
}

// --- Scanners ---

type scannable interface {
	Scan(dest ...any) error
}

func scanProject(row scannable) (project.Project, error) {
	var p project.Project
	var configJSON []byte
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.Provider, &configJSON, &p.CreatedAt, &p.UpdatedAt)
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

func scanTask(row scannable) (task.Task, error) {
	var t task.Task
	var agentID *string
	var resultJSON []byte
	err := row.Scan(&t.ID, &t.ProjectID, &agentID, &t.Title, &t.Prompt, &t.Status, &resultJSON, &t.CostUSD, &t.CreatedAt, &t.UpdatedAt)
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
