package netutil

import (
	"net"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		private bool
	}{
		// IPv4
		{"IPv4 private 10.x", "10.0.0.1", true},
		{"IPv4 private 172.16.x", "172.16.0.1", true},
		{"IPv4 private 192.168.x", "192.168.1.1", true},
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv4 link-local", "169.254.1.1", true},
		{"IPv4 public", "8.8.8.8", false},
		{"IPv4 unspecified", "0.0.0.0", true},
		// IPv6
		{"IPv6 loopback", "::1", true},
		{"IPv6 link-local", "fe80::1", true},
		{"IPv6 ULA fc00", "fc00::1", true},
		{"IPv6 ULA fd", "fd12:3456::1", true},
		{"IPv6 mapped loopback", "::ffff:127.0.0.1", true},
		{"IPv6 mapped private", "::ffff:10.0.0.1", true},
		{"IPv6 mapped public", "::ffff:8.8.8.8", false},
		{"IPv6 public", "2001:4860:4860::8888", false},
		{"IPv6 unspecified", "::", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			got := IsPrivateIP(ip)
			if got != tt.private {
				t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}
