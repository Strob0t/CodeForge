package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/llmkey"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// ConversationExport bundles a conversation with its messages for GDPR export.
type ConversationExport struct {
	Conversation conversation.Conversation `json:"conversation"`
	Messages     []conversation.Message    `json:"messages"`
}

// UserDataExport contains all personal data for a user, structured for
// GDPR Article 20 (Right to Data Portability) compliance.
type UserDataExport struct {
	ExportedAt    time.Time             `json:"exported_at"`
	FormatVersion string                `json:"format_version"`
	User          *user.User            `json:"user"`
	APIKeys       []user.APIKey         `json:"api_keys"`
	LLMKeys       []llmkey.LLMKey       `json:"llm_keys"`
	Sessions      []run.Session         `json:"sessions"`
	Conversations []ConversationExport  `json:"conversations"`
	CostRecords   []run.Run             `json:"cost_records"`
	AuditTrail    []database.AuditEntry `json:"audit_trail"`
}

// GDPRService provides GDPR data export and deletion operations.
type GDPRService struct {
	store  database.Store
	tokens userTokenInvalidator
}

// userTokenInvalidator ends a user's sessions on this replica
// (TokenManager): an erased user's tokens stop working here at once and the
// user's WebSocket connections close (KI-143).
type userTokenInvalidator interface {
	EndUserSessions(userID string)
}

// SetTokenInvalidator sets what erasing a user tells about it.
func (s *GDPRService) SetTokenInvalidator(inv userTokenInvalidator) {
	s.tokens = inv
}

// NewGDPRService creates a new GDPR service backed by the given store.
func NewGDPRService(store database.Store) *GDPRService {
	return &GDPRService{store: store}
}

// ExportUserData collects all personal data associated with the given user ID.
// Returns a structured export suitable for JSON serialization (GDPR Article 20).
//
// Data is gathered through project ownership: all projects in the tenant are
// enumerated, then sessions/conversations/runs are collected per project.
// Audit trail entries are filtered to the specific user (admin_id match).
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

	// Collect sessions and conversations across all projects in the tenant.
	var allSessions []run.Session
	var allConversations []ConversationExport
	var allCostRecords []run.Run

	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		slog.Warn("gdpr export: failed to list projects", "user_id", userID, "error", err)
		projects = nil
	}

	for i := range projects {
		sessions, sErr := s.store.ListSessions(ctx, projects[i].ID)
		if sErr != nil {
			slog.Warn("gdpr export: failed to list sessions", "project_id", projects[i].ID, "error", sErr)
			continue
		}
		allSessions = append(allSessions, sessions...)

		conversations, cErr := s.store.ListConversationsByProject(ctx, projects[i].ID)
		if cErr != nil {
			slog.Warn("gdpr export: failed to list conversations", "project_id", projects[i].ID, "error", cErr)
			continue
		}
		for j := range conversations {
			msgs, mErr := s.store.ListMessages(ctx, conversations[j].ID)
			if mErr != nil {
				slog.Warn("gdpr export: failed to list messages", "conversation_id", conversations[j].ID, "error", mErr)
				msgs = []conversation.Message{}
			}
			allConversations = append(allConversations, ConversationExport{
				Conversation: conversations[j],
				Messages:     msgs,
			})
		}

		// Collect runs (cost records) from tasks in this project.
		tasks, tErr := s.store.ListTasks(ctx, projects[i].ID)
		if tErr != nil {
			slog.Warn("gdpr export: failed to list tasks", "project_id", projects[i].ID, "error", tErr)
			continue
		}
		for j := range tasks {
			runs, rErr := s.store.ListRunsByTask(ctx, tasks[j].ID)
			if rErr != nil {
				slog.Warn("gdpr export: failed to list runs", "task_id", tasks[j].ID, "error", rErr)
				continue
			}
			allCostRecords = append(allCostRecords, runs...)
		}
	}

	// Collect audit trail entries for this user.
	auditTrail, err := s.store.ListAuditEntriesByAdmin(ctx, userID, 10000)
	if err != nil {
		slog.Warn("gdpr export: failed to list audit trail", "user_id", userID, "error", err)
		auditTrail = []database.AuditEntry{}
	}

	return &UserDataExport{
		ExportedAt:    time.Now().UTC(),
		FormatVersion: "1.0",
		User:          u,
		APIKeys:       apiKeys,
		LLMKeys:       llmKeys,
		Sessions:      allSessions,
		Conversations: allConversations,
		CostRecords:   allCostRecords,
		AuditTrail:    auditTrail,
	}, nil
}

// DeleteUserData removes all personal data for the given user (GDPR Article 17
// - Right to Erasure), see eraseUser.
func (s *GDPRService) DeleteUserData(ctx context.Context, userID string) error {
	if s.tokens != nil {
		defer s.tokens.EndUserSessions(userID)
	}
	return eraseUser(ctx, s.store, userID)
}

// userErasureStore is what erasing a user needs.
type userErasureStore interface {
	AnonymizeAuditLogForUser(ctx context.Context, userID string) (int64, error)
	AnonymizeConsentsForUser(ctx context.Context, userID string) (int64, error)
	AnonymizeChannelMessagesForUser(ctx context.Context, userID string) (int64, error)
	AnonymizeQuarantineReviewsForUser(ctx context.Context, userID string) (int64, error)
	DeleteUser(ctx context.Context, id string) error
}

// eraseUser is the one way a user is deleted, whether through the GDPR
// endpoints or account deletion: rows that outlive the user keep their content
// without the user's personal data (ADR-009) - audit entries lose email and IP
// address, consent records (proof of consent) lose IP address and user agent,
// channel messages get a placeholder sender name, quarantine reviews a
// placeholder reviewer name. These run first, while the rows can still be
// found by the user's ID; if one fails, the user is not
// deleted and the erasure can be retried. Deleting the user then removes the
// dependent rows (ON DELETE CASCADE) and unlinks the kept ones (ON DELETE SET
// NULL).
func eraseUser(ctx context.Context, store userErasureStore, userID string) error {
	steps := []struct {
		name      string
		anonymize func(ctx context.Context, userID string) (int64, error)
	}{
		{"audit_log", store.AnonymizeAuditLogForUser},
		{"user_consents", store.AnonymizeConsentsForUser},
		{"channel_messages", store.AnonymizeChannelMessagesForUser},
		{"quarantine_reviews", store.AnonymizeQuarantineReviewsForUser},
	}
	for _, step := range steps {
		n, err := step.anonymize(ctx, userID)
		if err != nil {
			return fmt.Errorf("anonymize %s: %w", step.name, err)
		}
		slog.Info("gdpr: personal data anonymized", "user_id", userID, "table", step.name, "rows", n)
	}

	if err := store.DeleteUser(ctx, userID); err != nil {
		return fmt.Errorf("delete user data: %w", err)
	}
	slog.Info("gdpr: user data deleted", "user_id", userID)
	return nil
}
