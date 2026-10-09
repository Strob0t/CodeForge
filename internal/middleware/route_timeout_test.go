package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// routeTimeoutRouter mounts /api/v1 under a group with RouteTimeout, like
// cmd/codeforge: the full route pattern is known only below the mount.
func routeTimeoutRouter(def time.Duration, overrides map[string]time.Duration, h http.HandlerFunc) *chi.Mux {
	r := chi.NewRouter()
	r.Group(func(api chi.Router) {
		api.Use(RouteTimeout(r, def, overrides))
		api.Route("/api/v1", func(v1 chi.Router) {
			v1.Get("/projects", h)
			v1.Post("/projects/{id}/clone", h)
			v1.Get("/tasks/{id}/subscribe", h)
		})
	})
	return r
}

// reportDeadline answers with the time left until the request's deadline in
// the X-Deadline-Ms header, or "none".
func reportDeadline(w http.ResponseWriter, r *http.Request) {
	left := "none"
	if deadline, ok := r.Context().Deadline(); ok {
		left = strconv.FormatInt(time.Until(deadline).Milliseconds(), 10)
	}
	w.Header().Set("X-Deadline-Ms", left)
	w.WriteHeader(http.StatusNoContent)
}

// KI-213: the 30 s API timeout killed clones and ended SSE streams; those
// routes get their own bound (0: none), every other route keeps the default.
func TestRouteTimeout_PerRoute(t *testing.T) {
	overrides := map[string]time.Duration{
		"/api/v1/projects/{id}/clone":     10 * time.Minute,
		"/api/v1/tasks/{id}/subscribe":    0,
		"/api/v1/not-mounted/{id}/ignore": time.Hour,
	}
	r := routeTimeoutRouter(30*time.Second, overrides, reportDeadline)
	tests := []struct {
		method, path string
		wantMinMs    int64
		wantMaxMs    int64 // 0 with wantNone
		wantNone     bool
	}{
		{http.MethodGet, "/api/v1/projects", 29_000, 30_000, false},
		{http.MethodPost, "/api/v1/projects/p1/clone", 599_000, 600_000, false},
		{http.MethodGet, "/api/v1/tasks/t1/subscribe", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), tt.method, tt.path, http.NoBody))
			body := rec.Header().Get("X-Deadline-Ms")
			if tt.wantNone {
				if body != "none" {
					t.Fatalf("deadline in %s ms, want none", body)
				}
				return
			}
			var ms int64
			if _, err := fmt.Sscan(body, &ms); err != nil {
				t.Fatalf("body %q: %v", body, err)
			}
			if ms < tt.wantMinMs || ms > tt.wantMaxMs {
				t.Fatalf("deadline in %d ms, want %d..%d", ms, tt.wantMinMs, tt.wantMaxMs)
			}
		})
	}
}

func TestRouteTimeout_DefaultStillEndsSlowRequests(t *testing.T) {
	r := routeTimeoutRouter(20*time.Millisecond, nil, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/projects", http.NoBody))
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status %d, want 504", rec.Code)
	}
}

// The server's write timeout (server.write_timeout, 60 s) would cut a long
// clone's response or a stream; a route with its own bound moves the
// connection's write deadline with it.
func TestRouteTimeout_MovesTheWriteDeadline(t *testing.T) {
	overrides := map[string]time.Duration{
		"/api/v1/projects/{id}/clone":  time.Minute,
		"/api/v1/tasks/{id}/subscribe": 0,
	}
	r := routeTimeoutRouter(time.Minute, overrides, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, "done")
	})
	srv := httptest.NewUnstartedServer(r)
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Start()
	defer srv.Close()

	get := func(method, path string) (string, error) {
		req, err := http.NewRequestWithContext(context.Background(), method, srv.URL+path, http.NoBody)
		if err != nil {
			return "", err
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		return string(b), err
	}

	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/projects/p1/clone"},
		{http.MethodGet, "/api/v1/tasks/t1/subscribe"},
	} {
		body, err := get(route.method, route.path)
		if err != nil || body != "done" {
			t.Errorf("%s: body %q, err %v; want the response after the server's write timeout", route.path, body, err)
		}
	}
	// Other routes keep the server's write timeout.
	if body, err := get(http.MethodGet, "/api/v1/projects"); err == nil && strings.Contains(body, "done") {
		t.Error("a default route outlived the server's write timeout")
	}
}
