package user

import (
	"errors"
	"fmt"
	"time"
)

// APIKeyPrefix is prepended to generated API keys for identification.
const APIKeyPrefix = "cfk_"

// Resource-based API key scopes.
const (
	ScopeProjectsRead  = "projects:read"
	ScopeProjectsWrite = "projects:write"
	ScopeRunsRead      = "runs:read"
	ScopeRunsWrite     = "runs:write"
	ScopeAgentsRead    = "agents:read"
	ScopeAgentsWrite   = "agents:write"
	ScopeAdminAll      = "admin:all"
)

// ValidScopes is the set of all valid API key scopes.
var ValidScopes = map[string]bool{
	ScopeProjectsRead:  true,
	ScopeProjectsWrite: true,
	ScopeRunsRead:      true,
	ScopeRunsWrite:     true,
	ScopeAgentsRead:    true,
	ScopeAgentsWrite:   true,
	ScopeAdminAll:      true,
}

// APIKey represents a stored API key linked to a user.
type APIKey struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"` // first 8 chars for display
	KeyHash   string    `json:"-"`      // SHA-256 hash, never serialized
	Scopes    []string  `json:"scopes,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitzero"`
	CreatedAt time.Time `json:"created_at"`
}

// HasScope checks whether the API key has the required scope.
// A nil/empty Scopes slice means full access (backward compat for old keys).
// The admin:all scope grants access to everything.
func (k *APIKey) HasScope(required string) bool {
	if k.Scopes == nil {
		return true // nil = full access (backward compat)
	}
	for _, s := range k.Scopes {
		if s == required || s == ScopeAdminAll {
			return true
		}
	}
	return false
}

// CreateAPIKeyRequest is the input for creating a new API key.
type CreateAPIKeyRequest struct {
	Name      string   `json:"name"`
	ExpiresIn int      `json:"expires_in,omitempty"` // seconds; 0 = no expiry
	Scopes    []string `json:"scopes,omitempty"`
}

// Validate checks that the CreateAPIKeyRequest has all required fields.
func (r *CreateAPIKeyRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if err := ValidateScopes(r.Scopes); err != nil {
		return err
	}
	return nil
}

// ValidateScopes checks that all scopes are recognized.
func ValidateScopes(scopes []string) error {
	for _, s := range scopes {
		if !ValidScopes[s] {
			return fmt.Errorf("invalid scope: %s", s)
		}
	}
	return nil
}

// CreateAPIKeyResponse is returned after creating an API key.
// The PlainKey is only shown once at creation time.
type CreateAPIKeyResponse struct {
	APIKey   APIKey `json:"api_key"`
	PlainKey string `json:"plain_key"` // only returned once
}
