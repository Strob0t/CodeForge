package service_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-100: the connection test, create and update of an sse or
// streamable_http MCP server refuse a url whose host is or resolves to a
// loopback, link-local, metadata, ... address (always) or a private one
// (unless the platform operator allowlisted it), before anything connects.

// recordingMCPStore stores MCP servers in memory.
type recordingMCPStore struct {
	*runtimeMockStore
	mu      sync.Mutex
	servers map[string]mcp.ServerDef
	writes  int
}

func newRecordingMCPStore(servers ...mcp.ServerDef) *recordingMCPStore {
	s := &recordingMCPStore{runtimeMockStore: &runtimeMockStore{}, servers: make(map[string]mcp.ServerDef)}
	for i := range servers {
		s.servers[servers[i].ID] = servers[i]
	}
	return s
}

func (s *recordingMCPStore) CreateMCPServer(_ context.Context, srv *mcp.ServerDef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	s.servers[srv.ID] = *srv
	return nil
}

func (s *recordingMCPStore) UpdateMCPServer(_ context.Context, srv *mcp.ServerDef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	s.servers[srv.ID] = *srv
	return nil
}

func (s *recordingMCPStore) GetMCPServer(_ context.Context, id string) (*mcp.ServerDef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	srv, ok := s.servers[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &srv, nil
}

// countingServer is a local server that counts its requests.
func countingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func newMCPTestService(t *testing.T, allowed []string, store *recordingMCPStore) *service.MCPService {
	t.Helper()
	svc := service.NewMCPService(&config.MCP{AllowedPrivateHosts: allowed}, &config.Limits{MCPTestTimeout: 5 * time.Second})
	svc.SetStore(store)
	return svc
}

func TestMCPConnectionTest_RefusesBeforeConnecting(t *testing.T) {
	local, hits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	port := strings.TrimPrefix(local.URL, "http://127.0.0.1:")
	// The allowlist never opens loopback or link-local.
	svc := newMCPTestService(t, []string{"localhost", "127.0.0.1", "169.254.0.0/16"}, newRecordingMCPStore())

	tests := []struct {
		url  string
		want string
	}{
		{local.URL + "/sse", "127.0.0.1 is a loopback address; MCP servers may never use it"},
		{"http://localhost:" + port + "/mcp", "a loopback address"},
		{"http://[::1]:" + port + "/mcp", "::1 is a loopback address"},
		{"http://[::ffff:127.0.0.1]:" + port + "/mcp", "a loopback address"},
		{"http://169.254.169.254/latest/meta-data/", "a link-local address"},
		{"http://[fd00:ec2::254]/latest/meta-data/", "a cloud metadata address"},
		{"http://0.0.0.0:" + port + "/sse", "an unspecified address"},
	}
	for _, tt := range tests {
		for _, transport := range []mcp.TransportType{mcp.TransportSSE, mcp.TransportStreamableHTTP} {
			t.Run(string(transport)+" "+tt.url, func(t *testing.T) {
				result, err := svc.TestConnection(context.Background(), &mcp.ServerDef{Name: "s", Transport: transport, URL: tt.url})
				if !errors.Is(err, domain.ErrValidation) || result != nil {
					t.Fatalf("TestConnection = %+v, %v; want a validation error", result, err)
				}
				if !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("error %q does not say %q", err, tt.want)
				}
			})
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("the local server saw %d requests, want none", hits.Load())
	}
}

func TestMCPConnectionTest_PrivateAddressNamesTheAllowlist(t *testing.T) {
	svc := newMCPTestService(t, nil, newRecordingMCPStore())
	_, err := svc.TestConnection(context.Background(), &mcp.ServerDef{Name: "s", Transport: mcp.TransportSSE, URL: "http://10.0.0.1:6280/sse"})
	if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "10.0.0.1 is a private address") ||
		!strings.Contains(err.Error(), "mcp.allowed_private_hosts") {
		t.Fatalf("TestConnection = %v, want a validation error naming mcp.allowed_private_hosts", err)
	}
}

