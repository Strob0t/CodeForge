package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

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

	// Pass 1: Insert all steps with empty depends_on to obtain UUIDs.
	idMap := make(map[string]string, len(p.Steps))
	for i := range p.Steps {
		step := &p.Steps[i]
		step.PlanID = p.ID
		err = tx.QueryRow(ctx,
			`INSERT INTO plan_steps (tenant_id, plan_id, task_id, agent_id, policy_profile, mode_id, deliver_mode, depends_on, status, round)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 RETURNING id, created_at, updated_at`,
			tid, step.PlanID, step.TaskID, step.AgentID, step.PolicyProfile, step.ModeID, step.DeliverMode,
			[]string{}, string(step.Status), step.Round,
		).Scan(&step.ID, &step.CreatedAt, &step.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert step %d: %w", i, err)
		}
		idMap[fmt.Sprintf("%d", i)] = step.ID
	}

	// Pass 2: Remap index-based depends_on to UUIDs and update.
	for i := range p.Steps {
		step := &p.Steps[i]
		if len(step.DependsOn) == 0 {
			continue
		}
		remapped := make([]string, 0, len(step.DependsOn))
		for _, dep := range step.DependsOn {
			if uuid, ok := idMap[dep]; ok {
				remapped = append(remapped, uuid)
			} else {
				remapped = append(remapped, dep) // already a UUID
			}
		}
		step.DependsOn = remapped
		_, err = tx.Exec(ctx,
			`UPDATE plan_steps SET depends_on = $1 WHERE id = $2`,
			step.DependsOn, step.ID,
		)
		if err != nil {
			return fmt.Errorf("update step %d depends_on: %w", i, err)
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, COALESCE(team_id::text, ''), name, description, protocol, status, max_parallel, version, created_at, updated_at
		 FROM execution_plans WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	p, err := scanPlan(row)
	if err != nil {
		return nil, notFoundWrap(err, "get plan %s", id)
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
	return scanRows(rows, func(r pgx.Rows) (plan.ExecutionPlan, error) {
		return scanPlan(r)
	})
}

func (s *Store) UpdatePlanStatus(ctx context.Context, id string, status plan.Status) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE execution_plans SET status = $2 WHERE id = $1 AND tenant_id = $3`,
		id, string(status), tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update plan status %s", id)
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
		`SELECT id, plan_id, task_id, agent_id, policy_profile, mode_id, deliver_mode, depends_on, status, run_id, round, error, created_at, updated_at
		 FROM plan_steps WHERE plan_id = $1 AND tenant_id = $2 ORDER BY created_at ASC`, planID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list plan steps: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (plan.Step, error) {
		return scanPlanStep(r)
	})
}

func (s *Store) UpdatePlanStepStatus(ctx context.Context, stepID string, status plan.StepStatus, runID, errMsg string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE plan_steps SET status = $2, run_id = CASE WHEN $3 = '' THEN run_id ELSE $3::uuid END, error = $4
		 WHERE id = $1 AND tenant_id = $5`,
		stepID, string(status), runID, errMsg, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update plan step status %s", stepID)
}

func (s *Store) GetPlanStepByRunID(ctx context.Context, runID string) (*plan.Step, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, plan_id, task_id, agent_id, policy_profile, mode_id, deliver_mode, depends_on, status, run_id, round, error, created_at, updated_at
		 FROM plan_steps WHERE run_id = $1 AND tenant_id = $2`, runID, tenantFromCtx(ctx))

	st, err := scanPlanStep(row)
	if err != nil {
		return nil, notFoundWrap(err, "get plan step by run %s", runID)
	}
	return &st, nil
}

func (s *Store) UpdatePlanStepRound(ctx context.Context, stepID string, round int) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE plan_steps SET round = $2 WHERE id = $1 AND tenant_id = $3`,
		stepID, round, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update plan step round %s", stepID)
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
	err := row.Scan(&st.ID, &st.PlanID, &st.TaskID, &st.AgentID, &st.PolicyProfile, &st.ModeID, &st.DeliverMode,
		&st.DependsOn, &st.Status, &runID, &st.Round, &st.Error, &st.CreatedAt, &st.UpdatedAt)
	if runID != nil {
		st.RunID = *runID
	}
	return st, err
}
