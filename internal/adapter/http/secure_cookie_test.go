package http

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestIsSecureRequestWithConfig(t *testing.T) {
	tests := []struct {
		name   string
		tls    bool
		header string
		force  bool
		want   bool
	}{
		{"plain HTTP", false, "", false, false},
		{"direct TLS", true, "", false, true},
		{"proxy HTTPS header", false, "https", false, true},
		{"force overrides plain", false, "", true, true},
		{"force with proxy header", false, "https", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/", http.NoBody)
			if tt.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if tt.header != "" {
				r.Header.Set("X-Forwarded-Proto", tt.header)
			}
			got := isSecureRequestWithConfig(r, tt.force)
			if got != tt.want {
				t.Errorf("isSecureRequestWithConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandlers_isSecureCookie(t *testing.T) {
	tests := []struct {
		name  string
		force bool
		tls   bool
		want  bool
	}{
		{"default no TLS", false, false, false},
		{"default with TLS", false, true, true},
		{"forced no TLS", true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handlers{ForceSecureCookies: tt.force}
			r, _ := http.NewRequest("GET", "/", http.NoBody)
			if tt.tls {
				r.TLS = &tls.ConnectionState{}
			}
			got := h.isSecureCookie(r)
			if got != tt.want {
				t.Errorf("isSecureCookie() = %v, want %v", got, tt.want)
			}
		})
	}
}
