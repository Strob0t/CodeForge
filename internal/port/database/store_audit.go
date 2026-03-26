package database

import "context"

// AuditStore defines database operations for admin audit logging.
type AuditStore interface {
	InsertAuditEntry(ctx context.Context, e *AuditEntry) error
	ListAuditEntries(ctx context.Context, action string, limit, offset int) ([]AuditEntry, error)
}
