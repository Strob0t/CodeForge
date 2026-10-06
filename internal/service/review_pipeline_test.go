package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/boundary"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-17: the contract-first review pipeline (Phase 31) starts for real, its
// boundary analysis is stored, and its refactoring step goes through the
// threshold HITL.

const reviewTenant = "aaaaaaaa-0000-0000-0000-00000000000a"

// --- fakes ---

type fakeReviewStore struct {
	projects   map[string]*project.Project
	agents     []agent.Agent
	tasks      []task.CreateRequest
	runs       map[string]*run.Run
	plans      map[string]*plan.ExecutionPlan
	boundaries *boundary.ProjectBoundaryConfig
	boundsErr  error // GetProjectBoundaries fails with it
	upserted   *boundary.ProjectBoundaryConfig
	pipelines  map[string]*review.Pipeline // by plan ID

	failTaskAt   int   // CreateTask fails for the n-th task (1-based); 0 never
	pipelineErr  error // GetReviewPipeline fails with it
	activeReview bool  // HasActiveReviewPipeline answers it
	// onGetProject runs at the start of GetProject, once (it may call into
	// the service).
	onGetProject   func()
	recordErr      error // CreateReviewPipeline fails with it
	cancelledTasks []string

	// KI-94: the user edits recorded per plan, the limit they were last
	// listed with, a listing error, and the plans whose edits were deleted.
	userEdits    map[string][]review.UserEdit
	editsLimit   int
	editsErr     error
	editsDeleted []string
}

func (f *fakeReviewStore) UpdateTaskStatus(_ context.Context, id string, status task.Status) error {
	if status == task.StatusCancelled {
		f.cancelledTasks = append(f.cancelledTasks, id)
	}
	return nil
}

func (f *fakeReviewStore) CreateReviewPipeline(_ context.Context, rp *review.Pipeline) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	if f.pipelines == nil {
		f.pipelines = map[string]*review.Pipeline{}
	}
	if rp.State == "" {
		rp.State = review.PipelinePending
	}
	stored := *rp
	f.pipelines[rp.PlanID] = &stored
	return nil
}

func (f *fakeReviewStore) GetPlanStepByRunID(_ context.Context, runID string) (*plan.Step, error) {
	for _, p := range f.plans {
		for i := range p.Steps {
			if p.Steps[i].RunID == runID {
				st := p.Steps[i]
				st.PlanID = p.ID
				return &st, nil
			}
		}
	}
	return nil, domain.ErrNotFound
}

// fakeRunEnds answers which runs' workers may still write.
type fakeRunEnds struct{ mayWrite map[string]bool }

func (f *fakeRunEnds) WorkerMayStillWrite(runID string) bool { return f.mayWrite[runID] }
func (f *fakeRunEnds) WorkerStopGrace() time.Duration        { return time.Minute }

// fakeEndedRefactorings lists ended refactorings for the watchdog check.
type fakeEndedRefactorings struct {
	rows   []database.EndedReviewRefactoring
	before time.Time
}

func (f *fakeEndedRefactorings) ListEndedReviewRefactorings(_ context.Context, endedBefore time.Time, _ int) ([]database.EndedReviewRefactoring, error) {
	f.before = endedBefore
	return f.rows, nil
}

func (f *fakeReviewStore) HasActiveReviewPipeline(context.Context, string) (bool, error) {
	return f.activeReview, nil
}

