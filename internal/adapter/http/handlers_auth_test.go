package http_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// validPassword satisfies the complexity requirements: >=10 chars, uppercase, lowercase, digit.
const validPassword = "TestPass123"

// withUserContext injects a user into the request context, matching the key used by middleware.Auth.
func withUserContext(req *http.Request, u *user.User) *http.Request {
	return req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
}

// httpSetupAdmin performs the initial setup flow against the test router, returning
// the access token and the refresh cookie for further authenticated requests.
// Uses validPassword as the password for all setup calls.
func httpSetupAdmin(t *testing.T, router chi.Router, email string) (accessToken string, refreshCookie *http.Cookie) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":       email,
		"name":        "Test Admin",
		"password":    validPassword,
		"setup_token": testSetupToken(t, router),
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp user.LoginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("setup: decode response: %v", err)
	}
	accessToken = resp.AccessToken

	for _, c := range w.Result().Cookies() {
		if c.Name == "codeforge_refresh" {
			refreshCookie = c
			break
		}
	}
	return accessToken, refreshCookie
}

// --- Public Endpoint Tests ---

func TestHandleLogin_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	// Set up admin first.
	httpSetupAdmin(t, r, "login@test.com")

	// Now login with the same credentials.
	body, _ := json.Marshal(user.LoginRequest{
		Email:    "login@test.com",
		Password: validPassword,
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp user.LoginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken == "" {
		t.Fatal("expected non-empty access_token")
	}

	// Check refresh cookie is set.
	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "codeforge_refresh" {
			found = true
			if !c.HttpOnly {
				t.Fatal("refresh cookie must be httpOnly")
			}
		}
	}
	if !found {
		t.Fatal("expected codeforge_refresh cookie")
	}
}

// S10-A review: a disabled tenant's users are refused on login and refresh
// with 403 tenant is disabled; no token or cookie is issued.
func TestHandleAuth_DisabledTenant(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	_, refreshCookie := httpSetupAdmin(t, r, "disabled-tenant@test.com")
	if refreshCookie == nil {
		t.Fatal("no refresh cookie from setup")
	}
	store.mu.Lock()
	store.tenants = append(store.tenants, tenant.Tenant{ID: tenantctx.DefaultTenantID, Name: "Default", Slug: "default", Enabled: false})
	store.mu.Unlock()

	body, _ := json.Marshal(user.LoginRequest{Email: "disabled-tenant@test.com", Password: validPassword})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "tenant is disabled") {
		t.Fatalf("login: status %d (%s), want 403 tenant is disabled", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatalf("login set cookies: %v", w.Result().Cookies())
	}

	req = httptest.NewRequest("POST", "/api/v1/auth/refresh", http.NoBody)
	req.AddCookie(refreshCookie)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "tenant is disabled") {
		t.Fatalf("refresh: status %d (%s), want 403 tenant is disabled", w.Code, w.Body.String())
	}
}

