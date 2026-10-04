package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// --- Review pipelines (KI-17) ---

const reviewPipelineColumns = `plan_id, tenant_id, project_id, state, baseline_sha, result_sha, step_id, run_id, impact, created_at, updated_at`

const reviewPipelineExistsSQL = `SELECT EXISTS (SELECT 1 FROM review_pipelines WHERE plan_id = $1 AND tenant_id = $2)`

// activeReviewPipelineSQL ($1 project, $2 tenant, $3 plan to leave out or
// NULL, $4 statuses of plans that have not ended, $5 undecided states): a
// review pipeline of the project whose plan has not ended or whose
// refactoring is not decided (it is measured once its worker stopped, or
// waits for keep or undo).
const activeReviewPipelineSQL = `SELECT EXISTS (
	SELECT 1 FROM review_pipelines rp JOIN execution_plans p ON p.id = rp.plan_id
	WHERE rp.project_id = $1 AND rp.tenant_id = $2 AND ($3::uuid IS NULL OR rp.plan_id <> $3::uuid)
	  AND (p.status = ANY($4) OR rp.state = ANY($5)))`

var undecidedReviewStates = []string{string(review.PipelineRefactoring), string(review.PipelineAwaitingDecision)}

// agentInUseSQL ($1 plan, $2 tenant, $3 statuses of plans that have not
// ended): an agent of the plan's steps is assigned to a step of another
// plan that has not ended.
const agentInUseSQL = `SELECT EXISTS (
	SELECT 1 FROM plan_steps s JOIN execution_plans p ON p.id = s.plan_id
	WHERE s.tenant_id = $2 AND p.id <> $1 AND p.status = ANY($3)
	  AND s.agent_id IN (SELECT agent_id FROM plan_steps WHERE plan_id = $1 AND tenant_id = $2))`

var activePlanStatuses = []string{string(plan.StatusPending), string(plan.StatusRunning)}

// HasActiveReviewPipeline reports whether the project has a review pipeline
// whose plan has not ended or whose refactoring waits for a decision.
func (s *Store) HasActiveReviewPipeline(ctx context.Context, projectID string) (bool, error) {
	var active bool
	err := s.pool.QueryRow(ctx, activeReviewPipelineSQL,
		projectID, tenantFromCtx(ctx), nil, activePlanStatuses, undecidedReviewStates).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("check active review pipelines: %w", err)
	}
	return active, nil
}

// CreateReviewPipeline records a review pipeline's plan in the current
// tenant, in state pending unless rp names another. It is the guard of one
// review pipeline per project (S6-F 7): under a lock on the project row it
// refuses, with domain.ErrConflict, a project that already has an active
// review pipeline (review.ErrPipelineActive) and a plan whose agent is
// assigned to another plan that has not ended (review.ErrAgentInUse).
func (s *Store) CreateReviewPipeline(ctx context.Context, rp *review.Pipeline) error {
	tid := tenantFromCtx(ctx)
	if rp.State == "" {
		rp.State = review.PipelinePending
	}
	impact, err := marshalImpact(rp.Impact)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var locked bool
	if err := tx.QueryRow(ctx, `SELECT true FROM projects WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
		rp.ProjectID, tid).Scan(&locked); err != nil {
		return notFoundWrap(err, "lock project %s for a review pipeline", rp.ProjectID)
	}
	var active, agentBusy bool
	if err := tx.QueryRow(ctx, activeReviewPipelineSQL,
		rp.ProjectID, tid, rp.PlanID, activePlanStatuses, undecidedReviewStates).Scan(&active); err != nil {
		return fmt.Errorf("check active review pipelines: %w", err)
	}
	if active {
		return fmt.Errorf("create review pipeline: %w: %w", domain.ErrConflict, review.ErrPipelineActive)
	}
	if err := tx.QueryRow(ctx, agentInUseSQL, rp.PlanID, tid, activePlanStatuses).Scan(&agentBusy); err != nil {
		return fmt.Errorf("check the review pipeline's agent: %w", err)
	}
	if agentBusy {
		return fmt.Errorf("create review pipeline: %w: %w", domain.ErrConflict, review.ErrAgentInUse)
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO review_pipelines (plan_id, tenant_id, project_id, state, baseline_sha, result_sha, step_id, run_id, impact)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING created_at, updated_at`,
		rp.PlanID, tid, rp.ProjectID, string(rp.State), rp.BaselineSHA, rp.ResultSHA, rp.StepID, rp.RunID, impact,
	).Scan(&rp.CreatedAt, &rp.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create review pipeline: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit review pipeline: %w", err)
	}
	rp.TenantID = tid
	return nil
}

