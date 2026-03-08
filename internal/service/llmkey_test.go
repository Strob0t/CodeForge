package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// llmKeyMockStore implements only the LLMKey-related methods for testing.
type llmKeyMockStore struct {
	database.Store // embed interface to satisfy compiler for unused methods
	keys           []llmkey.LLMKey
	createErr      error
	deleteErr      error
}

func (m *llmKeyMockStore) CreateLLMKey(_ context.Context, k *llmkey.LLMKey) error {
	if m.createErr != nil {
		return m.createErr
	}
	k.ID = "generated-id"
	m.keys = append(m.keys, *k)
	return nil
}

func (m *llmKeyMockStore) ListLLMKeysByUser(_ context.Context, userID string) ([]llmkey.LLMKey, error) {
	var result []llmkey.LLMKey
	for i := range m.keys {
		if m.keys[i].UserID == userID {
			result = append(result, m.keys[i])
		}
	}
	return result, nil
}

func (m *llmKeyMockStore) GetLLMKeyByUserProvider(_ context.Context, userID, provider string) (*llmkey.LLMKey, error) {
	for i := range m.keys {
		if m.keys[i].UserID == userID && m.keys[i].Provider == provider {
			return &m.keys[i], nil
		}
	}
	return nil, fmt.Errorf("not found: %w", domain.ErrNotFound)
}

func (m *llmKeyMockStore) DeleteLLMKey(_ context.Context, id, userID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	for i := range m.keys {
		if m.keys[i].ID == id && m.keys[i].UserID == userID {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found: %w", domain.ErrNotFound)
}

func newTestLLMKeyService() (*LLMKeyService, *llmKeyMockStore) {
	store := &llmKeyMockStore{}
	key := crypto.DeriveKey("test-jwt-secret")
	svc := NewLLMKeyService(store, key)
	return svc, store
}

func TestLLMKeyService_Create(t *testing.T) {
	svc, store := newTestLLMKeyService()
	ctx := context.Background()

	req := llmkey.CreateRequest{
		Provider: "openai",
		Label:    "My OpenAI Key",
		APIKey:   "sk-test1234567890",
	}

	result, err := svc.Create(ctx, "user-1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "generated-id" {
		t.Fatalf("expected ID 'generated-id', got %q", result.ID)
	}
	if result.UserID != "user-1" {
		t.Fatalf("expected UserID 'user-1', got %q", result.UserID)
	}
	if result.Provider != "openai" {
		t.Fatalf("expected Provider 'openai', got %q", result.Provider)
	}
	if result.Label != "My OpenAI Key" {
		t.Fatalf("expected Label 'My OpenAI Key', got %q", result.Label)
	}
	if result.KeyPrefix != "sk-test1****" {
		t.Fatalf("expected KeyPrefix 'sk-test1****', got %q", result.KeyPrefix)
	}
	// EncryptedKey must be cleared from the returned value.
	if result.EncryptedKey != nil {
		t.Fatal("EncryptedKey should be nil in response")
	}
	// The store should hold the encrypted key.
	if len(store.keys) != 1 {
		t.Fatalf("expected 1 key in store, got %d", len(store.keys))
	}
	if len(store.keys[0].EncryptedKey) == 0 {
		t.Fatal("EncryptedKey should be non-empty in store")
	}
}

func TestLLMKeyService_Create_ValidationError(t *testing.T) {
	svc, _ := newTestLLMKeyService()
	ctx := context.Background()

	tests := []struct {
		name string
		req  llmkey.CreateRequest
	}{
		{"empty provider", llmkey.CreateRequest{Provider: "", Label: "x", APIKey: "sk-x"}},
		{"invalid provider", llmkey.CreateRequest{Provider: "bad", Label: "x", APIKey: "sk-x"}},
		{"empty label", llmkey.CreateRequest{Provider: "openai", Label: "", APIKey: "sk-x"}},
		{"empty api_key", llmkey.CreateRequest{Provider: "openai", Label: "x", APIKey: ""}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(ctx, "user-1", tc.req)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLLMKeyService_List(t *testing.T) {
	svc, _ := newTestLLMKeyService()
	ctx := context.Background()

	// Create two keys for user-1.
	for _, p := range []string{"openai", "anthropic"} {
		_, err := svc.Create(ctx, "user-1", llmkey.CreateRequest{
			Provider: p, Label: p + " key", APIKey: "sk-" + p + "-12345678",
		})
		if err != nil {
			t.Fatalf("create %s: %v", p, err)
		}
	}

	keys, err := svc.List(ctx, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	for i, k := range keys {
		if k.EncryptedKey != nil {
			t.Fatalf("key %d: EncryptedKey should be nil", i)
		}
	}
}

func TestLLMKeyService_List_Empty(t *testing.T) {
	svc, _ := newTestLLMKeyService()
	ctx := context.Background()

	keys, err := svc.List(ctx, "user-nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}

func TestLLMKeyService_Delete(t *testing.T) {
	svc, store := newTestLLMKeyService()
	ctx := context.Background()

	_, err := svc.Create(ctx, "user-1", llmkey.CreateRequest{
		Provider: "openai", Label: "test", APIKey: "sk-delete-me1234",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.Delete(ctx, store.keys[0].ID, "user-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(store.keys) != 0 {
		t.Fatalf("expected 0 keys after delete, got %d", len(store.keys))
	}
}

func TestLLMKeyService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestLLMKeyService()
	ctx := context.Background()

	err := svc.Delete(ctx, "nonexistent-id", "user-1")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestLLMKeyService_ResolveKeyForProvider_Found(t *testing.T) {
	svc, _ := newTestLLMKeyService()
	ctx := context.Background()

	apiKey := "sk-resolve-test-12345678"
	_, err := svc.Create(ctx, "user-1", llmkey.CreateRequest{
		Provider: "anthropic", Label: "My Claude", APIKey: apiKey,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resolved, err := svc.ResolveKeyForProvider(ctx, "user-1", "anthropic")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != apiKey {
		t.Fatalf("expected %q, got %q", apiKey, resolved)
	}
}

func TestLLMKeyService_ResolveKeyForProvider_NotFound(t *testing.T) {
	svc, _ := newTestLLMKeyService()
	ctx := context.Background()

	resolved, err := svc.ResolveKeyForProvider(ctx, "user-1", "openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != "" {
		t.Fatalf("expected empty string for missing provider, got %q", resolved)
	}
}

func TestLLMKeyService_EncryptionRoundtrip(t *testing.T) {
	svc, store := newTestLLMKeyService()
	ctx := context.Background()

	apiKey := "sk-roundtrip-secret-key-value"
	_, err := svc.Create(ctx, "user-1", llmkey.CreateRequest{
		Provider: "gemini", Label: "Gemini", APIKey: apiKey,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Verify the stored key is encrypted (not plaintext).
	if string(store.keys[0].EncryptedKey) == apiKey {
		t.Fatal("stored key should be encrypted, not plaintext")
	}

	// Verify roundtrip via resolve.
	resolved, err := svc.ResolveKeyForProvider(ctx, "user-1", "gemini")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != apiKey {
		t.Fatalf("roundtrip failed: expected %q, got %q", apiKey, resolved)
	}
}
