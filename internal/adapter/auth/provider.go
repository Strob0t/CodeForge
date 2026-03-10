// Package auth implements OAuth device flow adapters for subscription-based
// LLM providers (Claude Code Max, GitHub Copilot).
package auth

import (
	"context"
	"errors"
	"net/http"
)

// SubscriptionProvider defines the interface for OAuth-based subscription providers.
type SubscriptionProvider interface {
	Name() string
	DeviceFlowStart(ctx context.Context) (*DeviceCode, error)
	DeviceFlowPoll(ctx context.Context, code string) (*Token, error)
	ExchangeForAPIKey(ctx context.Context, token *Token) (string, error)
	EnvVarName() string
}

// DeviceCode holds the response from initiating a device authorization flow.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// Token holds OAuth tokens.
type Token struct {
	AccessToken  string `json:"access_token"`            //nolint:gosec // G117: OAuth field name, not a credential
	RefreshToken string `json:"refresh_token,omitempty"` //nolint:gosec // G117: OAuth field name, not a credential
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

// ErrAuthPending indicates the user hasn't completed authorization yet.
var ErrAuthPending = errors.New("authorization pending")

// ErrSlowDown indicates the polling interval should be increased.
var ErrSlowDown = errors.New("slow down")

// ErrExpired indicates the device code has expired.
var ErrExpired = errors.New("device code expired")

// Option configures a subscription provider.
type Option struct {
	httpClient *http.Client
}

// WithHTTPClient overrides the default HTTP client (primarily for testing).
func WithHTTPClient(c *http.Client) Option {
	return Option{httpClient: c}
}
