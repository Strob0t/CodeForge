package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

// mcpMaxRedirects bounds the redirects a connection test follows (httpx
// DEFAULT_MAX_REDIRECTS, which the worker's MCP SDK uses).
const mcpMaxRedirects = 20

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

// sameOriginRedirect follows a redirect exactly when the worker's MCP SDK
// does (mcp 1.30.0, next_request_within_origin and _within_origin in
// mcp/shared/_httpx_utils.py, on httpx 0.28.1): the method stays (a POST
// turned into a GET by 301/302/303 is not followed), the Location brings no
// userinfo of its own, the url stays on the origin of the request just sent,
// and at most mcpMaxRedirects times. A redirect that is not followed is
// returned as the response (http.ErrUseLastResponse), as the SDK hands it
// back, and fails the connection.
func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	sent := via[len(via)-1]
	if len(via) > mcpMaxRedirects || req.Method != sent.Method ||
		(req.URL.User != nil && req.URL.User.String() != sent.URL.User.String()) || !withinOrigin(sent.URL, req.URL) {
		return http.ErrUseLastResponse
	}
	return nil
}

// withinOrigin reports whether next is on the origin of sent: the same
// scheme, host and port (a default port written out or not, as httpx
// normalises it), or the https upgrade of sent on the same host with
// default ports.
func withinOrigin(sent, next *url.URL) bool {
	sentHost, nextHost := strings.ToLower(sent.Hostname()), strings.ToLower(next.Hostname())
	if sentHost != nextHost {
		return false
	}
	sentPort, nextPort := nonDefaultPort(sent), nonDefaultPort(next)
	if sent.Scheme == next.Scheme && sentPort == nextPort {
		return true
	}
	return sent.Scheme == "http" && sentPort == "" && next.Scheme == "https" && nextPort == ""
}

// nonDefaultPort returns the port of u, or "" for none or the default port
// of its scheme.
func nonDefaultPort(u *url.URL) string {
	switch port := u.Port(); {
	case u.Scheme == "http" && port == "80", u.Scheme == "https" && port == "443":
		return ""
	default:
		return port
	}
}
