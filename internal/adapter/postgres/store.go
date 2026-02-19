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
	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
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
		`SELECT id, name, description, repo_url, provider, workspace_path, config, policy_profile, version, created_at, updated_at
		 FROM projects WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantFromCtx(ctx))
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
		`SELECT id, name, description, repo_url, provider, workspace_path, config, policy_profile, version, created_at, updated_at
		 FROM projects WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

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
		`INSERT INTO projects (tenant_id, name, description, repo_url, provider, config)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, description, repo_url, provider, workspace_path, config, policy_profile, version, created_at, updated_at`,
		tenantFromCtx(ctx), req.Name, req.Description, req.RepoURL, req.Provider, configJSON)

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
		`UPDATE projects SET name = $2, description = $3, repo_url = $4, provider = $5, workspace_path = $6, config = $7, policy_profile = $8
		 WHERE id = $1 AND version = $9 AND tenant_id = $10`,
		p.ID, p.Name, p.Description, p.RepoURL, p.Provider, p.WorkspacePath, configJSON, p.PolicyProfile, p.Version, tenantFromCtx(ctx))
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
	tag, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
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
		`SELECT id, project_id, name, backend, status, config, resource_limits, version, created_at, updated_at
		 FROM agents WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, projectID, tenantFromCtx(ctx))
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
		`SELECT id, project_id, name, backend, status, config, resource_limits, version, created_at, updated_at
		 FROM agents WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	a, err := scanAgent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get agent %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get agent %s: %w", id, err)
	}
	return &a, nil
}

