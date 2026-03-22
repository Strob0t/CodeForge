package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Default GitHub OAuth endpoints.
var (
	githubDeviceCodeURL = "https://github.com/login/device/code"
	githubTokenURL      = "https://github.com/login/oauth/access_token" //nolint:gosec // G101: URL, not a credential
)

// Well-known VS Code OAuth client ID (fallback).
const defaultGitHubClientID = "01ab8ac9400c4e429b23"

// GitHubProvider implements SubscriptionProvider for GitHub Copilot.
type GitHubProvider struct {
	httpClient    *http.Client
	deviceCodeURL string
	tokenURL      string
	clientID      string
}

// NewGitHubProvider creates a new GitHub Copilot OAuth provider.
func NewGitHubProvider(opts ...Option) *GitHubProvider {
	p := &GitHubProvider{
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		deviceCodeURL: githubDeviceCodeURL,
		tokenURL:      githubTokenURL,
		clientID:      defaultGitHubClientID,
	}
	for _, opt := range opts {
		if opt.httpClient != nil {
			p.httpClient = opt.httpClient
		}
		if opt.clientID != "" {
			p.clientID = opt.clientID
		}
	}
	return p
}

func (p *GitHubProvider) Name() string       { return "github_copilot" }
func (p *GitHubProvider) EnvVarName() string { return "GITHUB_TOKEN" }

// DeviceFlowStart initiates the OAuth device authorization flow with GitHub.
func (p *GitHubProvider) DeviceFlowStart(ctx context.Context) (*DeviceCode, error) {
	body, err := json.Marshal(map[string]string{
		"client_id": p.clientID,
		"scope":     "copilot",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.deviceCodeURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req) //nolint:gosec // G704: URL from struct field, not user input
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed (HTTP %d): %s",
			resp.StatusCode, truncate(respBody))
	}

	var dc DeviceCode
	if err := json.Unmarshal(respBody, &dc); err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}

	slog.Info("github device flow started", "user_code", dc.UserCode, "verification_uri", dc.VerificationURI)
	return &dc, nil
}

// DeviceFlowPoll polls for the completion of the device authorization flow.
// Note: GitHub returns errors in the response body with HTTP 200, not 4xx.
func (p *GitHubProvider) DeviceFlowPoll(ctx context.Context, code string) (*Token, error) {
	body, err := json.Marshal(map[string]string{
		"client_id":   p.clientID,
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
		"device_code": code,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req) //nolint:gosec // G704: URL from struct field, not user input
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed (HTTP %d): %s",
			resp.StatusCode, truncate(respBody))
	}

	// GitHub returns errors as 200 with an "error" field in the JSON body.
	var errResp oauthError
	if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != "" {
		switch errResp.Error {
		case "authorization_pending":
			return nil, ErrAuthPending
		case "slow_down":
			return nil, ErrSlowDown
		case "expired_token":
			return nil, ErrExpired
		default:
			return nil, fmt.Errorf("oauth error: %s", errResp.Error)
		}
	}

	var tok Token
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &tok, nil
}

// ExchangeForAPIKey returns the access token directly. For GitHub Copilot,
// the OAuth access token is used as the API key.
func (p *GitHubProvider) ExchangeForAPIKey(_ context.Context, token *Token) (string, error) {
	return token.AccessToken, nil
}
