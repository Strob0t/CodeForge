package llmkey

import (
	"errors"
	"time"
)

// AllowedProviders lists the LLM providers users can configure keys for.
var AllowedProviders = map[string]bool{
	"openai":     true,
	"anthropic":  true,
	"gemini":     true,
	"groq":       true,
	"mistral":    true,
	"openrouter": true,
	"deepseek":   true,
	"together":   true,
	"xai":        true,
}

// LLMKey represents an encrypted per-user LLM provider API key.
type LLMKey struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	TenantID     string    `json:"tenant_id"`
	Provider     string    `json:"provider"`
	Label        string    `json:"label"`
	EncryptedKey []byte    `json:"-"` // never expose
	KeyPrefix    string    `json:"key_prefix"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateRequest is the request body for creating a user LLM key.
type CreateRequest struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	APIKey   string `json:"api_key"` //nolint:gosec // not a hardcoded credential
}

// Validate checks that the create request has valid fields.
func (r CreateRequest) Validate() error {
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

// MakeKeyPrefix returns the first 8 characters of the key for display (e.g. "sk-ab****").
func MakeKeyPrefix(apiKey string) string {
	if len(apiKey) <= 8 {
		return apiKey[:len(apiKey)/2] + "****"
	}
	return apiKey[:8] + "****"
}
