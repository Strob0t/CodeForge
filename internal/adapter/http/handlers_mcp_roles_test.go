package http_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// TestMCPServerRoutesNeedAdmin (KI-71): an MCP server definition names a
// command the worker runs for agents. Only platform admins define servers
// (they belong to no project); admins assign them to projects and test them;
// every user reads them.
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
		{"create server", http.MethodPost, "/api/v1/mcp/servers", stdioServer, "platform"},
		{"update server", http.MethodPut, "/api/v1/mcp/servers/s1", stdioServer, "platform"},
		{"delete server", http.MethodDelete, "/api/v1/mcp/servers/s1", "", "platform"},
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
