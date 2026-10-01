package postgres_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// TestMCPServers_TenantAdminsManageTheirOwn (KI-71 review): MCP servers are
// tenant-scoped and a server is assigned only to a project of its tenant, so
// a tenant's admins create, change, test and assign the servers of their
// own tenant. They never see or assign another tenant's server. The API runs
// on the PostgreSQL store.
func TestMCPServers_TenantAdminsManageTheirOwn(t *testing.T) {
	store := setupStore(t)
	tenantA, tenantB := createTestTenant(t, store), createTestTenant(t, store)
	ctxA, ctxB := ctxWithTenant(t, tenantA), ctxWithTenant(t, tenantB)
	projA, projB := createMCPProject(ctxA, t, store), createMCPProject(ctxB, t, store)
	serverA := &mcp.ServerDef{ID: uuid.NewString(), Name: "a", Transport: mcp.TransportSSE, URL: "http://a.example/sse", Status: mcp.ServerStatusRegistered}
	if err := store.CreateMCPServer(ctxA, serverA); err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}

	mcpSvc := service.NewMCPService(&config.MCP{}, &config.Limits{MCPTestTimeout: time.Second})
	mcpSvc.SetStore(store)
	r := chi.NewRouter()
	cfhttp.MountRoutes(r, &cfhttp.Handlers{MCP: mcpSvc, Limits: &config.Limits{MaxRequestBodySize: 1 << 20}}, config.Webhook{})
	adminB := &user.User{ID: uuid.NewString(), Role: user.RoleAdmin, TenantID: tenantB}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(middleware.ContextWithTestUser(ctxB, adminB))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// A tenant-B admin creates a server of tenant B and assigns it to a tenant-B project.
	w := call(http.MethodPost, "/api/v1/mcp/servers", `{"name":"b","transport":"sse","url":"http://b.example/sse","enabled":true}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create as tenant admin: status %d: %s", w.Code, w.Body.String())
	}
	var created mcp.ServerDef
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if w = call(http.MethodPost, "/api/v1/projects/"+projB.ID+"/mcp-servers", `{"server_id":"`+created.ID+`"}`); w.Code >= 300 {
		t.Fatalf("assign in tenant B: status %d: %s", w.Code, w.Body.String())
	}
	if w = call(http.MethodPut, "/api/v1/mcp/servers/"+created.ID, `{"name":"b2","transport":"sse","url":"http://b.example/sse","enabled":true}`); w.Code != http.StatusOK {
		t.Fatalf("update as tenant admin: status %d: %s", w.Code, w.Body.String())
	}
	runServers := mcpSvc.ResolveForRun(ctxB, projB.ID, "")
	if len(runServers) != 1 || runServers[0].ID != created.ID {
		t.Fatalf("tenant B's run gets %+v, want its server", runServers)
	}

	// Tenant A's server stays invisible and unassignable for tenant B.
	if w = call(http.MethodGet, "/api/v1/mcp/servers", ""); strings.Contains(w.Body.String(), serverA.ID) || !strings.Contains(w.Body.String(), created.ID) {
		t.Fatalf("tenant B lists %s", w.Body.String())
	}
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/mcp/servers/" + serverA.ID, ""},
		{http.MethodPut, "/api/v1/mcp/servers/" + serverA.ID, `{"name":"x","transport":"sse","url":"http://x.example/sse"}`},
		{http.MethodDelete, "/api/v1/mcp/servers/" + serverA.ID, ""},
		{http.MethodPost, "/api/v1/projects/" + projB.ID + "/mcp-servers", `{"server_id":"` + serverA.ID + `"}`},
		{http.MethodPost, "/api/v1/projects/" + projA.ID + "/mcp-servers", `{"server_id":"` + created.ID + `"}`},
	} {
		if w = call(c.method, c.path, c.body); w.Code != http.StatusNotFound {
			t.Errorf("%s %s as tenant-B admin: status %d, want 404: %s", c.method, c.path, w.Code, w.Body.String())
		}
	}
	if got, err := store.GetMCPServer(ctxA, serverA.ID); err != nil || got.Name != "a" {
		t.Fatalf("tenant A's server = %+v, %v; want it unchanged", got, err)
	}
	if linked, _ := store.ListMCPServersByProject(context.Background(), projA.ID); len(linked) != 0 {
		t.Fatalf("tenant A's project got servers %+v", linked)
	}
}
