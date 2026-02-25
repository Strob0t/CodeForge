package middleware

import (
	"net/http"
	"os"
)

// DevModeOnly returns middleware that restricts access to development environment.
// Checks APP_ENV environment variable; only "development" is allowed.
func DevModeOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("APP_ENV") != "development" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"feature only available in development mode (APP_ENV=development)"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
