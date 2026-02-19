package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// tenantFromCtx extracts the tenant ID from the request context.
// All tenant-scoped queries must use this to enforce isolation.
func tenantFromCtx(ctx context.Context) string {
	return middleware.TenantIDFromContext(ctx)
}

// --- Tenant CRUD ---

func (s *Store) CreateTenant(ctx context.Context, req tenant.CreateRequest) (*tenant.Tenant, error) {
	var t tenant.Tenant
	var settingsJSON []byte
	err := s.pool.QueryRow(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2)
		 RETURNING id, name, slug, enabled, settings, created_at, updated_at`,
		req.Name, req.Slug,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.Enabled, &settingsJSON, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	if settingsJSON != nil {
		_ = json.Unmarshal(settingsJSON, &t.Settings)
	}
	return &t, nil
}

func (s *Store) GetTenant(ctx context.Context, id string) (*tenant.Tenant, error) {
	var t tenant.Tenant
	var settingsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, enabled, settings, created_at, updated_at
		 FROM tenants WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.Enabled, &settingsJSON, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get tenant %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get tenant %s: %w", id, err)
	}
	if settingsJSON != nil {
		_ = json.Unmarshal(settingsJSON, &t.Settings)
	}
	return &t, nil
}

func (s *Store) ListTenants(ctx context.Context) ([]tenant.Tenant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, slug, enabled, settings, created_at, updated_at
		 FROM tenants ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []tenant.Tenant
	for rows.Next() {
		var t tenant.Tenant
		var settingsJSON []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Enabled, &settingsJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		if settingsJSON != nil {
			_ = json.Unmarshal(settingsJSON, &t.Settings)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *Store) UpdateTenant(ctx context.Context, t *tenant.Tenant) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tenants SET name = $2, enabled = $3, updated_at = now()
		 WHERE id = $1`,
		t.ID, t.Name, t.Enabled)
	if err != nil {
		return fmt.Errorf("update tenant %s: %w", t.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update tenant %s: %w", t.ID, domain.ErrNotFound)
	}
	return nil
}
