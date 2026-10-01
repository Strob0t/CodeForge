package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
)

// TestRunStateManager_ClearCancelledConversation covers the cancel flag of a
// conversation run: a stop sets it, the start of the next run clears it (KI-24).
func TestRunStateManager_ClearCancelledConversation(t *testing.T) {
	tests := []struct {
		name   string
		cancel []string
		clear  string
		want   map[string]bool
	}{
		{
			name:   "clears the cancelled conversation",
			cancel: []string{"conv-1"},
			clear:  "conv-1",
			want:   map[string]bool{"conv-1": false},
		},
		{
			name:   "leaves other conversations cancelled",
			cancel: []string{"conv-1", "conv-2"},
			clear:  "conv-1",
			want:   map[string]bool{"conv-1": false, "conv-2": true},
		},
		{
			name:  "clearing a conversation that was never cancelled is a no-op",
			clear: "conv-1",
			want:  map[string]bool{"conv-1": false},
		},
		{
			name:   "empty id",
			cancel: []string{""},
			clear:  "",
			want:   map[string]bool{"": false},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewRunStateManager()
			for _, id := range tc.cancel {
				m.SetCancelledConversation(id)
			}
			m.ClearCancelledConversation(tc.clear)
			m.ClearCancelledConversation(tc.clear) // idempotent
			for id, want := range tc.want {
				if got := m.IsConversationCancelled(id); got != want {
					t.Errorf("IsConversationCancelled(%q) = %v, want %v", id, got, want)
				}
			}
		})
	}
}

// TestRunStateManager_CancelledConversationsConcurrent runs stops, starts and
// lookups of the same and of different conversations concurrently; the race
// detector reports unsynchronized access.
func TestRunStateManager_CancelledConversationsConcurrent(t *testing.T) {
	m := NewRunStateManager()
	const workers = 16
	const rounds = 200

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			own := fmt.Sprintf("conv-%d", w)
			for range rounds {
				m.SetCancelledConversation("shared")
				m.SetCancelledConversation(own)
				_ = m.IsConversationCancelled("shared")
				m.ClearCancelledConversation("shared")
				m.ClearCancelledConversation(own)
				_ = m.IsConversationCancelled(own)
			}
		}()
	}
	wg.Wait()

	for w := range workers {
		if id := fmt.Sprintf("conv-%d", w); m.IsConversationCancelled(id) {
			t.Errorf("%s still cancelled after its last clear", id)
		}
	}
	m.SetCancelledConversation("shared")
	if !m.IsConversationCancelled("shared") {
		t.Error("shared conversation not cancelled after a stop")
	}
	m.ClearCancelledConversation("shared")
	if m.IsConversationCancelled("shared") {
		t.Error("shared conversation still cancelled after the next start")
	}
}

// TestStopDeferral_KeepsEveryKindInArrivalOrder (S2-G fix 2, 5): a stop
// kept one deferred worker message per run, so a quality gate result and a
// completion that both arrived during a stop overwrote each other, and a
// failed stop replayed only the last. Each kind is kept (a repeated message
// of a kind replaces its earlier copy in place) and replayed in arrival
// order.
func TestStopDeferral_KeepsEveryKindInArrivalOrder(t *testing.T) {
	m := NewRunStateManager()
	var replayed []string
	handle := func(name string) func(context.Context) error {
		return func(context.Context) error {
			replayed = append(replayed, name)
			return nil
		}
	}

	if m.DeferIfStopping("run-1", deferredCompletion, handle("completion")) {
		t.Fatal("a message was deferred without a stop")
	}
	m.BeginStop("run-1")
	if !m.DeferIfStopping("run-1", deferredCompletion, handle("completion")) ||
		!m.DeferIfStopping("run-1", deferredGateResult, handle("gate result")) ||
		!m.DeferIfStopping("run-1", deferredCompletion, handle("completion again")) {
		t.Fatal("a message during the stop was not deferred")
	}

	deferred := m.EndStop("run-1")
	if deferred == nil {
		t.Fatal("EndStop returned nothing to replay")
	}
	if err := deferred(context.Background()); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if want := []string{"completion again", "gate result"}; !slices.Equal(replayed, want) {
		t.Fatalf("replayed %v, want %v", replayed, want)
	}
	if m.EndStop("run-1") != nil {
		t.Fatal("the deferred messages were kept after the stop")
	}
}

// TestStopDeferral_ReplayJoinsErrors: a failed replay of one message does
// not skip the next.
func TestStopDeferral_ReplayJoinsErrors(t *testing.T) {
	m := NewRunStateManager()
	m.BeginStop("run-2")
	ran := false
	m.DeferIfStopping("run-2", deferredCompletion, func(context.Context) error { return errors.New("store down") })
	m.DeferIfStopping("run-2", deferredGateResult, func(context.Context) error { ran = true; return nil })

	if err := m.EndStop("run-2")(context.Background()); err == nil || !ran {
		t.Fatalf("replay = %v, gate result replayed = %v; want the error and the gate result replayed", err, ran)
	}
}
