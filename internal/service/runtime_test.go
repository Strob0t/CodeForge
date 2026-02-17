package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// --- Mocks ---

var errMockNotFound = fmt.Errorf("mock: %w", domain.ErrNotFound)

type runtimeMockStore struct {
	mu             sync.Mutex
	projects       []project.Project
	agents         []agent.Agent
	tasks          []task.Task
	runs           []run.Run
	teams          []agent.Team
	contextPacks   []cfcontext.ContextPack
	sharedContexts []cfcontext.SharedContext
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
func (m *runtimeMockStore) CreateAgent(_ context.Context, projectID, name, backend string, cfg map[string]string, limits *resource.Limits) (*agent.Agent, error) {
	a := agent.Agent{ID: "agent-id", ProjectID: projectID, Name: name, Backend: backend, Status: agent.StatusIdle, Config: cfg, ResourceLimits: limits}
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
func (m *runtimeMockStore) CompleteRun(_ context.Context, id string, status run.Status, output, errMsg string, costUSD float64, stepCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].ID != id {
			continue
		}
		m.runs[i].Status = status
		m.runs[i].Output = output
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

// --- Plan stub methods (satisfy database.Store interface) ---

func (m *runtimeMockStore) CreatePlan(_ context.Context, _ *plan.ExecutionPlan) error { return nil }
func (m *runtimeMockStore) GetPlan(_ context.Context, _ string) (*plan.ExecutionPlan, error) {
	return nil, errMockNotFound
}
func (m *runtimeMockStore) ListPlansByProject(_ context.Context, _ string) ([]plan.ExecutionPlan, error) {
	return nil, nil
}
func (m *runtimeMockStore) UpdatePlanStatus(_ context.Context, _ string, _ plan.Status) error {
	return nil
}
func (m *runtimeMockStore) CreatePlanStep(_ context.Context, _ *plan.Step) error { return nil }
func (m *runtimeMockStore) ListPlanSteps(_ context.Context, _ string) ([]plan.Step, error) {
	return nil, nil
}
func (m *runtimeMockStore) UpdatePlanStepStatus(_ context.Context, _ string, _ plan.StepStatus, _, _ string) error {
	return nil
}
func (m *runtimeMockStore) GetPlanStepByRunID(_ context.Context, _ string) (*plan.Step, error) {
	return nil, errMockNotFound
}
func (m *runtimeMockStore) UpdatePlanStepRound(_ context.Context, _ string, _ int) error { return nil }

// --- Agent Team methods (satisfy database.Store interface) ---

func (m *runtimeMockStore) CreateTeam(_ context.Context, req agent.CreateTeamRequest) (*agent.Team, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := agent.Team{
		ID:        fmt.Sprintf("team-%d", len(m.teams)+1),
		ProjectID: req.ProjectID,
		Name:      req.Name,
		Protocol:  req.Protocol,
		Status:    agent.TeamStatusInitializing,
		Version:   1,
	}
	for i, mr := range req.Members {
		t.Members = append(t.Members, agent.TeamMember{
			ID:      fmt.Sprintf("tm-%d-%d", len(m.teams)+1, i),
			TeamID:  t.ID,
			AgentID: mr.AgentID,
			Role:    mr.Role,
		})
	}
	m.teams = append(m.teams, t)
	return &t, nil
}

func (m *runtimeMockStore) GetTeam(_ context.Context, id string) (*agent.Team, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.teams {
		if m.teams[i].ID == id {
			return &m.teams[i], nil
		}
	}
	return nil, errMockNotFound
}

func (m *runtimeMockStore) ListTeamsByProject(_ context.Context, projectID string) ([]agent.Team, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []agent.Team
	for i := range m.teams {
		if m.teams[i].ProjectID == projectID {
			result = append(result, m.teams[i])
		}
	}
	return result, nil
}

func (m *runtimeMockStore) UpdateTeamStatus(_ context.Context, id string, status agent.TeamStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.teams {
		if m.teams[i].ID == id {
			m.teams[i].Status = status
			return nil
		}
	}
	return errMockNotFound
}

func (m *runtimeMockStore) DeleteTeam(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.teams {
		if m.teams[i].ID == id {
			m.teams = append(m.teams[:i], m.teams[i+1:]...)
			return nil
		}
	}
	return errMockNotFound
}

// --- Context Pack mocks ---

func (m *runtimeMockStore) CreateContextPack(_ context.Context, pack *cfcontext.ContextPack) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pack.ID = fmt.Sprintf("cp-%d", len(m.contextPacks)+1)
	pack.CreatedAt = time.Now()
	for i := range pack.Entries {
		pack.Entries[i].ID = fmt.Sprintf("ce-%d-%d", len(m.contextPacks)+1, i)
		pack.Entries[i].PackID = pack.ID
	}
	m.contextPacks = append(m.contextPacks, *pack)
	return nil
}

func (m *runtimeMockStore) GetContextPack(_ context.Context, id string) (*cfcontext.ContextPack, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.contextPacks {
		if m.contextPacks[i].ID == id {
			cp := m.contextPacks[i]
			return &cp, nil
		}
	}
	return nil, errMockNotFound
}

func (m *runtimeMockStore) GetContextPackByTask(_ context.Context, taskID string) (*cfcontext.ContextPack, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.contextPacks) - 1; i >= 0; i-- {
		if m.contextPacks[i].TaskID == taskID {
			cp := m.contextPacks[i]
			return &cp, nil
		}
	}
	return nil, errMockNotFound
}

