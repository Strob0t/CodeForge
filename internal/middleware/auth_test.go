package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

const mwTestTenantID = "00000000-0000-0000-0000-000000000000"

func newTestAuthSvc() *service.AuthService {
	cfg := config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-for-middleware",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
	}
	// nil store is fine — middleware only calls ValidateAccessToken/ValidateAPIKey,
	// and for these tests we only test token parsing (no DB calls).
	return service.NewAuthService(nil, &cfg)
}

func newTestAuthSvcWithStore(ts *testStore) *service.AuthService {
	cfg := config.Auth{
		Enabled:            true,
		JWTSecret:          "test-secret-key-for-middleware",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
		DefaultAdminEmail:  "admin@mw.com",
		DefaultAdminPass:   "Adminpass123",
	}
	return service.NewAuthService(ts, &cfg)
}

// registerAndLoginMW registers a user and returns their access token.
func registerAndLoginMW(t *testing.T, svc *service.AuthService, email, password string) string {
	t.Helper()
	ctx := context.Background()

	_, err := svc.Register(ctx, &user.CreateRequest{
		Email:    email,
		Name:     "MW Test User",
		Password: password,
		Role:     user.RoleEditor,
		TenantID: mwTestTenantID,
	})
	if err != nil {
		t.Fatalf("register %s: %v", email, err)
	}

	resp, _, err := svc.Login(ctx, user.LoginRequest{
		Email:    email,
		Password: password,
	}, mwTestTenantID)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}

	return resp.AccessToken
}

// --- Existing tests (preserved) ---

