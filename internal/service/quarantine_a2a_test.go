package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// Held inbound A2A tasks were never reconciled with their quarantine
// message (S2-G fix, 9): rejecting the message left the task submitted
// forever, and approving it published the prompt even after the caller had
// cancelled the task. The A2A task follows its message now.

// a2aQuarantineStore keeps quarantine messages and A2A tasks.
type a2aQuarantineStore struct {
	*mockQuarantineStore
	a2aTasks map[string]*a2adomain.A2ATask
}

func (s *a2aQuarantineStore) GetA2ATask(_ context.Context, id string) (*a2adomain.A2ATask, error) {
	t, ok := s.a2aTasks[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (s *a2aQuarantineStore) UpdateA2ATask(_ context.Context, t *a2adomain.A2ATask) error {
	cp := *t
	s.a2aTasks[t.ID] = &cp
	return nil
}

// heldA2AEnv holds the inbound A2A task a2a-held, whose prompt waits in the
// quarantine as message q-a2a.
func heldA2AEnv(t *testing.T, state a2adomain.TaskState) (*a2aQuarantineStore, *mockQueue, *QuarantineService) {
	t.Helper()
	payload, err := json.Marshal(messagequeue.A2ATaskCreatedPayload{TaskID: "a2a-held", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	store := &a2aQuarantineStore{mockQuarantineStore: newMockQuarantineStore(), a2aTasks: map[string]*a2adomain.A2ATask{}}
	store.messages["q-a2a"] = &quarantine.Message{ID: "q-a2a", Subject: messagequeue.SubjectA2ATaskCreated, Payload: payload, Status: quarantine.StatusPending}
	task := a2adomain.NewA2ATask("a2a-held")
	task.State = state
	store.a2aTasks[task.ID] = task
	queue := &mockQueue{}
	return store, queue, NewQuarantineService(store, queue, &mockBroadcaster{}, config.Quarantine{Enabled: true})
}

func TestQuarantine_RejectRejectsTheHeldA2ATask(t *testing.T) {
	store, _, svc := heldA2AEnv(t, a2adomain.TaskStateSubmitted)

	if err := svc.Reject(context.Background(), "q-a2a", "admin", "no"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if got := store.a2aTasks["a2a-held"].State; got != a2adomain.TaskStateRejected {
		t.Fatalf("a2a task state = %s, want rejected", got)
	}
}

func TestQuarantine_ApproveStartsTheHeldA2ATask(t *testing.T) {
	store, queue, svc := heldA2AEnv(t, a2adomain.TaskStateSubmitted)

	if err := svc.Approve(context.Background(), "q-a2a", "admin", "ok"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if len(queue.published) != 1 || queue.published[0].subject != messagequeue.SubjectA2ATaskCreated {
		t.Fatalf("published = %v, want the held prompt", queue.published)
	}
	if got := store.a2aTasks["a2a-held"].State; got != a2adomain.TaskStateWorking {
		t.Fatalf("a2a task state = %s, want working", got)
	}
}

// TestQuarantine_ApproveOfACancelledA2ATaskPublishesNothing: the caller
// cancelled its held task (and its withdrawal of the message did not
// happen): approving the message rejects it instead of publishing.
func TestQuarantine_ApproveOfACancelledA2ATaskPublishesNothing(t *testing.T) {
	for _, state := range []a2adomain.TaskState{a2adomain.TaskStateCanceled, a2adomain.TaskStateRejected} {
		t.Run(string(state), func(t *testing.T) {
			store, queue, svc := heldA2AEnv(t, state)

			err := svc.Approve(context.Background(), "q-a2a", "admin", "ok")
			if !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("Approve = %v, want ErrConflict", err)
			}
			if len(queue.published) != 0 {
				t.Fatalf("published = %v, want nothing", queue.published)
			}
			if got := store.messages["q-a2a"].Status; got != quarantine.StatusRejected {
				t.Fatalf("message status = %s, want rejected", got)
			}
		})
	}

	t.Run("task missing", func(t *testing.T) {
		store, queue, svc := heldA2AEnv(t, a2adomain.TaskStateSubmitted)
		delete(store.a2aTasks, "a2a-held")
		if err := svc.Approve(context.Background(), "q-a2a", "admin", "ok"); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("Approve = %v, want ErrConflict", err)
		}
		if len(queue.published) != 0 {
			t.Fatalf("published = %v, want nothing for a message without its task", queue.published)
		}
	})
}

// TestQuarantine_WithdrawnMessageIsNeverPublished: a caller's cancel
// withdraws its held prompt; an approval afterwards publishes nothing.
func TestQuarantine_WithdrawnMessageIsNeverPublished(t *testing.T) {
	store, queue, svc := heldA2AEnv(t, a2adomain.TaskStateSubmitted)

	if err := svc.Withdraw(context.Background(), "q-a2a", "cancelled by the A2A caller"); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if got := store.messages["q-a2a"].Status; got != quarantine.StatusRejected {
		t.Fatalf("message status = %s, want rejected", got)
	}
	if err := svc.Withdraw(context.Background(), "q-a2a", "again"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Withdraw(again) = %v, want ErrConflict", err)
	}
	if err := svc.Approve(context.Background(), "q-a2a", "admin", "ok"); err == nil {
		t.Fatal("a withdrawn message was approved")
	}
	if len(queue.published) != 0 {
		t.Fatalf("published = %v, want nothing", queue.published)
	}
}

// TestQuarantine_ScreenMessageNamesTheHeldMessage: the executor records the
// held message on its task, so the caller's cancel can withdraw it.
func TestQuarantine_ScreenMessageNamesTheHeldMessage(t *testing.T) {
	store := newMockQuarantineStore()
	svc := NewQuarantineService(store, &mockQueue{}, &mockBroadcaster{}, config.Quarantine{
		Enabled: true, QuarantineThreshold: 0.7, BlockThreshold: 0.95, MinTrustBypass: "verified", ExpiryHours: 72,
	})
	ann := &trust.Annotation{Origin: "a2a", TrustLevel: trust.LevelUntrusted, SourceID: "ext"}

	verdict, id, err := svc.ScreenMessage(context.Background(), ann, "run.start", []byte(`{"path":"../../etc/passwd"}`), "proj-1")
	if err != nil || verdict != quarantine.VerdictHeld {
		t.Fatalf("ScreenMessage = %s, %v; want held", verdict, err)
	}
	if msg, ok := store.messages[id]; !ok || msg.Status != quarantine.StatusPending {
		t.Fatalf("held message %q not stored pending", id)
	}
}
