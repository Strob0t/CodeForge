//go:build integration

package integration_test

import (
	"bytes"
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

// setupAdmin creates an admin user via the initial setup endpoint and returns
// the login response along with the refresh cookie.
func setupAdmin(t *testing.T, email, password string) (loginResponse, []*http.Cookie) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"name":     "Test Admin",
		"password": password,
	})
	resp, err := http.Post(testAuthServer.URL+"/api/v1/auth/setup", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("setup admin: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d", resp.StatusCode)
	}

	var lr loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	return lr, resp.Cookies()
}

// login performs a POST /api/v1/auth/login and returns the response and cookies.
func login(t *testing.T, email, password string) (loginResponse, []*http.Cookie) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	resp, err := http.Post(testAuthServer.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", resp.StatusCode)
	}

	var lr loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return lr, resp.Cookies()
}

// authedRequest creates an HTTP request with the Bearer token set.
func authedRequest(method, url, token string, body []byte) *http.Request {
	var r *http.Request
	if body != nil {
		r, _ = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		r, _ = http.NewRequest(method, url, http.NoBody)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	return r
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
	cleanDB(testPool)

	lr, _ := setupAdmin(t, "auth-flow@test.com", "Password123")

	// GET /auth/me with valid token → 200
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req := authedRequest("GET", testAuthServer.URL+"/api/v1/auth/me", lr.AccessToken, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /auth/me: expected 200, got %d", resp.StatusCode)
	}

	// POST /auth/logout
	req = authedRequest("POST", testAuthServer.URL+"/api/v1/auth/logout", lr.AccessToken, nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /auth/logout: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: expected 200, got %d", resp.StatusCode)
	}

	// GET /auth/me after logout → 401 (token revoked)
	req = authedRequest("GET", testAuthServer.URL+"/api/v1/auth/me", lr.AccessToken, nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /auth/me after logout: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /auth/me after logout: expected 401, got %d", resp.StatusCode)
	}
}

func TestIntegration_AuthFlow_TokenRefresh(t *testing.T) {
	cleanDB(testPool)

	lr, cookies := setupAdmin(t, "refresh@test.com", "Password123")
	rc := refreshCookie(cookies)
	if rc == nil {
		t.Fatal("expected refresh cookie after setup")
	}

	// POST /auth/refresh with refresh cookie → new tokens
	req, _ := http.NewRequest("POST", testAuthServer.URL+"/api/v1/auth/refresh", http.NoBody)
	req.AddCookie(rc)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /auth/refresh: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

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
	req = authedRequest("GET", testAuthServer.URL+"/api/v1/auth/me", newLR.AccessToken, nil)
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /auth/me with new token: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /auth/me with new token: expected 200, got %d", resp2.StatusCode)
	}

	// Old refresh cookie should be rejected
	req, _ = http.NewRequest("POST", testAuthServer.URL+"/api/v1/auth/refresh", http.NoBody)
	req.AddCookie(rc)
	resp3, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /auth/refresh with old cookie: %v", err)
	}
	_ = resp3.Body.Close()
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old refresh cookie: expected 401, got %d", resp3.StatusCode)
	}
}

func TestIntegration_AuthFlow_PasswordReset(t *testing.T) {
	cleanDB(testPool)

	setupAdmin(t, "resetpw@test.com", "Password123")

	// POST /auth/forgot-password → always 200
	body, _ := json.Marshal(map[string]string{"email": "resetpw@test.com"})
	resp, err := http.Post(testAuthServer.URL+"/api/v1/auth/forgot-password", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("forgot-password: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password: expected 200, got %d", resp.StatusCode)
	}

	// POST /auth/reset-password with invalid token → 400
	resetBody, _ := json.Marshal(map[string]string{"token": "invalid-token", "new_password": "NewPass12345"})
	resp2, err := http.Post(testAuthServer.URL+"/api/v1/auth/reset-password", "application/json", bytes.NewReader(resetBody))
	if err != nil {
		t.Fatalf("reset-password: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("reset-password with invalid token: expected 400, got %d", resp2.StatusCode)
	}

	// Unknown email still returns 200 (enumeration prevention)
	unknownBody, _ := json.Marshal(map[string]string{"email": "nobody@test.com"})
	resp3, err := http.Post(testAuthServer.URL+"/api/v1/auth/forgot-password", "application/json", bytes.NewReader(unknownBody))
	if err != nil {
		t.Fatalf("forgot-password unknown: %v", err)
	}
	_ = resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password unknown: expected 200, got %d", resp3.StatusCode)
	}
}

func TestIntegration_AuthFlow_APIKey(t *testing.T) {
	cleanDB(testPool)

	lr, _ := setupAdmin(t, "apikey@test.com", "Password123")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// POST /auth/api-keys → 201
	createBody, _ := json.Marshal(map[string]string{"name": "ci-key"})
	req := authedRequest("POST", testAuthServer.URL+"/api/v1/auth/api-keys", lr.AccessToken, createBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
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
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create api key: expected 201, got %d", resp.StatusCode)
	}
	if createResp.PlainKey == "" {
		t.Fatal("expected non-empty plain key")
	}

	// GET /auth/api-keys → 1 key
	req = authedRequest("GET", testAuthServer.URL+"/api/v1/auth/api-keys", lr.AccessToken, nil)
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatalf("list api keys: %v", err)
	}
	var keys []map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&keys); err != nil {
		t.Fatalf("decode list api keys: %v", err)
	}
	_ = resp2.Body.Close()
	if len(keys) != 1 {
		t.Fatalf("expected 1 api key, got %d", len(keys))
	}

	// DELETE /auth/api-keys/{id} → 204
	req = authedRequest("DELETE", testAuthServer.URL+"/api/v1/auth/api-keys/"+createResp.APIKey.ID, lr.AccessToken, nil)
	resp3, err := client.Do(req)
	if err != nil {
		t.Fatalf("delete api key: %v", err)
	}
	_ = resp3.Body.Close()
	if resp3.StatusCode != http.StatusNoContent {
		t.Fatalf("delete api key: expected 204, got %d", resp3.StatusCode)
	}

	// GET /auth/api-keys → 0 keys
	req = authedRequest("GET", testAuthServer.URL+"/api/v1/auth/api-keys", lr.AccessToken, nil)
	resp4, err := client.Do(req)
	if err != nil {
		t.Fatalf("list api keys after delete: %v", err)
	}
	var keysAfter []map[string]any
	if err := json.NewDecoder(resp4.Body).Decode(&keysAfter); err != nil {
		t.Fatalf("decode list after delete: %v", err)
	}
	_ = resp4.Body.Close()
	if len(keysAfter) != 0 {
		t.Fatalf("expected 0 api keys after delete, got %d", len(keysAfter))
	}
}
