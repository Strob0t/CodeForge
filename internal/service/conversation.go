package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// ConversationService manages conversations and LLM interactions.
type ConversationService struct {
	db    database.Store
	llm   *litellm.Client
	hub   broadcast.Broadcaster
	model string // default model name for LiteLLM
}

// NewConversationService creates a new ConversationService.
func NewConversationService(db database.Store, llm *litellm.Client, hub broadcast.Broadcaster, defaultModel string) *ConversationService {
	if defaultModel == "" {
		defaultModel = "default"
	}
	return &ConversationService{db: db, llm: llm, hub: hub, model: defaultModel}
}

// Create creates a new conversation for a project.
func (s *ConversationService) Create(ctx context.Context, req conversation.CreateRequest) (*conversation.Conversation, error) {
	if req.ProjectID == "" {
		return nil, errors.New("project_id is required")
	}
	c := &conversation.Conversation{
		ProjectID: req.ProjectID,
		Title:     req.Title,
	}
	if c.Title == "" {
		c.Title = "New Conversation"
	}
	return s.db.CreateConversation(ctx, c)
}

// Get returns a conversation by ID.
func (s *ConversationService) Get(ctx context.Context, id string) (*conversation.Conversation, error) {
	return s.db.GetConversation(ctx, id)
}

// ListByProject returns all conversations for a project.
func (s *ConversationService) ListByProject(ctx context.Context, projectID string) ([]conversation.Conversation, error) {
	return s.db.ListConversationsByProject(ctx, projectID)
}

// Delete removes a conversation.
func (s *ConversationService) Delete(ctx context.Context, id string) error {
	return s.db.DeleteConversation(ctx, id)
}

// ListMessages returns all messages in a conversation.
func (s *ConversationService) ListMessages(ctx context.Context, conversationID string) ([]conversation.Message, error) {
	return s.db.ListMessages(ctx, conversationID)
}

// SendMessage stores the user message, calls LiteLLM, stores the assistant response,
// and broadcasts it via WebSocket AG-UI events.
func (s *ConversationService) SendMessage(ctx context.Context, conversationID string, req conversation.SendMessageRequest) (*conversation.Message, error) {
	if req.Content == "" {
		return nil, errors.New("content is required")
	}

	// Verify conversation exists
	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	// Store user message
	userMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        req.Content,
	}
	if _, err = s.db.CreateMessage(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("store user message: %w", err)
	}

	// Build chat messages from history
	messages, err := s.db.ListMessages(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	chatMessages := make([]litellm.ChatMessage, 0, len(messages)+1)
	// Add system prompt
	chatMessages = append(chatMessages, litellm.ChatMessage{
		Role:    "system",
		Content: fmt.Sprintf("You are an AI coding assistant for the project. Help the user with their development tasks. Project ID: %s", conv.ProjectID),
	})
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		chatMessages = append(chatMessages, litellm.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// Broadcast run started event
	s.hub.BroadcastEvent(ctx, ws.AGUIRunStarted, ws.AGUIRunStartedEvent{
		RunID:     conversationID,
		ThreadID:  conversationID,
		AgentName: "assistant",
	})

	// Call LiteLLM (non-streaming for now; streaming will be added in Phase 8)
	llmResp, err := s.llm.ChatCompletion(ctx, litellm.ChatCompletionRequest{
		Model:    s.model,
		Messages: chatMessages,
	})
	if err != nil {
		slog.Error("llm chat completion failed", "conversation_id", conversationID, "error", err)
		// Broadcast run finished with error
		s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
			RunID:  conversationID,
			Status: "failed",
		})
		return nil, fmt.Errorf("llm completion: %w", err)
	}

	// Store assistant message
	assistantMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        llmResp.Content,
		TokensIn:       llmResp.TokensIn,
		TokensOut:      llmResp.TokensOut,
		Model:          llmResp.Model,
	}
	assistantMsg, err = s.db.CreateMessage(ctx, assistantMsg)
	if err != nil {
		return nil, fmt.Errorf("store assistant message: %w", err)
	}

	// Broadcast the response via AG-UI text message event
	s.hub.BroadcastEvent(ctx, ws.AGUITextMessage, ws.AGUITextMessageEvent{
		RunID:   conversationID,
		Role:    "assistant",
		Content: llmResp.Content,
	})

	// Broadcast run finished
	s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
		RunID:  conversationID,
		Status: "completed",
	})

	return assistantMsg, nil
}
