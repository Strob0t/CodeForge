package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-175: the scope a route needs from an API key, by method and path. The
// table in scopes.go documents the groups; this pins the mapping.
func TestAPIKeyScope_Table(t *testing.T) {
	for _, tc := range []struct {
		method, path, want string
	}{
		{http.MethodGet, "/api/v1", ""},
		{http.MethodGet, "/api/v1/", ""},
		{http.MethodGet, "/api/v1/source", ""},
		{http.MethodGet, "/api/v1/auth/me", ""},
		{http.MethodGet, "/api/v1/projects", user.ScopeProjectsRead},
		{http.MethodPost, "/api/v1/projects", user.ScopeProjectsWrite},
		{http.MethodDelete, "/api/v1/projects/p1", user.ScopeProjectsWrite},
		{http.MethodGet, "/api/v1/projects/p1/files/content", user.ScopeProjectsRead},
		{http.MethodPut, "/api/v1/projects/p1/files/content", user.ScopeProjectsWrite},
		{http.MethodGet, "/api/v1/projects/p1/roadmap", user.ScopeProjectsRead},
		{http.MethodPost, "/api/v1/search", user.ScopeProjectsRead},
		{http.MethodGet, "/api/v1/milestones/m1", user.ScopeProjectsRead},
		{http.MethodGet, "/api/v1/costs", user.ScopeProjectsRead},
		{http.MethodGet, "/api/v1/projects/p1/conversations", user.ScopeRunsRead},
		{http.MethodPost, "/api/v1/projects/p1/conversations", user.ScopeRunsWrite},
		{http.MethodPost, "/api/v1/conversations/c1/messages", user.ScopeRunsWrite},
		{http.MethodPost, "/api/v1/projects/p1/auto-agent/start", user.ScopeRunsWrite},
		{http.MethodPost, "/api/v1/projects/p1/decompose", user.ScopeRunsWrite},
		{http.MethodPost, "/api/v1/projects/p1/goals/ai-discover", user.ScopeRunsWrite},
		{http.MethodGet, "/api/v1/projects/p1/goals", user.ScopeProjectsRead},
		{http.MethodGet, "/api/v1/projects/p1/tasks", user.ScopeRunsRead},
		{http.MethodPost, "/api/v1/runs", user.ScopeRunsWrite},
		{http.MethodGet, "/api/v1/runs/r1/events", user.ScopeRunsRead},
		{http.MethodPost, "/api/v1/plans/p1/start", user.ScopeRunsWrite},
		{http.MethodGet, "/api/v1/policies", user.ScopeRunsRead},
		{http.MethodGet, "/api/v1/agents/a1", user.ScopeAgentsRead},
		{http.MethodPost, "/api/v1/agents/a1/dispatch", user.ScopeAgentsWrite},
		{http.MethodPost, "/api/v1/projects/p1/agents", user.ScopeAgentsWrite},
		{http.MethodGet, "/api/v1/a2a/agents", user.ScopeAgentsRead},
		{http.MethodGet, "/api/v1/projects/p1/webhooks", user.ScopeAdminAll},
		{http.MethodPost, "/api/v1/projects/p1/mcp-servers", user.ScopeAdminAll},
		{http.MethodGet, "/api/v1/users", user.ScopeAdminAll},
		{http.MethodPut, "/api/v1/settings", user.ScopeAdminAll},
		{http.MethodGet, "/api/v1/llm/models", user.ScopeAdminAll},
		{http.MethodPost, "/api/v1/auth/api-keys", user.ScopeAdminAll},
		{http.MethodGet, "/api/v1/audit-logs", user.ScopeAdminAll},
		{http.MethodGet, "/api/v1/tenants", user.ScopeAdminAll},
		{http.MethodGet, "/api/v1/knowledge-bases", user.ScopeAdminAll},
		{http.MethodGet, "/api/v1/unknown-future-route", user.ScopeAdminAll},
		// A read-only query needs the read scope for POST only; any other
		// method on its path is a write (S10-A review).
		{http.MethodPost, "/api/v1/prompt-sections/preview", user.ScopeRunsRead},
		{http.MethodDelete, "/api/v1/prompt-sections/preview", user.ScopeRunsWrite},
		{http.MethodPost, "/api/v1/projects/p1/search", user.ScopeProjectsRead},
		{http.MethodPut, "/api/v1/projects/p1/search", user.ScopeProjectsWrite},
		// Classified on the raw path, as chi routes: an encoded "/" stays
		// inside its segment.
		{http.MethodPost, "/api/v1/projects/p1%2Fsearch/clone", user.ScopeProjectsWrite},
		// Route patterns classify like their paths.
		{http.MethodPost, "/projects/{id}/clone", user.ScopeProjectsWrite},
		{http.MethodPost, "/projects/{id}/search", user.ScopeProjectsRead},
		{http.MethodGet, "/routing/stats", user.ScopeRunsRead},
	} {
		if got := cfhttp.APIKeyScope(tc.method, tc.path); got != tc.want {
			t.Errorf("%s %s: scope %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func apiKeyRequest(t *testing.T, r http.Handler, key *user.APIKey, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	admin := &user.User{ID: "ad", Email: "ad@example.com", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	ctx := middleware.ContextWithTestUser(req.Context(), admin)
	if key != nil {
		ctx = middleware.ContextWithTestAPIKey(ctx, key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req.WithContext(ctx))
	return w
}

// scopePassed, as the wanted status, means any answer but 403: the scope
// check let the request through to its handler, whose own answer (the
// mocks behind it) is not under test.
const scopePassed = 0

// A key with scopes calls only the routes of its scope groups; a key
// without scopes keeps its user's full rights (keys from before the
// enforcement); admin:all is everything; a JWT request is not affected.
func TestAPIKeyScopes_AreEnforced(t *testing.T) {
	store := &mockStore{projects: []project.Project{{ID: "p1", TenantID: tenantctx.DefaultTenantID}}}
	r := newTestRouterWithStore(store)
	readOnly := &user.APIKey{ID: "k1", UserID: "ad", Scopes: []string{user.ScopeProjectsRead}}
	unscoped := &user.APIKey{ID: "k2", UserID: "ad"}
	all := &user.APIKey{ID: "k3", UserID: "ad", Scopes: []string{user.ScopeAdminAll}}
	runsRead := &user.APIKey{ID: "k4", UserID: "ad", Scopes: []string{user.ScopeRunsRead}}

	for _, tc := range []struct {
		name   string
		key    *user.APIKey
		method string
		path   string
		body   string
		want   int
	}{
		{"read key reads projects", readOnly, http.MethodGet, "/api/v1/projects", "", http.StatusOK},
		{"read key reads itself", readOnly, http.MethodGet, "/api/v1/auth/me", "", http.StatusOK},
		{"read key creates a project", readOnly, http.MethodPost, "/api/v1/projects", `{"name":"x","provider":"local"}`, http.StatusForbidden},
		{"read key deletes a project", readOnly, http.MethodDelete, "/api/v1/projects/p1", "", http.StatusForbidden},
		{"read key reads runs", readOnly, http.MethodGet, "/api/v1/runs/r1", "", http.StatusForbidden},
		{"read key reads settings", readOnly, http.MethodGet, "/api/v1/settings", "", http.StatusForbidden},
		{"unscoped key creates a project", unscoped, http.MethodPost, "/api/v1/projects", `{"name":"x","provider":"local"}`, http.StatusCreated},
		{"unscoped key reads settings", unscoped, http.MethodGet, "/api/v1/settings", "", http.StatusOK},
		{"admin:all key reads settings", all, http.MethodGet, "/api/v1/settings", "", http.StatusOK},
		{"admin:all key deletes a project", all, http.MethodDelete, "/api/v1/projects/p1", "", http.StatusNoContent},
		{"jwt deletes a project", nil, http.MethodDelete, "/api/v1/projects/p1", "", http.StatusNoContent},
		// The scope follows the route chi dispatches to: the raw path (an
		// escaped "/" stays in its segment, so this is POST /projects/{id}/clone,
		// not the read-only /projects/{id}/search) and the method (a read-only
		// query is read for POST only).
		{"read key clones a project with an encoded id", readOnly, http.MethodPost, "/api/v1/projects/p1%2Fsearch/clone", "", http.StatusForbidden},
		{"read key searches a project", readOnly, http.MethodPost, "/api/v1/projects/p1/search", `{"query":"x"}`, scopePassed},
		{"runs:read key previews prompt sections", runsRead, http.MethodPost, "/api/v1/prompt-sections/preview", `{}`, scopePassed},
		{"runs:read key deletes the preview path", runsRead, http.MethodDelete, "/api/v1/prompt-sections/preview", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store.projects = []project.Project{{ID: "p1", TenantID: tenantctx.DefaultTenantID}}
			w := apiKeyRequest(t, r, tc.key, tc.method, tc.path, tc.body)
			if tc.want == scopePassed {
				if w.Code == http.StatusForbidden {
					t.Fatalf("status 403 (%s), want the scope check passed", w.Body.String())
				}
				return
			}
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d (%s)", w.Code, tc.want, w.Body.String())
			}
			if tc.want == http.StatusForbidden && !strings.Contains(w.Body.String(), "insufficient scope") {
				t.Fatalf("body %s, want insufficient scope", w.Body.String())
			}
		})
	}
}

// Creating a key with an unknown scope is a 400; an empty scope list means
// no restriction (the key keeps its user's rights) and is stored as none.
func TestCreateAPIKey_Scopes(t *testing.T) {
	r := newTestRouter()
	u := &user.User{ID: "user-1", Email: "k@test.com", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}
	create := func(t *testing.T, scopes string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/api-keys", bytes.NewReader([]byte(`{"name":"ci","scopes":`+scopes+`}`)))
		req.Header.Set("Content-Type", "application/json")
		req = withUserContext(req, u)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	w := create(t, `["projects:read","bogus:scope"]`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid scope: bogus:scope") {
		t.Fatalf("unknown scope: status %d (%s), want 400 naming the scope", w.Code, w.Body.String())
	}

	w = create(t, `[]`)
	if w.Code != http.StatusCreated {
		t.Fatalf("empty scopes: status %d (%s)", w.Code, w.Body.String())
	}
	var resp user.CreateAPIKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.APIKey.Scopes != nil {
		t.Fatalf("empty scopes stored as %v, want none", resp.APIKey.Scopes)
	}

	w = create(t, `["runs:write"]`)
	if w.Code != http.StatusCreated {
		t.Fatalf("valid scopes: status %d (%s)", w.Code, w.Body.String())
	}
}
