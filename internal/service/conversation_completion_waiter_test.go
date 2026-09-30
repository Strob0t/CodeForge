package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-76: the auto-agent registered its completion waiter only after it had
// dispatched the run, so a run that ended fast was missed and the auto-agent
// waited for its 30 minute timeout. A waiter is registered before the run is
// dispatched.

func completeConversationRunOf(t *testing.T, svc *service.ConversationService, convID, status string) {
	t.Helper()
	payload := messagequeue.ConversationRunCompletePayload{RunID: convID, ConversationID: convID, Status: status, AssistantContent: "done"}
	if err := svc.HandleConversationRunComplete(context.Background(), "", makeRunCompletePayload(&payload)); err != nil {
		t.Fatalf("HandleConversationRunComplete: %v", err)
	}
}

func TestExpectCompletion_ARunThatEndsBeforeTheWaitIsNotMissed(t *testing.T) {
	svc, _, _ := newConvRunCompleteEnv()
	waiter, err := svc.ExpectCompletion("conv-fast")
	if err != nil {
		t.Fatalf("ExpectCompletion: %v", err)
	}
	defer waiter.Close()

	completeConversationRunOf(t, svc, "conv-fast", "completed") // before anyone waits

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := waiter.Wait(ctx)
	if err != nil || result.Status != "completed" {
		t.Fatalf("Wait = %+v, %v; want the completed run", result, err)
	}
}

func TestExpectCompletion_OneWaiterPerConversation(t *testing.T) {
	svc, _, _ := newConvRunCompleteEnv()
	waiter, err := svc.ExpectCompletion("conv-1")
	if err != nil {
		t.Fatalf("ExpectCompletion: %v", err)
	}
	if _, err := svc.ExpectCompletion("conv-1"); err == nil {
		t.Fatal("a second waiter for the conversation was registered")
	}
	waiter.Close()
	waiter.Close() // idempotent
	again, err := svc.ExpectCompletion("conv-1")
	if err != nil {
		t.Fatalf("ExpectCompletion after Close: %v", err)
	}
	again.Close()
}

// TestHandleConversationRunComplete_DoesNotBlockOnAFullWaiter: a second
// completion for a waiter that has not read the first one used to block the
// completion handler forever, holding the waiters' lock.
func TestHandleConversationRunComplete_DoesNotBlockOnAFullWaiter(t *testing.T) {
	svc, _, _ := newConvRunCompleteEnv()
	waiter, err := svc.ExpectCompletion("conv-busy")
	if err != nil {
		t.Fatalf("ExpectCompletion: %v", err)
	}
	defer waiter.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		completeConversationRunOf(t, svc, "conv-busy", "completed")
		completeConversationRunOf(t, svc, "conv-busy", "failed")
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the second completion blocked")
	}
	result, err := waiter.Wait(context.Background())
	if err != nil || result.Status != "completed" {
		t.Fatalf("Wait = %+v, %v; want the first completion", result, err)
	}
}

// TestStopConversation_EndsTheRun: a stop ends the conversation's run for
// the runtime too (the HTTP handler did that separately, other callers such
// as the auto-agent did not), so the next message is not refused.
func TestStopConversation_EndsTheRun(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	start := conversationRunStarters[0].start
	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := env.conv.StopConversation(ctx, env.convID); err != nil {
		t.Fatalf("StopConversation: %v", err)
	}
	if err := start(ctx, env.conv, env.convID); errors.Is(err, service.ErrConversationRunInProgress) || err != nil {
		t.Fatalf("next message after a stop: %v", err)
	}
}
