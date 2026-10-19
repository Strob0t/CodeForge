package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/port/database"
)

// Retention queries (GDPR Art. 5(1)(e), docs/data-retention.md) back the
// RetentionService system job.
//
// INTENTIONALLY CROSS-TENANT, for every method in this file: the retention
// policy is one instance-wide configuration (`retention.*`), so the cutoff is
// the same for every tenant, and these tables carry tenant_id without a
// foreign key to tenants - a sweep over the tenant list could miss rows. The
// only predicate is the row's age against the cutoff the caller computed from
// the policy, and each call changes a bounded number of rows (the caller
// loops), so a call cannot remove more than the policy allows or hold locks
// for long.
//
// Sessions and runs age by their last activity, which can change while a
// statement runs: the batch is selected from the statement's snapshot, so the
// statement repeats the age predicate on the rows it deletes - PostgreSQL
// re-evaluates it on the newest version of a row written concurrently (READ
// COMMITTED), and a row that became active again is left alone. Audit entries
// and consent records age by their creation, which never changes; handoff
// claims by the time their stage was done and webhook delivery claims by the
// time they were received (KI-90).

// retentionLockKey is the advisory lock that lets one Go Core replica (or one
// blue-green color) sweep at a time.
const retentionLockKey = `hashtext('codeforge:retention')`

// WithRetentionLock runs sweep while this process holds the retention advisory
// lock and reports whether it got it; if another session holds it, sweep does
// not run. The lock is session-level, so it lives on one pool connection for
// the whole sweep, and the sweep's statements run on that same connection: a
// sweep never needs a second connection, whatever the pool size. If the lock
// cannot be released, the connection is closed, which releases it.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (s *Store) WithRetentionLock(ctx context.Context, sweep func(ctx context.Context, purge database.RetentionPurger)) (bool, error) {
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

	sweep(ctx, retentionPurger{conn: conn})
	return true, nil
}

// retentionPurger runs the retention statements on the connection that holds
// the retention lock.
type retentionPurger struct {
	conn *pgxpool.Conn
}

