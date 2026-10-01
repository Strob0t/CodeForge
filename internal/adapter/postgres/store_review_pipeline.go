package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/review"
)

// --- Review pipelines (KI-17) ---

const reviewPipelineColumns = `plan_id, tenant_id, project_id, state, baseline_sha, result_sha, step_id, run_id, impact, created_at, updated_at`

const reviewPipelineExistsSQL = `SELECT EXISTS (SELECT 1 FROM review_pipelines WHERE plan_id = $1 AND tenant_id = $2)`

// CreateReviewPipeline records a review pipeline's plan in the current
// tenant, in state pending unless rp names another.
func (s *Store) CreateReviewPipeline(ctx context.Context, rp *review.Pipeline) error {
	tid := tenantFromCtx(ctx)
	if rp.State == "" {
		rp.State = review.PipelinePending
	}
	impact, err := marshalImpact(rp.Impact)
	if err != nil {
		return err
	}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO review_pipelines (plan_id, tenant_id, project_id, state, baseline_sha, result_sha, step_id, run_id, impact)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING created_at, updated_at`,
		rp.PlanID, tid, rp.ProjectID, string(rp.State), rp.BaselineSHA, rp.ResultSHA, rp.StepID, rp.RunID, impact,
	).Scan(&rp.CreatedAt, &rp.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create review pipeline: %w", err)
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
