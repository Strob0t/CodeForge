package netutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

// KI-100: an outbound connection to a URL a tenant supplied (MCP servers)
// never reaches loopback, link-local (cloud metadata), unspecified,
// multicast or reserved addresses, and reaches private ones only for hosts
// the platform operator allowlisted.

func TestOutboundPolicy_CheckAddr(t *testing.T) {
	policy, err := NewOutboundPolicy([]string{"docs-mcp", "Tools.Internal.", "10.20.0.0/16", "fd12:3456::/32", "192.168.7.7"})
	if err != nil {
		t.Fatalf("NewOutboundPolicy: %v", err)
	}
	tests := []struct {
		name      string
		host      string
		ip        string
		allowed   bool
		allowable bool // refused only because it is private
	}{
		// IPv4
		{"public v4", "example.com", "93.184.216.34", true, false},
		{"loopback v4", "evil.example", "127.0.0.1", false, false},
		{"loopback v4 range", "evil.example", "127.255.0.9", false, false},
		{"unspecified v4", "evil.example", "0.0.0.0", false, false},
		{"this network v4", "evil.example", "0.1.2.3", false, false},
		{"link-local v4", "evil.example", "169.254.10.10", false, false},
		{"metadata v4", "evil.example", "169.254.169.254", false, false},
		{"metadata alibaba", "evil.example", "100.100.100.200", false, false},
		{"multicast v4", "evil.example", "239.1.2.3", false, false},
		{"broadcast", "evil.example", "255.255.255.255", false, false},
		{"reserved v4", "evil.example", "240.0.0.1", false, false},
		{"rfc1918 10", "evil.example", "10.0.0.1", false, true},
		{"rfc1918 172", "evil.example", "172.16.5.4", false, true},
		{"rfc1918 192", "evil.example", "192.168.1.1", false, true},
		{"cgnat", "evil.example", "100.64.0.1", false, true},
		{"benchmarking", "evil.example", "198.18.0.1", false, true},
		{"just outside 172.16/12", "evil.example", "172.32.0.1", true, false},
		// IPv6
		{"public v6", "example.com", "2606:2800:220:1:248:1893:25c8:1946", true, false},
		{"loopback v6", "evil.example", "::1", false, false},
		{"unspecified v6", "evil.example", "::", false, false},
		{"link-local v6", "evil.example", "fe80::1", false, false},
		{"link-local v6 with zone", "evil.example", "fe80::1%eth0", false, false},
		{"multicast v6", "evil.example", "ff02::1", false, false},
		{"metadata v6", "evil.example", "fd00:ec2::254", false, false},
		{"ula", "evil.example", "fd00:1::5", false, true},
		{"site-local v6", "evil.example", "fec0::1", false, true},
		// IPv4 inside IPv6
		{"v4-mapped loopback", "evil.example", "::ffff:127.0.0.1", false, false},
		{"v4-mapped metadata", "evil.example", "::ffff:169.254.169.254", false, false},
		{"v4-mapped private", "evil.example", "::ffff:10.0.0.1", false, true},
		{"v4-mapped public", "example.com", "::ffff:93.184.216.34", true, false},
		{"v4-compatible loopback", "evil.example", "::127.0.0.1", false, false},
		{"nat64 metadata", "evil.example", "64:ff9b::169.254.169.254", false, false},
		{"nat64 private", "evil.example", "64:ff9b::10.0.0.1", false, true},
		{"nat64 public", "example.com", "64:ff9b::93.184.216.34", true, false},
		// Allowlist
		{"allowlisted host", "docs-mcp", "172.18.0.5", true, false},
		{"allowlisted host, case and trailing dot", "TOOLS.internal.", "10.9.9.9", true, false},
		{"allowlisted host stays off loopback", "docs-mcp", "127.0.0.1", false, false},
		{"allowlisted host stays off metadata", "docs-mcp", "169.254.169.254", false, false},
		{"allowlisted host stays off v6 metadata", "docs-mcp", "fd00:ec2::254", false, false},
		{"allowlisted cidr", "evil.example", "10.20.3.4", true, false},
		{"allowlisted cidr, v4-mapped", "evil.example", "::ffff:10.20.3.4", true, false},
		{"outside the allowlisted cidr", "evil.example", "10.21.0.1", false, true},
		{"allowlisted v6 cidr", "evil.example", "fd12:3456::9", true, false},
		{"allowlisted single ip", "evil.example", "192.168.7.7", true, false},
		{"other host on an allowlisted name's network", "docs-mcp.evil.example", "172.18.0.5", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := policy.CheckAddr(tt.host, netip.MustParseAddr(tt.ip))
			if tt.allowed {
				if err != nil {
					t.Fatalf("CheckAddr(%s, %s) = %v, want allowed", tt.host, tt.ip, err)
				}
				return
			}
			if !errors.Is(err, ErrAddressRefused) {
				t.Fatalf("CheckAddr(%s, %s) = %v, want ErrAddressRefused", tt.host, tt.ip, err)
			}
			var refused *RefusedAddressError
			if !errors.As(err, &refused) || refused.Allowable() != tt.allowable {
				t.Fatalf("CheckAddr(%s, %s) = %v, allowable = %v, want %v", tt.host, tt.ip, err, refused != nil && refused.Allowable(), tt.allowable)
			}
		})
	}
}