func (f *fakeReviewStore) ListPlansByProject(_ context.Context, projectID string) ([]plan.ExecutionPlan, error) {
	var out []plan.ExecutionPlan
	for _, p := range f.plans {
		if p.ProjectID == projectID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (f *fakeReviewStore) ListPendingReviewDecisions(_ context.Context, projectID string) ([]review.Pipeline, error) {
	var out []review.Pipeline
	for _, rp := range f.pipelines {
		if rp.ProjectID == projectID && rp.State == review.PipelineAwaitingDecision {
			out = append(out, *rp)
		}
	}
	return out, nil
}

func (f *fakeReviewStore) UpdateReviewPipeline(_ context.Context, rp *review.Pipeline, from review.PipelineState) error {
	stored, ok := f.pipelines[rp.PlanID]
	if !ok {
		return domain.ErrNotFound
	}
	if stored.State != from {
		return domain.ErrConflict
	}
	updated := *rp
	f.pipelines[rp.PlanID] = &updated
	return nil
}

func (f *fakeReviewStore) GetReviewPipeline(_ context.Context, planID string) (*review.Pipeline, error) {
	if f.pipelineErr != nil {
		return nil, f.pipelineErr
	}
	rp, ok := f.pipelines[planID]
	if !ok {
		return nil, fmt.Errorf("review pipeline of plan %s: %w", planID, domain.ErrNotFound)
	}
	stored := *rp
	return &stored, nil
}

func (f *fakeReviewStore) GetProject(_ context.Context, id string) (*project.Project, error) {
	if hook := f.onGetProject; hook != nil {
		f.onGetProject = nil
		hook()
	}
	if p, ok := f.projects[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("project %s: %w", id, domain.ErrNotFound)
}

func (f *fakeReviewStore) ListAgents(_ context.Context, _ string) ([]agent.Agent, error) {
	return f.agents, nil
}

func (f *fakeReviewStore) CreateTask(_ context.Context, req task.CreateRequest) (*task.Task, error) {
	if f.failTaskAt == len(f.tasks)+1 {
		return nil, errors.New("insert task: connection reset")
	}
	f.tasks = append(f.tasks, req)
	return &task.Task{ID: fmt.Sprintf("task-%d", len(f.tasks)), ProjectID: req.ProjectID, Prompt: req.Prompt}, nil
}

func (f *fakeReviewStore) GetRun(_ context.Context, id string) (*run.Run, error) {
	if r, ok := f.runs[id]; ok {
		return r, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeReviewStore) GetPlan(_ context.Context, id string) (*plan.ExecutionPlan, error) {
	if p, ok := f.plans[id]; ok {
		return p, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeReviewStore) GetProjectBoundaries(_ context.Context, _ string) (*boundary.ProjectBoundaryConfig, error) {
	if f.boundsErr != nil {
		return nil, f.boundsErr
	}
	if f.boundaries == nil {
		return nil, domain.ErrNotFound
	}
	return f.boundaries, nil
}

func (f *fakeReviewStore) UpsertProjectBoundaries(_ context.Context, cfg *boundary.ProjectBoundaryConfig) error {
	f.upserted = cfg
	return nil
}

type fakeReviewPlanner struct {
	created     *plan.CreatePlanRequest
	createFails bool
	startFails  bool  // the first step does not start: the plan fails
	startErr    error // StartPlan fails with it
	cancelled   []string
	approved    []string
	rejected    []string
}

func (f *fakeReviewPlanner) CreatePlan(_ context.Context, req *plan.CreatePlanRequest) (*plan.ExecutionPlan, error) {
	if f.createFails {
		return nil, errors.New("insert plan: connection reset")
	}
	f.created = req
	p := &plan.ExecutionPlan{ID: "plan-1", ProjectID: req.ProjectID, TeamID: req.TeamID, Status: plan.StatusPending}
	for i, st := range req.Steps {
		p.Steps = append(p.Steps, plan.Step{ID: fmt.Sprintf("step-%d", i), TaskID: st.TaskID, AgentID: st.AgentID, ModeID: st.ModeID})
	}
	return p, nil
}

func (f *fakeReviewPlanner) StartPlan(_ context.Context, id string) (*plan.ExecutionPlan, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.startFails {
		return &plan.ExecutionPlan{ID: id, Status: plan.StatusFailed, Steps: []plan.Step{{ID: "step-0", Error: "get agent: not found"}}}, nil
	}
	return &plan.ExecutionPlan{ID: id, Status: plan.StatusRunning}, nil
}

func (f *fakeReviewPlanner) CancelPlan(_ context.Context, id string) error {
	f.cancelled = append(f.cancelled, id)
	return nil
}

func (f *fakeReviewPlanner) ApproveStep(_ context.Context, _, stepID string) error {
	f.approved = append(f.approved, stepID)
	return nil
}

func (f *fakeReviewPlanner) RejectStep(_ context.Context, _, stepID string) error {
	f.rejected = append(f.rejected, stepID)
	return nil
}

type fakeReviewTeams struct {
	created     []*agent.CreateTeamRequest
	createFails bool
	ended       []string
}

func (f *fakeReviewTeams) CleanupTeam(_ context.Context, teamID string, _ bool) error {
	f.ended = append(f.ended, teamID)
	return nil
}

// reviewEventRecorder records the review events with the tenant they went to.
type reviewEventRecorder struct {
	mu     sync.Mutex
	events []recordedReviewEvent
}

type recordedReviewEvent struct {
	EventType string
	Tenant    string
	Data      event.ReviewImpactEvent
	IsImpact  bool // the payload is a ReviewImpactEvent
}

func (r *reviewEventRecorder) BroadcastEvent(ctx context.Context, eventType string, payload any) {
	ev, ok := payload.(event.ReviewImpactEvent)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recordedReviewEvent{EventType: eventType, Tenant: tenantctx.FromContext(ctx), Data: ev, IsImpact: ok})
}

func (r *reviewEventRecorder) snapshot() []recordedReviewEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedReviewEvent(nil), r.events...)
}

func (f *fakeReviewTeams) CreateTeam(_ context.Context, req *agent.CreateTeamRequest) (*agent.Team, error) {
	if f.createFails {
		return nil, errors.New("insert team: connection reset")
	}
	f.created = append(f.created, req)
	return &agent.Team{ID: "team-1", ProjectID: req.ProjectID}, nil
}

// --- git workspace helpers ---

func reviewGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

// newReviewWorkspace is a git repository with committed files a.go
// (100 lines) and api.proto.
func newReviewWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	reviewGit(t, dir, "init", "-q")
	writeLines(t, dir, "a.go", 100, "line")
	writeLines(t, dir, "api.proto", 5, "message")
	reviewGit(t, dir, "add", "-A")
	reviewGit(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func writeLines(t *testing.T, dir, name string, n int, word string) {
	t.Helper()
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "%s %d\n", word, i)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test file in a temp dir
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func hasRef(t *testing.T, dir, ref string) bool {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", ref)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// --- setup ---

type reviewFixture struct {
	svc     *ReviewPipelineService
	store   *fakeReviewStore
	planner *fakeReviewPlanner
	teams   *fakeReviewTeams
	hub     *reviewEventRecorder
	dir     string
	ctx     context.Context
}

func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	dir := newReviewWorkspace(t)
	f := &reviewFixture{
		store: &fakeReviewStore{
			projects: map[string]*project.Project{"proj-1": {ID: "proj-1", WorkspacePath: dir}},
			agents: []agent.Agent{
				{ID: "busy", ProjectID: "proj-1", Status: agent.StatusRunning},
				{ID: "idle", ProjectID: "proj-1", Status: agent.StatusIdle},
			},
			runs:  map[string]*run.Run{},
			plans: map[string]*plan.ExecutionPlan{},
		},
		planner: &fakeReviewPlanner{},
		teams:   &fakeReviewTeams{},
		hub:     &reviewEventRecorder{},
		dir:     dir,
		ctx:     tenantctx.WithTenant(context.Background(), reviewTenant),
	}
	f.svc = NewReviewPipelineService(f.store, NewPipelineService(NewModeService()), f.planner, f.teams,
		git.NewPool(1), f.hub, DefaultDiffImpactConfig())
	return f
}

// --- start ---

func TestReviewPipeline_StartReviewRefactor(t *testing.T) {
	f := newReviewFixture(t)

	p, err := f.svc.StartReviewPipeline(f.ctx, "proj-1")
	if err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	if p.Status != plan.StatusRunning {
		t.Fatalf("plan status = %s, want running", p.Status)
	}

	req := f.planner.created
	wantModes := []string{"boundary_analyzer", "contract_reviewer", "reviewer", "refactorer"}
	if req == nil || len(req.Steps) != len(wantModes) {
		t.Fatalf("plan request = %+v, want %d steps", req, len(wantModes))
	}
	for i, st := range req.Steps {
		if st.ModeID != wantModes[i] || st.AgentID != "idle" || st.TaskID != fmt.Sprintf("task-%d", i+1) {
			t.Errorf("step %d = mode %q agent %q task %q, want %s on the idle agent with its own task", i, st.ModeID, st.AgentID, st.TaskID, wantModes[i])
		}
		if f.store.tasks[i].Prompt == "" {
			t.Errorf("step %d task has no prompt", i)
		}
	}
	if req.TeamID != "team-1" || len(f.teams.created) != 1 || f.teams.created[0].Members[0].AgentID != "idle" {
		t.Fatalf("team = %q (%+v), want a team of the idle agent", req.TeamID, f.teams.created)
	}
	// The baseline is taken when the refactorer step starts (S6-F 2).
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("a baseline was recorded when the pipeline started")
	}
	rp := f.store.pipelines["plan-1"]
	if rp == nil || rp.State != review.PipelinePending || rp.BaselineSHA != "" || rp.ProjectID != "proj-1" {
		t.Fatalf("review record = %+v, want a pending one", rp)
	}
}

// S6-F 2: the baseline is the workspace when the refactorer step starts,
// recorded once; a refactorer step of another plan needs nothing.
func TestReviewPipeline_PrepareRecordsTheBaseline(t *testing.T) {
	f := newReviewFixture(t)
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	step := &plan.Step{ID: "step-3", PlanID: "plan-1", ModeID: "refactorer"}
	if !f.svc.NeedsPreparation(step) || f.svc.NeedsPreparation(&plan.Step{ModeID: "reviewer"}) {
		t.Fatal("only refactorer steps need preparation")
	}
	if err := f.svc.PrepareStep(f.ctx, step); err != nil {
		t.Fatalf("PrepareStep: %v", err)
	}
	rp := f.store.pipelines["plan-1"]
	if rp.State != review.PipelineRefactoring || rp.StepID != "step-3" ||
		rp.BaselineSHA != reviewGit(t, f.dir, "rev-parse", reviewBaselineRef("plan-1")) {
		t.Fatalf("review record = %+v, want the baseline of step-3", rp)
	}

	// A re-planned refactoring step keeps the first baseline.
	first := rp.BaselineSHA
	writeLines(t, f.dir, "a.go", 120, "partial")
	if err := f.svc.PrepareStep(f.ctx, step); err != nil || f.store.pipelines["plan-1"].BaselineSHA != first {
		t.Fatalf("second PrepareStep = %v, baseline %s, want the first one kept", err, f.store.pipelines["plan-1"].BaselineSHA)
	}

	if err := f.svc.PrepareStep(f.ctx, &plan.Step{ID: "s", PlanID: "plan-9", ModeID: "refactorer"}); err != nil {
		t.Fatalf("PrepareStep of another plan: %v", err)
	}
}

func TestReviewPipeline_StartBoundaryAnalysis(t *testing.T) {
	f := newReviewFixture(t)

	if _, err := f.svc.StartBoundaryAnalysis(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartBoundaryAnalysis: %v", err)
	}
	req := f.planner.created
	if len(req.Steps) != 1 || req.Steps[0].ModeID != "boundary_analyzer" || req.TeamID != "" {
		t.Fatalf("plan request = %+v, want one boundary_analyzer step without a team", req)
	}
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("a pipeline that does not refactor needs no baseline")
	}
	if rp := f.store.pipelines["plan-1"]; rp == nil || rp.BaselineSHA != "" {
		t.Fatalf("review record = %+v, want one without a baseline", rp)
	}
}

func TestReviewPipeline_StartErrors(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(f *reviewFixture)
		project string
		wantErr error
	}{
		{"unknown project", func(*reviewFixture) {}, "other", domain.ErrNotFound},
		{"no workspace", func(f *reviewFixture) { f.store.projects["proj-1"].WorkspacePath = "" }, "proj-1", domain.ErrValidation},
		{"no agents", func(f *reviewFixture) { f.store.agents = nil }, "proj-1", ErrReviewNoAgents},
		{"no idle agent", func(f *reviewFixture) { f.store.agents = f.store.agents[:1] }, "proj-1", domain.ErrValidation},
		{"first step does not start", func(f *reviewFixture) { f.planner.startFails = true }, "proj-1", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReviewFixture(t)
			tt.modify(f)
			p, err := f.svc.StartReviewPipeline(f.ctx, tt.project)
			if err == nil || p != nil {
				t.Fatalf("StartReviewPipeline = %+v, %v, want an error and no plan", p, err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// --- gate ---

// gateFixture is a started review plan whose refactoring step's run just
// completed; change edits the workspace like the refactorer would.
func gateFixture(t *testing.T, boundaries []boundary.BoundaryFile, change func(dir string)) (*reviewFixture, *plan.Step) {
	t.Helper()
	f := newReviewFixture(t)
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	if boundaries != nil {
		f.store.boundaries = &boundary.ProjectBoundaryConfig{ProjectID: "proj-1", Boundaries: boundaries}
	}
	step := &plan.Step{ID: "step-3", PlanID: "plan-1", ModeID: "refactorer", RunID: "run-4", Status: plan.StepStatusRunning}
	f.store.plans["plan-1"] = &plan.ExecutionPlan{ID: "plan-1", ProjectID: "proj-1", Status: plan.StatusRunning, Steps: []plan.Step{*step}}
	if err := f.svc.PrepareStep(f.ctx, step); err != nil {
		t.Fatalf("PrepareStep: %v", err)
	}
	change(f.dir)
	return f, step
}

func TestReviewPipeline_GateScoresTheRefactoring(t *testing.T) {
	tests := []struct {
		name       string
		boundaries []boundary.BoundaryFile
		change     func(dir string)
		wantStatus plan.StepStatus
		wantEvent  string // "" = none
		check      func(t *testing.T, ev event.ReviewImpactEvent)
	}{
		{
			name:       "low: small change is applied",
			change:     func(dir string) { writeLines(t, dir, "a.go", 102, "line") },
			wantStatus: plan.StepStatusCompleted,
		},
		{
			name:       "medium: applied with a notification",
			change:     func(dir string) { writeLines(t, dir, "a.go", 160, "line") },
			wantStatus: plan.StepStatusCompleted,
			wantEvent:  event.EventReviewRefactorApplied,
			check: func(t *testing.T, ev event.ReviewImpactEvent) {
				if ev.ImpactLevel != "medium" || ev.LinesAdded != 60 || ev.LinesRemoved != 0 {
					t.Errorf("event = %+v, want medium with 60 lines added", ev)
				}
			},
		},
		{
			name:       "high: large change waits for approval",
			change:     func(dir string) { writeLines(t, dir, "a.go", 300, "rewritten") },
			wantStatus: plan.StepStatusWaitingApproval,
			wantEvent:  event.EventReviewApprovalRequired,
			check: func(t *testing.T, ev event.ReviewImpactEvent) {
				if ev.ImpactLevel != "high" || ev.FilesChanged != 1 || ev.RunID != "run-4" || ev.PlanID != "plan-1" ||
					ev.StepID != "step-3" || ev.ProjectID != "proj-1" || ev.Reason != "" {
					t.Errorf("event = %+v, want high for run-4 / plan-1 / step-3 / proj-1", ev)
				}
			},
		},
		{
			name:       "high: a new file is structural",
			change:     func(dir string) { writeLines(t, dir, "new.go", 3, "x") },
			wantStatus: plan.StepStatusWaitingApproval,
			wantEvent:  event.EventReviewApprovalRequired,
			check: func(t *testing.T, ev event.ReviewImpactEvent) {
				if !ev.Structural {
					t.Errorf("event = %+v, want structural", ev)
				}
			},
		},
		{
			name:       "high: a boundary file crosses layers",
			boundaries: []boundary.BoundaryFile{{Path: "api.proto", Type: boundary.BoundaryTypeAPI}},
			change:     func(dir string) { writeLines(t, dir, "api.proto", 6, "message") },
			wantStatus: plan.StepStatusWaitingApproval,
			wantEvent:  event.EventReviewApprovalRequired,
			check: func(t *testing.T, ev event.ReviewImpactEvent) {
				if !ev.CrossLayer || ev.Structural {
					t.Errorf("event = %+v, want cross-layer, not structural", ev)
				}
			},
		},
		{
			// S6-F 9: boundary paths are compared normalised.
			name:       "high: a boundary written as ./path crosses layers",
			boundaries: []boundary.BoundaryFile{{Path: "./api.proto", Type: boundary.BoundaryTypeAPI}},
			change:     func(dir string) { writeLines(t, dir, "api.proto", 6, "message") },
			wantStatus: plan.StepStatusWaitingApproval,
			wantEvent:  event.EventReviewApprovalRequired,
			check: func(t *testing.T, ev event.ReviewImpactEvent) {
				if !ev.CrossLayer {
					t.Errorf("event = %+v, want cross-layer", ev)
				}
			},
		},
		{
			name:       "high: a counterpart with backslashes and dot segments crosses layers",
			boundaries: []boundary.BoundaryFile{{Path: "api.proto", Type: boundary.BoundaryTypeAPI, Counterpart: `.\gen\..\a.go`}},
			change:     func(dir string) { writeLines(t, dir, "a.go", 102, "line") },
			wantStatus: plan.StepStatusWaitingApproval,
			wantEvent:  event.EventReviewApprovalRequired,
			check: func(t *testing.T, ev event.ReviewImpactEvent) {
				if !ev.CrossLayer {
					t.Errorf("event = %+v, want cross-layer", ev)
				}
			},
		},
		{
			name: "unmeasurable change needs approval",
			// The repository lost its objects: the change cannot be diffed.
			change:     func(dir string) { _ = os.RemoveAll(filepath.Join(dir, ".git", "objects")) },
			wantStatus: plan.StepStatusWaitingApproval,
			wantEvent:  event.EventReviewApprovalRequired,
			check: func(t *testing.T, ev event.ReviewImpactEvent) {
				if ev.Reason == "" {
					t.Errorf("event = %+v, want the reason", ev)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, step := gateFixture(t, tt.boundaries, tt.change)

			if got := gateNow(f, step); got != tt.wantStatus {
				t.Fatalf("GateStep = %s, want %s", got, tt.wantStatus)
			}
			events := f.hub.snapshot()
			if tt.wantEvent == "" {
				if len(events) != 0 {
					t.Fatalf("events = %+v, want none", events)
				}
				return
			}
			if len(events) != 1 || events[0].EventType != tt.wantEvent || events[0].Tenant != reviewTenant {
				t.Fatalf("events = %+v, want one %s in the tenant", events, tt.wantEvent)
			}
			if !events[0].IsImpact {
				t.Fatal("event payload is not a ReviewImpactEvent")
			}
			if tt.check != nil {
				tt.check(t, events[0].Data)
			}
		})
	}
}

// S6-F 9: whether a change crosses a boundary cannot be told when the
// project's boundaries cannot be loaded: the refactoring waits for approval
// however small it is.
func TestReviewPipeline_BoundaryLookupErrorNeedsApproval(t *testing.T) {
	f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 102, "line") })
	f.store.boundsErr = errors.New("connection reset")

	if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s, want waiting_approval", got)
	}
	events := f.hub.snapshot()
	if len(events) != 1 || events[0].EventType != event.EventReviewApprovalRequired ||
		!strings.Contains(events[0].Data.Reason, "boundaries") {
		t.Fatalf("events = %+v, want one approval request naming the boundaries", events)
	}
}

// The workspace's baseline ref is agent-writable (an agent with Bash can run
// git update-ref): the gate trusts only the baseline the Go Core recorded,
// and anything that does not match it means the change cannot be measured,
// so the refactoring waits for approval however small it looks (V1).
func TestReviewPipeline_GateDoesNotTrustTheRef(t *testing.T) {
	ref := reviewBaselineRef("plan-1")
	tests := []struct {
		name   string
		tamper func(t *testing.T, f *reviewFixture)
	}{
		{"baseline ref deleted", func(t *testing.T, f *reviewFixture) {
			reviewGit(t, f.dir, "update-ref", "-d", ref)
		}},
		{"baseline ref repointed", func(t *testing.T, f *reviewFixture) {
			reviewGit(t, f.dir, "update-ref", ref, "HEAD")
		}},
		{"workspace is no repository any more", func(t *testing.T, f *reviewFixture) {
			if err := os.RemoveAll(filepath.Join(f.dir, ".git")); err != nil {
				t.Fatal(err)
			}
		}},
		{"recorded baseline commit is gone", func(t *testing.T, f *reviewFixture) {
			const gone = "1111111111111111111111111111111111111111"
			f.store.pipelines["plan-1"].BaselineSHA = gone
			path := filepath.Join(f.dir, ".git", filepath.FromSlash(ref))
			if err := os.WriteFile(path, []byte(gone+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"no baseline recorded", func(_ *testing.T, f *reviewFixture) {
			f.store.pipelines["plan-1"].BaselineSHA = ""
		}},
		{"the refactoring started without a baseline", func(_ *testing.T, f *reviewFixture) {
			f.store.pipelines["plan-1"].State = review.PipelinePending
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 101, "line") }) // low impact
			tt.tamper(t, f)

			if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
				t.Fatalf("GateStep = %s, want waiting for approval", got)
			}
			events := f.hub.snapshot()
			if len(events) != 1 || events[0].EventType != event.EventReviewApprovalRequired || events[0].Data.Reason == "" {
				t.Fatalf("events = %+v, want one approval request with the reason", events)
			}
		})
	}
}

// Changes the user made before the refactorer step started - before the
// pipeline or while its reports were written - are not part of the
// refactoring (S6-F 2).
func TestReviewPipeline_GateMeasuresOnlyTheRefactorersChange(t *testing.T) {
	f := newReviewFixture(t)
	writeLines(t, f.dir, "a.go", 400, "user")
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	writeLines(t, f.dir, "notes.md", 300, "while the reports were written")
	step := &plan.Step{ID: "step-3", PlanID: "plan-1", ModeID: "refactorer", RunID: "run-4"}
	f.store.plans["plan-1"] = &plan.ExecutionPlan{ID: "plan-1", ProjectID: "proj-1", Status: plan.StatusRunning, Steps: []plan.Step{*step}}
	if err := f.svc.PrepareStep(f.ctx, step); err != nil {
		t.Fatalf("PrepareStep: %v", err)
	}
	writeLines(t, f.dir, "api.proto", 6, "message")

	if got := gateNow(f, step); got != plan.StepStatusCompleted {
		t.Fatalf("GateStep = %s, want completed (a one-line refactoring)", got)
	}
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) || hasRef(t, f.dir, reviewResultRef("plan-1")) {
		t.Fatal("the refs of a decided refactoring are kept")
	}
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineDone {
		t.Fatalf("review state = %s, want done", rp.State)
	}
}

