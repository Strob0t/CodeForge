package postgres

import (
	"context"
	"fmt"
	"time"
)

// DeleteExpiredSessions deletes sessions older than the given time in batches.
// Returns the number of rows deleted. Uses LIMIT to avoid long locks.
func (s *Store) DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tid := tenantFromCtx(ctx)

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE id IN (
		   SELECT id FROM sessions WHERE tenant_id = $1 AND created_at < $2 LIMIT $3
		 )`, tid, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredConversations deletes conversations older than the given time in batches.
// Messages are cascade-deleted via FK constraints. Returns the number of rows deleted.
func (s *Store) DeleteExpiredConversations(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tid := tenantFromCtx(ctx)

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM conversations WHERE id IN (
		   SELECT id FROM conversations WHERE tenant_id = $1 AND created_at < $2 LIMIT $3
		 )`, tid, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired conversations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredRuns deletes runs older than the given time in batches.
// Returns the number of rows deleted.
func (s *Store) DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tid := tenantFromCtx(ctx)

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM runs WHERE id IN (
		   SELECT id FROM runs WHERE tenant_id = $1 AND created_at < $2 LIMIT $3
		 )`, tid, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired runs: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredAuditEntries deletes audit log entries older than the given time in batches.
// Returns the number of rows deleted.
func (s *Store) DeleteExpiredAuditEntries(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tid := tenantFromCtx(ctx)

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM audit_log WHERE id IN (
		   SELECT id FROM audit_log WHERE tenant_id = $1 AND created_at < $2 LIMIT $3
		 )`, tid, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired audit entries: %w", err)
	}
	return tag.RowsAffected(), nil
}
