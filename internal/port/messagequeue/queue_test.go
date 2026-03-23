//go:build !smoke

package messagequeue_test

import (
	"reflect"
	"strings"
	"testing"

	mq "github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// TestSubjectConstants_NoDuplicates verifies that no two Subject* constants
// in the messagequeue package have the same string value. Duplicate subjects
// would cause messages to be misrouted between handlers.
func TestSubjectConstants_NoDuplicates(t *testing.T) {
	// Collect all exported string constants whose name starts with "Subject".
	subjects := allSubjectConstants(t)

	seen := make(map[string]string, len(subjects))
	for name, value := range subjects {
		if prev, exists := seen[value]; exists {
			t.Errorf("duplicate subject value %q: %s and %s", value, prev, name)
		}
		seen[value] = name
	}

	t.Logf("checked %d subject constants, all unique", len(subjects))
}

// TestSubjectConstants_NonEmpty verifies that all Subject* constants are
// non-empty strings. An empty subject would cause publish/subscribe failures.
func TestSubjectConstants_NonEmpty(t *testing.T) {
	subjects := allSubjectConstants(t)

	for name, value := range subjects {
		if value == "" {
			t.Errorf("subject constant %s has empty value", name)
		}
	}
}

// TestSubjectConstants_ValidFormat verifies that subject values use the
// expected dotted notation (e.g., "conversation.run.start").
func TestSubjectConstants_ValidFormat(t *testing.T) {
	subjects := allSubjectConstants(t)

	for name, value := range subjects {
		// Must not start or end with a dot
		if strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
			t.Errorf("subject %s has invalid format: %q (leading/trailing dot)", name, value)
		}
		// Must contain at least one dot (all CodeForge subjects are hierarchical)
		if !strings.Contains(value, ".") {
			t.Errorf("subject %s = %q is not hierarchical (no dots)", name, value)
		}
	}
}

// TestSubjectConstants_MinimumCount ensures we have a reasonable number of
// subject constants. If someone accidentally removes constants, this catches it.
func TestSubjectConstants_MinimumCount(t *testing.T) {
	subjects := allSubjectConstants(t)

	// As of Phase 33, there are 40+ subject constants.
	const minExpected = 35
	if len(subjects) < minExpected {
		t.Errorf("expected at least %d subject constants, found %d", minExpected, len(subjects))
	}
}

// TestQueueInterface_Methods verifies the Queue interface has all expected methods.
func TestQueueInterface_Methods(t *testing.T) {
	queueType := reflect.TypeOf((*mq.Queue)(nil)).Elem()

	expectedMethods := []string{
		"Publish",
		"PublishWithDedup",
		"Subscribe",
		"Drain",
		"Close",
		"IsConnected",
	}

	for _, method := range expectedMethods {
		_, ok := queueType.MethodByName(method)
		if !ok {
			t.Errorf("Queue interface missing method: %s", method)
		}
	}

	if queueType.NumMethod() < len(expectedMethods) {
		t.Errorf("Queue interface has %d methods, expected at least %d",
			queueType.NumMethod(), len(expectedMethods))
	}
}

