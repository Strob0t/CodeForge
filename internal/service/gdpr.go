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
	store database.Store
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
