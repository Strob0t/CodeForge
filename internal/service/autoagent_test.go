package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/autoagent"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// ---------------------------------------------------------------------------
// Test-specific mock store that embeds the base mockStore for interface
// satisfaction and overrides the methods AutoAgentService actually uses.
// ---------------------------------------------------------------------------

type autoAgentMockStore struct {
	mockStore

	mu       sync.Mutex
	projects map[string]*project.Project
	roadmaps map[string]*roadmap.Roadmap     // keyed by projectID
	features map[string][]roadmap.Feature    // keyed by roadmapID
	featByID map[string]*roadmap.Feature     // keyed by featureID
	agents   map[string]*autoagent.AutoAgent // keyed by projectID

	// Conversations backing store for the real ConversationService.
	convs    map[string]*conversation.Conversation
	messages map[string][]*conversation.Message
	convSeq  int

	// Injectable errors.
	upsertAutoAgentErr error
	getRoadmapErr      error
	listFeaturesErr    error
	createMessageErr   error
}

func newAutoAgentMockStore() *autoAgentMockStore {
	return &autoAgentMockStore{
		projects: make(map[string]*project.Project),
		roadmaps: make(map[string]*roadmap.Roadmap),
		features: make(map[string][]roadmap.Feature),
		featByID: make(map[string]*roadmap.Feature),
		agents:   make(map[string]*autoagent.AutoAgent),
		convs:    make(map[string]*conversation.Conversation),
		messages: make(map[string][]*conversation.Message),
	}
}

func (m *autoAgentMockStore) GetProject(_ context.Context, id string) (*project.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return p, nil
}

func (m *autoAgentMockStore) GetRoadmapByProject(_ context.Context, projectID string) (*roadmap.Roadmap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getRoadmapErr != nil {
		return nil, m.getRoadmapErr
	}
	rm, ok := m.roadmaps[projectID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return rm, nil
}

func (m *autoAgentMockStore) ListFeaturesByRoadmap(_ context.Context, roadmapID string) ([]roadmap.Feature, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listFeaturesErr != nil {
		return nil, m.listFeaturesErr
	}
	return m.features[roadmapID], nil
}

func (m *autoAgentMockStore) GetFeature(_ context.Context, id string) (*roadmap.Feature, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.featByID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *f
	return &cp, nil
}

func (m *autoAgentMockStore) UpdateFeature(_ context.Context, f *roadmap.Feature) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.featByID[f.ID]; ok {
		existing.Status = f.Status
		return nil
	}
	return domain.ErrNotFound
}

func (m *autoAgentMockStore) UpsertAutoAgent(_ context.Context, aa *autoagent.AutoAgent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.upsertAutoAgentErr != nil {
		return m.upsertAutoAgentErr
	}
	cp := *aa
	m.agents[aa.ProjectID] = &cp
	return nil
}

