package gitlab

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

// KI-85 review (security finding 2): the GitLab base URL is the host of a
// project's repo_url, which tenant editors choose, and a manual sync takes
// it from the request. The provider connects only to addresses the outbound
// policy allows (pm.allowed_private_hosts), follows redirects only within
// the origin, and its errors never carry a response body, the token or the
// secrets of a URL.

const testToken = "glpat-SECRET-token-1234"

// fakeNet resolves names to fixed addresses and connects every address the
// policy checked to the local server that stands for it.
type fakeNet struct {
	names   map[string][]string
	servers map[string]*httptest.Server // by address
	dials   atomic.Int32
}

func (n *fakeNet) policy(t *testing.T, allowed ...string) *netutil.OutboundPolicy {
	t.Helper()
	policy, err := netutil.NewOutboundPolicy(allowed,
		netutil.WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
			var addrs []netip.Addr
			for _, ip := range n.names[host] {
				addrs = append(addrs, netip.MustParseAddr(ip))
			}
			if len(addrs) == 0 {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			return addrs, nil
		}),
		netutil.WithDial(func(ctx context.Context, network, address string) (net.Conn, error) {
			n.dials.Add(1)
			host, _, _ := net.SplitHostPort(address)
			srv, ok := n.servers[host]
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

// newLoopbackProvider is a provider whose policy allows 127.0.0.1, where
// httptest servers listen.
func newLoopbackProvider(t *testing.T, baseURL, token string) *Provider {
	t.Helper()
	policy, err := netutil.NewOutboundPolicy([]string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	return newProvider(baseURL, token, newHTTPClient(policy))
}

func TestProvider_RefusesPrivateAndLoopbackHostsBeforeConnecting(t *testing.T) {
	names := map[string][]string{
		"gitlab.internal": {"10.0.0.5"},
		"localhost":       {"127.0.0.1"},
		"mixed.example":   {"203.0.113.10", "10.0.0.5"},
		"rebind.example":  {"::ffff:192.168.1.1"},
	}
	tests := []struct {
		name      string
		baseURL   string
		allowable bool // the operator could allow it (pm.allowed_private_hosts)
	}{
		{name: "private name", baseURL: "https://gitlab.internal", allowable: true},
		{name: "private literal", baseURL: "http://10.0.0.5:8080", allowable: true},
		{name: "IPv4-mapped private", baseURL: "http://rebind.example", allowable: true},
		{name: "loopback literal", baseURL: "http://127.0.0.1:8080", allowable: true},
		{name: "localhost", baseURL: "http://localhost:8080", allowable: true},
		{name: "IPv6 loopback", baseURL: "http://[::1]:8080", allowable: true},
		{name: "a public and a private address", baseURL: "https://mixed.example", allowable: true},
		{name: "cloud metadata", baseURL: "http://169.254.169.254"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeNet{names: names}
			p := newProvider(tt.baseURL, testToken, newHTTPClient(fake.policy(t)))

			_, err := p.ListItems(context.Background(), "group/app")

			if !errors.Is(err, netutil.ErrAddressRefused) {
				t.Fatalf("ListItems(%s) = %v, want ErrAddressRefused", tt.baseURL, err)
			}
			if got := fake.dials.Load(); got != 0 {
				t.Fatalf("%d connections were dialled, want none", got)
			}
			if hint := strings.Contains(err.Error(), "pm.allowed_private_hosts"); hint != tt.allowable {
				t.Fatalf("error %q names pm.allowed_private_hosts: %v, want %v", err, hint, tt.allowable)
			}
			if strings.Contains(err.Error(), testToken) {
				t.Fatalf("error %q carries the token", err)
			}
		})
	}
}

// TestNewProvider_RefusesLoopbackWithoutAnAllowlist: the providers a sync
// builds through the registry use the package policy, which allows no
// private or loopback address until SetOutboundPolicy opens one.
func TestNewProvider_RefusesLoopbackWithoutAnAllowlist(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	p, err := pmprovider.New(providerName, map[string]string{"base_url": srv.URL, "token": testToken})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.ListItems(context.Background(), "group/app"); !errors.Is(err, netutil.ErrAddressRefused) {
		t.Fatalf("ListItems on loopback = %v, want ErrAddressRefused", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the loopback server was reached")
	}
}

// TestSetOutboundPolicy_AppliesToRegistryProviders: the policy the Go Core
// builds from pm.allowed_private_hosts reaches the providers the registry
// builds.
func TestSetOutboundPolicy_AppliesToRegistryProviders(t *testing.T) {
	previous := client.Load()
	t.Cleanup(func() { client.Store(previous) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != testToken {
			t.Errorf("PRIVATE-TOKEN = %q", r.Header.Get("PRIVATE-TOKEN"))
		}
		_, _ = w.Write([]byte(`[{"iid":7,"title":"t","state":"opened"}]`))
	}))
	defer srv.Close()
	policy, err := netutil.NewOutboundPolicy([]string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	SetOutboundPolicy(policy)

	p, err := pmprovider.New(providerName, map[string]string{"base_url": srv.URL, "token": testToken})
	if err != nil {
		t.Fatal(err)
	}
	items, err := p.ListItems(context.Background(), "group/app")
	if err != nil || len(items) != 1 || items[0].ID != "7" {
		t.Fatalf("ListItems = %+v, %v; want issue 7", items, err)
	}
}

func TestProvider_AllowlistedPrivateHostIsReached(t *testing.T) {
	for name, allowed := range map[string][]string{
		"host name":   {"gitlab.internal"},
		"address":     {"10.20.0.5"},
		"CIDR prefix": {"10.20.0.0/16"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("PRIVATE-TOKEN") != testToken {
					t.Errorf("PRIVATE-TOKEN = %q", r.Header.Get("PRIVATE-TOKEN"))
				}
				_, _ = w.Write([]byte(`[{"iid":3,"title":"internal","state":"opened"}]`))
			}))
			defer srv.Close()
			fake := &fakeNet{
				names:   map[string][]string{"gitlab.internal": {"10.20.0.5"}, "other.internal": {"10.30.0.5"}},
				servers: map[string]*httptest.Server{"10.20.0.5": srv, "10.30.0.5": srv},
			}
			policy := fake.policy(t, allowed...)

			items, err := newProvider("http://gitlab.internal", testToken, newHTTPClient(policy)).ListItems(context.Background(), "group/app")
			if err != nil || len(items) != 1 || items[0].Title != "internal" {
				t.Fatalf("ListItems on the allowlisted host = %+v, %v", items, err)
			}
			// Another private host stays refused.
			_, err = newProvider("http://other.internal", testToken, newHTTPClient(policy)).ListItems(context.Background(), "group/app")
			if !errors.Is(err, netutil.ErrAddressRefused) {
				t.Fatalf("ListItems on a host that is not allowlisted = %v, want ErrAddressRefused", err)
			}
		})
	}
}

