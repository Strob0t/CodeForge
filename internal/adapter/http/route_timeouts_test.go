package http_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Every route with its own timeout is a mounted route: a renamed route would
// silently fall back to the 30 s default (KI-213).
func TestLongRunningRoutes_AreMounted(t *testing.T) {
	router := newTestRouterWithLLM(&mockStore{}, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) { h.A2A = service.NewA2AService(&mockStore{}, &mockQueue{}) })
	mounted := map[string]bool{}
	err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	routes := cfhttp.LongRunningRoutes(17 * time.Minute)
	for _, pattern := range []string{
		"/api/v1/projects/{id}/clone",
		"/api/v1/projects/{id}/setup",
		"/api/v1/projects/{id}/git/pull",
		"/api/v1/projects/batch/pull",
		"/api/v1/a2a/tasks/{id}/subscribe",
	} {
		if _, ok := routes[pattern]; !ok {
			t.Errorf("%s has no timeout of its own", pattern)
		}
	}
	for pattern, d := range routes {
		if !mounted[pattern] {
			t.Errorf("%s is not a mounted route", pattern)
		}
		want := 17 * time.Minute
		if pattern == "/api/v1/a2a/tasks/{id}/subscribe" {
			want = 0 // a stream ends with its task or its client
		}
		if d != want {
			t.Errorf("%s: timeout %s, want %s", pattern, d, want)
		}
	}
}
