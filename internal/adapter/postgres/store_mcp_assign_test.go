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

// TestStore_UpsertMCPServerToolsStaysInTheTenant (KI-101): the tool cache of
// a server is replaced only for a server of the caller's tenant; another
// tenant's server is domain.ErrNotFound and keeps its tools.
func TestStore_UpsertMCPServerToolsStaysInTheTenant(t *testing.T) {
	store := setupStore(t)
	pool := retentionPool(t)
	tenantA := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, createTestTenant(t, store))
	srvA := &mcp.ServerDef{ID: uuid.NewString(), Name: "a", Transport: mcp.TransportSSE, URL: "http://mcp.example/sse", Enabled: true, Status: mcp.ServerStatusRegistered}
	if err := store.CreateMCPServer(ctxA, srvA); err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	toolsA := []mcp.ServerTool{{Name: "search", Description: "a's tool"}, {Name: "fetch"}}
	if err := store.UpsertMCPServerTools(ctxA, srvA.ID, toolsA); err != nil {
		t.Fatalf("UpsertMCPServerTools: %v", err)
	}

	for name, tools := range map[string][]mcp.ServerTool{
		"replace":   {{Name: "planted", Description: "by another tenant"}},
		"clear all": nil,
	} {
		if err := store.UpsertMCPServerTools(ctxB, srvA.ID, tools); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s in another tenant's name = %v, want domain.ErrNotFound", name, err)
		}
	}
	if err := store.UpsertMCPServerTools(ctxA, uuid.NewString(), toolsA); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("upsert for an unknown server = %v, want domain.ErrNotFound", err)
	}

	got, err := store.ListMCPServerTools(ctxA, srvA.ID)
	if err != nil {
		t.Fatalf("ListMCPServerTools: %v", err)
	}
	if len(got) != 2 || got[0].Name != "fetch" || got[1].Name != "search" {
		t.Fatalf("tools after another tenant's upsert = %+v, want a's two tools", got)
	}
	var rowTenant string
	if err := pool.QueryRow(context.Background(),
		`SELECT DISTINCT tenant_id::text FROM mcp_server_tools WHERE server_id = $1`, srvA.ID,
	).Scan(&rowTenant); err != nil {
		t.Fatalf("read tool tenant: %v", err)
	}
	if rowTenant != tenantA {
		t.Errorf("tool rows carry tenant %s, want the server's tenant %s", rowTenant, tenantA)
	}

	// The owner replaces and clears its own tools.
	if err := store.UpsertMCPServerTools(ctxA, srvA.ID, toolsA[:1]); err != nil {
		t.Fatalf("UpsertMCPServerTools (replace): %v", err)
	}
	if got, _ := store.ListMCPServerTools(ctxA, srvA.ID); len(got) != 1 || got[0].Name != "search" {
		t.Fatalf("tools after replace = %+v, want only search", got)
	}
	if err := store.UpsertMCPServerTools(ctxA, srvA.ID, nil); err != nil {
		t.Fatalf("UpsertMCPServerTools (clear): %v", err)
	}
	if got, _ := store.ListMCPServerTools(ctxA, srvA.ID); len(got) != 0 {
		t.Fatalf("tools after clear = %+v, want none", got)
	}
}
