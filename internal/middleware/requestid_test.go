package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/logger"
)

func TestRequestIDGenerated(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := logger.RequestID(r.Context())
		if id == "" {
			t.Error("expected generated request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	respID := rec.Header().Get("X-Request-ID")
	if respID == "" {
		t.Error("expected X-Request-ID in response header")
	}
	if len(respID) != 32 {
		t.Errorf("expected 32-char hex ID, got %d chars: %q", len(respID), respID)
	}
}

func TestRequestIDPropagated(t *testing.T) {
	const existingID = "my-custom-id-123"

	var capturedID string
	handler := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedID = logger.RequestID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("X-Request-ID", existingID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedID != existingID {
		t.Errorf("expected %q in context, got %q", existingID, capturedID)
	}

	if rec.Header().Get("X-Request-ID") != existingID {
		t.Errorf("expected %q in response header, got %q", existingID, rec.Header().Get("X-Request-ID"))
	}
}
