package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// S6-G review, item 12: agent.tool_output_max_chars reaches the worker with
// every run start too (it already did with conversation runs); 0 leaves the
// worker's default.
func TestRunStart_CarriesToolOutputMaxChars(t *testing.T) {
	for _, maxChars := range []int{0, 1234} {
		svc, _, queue, _ := newRuntimeTestEnv()
		svc.SetToolOutputMaxChars(maxChars)
		if _, err := svc.StartRun(context.Background(), &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"}); err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		msg, ok := queue.lastMessage(messagequeue.SubjectRunStart)
		if !ok {
			t.Fatal("no runs.start published")
		}
		var start messagequeue.RunStartPayload
		if err := json.Unmarshal(msg.Data, &start); err != nil {
			t.Fatal(err)
		}
		if start.ToolOutputMaxChars != maxChars {
			t.Fatalf("tool_output_max_chars = %d, want %d", start.ToolOutputMaxChars, maxChars)
		}
	}
}
