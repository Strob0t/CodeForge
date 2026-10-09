package postgres

import (
	"context"
)

// CreateReviewTrigger inserts a new review trigger record and returns its generated ID.
func (s *Store) CreateReviewTrigger(ctx context.Context, projectID, commitSHA, source string) (string, error) {
	tid := tenantFromCtx(ctx)
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO review_triggers (project_id, tenant_id, commit_sha, source)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		projectID, tid, commitSHA, source,
	).Scan(&id)
	return id, err
}
