// Package llmkey defines the domain model for per-user LLM provider API keys.
package llmkey

import (
	"errors"
	"time"
)

// AllowedProviders is the set of valid LLM provider identifiers.
var AllowedProviders = map[string]bool{
	"openai":     true,
	"anthropic":  true,
	"gemini":     true,
	"groq":       true,
	"mistral":    true,
	"openrouter": true,
	"together":   true,
	"deepseek":   true,
	"cohere":     true,
}

// LLMKey represents an encrypted LLM provider API key belonging to a user.
type LLMKey struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	TenantID     string    `json:"tenant_id"`
	Provider     string    `json:"provider"`
	Label        string    `json:"label"`
	EncryptedKey []byte    `json:"-"`
	KeyPrefix    string    `json:"key_prefix"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateRequest is the input for storing a new LLM key.
type CreateRequest struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	APIKey   string `json:"api_key"` //nolint:gosec // not a hardcoded credential
}

// Validate checks the CreateRequest fields.
func (r *CreateRequest) Validate() error {
	if r.Provider == "" {
		return errors.New("provider is required")
	}
	if !AllowedProviders[r.Provider] {
		return errors.New("unsupported provider: " + r.Provider)
	}
	if r.Label == "" {
		return errors.New("label is required")
	}
	if r.APIKey == "" {
		return errors.New("api_key is required")
	}
	return nil
}

// MakeKeyPrefix returns a safe display prefix from a plaintext API key.
// Shows the first 8 characters followed by "****".
func MakeKeyPrefix(apiKey string) string {
	if len(apiKey) <= 8 {
		return apiKey[:len(apiKey)/2] + "****"
	}
	return apiKey[:8] + "****"
}