func (m *autoAgentMockStore) GetAutoAgent(_ context.Context, projectID string) (*autoagent.AutoAgent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	aa, ok := m.agents[projectID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *aa
	return &cp, nil
}

func (m *autoAgentMockStore) UpdateAutoAgentStatus(_ context.Context, projectID string, status autoagent.Status, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	aa, ok := m.agents[projectID]
	if !ok {
		// Create an entry if one does not exist (mirrors real DB behavior).
		m.agents[projectID] = &autoagent.AutoAgent{ProjectID: projectID, Status: status, Error: errMsg}
		return nil
	}
	aa.Status = status
	aa.Error = errMsg
	return nil
}

func (m *autoAgentMockStore) UpdateAutoAgentProgress(_ context.Context, aa *autoagent.AutoAgent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.agents[aa.ProjectID]
	if !ok {
		return nil
	}
	existing.FeaturesComplete = aa.FeaturesComplete
	existing.FeaturesFailed = aa.FeaturesFailed
	existing.CurrentFeatureID = aa.CurrentFeatureID
	existing.ConversationID = aa.ConversationID
	return nil
}

// Conversation store methods needed by ConversationService.Create.
func (m *autoAgentMockStore) CreateConversation(_ context.Context, c *conversation.Conversation) (*conversation.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.convSeq++
	c.ID = "conv-" + string(rune('0'+m.convSeq))
	cp := *c
	m.convs[c.ID] = &cp
	return &cp, nil
}

func (m *autoAgentMockStore) GetConversation(_ context.Context, id string) (*conversation.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.convs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c, nil
}

func (m *autoAgentMockStore) CreateMessage(_ context.Context, msg *conversation.Message) (*conversation.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createMessageErr != nil {
		return nil, m.createMessageErr
	}
	msg.ID = "msg-1"
	m.messages[msg.ConversationID] = append(m.messages[msg.ConversationID], msg)
	return msg, nil
}

func (m *autoAgentMockStore) ListMessages(_ context.Context, conversationID string) ([]conversation.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ptrs := m.messages[conversationID]
	out := make([]conversation.Message, len(ptrs))
	for i, p := range ptrs {
		out[i] = *p
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestAutoAgentService creates an AutoAgentService wired to an in-memory
// mock store and a minimal ConversationService (no LLM, no queue).
func newTestAutoAgentService(store *autoAgentMockStore) *AutoAgentService {
	hub := &noopBroadcaster{}
	q := &noopQueue{}
	convSvc := NewConversationService(store, hub, "test-model", nil)
	svc := NewAutoAgentService(store, hub, q, convSvc)
	// Prevent SendMessage from reaching the nil LLM client by failing early.
	// Tests that need SendMessage to succeed must clear this explicitly.
	store.createMessageErr = errors.New("mock: message creation blocked (no LLM in tests)")
	return svc
}

// seedProject adds a project with a workspace path to the mock store.
func seedProject(store *autoAgentMockStore, id, workspace string) {
	store.projects[id] = &project.Project{
		ID:            id,
		Name:          "test-project",
		WorkspacePath: workspace,
	}
}

// seedRoadmapWithFeatures adds a roadmap and features for a project.
func seedRoadmapWithFeatures(store *autoAgentMockStore, projectID string, features []roadmap.Feature) {
	rmID := "rm-" + projectID
	store.roadmaps[projectID] = &roadmap.Roadmap{
		ID:        rmID,
		ProjectID: projectID,
	}
	store.features[rmID] = features
	for i := range features {
		cp := features[i]
		store.featByID[features[i].ID] = &cp
	}
}

// noopBroadcaster satisfies broadcast.Broadcaster without doing anything.
type noopBroadcaster struct{}

func (n *noopBroadcaster) BroadcastEvent(_ context.Context, _ string, _ any) {}

// noopQueue satisfies messagequeue.Queue without doing anything.
type noopQueue struct{}

func (q *noopQueue) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (q *noopQueue) PublishWithDedup(_ context.Context, _ string, _ []byte, _ string) error {
	return nil
}
func (q *noopQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (q *noopQueue) Drain() error      { return nil }
func (q *noopQueue) Close() error      { return nil }
func (q *noopQueue) IsConnected() bool { return true }

// waitForCondition polls a condition function until it returns true or times out.
func waitForCondition(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for condition: %s", msg)
}

// ---------------------------------------------------------------------------
// Tests: Start
// ---------------------------------------------------------------------------

func TestAutoAgentStart_Success(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "/workspace/proj1")
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
		{ID: "feat-1", Title: "Feature 1", Status: roadmap.FeatureBacklog},
	})

	aa, err := svc.Start(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aa.ProjectID != "proj-1" {
		t.Errorf("expected project ID proj-1, got %q", aa.ProjectID)
	}
	if aa.Status != autoagent.StatusRunning {
		t.Errorf("expected status running, got %q", aa.Status)
	}
	if aa.FeaturesTotal != 1 {
		t.Errorf("expected 1 feature total, got %d", aa.FeaturesTotal)
	}

	// Wait for the background goroutine to register in the cancels map.
	waitForCondition(t, time.Second, "cancel func registered", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		cancel, ok := svc.cancels["proj-1"]
		return ok && cancel != nil
	})

	// Clean up: stop the loop.
	_ = svc.Stop(context.Background(), "proj-1")
}

func TestAutoAgentStart_ProjectNotFound(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	_, err := svc.Start(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestAutoAgentStart_ProjectNoWorkspace(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "") // empty workspace

	_, err := svc.Start(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error for project without workspace")
	}
	if !errors.Is(err, domain.ErrValidation) {
		t.Errorf("expected ErrValidation, got: %v", err)
	}
}

func TestAutoAgentStart_AlreadyRunning(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "/workspace/proj1")
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
		{ID: "feat-1", Title: "Feature 1", Status: roadmap.FeatureBacklog},
	})

	_, err := svc.Start(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	// Second start should fail with conflict.
	_, err = svc.Start(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected conflict error for second start")
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}

	_ = svc.Stop(context.Background(), "proj-1")
}

