package middleware

import "net/http"

// NoStore marks every answer "Cache-Control: no-store": API answers carry
// personal and tenant data (GET /me/export holds all of a user's), which no
// browser or proxy cache may keep. The frontend keeps its own in-memory
// cache and relies on no HTTP caching. A handler that sets its own
// Cache-Control (an event stream) overrides it.
func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