func (m *runtimeMockStore) DeleteContextPack(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.contextPacks {
		if m.contextPacks[i].ID == id {
			m.contextPacks = append(m.contextPacks[:i], m.contextPacks[i+1:]...)
			return nil
		}
	}
	return errMockNotFound
}

// --- Shared Context mocks ---

func (m *runtimeMockStore) CreateSharedContext(_ context.Context, sc *cfcontext.SharedContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sc.ID = fmt.Sprintf("sc-%d", len(m.sharedContexts)+1)
	sc.Version = 1
	sc.CreatedAt = time.Now()
	sc.UpdatedAt = sc.CreatedAt
	m.sharedContexts = append(m.sharedContexts, *sc)
	return nil
}

func (m *runtimeMockStore) GetSharedContext(_ context.Context, id string) (*cfcontext.SharedContext, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.sharedContexts {
		if m.sharedContexts[i].ID == id {
			sc := m.sharedContexts[i]
			return &sc, nil
		}
	}
	return nil, errMockNotFound
}

func (m *runtimeMockStore) GetSharedContextByTeam(_ context.Context, teamID string) (*cfcontext.SharedContext, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.sharedContexts {
		if m.sharedContexts[i].TeamID == teamID {
			sc := m.sharedContexts[i]
			return &sc, nil
		}
	}
	return nil, errMockNotFound
}

func (m *runtimeMockStore) AddSharedContextItem(_ context.Context, req cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.sharedContexts {
		if m.sharedContexts[i].TeamID == req.TeamID {
			item := cfcontext.SharedContextItem{
				ID:        fmt.Sprintf("sci-%d-%d", i, len(m.sharedContexts[i].Items)),
				SharedID:  m.sharedContexts[i].ID,
				Key:       req.Key,
				Value:     req.Value,
				Author:    req.Author,
				Tokens:    cfcontext.EstimateTokens(req.Value),
				CreatedAt: time.Now(),
			}
			m.sharedContexts[i].Items = append(m.sharedContexts[i].Items, item)
			m.sharedContexts[i].Version++
			return &item, nil
		}
	}
	return nil, errMockNotFound
}

func (m *runtimeMockStore) DeleteSharedContext(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.sharedContexts {
		if m.sharedContexts[i].ID == id {
			m.sharedContexts = append(m.sharedContexts[:i], m.sharedContexts[i+1:]...)
			return nil
		}
	}
	return errMockNotFound
}

// --- Repo Map mocks ---

func (m *runtimeMockStore) UpsertRepoMap(_ context.Context, rm *cfcontext.RepoMap) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rm.ID == "" {
		rm.ID = fmt.Sprintf("rm-%d", time.Now().UnixNano())
	}
	rm.Version = 1
	rm.CreatedAt = time.Now()
	rm.UpdatedAt = rm.CreatedAt
	return nil
}

