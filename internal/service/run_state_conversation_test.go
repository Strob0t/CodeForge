package service

import (
	"testing"
)

// The conversation run state holds a conversation's active run (one at a
// time), the cancel mark of a stopped run, and nothing once neither exists
// (review 2, findings 5, 8 and 12).

func (m *RunStateManager) conversationEntries() int {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	return len(m.convRuns)
}

func TestRunStateManager_ConversationRuns(t *testing.T) {
	const conv = "conv-1"
	type check struct {
		active    string // "" = no active run
		cancelled bool
		entries   int
	}
	tests := []struct {
		name  string
		steps func(m *RunStateManager)
		want  check
	}{
		{
			name:  "a dispatched run is active",
			steps: func(m *RunStateManager) { m.BeginConversationRun(conv, "t1"); m.ConversationRunDispatched(conv, "t1") },
			want:  check{active: "t1", entries: 1},
		},
		{
			name: "its completion ends it and leaves nothing",
			steps: func(m *RunStateManager) {
				m.BeginConversationRun(conv, "t1")
				m.ConversationRunDispatched(conv, "t1")
				m.EndConversationRun(conv, "t1")
			},
			want: check{},
		},
		{
			name: "a completion without turn ends the active run",
			steps: func(m *RunStateManager) {
				m.BeginConversationRun(conv, "t1")
				m.ConversationRunDispatched(conv, "t1")
				m.EndConversationRun(conv, "")
			},
			want: check{},
		},
		{
			name: "a completion of another turn leaves the active run",
			steps: func(m *RunStateManager) {
				m.BeginConversationRun(conv, "t1")
				m.EndConversationRun(conv, "t0")
			},
			want: check{active: "t1", entries: 1},
		},
		{
			name:  "an aborted dispatch leaves nothing",
			steps: func(m *RunStateManager) { m.BeginConversationRun(conv, "t1"); m.AbortConversationRun(conv, "t1") },
			want:  check{},
		},
		{
			name: "a stop ends the active run and marks the conversation",
			steps: func(m *RunStateManager) {
				m.BeginConversationRun(conv, "t1")
				m.ConversationRunDispatched(conv, "t1")
				m.SetCancelledConversation(conv)
			},
			want: check{cancelled: true, entries: 1},
		},
		{
			name: "the stopped run's completion clears the mark",
			steps: func(m *RunStateManager) {
				m.BeginConversationRun(conv, "t1")
				m.ConversationRunDispatched(conv, "t1")
				m.SetCancelledConversation(conv)
				m.EndConversationRun(conv, "t1")
			},
			want: check{},
		},
		{
			name: "the next run keeps the mark until its start is published",
			steps: func(m *RunStateManager) {
				m.SetCancelledConversation(conv)
				m.BeginConversationRun(conv, "t2")
			},
			want: check{active: "t2", cancelled: true, entries: 1},
		},
		{
			name: "the next run's published start clears the mark",
			steps: func(m *RunStateManager) {
				m.SetCancelledConversation(conv)
				m.BeginConversationRun(conv, "t2")
				m.ConversationRunDispatched(conv, "t2")
			},
			want: check{active: "t2", entries: 1},
		},
		{
			name: "a stop while the next run is dispatched wins",
			steps: func(m *RunStateManager) {
				m.BeginConversationRun(conv, "t2")
				m.SetCancelledConversation(conv)
				m.ConversationRunDispatched(conv, "t2")
			},
			want: check{cancelled: true, entries: 1},
		},
		{
			name: "an aborted dispatch keeps the stop's mark",
			steps: func(m *RunStateManager) {
				m.SetCancelledConversation(conv)
				m.BeginConversationRun(conv, "t2")
				m.AbortConversationRun(conv, "t2")
			},
			want: check{cancelled: true, entries: 1},
		},
		{
			name: "deleting the conversation forgets everything",
			steps: func(m *RunStateManager) {
				m.BeginConversationRun(conv, "t1")
				m.SetCancelledConversation(conv)
				m.BeginConversationRun(conv, "t2")
				m.SetBypassedConversation(conv)
				m.ForgetConversation(conv)
			},
			want: check{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewRunStateManager()
			tc.steps(m)
			turn, active := m.ActiveConversationTurn(conv)
			if tc.want.active == "" && active || tc.want.active != "" && turn != tc.want.active {
				t.Errorf("active turn = %q (%v), want %q", turn, active, tc.want.active)
			}
			if got := m.IsConversationCancelled(conv); got != tc.want.cancelled {
				t.Errorf("cancelled = %v, want %v", got, tc.want.cancelled)
			}
			if got := m.conversationEntries(); got != tc.want.entries {
				t.Errorf("entries = %d, want %d", got, tc.want.entries)
			}
			if tc.name == "deleting the conversation forgets everything" && m.IsConversationBypassed(conv) {
				t.Error("bypass survived the delete")
			}
		})
	}
}

func TestRunStateManager_OneConversationRunAtATime(t *testing.T) {
	m := NewRunStateManager()
	if !m.BeginConversationRun("conv-1", "t1") {
		t.Fatal("first run refused")
	}
	if m.BeginConversationRun("conv-1", "t2") {
		t.Error("second run accepted while the first is active")
	}
	if !m.BeginConversationRun("conv-2", "t3") {
		t.Error("another conversation's run refused")
	}
	m.EndConversationRun("conv-1", "t1")
	if !m.BeginConversationRun("conv-1", "t4") {
		t.Error("next run refused after the first ended")
	}
}
