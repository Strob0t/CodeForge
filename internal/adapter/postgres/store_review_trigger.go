package postgres

import (
	"context"
	"time"
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

// FindRecentReviewTrigger returns true if a trigger for the given project and commit SHA
// was created within the specified duration.
func (s *Store) FindRecentReviewTrigger(ctx context.Context, projectID, commitSHA string, within time.Duration) (bool, error) {
	tid := tenantFromCtx(ctx)
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM review_triggers
			WHERE project_id = $1 AND tenant_id = $2 AND commit_sha = $3
			AND triggered_at > $4
		)`,
		projectID, tid, commitSHA, time.Now().Add(-within),
	).Scan(&exists)
	return exists, err
}
