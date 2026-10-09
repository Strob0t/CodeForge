package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/port/llm"
)

// A ping_pong step whose review is decided in the background stays pending
// after its round began (S2-F review, F1). The plan was advanced again when
// the review was decided and took the pending step's bumped round for a run:
// it switched to the other step and bumped its round too, so with one round
// per step the plan completed without running any step. The same happened
// when a debated step went back to pending to run after its debate.

// noReviewLLM answers every review router prompt with "no review needed".
type noReviewLLM struct{ needsReviewLLM }

func noReview() *llm.ChatCompletionResponse {
	return &llm.ChatCompletionResponse{Content: `{"needs_review": false, "confidence": 0.95, "reason": "simple", "suggested_reviewers": []}`}
}

func (noReviewLLM) ChatCompletion(context.Context, llm.ChatCompletionRequest) (*llm.ChatCompletionResponse, error) {
	return noReview(), nil
}

func (noReviewLLM) ChatCompletionStream(context.Context, llm.ChatCompletionRequest, func(llm.StreamChunk)) (*llm.ChatCompletionResponse, error) {
	return noReview(), nil
}

// runningStep waits until step i of a plan runs a run (its review is decided
// in the background) and returns the plan.
func runningStep(t *testing.T, store *orchMockStore, planID string, i int) *plan.ExecutionPlan {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		p := planState(t, store, planID)
		if st := p.Steps[i]; st.Status == plan.StepStatusRunning && st.RunID != "" {
			return p
		}
		if time.Now().After(deadline) {
			t.Fatalf("step %d of %s = %s with run %q, want running with a run (plan %s)",
				i, planID, p.Steps[i].Status, p.Steps[i].RunID, p.Status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPingPong_WithTheReviewRouter_EveryStepRunsItsRound(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider llm.Provider
		debated  bool
	}{
		{"review not routed", noReviewLLM{}, false},
		{"routed to a debate", needsReviewLLM{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, orchSvc, cfg := newDebateSetupWithLLM(true, tc.provider)
			cfg.PingPongMaxRounds = 1
			ctx := context.Background()
			p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
				Name: "pp", ProjectID: "proj-1", Protocol: plan.ProtocolPingPong,
				Steps: []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"}},
			})
			if err != nil {
				t.Fatalf("CreatePlan: %v", err)
			}
			within(t, "StartPlan", func() {
				if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
					t.Errorf("StartPlan: %v", err)
				}
			})

			for i := range 2 {
				if tc.debated {
					debate := debatePlanOf(t, store, p.ID)
					completeStepRun(t, store, orchSvc, debate.ID, 0)
					completeStepRun(t, store, orchSvc, debate.ID, 1)
				}
				got := runningStep(t, store, p.ID, i)
				if other := got.Steps[1-i]; other.Status == plan.StepStatusRunning {
					t.Fatalf("step %d runs while step %d runs", 1-i, i)
				}
				if got.Steps[i].Round != 1 {
					t.Errorf("step %d round = %d, want 1", i, got.Steps[i].Round)
				}
				completeStepRun(t, store, orchSvc, p.ID, i)
			}

			got := planState(t, store, p.ID)
			if got.Status != plan.StatusCompleted {
				t.Fatalf("plan = %s, want completed", got.Status)
			}
			for i, st := range got.Steps {
				if st.Status != plan.StepStatusCompleted || st.Round != 1 {
					t.Errorf("step %d = %s in round %d, want completed in round 1", i, st.Status, st.Round)
				}
			}
		})
	}
}
