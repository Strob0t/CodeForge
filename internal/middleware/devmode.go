package middleware

import (
	"net/http"
)

// DevModeOnly returns middleware that restricts access to development environment.
// The appEnv parameter should come from cfg.AppEnv (loaded from the APP_ENV env var
// via the config loader). Only "development" is allowed.
func DevModeOnly(appEnv string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if appEnv != "development" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"feature only available in development mode (APP_ENV=development)"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
