package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// S7-G review: GET /me/export answers with all of a user's personal data and
// carried no Cache-Control, so a browser or a proxy could keep a copy. Every
// /api/v1 answer is marked no-store (middleware.NoStore).
func TestAPIAnswersAreNotStored(t *testing.T) {
	u := user.User{ID: "u-1", Email: "ada@example.org", Name: "Ada", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}
	store := &mockStore{users: []user.User{u}}
	r := newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) { h.GDPR = service.NewGDPRService(store) })

	for _, path := range []string{"/api/v1/me/export", "/api/v1/", "/api/v1/projects"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
			req = req.WithContext(middleware.ContextWithTestUser(req.Context(), &u))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if path == "/api/v1/me/export" && !strings.Contains(w.Body.String(), `"ada@example.org"`) {
				t.Errorf("export body = %s, want the user's data", w.Body.String())
			}
		})
	}
}
