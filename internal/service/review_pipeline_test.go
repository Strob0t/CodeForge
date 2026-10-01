package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
	upserted   *boundary.ProjectBoundaryConfig
	pipelines  map[string]*review.Pipeline // by plan ID
}

func (f *fakeReviewStore) CreateReviewPipeline(_ context.Context, rp *review.Pipeline) error {
	if f.pipelines == nil {
		f.pipelines = map[string]*review.Pipeline{}
	}
	stored := *rp
	f.pipelines[rp.PlanID] = &stored
	return nil
}

func (f *fakeReviewStore) GetReviewPipeline(_ context.Context, planID string) (*review.Pipeline, error) {
	rp, ok := f.pipelines[planID]
	if !ok {
		return nil, fmt.Errorf("review pipeline of plan %s: %w", planID, domain.ErrNotFound)
	}
	stored := *rp
	return &stored, nil
}

func (f *fakeReviewStore) GetProject(_ context.Context, id string) (*project.Project, error) {
	if p, ok := f.projects[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("project %s: %w", id, domain.ErrNotFound)
}

func (f *fakeReviewStore) ListAgents(_ context.Context, _ string) ([]agent.Agent, error) {
	return f.agents, nil
}

func (f *fakeReviewStore) CreateTask(_ context.Context, req task.CreateRequest) (*task.Task, error) {
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
	created    *plan.CreatePlanRequest
	startFails bool
	cancelled  []string
	approved   []string
	rejected   []string
}

func (f *fakeReviewPlanner) CreatePlan(_ context.Context, req *plan.CreatePlanRequest) (*plan.ExecutionPlan, error) {
	f.created = req
	p := &plan.ExecutionPlan{ID: "plan-1", ProjectID: req.ProjectID, TeamID: req.TeamID, Status: plan.StatusPending}
	for i, st := range req.Steps {
		p.Steps = append(p.Steps, plan.Step{ID: fmt.Sprintf("step-%d", i), TaskID: st.TaskID, AgentID: st.AgentID, ModeID: st.ModeID})
	}
	return p, nil
}

func (f *fakeReviewPlanner) StartPlan(_ context.Context, id string) (*plan.ExecutionPlan, error) {
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

type fakeReviewTeams struct{ created []*agent.CreateTeamRequest }

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
	if !hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("no workspace baseline recorded for the refactoring")
	}
	rp := f.store.pipelines["plan-1"]
	if rp == nil || rp.BaselineSHA != reviewGit(t, f.dir, "rev-parse", reviewBaselineRef("plan-1")) || rp.ProjectID != "proj-1" {
		t.Fatalf("review record = %+v, want the plan's baseline commit", rp)
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

			if got := f.svc.GateStep(f.ctx, step); got != tt.wantStatus {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 101, "line") }) // low impact
			tt.tamper(t, f)

			if got := f.svc.GateStep(f.ctx, step); got != plan.StepStatusWaitingApproval {
				t.Fatalf("GateStep = %s, want waiting for approval", got)
			}
			events := f.hub.snapshot()
			if len(events) != 1 || events[0].EventType != event.EventReviewApprovalRequired || events[0].Data.Reason == "" {
				t.Fatalf("events = %+v, want one approval request with the reason", events)
			}
		})
	}
}

// Changes the user had in the workspace before the pipeline started are not
// part of the refactoring.
func TestReviewPipeline_GateMeasuresOnlyThePipelinesChange(t *testing.T) {
	f := newReviewFixture(t)
	writeLines(t, f.dir, "a.go", 400, "user")
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	f.store.plans["plan-1"] = &plan.ExecutionPlan{ID: "plan-1", ProjectID: "proj-1", Status: plan.StatusRunning}
	writeLines(t, f.dir, "api.proto", 6, "message")

	step := &plan.Step{ID: "step-3", PlanID: "plan-1", ModeID: "refactorer", RunID: "run-4"}
	if got := f.svc.GateStep(f.ctx, step); got != plan.StepStatusCompleted {
		t.Fatalf("GateStep = %s, want completed (a one-line refactoring)", got)
	}
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("the baseline of a decided refactoring is kept")
	}
}

