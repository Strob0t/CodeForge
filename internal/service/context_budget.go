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