// A refactorer step of a plan the review pipeline did not start has no
// baseline and is not gated.
func TestReviewPipeline_GateIgnoresOtherPlans(t *testing.T) {
	f := newReviewFixture(t)
	f.store.plans["plan-9"] = &plan.ExecutionPlan{ID: "plan-9", ProjectID: "proj-1"}
	writeLines(t, f.dir, "a.go", 500, "x")
	step := &plan.Step{ID: "s", PlanID: "plan-9", ModeID: "refactorer", RunID: "r"}
	if got := gateNow(f, step); got != plan.StepStatusCompleted {
		t.Fatalf("GateStep = %s, want completed", got)
	}
	if other := (&plan.Step{ID: "s2", PlanID: "plan-9", ModeID: "coder"}); gateNow(f, other) != plan.StepStatusCompleted {
		t.Fatal("a step in another mode must complete")
	}
}

// --- decide ---

func waitingStep(t *testing.T) (*reviewFixture, *plan.Step) {
	t.Helper()
	f, step := gateFixture(t, nil, func(dir string) {
		writeLines(t, dir, "a.go", 300, "rewritten")
		writeLines(t, dir, "new.go", 3, "x")
	})
	if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s", got)
	}
	step.Status = plan.StepStatusWaitingApproval
	f.store.plans["plan-1"].Steps = []plan.Step{*step}
	return f, step
}

