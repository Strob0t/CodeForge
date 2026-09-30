package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
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
