package middleware

import (
	"net/http"
)

// RequireScope returns middleware that checks API key scopes.
// JWT requests pass through (JWT users have role-based access via RBAC).
// API keys with nil scopes pass through (backward compat for old keys).
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := APIKeyFromContext(r.Context())
			if key == nil {
				// Not an API key request (JWT or no auth) — pass through.
				next.ServeHTTP(w, r)
				return
			}

			// Nil scopes means unrestricted (backward compat for old keys).
			if key.Scopes == nil {
				next.ServeHTTP(w, r)
				return
			}

			if !key.HasScope(scope) {
				http.Error(w, `{"error":"insufficient scope"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
