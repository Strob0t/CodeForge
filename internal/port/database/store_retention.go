package database

import (
	"context"
	"time"
)

// RetentionStore is the port of the data retention job (RetentionService,
// GDPR Art. 5(1)(e)). The policy is one instance-wide configuration, so every
// purge method spans all tenants; each changes at most batchSize rows whose
// age is past the cutoff and returns how many it changed.
type RetentionStore interface {
	// WithRetentionLock runs sweep while it holds the instance-wide retention
	// lock and reports whether it got the lock. When another replica holds it,
	// sweep does not run.
	WithRetentionLock(ctx context.Context, sweep func(ctx context.Context)) (bool, error)

	DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExpiredConversationMessages(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExpiredConversations(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExpiredAuditEntries(ctx context.Context, before time.Time, batchSize int) (int64, error)
	AnonymizeExpiredIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error)
	AnonymizeExpiredConsentIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error)
}
