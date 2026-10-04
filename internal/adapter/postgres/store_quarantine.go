package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// QuarantineMessage inserts a new quarantined message into the database.
func (s *Store) QuarantineMessage(ctx context.Context, msg *quarantine.Message) error {
	const q = `
		INSERT INTO quarantine_messages (tenant_id, project_id, subject, payload, trust_origin, trust_level,
			risk_score, risk_factors, status, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`

	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), msg.ProjectID, msg.Subject, msg.Payload, msg.TrustOrigin, msg.TrustLevel,
		msg.RiskScore, msg.RiskFactors, string(msg.Status), msg.CreatedAt, msg.ExpiresAt,
	).Scan(&msg.ID)
}

// quarantineColumns are the columns scanQuarantineMessage reads.
const quarantineColumns = `id, tenant_id, project_id, subject, payload, trust_origin, trust_level,
	risk_score, risk_factors, status, COALESCE(reviewed_by_user_id::text, ''), reviewed_by, review_note,
	created_at, reviewed_at, expires_at`

func scanQuarantineMessage(row scannable) (*quarantine.Message, error) {
	var msg quarantine.Message
	err := row.Scan(
		&msg.ID, &msg.TenantID, &msg.ProjectID, &msg.Subject, &msg.Payload, &msg.TrustOrigin, &msg.TrustLevel,
		&msg.RiskScore, &msg.RiskFactors, &msg.Status, &msg.ReviewedByID, &msg.ReviewedBy, &msg.ReviewNote,
		&msg.CreatedAt, &msg.ReviewedAt, &msg.ExpiresAt,
	)
	return &msg, err
}

