package postgres

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// CreateFeedbackAudit inserts a new feedback audit entry.
func (s *Store) CreateFeedbackAudit(ctx context.Context, a *feedback.AuditEntry) error {
	const q = `
		INSERT INTO feedback_audit (tenant_id, run_id, call_id, tool, provider, decision, responder, response_time_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`

	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), a.RunID, a.CallID, a.Tool,
		string(a.Provider), string(a.Decision), a.Responder, a.ResponseTimeMs,
	).Scan(&a.ID, &a.CreatedAt)
}

// ListFeedbackByRun returns all feedback audit entries for a run.
func (s *Store) ListFeedbackByRun(ctx context.Context, runID string) ([]feedback.AuditEntry, error) {
	const q = `
		SELECT id, tenant_id, run_id, call_id, tool, provider, decision, responder, response_time_ms, created_at
		FROM feedback_audit
		WHERE run_id = $1 AND tenant_id = $2
		ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, q, runID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list feedback by run: %w", err)
	}
	defer rows.Close()

	var result []feedback.AuditEntry
	for rows.Next() {
		var a feedback.AuditEntry
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.RunID, &a.CallID, &a.Tool,
			&a.Provider, &a.Decision, &a.Responder, &a.ResponseTimeMs, &a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan feedback audit: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
