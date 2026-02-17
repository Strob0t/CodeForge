package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// --- Mocks ---

var errMockNotFound = fmt.Errorf("mock: %w", domain.ErrNotFound)

type runtimeMockStore struct {
	mu       sync.Mutex
	projects []project.Project
	agents   []agent.Agent
	tasks    []task.Task
	runs     []run.Run
}

func (m *runtimeMockStore) ListProjects(_ context.Context) ([]project.Project, error) {
	return m.projects, nil
}
func (m *runtimeMockStore) GetProject(_ context.Context, id string) (*project.Project, error) {
	for i := range m.projects {
		if m.projects[i].ID == id {
			return &m.projects[i], nil
		}
	}
	return nil, errMockNotFound
}
func (m *runtimeMockStore) CreateProject(_ context.Context, req project.CreateRequest) (*project.Project, error) {
	p := project.Project{ID: "proj-id", Name: req.Name, Provider: req.Provider}
	m.projects = append(m.projects, p)
	return &p, nil
}
func (m *runtimeMockStore) UpdateProject(_ context.Context, _ *project.Project) error { return nil }
func (m *runtimeMockStore) DeleteProject(_ context.Context, _ string) error           { return nil }

func (m *runtimeMockStore) ListAgents(_ context.Context, _ string) ([]agent.Agent, error) {
	return m.agents, nil
}
func (m *runtimeMockStore) GetAgent(_ context.Context, id string) (*agent.Agent, error) {
	for i := range m.agents {
		if m.agents[i].ID == id {
			return &m.agents[i], nil
		}
	}
	return nil, errMockNotFound
}
func (m *runtimeMockStore) CreateAgent(_ context.Context, projectID, name, backend string, config map[string]string) (*agent.Agent, error) {
	a := agent.Agent{ID: "agent-id", ProjectID: projectID, Name: name, Backend: backend, Status: agent.StatusIdle, Config: config}
	m.agents = append(m.agents, a)
	return &a, nil
}
func (m *runtimeMockStore) UpdateAgentStatus(_ context.Context, id string, status agent.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.agents {
		if m.agents[i].ID == id {
			m.agents[i].Status = status
			return nil
		}
	}
	return errMockNotFound
}
func (m *runtimeMockStore) DeleteAgent(_ context.Context, _ string) error { return nil }

func (m *runtimeMockStore) ListTasks(_ context.Context, _ string) ([]task.Task, error) {
	return m.tasks, nil
}
func (m *runtimeMockStore) GetTask(_ context.Context, id string) (*task.Task, error) {
	for i := range m.tasks {
		if m.tasks[i].ID == id {
			return &m.tasks[i], nil
		}
	}
	return nil, errMockNotFound
}
func (m *runtimeMockStore) CreateTask(_ context.Context, req task.CreateRequest) (*task.Task, error) {
	t := task.Task{ID: "task-id", ProjectID: req.ProjectID, Title: req.Title, Status: task.StatusPending}
	m.tasks = append(m.tasks, t)
	return &t, nil
}
func (m *runtimeMockStore) UpdateTaskStatus(_ context.Context, id string, status task.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.tasks {
		if m.tasks[i].ID == id {
			m.tasks[i].Status = status
			return nil
		}
	}
	return errMockNotFound
}
func (m *runtimeMockStore) UpdateTaskResult(_ context.Context, _ string, _ task.Result, _ float64) error {
	return nil
}

func (m *runtimeMockStore) CreateRun(_ context.Context, r *run.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == "" {
		r.ID = fmt.Sprintf("run-%d", len(m.runs)+1)
	}
	r.CreatedAt = time.Now()
	r.StartedAt = time.Now()
	m.runs = append(m.runs, *r)
	return nil
}
func (m *runtimeMockStore) GetRun(_ context.Context, id string) (*run.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].ID == id {
			return &m.runs[i], nil
		}
	}
	return nil, errMockNotFound
}
func (m *runtimeMockStore) UpdateRunStatus(_ context.Context, id string, status run.Status, stepCount int, costUSD float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].ID == id {
			m.runs[i].Status = status
			m.runs[i].StepCount = stepCount
			m.runs[i].CostUSD = costUSD
			return nil
		}
	}
	return errMockNotFound
}
func (m *runtimeMockStore) CompleteRun(_ context.Context, id string, status run.Status, errMsg string, costUSD float64, stepCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].ID != id {
			continue
		}
		m.runs[i].Status = status
		m.runs[i].Error = errMsg
		m.runs[i].CostUSD = costUSD
		m.runs[i].StepCount = stepCount
		now := time.Now()
		m.runs[i].CompletedAt = &now
		return nil
	}
	return errMockNotFound
}
func (m *runtimeMockStore) ListRunsByTask(_ context.Context, taskID string) ([]run.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []run.Run
	for i := range m.runs {
		if m.runs[i].TaskID == taskID {
			result = append(result, m.runs[i])
		}
	}
	return result, nil
}

