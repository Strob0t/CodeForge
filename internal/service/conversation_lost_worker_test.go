package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-65: a conversation run is acked on accept (ADR-016); a worker that dies
// leaves the conversation "running", and every next message gets 409 until
// the user stops it. The conversation's active turn is stored before its
// start is published and ended with the run; the worker's heartbeats for the
// turn are recorded, and the stuck-work watchdog ends a turn whose heartbeats
// stopped through the normal completion path, as a failed run.

func TestConversationRun_StoresTheActiveTurn(t *testing.T) {
	for _, starter := range conversationRunStarters {
		t.Run(starter.name, func(t *testing.T) {
			env := newConvStopEnv(t, nil, nil)
			ctx := context.Background()

			if err := starter.start(ctx, env.conv, env.convID); err != nil {
				t.Fatalf("start: %v", err)
			}
			turn := env.lastTurn(t)
			if got := env.store.activeTurnOf(env.convID); got != turn {
				t.Fatalf("stored active turn = %q, want the run's %q", got, turn)
			}

			env.completeConversationRun(t, turn)
			if got := env.store.activeTurnOf(env.convID); got != "" {
				t.Fatalf("stored active turn after the run's completion = %q, want none", got)
			}

			if err := starter.start(ctx, env.conv, env.convID); err != nil {
				t.Fatalf("next start: %v", err)
			}
			if err := env.conv.StopConversation(ctx, env.convID); err != nil {
				t.Fatalf("StopConversation: %v", err)
			}
			if got := env.store.activeTurnOf(env.convID); got != "" {
				t.Fatalf("stored active turn after a stop = %q, want none", got)
			}
		})
	}
}

// TestConversationRun_LateCompletionKeepsTheNextStoredTurn: the completion
// of a stopped run does not end the next run's stored turn.
func TestConversationRun_LateCompletionKeepsTheNextStoredTurn(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	start := conversationRunStarters[0].start

	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := env.lastTurn(t)
	if err := env.conv.StopConversation(ctx, env.convID); err != nil {
		t.Fatalf("StopConversation: %v", err)
	}
	env.runtime.MarkConversationRunCancelled(env.convID)
	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("next run: %v", err)
	}
	next := env.lastTurn(t)

	env.completeConversationRun(t, first)
	if got := env.store.activeTurnOf(env.convID); got != next {
		t.Fatalf("stored active turn = %q, want the next run's %q", got, next)
	}
}

func TestConversationRun_FailedDispatchEndsTheStoredTurn(t *testing.T) {
	env := newConvStopEnv(t, nil, &flakyStartQueue{})
	if err := conversationRunStarters[0].start(context.Background(), env.conv, env.convID); err == nil {
		t.Fatal("start with a failing publish succeeded")
	}
	if got := env.store.activeTurnOf(env.convID); got != "" {
		t.Fatalf("stored active turn of an undispatched run = %q, want none", got)
	}
}

// TestStopConversation_UnknownConversation: a stop names a conversation of
// the caller's tenant (the store is tenant-scoped); another tenant's
// conversation is not found, and its run is not cancelled.
func TestStopConversation_UnknownConversation(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	err := env.conv.StopConversation(context.Background(), "conv-of-another-tenant")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("StopConversation = %v, want not found", err)
	}
	if _, ok := env.starts.lastMessage(messagequeue.SubjectConversationRunCancel); ok {
		t.Fatal("a cancel was published for a conversation the caller cannot see")
	}
}

// heartbeatOfTurn records the worker heartbeat of the conversation's run turn.
func (e *convStopEnv) heartbeatOfTurn(t *testing.T, convID, turn string) {
	t.Helper()
	if err := e.runtime.HandleHeartbeat(context.Background(), &messagequeue.RunHeartbeatPayload{RunID: convID, TurnID: turn}); err != nil {
		t.Fatalf("HandleHeartbeat: %v", err)
	}
}

func TestEndConversationRunsWithLostWorker(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	env.conv.SetRuntimeConfig(&config.Runtime{HeartbeatTimeout: 2 * time.Minute, HeartbeatInterval: 30 * time.Second})
	ctx := context.Background()
	start := conversationRunStarters[0].start

	newConversation := func(title string) string {
		t.Helper()
		c, err := env.conv.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: title})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return c.ID
	}
	lost, healthy, queued := env.convID, newConversation("healthy"), newConversation("queued")
	turns := map[string]string{}
	for _, id := range []string{lost, healthy, queued} {
		if err := start(ctx, env.conv, id); err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
		turns[id] = env.lastTurn(t)
	}
	env.heartbeatOfTurn(t, lost, turns[lost])
	env.heartbeatOfTurn(t, healthy, turns[healthy])
	env.store.setTurnHeartbeat(lost, time.Now().Add(-time.Hour))

	ended, err := env.conv.EndConversationRunsWithLostWorker(ctx)
	if err != nil {
		t.Fatalf("EndConversationRunsWithLostWorker: %v", err)
	}
	if ended != 1 {
		t.Fatalf("ended %d conversation runs, want 1", ended)
	}

	// The lost run ended as failed: the conversation takes its next message.
	if got := env.store.activeTurnOf(lost); got != "" {
		t.Errorf("stored turn of the lost run = %q, want none", got)
	}
	var finished *event.AGUIRunFinishedEvent
	for _, ev := range env.hub.snapshot() {
		if f, ok := ev.Data.(event.AGUIRunFinishedEvent); ok && ev.EventType == event.AGUIRunFinished && f.RunID == lost {
			finished = &f
		}
	}
	if finished == nil || finished.Status != "failed" || !strings.Contains(finished.Error, "heartbeat") {
		t.Fatalf("run finished event = %+v, want failed for a lost heartbeat", finished)
	}
	msg, ok := env.starts.lastMessage(messagequeue.SubjectConversationRunCancel)
	if !ok {
		t.Fatal("the worker was not told to stop the lost run")
	}
	var cancel struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(msg.Data, &cancel); err != nil || cancel.RunID != lost {
		t.Fatalf("conversation.run.cancel = %s, want %s", msg.Data, lost)
	}
	if err := start(ctx, env.conv, lost); err != nil {
		t.Fatalf("next message of the conversation whose run was lost: %v", err)
	}

	// A run that sends heartbeats and a run not accepted yet stay active.
	for _, id := range []string{healthy, queued} {
		if got := env.store.activeTurnOf(id); got != turns[id] {
			t.Errorf("%s: stored turn = %q, want %q", id, got, turns[id])
		}
		if err := start(ctx, env.conv, id); !errors.Is(err, service.ErrConversationRunInProgress) {
			t.Errorf("%s: next message = %v, want ErrConversationRunInProgress", id, err)
		}
	}
}

func TestEndConversationRunsWithLostWorker_DisabledWithoutHeartbeatTimeout(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	if err := conversationRunStarters[0].start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("start: %v", err)
	}
	turn := env.lastTurn(t)
	env.heartbeatOfTurn(t, env.convID, turn)
	env.store.setTurnHeartbeat(env.convID, time.Now().Add(-24*time.Hour))

	if ended, err := env.conv.EndConversationRunsWithLostWorker(ctx); err != nil || ended != 0 {
		t.Fatalf("EndConversationRunsWithLostWorker = %d, %v; want 0, nil", ended, err)
	}
	if got := env.store.activeTurnOf(env.convID); got != turn {
		t.Fatalf("stored turn = %q, want %q", got, turn)
	}
}
