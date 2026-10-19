package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP returns middleware that replaces r.RemoteAddr with the client IP.
//
// Forwarding headers are honoured only when the direct peer is one of the trusted
// proxies: X-Forwarded-For is walked from the right, skipping trusted hops, and the
// first untrusted address wins; X-Real-IP is the fallback. Requests from any other
// peer keep the peer address, so clients cannot pick their own identity (and rate
// limit bucket) by sending these headers - unlike chi's deprecated RealIP.
func ClientIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if addr := resolveClientAddr(r, trusted); addr.IsValid() {
				r.RemoteAddr = addr.String()
			}
			next.ServeHTTP(w, r)
		})
	}
}

func resolveClientAddr(r *http.Request, trusted []netip.Prefix) netip.Addr {
	peer := parseHostAddr(r.RemoteAddr)
	if !peer.IsValid() || !isTrustedProxy(peer, trusted) {
		return peer
	}

	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		hop = hop.Unmap()
		if !isTrustedProxy(hop, trusted) {
			return hop
		}
	}

	if realIP, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); err == nil {
		return realIP.Unmap()
	}
	return peer
}

// parseHostAddr parses a RemoteAddr of the form "host:port" or a bare host.
func parseHostAddr(remoteAddr string) netip.Addr {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func isTrustedProxy(addr netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
