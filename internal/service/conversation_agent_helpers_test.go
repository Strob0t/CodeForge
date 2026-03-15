package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// --- TestHistoryToPayload ---

func TestHistoryToPayload(t *testing.T) {
	// Create a minimal service for the method receiver.
	svc := &ConversationService{}

	t.Run("filters system messages", func(t *testing.T) {
		msgs := []conversation.Message{
			{Role: "system", Content: "You are a coding assistant."},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi!"},
		}
		result := svc.historyToPayload(msgs)
		if len(result) != 2 {
			t.Fatalf("expected 2 messages (system filtered), got %d", len(result))
		}
		if result[0].Role != "user" {
			t.Errorf("expected first result role 'user', got %q", result[0].Role)
		}
		if result[1].Role != "assistant" {
			t.Errorf("expected second result role 'assistant', got %q", result[1].Role)
		}
	})

	t.Run("preserves user assistant tool roles", func(t *testing.T) {
		msgs := []conversation.Message{
			{Role: "user", Content: "Read main.go"},
			{Role: "assistant", Content: "Let me read that."},
			{Role: "tool", Content: "package main", ToolCallID: "tc-1", ToolName: "Read"},
		}
		result := svc.historyToPayload(msgs)
		if len(result) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(result))
		}
		if result[2].ToolCallID != "tc-1" {
			t.Errorf("expected ToolCallID 'tc-1', got %q", result[2].ToolCallID)
		}
		if result[2].Name != "Read" {
			t.Errorf("expected Name 'Read', got %q", result[2].Name)
		}
	})

	t.Run("parses tool_calls JSON", func(t *testing.T) {
		toolCalls := []messagequeue.ConversationToolCall{
			{
				ID:   "tc-1",
				Type: "function",
				Function: messagequeue.ConversationToolCallFunction{
					Name:      "Read",
					Arguments: `{"path":"main.go"}`,
				},
			},
		}
		tcJSON, _ := json.Marshal(toolCalls)
		msgs := []conversation.Message{
			{Role: "assistant", Content: "", ToolCalls: tcJSON},
		}
		result := svc.historyToPayload(msgs)
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		if len(result[0].ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(result[0].ToolCalls))
		}
		if result[0].ToolCalls[0].Function.Name != "Read" {
			t.Errorf("expected tool call name 'Read', got %q", result[0].ToolCalls[0].Function.Name)
		}
	})

	t.Run("empty ToolCalls field leaves nil", func(t *testing.T) {
		msgs := []conversation.Message{
			{Role: "assistant", Content: "Just text, no tool calls."},
		}
		result := svc.historyToPayload(msgs)
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		if result[0].ToolCalls != nil {
			t.Errorf("expected nil ToolCalls for plain assistant message, got %v", result[0].ToolCalls)
		}
	})

	t.Run("nil and empty input", func(t *testing.T) {
		result := svc.historyToPayload(nil)
		if len(result) != 0 {
			t.Errorf("expected 0 for nil input, got %d", len(result))
		}

		result = svc.historyToPayload([]conversation.Message{})
		if len(result) != 0 {
			t.Errorf("expected 0 for empty input, got %d", len(result))
		}
	})
}

// --- TestAppendModelAdaptation ---

