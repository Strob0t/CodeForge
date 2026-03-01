package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/service"
)

// mockDecomposeResponse returns a valid LLM response for decomposition.
func mockDecomposeResponse() plan.DecomposeResult {
	return plan.DecomposeResult{
		PlanName:    "Auth Feature",
		Description: "Implement user authentication",
		Strategy:    plan.StrategySingle,
		Protocol:    plan.ProtocolSequential,
		Subtasks: []plan.SubtaskDefinition{
			{Title: "Add login endpoint", Prompt: "Create POST /login with JWT", DependsOn: []int{}, AgentHint: "aider"},
			{Title: "Add auth middleware", Prompt: "Create auth middleware checking JWT", DependsOn: []int{0}, AgentHint: ""},
		},
	}
}

// newMockLLMServer creates a test server that returns a completion response with the given body.
func newMockLLMServer(t *testing.T, responseBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": responseBody}},
			},
			"usage": map[string]int{"prompt_tokens": 50, "completion_tokens": 100},
			"model": "gpt-4o-mini",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func newMetaTestSetup(t *testing.T, llmBody string) (*orchMockStore, *service.MetaAgentService, *httptest.Server) {
	t.Helper()
	return newMetaTestSetupFull(t, llmBody, "semi_auto")
}

func newMetaTestSetupFull(t *testing.T, llmBody, mode string) (*orchMockStore, *service.MetaAgentService, *httptest.Server) {
	t.Helper()
	srv := newMockLLMServer(t, llmBody)
	llmClient := litellm.NewClient(srv.URL, "")

	store := &orchMockStore{}
	store.projects = []project.Project{{ID: "p1", Name: "TestProject"}}
	store.agents = []agent.Agent{
		{ID: "a1", ProjectID: "p1", Name: "Coder", Backend: "aider", Status: agent.StatusIdle},
	}

	orchCfg := &config.Orchestrator{
		MaxParallel:        4,
		PingPongMaxRounds:  3,
		Mode:               mode,
		DecomposeModel:     "gpt-4o-mini",
		DecomposeMaxTokens: 4096,
	}

	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	queue := &runtimeMockQueue{}

	runtimeSvc := service.NewRuntimeService(store, queue, bc, es,
		service.NewPolicyService("headless-safe-sandbox", nil),
		&config.Runtime{StallThreshold: 5},
	)

	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, orchCfg)
	runtimeSvc.SetOnRunComplete(orchSvc.HandleRunCompleted)

	meta := service.NewMetaAgentService(store, llmClient, orchSvc, orchCfg, &config.Limits{MaxInputLen: 10000})
	return store, meta, srv
}

func TestDecomposeFeatureSuccess(t *testing.T) {
	body, _ := json.Marshal(mockDecomposeResponse())
	store, meta, srv := newMetaTestSetup(t, string(body))
	defer srv.Close()

	req := &plan.DecomposeRequest{
		ProjectID: "p1",
		Feature:   "Implement user authentication with JWT tokens",
	}

	p, err := meta.DecomposeFeature(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected plan, got nil")
	}
	if p.Name != "Auth Feature" {
		t.Errorf("expected plan name 'Auth Feature', got %q", p.Name)
	}
	if p.Protocol != plan.ProtocolSequential {
		t.Errorf("expected sequential protocol, got %q", p.Protocol)
	}
	if p.Status != plan.StatusPending {
		t.Errorf("expected pending status, got %q", p.Status)
	}
	if len(p.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(p.Steps))
	}
	if len(store.tasks) != 2 {
		t.Errorf("expected 2 tasks in store, got %d", len(store.tasks))
	}
}

