package mcp

import (
	"net/http"
	"strings"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// AuthMiddleware wraps an http.Handler and validates the Authorization header.
// It checks for a Bearer token or API key matching the expected value.
// If apiKey is empty, the middleware passes all requests through (auth disabled)
// but still injects tenant context so downstream handlers always have a tenant ID.
func AuthMiddleware(apiKey, tenantID string, next http.Handler) http.Handler {
	if apiKey == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := tenantctx.WithTenant(r.Context(), tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
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

		// Inject tenant context for downstream handlers.
		ctx := tenantctx.WithTenant(r.Context(), tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
