package postgres

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/review"
)

// CreateReviewPolicy inserts a new review policy.
func (s *Store) CreateReviewPolicy(ctx context.Context, p *review.ReviewPolicy) error {
	const q = `INSERT INTO review_policies
		(id, project_id, tenant_id, name, trigger_type, commit_threshold, cron_expr,
		 branch_pattern, template_id, enabled, commit_counter, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
	_, err := s.pool.Exec(ctx, q,
		p.ID, p.ProjectID, p.TenantID, p.Name, string(p.TriggerType),
		p.CommitThreshold, p.CronExpr, p.BranchPattern, p.TemplateID,
		p.Enabled, p.CommitCounter, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

// GetReviewPolicy retrieves a review policy by ID.
func (s *Store) GetReviewPolicy(ctx context.Context, id string) (*review.ReviewPolicy, error) {
	tid := tenantFromCtx(ctx)
	const q = `SELECT id, project_id, tenant_id, name, trigger_type, commit_threshold,
		cron_expr, branch_pattern, template_id, enabled, commit_counter, created_at, updated_at
		FROM review_policies WHERE id = $1 AND tenant_id = $2`
	p := &review.ReviewPolicy{}
	err := s.pool.QueryRow(ctx, q, id, tid).Scan(
		&p.ID, &p.ProjectID, &p.TenantID, &p.Name, &p.TriggerType,
		&p.CommitThreshold, &p.CronExpr, &p.BranchPattern, &p.TemplateID,
		&p.Enabled, &p.CommitCounter, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get review policy %s", id)
	}
	return p, nil
}

// ListReviewPoliciesByProject returns all review policies for a project.
func (s *Store) ListReviewPoliciesByProject(ctx context.Context, projectID string) ([]review.ReviewPolicy, error) {
	tid := tenantFromCtx(ctx)
	const q = `SELECT id, project_id, tenant_id, name, trigger_type, commit_threshold,
		cron_expr, branch_pattern, template_id, enabled, commit_counter, created_at, updated_at
		FROM review_policies WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, projectID, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []review.ReviewPolicy
	for rows.Next() {
		var p review.ReviewPolicy
		if err := rows.Scan(
			&p.ID, &p.ProjectID, &p.TenantID, &p.Name, &p.TriggerType,
			&p.CommitThreshold, &p.CronExpr, &p.BranchPattern, &p.TemplateID,
			&p.Enabled, &p.CommitCounter, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// UpdateReviewPolicy updates a review policy.
func (s *Store) UpdateReviewPolicy(ctx context.Context, p *review.ReviewPolicy) error {
	tid := tenantFromCtx(ctx)
	const q = `UPDATE review_policies SET
		name=$2, trigger_type=$3, commit_threshold=$4, cron_expr=$5,
		branch_pattern=$6, template_id=$7, enabled=$8, updated_at=$9
		WHERE id=$1 AND tenant_id=$10`
	_, err := s.pool.Exec(ctx, q,
		p.ID, p.Name, string(p.TriggerType), p.CommitThreshold,
		p.CronExpr, p.BranchPattern, p.TemplateID, p.Enabled, p.UpdatedAt, tid,
	)
	return err
}

// DeleteReviewPolicy deletes a review policy by ID.
func (s *Store) DeleteReviewPolicy(ctx context.Context, id string) error {
	tid := tenantFromCtx(ctx)
	_, err := s.pool.Exec(ctx, `DELETE FROM review_policies WHERE id = $1 AND tenant_id = $2`, id, tid)
	return err
}

// ListEnabledPoliciesByTrigger returns all enabled policies of the given trigger type.
func (s *Store) ListEnabledPoliciesByTrigger(ctx context.Context, triggerType review.TriggerType) ([]review.ReviewPolicy, error) {
	tid := tenantFromCtx(ctx)
	const q = `SELECT id, project_id, tenant_id, name, trigger_type, commit_threshold,
		cron_expr, branch_pattern, template_id, enabled, commit_counter, created_at, updated_at
		FROM review_policies WHERE trigger_type = $1 AND enabled = true AND tenant_id = $2`
	rows, err := s.pool.Query(ctx, q, string(triggerType), tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []review.ReviewPolicy
	for rows.Next() {
		var p review.ReviewPolicy
		if err := rows.Scan(
			&p.ID, &p.ProjectID, &p.TenantID, &p.Name, &p.TriggerType,
			&p.CommitThreshold, &p.CronExpr, &p.BranchPattern, &p.TemplateID,
			&p.Enabled, &p.CommitCounter, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// IncrementCommitCounter atomically increments a policy's commit counter and returns the new value.
func (s *Store) IncrementCommitCounter(ctx context.Context, policyID string, count int) (int, error) {
	tid := tenantFromCtx(ctx)
	var newCounter int
	err := s.pool.QueryRow(ctx,
		`UPDATE review_policies SET commit_counter = commit_counter + $2 WHERE id = $1 AND tenant_id = $3 RETURNING commit_counter`,
		policyID, count, tid,
	).Scan(&newCounter)
	return newCounter, err
}

// ResetCommitCounter resets a policy's commit counter to zero.
func (s *Store) ResetCommitCounter(ctx context.Context, policyID string) error {
	tid := tenantFromCtx(ctx)
	_, err := s.pool.Exec(ctx,
		`UPDATE review_policies SET commit_counter = 0 WHERE id = $1 AND tenant_id = $2`, policyID, tid)
	return err
}

// CreateReview inserts a new review.
func (s *Store) CreateReview(ctx context.Context, r *review.Review) error {
	const q = `INSERT INTO reviews
		(id, policy_id, project_id, tenant_id, plan_id, status, trigger_ref, created_at, completed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	_, err := s.pool.Exec(ctx, q,
		r.ID, r.PolicyID, r.ProjectID, r.TenantID, r.PlanID,
		string(r.Status), r.TriggerRef, r.CreatedAt, r.CompletedAt,
	)
	return err
}

// GetReview retrieves a review by ID.
func (s *Store) GetReview(ctx context.Context, id string) (*review.Review, error) {
	tid := tenantFromCtx(ctx)
	const q = `SELECT id, policy_id, project_id, tenant_id, plan_id, status,
		trigger_ref, created_at, completed_at
		FROM reviews WHERE id = $1 AND tenant_id = $2`
	r := &review.Review{}
	err := s.pool.QueryRow(ctx, q, id, tid).Scan(
		&r.ID, &r.PolicyID, &r.ProjectID, &r.TenantID, &r.PlanID,
		&r.Status, &r.TriggerRef, &r.CreatedAt, &r.CompletedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get review %s", id)
	}
	return r, nil
}

// ListReviewsByProject returns all reviews for a project, most recent first.
func (s *Store) ListReviewsByProject(ctx context.Context, projectID string) ([]review.Review, error) {
	tid := tenantFromCtx(ctx)
	const q = `SELECT id, policy_id, project_id, tenant_id, plan_id, status,
		trigger_ref, created_at, completed_at
		FROM reviews WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, projectID, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []review.Review
	for rows.Next() {
		var r review.Review
		if err := rows.Scan(
			&r.ID, &r.PolicyID, &r.ProjectID, &r.TenantID, &r.PlanID,
			&r.Status, &r.TriggerRef, &r.CreatedAt, &r.CompletedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UpdateReviewStatus updates a review's status and optional completion time.
func (s *Store) UpdateReviewStatus(ctx context.Context, id string, status review.Status, completedAt *time.Time) error {
	tid := tenantFromCtx(ctx)
	_, err := s.pool.Exec(ctx,
		`UPDATE reviews SET status=$2, completed_at=$3 WHERE id=$1 AND tenant_id=$4`,
		id, string(status), completedAt, tid,
	)
	return err
}

// GetReviewByPlanID retrieves the review linked to a given plan ID.
func (s *Store) GetReviewByPlanID(ctx context.Context, planID string) (*review.Review, error) {
	tid := tenantFromCtx(ctx)
	const q = `SELECT id, policy_id, project_id, tenant_id, plan_id, status,
		trigger_ref, created_at, completed_at
		FROM reviews WHERE plan_id = $1 AND tenant_id = $2`
	r := &review.Review{}
	err := s.pool.QueryRow(ctx, q, planID, tid).Scan(
		&r.ID, &r.PolicyID, &r.ProjectID, &r.TenantID, &r.PlanID,
		&r.Status, &r.TriggerRef, &r.CreatedAt, &r.CompletedAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get review by plan %s", planID)
	}
	return r, nil
}
