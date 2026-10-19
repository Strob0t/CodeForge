package svn

import (
	"fmt"
	"sync/atomic"

	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// allowFileURLs is the operator setting svn.allow_file_urls.
var allowFileURLs atomic.Bool

// SetAllowFileURLs sets whether working copies of local (file://)
// repositories may be used (svn.allow_file_urls, KI-189).
func SetAllowFileURLs(allow bool) { allowFileURLs.Store(allow) }

// outbound is the policy built from svn.allowed_private_hosts (nil: none).
var outbound atomic.Pointer[netutil.OutboundPolicy]

// SetAllowedPrivateHosts sets the private hosts (names, addresses, CIDR
// prefixes) svn may contact (svn.allowed_private_hosts); link-local and
// metadata addresses stay refused.
func SetAllowedPrivateHosts(hosts []string) error {
	policy, err := netutil.NewOutboundPolicy(hosts)
	if err != nil {
		return fmt.Errorf("svn.allowed_private_hosts: %w", err)
	}
	outbound.Store(policy)
	return nil
}

func init() {
	gitprovider.Register(providerName, func(cfg map[string]string) (gitprovider.Provider, error) {
		return newRegistered(cfg), nil
	})
}

// newRegistered creates the provider the registry hands out.
func newRegistered(cfg map[string]string) *Provider {
	p := NewProvider(nil)
	if u, ok := cfg["username"]; ok {
		p.username = u
	}
	if pw, ok := cfg["password"]; ok {
		p.password = pw
	}
	// Operators hosting local repositories allow file:// URLs
	// (svn.allow_file_urls); the project config cannot (KI-189).
	p.allowFileURLs = allowFileURLs.Load()
	if policy := outbound.Load(); policy != nil {
		p.outbound = policy
	}
	// The project's repository URL (set by the project service, never by
	// the project config): svn contacts nothing outside it.
	p.repoURL = cfg["repo_url"]
	return p
}
