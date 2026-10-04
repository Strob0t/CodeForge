package database

import (
	"context"
	"time"
)

// RetentionStore is the port of the data retention job (RetentionService,
// GDPR Art. 5(1)(e)).
type RetentionStore interface {
	// WithRetentionLock runs sweep while it holds the instance-wide retention
	// lock and reports whether it got the lock; while another replica holds
	// it, sweep does not run. sweep purges through purge, which runs on the
	// connection that holds the lock: a sweep uses one database connection.
	WithRetentionLock(ctx context.Context, sweep func(ctx context.Context, purge RetentionPurger)) (bool, error)
}

// RetentionPurger removes or anonymizes expired data. The policy is one
// instance-wide configuration, so every method spans all tenants; each changes
// at most batchSize rows whose age is past the cutoff and returns how many it
// changed.
type RetentionPurger interface {
	DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error)
	// DeleteExpiredConversations deletes up to batchSize conversations with
	// their messages, at most messageBatch messages per statement.
	DeleteExpiredConversations(ctx context.Context, before time.Time, batchSize, messageBatch int) (int64, error)
	DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExpiredAuditEntries(ctx context.Context, before time.Time, batchSize int) (int64, error)
	AnonymizeExpiredIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error)
	AnonymizeExpiredConsentIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error)
	// DeleteExpiredOAuthStates deletes the OAuth states of abandoned flows
	// (past their own expiry; a system step of every sweep).
	DeleteExpiredOAuthStates(ctx context.Context) (int64, error)
}
