// Package copilot implements GitHub Copilot token exchange for registering
// Copilot as an LLM provider with LiteLLM.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// hostsEntry is a single entry in the GitHub Copilot hosts.json file.
type hostsEntry struct {
	OAuthToken string `json:"oauth_token"` //nolint:gosec // G117: not a hardcoded credential
}

// tokenResponse is the response from the Copilot token exchange endpoint.
type tokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// Client handles GitHub Copilot token discovery and exchange.
type Client struct {
	hostsPath  string
	httpClient *http.Client

	mu     sync.RWMutex
	token  string
	expiry time.Time
}

// NewClient creates a new Copilot client. If hostsPath is empty, the default
// location (~/.config/github-copilot/hosts.json) is used.
func NewClient(hostsPath string) *Client {
	if hostsPath == "" {
		home, _ := os.UserHomeDir()
		hostsPath = filepath.Join(home, ".config", "github-copilot", "hosts.json")
	}
	return &Client{
		hostsPath:  hostsPath,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// readOAuthToken reads the OAuth token from the Copilot hosts file.
func (c *Client) readOAuthToken() (string, error) {
	data, err := os.ReadFile(c.hostsPath)
	if err != nil {
		return "", fmt.Errorf("read hosts file %s: %w", c.hostsPath, err)
	}

	var hosts map[string]hostsEntry
	if err := json.Unmarshal(data, &hosts); err != nil {
		return "", fmt.Errorf("parse hosts file: %w", err)
	}

	// Look for github.com entry.
	entry, ok := hosts["github.com"]
	if !ok {
		return "", fmt.Errorf("no github.com entry in %s", c.hostsPath)
	}
	if entry.OAuthToken == "" {
		return "", fmt.Errorf("empty oauth_token for github.com in %s", c.hostsPath)
	}
	return entry.OAuthToken, nil
}

// ExchangeToken exchanges the Copilot OAuth token for a short-lived bearer
// token via the GitHub Copilot internal API. Results are cached until expiry
// (minus a 60-second buffer).
func (c *Client) ExchangeToken(ctx context.Context) (string, time.Time, error) {
	c.mu.RLock()
	if c.token != "" && time.Now().Before(c.expiry) {
		tok, exp := c.token, c.expiry
		c.mu.RUnlock()
		return tok, exp, nil
	}
	c.mu.RUnlock()

	oauthToken, err := c.readOAuthToken()
	if err != nil {
		return "", time.Time{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/copilot_internal/v2/token", nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+oauthToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL is hardcoded, not user-controlled
	if err != nil {
		return "", time.Time{}, fmt.Errorf("token exchange request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("token exchange failed (HTTP %d): %s",
			resp.StatusCode, string(body[:min(500, len(body))]))
	}

	var tokResp tokenResponse
	if err := json.Unmarshal(body, &tokResp); err != nil {
		return "", time.Time{}, fmt.Errorf("parse token response: %w", err)
	}

	expiry := time.Unix(tokResp.ExpiresAt, 0).Add(-60 * time.Second)

	c.mu.Lock()
	c.token = tokResp.Token
	c.expiry = expiry
	c.mu.Unlock()

	slog.Info("copilot token exchanged", "expires_at", expiry)
	return tokResp.Token, expiry, nil
}

// HasHostsFile returns true if the Copilot hosts file exists.
func (c *Client) HasHostsFile() bool {
	_, err := os.Stat(c.hostsPath)
	return err == nil
}