func TestReviewPipeline_RejectUndoesTheRefactoring(t *testing.T) {
	f, step := waitingStep(t)
	baseline := reviewGit(t, f.dir, "show", "HEAD:a.go")
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineAwaitingDecision || rp.ResultSHA == "" || rp.Impact == nil {
		t.Fatalf("review record = %+v, want the measured refactoring awaiting a decision", rp)
	}

	d, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false)
	if err != nil {
		t.Fatalf("Decide(reject): %v", err)
	}
	if d.Status != "rejected" || d.HeadRestored || d.Message != "" || !slices.Equal(d.RestoredPaths, []string{"a.go", "new.go"}) {
		t.Fatalf("decision = %+v, want a.go and new.go restored, HEAD untouched", d)
	}
	if got := readFile(t, f.dir, "a.go"); got != baseline+"\n" {
		t.Fatalf("a.go after reject is not the baseline:\n%.60s", got)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "new.go")); !os.IsNotExist(err) {
		t.Fatalf("new.go after reject: %v, want it removed", err)
	}
	if len(f.planner.rejected) != 1 || len(f.planner.approved) != 0 {
		t.Fatalf("rejected %v approved %v, want the step rejected", f.planner.rejected, f.planner.approved)
	}
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) || hasRef(t, f.dir, reviewResultRef("plan-1")) {
		t.Fatal("refs kept after the decision")
	}
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineDone {
		t.Fatalf("review state = %s, want done", rp.State)
	}
}

// S6-F 2: the undo is path-scoped. Edits the user made before the refactorer
// started and edits made since the measurement in other paths are kept.
func TestReviewPipeline_RejectKeepsTheUsersEdits(t *testing.T) {
	f := newReviewFixture(t)
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	writeLines(t, f.dir, "before.md", 3, "edited while the reports were written")
	step := &plan.Step{ID: "step-3", PlanID: "plan-1", ModeID: "refactorer", RunID: "run-4", Status: plan.StepStatusRunning}
	f.store.plans["plan-1"] = &plan.ExecutionPlan{ID: "plan-1", ProjectID: "proj-1", Status: plan.StatusRunning, Steps: []plan.Step{*step}}
	if err := f.svc.PrepareStep(f.ctx, step); err != nil {
		t.Fatalf("PrepareStep: %v", err)
	}
	writeLines(t, f.dir, "a.go", 300, "rewritten")
	if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s", got)
	}
	f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusWaitingApproval
	writeLines(t, f.dir, "after.md", 2, "edited while the decision waited")

	if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err != nil {
		t.Fatalf("Decide(reject): %v", err)
	}
	if strings.Contains(readFile(t, f.dir, "a.go"), "rewritten") {
		t.Fatal("the refactoring was not undone")
	}
	if !strings.Contains(readFile(t, f.dir, "before.md"), "while the reports") || !strings.Contains(readFile(t, f.dir, "after.md"), "decision waited") {
		t.Fatal("the user's edits were undone with the refactoring")
	}
}

// An edit in the same lines as the refactoring since the measurement cannot
// be separated from it: the undo fails, names the file and changes nothing.
func TestReviewPipeline_RejectOverlappingEditFails(t *testing.T) {
	f, step := waitingStep(t)
	writeLines(t, f.dir, "a.go", 300, "edited again")
	before := readFile(t, f.dir, "a.go")

	_, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false)
	if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "a.go") {
		t.Fatalf("Decide(reject) = %v, want a validation error naming a.go", err)
	}
	if readFile(t, f.dir, "a.go") != before || len(f.planner.rejected) != 0 {
		t.Fatal("a failed undo changed the workspace or rejected the step")
	}
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineAwaitingDecision {
		t.Fatalf("review state = %s, want the decision still pending", rp.State)
	}
}

