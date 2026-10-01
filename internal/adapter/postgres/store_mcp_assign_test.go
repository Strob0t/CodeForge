package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// TestStore_MCPProjectAssignmentsStayInTheTenant (KI-71 review): a server is
// linked only to a project of its own tenant, the link carries that tenant,
// and only that tenant removes it.
func TestStore_MCPProjectAssignmentsStayInTheTenant(t *testing.T) {
	store := setupStore(t)
	pool := retentionPool(t)
	tenantA := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, createTestTenant(t, store))
	projA := createMCPProject(ctxA, t, store)
	projB := createMCPProject(ctxB, t, store)
	srvA := &mcp.ServerDef{ID: uuid.NewString(), Name: "a", Transport: mcp.TransportSSE, URL: "http://mcp.example/sse", Enabled: true, Status: mcp.ServerStatusRegistered}
	if err := store.CreateMCPServer(ctxA, srvA); err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}

	for name, call := range map[string]func() error{
		"server to a project of another tenant": func() error { return store.AssignMCPServerToProject(ctxA, projB.ID, srvA.ID) },
		"another tenant's server":               func() error { return store.AssignMCPServerToProject(ctxB, projB.ID, srvA.ID) },
		"in another tenant's name":              func() error { return store.AssignMCPServerToProject(ctxB, projA.ID, srvA.ID) },
	} {
		if err := call(); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("assign %s = %v, want domain.ErrNotFound", name, err)
		}
	}

	for range 2 { // assigning an assigned server changes nothing
		if err := store.AssignMCPServerToProject(ctxA, projA.ID, srvA.ID); err != nil {
			t.Fatalf("AssignMCPServerToProject: %v", err)
		}
	}
	var linkTenant string
	if err := pool.QueryRow(context.Background(),
		`SELECT tenant_id::text FROM project_mcp_servers WHERE project_id = $1 AND mcp_server_id = $2`, projA.ID, srvA.ID,
	).Scan(&linkTenant); err != nil {
		t.Fatalf("read link: %v", err)
	}
	if linkTenant != tenantA {
		t.Errorf("link tenant = %s, want the project tenant %s", linkTenant, tenantA)
	}
	if servers, err := store.ListMCPServersByProject(ctxA, projA.ID); err != nil || len(servers) != 1 {
		t.Fatalf("ListMCPServersByProject = %v, %v; want the server", servers, err)
	}

	if err := store.UnassignMCPServerFromProject(ctxB, projA.ID, srvA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unassign in another tenant's name = %v, want domain.ErrNotFound", err)
	}
	if servers, _ := store.ListMCPServersByProject(ctxA, projA.ID); len(servers) != 1 {
		t.Fatal("another tenant removed the link")
	}
	if err := store.UnassignMCPServerFromProject(ctxA, projA.ID, srvA.ID); err != nil {
		t.Fatalf("UnassignMCPServerFromProject: %v", err)
	}
	if servers, _ := store.ListMCPServersByProject(ctxA, projA.ID); len(servers) != 0 {
		t.Fatalf("servers after unassign = %v, want none", servers)
	}
}

func createMCPProject(ctx context.Context, t *testing.T, store *postgres.Store) *project.Project {
	t.Helper()
	proj, err := store.CreateProject(ctx, &project.CreateRequest{Name: "mcp-assign", Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return proj
}
