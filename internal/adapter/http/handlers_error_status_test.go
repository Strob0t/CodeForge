package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	_ "github.com/Strob0t/CodeForge/internal/adapter/githubpm" // registers the github-issues PM provider
	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-149: input the caller can correct answers 400 and an unknown resource
// 404; before, these errors carried no domain sentinel and answered 500.

const shortPassword = "Short1"

type statusCase struct {
	name   string
	method string
	path   string
	body   string                            // JSON
	as     func(store *mockStore) *user.User // nil: no user in the context
	want   int
}

// errorStatusRouter wires the services the default test router leaves out
// (consent, PM sync) and seeds a project, an admin and two consent purposes.
func errorStatusRouter(t *testing.T) (chi.Router, *mockStore) {
	t.Helper()
	store := &mockStore{}
	store.projects = append(store.projects, project.Project{ID: "proj-1", Name: "test"})
	store.consentPurposes = []database.ConsentPurpose{
		{ID: "analytics", TenantID: middleware.DefaultTenantID, Label: "Analytics", Version: 1},
		{ID: "essential", TenantID: middleware.DefaultTenantID, Label: "Essential", Required: true, Version: 1},
	}
	r := newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) {
			h.Consent = service.NewConsentService(store)
			h.Sync = service.NewSyncService(store)
		})
	httpSetupAdmin(t, r, "admin@test.com")
	return r, store
}

func adminOf(store *mockStore) *user.User {
	return &user.User{
		ID:       store.users[0].ID,
		Email:    store.users[0].Email,
		Name:     store.users[0].Name,
		Role:     user.RoleAdmin,
		TenantID: middleware.DefaultTenantID,
	}
}

func TestErrorStatus_ValidationAndNotFound(t *testing.T) {
	syncPath := "/api/v1/projects/proj-1/roadmap/sync"
	const token = `"provider_config":{"token":"ghp_test"}`
	tests := []statusCase{
		{
			name: "setup with a short password", method: "POST", path: "/api/v1/auth/setup",
			body: `{"email":"second@test.com","name":"S","password":"` + shortPassword + `"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "create user with a short password", method: "POST", path: "/api/v1/users/",
			body: `{"email":"new@test.com","name":"New","password":"` + shortPassword + `","role":"viewer"}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
		{
			name: "change password to a short one", method: "POST", path: "/api/v1/auth/change-password",
			body: `{"old_password":"` + validPassword + `","new_password":"` + shortPassword + `"}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
		{
			name: "change password with a wrong current password", method: "POST", path: "/api/v1/auth/change-password",
			body: `{"old_password":"WrongPassword1","new_password":"AnotherPass12"}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
		{
			name: "confirm a reset with a short password", method: "POST", path: "/api/v1/auth/reset-password",
			body: `{"token":"some-token","new_password":"` + shortPassword + `"}`,
			want: http.StatusBadRequest,
		},
		{
			name: "force a short password", method: "POST", path: "/api/v1/users/{admin}/force-password-change",
			body: `{"new_password":"` + shortPassword + `"}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
		{
			name: "update a user to an unknown role", method: "PUT", path: "/api/v1/users/{admin}",
			body: `{"role":"superuser"}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
		{
			name: "consent to an unknown purpose", method: "PUT", path: "/api/v1/me/consent/no-such-purpose",
			body: `{"granted":true}`,
			as:   adminOf, want: http.StatusNotFound,
		},
		{
			name: "withdraw a required consent", method: "PUT", path: "/api/v1/me/consent/essential",
			body: `{"granted":false}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
		{
			name: "sync with an unknown PM provider", method: "POST", path: syncPath,
			body: `{"provider":"no-such-pm","project_ref":"owner/repo"}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
		{
			name: "sync with a malformed PM reference", method: "POST", path: syncPath,
			body: `{"provider":"github-issues","project_ref":"not-owner-slash-repo",` + token + `}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
		{
			name: "sync in an unknown direction", method: "POST", path: syncPath,
			body: `{"provider":"github-issues","project_ref":"owner/repo","direction":"sideways",` + token + `}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
		{
			name: "import from an unknown PM provider", method: "POST", path: "/api/v1/projects/proj-1/roadmap/import/pm",
			body: `{"provider":"no-such-pm","project_ref":"owner/repo"}`,
			as:   adminOf, want: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, store := errorStatusRouter(t)
			path := strings.ReplaceAll(tt.path, "{admin}", store.users[0].ID)
			req := httptest.NewRequest(tt.method, path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.as != nil {
				req = withUserContext(req, tt.as(store))
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("%s %s = %d, want %d: %s", tt.method, path, w.Code, tt.want, w.Body.String())
			}
		})
	}
}
