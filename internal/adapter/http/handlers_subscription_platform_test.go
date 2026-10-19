package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// TestSubscriptionConnectIsPlatformAdminOnly: connecting or disconnecting a
// subscription provider writes the platform's .env (the API key LiteLLM uses
// for every tenant), so only platform admins may do it, as for the model list
// (KI-75). Listing and status stay open. Without a configured subscription
// service a request that passes the check gets 501.
func TestSubscriptionConnectIsPlatformAdminOnly(t *testing.T) {
	const otherTenant = "11111111-2222-3333-4444-555555555555"
	platformAdmin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	tenantAdmin := &user.User{ID: "ta", Role: user.RoleAdmin, TenantID: otherTenant}
	defaultEditor := &user.User{ID: "de", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}

	tests := []struct {
		name       string
		method     string
		path       string
		user       *user.User
		wantStatus int
	}{
		{"platform admin connects", http.MethodPost, "/api/v1/auth/providers/github_copilot/connect", platformAdmin, http.StatusNotImplemented},
		{"admin of another tenant cannot connect", http.MethodPost, "/api/v1/auth/providers/github_copilot/connect", tenantAdmin, http.StatusForbidden},
		{"editor cannot connect", http.MethodPost, "/api/v1/auth/providers/github_copilot/connect", defaultEditor, http.StatusForbidden},
		{"platform admin disconnects", http.MethodDelete, "/api/v1/auth/providers/github_copilot/disconnect", platformAdmin, http.StatusNotImplemented},
		{"admin of another tenant cannot disconnect", http.MethodDelete, "/api/v1/auth/providers/github_copilot/disconnect", tenantAdmin, http.StatusForbidden},
		{"anyone lists", http.MethodGet, "/api/v1/auth/providers", tenantAdmin, http.StatusNotImplemented},
		{"anyone reads the status", http.MethodGet, "/api/v1/auth/providers/github_copilot/status", defaultEditor, http.StatusNotImplemented},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, http.NoBody)
			req = req.WithContext(middleware.ContextWithTestUser(req.Context(), tt.user))
			w := httptest.NewRecorder()
			newTestRouter().ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
