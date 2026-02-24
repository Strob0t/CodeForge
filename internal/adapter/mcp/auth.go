package mcp

import (
	"net/http"
	"strings"
)

// AuthMiddleware wraps an http.Handler and validates the Authorization header.
// It checks for a Bearer token or API key matching the expected value.
// If apiKey is empty, the middleware passes all requests through (auth disabled).
func AuthMiddleware(apiKey string, next http.Handler) http.Handler {
	if apiKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth {
			// No "Bearer " prefix found, try plain API key header
			token = auth
		}

		if token != apiKey {
			http.Error(w, "invalid credentials", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
