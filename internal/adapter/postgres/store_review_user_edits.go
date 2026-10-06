package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/review"
)

// --- User edits during a review refactoring (KI-94) ---

// recordReviewUserEditsSQL ($1 project, $2 tenant, $3 paths, $4 operation,
// $5 user or NULL, $6 state refactoring) records the paths for each review
// pipeline of the project whose refactoring is not measured yet, one row per
// path. FOR SHARE orders it with the measurement: a pipeline that leaves
// state refactoring meanwhile either waits for the insert or is seen in its
// new state, and then nothing is recorded.
const recordReviewUserEditsSQL = `
	INSERT INTO review_user_edits (plan_id, path, tenant_id, step_id, operation, user_id)
	SELECT rp.plan_id, p.path, rp.tenant_id, rp.step_id, $4, $5
	FROM review_pipelines rp CROSS JOIN (SELECT DISTINCT unnest($3::text[])) AS p(path)
	WHERE rp.project_id = $1 AND rp.tenant_id = $2 AND rp.state = $6
	FOR SHARE OF rp
	ON CONFLICT (plan_id, path) DO UPDATE
	SET step_id = EXCLUDED.step_id, operation = EXCLUDED.operation, user_id = EXCLUDED.user_id, edited_at = now()`

// RecordReviewUserEdits records that userID changes paths of the project
// through the editor or the file API, for each review pipeline of the
// project in the current tenant whose refactoring is not measured yet
// (state refactoring); nothing when there is none. A path recorded before
// keeps one row with its latest change. A user without an account
// (user.IsAccountless) is recorded without one; a user whose account is gone
// gets user.ErrAccountGone.
func (s *Store) RecordReviewUserEdits(ctx context.Context, projectID, userID string, op review.UserEditOp, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, recordReviewUserEditsSQL,
		projectID, tenantFromCtx(ctx), paths, string(op), accountRef(userID), string(review.PipelineRefactoring))
	if err = accountGone(err, "review_user_edits_user_id_fkey"); err != nil {
		return fmt.Errorf("record review user edits of project %s: %w", projectID, err)
	}
	return nil
}

// ListReviewUserEdits returns up to limit of the user edits recorded for the
// review pipeline of a plan in the current tenant, by path, with the user's
// current name, and how many there are in all.
func (s *Store) ListReviewUserEdits(ctx context.Context, planID string, limit int) ([]review.UserEdit, int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT e.path, e.operation, COALESCE(e.user_id::text, ''), COALESCE(u.name, ''), e.edited_at, COUNT(*) OVER ()
		 FROM review_user_edits e LEFT JOIN users u ON u.id = e.user_id AND u.tenant_id = e.tenant_id
		 WHERE e.plan_id = $1 AND e.tenant_id = $2
		 ORDER BY e.path
		 LIMIT $3`,
		planID, tenantFromCtx(ctx), limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list review user edits of plan %s: %w", planID, err)
	}
	total := 0
	edits, err := scanRows(rows, func(r pgx.Rows) (review.UserEdit, error) {
		var e review.UserEdit
		var op string
		err := r.Scan(&e.Path, &op, &e.UserID, &e.UserName, &e.EditedAt, &total)
		e.Operation = review.UserEditOp(op)
		return e, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list review user edits of plan %s: %w", planID, err)
	}
	return edits, total, nil
}

// DeleteReviewUserEdits deletes the user edits recorded for the review
// pipeline of a plan in the current tenant (its refactoring was decided).
func (s *Store) DeleteReviewUserEdits(ctx context.Context, planID string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM review_user_edits WHERE plan_id = $1 AND tenant_id = $2`,
		planID, tenantFromCtx(ctx)); err != nil {
		return fmt.Errorf("delete review user edits of plan %s: %w", planID, err)
	}
	return nil
}