// Reject restores the recorded baseline, not the commit the ref points to: an
// agent that repoints the ref at its own change cannot make the undo keep it.
func TestReviewPipeline_RejectUsesTheRecordedBaseline(t *testing.T) {
	f, step := waitingStep(t)
	original := reviewGit(t, f.dir, "show", "HEAD:a.go")
	reviewGit(t, f.dir, "add", "-A")
	tree := reviewGit(t, f.dir, "write-tree")
	forged := reviewGit(t, f.dir, "commit-tree", tree, "-m", "forged baseline")
	reviewGit(t, f.dir, "update-ref", reviewBaselineRef("plan-1"), forged)
	reviewGit(t, f.dir, "reset", "-q")

	if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err != nil {
		t.Fatalf("Decide(reject): %v", err)
	}
	if got := readFile(t, f.dir, "a.go"); got != original+"\n" {
		t.Fatalf("a.go after reject is not the recorded baseline:\n%.60s", got)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "new.go")); !os.IsNotExist(err) {
		t.Fatalf("new.go after reject: %v, want it removed", err)
	}
}

// A refactoring the agent committed is undone too: HEAD still points at the
// refactoring's commit, so it moves back with a compare-and-swap, and the
// workspace is clean again.
func TestReviewPipeline_RejectUndoesACommittedRefactoring(t *testing.T) {
	var started string
	f, step := gateFixture(t, nil, func(dir string) {
		started = reviewGit(t, dir, "rev-parse", "HEAD")
		writeLines(t, dir, "a.go", 300, "rewritten")
		reviewGit(t, dir, "commit", "-qam", "refactor")
	})
	if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s, want waiting for approval", got)
	}
	f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusWaitingApproval

	d, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false)
	if err != nil {
		t.Fatalf("Decide(reject): %v", err)
	}
	if !d.HeadRestored || d.Message != "" {
		t.Fatalf("decision = %+v, want HEAD moved back", d)
	}
	if head := reviewGit(t, f.dir, "rev-parse", "HEAD"); head != started {
		t.Fatalf("HEAD = %s after reject, want %s", head, started)
	}
	if status := reviewGit(t, f.dir, "status", "--porcelain"); status != "" {
		t.Fatalf("workspace after reject is not the baseline:\n%s", status)
	}
}

// HEAD that moved on since the measurement (the user committed) is left
// where it is: only the files are restored, and the answer says so.
func TestReviewPipeline_RejectLeavesAMovedHead(t *testing.T) {
	f, step := gateFixture(t, nil, func(dir string) {
		writeLines(t, dir, "a.go", 300, "rewritten")
		reviewGit(t, dir, "commit", "-qam", "refactor")
	})
	if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s, want waiting for approval", got)
	}
	f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusWaitingApproval
	writeLines(t, f.dir, "user.go", 2, "user")
	reviewGit(t, f.dir, "add", "user.go")
	reviewGit(t, f.dir, "commit", "-qm", "user's commit")
	userHead := reviewGit(t, f.dir, "rev-parse", "HEAD")

	d, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false)
	if err != nil {
		t.Fatalf("Decide(reject): %v", err)
	}
	if d.HeadRestored || !strings.Contains(d.Message, "HEAD was left") || !slices.Equal(d.RestoredPaths, []string{"a.go"}) {
		t.Fatalf("decision = %+v, want HEAD left with a message and a.go restored", d)
	}
	if head := reviewGit(t, f.dir, "rev-parse", "HEAD"); head != userHead {
		t.Fatalf("HEAD = %s, want the user's commit %s", head, userHead)
	}
	if strings.Contains(readFile(t, f.dir, "a.go"), "rewritten") || readFile(t, f.dir, "user.go") == "" {
		t.Fatal("want a.go restored and the user's file kept")
	}
}

func TestReviewPipeline_ApproveKeepsTheRefactoring(t *testing.T) {
	f, step := waitingStep(t)

	d, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, true)
	if err != nil || d.Status != "approved" {
		t.Fatalf("Decide(approve) = %+v, %v", d, err)
	}
	if !strings.Contains(readFile(t, f.dir, "a.go"), "rewritten") {
		t.Fatal("approved refactoring was undone")
	}
	if len(f.planner.approved) != 1 || len(f.planner.rejected) != 0 {
		t.Fatalf("approved %v rejected %v, want the step approved", f.planner.approved, f.planner.rejected)
	}
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineDone || hasRef(t, f.dir, reviewResultRef("plan-1")) {
		t.Fatalf("review state = %s, want done and the refs dropped", rp.State)
	}
}

func TestReviewPipeline_DecideErrors(t *testing.T) {
	t.Run("run of another step", func(t *testing.T) {
		f, step := waitingStep(t)
		if _, err := f.svc.Decide(f.ctx, "run-other", "plan-1", step.ID, true); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Decide = %v, want not found", err)
		}
	})
	t.Run("nothing to decide", func(t *testing.T) {
		f, step := waitingStep(t)
		f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusCompleted
		f.store.pipelines["plan-1"].State = review.PipelineDone
		if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("Decide = %v, want a validation error", err)
		}
		if len(f.planner.rejected) != 0 {
			t.Fatal("a step that is not waiting was rejected")
		}
	})
	t.Run("undo fails: the step keeps waiting", func(t *testing.T) {
		f, step := waitingStep(t)
		if err := os.RemoveAll(filepath.Join(f.dir, ".git")); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err == nil {
			t.Fatal("Decide succeeded without undoing the refactoring")
		}
		if len(f.planner.rejected) != 0 {
			t.Fatal("step rejected although the refactoring was not undone")
		}
	})
}

func TestReviewPipeline_RejectWithoutBaselineIsRefused(t *testing.T) {
	t.Run("no baseline recorded", func(t *testing.T) {
		f, step := waitingStep(t)
		f.store.pipelines["plan-1"].BaselineSHA = ""
		if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("Decide(reject) = %v, want a validation error", err)
		}
		if len(f.planner.rejected) != 0 {
			t.Fatal("step rejected although the refactoring could not be undone")
		}
	})
	t.Run("recorded baseline commit is gone", func(t *testing.T) {
		f, step := waitingStep(t)
		f.store.pipelines["plan-1"].BaselineSHA = "1111111111111111111111111111111111111111"
		if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err == nil {
			t.Fatal("Decide(reject) succeeded without the baseline commit")
		}
		if len(f.planner.rejected) != 0 {
			t.Fatal("step rejected although the refactoring could not be undone")
		}
	})
}

// A review plan that ends without a refactoring waiting for a decision is
// done and leaves no refs behind.
func TestReviewPipeline_PlanEndDropsTheRefs(t *testing.T) {
	f, _ := gateFixture(t, nil, func(string) {})

	f.svc.PlanEnded(f.ctx, "plan-1", "completed")
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("baseline kept after the plan ended")
	}
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineDone {
		t.Fatalf("review state = %s, want done", rp.State)
	}
	f.svc.PlanEnded(f.ctx, "unknown", "completed") // not a review plan: nothing to do
}

// --- boundaries ---

func TestReviewPipeline_StoresTheBoundaryAnalysis(t *testing.T) {
	f := newReviewFixture(t)
	f.store.boundaries = &boundary.ProjectBoundaryConfig{ProjectID: "proj-1", Boundaries: []boundary.BoundaryFile{
		{Path: "manual.proto", Type: boundary.BoundaryTypeAPI},
		{Path: "old-auto.sql", Type: boundary.BoundaryTypeData, AutoDetected: true},
	}}
	f.store.runs["run-1"] = &run.Run{ID: "run-1", ProjectID: "proj-1", Output: "Found these:\n```json\n" +
		`[{"path": "api/user.proto", "type": "api", "counterpart": "web/user.ts"},` +
		`{"path": "", "type": "api"}, {"path": "db/001.sql", "type": "bogus"}, {"path": "events.json", "type": "inter-service"}]` +
		"\n```"}
	step := &plan.Step{ID: "s", PlanID: "p", ModeID: "boundary_analyzer", RunID: "run-1"}

	if got := gateNow(f, step); got != plan.StepStatusCompleted {
		t.Fatalf("GateStep = %s, want completed", got)
	}
	cfg := f.store.upserted
	if cfg == nil {
		t.Fatal("boundaries not stored")
	}
	var paths []string
	for _, b := range cfg.Boundaries {
		paths = append(paths, fmt.Sprintf("%s:%v", b.Path, b.AutoDetected))
	}
	want := "manual.proto:false api/user.proto:true events.json:true"
	if strings.Join(paths, " ") != want {
		t.Fatalf("stored boundaries = %v, want %s (manual kept, old auto replaced, invalid dropped)", paths, want)
	}
}

