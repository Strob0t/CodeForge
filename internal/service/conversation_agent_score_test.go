package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// mockPromptScoreStore records InsertPromptScore calls for test assertions.
type mockPromptScoreStore struct {
	mu     sync.Mutex
	scores []prompt.PromptScore
}

func (m *mockPromptScoreStore) InsertPromptScore(_ context.Context, score *prompt.PromptScore) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scores = append(m.scores, *score)
	return nil
}

func (m *mockPromptScoreStore) GetScoresByFingerprint(_ context.Context, _ string) ([]prompt.PromptScore, error) {
	return nil, nil
}

func (m *mockPromptScoreStore) GetAggregatedScores(_ context.Context, _, _, _ string) (map[string]map[prompt.SignalType]float64, error) {
	return nil, nil
}

func (m *mockPromptScoreStore) getScores() []prompt.PromptScore {
	m.mu.Lock()
	defer m.mu.Unlock()
	dst := make([]prompt.PromptScore, len(m.scores))
	copy(dst, m.scores)
	return dst
}

// --- Tests ---

func TestHandleConversationRunComplete_ScoreRecording(t *testing.T) {
	t.Parallel()

	t.Run("records success and cost scores on completed run", func(t *testing.T) {
		t.Parallel()

		scoreStore := &mockPromptScoreStore{}
		collector := service.NewPromptScoreCollector(scoreStore)

		store := &convRunCompleteMockStore{}
		store.conversations = []conversation.Conversation{
			{ID: "conv-score-1", ProjectID: "proj-1", Mode: "coder"},
		}
		bc := &runtimeMockBroadcaster{}
		modes := service.NewModeService()
		svc := service.NewConversationService(store, bc, "gpt-4o", modes)
		svc.SetPromptScoreCollector(collector)

		// Set up a mock assembler that returns a fingerprint for the "coder" mode.
		assembler := service.NewTestPromptAssemblerWithFingerprint(map[string]string{
			"coder": "fp-abc123",
		})
		svc.SetPromptAssembler(assembler)

		payload := messagequeue.ConversationRunCompletePayload{
			RunID:            "run-score-1",
			ConversationID:   "conv-score-1",
			AssistantContent: "Done!",
			Status:           "completed",
			CostUSD:          0.10,
			TokensOut:        500,
			Model:            "openai/gpt-4o",
		}
		ctx := context.Background()
		if err := svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload)); err != nil {
			t.Fatalf("HandleConversationRunComplete: %v", err)
		}

		scores := scoreStore.getScores()
		if len(scores) < 2 {
			t.Fatalf("expected at least 2 scores (success + cost), got %d", len(scores))
		}

		// Check success score.
		var foundSuccess, foundCost bool
		for _, s := range scores {
			switch s.SignalType {
			case prompt.SignalSuccess:
				foundSuccess = true
				if s.Score != 1.0 {
					t.Errorf("expected success score 1.0, got %f", s.Score)
				}
				if s.ModelFamily != "openai" {
					t.Errorf("expected model family 'openai', got %q", s.ModelFamily)
				}
				if s.PromptFingerprint == "" {
					t.Error("expected non-empty fingerprint")
				}
				if s.RunID != "run-score-1" {
					t.Errorf("expected run_id 'run-score-1', got %q", s.RunID)
				}
			case prompt.SignalCost:
				foundCost = true
				if s.Score <= 0 {
					t.Errorf("expected positive cost score, got %f", s.Score)
				}
			}
		}
		if !foundSuccess {
			t.Error("expected a success score to be recorded")
		}
		if !foundCost {
			t.Error("expected a cost score to be recorded")
		}
	})

	t.Run("records failed success score on failed run", func(t *testing.T) {
		t.Parallel()

		scoreStore := &mockPromptScoreStore{}
		collector := service.NewPromptScoreCollector(scoreStore)

		store := &convRunCompleteMockStore{}
		store.conversations = []conversation.Conversation{
			{ID: "conv-score-2", ProjectID: "proj-1", Mode: "coder"},
		}
		bc := &runtimeMockBroadcaster{}
		modes := service.NewModeService()
		svc := service.NewConversationService(store, bc, "gpt-4o", modes)
		svc.SetPromptScoreCollector(collector)

		assembler := service.NewTestPromptAssemblerWithFingerprint(map[string]string{
			"coder": "fp-abc123",
		})
		svc.SetPromptAssembler(assembler)

		payload := messagequeue.ConversationRunCompletePayload{
			RunID:          "run-score-2",
			ConversationID: "conv-score-2",
			Status:         "failed",
			Error:          "LLM timeout",
			Model:          "openai/gpt-4o",
		}
		ctx := context.Background()
		_ = svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))

		scores := scoreStore.getScores()
		if len(scores) != 1 {
			t.Fatalf("expected 1 score (success only, no cost for failed run), got %d", len(scores))
		}
		if scores[0].SignalType != prompt.SignalSuccess {
			t.Errorf("expected signal type %q, got %q", prompt.SignalSuccess, scores[0].SignalType)
		}
		if scores[0].Score != 0.0 {
			t.Errorf("expected success score 0.0 for failed run, got %f", scores[0].Score)
		}
	})

	t.Run("no panic when score collector is nil", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newConvRunCompleteEnv()
		// scoreCollector is nil by default.

		payload := messagequeue.ConversationRunCompletePayload{
			RunID:            "run-score-nil",
			ConversationID:   "conv-1",
			AssistantContent: "done",
			Status:           "completed",
			Model:            "openai/gpt-4o",
		}
		ctx := context.Background()
		err := svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))
		if err != nil {
			t.Fatalf("unexpected error with nil score collector: %v", err)
		}
	})

	t.Run("no score recorded when model is empty", func(t *testing.T) {
		t.Parallel()

		scoreStore := &mockPromptScoreStore{}
		collector := service.NewPromptScoreCollector(scoreStore)

		store := &convRunCompleteMockStore{}
		store.conversations = []conversation.Conversation{
			{ID: "conv-score-3", ProjectID: "proj-1", Mode: "coder"},
		}
		bc := &runtimeMockBroadcaster{}
		modes := service.NewModeService()
		svc := service.NewConversationService(store, bc, "gpt-4o", modes)
		svc.SetPromptScoreCollector(collector)

		assembler := service.NewTestPromptAssemblerWithFingerprint(map[string]string{
			"coder": "fp-abc123",
		})
		svc.SetPromptAssembler(assembler)

		payload := messagequeue.ConversationRunCompletePayload{
			RunID:            "run-score-3",
			ConversationID:   "conv-score-3",
			AssistantContent: "done",
			Status:           "completed",
			Model:            "", // empty model
		}
		ctx := context.Background()
		_ = svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))

		scores := scoreStore.getScores()
		if len(scores) != 0 {
			t.Errorf("expected 0 scores when model is empty, got %d", len(scores))
		}
	})

	t.Run("no score when fingerprint is empty (unknown mode)", func(t *testing.T) {
		t.Parallel()

		scoreStore := &mockPromptScoreStore{}
		collector := service.NewPromptScoreCollector(scoreStore)

		store := &convRunCompleteMockStore{}
		store.conversations = []conversation.Conversation{
			{ID: "conv-score-4", ProjectID: "proj-1", Mode: "unknown-mode"},
		}
		bc := &runtimeMockBroadcaster{}
		modes := service.NewModeService()
		svc := service.NewConversationService(store, bc, "gpt-4o", modes)
		svc.SetPromptScoreCollector(collector)

		// Assembler returns empty fingerprint for unknown mode.
		assembler := service.NewTestPromptAssemblerWithFingerprint(map[string]string{
			"coder": "fp-abc123",
		})
		svc.SetPromptAssembler(assembler)

		payload := messagequeue.ConversationRunCompletePayload{
			RunID:            "run-score-4",
			ConversationID:   "conv-score-4",
			AssistantContent: "done",
			Status:           "completed",
			Model:            "openai/gpt-4o",
		}
		ctx := context.Background()
		_ = svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))

		scores := scoreStore.getScores()
		if len(scores) != 0 {
			t.Errorf("expected 0 scores when fingerprint is empty, got %d", len(scores))
		}
	})

	t.Run("no cost score when cost is zero", func(t *testing.T) {
		t.Parallel()

		scoreStore := &mockPromptScoreStore{}
		collector := service.NewPromptScoreCollector(scoreStore)

		store := &convRunCompleteMockStore{}
		store.conversations = []conversation.Conversation{
			{ID: "conv-score-5", ProjectID: "proj-1", Mode: "coder"},
		}
		bc := &runtimeMockBroadcaster{}
		modes := service.NewModeService()
		svc := service.NewConversationService(store, bc, "gpt-4o", modes)
		svc.SetPromptScoreCollector(collector)

		assembler := service.NewTestPromptAssemblerWithFingerprint(map[string]string{
			"coder": "fp-abc123",
		})
		svc.SetPromptAssembler(assembler)

		payload := messagequeue.ConversationRunCompletePayload{
			RunID:            "run-score-5",
			ConversationID:   "conv-score-5",
			AssistantContent: "done",
			Status:           "completed",
			CostUSD:          0, // no cost
			TokensOut:        500,
			Model:            "openai/gpt-4o",
		}
		ctx := context.Background()
		_ = svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))

		scores := scoreStore.getScores()
		if len(scores) != 1 {
			t.Fatalf("expected 1 score (success only, no cost since CostUSD=0), got %d", len(scores))
		}
		if scores[0].SignalType != prompt.SignalSuccess {
			t.Errorf("expected success signal, got %q", scores[0].SignalType)
		}
	})
}

func TestExtractModelFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  string
	}{
		{"openai/gpt-4o", "openai"},
		{"anthropic/claude-3", "anthropic"},
		{"ollama/llama3", "ollama"},
		{"gpt-4o", "gpt-4o"},                      // no slash -> return as-is
		{"", ""},                                  // empty string
		{"lm_studio/qwen/qwen3-30b", "lm_studio"}, // multiple slashes -> first segment
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			got := service.ExtractModelFamily(tc.model)
			if got != tc.want {
				t.Errorf("ExtractModelFamily(%q) = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}