func TestDecomposeFeatureFullAutoStart(t *testing.T) {
	result := mockDecomposeResponse()
	result.Subtasks = []plan.SubtaskDefinition{
		{Title: "Single task", Prompt: "Do the thing", DependsOn: []int{}},
	}
	body, _ := json.Marshal(result)

	_, metaFull, srv := newMetaTestSetupFull(t, string(body), "full_auto")
	defer srv.Close()

	p, err := metaFull.DecomposeFeature(context.Background(), &plan.DecomposeRequest{
		ProjectID: "p1",
		Feature:   "Quick fix",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != plan.StatusRunning {
		t.Errorf("expected running status (auto-started), got %q", p.Status)
	}
}

func TestDecomposeFeatureAutoStartOverride(t *testing.T) {
	result := mockDecomposeResponse()
	result.Subtasks = []plan.SubtaskDefinition{
		{Title: "Single task", Prompt: "Do the thing", DependsOn: []int{}},
	}
	body, _ := json.Marshal(result)

	_, meta, srv := newMetaTestSetupFull(t, string(body), "semi_auto")
	defer srv.Close()

	p, err := meta.DecomposeFeature(context.Background(), &plan.DecomposeRequest{
		ProjectID: "p1",
		Feature:   "Quick fix",
		AutoStart: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != plan.StatusRunning {
		t.Errorf("expected running status (auto_start override), got %q", p.Status)
	}
}

func TestDecomposeFeatureValidationError(t *testing.T) {
	_, meta, srv := newMetaTestSetup(t, "{}")
	defer srv.Close()

	_, err := meta.DecomposeFeature(context.Background(), &plan.DecomposeRequest{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestDecomposeFeatureNoAgents(t *testing.T) {
	body, _ := json.Marshal(mockDecomposeResponse())
	store, meta, srv := newMetaTestSetup(t, string(body))
	defer srv.Close()

	store.agents = nil // remove agents

	_, err := meta.DecomposeFeature(context.Background(), &plan.DecomposeRequest{
		ProjectID: "p1",
		Feature:   "Something",
	})
	if err == nil {
		t.Fatal("expected error for no agents, got nil")
	}
}

func TestDecomposeFeatureLLMError(t *testing.T) {
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "model unavailable"}`))
	}))
	defer errSrv.Close()

	store := &orchMockStore{}
	store.projects = []project.Project{{ID: "p1", Name: "TestProject"}}
	store.agents = []agent.Agent{{ID: "a1", ProjectID: "p1", Name: "C", Backend: "aider"}}

	llmClient := litellm.NewClient(errSrv.URL, "")
	orchCfg := &config.Orchestrator{Mode: "semi_auto", DecomposeModel: "m", DecomposeMaxTokens: 4096}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	orchSvc := service.NewOrchestratorService(store, bc, es, nil, orchCfg)
	meta := service.NewMetaAgentService(store, llmClient, orchSvc, orchCfg, &config.Limits{MaxInputLen: 10000})

	_, err := meta.DecomposeFeature(context.Background(), &plan.DecomposeRequest{
		ProjectID: "p1",
		Feature:   "Something",
	})
	if err == nil {
		t.Fatal("expected LLM error, got nil")
	}
}

func TestDecomposeFeatureInvalidLLMJSON(t *testing.T) {
	_, meta, srv := newMetaTestSetup(t, "this is not json at all")
	defer srv.Close()

	_, err := meta.DecomposeFeature(context.Background(), &plan.DecomposeRequest{
		ProjectID: "p1",
		Feature:   "Something",
	})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestDecomposeFeatureAgentSelection(t *testing.T) {
	result := plan.DecomposeResult{
		PlanName: "Test",
		Strategy: plan.StrategySingle,
		Protocol: plan.ProtocolSequential,
		Subtasks: []plan.SubtaskDefinition{
			{Title: "Task", Prompt: "Do it", DependsOn: []int{}, AgentHint: "openhands"},
		},
	}
	body, _ := json.Marshal(result)
	store, meta, srv := newMetaTestSetup(t, string(body))
	defer srv.Close()

	// Add a second agent with openhands backend
	store.agents = append(store.agents, agent.Agent{
		ID: "a2", ProjectID: "p1", Name: "OpenHands Agent", Backend: "openhands", Status: agent.StatusIdle,
	})

	p, err := meta.DecomposeFeature(context.Background(), &plan.DecomposeRequest{
		ProjectID: "p1",
		Feature:   "Test feature",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(p.Steps))
	}
	if p.Steps[0].AgentID != "a2" {
		t.Errorf("expected agent a2 (openhands), got %q", p.Steps[0].AgentID)
	}
}

func TestDecomposeFeatureMarkdownFences(t *testing.T) {
	result := mockDecomposeResponse()
	result.Subtasks = []plan.SubtaskDefinition{
		{Title: "Task", Prompt: "Do it", DependsOn: []int{}},
	}
	body, _ := json.Marshal(result)
	wrapped := "```json\n" + string(body) + "\n```"

	_, meta, srv := newMetaTestSetup(t, wrapped)
	defer srv.Close()

	p, err := meta.DecomposeFeature(context.Background(), &plan.DecomposeRequest{
		ProjectID: "p1",
		Feature:   "Test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Auth Feature" {
		t.Errorf("expected 'Auth Feature', got %q", p.Name)
	}
}