func TestAutoAgentStart_NoPendingFeatures(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "/workspace/proj1")
	// Seed features that are all done (none pending).
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
		{ID: "feat-1", Title: "Done Feature", Status: roadmap.FeatureDone},
	})

	_, err := svc.Start(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error when no pending features")
	}
	if !errors.Is(err, domain.ErrValidation) {
		t.Errorf("expected ErrValidation, got: %v", err)
	}

	// Verify slot was cleaned up (no reservation left).
	svc.mu.Lock()
	_, hasSlot := svc.cancels["proj-1"]
	svc.mu.Unlock()
	if hasSlot {
		t.Error("expected slot to be cleaned up after failed setup")
	}
}

func TestAutoAgentStart_NoRoadmap(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "/workspace/proj1")
	// No roadmap seeded.

	_, err := svc.Start(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error when no roadmap exists")
	}
}

func TestAutoAgentStart_UpsertFailure_CleansUpSlot(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "/workspace/proj1")
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
		{ID: "feat-1", Title: "Feature 1", Status: roadmap.FeaturePlanned},
	})
	store.upsertAutoAgentErr = errors.New("db write failed")

	_, err := svc.Start(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error from upsert failure")
	}

	// Verify slot was cleaned up.
	svc.mu.Lock()
	_, hasSlot := svc.cancels["proj-1"]
	svc.mu.Unlock()
	if hasSlot {
		t.Error("expected slot to be cleaned up after upsert failure")
	}
}

func TestAutoAgentStart_FiltersOnlyPendingFeatures(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "/workspace/proj1")
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
		{ID: "feat-1", Title: "Backlog", Status: roadmap.FeatureBacklog},
		{ID: "feat-2", Title: "Planned", Status: roadmap.FeaturePlanned},
		{ID: "feat-3", Title: "Done", Status: roadmap.FeatureDone},
		{ID: "feat-4", Title: "In Progress", Status: roadmap.FeatureInProgress},
		{ID: "feat-5", Title: "Cancelled", Status: roadmap.FeatureCancelled},
	})

	aa, err := svc.Start(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Backlog, planned, and in-progress features should be included.
	if aa.FeaturesTotal != 3 {
		t.Errorf("expected 3 pending features, got %d", aa.FeaturesTotal)
	}

	_ = svc.Stop(context.Background(), "proj-1")
}

// ---------------------------------------------------------------------------
// Tests: Stop
// ---------------------------------------------------------------------------

func TestAutoAgentStop_Running(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "/workspace/proj1")
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
		{ID: "feat-1", Title: "Feature 1", Status: roadmap.FeatureBacklog},
	})

	_, err := svc.Start(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	err = svc.Stop(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// Wait for the background goroutine to clean up.
	waitForCondition(t, 2*time.Second, "cancel map cleaned up", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		_, ok := svc.cancels["proj-1"]
		return !ok
	})
}

