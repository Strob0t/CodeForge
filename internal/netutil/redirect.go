package netutil

import (
	"net/http"
	"net/url"
	"strings"
)

// maxRedirects bounds the redirects SameOriginRedirect follows (httpx
// DEFAULT_MAX_REDIRECTS, which the worker's MCP SDK uses).
const maxRedirects = 20

// SameOriginRedirect is the CheckRedirect of clients whose requests carry
// credentials for one server (KI-100: MCP connection tests; KI-85: the
// GitLab PM provider's PRIVATE-TOKEN, which Go forwards to any host). It
// follows a redirect exactly when the worker's MCP SDK does (mcp 1.30.0,
// next_request_within_origin and _within_origin in
// mcp/shared/_httpx_utils.py, on httpx 0.28.1): the method stays (a POST or
// PUT turned into a GET by 301/302/303 is not followed), the Location brings
// no userinfo of its own, the url stays on the origin of the request just
// sent, and at most maxRedirects times. A redirect that is not followed is
// returned as the response (http.ErrUseLastResponse).
func SameOriginRedirect(req *http.Request, via []*http.Request) error {
	sent := via[len(via)-1]
	if len(via) > maxRedirects || req.Method != sent.Method ||
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
