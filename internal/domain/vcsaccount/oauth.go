package vcsaccount

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	// oauthStateBytes is the number of random bytes for the OAuth state parameter.
	oauthStateBytes = 32
	// oauthStateExpiry is how long an OAuth state token remains valid.
	oauthStateExpiry = 10 * time.Minute
)

// OAuthState holds CSRF protection data for an OAuth authorization flow.
type OAuthState struct {
	State     string    `json:"state"`      // crypto-random hex string (64 chars)
	Provider  string    `json:"provider"`   // github, gitlab, etc.
	TenantID  string    `json:"tenant_id"`  // tenant isolation
	ExpiresAt time.Time `json:"expires_at"` // 10 minute expiry
	CreatedAt time.Time `json:"created_at"`
}

// OAuthToken stores the tokens received from an OAuth provider token exchange.
// Secret fields use json:"-" to prevent accidental serialization (same pattern
// as VCSAccount.EncryptedToken).
type OAuthToken struct {
	AccessToken  string    `json:"-"`          // secret, never expose in JSON
	TokenType    string    `json:"token_type"` // usually "bearer"
	RefreshToken string    `json:"-"`          // secret, may be empty
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"` // zero value means no expiry
}

// NewOAuthState creates a new OAuthState with a cryptographically random state
// parameter and a 10-minute expiry window.
func NewOAuthState(provider, tenantID string) (*OAuthState, error) {
	b := make([]byte, oauthStateBytes)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate oauth state: %w", err)
	}

	now := time.Now()

	return &OAuthState{
		State:     hex.EncodeToString(b),
		Provider:  provider,
		TenantID:  tenantID,
		ExpiresAt: now.Add(oauthStateExpiry),
		CreatedAt: now,
	}, nil
}

// IsExpired reports whether the OAuth state has passed its expiry time.
func (s *OAuthState) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}
