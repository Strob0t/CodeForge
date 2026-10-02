package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/netutil"
)

// SSRF protection for sse and streamable_http MCP servers (KI-100): a
// tenant admin chooses their url, and the Go Core (connection test) and the
// worker (runs) connect to it. Loopback, link-local (cloud metadata),
// unspecified, multicast and reserved addresses are never used; private
// ones only for the hosts in mcp.allowed_private_hosts, which only the
// platform operator sets.

// mcpURLCheckTimeout bounds the DNS lookup of a url check.
const mcpURLCheckTimeout = 5 * time.Second

// mcpMaxRedirects bounds the redirects a connection test follows.
const mcpMaxRedirects = 10

// SetOutboundPolicy replaces the policy built from mcp.allowed_private_hosts
// (tests use it with a fake resolver and dialer).
func (s *MCPService) SetOutboundPolicy(p *netutil.OutboundPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outbound = p
}

func (s *MCPService) outboundPolicy() *netutil.OutboundPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.outbound
}

// checkServerURL refuses (domain.ErrValidation) the url of an sse or
// streamable_http server whose host is, or resolves to, an address the
// policy refuses, before anything connects to it. A host that does not
// resolve passes: nothing can connect to it now, and every connection is
// checked again at the address it dials.
func (s *MCPService) checkServerURL(ctx context.Context, def *mcp.ServerDef) error {
	if def.Transport != mcp.TransportSSE && def.Transport != mcp.TransportStreamableHTTP {
		return nil
	}
	u, err := url.Parse(def.URL)
	if err != nil {
		return fmt.Errorf("%w: url must be an http or https URL with a host", domain.ErrValidation)
	}
	ctx, cancel := context.WithTimeout(ctx, mcpURLCheckTimeout)
	defer cancel()
	err = s.outboundPolicy().CheckHost(ctx, u.Hostname())
	var refused *netutil.RefusedAddressError
	if !errors.As(err, &refused) {
		return nil
	}
	if refused.Allowable() {
		return fmt.Errorf("%w: url: %w; only the platform operator can allow a private host (mcp.allowed_private_hosts)", domain.ErrValidation, err)
	}
	return fmt.Errorf("%w: url: %w; MCP servers may never use it", domain.ErrValidation, err)
}

// mcpHTTPClient connects only to addresses the policy allows, checked at the
// address it dials (DNS rebinding, redirects), never through a proxy, and
// follows a redirect only within the server's origin: the headers it sends
// belong to that server. The worker's MCP SDK follows the same rule.
func (s *MCPService) mcpHTTPClient() *http.Client {
	return &http.Client{
		Transport:     s.outboundPolicy().Transport(),
		CheckRedirect: sameOriginRedirect,
	}
}

func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= mcpMaxRedirects {
		return fmt.Errorf("stopped after %d redirects", mcpMaxRedirects)
	}
	if origin := via[0].URL; req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
		return fmt.Errorf("redirect to another origin (%s://%s) not followed", req.URL.Scheme, req.URL.Host)
	}
	return nil
}