func TestReviewPipeline_BoundaryAnalysisWithoutResultKeepsBoundaries(t *testing.T) {
	f := newReviewFixture(t)
	f.store.runs["run-1"] = &run.Run{ID: "run-1", ProjectID: "proj-1", Output: "I could not find any boundaries."}
	gateNow(f, &plan.Step{ModeID: "boundary_analyzer", RunID: "run-1"})
	if f.store.upserted != nil {
		t.Fatalf("stored %+v from an output without BOUNDARIES.json", f.store.upserted)
	}
}

// S6-F 14: the review-refactor pipeline measures and undoes its refactoring
// with git: a workspace that is no git repository is a validation error (400)
// before anything is created, not a 500 after the plan exists. The boundary
// analysis only reads and runs there.
func TestReviewPipeline_StartNeedsAGitWorkspace(t *testing.T) {
	f := newReviewFixture(t)
	plain := t.TempDir()
	f.store.projects["proj-1"].WorkspacePath = plain

	p, err := f.svc.StartReviewPipeline(f.ctx, "proj-1")
	if !errors.Is(err, domain.ErrValidation) || p != nil || !strings.Contains(err.Error(), "needs a git workspace") {
		t.Fatalf("StartReviewPipeline = %+v, %v, want a validation error naming the git workspace", p, err)
	}
	if len(f.store.tasks) != 0 || f.planner.created != nil {
		t.Fatalf("created %d tasks and plan %+v, want nothing", len(f.store.tasks), f.planner.created)
	}

	if _, err := f.svc.StartBoundaryAnalysis(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartBoundaryAnalysis in a plain directory: %v", err)
	}
}

// S6-F 13: a start that fails after it created something leaves no orphans:
// the tasks it created are cancelled, its team is ended and a plan that did
// not end is cancelled.
func TestReviewPipeline_FailedStartLeavesNoOrphans(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(f *reviewFixture)
		wantTasks     []string // cancelled
		wantTeamEnded bool
		wantCancelled bool // the plan
	}{
		{"third task fails", func(f *reviewFixture) { f.store.failTaskAt = 3 }, []string{"task-1", "task-2"}, false, false},
		{"team fails", func(f *reviewFixture) { f.teams.createFails = true }, []string{"task-1", "task-2", "task-3", "task-4"}, false, false},
		{"plan fails", func(f *reviewFixture) { f.planner.createFails = true }, []string{"task-1", "task-2", "task-3", "task-4"}, true, false},
		{"record fails", func(f *reviewFixture) { f.store.recordErr = errors.New("insert: connection reset") },
			[]string{"task-1", "task-2", "task-3", "task-4"}, true, true},
		{"start fails", func(f *reviewFixture) { f.planner.startErr = errors.New("update plan: connection reset") },
			[]string{"task-1", "task-2", "task-3", "task-4"}, true, true},
		{"first step does not start", func(f *reviewFixture) { f.planner.startFails = true },
			[]string{"task-1", "task-2", "task-3", "task-4"}, true, false}, // the plan already failed
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReviewFixture(t)
			tt.modify(f)
			if p, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err == nil || p != nil {
				t.Fatalf("StartReviewPipeline = %+v, %v, want an error", p, err)
			}
			if !slices.Equal(f.store.cancelledTasks, tt.wantTasks) {
				t.Errorf("cancelled tasks = %v, want %v", f.store.cancelledTasks, tt.wantTasks)
			}
			if ended := slices.Equal(f.teams.ended, []string{"team-1"}); ended != tt.wantTeamEnded {
				t.Errorf("ended teams = %v, want team-1 ended: %v", f.teams.ended, tt.wantTeamEnded)
			}
			if cancelled := slices.Equal(f.planner.cancelled, []string{"plan-1"}); cancelled != tt.wantCancelled {
				t.Errorf("cancelled plans = %v, want plan-1 cancelled: %v", f.planner.cancelled, tt.wantCancelled)
			}
		})
	}
}

// --- S6-F 4: a refactoring that ended failed or cancelled ---

// endedRefactoring is a review plan whose refactoring step's run changed the
// workspace with change and ended with stepStatus; the plan ended with it.
func endedRefactoring(t *testing.T, stepStatus plan.StepStatus, planStatus string, change func(dir string)) (*reviewFixture, *plan.Step) {
	t.Helper()
	f, step := gateFixture(t, nil, change)
	f.store.plans["plan-1"].Steps[0].Status = stepStatus
	f.store.plans["plan-1"].Status = plan.Status(planStatus)
	f.svc.PlanEnded(f.ctx, "plan-1", planStatus)
	return f, step
}

func TestReviewPipeline_FailedRefactoringAsksKeepOrUndo(t *testing.T) {
	for _, tt := range []struct {
		name       string
		stepStatus plan.StepStatus
		planStatus string
	}{
		{"failed", plan.StepStatusFailed, "failed"},
		{"cancelled", plan.StepStatusCancelled, "cancelled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, step := endedRefactoring(t, tt.stepStatus, tt.planStatus, func(dir string) { writeLines(t, dir, "a.go", 104, "half done") })

			rp := f.store.pipelines["plan-1"]
			if rp.State != review.PipelineAwaitingDecision || rp.RunID != "run-4" || rp.Impact == nil || rp.Impact.Reason == "" {
				t.Fatalf("review record = %+v, want a keep/undo decision for run-4 with a reason", rp)
			}
			if !hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
				t.Fatal("baseline dropped before the decision")
			}
			events := f.hub.snapshot()
			if len(events) != 1 || events[0].EventType != event.EventReviewApprovalRequired || events[0].Data.RunID != "run-4" ||
				events[0].Data.ProjectID != "proj-1" || events[0].Data.FilesChanged != 1 {
				t.Fatalf("events = %+v, want one approval request for the change", events)
			}

			// Undo: the change is reverted; the plan stays as it ended.
			d, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false)
			if err != nil || d.Status != "rejected" {
				t.Fatalf("Decide(undo) = %+v, %v", d, err)
			}
			if strings.Contains(readFile(t, f.dir, "a.go"), "half done") {
				t.Fatal("the failed refactoring was not undone")
			}
			if len(f.planner.rejected)+len(f.planner.approved) != 0 {
				t.Fatalf("plan steps decided (%v / %v), want the ended plan left alone", f.planner.approved, f.planner.rejected)
			}
			if f.store.pipelines["plan-1"].State != review.PipelineDone || hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
				t.Fatal("decision not finished: want done and the baseline dropped")
			}
		})
	}
}

func TestReviewPipeline_FailedRefactoringKept(t *testing.T) {
	f, step := endedRefactoring(t, plan.StepStatusFailed, "failed", func(dir string) { writeLines(t, dir, "a.go", 104, "half done") })

	if d, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, true); err != nil || d.Status != "approved" {
		t.Fatalf("Decide(keep) = %+v, %v", d, err)
	}
	if !strings.Contains(readFile(t, f.dir, "a.go"), "half done") || len(f.planner.approved) != 0 {
		t.Fatal("want the change kept and the ended plan left alone")
	}
	if f.store.pipelines["plan-1"].State != review.PipelineDone {
		t.Fatal("decision not finished")
	}
}

