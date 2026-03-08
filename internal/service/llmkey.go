package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// LLMKeyService manages per-user LLM provider API keys.
type LLMKeyService struct {
	db  database.Store
	key []byte // AES-256 encryption key (derived from JWT secret)
}

// NewLLMKeyService creates a new LLMKeyService.
func NewLLMKeyService(db database.Store, encryptionKey []byte) *LLMKeyService {
	return &LLMKeyService{db: db, key: encryptionKey}
}

// Create validates, encrypts, and stores a new LLM provider key for a user.
func (s *LLMKeyService) Create(ctx context.Context, userID string, req llmkey.CreateRequest) (*llmkey.LLMKey, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	encrypted, err := crypto.Encrypt([]byte(req.APIKey), s.key)
	if err != nil {
		return nil, fmt.Errorf("encrypt api key: %w", err)
	}

	k := &llmkey.LLMKey{
		UserID:       userID,
		Provider:     req.Provider,
		Label:        req.Label,
		EncryptedKey: encrypted,
		KeyPrefix:    llmkey.MakeKeyPrefix(req.APIKey),
	}

	if err := s.db.CreateLLMKey(ctx, k); err != nil {
		return nil, err
	}

	// Never return the encrypted key to the caller.
	k.EncryptedKey = nil
	return k, nil
}

// List returns all LLM keys for a user (without encrypted key material).
func (s *LLMKeyService) List(ctx context.Context, userID string) ([]llmkey.LLMKey, error) {
	keys, err := s.db.ListLLMKeysByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		keys[i].EncryptedKey = nil
	}
	return keys, nil
}

// Delete removes an LLM key by ID, scoped to the user.
func (s *LLMKeyService) Delete(ctx context.Context, id, userID string) error {
	return s.db.DeleteLLMKey(ctx, id, userID)
}

// ResolveKeyForProvider decrypts and returns the plaintext API key for a
// given provider. Returns "" if the user has no key stored for that provider
// (the caller should fall back to the global key).
func (s *LLMKeyService) ResolveKeyForProvider(ctx context.Context, userID, provider string) (string, error) {
	k, err := s.db.GetLLMKeyByUserProvider(ctx, userID, provider)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("get llm key: %w", err)
	}

	plaintext, err := crypto.Decrypt(k.EncryptedKey, s.key)
	if err != nil {
		return "", fmt.Errorf("decrypt api key: %w", err)
	}

	return string(plaintext), nil
}
