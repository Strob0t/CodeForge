// Package http provides HTTP middleware and handler adapters.
package http

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Strob0t/CodeForge/internal/logger"
)

// SecurityHeaders returns middleware that sets standard HTTP security headers.
//
// FIX-095: CSRF protection note — explicit CSRF tokens are not needed because:
//  1. This is an API-only backend (no HTML forms, no cookie-based auth for mutations).
//  2. Authentication uses Bearer tokens via the Authorization header, which browsers
//     do not attach automatically to cross-origin requests.
//  3. CORS middleware restricts allowed origins, blocking cross-origin preflight.
//  4. SameSite cookie attributes (when cookies are used for sessions) prevent
//     cross-site request attachment.
//
// If cookie-based auth is ever added for browser form submissions, add CSRF tokens.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("X-XSS-Protection", "0")
		// img-src data: is required for canvas PNG export, inline base64 chat images, and SVG data URIs.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// CORS returns middleware that sets CORS headers for development.
// When allowedOrigin is "*", credentials are not allowed (browser security).
// In non-development environments, wildcard origins are rejected to prevent
// misconfiguration in production. The appEnv parameter should come from cfg.AppEnv.
func CORS(allowedOrigin, appEnv string) func(http.Handler) http.Handler {
	if allowedOrigin == "*" {
		if appEnv != "" && appEnv != "development" {
			slog.Error("CORS wildcard (*) is not allowed in non-development environments; set CODEFORGE_CORS_ORIGIN to a specific origin")
			// Fall back to deny-all rather than allow-all in production.
			allowedOrigin = ""
		} else {
			slog.Warn("CORS origin is wildcard (*) - credentials will not be allowed; set CODEFORGE_CORS_ORIGIN for production")
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Tenant-ID, X-Idempotency-Key")
			// Wildcard origin + credentials is rejected by browsers (spec violation).
			if allowedOrigin != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Logger returns middleware that logs HTTP requests using slog.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", logger.RequestID(r.Context()),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker, required for WebSocket upgrades.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("upstream ResponseWriter does not implement http.Hijacker")
}

// Flush implements http.Flusher, required for streaming responses (SSE).
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
