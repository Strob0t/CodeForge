package middleware

import (
	"net/http"

	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// RequireRole returns middleware that restricts access to users with one of the given roles.
func RequireRole(roles ...user.Role) func(http.Handler) http.Handler {
	allowed := make(map[user.Role]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFromContext(r.Context())
			if u == nil {
				http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
				return
			}

			if !allowed[u.Role] {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePlatformAdmin restricts a route to platform admins: the admins of
// the default (bootstrap) tenant, whose admin the server seeds and whose
// user every request is when authentication is disabled. Routes that change
// what all tenants share, such as the LiteLLM proxy's model list, use it
// (KI-75).
func RequirePlatformAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		if u == nil {
			http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
			return
		}
		if !u.IsPlatformAdmin() {
			http.Error(w, `{"error":"forbidden: platform admin required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