func TestOutboundPolicy_EmptyAllowlistRefusesPrivate(t *testing.T) {
	policy, err := NewOutboundPolicy(nil)
	if err != nil {
		t.Fatalf("NewOutboundPolicy: %v", err)
	}
	if err := policy.CheckAddr("docs-mcp", netip.MustParseAddr("172.18.0.5")); !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("private address without an allowlist = %v, want refused", err)
	}
	if err := policy.CheckAddr("example.com", netip.MustParseAddr("93.184.216.34")); err != nil {
		t.Fatalf("public address = %v, want allowed", err)
	}
	if err := policy.CheckAddr("x", netip.Addr{}); !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("invalid address = %v, want refused", err)
	}
}

func TestOutboundPolicy_AllowlistNeverOpensTheRefusedRanges(t *testing.T) {
	policy, err := NewOutboundPolicy([]string{"0.0.0.0/0", "::/0", "localhost", "127.0.0.1", "169.254.169.254"})
	if err != nil {
		t.Fatalf("NewOutboundPolicy: %v", err)
	}
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "0.0.0.0", "224.0.0.1", "::1", "fe80::1", "fd00:ec2::254", "100.100.100.200"} {
		if err := policy.CheckAddr("localhost", netip.MustParseAddr(ip)); !errors.Is(err, ErrAddressRefused) {
			t.Errorf("CheckAddr(localhost, %s) = %v, want refused", ip, err)
		}
	}
	if err := policy.CheckAddr("any.example", netip.MustParseAddr("10.0.0.1")); err != nil {
		t.Errorf("private address with 0.0.0.0/0 allowlisted = %v, want allowed", err)
	}
}

func TestNewOutboundPolicy_InvalidEntries(t *testing.T) {
	for _, entry := range []string{"", " ", "docs-mcp:6280", "http://docs-mcp", "10.0.0.0/33", "*.internal", "docs mcp", "a/b", ".internal"} {
		t.Run(fmt.Sprintf("%q", entry), func(t *testing.T) {
			if _, err := NewOutboundPolicy([]string{entry}); err == nil {
				t.Fatalf("NewOutboundPolicy(%q) = nil error, want an error", entry)
			}
		})
	}
	if _, err := NewOutboundPolicy([]string{" docs-mcp ", "docs_mcp_1", "fd00::/8", "::1"}); err != nil {
		t.Fatalf("valid entries: %v", err)
	}
}

// fakeLookup resolves the given names; anything else does not resolve.
func fakeLookup(names map[string][]string) OutboundOption {
	return WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
		ips, ok := names[host]
		if !ok {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		out := make([]netip.Addr, 0, len(ips))
		for _, ip := range ips {
			out = append(out, netip.MustParseAddr(ip))
		}
		return out, nil
	})
}

func TestOutboundPolicy_CheckHost(t *testing.T) {
	policy, err := NewOutboundPolicy([]string{"docs-mcp"}, fakeLookup(map[string][]string{
		"public.example":   {"93.184.216.34"},
		"internal.example": {"10.1.2.3"},
		"mixed.example":    {"93.184.216.34", "127.0.0.1"},
		"docs-mcp":         {"172.18.0.5"},
	}))
	if err != nil {
		t.Fatalf("NewOutboundPolicy: %v", err)
	}
	tests := []struct {
		host      string
		refused   bool
		allowable bool
	}{
		{"public.example", false, false},
		{"internal.example", true, true},
		{"mixed.example", true, false},
		{"docs-mcp", false, false},
		{"127.0.0.1", true, false},
		{"::1", true, false},
		{"[::1]", true, false},
		{"169.254.169.254", true, false},
		{"93.184.216.34", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			err := policy.CheckHost(context.Background(), tt.host)
			if refused := errors.Is(err, ErrAddressRefused); refused != tt.refused {
				t.Fatalf("CheckHost(%s) = %v, refused = %v, want %v", tt.host, err, refused, tt.refused)
			}
			var refused *RefusedAddressError
			if tt.refused && (!errors.As(err, &refused) || refused.Allowable() != tt.allowable) {
				t.Fatalf("CheckHost(%s) = %v, want allowable = %v", tt.host, err, tt.allowable)
			}
		})
	}
	// A host that does not resolve is no refusal: the connection fails or is
	// checked again when it dials.
	if err := policy.CheckHost(context.Background(), "nowhere.example"); err == nil || errors.Is(err, ErrAddressRefused) {
		t.Fatalf("CheckHost(unresolvable) = %v, want a lookup error that is no refusal", err)
	}
}