// allSubjectConstants uses reflection-free string matching to collect
// all Subject* constants from the messagequeue package.
func allSubjectConstants(t *testing.T) map[string]string {
	t.Helper()

	// We enumerate known constants explicitly since Go does not support
	// runtime reflection on package-level constants.
	subjects := map[string]string{
		"SubjectTaskAgent":                      mq.SubjectTaskAgent,
		"SubjectTaskResult":                     mq.SubjectTaskResult,
		"SubjectTaskOutput":                     mq.SubjectTaskOutput,
		"SubjectTaskCancel":                     mq.SubjectTaskCancel,
		"SubjectAgentOutput":                    mq.SubjectAgentOutput,
		"SubjectRunStart":                       mq.SubjectRunStart,
		"SubjectRunToolCallRequest":             mq.SubjectRunToolCallRequest,
		"SubjectRunToolCallResponse":            mq.SubjectRunToolCallResponse,
		"SubjectRunToolCallResult":              mq.SubjectRunToolCallResult,
		"SubjectRunComplete":                    mq.SubjectRunComplete,
		"SubjectRunCancel":                      mq.SubjectRunCancel,
		"SubjectRunOutput":                      mq.SubjectRunOutput,
		"SubjectRunHeartbeat":                   mq.SubjectRunHeartbeat,
		"SubjectQualityGateRequest":             mq.SubjectQualityGateRequest,
		"SubjectQualityGateResult":              mq.SubjectQualityGateResult,
		"SubjectSharedUpdated":                  mq.SubjectSharedUpdated,
		"SubjectContextRerankRequest":           mq.SubjectContextRerankRequest,
		"SubjectContextRerankResult":            mq.SubjectContextRerankResult,
		"SubjectRepoMapRequest":                 mq.SubjectRepoMapRequest,
		"SubjectRepoMapResult":                  mq.SubjectRepoMapResult,
		"SubjectRetrievalIndexRequest":          mq.SubjectRetrievalIndexRequest,
		"SubjectRetrievalIndexResult":           mq.SubjectRetrievalIndexResult,
		"SubjectRetrievalSearchRequest":         mq.SubjectRetrievalSearchRequest,
		"SubjectRetrievalSearchResult":          mq.SubjectRetrievalSearchResult,
		"SubjectSubAgentSearchRequest":          mq.SubjectSubAgentSearchRequest,
		"SubjectSubAgentSearchResult":           mq.SubjectSubAgentSearchResult,
		"SubjectGraphBuildRequest":              mq.SubjectGraphBuildRequest,
		"SubjectGraphBuildResult":               mq.SubjectGraphBuildResult,
		"SubjectGraphSearchRequest":             mq.SubjectGraphSearchRequest,
		"SubjectGraphSearchResult":              mq.SubjectGraphSearchResult,
		"SubjectConversationRunStart":           mq.SubjectConversationRunStart,
		"SubjectConversationRunComplete":        mq.SubjectConversationRunComplete,
		"SubjectConversationRunCancel":          mq.SubjectConversationRunCancel,
		"SubjectConversationCompactRequest":     mq.SubjectConversationCompactRequest,
		"SubjectConversationCompactComplete":    mq.SubjectConversationCompactComplete,
		"SubjectEvalGemmasRequest":              mq.SubjectEvalGemmasRequest,
		"SubjectEvalGemmasResult":               mq.SubjectEvalGemmasResult,
		"SubjectA2ATaskCreated":                 mq.SubjectA2ATaskCreated,
		"SubjectA2ATaskComplete":                mq.SubjectA2ATaskComplete,
		"SubjectA2ATaskCancel":                  mq.SubjectA2ATaskCancel,
		"SubjectBenchmarkRunRequest":            mq.SubjectBenchmarkRunRequest,
		"SubjectBenchmarkRunResult":             mq.SubjectBenchmarkRunResult,
		"SubjectBenchmarkTaskStarted":           mq.SubjectBenchmarkTaskStarted,
		"SubjectBenchmarkTaskProgress":          mq.SubjectBenchmarkTaskProgress,
		"SubjectMemoryStore":                    mq.SubjectMemoryStore,
		"SubjectMemoryRecall":                   mq.SubjectMemoryRecall,
		"SubjectMemoryRecallResult":             mq.SubjectMemoryRecallResult,
		"SubjectHandoffRequest":                 mq.SubjectHandoffRequest,
		"SubjectTrajectoryEvent":                mq.SubjectTrajectoryEvent,
		"SubjectBackendHealthRequest":           mq.SubjectBackendHealthRequest,
		"SubjectBackendHealthResult":            mq.SubjectBackendHealthResult,
		"SubjectReviewTriggerRequest":           mq.SubjectReviewTriggerRequest,
		"SubjectReviewTriggerComplete":          mq.SubjectReviewTriggerComplete,
		"SubjectReviewApprovalRequired":         mq.SubjectReviewApprovalRequired,
		"SubjectPromptEvolutionReflect":         mq.SubjectPromptEvolutionReflect,
		"SubjectPromptEvolutionReflectComplete": mq.SubjectPromptEvolutionReflectComplete,
		"SubjectPromptEvolutionMutateComplete":  mq.SubjectPromptEvolutionMutateComplete,
		"SubjectPromptEvolutionPromoted":        mq.SubjectPromptEvolutionPromoted,
		"SubjectPromptEvolutionReverted":        mq.SubjectPromptEvolutionReverted,
	}

	return subjects
}
