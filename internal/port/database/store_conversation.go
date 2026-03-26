package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
)

// ConversationStore defines database operations for conversations and messages.
type ConversationStore interface {
	CreateConversation(ctx context.Context, c *conversation.Conversation) (*conversation.Conversation, error)
	GetConversation(ctx context.Context, id string) (*conversation.Conversation, error)
	ListConversationsByProject(ctx context.Context, projectID string) ([]conversation.Conversation, error)
	DeleteConversation(ctx context.Context, id string) error
	CreateMessage(ctx context.Context, m *conversation.Message) (*conversation.Message, error)
	CreateToolMessages(ctx context.Context, conversationID string, msgs []conversation.Message) error
	ListMessages(ctx context.Context, conversationID string) ([]conversation.Message, error)
	DeleteConversationMessages(ctx context.Context, conversationID string) error
	UpdateConversationMode(ctx context.Context, conversationID, mode string) error
	UpdateConversationModel(ctx context.Context, conversationID, model string) error
	SearchConversationMessages(ctx context.Context, query string, projectIDs []string, limit int) ([]conversation.Message, error)

	// Retention
	DeleteExpiredConversations(ctx context.Context, before time.Time, batchSize int) (int64, error)
}
