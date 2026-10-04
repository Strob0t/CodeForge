package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Platform admins manage what all tenants share (the LiteLLM proxy): they are
// the admins of the default (bootstrap) tenant (KI-75).
func TestRequirePlatformAdmin(t *testing.T) {
	const otherTenant = "11111111-2222-3333-4444-555555555555"
	tests := []struct {
		name string
		user *user.User
		want int
	}{
		{"admin of the default tenant", &user.User{ID: "u1", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}, http.StatusOK},
		{"admin of another tenant", &user.User{ID: "u2", Role: user.RoleAdmin, TenantID: otherTenant}, http.StatusForbidden},
		{"editor of the default tenant", &user.User{ID: "u3", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}, http.StatusForbidden},
		{"viewer of the default tenant", &user.User{ID: "u4", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}, http.StatusForbidden},
		{"admin without a tenant", &user.User{ID: "u5", Role: user.RoleAdmin}, http.StatusForbidden},
		{"no user", nil, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			req := httptest.NewRequest(http.MethodPost, "/api/v1/llm/models", http.NoBody)
			if tt.user != nil {
				req = req.WithContext(middleware.ContextWithTestUser(req.Context(), tt.user))
			}
			rec := httptest.NewRecorder()

			middleware.RequirePlatformAdmin(inner).ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

// With authentication disabled every request is the default tenant's admin.
func TestRequirePlatformAdmin_AuthDisabled(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := middleware.Auth(nil, false)(middleware.RequirePlatformAdmin(inner))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/llm/models", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