func (s *Store) CreateAgent(ctx context.Context, projectID, name, backend string, config map[string]string, limits *resource.Limits) (*agent.Agent, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	var limitsJSON []byte
	if limits != nil {
		limitsJSON, err = json.Marshal(limits)
		if err != nil {
			return nil, fmt.Errorf("marshal resource_limits: %w", err)
		}
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO agents (tenant_id, project_id, name, backend, config, resource_limits)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, project_id, name, backend, status, config, resource_limits, version, created_at, updated_at`,
		tenantFromCtx(ctx), projectID, name, backend, configJSON, limitsJSON)

	a, err := scanAgent(row)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	return &a, nil
}

func (s *Store) UpdateAgentStatus(ctx context.Context, id string, status agent.Status) error {
	tag, err := s.pool.Exec(ctx, `UPDATE agents SET status = $2 WHERE id = $1 AND tenant_id = $3`, id, string(status), tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update agent status %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update agent status %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
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
		 FROM tasks WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, projectID, tenantFromCtx(ctx))
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
		 FROM tasks WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

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
		`UPDATE tasks SET result = $2, cost_usd = $3, status = $4 WHERE id = $1 AND tenant_id = $5`,
		id, resultJSON, costUSD, string(task.StatusCompleted), tenantFromCtx(ctx))
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
		`INSERT INTO runs (tenant_id, task_id, agent_id, project_id, team_id, policy_profile, exec_mode, deliver_mode, status, output)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, started_at, created_at, updated_at, version`,
		tenantFromCtx(ctx), r.TaskID, r.AgentID, r.ProjectID, nullIfEmpty(r.TeamID), r.PolicyProfile, string(r.ExecMode), string(r.DeliverMode), string(r.Status), r.Output)

	return row.Scan(&r.ID, &r.StartedAt, &r.CreatedAt, &r.UpdatedAt, &r.Version)
}

func (s *Store) GetRun(ctx context.Context, id string) (*run.Run, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, task_id, agent_id, project_id, COALESCE(team_id::text, ''), policy_profile, exec_mode, deliver_mode, status,
		        step_count, cost_usd, tokens_in, tokens_out, model, output, error, version, started_at, completed_at, created_at, updated_at
		 FROM runs WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	r, err := scanRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get run %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get run %s: %w", id, err)
	}
	return &r, nil
}

func (s *Store) UpdateRunStatus(ctx context.Context, id string, status run.Status, stepCount int, costUSD float64, tokensIn, tokensOut int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, step_count = $3, cost_usd = $4, tokens_in = $5, tokens_out = $6, updated_at = now()
		 WHERE id = $1 AND tenant_id = $7`,
		id, string(status), stepCount, costUSD, tokensIn, tokensOut, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update run status %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update run status %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) CompleteRun(ctx context.Context, id string, status run.Status, output, errMsg string, costUSD float64, stepCount int, tokensIn, tokensOut int64, model string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status = $2, output = $3, error = $4, cost_usd = $5, step_count = $6,
		 tokens_in = $7, tokens_out = $8, model = $9, completed_at = now(), updated_at = now()
		 WHERE id = $1 AND tenant_id = $10`,
		id, string(status), output, errMsg, costUSD, stepCount, tokensIn, tokensOut, model, tenantFromCtx(ctx))

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
		`SELECT id, tenant_id, task_id, agent_id, project_id, COALESCE(team_id::text, ''), policy_profile, exec_mode, deliver_mode, status,
		        step_count, cost_usd, tokens_in, tokens_out, model, output, error, version, started_at, completed_at, created_at, updated_at
		 FROM runs WHERE task_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, taskID, tenantFromCtx(ctx))
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

	tid := tenantFromCtx(ctx)

	var t agent.Team
	err = tx.QueryRow(ctx,
		`INSERT INTO agent_teams (tenant_id, project_id, name, protocol)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, project_id, name, protocol, status, version, created_at, updated_at`,
		tid, req.ProjectID, req.Name, req.Protocol,
	).Scan(&t.ID, &t.ProjectID, &t.Name, &t.Protocol, &t.Status, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert team: %w", err)
	}

	for _, m := range req.Members {
		var member agent.TeamMember
		err = tx.QueryRow(ctx,
			`INSERT INTO team_members (tenant_id, team_id, agent_id, role)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, team_id, agent_id, role`,
			tid, t.ID, m.AgentID, string(m.Role),
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
		 FROM agent_teams WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

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
		 FROM agent_teams WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, projectID, tenantFromCtx(ctx))
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
		`UPDATE agent_teams SET status = $2 WHERE id = $1 AND tenant_id = $3`,
		id, string(status), tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update team status %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update team status %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) DeleteTeam(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agent_teams WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
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
		`SELECT id, team_id, agent_id, role FROM team_members WHERE team_id = $1 AND tenant_id = $2`, teamID, tenantFromCtx(ctx))
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
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	tid := tenantFromCtx(ctx)

	// Insert plan row
	err = tx.QueryRow(ctx,
		`INSERT INTO execution_plans (tenant_id, project_id, team_id, name, description, protocol, status, max_parallel)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, version, created_at, updated_at`,
		tid, p.ProjectID, nullIfEmpty(p.TeamID), p.Name, p.Description, string(p.Protocol), string(p.Status), p.MaxParallel,
	).Scan(&p.ID, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert plan: %w", err)
	}

	// Insert steps, building index->UUID map for dependency remapping
	idMap := make(map[int]string, len(p.Steps))
	for i := range p.Steps {
		step := &p.Steps[i]
		step.PlanID = p.ID
		err = tx.QueryRow(ctx,
			`INSERT INTO plan_steps (tenant_id, plan_id, task_id, agent_id, policy_profile, deliver_mode, depends_on, status, round)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 RETURNING id, created_at, updated_at`,
			tid, step.PlanID, step.TaskID, step.AgentID, step.PolicyProfile, step.DeliverMode,
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
		 FROM execution_plans WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

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
		 FROM execution_plans WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, projectID, tenantFromCtx(ctx))
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
		`UPDATE execution_plans SET status = $2 WHERE id = $1 AND tenant_id = $3`,
		id, string(status), tenantFromCtx(ctx))
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
		`INSERT INTO plan_steps (tenant_id, plan_id, task_id, agent_id, policy_profile, deliver_mode, depends_on, status, round)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at, updated_at`,
		tenantFromCtx(ctx), step.PlanID, step.TaskID, step.AgentID, step.PolicyProfile, step.DeliverMode,
		step.DependsOn, string(step.Status), step.Round,
	).Scan(&step.ID, &step.CreatedAt, &step.UpdatedAt)
}

