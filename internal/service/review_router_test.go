package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// newTestLiteLLMServer creates a mock LiteLLM server returning the given content.
func newTestLiteLLMServer(content string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]string{"content": content},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 20},
			"model": "test-model",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck,gosec // G104: test code
	}))
}

func newTestReviewRouter(serverURL string, enabled bool) *ReviewRouterService {
	llm := litellm.NewClient(serverURL, "test-key")
	cfg := &config.Orchestrator{
		ReviewRouterEnabled:       enabled,
		ReviewConfidenceThreshold: 0.7,
		ReviewRouterModel:         "test-model",
		DecomposeModel:            "test-model",
	}
	return NewReviewRouterService(llm, cfg, &config.Limits{MaxInputLen: 10000})
}

func TestReviewRouter_DisabledReturnsNoReview(t *testing.T) {
	router := newTestReviewRouter("http://unused", false)

	step := &plan.Step{ID: "step-1", TaskID: "task-1", AgentID: "agent-1"}
	decision, err := router.Evaluate(context.Background(), step, "test task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.NeedsReview {
		t.Error("expected NeedsReview=false when router is disabled")
	}
	if decision.Confidence != 1.0 {
		t.Errorf("expected confidence=1.0, got %f", decision.Confidence)
	}
}

func TestReviewRouter_HighConfidenceNoReview(t *testing.T) {
	resp := `{"needs_review": false, "confidence": 0.95, "reason": "simple task", "suggested_reviewers": []}`
	srv := newTestLiteLLMServer(resp)
	defer srv.Close()

	router := newTestReviewRouter(srv.URL, true)
	step := &plan.Step{ID: "step-1", TaskID: "task-1", AgentID: "agent-1"}

	decision, err := router.Evaluate(context.Background(), step, "add a test file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.NeedsReview {
		t.Error("expected NeedsReview=false for simple task")
	}
	if decision.Confidence != 0.95 {
		t.Errorf("expected confidence=0.95, got %f", decision.Confidence)
	}
}

func TestReviewRouter_LowConfidenceNeedsReview(t *testing.T) {
	resp := `{"needs_review": true, "confidence": 0.4, "reason": "architecture change", "suggested_reviewers": ["architect", "reviewer"]}`
	srv := newTestLiteLLMServer(resp)
	defer srv.Close()

	router := newTestReviewRouter(srv.URL, true)
	step := &plan.Step{ID: "step-2", TaskID: "task-2", AgentID: "agent-1"}

	decision, err := router.Evaluate(context.Background(), step, "refactor auth system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.NeedsReview {
		t.Error("expected NeedsReview=true for architecture change")
	}
	if decision.Confidence != 0.4 {
		t.Errorf("expected confidence=0.4, got %f", decision.Confidence)
	}
	if len(decision.SuggestedReviewers) != 2 {
		t.Errorf("expected 2 suggested reviewers, got %d", len(decision.SuggestedReviewers))
	}
}

func TestReviewRouter_ThresholdBoundaryExact(t *testing.T) {
	// Confidence exactly at threshold — should NOT be routed (< threshold triggers routing)
	resp := `{"needs_review": true, "confidence": 0.7, "reason": "boundary case", "suggested_reviewers": []}`
	srv := newTestLiteLLMServer(resp)
	defer srv.Close()

	router := newTestReviewRouter(srv.URL, true)
	step := &plan.Step{ID: "step-3", TaskID: "task-3", AgentID: "agent-1"}

	decision, err := router.Evaluate(context.Background(), step, "update config")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.NeedsReview {
		t.Error("expected NeedsReview=true from LLM")
	}

	// ShouldRoute: confidence (0.7) is NOT less than threshold (0.7) — should NOT route
	if router.ShouldRoute(decision) {
		t.Error("expected ShouldRoute=false when confidence equals threshold")
	}
}

func TestReviewRouter_ThresholdBoundaryJustBelow(t *testing.T) {
	// Confidence just below threshold — should be routed
	resp := `{"needs_review": true, "confidence": 0.69, "reason": "complex change", "suggested_reviewers": ["reviewer"]}`
	srv := newTestLiteLLMServer(resp)
	defer srv.Close()

	router := newTestReviewRouter(srv.URL, true)
	step := &plan.Step{ID: "step-4", TaskID: "task-4", AgentID: "agent-1"}

	decision, err := router.Evaluate(context.Background(), step, "database migration")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !router.ShouldRoute(decision) {
		t.Error("expected ShouldRoute=true when confidence (0.69) < threshold (0.7)")
	}
}

