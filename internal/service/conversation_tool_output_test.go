package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// agent.tool_output_max_chars reaches the worker with every conversation run,
// which truncates long tool results in the history with it (KI-38).
func TestConversationRunStart_CarriesToolOutputMaxChars(t *testing.T) {
	for _, starter := range conversationRunStarters {
		t.Run(starter.name, func(t *testing.T) {
			env := newConvStopEnv(t, nil, nil)
			env.conv.SetAgentConfig(&config.Agent{MaxLoopIterations: 10, ToolOutputMaxChars: 1234})

			if err := starter.start(context.Background(), env.conv, env.convID); err != nil {
				t.Fatalf("start: %v", err)
			}
			msg, ok := env.starts.lastMessage(messagequeue.SubjectConversationRunStart)
			if !ok {
				t.Fatal("no conversation run start published")
			}
			var start messagequeue.ConversationRunStartPayload
			if err := json.Unmarshal(msg.Data, &start); err != nil {
				t.Fatal(err)
			}
			if start.ToolOutputMaxChars != 1234 {
				t.Fatalf("tool_output_max_chars = %d, want 1234", start.ToolOutputMaxChars)
			}
		})
	}
}

// KI-153: only the auto-agent's feature turns are implementation turns; the
// worker then offers no planning tools. User turns keep them.
func TestConversationRunStart_ImplementationTurn(t *testing.T) {
	for _, implementation := range []bool{false, true} {
		env := newConvStopEnv(t, nil, nil)
		req := &conversation.SendMessageRequest{Content: "Implement the feature", ImplementationTurn: implementation}
		if err := env.conv.SendMessageAgentic(context.Background(), env.convID, req); err != nil {
			t.Fatalf("SendMessageAgentic: %v", err)
		}
		msg, ok := env.starts.lastMessage(messagequeue.SubjectConversationRunStart)
		if !ok {
			t.Fatal("no conversation run start published")
		}
		var start messagequeue.ConversationRunStartPayload
		if err := json.Unmarshal(msg.Data, &start); err != nil {
			t.Fatal(err)
		}
		if start.ImplementationTurn != implementation {
			t.Fatalf("implementation_turn = %v, want %v", start.ImplementationTurn, implementation)
		}
	}
	// The flag is not taken from the API body.
	var req conversation.SendMessageRequest
	if err := json.Unmarshal([]byte(`{"content":"x","ImplementationTurn":true,"implementation_turn":true}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.ImplementationTurn {
		t.Fatal("implementation_turn was read from the request body")
	}
}
