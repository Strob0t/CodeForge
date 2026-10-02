package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// TestMCPServerURLs_RefusedWith400 (KI-100): a tenant admin chooses the url
// of an sse or streamable_http server and the Go Core connects to it. A url
// on loopback, link-local (cloud metadata) or a private address that the
// platform operator did not allowlist is refused with 400 on create, update
// and both connection tests, before anything connects.
func TestMCPServerURLs_RefusedWith400(t *testing.T) {
	var hits atomic.Int32
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer local.Close()
	tenantAdmin := &user.User{ID: "ta", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}

	for _, tt := range []struct{ url, reason string }{
		{local.URL + "/mcp", "127.0.0.1 is a loopback address; MCP servers may never use it"},
		{"http://169.254.169.254/latest/meta-data/", "169.254.169.254 is a link-local address"},
		{"http://10.0.0.5:6280/mcp", "10.0.0.5 is a private address; only the platform operator can allow a private host (mcp.allowed_private_hosts)"},
	} {
		def := map[string]any{"name": "remote", "transport": "streamable_http", "url": tt.url, "enabled": true}
		body, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		saved := mcp.ServerDef{ID: "s1", Name: "remote", Transport: mcp.TransportStreamableHTTP, URL: tt.url, Status: mcp.ServerStatusRegistered}
		for _, call := range []struct{ name, method, path, body string }{
			{"create", http.MethodPost, "/api/v1/mcp/servers", string(body)},
			{"update", http.MethodPut, "/api/v1/mcp/servers/s1", string(body)},
			{"test new", http.MethodPost, "/api/v1/mcp/servers/test", string(body)},
			{"test saved", http.MethodPost, "/api/v1/mcp/servers/s1/test", ""},
		} {
			t.Run(call.name+" "+tt.url, func(t *testing.T) {
				before := saved
				before.URL = "http://203.0.113.10/mcp"
				if call.name == "test saved" {
					before = saved // a server stored before the check
				}
				store := &mockStore{mcpServers: []mcp.ServerDef{before}}

				w := serveMCP(t, store, tenantAdmin, call.method, call.path, call.body)

				if w.Code != http.StatusBadRequest {
					t.Fatalf("status %d, want 400: %s", w.Code, w.Body.String())
				}
				if !strings.Contains(w.Body.String(), tt.reason) {
					t.Fatalf("body %s does not say %q", w.Body.String(), tt.reason)
				}
				if len(store.mcpServers) != 1 || store.mcpServers[0].URL != before.URL || store.mcpServers[0].Status != mcp.ServerStatusRegistered {
					t.Fatalf("a refused request changed the stored servers: %+v", store.mcpServers)
				}
			})
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("the local server saw %d requests, want none", hits.Load())
	}
}

// TestMCPServerOnARefusedAddress_CanStillBeDisabled (KI-100 review): a server
// saved before KI-100 on an address that is refused now is checked only when
// its url or transport changes.
func TestMCPServerOnARefusedAddress_CanStillBeDisabled(t *testing.T) {
	tenantAdmin := &user.User{ID: "ta", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	saved := mcp.ServerDef{ID: "s1", Name: "old", Transport: mcp.TransportSSE, URL: "http://10.0.0.5:6280/sse", Enabled: true, Status: mcp.ServerStatusRegistered}

	store := &mockStore{mcpServers: []mcp.ServerDef{saved}}
	w := serveMCP(t, store, tenantAdmin, http.MethodPut, "/api/v1/mcp/servers/s1",
		`{"name":"old","transport":"sse","url":"http://10.0.0.5:6280/sse","enabled":false}`)
	if w.Code != http.StatusOK || store.mcpServers[0].Enabled {
		t.Fatalf("disable: status %d, stored %+v; want 200 and disabled: %s", w.Code, store.mcpServers[0], w.Body.String())
	}

	w = serveMCP(t, store, tenantAdmin, http.MethodPut, "/api/v1/mcp/servers/s1",
		`{"name":"old","transport":"sse","url":"http://10.0.0.6:6280/sse","enabled":false}`)
	if w.Code != http.StatusBadRequest || store.mcpServers[0].URL != saved.URL {
		t.Fatalf("another refused url: status %d, stored %+v; want 400 and unchanged: %s", w.Code, store.mcpServers[0], w.Body.String())
	}

	if w = serveMCP(t, store, tenantAdmin, http.MethodDelete, "/api/v1/mcp/servers/s1", ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d: %s", w.Code, w.Body.String())
	}
}
