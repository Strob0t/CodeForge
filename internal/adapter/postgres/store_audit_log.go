package postgres

import (
	"context"
	"fmt"
	"net/netip"

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