// DeleteExpiredOAuthStates deletes the states of abandoned OAuth flows, on
// the lock connection.
//
// INTENTIONALLY CROSS-TENANT: a system step of the retention sweep; it deletes
// only rows past their own expiry (each keyed by its secret state).
func (p retentionPurger) DeleteExpiredOAuthStates(ctx context.Context) (int64, error) {
	tag, err := p.conn.Exec(ctx, deleteExpiredOAuthStatesSQL)
	if err != nil {
		return 0, fmt.Errorf("delete expired oauth states: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredSessions deletes up to batchSize agent sessions that were last
// used before the cutoff (last_activity_at: reuse and status changes count,
// foreign key actions do not) and returns how many it deleted.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (p retentionPurger) DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := p.conn.Exec(ctx,
		`DELETE FROM sessions WHERE id IN (
		   SELECT id FROM sessions WHERE last_activity_at < $1 LIMIT $2
		 ) AND last_activity_at < $1`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredConversations deletes up to batchSize conversations whose last
// activity (updated_at, bumped by every new message) is before the cutoff,
// with their messages, and returns how many conversations it deleted.
//
// One transaction per batch: it locks the conversations (FOR UPDATE; PostgreSQL
// re-checks the age of a row written since the snapshot, and SKIP LOCKED
// passes over conversations being written right now), deletes their messages
// in statements of at most messageBatch rows, then the conversations. While
// the batch holds the lock, no message can be added to them (CreateMessage
// updates the conversation first, and any insert needs a key-share lock on
// it), so a conversation is deleted only once its messages are gone and never
// cascades over an unbounded number of messages. Sessions that belong only to
// such a conversation go with it (trigger of migration 092); sessions that
// also belong to a task are kept, detached.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (p retentionPurger) DeleteExpiredConversations(ctx context.Context, before time.Time, batchSize, messageBatch int) (int64, error) {
	tx, err := p.conn.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete expired conversations: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx,
		`SELECT id FROM conversations WHERE updated_at < $1 LIMIT $2 FOR UPDATE SKIP LOCKED`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired conversations: select: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, fmt.Errorf("delete expired conversations: select: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	for {
		tag, err := tx.Exec(ctx,
			`DELETE FROM conversation_messages WHERE id IN (
			   SELECT id FROM conversation_messages WHERE conversation_id = ANY($1) LIMIT $2
			 )`, ids, messageBatch)
		if err != nil {
			return 0, fmt.Errorf("delete expired conversation messages: %w", err)
		}
		if tag.RowsAffected() < int64(messageBatch) {
			break
		}
	}

	tag, err := tx.Exec(ctx, `DELETE FROM conversations WHERE id = ANY($1)`, ids)
	if err != nil {
		return 0, fmt.Errorf("delete expired conversations: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("delete expired conversations: commit: %w", err)
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
func (p retentionPurger) DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := p.conn.Exec(ctx,
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
func (p retentionPurger) DeleteExpiredAuditEntries(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := p.conn.Exec(ctx,
		`DELETE FROM audit_log WHERE id IN (
		   SELECT id FROM audit_log WHERE created_at < $1 LIMIT $2
		 )`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired audit entries: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AnonymizeExpiredIPAddresses nulls ip_address on up to batchSize audit
// entries created before the cutoff and returns how many it changed; the
// entries themselves are kept. IP addresses are personal data per CJEU
// C-582/14 (Breyer); retention: 180 days per CNIL traceability guidance.
// PostgreSQL has no UPDATE ... LIMIT, so the batch is selected by id.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (p retentionPurger) AnonymizeExpiredIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := p.conn.Exec(ctx,
		`UPDATE audit_log SET ip_address = NULL WHERE id IN (
		   SELECT id FROM audit_log WHERE ip_address IS NOT NULL AND created_at < $1 LIMIT $2
		 ) AND ip_address IS NOT NULL`,
		before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("anonymize expired ip addresses: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AnonymizeExpiredConsentIPAddresses clears the IP address and user agent of
// up to batchSize consent records created before the cutoff and returns how
// many it changed; the records (user, purpose, decision) are kept as proof of
// consent.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (p retentionPurger) AnonymizeExpiredConsentIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := p.conn.Exec(ctx,
		`UPDATE user_consents SET ip_address = NULL, user_agent = NULL WHERE id IN (
		   SELECT id FROM user_consents
		   WHERE (ip_address IS NOT NULL OR user_agent IS NOT NULL) AND created_at < $1 LIMIT $2
		 ) AND (ip_address IS NOT NULL OR user_agent IS NOT NULL)`,
		before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("anonymize expired consent ip addresses: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredHandoffClaims deletes up to batchSize handoff claims whose
// stage was done (carried out or refused) before the cutoff and returns how
// many it deleted (KI-90). A claim never done is kept, however old: a
// redelivery of its message may still take it over and reuse its task (a
// released claim's claimed_at is -infinity).
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (p retentionPurger) DeleteExpiredHandoffClaims(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := p.conn.Exec(ctx,
		`DELETE FROM handoff_claims WHERE (tenant_id, handoff_id, stage) IN (
		   SELECT tenant_id, handoff_id, stage FROM handoff_claims WHERE done_at < $1 LIMIT $2
		 ) AND done_at < $1`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired handoff claims: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredWebhookDeliveries deletes up to batchSize webhook delivery
// claims received before the cutoff - the caller passes the end of their
// dedup window (webhook.delivery_retention) - and returns how many it
// deleted (KI-90). A webhook prunes its own claims on its next delivery; this
// covers the webhooks that receive none.
//
// INTENTIONALLY CROSS-TENANT: instance-wide retention job (see file comment).
func (p retentionPurger) DeleteExpiredWebhookDeliveries(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := p.conn.Exec(ctx,
		`DELETE FROM webhook_deliveries WHERE (webhook_id, delivery_key) IN (
		   SELECT webhook_id, delivery_key FROM webhook_deliveries WHERE received_at < $1 LIMIT $2
		 ) AND received_at < $1`, before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("delete expired webhook deliveries: %w", err)
	}
	return tag.RowsAffected(), nil
}
