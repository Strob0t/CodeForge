package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	lspDomain "github.com/Strob0t/CodeForge/internal/domain/lsp"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
	lspPort "github.com/Strob0t/CodeForge/internal/port/lsp"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// These tests pin down which tenant every WebSocket event is scoped to
// (KI-12). The hub delivers an event only to the clients of the tenant in
// its context and drops events without one, so a service must attach the
// owning tenant, never the default tenant by accident.

const (
	scopeTenantA = "aaaaaaaa-0000-0000-0000-000000000001"
	scopeTenantB = "bbbbbbbb-0000-0000-0000-000000000002"
)

// scopedEvent is one broadcast and the tenant its context carried.
type scopedEvent struct {
	eventType string
	tenant    string // "" when the context carries no tenant (the hub drops it)
}

// tenantRecorder is a broadcast.Broadcaster that records event scopes.
type tenantRecorder struct {
	mu     sync.Mutex
	events []scopedEvent
}

func (r *tenantRecorder) BroadcastEvent(ctx context.Context, eventType string, _ any) {
	tenant, _ := tenantctx.Lookup(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, scopedEvent{eventType: eventType, tenant: tenant})
}

func (r *tenantRecorder) snapshot() []scopedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]scopedEvent(nil), r.events...)
}

// assertScoped fails unless at least one event was broadcast and every event
// is scoped to want.
func assertScoped(t *testing.T, events []scopedEvent, want string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("no event was broadcast")
	}
	for _, ev := range events {
		if ev.tenant != want {
			t.Errorf("event %s scoped to tenant %q, want %q", ev.eventType, ev.tenant, want)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

// --- Worker streams (runs.output, trajectory, agents.output, tasks.*) ---

func TestRuntimeWorkerStreams_ScopeToEchoedTenant(t *testing.T) {
	tests := []struct {
		name       string
		subject    string
		payload    map[string]any
		wantTenant string
	}{
		{
			name:       "run output",
			subject:    messagequeue.SubjectRunOutput,
			payload:    map[string]any{"run_id": "run-1", "task_id": "run-1", "tenant_id": scopeTenantA, "line": "hello", "stream": "stdout"},
			wantTenant: scopeTenantA,
		},
		{
			name:       "run output without tenant is not attributed to the default tenant",
			subject:    messagequeue.SubjectRunOutput,
			payload:    map[string]any{"run_id": "run-1", "task_id": "run-1", "line": "hello", "stream": "stdout"},
			wantTenant: "",
		},
		{
			name:    "trajectory event with roadmap proposal",
			subject: messagequeue.SubjectTrajectoryEvent,
			payload: map[string]any{
				"event_type": "agent.roadmap_proposed", "run_id": "run-1", "project_id": "proj-1", "tenant_id": scopeTenantB,
				"data": map[string]any{"proposal_id": "p-1", "action": "create_milestone", "milestone_title": "M"},
			},
			wantTenant: scopeTenantB,
		},
		{
			name:       "trajectory event without tenant",
			subject:    messagequeue.SubjectTrajectoryEvent,
			payload:    map[string]any{"event_type": "agent.tool_called", "run_id": "run-1", "project_id": "proj-1"},
			wantTenant: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &tenantRecorder{}
			queue := newHandlerCapturingQueue()
			svc := &RuntimeService{
				hub:        rec,
				queue:      queue,
				events:     &trajectoryMockEventStore{},
				runtimeCfg: &config.Runtime{},
				state:      NewRunStateManager(),
			}
			if _, err := svc.StartSubscribers(context.Background()); err != nil {
				t.Fatalf("StartSubscribers: %v", err)
			}
			handler, ok := queue.getHandler(tt.subject)
			if !ok {
				t.Fatalf("no handler for %s", tt.subject)
			}

			if err := handler(context.Background(), tt.subject, mustJSON(t, tt.payload)); err != nil {
				t.Fatalf("handler: %v", err)
			}
			assertScoped(t, rec.snapshot(), tt.wantTenant)
		})
	}
}

func TestAgentWorkerStreams_ScopeToEchoedTenant(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		start   func(*AgentService, context.Context) (func(), error)
		payload map[string]any
	}{
		{"task output", messagequeue.SubjectTaskOutput, (*AgentService).StartOutputSubscriber,
			map[string]any{"task_id": "task-1", "tenant_id": scopeTenantA, "line": "x", "stream": "stdout"}},
		{"agent output", messagequeue.SubjectAgentOutput, (*AgentService).StartAgentOutputSubscriber,
			map[string]any{"task_id": "task-1", "tenant_id": scopeTenantA, "line": "x", "stream": "stdout"}},
		{"task result", messagequeue.SubjectTaskResult, (*AgentService).StartResultSubscriber,
			map[string]any{"task_id": "task-1", "project_id": "proj-1", "tenant_id": scopeTenantA, "status": "completed", "output": "done"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &tenantRecorder{}
			queue := newHandlerCapturingQueue()
			svc := NewAgentService(&mockStore{}, queue, rec)
			if _, err := tt.start(svc, context.Background()); err != nil {
				t.Fatalf("subscribe: %v", err)
			}
			handler, ok := queue.getHandler(tt.subject)
			if !ok {
				t.Fatalf("no handler for %s", tt.subject)
			}

			if err := handler(context.Background(), tt.subject, mustJSON(t, tt.payload)); err != nil {
				t.Fatalf("handler: %v", err)
			}
			assertScoped(t, rec.snapshot(), scopeTenantA)
		})
	}
}

