package netutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// IsPrivateIP checks if an IP is in a private/reserved range.
// Covers IPv4 (RFC 1918, loopback, link-local), IPv6 (ULA fc00::/7,
// link-local fe80::/10), and IPv4-mapped IPv6 addresses (::ffff:x.x.x.x).
func IsPrivateIP(ip net.IP) bool {
	// Check IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) by converting
	// to their IPv4 equivalent so the IPv4 ranges below match them.
	if mapped := ip.To4(); mapped != nil {
		ip = mapped
	}

	privateRanges := []net.IPNet{
		// IPv4
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},
		{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
		{IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)},
		{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(32, 32)},
		// IPv6 — Unique Local Addresses (ULA, RFC 4193)
		{IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},
		// IPv6 — Link-Local (fe80::/10, RFC 4291)
		{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)},
	}
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// SafeTransport returns an http.Transport that blocks connections to private IPs.
func SafeTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS lookup failed: %w", err)
			}
			for _, ip := range ips {
				if IsPrivateIP(ip.IP) {
					return nil, fmt.Errorf("address %q resolves to private IP %s", host, ip.IP)
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
}
