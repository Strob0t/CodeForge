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

	validUUID := "11111111-2222-3333-4444-555555555555"
	req := httptest.NewRequest("GET", "/", http.NoBody)
	req.Header.Set("X-Tenant-ID", validUUID)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got != validUUID {
		t.Fatalf("expected %s, got %s", validUUID, got)
	}
}

func TestTenantIDInvalidUUID_Returns400(t *testing.T) {
	handler := middleware.TenantID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", http.NoBody)
	req.Header.Set("X-Tenant-ID", "not-a-uuid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid UUID, got %d", rec.Code)
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