func TestReviewRouter_InvalidJSONFallsBackToNoReview(t *testing.T) {
	srv := newTestLiteLLMServer("This is not JSON at all, just some random text.")
	defer srv.Close()

	router := newTestReviewRouter(srv.URL, true)
	step := &plan.Step{ID: "step-5", TaskID: "task-5", AgentID: "agent-1"}

	decision, err := router.Evaluate(context.Background(), step, "some task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.NeedsReview {
		t.Error("expected NeedsReview=false when LLM returns invalid JSON")
	}
	if decision.Confidence != 0.5 {
		t.Errorf("expected fallback confidence=0.5, got %f", decision.Confidence)
	}
}

func TestReviewRouter_ConfidenceClampedToRange(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantConf float64
	}{
		{"above 1", `{"needs_review": true, "confidence": 1.5, "reason": "test", "suggested_reviewers": []}`, 1.0},
		{"below 0", `{"needs_review": true, "confidence": -0.5, "reason": "test", "suggested_reviewers": []}`, 0.0},
		{"normal", `{"needs_review": true, "confidence": 0.6, "reason": "test", "suggested_reviewers": []}`, 0.6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestLiteLLMServer(tt.input)
			defer srv.Close()

			router := newTestReviewRouter(srv.URL, true)
			step := &plan.Step{ID: "step-clamp", TaskID: "task-clamp"}

			decision, err := router.Evaluate(context.Background(), step, "test")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if decision.Confidence != tt.wantConf {
				t.Errorf("expected confidence=%f, got %f", tt.wantConf, decision.Confidence)
			}
		})
	}
}

func TestReviewRouter_ShouldRouteLogic(t *testing.T) {
	router := newTestReviewRouter("http://unused", true)

	tests := []struct {
		name     string
		decision *orchestration.ReviewDecision
		want     bool
	}{
		{
			"no review needed",
			&orchestration.ReviewDecision{NeedsReview: false, Confidence: 0.3},
			false,
		},
		{
			"needs review below threshold",
			&orchestration.ReviewDecision{NeedsReview: true, Confidence: 0.5},
			true,
		},
		{
			"needs review above threshold",
			&orchestration.ReviewDecision{NeedsReview: true, Confidence: 0.9},
			false,
		},
		{
			"needs review at threshold",
			&orchestration.ReviewDecision{NeedsReview: true, Confidence: 0.7},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.ShouldRoute(tt.decision)
			if got != tt.want {
				t.Errorf("ShouldRoute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReviewRouter_PromptTemplateRendering(t *testing.T) {
	// Verify the review_router.tmpl template renders without error
	resp := `{"needs_review": false, "confidence": 0.9, "reason": "ok", "suggested_reviewers": []}`
	srv := newTestLiteLLMServer(resp)
	defer srv.Close()

	router := newTestReviewRouter(srv.URL, true)
	step := &plan.Step{
		ID:      "step-tmpl",
		TaskID:  "task-tmpl",
		AgentID: "agent-tmpl",
		ModeID:  "coder",
	}

	decision, err := router.Evaluate(context.Background(), step, "implement feature X with tests")
	if err != nil {
		t.Fatalf("template rendering failed: %v", err)
	}

	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
}

func TestReviewRouter_LLMErrorReturnsError(t *testing.T) {
	// Server that always returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "internal server error")
	}))
	defer srv.Close()

	router := newTestReviewRouter(srv.URL, true)
	step := &plan.Step{ID: "step-err", TaskID: "task-err"}

	_, err := router.Evaluate(context.Background(), step, "test")
	if err == nil {
		t.Fatal("expected error from failing LLM server")
	}
}

func TestParseReviewDecision(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *orchestration.ReviewDecision
		wantErr bool
	}{
		{
			"valid JSON",
			`{"needs_review": true, "confidence": 0.85, "reason": "security change", "suggested_reviewers": ["security"]}`,
			&orchestration.ReviewDecision{NeedsReview: true, Confidence: 0.85, Reason: "security change", SuggestedReviewers: []string{"security"}},
			false,
		},
		{
			"JSON in markdown fence",
			"```json\n{\"needs_review\": false, \"confidence\": 0.9, \"reason\": \"simple\"}\n```",
			&orchestration.ReviewDecision{NeedsReview: false, Confidence: 0.9, Reason: "simple"},
			false,
		},
		{
			"invalid JSON",
			"not json at all",
			nil,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseReviewDecision(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.NeedsReview != tt.want.NeedsReview {
				t.Errorf("NeedsReview: got %v, want %v", got.NeedsReview, tt.want.NeedsReview)
			}
			if got.Confidence != tt.want.Confidence {
				t.Errorf("Confidence: got %f, want %f", got.Confidence, tt.want.Confidence)
			}
		})
	}
}
