package postgres

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/port/database"
)

// InsertAuditEntry writes an immutable audit log row.
func (s *Store) InsertAuditEntry(ctx context.Context, e *database.AuditEntry) error {
	tid := tenantFromCtx(ctx)

	// Convert IP string to pgx-compatible type (nil if empty/invalid).
	var ipVal *netip.Addr
	if e.IPAddress != "" {
		if addr, err := netip.ParseAddr(e.IPAddress); err == nil {
			ipVal = &addr
		}
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_log (tenant_id, admin_id, admin_email, action, resource, resource_id, details, ip_address)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tid, e.AdminID, e.AdminEmail, e.Action, e.Resource,
		nullIfEmpty(e.ResourceID), e.Details, ipVal)
	if err != nil {
		return fmt.Errorf("insert audit entry: %w", err)
	}
	return nil
}

// ListAuditEntries returns audit log entries for the current tenant, ordered newest-first.
// Optionally filters by action when non-empty.
func (s *Store) ListAuditEntries(ctx context.Context, action string, limit, offset int) ([]database.AuditEntry, error) {
	tid := tenantFromCtx(ctx)

	query := `SELECT id, tenant_id, admin_id, admin_email, action, resource,
	                 COALESCE(resource_id, ''), COALESCE(details::text, ''), COALESCE(host(ip_address), ''), created_at
	          FROM audit_log WHERE tenant_id = $1`
	args := []any{tid}
	argIdx := 2

	if action != "" {
		query += fmt.Sprintf(` AND action = $%d`, argIdx)
		args = append(args, action)
		argIdx++
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (database.AuditEntry, error) {
		var e database.AuditEntry
		var detailsStr string
		if err := r.Scan(&e.ID, &e.TenantID, &e.AdminID, &e.AdminEmail, &e.Action,
			&e.Resource, &e.ResourceID, &detailsStr, &e.IPAddress, &e.CreatedAt); err != nil {
			return e, err
		}
		if detailsStr != "" {
			e.Details = []byte(detailsStr)
		}
		return e, nil
	})
}

// AnonymizeAuditLogForUser nulls PII fields (admin_email, ip_address) for a
// specific admin user. Called before user deletion per GDPR Art. 17 / ADR-009.
// The audit trail entries (action, resource, timestamps) are preserved.
func (s *Store) AnonymizeAuditLogForUser(ctx context.Context, adminID string) (int64, error) {
	tid := tenantFromCtx(ctx)
	tag, err := s.pool.Exec(ctx,
		`UPDATE audit_log SET admin_email = NULL, ip_address = NULL WHERE admin_id = $1 AND tenant_id = $2`,
		adminID, tid)
	if err != nil {
		return 0, fmt.Errorf("anonymize audit log for user: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AnonymizeExpiredIPAddresses nulls ip_address on audit entries older than
// the given cutoff. IP addresses are personal data per CJEU C-582/14 (Breyer).
// Recommended retention: 180 days per CNIL traceability guidance.
// NOTE: This is a cross-tenant system maintenance job — intentionally not tenant-scoped.
// It runs under system context and affects all tenants uniformly.
func (s *Store) AnonymizeExpiredIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE audit_log SET ip_address = NULL
		 WHERE ip_address IS NOT NULL AND created_at < $1
		 LIMIT $2`,
		before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("anonymize expired ip addresses: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListAuditEntriesByAdmin returns audit log entries for a specific admin user,
// scoped to the current tenant. Used for GDPR data export (Article 20).
func (s *Store) ListAuditEntriesByAdmin(ctx context.Context, adminID string, limit int) ([]database.AuditEntry, error) {
	tid := tenantFromCtx(ctx)

	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, admin_id, admin_email, action, resource,
		        COALESCE(resource_id, ''), COALESCE(details::text, ''), COALESCE(host(ip_address), ''), created_at
		 FROM audit_log WHERE tenant_id = $1 AND admin_id = $2
		 ORDER BY created_at DESC LIMIT $3`,
		tid, adminID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit entries by admin: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (database.AuditEntry, error) {
		var e database.AuditEntry
		var detailsStr string
		if err := r.Scan(&e.ID, &e.TenantID, &e.AdminID, &e.AdminEmail, &e.Action,
			&e.Resource, &e.ResourceID, &detailsStr, &e.IPAddress, &e.CreatedAt); err != nil {
			return e, err
		}
		if detailsStr != "" {
			e.Details = []byte(detailsStr)
		}
		return e, nil
	})
}
