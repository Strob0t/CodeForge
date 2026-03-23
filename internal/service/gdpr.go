package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// UserDataExport contains all personal data for a user, structured for
// GDPR Article 20 (Right to Data Portability) compliance.
type UserDataExport struct {
	User    *user.User      `json:"user"`
	APIKeys []user.APIKey   `json:"api_keys"`
	LLMKeys []llmkey.LLMKey `json:"llm_keys"`
}

// GDPRService provides GDPR data export and deletion operations.
type GDPRService struct {
	store database.Store
}

// NewGDPRService creates a new GDPR service backed by the given store.
func NewGDPRService(store database.Store) *GDPRService {
	return &GDPRService{store: store}
}

// ExportUserData collects all personal data associated with the given user ID.
// Returns a structured export suitable for JSON serialization (GDPR Article 20).
func (s *GDPRService) ExportUserData(ctx context.Context, userID string) (*UserDataExport, error) {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	apiKeys, err := s.store.ListAPIKeysByUser(ctx, userID)
	if err != nil {
		slog.Warn("gdpr export: failed to list api keys", "user_id", userID, "error", err)
		apiKeys = []user.APIKey{}
	}

	llmKeys, err := s.store.ListLLMKeysByUser(ctx, userID)
	if err != nil {
		slog.Warn("gdpr export: failed to list llm keys", "user_id", userID, "error", err)
		llmKeys = []llmkey.LLMKey{}
	}

	return &UserDataExport{
		User:    u,
		APIKeys: apiKeys,
		LLMKeys: llmKeys,
	}, nil
}

// DeleteUserData removes all personal data for the given user via cascade
// deletion (GDPR Article 17 — Right to Erasure). The database FK constraints
// with ON DELETE CASCADE handle dependent rows automatically.
func (s *GDPRService) DeleteUserData(ctx context.Context, userID string) error {
	if err := s.store.DeleteUser(ctx, userID); err != nil {
		return fmt.Errorf("delete user data: %w", err)
	}
	slog.Info("gdpr: user data deleted", "user_id", userID)
	return nil
}
