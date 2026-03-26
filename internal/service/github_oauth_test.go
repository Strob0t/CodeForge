package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
	"github.com/Strob0t/CodeForge/internal/service"
)

// --- Mock Store for OAuth tests ---

type oauthMockStore struct {
	runtimeMockStore
	states   map[string]*vcsaccount.OAuthState
	accounts []*vcsaccount.VCSAccount
}

func newOAuthMockStore() *oauthMockStore {
	return &oauthMockStore{
		states: make(map[string]*vcsaccount.OAuthState),
	}
}

func (m *oauthMockStore) CreateOAuthState(_ context.Context, state *vcsaccount.OAuthState) error {
	m.states[state.State] = state
	return nil
}

func (m *oauthMockStore) GetOAuthState(_ context.Context, stateToken string) (*vcsaccount.OAuthState, error) {
	st, ok := m.states[stateToken]
	if !ok {
		return nil, errors.New("not found")
	}
	if time.Now().After(st.ExpiresAt) {
		return nil, errors.New("expired")
	}
	return st, nil
}

func (m *oauthMockStore) DeleteOAuthState(_ context.Context, stateToken string) error {
	delete(m.states, stateToken)
	return nil
}

func (m *oauthMockStore) DeleteExpiredOAuthStates(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *oauthMockStore) CreateVCSAccount(_ context.Context, a *vcsaccount.VCSAccount) (*vcsaccount.VCSAccount, error) {
	a.ID = "acc-test-123"
	m.accounts = append(m.accounts, a)
	return a, nil
}

// --- Tests ---

func TestGitHubOAuth_AuthorizeURL(t *testing.T) {
	store := newOAuthMockStore()
	svc := service.NewGitHubOAuthService(service.GitHubOAuthConfig{
		ClientID:    "test-client-id",
		RedirectURI: "http://localhost:3000/callback",
		Scopes:      []string{"repo", "read:user"},
	}, store, []byte("0123456789abcdef0123456789abcdef"))

	authURL, err := svc.AuthorizeURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizeURL: %v", err)
	}

	if !strings.Contains(authURL, "https://github.com/login/oauth/authorize") {
		t.Errorf("expected GitHub authorize URL, got %q", authURL)
	}
	if !strings.Contains(authURL, "client_id=test-client-id") {
		t.Errorf("expected client_id in URL, got %q", authURL)
	}
	if !strings.Contains(authURL, "scope=repo+read") {
		t.Errorf("expected scope in URL, got %q", authURL)
	}

	// Verify state was stored.
	if len(store.states) != 1 {
		t.Fatalf("expected 1 stored state, got %d", len(store.states))
	}
}

func TestGitHubOAuth_HandleCallback_Success(t *testing.T) {
	// Mock GitHub token endpoint.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"access_token": "gho_test_token_12345",
			"token_type":   "bearer",
			"scope":        "repo,read:user",
		})
	}))
	defer tokenSrv.Close()

	store := newOAuthMockStore()
	encKey, err := vcsaccount.DeriveKey("test-jwt-secret", nil, "codeforge/vcsaccount/v1")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	svc := service.NewGitHubOAuthService(service.GitHubOAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURI:  "http://localhost:3000/callback",
		Scopes:       []string{"repo"},
	}, store, encKey)

	// Override the httpClient and token URL to use the test server.
	svc.SetHTTPClient(tokenSrv.Client())
	svc.SetTokenURL(tokenSrv.URL)

	// Pre-store a valid state.
	state, err := vcsaccount.NewOAuthState("github", "tenant-1")
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}
	store.states[state.State] = state

	account, err := svc.HandleCallback(context.Background(), "test-auth-code", state.State)
	if err != nil {
		t.Fatalf("HandleCallback: %v", err)
	}

	if account.Provider != "github" {
		t.Errorf("Provider = %q, want %q", account.Provider, "github")
	}
	if account.AuthMethod != "oauth" {
		t.Errorf("AuthMethod = %q, want %q", account.AuthMethod, "oauth")
	}
	if account.ID != "acc-test-123" {
		t.Errorf("ID = %q, want %q", account.ID, "acc-test-123")
	}

	// State should be consumed (deleted).
	if _, ok := store.states[state.State]; ok {
		t.Error("state should have been deleted after callback")
	}

	// Token should be encrypted.
	if len(account.EncryptedToken) == 0 {
		t.Error("encrypted token should not be empty")
	}
	decrypted, err := vcsaccount.Decrypt(account.EncryptedToken, encKey)
	if err != nil {
		t.Fatalf("decrypt token: %v", err)
	}
	if string(decrypted) != "gho_test_token_12345" {
		t.Errorf("decrypted token = %q, want %q", string(decrypted), "gho_test_token_12345")
	}
}

