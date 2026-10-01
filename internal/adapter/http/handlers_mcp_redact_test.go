package http_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const (
	mcpTokenValue  = "ghp_secret_token_value"
	mcpBearerValue = "Bearer secret_header_value"
)

func mcpServerWithSecrets() mcp.ServerDef {
	return mcp.ServerDef{
		ID: "s1", Name: "github", Transport: mcp.TransportStdio, Command: "mcp-github", Enabled: true,
		Env:     map[string]string{"GITHUB_TOKEN": mcpTokenValue, "EMPTY": ""},
		Headers: map[string]string{"Authorization": mcpBearerValue},
		Status:  mcp.ServerStatusRegistered,
	}
}

func serveMCP(t *testing.T, store *mockStore, u *user.User, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestMCPServerReads_RedactSecrets (KI-71 review): MCP server env variables
// and headers carry credentials. Every read shows the keys and whether a
// value is set (mcp.RedactedValue), never the value - to admins too.
func TestMCPServerReads_RedactSecrets(t *testing.T) {
	platformAdmin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	viewer := &user.User{ID: "vi", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}

	for _, path := range []string{"/api/v1/mcp/servers", "/api/v1/mcp/servers/s1", "/api/v1/projects/p1/mcp-servers"} {
		for _, u := range []*user.User{viewer, platformAdmin} {
			t.Run(path+"/"+u.ID, func(t *testing.T) {
				store := &mockStore{mcpServers: []mcp.ServerDef{mcpServerWithSecrets()}}
				store.mcpProjectLinks = append(store.mcpProjectLinks, struct{ ProjectID, ServerID string }{"p1", "s1"})

				w := serveMCP(t, store, u, http.MethodGet, path, "")

				if w.Code != http.StatusOK {
					t.Fatalf("status %d: %s", w.Code, w.Body.String())
				}
				body := w.Body.String()
				for _, secret := range []string{mcpTokenValue, mcpBearerValue} {
					if strings.Contains(body, secret) {
						t.Fatalf("response carries a secret value: %s", body)
					}
				}
				if !strings.Contains(body, `"GITHUB_TOKEN":"***"`) || !strings.Contains(body, `"Authorization":"***"`) ||
					!strings.Contains(body, `"EMPTY":""`) {
					t.Fatalf("response does not show which values are set: %s", body)
				}
				if store.mcpServers[0].Env["GITHUB_TOKEN"] != mcpTokenValue {
					t.Fatal("the read changed the stored server")
				}
			})
		}
	}
}

// TestMCPServerUpdate_KeepsRedactedValues (KI-71 review): an update that
// sends a value back as mcp.RedactedValue keeps the stored value; new values
// replace it, and the response is redacted. A redacted value for a key with
// no stored value is refused.
func TestMCPServerUpdate_KeepsRedactedValues(t *testing.T) {
	platformAdmin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	update := func(t *testing.T, store *mockStore, def map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		return serveMCP(t, store, platformAdmin, http.MethodPut, "/api/v1/mcp/servers/s1", string(body))
	}
	base := func(env, headers map[string]string) map[string]any {
		return map[string]any{
			"name": "github", "transport": "stdio", "command": "mcp-github", "enabled": true,
			"env": env, "headers": headers,
		}
	}

	t.Run("sent back as read", func(t *testing.T) {
		store := &mockStore{mcpServers: []mcp.ServerDef{mcpServerWithSecrets()}}
		w := update(t, store, base(
			map[string]string{"GITHUB_TOKEN": mcp.RedactedValue, "EMPTY": "", "REGION": "eu"},
			map[string]string{"Authorization": mcp.RedactedValue},
		))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		got := store.mcpServers[0]
		if want := map[string]string{"GITHUB_TOKEN": mcpTokenValue, "EMPTY": "", "REGION": "eu"}; !maps.Equal(got.Env, want) {
			t.Errorf("stored env = %v, want %v", got.Env, want)
		}
		if got.Headers["Authorization"] != mcpBearerValue {
			t.Errorf("stored headers = %v, want the old Authorization value", got.Headers)
		}
		if strings.Contains(w.Body.String(), mcpTokenValue) || strings.Contains(w.Body.String(), mcpBearerValue) ||
			strings.Contains(w.Body.String(), `"REGION":"eu"`) {
			t.Errorf("update response carries a secret value: %s", w.Body.String())
		}
	})

	t.Run("new value", func(t *testing.T) {
		store := &mockStore{mcpServers: []mcp.ServerDef{mcpServerWithSecrets()}}
		w := update(t, store, base(map[string]string{"GITHUB_TOKEN": "ghp_new"}, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if got := store.mcpServers[0]; got.Env["GITHUB_TOKEN"] != "ghp_new" || len(got.Headers) != 0 {
			t.Errorf("stored = env %v, headers %v; want the new token and no headers", got.Env, got.Headers)
		}
	})

	t.Run("redacted value without a stored one", func(t *testing.T) {
		store := &mockStore{mcpServers: []mcp.ServerDef{mcpServerWithSecrets()}}
		w := update(t, store, base(map[string]string{"OTHER_TOKEN": mcp.RedactedValue}, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status %d, want 400: %s", w.Code, w.Body.String())
		}
		if store.mcpServers[0].Env["GITHUB_TOKEN"] != mcpTokenValue {
			t.Error("a refused update changed the stored server")
		}
	})

	t.Run("create with a redacted value", func(t *testing.T) {
		store := &mockStore{}
		body, err := json.Marshal(base(map[string]string{"GITHUB_TOKEN": mcp.RedactedValue}, nil))
		if err != nil {
			t.Fatal(err)
		}
		w := serveMCP(t, store, platformAdmin, http.MethodPost, "/api/v1/mcp/servers", string(body))
		if w.Code != http.StatusBadRequest || len(store.mcpServers) != 0 {
			t.Fatalf("status %d, stored %d servers; want 400 and none: %s", w.Code, len(store.mcpServers), w.Body.String())
		}
	})

	t.Run("create answers redacted", func(t *testing.T) {
		store := &mockStore{}
		body, err := json.Marshal(base(map[string]string{"GITHUB_TOKEN": mcpTokenValue}, nil))
		if err != nil {
			t.Fatal(err)
		}
		w := serveMCP(t, store, platformAdmin, http.MethodPost, "/api/v1/mcp/servers", string(body))
		if w.Code != http.StatusCreated || strings.Contains(w.Body.String(), mcpTokenValue) {
			t.Fatalf("status %d, body %s; want 201 without the token", w.Code, w.Body.String())
		}
		if store.mcpServers[0].Env["GITHUB_TOKEN"] != mcpTokenValue {
			t.Errorf("stored env = %v, want the token", store.mcpServers[0].Env)
		}
	})
}

// TestMCPServerConnectionTest_KeepsRedactedValues (KI-71 review): testing an
// edited saved server that still carries the redacted values it was read
// with connects with the stored values - to the stored url only.
func TestMCPServerConnectionTest_KeepsRedactedValues(t *testing.T) {
	received := make(chan string, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		http.Error(w, "not an MCP server", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	saved := mcp.ServerDef{
		ID: "s1", Name: "remote", Transport: mcp.TransportStreamableHTTP, URL: upstream.URL,
		Headers: map[string]string{"Authorization": mcpBearerValue}, Status: mcp.ServerStatusRegistered,
	}
	edited := saved.Redacted()
	body, err := json.Marshal(edited)
	if err != nil {
		t.Fatal(err)
	}
	platformAdmin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}

	w := serveMCP(t, &mockStore{mcpServers: []mcp.ServerDef{saved}}, platformAdmin, http.MethodPost, "/api/v1/mcp/servers/test", string(body))

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	select {
	case got := <-received:
		if got != mcpBearerValue {
			t.Fatalf("the test connected with Authorization %q, want the stored value", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the test never connected")
	}

	// The stored values go only to the stored url: another one gets nothing.
	elsewhere := make(chan string, 4)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere <- r.Header.Get("Authorization")
		http.Error(w, "not an MCP server", http.StatusInternalServerError)
	}))
	defer other.Close()
	edited.URL = other.URL
	if body, err = json.Marshal(edited); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/mcp/servers/test", "/api/v1/mcp/servers/s1"} {
		method := http.MethodPost
		if path == "/api/v1/mcp/servers/s1" {
			method = http.MethodPut
		}
		store := &mockStore{mcpServers: []mcp.ServerDef{saved}}
		w = serveMCP(t, store, platformAdmin, method, path, string(body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s %s with another url: status %d, want 400: %s", method, path, w.Code, w.Body.String())
		}
		if store.mcpServers[0].URL != upstream.URL {
			t.Fatalf("%s %s changed the stored server: %+v", method, path, store.mcpServers[0])
		}
	}
	select {
	case got := <-elsewhere:
		t.Fatalf("the stored header value went to another url: Authorization %q", got)
	default:
	}
}
