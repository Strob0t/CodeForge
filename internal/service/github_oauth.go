package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

const githubTokenURL = "https://github.com/login/oauth/access_token" //nolint:gosec // G101: not a credential, just a well-known OAuth endpoint URL

// GitHubOAuthConfig holds the configuration for GitHub OAuth.
type GitHubOAuthConfig struct {
	ClientID     string
	ClientSecret string   //nolint:gosec // G117: config struct field, not a hardcoded secret
	RedirectURI  string   // e.g. "http://localhost:3000/api/v1/auth/github/callback"
	Scopes       []string // e.g. ["repo", "read:user"]
}

// GitHubOAuthService handles the OAuth 2.0 authorization code flow for GitHub.
type GitHubOAuthService struct {
	cfg        GitHubOAuthConfig
	db         database.Store
	encKey     []byte
	httpClient *http.Client
	tokenURL   string // defaults to GitHub; overridable for tests
}

// NewGitHubOAuthService creates a new GitHub OAuth service.
func NewGitHubOAuthService(cfg GitHubOAuthConfig, db database.Store, encryptionKey []byte) *GitHubOAuthService {
	return &GitHubOAuthService{
		cfg:        cfg,
		db:         db,
		encKey:     encryptionKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		tokenURL:   githubTokenURL,
	}
}

// AuthorizeURL generates the GitHub OAuth authorization URL and stores the CSRF state.
func (s *GitHubOAuthService) AuthorizeURL(ctx context.Context) (string, error) {
	state, err := vcsaccount.NewOAuthState("github", "")
	if err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}

	if err := s.db.CreateOAuthState(ctx, state); err != nil {
		return "", fmt.Errorf("store oauth state: %w", err)
	}

	params := url.Values{
		"client_id":    {s.cfg.ClientID},
		"redirect_uri": {s.cfg.RedirectURI},
		"scope":        {strings.Join(s.cfg.Scopes, " ")},
		"state":        {state.State},
	}

	return "https://github.com/login/oauth/authorize?" + params.Encode(), nil
}

// githubTokenResponse represents the JSON response from GitHub's token endpoint.
type githubTokenResponse struct {
	AccessToken string `json:"access_token"` //nolint:gosec // G117: JSON deserialization target, not a hardcoded secret
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// HandleCallback validates the state, exchanges the code for a token, and creates a VCS account.
func (s *GitHubOAuthService) HandleCallback(ctx context.Context, code, stateParam string) (*vcsaccount.VCSAccount, error) {
	if code == "" {
		return nil, fmt.Errorf("authorization code is required")
	}
	if stateParam == "" {
		return nil, fmt.Errorf("state parameter is required")
	}

	// Validate and consume the state token (CSRF check).
	oauthState, err := s.db.GetOAuthState(ctx, stateParam)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired oauth state: %w", err)
	}

	// Delete the state immediately to prevent replay.
	_ = s.db.DeleteOAuthState(ctx, oauthState.State)

	// Exchange the authorization code for an access token.
	token, err := s.exchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	// Encrypt the access token for storage.
	encrypted, err := crypto.Encrypt([]byte(token.AccessToken), s.encKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt token: %w", err)
	}

	account := &vcsaccount.VCSAccount{
		Provider:       "github",
		Label:          "GitHub (OAuth)",
		AuthMethod:     "oauth",
		EncryptedToken: encrypted,
	}

	created, err := s.db.CreateVCSAccount(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("create vcs account: %w", err)
	}

	return created, nil
}

// SetTokenURL overrides the GitHub token endpoint URL (used in tests).
func (s *GitHubOAuthService) SetTokenURL(u string) {
	s.tokenURL = u
}

// SetHTTPClient overrides the HTTP client (used in tests with httptest.Server).
func (s *GitHubOAuthService) SetHTTPClient(c *http.Client) {
	s.httpClient = c
}

// exchangeCode sends the authorization code to GitHub's token endpoint.
func (s *GitHubOAuthService) exchangeCode(ctx context.Context, code string) (*vcsaccount.OAuthToken, error) {
	data := url.Values{
		"client_id":     {s.cfg.ClientID},
		"client_secret": {s.cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {s.cfg.RedirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.tokenURL,
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req) //nolint:gosec // G704: tokenURL defaults to GitHub constant; only overridable in tests
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp githubTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("github returned empty access token")
	}

	return &vcsaccount.OAuthToken{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		Scope:       tokenResp.Scope,
	}, nil
}