// TestMCPUpdate_ChecksTheURLOnlyWhenItChanges (KI-100 review): a server
// saved before KI-100 on an address that is refused now can still be
// disabled, renamed or edited otherwise (the worker refuses it when it
// connects); a changed url or transport is checked.
func TestMCPUpdate_ChecksTheURLOnlyWhenItChanges(t *testing.T) {
	saved := mcp.ServerDef{
		ID: "s1", Name: "old", Transport: mcp.TransportSSE, URL: "http://user:pw@10.0.0.5/sse?api_key=k",
		Headers: map[string]string{"Authorization": "Bearer h"}, Enabled: true, Status: mcp.ServerStatusRegistered,
	}
	tests := []struct {
		name    string
		edit    func(d *mcp.ServerDef)
		wantErr bool
	}{
		{name: "disable", edit: func(d *mcp.ServerDef) { d.Enabled = false }},
		{name: "rename", edit: func(d *mcp.ServerDef) { d.Name = "renamed"; d.Description = "kept for history" }},
		{name: "another refused url", edit: func(d *mcp.ServerDef) { d.URL = "http://127.0.0.1:6280/sse"; d.Headers = nil }, wantErr: true},
		{name: "another private url", edit: func(d *mcp.ServerDef) { d.URL = "http://10.0.0.6/sse"; d.Headers = nil }, wantErr: true},
		{name: "another transport", edit: func(d *mcp.ServerDef) { d.Transport = mcp.TransportStreamableHTTP; d.Headers = nil; d.URL = saved.URL }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newRecordingMCPStore(saved)
			svc := newMCPTestService(t, nil, store)
			update := saved.Redacted() // what the client read
			tt.edit(&update)

			err := svc.UpdateDB(context.Background(), &update)

			if tt.wantErr {
				if !errors.Is(err, domain.ErrValidation) || store.writes != 0 {
					t.Fatalf("UpdateDB = %v with %d writes, want a validation error and nothing stored", err, store.writes)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateDB: %v", err)
			}
			got := store.servers["s1"]
			if got.URL != saved.URL || got.Headers["Authorization"] != "Bearer h" {
				t.Errorf("stored = %+v, want the url and headers kept", got)
			}
		})
	}
}

func TestMCPCreateUpdate_RefuseURLs(t *testing.T) {
	saved := mcp.ServerDef{ID: "s1", Name: "remote", Transport: mcp.TransportSSE, URL: "http://203.0.113.10/sse", Status: mcp.ServerStatusRegistered}
	refused := []string{
		"http://127.0.0.1:6280/sse",
		"http://localhost/sse",
		"http://169.254.169.254/",
		"http://[fe80::1]/sse",
		"http://10.1.2.3/sse",
		"http://[fd00:1::5]/sse",
		"http://100.64.0.1/sse",
		"http://224.0.0.1/sse",
	}
	for _, rawURL := range refused {
		t.Run(rawURL, func(t *testing.T) {
			store := newRecordingMCPStore(saved)
			svc := newMCPTestService(t, nil, store)
			if _, err := svc.CreateDB(context.Background(), &mcp.ServerDef{Name: "n", Transport: mcp.TransportStreamableHTTP, URL: rawURL}); !errors.Is(err, domain.ErrValidation) {
				t.Errorf("CreateDB = %v, want domain.ErrValidation", err)
			}
			update := saved
			update.URL = rawURL
			if err := svc.UpdateDB(context.Background(), &update); !errors.Is(err, domain.ErrValidation) {
				t.Errorf("UpdateDB = %v, want domain.ErrValidation", err)
			}
			if store.writes != 0 || store.servers["s1"].URL != saved.URL {
				t.Errorf("a refused url was stored: %d writes, %+v", store.writes, store.servers["s1"])
			}
		})
	}

	t.Run("public and allowlisted urls", func(t *testing.T) {
		store := newRecordingMCPStore(saved)
		svc := newMCPTestService(t, []string{"10.0.0.0/8", "fd00:1::/32"}, store)
		for _, rawURL := range []string{"https://203.0.113.10/mcp", "http://10.1.2.3/sse", "http://[fd00:1::5]:8080/sse", "http://mcp.example.invalid/sse"} {
			if _, err := svc.CreateDB(context.Background(), &mcp.ServerDef{Name: "n", Transport: mcp.TransportSSE, URL: rawURL}); err != nil {
				t.Errorf("CreateDB(%s) = %v, want stored", rawURL, err)
			}
		}
		update := saved
		update.URL = "http://10.9.9.9/sse"
		if err := svc.UpdateDB(context.Background(), &update); err != nil {
			t.Errorf("UpdateDB to an allowlisted address = %v", err)
		}
	})

	t.Run("stdio servers have no url to check", func(t *testing.T) {
		svc := newMCPTestService(t, nil, newRecordingMCPStore())
		if _, err := svc.CreateDB(context.Background(), &mcp.ServerDef{Name: "n", Transport: mcp.TransportStdio, Command: "mcp-files", URL: "http://127.0.0.1/"}); err != nil {
			t.Errorf("CreateDB(stdio) = %v", err)
		}
	})
}