// A refactorer step of a plan the review pipeline did not start has no
// baseline and is not gated.
func TestReviewPipeline_GateIgnoresOtherPlans(t *testing.T) {
	f := newReviewFixture(t)
	f.store.plans["plan-9"] = &plan.ExecutionPlan{ID: "plan-9", ProjectID: "proj-1"}
	writeLines(t, f.dir, "a.go", 500, "x")
	step := &plan.Step{ID: "s", PlanID: "plan-9", ModeID: "refactorer", RunID: "r"}
	if got := f.svc.GateStep(f.ctx, step); got != plan.StepStatusCompleted {
		t.Fatalf("GateStep = %s, want completed", got)
	}
	if other := (&plan.Step{ID: "s2", PlanID: "plan-9", ModeID: "coder"}); f.svc.GateStep(f.ctx, other) != plan.StepStatusCompleted {
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
	if got := f.svc.GateStep(f.ctx, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s", got)
	}
	step.Status = plan.StepStatusWaitingApproval
	f.store.plans["plan-1"].Steps = []plan.Step{*step}
	return f, step
}

func TestReviewPipeline_RejectUndoesTheRefactoring(t *testing.T) {
	f, step := waitingStep(t)
	baseline := reviewGit(t, f.dir, "show", "HEAD:a.go")

	if err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err != nil {
		t.Fatalf("Decide(reject): %v", err)
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
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("baseline kept after the decision")
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

	if err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err != nil {
		t.Fatalf("Decide(reject): %v", err)
	}
	if got := readFile(t, f.dir, "a.go"); got != original+"\n" {
		t.Fatalf("a.go after reject is not the recorded baseline:\n%.60s", got)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "new.go")); !os.IsNotExist(err) {
		t.Fatalf("new.go after reject: %v, want it removed", err)
	}
}

// A refactoring the agent committed is undone too: HEAD returns to the commit
// checked out when the pipeline started, the working tree and the user's
// index to their state then (the baseline is a base checkpoint commit).
func TestReviewPipeline_RejectUndoesACommittedRefactoring(t *testing.T) {
	var started string
	f, step := gateFixture(t, nil, func(dir string) {
		started = reviewGit(t, dir, "rev-parse", "HEAD")
		writeLines(t, dir, "a.go", 300, "rewritten")
		reviewGit(t, dir, "commit", "-qam", "refactor")
	})
	if got := f.svc.GateStep(f.ctx, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s, want waiting for approval", got)
	}
	f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusWaitingApproval

	if err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err != nil {
		t.Fatalf("Decide(reject): %v", err)
	}
	if head := reviewGit(t, f.dir, "rev-parse", "HEAD"); head != started {
		t.Fatalf("HEAD = %s after reject, want %s", head, started)
	}
	if status := reviewGit(t, f.dir, "status", "--porcelain"); status != "" {
		t.Fatalf("workspace after reject is not the baseline:\n%s", status)
	}
}

func TestReviewPipeline_ApproveKeepsTheRefactoring(t *testing.T) {
	f, step := waitingStep(t)

	if err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, true); err != nil {
		t.Fatalf("Decide(approve): %v", err)
	}
	if !strings.Contains(readFile(t, f.dir, "a.go"), "rewritten") {
		t.Fatal("approved refactoring was undone")
	}
	if len(f.planner.approved) != 1 || len(f.planner.rejected) != 0 {
		t.Fatalf("approved %v rejected %v, want the step approved", f.planner.approved, f.planner.rejected)
	}
}

func TestReviewPipeline_DecideErrors(t *testing.T) {
	t.Run("run of another step", func(t *testing.T) {
		f, step := waitingStep(t)
		if err := f.svc.Decide(f.ctx, "run-other", "plan-1", step.ID, true); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Decide = %v, want not found", err)
		}
	})
	t.Run("step not waiting", func(t *testing.T) {
		f, step := waitingStep(t)
		f.store.plans["plan-1"].Steps[0].Status = plan.StepStatusCompleted
		if err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); !errors.Is(err, domain.ErrValidation) {
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
		if err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err == nil {
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
		if err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("Decide(reject) = %v, want a validation error", err)
		}
		if len(f.planner.rejected) != 0 {
			t.Fatal("step rejected although the refactoring could not be undone")
		}
	})
	t.Run("recorded baseline commit is gone", func(t *testing.T) {
		f, step := waitingStep(t)
		f.store.pipelines["plan-1"].BaselineSHA = "1111111111111111111111111111111111111111"
		if err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err == nil {
			t.Fatal("Decide(reject) succeeded without the baseline commit")
		}
		if len(f.planner.rejected) != 0 {
			t.Fatal("step rejected although the refactoring could not be undone")
		}
	})
}

// A review plan that ends without a decision (a step failed, the plan was
// cancelled) leaves no baseline behind.
func TestReviewPipeline_PlanEndDropsTheBaseline(t *testing.T) {
	f := newReviewFixture(t)
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	f.store.plans["plan-1"] = &plan.ExecutionPlan{ID: "plan-1", ProjectID: "proj-1", Steps: []plan.Step{{ModeID: "refactorer"}}}

	f.svc.PlanEnded(f.ctx, "plan-1", "failed")
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("baseline kept after the plan ended")
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

	if got := f.svc.GateStep(f.ctx, step); got != plan.StepStatusCompleted {
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
	f.svc.GateStep(f.ctx, &plan.Step{ModeID: "boundary_analyzer", RunID: "run-1"})
	if f.store.upserted != nil {
		t.Fatalf("stored %+v from an output without BOUNDARIES.json", f.store.upserted)
	}
}
