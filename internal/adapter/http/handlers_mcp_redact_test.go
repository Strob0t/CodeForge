package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/service"
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
	return serveMCPWith(t, store, u, method, path, body)
}

// serveMCPWith is serveMCP with handler mods (routedMCPPolicy).
func serveMCPWith(t *testing.T, store *mockStore, u *user.User, method, path, body string, mods ...func(*cfhttp.Handlers)) *httptest.ResponseRecorder {
	t.Helper()
	r := newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000", mods...)
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// routedMCPPolicy makes the MCP service resolve each name to a public
// address (from 203.0.113.0/24) and connect that address to the name's local
// server, so tests reach local servers through the outbound policy (KI-100).
func routedMCPPolicy(t *testing.T, servers map[string]*httptest.Server) func(*cfhttp.Handlers) {
	t.Helper()
	names := make(map[string]netip.Addr, len(servers))
	listeners := make(map[netip.Addr]string, len(servers))
	next := netip.MustParseAddr("203.0.113.1")
	for name, srv := range servers {
		names[name] = next
		listeners[next] = srv.Listener.Addr().String()
		next = next.Next()
	}
	policy, err := netutil.NewOutboundPolicy(nil,
		netutil.WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
			if addr, ok := names[host]; ok {
				return []netip.Addr{addr}, nil
			}
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}),
		netutil.WithDial(func(ctx context.Context, network, address string) (net.Conn, error) {
			addrPort, err := netip.ParseAddrPort(address)
			if err != nil {
				return nil, err
			}
			listener, ok := listeners[addrPort.Addr()]
			if !ok {
				return nil, errors.New("no route to " + address)
			}
			var d net.Dialer
			return d.DialContext(ctx, network, listener)
		}))
	if err != nil {
		t.Fatal(err)
	}
	return func(h *cfhttp.Handlers) { h.MCP.SetOutboundPolicy(policy) }
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

	// Security review of the KI-71 round 3: the same command with other
	// arguments (npx -y another-package) would start another program with
	// the stored secrets.
	t.Run("redacted value with changed arguments", func(t *testing.T) {
		store := &mockStore{mcpServers: []mcp.ServerDef{mcpServerWithSecrets()}}
		def := base(map[string]string{"GITHUB_TOKEN": mcp.RedactedValue}, nil)
		def["args"] = []string{"-y", "other-package"}
		w := update(t, store, def)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status %d, want 400: %s", w.Code, w.Body.String())
		}
		if got := store.mcpServers[0]; len(got.Args) != 0 || got.Env["GITHUB_TOKEN"] != mcpTokenValue {
			t.Errorf("a refused update changed the stored server: %+v", got)
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
	elsewhere := make(chan string, 4)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere <- r.Header.Get("Authorization")
		http.Error(w, "not an MCP server", http.StatusInternalServerError)
	}))
	defer other.Close()
	// KI-100: the core never connects to loopback, so the two local servers
	// stand for public hosts.
	routed := routedMCPPolicy(t, map[string]*httptest.Server{"upstream.example": upstream, "other.example": other})

	saved := mcp.ServerDef{
		ID: "s1", Name: "remote", Transport: mcp.TransportStreamableHTTP, URL: "http://upstream.example/mcp",
		Headers: map[string]string{"Authorization": mcpBearerValue}, Status: mcp.ServerStatusRegistered,
	}
	edited := saved.Redacted()
	body, err := json.Marshal(edited)
	if err != nil {
		t.Fatal(err)
	}
	platformAdmin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}

	w := serveMCPWith(t, &mockStore{mcpServers: []mcp.ServerDef{saved}}, platformAdmin, http.MethodPost, "/api/v1/mcp/servers/test", string(body), routed)

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
	edited.URL = "http://other.example/mcp"
	if body, err = json.Marshal(edited); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/mcp/servers/test", "/api/v1/mcp/servers/s1"} {
		method := http.MethodPost
		if path == "/api/v1/mcp/servers/s1" {
			method = http.MethodPut
		}
		store := &mockStore{mcpServers: []mcp.ServerDef{saved}}
		w = serveMCPWith(t, store, platformAdmin, method, path, string(body), routed)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s %s with another url: status %d, want 400: %s", method, path, w.Code, w.Body.String())
		}
		if store.mcpServers[0].URL != saved.URL {
			t.Fatalf("%s %s changed the stored server: %+v", method, path, store.mcpServers[0])
		}
	}
	select {
	case got := <-elsewhere:
		t.Fatalf("the stored header value went to another url: Authorization %q", got)
	default:
	}
}

