package user_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Platform admins (admins of the default tenant) manage what all tenants
// share, such as the LiteLLM models (KI-75). The frontend reads the flag from
// the user it gets on login, refresh and GET /auth/me.
func TestUser_IsPlatformAdmin(t *testing.T) {
	const otherTenant = "11111111-2222-3333-4444-555555555555"
	tests := []struct {
		name string
		u    user.User
		want bool
	}{
		{"admin of the default tenant", user.User{Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}, true},
		{"admin of another tenant", user.User{Role: user.RoleAdmin, TenantID: otherTenant}, false},
		{"editor of the default tenant", user.User{Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}, false},
		{"viewer of the default tenant", user.User{Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}, false},
		{"admin without tenant", user.User{Role: user.RoleAdmin}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.u.IsPlatformAdmin(); got != tt.want {
				t.Fatalf("IsPlatformAdmin() = %v, want %v", got, tt.want)
			}
			for _, v := range []any{tt.u, &tt.u, user.LoginResponse{User: tt.u}} {
				data, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				want := `"is_platform_admin":false`
				if tt.want {
					want = `"is_platform_admin":true`
				}
				if !strings.Contains(string(data), want) {
					t.Fatalf("%T JSON %s lacks %s", v, data, want)
				}
			}
		})
	}
}

func TestUser_JSONKeepsFieldsAndHidesSecrets(t *testing.T) {
	u := user.User{ID: "u1", Email: "a@b.c", Name: "A", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID,
		Enabled: true, PasswordHash: "bcrypt-HASH", FailedAttempts: 3}
	data, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"id":"u1"`, `"email":"a@b.c"`, `"role":"admin"`, `"tenant_id":"` + tenantctx.DefaultTenantID + `"`, `"enabled":true`, `"must_change_password":false`} {
		if !strings.Contains(string(data), field) {
			t.Errorf("JSON %s lacks %s", data, field)
		}
	}
	if strings.Contains(string(data), "bcrypt-HASH") || strings.Contains(string(data), "failed") {
		t.Errorf("JSON exposes internal fields: %s", data)
	}

	var back user.User
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != "u1" || back.Role != user.RoleAdmin || back.TenantID != tenantctx.DefaultTenantID {
		t.Errorf("round trip lost fields: %+v", back)
	}
}
