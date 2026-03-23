package http

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// hijackableRecorder wraps httptest.ResponseRecorder to implement http.Hijacker.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// Return dummy values — we only test that the call delegates.
	return nil, nil, nil
}

func TestResponseWriterHijack(t *testing.T) {
	inner := &hijackableRecorder{httptest.NewRecorder()}
	rw := &responseWriter{ResponseWriter: inner, status: http.StatusOK}

	// responseWriter must satisfy http.Hijacker.
	hj, ok := http.ResponseWriter(rw).(http.Hijacker)
	if !ok {
		t.Fatal("responseWriter does not implement http.Hijacker")
	}

	_, _, err := hj.Hijack()
	if err != nil {
		t.Fatalf("Hijack returned unexpected error: %v", err)
	}
}

func TestResponseWriterHijackFallback(t *testing.T) {
	// Standard httptest.ResponseRecorder does NOT implement Hijacker.
	inner := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: inner, status: http.StatusOK}

	hj, ok := http.ResponseWriter(rw).(http.Hijacker)
	if !ok {
		t.Fatal("responseWriter does not implement http.Hijacker")
	}

	_, _, err := hj.Hijack()
	if err == nil {
		t.Fatal("expected error when upstream does not implement Hijacker")
	}
}

func TestCORSWildcardRestriction(t *testing.T) {
	noop := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name         string
		origin       string
		appEnv       string
		expectOrigin string // expected Access-Control-Allow-Origin header
	}{
		{"empty env rejects wildcard", "*", "", ""},
		{"development allows wildcard", "*", "development", "*"},
		{"production rejects wildcard", "*", "production", ""},
		{"staging rejects wildcard", "*", "staging", ""},
		{"specific origin always allowed", "https://app.example.com", "", "https://app.example.com"},
		{"specific origin in production", "https://app.example.com", "production", "https://app.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := CORS(tt.origin, tt.appEnv)(noop)
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			got := rec.Header().Get("Access-Control-Allow-Origin")
			if got != tt.expectOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.expectOrigin)
			}
		})
	}
}

func TestResponseWriterFlush(t *testing.T) {
	inner := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: inner, status: http.StatusOK}

	// responseWriter must satisfy http.Flusher.
	f, ok := http.ResponseWriter(rw).(http.Flusher)
	if !ok {
		t.Fatal("responseWriter does not implement http.Flusher")
	}

	// Should not panic.
	f.Flush()

	if !inner.Flushed {
		t.Fatal("expected inner ResponseRecorder to be flushed")
	}
}