// Nothing to decide: a failed refactoring that changed nothing, or a step
// whose run never started, ends the pipeline at once.
func TestReviewPipeline_FailedRefactoringWithoutChange(t *testing.T) {
	t.Run("no change", func(t *testing.T) {
		f, _ := endedRefactoring(t, plan.StepStatusFailed, "failed", func(string) {})
		if f.store.pipelines["plan-1"].State != review.PipelineDone || hasRef(t, f.dir, reviewBaselineRef("plan-1")) || len(f.hub.snapshot()) != 0 {
			t.Fatal("want the pipeline done, the baseline dropped and no event")
		}
	})
	t.Run("no run", func(t *testing.T) {
		f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 300, "user") })
		step.RunID = ""
		f.store.plans["plan-1"].Steps[0] = *step
		f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusFailed
		f.svc.PlanEnded(f.ctx, "plan-1", "failed")
		if f.store.pipelines["plan-1"].State != review.PipelineDone || len(f.hub.snapshot()) != 0 {
			t.Fatal("a refactoring that never ran asked for a decision")
		}
	})
}

// A step that waited for approval when its plan was cancelled keeps its
// pending decision, which works on the cancelled step (S6-F 4/5).
func TestReviewPipeline_CancelledWhileWaitingKeepsTheDecision(t *testing.T) {
	f, step := waitingStep(t)
	f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusCancelled
	f.svc.PlanEnded(f.ctx, "plan-1", "cancelled")

	if f.store.pipelines["plan-1"].State != review.PipelineAwaitingDecision || !hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("the pending decision of a cancelled plan was dropped")
	}
	if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err != nil {
		t.Fatalf("Decide(undo) on the cancelled step: %v", err)
	}
	if len(f.planner.rejected) != 0 || f.store.pipelines["plan-1"].State != review.PipelineDone {
		t.Fatal("want the change undone without touching the cancelled plan")
	}
}

// --- S6-F 6: pending decisions survive a missed event ---

func TestReviewPipeline_PendingDecisions(t *testing.T) {
	f, step := waitingStep(t)

	pending, err := f.svc.PendingDecisions(f.ctx, "proj-1")
	if err != nil {
		t.Fatalf("PendingDecisions: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %+v, want one", pending)
	}
	d := pending[0]
	if d.RunID != "run-4" || d.PlanID != "plan-1" || d.StepID != step.ID || d.ProjectID != "proj-1" ||
		d.ImpactLevel != "high" || !d.Structural || d.FilesChanged != 2 || d.StepStatus != string(plan.StepStatusWaitingApproval) {
		t.Fatalf("pending decision = %+v, want the waiting refactoring with its impact", d)
	}

	if _, err := f.svc.PendingDecisions(f.ctx, "other"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("PendingDecisions of an unknown project = %v, want not found", err)
	}
	if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, true); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if pending, err := f.svc.PendingDecisions(f.ctx, "proj-1"); err != nil || len(pending) != 0 {
		t.Fatalf("PendingDecisions after the decision = %+v, %v, want none", pending, err)
	}
}

// When the review record cannot be loaded, the approval request takes its
// project from the plan: the dialog filters requests by project.
func TestReviewPipeline_ApprovalRequestTakesTheProjectFromThePlan(t *testing.T) {
	f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 101, "line") })
	f.store.pipelines = nil // GetReviewPipeline fails below
	f.store.pipelineErr = errors.New("connection reset")

	if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s, want waiting for approval", got)
	}
	events := f.hub.snapshot()
	if len(events) != 1 || events[0].Data.ProjectID != "proj-1" || events[0].Data.Reason == "" {
		t.Fatalf("events = %+v, want one approval request for proj-1 with the reason", events)
	}
}

// --- S6-F 7: one review pipeline per project, agents not shared ---

func TestReviewPipeline_SecondPipelineOnTheProjectConflicts(t *testing.T) {
	f := newReviewFixture(t)
	f.store.activeReview = true

	for name, start := range map[string]func() (*plan.ExecutionPlan, error){
		"review-refactor":   func() (*plan.ExecutionPlan, error) { return f.svc.StartReviewPipeline(f.ctx, "proj-1") },
		"boundary analysis": func() (*plan.ExecutionPlan, error) { return f.svc.StartBoundaryAnalysis(f.ctx, "proj-1") },
	} {
		p, err := start()
		if !errors.Is(err, domain.ErrConflict) || !errors.Is(err, review.ErrPipelineActive) || p != nil {
			t.Fatalf("%s: %+v, %v, want a conflict with the active pipeline", name, p, err)
		}
	}
	if len(f.store.tasks) != 0 || f.planner.created != nil {
		t.Fatal("a refused pipeline created tasks or a plan")
	}
}

// The guard in the store refuses a pipeline that raced past the check: the
// start fails with the conflict and leaves nothing behind.
func TestReviewPipeline_GuardConflictUndoesTheStart(t *testing.T) {
	f := newReviewFixture(t)
	f.store.recordErr = fmt.Errorf("create review pipeline: %w: %w", domain.ErrConflict, review.ErrPipelineActive)

	if p, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); !errors.Is(err, review.ErrPipelineActive) || p != nil {
		t.Fatalf("StartReviewPipeline = %+v, %v, want the guard's conflict", p, err)
	}
	if len(f.planner.cancelled) != 1 || len(f.store.cancelledTasks) != 4 || len(f.teams.ended) != 1 {
		t.Fatalf("cancelled plans %v, tasks %v, ended teams %v: want everything undone", f.planner.cancelled, f.store.cancelledTasks, f.teams.ended)
	}
}

// An agent that is idle between the steps of another plan is not picked.
func TestReviewPipeline_PickAgentSkipsAgentsOfActivePlans(t *testing.T) {
	f := newReviewFixture(t)
	f.store.plans["other"] = &plan.ExecutionPlan{ID: "other", ProjectID: "proj-1", Status: plan.StatusRunning,
		Steps: []plan.Step{{ID: "s1", AgentID: "idle", Status: plan.StepStatusCompleted}, {ID: "s2", AgentID: "idle", Status: plan.StepStatusPending}}}
	f.store.plans["ended"] = &plan.ExecutionPlan{ID: "ended", ProjectID: "proj-1", Status: plan.StatusCompleted,
		Steps: []plan.Step{{ID: "s3", AgentID: "free"}}}

	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("StartReviewPipeline = %v, want no free agent", err)
	}

	f.store.agents = append(f.store.agents, agent.Agent{ID: "free", ProjectID: "proj-1", Status: agent.StatusIdle})
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	if got := f.planner.created.Steps[0].AgentID; got != "free" {
		t.Fatalf("picked agent %s, want the one no running plan uses", got)
	}
}

// Review finding 7: retention purges old runs and clears the plan step's run
// reference; the pending decision is matched on the review record and stays
// decidable, for a waiting and for a failed step.
func TestReviewPipeline_DecisionSurvivesThePurgedRun(t *testing.T) {
	t.Run("waiting step", func(t *testing.T) {
		f, step := waitingStep(t)
		f.store.plans["plan-1"].Steps[0].RunID = ""
		if d, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err != nil || d.Status != "rejected" {
			t.Fatalf("Decide(undo) = %+v, %v", d, err)
		}
		if len(f.planner.rejected) != 1 || f.store.pipelines["plan-1"].State != review.PipelineDone {
			t.Fatal("want the step rejected and the decision finished")
		}
	})
	t.Run("failed step", func(t *testing.T) {
		f, step := endedRefactoring(t, plan.StepStatusFailed, "failed", func(dir string) { writeLines(t, dir, "a.go", 104, "half") })
		f.store.plans["plan-1"].Steps[0].RunID = ""
		if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, true); err != nil {
			t.Fatalf("Decide(keep): %v", err)
		}
		if f.store.pipelines["plan-1"].State != review.PipelineDone {
			t.Fatal("decision not finished")
		}
	})
	t.Run("another run is still refused", func(t *testing.T) {
		f, step := waitingStep(t)
		f.store.plans["plan-1"].Steps[0].RunID = ""
		if _, err := f.svc.Decide(f.ctx, "run-other", "plan-1", step.ID, true); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Decide = %v, want not found", err)
		}
	})
}

