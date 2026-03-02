package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/service"
)

// convMockStore provides in-memory conversation and message storage.
type convMockStore struct {
	runtimeMockStore
	conversations []conversation.Conversation
	messages      []conversation.Message
}

func (m *convMockStore) CreateConversation(_ context.Context, c *conversation.Conversation) (*conversation.Conversation, error) {
	c.ID = fmt.Sprintf("conv-%d", len(m.conversations)+1)
	m.conversations = append(m.conversations, *c)
	return c, nil
}

func (m *convMockStore) GetConversation(_ context.Context, id string) (*conversation.Conversation, error) {
	for i := range m.conversations {
		if m.conversations[i].ID == id {
			return &m.conversations[i], nil
		}
	}
	return nil, errMockNotFound
}

func (m *convMockStore) ListConversationsByProject(_ context.Context, projectID string) ([]conversation.Conversation, error) {
	var result []conversation.Conversation
	for i := range m.conversations {
		if m.conversations[i].ProjectID == projectID {
			result = append(result, m.conversations[i])
		}
	}
	return result, nil
}

func (m *convMockStore) DeleteConversation(_ context.Context, id string) error {
	for i := range m.conversations {
		if m.conversations[i].ID == id {
			m.conversations = append(m.conversations[:i], m.conversations[i+1:]...)
			return nil
		}
	}
	return errMockNotFound
}

func (m *convMockStore) CreateMessage(_ context.Context, msg *conversation.Message) (*conversation.Message, error) {
	msg.ID = fmt.Sprintf("msg-%d", len(m.messages)+1)
	m.messages = append(m.messages, *msg)
	return msg, nil
}

func (m *convMockStore) ListMessages(_ context.Context, conversationID string) ([]conversation.Message, error) {
	var result []conversation.Message
	for i := range m.messages {
		if m.messages[i].ConversationID == conversationID {
			result = append(result, m.messages[i])
		}
	}
	return result, nil
}

func TestConversation_Create(t *testing.T) {
	store := &convMockStore{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, nil, bc, "gpt-4o", modes)
	ctx := context.Background()

	// Create with title
	conv, err := svc.Create(ctx, conversation.CreateRequest{
		ProjectID: "proj-1",
		Title:     "Test Chat",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if conv.ProjectID != "proj-1" {
		t.Errorf("expected ProjectID proj-1, got %s", conv.ProjectID)
	}
	if conv.Title != "Test Chat" {
		t.Errorf("expected title 'Test Chat', got %q", conv.Title)
	}

	// Empty title defaults to "New Conversation"
	conv2, err := svc.Create(ctx, conversation.CreateRequest{
		ProjectID: "proj-1",
		Title:     "",
	})
	if err != nil {
		t.Fatalf("Create default title: %v", err)
	}
	if conv2.Title != "New Conversation" {
		t.Errorf("expected default title 'New Conversation', got %q", conv2.Title)
	}

	// Missing project_id fails
	_, err = svc.Create(ctx, conversation.CreateRequest{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestConversation_SendMessage_EmptyContent(t *testing.T) {
	store := &convMockStore{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, nil, bc, "gpt-4o", modes)
	ctx := context.Background()

	// SendMessage with empty content should fail validation before touching LLM
	_, err := svc.SendMessage(ctx, "conv-1", conversation.SendMessageRequest{Content: ""})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestConversation_ListMessages(t *testing.T) {
	store := &convMockStore{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, nil, bc, "gpt-4o", modes)
	ctx := context.Background()

	// Create a conversation
	conv, err := svc.Create(ctx, conversation.CreateRequest{
		ProjectID: "proj-1",
		Title:     "Messages Test",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manually seed messages into mock store
	store.messages = append(store.messages,
		conversation.Message{ID: "m1", ConversationID: conv.ID, Role: "user", Content: "hello"},
		conversation.Message{ID: "m2", ConversationID: conv.ID, Role: "assistant", Content: "hi there"},
		conversation.Message{ID: "m3", ConversationID: "other-conv", Role: "user", Content: "unrelated"},
	)

	msgs, err := svc.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages for conv %s, got %d", conv.ID, len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("expected first message 'hello', got %q", msgs[0].Content)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("expected second message role 'assistant', got %q", msgs[1].Role)
	}
}