// GetQuarantinedMessage retrieves a single quarantined message by ID.
func (s *Store) GetQuarantinedMessage(ctx context.Context, id string) (*quarantine.Message, error) {
	msg, err := scanQuarantineMessage(s.pool.QueryRow(ctx,
		`SELECT `+quarantineColumns+` FROM quarantine_messages WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx)))
	if err != nil {
		return nil, notFoundWrap(err, "get quarantined message %s", id)
	}
	return msg, nil
}

// ListQuarantinedMessages returns quarantined messages filtered by project and status.
// Pass empty status to list all statuses. Results are ordered newest-first.
func (s *Store) ListQuarantinedMessages(ctx context.Context, projectID string, status quarantine.Status, limit, offset int) ([]*quarantine.Message, error) {
	var q string
	var args []any
	tid := tenantFromCtx(ctx)

	if status != "" {
		q = `
			SELECT ` + quarantineColumns + `
			FROM quarantine_messages
			WHERE project_id = $1 AND status = $2 AND tenant_id = $3
			ORDER BY created_at DESC
			LIMIT $4 OFFSET $5`
		args = []any{projectID, string(status), tid, limit, offset}
	} else {
		q = `
			SELECT ` + quarantineColumns + `
			FROM quarantine_messages
			WHERE project_id = $1 AND tenant_id = $2
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4`
		args = []any{projectID, tid, limit, offset}
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list quarantined messages: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (*quarantine.Message, error) { return scanQuarantineMessage(r) })
}

// ListExpiredQuarantineMessages returns up to limit pending messages whose
// review deadline has passed, the oldest deadline first (KI-91).
//
// INTENTIONALLY CROSS-TENANT: the expiry sweep is a system job that expires
// the overdue messages of every tenant. Each message carries its tenant_id,
// and the caller expires it in that tenant's context
// (ExpireQuarantineMessage is tenant-scoped).
func (s *Store) ListExpiredQuarantineMessages(ctx context.Context, limit int) ([]*quarantine.Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+quarantineColumns+` FROM quarantine_messages
		 WHERE status = 'pending' AND expires_at <= now()
		 ORDER BY expires_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list expired quarantined messages: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (*quarantine.Message, error) { return scanQuarantineMessage(r) })
}

// ExpireQuarantineMessage expires a pending, overdue message of the caller's
// tenant and rejects the inbound A2A task waiting for it, in one transaction
// (see database.QuarantineStore). Both updates are conditional: the message
// changes only while pending and overdue - an admin's decision, the sender's
// withdrawal or another replica's sweep that got there first (PostgreSQL
// re-checks the predicate after waiting for its row lock) leaves it, and the
// task, alone - and the task only while it is submitted and names the
// message. The message's row is locked before the task's, and no other
// writer holds both, so the transaction cannot deadlock with a decision.
func (s *Store) ExpireQuarantineMessage(ctx context.Context, id, heldTaskID string, review *quarantine.Review) (database.QuarantineExpiry, error) {
	var res database.QuarantineExpiry
	tid := tenantFromCtx(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("expire quarantined message %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE quarantine_messages SET status = 'expired', reviewed_by_user_id = NULL,
		        reviewed_by = $3, review_note = $4, reviewed_at = now()
		 WHERE id = $1 AND tenant_id = $2 AND status = 'pending' AND expires_at <= now()`,
		id, tid, review.ReviewerName, review.Note)
	if err != nil {
		return res, fmt.Errorf("expire quarantined message %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return res, nil
	}
	if heldTaskID != "" {
		err := tx.QueryRow(ctx,
			`UPDATE a2a_tasks SET state = 'rejected', version = version + 1, updated_at = now()
			 WHERE id = $1 AND tenant_id = $2 AND state = 'submitted'
			   AND metadata->>'`+a2adomain.MetadataQuarantineMessageID+`' = $3
			 RETURNING direction`,
			heldTaskID, tid, id).Scan(&res.RejectedTaskDirection)
		switch {
		case err == nil:
			res.RejectedTaskID = heldTaskID
		case !errors.Is(err, pgx.ErrNoRows):
			return database.QuarantineExpiry{}, fmt.Errorf("reject the held a2a task %s of expired message %s: %w", heldTaskID, id, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return database.QuarantineExpiry{}, fmt.Errorf("expire quarantined message %s: commit: %w", id, err)
	}
	res.Expired = true
	return res, nil
}

// UpdateQuarantineStatus records the review of a pending message: its status,
// the reviewer (user ID and name at the time) and the note. A reviewer that
// is one of the synthetic identities without a users row (auth disabled,
// internal service key) is recorded by name only; a reviewer whose row is
// gone gets user.ErrAccountGone and nothing changes (see accountRef). Only a
// pending message changes (domain.ErrConflict otherwise), and an overdue one
// is no longer approved (KI-91: it expires, even before the sweep reached it).
func (s *Store) UpdateQuarantineStatus(ctx context.Context, id string, status quarantine.Status, review *quarantine.Review) error {
	now := time.Now().UTC()
	const q = `
		UPDATE quarantine_messages
		SET status = $2, reviewed_by_user_id = $3, reviewed_by = $4, review_note = $5, reviewed_at = $6
		WHERE id = $1 AND tenant_id = $7 AND status = 'pending'
		  AND ($2 <> 'approved' OR expires_at > now())`

	tag, err := s.pool.Exec(ctx, q, id, string(status), accountRef(review.ReviewerID), review.ReviewerName, review.Note, now, tenantFromCtx(ctx))
	return s.guardedUpdateResult(ctx, tag, accountGone(err, "quarantine_messages_reviewed_by_user_id_fkey"),
		quarantineExistsSQL, "update quarantine status for message", id)
}

// UnconsumedQuarantineRelease returns the ID of an approved, unconsumed
// quarantine message of subject in the current tenant whose payload is
// exactly payload (the oldest approval first); domain.ErrNotFound if none.
func (s *Store) UnconsumedQuarantineRelease(ctx context.Context, subject string, payload []byte) (string, error) {
	const q = `
		SELECT id FROM quarantine_messages
		WHERE tenant_id = $1 AND subject = $2 AND status = 'approved' AND consumed_at IS NULL AND payload = $3
		ORDER BY reviewed_at
		LIMIT 1`
	var id string
	if err := s.pool.QueryRow(ctx, q, tenantFromCtx(ctx), subject, payload).Scan(&id); err != nil {
		return "", notFoundWrap(err, "unconsumed quarantine release on %s", subject)
	}
	return id, nil
}

// ConsumeQuarantineRelease records that the replay of the approved message
// id was carried out; only an approved, unconsumed message changes.
func (s *Store) ConsumeQuarantineRelease(ctx context.Context, id string) error {
	const q = `
		UPDATE quarantine_messages SET consumed_at = now()
		WHERE id = $1 AND tenant_id = $2 AND status = 'approved' AND consumed_at IS NULL`
	tag, err := s.pool.Exec(ctx, q, id, tenantFromCtx(ctx))
	return s.guardedUpdateResult(ctx, tag, err, quarantineExistsSQL, "consume quarantine release", id)
}

const quarantineExistsSQL = `SELECT EXISTS (SELECT 1 FROM quarantine_messages WHERE id = $1 AND tenant_id = $2)`

// AnonymizeQuarantineReviewsForUser replaces the reviewer name of the user's
// reviews in the current tenant with quarantine.ErasedReviewerName. Called
// before the user is deleted (GDPR Art. 17); the foreign key then sets
// reviewed_by_user_id to NULL, and the decisions stay.
func (s *Store) AnonymizeQuarantineReviewsForUser(ctx context.Context, userID string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE quarantine_messages SET reviewed_by = $3 WHERE reviewed_by_user_id = $1 AND tenant_id = $2`,
		userID, tenantFromCtx(ctx), quarantine.ErasedReviewerName)
	if err != nil {
		return 0, fmt.Errorf("anonymize quarantine reviews for user: %w", err)
	}
	return tag.RowsAffected(), nil
}