type runtimeMockQueue struct {
	mu       sync.Mutex
	messages []publishedMsg
}

type publishedMsg struct {
	Subject string
	Data    []byte
}

func (m *runtimeMockQueue) Publish(_ context.Context, subject string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, publishedMsg{Subject: subject, Data: data})
	return nil
}
func (m *runtimeMockQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (m *runtimeMockQueue) Drain() error      { return nil }
func (m *runtimeMockQueue) Close() error      { return nil }
func (m *runtimeMockQueue) IsConnected() bool { return true }

func (m *runtimeMockQueue) lastMessage(subject string) (publishedMsg, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Subject == subject {
			return m.messages[i], true
		}
	}
	return publishedMsg{}, false
}

type runtimeMockBroadcaster struct {
	mu     sync.Mutex
	events []broadcastedEvent
}

type broadcastedEvent struct {
	EventType string
	Data      any
}

func (m *runtimeMockBroadcaster) BroadcastEvent(_ context.Context, eventType string, data any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, broadcastedEvent{EventType: eventType, Data: data})
}

type runtimeMockEventStore struct{}

func (m *runtimeMockEventStore) Append(_ context.Context, _ *event.AgentEvent) error { return nil }
func (m *runtimeMockEventStore) LoadByTask(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *runtimeMockEventStore) LoadByAgent(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}

// --- Helper ---

func newRuntimeTestEnv() (*service.RuntimeService, *runtimeMockStore, *runtimeMockQueue, *runtimeMockBroadcaster) {
	store := &runtimeMockStore{
		agents: []agent.Agent{
			{ID: "agent-1", ProjectID: "proj-1", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
		},
		tasks: []task.Task{
			{ID: "task-1", ProjectID: "proj-1", Title: "Fix bug", Prompt: "Fix the null pointer", Status: task.StatusPending},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc)
	return svc, store, queue, bc
}

// --- Tests ---

func TestStartRun_Success(t *testing.T) {
	svc, store, queue, bc := newRuntimeTestEnv()
	ctx := context.Background()

	req := run.StartRequest{
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
	}
	r, err := svc.StartRun(ctx, &req)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}
	if r.ID == "" {
		t.Fatal("expected run ID to be set")
	}
	if r.Status != run.StatusRunning {
		t.Fatalf("expected status running, got %s", r.Status)
	}
	if r.PolicyProfile != "headless-safe-sandbox" {
		t.Fatalf("expected default policy profile, got %q", r.PolicyProfile)
	}

	// Verify NATS message was published
	msg, ok := queue.lastMessage(messagequeue.SubjectRunStart)
	if !ok {
		t.Fatal("expected run start message to be published to NATS")
	}
	var payload messagequeue.RunStartPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("failed to unmarshal run start payload: %v", err)
	}
	if payload.RunID != r.ID {
		t.Fatalf("expected run_id %q in payload, got %q", r.ID, payload.RunID)
	}

	// Verify agent status was updated
	ag, _ := store.GetAgent(ctx, "agent-1")
	if ag.Status != agent.StatusRunning {
		t.Fatalf("expected agent status running, got %s", ag.Status)
	}

	// Verify WS event was broadcast
	if len(bc.events) == 0 {
		t.Fatal("expected at least one WS event to be broadcast")
	}
}

func TestStartRun_MissingTaskID(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	req := run.StartRequest{
		AgentID:   "agent-1",
		ProjectID: "proj-1",
	}
	_, err := svc.StartRun(ctx, &req)
	if err == nil {
		t.Fatal("expected error for missing task_id")
	}
}

func TestStartRun_MissingAgentID(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	req := run.StartRequest{
		TaskID:    "task-1",
		ProjectID: "proj-1",
	}
	_, err := svc.StartRun(ctx, &req)
	if err == nil {
		t.Fatal("expected error for missing agent_id")
	}
}

