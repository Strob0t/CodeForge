package database

import (
	"context"
	"time"
)

// AuditStore defines database operations for admin audit logging.
type AuditStore interface {
	InsertAuditEntry(ctx context.Context, e *AuditEntry) error
	ListAuditEntries(ctx context.Context, action string, limit, offset int) ([]AuditEntry, error)
	ListAuditEntriesByAdmin(ctx context.Context, adminID string, limit int) ([]AuditEntry, error)
	DeleteExpiredAuditEntries(ctx context.Context, before time.Time, batchSize int) (int64, error)
	AnonymizeAuditLogForUser(ctx context.Context, adminID string) (int64, error)
	AnonymizeExpiredIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error)
}
