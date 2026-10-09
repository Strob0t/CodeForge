package netutil

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
)

// TestSameOriginRedirect (KI-100 review, KI-85 review): the MCP connection
// test and the GitLab PM provider follow a redirect exactly when the
// worker's MCP SDK does (mcp 1.30.0,
// mcp.shared._httpx_utils.next_request_within_origin with httpx 0.28.1): the
// method stays, the Location brings no userinfo of its own, the url stays on
// the origin of the request just sent (same scheme, host and port, a default
// port written out or not, or an http to https upgrade on the same host with
// default ports), at most 20 times.
func TestSameOriginRedirect(t *testing.T) {
	tests := []struct {
		name   string
		sent   string
		method string // of the sent request; the next one is GET unless set below
		next   string
		nextM  string
		hops   int
		follow bool
	}{
		{name: "path on the same origin", sent: "http://mcp.example/sse", next: "http://mcp.example/sse/", follow: true},
		{name: "host case", sent: "http://MCP.example/a", next: "http://mcp.example/b", follow: true},
		{name: "explicit default port", sent: "http://mcp.example:80/a", next: "http://mcp.example/b", follow: true},
		{name: "explicit default https port", sent: "https://mcp.example/a", next: "https://mcp.example:443/b", follow: true},
		{name: "upgrade to https", sent: "http://mcp.example/a", next: "https://mcp.example/a", follow: true},
		{name: "upgrade with explicit default ports", sent: "http://mcp.example:80/a", next: "https://mcp.example:443/a", follow: true},
		{name: "upgrade to another port", sent: "http://mcp.example/a", next: "https://mcp.example:8443/a"},
		{name: "upgrade from another port", sent: "http://mcp.example:8080/a", next: "https://mcp.example/a"},
		{name: "downgrade to http", sent: "https://mcp.example/a", next: "http://mcp.example/a"},
		{name: "another port", sent: "http://mcp.example:8080/a", next: "http://mcp.example:8081/a"},
		{name: "another host", sent: "http://mcp.example/a", next: "http://other.example/a"},
		{name: "subdomain", sent: "http://mcp.example/a", next: "http://a.mcp.example/a"},
		{name: "POST kept (307)", sent: "http://mcp.example/a", method: http.MethodPost, next: "http://mcp.example/b", nextM: http.MethodPost, follow: true},
		{name: "POST turned into GET (303)", sent: "http://mcp.example/a", method: http.MethodPost, next: "http://mcp.example/b"},
		{name: "userinfo of its own", sent: "http://mcp.example/a", next: "http://u:p@mcp.example/b"},
		{name: "userinfo kept from the sent url", sent: "http://u:p@mcp.example/a", next: "http://u:p@mcp.example/b", follow: true},
		{name: "20th redirect", sent: "http://mcp.example/a", next: "http://mcp.example/b", hops: 20, follow: true},
		{name: "21st redirect", sent: "http://mcp.example/a", next: "http://mcp.example/b", hops: 21},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			nextMethod := tt.nextM
			if nextMethod == "" {
				nextMethod = http.MethodGet
			}
			hops := tt.hops
			if hops == 0 {
				hops = 1
			}
			via := make([]*http.Request, hops)
			for i := range via {
				via[i] = &http.Request{Method: http.MethodGet, URL: mustParseURL(t, "http://mcp.example/first")}
			}
			via[hops-1] = &http.Request{Method: method, URL: mustParseURL(t, tt.sent)}
			next := &http.Request{Method: nextMethod, URL: mustParseURL(t, tt.next)}

			err := SameOriginRedirect(next, via)

			if tt.follow && err != nil {
				t.Fatalf("redirect %s -> %s refused: %v", tt.sent, tt.next, err)
			}
			if !tt.follow && !errors.Is(err, http.ErrUseLastResponse) {
				t.Fatalf("redirect %s -> %s = %v, want http.ErrUseLastResponse (not followed)", tt.sent, tt.next, err)
			}
		})
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
