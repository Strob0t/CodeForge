package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// tenantMCPStore answers ListMCPServersByProject with the servers of the
// tenant in the context, as the tenant-scoped store does.
type tenantMCPStore struct {
	*runtimeMockStore
	byTenant map[string][]mcp.ServerDef
}

func (s *tenantMCPStore) ListMCPServersByProject(ctx context.Context, _ string) ([]mcp.ServerDef, error) {
	return s.byTenant[tenantctx.FromContext(ctx)], nil
}

// TestResolveForRun_UsesTheRunsTenant (KI-71 review): ResolveForRun read the
// project's MCP servers with context.Background(), so only the default
// tenant's servers ever reached a run. It now reads them in the run's tenant.
func TestResolveForRun_UsesTheRunsTenant(t *testing.T) {
	const tenantB = "bbbbbbbb-0000-0000-0000-000000000002"
	store := &tenantMCPStore{
		runtimeMockStore: &runtimeMockStore{},
		byTenant: map[string][]mcp.ServerDef{
			tenantctx.DefaultTenantID: {{ID: "default-server", Name: "d", Transport: mcp.TransportSSE, URL: "http://d", Enabled: true}},
			tenantB:                   {{ID: "b-server", Name: "b", Transport: mcp.TransportSSE, URL: "http://b", Enabled: true}},
		},
	}
	svc := service.NewMCPService(&config.MCP{}, nil)
	svc.SetStore(store)

	for tenant, want := range map[string]string{tenantB: "b-server", tenantctx.DefaultTenantID: "default-server"} {
		got := svc.ResolveForRun(tenantctx.WithTenant(context.Background(), tenant), "proj-1", "")
		if len(got) != 1 || got[0].ID != want {
			t.Errorf("ResolveForRun in tenant %s = %+v, want only %s", tenant, got, want)
		}
	}
}
