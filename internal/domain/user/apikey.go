package user

import (
	"errors"
	"time"
)

// APIKeyPrefix is prepended to generated API keys for identification.
const APIKeyPrefix = "cfk_"

// APIKey represents a stored API key linked to a user.
type APIKey struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"` // first 8 chars for display
	KeyHash   string    `json:"-"`      // SHA-256 hash, never serialized
	ExpiresAt time.Time `json:"expires_at,omitzero"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateAPIKeyRequest is the input for creating a new API key.
type CreateAPIKeyRequest struct {
	Name      string `json:"name"`
	ExpiresIn int    `json:"expires_in,omitempty"` // seconds; 0 = no expiry
}

// Validate checks that the CreateAPIKeyRequest has all required fields.
func (r *CreateAPIKeyRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

// CreateAPIKeyResponse is returned after creating an API key.
// The PlainKey is only shown once at creation time.
type CreateAPIKeyResponse struct {
	APIKey   APIKey `json:"api_key"`
	PlainKey string `json:"plain_key"` // only returned once
}
