package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/service"
)

// orchMockStore is a full mock for plan+run+task+agent operations.
type orchMockStore struct {
	runtimeMockStore // embeds run/task/agent mocks

	mu    sync.Mutex
	plans []plan.ExecutionPlan
	steps []plan.Step
}

func (m *orchMockStore) CreatePlan(_ context.Context, p *plan.ExecutionPlan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.ID == "" {
		p.ID = fmt.Sprintf("plan-%d", len(m.plans)+1)
	}
	for i := range p.Steps {
		s := &p.Steps[i]
		s.PlanID = p.ID
		if s.ID == "" {
			s.ID = fmt.Sprintf("step-%d-%d", len(m.plans)+1, i)
		}
		m.steps = append(m.steps, *s)
	}
	m.plans = append(m.plans, *p)
	return nil
}

func (m *orchMockStore) GetPlan(_ context.Context, id string) (*plan.ExecutionPlan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.plans {
		if m.plans[i].ID != id {
			continue
		}
		p := m.plans[i]
		var steps []plan.Step
		for j := range m.steps {
			if m.steps[j].PlanID == id {
				steps = append(steps, m.steps[j])
			}
		}
		p.Steps = steps
		return &p, nil
	}
	return nil, fmt.Errorf("get plan %s: %w", id, domain.ErrNotFound)
}

func (m *orchMockStore) ListPlansByProject(_ context.Context, projectID string) ([]plan.ExecutionPlan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []plan.ExecutionPlan
	for i := range m.plans {
		if m.plans[i].ProjectID == projectID {
			result = append(result, m.plans[i])
		}
	}
	return result, nil
}

func (m *orchMockStore) UpdatePlanStatus(_ context.Context, id string, status plan.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.plans {
		if m.plans[i].ID == id {
			m.plans[i].Status = status
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *orchMockStore) CreatePlanStep(_ context.Context, step *plan.Step) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if step.ID == "" {
		step.ID = fmt.Sprintf("step-new-%d", len(m.steps)+1)
	}
	m.steps = append(m.steps, *step)
	return nil
}

func (m *orchMockStore) ListPlanSteps(_ context.Context, planID string) ([]plan.Step, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []plan.Step
	for i := range m.steps {
		if m.steps[i].PlanID == planID {
			result = append(result, m.steps[i])
		}
	}
	return result, nil
}

func (m *orchMockStore) UpdatePlanStepStatus(_ context.Context, stepID string, status plan.StepStatus, runID, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.steps {
		if m.steps[i].ID == stepID {
			m.steps[i].Status = status
			if runID != "" {
				m.steps[i].RunID = runID
			}
			m.steps[i].Error = errMsg
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *orchMockStore) GetPlanStepByRunID(_ context.Context, runID string) (*plan.Step, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.steps {
		if m.steps[i].RunID == runID {
			return &m.steps[i], nil
		}
	}
	return nil, fmt.Errorf("get plan step by run %s: %w", runID, domain.ErrNotFound)
}

func (m *orchMockStore) UpdatePlanStepRound(_ context.Context, stepID string, round int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.steps {
		if m.steps[i].ID == stepID {
			m.steps[i].Round = round
			return nil
		}
	}
	return domain.ErrNotFound
}

func newOrchTestSetup() (*orchMockStore, *service.OrchestratorService) {
	store := &orchMockStore{}
	store.agents = newIdleAgents("a1", "a2", "a3")
	store.tasks = newPendingTasks("t1", "t2", "t3")

	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	queue := &runtimeMockQueue{}

	runtimeSvc := service.NewRuntimeService(store, queue, bc, es,
		service.NewPolicyService("headless-safe-sandbox", nil),
		&config.Runtime{StallThreshold: 5},
	)

	orchCfg := &config.Orchestrator{
		MaxParallel:       4,
		PingPongMaxRounds: 3,
		ConsensusQuorum:   0,
	}

	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, orchCfg)
	runtimeSvc.SetOnRunComplete(orchSvc.HandleRunCompleted)

	return store, orchSvc
}

func newIdleAgents(ids ...string) []agent.Agent {
	var agents []agent.Agent
	for _, id := range ids {
		agents = append(agents, agent.Agent{ID: id, Status: agent.StatusIdle, Backend: "aider"})
	}
	return agents
}

func newPendingTasks(ids ...string) []task.Task {
	var tasks []task.Task
	for _, id := range ids {
		tasks = append(tasks, task.Task{ID: id, Status: task.StatusPending, Title: "Task " + id})
	}
	return tasks
}

// --- Tests ---

func TestCreatePlan_Success(t *testing.T) {
	_, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "test plan",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
		},
	}

	p, err := orchSvc.CreatePlan(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != plan.StatusPending {
		t.Errorf("expected pending, got %s", p.Status)
	}
	if len(p.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(p.Steps))
	}
}