// upstream starts a local server and counts its requests.
func upstream(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// dialTo sends every connection to target, whatever address was checked:
// it stands in for a network in which the checked address is the server.
func dialTo(target string, dialed *[]string) OutboundOption {
	return WithDial(func(ctx context.Context, network, address string) (net.Conn, error) {
		*dialed = append(*dialed, address)
		var d net.Dialer
		return d.DialContext(ctx, network, target)
	})
}

func TestOutboundPolicy_TransportRefusesLoopbackBeforeConnecting(t *testing.T) {
	srv, hits := upstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	policy, err := NewOutboundPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: policy.Transport()}

	resp, err := client.Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("request to 127.0.0.1 succeeded, want refused")
	}
	if !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("request error = %v, want ErrAddressRefused", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the loopback server saw %d requests, want none", hits.Load())
	}
}

func TestOutboundPolicy_TransportRefusesRedirectToPrivateAddress(t *testing.T) {
	internal, internalHits := upstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	public, _ := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/secret", http.StatusFound)
	})
	var dialed []string
	policy, err := NewOutboundPolicy(nil,
		fakeLookup(map[string][]string{"public.example": {"93.184.216.34"}}),
		dialTo(strings.TrimPrefix(public.URL, "http://"), &dialed))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: policy.Transport()}

	resp, err := client.Get("http://public.example/")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("the redirect to 127.0.0.1 was followed")
	}
	if !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("request error = %v, want ErrAddressRefused", err)
	}
	if internalHits.Load() != 0 {
		t.Fatalf("the internal server saw %d requests, want none", internalHits.Load())
	}
	if len(dialed) != 1 || dialed[0] != "93.184.216.34:80" {
		t.Fatalf("dialed %v, want only the checked public address", dialed)
	}
}

func TestOutboundPolicy_TransportChecksTheAddressItDials(t *testing.T) {
	srv, hits := upstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	// DNS rebinding: the name resolved to a public address when it was
	// checked before and resolves to a private one when the client dials.
	var lookups atomic.Int32
	rebinding := WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
		if lookups.Add(1) == 1 {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("10.0.0.7")}, nil
	})
	var dialed []string
	policy, err := NewOutboundPolicy(nil, rebinding, dialTo(strings.TrimPrefix(srv.URL, "http://"), &dialed))
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.CheckHost(context.Background(), "rebind.example"); err != nil {
		t.Fatalf("first check: %v", err)
	}
	client := &http.Client{Transport: policy.Transport()}
	resp, err := client.Get("http://rebind.example:8080/")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("the rebound private address was dialed")
	}
	if !errors.Is(err, ErrAddressRefused) || hits.Load() != 0 || len(dialed) != 0 {
		t.Fatalf("error %v, %d requests, dialed %v; want refused before dialing", err, hits.Load(), dialed)
	}
}

func TestOutboundPolicy_TransportDialsAllowlistedHost(t *testing.T) {
	srv, hits := upstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	var dialed []string
	policy, err := NewOutboundPolicy([]string{"docs-mcp"},
		fakeLookup(map[string][]string{"docs-mcp": {"172.18.0.5"}}),
		dialTo(strings.TrimPrefix(srv.URL, "http://"), &dialed))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: policy.Transport()}
	resp, err := client.Get("http://docs-mcp:6280/mcp")
	if err != nil {
		t.Fatalf("request to an allowlisted host: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || hits.Load() != 1 {
		t.Fatalf("status %d, %d requests; want 204 and one request", resp.StatusCode, hits.Load())
	}
	if len(dialed) != 1 || dialed[0] != "172.18.0.5:6280" {
		t.Fatalf("dialed %v, want the checked address", dialed)
	}
}

func TestOutboundPolicy_TransportIgnoresProxyEnvironment(t *testing.T) {
	policy, err := NewOutboundPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Transport().Proxy != nil {
		t.Fatal("the transport uses a proxy: the proxy, not the policy, would choose the address")
	}
}
