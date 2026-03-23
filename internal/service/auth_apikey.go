package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// APIKeyManager handles API key creation, listing, deletion, and validation.
type APIKeyManager struct {
	store database.Store
}

// NewAPIKeyManager creates an API key manager.
func NewAPIKeyManager(store database.Store) *APIKeyManager {
	return &APIKeyManager{store: store}
}

// CreateAPIKey generates a new API key for a user.
func (m *APIKeyManager) CreateAPIKey(ctx context.Context, userID string, req user.CreateAPIKeyRequest) (*user.CreateAPIKeyResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	rawKey, err := crypto.GenerateRandomToken()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	plainKey := user.APIKeyPrefix + rawKey

	var expiresAt time.Time
	if req.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
	}

	key := &user.APIKey{
		ID:        crypto.GenerateUUIDv4(),
		UserID:    userID,
		Name:      req.Name,
		Prefix:    plainKey[:12], // "cfk_" + 8 chars
		KeyHash:   crypto.HashSHA256(plainKey),
		ExpiresAt: expiresAt,
		Scopes:    req.Scopes,
	}

	if err := m.store.CreateAPIKey(ctx, key); err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	return &user.CreateAPIKeyResponse{
		APIKey:   *key,
		PlainKey: plainKey,
	}, nil
}

// ListAPIKeys returns all API keys for a user.
func (m *APIKeyManager) ListAPIKeys(ctx context.Context, userID string) ([]user.APIKey, error) {
	return m.store.ListAPIKeysByUser(ctx, userID)
}

// DeleteAPIKey removes an API key owned by the given user.
func (m *APIKeyManager) DeleteAPIKey(ctx context.Context, id, userID string) error {
	return m.store.DeleteAPIKey(ctx, id, userID)
}

// ValidateAPIKey looks up an API key by its SHA-256 hash.
// Returns the user and the API key (for scope checking).
func (m *APIKeyManager) ValidateAPIKey(ctx context.Context, rawKey string) (*user.User, *user.APIKey, error) {
	keyHash := crypto.HashSHA256(rawKey)
	apiKey, err := m.store.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return nil, nil, errors.New("invalid api key")
	}

	if !apiKey.ExpiresAt.IsZero() && time.Now().After(apiKey.ExpiresAt) {
		return nil, nil, errors.New("api key expired")
	}

	u, err := m.store.GetUser(ctx, apiKey.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("get user: %w", err)
	}
	return u, apiKey, nil
}