func (s *Store) ListPlanSteps(ctx context.Context, planID string) ([]plan.Step, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, plan_id, task_id, agent_id, policy_profile, deliver_mode, depends_on, status, run_id, round, error, created_at, updated_at
		 FROM plan_steps WHERE plan_id = $1 AND tenant_id = $2 ORDER BY created_at ASC`, planID, tenantFromCtx(ctx))
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
		 WHERE id = $1 AND tenant_id = $5`,
		stepID, string(status), runID, errMsg, tenantFromCtx(ctx))
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
		 FROM plan_steps WHERE run_id = $1 AND tenant_id = $2`, runID, tenantFromCtx(ctx))

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
		`UPDATE plan_steps SET round = $2 WHERE id = $1 AND tenant_id = $3`,
		stepID, round, tenantFromCtx(ctx))
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
	var configJSON, limitsJSON []byte
	err := row.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Backend, &a.Status, &configJSON, &limitsJSON, &a.Version, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	if configJSON != nil {
		if err := json.Unmarshal(configJSON, &a.Config); err != nil {
			return a, fmt.Errorf("unmarshal agent config: %w", err)
		}
	}
	if limitsJSON != nil {
		var limits resource.Limits
		if err := json.Unmarshal(limitsJSON, &limits); err != nil {
			return a, fmt.Errorf("unmarshal agent resource_limits: %w", err)
		}
		a.ResourceLimits = &limits
	}
	return a, nil
}

func scanProject(row scannable) (project.Project, error) {
	var p project.Project
	var configJSON []byte
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.Provider, &p.WorkspacePath, &configJSON, &p.PolicyProfile, &p.Version, &p.CreatedAt, &p.UpdatedAt)
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
		&r.ID, &r.TenantID, &r.TaskID, &r.AgentID, &r.ProjectID, &r.TeamID, &r.PolicyProfile,
		&r.ExecMode, &r.DeliverMode, &r.Status, &r.StepCount, &r.CostUSD,
		&r.TokensIn, &r.TokensOut, &r.Model,
		&r.Output, &r.Error,
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

	tid := tenantFromCtx(ctx)

	err = tx.QueryRow(ctx,
		`INSERT INTO context_packs (tenant_id, task_id, project_id, token_budget, tokens_used)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		tid, pack.TaskID, pack.ProjectID, pack.TokenBudget, pack.TokensUsed,
	).Scan(&pack.ID, &pack.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert context_pack: %w", err)
	}

	for i := range pack.Entries {
		e := &pack.Entries[i]
		e.PackID = pack.ID
		err = tx.QueryRow(ctx,
			`INSERT INTO context_entries (tenant_id, pack_id, kind, path, content, tokens, priority)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			tid, e.PackID, e.Kind, e.Path, e.Content, e.Tokens, e.Priority,
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
		 FROM context_packs WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx),
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
		 FROM context_packs WHERE task_id = $1 AND tenant_id = $2 ORDER BY created_at DESC LIMIT 1`, taskID, tenantFromCtx(ctx),
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
	tag, err := s.pool.Exec(ctx, `DELETE FROM context_packs WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
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
		 FROM context_entries WHERE pack_id = $1 AND tenant_id = $2 ORDER BY priority DESC`, packID, tenantFromCtx(ctx))
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
		`INSERT INTO shared_contexts (tenant_id, team_id, project_id) VALUES ($1, $2, $3)
		 RETURNING id, version, created_at, updated_at`,
		tenantFromCtx(ctx), sc.TeamID, sc.ProjectID,
	).Scan(&sc.ID, &sc.Version, &sc.CreatedAt, &sc.UpdatedAt)
}

