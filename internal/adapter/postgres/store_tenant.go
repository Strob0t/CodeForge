package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/tenant"
)

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
	if err := unmarshalJSONField(settingsJSON, &t.Settings, "settings"); err != nil {
		slog.Warn("failed to unmarshal tenant settings", "tenant_id", t.ID, "error", err)
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
		return nil, notFoundWrap(err, "get tenant %s", id)
	}
	if err := unmarshalJSONField(settingsJSON, &t.Settings, "settings"); err != nil {
		slog.Warn("failed to unmarshal tenant settings", "tenant_id", t.ID, "error", err)
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
	return scanRows(rows, func(r pgx.Rows) (tenant.Tenant, error) {
		var t tenant.Tenant
		var settingsJSON []byte
		if err := r.Scan(&t.ID, &t.Name, &t.Slug, &t.Enabled, &settingsJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return t, err
		}
		if err := unmarshalJSONField(settingsJSON, &t.Settings, "settings"); err != nil {
			slog.Warn("failed to unmarshal tenant settings", "tenant_id", t.ID, "error", err)
		}
		return t, nil
	})
}

func (s *Store) UpdateTenant(ctx context.Context, t *tenant.Tenant) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tenants SET name = $2, enabled = $3, updated_at = now()
		 WHERE id = $1`,
		t.ID, t.Name, t.Enabled)
	return execExpectOne(tag, err, "update tenant %s", t.ID)
}
