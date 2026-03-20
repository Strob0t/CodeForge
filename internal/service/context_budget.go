package service

import "github.com/Strob0t/CodeForge/internal/port/messagequeue"

// contextDecayThreshold is the number of history messages at which
// the adaptive budget reaches zero. ~30 exchanges (user+assistant pairs).
const contextDecayThreshold = 60

// AdaptiveContextBudget calculates the context injection token budget
// based on conversation history length. Early turns get the full budget;
// as history grows the budget shrinks linearly to zero.
//
// Rationale: on turn 1 the agent knows nothing about the codebase and
// benefits most from pre-injected context (RepoMap, Retrieval, etc.).
// By turn 15+ the agent has read files and built its own context through
// tool calls, so injecting more wastes tokens.
// phaseContextScale maps review pipeline mode IDs to their context budget
// percentage. Focused phases (reviewer, contract_reviewer) need less context
// than boundary analysis which requires full codebase visibility.
var phaseContextScale = map[string]int{
	"boundary_analyzer": 100,
	"contract_reviewer": 60,
	"reviewer":          50,
	"refactorer":        70,
}

// PhaseAwareContextBudget scales the context budget based on the active
// review pipeline phase. Boundary analysis gets the full budget; focused
// review/refactor phases get a reduced slice to save tokens.
func PhaseAwareContextBudget(baseBudget int, modeID string) int {
	if baseBudget <= 0 {
		return 0
	}
	pct, ok := phaseContextScale[modeID]
	if !ok {
		return baseBudget
	}
	return baseBudget * pct / 100
}

// complexityMultipliers maps complexity tiers to budget scaling factors.
// Simple tasks need minimal context; complex/reasoning tasks benefit from more.
var complexityMultipliers = map[string]float64{
	"simple":    0.25,
	"medium":    1.0,
	"complex":   2.0,
	"reasoning": 2.0,
}

// ComplexityBudget scales the base context token budget by a multiplier
// determined by the task complexity tier. Unknown tiers default to 1.0x.
// The result composes with PhaseAwareContextBudget and AdaptiveContextBudget:
//
//	effective = AdaptiveContextBudget(
//	    PhaseAwareContextBudget(
//	        ComplexityBudget(tier, base), modeID),
//	    history)
func ComplexityBudget(tier string, baseBudget int) int {
	if baseBudget <= 0 {
		return 0
	}
	m, ok := complexityMultipliers[tier]
	if !ok {
		m = 1.0
	}
	return int(float64(baseBudget) * m)
}

func AdaptiveContextBudget(baseBudget int, history []messagequeue.ConversationMessagePayload) int {
	if baseBudget <= 0 {
		return 0
	}
	n := len(history)
	if n >= contextDecayThreshold {
		return 0
	}
	// Linear decay: budget * (1 - n/threshold)
	scaled := baseBudget * (contextDecayThreshold - n) / contextDecayThreshold
	if scaled < 0 {
		return 0
	}
	return scaled
}