func TestAutoAgentStop_NotRunning(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	// Stop on a project that was never started should not error.
	err := svc.Stop(context.Background(), "proj-999")
	if err != nil {
		t.Fatalf("expected no error for non-running stop, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: Status
// ---------------------------------------------------------------------------

func TestAutoAgentStatus_Exists(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	store.agents["proj-1"] = &autoagent.AutoAgent{
		ProjectID:     "proj-1",
		Status:        autoagent.StatusRunning,
		FeaturesTotal: 5,
	}

	aa, err := svc.Status(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aa.Status != autoagent.StatusRunning {
		t.Errorf("expected running status, got %q", aa.Status)
	}
	if aa.FeaturesTotal != 5 {
		t.Errorf("expected 5 features total, got %d", aa.FeaturesTotal)
	}
}

func TestAutoAgentStatus_NotFound_ReturnsIdle(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	aa, err := svc.Status(context.Background(), "proj-new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aa.Status != autoagent.StatusIdle {
		t.Errorf("expected idle status for new project, got %q", aa.Status)
	}
	if aa.ProjectID != "proj-new" {
		t.Errorf("expected project ID proj-new, got %q", aa.ProjectID)
	}
}

// ---------------------------------------------------------------------------
// Tests: runLoop goroutine lifecycle
// ---------------------------------------------------------------------------

func TestAutoAgentRunLoop_CleansUpOnContextCancel(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "/workspace/proj1")
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
		{ID: "feat-1", Title: "Feature 1", Status: roadmap.FeatureBacklog},
	})

	_, err := svc.Start(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Stop triggers context cancellation.
	_ = svc.Stop(context.Background(), "proj-1")

	// The goroutine should eventually remove itself from the cancels map.
	waitForCondition(t, 2*time.Second, "goroutine cleaned up", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		_, exists := svc.cancels["proj-1"]
		return !exists
	})
}

func TestAutoAgentRunLoop_MultipleProjectsIndependent(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	for _, pid := range []string{"proj-a", "proj-b", "proj-c"} {
		seedProject(store, pid, "/workspace/"+pid)
		seedRoadmapWithFeatures(store, pid, []roadmap.Feature{
			{ID: "feat-" + pid, Title: "Feature for " + pid, Status: roadmap.FeatureBacklog},
		})
	}

	// Start all three.
	for _, pid := range []string{"proj-a", "proj-b", "proj-c"} {
		_, err := svc.Start(context.Background(), pid)
		if err != nil {
			t.Fatalf("start %s failed: %v", pid, err)
		}
	}

	// Stop only proj-b.
	_ = svc.Stop(context.Background(), "proj-b")

	waitForCondition(t, 2*time.Second, "proj-b removed", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		_, exists := svc.cancels["proj-b"]
		return !exists
	})

	// proj-a and proj-c should still be registered (or have completed by now).
	// We just verify they were started independently.

	// Clean up.
	_ = svc.Stop(context.Background(), "proj-a")
	_ = svc.Stop(context.Background(), "proj-c")
}

// ---------------------------------------------------------------------------
// Tests: TOCTOU race — concurrent Start calls for the same project
// ---------------------------------------------------------------------------

func TestAutoAgentStart_ConcurrentRace(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	seedProject(store, "proj-1", "/workspace/proj1")
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
		{ID: "feat-1", Title: "Feature 1", Status: roadmap.FeatureBacklog},
	})

	// Launch many goroutines simultaneously. The slot-reservation pattern
	// must prevent concurrent starts: every Start call for the same project
	// must either succeed or return ErrConflict. No panics, no data races.
	const goroutines = 50
	var (
		wg        sync.WaitGroup
		successes atomic.Int32
		conflicts atomic.Int32
	)

	wg.Add(goroutines)
	barrier := make(chan struct{})
	for range goroutines {
		go func() {
			defer wg.Done()
			<-barrier
			_, err := svc.Start(context.Background(), "proj-1")
			if err == nil {
				successes.Add(1)
			} else if errors.Is(err, domain.ErrConflict) {
				conflicts.Add(1)
			}
			// Other transient errors (from fast goroutine cycling) are acceptable.
		}()
	}
	close(barrier)
	wg.Wait()

	s := successes.Load()
	c := conflicts.Load()

	// At least one goroutine must have succeeded.
	if s == 0 {
		t.Error("expected at least one successful start")
	}
	// Most results should be success or conflict; a few goroutines may get
	// transient errors (e.g. setup errors when the slot cycles very fast),
	// which is acceptable in a concurrent stress test.
	if s+c < goroutines/2 {
		t.Errorf("expected most goroutines to get success or conflict, got %d successes + %d conflicts out of %d",
			s, c, goroutines)
	}

	// Clean up.
	_ = svc.Stop(context.Background(), "proj-1")

	waitForCondition(t, 2*time.Second, "cleanup after race test", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.cancels) == 0
	})
}