func TestGitHubOAuth_HandleCallback_InvalidState(t *testing.T) {
	store := newOAuthMockStore()
	svc := service.NewGitHubOAuthService(service.GitHubOAuthConfig{
		ClientID: "test-client-id",
	}, store, []byte("0123456789abcdef0123456789abcdef"))

	_, err := svc.HandleCallback(context.Background(), "some-code", "invalid-state")
	if err == nil {
		t.Fatal("expected error for invalid state")
	}
	if !strings.Contains(err.Error(), "invalid or expired oauth state") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGitHubOAuth_HandleCallback_EmptyCode(t *testing.T) {
	store := newOAuthMockStore()
	svc := service.NewGitHubOAuthService(service.GitHubOAuthConfig{
		ClientID: "test-client-id",
	}, store, []byte("0123456789abcdef0123456789abcdef"))

	_, err := svc.HandleCallback(context.Background(), "", "some-state")
	if err == nil {
		t.Fatal("expected error for empty code")
	}
	if !strings.Contains(err.Error(), "authorization code is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGitHubOAuth_HandleCallback_EmptyState(t *testing.T) {
	store := newOAuthMockStore()
	svc := service.NewGitHubOAuthService(service.GitHubOAuthConfig{
		ClientID: "test-client-id",
	}, store, []byte("0123456789abcdef0123456789abcdef"))

	_, err := svc.HandleCallback(context.Background(), "some-code", "")
	if err == nil {
		t.Fatal("expected error for empty state")
	}
	if !strings.Contains(err.Error(), "state parameter is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGitHubOAuth_HandleCallback_TokenExchangeError(t *testing.T) {
	// Mock GitHub returning an error.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
	}))
	defer tokenSrv.Close()

	store := newOAuthMockStore()
	svc := service.NewGitHubOAuthService(service.GitHubOAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
	}, store, []byte("0123456789abcdef0123456789abcdef"))
	svc.SetHTTPClient(tokenSrv.Client())
	svc.SetTokenURL(tokenSrv.URL)

	state, err := vcsaccount.NewOAuthState("github", "tenant-1")
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}
	store.states[state.State] = state

	_, err = svc.HandleCallback(context.Background(), "bad-code", state.State)
	if err == nil {
		t.Fatal("expected error for bad token exchange")
	}
	if !strings.Contains(err.Error(), "token exchange") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGitHubOAuth_HandleCallback_EmptyAccessToken(t *testing.T) {
	// Mock GitHub returning a 200 but with an empty access token.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"access_token": "",
			"token_type":   "bearer",
		})
	}))
	defer tokenSrv.Close()

	store := newOAuthMockStore()
	svc := service.NewGitHubOAuthService(service.GitHubOAuthConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
	}, store, []byte("0123456789abcdef0123456789abcdef"))
	svc.SetHTTPClient(tokenSrv.Client())
	svc.SetTokenURL(tokenSrv.URL)

	state, err := vcsaccount.NewOAuthState("github", "tenant-1")
	if err != nil {
		t.Fatalf("NewOAuthState: %v", err)
	}
	store.states[state.State] = state

	_, err = svc.HandleCallback(context.Background(), "some-code", state.State)
	if err == nil {
		t.Fatal("expected error for empty access token")
	}
	if !strings.Contains(err.Error(), "empty access token") {
		t.Errorf("unexpected error: %v", err)
	}
}
