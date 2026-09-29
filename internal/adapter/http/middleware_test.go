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

	const appOrigin = "https://app.example.com"

	tests := []struct {
		name          string
		origin        string // configured allowed origin
		appEnv        string
		requestOrigin string // Origin header sent by the browser
		expectOrigin  string // expected Access-Control-Allow-Origin header
	}{
		{"empty env rejects wildcard", "*", "", appOrigin, ""},
		{"development allows wildcard", "*", "development", appOrigin, "*"},
		{"production rejects wildcard", "*", "production", appOrigin, ""},
		{"staging rejects wildcard", "*", "staging", appOrigin, ""},
		{"specific origin always allowed", appOrigin, "", appOrigin, appOrigin},
		{"specific origin in production", appOrigin, "production", appOrigin, appOrigin},
		{"foreign origin not reflected", appOrigin, "production", "https://evil.example.com", ""},
		{"missing origin not reflected", appOrigin, "production", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := CORS(tt.origin, tt.appEnv)(noop)
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}
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
