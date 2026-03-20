package messagequeue

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// knownPrefixes lists subject prefixes that are expected but validated only
// as valid JSON (no struct schema). This avoids noisy warnings for subjects
// that are intentionally loosely typed (e.g. heartbeats, status events).
var knownPrefixes = []string{
	"runs.", "tasks.", "mcp.", "memory.", "handoff.",
	"backends.", "conversation.",
}

// unknownSubjectOnce tracks subjects already warned about to avoid log spam.
var unknownSubjectOnce sync.Map

// Validate checks whether data is valid JSON conforming to the schema
// associated with the given subject. Unknown subjects pass validation
// with a one-time warning (future-proof for new message types).
func Validate(subject string, data []byte) error {
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON on subject %s", subject)
	}

	// Reject empty JSON objects — a valid payload must carry at least one field.
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "{}" {
		return fmt.Errorf("empty JSON object on subject %s", subject)
	}

	// Map subject to payload struct for structural validation.
	var target any
	switch {
	// --- Task subjects ---
	case subject == SubjectTaskResult:
		target = &TaskResultPayload{}
	case subject == SubjectTaskCancel:
		target = &TaskCancelPayload{}

	// --- Retrieval subjects (Phase 6B) ---
	case subject == SubjectRetrievalIndexRequest:
		target = &RetrievalIndexRequestPayload{}
	case subject == SubjectRetrievalIndexResult:
		target = &RetrievalIndexResultPayload{}
	case subject == SubjectRetrievalSearchRequest:
		target = &RetrievalSearchRequestPayload{}
	case subject == SubjectRetrievalSearchResult:
		target = &RetrievalSearchResultPayload{}

	// --- Retrieval Sub-Agent subjects (Phase 6C) ---
	case subject == SubjectSubAgentSearchRequest:
		target = &SubAgentSearchRequestPayload{}
	case subject == SubjectSubAgentSearchResult:
		target = &SubAgentSearchResultPayload{}

	// --- GraphRAG subjects (Phase 6D) ---
	case subject == SubjectGraphBuildRequest:
		target = &GraphBuildRequestPayload{}
	case subject == SubjectGraphBuildResult:
		target = &GraphBuildResultPayload{}
	case subject == SubjectGraphSearchRequest:
		target = &GraphSearchRequestPayload{}
	case subject == SubjectGraphSearchResult:
		target = &GraphSearchResultPayload{}

	// --- Conversation run subjects (Phase 17C) ---
	case subject == SubjectConversationRunStart:
		target = &ConversationRunStartPayload{}
	case subject == SubjectConversationRunComplete:
		target = &ConversationRunCompletePayload{}

	// --- GEMMAS Evaluation subjects (Phase 20G) ---
	case subject == SubjectEvalGemmasRequest:
		target = &GemmasEvalRequestPayload{}
	case subject == SubjectEvalGemmasResult:
		target = &GemmasEvalResultPayload{}

	// --- Benchmark subjects (Phase 26/28) ---
	case subject == SubjectBenchmarkRunRequest:
		target = &BenchmarkRunRequestPayload{}
	case subject == SubjectBenchmarkRunResult:
		target = &BenchmarkRunResultPayload{}
	case subject == SubjectBenchmarkTaskStarted:
		target = &BenchmarkTaskStartedPayload{}
	case subject == SubjectBenchmarkTaskProgress:
		target = &BenchmarkTaskProgressPayload{}

	// --- A2A subjects (Phase 27) ---
	case subject == SubjectA2ATaskCreated:
		target = &A2ATaskCreatedPayload{}
	case subject == SubjectA2ATaskComplete:
		target = &A2ATaskCompletePayload{}

	// --- Review/Refactor subjects (Phase 31) ---
	case subject == SubjectReviewTriggerRequest:
		target = &ReviewTriggerRequestPayload{}
	case subject == SubjectReviewBoundaryAnalyzed:
		target = &ReviewBoundaryAnalyzedPayload{}
	case subject == SubjectReviewApprovalRequired:
		target = &ReviewApprovalRequiredPayload{}
	case subject == SubjectReviewApprovalResponse:
		target = &ReviewApprovalResponsePayload{}

	// --- Prompt Evolution subjects (Phase 33) ---
	case subject == SubjectPromptEvolutionReflect:
		target = &PromptEvolutionReflectPayload{}
	case subject == SubjectPromptEvolutionReflectComplete:
		target = &PromptEvolutionReflectCompletePayload{}
	case subject == SubjectPromptEvolutionMutateComplete:
		target = &PromptEvolutionMutateCompletePayload{}

	// --- Wildcard prefixes (valid JSON is sufficient) ---
	case strings.HasPrefix(subject, SubjectTaskAgent+"."):
		// tasks.agent.{backend} — the payload is a Task, not a custom schema.
		return nil

	default:
		// Check if the subject matches a known prefix — if so, accept valid JSON.
		for _, prefix := range knownPrefixes {
			if strings.HasPrefix(subject, prefix) {
				return nil
			}
		}
		// Log unknown subjects once per subject to avoid log spam.
		if _, loaded := unknownSubjectOnce.LoadOrStore(subject, struct{}{}); !loaded {
			slog.Warn("NATS validator: unknown subject, skipping schema check", "subject", subject)
		}
		return nil
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("schema validation failed for %s: %w", subject, err)
	}
	return nil
}