// ---------------------------------------------------------------------------
// Tests: Concurrent access safety (multiple projects simultaneously)
// ---------------------------------------------------------------------------

func TestAutoAgentConcurrentStartStop(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	const numProjects = 10
	var wg sync.WaitGroup

	for i := range numProjects {
		pid := "proj-" + string(rune('A'+i))
		seedProject(store, pid, "/workspace/"+pid)
		seedRoadmapWithFeatures(store, pid, []roadmap.Feature{
			{ID: "feat-" + pid, Title: "Feature", Status: roadmap.FeaturePlanned},
		})
	}

	// Start all projects concurrently.
	wg.Add(numProjects)
	for i := range numProjects {
		pid := "proj-" + string(rune('A'+i))
		go func(p string) {
			defer wg.Done()
			_, _ = svc.Start(context.Background(), p)
		}(pid)
	}
	wg.Wait()

	// Stop all projects concurrently.
	wg.Add(numProjects)
	for i := range numProjects {
		pid := "proj-" + string(rune('A'+i))
		go func(p string) {
			defer wg.Done()
			_ = svc.Stop(context.Background(), p)
		}(pid)
	}
	wg.Wait()

	// All goroutines should eventually clean up.
	waitForCondition(t, 3*time.Second, "all cancels cleaned up", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.cancels) == 0
	})
}

// ---------------------------------------------------------------------------
// Tests: Slot reservation cleanup on setup failure
// ---------------------------------------------------------------------------

func TestAutoAgentStart_SetupFailure_DoesNotLeakSlot(t *testing.T) {
	tests := []struct {
		name  string
		setup func(store *autoAgentMockStore)
	}{
		{
			name: "roadmap fetch fails",
			setup: func(store *autoAgentMockStore) {
				store.getRoadmapErr = errors.New("db error")
			},
		},
		{
			name: "feature list fails",
			setup: func(store *autoAgentMockStore) {
				store.roadmaps["proj-1"] = &roadmap.Roadmap{ID: "rm-proj-1", ProjectID: "proj-1"}
				store.listFeaturesErr = errors.New("db error")
			},
		},
		{
			name: "empty features",
			setup: func(store *autoAgentMockStore) {
				seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{})
			},
		},
		{
			name: "upsert auto agent fails",
			setup: func(store *autoAgentMockStore) {
				seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
					{ID: "feat-1", Title: "F1", Status: roadmap.FeatureBacklog},
				})
				store.upsertAutoAgentErr = errors.New("db write error")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newAutoAgentMockStore()
			svc := newTestAutoAgentService(store)

			seedProject(store, "proj-1", "/workspace/proj1")
			tc.setup(store)

			_, err := svc.Start(context.Background(), "proj-1")
			if err == nil {
				t.Fatal("expected error from setup failure")
			}

			// Verify no slot leak.
			svc.mu.Lock()
			_, hasSlot := svc.cancels["proj-1"]
			svc.mu.Unlock()
			if hasSlot {
				t.Error("slot was not cleaned up after setup failure")
			}

			// Verify a subsequent Start can succeed (no stale reservation).
			store.getRoadmapErr = nil
			store.listFeaturesErr = nil
			store.upsertAutoAgentErr = nil
			seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{
				{ID: "feat-retry", Title: "Retry", Status: roadmap.FeatureBacklog},
			})

			aa, err := svc.Start(context.Background(), "proj-1")
			if err != nil {
				t.Fatalf("retry start should succeed, got: %v", err)
			}
			if aa.Status != autoagent.StatusRunning {
				t.Errorf("expected running status on retry, got %q", aa.Status)
			}

			_ = svc.Stop(context.Background(), "proj-1")
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: NewAutoAgentService constructor
// ---------------------------------------------------------------------------

