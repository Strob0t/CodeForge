package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// TestMCPServerRoutesNeedAdmin (KI-71): an MCP server definition names a
// command the worker runs for agents, as the tool user. A tenant's admins
// define, test and assign its servers (KI-71 review: servers are
// tenant-scoped, a platform-admin-only rule left every other tenant without
// any); every user reads them.
func TestMCPServerRoutesNeedAdmin(t *testing.T) {
	const otherTenant = "11111111-2222-3333-4444-555555555555"
	platformAdmin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	tenantAdmin := &user.User{ID: "ta", Role: user.RoleAdmin, TenantID: otherTenant}
	editor := &user.User{ID: "ed", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}
	viewer := &user.User{ID: "vi", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}

	const stdioServer = `{"name":"s","transport":"stdio","command":"/bin/sh","args":["-c","id"]}`
	// An invalid sse definition: the test endpoint answers 400 without connecting.
	const invalidSSE = `{"name":"s","transport":"sse"}`

	routes := []struct {
		name   string
		method string
		path   string
		body   string
		// The least role that may call the route: "platform", "admin" or "any".
		needs string
	}{
		{"create server", http.MethodPost, "/api/v1/mcp/servers", stdioServer, "admin"},
		{"update server", http.MethodPut, "/api/v1/mcp/servers/s1", stdioServer, "admin"},
		{"delete server", http.MethodDelete, "/api/v1/mcp/servers/s1", "", "admin"},
		{"test new server", http.MethodPost, "/api/v1/mcp/servers/test", invalidSSE, "admin"},
		{"test saved server", http.MethodPost, "/api/v1/mcp/servers/s1/test", "", "admin"},
		{"assign to project", http.MethodPost, "/api/v1/projects/p1/mcp-servers", `{"server_id":"s1"}`, "admin"},
		{"unassign from project", http.MethodDelete, "/api/v1/projects/p1/mcp-servers/s1", "", "admin"},
		{"list servers", http.MethodGet, "/api/v1/mcp/servers", "", "any"},
		{"get server", http.MethodGet, "/api/v1/mcp/servers/s1", "", "any"},
		{"list tools", http.MethodGet, "/api/v1/mcp/servers/s1/tools", "", "any"},
		{"list project servers", http.MethodGet, "/api/v1/projects/p1/mcp-servers", "", "any"},
	}
	allowed := func(needs string, u *user.User) bool {
		switch needs {
		case "platform":
			return u == platformAdmin
		case "admin":
			return u.Role == user.RoleAdmin
		default:
			return true
		}
	}

	for _, rt := range routes {
		for _, u := range []*user.User{viewer, editor, tenantAdmin, platformAdmin} {
			t.Run(rt.name+"/"+u.ID, func(t *testing.T) {
				r := newTestRouterWithStore(&mockStore{})
				req := httptest.NewRequest(rt.method, rt.path, bytes.NewReader([]byte(rt.body)))
				req.Header.Set("Content-Type", "application/json")
				req = req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				forbidden := w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized
				if want := !allowed(rt.needs, u); forbidden != want {
					t.Fatalf("%s %s as %s (%s): status %d, forbidden = %v, want %v (body %s)",
						rt.method, rt.path, u.ID, u.Role, w.Code, forbidden, want, w.Body.String())
				}
			})
		}
	}
}

// TestMCPServerTest_StdioIsRefusedInTheCore (KI-71): the Go Core never starts
// a stdio MCP server; only sse and streamable_http servers are tested there.
func TestMCPServerTest_StdioIsRefusedInTheCore(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	stdio := mcp.ServerDef{
		ID: "s1", Name: "s", Transport: mcp.TransportStdio, Command: "/bin/sh",
		Args: []string{"-c", "touch " + marker}, Status: mcp.ServerStatusRegistered,
	}
	body, err := json.Marshal(stdio)
	if err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{"new server": "/api/v1/mcp/servers/test", "saved server": "/api/v1/mcp/servers/s1/test"} {
		t.Run(name, func(t *testing.T) {
			store := &mockStore{mcpServers: []mcp.ServerDef{stdio}}
			r := newTestRouterWithStore(store)
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400 (body %s)", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "stdio servers cannot be tested from the core; they run in the worker") {
				t.Fatalf("body %s", w.Body.String())
			}
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("the core started the stdio command")
			}
			if store.mcpServers[0].Status != mcp.ServerStatusRegistered {
				t.Fatalf("server status changed to %q", store.mcpServers[0].Status)
			}
		})
	}
}