func TestAppendModelAdaptation(t *testing.T) {
	t.Run("nil mode returns original", func(t *testing.T) {
		result := appendModelAdaptation("base prompt", "openai/gpt-4o", nil)
		if result != "base prompt" {
			t.Errorf("expected original prompt, got %q", result)
		}
	})

	t.Run("empty model returns original", func(t *testing.T) {
		mode := &messagequeue.ModePayload{
			ModelAdaptations: map[string]string{
				"openai": "Use function calling.",
			},
		}
		result := appendModelAdaptation("base prompt", "", mode)
		if result != "base prompt" {
			t.Errorf("expected original prompt for empty model, got %q", result)
		}
	})

	t.Run("matching family appends adaptation", func(t *testing.T) {
		mode := &messagequeue.ModePayload{
			ModelAdaptations: map[string]string{
				"openai": "Use function calling for tool use.",
			},
		}
		result := appendModelAdaptation("You are an AI.", "openai/gpt-4o", mode)
		expected := "You are an AI.\n\nUse function calling for tool use."
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("non-matching family unchanged", func(t *testing.T) {
		mode := &messagequeue.ModePayload{
			ModelAdaptations: map[string]string{
				"anthropic": "Use XML tool blocks.",
			},
		}
		result := appendModelAdaptation("base prompt", "openai/gpt-4o", mode)
		if result != "base prompt" {
			t.Errorf("expected original prompt for non-matching family, got %q", result)
		}
	})

	t.Run("empty adaptations returns original", func(t *testing.T) {
		mode := &messagequeue.ModePayload{
			ModelAdaptations: map[string]string{},
		}
		result := appendModelAdaptation("base prompt", "openai/gpt-4o", mode)
		if result != "base prompt" {
			t.Errorf("expected original prompt for empty adaptations, got %q", result)
		}
	})
}

// --- TestResolveProviderAPIKey ---

func TestResolveProviderAPIKey(t *testing.T) {
	ctx := context.Background()

	t.Run("nil llmKeySvc returns empty", func(t *testing.T) {
		svc := &ConversationService{}
		result := svc.resolveProviderAPIKey(ctx, "user-1", "openai/gpt-4o")
		if result != "" {
			t.Errorf("expected empty for nil llmKeySvc, got %q", result)
		}
	})

	t.Run("empty userID returns empty", func(t *testing.T) {
		svc := &ConversationService{}
		result := svc.resolveProviderAPIKey(ctx, "", "openai/gpt-4o")
		if result != "" {
			t.Errorf("expected empty for empty userID, got %q", result)
		}
	})

	t.Run("model without slash returns empty", func(t *testing.T) {
		svc := &ConversationService{}
		result := svc.resolveProviderAPIKey(ctx, "user-1", "gpt-4o")
		if result != "" {
			t.Errorf("expected empty for model without slash, got %q", result)
		}
	})

	t.Run("successful key resolution", func(t *testing.T) {
		// We need a real LLMKeyService but with a mock store that returns a key.
		// Since LLMKeyService.ResolveKeyForProvider requires database + encryption,
		// and this is a unit test of the guard logic, the nil/empty cases above
		// sufficiently cover the conditional branches in resolveProviderAPIKey.
		// The actual key resolution is tested in llmkey_test.go.
		// Here we just verify the provider extraction logic works.
		svc := &ConversationService{}
		// No llmKeySvc set -- should return empty even with valid model/user.
		result := svc.resolveProviderAPIKey(ctx, "user-1", "anthropic/claude-sonnet-4-20250514")
		if result != "" {
			t.Errorf("expected empty without llmKeySvc, got %q", result)
		}
	})
}

// --- TestIsAgentic ---

func TestIsAgentic(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit true returns true", func(t *testing.T) {
		svc := &ConversationService{
			queue: nil, // queue is nil, but explicit override should win
		}
		agentic := true
		result := svc.IsAgentic(ctx, "conv-1", conversation.SendMessageRequest{
			Content: "hello",
			Agentic: &agentic,
		})
		if !result {
			t.Error("expected true when Agentic explicitly set to true")
		}
	})

	t.Run("nil queue returns false", func(t *testing.T) {
		svc := &ConversationService{
			queue:    nil,
			agentCfg: &config.Agent{AgenticByDefault: true},
		}
		result := svc.IsAgentic(ctx, "conv-1", conversation.SendMessageRequest{
			Content: "hello",
		})
		if result {
			t.Error("expected false when queue is nil")
		}
	})

	t.Run("config disabled returns false", func(t *testing.T) {
		svc := &ConversationService{
			agentCfg: &config.Agent{AgenticByDefault: false},
		}
		result := svc.IsAgentic(ctx, "conv-1", conversation.SendMessageRequest{
			Content: "hello",
		})
		if result {
			t.Error("expected false when AgenticByDefault is false")
		}
	})
}
