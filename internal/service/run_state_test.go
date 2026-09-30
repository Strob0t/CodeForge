package service

import (
	"fmt"
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
