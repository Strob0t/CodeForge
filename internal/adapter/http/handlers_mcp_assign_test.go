package http_test

import (
	"net/http"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// TestMCPAssign_ProjectMustBeInTheTenant (KI-71 review): an MCP server is
// assigned only to a project the caller's tenant has (the tenant-scoped
// store answers not found for another tenant's project).
func TestMCPAssign_ProjectMustBeInTheTenant(t *testing.T) {
	admin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	server := mcp.ServerDef{ID: "s1", Name: "remote", Transport: mcp.TransportSSE, URL: "http://mcp.example/sse"}

	t.Run("project of another tenant", func(t *testing.T) {
		store := &mockStore{mcpServers: []mcp.ServerDef{server}}
		w := serveMCP(t, store, admin, http.MethodPost, "/api/v1/projects/p-other/mcp-servers", `{"server_id":"s1"}`)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status %d, want 404: %s", w.Code, w.Body.String())
		}
		if len(store.mcpProjectLinks) != 0 {
			t.Fatalf("links = %v, want none", store.mcpProjectLinks)
		}
	})

	t.Run("project of the tenant", func(t *testing.T) {
		store := &mockStore{mcpServers: []mcp.ServerDef{server}, projects: []project.Project{{ID: "p1", Name: "p"}}}
		w := serveMCP(t, store, admin, http.MethodPost, "/api/v1/projects/p1/mcp-servers", `{"server_id":"s1"}`)
		if w.Code >= http.StatusBadRequest {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if len(store.mcpProjectLinks) != 1 || store.mcpProjectLinks[0].ProjectID != "p1" {
			t.Fatalf("links = %v, want p1 -> s1", store.mcpProjectLinks)
		}
	})
}
