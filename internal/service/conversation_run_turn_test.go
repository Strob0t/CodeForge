package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A conversation runs one run at a time: a new run is refused while one is
// active (review 2, finding 5). The new run's turn is known before its start
// is published, so its first tool call is never taken for a call of a stopped
// run (finding 8). A run's completion ends it (finding 12).

// completeConversationRun reports the end of the conversation's run with turn.
func (e *convStopEnv) completeConversationRun(t *testing.T, turn string) {
	t.Helper()
	data, err := json.Marshal(messagequeue.ConversationRunCompletePayload{
		RunID: e.convID, ConversationID: e.convID, Status: "completed", AssistantContent: "done", TurnID: turn,
	})
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}
	if err := e.conv.HandleConversationRunComplete(context.Background(), messagequeue.SubjectConversationRunComplete, data); err != nil {
		t.Fatalf("HandleConversationRunComplete: %v", err)
	}
}

func (e *convStopEnv) runStarts() int {
	e.starts.mu.Lock()
	defer e.starts.mu.Unlock()
	n := 0
	for _, msg := range e.starts.messages {
		if msg.Subject == messagequeue.SubjectConversationRunStart {
			n++
		}
	}
	return n
}

func TestConversationRun_OneRunAtATime(t *testing.T) {
	for _, starter := range conversationRunStarters {
		t.Run(starter.name, func(t *testing.T) {
			env := newConvStopEnv(t, nil, nil)
			ctx := context.Background()

			if err := starter.start(ctx, env.conv, env.convID); err != nil {
				t.Fatalf("first run: %v", err)
			}
			first := env.lastTurn(t)
			messages, _ := env.conv.ListMessages(ctx, env.convID)

			err := starter.start(ctx, env.conv, env.convID)
			if !errors.Is(err, service.ErrConversationRunInProgress) || !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("second run while the first is active: err = %v, want ErrConversationRunInProgress", err)
			}
			if n := env.runStarts(); n != 1 {
				t.Errorf("run starts = %d, want 1", n)
			}
			if after, _ := env.conv.ListMessages(ctx, env.convID); len(after) != len(messages) {
				t.Errorf("messages = %d, want %d: the refused message is not stored", len(after), len(messages))
			}

			// The first run ends: the next one starts.
			env.completeConversationRun(t, first)
			if err := starter.start(ctx, env.conv, env.convID); err != nil {
				t.Fatalf("run after the first ended: %v", err)
			}
			// A stop ends it too.
			env.runtime.MarkConversationRunCancelled(env.convID)
			if err := starter.start(ctx, env.conv, env.convID); err != nil {
				t.Fatalf("run after a stop: %v", err)
			}
		})
	}
}

