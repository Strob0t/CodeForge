package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("::1/128")}

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{name: "untrusted peer keeps its address", remoteAddr: "203.0.113.7:4711", want: "203.0.113.7"},
		{
			name:       "untrusted peer cannot spoof X-Forwarded-For",
			remoteAddr: "203.0.113.7:4711",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.1"},
			want:       "203.0.113.7",
		},
		{
			name:       "untrusted peer cannot spoof X-Real-IP or True-Client-IP",
			remoteAddr: "203.0.113.7:4711",
			headers:    map[string]string{"X-Real-IP": "198.51.100.1", "True-Client-IP": "198.51.100.2"},
			want:       "203.0.113.7",
		},
		{
			name:       "trusted proxy: rightmost untrusted X-Forwarded-For hop wins",
			remoteAddr: "10.0.0.2:4711",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.9, 192.0.2.4, 10.0.0.5"},
			want:       "192.0.2.4",
		},
		{
			name:       "trusted proxy: X-Real-IP fallback",
			remoteAddr: "10.0.0.2:4711",
			headers:    map[string]string{"X-Real-IP": "192.0.2.4"},
			want:       "192.0.2.4",
		},
		{
			name:       "trusted proxy: invalid header falls back to the peer",
			remoteAddr: "10.0.0.2:4711",
			headers:    map[string]string{"X-Forwarded-For": "not-an-ip"},
			want:       "10.0.0.2",
		},
		{name: "IPv4-mapped IPv6 peer is unmapped", remoteAddr: "[::ffff:203.0.113.7]:4711", want: "203.0.113.7"},
		{
			name:       "trusted IPv6 loopback proxy",
			remoteAddr: "[::1]:4711",
			headers:    map[string]string{"X-Forwarded-For": "2001:db8::1"},
			want:       "2001:db8::1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			h := ClientIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr
			}))
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			if got != tt.want {
				t.Errorf("RemoteAddr = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientIPWithoutTrustedProxiesIgnoresHeaders(t *testing.T) {
	var got string
	h := ClientIP(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.RemoteAddr }))
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.RemoteAddr = "127.0.0.1:4711"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "127.0.0.1" {
		t.Errorf("RemoteAddr = %q, want 127.0.0.1", got)
	}
}
