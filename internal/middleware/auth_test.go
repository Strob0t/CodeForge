package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

func newTestAuthSvc() *service.AuthService {
	cfg := config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-for-middleware",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
	}
	// nil store is fine — middleware only calls ValidateAccessToken/ValidateAPIKey,
	// and for these tests we only test token parsing (no DB calls).
	return service.NewAuthService(nil, &cfg)
}

func TestAuth_Disabled_InjectsDefaultAdmin(t *testing.T) {
	handler := middleware.Auth(nil, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := middleware.UserFromContext(r.Context())
		if u == nil {
			t.Fatal("expected default user in context")
		}
		if u.Role != "admin" {
			t.Errorf("role = %q, want admin", u.Role)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuth_Enabled_NoHeader_Returns401(t *testing.T) {
	svc := newTestAuthSvc()
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_PublicPath_NoAuthRequired(t *testing.T) {
	svc := newTestAuthSvc()
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/health", "/health/ready", "/api/v1/auth/login", "/api/v1/auth/refresh"} {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("path %s: status = %d, want 200", path, rec.Code)
		}
	}
}

func TestAuth_InvalidBearerToken_Returns401(t *testing.T) {
	svc := newTestAuthSvc()
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
