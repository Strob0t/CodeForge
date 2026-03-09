package service

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

func TestAdaptiveContextBudget(t *testing.T) {
	baseBudget := 2048

	tests := []struct {
		name         string
		historyCount int
		wantMin      int
		wantMax      int
	}{
		{
			name:         "first message gets full budget",
			historyCount: 0,
			wantMin:      baseBudget,
			wantMax:      baseBudget,
		},
		{
			name:         "single prior exchange keeps most of the budget",
			historyCount: 2,
			wantMin:      1900,
			wantMax:      baseBudget,
		},
		{
			name:         "moderate history reduces budget",
			historyCount: 20,
			wantMin:      512,
			wantMax:      baseBudget - 1,
		},
		{
			name:         "long history gets minimal budget",
			historyCount: 40,
			wantMin:      256,
			wantMax:      768,
		},
		{
			name:         "very long history gets zero",
			historyCount: 80,
			wantMin:      0,
			wantMax:      0,
		},
		{
			name:         "exactly at threshold gets zero",
			historyCount: 60,
			wantMin:      0,
			wantMax:      0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			history := make([]messagequeue.ConversationMessagePayload, tc.historyCount)
			for i := range history {
				if i%2 == 0 {
					history[i] = messagequeue.ConversationMessagePayload{Role: "user", Content: "hello"}
				} else {
					history[i] = messagequeue.ConversationMessagePayload{Role: "assistant", Content: "hi there"}
				}
			}
			got := AdaptiveContextBudget(baseBudget, history)
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("AdaptiveContextBudget(%d, %d msgs) = %d, want [%d, %d]",
					baseBudget, tc.historyCount, got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestAdaptiveContextBudget_NeverNegative(t *testing.T) {
	got := AdaptiveContextBudget(100, make([]messagequeue.ConversationMessagePayload, 1000))
	if got < 0 {
		t.Errorf("budget must never be negative, got %d", got)
	}
}

func TestAdaptiveContextBudget_ZeroBase(t *testing.T) {
	got := AdaptiveContextBudget(0, nil)
	if got != 0 {
		t.Errorf("zero base budget should return 0, got %d", got)
	}
}
