package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// S3-F review C4: only platform admins (admins of the default tenant) may
// adopt from workspace.adopt_roots; another tenant's admin may not.
func TestIsPlatformAdmin(t *testing.T) {
	tests := []struct {
		name string
		user *user.User
		want bool
	}{
		{name: "no user"},
		{name: "default tenant admin", user: &user.User{Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}, want: true},
		{name: "another tenant's admin", user: &user.User{Role: user.RoleAdmin, TenantID: "11111111-1111-1111-1111-111111111111"}},
		{name: "default tenant editor", user: &user.User{Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/adopt", http.NoBody)
			if tc.user != nil {
				r = r.WithContext(middleware.ContextWithTestUser(r.Context(), tc.user))
			}
			if got := isPlatformAdmin(r); got != tc.want {
				t.Fatalf("isPlatformAdmin = %v, want %v", got, tc.want)
			}
		})
	}
}
