package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// Run, plan and team status updates never leave a terminal state: a run that
// was cancelled or timed out while a tool call waited for HITL approval must
// not be moved back to running (KI-31). The store reports the refused update
// as domain.ErrConflict, a missing row (or another tenant's) as
// domain.ErrNotFound.

// statusFixture is a project with a task and an agent in a fresh tenant.
type statusFixture struct {
	store   *postgres.Store
	ctx     context.Context
	project *project.Project
	task    *task.Task
	agent   *agent.Agent
}

func newStatusFixture(t *testing.T) *statusFixture {
	t.Helper()
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))

	proj, err := store.CreateProject(ctx, &project.CreateRequest{Name: "status-guard", Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	tsk, err := store.CreateTask(ctx, task.CreateRequest{ProjectID: proj.ID, Title: "status-guard", Prompt: "p"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	ag, err := store.CreateAgent(ctx, proj.ID, "status-guard", "aider", nil, nil)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	return &statusFixture{store: store, ctx: ctx, project: proj, task: tsk, agent: ag}
}

func (f *statusFixture) newRun(t *testing.T, status run.Status) *run.Run {
	t.Helper()
	r := &run.Run{
		TaskID: f.task.ID, AgentID: f.agent.ID, ProjectID: f.project.ID,
		ExecMode: run.ExecModeMount, Status: status,
	}
	if err := f.store.CreateRun(f.ctx, r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	return r
}

func (f *statusFixture) runStatus(t *testing.T, id string) *run.Run {
	t.Helper()
	r, err := f.store.GetRun(f.ctx, id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	return r
}

func TestStore_RunUpdatesLeaveNoTerminalState(t *testing.T) {
	f := newStatusFixture(t)

	for _, terminal := range []run.Status{run.StatusCompleted, run.StatusFailed, run.StatusCancelled, run.StatusTimeout} {
		t.Run(string(terminal), func(t *testing.T) {
			r := f.newRun(t, run.StatusRunning)
			first := &run.CompletionRequest{ID: r.ID, Status: terminal, Output: "first", Error: "first", StepCount: 3, CostUSD: 0.5}
			if err := f.store.CompleteRun(f.ctx, first); err != nil {
				t.Fatalf("CompleteRun(%s): %v", terminal, err)
			}

			// The HITL write-back: the tool call handler moves the run back to running.
			err := f.store.UpdateRunStatus(f.ctx, r.ID, run.StatusRunning, 4, 0.7, 10, 20)
			if !errors.Is(err, domain.ErrConflict) {
				t.Errorf("UpdateRunStatus(running) from %s: err = %v, want ErrConflict", terminal, err)
			}
			// A second completion of the run, e.g. the worker's late run.complete.
			second := &run.CompletionRequest{ID: r.ID, Status: run.StatusCompleted, Output: "second", StepCount: 9}
			if err := f.store.CompleteRun(f.ctx, second); !errors.Is(err, domain.ErrConflict) {
				t.Errorf("second CompleteRun from %s: err = %v, want ErrConflict", terminal, err)
			}

			got := f.runStatus(t, r.ID)
			if got.Status != terminal || got.Output != "first" || got.StepCount != 3 {
				t.Errorf("run = %s/%q/%d steps, want %s/%q/3 steps unchanged", got.Status, got.Output, got.StepCount, terminal, "first")
			}
		})
	}
}

func TestStore_RunUpdatesFromActiveStates(t *testing.T) {
	f := newStatusFixture(t)

	tests := []struct {
		name   string
		from   run.Status
		update func(ctx context.Context, store *postgres.Store, id string) error
		want   run.Status
	}{
		{
			name: "pending to running",
			from: run.StatusPending,
			update: func(ctx context.Context, store *postgres.Store, id string) error {
				return store.UpdateRunStatus(ctx, id, run.StatusRunning, 0, 0, 0, 0)
			},
			want: run.StatusRunning,
		},
		{
			name: "running step count",
			from: run.StatusRunning,
			update: func(ctx context.Context, store *postgres.Store, id string) error {
				return store.UpdateRunStatus(ctx, id, run.StatusRunning, 1, 0.1, 5, 6)
			},
			want: run.StatusRunning,
		},
		{
			name: "running to quality gate",
			from: run.StatusRunning,
			update: func(ctx context.Context, store *postgres.Store, id string) error {
				return store.UpdateRunStatus(ctx, id, run.StatusQualityGate, 2, 0.1, 5, 6)
			},
			want: run.StatusQualityGate,
		},
		{
			name: "quality gate completes",
			from: run.StatusQualityGate,
			update: func(ctx context.Context, store *postgres.Store, id string) error {
				return store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusCompleted})
			},
			want: run.StatusCompleted,
		},
		{
			name: "pending is cancelled",
			from: run.StatusPending,
			update: func(ctx context.Context, store *postgres.Store, id string) error {
				return store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusCancelled})
			},
			want: run.StatusCancelled,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := f.newRun(t, tc.from)
			if err := tc.update(f.ctx, f.store, r.ID); err != nil {
				t.Fatalf("update: %v", err)
			}
			if got := f.runStatus(t, r.ID).Status; got != tc.want {
				t.Errorf("status = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestStore_RunUpdatesMissingOrForeignRun(t *testing.T) {
	f := newStatusFixture(t)
	terminal := f.newRun(t, run.StatusRunning)
	if err := f.store.CompleteRun(f.ctx, &run.CompletionRequest{ID: terminal.ID, Status: run.StatusCancelled}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	active := f.newRun(t, run.StatusRunning)
	otherTenant := ctxWithTenant(t, createTestTenant(t, f.store))

	tests := []struct {
		name string
		ctx  context.Context
		id   string
	}{
		{name: "unknown run", ctx: f.ctx, id: uuid.New().String()},
		// Another tenant's run does not exist for the caller, terminal or not.
		{name: "other tenant's active run", ctx: otherTenant, id: active.ID},
		{name: "other tenant's terminal run", ctx: otherTenant, id: terminal.ID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := f.store.UpdateRunStatus(tc.ctx, tc.id, run.StatusRunning, 1, 0, 0, 0); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("UpdateRunStatus: err = %v, want ErrNotFound", err)
			}
			if err := f.store.CompleteRun(tc.ctx, &run.CompletionRequest{ID: tc.id, Status: run.StatusCompleted}); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("CompleteRun: err = %v, want ErrNotFound", err)
			}
		})
	}
	if got := f.runStatus(t, active.ID).Status; got != run.StatusRunning {
		t.Errorf("other tenant changed the run: status %s", got)
	}
}

// TestStore_CompleteRun_ExactlyOnceUnderConcurrency completes one run from
// several goroutines: exactly one completion wins, the others conflict.
func TestStore_CompleteRun_ExactlyOnceUnderConcurrency(t *testing.T) {
	f := newStatusFixture(t)
	r := f.newRun(t, run.StatusRunning)
	statuses := []run.Status{run.StatusCompleted, run.StatusFailed, run.StatusCancelled, run.StatusTimeout,
		run.StatusCompleted, run.StatusFailed, run.StatusCancelled, run.StatusTimeout}

	errs := make([]error, len(statuses))
	var wg sync.WaitGroup
	for i, status := range statuses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = f.store.CompleteRun(f.ctx, &run.CompletionRequest{ID: r.ID, Status: status})
		}()
	}
	wg.Wait()

	var winner run.Status
	for i, err := range errs {
		switch {
		case err == nil:
			if winner != "" {
				t.Errorf("second completion succeeded (%s after %s)", statuses[i], winner)
			}
			winner = statuses[i]
		case !errors.Is(err, domain.ErrConflict):
			t.Errorf("completion %d: err = %v, want nil or ErrConflict", i, err)
		}
	}
	if winner == "" {
		t.Fatal("no completion succeeded")
	}
	if got := f.runStatus(t, r.ID).Status; got != winner {
		t.Errorf("status = %s, want the winning %s", got, winner)
	}
}

// TestStore_RunCancelledDuringStepUpdateStaysCancelled races the tool call
// handler's step update (running) against a cancel: the run always ends
// cancelled.
func TestStore_RunCancelledDuringStepUpdateStaysCancelled(t *testing.T) {
	f := newStatusFixture(t)
	for range 20 {
		r := f.newRun(t, run.StatusRunning)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			err := f.store.UpdateRunStatus(f.ctx, r.ID, run.StatusRunning, 1, 0, 0, 0)
			if err != nil && !errors.Is(err, domain.ErrConflict) {
				t.Errorf("UpdateRunStatus: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := f.store.CompleteRun(f.ctx, &run.CompletionRequest{ID: r.ID, Status: run.StatusCancelled}); err != nil {
				t.Errorf("CompleteRun: %v", err)
			}
		}()
		wg.Wait()
		if got := f.runStatus(t, r.ID).Status; got != run.StatusCancelled {
			t.Fatalf("status = %s, want cancelled", got)
		}
	}
}

func TestStore_PlanStatusLeavesNoTerminalState(t *testing.T) {
	f := newStatusFixture(t)
	newPlan := func(t *testing.T) *plan.ExecutionPlan {
		t.Helper()
		p := &plan.ExecutionPlan{
			ProjectID: f.project.ID, Name: "status-guard", Protocol: plan.ProtocolSequential,
			Status: plan.StatusPending, MaxParallel: 1,
		}
		if err := f.store.CreatePlan(f.ctx, p); err != nil {
			t.Fatalf("CreatePlan: %v", err)
		}
		return p
	}
	planStatus := func(t *testing.T, id string) plan.Status {
		t.Helper()
		p, err := f.store.GetPlan(f.ctx, id)
		if err != nil {
			t.Fatalf("GetPlan: %v", err)
		}
		return p.Status
	}

	for _, terminal := range []plan.Status{plan.StatusCompleted, plan.StatusFailed, plan.StatusCancelled} {
		t.Run(string(terminal), func(t *testing.T) {
			p := newPlan(t)
			if err := f.store.UpdatePlanStatus(f.ctx, p.ID, plan.StatusRunning); err != nil {
				t.Fatalf("pending to running: %v", err)
			}
			if err := f.store.UpdatePlanStatus(f.ctx, p.ID, terminal); err != nil {
				t.Fatalf("running to %s: %v", terminal, err)
			}
			for _, next := range []plan.Status{plan.StatusRunning, plan.StatusPending, plan.StatusCompleted, plan.StatusFailed, plan.StatusCancelled} {
				if err := f.store.UpdatePlanStatus(f.ctx, p.ID, next); !errors.Is(err, domain.ErrConflict) {
					t.Errorf("%s to %s: err = %v, want ErrConflict", terminal, next, err)
				}
			}
			if got := planStatus(t, p.ID); got != terminal {
				t.Errorf("status = %s, want %s", got, terminal)
			}
		})
	}

	t.Run("missing or foreign plan", func(t *testing.T) {
		p := newPlan(t)
		otherTenant := ctxWithTenant(t, createTestTenant(t, f.store))
		if err := f.store.UpdatePlanStatus(f.ctx, uuid.New().String(), plan.StatusRunning); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("unknown plan: err = %v, want ErrNotFound", err)
		}
		if err := f.store.UpdatePlanStatus(otherTenant, p.ID, plan.StatusRunning); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("other tenant's plan: err = %v, want ErrNotFound", err)
		}
		if got := planStatus(t, p.ID); got != plan.StatusPending {
			t.Errorf("status = %s, want pending", got)
		}
	})
}