func TestAgentDispatch_SendsTenantToBackend(t *testing.T) {
	queue := newHandlerCapturingQueue()
	store := &mockStore{
		agents: []agent.Agent{{ID: "agent-1", ProjectID: "proj-1", Name: "a", Backend: "tenant-probe", Status: agent.StatusIdle}},
		tasks:  []task.Task{{ID: "task-1", ProjectID: "proj-1", Title: "t", Prompt: "p", Status: task.StatusPending}},
	}
	backend := registerTenantProbeBackend(t)
	svc := NewAgentService(store, queue, &tenantRecorder{})

	ctx := tenantctx.WithTenant(context.Background(), scopeTenantB)
	if err := svc.Dispatch(ctx, "agent-1", "task-1"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got := backend.lastTenant(); got != scopeTenantB {
		t.Fatalf("backend received task for tenant %q, want %q", got, scopeTenantB)
	}
}

// --- Request/result round trips with the Python worker ---

func TestContextRequests_CarryTenant(t *testing.T) {
	store := &mockStore{projects: []project.Project{{ID: "proj-1", WorkspacePath: "/ws"}}}
	orchCfg := &config.Orchestrator{}
	limits := &config.Limits{}
	ctx := tenantctx.WithTenant(context.Background(), scopeTenantA)

	tests := []struct {
		name    string
		subject string
		request func(q *mockQueue, hub *tenantRecorder) error
	}{
		{"repomap", messagequeue.SubjectRepoMapRequest, func(q *mockQueue, hub *tenantRecorder) error {
			return NewRepoMapService(store, q, hub, orchCfg).RequestGeneration(ctx, "proj-1", nil)
		}},
		{"retrieval index", messagequeue.SubjectRetrievalIndexRequest, func(q *mockQueue, hub *tenantRecorder) error {
			return NewRetrievalService(store, q, hub, orchCfg, limits).RequestIndex(ctx, "proj-1", "/ws", "")
		}},
		{"graph build", messagequeue.SubjectGraphBuildRequest, func(q *mockQueue, hub *tenantRecorder) error {
			return NewGraphService(store, q, hub, orchCfg, limits).RequestBuild(ctx, "proj-1", "/ws")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &mockQueue{}
			hub := &tenantRecorder{}
			if err := tt.request(q, hub); err != nil {
				t.Fatalf("request: %v", err)
			}
			if len(q.published) != 1 || q.published[0].subject != tt.subject {
				t.Fatalf("published %+v, want one %s message", q.published, tt.subject)
			}
			var got struct {
				TenantID string `json:"tenant_id"`
			}
			if err := json.Unmarshal(q.published[0].data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.TenantID != scopeTenantA {
				t.Fatalf("request tenant_id = %q, want %q", got.TenantID, scopeTenantA)
			}
			assertScoped(t, hub.snapshot(), scopeTenantA)
		})
	}
}

func TestContextResults_ScopeToEchoedTenant(t *testing.T) {
	store := &mockStore{}
	orchCfg := &config.Orchestrator{}
	limits := &config.Limits{}
	ctx := context.Background() // NATS handlers run without a request tenant

	tests := []struct {
		name   string
		handle func(hub *tenantRecorder, tenant string) error
	}{
		{"repomap result", func(hub *tenantRecorder, tenant string) error {
			return NewRepoMapService(store, &mockQueue{}, hub, orchCfg).HandleResult(ctx, &messagequeue.RepoMapResultPayload{ProjectID: "proj-1", TenantID: tenant})
		}},
		{"repomap failure", func(hub *tenantRecorder, tenant string) error {
			return NewRepoMapService(store, &mockQueue{}, hub, orchCfg).HandleResult(ctx, &messagequeue.RepoMapResultPayload{ProjectID: "proj-1", TenantID: tenant, Error: "boom"})
		}},
		{"retrieval index result", func(hub *tenantRecorder, tenant string) error {
			return NewRetrievalService(store, &mockQueue{}, hub, orchCfg, limits).HandleIndexResult(ctx, &messagequeue.RetrievalIndexResultPayload{ProjectID: "proj-1", TenantID: tenant, Status: "ready"})
		}},
		{"graph build result", func(hub *tenantRecorder, tenant string) error {
			return NewGraphService(store, &mockQueue{}, hub, orchCfg, limits).HandleBuildResult(ctx, &messagequeue.GraphBuildResultPayload{ProjectID: "proj-1", TenantID: tenant, Status: "ready"})
		}},
	}
	for _, tt := range tests {
		for _, tenant := range []string{scopeTenantA, ""} {
			t.Run(tt.name+"/tenant="+tenant, func(t *testing.T) {
				hub := &tenantRecorder{}
				if err := tt.handle(hub, tenant); err != nil {
					t.Fatalf("handle: %v", err)
				}
				assertScoped(t, hub.snapshot(), tenant)
			})
		}
	}
}

func TestReviewApprovalRequired_ScopesToPayloadTenant(t *testing.T) {
	hub := &tenantRecorder{}
	svc := NewReviewApprovalService(&mockQueue{}, hub)
	data := mustJSON(t, messagequeue.ReviewApprovalRequiredPayload{RunID: "run-1", ProjectID: "proj-1", TenantID: scopeTenantB, ImpactLevel: "high"})

	if err := svc.HandleApprovalRequired(context.Background(), messagequeue.SubjectReviewApprovalRequired, data); err != nil {
		t.Fatalf("HandleApprovalRequired: %v", err)
	}
	assertScoped(t, hub.snapshot(), scopeTenantB)
}

func TestBenchmarkProgress_ScopesToPayloadTenant(t *testing.T) {
	tests := []struct {
		name   string
		handle func(*BenchmarkService, context.Context, string, []byte) error
		data   []byte
	}{
		{"task started", (*BenchmarkService).HandleBenchmarkTaskStarted,
			mustJSON(t, messagequeue.BenchmarkTaskStartedPayload{RunID: "run-1", TaskID: "t-1", TenantID: scopeTenantA})},
		{"task progress", (*BenchmarkService).HandleBenchmarkTaskProgress,
			mustJSON(t, messagequeue.BenchmarkTaskProgressPayload{RunID: "run-1", TaskID: "t-1", TenantID: scopeTenantA})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := &tenantRecorder{}
			svc := &BenchmarkService{hub: hub}
			if err := tt.handle(svc, context.Background(), "", tt.data); err != nil {
				t.Fatalf("handle: %v", err)
			}
			assertScoped(t, hub.snapshot(), scopeTenantA)
		})
	}
}

// --- Background jobs ---

func TestReleaseStaleWork_ScopesEachTaskToItsTenant(t *testing.T) {
	store := &activeWorkMockStore{releasedTasks: []task.Task{
		{ID: "t-a", ProjectID: "p-a", TenantID: scopeTenantA},
		{ID: "t-b", ProjectID: "p-b", TenantID: scopeTenantB},
		{ID: "t-none", ProjectID: "p-none"},
	}}
	hub := &tenantRecorder{}
	svc := NewActiveWorkService(store, hub)

	if _, err := svc.ReleaseStaleWork(context.Background(), time.Minute); err != nil {
		t.Fatalf("ReleaseStaleWork: %v", err)
	}

	got := hub.snapshot()
	want := []string{scopeTenantA, scopeTenantB, ""}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].tenant != want[i] {
			t.Errorf("event %d scoped to %q, want %q", i, got[i].tenant, want[i])
		}
	}
}

