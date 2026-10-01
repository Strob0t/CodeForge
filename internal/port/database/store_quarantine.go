package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
)

// QuarantineStore defines database operations for message quarantine (Phase 23B).
type QuarantineStore interface {
	QuarantineMessage(ctx context.Context, msg *quarantine.Message) error
	GetQuarantinedMessage(ctx context.Context, id string) (*quarantine.Message, error)
	ListQuarantinedMessages(ctx context.Context, projectID string, status quarantine.Status, limit, offset int) ([]*quarantine.Message, error)
	// UpdateQuarantineStatus resolves a pending message (S2-G fix, 9: one
	// resolution wins): domain.ErrConflict when it is no longer pending,
	// domain.ErrNotFound for an unknown message or one of another tenant.
	UpdateQuarantineStatus(ctx context.Context, id string, status quarantine.Status, reviewedBy, note string) error
}
