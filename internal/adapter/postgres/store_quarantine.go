package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
)

// QuarantineMessage inserts a new quarantined message into the database.
func (s *Store) QuarantineMessage(ctx context.Context, msg *quarantine.Message) error {
	const q = `
		INSERT INTO quarantine_messages (project_id, subject, payload, trust_origin, trust_level,
			risk_score, risk_factors, status, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`

	return s.pool.QueryRow(ctx, q,
		msg.ProjectID, msg.Subject, msg.Payload, msg.TrustOrigin, msg.TrustLevel,
		msg.RiskScore, msg.RiskFactors, string(msg.Status), msg.CreatedAt, msg.ExpiresAt,
	).Scan(&msg.ID)
}

// GetQuarantinedMessage retrieves a single quarantined message by ID.
func (s *Store) GetQuarantinedMessage(ctx context.Context, id string) (*quarantine.Message, error) {
	const q = `
		SELECT id, project_id, subject, payload, trust_origin, trust_level,
			risk_score, risk_factors, status, reviewed_by, review_note,
			created_at, reviewed_at, expires_at
		FROM quarantine_messages
		WHERE id = $1`

	var msg quarantine.Message
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&msg.ID, &msg.ProjectID, &msg.Subject, &msg.Payload, &msg.TrustOrigin, &msg.TrustLevel,
		&msg.RiskScore, &msg.RiskFactors, &msg.Status, &msg.ReviewedBy, &msg.ReviewNote,
		&msg.CreatedAt, &msg.ReviewedAt, &msg.ExpiresAt,
	)
	if err != nil {
		return nil, notFoundWrap(err, "get quarantined message %s", id)
	}
	return &msg, nil
}

// ListQuarantinedMessages returns quarantined messages filtered by project and status.
// Pass empty status to list all statuses. Results are ordered newest-first.
func (s *Store) ListQuarantinedMessages(ctx context.Context, projectID string, status quarantine.Status, limit, offset int) ([]*quarantine.Message, error) {
	var q string
	var args []interface{}

	if status != "" {
		q = `
			SELECT id, project_id, subject, payload, trust_origin, trust_level,
				risk_score, risk_factors, status, reviewed_by, review_note,
				created_at, reviewed_at, expires_at
			FROM quarantine_messages
			WHERE project_id = $1 AND status = $2
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4`
		args = []interface{}{projectID, string(status), limit, offset}
	} else {
		q = `
			SELECT id, project_id, subject, payload, trust_origin, trust_level,
				risk_score, risk_factors, status, reviewed_by, review_note,
				created_at, reviewed_at, expires_at
			FROM quarantine_messages
			WHERE project_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3`
		args = []interface{}{projectID, limit, offset}
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list quarantined messages: %w", err)
	}
	defer rows.Close()

	var result []*quarantine.Message
	for rows.Next() {
		var msg quarantine.Message
		if err := rows.Scan(
			&msg.ID, &msg.ProjectID, &msg.Subject, &msg.Payload, &msg.TrustOrigin, &msg.TrustLevel,
			&msg.RiskScore, &msg.RiskFactors, &msg.Status, &msg.ReviewedBy, &msg.ReviewNote,
			&msg.CreatedAt, &msg.ReviewedAt, &msg.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan quarantined message: %w", err)
		}
		result = append(result, &msg)
	}
	return result, rows.Err()
}

// UpdateQuarantineStatus sets the review status of a quarantined message.
func (s *Store) UpdateQuarantineStatus(ctx context.Context, id string, status quarantine.Status, reviewedBy, note string) error {
	now := time.Now().UTC()
	const q = `
		UPDATE quarantine_messages
		SET status = $2, reviewed_by = $3, review_note = $4, reviewed_at = $5
		WHERE id = $1`

	tag, err := s.pool.Exec(ctx, q, id, string(status), reviewedBy, note, now)
	return execExpectOne(tag, err, "update quarantine status for message %s", id)
}
