package middleware

import (
	"net/http"
)

// RequireScope returns middleware that checks API key scopes.
// JWT requests pass through (JWT users have role-based access via RBAC).
// API keys without scopes pass through (they keep their user's rights).
func RequireScope(scope string) func(http.Handler) http.Handler {
	return RequireScopeFunc(func(*http.Request) string { return scope })
}

// RequireScopeFunc is RequireScope with the scope resolved per request
// (the API's route table, KI-175); an empty scope needs no check.
func RequireScopeFunc(scopeOf func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := APIKeyFromContext(r.Context())
			if key == nil {
				// Not an API key request (JWT or no auth) — pass through.
				next.ServeHTTP(w, r)
				return
			}

			// No scopes, nil or empty, means unrestricted: keys from before
			// the enforcement, and a key created with "scopes": [] once
			// stored and read back (an empty list, not nil).
			if len(key.Scopes) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			if scope := scopeOf(r); scope != "" && !key.HasScope(scope) {
				http.Error(w, `{"error":"insufficient scope"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