func TestAutoAgentLoop_ScopesToTheStartingTenant(t *testing.T) {
	store := newAutoAgentMockStore()
	hub := &tenantRecorder{}
	convSvc := NewConversationService(store, hub, "test-model", nil)
	svc := NewAutoAgentService(store, hub, &noopQueue{}, convSvc)
	store.createMessageErr = errors.New("mock: no LLM in tests") // every feature fails fast
	seedProject(store, "proj-1", "/workspace/proj1")
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{{ID: "feat-1", Title: "F1", Status: roadmap.FeatureBacklog}})

	if _, err := svc.Start(tenantctx.WithTenant(context.Background(), scopeTenantA), "proj-1"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForCondition(t, 2*time.Second, "loop finished", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		_, running := svc.cancels["proj-1"]
		return !running && len(hub.snapshot()) >= 3 // start, feature, final status
	})
	assertScoped(t, hub.snapshot(), scopeTenantA)
}

// fakeLSPClient captures the diagnostic callback the service registers.
type fakeLSPClient struct {
	lspPort.Client
	mu       sync.Mutex
	callback func(uri string, diags []lspDomain.Diagnostic)
}

func (c *fakeLSPClient) Start(context.Context) error { return nil }
func (c *fakeLSPClient) Stop(context.Context) error  { return nil }
func (c *fakeLSPClient) Status() lspDomain.ServerStatus {
	return lspDomain.ServerStatusReady
}

