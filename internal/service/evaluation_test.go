package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// evalMockEventStore returns configurable events per run.
type evalMockEventStore struct {
	eventsByRun map[string][]event.AgentEvent
}

func (m *evalMockEventStore) Append(_ context.Context, _ *event.AgentEvent) error { return nil }
func (m *evalMockEventStore) LoadByTask(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *evalMockEventStore) LoadByAgent(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *evalMockEventStore) LoadByRun(_ context.Context, runID string) ([]event.AgentEvent, error) {
	if m.eventsByRun != nil {
		return m.eventsByRun[runID], nil
	}
	return nil, nil
}
func (m *evalMockEventStore) LoadTrajectory(_ context.Context, _ string, _ eventstore.TrajectoryFilter, _ string, _ int) (*eventstore.TrajectoryPage, error) {
	return &eventstore.TrajectoryPage{}, nil
}
func (m *evalMockEventStore) TrajectoryStats(_ context.Context, _ string) (*eventstore.TrajectorySummary, error) {
	return &eventstore.TrajectorySummary{}, nil
}
func (m *evalMockEventStore) LoadEventsRange(_ context.Context, _, _, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *evalMockEventStore) ListCheckpoints(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *evalMockEventStore) AppendAudit(_ context.Context, _ *event.AuditEntry) error { return nil }
func (m *evalMockEventStore) LoadAudit(_ context.Context, _ *event.AuditFilter, _ string, _ int) (*event.AuditPage, error) {
	return nil, nil
}

func TestEvaluationService_HandlePlanComplete_PublishesForCompleted(t *testing.T) {
	// Set up a plan with 2 completed steps each having a run.
	store := &orchMockStore{}
	store.plans = append(store.plans, plan.ExecutionPlan{
		ID:        "plan-eval-1",
		ProjectID: "proj-1",
		Status:    plan.StatusCompleted,
	})
	store.steps = append(store.steps,
		plan.Step{
			ID:      "step-1",
			PlanID:  "plan-eval-1",
			AgentID: "agent-a",
			RunID:   "run-1",
			Status:  plan.StepStatusCompleted,
			Round:   1,
		},
		plan.Step{
			ID:      "step-2",
			PlanID:  "plan-eval-1",
			AgentID: "agent-b",
			RunID:   "run-2",
			Status:  plan.StepStatusCompleted,
			Round:   1,
		},
	)

	es := &evalMockEventStore{
		eventsByRun: map[string][]event.AgentEvent{
			"run-1": {
				{AgentID: "agent-a", Type: event.TypeToolCallRequested, Payload: json.RawMessage(`{"tool":"read","path":"/main.go"}`)},
				{AgentID: "agent-a", Type: event.TypeToolCallResultEv, Payload: json.RawMessage(`{"output":"file contents"}`)},
				{AgentID: "agent-a", Type: event.TypeRunStarted, Payload: json.RawMessage(`{}`)}, // should be skipped
			},
			"run-2": {
				{AgentID: "agent-b", Type: event.TypeToolCallRequested, Payload: json.RawMessage(`{"tool":"write","path":"/fix.go"}`)},
			},
		},
	}

	queue := &runtimeMockQueue{}

	evalSvc := service.NewEvaluationService(store, es, queue)
	evalSvc.HandlePlanComplete(context.Background(), "plan-eval-1", "completed")

	// Check that a message was published to the evaluation subject.
	msg, ok := queue.lastMessage(messagequeue.SubjectEvalGemmasRequest)
	if !ok {
		t.Fatal("expected a message published to evaluation.gemmas.request")
	}

	var payload messagequeue.GemmasEvalRequestPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload.PlanID != "plan-eval-1" {
		t.Errorf("expected plan_id 'plan-eval-1', got %q", payload.PlanID)
	}

	// 2 events from run-1 (tool_called.requested + tool_result) + 1 from run-2 = 3 messages
	if len(payload.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(payload.Messages))
	}
}

func TestEvaluationService_HandlePlanComplete_SkipsFailedPlans(t *testing.T) {
	store := &orchMockStore{}
	store.plans = append(store.plans, plan.ExecutionPlan{
		ID:     "plan-fail-1",
		Status: plan.StatusFailed,
	})

	es := &evalMockEventStore{}
	queue := &runtimeMockQueue{}

	evalSvc := service.NewEvaluationService(store, es, queue)
	evalSvc.HandlePlanComplete(context.Background(), "plan-fail-1", "failed")

	// No message should be published for failed plans.
	queue.mu.Lock()
	count := len(queue.messages)
	queue.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 messages for failed plan, got %d", count)
	}
}

func TestEvaluationService_HandlePlanComplete_NoMessagesSkipsPublish(t *testing.T) {
	store := &orchMockStore{}
	store.plans = append(store.plans, plan.ExecutionPlan{
		ID:        "plan-empty-1",
		ProjectID: "proj-1",
		Status:    plan.StatusCompleted,
	})
	store.steps = append(store.steps,
		plan.Step{
			ID:      "step-empty-1",
			PlanID:  "plan-empty-1",
			AgentID: "agent-a",
			RunID:   "run-empty",
			Status:  plan.StepStatusCompleted,
			Round:   0,
		},
	)

	// Run has events but none are tool call types.
	es := &evalMockEventStore{
		eventsByRun: map[string][]event.AgentEvent{
			"run-empty": {
				{AgentID: "agent-a", Type: event.TypeRunStarted, Payload: json.RawMessage(`{}`)},
				{AgentID: "agent-a", Type: event.TypeRunCompleted, Payload: json.RawMessage(`{}`)},
			},
		},
	}

	queue := &runtimeMockQueue{}

	evalSvc := service.NewEvaluationService(store, es, queue)
	evalSvc.HandlePlanComplete(context.Background(), "plan-empty-1", "completed")

	// No messages extracted -> no publish.
	queue.mu.Lock()
	count := len(queue.messages)
	queue.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 messages when no agent messages found, got %d", count)
	}
}

func TestOrchestratorService_MultipleCallbacks(t *testing.T) {
	_, orchSvc := newOrchTestSetup()

	var called []string
	orchSvc.AddOnPlanComplete(func(_ context.Context, planID, status string) {
		called = append(called, "cb1:"+planID+":"+status)
	})
	orchSvc.AddOnPlanComplete(func(_ context.Context, planID, status string) {
		called = append(called, "cb2:"+planID+":"+status)
	})

	ctx := context.Background()
	req := &plan.CreatePlanRequest{
		Name:      "multi-cb plan",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolParallel,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
		},
	}

	p, err := orchSvc.CreatePlan(ctx, req)
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("start plan: %v", err)
	}

	// Complete the step's run.
	reloaded, _ := orchSvc.GetPlan(ctx, p.ID)
	if len(reloaded.Steps) == 0 {
		t.Fatal("no steps found")
	}
	runID := reloaded.Steps[0].RunID
	if runID == "" {
		t.Fatal("step has no run ID")
	}
	orchSvc.HandleRunCompleted(ctx, runID, "completed")

	if len(called) != 2 {
		t.Fatalf("expected 2 callbacks, got %d: %v", len(called), called)
	}
	if called[0] != "cb1:"+p.ID+":completed" {
		t.Errorf("unexpected first callback: %s", called[0])
	}
	if called[1] != "cb2:"+p.ID+":completed" {
		t.Errorf("unexpected second callback: %s", called[1])
	}
}
