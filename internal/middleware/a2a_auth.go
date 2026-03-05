package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

// A2ATrustLevel represents the trust level for an A2A request.
type A2ATrustLevel string

const (
	// A2ATrustUntrusted means the request is unauthenticated.
	A2ATrustUntrusted A2ATrustLevel = "untrusted"
	// A2ATrustPartial means the request has a valid API key.
	A2ATrustPartial A2ATrustLevel = "partial"
)

type ctxKeyA2ATrust struct{}

// A2AAuth returns middleware that validates Bearer tokens for A2A endpoints.
// If validKeys is empty, all requests pass through with "untrusted" trust level.
// Token comparison uses constant-time comparison to prevent timing attacks.
func A2AAuth(validKeys []string) func(http.Handler) http.Handler {
	keys := make([]string, len(validKeys))
	copy(keys, validKeys)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Open mode: no keys configured — allow all with untrusted trust.
			if len(keys) == 0 {
				ctx := context.WithValue(r.Context(), ctxKeyA2ATrust{}, A2ATrustUntrusted)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			if !constantTimeContains(keys, token) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyA2ATrust{}, A2ATrustPartial)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// constantTimeContains checks if token matches any key using constant-time comparison.
// Iterates all keys without early return to avoid leaking the matching key's index.
func constantTimeContains(keys []string, token string) bool {
	var match int
	for _, k := range keys {
		match |= subtle.ConstantTimeCompare([]byte(k), []byte(token))
	}
	return match == 1
}

// A2ATrustFromContext returns the A2A trust level from the request context.
func A2ATrustFromContext(ctx context.Context) A2ATrustLevel {
	v, ok := ctx.Value(ctxKeyA2ATrust{}).(A2ATrustLevel)
	if !ok {
		return A2ATrustUntrusted
	}
	return v
}
