package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// writeDeadlineGrace is how long a route with its own bound may still write
// its response (the 504 of chi's Timeout) after the bound passed.
const writeDeadlineGrace = 10 * time.Second

// RouteTimeout bounds each request like chi's Timeout: with def, or for the
// route patterns in overrides with their own duration, where 0 means none
// (streams that end with their task or their client). Long git operations and
// SSE streams outlive the default (KI-213).
//
// routes is the root router: below a mounted sub-router the full pattern
// ("/api/v1/projects/{id}/clone") is known only after routing, so it is looked
// up. An overridden route also moves the connection's write deadline
// (http.Server.WriteTimeout) with its bound.
func RouteTimeout(routes chi.Routes, def time.Duration, overrides map[string]time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		bounded := chimw.Timeout(def)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.RawPath
			if path == "" {
				path = r.URL.Path
			}
			d, ok := overrides[routes.Find(chi.NewRouteContext(), r.Method, path)]
			if !ok {
				bounded.ServeHTTP(w, r)
				return
			}
			var writeDeadline time.Time // zero: none
			if d > 0 {
				writeDeadline = time.Now().Add(d + writeDeadlineGrace)
			}
			if err := http.NewResponseController(w).SetWriteDeadline(writeDeadline); err != nil {
				slog.Warn("route timeout: the write deadline stays", "path", r.URL.Path, "error", err)
			}
			if d <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			chimw.Timeout(d)(next).ServeHTTP(w, r)
		})
	}
}
