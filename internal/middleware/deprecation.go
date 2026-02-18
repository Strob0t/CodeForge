// Package middleware provides HTTP middleware for CodeForge.
package middleware

import (
	"net/http"
	"time"
)

// Deprecation returns middleware that adds RFC 8594 deprecation headers.
// The Sunset header uses RFC 7231 date format (HTTP-date).
// Apply this to API version groups that are scheduled for removal.
func Deprecation(sunset time.Time) func(http.Handler) http.Handler {
	sunsetStr := sunset.UTC().Format(http.TimeFormat)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Deprecation", "true")
			w.Header().Set("Sunset", sunsetStr)
			next.ServeHTTP(w, r)
		})
	}
}
