package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// The routes whose body is optional (readOptionalJSON) still accept a
// request without one, and refuse a body that is not JSON with 400 instead
// of acting on the zero value (S10-A review). StartLSP reads its body after
// its own preconditions and is covered by TestReadOptionalJSON.
func TestOptionalBodyRoutes(t *testing.T) {
	store := &mockStore{projects: []project.Project{{ID: "p1", TenantID: tenantctx.DefaultTenantID, Name: "p1", Provider: "local"}}}
	r := fullTestRouter(store)
	editor := &user.User{ID: "ed", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}
	for _, path := range []string{
		"/api/v1/projects/p1/clone",
		"/api/v1/projects/p1/setup",
		"/api/v1/projects/p1/review-refactor",
		"/api/v1/projects/p1/goals/ai-discover",
	} {
		serve := func(body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = withUserContext(req, editor)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			return w
		}
		t.Run(path, func(t *testing.T) {
			store.projects = []project.Project{{ID: "p1", TenantID: tenantctx.DefaultTenantID, Name: "p1", Provider: "local"}}
			if w := serve(""); strings.Contains(w.Body.String(), "invalid request body") || w.Code == http.StatusRequestEntityTooLarge {
				t.Fatalf("no body: status %d (%s), want the request handled", w.Code, w.Body.String())
			}
			if w := serve(`{"branch":`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid request body") {
				t.Fatalf("malformed body: status %d (%s), want 400 invalid request body", w.Code, w.Body.String())
			}
			if w := serve(`{"pad":"` + strings.Repeat("x", 1<<20) + `"}`); w.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("oversized body: status %d (%s), want 413", w.Code, w.Body.String())
			}
		})
	}
}