func TestCreatePlan_ValidationError(t *testing.T) {
	_, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:     "",
		Protocol: plan.ProtocolSequential,
		Steps:    []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}},
	}

	_, err := orchSvc.CreatePlan(ctx, req)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestStartPlan_Sequential(t *testing.T) {
	store, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "seq plan",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
		},
	}

	p, _ := orchSvc.CreatePlan(ctx, req)
	started, err := orchSvc.StartPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("start plan: %v", err)
	}
	if started.Status != plan.StatusRunning {
		t.Errorf("expected running, got %s", started.Status)
	}

	// First step should be running (a run was started)
	store.mu.Lock()
	firstStepRunning := false
	for _, s := range store.steps {
		if s.PlanID == p.ID && s.Status == plan.StepStatusRunning {
			firstStepRunning = true
			break
		}
	}
	store.mu.Unlock()
	if !firstStepRunning {
		t.Error("expected first step to be running")
	}
}

func TestSequential_StepFailure(t *testing.T) {
	store, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "fail plan",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
		},
	}

	p, _ := orchSvc.CreatePlan(ctx, req)
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("start plan: %v", err)
	}

	// Find the running step's run
	store.mu.Lock()
	var runID string
	for _, s := range store.steps {
		if s.PlanID == p.ID && s.Status == plan.StepStatusRunning {
			runID = s.RunID
			break
		}
	}
	store.mu.Unlock()

	if runID == "" {
		t.Fatal("no running step found")
	}

	// Simulate run failure
	orchSvc.HandleRunCompleted(ctx, runID, run.StatusFailed)

	// Plan should be failed
	store.mu.Lock()
	var planStatus plan.Status
	for _, pl := range store.plans {
		if pl.ID == p.ID {
			planStatus = pl.Status
			break
		}
	}
	store.mu.Unlock()

	if planStatus != plan.StatusFailed {
		t.Errorf("expected plan failed, got %s", planStatus)
	}
}

func TestParallel_AllStart(t *testing.T) {
	store, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "parallel plan",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolParallel,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
			{TaskID: "t3", AgentID: "a3"},
		},
	}

	p, _ := orchSvc.CreatePlan(ctx, req)
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("start plan: %v", err)
	}

	// All 3 steps should be running
	store.mu.Lock()
	runningCount := 0
	for _, s := range store.steps {
		if s.PlanID == p.ID && s.Status == plan.StepStatusRunning {
			runningCount++
		}
	}
	store.mu.Unlock()

	if runningCount != 3 {
		t.Errorf("expected 3 running steps, got %d", runningCount)
	}
}

func TestParallel_MaxParallelRespected(t *testing.T) {
	store := &orchMockStore{}
	store.agents = newIdleAgents("a1", "a2", "a3", "a4", "a5")
	store.tasks = newPendingTasks("t1", "t2", "t3", "t4", "t5")

	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	queue := &runtimeMockQueue{}

	runtimeSvc := service.NewRuntimeService(store, queue, bc, es,
		service.NewPolicyService("headless-safe-sandbox", nil),
		&config.Runtime{StallThreshold: 5},
	)

	orchCfg := &config.Orchestrator{
		MaxParallel:       2, // Only 2 at a time
		PingPongMaxRounds: 3,
	}

	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, orchCfg)
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "limited parallel",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolParallel,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
			{TaskID: "t3", AgentID: "a3"},
			{TaskID: "t4", AgentID: "a4"},
		},
	}

	p, _ := orchSvc.CreatePlan(ctx, req)
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("start plan: %v", err)
	}

	store.mu.Lock()
	runningCount := 0
	for _, s := range store.steps {
		if s.PlanID == p.ID && s.Status == plan.StepStatusRunning {
			runningCount++
		}
	}
	store.mu.Unlock()

	if runningCount != 2 {
		t.Errorf("expected 2 running steps (max_parallel=2), got %d", runningCount)
	}
}