func (c *fakeLSPClient) SetDiagnosticCallback(fn func(uri string, diags []lspDomain.Diagnostic)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callback = fn
}

func (c *fakeLSPClient) emit(uri string) {
	c.mu.Lock()
	fn := c.callback
	c.mu.Unlock()
	fn(uri, []lspDomain.Diagnostic{{Message: "unused variable"}})
}

func TestLSPDiagnostics_ScopeToTheTenantThatStartedTheServer(t *testing.T) {
	client := &fakeLSPClient{}
	hub := &tenantRecorder{}
	svc := NewLSPService(&config.LSP{StartTimeout: time.Second, DiagnosticDelay: time.Millisecond}, hub, &mockStore{},
		func(string, lspDomain.LanguageServerConfig, string) lspPort.Client { return client })

	ctx := tenantctx.WithTenant(context.Background(), scopeTenantB)
	if err := svc.StartServers(ctx, "proj-1", "/ws", []string{"go"}); err != nil {
		t.Fatalf("StartServers: %v", err)
	}
	client.emit("file:///ws/main.go")

	deadline := time.Now().Add(2 * time.Second)
	for {
		var diag []scopedEvent
		for _, ev := range hub.snapshot() {
			if ev.eventType == "lsp.diagnostic" {
				diag = append(diag, ev)
			}
		}
		if len(diag) > 0 {
			assertScoped(t, hub.snapshot(), scopeTenantB)
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no diagnostic event broadcast")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// --- Helpers ---

// tenantProbeBackend records the tenant of the tasks it is asked to execute.
type tenantProbeBackend struct {
	mu     sync.Mutex
	tenant string
}

func (b *tenantProbeBackend) Name() string { return "tenant-probe" }
func (b *tenantProbeBackend) Capabilities() agentbackend.Capabilities {
	return agentbackend.Capabilities{}
}
func (b *tenantProbeBackend) Stop(context.Context, string) error { return nil }

func (b *tenantProbeBackend) Execute(_ context.Context, t *task.Task) (*task.Result, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tenant = t.TenantID
	return nil, nil
}

func (b *tenantProbeBackend) lastTenant() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tenant
}

var (
	probeBackend     = &tenantProbeBackend{}
	probeBackendOnce sync.Once
)

// registerTenantProbeBackend registers the probe once per test binary (the
// backend registry panics on duplicate names, e.g. with -count>1).
func registerTenantProbeBackend(t *testing.T) *tenantProbeBackend {
	t.Helper()
	probeBackendOnce.Do(func() {
		agentbackend.Register("tenant-probe", func(map[string]string) (agentbackend.Backend, error) {
			return probeBackend, nil
		})
	})
	return probeBackend
}

func TestDetachTenant(t *testing.T) {
	parent, cancel := context.WithCancel(tenantctx.WithTenant(context.Background(), scopeTenantA))
	detached := detachTenant(parent)
	cancel()

	if detached.Err() != nil {
		t.Fatal("detached context was cancelled with its parent")
	}
	if got, ok := tenantctx.Lookup(detached); !ok || got != scopeTenantA {
		t.Fatalf("detached tenant = (%q, %v), want %q", got, ok, scopeTenantA)
	}
	if _, ok := tenantctx.Lookup(detachTenant(context.Background())); ok {
		t.Fatal("detachTenant invented a tenant for a context without one")
	}
}

func TestWithPayloadTenant(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		payload string
		want    string
	}{
		{"no tenant anywhere", context.Background(), "", ""},
		{"payload tenant", context.Background(), scopeTenantA, scopeTenantA},
		{"request tenant wins over payload", tenantctx.WithTenant(context.Background(), scopeTenantB), scopeTenantA, scopeTenantB},
		{"empty payload keeps request tenant", tenantctx.WithTenant(context.Background(), scopeTenantB), "", scopeTenantB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := tenantctx.Lookup(withPayloadTenant(tt.ctx, tt.payload))
			if got != tt.want {
				t.Fatalf("tenant = %q, want %q", got, tt.want)
			}
		})
	}
}
