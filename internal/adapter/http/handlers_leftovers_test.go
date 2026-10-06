package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const leftoverOtherTenant = "11111111-2222-3333-4444-555555555555"

func serveAs(t *testing.T, r http.Handler, u *user.User, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// KI-176 (R1-14, R3-8): the admin GDPR export and the user deletes act only
// within the caller's tenant; another tenant's user is 404 and keeps its
// row and its sessions.
func TestAdminUserRoutes_StayInTheCallersTenant(t *testing.T) {
	admin := &user.User{ID: "ad", Email: "ad@example.com", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/users/u2/export"},
		{http.MethodDelete, "/api/v1/users/u2"},
		{http.MethodDelete, "/api/v1/users/u2/data"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			store := &mockStore{users: []user.User{
				{ID: "u1", Email: "a@example.com", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID, Enabled: true},
				{ID: "u2", Email: "b@example.com", Role: user.RoleViewer, TenantID: leftoverOtherTenant, Enabled: true},
			}}
			dropper := &recordingDropper{}
			r := newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
				func(h *cfhttp.Handlers) {
					h.Auth.SetConnectionDropper(dropper)
					h.GDPR = service.NewGDPRService(store)
					h.GDPR.SetTokenInvalidator(h.Auth.Tokens())
				})
			w := serveAs(t, r, admin, tc.method, tc.path, "")
			if w.Code != http.StatusNotFound {
				t.Fatalf("status %d, want 404 (%s)", w.Code, w.Body.String())
			}
			if len(store.users) != 2 {
				t.Fatalf("users left: %d, want 2", len(store.users))
			}
			if len(dropper.users) != 0 {
				t.Fatalf("sessions ended for %v, want none", dropper.users)
			}
			if strings.Contains(w.Body.String(), "b@example.com") {
				t.Fatalf("the answer leaks the foreign user: %s", w.Body.String())
			}
		})
	}
}

// recordingDropper records whose WebSocket connections were closed.
type recordingDropper struct{ users []string }

func (d *recordingDropper) DropUser(userID string) int {
	d.users = append(d.users, userID)
	return 1
}

// KI-176 (R1-15, R4-10, R10a-22): the push-config token is returned to
// admins only; others get the config with an empty token and has_token.
func TestListA2APushConfigs_TokenOnlyForAdmins(t *testing.T) {
	ms := &a2aTestStore{pushConfigs: []database.A2APushConfig{{ID: "pc-1", TaskID: "t-1", URL: "https://example.com/hook", Token: "s3cret"}}}
	h := newA2AHandlers(ms)
	for _, tc := range []struct {
		role      user.Role
		wantToken string
	}{
		{user.RoleAdmin, "s3cret"},
		{user.RoleEditor, ""},
		{user.RoleViewer, ""},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			req := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/a2a/tasks/t-1/push-config", http.NoBody), "id", "t-1")
			req = withUserContext(req, &user.User{ID: "u", Role: tc.role, TenantID: tenantctx.DefaultTenantID})
			w := httptest.NewRecorder()
			h.ListA2APushConfigs(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status %d (%s)", w.Code, w.Body.String())
			}
			var configs []struct {
				ID       string `json:"id"`
				URL      string `json:"url"`
				Token    string `json:"token"`
				HasToken bool   `json:"has_token"`
			}
			if err := json.NewDecoder(w.Body).Decode(&configs); err != nil {
				t.Fatal(err)
			}
			if len(configs) != 1 || configs[0].Token != tc.wantToken || !configs[0].HasToken || configs[0].URL != "https://example.com/hook" {
				t.Fatalf("configs = %+v, want token %q with has_token", configs, tc.wantToken)
			}
		})
	}
}

// KI-176 (R1-16): every handler that decodes a body applies the shared body
// limit; an oversized body is 413 whether the body is required or optional.
func TestBodyLimit_AppliesToEveryDecodingHandler(t *testing.T) {
	store := &mockStore{projects: []project.Project{{ID: "p1", TenantID: tenantctx.DefaultTenantID, WorkspacePath: t.TempDir()}}}
	r := newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) {
			h.Limits.MaxRequestBodySize = 64
			h.PromptEvolution = service.NewPromptEvolutionService(&mockQueue{}, nil, &prompt.EvolutionConfig{})
			h.LSP = service.NewLSPService(&config.LSP{}, &mockBroadcaster{}, store, nil)
		})
	admin := &user.User{ID: "ad", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	big := `{"branch":"` + strings.Repeat("x", 200) + `"}`
	for _, path := range []string{
		"/api/v1/projects/p1/clone",
		"/api/v1/projects/p1/setup",
		"/api/v1/projects/p1/review-refactor",
		"/api/v1/projects/p1/lsp/start",
		"/api/v1/projects/p1/goals/ai-discover",
		"/api/v1/teams/t1/shared-context",
		"/api/v1/teams/t1/shared-context/items",
		"/api/v1/prompt-evolution/reflect",
	} {
		t.Run(path, func(t *testing.T) {
			w := serveAs(t, r, admin, http.MethodPost, path, big)
			if w.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status %d, want 413 (%s)", w.Code, w.Body.String())
			}
		})
	}
}

// KI-176 (R1-21): validation and not-found errors answer 400 and 404, not 500.
func TestLeftoverErrors_AreNot500(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	admin := &user.User{ID: "ad", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"settings with an empty key", http.MethodPut, "/api/v1/settings", `{"settings":{"":"x"}}`, http.StatusBadRequest},
		{"routing outcome with an invalid task type", http.MethodPost, "/api/v1/routing/outcomes", `{"model_name":"m","task_type":"bogus","complexity_tier":"simple"}`, http.StatusBadRequest},
		{"ai goal discovery on an unknown project", http.MethodPost, "/api/v1/projects/nope/goals/ai-discover", `{}`, http.StatusNotFound},
		{"goal detection on an unknown project", http.MethodPost, "/api/v1/projects/nope/goals/detect", ``, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := serveAs(t, r, admin, tc.method, tc.path, tc.body)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d (%s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
