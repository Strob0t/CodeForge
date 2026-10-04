package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
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

// KI-85: the webhook routes are public; a webhook's tenant is the one its
// registration names. The X-Tenant-ID header is never honoured there - a
// caller could otherwise pick the tenant a delivery acts in - and an invalid
// one is not even looked at.
func TestTenantID_WebhookRoutesIgnoreTheHeader(t *testing.T) {
	for _, header := range []string{"11111111-2222-3333-4444-555555555555", "not-a-uuid", ""} {
		for _, path := range []string{
			"/api/v1/webhooks/vcs/github/aaaaaaaa-0000-4000-8000-000000000001",
			"/api/v1/webhooks/pm/plane/aaaaaaaa-0000-4000-8000-000000000001",
			"/api/v1/webhooks/channels/aaaaaaaa-0000-4000-8000-000000000001",
		} {
			t.Run(path+"/"+header, func(t *testing.T) {
				var explicit bool
				handler := middleware.TenantID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, explicit = tenantctx.Explicit(r.Context())
					w.WriteHeader(http.StatusNoContent)
				}))
				req := httptest.NewRequest(http.MethodPost, path, http.NoBody)
				if header != "" {
					req.Header.Set("X-Tenant-ID", header)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code != http.StatusNoContent {
					t.Fatalf("status %d, want the handler to run (204)", rec.Code)
				}
				if explicit {
					t.Fatal("the webhook request got a tenant before its webhook was resolved")
				}
			})
		}
	}
}
