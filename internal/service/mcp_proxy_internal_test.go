package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/netutil"
)

// TestMCPConnectionTest_UseProxy (KI-100 review): with mcp.use_proxy the
// connection test goes through the proxy of the environment; the url's host
// is still checked before anything connects, the address is not pinned (the
// proxy dials). Startup warns that DNS rebinding is then the proxy's job.
func TestMCPConnectionTest_UseProxy(t *testing.T) {
	var mu sync.Mutex
	var proxied []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proxied = append(proxied, r.RequestURI)
		mu.Unlock()
		http.Error(w, "not an MCP server", http.StatusBadGateway)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	svc := NewMCPService(&config.MCP{UseProxy: true}, &config.Limits{MCPTestTimeout: 5 * time.Second})
	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "DNS rebinding") {
		t.Fatalf("no startup warning about use_proxy:\n%s", logs.String())
	}
	svc.proxy = http.ProxyURL(proxyURL)
	policy, err := netutil.NewOutboundPolicy(nil, netutil.WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
		switch host {
		case "mcp.example.com":
			return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
		case "internal.example.com":
			return []netip.Addr{netip.MustParseAddr("10.1.2.3")}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}), netutil.WithDial(func(_ context.Context, _, address string) (net.Conn, error) {
		return nil, errors.New("the policy dialled " + address + " although the proxy connects")
	}))
	if err != nil {
		t.Fatal(err)
	}
	svc.SetOutboundPolicy(policy)

	result, err := svc.TestConnection(context.Background(), &mcp.ServerDef{Name: "s", Transport: mcp.TransportStreamableHTTP, URL: "http://mcp.example.com/mcp"})
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	mu.Lock()
	got := append([]string(nil), proxied...)
	mu.Unlock()
	if result.Success || len(got) == 0 || got[0] != "http://mcp.example.com/mcp" {
		t.Fatalf("result %+v, proxied %v; want the request sent through the proxy", result, got)
	}

	// The host is still checked before anything connects.
	_, err = svc.TestConnection(context.Background(), &mcp.ServerDef{Name: "s", Transport: mcp.TransportStreamableHTTP, URL: "http://internal.example.com/mcp"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("private host with use_proxy = %v, want a validation error", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(proxied) != len(got) {
		t.Fatalf("the refused url reached the proxy: %v", proxied)
	}
}

func TestMCPRunServerPayloads_CarryUseProxy(t *testing.T) {
	svc := NewMCPService(&config.MCP{UseProxy: true}, nil)
	for _, def := range []mcp.ServerDef{
		{ID: "a", Name: "remote", Transport: mcp.TransportSSE, URL: "https://mcp.example.com/sse", Enabled: true},
		{ID: "b", Name: "local", Transport: mcp.TransportStdio, Command: "mcp-files", Enabled: true},
	} {
		if err := svc.Register(def); err != nil {
			t.Fatal(err)
		}
	}
	got := svc.RunServerPayloads(context.Background(), "", "")
	if len(got) != 2 || !got[0].UseProxy || got[1].UseProxy {
		t.Fatalf("payloads = %+v, want use_proxy on the remote server only", got)
	}
}
