package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
)

// LLMKeyStore defines database operations for per-user LLM provider API keys.
type LLMKeyStore interface {
	CreateLLMKey(ctx context.Context, k *llmkey.LLMKey) error
	ListLLMKeysByUser(ctx context.Context, userID string) ([]llmkey.LLMKey, error)
	GetLLMKeyByUserProvider(ctx context.Context, userID, provider string) (*llmkey.LLMKey, error)
	DeleteLLMKey(ctx context.Context, id, userID string) error
}