// TestConversationRun_DeleteForgetsTheRunState: deleting a conversation drops
// its run state (active run, stop mark).
func TestConversationRun_DeleteForgetsTheRunState(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	if err := conversationRunStarters[0].start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := env.conv.Delete(ctx, env.convID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := env.runtime.BeginConversationRun(env.convID, "turn-after-delete"); err != nil {
		t.Errorf("run state of the deleted conversation kept: %v", err)
	}
}

// TestConversationRun_LateCompletionOfAStoppedRun: the stopped run's
// completion, arriving while the next run is active, does not end the next
// run.
func TestConversationRun_LateCompletionOfAStoppedRun(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	ctx := context.Background()
	start := conversationRunStarters[0].start

	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := env.lastTurn(t)
	env.runtime.MarkConversationRunCancelled(env.convID)
	if err := start(ctx, env.conv, env.convID); err != nil {
		t.Fatalf("next run: %v", err)
	}
	next := env.lastTurn(t)

	env.completeConversationRun(t, first)
	if err := start(ctx, env.conv, env.convID); !errors.Is(err, service.ErrConversationRunInProgress) {
		t.Errorf("run while the next run is active: err = %v, want ErrConversationRunInProgress", err)
	}
	if resp := env.toolCallInTurn(t, "call-next", next); resp.Decision != "allow" {
		t.Errorf("call of the next run: %s (%s), want allow", resp.Decision, resp.Reason)
	}
}

// fastWorkerQueue plays a worker that makes its first tool call before the
// publish of the run start returns to the dispatcher.
type fastWorkerQueue struct {
	runtimeMockQueue
	env      *convStopEnv
	t        *testing.T
	decision string
	reason   string
}

func (q *fastWorkerQueue) PublishWithDedup(ctx context.Context, subject string, data []byte, key string) error {
	if err := q.runtimeMockQueue.PublishWithDedup(ctx, subject, data, key); err != nil {
		return err
	}
	q.firstCall(subject, data)
	return nil
}

func (q *fastWorkerQueue) Publish(ctx context.Context, subject string, data []byte) error {
	if err := q.runtimeMockQueue.Publish(ctx, subject, data); err != nil {
		return err
	}
	q.firstCall(subject, data)
	return nil
}

func (q *fastWorkerQueue) firstCall(subject string, data []byte) {
	if subject != messagequeue.SubjectConversationRunStart {
		return
	}
	var start messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &start); err != nil {
		q.t.Errorf("unmarshal run start: %v", err)
		return
	}
	resp := q.env.toolCallInTurn(q.t, "call-fast", start.TurnID)
	q.decision, q.reason = resp.Decision, resp.Reason
}

func TestConversationRun_FastFirstCallOfTheNextRun(t *testing.T) {
	for _, starter := range conversationRunStarters {
		t.Run(starter.name, func(t *testing.T) {
			queue := &fastWorkerQueue{t: t}
			env := newConvStopEnv(t, nil, queue)
			queue.env = env
			env.runtime.MarkConversationRunCancelled(env.convID) // an earlier run was stopped

			if err := starter.start(context.Background(), env.conv, env.convID); err != nil {
				t.Fatalf("start: %v", err)
			}
			if queue.decision != "allow" {
				t.Errorf("first call of the new run: %s (%s), want allow", queue.decision, queue.reason)
			}
		})
	}
}

// flakyStartQueue fails the first run start publish, then works.
type flakyStartQueue struct {
	runtimeMockQueue
	failed bool
}

func (q *flakyStartQueue) PublishWithDedup(ctx context.Context, subject string, data []byte, key string) error {
	if subject == messagequeue.SubjectConversationRunStart && !q.failed {
		q.failed = true
		return errors.New("nats unavailable")
	}
	return q.runtimeMockQueue.PublishWithDedup(ctx, subject, data, key)
}

func (q *flakyStartQueue) Publish(ctx context.Context, subject string, data []byte) error {
	if subject == messagequeue.SubjectConversationRunStart && !q.failed {
		q.failed = true
		return errors.New("nats unavailable")
	}
	return q.runtimeMockQueue.Publish(ctx, subject, data)
}

// TestConversationRun_FailedDispatchReleasesTheConversation: a run whose
// start could not be published is not active; the next run starts, and until
// then a stop's mark still holds.
func TestConversationRun_FailedDispatchReleasesTheConversation(t *testing.T) {
	for _, starter := range conversationRunStarters {
		t.Run(starter.name, func(t *testing.T) {
			env := newConvStopEnv(t, nil, &flakyStartQueue{})
			env.runtime.MarkConversationRunCancelled(env.convID)

			if err := starter.start(context.Background(), env.conv, env.convID); err == nil {
				t.Fatal("first start: want the publish error")
			}
			if resp := env.toolCall(t, "call-of-stopped-run"); resp.Decision != "deny" {
				t.Errorf("call after the failed dispatch: %s, want deny (the stop's mark holds)", resp.Decision)
			}
			if err := starter.start(context.Background(), env.conv, env.convID); err != nil {
				t.Fatalf("next start: %v", err)
			}
		})
	}
}
