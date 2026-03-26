package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// FeedbackStore defines database operations for feedback audit entries.
type FeedbackStore interface {
	CreateFeedbackAudit(ctx context.Context, a *feedback.AuditEntry) error
	ListFeedbackByRun(ctx context.Context, runID string) ([]feedback.AuditEntry, error)
}
