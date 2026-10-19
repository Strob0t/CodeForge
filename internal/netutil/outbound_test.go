package netutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// KI-100: an outbound connection to a URL a tenant supplied (MCP servers)
// never reaches loopback, link-local (cloud metadata), unspecified,
// multicast or reserved addresses, and reaches private ones only for hosts
// the platform operator allowlisted.

// outboundCases is internal/netutil/testdata/outbound_cases.json, which
// workers/tests/test_mcp_outbound.py reads too: the Go Core and the worker
// decide alike.
type outboundCases struct {
	Policies []struct {
		Name                string   `json:"name"`
		AllowedPrivateHosts []string `json:"allowed_private_hosts"`
		Trusted             bool     `json:"trusted"`
		Cases               []struct {
			Name      string `json:"name"`
			Host      string `json:"host"`
			IP        string `json:"ip"`
			Allowed   bool   `json:"allowed"`
			Allowable bool   `json:"allowable"`
		} `json:"cases"`
	} `json:"policies"`
	InvalidEntries []string `json:"invalid_entries"`
	ValidEntries   []string `json:"valid_entries"`
}

func loadOutboundCases(t *testing.T) outboundCases {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "outbound_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases outboundCases
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestOutboundPolicy_CheckAddr(t *testing.T) {
	for _, pc := range loadOutboundCases(t).Policies {
		if pc.Trusted {
			// Operator (YAML) servers: only the worker connects to them and
			// checks them (workers/tests/test_mcp_outbound.py runs these).
			continue
		}
		policy, err := NewOutboundPolicy(pc.AllowedPrivateHosts)
		if err != nil {
			t.Fatalf("%s: NewOutboundPolicy: %v", pc.Name, err)
		}
		for _, tt := range pc.Cases {
			t.Run(pc.Name+"/"+tt.Name, func(t *testing.T) {
				err := policy.CheckAddr(tt.Host, netip.MustParseAddr(tt.IP))
				if tt.Allowed {
					if err != nil {
						t.Fatalf("%s -> %s = %v, want allowed", tt.Host, tt.IP, err)
					}
					return
				}
				if !errors.Is(err, ErrAddressRefused) {
					t.Fatalf("%s -> %s = %v, want ErrAddressRefused", tt.Host, tt.IP, err)
				}
				var refused *RefusedAddressError
				if !errors.As(err, &refused) || refused.Allowable() != tt.Allowable {
					t.Fatalf("%s -> %s = %v, allowable = %v, want %v", tt.Host, tt.IP, err, refused != nil && refused.Allowable(), tt.Allowable)
				}
			})
		}
	}
}

func TestOutboundPolicy_InvalidAddressIsRefused(t *testing.T) {
	policy, err := NewOutboundPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.CheckAddr("x", netip.Addr{}); !errors.Is(err, ErrAddressRefused) {
		t.Fatalf("invalid address = %v, want refused", err)
	}
}

func TestNewOutboundPolicy_Entries(t *testing.T) {
	cases := loadOutboundCases(t)
	for _, entry := range cases.InvalidEntries {
		t.Run(fmt.Sprintf("invalid %q", entry), func(t *testing.T) {
			if _, err := NewOutboundPolicy([]string{entry}); err == nil {
				t.Fatalf("NewOutboundPolicy(%q) = nil error, want an error", entry)
			}
		})
	}
	if _, err := NewOutboundPolicy(cases.ValidEntries); err != nil {
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
		{"mixed.example", true, true}, // loopback: only an explicit entry opens it
		{"docs-mcp", false, false},
		{"127.0.0.1", true, true},
		{"::1", true, true},
		{"[::1]", true, true},
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

// TestOutboundPolicy_TransportReachesAllowlistedLoopback (KI-100 review): the
// dev topology (docs-mcp published on 127.0.0.1:6280) works once the
// operator opens loopback explicitly; 0.0.0.0/0 does not open it.
func TestOutboundPolicy_TransportReachesAllowlistedLoopback(t *testing.T) {
	srv, hits := upstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, entry := range []string{"127.0.0.1", "127.0.0.0/8"} {
		policy, err := NewOutboundPolicy([]string{entry})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := (&http.Client{Transport: policy.Transport()}).Get(srv.URL)
		if err != nil {
			t.Fatalf("allowlist %q: %v", entry, err)
		}
		_ = resp.Body.Close()
	}
	broad, err := NewOutboundPolicy([]string{"0.0.0.0/0"})
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := (&http.Client{Transport: broad.Transport()}).Get(srv.URL); err == nil {
		_ = resp.Body.Close()
		t.Fatal("0.0.0.0/0 opened loopback")
	}
	if hits.Load() != 2 {
		t.Fatalf("the loopback server saw %d requests, want 2", hits.Load())
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
