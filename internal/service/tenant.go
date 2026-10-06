package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// TenantService manages tenant lifecycle.
type TenantService struct {
	store database.Store
}

// NewTenantService creates a new TenantService.
func NewTenantService(store database.Store) *TenantService {
	return &TenantService{store: store}
}

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// Create validates and creates a new tenant.
func (s *TenantService) Create(ctx context.Context, req tenant.CreateRequest) (*tenant.Tenant, error) {
	if req.Name == "" {
		return nil, errors.New("tenant name is required")
	}
	if !slugRegex.MatchString(req.Slug) {
		return nil, fmt.Errorf("invalid slug %q: must be 3-64 lowercase alphanumeric characters or hyphens", req.Slug)
	}
	return s.store.CreateTenant(ctx, req)
}

// Get returns a tenant by ID: the caller's own tenant, or any tenant for a
// caller of the default tenant, whose admins are the platform admins
// (KI-174). Another tenant is not found.
func (s *TenantService) Get(ctx context.Context, id string) (*tenant.Tenant, error) {
	if !reachable(ctx, id) {
		return nil, fmt.Errorf("get tenant %s: %w", id, domain.ErrNotFound)
	}
	return s.store.GetTenant(ctx, id)
}

// List returns all tenants (platform admins; the route checks the role).
func (s *TenantService) List(ctx context.Context) ([]tenant.Tenant, error) {
	return s.store.ListTenants(ctx)
}

// Update modifies an existing tenant, with the scope of Get. The default
// tenant cannot be disabled: it holds the platform admins and the worker's
// service identity.
func (s *TenantService) Update(ctx context.Context, id string, req tenant.UpdateRequest) (*tenant.Tenant, error) {
	t, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		t.Name = req.Name
	}
	if req.Enabled != nil {
		if !*req.Enabled && id == tenantctx.DefaultTenantID {
			return nil, fmt.Errorf("%w: the default tenant cannot be disabled", domain.ErrValidation)
		}
		t.Enabled = *req.Enabled
	}
	if err := s.store.UpdateTenant(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// reachable reports whether the caller of ctx may act on tenant id: its own
// tenant, or any tenant from the default tenant.
func reachable(ctx context.Context, id string) bool {
	caller := tenantctx.FromContext(ctx)
	return caller == id || caller == tenantctx.DefaultTenantID
}

// ValidateExists checks that the tenant exists and is enabled: a disabled
// tenant is tenant.ErrDisabled, a missing one domain.ErrNotFound; any other
// error is the store's (KI-174).
func (s *TenantService) ValidateExists(ctx context.Context, id string) error {
	t, err := s.store.GetTenant(ctx, id)
	if err != nil {
		return fmt.Errorf("tenant %s: %w", id, err)
	}
	if !t.Enabled {
		return fmt.Errorf("tenant %s: %w", id, tenant.ErrDisabled)
	}
	return nil
}
