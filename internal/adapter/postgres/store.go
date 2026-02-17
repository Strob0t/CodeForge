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
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
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
		`INSERT INTO runs (task_id, agent_id, project_id, team_id, policy_profile, exec_mode, deliver_mode, status, output)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, started_at, created_at, updated_at, version`,
		r.TaskID, r.AgentID, r.ProjectID, nullIfEmpty(r.TeamID), r.PolicyProfile, string(r.ExecMode), string(r.DeliverMode), string(r.Status), r.Output)

	return row.Scan(&r.ID, &r.StartedAt, &r.CreatedAt, &r.UpdatedAt, &r.Version)
}

func (s *Store) GetRun(ctx context.Context, id string) (*run.Run, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, task_id, agent_id, project_id, COALESCE(team_id::text, ''), policy_profile, exec_mode, deliver_mode, status,
		        step_count, cost_usd, output, error, version, started_at, completed_at, created_at, updated_at
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

func (s *Store) CompleteRun(ctx context.Context, id string, status run.Status, output, errMsg string, costUSD float64, stepCount int) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, output = $3, error = $4, cost_usd = $5, step_count = $6, completed_at = now(), updated_at = now()
		 WHERE id = $1`,
		id, string(status), output, errMsg, costUSD, stepCount)

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
		`SELECT id, task_id, agent_id, project_id, COALESCE(team_id::text, ''), policy_profile, exec_mode, deliver_mode, status,
		        step_count, cost_usd, output, error, version, started_at, completed_at, created_at, updated_at
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

// --- Agent Teams ---

func (s *Store) CreateTeam(ctx context.Context, req agent.CreateTeamRequest) (*agent.Team, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var t agent.Team
	err = tx.QueryRow(ctx,
		`INSERT INTO agent_teams (project_id, name, protocol)
		 VALUES ($1, $2, $3)
		 RETURNING id, project_id, name, protocol, status, version, created_at, updated_at`,
		req.ProjectID, req.Name, req.Protocol,
	).Scan(&t.ID, &t.ProjectID, &t.Name, &t.Protocol, &t.Status, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert team: %w", err)
	}

	for _, m := range req.Members {
		var member agent.TeamMember
		err = tx.QueryRow(ctx,
			`INSERT INTO team_members (team_id, agent_id, role)
			 VALUES ($1, $2, $3)
			 RETURNING id, team_id, agent_id, role`,
			t.ID, m.AgentID, string(m.Role),
		).Scan(&member.ID, &member.TeamID, &member.AgentID, &member.Role)
		if err != nil {
			return nil, fmt.Errorf("insert team member: %w", err)
		}
		t.Members = append(t.Members, member)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit team: %w", err)
	}
	return &t, nil
}

func (s *Store) GetTeam(ctx context.Context, id string) (*agent.Team, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, protocol, status, version, created_at, updated_at
		 FROM agent_teams WHERE id = $1`, id)

	t, err := scanTeam(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get team %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get team %s: %w", id, err)
	}

	members, err := s.listTeamMembers(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	t.Members = members
	return &t, nil
}

func (s *Store) ListTeamsByProject(ctx context.Context, projectID string) ([]agent.Team, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, name, protocol, status, version, created_at, updated_at
		 FROM agent_teams WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	var teams []agent.Team
	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load members for each team
	for i := range teams {
		members, err := s.listTeamMembers(ctx, teams[i].ID)
		if err != nil {
			return nil, err
		}
		teams[i].Members = members
	}
	return teams, nil
}

func (s *Store) UpdateTeamStatus(ctx context.Context, id string, status agent.TeamStatus) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_teams SET status = $2 WHERE id = $1`,
		id, string(status))
	if err != nil {
		return fmt.Errorf("update team status %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update team status %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) DeleteTeam(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agent_teams WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete team %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete team %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) listTeamMembers(ctx context.Context, teamID string) ([]agent.TeamMember, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, agent_id, role FROM team_members WHERE team_id = $1`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	defer rows.Close()

	var members []agent.TeamMember
	for rows.Next() {
		var m agent.TeamMember
		if err := rows.Scan(&m.ID, &m.TeamID, &m.AgentID, &m.Role); err != nil {
			return nil, fmt.Errorf("scan team member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// --- Execution Plans ---

func (s *Store) CreatePlan(ctx context.Context, p *plan.ExecutionPlan) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op // rollback after commit is a no-op

	// Insert plan row
	err = tx.QueryRow(ctx,
		`INSERT INTO execution_plans (project_id, team_id, name, description, protocol, status, max_parallel)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, version, created_at, updated_at`,
		p.ProjectID, nullIfEmpty(p.TeamID), p.Name, p.Description, string(p.Protocol), string(p.Status), p.MaxParallel,
	).Scan(&p.ID, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert plan: %w", err)
	}

	// Insert steps, building index→UUID map for dependency remapping
	idMap := make(map[int]string, len(p.Steps))
	for i := range p.Steps {
		step := &p.Steps[i]
		step.PlanID = p.ID
		err = tx.QueryRow(ctx,
			`INSERT INTO plan_steps (plan_id, task_id, agent_id, policy_profile, deliver_mode, depends_on, status, round)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 RETURNING id, created_at, updated_at`,
			step.PlanID, step.TaskID, step.AgentID, step.PolicyProfile, step.DeliverMode,
			step.DependsOn, string(step.Status), step.Round,
		).Scan(&step.ID, &step.CreatedAt, &step.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert step %d: %w", i, err)
		}
		idMap[i] = step.ID
	}

	return tx.Commit(ctx)
}

func (s *Store) GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, COALESCE(team_id::text, ''), name, description, protocol, status, max_parallel, version, created_at, updated_at
		 FROM execution_plans WHERE id = $1`, id)

	p, err := scanPlan(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get plan %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get plan %s: %w", id, err)
	}

	steps, err := s.ListPlanSteps(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Steps = steps
	return &p, nil
}

func (s *Store) ListPlansByProject(ctx context.Context, projectID string) ([]plan.ExecutionPlan, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, COALESCE(team_id::text, ''), name, description, protocol, status, max_parallel, version, created_at, updated_at
		 FROM execution_plans WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	var plans []plan.ExecutionPlan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (s *Store) UpdatePlanStatus(ctx context.Context, id string, status plan.Status) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE execution_plans SET status = $2 WHERE id = $1`,
		id, string(status))
	if err != nil {
		return fmt.Errorf("update plan status %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update plan status %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) CreatePlanStep(ctx context.Context, step *plan.Step) error {
	return s.pool.QueryRow(ctx,
		`INSERT INTO plan_steps (plan_id, task_id, agent_id, policy_profile, deliver_mode, depends_on, status, round)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at, updated_at`,
		step.PlanID, step.TaskID, step.AgentID, step.PolicyProfile, step.DeliverMode,
		step.DependsOn, string(step.Status), step.Round,
	).Scan(&step.ID, &step.CreatedAt, &step.UpdatedAt)
}

func (s *Store) ListPlanSteps(ctx context.Context, planID string) ([]plan.Step, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, plan_id, task_id, agent_id, policy_profile, deliver_mode, depends_on, status, run_id, round, error, created_at, updated_at
		 FROM plan_steps WHERE plan_id = $1 ORDER BY created_at ASC`, planID)
	if err != nil {
		return nil, fmt.Errorf("list plan steps: %w", err)
	}
	defer rows.Close()

	var steps []plan.Step
	for rows.Next() {
		st, err := scanPlanStep(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

func (s *Store) UpdatePlanStepStatus(ctx context.Context, stepID string, status plan.StepStatus, runID, errMsg string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE plan_steps SET status = $2, run_id = CASE WHEN $3 = '' THEN run_id ELSE $3::uuid END, error = $4
		 WHERE id = $1`,
		stepID, string(status), runID, errMsg)
	if err != nil {
		return fmt.Errorf("update plan step status %s: %w", stepID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update plan step status %s: %w", stepID, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) GetPlanStepByRunID(ctx context.Context, runID string) (*plan.Step, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, plan_id, task_id, agent_id, policy_profile, deliver_mode, depends_on, status, run_id, round, error, created_at, updated_at
		 FROM plan_steps WHERE run_id = $1`, runID)

	st, err := scanPlanStep(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get plan step by run %s: %w", runID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get plan step by run %s: %w", runID, err)
	}
	return &st, nil
}

func (s *Store) UpdatePlanStepRound(ctx context.Context, stepID string, round int) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE plan_steps SET round = $2 WHERE id = $1`,
		stepID, round)
	if err != nil {
		return fmt.Errorf("update plan step round %s: %w", stepID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update plan step round %s: %w", stepID, domain.ErrNotFound)
	}
	return nil
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
		&r.ID, &r.TaskID, &r.AgentID, &r.ProjectID, &r.TeamID, &r.PolicyProfile,
		&r.ExecMode, &r.DeliverMode, &r.Status, &r.StepCount, &r.CostUSD, &r.Output, &r.Error,
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

func scanTeam(row scannable) (agent.Team, error) {
	var t agent.Team
	err := row.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Protocol, &t.Status, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func scanPlan(row scannable) (plan.ExecutionPlan, error) {
	var p plan.ExecutionPlan
	err := row.Scan(&p.ID, &p.ProjectID, &p.TeamID, &p.Name, &p.Description, &p.Protocol, &p.Status,
		&p.MaxParallel, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func scanPlanStep(row scannable) (plan.Step, error) {
	var st plan.Step
	var runID *string
	err := row.Scan(&st.ID, &st.PlanID, &st.TaskID, &st.AgentID, &st.PolicyProfile, &st.DeliverMode,
		&st.DependsOn, &st.Status, &runID, &st.Round, &st.Error, &st.CreatedAt, &st.UpdatedAt)
	if runID != nil {
		st.RunID = *runID
	}
	return st, err
}

// --- Context Packs ---

// CreateContextPack inserts a context pack and its entries in a transaction.
func (s *Store) CreateContextPack(ctx context.Context, pack *cfcontext.ContextPack) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	err = tx.QueryRow(ctx,
		`INSERT INTO context_packs (task_id, project_id, token_budget, tokens_used)
		 VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		pack.TaskID, pack.ProjectID, pack.TokenBudget, pack.TokensUsed,
	).Scan(&pack.ID, &pack.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert context_pack: %w", err)
	}

	for i := range pack.Entries {
		e := &pack.Entries[i]
		e.PackID = pack.ID
		err = tx.QueryRow(ctx,
			`INSERT INTO context_entries (pack_id, kind, path, content, tokens, priority)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			e.PackID, e.Kind, e.Path, e.Content, e.Tokens, e.Priority,
		).Scan(&e.ID)
		if err != nil {
			return fmt.Errorf("insert context_entry %d: %w", i, err)
		}
	}

	return tx.Commit(ctx)
}

// GetContextPack returns a context pack by ID with all entries.
func (s *Store) GetContextPack(ctx context.Context, id string) (*cfcontext.ContextPack, error) {
	var p cfcontext.ContextPack
	err := s.pool.QueryRow(ctx,
		`SELECT id, task_id, project_id, token_budget, tokens_used, created_at
		 FROM context_packs WHERE id = $1`, id,
	).Scan(&p.ID, &p.TaskID, &p.ProjectID, &p.TokenBudget, &p.TokensUsed, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get context_pack: %w", err)
	}

	entries, err := s.loadContextEntries(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Entries = entries
	return &p, nil
}

// GetContextPackByTask returns the context pack for a task.
func (s *Store) GetContextPackByTask(ctx context.Context, taskID string) (*cfcontext.ContextPack, error) {
	var p cfcontext.ContextPack
	err := s.pool.QueryRow(ctx,
		`SELECT id, task_id, project_id, token_budget, tokens_used, created_at
		 FROM context_packs WHERE task_id = $1 ORDER BY created_at DESC LIMIT 1`, taskID,
	).Scan(&p.ID, &p.TaskID, &p.ProjectID, &p.TokenBudget, &p.TokensUsed, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get context_pack by task: %w", err)
	}

	entries, err := s.loadContextEntries(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Entries = entries
	return &p, nil
}

// DeleteContextPack removes a context pack and its entries (CASCADE).
func (s *Store) DeleteContextPack(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM context_packs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete context_pack: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) loadContextEntries(ctx context.Context, packID string) ([]cfcontext.ContextEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, pack_id, kind, path, content, tokens, priority
		 FROM context_entries WHERE pack_id = $1 ORDER BY priority DESC`, packID)
	if err != nil {
		return nil, fmt.Errorf("load context_entries: %w", err)
	}
	defer rows.Close()

	var entries []cfcontext.ContextEntry
	for rows.Next() {
		var e cfcontext.ContextEntry
		if err := rows.Scan(&e.ID, &e.PackID, &e.Kind, &e.Path, &e.Content, &e.Tokens, &e.Priority); err != nil {
			return nil, fmt.Errorf("scan context_entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// --- Shared Context ---

// CreateSharedContext inserts a new shared context for a team.
func (s *Store) CreateSharedContext(ctx context.Context, sc *cfcontext.SharedContext) error {
	return s.pool.QueryRow(ctx,
		`INSERT INTO shared_contexts (team_id, project_id) VALUES ($1, $2)
		 RETURNING id, version, created_at, updated_at`,
		sc.TeamID, sc.ProjectID,
	).Scan(&sc.ID, &sc.Version, &sc.CreatedAt, &sc.UpdatedAt)
}

// GetSharedContext returns a shared context by ID with all items.
func (s *Store) GetSharedContext(ctx context.Context, id string) (*cfcontext.SharedContext, error) {
	var sc cfcontext.SharedContext
	err := s.pool.QueryRow(ctx,
		`SELECT id, team_id, project_id, version, created_at, updated_at
		 FROM shared_contexts WHERE id = $1`, id,
	).Scan(&sc.ID, &sc.TeamID, &sc.ProjectID, &sc.Version, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get shared_context: %w", err)
	}

	items, err := s.loadSharedContextItems(ctx, sc.ID)
	if err != nil {
		return nil, err
	}
	sc.Items = items
	return &sc, nil
}

// GetSharedContextByTeam returns the shared context for a team.
func (s *Store) GetSharedContextByTeam(ctx context.Context, teamID string) (*cfcontext.SharedContext, error) {
	var sc cfcontext.SharedContext
	err := s.pool.QueryRow(ctx,
		`SELECT id, team_id, project_id, version, created_at, updated_at
		 FROM shared_contexts WHERE team_id = $1`, teamID,
	).Scan(&sc.ID, &sc.TeamID, &sc.ProjectID, &sc.Version, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get shared_context by team: %w", err)
	}

	items, err := s.loadSharedContextItems(ctx, sc.ID)
	if err != nil {
		return nil, err
	}
	sc.Items = items
	return &sc, nil
}

// AddSharedContextItem inserts a new item and bumps the shared context version.
func (s *Store) AddSharedContextItem(ctx context.Context, req cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	// Resolve shared context ID from team ID.
	var sharedID string
	err = tx.QueryRow(ctx,
		`SELECT id FROM shared_contexts WHERE team_id = $1`, req.TeamID,
	).Scan(&sharedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("resolve shared_context: %w", err)
	}

	tokens := cfcontext.EstimateTokens(req.Value)
	var item cfcontext.SharedContextItem
	err = tx.QueryRow(ctx,
		`INSERT INTO shared_context_items (shared_id, key, value, author, tokens)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (shared_id, key) DO UPDATE SET value = EXCLUDED.value, author = EXCLUDED.author, tokens = EXCLUDED.tokens
		 RETURNING id, shared_id, key, value, author, tokens, created_at`,
		sharedID, req.Key, req.Value, req.Author, tokens,
	).Scan(&item.ID, &item.SharedID, &item.Key, &item.Value, &item.Author, &item.Tokens, &item.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert shared_context_item: %w", err)
	}

	// Bump version.
	if _, err := tx.Exec(ctx,
		`UPDATE shared_contexts SET version = version + 1 WHERE id = $1`, sharedID,
	); err != nil {
		return nil, fmt.Errorf("bump shared_context version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &item, nil
}

// DeleteSharedContext removes a shared context and its items (CASCADE).
func (s *Store) DeleteSharedContext(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM shared_contexts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete shared_context: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// nullIfEmpty returns nil for empty strings (for nullable UUID columns).
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Store) loadSharedContextItems(ctx context.Context, sharedID string) ([]cfcontext.SharedContextItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, shared_id, key, value, author, tokens, created_at
		 FROM shared_context_items WHERE shared_id = $1 ORDER BY created_at`, sharedID)
	if err != nil {
		return nil, fmt.Errorf("load shared_context_items: %w", err)
	}
	defer rows.Close()

	var items []cfcontext.SharedContextItem
	for rows.Next() {
		var item cfcontext.SharedContextItem
		if err := rows.Scan(&item.ID, &item.SharedID, &item.Key, &item.Value, &item.Author, &item.Tokens, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan shared_context_item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