// Review finding 8: the plan is cancelled while its refactoring step is
// prepared - PlanEnded ends the pipeline and drops its refs before the
// baseline is written, and the preparation then loses the compare-and-swap:
// it must not leave its baseline ref behind.
func TestReviewPipeline_PrepareRacingThePlanEnd(t *testing.T) {
	f := newReviewFixture(t)
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	step := &plan.Step{ID: "step-3", PlanID: "plan-1", ModeID: "refactorer"}
	f.store.plans["plan-1"] = &plan.ExecutionPlan{ID: "plan-1", ProjectID: "proj-1", Status: plan.StatusCancelled, Steps: []plan.Step{*step}}
	f.store.onGetProject = func() { f.svc.PlanEnded(f.ctx, "plan-1", "cancelled") }

	if err := f.svc.PrepareStep(f.ctx, step); err != nil {
		t.Fatalf("PrepareStep: %v", err)
	}
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineDone || rp.BaselineSHA != "" {
		t.Fatalf("review record = %+v, want the ended pipeline untouched", rp)
	}
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("the baseline ref of a pipeline that ended meanwhile dangles")
	}
}

// A preparation that loses to another one puts the ref back to the baseline
// the record holds, so the gate does not see a moved ref.
func TestReviewPipeline_PrepareLosingToAnotherKeepsTheRecordedRef(t *testing.T) {
	f := newReviewFixture(t)
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	step := &plan.Step{ID: "step-3", PlanID: "plan-1", ModeID: "refactorer"}
	f.store.onGetProject = func() {
		if err := f.svc.PrepareStep(f.ctx, step); err != nil {
			t.Errorf("inner PrepareStep: %v", err)
		}
		writeLines(t, f.dir, "a.go", 120, "changed between the two snapshots")
	}

	if err := f.svc.PrepareStep(f.ctx, step); err != nil {
		t.Fatalf("PrepareStep: %v", err)
	}
	rp := f.store.pipelines["plan-1"]
	if rp.State != review.PipelineRefactoring || reviewGit(t, f.dir, "rev-parse", reviewBaselineRef("plan-1")) != rp.BaselineSHA {
		t.Fatalf("baseline ref does not point at the recorded baseline %s", rp.BaselineSHA)
	}
}

// gateNow gates a step and applies the answer at once, as the orchestrator
// does once it stored the step status.
func gateNow(f *reviewFixture, step *plan.Step) plan.StepStatus {
	status, apply := f.svc.GateStep(f.ctx, step)
	if apply != nil {
		apply(f.ctx)
	}
	return status
}

// Review finding 9: the gate runs without the scheduling lock, and the step
// waits only once the orchestrator stored its status. The decision is
// recorded and announced only then (apply); until it is, keep works and
// undo asks to try again, and a step kept meanwhile is not offered again.
func TestReviewPipeline_DecisionBeforeTheGateIsApplied(t *testing.T) {
	t.Run("nothing is offered before the step waits", func(t *testing.T) {
		f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 300, "rewritten") })
		status, apply := f.svc.GateStep(f.ctx, step)
		if status != plan.StepStatusWaitingApproval || apply == nil {
			t.Fatalf("GateStep = %s (apply %v), want waiting with an apply", status, apply != nil)
		}
		if len(f.hub.snapshot()) != 0 || f.store.pipelines["plan-1"].State != review.PipelineRefactoring {
			t.Fatal("a decision was offered before the step waits")
		}
		// The step still runs: there is nothing to decide.
		if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, true); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("Decide while gating = %v, want a validation error", err)
		}
		f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusWaitingApproval
		apply(f.ctx)
		if events := f.hub.snapshot(); len(events) != 1 || f.store.pipelines["plan-1"].State != review.PipelineAwaitingDecision {
			t.Fatalf("after apply: %d events, state %s, want one request and the decision recorded", len(events), f.store.pipelines["plan-1"].State)
		}
	})
	t.Run("kept between the status and the record", func(t *testing.T) {
		f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 300, "rewritten") })
		_, apply := f.svc.GateStep(f.ctx, step)
		f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusWaitingApproval

		if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("undo before the record = %v, want try again (conflict)", err)
		}
		if d, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, true); err != nil || d.Status != "approved" {
			t.Fatalf("keep before the record = %+v, %v", d, err)
		}
		if len(f.planner.approved) != 1 || f.store.pipelines["plan-1"].State != review.PipelineDone {
			t.Fatal("want the step approved and the pipeline done")
		}
		apply(f.ctx)
		if len(f.hub.snapshot()) != 0 || f.store.pipelines["plan-1"].State != review.PipelineDone {
			t.Fatal("a decided refactoring was offered again")
		}
	})
}

// Review finding 4: a cancelled (or timed-out, stopped) refactoring run is
// terminal at once, but its worker may still write until it confirms the
// stop. The change is measured only then - or, when the worker never
// confirms, by the watchdog check after the grace - so the worker's late
// writes are part of the refactoring and an undo reverts them.
func TestReviewPipeline_EndedRefactoringWaitsForItsWorker(t *testing.T) {
	f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 104, "half done") })
	runs := &fakeRunEnds{mayWrite: map[string]bool{"run-4": true}}
	f.svc.SetRunEnds(runs)
	f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusCancelled
	f.store.plans["plan-1"].Status = plan.StatusCancelled

	f.svc.PlanEnded(f.ctx, "plan-1", "cancelled")
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineRefactoring || len(f.hub.snapshot()) != 0 {
		t.Fatalf("state %s, %d events: want the refactoring unmeasured while its worker may write", rp.State, len(f.hub.snapshot()))
	}

	writeLines(t, f.dir, "late.go", 3, "written after the stop") // the worker still writes
	runs.mayWrite["run-4"] = false
	f.svc.WorkerStopped(f.ctx, "run-4")

	rp := f.store.pipelines["plan-1"]
	if rp.State != review.PipelineAwaitingDecision || rp.Impact == nil || rp.Impact.FilesChanged != 2 || len(f.hub.snapshot()) != 1 {
		t.Fatalf("record %+v (impact %+v): want the decision with both files", rp, rp.Impact)
	}
	if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err != nil {
		t.Fatalf("Decide(undo): %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "late.go")); !os.IsNotExist(err) {
		t.Fatalf("late.go after the undo: %v, want the worker's late write undone", err)
	}

	// A second confirmation finds nothing left to measure.
	f.svc.WorkerStopped(f.ctx, "run-4")
	if len(f.hub.snapshot()) != 1 {
		t.Fatal("a refactoring was offered twice")
	}
}

// The watchdog check measures ended refactorings whose worker never
// confirmed the stop, once the grace has passed, in their own tenant.
func TestReviewPipeline_WatchdogMeasuresEndedRefactorings(t *testing.T) {
	f, _ := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 104, "half done") })
	f.svc.SetRunEnds(&fakeRunEnds{mayWrite: map[string]bool{"run-4": true}})
	f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusCancelled
	f.store.plans["plan-1"].Status = plan.StatusCancelled
	f.svc.PlanEnded(f.ctx, "plan-1", "cancelled")

	lister := &fakeEndedRefactorings{rows: []database.EndedReviewRefactoring{{PlanID: "plan-1", TenantID: reviewTenant, PlanStatus: "cancelled"}}}
	n, err := f.svc.EndUndecidedRefactorings(context.Background(), lister)
	if err != nil || n != 1 {
		t.Fatalf("EndUndecidedRefactorings = %d, %v, want 1", n, err)
	}
	if time.Since(lister.before) < time.Minute {
		t.Fatalf("listed refactorings ended before %s, want at least the grace ago", lister.before)
	}
	events := f.hub.snapshot()
	if f.store.pipelines["plan-1"].State != review.PipelineAwaitingDecision || len(events) != 1 || events[0].Tenant != reviewTenant {
		t.Fatal("want the decision recorded and announced in the pipeline's tenant")
	}
}