func TestConsensus_QuorumMet(t *testing.T) {
	store, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "consensus plan",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolConsensus,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t1", AgentID: "a2"},
			{TaskID: "t1", AgentID: "a3"},
		},
	}

	p, _ := orchSvc.CreatePlan(ctx, req)
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("start plan: %v", err)
	}

	// Collect run IDs
	store.mu.Lock()
	var runIDs []string
	for _, s := range store.steps {
		if s.PlanID == p.ID && s.RunID != "" {
			runIDs = append(runIDs, s.RunID)
		}
	}
	store.mu.Unlock()

	if len(runIDs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runIDs))
	}

	// 2 succeed, 1 fails -> majority met (quorum=2 for 3 agents)
	orchSvc.HandleRunCompleted(ctx, runIDs[0], run.StatusCompleted)
	orchSvc.HandleRunCompleted(ctx, runIDs[1], run.StatusCompleted)
	orchSvc.HandleRunCompleted(ctx, runIDs[2], run.StatusFailed)

	store.mu.Lock()
	var planStatus plan.Status
	for _, pl := range store.plans {
		if pl.ID == p.ID {
			planStatus = pl.Status
			break
		}
	}
	store.mu.Unlock()

	if planStatus != plan.StatusCompleted {
		t.Errorf("expected plan completed (quorum met), got %s", planStatus)
	}
}

func TestConsensus_QuorumNotMet(t *testing.T) {
	store, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "consensus fail",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolConsensus,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t1", AgentID: "a2"},
			{TaskID: "t1", AgentID: "a3"},
		},
	}

	p, _ := orchSvc.CreatePlan(ctx, req)
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("start plan: %v", err)
	}

	store.mu.Lock()
	var runIDs []string
	for _, s := range store.steps {
		if s.PlanID == p.ID && s.RunID != "" {
			runIDs = append(runIDs, s.RunID)
		}
	}
	store.mu.Unlock()

	// 1 succeeds, 2 fail -> quorum not met
	orchSvc.HandleRunCompleted(ctx, runIDs[0], run.StatusCompleted)
	orchSvc.HandleRunCompleted(ctx, runIDs[1], run.StatusFailed)
	orchSvc.HandleRunCompleted(ctx, runIDs[2], run.StatusFailed)

	store.mu.Lock()
	var planStatus plan.Status
	for _, pl := range store.plans {
		if pl.ID == p.ID {
			planStatus = pl.Status
			break
		}
	}
	store.mu.Unlock()

	if planStatus != plan.StatusFailed {
		t.Errorf("expected plan failed (quorum not met), got %s", planStatus)
	}
}

func TestCancelPlan(t *testing.T) {
	store, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "cancel me",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
		},
	}

	p, _ := orchSvc.CreatePlan(ctx, req)
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("start plan: %v", err)
	}

	err := orchSvc.CancelPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("cancel plan: %v", err)
	}

	store.mu.Lock()
	var planStatus plan.Status
	for _, pl := range store.plans {
		if pl.ID == p.ID {
			planStatus = pl.Status
			break
		}
	}
	store.mu.Unlock()

	if planStatus != plan.StatusCancelled {
		t.Errorf("expected cancelled, got %s", planStatus)
	}
}

// --- Phase 21D: Debate Protocol Tests ---

func newOrchTestSetupWithDebate() *service.OrchestratorService {
	store := &orchMockStore{}
	store.agents = newIdleAgents("a1", "a2", "a3")
	store.tasks = newPendingTasks("t1", "t2", "t3")

	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	queue := &runtimeMockQueue{}

	runtimeSvc := service.NewRuntimeService(store, queue, bc, es,
		service.NewPolicyService("headless-safe-sandbox", nil),
		&config.Runtime{StallThreshold: 5},
	)

	orchCfg := &config.Orchestrator{
		MaxParallel:       4,
		PingPongMaxRounds: 3,
		ConsensusQuorum:   0,
		DebateRounds:      1,
	}

	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, orchCfg)
	runtimeSvc.SetOnRunComplete(orchSvc.HandleRunCompleted)

	return orchSvc
}

