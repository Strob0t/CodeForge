package http_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// routePathParams replaces chi's {param} segments with a placeholder value.
var routePathParams = regexp.MustCompile(`\{[^}]+\}`)

// roleMiddlewareName matches the closures of middleware.RequireRole, inlined
// into a mount function ("...mountProjectRoutes.RequireRole.func10",
// "...mountA2ARoutes.func1.RequireRole.1") or not
// ("...middleware.RequireRole.func1"), and middleware.RequirePlatformAdmin.
var roleMiddlewareName = regexp.MustCompile(`\.RequireRole\.(func)?\d+$|\.RequirePlatformAdmin$`)

func hasRoleMiddleware(mws []func(http.Handler) http.Handler) bool {
	for _, mw := range mws {
		if roleMiddlewareName.MatchString(runtime.FuncForPC(reflect.ValueOf(mw).Pointer()).Name()) {
			return true
		}
	}
	return false
}

// fullTestRouter mounts every route group, the optional ones included, in
// development mode (the benchmark routes exist only there).
func fullTestRouter(store *mockStore) chi.Router {
	return newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) {
			h.AppEnv = "development"
			h.A2A = service.NewA2AService(store, &mockQueue{})
			h.PromptEvolution = service.NewPromptEvolutionService(&mockQueue{}, nil, &prompt.EvolutionConfig{})
		})
}

// KI-171: every non-GET route under /api/v1 requires an editor or admin
// (RequireRole or RequirePlatformAdmin in its middleware chain, and a viewer
// gets 403), unless it is on the viewer allowlist of routes.go: the public
// auth and webhook routes, the caller's own session, account and settings,
// and read-only queries that take a body. A new mutating route without a
// role check or an allowlist entry fails this test; so does a stale
// allowlist entry.
func TestMutatingRoutes_RequireEditorOrAdmin(t *testing.T) {
	r := fullTestRouter(&mockStore{})
	viewer := &user.User{ID: "vi", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}
	seen := map[string]bool{}

	err := chi.Walk(r, func(method, route string, _ http.Handler, mws ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/")
		if method == http.MethodGet || !strings.HasPrefix(route, "/api/v1/") {
			return nil
		}
		key := method + " " + route
		seen[key] = true
		if _, allowed := cfhttp.ViewerRoutes[key]; allowed {
			return nil
		}
		if !hasRoleMiddleware(mws) {
			t.Errorf("%s has no RequireRole/RequirePlatformAdmin middleware and no viewer allowlist entry", key)
		}
		path := routePathParams.ReplaceAllString(route, "x")
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(middleware.ContextWithTestUser(req.Context(), viewer))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s as a viewer: status %d, want 403 (%s)", key, w.Code, w.Body.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for key, reason := range cfhttp.ViewerRoutes {
		if !seen[key] {
			t.Errorf("viewer allowlist entry %q (%s) matches no mounted route", key, reason)
		}
	}
}

// The agentic starts the review named (R2-1, R3-1, R10a-1): a viewer is
// refused, an editor is not.
func TestAgenticStarts_NeedEditor(t *testing.T) {
	viewer := &user.User{ID: "vi", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}
	editor := &user.User{ID: "ed", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}
	routes := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/projects/p1/conversations", `{"agentic":true,"mode":"prototyper"}`},
		{http.MethodPost, "/api/v1/conversations/c1/messages", `{"content":"rm -rf","agentic":true,"mode":"prototyper"}`},
		{http.MethodPost, "/api/v1/projects/p1/goals/ai-discover", `{}`},
		{http.MethodPost, "/api/v1/projects/p1/decompose", `{"description":"x","auto_start":true}`},
		{http.MethodPost, "/api/v1/projects/p1/plan-feature", `{"description":"x","auto_start":true}`},
	}
	r := fullTestRouter(&mockStore{})
	for _, rt := range routes {
		for _, u := range []*user.User{viewer, editor} {
			t.Run(rt.path+"/"+string(u.Role), func(t *testing.T) {
				req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
				req.Header.Set("Content-Type", "application/json")
				req = req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if forbidden := w.Code == http.StatusForbidden; forbidden != (u == viewer) {
					t.Fatalf("status %d (%s)", w.Code, w.Body.String())
				}
			})
		}
	}
}

// PUT /channels/{id}/members/{uid} changes the caller's own notification
// setting; another member's is an admin's business (R1-2, R3-9).
func TestUpdateMemberNotify_OwnSettingsOnly(t *testing.T) {
	viewer := &user.User{ID: "vi", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}
	editor := &user.User{ID: "ed", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}
	admin := &user.User{ID: "ad", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	store := &mockStore{}
	r := newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) { h.Channels = service.NewChannelService(store, &mockBroadcaster{}) })
	for _, tc := range []struct {
		name string
		u    *user.User
		uid  string
		want int
	}{
		{"viewer, own", viewer, "vi", http.StatusOK},
		{"viewer, other", viewer, "ed", http.StatusForbidden},
		{"editor, other", editor, "vi", http.StatusForbidden},
		{"admin, other", admin, "vi", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/channels/c1/members/"+tc.uid, strings.NewReader(`{"notify":"all"}`))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(middleware.ContextWithTestUser(req.Context(), tc.u))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d (%s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