func TestStore_TeamStatusLeavesNoTerminalState(t *testing.T) {
	f := newStatusFixture(t)
	newTeam := func(t *testing.T) *agent.Team {
		t.Helper()
		team, err := f.store.CreateTeam(f.ctx, agent.CreateTeamRequest{ProjectID: f.project.ID, Name: "status-guard", Protocol: "sequential"})
		if err != nil {
			t.Fatalf("CreateTeam: %v", err)
		}
		return team
	}
	teamStatus := func(t *testing.T, id string) agent.TeamStatus {
		t.Helper()
		team, err := f.store.GetTeam(f.ctx, id)
		if err != nil {
			t.Fatalf("GetTeam: %v", err)
		}
		return team.Status
	}

	for _, terminal := range []agent.TeamStatus{agent.TeamStatusCompleted, agent.TeamStatusFailed} {
		t.Run(string(terminal), func(t *testing.T) {
			team := newTeam(t)
			if err := f.store.UpdateTeamStatus(f.ctx, team.ID, agent.TeamStatusActive); err != nil {
				t.Fatalf("to active: %v", err)
			}
			if err := f.store.UpdateTeamStatus(f.ctx, team.ID, terminal); err != nil {
				t.Fatalf("to %s: %v", terminal, err)
			}
			for _, next := range []agent.TeamStatus{agent.TeamStatusActive, agent.TeamStatusInitializing, agent.TeamStatusCompleted, agent.TeamStatusFailed} {
				if err := f.store.UpdateTeamStatus(f.ctx, team.ID, next); !errors.Is(err, domain.ErrConflict) {
					t.Errorf("%s to %s: err = %v, want ErrConflict", terminal, next, err)
				}
			}
			if got := teamStatus(t, team.ID); got != terminal {
				t.Errorf("status = %s, want %s", got, terminal)
			}
		})
	}

	t.Run("missing or foreign team", func(t *testing.T) {
		team := newTeam(t)
		otherTenant := ctxWithTenant(t, createTestTenant(t, f.store))
		if err := f.store.UpdateTeamStatus(f.ctx, uuid.New().String(), agent.TeamStatusActive); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("unknown team: err = %v, want ErrNotFound", err)
		}
		if err := f.store.UpdateTeamStatus(otherTenant, team.ID, agent.TeamStatusCompleted); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("other tenant's team: err = %v, want ErrNotFound", err)
		}
		if got := teamStatus(t, team.ID); got != agent.TeamStatusInitializing {
			t.Errorf("status = %s, want initializing", got)
		}
	})
}
