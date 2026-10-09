package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const tenantTestOther = "11111111-2222-3333-4444-555555555555"

// tenantFakeStore holds tenants by ID.
type tenantFakeStore struct {
	mockStore
	tenants map[string]*tenant.Tenant
	updated []tenant.Tenant
}

func (s *tenantFakeStore) GetTenant(_ context.Context, id string) (*tenant.Tenant, error) {
	t, ok := s.tenants[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (s *tenantFakeStore) UpdateTenant(_ context.Context, t *tenant.Tenant) error {
	s.updated = append(s.updated, *t)
	s.tenants[t.ID] = t
	return nil
}

func newTenantFakeStore() *tenantFakeStore {
	return &tenantFakeStore{tenants: map[string]*tenant.Tenant{
		tenantctx.DefaultTenantID: {ID: tenantctx.DefaultTenantID, Name: "Default", Slug: "default", Enabled: true},
		tenantTestOther:           {ID: tenantTestOther, Name: "Other", Slug: "other", Enabled: true},
	}}
}

// KI-174: ValidateExists tells a disabled tenant from a missing one with
// typed errors, so the gate can cache the verdicts and refuse with a clear
// message.
func TestTenantValidateExists(t *testing.T) {
	store := newTenantFakeStore()
	store.tenants[tenantTestOther].Enabled = false
	svc := NewTenantService(store)
	ctx := context.Background()

	if err := svc.ValidateExists(ctx, tenantctx.DefaultTenantID); err != nil {
		t.Fatalf("enabled tenant: %v", err)
	}
	if err := svc.ValidateExists(ctx, tenantTestOther); !errors.Is(err, tenant.ErrDisabled) {
		t.Fatalf("disabled tenant: %v, want ErrDisabled", err)
	}
	if err := svc.ValidateExists(ctx, "33333333-3333-3333-3333-333333333333"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown tenant: %v, want ErrNotFound", err)
	}
}

// Get and Update are scoped to the caller's tenant; a caller of the default
// tenant (a platform admin) reaches every tenant.
func TestTenantGetAndUpdate_AreScopedToTheCallersTenant(t *testing.T) {
	store := newTenantFakeStore()
	svc := NewTenantService(store)
	other := tenantctx.WithTenant(context.Background(), tenantTestOther)
	platform := tenantctx.WithTenant(context.Background(), tenantctx.DefaultTenantID)

	if _, err := svc.Get(other, tenantTestOther); err != nil {
		t.Fatalf("own tenant: %v", err)
	}
	if _, err := svc.Get(other, tenantctx.DefaultTenantID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign tenant: %v, want ErrNotFound", err)
	}
	if _, err := svc.Update(other, tenantctx.DefaultTenantID, tenant.UpdateRequest{Name: "x"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign update: %v, want ErrNotFound", err)
	}
	if len(store.updated) != 0 {
		t.Fatalf("foreign update wrote %+v", store.updated)
	}
	if _, err := svc.Get(platform, tenantTestOther); err != nil {
		t.Fatalf("platform admin reads another tenant: %v", err)
	}
	if _, err := svc.Update(platform, tenantTestOther, tenant.UpdateRequest{Name: "Renamed"}); err != nil {
		t.Fatalf("platform admin updates another tenant: %v", err)
	}
	if got := store.tenants[tenantTestOther].Name; got != "Renamed" {
		t.Fatalf("name = %q", got)
	}
}

// Disabling the default tenant would lock out the platform admins and the
// worker's service identity: it is refused as a validation error.
func TestTenantUpdate_RefusesDisablingTheDefaultTenant(t *testing.T) {
	store := newTenantFakeStore()
	svc := NewTenantService(store)
	ctx := tenantctx.WithTenant(context.Background(), tenantctx.DefaultTenantID)
	disabled := false
	_, err := svc.Update(ctx, tenantctx.DefaultTenantID, tenant.UpdateRequest{Enabled: &disabled})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if !store.tenants[tenantctx.DefaultTenantID].Enabled {
		t.Fatal("the default tenant was disabled")
	}
	enabled := true
	if _, err := svc.Update(ctx, tenantctx.DefaultTenantID, tenant.UpdateRequest{Enabled: &enabled}); err != nil {
		t.Fatalf("enabling: %v", err)
	}
}
