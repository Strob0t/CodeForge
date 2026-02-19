package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/port/database"
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

// Get returns a tenant by ID.
func (s *TenantService) Get(ctx context.Context, id string) (*tenant.Tenant, error) {
	return s.store.GetTenant(ctx, id)
}

// List returns all tenants.
func (s *TenantService) List(ctx context.Context) ([]tenant.Tenant, error) {
	return s.store.ListTenants(ctx)
}

// Update modifies an existing tenant.
func (s *TenantService) Update(ctx context.Context, id string, req tenant.UpdateRequest) (*tenant.Tenant, error) {
	t, err := s.store.GetTenant(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		t.Name = req.Name
	}
	if req.Enabled != nil {
		t.Enabled = *req.Enabled
	}
	if err := s.store.UpdateTenant(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// ValidateExists checks that the tenant exists and is enabled.
func (s *TenantService) ValidateExists(ctx context.Context, id string) error {
	t, err := s.store.GetTenant(ctx, id)
	if err != nil {
		return fmt.Errorf("tenant %s: %w", id, err)
	}
	if !t.Enabled {
		return fmt.Errorf("tenant %s is disabled", id)
	}
	return nil
}
