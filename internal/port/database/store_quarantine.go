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
	// UpdateQuarantineStatus records the review of a pending message (S2-G
	// fix, 9: one resolution wins): domain.ErrConflict when it is no longer
	// pending, domain.ErrNotFound for an unknown message or one of another
	// tenant. The review names the logged-in reviewer (KI-79).
	UpdateQuarantineStatus(ctx context.Context, id string, status quarantine.Status, review *quarantine.Review) error
	// AnonymizeQuarantineReviewsForUser replaces the reviewer name of the
	// user's reviews in the current tenant with quarantine.ErasedReviewerName
	// (GDPR erasure, before the user is deleted).
	AnonymizeQuarantineReviewsForUser(ctx context.Context, userID string) (int64, error)
	// UnconsumedQuarantineRelease returns the ID of an approved quarantine
	// message of subject in the current tenant whose payload is exactly
	// payload (Approve replays it byte for byte) and whose replay was not
	// consumed yet; domain.ErrNotFound if there is none.
	UnconsumedQuarantineRelease(ctx context.Context, subject string, payload []byte) (string, error)
	// ConsumeQuarantineRelease records that the replay of the approved
	// message id was carried out: domain.ErrConflict when it was consumed
	// already or is not approved, domain.ErrNotFound for an unknown message
	// or one of another tenant.
	ConsumeQuarantineRelease(ctx context.Context, id string) error
	// ListExpiredQuarantineMessages returns up to limit pending messages of
	// every tenant whose review deadline (expires_at) has passed, the oldest
	// deadline first, each with its tenant (the expiry sweep, KI-91).
	ListExpiredQuarantineMessages(ctx context.Context, limit int) ([]*quarantine.Message, error)
	// ExpireQuarantineMessage moves a pending, overdue message of the
	// current tenant to expired with review, and in the same transaction
	// rejects the inbound A2A task heldTaskID ("" for none) if it still
	// waits for the message (submitted, naming it or no held message yet).
	// A message that is not pending, not overdue or not the tenant's is left
	// alone (Expired false).
	ExpireQuarantineMessage(ctx context.Context, id, heldTaskID string, review *quarantine.Review) (QuarantineExpiry, error)
}

// QuarantineExpiry is what ExpireQuarantineMessage changed.
type QuarantineExpiry struct {
	// Expired reports that the message moved from pending to expired.
	Expired bool
	// RejectedTaskID is the held A2A task rejected with the message, and
	// RejectedTaskDirection its direction ("" when no task waited for it).
	RejectedTaskID        string
	RejectedTaskDirection string
}
