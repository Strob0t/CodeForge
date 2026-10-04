package user

import "testing"

func TestUser_CreateRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateRequest
		wantErr string
	}{
		{name: "valid", req: CreateRequest{Email: "a@b.com", Name: "A", Password: "Abcdef1234", Role: RoleAdmin}},
		{name: "missing email", req: CreateRequest{Name: "A", Password: "Abcdef1234", Role: RoleAdmin}, wantErr: "email is required"},
		{name: "invalid email", req: CreateRequest{Email: "bad", Name: "A", Password: "Abcdef1234", Role: RoleAdmin}, wantErr: "invalid email format"},
		{name: "missing name", req: CreateRequest{Email: "a@b.com", Password: "Abcdef1234", Role: RoleAdmin}, wantErr: "name is required"},
		{name: "missing password", req: CreateRequest{Email: "a@b.com", Name: "A", Role: RoleAdmin}, wantErr: "password is required"},
		{name: "short password", req: CreateRequest{Email: "a@b.com", Name: "A", Password: "short", Role: RoleAdmin}, wantErr: "password complexity: password must be at least 10 characters"},
		{name: "missing uppercase", req: CreateRequest{Email: "a@b.com", Name: "A", Password: "abcdefg123", Role: RoleAdmin}, wantErr: "password complexity: password must contain at least one uppercase letter"},
		{name: "invalid role", req: CreateRequest{Email: "a@b.com", Name: "A", Password: "Abcdef1234", Role: "superadmin"}, wantErr: "invalid role: must be admin, editor, or viewer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if got := err.Error(); got != tt.wantErr {
				t.Fatalf("error = %q, want %q", got, tt.wantErr)
			}
		})
	}
}

func TestLoginRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     LoginRequest
		wantErr string
	}{
		{name: "valid", req: LoginRequest{Email: "a@b.com", Password: "secret"}},
		{name: "missing email", req: LoginRequest{Password: "secret"}, wantErr: "email is required"},
		{name: "missing password", req: LoginRequest{Email: "a@b.com"}, wantErr: "password is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if got := err.Error(); got != tt.wantErr {
				t.Fatalf("error = %q, want %q", got, tt.wantErr)
			}
		})
	}
}

func TestCreateAPIKeyRequest_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		req := CreateAPIKeyRequest{Name: "ci-key"}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		req := CreateAPIKeyRequest{}
		err := req.Validate()
		if err == nil || err.Error() != "name is required" {
			t.Fatalf("expected 'name is required', got %v", err)
		}
	})
}

// KI-89 review: only the two synthetic identities count as accountless; any
// other ID names an account (which may be gone).
func TestIsAccountless(t *testing.T) {
	for id, want := range map[string]bool{
		AuthDisabledUserID:                     true,
		InternalServiceUserID:                  true,
		"00000000-0000-0000-0000-000000000002": false,
		"3f2b9c1e-7d4a-4e8b-9a6f-1c2d3e4f5a6b": false,
		"":                                     false,
		" " + AuthDisabledUserID:               false,
	} {
		if got := IsAccountless(id); got != want {
			t.Errorf("IsAccountless(%q) = %v, want %v", id, got, want)
		}
	}
}
