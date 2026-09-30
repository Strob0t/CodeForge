package postgres

import (
	"context"
	"fmt"
	"time"
)

// Retention queries (GDPR Art. 5(1)(e), docs/data-retention.md) back the
// RetentionService system job.
//
// INTENTIONALLY CROSS-TENANT, for every method in this file: the retention
// policy is one instance-wide configuration (`retention.*`), so the cutoff is
// the same for every tenant, and these tables carry tenant_id without a
// foreign key to tenants - a sweep over the tenant list could miss rows. The
// only predicate is the row's age against the cutoff the caller computed from
// the policy, and each call removes at most batchSize rows (the caller loops),
// so a call cannot remove more than the policy allows or hold long locks.

// DeleteExpiredSessions deletes up to batchSize agent sessions that were last
// updated before the cutoff (idle age, so a session in use is kept) and
// returns how many it deleted.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (s *Store) DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE id IN (
		   SELECT id FROM sessions WHERE updated_at < $1 LIMIT $2
		 )`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredConversations deletes up to batchSize conversations whose last
// activity (updated_at, bumped by every new message) is before the cutoff and
// returns how many it deleted. Messages go with them (ON DELETE CASCADE), and
// so do sessions that belong only to such a conversation (trigger of migration
// 092); sessions that also belong to a task are kept, detached.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (s *Store) DeleteExpiredConversations(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM conversations WHERE id IN (
		   SELECT id FROM conversations WHERE updated_at < $1 LIMIT $2
		 )`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired conversations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredRuns deletes up to batchSize runs - with their cost and token
// records - that were last updated before the cutoff and returns how many it
// deleted.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (s *Store) DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM runs WHERE id IN (
		   SELECT id FROM runs WHERE updated_at < $1 LIMIT $2
		 )`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired runs: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredAuditEntries deletes up to batchSize audit log entries created
// before the cutoff and returns how many it deleted.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (s *Store) DeleteExpiredAuditEntries(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM audit_log WHERE id IN (
		   SELECT id FROM audit_log WHERE created_at < $1 LIMIT $2
		 )`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired audit entries: %w", err)
	}
	return tag.RowsAffected(), nil
}
