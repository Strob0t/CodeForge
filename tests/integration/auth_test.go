//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// loginResponse matches the JSON returned by POST /api/v1/auth/login.
type loginResponse struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"user"`
}

// adminPassword is the password of every admin created by setupAdmin.
const adminPassword = "Password123"

// setupAdmin creates the first (admin) user of the tenant via the initial setup
// endpoint and returns the login response along with the refresh cookie. It
// arms the setup token for the tenant first, as the Core does on a start
// without users (KI-119); the tests of this package do not run in parallel.
func setupAdmin(t *testing.T, tenantID, email string) (loginResponse, []*http.Cookie) {
	t.Helper()
	token, err := testAuth.PrepareSetupToken(context.Background(), tenantID)
	if err != nil || token == "" {
		t.Fatalf("prepare setup token: %q, %v", token, err)
	}
	body, _ := json.Marshal(map[string]string{
		"email":       email,
		"name":        "Test Admin",
		"password":    adminPassword,
		"setup_token": token,
	})
	resp := doRequest(t, tenantRequest(t, http.MethodPost, "/api/v1/auth/setup", tenantID, "", body))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d", resp.StatusCode)
	}

	var lr loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	return lr, resp.Cookies()
}

// refreshCookie extracts the codeforge_refresh cookie from a cookie slice.
func refreshCookie(cookies []*http.Cookie) *http.Cookie {
	for _, c := range cookies {
		if c.Name == "codeforge_refresh" {
			return c
		}
	}
	return nil
}

// --- Tests ---

func TestIntegration_AuthFlow_LoginProtectedLogout(t *testing.T) {
	tenantID := newTestTenant(t)
	lr, _ := setupAdmin(t, tenantID, "auth-flow@test.com")

	// GET /auth/me with valid token → 200
	resp := doRequest(t, tenantRequest(t, "GET", "/api/v1/auth/me", tenantID, lr.AccessToken, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /auth/me: expected 200, got %d", resp.StatusCode)
	}

	// POST /auth/logout
	resp = doRequest(t, tenantRequest(t, "POST", "/api/v1/auth/logout", tenantID, lr.AccessToken, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: expected 200, got %d", resp.StatusCode)
	}

	// GET /auth/me after logout → 401 (token revoked)
	resp = doRequest(t, tenantRequest(t, "GET", "/api/v1/auth/me", tenantID, lr.AccessToken, nil))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /auth/me after logout: expected 401, got %d", resp.StatusCode)
	}
}

func TestIntegration_AuthFlow_TokenRefresh(t *testing.T) {
	tenantID := newTestTenant(t)
	lr, cookies := setupAdmin(t, tenantID, "refresh@test.com")
	rc := refreshCookie(cookies)
	if rc == nil {
		t.Fatal("expected refresh cookie after setup")
	}

	// POST /auth/refresh with refresh cookie → new tokens
	req := tenantRequest(t, "POST", "/api/v1/auth/refresh", tenantID, "", nil)
	req.AddCookie(rc)
	resp := doRequest(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: expected 200, got %d", resp.StatusCode)
	}

	var newLR loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&newLR); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if newLR.AccessToken == "" {
		t.Fatal("expected non-empty new access token")
	}
	if newLR.AccessToken == lr.AccessToken {
		t.Error("new access token should differ from old")
	}

	// New token should work for /auth/me
	resp2 := doRequest(t, tenantRequest(t, "GET", "/api/v1/auth/me", tenantID, newLR.AccessToken, nil))
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /auth/me with new token: expected 200, got %d", resp2.StatusCode)
	}

	// Old refresh cookie should be rejected
	req = tenantRequest(t, "POST", "/api/v1/auth/refresh", tenantID, "", nil)
	req.AddCookie(rc)
	resp3 := doRequest(t, req)
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old refresh cookie: expected 401, got %d", resp3.StatusCode)
	}
}

func TestIntegration_AuthFlow_PasswordReset(t *testing.T) {
	tenantID := newTestTenant(t)
	setupAdmin(t, tenantID, "resetpw@test.com")

	// POST /auth/forgot-password → always 200
	body, _ := json.Marshal(map[string]string{"email": "resetpw@test.com"})
	resp := doRequest(t, tenantRequest(t, "POST", "/api/v1/auth/forgot-password", tenantID, "", body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password: expected 200, got %d", resp.StatusCode)
	}

	// POST /auth/reset-password with invalid token → 400
	resetBody, _ := json.Marshal(map[string]string{"token": "invalid-token", "new_password": "NewPass12345"})
	resp2 := doRequest(t, tenantRequest(t, "POST", "/api/v1/auth/reset-password", tenantID, "", resetBody))
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("reset-password with invalid token: expected 400, got %d", resp2.StatusCode)
	}

	// Unknown email still returns 200 (enumeration prevention)
	unknownBody, _ := json.Marshal(map[string]string{"email": "nobody@test.com"})
	resp3 := doRequest(t, tenantRequest(t, "POST", "/api/v1/auth/forgot-password", tenantID, "", unknownBody))
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password unknown: expected 200, got %d", resp3.StatusCode)
	}
}

func TestIntegration_AuthFlow_APIKey(t *testing.T) {
	tenantID := newTestTenant(t)
	lr, _ := setupAdmin(t, tenantID, "apikey@test.com")

	// POST /auth/api-keys → 201
	createBody, _ := json.Marshal(map[string]string{"name": "ci-key"})
	resp := doRequest(t, tenantRequest(t, "POST", "/api/v1/auth/api-keys", tenantID, lr.AccessToken, createBody))
	var createResp struct {
		PlainKey string `json:"plain_key"`
		APIKey   struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create api key: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create api key: expected 201, got %d", resp.StatusCode)
	}
	if createResp.PlainKey == "" {
		t.Fatal("expected non-empty plain key")
	}

	// GET /auth/api-keys → 1 key
	resp2 := doRequest(t, tenantRequest(t, "GET", "/api/v1/auth/api-keys", tenantID, lr.AccessToken, nil))
	var keys []map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&keys); err != nil {
		t.Fatalf("decode list api keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 api key, got %d", len(keys))
	}

	// DELETE /auth/api-keys/{id} → 204
	resp3 := doRequest(t, tenantRequest(t, "DELETE", "/api/v1/auth/api-keys/"+createResp.APIKey.ID, tenantID, lr.AccessToken, nil))
	if resp3.StatusCode != http.StatusNoContent {
		t.Fatalf("delete api key: expected 204, got %d", resp3.StatusCode)
	}

	// GET /auth/api-keys → 0 keys
	resp4 := doRequest(t, tenantRequest(t, "GET", "/api/v1/auth/api-keys", tenantID, lr.AccessToken, nil))
	var keysAfter []map[string]any
	if err := json.NewDecoder(resp4.Body).Decode(&keysAfter); err != nil {
		t.Fatalf("decode list after delete: %v", err)
	}
	if len(keysAfter) != 0 {
		t.Fatalf("expected 0 api keys after delete, got %d", len(keysAfter))
	}
}