func TestAuth_Disabled_InjectsDefaultAdmin(t *testing.T) {
	handler := middleware.Auth(nil, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := middleware.UserFromContext(r.Context())
		if u == nil {
			t.Fatal("expected default user in context")
		}
		if u.Role != "admin" {
			t.Errorf("role = %q, want admin", u.Role)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAuth_Enabled_NoHeader_Returns401(t *testing.T) {
	svc := newTestAuthSvc()
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// KI-55: starting the GitHub OAuth flow binds the state to the caller's
// tenant, so it needs a session; only GitHub's redirect to the callback
// arrives without one.
func TestAuth_GitHubOAuth_StartNeedsASession(t *testing.T) {
	svc := newTestAuthSvc()
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for path, want := range map[string]int{
		"/api/v1/auth/github":          http.StatusUnauthorized,
		"/api/v1/auth/github/callback": http.StatusOK,
	} {
		req := httptest.NewRequest(http.MethodPost, path, http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("path %s: status = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestAuth_PublicPath_NoAuthRequired(t *testing.T) {
	svc := newTestAuthSvc()
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/health", "/health/ready", "/api/v1/auth/login", "/api/v1/auth/refresh"} {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("path %s: status = %d, want 200", path, rec.Code)
		}
	}
}

func TestAuth_InvalidBearerToken_Returns401(t *testing.T) {
	svc := newTestAuthSvc()
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// --- New tests (Phase 3) ---

func TestAuth_ValidBearerToken_InjectsUser(t *testing.T) {
	ts := &testStore{}
	svc := newTestAuthSvcWithStore(ts)
	accessToken := registerAndLoginMW(t, svc, "valid@mw.com", "Password123")

	var gotUser *user.User
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = middleware.UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context")
	}
	if gotUser.Email != "valid@mw.com" {
		t.Errorf("email = %q, want valid@mw.com", gotUser.Email)
	}
	if gotUser.Role != user.RoleEditor {
		t.Errorf("role = %q, want editor", gotUser.Role)
	}
}

func TestAuth_MustChangePassword_NonExempt_403(t *testing.T) {
	ts := &testStore{}
	svc := newTestAuthSvcWithStore(ts)

	ctx := context.Background()
	_, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "mustchange@mw.com",
		Name:     "Must Change",
		Password: "Password123",
		Role:     user.RoleEditor,
		TenantID: mwTestTenantID,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Set MustChangePassword before login so the JWT includes the flag
	for i := range ts.users {
		if ts.users[i].Email == "mustchange@mw.com" {
			ts.users[i].MustChangePassword = true
		}
	}

	resp, _, err := svc.Login(ctx, user.LoginRequest{
		Email:    "mustchange@mw.com",
		Password: "Password123",
	}, mwTestTenantID)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestAuth_MustChangePassword_ExemptPath_200(t *testing.T) {
	ts := &testStore{}
	svc := newTestAuthSvcWithStore(ts)

	ctx := context.Background()
	_, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "exempt@mw.com",
		Name:     "Exempt User",
		Password: "Password123",
		Role:     user.RoleEditor,
		TenantID: mwTestTenantID,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Set MustChangePassword before login
	for i := range ts.users {
		if ts.users[i].Email == "exempt@mw.com" {
			ts.users[i].MustChangePassword = true
		}
	}

	resp, _, err := svc.Login(ctx, user.LoginRequest{
		Email:    "exempt@mw.com",
		Password: "Password123",
	}, mwTestTenantID)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// /api/v1/auth/change-password is exempt
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (exempt path)", rec.Code)
	}
}

// The WebSocket upgrade authenticates with a single-use ticket that the hub
// redeems (see internal/adapter/ws). The middleware must pass /ws through
// without authenticating anyone, and a JWT in the URL must never authenticate
// the request (CWE-598: tokens in URLs leak into logs and browser history).
func TestAuth_WebSocket_DelegatesToTicketAuth(t *testing.T) {
	ts := &testStore{}
	svc := newTestAuthSvcWithStore(ts)
	accessToken := registerAndLoginMW(t, svc, "ws@mw.com", "Password123")

	tests := []struct {
		name string
		url  string
	}{
		{"no credentials", "/ws"},
		{"valid JWT in URL", "/ws?token=" + accessToken},
		{"garbage token in URL", "/ws?token=garbage.token.here"},
		{"ticket", "/ws?ticket=00000000-0000-0000-0000-000000000001"},
		{"trailing slash", "/ws/?token=" + accessToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			var gotUser *user.User
			handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				gotUser = middleware.UserFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, tt.url, http.NoBody)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if !reached || rec.Code != http.StatusOK {
				t.Fatalf("status = %d, reached = %v; want the WS handler to decide", rec.Code, reached)
			}
			if gotUser != nil {
				t.Fatalf("user %q authenticated from the URL, want none", gotUser.Email)
			}
		})
	}
}

func TestAuth_APIKey_ValidKey_InjectsUser(t *testing.T) {
	ts := &testStore{}
	svc := newTestAuthSvcWithStore(ts)
	ctx := context.Background()

	u, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "apikey@mw.com",
		Name:     "API Key User",
		Password: "Password123",
		Role:     user.RoleEditor,
		TenantID: mwTestTenantID,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	keyResp, err := svc.CreateAPIKey(ctx, u.ID, user.CreateAPIKeyRequest{Name: "mw-key"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	var gotUser *user.User
	var gotKey *user.APIKey
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = middleware.UserFromContext(r.Context())
		gotKey = middleware.APIKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	req.Header.Set("X-API-Key", keyResp.PlainKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context")
	}
	if gotUser.ID != u.ID {
		t.Errorf("user id = %q, want %q", gotUser.ID, u.ID)
	}
	if gotKey == nil {
		t.Fatal("expected API key in context")
	}
	if gotKey.Name != "mw-key" {
		t.Errorf("api key name = %q, want mw-key", gotKey.Name)
	}
}

func TestAuth_InvalidAuthScheme_401(t *testing.T) {
	ts := &testStore{}
	svc := newTestAuthSvcWithStore(ts)

	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_InternalServiceKey_GrantsAdmin(t *testing.T) {
	ts := &testStore{}
	svc := newTestAuthSvcWithStore(ts)
	const internalKey = "cf-internal-secret-key-1234"

	var gotUser *user.User
	handler := middleware.Auth(svc, true, internalKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = middleware.UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	req.Header.Set("X-API-Key", internalKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context")
	}
	if gotUser.Email != "service@internal" {
		t.Errorf("email = %q, want service@internal", gotUser.Email)
	}
	if gotUser.Role != user.RoleAdmin {
		t.Errorf("role = %q, want admin", gotUser.Role)
	}
}

func TestAuth_WebhookPrefix_NoAuthRequired(t *testing.T) {
	svc := newTestAuthSvc()
	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (webhook prefix bypasses auth)", rec.Code)
	}
}

func TestAuth_APIKey_MustChangePassword_403(t *testing.T) {
	ts := &testStore{}
	svc := newTestAuthSvcWithStore(ts)
	ctx := context.Background()

	u, err := svc.Register(ctx, &user.CreateRequest{
		Email:    "mustchange-apikey@mw.com",
		Name:     "MustChange APIKey User",
		Password: "Password123",
		Role:     user.RoleEditor,
		TenantID: mwTestTenantID,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Set MustChangePassword on the user
	for i := range ts.users {
		if ts.users[i].ID == u.ID {
			ts.users[i].MustChangePassword = true
		}
	}

	keyResp, err := svc.CreateAPIKey(ctx, u.ID, user.CreateAPIKeyRequest{Name: "mcp-key"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	handler := middleware.Auth(svc, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	req.Header.Set("X-API-Key", keyResp.PlainKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
