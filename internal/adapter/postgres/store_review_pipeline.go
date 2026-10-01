package postgres

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/review"
)

// --- Review pipelines (KI-17) ---

// CreateReviewPipeline records a review pipeline's plan in the current tenant.
func (s *Store) CreateReviewPipeline(ctx context.Context, rp *review.Pipeline) error {
	tid := tenantFromCtx(ctx)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO review_pipelines (plan_id, tenant_id, project_id, baseline_sha)
		 VALUES ($1, $2, $3, $4)
		 RETURNING created_at`,
		rp.PlanID, tid, rp.ProjectID, rp.BaselineSHA,
	).Scan(&rp.CreatedAt)
	if err != nil {
		return fmt.Errorf("create review pipeline: %w", err)
	}
	rp.TenantID = tid
	return nil
}

// GetReviewPipeline returns the review pipeline of a plan in the current
// tenant; domain.ErrNotFound when the review pipeline did not start the plan.
func (s *Store) GetReviewPipeline(ctx context.Context, planID string) (*review.Pipeline, error) {
	var rp review.Pipeline
	err := s.pool.QueryRow(ctx,
		`SELECT plan_id, tenant_id, project_id, baseline_sha, created_at
		 FROM review_pipelines WHERE plan_id = $1 AND tenant_id = $2`,
		planID, tenantFromCtx(ctx),
	).Scan(&rp.PlanID, &rp.TenantID, &rp.ProjectID, &rp.BaselineSHA, &rp.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get review pipeline of plan %s", planID)
	}
	return &rp, nil
}
