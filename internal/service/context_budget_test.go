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

func TestComplexityBudget(t *testing.T) {
	tests := []struct {
		name       string
		tier       string
		baseBudget int
		want       int
	}{
		{"simple tier scales to 0.25x", "simple", 2048, 512},
		{"medium tier scales to 1.0x", "medium", 2048, 2048},
		{"complex tier scales to 2.0x", "complex", 2048, 4096},
		{"reasoning tier scales to 2.0x", "reasoning", 2048, 4096},
		{"unknown tier defaults to 1.0x", "unknown", 2048, 2048},
		{"zero base returns zero", "complex", 0, 0},
		{"negative base returns zero", "medium", -100, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComplexityBudget(tt.tier, tt.baseBudget)
			if got != tt.want {
				t.Errorf("ComplexityBudget(%q, %d) = %d, want %d", tt.tier, tt.baseBudget, got, tt.want)
			}
		})
	}
}

func TestComplexityBudget_CompositeWithPhaseAndTurn(t *testing.T) {
	// Composite test: tier=complex (2.0x) + phase=reviewer (50%) + turn 30 (50% decay)
	baseBudget := 2048

	// Step 1: Apply complexity scaling
	complexityScaled := ComplexityBudget("complex", baseBudget) // expect 4096

	// Step 2: Apply phase scaling
	phaseScaled := PhaseAwareContextBudget(complexityScaled, "reviewer") // expect 2048

	// Step 3: Apply turn decay (30 messages out of 60 threshold = 50% remaining)
	history := make([]messagequeue.ConversationMessagePayload, 30)
	for i := range history {
		history[i] = messagequeue.ConversationMessagePayload{Role: "user", Content: "msg"}
	}
	final := AdaptiveContextBudget(phaseScaled, history) // 2048 * 50% = 1024

	if final != 1024 {
		t.Errorf("Composite(complex, reviewer, turn 30) = %d, want 1024", final)
	}
}

func TestComplexityBudget_CompositeSimpleNoPhaseFirstTurn(t *testing.T) {
	// Composite test: tier=simple (0.25x) + phase="" (100%) + turn 0 (100%)
	baseBudget := 2048

	// Step 1: Apply complexity scaling
	complexityScaled := ComplexityBudget("simple", baseBudget) // expect 512

	// Step 2: Apply phase scaling (unknown mode = 100%)
	phaseScaled := PhaseAwareContextBudget(complexityScaled, "") // expect 512

	// Step 3: Apply turn decay (0 messages = 100%)
	final := AdaptiveContextBudget(phaseScaled, nil) // expect 512

	if final != 512 {
		t.Errorf("Composite(simple, no-phase, turn 0) = %d, want 512", final)
	}
}

func TestPhaseAwareContextBudget(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		base    int
		wantPct int
	}{
		{"boundary_analyzer gets full", "boundary_analyzer", 2000, 100},
		{"contract_reviewer gets 60%", "contract_reviewer", 2000, 60},
		{"reviewer gets 50%", "reviewer", 2000, 50},
		{"refactorer gets 70%", "refactorer", 2000, 70},
		{"unknown gets full", "coder", 2000, 100},
		{"zero base returns zero", "reviewer", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PhaseAwareContextBudget(tt.base, tt.mode)
			want := tt.base * tt.wantPct / 100
			if got != want {
				t.Errorf("PhaseAwareContextBudget(%d, %q) = %d, want %d", tt.base, tt.mode, got, want)
			}
		})
	}
}
