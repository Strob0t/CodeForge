package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/service"
)

func TestStartGitHubOAuth_NotConfigured(t *testing.T) {
	h := &cfhttp.Handlers{
		Limits: &config.Limits{MaxRequestBodySize: 1 << 20},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github", http.NoBody)
	rec := httptest.NewRecorder()

	h.StartGitHubOAuth(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected %d, got %d: %s", http.StatusNotImplemented, rec.Code, rec.Body.String())
	}
}

func TestGitHubOAuthCallback_NotConfigured(t *testing.T) {
	h := &cfhttp.Handlers{
		Limits: &config.Limits{MaxRequestBodySize: 1 << 20},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github/callback?code=abc&state=xyz", http.NoBody)
	rec := httptest.NewRecorder()

	h.GitHubOAuthCallback(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected %d, got %d: %s", http.StatusNotImplemented, rec.Code, rec.Body.String())
	}
}

func newGitHubOAuthHandlers() *cfhttp.Handlers {
	store := &mockStore{}
	oauthSvc := service.NewGitHubOAuthService(
		service.GitHubOAuthConfig{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  "http://localhost:3000/api/v1/auth/github/callback",
			Scopes:       []string{"repo"},
		},
		store,
		[]byte("test-encryption-key-32bytes!!!!!"),
	)
	return &cfhttp.Handlers{
		GitHubOAuth: oauthSvc,
		Limits:      &config.Limits{MaxRequestBodySize: 1 << 20},
	}
}

func TestGitHubOAuthCallback_MissingParams(t *testing.T) {
	h := newGitHubOAuthHandlers()

	tests := []struct {
		name  string
		query string
	}{
		{"missing both", ""},
		{"missing code", "?state=abc"},
		{"missing state", "?code=abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github/callback"+tt.query, http.NoBody)
			rec := httptest.NewRecorder()

			h.GitHubOAuthCallback(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGitHubOAuthCallback_GitHubErrorParam(t *testing.T) {
	h := newGitHubOAuthHandlers()

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/github/callback?error=access_denied&state=abc&code=xyz", http.NoBody)
	rec := httptest.NewRecorder()

	h.GitHubOAuthCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] == "" {
		t.Fatal("expected error message in response body")
	}
}
