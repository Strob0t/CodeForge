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

// Default Anthropic OAuth endpoints.
var (
	anthropicDeviceCodeURL = "https://console.anthropic.com/v1/oauth/device/code"
	anthropicTokenURL      = "https://console.anthropic.com/v1/oauth/token"                  //nolint:gosec // G101: URL, not a credential
	anthropicAPIKeyURL     = "https://api.anthropic.com/api/oauth/claude_cli/create_api_key" //nolint:gosec // G101: URL, not a credential
)

// AnthropicProvider implements SubscriptionProvider for Claude Code Max.
type AnthropicProvider struct {
	httpClient    *http.Client
	deviceCodeURL string
	tokenURL      string
	apiKeyURL     string
}

// NewAnthropicProvider creates a new Anthropic OAuth provider.
func NewAnthropicProvider(opts ...Option) *AnthropicProvider {
	p := &AnthropicProvider{
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		deviceCodeURL: anthropicDeviceCodeURL,
		tokenURL:      anthropicTokenURL,
		apiKeyURL:     anthropicAPIKeyURL,
	}
	for _, opt := range opts {
		if opt.httpClient != nil {
			p.httpClient = opt.httpClient
		}
	}
	return p
}

func (p *AnthropicProvider) Name() string       { return "anthropic" }
func (p *AnthropicProvider) EnvVarName() string { return "ANTHROPIC_API_KEY" }

// DeviceFlowStart initiates the OAuth device authorization flow with Anthropic.
func (p *AnthropicProvider) DeviceFlowStart(ctx context.Context) (*DeviceCode, error) {
	body, err := json.Marshal(map[string]string{
		"client_id": "codeforge",
		"scope":     "user:inference",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.deviceCodeURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req) //nolint:gosec // G704: URL from struct field, not user input
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed (HTTP %d): %s",
			resp.StatusCode, truncate(respBody))
	}

	var dc DeviceCode
	if err := json.Unmarshal(respBody, &dc); err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}

	slog.Info("anthropic device flow started", "user_code", dc.UserCode, "verification_uri", dc.VerificationURI)
	return &dc, nil
}

// oauthError represents an OAuth error response body.
type oauthError struct {
	Error string `json:"error"`
}

// DeviceFlowPoll polls for the completion of the device authorization flow.
func (p *AnthropicProvider) DeviceFlowPoll(ctx context.Context, code string) (*Token, error) {
	body, err := json.Marshal(map[string]string{
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

	resp, err := p.httpClient.Do(req) //nolint:gosec // G704: URL from struct field, not user input
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, mapOAuthError(respBody, resp.StatusCode)
	}

	var tok Token
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &tok, nil
}

// apiKeyResponse is the response from the Anthropic API key creation endpoint.
type apiKeyResponse struct {
	APIKey string `json:"api_key"` //nolint:gosec // G117: JSON field name, not a credential
}

// ExchangeForAPIKey converts an OAuth access token into a standard API key.
func (p *AnthropicProvider) ExchangeForAPIKey(ctx context.Context, token *Token) (string, error) {
	body, err := json.Marshal(map[string]string{
		"name": "codeforge",
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiKeyURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := p.httpClient.Do(req) //nolint:gosec // G704: URL from struct field, not user input
	if err != nil {
		return "", fmt.Errorf("api key request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api key request failed (HTTP %d): %s",
			resp.StatusCode, truncate(respBody))
	}

	var keyResp apiKeyResponse
	if err := json.Unmarshal(respBody, &keyResp); err != nil {
		return "", fmt.Errorf("parse api key response: %w", err)
	}

	slog.Info("anthropic API key created via OAuth")
	return keyResp.APIKey, nil
}

// mapOAuthError maps an OAuth error response to a sentinel error.
func mapOAuthError(body []byte, statusCode int) error {
	var oErr oauthError
	if err := json.Unmarshal(body, &oErr); err == nil {
		switch oErr.Error {
		case "authorization_pending":
			return ErrAuthPending
		case "slow_down":
			return ErrSlowDown
		case "expired_token":
			return ErrExpired
		}
		return fmt.Errorf("oauth error: %s", oErr.Error)
	}
	return fmt.Errorf("token request failed (HTTP %d): %s", statusCode, truncate(body))
}

// truncate returns at most 500 bytes from b as a string.
func truncate(b []byte) string {
	const maxLen = 500
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen])
}