func TestStartRun_AgentNotFound(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	req := run.StartRequest{
		TaskID:    "task-1",
		AgentID:   "nonexistent",
		ProjectID: "proj-1",
	}
	_, err := svc.StartRun(ctx, &req)
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestStartRun_TaskNotFound(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	req := run.StartRequest{
		TaskID:    "nonexistent",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
	}
	_, err := svc.StartRun(ctx, &req)
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestStartRun_CustomPolicyProfile(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	req := run.StartRequest{
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
	}
	r, err := svc.StartRun(ctx, &req)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}
	if r.PolicyProfile != "plan-readonly" {
		t.Fatalf("expected policy profile 'plan-readonly', got %q", r.PolicyProfile)
	}
}

func TestStartRun_UnknownPolicyProfile(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	req := run.StartRequest{
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "nonexistent",
	}
	_, err := svc.StartRun(ctx, &req)
	if err == nil {
		t.Fatal("expected error for unknown policy profile")
	}
}

func TestHandleToolCallRequest_Allow(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// Create a running run using the headless-safe-sandbox profile which allows Read
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-1",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StepCount:     0,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	req := messagequeue.ToolCallRequestPayload{
		RunID:  "run-1",
		CallID: "call-1",
		Tool:   "Read",
		Path:   "src/main.go",
	}
	err := svc.HandleToolCallRequest(ctx, &req)
	if err != nil {
		t.Fatalf("HandleToolCallRequest failed: %v", err)
	}

	// Check response was published
	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response to be published")
	}
	var resp messagequeue.ToolCallResponsePayload
	_ = json.Unmarshal(msg.Data, &resp)
	if resp.Decision != "allow" {
		t.Fatalf("expected 'allow' decision, got %q", resp.Decision)
	}
	if resp.CallID != "call-1" {
		t.Fatalf("expected call_id 'call-1', got %q", resp.CallID)
	}
}

func TestHandleToolCallRequest_DenyByPolicy(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// plan-readonly profile denies Edit/Write/Bash
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-2",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
		Status:        run.StatusRunning,
		StepCount:     0,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	req := messagequeue.ToolCallRequestPayload{
		RunID:  "run-2",
		CallID: "call-2",
		Tool:   "Edit",
		Path:   "src/main.go",
	}
	err := svc.HandleToolCallRequest(ctx, &req)
	if err != nil {
		t.Fatalf("HandleToolCallRequest failed: %v", err)
	}

	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response to be published")
	}
	var resp messagequeue.ToolCallResponsePayload
	_ = json.Unmarshal(msg.Data, &resp)
	if resp.Decision == "allow" {
		t.Fatal("expected denial for Edit in plan-readonly, got allow")
	}
}

func TestHandleToolCallRequest_TerminationMaxSteps(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// headless-safe-sandbox has max_steps: 200 by default
	// Set step count to 200 to trigger termination
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-3",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StepCount:     200,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	req := messagequeue.ToolCallRequestPayload{
		RunID:  "run-3",
		CallID: "call-3",
		Tool:   "Read",
		Path:   "main.go",
	}
	err := svc.HandleToolCallRequest(ctx, &req)
	if err != nil {
		t.Fatalf("HandleToolCallRequest failed: %v", err)
	}

	// Should be denied due to max steps
	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response to be published")
	}
	var resp messagequeue.ToolCallResponsePayload
	_ = json.Unmarshal(msg.Data, &resp)
	if resp.Decision != "deny" {
		t.Fatalf("expected 'deny' for max steps termination, got %q", resp.Decision)
	}

	// Run should be completed with timeout status
	r, _ := store.GetRun(ctx, "run-3")
	if r.Status != run.StatusTimeout {
		t.Fatalf("expected run status timeout, got %s", r.Status)
	}
}

func TestHandleToolCallRequest_RunNotRunning(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-done",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusCompleted,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	req := messagequeue.ToolCallRequestPayload{
		RunID:  "run-done",
		CallID: "call-x",
		Tool:   "Read",
	}
	err := svc.HandleToolCallRequest(ctx, &req)
	if err != nil {
		t.Fatalf("HandleToolCallRequest failed: %v", err)
	}

	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response to be published")
	}
	var resp messagequeue.ToolCallResponsePayload
	_ = json.Unmarshal(msg.Data, &resp)
	if resp.Decision != "deny" {
		t.Fatalf("expected 'deny' for non-running run, got %q", resp.Decision)
	}
}