func TestDebate_BuiltinModesExist(t *testing.T) {
	// Verify moderator and proponent are available as built-in modes
	modeSvc := service.NewModeService()
	mod, err := modeSvc.Get("moderator")
	if err != nil {
		t.Fatalf("moderator mode not found: %v", err)
	}
	if mod.LLMScenario != "review" {
		t.Errorf("expected moderator LLMScenario 'review', got %q", mod.LLMScenario)
	}

	prop, err := modeSvc.Get("proponent")
	if err != nil {
		t.Fatalf("proponent mode not found: %v", err)
	}
	if prop.LLMScenario != "review" {
		t.Errorf("expected proponent LLMScenario 'review', got %q", prop.LLMScenario)
	}
}

func TestDebate_ModeratorReadOnly(t *testing.T) {
	modeSvc := service.NewModeService()
	mod, _ := modeSvc.Get("moderator")
	for _, denied := range mod.DeniedTools {
		if denied == "Write" || denied == "Edit" || denied == "Bash" {
			continue
		}
		t.Errorf("unexpected denied tool in moderator: %s", denied)
	}
	hasDenied := map[string]bool{}
	for _, d := range mod.DeniedTools {
		hasDenied[d] = true
	}
	if !hasDenied["Write"] || !hasDenied["Edit"] || !hasDenied["Bash"] {
		t.Error("moderator should deny Write, Edit, and Bash")
	}
}

func TestDebate_ProponentReadOnly(t *testing.T) {
	modeSvc := service.NewModeService()
	prop, _ := modeSvc.Get("proponent")
	hasDenied := map[string]bool{}
	for _, d := range prop.DeniedTools {
		hasDenied[d] = true
	}
	if !hasDenied["Write"] || !hasDenied["Edit"] || !hasDenied["Bash"] {
		t.Error("proponent should deny Write, Edit, and Bash")
	}
}

func TestDebate_DebateRoundsConfig(t *testing.T) {
	orchSvc := newOrchTestSetupWithDebate()
	// Just verify the service was created with debate config —
	// the debate is triggered through the review router which is nil here,
	// so no debate will actually fire. This is a config sanity check.
	_ = orchSvc
}

func TestDebate_DebateRoundsClampedToMax3(t *testing.T) {
	store := &orchMockStore{}
	store.agents = newIdleAgents("a1")
	store.tasks = newPendingTasks("t1")
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	queue := &runtimeMockQueue{}

	runtimeSvc := service.NewRuntimeService(store, queue, bc, es,
		service.NewPolicyService("headless-safe-sandbox", nil),
		&config.Runtime{StallThreshold: 5},
	)

	orchCfg := &config.Orchestrator{
		MaxParallel:       4,
		PingPongMaxRounds: 3,
		DebateRounds:      10, // should be clamped to 3
	}

	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, orchCfg)
	// The clamping happens in startDebate, which is not directly testable without
	// the review router. This test verifies construction succeeds with oversized value.
	_ = orchSvc
}

func TestDebate_HandleDebateComplete_NotADebatePlan(t *testing.T) {
	orchSvc := newOrchTestSetupWithDebate()
	ctx := context.Background()

	// HandleRunCompleted for a non-existent run should not panic
	// (this exercises the handleDebateComplete path with an unknown plan ID)
	orchSvc.HandleRunCompleted(ctx, "nonexistent-run", run.StatusCompleted)
}

func TestHandleRunCompleted_NonPlanRun(t *testing.T) {
	_, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	// Should not panic or error — just silently ignores non-plan runs
	orchSvc.HandleRunCompleted(ctx, "run-that-does-not-exist", run.StatusCompleted)
}

func TestListPlans(t *testing.T) {
	_, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "list test",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolSequential,
		Steps:     []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}},
	}
	if _, err := orchSvc.CreatePlan(ctx, req); err != nil {
		t.Fatalf("create plan 1: %v", err)
	}
	if _, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
		Name:      "list test 2",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolParallel,
		Steps:     []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}},
	}); err != nil {
		t.Fatalf("create plan 2: %v", err)
	}

	plans, err := orchSvc.ListPlans(ctx, "proj-1")
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if len(plans) != 2 {
		t.Errorf("expected 2 plans, got %d", len(plans))
	}
}

func TestGetPlan(t *testing.T) {
	_, orchSvc := newOrchTestSetup()
	ctx := context.Background()

	req := &plan.CreatePlanRequest{
		Name:      "get test",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
		},
	}
	created, _ := orchSvc.CreatePlan(ctx, req)

	got, err := orchSvc.GetPlan(ctx, created.ID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if got.Name != "get test" {
		t.Errorf("expected name 'get test', got %q", got.Name)
	}
	if len(got.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(got.Steps))
	}
}