// GetSharedContext returns a shared context by ID with all items.
func (s *Store) GetSharedContext(ctx context.Context, id string) (*cfcontext.SharedContext, error) {
	var sc cfcontext.SharedContext
	err := s.pool.QueryRow(ctx,
		`SELECT id, team_id, project_id, version, created_at, updated_at
		 FROM shared_contexts WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx),
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
		 FROM shared_contexts WHERE team_id = $1 AND tenant_id = $2`, teamID, tenantFromCtx(ctx),
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

	tid := tenantFromCtx(ctx)

	// Resolve shared context ID from team ID.
	var sharedID string
	err = tx.QueryRow(ctx,
		`SELECT id FROM shared_contexts WHERE team_id = $1 AND tenant_id = $2`, req.TeamID, tid,
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
		`INSERT INTO shared_context_items (tenant_id, shared_id, key, value, author, tokens)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (shared_id, key) DO UPDATE SET value = EXCLUDED.value, author = EXCLUDED.author, tokens = EXCLUDED.tokens
		 RETURNING id, shared_id, key, value, author, tokens, created_at`,
		tid, sharedID, req.Key, req.Value, req.Author, tokens,
	).Scan(&item.ID, &item.SharedID, &item.Key, &item.Value, &item.Author, &item.Tokens, &item.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert shared_context_item: %w", err)
	}

	// Bump version.
	if _, err := tx.Exec(ctx,
		`UPDATE shared_contexts SET version = version + 1 WHERE id = $1 AND tenant_id = $2`, sharedID, tid,
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
	tag, err := s.pool.Exec(ctx, `DELETE FROM shared_contexts WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete shared_context: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- Repo Maps ---

// UpsertRepoMap inserts or updates a repo map for a project.
func (s *Store) UpsertRepoMap(ctx context.Context, m *cfcontext.RepoMap) error {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO repo_maps (tenant_id, project_id, map_text, token_count, file_count, symbol_count, languages, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 1)
		 ON CONFLICT (project_id) DO UPDATE SET
		   map_text = EXCLUDED.map_text,
		   token_count = EXCLUDED.token_count,
		   file_count = EXCLUDED.file_count,
		   symbol_count = EXCLUDED.symbol_count,
		   languages = EXCLUDED.languages,
		   version = repo_maps.version + 1,
		   updated_at = now()
		 RETURNING id, version, created_at, updated_at`,
		tenantFromCtx(ctx), m.ProjectID, m.MapText, m.TokenCount, m.FileCount, m.SymbolCount, m.Languages,
	).Scan(&m.ID, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert repo_map: %w", err)
	}
	return nil
}

// GetRepoMap returns the repo map for a project.
func (s *Store) GetRepoMap(ctx context.Context, projectID string) (*cfcontext.RepoMap, error) {
	var m cfcontext.RepoMap
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, map_text, token_count, file_count, symbol_count, languages, version, created_at, updated_at
		 FROM repo_maps WHERE project_id = $1 AND tenant_id = $2`, projectID, tenantFromCtx(ctx),
	).Scan(&m.ID, &m.ProjectID, &m.MapText, &m.TokenCount, &m.FileCount, &m.SymbolCount, &m.Languages, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get repo_map for project %s: %w", projectID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get repo_map: %w", err)
	}
	return &m, nil
}

// DeleteRepoMap removes the repo map for a project.
func (s *Store) DeleteRepoMap(ctx context.Context, projectID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM repo_maps WHERE project_id = $1 AND tenant_id = $2`, projectID, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete repo_map: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete repo_map for project %s: %w", projectID, domain.ErrNotFound)
	}
	return nil
}

// --- Roadmaps ---

func (s *Store) CreateRoadmap(ctx context.Context, req roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO roadmaps (tenant_id, project_id, title, description)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, project_id, tenant_id, title, description, status, version, created_at, updated_at`,
		tenantFromCtx(ctx), req.ProjectID, req.Title, req.Description)

	r, err := scanRoadmap(row)
	if err != nil {
		return nil, fmt.Errorf("create roadmap: %w", err)
	}
	return &r, nil
}

func (s *Store) GetRoadmap(ctx context.Context, id string) (*roadmap.Roadmap, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, tenant_id, title, description, status, version, created_at, updated_at
		 FROM roadmaps WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	r, err := scanRoadmap(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get roadmap %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get roadmap %s: %w", id, err)
	}
	return &r, nil
}

func (s *Store) GetRoadmapByProject(ctx context.Context, projectID string) (*roadmap.Roadmap, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, tenant_id, title, description, status, version, created_at, updated_at
		 FROM roadmaps WHERE project_id = $1 AND tenant_id = $2`, projectID, tenantFromCtx(ctx))

	r, err := scanRoadmap(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get roadmap for project %s: %w", projectID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get roadmap for project %s: %w", projectID, err)
	}
	return &r, nil
}

