package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
)

// --- Tenant CRUD ---

const tenantColumns = `id, name, slug, enabled, settings, created_at, updated_at, tool_uid`

// sqlStateSequenceExhausted is PostgreSQL's "sequence generator limit exceeded".
const sqlStateSequenceExhausted = "2200H"

func scanTenant(row pgx.Row) (tenant.Tenant, error) {
	var t tenant.Tenant
	var settingsJSON []byte
	var toolUID *int32
	if err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.Enabled, &settingsJSON, &t.CreatedAt, &t.UpdatedAt, &toolUID); err != nil {
		return t, err
	}
	if toolUID != nil {
		uid := int(*toolUID)
		t.ToolUID = &uid
	}
	if err := unmarshalJSONField(settingsJSON, &t.Settings, "settings"); err != nil {
		slog.Warn("failed to unmarshal tenant settings", "tenant_id", t.ID, "error", err)
	}
	return t, nil
}

func (s *Store) CreateTenant(ctx context.Context, req tenant.CreateRequest) (*tenant.Tenant, error) {
	t, err := scanTenant(s.pool.QueryRow(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING `+tenantColumns,
		req.Name, req.Slug,
	))
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return &t, nil
}

// GetTenant reads a tenant by ID. INTENTIONALLY CROSS-TENANT: the tenant is
// the row itself, and the authentication path checks a user's tenant before
// a tenant context exists (KI-174); the TenantService scopes the admin
// reads to the caller's tenant.
func (s *Store) GetTenant(ctx context.Context, id string) (*tenant.Tenant, error) {
	t, err := scanTenant(s.pool.QueryRow(ctx, `SELECT `+tenantColumns+` FROM tenants WHERE id = $1`, id))
	if err != nil {
		return nil, notFoundWrap(err, "get tenant %s", id)
	}
	return &t, nil
}

// ListTenants lists every tenant. INTENTIONALLY CROSS-TENANT: the listing
// is for platform admins (the route requires them, KI-174).
func (s *Store) ListTenants(ctx context.Context) ([]tenant.Tenant, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+tenantColumns+` FROM tenants ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (tenant.Tenant, error) { return scanTenant(r) })
}

// UpdateTenant saves a tenant's name and enabled flag. INTENTIONALLY
// CROSS-TENANT: the tenant is the row itself; the TenantService scopes the
// update to the caller's own tenant or a platform admin (KI-174).
func (s *Store) UpdateTenant(ctx context.Context, t *tenant.Tenant) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tenants SET name = $2, enabled = $3, updated_at = now()
		 WHERE id = $1`,
		t.ID, t.Name, t.Enabled)
	return execExpectOne(tag, err, "update tenant %s", t.ID)
}

// AllocateToolUID returns the tenant's tool UID, allocating the next one of
// the sequence when it has none yet (KI-96). The row lock keeps two
// concurrent first allocations from burning a UID; set values never change
// (a trigger refuses it). Tenant management: the tenant is the row itself.
func (s *Store) AllocateToolUID(ctx context.Context, tenantID string) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("allocate tool uid for tenant %s: begin tx: %w", tenantID, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var current *int32
	if err := tx.QueryRow(ctx, `SELECT tool_uid FROM tenants WHERE id = $1 FOR UPDATE`, tenantID).Scan(&current); err != nil {
		return 0, notFoundWrap(err, "allocate tool uid for tenant %s", tenantID)
	}
	if current != nil {
		return int(*current), nil
	}
	var allocated int32
	err = tx.QueryRow(ctx,
		`UPDATE tenants SET tool_uid = nextval('tenant_tool_uid_seq') WHERE id = $1 RETURNING tool_uid`, tenantID,
	).Scan(&allocated)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == sqlStateSequenceExhausted {
			return 0, fmt.Errorf("allocate tool uid for tenant %s: %w", tenantID, tenant.ErrToolUIDRangeExhausted)
		}
		return 0, fmt.Errorf("allocate tool uid for tenant %s: %w", tenantID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("allocate tool uid for tenant %s: commit: %w", tenantID, err)
	}
	return int(allocated), nil
}

// AdvanceToolUIDSequence makes sure the sequence never hands out a UID up to
// atLeast again: after a database restore older than the workspaces volume,
// the worker's on-volume bindings name UIDs the database forgot (KI-96 S8).
// It reports whether the sequence moved.
func (s *Store) AdvanceToolUIDSequence(ctx context.Context, atLeast int) (bool, error) {
	if atLeast < tenant.ToolUIDMin {
		return false, nil
	}
	var lastValue int64
	var isCalled bool
	if err := s.pool.QueryRow(ctx, `SELECT last_value, is_called FROM tenant_tool_uid_seq`).Scan(&lastValue, &isCalled); err != nil {
		return false, fmt.Errorf("read tool uid sequence: %w", err)
	}
	handedOut := lastValue
	if !isCalled {
		handedOut = lastValue - 1
	}
	if int64(atLeast) <= handedOut {
		return false, nil
	}
	if _, err := s.pool.Exec(ctx, `SELECT setval('tenant_tool_uid_seq', $1, true)`, atLeast); err != nil {
		return false, fmt.Errorf("advance tool uid sequence to %d: %w", atLeast, err)
	}
	return true, nil
}

// ListAdoptedWorkspaces lists the projects whose workspace lies outside the
// workspace root (adopted by a platform admin).
//
// INTENTIONALLY CROSS-TENANT: at startup with workspace.tool_acls: required
// the Go Core checks every adopted workspace for its tenant's tool UID
// (KI-96); the rows carry their tenant_id and each is handled in its own
// tenant.
func (s *Store) ListAdoptedWorkspaces(ctx context.Context, root string) ([]project.Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, workspace_path FROM projects
		 WHERE workspace_path <> '' AND workspace_path <> $1 AND left(workspace_path, length($1) + 1) <> $1 || '/'
		 ORDER BY tenant_id, id`, root)
	if err != nil {
		return nil, fmt.Errorf("list adopted workspaces: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (project.Project, error) {
		var p project.Project
		err := r.Scan(&p.ID, &p.TenantID, &p.WorkspacePath)
		return p, err
	})
}
