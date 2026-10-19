package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
	"github.com/Strob0t/CodeForge/internal/service"
)

const (
	testCallbackURL   = "https://cf.example.com/api/v1/auth/github/callback"
	githubOAuthCookie = "codeforge_github_oauth"
)

func TestStartGitHubOAuth_NotConfigured(t *testing.T) {
	h := &cfhttp.Handlers{
		Limits: &config.Limits{MaxRequestBodySize: 1 << 20},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/github", http.NoBody)
	rec := httptest.NewRecorder()

	h.StartGitHubOAuth(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected %d, got %d: %s", http.StatusNotImplemented, rec.Code, rec.Body.String())
	}
	// The message says what to configure.
	if body := rec.Body.String(); !strings.Contains(body, "github.client_secret") || !strings.Contains(body, "github.callback_url") {
		t.Fatalf("501 body %s does not name the settings", body)
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

// oauthCallbackStore holds one OAuth state and counts created accounts.
type oauthCallbackStore struct {
	*mockStore
	state   *vcsaccount.OAuthState
	created int
}

func (s *oauthCallbackStore) CreateOAuthState(_ context.Context, st *vcsaccount.OAuthState) error {
	s.state = st
	return nil
}

func (s *oauthCallbackStore) ConsumeOAuthState(_ context.Context, token string) (*vcsaccount.OAuthState, error) {
	if s.state == nil || s.state.State != token {
		return nil, domain.ErrNotFound
	}
	st := s.state
	s.state = nil
	return st, nil
}

func (s *oauthCallbackStore) CreateVCSAccount(_ context.Context, a *vcsaccount.VCSAccount) (*vcsaccount.VCSAccount, error) {
	s.created++
	a.ID = "acc-1"
	return a, nil
}

func newGitHubOAuthHandlers(t *testing.T) (*cfhttp.Handlers, *oauthCallbackStore) {
	t.Helper()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"gho_x","token_type":"bearer"}`))
	}))
	t.Cleanup(tokenSrv.Close)

	store := &oauthCallbackStore{mockStore: &mockStore{}}
	oauthSvc := service.NewGitHubOAuthService(
		service.GitHubOAuthConfig{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURI:  testCallbackURL,
			Scopes:       []string{"repo"},
		},
		store,
		[]byte("test-encryption-key-32bytes!!!!!"),
	)
	oauthSvc.SetHTTPClient(tokenSrv.Client())
	oauthSvc.SetTokenURL(tokenSrv.URL)
	return &cfhttp.Handlers{
		GitHubOAuth: oauthSvc,
		Limits:      &config.Limits{MaxRequestBodySize: 1 << 20},
	}, store
}

// startFlow runs POST /auth/github and returns the state and its cookie.
func startFlow(t *testing.T, h *cfhttp.Handlers) (string, *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.StartGitHubOAuth(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/github", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	authURL, err := url.Parse(resp.URL)
	if err != nil || authURL.Host != "github.com" {
		t.Fatalf("authorize url = %q", resp.URL)
	}
	if got := authURL.Query().Get("redirect_uri"); got != testCallbackURL {
		t.Fatalf("redirect_uri = %q, want the configured %q", got, testCallbackURL)
	}
	state := authURL.Query().Get("state")
	for _, c := range rec.Result().Cookies() {
		if c.Name == githubOAuthCookie {
			if c.Value != state || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/api/v1/auth/github/callback" || c.MaxAge <= 0 {
				t.Fatalf("state cookie = %+v", c)
			}
			return state, c
		}
	}
	t.Fatal("no state cookie set")
	return "", nil
}

func callback(h *cfhttp.Handlers, query string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github/callback"+query, http.NoBody)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.GitHubOAuthCallback(rec, req)
	return rec
}

func TestGitHubOAuth_FlowConnectsTheAccount(t *testing.T) {
	h, store := newGitHubOAuthHandlers(t)
	state, cookie := startFlow(t, h)

	rec := callback(h, "?code=abc&state="+state, cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "https://cf.example.com/settings?github_oauth=connected" {
		t.Fatalf("callback = %d %q %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if store.created != 1 {
		t.Fatalf("accounts created = %d, want 1", store.created)
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		cleared = cleared || (c.Name == githubOAuthCookie && c.MaxAge < 0)
	}
	if !cleared {
		t.Fatal("the state cookie was not cleared")
	}
}

// The callback goes back to the web UI with a fixed reason code; nothing
// from the request is reflected into the redirect.
func TestGitHubOAuthCallback_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		query  func(state string) string
		cookie func(c *http.Cookie) *http.Cookie
		reason string
	}{
		{name: "missing code", query: func(s string) string { return "?state=" + s }, reason: "invalid_request"},
		{name: "missing state", query: func(string) string { return "?code=abc" }, reason: "invalid_request"},
		{name: "user denied", query: func(s string) string { return "?error=access_denied&state=" + s }, reason: "denied"},
		{name: "no state cookie (another browser)", query: func(s string) string { return "?code=abc&state=" + s },
			cookie: func(*http.Cookie) *http.Cookie { return nil }, reason: "state_mismatch"},
		{name: "cookie of another flow", query: func(s string) string { return "?code=abc&state=" + s },
			cookie: func(c *http.Cookie) *http.Cookie { return &http.Cookie{Name: c.Name, Value: "other"} }, reason: "state_mismatch"},
		{name: "unknown state", query: func(string) string { return "?code=abc&state=unknown" },
			cookie: func(c *http.Cookie) *http.Cookie { return &http.Cookie{Name: c.Name, Value: "unknown"} }, reason: "invalid_state"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, store := newGitHubOAuthHandlers(t)
			state, cookie := startFlow(t, h)
			if tc.cookie != nil {
				cookie = tc.cookie(cookie)
			}
			rec := callback(h, tc.query(state), cookie)
			want := "https://cf.example.com/settings?github_oauth=failed&reason=" + tc.reason
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
				t.Fatalf("callback = %d %q, want 303 %q", rec.Code, rec.Header().Get("Location"), want)
			}
			if store.created != 0 {
				t.Fatal("an account was created")
			}
		})
	}
}