// TestMCPRunServerPayloads_CarryTheAllowlist (KI-100): the worker connects
// to sse and streamable_http servers in runs and applies the same rules, so
// each such server carries the operator's allowlist. Runs and conversations
// build their payloads the same way (conversations dropped the headers).
func TestMCPRunServerPayloads_CarryTheAllowlist(t *testing.T) {
	svc := service.NewMCPService(&config.MCP{AllowedPrivateHosts: []string{"docs-mcp", "10.20.0.0/16"}}, nil)
	for _, def := range []mcp.ServerDef{
		{ID: "a-remote", Name: "remote", Description: "docs", Transport: mcp.TransportStreamableHTTP, URL: "http://docs-mcp:6280/mcp",
			Headers: map[string]string{"Authorization": "Bearer t"}, Enabled: true},
		{ID: "b-local", Name: "local", Transport: mcp.TransportStdio, Command: "mcp-files", Args: []string{"--root", "/w"},
			Env: map[string]string{"TOKEN": "x"}, Enabled: true},
		{ID: "c-off", Name: "off", Transport: mcp.TransportSSE, URL: "http://203.0.113.1/sse"},
	} {
		if err := svc.Register(def); err != nil {
			t.Fatal(err)
		}
	}

	got := svc.RunServerPayloads(context.Background(), "", "")

	if len(got) != 2 {
		t.Fatalf("payloads = %+v, want the two enabled servers", got)
	}
	remote, local := got[0], got[1]
	if remote.ID != "a-remote" || remote.Description != "docs" || remote.Transport != "streamable_http" ||
		remote.URL != "http://docs-mcp:6280/mcp" || remote.Headers["Authorization"] != "Bearer t" || !remote.Enabled {
		t.Errorf("remote payload = %+v", remote)
	}
	if strings.Join(remote.AllowedPrivateHosts, ",") != "docs-mcp,10.20.0.0/16" {
		t.Errorf("remote allowed_private_hosts = %v, want the configured list", remote.AllowedPrivateHosts)
	}
	if local.Command != "mcp-files" || strings.Join(local.Args, " ") != "--root /w" || local.Env["TOKEN"] != "x" ||
		local.AllowedPrivateHosts != nil {
		t.Errorf("stdio payload = %+v, want command, args, env and no allowlist", local)
	}
}

// routedPolicy resolves names to fixed addresses and connects every checked
// address to the local server that stands for it.
func routedPolicy(t *testing.T, names map[string]string, servers map[string]*httptest.Server) *netutil.OutboundPolicy {
	t.Helper()
	policy, err := netutil.NewOutboundPolicy(nil,
		netutil.WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
			ip, ok := names[host]
			if !ok {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			return []netip.Addr{netip.MustParseAddr(ip)}, nil
		}),
		netutil.WithDial(func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(address)
			srv, ok := servers[host]
			if !ok {
				return nil, errors.New("no route to " + address)
			}
			var d net.Dialer
			return d.DialContext(ctx, network, srv.Listener.Addr().String())
		}))
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func TestMCPConnectionTest_RedirectsStayInTheOrigin(t *testing.T) {
	internal, internalHits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	other, otherHits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	var target atomic.Value
	public, publicHits := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.Load().(string), http.StatusTemporaryRedirect)
	})
	svc := newMCPTestService(t, nil, newRecordingMCPStore())
	svc.SetOutboundPolicy(routedPolicy(t,
		map[string]string{"mcp.example": "203.0.113.10", "other.example": "203.0.113.20"},
		map[string]*httptest.Server{"203.0.113.10": public, "203.0.113.20": other}))

	for name, location := range map[string]string{
		"to a private address":   internal.URL + "/steal",
		"to another public host": "http://other.example/collect",
	} {
		t.Run(name, func(t *testing.T) {
			target.Store(location)
			before := publicHits.Load()
			result, err := svc.TestConnection(context.Background(), &mcp.ServerDef{
				Name: "s", Transport: mcp.TransportStreamableHTTP, URL: "http://mcp.example/mcp",
				Headers: map[string]string{"X-Api-Key": "k"},
			})
			if err != nil {
				t.Fatalf("TestConnection: %v", err)
			}
			if result.Success {
				t.Fatalf("result %+v, want a failed connection", result)
			}
			if publicHits.Load() == before {
				t.Fatal("the test never reached the public server")
			}
		})
	}
	if internalHits.Load() != 0 || otherHits.Load() != 0 {
		t.Fatalf("redirect targets saw %d (internal) and %d (other) requests, want none", internalHits.Load(), otherHits.Load())
	}
}