// GetReviewPipeline returns the review pipeline of a plan in the current
// tenant; domain.ErrNotFound when the review pipeline did not start the plan.
func (s *Store) GetReviewPipeline(ctx context.Context, planID string) (*review.Pipeline, error) {
	rp, err := scanReviewPipeline(s.pool.QueryRow(ctx,
		`SELECT `+reviewPipelineColumns+` FROM review_pipelines WHERE plan_id = $1 AND tenant_id = $2`,
		planID, tenantFromCtx(ctx)))
	if err != nil {
		return nil, notFoundWrap(err, "get review pipeline of plan %s", planID)
	}
	return rp, nil
}

// UpdateReviewPipeline stores rp's state, commits, step, run and impact if
// the record is still in state from (compare-and-swap): domain.ErrConflict
// when it moved on, domain.ErrNotFound when there is none in the tenant.
func (s *Store) UpdateReviewPipeline(ctx context.Context, rp *review.Pipeline, from review.PipelineState) error {
	impact, err := marshalImpact(rp.Impact)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE review_pipelines
		 SET state = $3, baseline_sha = $4, result_sha = $5, step_id = $6, run_id = $7, impact = $8, updated_at = now()
		 WHERE plan_id = $1 AND tenant_id = $2 AND state = $9`,
		rp.PlanID, tenantFromCtx(ctx), string(rp.State), rp.BaselineSHA, rp.ResultSHA, rp.StepID, rp.RunID, impact, string(from))
	return s.guardedUpdateResult(ctx, tag, err, reviewPipelineExistsSQL, "update review pipeline of plan", rp.PlanID)
}

// ListPendingReviewDecisions returns the review pipelines of a project in the
// current tenant whose refactoring waits for a keep or undo decision, oldest
// first.
func (s *Store) ListPendingReviewDecisions(ctx context.Context, projectID string) ([]review.Pipeline, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+reviewPipelineColumns+` FROM review_pipelines
		 WHERE project_id = $1 AND tenant_id = $2 AND state = $3
		 ORDER BY updated_at, plan_id`,
		projectID, tenantFromCtx(ctx), string(review.PipelineAwaitingDecision))
	if err != nil {
		return nil, fmt.Errorf("list pending review decisions: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (review.Pipeline, error) {
		rp, err := scanReviewPipeline(r)
		if err != nil {
			return review.Pipeline{}, err
		}
		return *rp, nil
	})
}

// ListEndedReviewRefactorings returns up to limit review pipelines whose
// refactoring started but is not decided although their plan ended before
// endedBefore (S6-F review 4), oldest first.
//
// INTENTIONALLY CROSS-TENANT: stuck-work watchdog check; each row carries its
// tenant and is handled in that tenant's context.
func (s *Store) ListEndedReviewRefactorings(ctx context.Context, endedBefore time.Time, limit int) ([]database.EndedReviewRefactoring, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT rp.plan_id, rp.tenant_id, p.status
		 FROM review_pipelines rp JOIN execution_plans p ON p.id = rp.plan_id AND p.tenant_id = rp.tenant_id
		 WHERE rp.state = $1 AND p.status = ANY($2) AND p.updated_at < $3
		 ORDER BY p.updated_at LIMIT $4`,
		string(review.PipelineRefactoring), planTerminalStatuses, endedBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("list ended review refactorings: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (database.EndedReviewRefactoring, error) {
		var e database.EndedReviewRefactoring
		err := r.Scan(&e.PlanID, &e.TenantID, &e.PlanStatus)
		return e, err
	})
}

func marshalImpact(impact *review.Impact) ([]byte, error) {
	if impact == nil {
		return nil, nil
	}
	data, err := json.Marshal(impact)
	if err != nil {
		return nil, fmt.Errorf("marshal review impact: %w", err)
	}
	return data, nil
}

func scanReviewPipeline(row pgx.Row) (*review.Pipeline, error) {
	var rp review.Pipeline
	var state string
	var impact []byte
	if err := row.Scan(&rp.PlanID, &rp.TenantID, &rp.ProjectID, &state, &rp.BaselineSHA, &rp.ResultSHA,
		&rp.StepID, &rp.RunID, &impact, &rp.CreatedAt, &rp.UpdatedAt); err != nil {
		return nil, err
	}
	rp.State = review.PipelineState(state)
	if impact != nil {
		rp.Impact = &review.Impact{}
		if err := json.Unmarshal(impact, rp.Impact); err != nil {
			return nil, fmt.Errorf("unmarshal review impact: %w", err)
		}
	}
	return &rp, nil
}
