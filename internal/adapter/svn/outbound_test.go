package svn

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/netutil"
)

// S10-D review: SVN projects take http:// and svn:// URLs to any host, and
// the Go Core runs svn against them: a blind SSRF into the operator's
// network. svn contacts a host only under netutil.OutboundPolicy: never
// link-local (cloud metadata) or reserved addresses, private and loopback
// ones only through svn.allowed_private_hosts.

// withLookup gives the provider a policy built from allowed whose DNS
// answers from hosts.
func withLookup(t *testing.T, p *Provider, allowed []string, hosts map[string]string) {
	t.Helper()
	policy, err := netutil.NewOutboundPolicy(allowed, netutil.WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
		if addr, ok := hosts[host]; ok {
			return []netip.Addr{netip.MustParseAddr(addr)}, nil
		}
		return nil, errors.New("no such host")
	}))
	if err != nil {
		t.Fatal(err)
	}
	p.outbound = policy
}

func TestSVN_ContactsHostsOnlyUnderTheOutboundPolicy(t *testing.T) {
	hosts := map[string]string{"svn.internal": "192.168.1.10", "svn.example.com": "93.184.215.14"}
	tests := []struct {
		name    string
		url     string
		allowed []string
		refused bool
	}{
		{"cloud metadata", "http://169.254.169.254/latest/meta-data", nil, true},
		{"cloud metadata, even allowlisted", "http://169.254.169.254/repo", []string{"169.254.0.0/16"}, true},
		{"private address", "svn://10.0.0.5/repo", nil, true},
		{"loopback", "http://127.0.0.1:3690/repo", nil, true},
		{"name of a private address", "https://svn.internal/repo", nil, true},
		{"private address over svn+ssh", "svn+ssh://10.0.0.5/repo", nil, true},
		{"allowlisted private address", "svn://10.0.0.5/repo", []string{"10.0.0.5"}, false},
		{"allowlisted private name", "https://svn.internal/repo", []string{"svn.internal"}, false},
		{"public host", "https://svn.example.com/repo", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, fake := newFakeProvider(tt.url)
			withLookup(t, p, tt.allowed, hosts)
			err := p.Clone(context.Background(), tt.url, filepath.Join(t.TempDir(), "wc"))
			if tt.refused {
				if !errors.Is(err, netutil.ErrAddressRefused) {
					t.Fatalf("Clone(%s) = %v, want ErrAddressRefused", tt.url, err)
				}
				if subs := fake.subcommands(); len(subs) != 0 {
					t.Fatalf("svn ran for a refused host: %v", subs)
				}
				return
			}
			if err != nil {
				t.Fatalf("Clone(%s): %v", tt.url, err)
			}
		})
	}
}

// The operator setting reaches providers the registry creates; a bad entry
// is an error.
func TestSetAllowedPrivateHosts(t *testing.T) {
	t.Cleanup(func() { _ = SetAllowedPrivateHosts(nil) })
	if err := SetAllowedPrivateHosts([]string{"not a host!"}); err == nil {
		t.Fatal("a bad allowlist entry was accepted")
	}
	if err := SetAllowedPrivateHosts([]string{"10.0.0.5"}); err != nil {
		t.Fatal(err)
	}
	p := newRegistered(map[string]string{})
	if err := p.contactable(context.Background(), "svn://10.0.0.5/repo"); err != nil {
		t.Fatalf("allowlisted host refused: %v", err)
	}
	if err := p.contactable(context.Background(), "svn://10.0.0.6/repo"); !errors.Is(err, netutil.ErrAddressRefused) {
		t.Fatalf("contactable(10.0.0.6) = %v, want ErrAddressRefused", err)
	}
}