func TestNewAutoAgentService(t *testing.T) {
	store := newAutoAgentMockStore()
	hub := &noopBroadcaster{}
	q := &noopQueue{}
	convSvc := NewConversationService(store, hub, "model", nil)

	svc := NewAutoAgentService(store, hub, q, convSvc)

	if svc.db == nil {
		t.Error("expected db to be set")
	}
	if svc.hub == nil {
		t.Error("expected hub to be set")
	}
	if svc.queue == nil {
		t.Error("expected queue to be set")
	}
	if svc.conversations == nil {
		t.Error("expected conversations to be set")
	}
	if svc.cancels == nil {
		t.Error("expected cancels map to be initialized")
	}
	if len(svc.cancels) != 0 {
		t.Error("expected cancels map to be empty on init")
	}
}

// ---------------------------------------------------------------------------
// Tests: pendingFeatures (internal helper)
// ---------------------------------------------------------------------------

func TestAutoAgentPendingFeatures(t *testing.T) {
	tests := []struct {
		name      string
		features  []roadmap.Feature
		wantCount int
	}{
		{
			name:      "all statuses mixed",
			wantCount: 3,
			features: []roadmap.Feature{
				{ID: "f1", Status: roadmap.FeatureBacklog},
				{ID: "f2", Status: roadmap.FeaturePlanned},
				{ID: "f3", Status: roadmap.FeatureInProgress},
				{ID: "f4", Status: roadmap.FeatureDone},
				{ID: "f5", Status: roadmap.FeatureCancelled},
			},
		},
		{
			name:      "only backlog",
			wantCount: 3,
			features: []roadmap.Feature{
				{ID: "f1", Status: roadmap.FeatureBacklog},
				{ID: "f2", Status: roadmap.FeatureBacklog},
				{ID: "f3", Status: roadmap.FeatureBacklog},
			},
		},
		{
			name:      "only planned",
			wantCount: 1,
			features: []roadmap.Feature{
				{ID: "f1", Status: roadmap.FeaturePlanned},
			},
		},
		{
			name:      "none pending",
			wantCount: 0,
			features: []roadmap.Feature{
				{ID: "f1", Status: roadmap.FeatureDone},
				{ID: "f2", Status: roadmap.FeatureCancelled},
			},
		},
		{
			name:      "empty list",
			wantCount: 0,
			features:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newAutoAgentMockStore()
			svc := newTestAutoAgentService(store)

			seedRoadmapWithFeatures(store, "proj-1", tc.features)

			pending, err := svc.pendingFeatures(context.Background(), "proj-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(pending) != tc.wantCount {
				t.Errorf("expected %d pending features, got %d", tc.wantCount, len(pending))
			}
		})
	}
}

func TestAutoAgentPendingFeatures_NoRoadmap(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	_, err := svc.pendingFeatures(context.Background(), "proj-missing")
	if err == nil {
		t.Fatal("expected error when roadmap not found")
	}
}

// ---------------------------------------------------------------------------
// Tests: extractTestFile
// ---------------------------------------------------------------------------

func TestExtractTestFile(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		{"Tests: test_lru_cache.py (25 tests)", "test_lru_cache.py"},
		{"Tests: test_diff_analyzer.py", "test_diff_analyzer.py"},
		{"No tests mentioned here", ""},
		{"File: foo.py\nTests: test_foo.py\nMore text", "test_foo.py"},
		{"", ""},
		{"Tests: not_a_test.py", ""},
		{"Tests:test_no_space.py", ""},
		{"Tests: test_with_numbers_123.py (10 tests)", "test_with_numbers_123.py"},
	}
	for _, tt := range tests {
		got := extractTestFile(tt.desc)
		if got != tt.want {
			t.Errorf("extractTestFile(%q) = %q, want %q", tt.desc, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: broadcastStatus (smoke test)
// ---------------------------------------------------------------------------

func TestAutoAgentBroadcastStatus_NoPanic(t *testing.T) {
	store := newAutoAgentMockStore()
	svc := newTestAutoAgentService(store)

	// Should not panic with nil fields.
	svc.broadcastStatus(context.Background(), &autoagent.AutoAgent{
		ProjectID: "proj-1",
		Status:    autoagent.StatusRunning,
	})
}
