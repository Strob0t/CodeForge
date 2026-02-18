package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/middleware"
)

func TestDeprecation_SetsHeaders(t *testing.T) {
	sunset := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	handler := middleware.Deprecation(sunset)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Deprecation"); got != "true" {
		t.Fatalf("expected Deprecation header 'true', got %q", got)
	}

	sunsetVal := rec.Header().Get("Sunset")
	if sunsetVal == "" {
		t.Fatal("expected Sunset header to be set")
	}

	// Verify the Sunset header is a valid RFC 7231 HTTP-date.
	parsed, err := time.Parse(http.TimeFormat, sunsetVal)
	if err != nil {
		t.Fatalf("Sunset header is not valid HTTP-date: %v", err)
	}
	if !parsed.Equal(sunset) {
		t.Fatalf("expected sunset %v, got %v", sunset, parsed)
	}
}

func TestDeprecation_NonDeprecatedRoutesUnaffected(t *testing.T) {
	// Routes that don't use the middleware should not get deprecation headers.
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v2/projects", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Deprecation"); got != "" {
		t.Fatalf("expected no Deprecation header on non-deprecated route, got %q", got)
	}
	if got := rec.Header().Get("Sunset"); got != "" {
		t.Fatalf("expected no Sunset header on non-deprecated route, got %q", got)
	}
}

func TestDeprecation_PassesThroughToNextHandler(t *testing.T) {
	sunset := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	called := false
	handler := middleware.Deprecation(sunset)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected next handler to be called")
	}
	if rec.Code != http.StatusTeapot {
		t.Fatalf("expected status %d, got %d", http.StatusTeapot, rec.Code)
	}
}
