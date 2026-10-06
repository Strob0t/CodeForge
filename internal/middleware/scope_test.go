package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// serveScoped runs a request with key (nil: a JWT request) through
// RequireScopeFunc resolving the given scope.
func serveScoped(key *user.APIKey, scope string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", http.NoBody)
	if key != nil {
		req = req.WithContext(middleware.ContextWithTestAPIKey(req.Context(), key))
	}
	rec := httptest.NewRecorder()
	mw := middleware.RequireScopeFunc(func(*http.Request) string { return scope })
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(rec, req)
	return rec
}

// KI-175: a scoped key reaches only the routes of its scopes; a key without
// scopes, nil or empty (a key created with "scopes": [] is stored and read
// back as an empty list, S10-A review), keeps its user's rights; a route
// without a scope and a JWT request pass.
func TestRequireScopeFunc(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   *user.APIKey
		scope string
		want  int
	}{
		{"jwt request", nil, user.ScopeProjectsWrite, http.StatusNoContent},
		{"nil scopes", &user.APIKey{ID: "k"}, user.ScopeProjectsWrite, http.StatusNoContent},
		{"empty scopes", &user.APIKey{ID: "k", Scopes: []string{}}, user.ScopeProjectsWrite, http.StatusNoContent},
		{"matching scope", &user.APIKey{ID: "k", Scopes: []string{user.ScopeProjectsWrite}}, user.ScopeProjectsWrite, http.StatusNoContent},
		{"admin:all", &user.APIKey{ID: "k", Scopes: []string{user.ScopeAdminAll}}, user.ScopeProjectsWrite, http.StatusNoContent},
		{"route without a scope", &user.APIKey{ID: "k", Scopes: []string{user.ScopeRunsRead}}, "", http.StatusNoContent},
		{"other scope", &user.APIKey{ID: "k", Scopes: []string{user.ScopeProjectsRead}}, user.ScopeProjectsWrite, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveScoped(tc.key, tc.scope)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusForbidden && !strings.Contains(rec.Body.String(), "insufficient scope") {
				t.Fatalf("body %q, want insufficient scope", rec.Body.String())
			}
		})
	}
}