func (m *runtimeMockStore) GetRepoMap(_ context.Context, _ string) (*cfcontext.RepoMap, error) {
	return nil, errMockNotFound
}

func (m *runtimeMockStore) DeleteRepoMap(_ context.Context, _ string) error {
	return nil
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
		projects: []project.Project{
			{ID: "proj-1", Name: "test-project", WorkspacePath: "/tmp/test-workspace"},
		},
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
	runtimeCfg := config.Runtime{
		StallThreshold:       5,
		QualityGateTimeout:   60 * time.Second,
		DefaultTestCommand:   "go test ./...",
		DefaultLintCommand:   "golangci-lint run ./...",
		DeliveryCommitPrefix: "codeforge:",
	}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)
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

	// Use plan-readonly (no quality gates) so run completes directly
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-c1",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
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

// --- Phase 4C: Stall Detection Tests ---

func TestHandleToolCallResult_StallDetected(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	// headless-safe-sandbox has StallDetection: true, StallThreshold: 5
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-stall",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StepCount:     10,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	// Start a run to create the stall tracker
	// The tracker is created in StartRun, but since we manually added the run,
	// we need to trigger the tracker manually via StartRun or directly test
	// We'll create a proper run via StartRun first
	req := run.StartRequest{
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
	}
	r, err := svc.StartRun(ctx, &req)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Feed 5 non-progress results (Read tool) to trigger stall
	for i := range 5 {
		result := messagequeue.ToolCallResultPayload{
			RunID:   r.ID,
			CallID:  fmt.Sprintf("call-%d", i),
			Tool:    "Read",
			Success: true,
			Output:  fmt.Sprintf("output-%d", i),
		}
		err := svc.HandleToolCallResult(ctx, &result)
		if err != nil {
			t.Fatalf("HandleToolCallResult[%d] failed: %v", i, err)
		}
	}

	// Run should be terminated as failed
	stalled, _ := store.GetRun(ctx, r.ID)
	if stalled.Status != run.StatusFailed {
		t.Fatalf("expected run status failed after stall, got %s", stalled.Status)
	}

	// Agent should be set back to idle
	ag, _ := store.GetAgent(ctx, "agent-1")
	if ag.Status != agent.StatusIdle {
		t.Fatalf("expected agent idle after stall, got %s", ag.Status)
	}

	// WS event should include stall status
	bc.mu.Lock()
	found := false
	for _, ev := range bc.events {
		if ev.EventType == "run.status" {
			if statusEv, ok := ev.Data.(ws.RunStatusEvent); ok && statusEv.RunID == r.ID && statusEv.Status == "failed" {
				found = true
			}
		}
	}
	bc.mu.Unlock()
	if !found {
		t.Fatal("expected run.status WS event with failed status after stall")
	}
}

func TestHandleToolCallResult_NoStallWithProgress(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
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

	// Alternate between non-progress and progress steps
	for i := range 10 {
		tool := "Read"
		if i%2 == 0 {
			tool = "Edit" // progress tool
		}
		result := messagequeue.ToolCallResultPayload{
			RunID:   r.ID,
			CallID:  fmt.Sprintf("call-%d", i),
			Tool:    tool,
			Success: true,
			Output:  fmt.Sprintf("unique-output-%d", i),
		}
		_ = svc.HandleToolCallResult(ctx, &result)
	}

	// Run should still be running (no stall because progress interleaved)
	rr, _ := store.GetRun(ctx, r.ID)
	if rr.Status == run.StatusFailed {
		t.Fatal("run should not have stalled with interleaved progress")
	}
}

// --- Phase 4C: Quality Gate Tests ---