func (s *Store) UpdateRoadmap(ctx context.Context, r *roadmap.Roadmap) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE roadmaps SET title = $2, description = $3, status = $4
		 WHERE id = $1 AND version = $5 AND tenant_id = $6`,
		r.ID, r.Title, r.Description, string(r.Status), r.Version, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update roadmap %s: %w", r.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update roadmap %s: %w", r.ID, domain.ErrConflict)
	}
	r.Version++
	return nil
}

func (s *Store) DeleteRoadmap(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM roadmaps WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete roadmap %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete roadmap %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// --- Milestones ---

func (s *Store) CreateMilestone(ctx context.Context, req roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO milestones (tenant_id, roadmap_id, title, description, due_date)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, roadmap_id, title, description, status, sort_order, due_date, version, created_at, updated_at`,
		tenantFromCtx(ctx), req.RoadmapID, req.Title, req.Description, req.DueDate)

	m, err := scanMilestone(row)
	if err != nil {
		return nil, fmt.Errorf("create milestone: %w", err)
	}
	return &m, nil
}

func (s *Store) GetMilestone(ctx context.Context, id string) (*roadmap.Milestone, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, roadmap_id, title, description, status, sort_order, due_date, version, created_at, updated_at
		 FROM milestones WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	m, err := scanMilestone(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get milestone %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get milestone %s: %w", id, err)
	}
	return &m, nil
}

func (s *Store) ListMilestones(ctx context.Context, roadmapID string) ([]roadmap.Milestone, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, roadmap_id, title, description, status, sort_order, due_date, version, created_at, updated_at
		 FROM milestones WHERE roadmap_id = $1 AND tenant_id = $2 ORDER BY sort_order ASC, created_at ASC`, roadmapID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list milestones: %w", err)
	}
	defer rows.Close()

	var milestones []roadmap.Milestone
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, err
		}
		milestones = append(milestones, m)
	}
	return milestones, rows.Err()
}

func (s *Store) UpdateMilestone(ctx context.Context, m *roadmap.Milestone) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE milestones SET title = $2, description = $3, status = $4, sort_order = $5, due_date = $6
		 WHERE id = $1 AND version = $7 AND tenant_id = $8`,
		m.ID, m.Title, m.Description, string(m.Status), m.SortOrder, m.DueDate, m.Version, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update milestone %s: %w", m.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update milestone %s: %w", m.ID, domain.ErrConflict)
	}
	m.Version++
	return nil
}

