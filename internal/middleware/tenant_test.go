package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/middleware"
)

func TestTenantIDFromHeader(t *testing.T) {
	var got string
	handler := middleware.TenantID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = middleware.TenantIDFromContext(r.Context())
	}))

	req := httptest.NewRequest("GET", "/", http.NoBody)
	req.Header.Set("X-Tenant-ID", "tenant-abc")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got != "tenant-abc" {
		t.Fatalf("expected tenant-abc, got %s", got)
	}
}

func TestTenantIDDefaultFallback(t *testing.T) {
	var got string
	handler := middleware.TenantID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = middleware.TenantIDFromContext(r.Context())
	}))

	req := httptest.NewRequest("GET", "/", http.NoBody)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got != middleware.DefaultTenantID {
		t.Fatalf("expected default tenant, got %s", got)
	}
}

func TestTenantIDFromContextMissing(t *testing.T) {
	req := httptest.NewRequest("GET", "/", http.NoBody)
	got := middleware.TenantIDFromContext(req.Context())
	if got != middleware.DefaultTenantID {
		t.Fatalf("expected default tenant, got %s", got)
	}
}