func TestHandleLogin_InvalidCredentials(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "login@test.com")

	body, _ := json.Marshal(user.LoginRequest{
		Email:    "login@test.com",
		Password: "WrongPassword99",
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLogin_BadBody(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader([]byte("{invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRefresh_Success(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	_, refreshCookie := httpSetupAdmin(t, r, "refresh@test.com")

	if refreshCookie == nil {
		t.Fatal("no refresh cookie from setup")
	}

	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", http.NoBody)
	req.AddCookie(refreshCookie)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp user.LoginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken == "" {
		t.Fatal("expected new access_token")
	}
}

func TestHandleRefresh_NoCookie(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetupStatus_NeedsSetup(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/auth/setup-status", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["needs_setup"] != true {
		t.Fatalf("expected needs_setup=true, got %v", resp["needs_setup"])
	}
}

func TestHandleSetupStatus_AlreadySetup(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "admin@test.com")

	req := httptest.NewRequest("GET", "/api/v1/auth/setup-status", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["needs_setup"] != false {
		t.Fatalf("expected needs_setup=false, got %v", resp["needs_setup"])
	}
}

func TestHandleInitialSetup_Success(t *testing.T) {
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{
		"email":       "admin@test.com",
		"name":        "Admin",
		"password":    validPassword,
		"setup_token": testSetupToken(t, r),
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp user.LoginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken == "" {
		t.Fatal("expected access_token in setup response")
	}
	if resp.User.Email != "admin@test.com" {
		t.Fatalf("expected email admin@test.com, got %q", resp.User.Email)
	}
}

func TestHandleInitialSetup_AlreadyDone(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "admin@test.com")

	// Try setup again.
	body, _ := json.Marshal(map[string]string{
		"email":    "admin2@test.com",
		"name":     "Admin 2",
		"password": validPassword,
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// KI-119: without the one-time setup token nobody becomes the first admin.
func TestHandleInitialSetup_RequiresSetupToken(t *testing.T) {
	tests := []struct {
		name  string
		token func(right string) (string, bool) // the value and whether to send the field
	}{
		{"missing", func(string) (string, bool) { return "", false }},
		{"empty", func(string) (string, bool) { return "", true }},
		{"wrong", func(right string) (string, bool) { return strings.Repeat("0", len(right)), true }},
		{"truncated", func(right string) (string, bool) { return right[:len(right)/2], true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{}
			r := newTestRouterWithStore(store)
			fields := map[string]string{"email": "admin@test.com", "name": "Admin", "password": validPassword}
			if v, send := tt.token(testSetupToken(t, r)); send {
				fields["setup_token"] = v
			}
			body, _ := json.Marshal(fields)
			req := httptest.NewRequest("POST", "/api/v1/auth/setup", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
			}
			if len(store.users) != 0 {
				t.Fatalf("no user may be created, got %d", len(store.users))
			}
			if c := w.Result().Cookies(); len(c) != 0 {
				t.Fatalf("no session may be issued, got cookies %v", c)
			}
		})
	}
}

func TestHandleRequestPasswordReset(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "reset@test.com")

	// Request password reset — always returns 200 regardless of email existence.
	body, _ := json.Marshal(map[string]string{"email": "reset@test.com"})
	req := httptest.NewRequest("POST", "/api/v1/auth/forgot-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Also with unknown email — still 200.
	body, _ = json.Marshal(map[string]string{"email": "unknown@test.com"})
	req = httptest.NewRequest("POST", "/api/v1/auth/forgot-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unknown email, got %d", w.Code)
	}
}

func TestHandleRequestPasswordReset_TokenNotLoggedInPlaintext(t *testing.T) {
	// Capture slog output to verify the reset token is NOT logged in full.
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "tokenlog@test.com")

	body, _ := json.Marshal(map[string]string{"email": "tokenlog@test.com"})
	req := httptest.NewRequest("POST", "/api/v1/auth/forgot-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	logged := buf.String()

	// The log should contain "token_prefix" (redacted), never a bare "token=" with full value.
	if strings.Contains(logged, "token_prefix") {
		// Good: redacted version is present.
		// Verify it ends with "..." (truncated).
		if !strings.Contains(logged, "...") {
			t.Fatal("token_prefix in log should end with '...' indicating truncation")
		}
	}

	// Verify there is no log attribute named exactly "token" (which would be the full value).
	// slog text format uses "token=" for key.
	// We check that "token=" only appears as "token_prefix=", never as a bare "token=".
	if strings.Contains(logged, " token=") {
		t.Fatal("full reset token must NOT appear in logs; use token_prefix instead")
	}
}

func TestHandleConfirmPasswordReset(t *testing.T) {
	// ConfirmPasswordReset with invalid token → 400.
	r := newTestRouter()

	body, _ := json.Marshal(map[string]string{
		"token":        "invalid-token-value",
		"new_password": validPassword,
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Authenticated Endpoint Tests ---

func TestHandleGetCurrentUser(t *testing.T) {
	r := newTestRouter()

	u := &user.User{
		ID:    "user-1",
		Email: "me@test.com",
		Name:  "Test User",
		Role:  user.RoleEditor,
	}
	req := httptest.NewRequest("GET", "/api/v1/auth/me", http.NoBody)
	req = withUserContext(req, u)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp user.User
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Email != "me@test.com" {
		t.Fatalf("expected email me@test.com, got %q", resp.Email)
	}
}

// TestHandleGetCurrentUser_PlatformAdminFlag: the frontend shows actions on
// what all tenants share (models, provider credentials) only to platform
// admins, read from is_platform_admin (KI-75).
func TestHandleGetCurrentUser_PlatformAdminFlag(t *testing.T) {
	tests := []struct {
		name string
		u    *user.User
		want bool
	}{
		{"platform admin", &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}, true},
		{"admin of another tenant", &user.User{ID: "ta", Role: user.RoleAdmin, TenantID: "11111111-2222-3333-4444-555555555555"}, false},
		{"editor of the default tenant", &user.User{ID: "de", Role: user.RoleEditor, TenantID: tenantctx.DefaultTenantID}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := withUserContext(httptest.NewRequest("GET", "/api/v1/auth/me", http.NoBody), tt.u)
			w := httptest.NewRecorder()
			newTestRouter().ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			var resp struct {
				ID              string `json:"id"`
				IsPlatformAdmin *bool  `json:"is_platform_admin"`
			}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatal(err)
			}
			if resp.ID != tt.u.ID || resp.IsPlatformAdmin == nil || *resp.IsPlatformAdmin != tt.want {
				t.Fatalf("got id %q is_platform_admin %v, want %v", resp.ID, resp.IsPlatformAdmin, tt.want)
			}
		})
	}
}

func TestHandleGetCurrentUser_NoAuth(t *testing.T) {
	r := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/auth/me", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleLogout(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	accessToken, _ := httpSetupAdmin(t, r, "logout@test.com")

	u := &user.User{
		ID:    store.users[0].ID,
		Email: "logout@test.com",
		Name:  "Test Admin",
		Role:  user.RoleAdmin,
	}
	req := httptest.NewRequest("POST", "/api/v1/auth/logout", http.NoBody)
	req = withUserContext(req, u)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check that the refresh cookie is cleared.
	for _, c := range w.Result().Cookies() {
		if c.Name == "codeforge_refresh" && c.MaxAge != -1 {
			t.Fatal("expected refresh cookie to be cleared (MaxAge=-1)")
		}
	}
}

func TestHandleChangePassword(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "change@test.com")

	u := &user.User{
		ID:    store.users[0].ID,
		Email: "change@test.com",
		Name:  "Test Admin",
		Role:  user.RoleAdmin,
	}

	body, _ := json.Marshal(user.ChangePasswordRequest{
		OldPassword: validPassword,
		NewPassword: "NewSecurePass1",
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserContext(req, u)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateAPIKey(t *testing.T) {
	r := newTestRouter()

	u := &user.User{
		ID:    "user-1",
		Email: "apikey@test.com",
		Name:  "Test User",
		Role:  user.RoleEditor,
	}

	body, _ := json.Marshal(user.CreateAPIKeyRequest{
		Name: "my-key",
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/api-keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserContext(req, u)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp user.CreateAPIKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.PlainKey == "" {
		t.Fatal("expected non-empty plain_key")
	}
	if len(resp.PlainKey) < 4 || resp.PlainKey[:4] != "cfk_" {
		t.Fatalf("expected key with cfk_ prefix, got %q", resp.PlainKey[:10])
	}
}

func TestHandleListAPIKeys_Empty(t *testing.T) {
	r := newTestRouter()

	u := &user.User{
		ID:    "user-1",
		Email: "apikey@test.com",
		Name:  "Test User",
		Role:  user.RoleEditor,
	}

	req := httptest.NewRequest("GET", "/api/v1/auth/api-keys", http.NoBody)
	req = withUserContext(req, u)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var keys []user.APIKey
	if err := json.NewDecoder(w.Body).Decode(&keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected empty list, got %d keys", len(keys))
	}
}

func TestHandleDeleteAPIKey(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	u := &user.User{
		ID:    "user-1",
		Email: "apikey@test.com",
		Name:  "Test User",
		Role:  user.RoleEditor,
	}

	// Create a key first.
	body, _ := json.Marshal(user.CreateAPIKeyRequest{Name: "delete-me"})
	req := httptest.NewRequest("POST", "/api/v1/auth/api-keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserContext(req, u)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create key: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created user.CreateAPIKeyResponse
	_ = json.NewDecoder(w.Body).Decode(&created)

	// Delete it.
	req = httptest.NewRequest("DELETE", "/api/v1/auth/api-keys/"+created.APIKey.ID, http.NoBody)
	req = withUserContext(req, u)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Admin Endpoint Tests ---

func TestHandleListUsers(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "admin@test.com")

	admin := &user.User{
		ID:       store.users[0].ID,
		Email:    "admin@test.com",
		Name:     "Admin",
		Role:     user.RoleAdmin,
		TenantID: middleware.DefaultTenantID,
	}

	req := httptest.NewRequest("GET", "/api/v1/users/", http.NoBody)
	req = withUserContext(req, admin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var users []user.User
	if err := json.NewDecoder(w.Body).Decode(&users); err != nil {
		t.Fatal(err)
	}
	if len(users) == 0 {
		t.Fatal("expected at least one user")
	}
}

func TestHandleCreateUser(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)

	admin := &user.User{
		ID:       "admin-1",
		Email:    "admin@test.com",
		Name:     "Admin",
		Role:     user.RoleAdmin,
		TenantID: middleware.DefaultTenantID,
	}

	body, _ := json.Marshal(user.CreateRequest{
		Email:    "new@test.com",
		Name:     "New User",
		Password: validPassword,
		Role:     user.RoleViewer,
	})
	req := httptest.NewRequest("POST", "/api/v1/users/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserContext(req, admin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created user.User
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Email != "new@test.com" {
		t.Fatalf("expected email new@test.com, got %q", created.Email)
	}
}

func TestHandleUpdateUser(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "admin@test.com")

	admin := &user.User{
		ID:       store.users[0].ID,
		Email:    "admin@test.com",
		Name:     "Admin",
		Role:     user.RoleAdmin,
		TenantID: middleware.DefaultTenantID,
	}

	userID := store.users[0].ID
	body, _ := json.Marshal(user.UpdateRequest{
		Name: "Updated Name",
	})
	req := httptest.NewRequest("PUT", "/api/v1/users/"+userID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserContext(req, admin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated user.User
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Updated Name" {
		t.Fatalf("expected name 'Updated Name', got %q", updated.Name)
	}
}

func TestHandleDeleteUser(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "admin@test.com")

	admin := &user.User{
		ID:       store.users[0].ID,
		Email:    "admin@test.com",
		Name:     "Admin",
		Role:     user.RoleAdmin,
		TenantID: middleware.DefaultTenantID,
	}

	// Create a second user to delete.
	body, _ := json.Marshal(user.CreateRequest{
		Email:    "delete@test.com",
		Name:     "Delete Me",
		Password: validPassword,
		Role:     user.RoleViewer,
	})
	req := httptest.NewRequest("POST", "/api/v1/users/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserContext(req, admin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create user: expected 201, got %d", w.Code)
	}

	var created user.User
	_ = json.NewDecoder(w.Body).Decode(&created)

	// Delete the created user.
	req = httptest.NewRequest("DELETE", "/api/v1/users/"+created.ID, http.NoBody)
	req = withUserContext(req, admin)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdminForcePasswordChange(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterWithStore(store)
	httpSetupAdmin(t, r, "admin@test.com")

	admin := &user.User{
		ID:       store.users[0].ID,
		Email:    "admin@test.com",
		Name:     "Admin",
		Role:     user.RoleAdmin,
		TenantID: middleware.DefaultTenantID,
	}

	userID := store.users[0].ID
	body, _ := json.Marshal(map[string]string{
		"new_password": "ForcedNewPass1",
	})
	req := httptest.NewRequest("POST", "/api/v1/users/"+userID+"/force-password-change", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUserContext(req, admin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