func (s *Store) DeleteMilestone(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM milestones WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete milestone %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete milestone %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// --- Features ---

func (s *Store) CreateFeature(ctx context.Context, req *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	externalIDsJSON, err := json.Marshal(req.ExternalIDs)
	if err != nil {
		return nil, fmt.Errorf("marshal external_ids: %w", err)
	}

	tid := tenantFromCtx(ctx)

	// Resolve roadmap_id from milestone.
	var roadmapID string
	if err := s.pool.QueryRow(ctx,
		`SELECT roadmap_id FROM milestones WHERE id = $1 AND tenant_id = $2`, req.MilestoneID, tid,
	).Scan(&roadmapID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("milestone %s: %w", req.MilestoneID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("resolve roadmap_id: %w", err)
	}

	labels := req.Labels
	if labels == nil {
		labels = []string{}
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO features (tenant_id, milestone_id, roadmap_id, title, description, labels, spec_ref, external_ids)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, milestone_id, roadmap_id, title, description, status, labels, spec_ref, external_ids, sort_order, version, created_at, updated_at`,
		tid, req.MilestoneID, roadmapID, req.Title, req.Description, labels, req.SpecRef, externalIDsJSON)

	f, err := scanFeature(row)
	if err != nil {
		return nil, fmt.Errorf("create feature: %w", err)
	}
	return &f, nil
}

func (s *Store) GetFeature(ctx context.Context, id string) (*roadmap.Feature, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, milestone_id, roadmap_id, title, description, status, labels, spec_ref, external_ids, sort_order, version, created_at, updated_at
		 FROM features WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	f, err := scanFeature(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get feature %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get feature %s: %w", id, err)
	}
	return &f, nil
}

func (s *Store) ListFeatures(ctx context.Context, milestoneID string) ([]roadmap.Feature, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, milestone_id, roadmap_id, title, description, status, labels, spec_ref, external_ids, sort_order, version, created_at, updated_at
		 FROM features WHERE milestone_id = $1 AND tenant_id = $2 ORDER BY sort_order ASC, created_at ASC`, milestoneID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	defer rows.Close()

	var features []roadmap.Feature
	for rows.Next() {
		f, err := scanFeature(rows)
		if err != nil {
			return nil, err
		}
		features = append(features, f)
	}
	return features, rows.Err()
}

func (s *Store) ListFeaturesByRoadmap(ctx context.Context, roadmapID string) ([]roadmap.Feature, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, milestone_id, roadmap_id, title, description, status, labels, spec_ref, external_ids, sort_order, version, created_at, updated_at
		 FROM features WHERE roadmap_id = $1 AND tenant_id = $2 ORDER BY sort_order ASC, created_at ASC`, roadmapID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list features by roadmap: %w", err)
	}
	defer rows.Close()

	var features []roadmap.Feature
	for rows.Next() {
		f, err := scanFeature(rows)
		if err != nil {
			return nil, err
		}
		features = append(features, f)
	}
	return features, rows.Err()
}

func (s *Store) UpdateFeature(ctx context.Context, f *roadmap.Feature) error {
	externalIDsJSON, err := json.Marshal(f.ExternalIDs)
	if err != nil {
		return fmt.Errorf("marshal external_ids: %w", err)
	}

	labels := f.Labels
	if labels == nil {
		labels = []string{}
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE features SET title = $2, description = $3, status = $4, labels = $5, spec_ref = $6, external_ids = $7, sort_order = $8
		 WHERE id = $1 AND version = $9 AND tenant_id = $10`,
		f.ID, f.Title, f.Description, string(f.Status), labels, f.SpecRef, externalIDsJSON, f.SortOrder, f.Version, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update feature %s: %w", f.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update feature %s: %w", f.ID, domain.ErrConflict)
	}
	f.Version++
	return nil
}

func (s *Store) DeleteFeature(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM features WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete feature %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete feature %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func scanRoadmap(row scannable) (roadmap.Roadmap, error) {
	var r roadmap.Roadmap
	err := row.Scan(&r.ID, &r.ProjectID, &r.TenantID, &r.Title, &r.Description, &r.Status, &r.Version, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func scanMilestone(row scannable) (roadmap.Milestone, error) {
	var m roadmap.Milestone
	err := row.Scan(&m.ID, &m.RoadmapID, &m.Title, &m.Description, &m.Status, &m.SortOrder, &m.DueDate, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func scanFeature(row scannable) (roadmap.Feature, error) {
	var f roadmap.Feature
	var externalIDsJSON []byte
	err := row.Scan(&f.ID, &f.MilestoneID, &f.RoadmapID, &f.Title, &f.Description, &f.Status, &f.Labels, &f.SpecRef, &externalIDsJSON, &f.SortOrder, &f.Version, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return f, err
	}
	if externalIDsJSON != nil {
		if err := json.Unmarshal(externalIDsJSON, &f.ExternalIDs); err != nil {
			return f, fmt.Errorf("unmarshal external_ids: %w", err)
		}
	}
	return f, nil
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
		 FROM shared_context_items WHERE shared_id = $1 AND tenant_id = $2 ORDER BY created_at`, sharedID, tenantFromCtx(ctx))
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

// --- Cost Aggregation ---

func (s *Store) CostSummaryGlobal(ctx context.Context) ([]cost.ProjectSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT r.project_id, COALESCE(p.name, ''), SUM(r.cost_usd), SUM(r.tokens_in), SUM(r.tokens_out), COUNT(*)
		 FROM runs r LEFT JOIN projects p ON r.project_id = p.id
		 WHERE r.tenant_id = $1
		 GROUP BY r.project_id, p.name
		 ORDER BY SUM(r.cost_usd) DESC`, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("cost summary global: %w", err)
	}
	defer rows.Close()

	var result []cost.ProjectSummary
	for rows.Next() {
		var ps cost.ProjectSummary
		if err := rows.Scan(&ps.ProjectID, &ps.ProjectName, &ps.TotalCostUSD, &ps.TotalTokensIn, &ps.TotalTokensOut, &ps.RunCount); err != nil {
			return nil, fmt.Errorf("scan cost summary: %w", err)
		}
		result = append(result, ps)
	}
	return result, rows.Err()
}

func (s *Store) CostSummaryByProject(ctx context.Context, projectID string) (*cost.Summary, error) {
	var cs cost.Summary
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(tokens_in), 0), COALESCE(SUM(tokens_out), 0), COUNT(*)
		 FROM runs WHERE project_id = $1 AND tenant_id = $2`, projectID, tenantFromCtx(ctx)).
		Scan(&cs.TotalCostUSD, &cs.TotalTokensIn, &cs.TotalTokensOut, &cs.RunCount)
	if err != nil {
		return nil, fmt.Errorf("cost summary by project: %w", err)
	}
	return &cs, nil
}

func (s *Store) CostByModel(ctx context.Context, projectID string) ([]cost.ModelSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT COALESCE(model, ''), SUM(cost_usd), SUM(tokens_in), SUM(tokens_out), COUNT(*)
		 FROM runs WHERE project_id = $1 AND tenant_id = $2
		 GROUP BY model ORDER BY SUM(cost_usd) DESC`, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("cost by model: %w", err)
	}
	defer rows.Close()

	var result []cost.ModelSummary
	for rows.Next() {
		var ms cost.ModelSummary
		if err := rows.Scan(&ms.Model, &ms.TotalCostUSD, &ms.TotalTokensIn, &ms.TotalTokensOut, &ms.RunCount); err != nil {
			return nil, fmt.Errorf("scan model summary: %w", err)
		}
		result = append(result, ms)
	}
	return result, rows.Err()
}

func (s *Store) CostTimeSeries(ctx context.Context, projectID string, days int) ([]cost.DailyCost, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := s.pool.Query(ctx,
		`SELECT TO_CHAR(created_at::date, 'YYYY-MM-DD'), SUM(cost_usd), SUM(tokens_in), SUM(tokens_out), COUNT(*)
		 FROM runs
		 WHERE project_id = $1 AND tenant_id = $2 AND created_at >= NOW() - ($3 || ' days')::interval
		 GROUP BY created_at::date
		 ORDER BY created_at::date`, projectID, tenantFromCtx(ctx), fmt.Sprintf("%d", days))
	if err != nil {
		return nil, fmt.Errorf("cost time series: %w", err)
	}
	defer rows.Close()

	var result []cost.DailyCost
	for rows.Next() {
		var dc cost.DailyCost
		if err := rows.Scan(&dc.Date, &dc.CostUSD, &dc.TokensIn, &dc.TokensOut, &dc.RunCount); err != nil {
			return nil, fmt.Errorf("scan daily cost: %w", err)
		}
		result = append(result, dc)
	}
	return result, rows.Err()
}

func (s *Store) RecentRunsWithCost(ctx context.Context, projectID string, limit int) ([]run.Run, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, task_id, agent_id, project_id, COALESCE(team_id::text, ''), policy_profile,
		        exec_mode, deliver_mode, status, step_count, cost_usd, tokens_in, tokens_out, model,
		        output, error, version, started_at, completed_at, created_at, updated_at
		 FROM runs WHERE project_id = $1 AND tenant_id = $2
		 ORDER BY created_at DESC LIMIT $3`, projectID, tenantFromCtx(ctx), limit)
	if err != nil {
		return nil, fmt.Errorf("recent runs with cost: %w", err)
	}
	defer rows.Close()

	var result []run.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// --- Branch Protection Rules ---

func (s *Store) CreateBranchProtectionRule(ctx context.Context, req bp.CreateRuleRequest) (*bp.ProtectionRule, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO branch_protection_rules
		 (tenant_id, project_id, branch_pattern, require_reviews, require_tests, require_lint, allow_force_push, allow_delete, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, project_id, tenant_id, branch_pattern, require_reviews, require_tests, require_lint,
		           allow_force_push, allow_delete, enabled, version, created_at, updated_at`,
		tenantFromCtx(ctx), req.ProjectID, req.BranchPattern, req.RequireReviews, req.RequireTests, req.RequireLint,
		req.AllowForcePush, req.AllowDelete, req.Enabled)

	r, err := scanBranchProtectionRule(row)
	if err != nil {
		return nil, fmt.Errorf("create branch protection rule: %w", err)
	}
	return &r, nil
}

func (s *Store) GetBranchProtectionRule(ctx context.Context, id string) (*bp.ProtectionRule, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, tenant_id, branch_pattern, require_reviews, require_tests, require_lint,
		        allow_force_push, allow_delete, enabled, version, created_at, updated_at
		 FROM branch_protection_rules WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	r, err := scanBranchProtectionRule(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get branch protection rule: %w", err)
	}
	return &r, nil
}

func (s *Store) ListBranchProtectionRules(ctx context.Context, projectID string) ([]bp.ProtectionRule, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, tenant_id, branch_pattern, require_reviews, require_tests, require_lint,
		        allow_force_push, allow_delete, enabled, version, created_at, updated_at
		 FROM branch_protection_rules WHERE project_id = $1 AND tenant_id = $2
		 ORDER BY created_at`, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list branch protection rules: %w", err)
	}
	defer rows.Close()

	var result []bp.ProtectionRule
	for rows.Next() {
		r, err := scanBranchProtectionRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) UpdateBranchProtectionRule(ctx context.Context, rule *bp.ProtectionRule) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE branch_protection_rules
		 SET branch_pattern = $1, require_reviews = $2, require_tests = $3, require_lint = $4,
		     allow_force_push = $5, allow_delete = $6, enabled = $7
		 WHERE id = $8 AND version = $9 AND tenant_id = $10`,
		rule.BranchPattern, rule.RequireReviews, rule.RequireTests, rule.RequireLint,
		rule.AllowForcePush, rule.AllowDelete, rule.Enabled,
		rule.ID, rule.Version, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update branch protection rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (s *Store) DeleteBranchProtectionRule(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM branch_protection_rules WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete branch protection rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// scanBranchProtectionRule scans a single row into a ProtectionRule.
func scanBranchProtectionRule(row pgx.Row) (bp.ProtectionRule, error) {
	var r bp.ProtectionRule
	err := row.Scan(
		&r.ID, &r.ProjectID, &r.TenantID, &r.BranchPattern,
		&r.RequireReviews, &r.RequireTests, &r.RequireLint,
		&r.AllowForcePush, &r.AllowDelete, &r.Enabled,
		&r.Version, &r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}

// --- Sessions ---

func (s *Store) CreateSession(ctx context.Context, sess *run.Session) error {
	tid := tenantFromCtx(ctx)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO sessions (tenant_id, project_id, task_id, parent_session_id, parent_run_id, current_run_id, status, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at, updated_at`,
		tid, sess.ProjectID, sess.TaskID,
		nullIfEmpty(sess.ParentSessionID), nullIfEmpty(sess.ParentRunID),
		nullIfEmpty(sess.CurrentRunID), string(sess.Status), sess.Metadata,
	).Scan(&sess.ID, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	sess.TenantID = tid
	return nil
}

func (s *Store) GetSession(ctx context.Context, id string) (*run.Session, error) {
	sess, err := scanSession(s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, project_id, task_id, COALESCE(parent_session_id::text, ''), COALESCE(parent_run_id::text, ''),
		        COALESCE(current_run_id::text, ''), status, COALESCE(metadata::text, '{}'), created_at, updated_at
		 FROM sessions WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &sess, nil
}

func (s *Store) ListSessions(ctx context.Context, projectID string) ([]run.Session, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, project_id, task_id, COALESCE(parent_session_id::text, ''), COALESCE(parent_run_id::text, ''),
		        COALESCE(current_run_id::text, ''), status, COALESCE(metadata::text, '{}'), created_at, updated_at
		 FROM sessions WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []run.Session
	for rows.Next() {
		var sess run.Session
		if err := rows.Scan(
			&sess.ID, &sess.TenantID, &sess.ProjectID, &sess.TaskID,
			&sess.ParentSessionID, &sess.ParentRunID, &sess.CurrentRunID,
			&sess.Status, &sess.Metadata, &sess.CreatedAt, &sess.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *Store) UpdateSessionStatus(ctx context.Context, id string, status run.SessionStatus, currentRunID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE sessions SET status = $1, current_run_id = $2 WHERE id = $3 AND tenant_id = $4`,
		string(status), nullIfEmpty(currentRunID), id, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("update session status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// scanSession scans a single row into a Session.
func scanSession(row pgx.Row) (run.Session, error) {
	var sess run.Session
	err := row.Scan(
		&sess.ID, &sess.TenantID, &sess.ProjectID, &sess.TaskID,
		&sess.ParentSessionID, &sess.ParentRunID, &sess.CurrentRunID,
		&sess.Status, &sess.Metadata, &sess.CreatedAt, &sess.UpdatedAt,
	)
	return sess, err
}