// KI-97: the url's password and credential arguments are secrets too.
const (
	mcpURLSecret   = "url_secret_value"
	mcpQuerySecret = "sk-query_secret_value"
	mcpArgToken    = "ghp_arg_token_value"
	mcpArgKey      = "arg_api_key_value"
)

func mcpServersWithURLAndArgSecrets() []mcp.ServerDef {
	return []mcp.ServerDef{
		{
			ID: "s1", Name: "github", Transport: mcp.TransportStdio, Command: "npx", Enabled: true,
			Args:   []string{"-y", "mcp-github", "--token=" + mcpArgToken, "--api-key", mcpArgKey, "--root", "/w"},
			Status: mcp.ServerStatusRegistered,
		},
		{
			ID: "s2", Name: "remote", Transport: mcp.TransportSSE, URL: "https://user:" + mcpURLSecret + "@mcp.example/sse?api_key=" + mcpQuerySecret,
			Enabled: true, Status: mcp.ServerStatusRegistered,
		},
	}
}

func TestMCPServerReads_RedactURLPasswordAndCredentialArgs(t *testing.T) {
	admin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	viewer := &user.User{ID: "vi", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}
	for _, path := range []string{"/api/v1/mcp/servers", "/api/v1/mcp/servers/s1", "/api/v1/mcp/servers/s2", "/api/v1/projects/p1/mcp-servers"} {
		for _, u := range []*user.User{viewer, admin} {
			t.Run(path+"/"+u.ID, func(t *testing.T) {
				store := &mockStore{mcpServers: mcpServersWithURLAndArgSecrets()}
				store.mcpProjectLinks = append(store.mcpProjectLinks,
					struct{ ProjectID, ServerID string }{"p1", "s1"}, struct{ ProjectID, ServerID string }{"p1", "s2"})

				w := serveMCP(t, store, u, http.MethodGet, path, "")

				if w.Code != http.StatusOK {
					t.Fatalf("status %d: %s", w.Code, w.Body.String())
				}
				body := w.Body.String()
				for _, secret := range []string{mcpURLSecret, mcpQuerySecret, mcpArgToken, mcpArgKey} {
					if strings.Contains(body, secret) {
						t.Fatalf("response carries a secret: %s", body)
					}
				}
				if path != "/api/v1/mcp/servers/s2" && !strings.Contains(body, `["-y","mcp-github","--token=***","--api-key","***","--root","/w"]`) {
					t.Fatalf("response does not show the arguments with redacted values: %s", body)
				}
				if path != "/api/v1/mcp/servers/s1" && !strings.Contains(body, `"url":"https://***@mcp.example/sse?api_key=***"`) {
					t.Fatalf("response does not show the url with a redacted password: %s", body)
				}
			})
		}
	}
}

