package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/autoagent"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// TestAutoAgentWait_StopsTheRunItGivesUpOn: when the auto-agent stops
// waiting for a feature's run (timeout or stop), the run is stopped too
// (KI-76): it kept changing the workspace next to the next feature's run, and
// its conversation refused every message with 409 until it ended.
func TestAutoAgentWait_StopsTheRunItGivesUpOn(t *testing.T) {
	store := newAutoAgentMockStore()
	queue := &mockQueue{}
	convSvc := NewConversationService(store, &noopBroadcaster{}, "test-model", nil)
	convSvc.SetQueue(queue)
	svc := NewAutoAgentService(store, &noopBroadcaster{}, queue, convSvc)
	conv, err := store.CreateConversation(context.Background(), &conversation.Conversation{ProjectID: "proj-1", Title: "feature"})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	waiter, err := convSvc.ExpectCompletion(conv.ID)
	if err != nil {
		t.Fatalf("ExpectCompletion: %v", err)
	}
	defer waiter.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the auto-agent is stopped while the run goes on
	if err := svc.waitForCompletion(ctx, conv.ID, waiter, &autoagent.AutoAgent{}); err == nil {
		t.Fatal("waitForCompletion returned no error for a run it gave up on")
	}

	stopped := false
	for _, msg := range queue.published {
		if msg.subject != messagequeue.SubjectConversationRunCancel {
			continue
		}
		var cancelMsg struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal(msg.data, &cancelMsg); err == nil && cancelMsg.RunID == conv.ID {
			stopped = true
		}
	}
	if !stopped {
		t.Fatal("the run the auto-agent gave up on was not stopped")
	}
}

// TestAutoAgentRun_IsAnImplementationTurn: the auto-agent's feature turns
// are implementation turns, offered no planning tools (KI-153).
func TestAutoAgentRun_IsAnImplementationTurn(t *testing.T) {
	store := newAutoAgentMockStore()
	seedProject(store, "proj-1", t.TempDir())
	queue := &mockQueue{}
	convSvc := NewConversationService(store, &noopBroadcaster{}, "test-model", nil)
	convSvc.SetQueue(queue)
	svc := NewAutoAgentService(store, &noopBroadcaster{}, queue, convSvc)
	conv, err := store.CreateConversation(context.Background(), &conversation.Conversation{ProjectID: "proj-1", Title: "feature"})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = svc.runAndWait(ctx, conv.ID, "Implement the feature", &autoagent.AutoAgent{}) // no worker: it times out

	var starts int
	for _, msg := range queue.published {
		if msg.subject != messagequeue.SubjectConversationRunStart {
			continue
		}
		starts++
		var start messagequeue.ConversationRunStartPayload
		if err := json.Unmarshal(msg.data, &start); err != nil {
			t.Fatal(err)
		}
		if !start.ImplementationTurn {
			t.Fatal("the auto-agent's turn is not an implementation turn")
		}
	}
	if starts != 1 {
		t.Fatalf("run starts = %d, want 1", starts)
	}
}
