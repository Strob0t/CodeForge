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
}
