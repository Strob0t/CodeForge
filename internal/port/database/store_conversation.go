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

	// Active turn (KI-65): the conversation's active run, set before its start
	// is published and ended when it ends or is stopped (turnID "" ends any
	// turn; EndConversationTurn reports whether it ended the active turn). A
	// heartbeat counts only for the active turn. The list spans all tenants
	// (watchdog use) and holds only turns that had a heartbeat.
	BeginConversationTurn(ctx context.Context, conversationID, turnID string) error
	EndConversationTurn(ctx context.Context, conversationID, turnID string) (bool, error)
	// ClaimConversationTurnCompletion records that the worker's completion
	// of turn turnID is being kept and reports whether this call claimed it:
	// false for a repeated completion, or a conversation of another tenant.
	ClaimConversationTurnCompletion(ctx context.Context, conversationID, turnID string) (bool, error)
	TouchConversationTurnHeartbeat(ctx context.Context, conversationID, turnID string) error
	ListConversationTurnsWithStaleHeartbeat(ctx context.Context, idleFor time.Duration, limit int) ([]conversation.ActiveTurn, error)

	// ProjectHasOtherActiveWork reports whether a run or task of the project
	// is running, or a conversation of the project other than
	// conversationID has an active turn (KI-195: a multi-rollout turn resets
	// the workspace between rollouts).
	ProjectHasOtherActiveWork(ctx context.Context, projectID, conversationID string) (bool, error)
}
