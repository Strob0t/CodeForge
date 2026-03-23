// Package auth implements OAuth device flow adapters for subscription-based
// LLM providers (Claude Code Max, GitHub Copilot).
package auth

import (
	"net/http"

	"github.com/Strob0t/CodeForge/internal/port/subscription"
)

// Type aliases — canonical definitions live in port/subscription.
type (
	SubscriptionProvider = subscription.Provider
	DeviceCode           = subscription.DeviceCode
	Token                = subscription.Token
)

// Error aliases — canonical definitions live in port/subscription.
var (
	ErrAuthPending = subscription.ErrAuthPending
	ErrSlowDown    = subscription.ErrSlowDown
	ErrExpired     = subscription.ErrExpired
)

// Compile-time assertions: both providers satisfy subscription.Provider.
var (
	_ subscription.Provider = (*AnthropicProvider)(nil)
	_ subscription.Provider = (*GitHubProvider)(nil)
)

// Option configures a subscription provider.
type Option struct {
	httpClient *http.Client
	clientID   string
}

// WithHTTPClient overrides the default HTTP client (primarily for testing).
func WithHTTPClient(c *http.Client) Option {
	return Option{httpClient: c}
}

// WithClientID overrides the default OAuth client ID.
func WithClientID(id string) Option {
	return Option{clientID: id}
}