func TestMCPServerUpdate_KeepsRedactedURLAndArgs(t *testing.T) {
	admin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	update := func(t *testing.T, store *mockStore, id string, def *mcp.ServerDef) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		return serveMCP(t, store, admin, http.MethodPut, "/api/v1/mcp/servers/"+id, string(body))
	}

	t.Run("sent back as read", func(t *testing.T) {
		store := &mockStore{mcpServers: mcpServersWithURLAndArgSecrets()}
		want := mcpServersWithURLAndArgSecrets()
		for i, id := range []string{"s1", "s2"} {
			read := want[i].Redacted()
			read.Description = "edited"
			w := update(t, store, id, &read)
			if w.Code != http.StatusOK {
				t.Fatalf("update %s: status %d: %s", id, w.Code, w.Body.String())
			}
			for _, secret := range []string{mcpURLSecret, mcpQuerySecret, mcpArgToken, mcpArgKey} {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatalf("update response carries a secret: %s", w.Body.String())
				}
			}
			if got := store.mcpServers[i]; got.URL != want[i].URL || !slices.Equal(got.Args, want[i].Args) || got.Description != "edited" {
				t.Errorf("stored %s = %+v, want the stored secrets kept", id, got)
			}
		}
	})

	// Round 2: a value that looks like a flag ("--api-key" after "--token")
	// is read as ***, and so is the value after it; sent back as read, the
	// arguments are kept as a whole.
	t.Run("a flag value that looks like a flag", func(t *testing.T) {
		saved := mcp.ServerDef{
			ID: "s3", Name: "cascade", Transport: mcp.TransportStdio, Command: "npx",
			Args: []string{"--token", "--api-key", mcpArgKey}, Status: mcp.ServerStatusRegistered,
		}
		store := &mockStore{mcpServers: []mcp.ServerDef{saved}}
		read := saved.Redacted()
		if !slices.Equal(read.Args, []string{"--token", "***", "***"}) {
			t.Fatalf("read args = %q", read.Args)
		}
		read.Enabled = false
		if w := update(t, store, "s3", &read); w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if got := store.mcpServers[0]; !slices.Equal(got.Args, saved.Args) || got.Enabled {
			t.Errorf("stored = %+v, want the arguments kept and the server disabled", got)
		}
	})

	for name, edit := range map[string]func(d *mcp.ServerDef){
		"another host keeps no password":       func(d *mcp.ServerDef) { d.URL = "https://user:***@evil.example/sse" },
		"another flag name keeps no value":     func(d *mcp.ServerDef) { d.Args[2] = "--password=***" },
		"another flag before a separate value": func(d *mcp.ServerDef) { d.Args[3] = "--secret" },
	} {
		t.Run(name, func(t *testing.T) {
			store := &mockStore{mcpServers: mcpServersWithURLAndArgSecrets()}
			i, id := 0, "s1"
			if strings.Contains(name, "host") {
				i, id = 1, "s2"
			}
			read := mcpServersWithURLAndArgSecrets()[i].Redacted()
			edit(&read)
			w := update(t, store, id, &read)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400: %s", w.Code, w.Body.String())
			}
			if want := mcpServersWithURLAndArgSecrets()[i]; store.mcpServers[i].URL != want.URL || !slices.Equal(store.mcpServers[i].Args, want.Args) {
				t.Fatalf("a refused update changed the stored server: %+v", store.mcpServers[i])
			}
		})
	}
}

// TestMCPServerConnectionTest_KeepsRedactedURLPassword (KI-97): testing a
// saved server read with a redacted url password connects with the stored
// password, to the stored host only.
func TestMCPServerConnectionTest_KeepsRedactedURLPassword(t *testing.T) {
	received := make(chan string, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, _ := r.BasicAuth()
		received <- password
		http.Error(w, "not an MCP server", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	elsewhere := make(chan string, 4)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere <- r.Header.Get("Authorization")
		http.Error(w, "not an MCP server", http.StatusInternalServerError)
	}))
	defer other.Close()
	routed := routedMCPPolicy(t, map[string]*httptest.Server{"upstream.example": upstream, "other.example": other})
	saved := mcp.ServerDef{
		ID: "s1", Name: "remote", Transport: mcp.TransportStreamableHTTP,
		URL: "http://user:" + mcpURLSecret + "@upstream.example/mcp", Status: mcp.ServerStatusRegistered,
	}
	admin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	read := saved.Redacted()
	body, err := json.Marshal(read)
	if err != nil {
		t.Fatal(err)
	}

	w := serveMCPWith(t, &mockStore{mcpServers: []mcp.ServerDef{saved}}, admin, http.MethodPost, "/api/v1/mcp/servers/test", string(body), routed)

	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), mcpURLSecret) {
		t.Fatalf("status %d, body %s; want 200 without the password", w.Code, w.Body.String())
	}
	select {
	case got := <-received:
		if got != mcpURLSecret {
			t.Fatalf("the test connected with password %q, want the stored one", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the test never connected")
	}

	read.URL = "http://user:***@other.example/mcp"
	if body, err = json.Marshal(read); err != nil {
		t.Fatal(err)
	}
	w = serveMCPWith(t, &mockStore{mcpServers: []mcp.ServerDef{saved}}, admin, http.MethodPost, "/api/v1/mcp/servers/test", string(body), routed)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("test with another host: status %d, want 400: %s", w.Code, w.Body.String())
	}
	select {
	case got := <-elsewhere:
		t.Fatalf("the stored password went to another host: Authorization %q", got)
	default:
	}
}