func TestHandleToolCallRequest_RunNotFound(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	req := messagequeue.ToolCallRequestPayload{
		RunID:  "nonexistent",
		CallID: "call-x",
		Tool:   "Read",
	}
	err := svc.HandleToolCallRequest(ctx, &req)
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

func TestHandleToolCallResult_Success(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:        "run-r1",
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
		Status:    run.StatusRunning,
		CostUSD:   0.01,
		StartedAt: time.Now(),
	})
	store.mu.Unlock()

	result := messagequeue.ToolCallResultPayload{
		RunID:   "run-r1",
		CallID:  "call-1",
		Success: true,
		Output:  "file contents",
		CostUSD: 0.005,
	}
	err := svc.HandleToolCallResult(ctx, &result)
	if err != nil {
		t.Fatalf("HandleToolCallResult failed: %v", err)
	}

	// Verify cost was updated
	r, _ := store.GetRun(ctx, "run-r1")
	if r.CostUSD < 0.015 {
		t.Fatalf("expected cost >= 0.015, got %f", r.CostUSD)
	}
}

func TestHandleRunComplete_Success(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-c1",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	payload := messagequeue.RunCompletePayload{
		RunID:     "run-c1",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "completed",
		Output:    "all done",
		StepCount: 5,
		CostUSD:   0.02,
	}
	err := svc.HandleRunComplete(ctx, &payload)
	if err != nil {
		t.Fatalf("HandleRunComplete failed: %v", err)
	}

	// Verify run status
	r, _ := store.GetRun(ctx, "run-c1")
	if r.Status != run.StatusCompleted {
		t.Fatalf("expected run status completed, got %s", r.Status)
	}

	// Verify agent set back to idle
	ag, _ := store.GetAgent(ctx, "agent-1")
	if ag.Status != agent.StatusIdle {
		t.Fatalf("expected agent status idle, got %s", ag.Status)
	}
}

func TestHandleRunComplete_Failed(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-c2",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	payload := messagequeue.RunCompletePayload{
		RunID:     "run-c2",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Error:     "something went wrong",
		StepCount: 3,
		CostUSD:   0.01,
	}
	err := svc.HandleRunComplete(ctx, &payload)
	if err != nil {
		t.Fatalf("HandleRunComplete failed: %v", err)
	}

	r, _ := store.GetRun(ctx, "run-c2")
	if r.Status != run.StatusFailed {
		t.Fatalf("expected run status failed, got %s", r.Status)
	}
}

func TestCancelRun_Success(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:        "run-cancel",
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
		Status:    run.StatusRunning,
		StartedAt: time.Now(),
	})
	store.mu.Unlock()

	err := svc.CancelRun(ctx, "run-cancel")
	if err != nil {
		t.Fatalf("CancelRun failed: %v", err)
	}

	// Verify run status
	r, _ := store.GetRun(ctx, "run-cancel")
	if r.Status != run.StatusCancelled {
		t.Fatalf("expected run status cancelled, got %s", r.Status)
	}

	// Verify cancel message published
	_, ok := queue.lastMessage(messagequeue.SubjectRunCancel)
	if !ok {
		t.Fatal("expected cancel message to be published to NATS")
	}
}

func TestCancelRun_NotActive(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:        "run-completed",
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
		Status:    run.StatusCompleted,
		StartedAt: time.Now(),
	})
	store.mu.Unlock()

	err := svc.CancelRun(ctx, "run-completed")
	if err == nil {
		t.Fatal("expected error for cancelling non-active run")
	}
}

func TestCancelRun_NotFound(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	err := svc.CancelRun(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

func TestGetRun_Success(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:     "run-get",
		TaskID: "task-1",
		Status: run.StatusRunning,
	})
	store.mu.Unlock()

	r, err := svc.GetRun(ctx, "run-get")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if r.ID != "run-get" {
		t.Fatalf("expected run ID 'run-get', got %q", r.ID)
	}
}

func TestGetRun_NotFound(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	_, err := svc.GetRun(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

func TestListRunsByTask(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs,
		run.Run{ID: "r1", TaskID: "task-1", Status: run.StatusCompleted},
		run.Run{ID: "r2", TaskID: "task-1", Status: run.StatusRunning},
		run.Run{ID: "r3", TaskID: "task-other", Status: run.StatusRunning},
	)
	store.mu.Unlock()

	runs, err := svc.ListRunsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListRunsByTask failed: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs for task-1, got %d", len(runs))
	}
}

func TestStartSubscribers(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	cancels, err := svc.StartSubscribers(ctx)
	if err != nil {
		t.Fatalf("StartSubscribers failed: %v", err)
	}
	if len(cancels) != 4 {
		t.Fatalf("expected 4 cancel functions (4 subscriptions), got %d", len(cancels))
	}

	// Call all cancel functions to ensure no panics
	for _, cancel := range cancels {
		cancel()
	}
}