func TestProvider_FollowsRedirectsOnlyWithinTheOrigin(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		if r.Header.Get("PRIVATE-TOKEN") != "" {
			t.Error("the token reached another host")
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer other.Close()
	gl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/api/v4/projects/group%2Fmoved/issues":
			http.Redirect(w, r, "/api/v4/projects/group%2Fapp/issues?per_page=50&state=opened", http.StatusMovedPermanently)
		case "/api/v4/projects/group%2Fapp/issues":
			if r.Header.Get("PRIVATE-TOKEN") != testToken {
				t.Errorf("PRIVATE-TOKEN after a same-origin redirect = %q", r.Header.Get("PRIVATE-TOKEN"))
			}
			_, _ = w.Write([]byte(`[{"iid":1,"title":"moved","state":"opened"}]`))
		case "/api/v4/projects/group%2Fleaves/issues":
			http.Redirect(w, r, "http://other.example/api/v4/projects/group%2Fapp/issues", http.StatusFound)
		case "/api/v4/projects/group%2Fport/issues":
			http.Redirect(w, r, "http://gitlab.example:8080/api/v4/projects/group%2Fapp/issues", http.StatusTemporaryRedirect)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gl.Close()
	fake := &fakeNet{
		names:   map[string][]string{"gitlab.example": {"203.0.113.10"}, "other.example": {"203.0.113.20"}},
		servers: map[string]*httptest.Server{"203.0.113.10": gl, "203.0.113.20": other},
	}
	p := newProvider("http://gitlab.example", testToken, newHTTPClient(fake.policy(t)))

	items, err := p.ListItems(context.Background(), "group/moved")
	if err != nil || len(items) != 1 || items[0].Title != "moved" {
		t.Fatalf("same-origin redirect: %+v, %v; want it followed", items, err)
	}
	for _, ref := range []string{"group/leaves", "group/port"} {
		_, err := p.ListItems(context.Background(), ref)
		if err == nil || !strings.Contains(err.Error(), "redirect") {
			t.Fatalf("%s: redirect to another origin = %v, want an error naming the refused redirect", ref, err)
		}
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host got %d requests, want none", otherHits.Load())
	}
}

func TestProvider_ErrorsCarryNoResponseBodyTokenOrURLSecret(t *testing.T) {
	const (
		bodyMarker = "INTERNAL-BODY-7f3a"
		urlUser    = "url-user-x"
		urlPass    = "url-pass-x"
		urlQuery   = "url-query-secret-x"
	)
	// The server answers with the body and the token, as an error page that
	// echoes the request might; with raw set, it sends raw instead of an
	// HTTP answer of its own.
	answer := func(status int, body, raw string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if raw == "" {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body + " token=" + testToken))
				return
			}
			conn, buf, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = buf.WriteString(raw)
			_ = buf.Flush()
		}))
	}
	tests := []struct {
		name    string
		status  int    // of the server at 203.0.113.10; 0: no server
		body    string // its answer, followed by the token
		raw     string // or what it sends instead
		baseURL string
	}{
		{name: "401", status: http.StatusUnauthorized, body: `{"message":"` + bodyMarker + `"}`, baseURL: "http://gitlab.example"},
		{name: "404", status: http.StatusNotFound, body: bodyMarker, baseURL: "http://gitlab.example"},
		{name: "500", status: http.StatusInternalServerError, body: "<html>" + bodyMarker + "</html>", baseURL: "http://gitlab.example"},
		{name: "not JSON", status: http.StatusOK, body: "<html>" + bodyMarker + "</html>", baseURL: "http://gitlab.example"},
		{name: "secrets in the base URL, refused address", baseURL: "http://" + urlUser + ":" + urlPass + "@10.0.0.5/?private_token=" + urlQuery},
		{name: "secrets in the base URL, connection fails", baseURL: "http://" + urlUser + ":" + urlPass + "@unreachable.example/?private_token=" + urlQuery},
		{name: "secrets in the base URL, answer 500", status: http.StatusInternalServerError, body: bodyMarker,
			baseURL: "http://" + urlUser + ":" + urlPass + "@gitlab.example/?private_token=" + urlQuery},
		{name: "secrets in an invalid base URL", baseURL: "http://" + urlUser + ":" + urlPass + "@gitlab.example/\x7f?private_token=" + urlQuery},
		{name: "reason phrase", status: http.StatusInternalServerError, raw: "HTTP/1.1 500 " + bodyMarker + "\r\nContent-Length: 0\r\n\r\n",
			baseURL: "http://gitlab.example"},
		{name: "not HTTP", status: http.StatusOK, raw: "-ERR " + bodyMarker + "\r\n", baseURL: "http://gitlab.example"},
		{name: "unparsable Location", status: http.StatusFound, raw: "HTTP/1.1 302 Found\r\nLocation: http://[" + bodyMarker + "\r\nContent-Length: 0\r\n\r\n",
			baseURL: "http://gitlab.example"},
	}
	ops := map[string]func(p *Provider) error{
		"ListItems": func(p *Provider) error { _, err := p.ListItems(context.Background(), "group/app"); return err },
		"GetItem":   func(p *Provider) error { _, err := p.GetItem(context.Background(), "group/app", "1"); return err },
		"CreateItem": func(p *Provider) error {
			_, err := p.CreateItem(context.Background(), "group/app", &pmprovider.Item{Title: "t"})
			return err
		},
		"UpdateItem": func(p *Provider) error {
			_, err := p.UpdateItem(context.Background(), "group/app", &pmprovider.Item{ID: "1", Title: "t"})
			return err
		},
	}
	for _, tt := range tests {
		fake := &fakeNet{names: map[string][]string{"gitlab.example": {"203.0.113.10"}, "unreachable.example": {"203.0.113.99"}}}
		if tt.status != 0 {
			srv := answer(tt.status, tt.body, tt.raw)
			t.Cleanup(srv.Close)
			fake.servers = map[string]*httptest.Server{"203.0.113.10": srv}
		}
		p := newProvider(tt.baseURL, testToken, newHTTPClient(fake.policy(t)))
		for opName, op := range ops {
			t.Run(tt.name+"/"+opName, func(t *testing.T) {
				err := op(p)
				if err == nil {
					t.Fatal("want an error")
				}
				// "'<'": a JSON syntax error quotes the character it stopped at.
				for _, secret := range []string{bodyMarker, "<html>", "'<'", testToken, urlUser, urlPass, urlQuery} {
					if strings.Contains(err.Error(), secret) {
						t.Fatalf("error %q carries %q", err, secret)
					}
				}
			})
		}
	}
}

func TestProvider_RefusesOversizedResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("["))
		chunk := []byte(strings.Repeat(" ", 64<<10))
		for written := 0; written <= maxResponseBytes; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
		_, _ = w.Write([]byte("]"))
	}))
	defer srv.Close()

	_, err := newLoopbackProvider(t, srv.URL, testToken).ListItems(context.Background(), "group/app")
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("ListItems on an oversized answer = %v, want a size error", err)
	}
}