func TestHandleRunComplete_QualityGateTriggered(t *testing.T) {
	svc, store, queue, bc := newRuntimeTestEnv()
	ctx := context.Background()

	// Create a run with headless-safe-sandbox (requires tests + lint)
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-qg",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	payload := messagequeue.RunCompletePayload{
		RunID:     "run-qg",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "completed",
		StepCount: 5,
		CostUSD:   0.02,
	}
	err := svc.HandleRunComplete(ctx, &payload)
	if err != nil {
		t.Fatalf("HandleRunComplete failed: %v", err)
	}

	// Run should be in quality_gate status (not finalized)
	r, _ := store.GetRun(ctx, "run-qg")
	if r.Status != run.StatusQualityGate {
		t.Fatalf("expected run status quality_gate, got %s", r.Status)
	}

	// Quality gate request should be published to NATS
	msg, ok := queue.lastMessage(messagequeue.SubjectQualityGateRequest)
	if !ok {
		t.Fatal("expected quality gate request to be published")
	}
	var gateReq messagequeue.QualityGateRequestPayload
	_ = json.Unmarshal(msg.Data, &gateReq)
	if gateReq.RunID != "run-qg" {
		t.Fatalf("expected run_id 'run-qg' in gate request, got %q", gateReq.RunID)
	}
	if !gateReq.RunTests {
		t.Fatal("expected run_tests=true in gate request")
	}
	if !gateReq.RunLint {
		t.Fatal("expected run_lint=true in gate request")
	}

	// WS event for quality gate started
	bc.mu.Lock()
	foundGate := false
	for _, ev := range bc.events {
		if ev.EventType == "run.qualitygate" {
			foundGate = true
		}
	}
	bc.mu.Unlock()
	if !foundGate {
		t.Fatal("expected run.qualitygate WS event")
	}
}

func TestHandleQualityGateResult_Pass(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-qgp",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusQualityGate,
		StepCount:     5,
		CostUSD:       0.02,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	passed := true
	result := messagequeue.QualityGateResultPayload{
		RunID:       "run-qgp",
		TestsPassed: &passed,
		LintPassed:  &passed,
	}
	err := svc.HandleQualityGateResult(ctx, &result)
	if err != nil {
		t.Fatalf("HandleQualityGateResult failed: %v", err)
	}

	// Run should be completed
	r, _ := store.GetRun(ctx, "run-qgp")
	if r.Status != run.StatusCompleted {
		t.Fatalf("expected run status completed after gate pass, got %s", r.Status)
	}

	// Agent should be idle
	ag, _ := store.GetAgent(ctx, "agent-1")
	if ag.Status != agent.StatusIdle {
		t.Fatalf("expected agent idle after gate pass, got %s", ag.Status)
	}
}

func TestHandleQualityGateResult_FailWithRollback(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// headless-safe-sandbox has RollbackOnGateFail: true
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-qgf",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusQualityGate,
		StepCount:     5,
		CostUSD:       0.02,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	failed := false
	passed := true
	result := messagequeue.QualityGateResultPayload{
		RunID:       "run-qgf",
		TestsPassed: &failed,
		LintPassed:  &passed,
	}
	err := svc.HandleQualityGateResult(ctx, &result)
	if err != nil {
		t.Fatalf("HandleQualityGateResult failed: %v", err)
	}

	// Run should be failed (rollback)
	r, _ := store.GetRun(ctx, "run-qgf")
	if r.Status != run.StatusFailed {
		t.Fatalf("expected run status failed after gate fail with rollback, got %s", r.Status)
	}
}

func TestHandleRunComplete_NoGateWhenPolicyOff(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// plan-readonly has no quality gates
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-nogate",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	payload := messagequeue.RunCompletePayload{
		RunID:     "run-nogate",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "completed",
		StepCount: 3,
		CostUSD:   0.01,
	}
	err := svc.HandleRunComplete(ctx, &payload)
	if err != nil {
		t.Fatalf("HandleRunComplete failed: %v", err)
	}

	// Run should be directly completed (no quality_gate intermediate)
	r, _ := store.GetRun(ctx, "run-nogate")
	if r.Status != run.StatusCompleted {
		t.Fatalf("expected run status completed (no gate), got %s", r.Status)
	}

	// No quality gate request should be published
	_, ok := queue.lastMessage(messagequeue.SubjectQualityGateRequest)
	if ok {
		t.Fatal("expected no quality gate request for plan-readonly profile")
	}
}

func TestStartSubscribers(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	cancels, err := svc.StartSubscribers(ctx)
	if err != nil {
		t.Fatalf("StartSubscribers failed: %v", err)
	}
	if len(cancels) != 5 {
		t.Fatalf("expected 5 cancel functions (5 subscriptions), got %d", len(cancels))
	}

	// Call all cancel functions to ensure no panics
	for _, cancel := range cancels {
		cancel()
	}
}
