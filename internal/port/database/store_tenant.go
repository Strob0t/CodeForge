package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/tenant"
)

// TenantStore defines database operations for tenant management.
type TenantStore interface {
	CreateTenant(ctx context.Context, req tenant.CreateRequest) (*tenant.Tenant, error)
	GetTenant(ctx context.Context, id string) (*tenant.Tenant, error)
	ListTenants(ctx context.Context) ([]tenant.Tenant, error)
	UpdateTenant(ctx context.Context, t *tenant.Tenant) error
	// AllocateToolUID returns the tenant's tool UID, allocating one when it
	// has none (KI-96); tenant.ErrToolUIDRangeExhausted when none is left.
	AllocateToolUID(ctx context.Context, tenantID string) (int, error)
	// AdvanceToolUIDSequence moves the tool UID sequence past atLeast, if it
	// is not already; it reports whether it moved.
	AdvanceToolUIDSequence(ctx context.Context, atLeast int) (bool, error)
}
