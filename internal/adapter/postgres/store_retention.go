package postgres

import (
	"context"
	"fmt"
	"log/slog"
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
//
// The batch is selected from the statement's snapshot, so every statement
// repeats the age predicate on the rows it changes: PostgreSQL re-evaluates it
// on the newest version of a row that was written concurrently (READ
// COMMITTED), and a row that became active again in between is left alone.

// retentionLockKey is the advisory lock that lets one Go Core replica (or one
// blue-green color) sweep at a time.
const retentionLockKey = `hashtext('codeforge:retention')`

// WithRetentionLock runs sweep while this process holds the retention advisory
// lock and reports whether it got it; if another session holds it, sweep does
// not run. The lock is session-level, so it lives on a dedicated connection
// for the whole sweep (the sweep's statements use other pool connections); if
// it cannot be released, the connection is closed, which releases it.
func (s *Store) WithRetentionLock(ctx context.Context, sweep func(ctx context.Context)) (bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire retention lock connection: %w", err)
	}
	defer conn.Release()

	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(`+retentionLockKey+`)`).Scan(&acquired); err != nil {
		return false, fmt.Errorf("try retention lock: %w", err)
	}
	if !acquired {
		return false, nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(`+retentionLockKey+`)`); err != nil {
			slog.Warn("retention: unlock failed, closing the lock connection", "error", err)
			_ = conn.Conn().Close(unlockCtx)
		}
	}()

	sweep(ctx)
	return true, nil
}

// DeleteExpiredSessions deletes up to batchSize agent sessions that were last
// used before the cutoff (last_activity_at: reuse and status changes count,
// foreign key actions do not) and returns how many it deleted.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (s *Store) DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE id IN (
		   SELECT id FROM sessions WHERE last_activity_at < $1 LIMIT $2
		 ) AND last_activity_at < $1`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredConversationMessages deletes up to batchSize messages of
// conversations whose last activity is before the cutoff and returns how many
// it deleted. It runs before DeleteExpiredConversations, so deleting a
// conversation no longer cascades over an unbounded number of messages. The
// conversation rows are share-locked (skipping those being written): a
// conversation that becomes active again is either re-checked against the
// cutoff or skipped, never emptied.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (s *Store) DeleteExpiredConversationMessages(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM conversation_messages WHERE id IN (
		   SELECT m.id FROM conversation_messages m
		   JOIN conversations c ON c.id = m.conversation_id
		   WHERE c.updated_at < $1
		   LIMIT $2
		   FOR SHARE OF c SKIP LOCKED
		 )`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired conversation messages: %w", err)
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
		 ) AND updated_at < $1`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired conversations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredRuns deletes up to batchSize runs - with their cost and token
// records, which are columns of the run - that were last updated before the
// cutoff and returns how many it deleted. Nothing else goes with them: plan
// steps and sessions that reference a run only lose the reference (ON DELETE
// SET NULL, migration 095), and tables that record a run_id without a foreign
// key (agent_events, feedback_audit, ...) keep their rows.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (s *Store) DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM runs WHERE id IN (
		   SELECT id FROM runs WHERE updated_at < $1 LIMIT $2
		 ) AND updated_at < $1`, before, batchSize)
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
		 ) AND created_at < $1`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired audit entries: %w", err)
	}
	return tag.RowsAffected(), nil
}
