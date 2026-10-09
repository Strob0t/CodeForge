package http_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// TestLLMModelManagementIsPlatformAdminOnly: all tenants share one LiteLLM
// proxy, so only the default tenant's admins may add or delete its models;
// every user keeps read access (KI-75).
func TestLLMModelManagementIsPlatformAdminOnly(t *testing.T) {
	const otherTenant = "11111111-2222-3333-4444-555555555555"
	platformAdmin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	tenantAdmin := &user.User{ID: "ta", Role: user.RoleAdmin, TenantID: otherTenant}
	defaultEditor := &user.User{ID: "de", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}
	tenantViewer := &user.User{ID: "tv", Role: user.RoleViewer, TenantID: otherTenant}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		user       *user.User
		wantStatus int
		wantProxy  bool // the request reached LiteLLM
	}{
		{"platform admin adds", http.MethodPost, "/api/v1/llm/models", `{"model_name":"m1","litellm_params":{"model":"openai/x"}}`, platformAdmin, http.StatusCreated, true},
		{"admin of another tenant cannot add", http.MethodPost, "/api/v1/llm/models", `{"model_name":"m1"}`, tenantAdmin, http.StatusForbidden, false},
		{"editor of the default tenant cannot add", http.MethodPost, "/api/v1/llm/models", `{"model_name":"m1"}`, defaultEditor, http.StatusForbidden, false},
		{"platform admin deletes", http.MethodDelete, "/api/v1/llm/models/m1", "", platformAdmin, http.StatusOK, true},
		{"admin of another tenant cannot delete", http.MethodDelete, "/api/v1/llm/models/m1", "", tenantAdmin, http.StatusForbidden, false},
		{"viewer of another tenant lists", http.MethodGet, "/api/v1/llm/models", "", tenantViewer, http.StatusOK, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var proxied atomic.Int32
			llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				proxied.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"data":[]}`))
					return
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer llmSrv.Close()
			r := newTestRouterWithLLM(&mockStore{}, service.NewPolicyService("headless-safe-sandbox", nil), llmSrv.URL)

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(middleware.ContextWithTestUser(req.Context(), tt.user))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if got := proxied.Load() > 0; got != tt.wantProxy {
				t.Fatalf("request reached LiteLLM = %v, want %v", got, tt.wantProxy)
			}
		})
	}
}
