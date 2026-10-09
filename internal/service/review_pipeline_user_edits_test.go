package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/review"
)

// KI-94 (owner decision 2026-10-04): the paths users changed through the
// editor or the file API while a refactoring ran count as its change, so
// its approval dialog lists them - in the approval request and in the
// pending decisions it loads - and they are deleted with the decision.

func (f *fakeReviewStore) ListReviewUserEdits(_ context.Context, planID string, limit int) ([]review.UserEdit, int, error) {
	f.editsLimit = limit
	if f.editsErr != nil {
		return nil, 0, f.editsErr
	}
	edits := f.userEdits[planID]
	total := len(edits)
	if len(edits) > limit {
		edits = edits[:limit]
	}
	return slices.Clone(edits), total, nil
}

func (f *fakeReviewStore) DeleteReviewUserEdits(_ context.Context, planID string) error {
	f.editsDeleted = append(f.editsDeleted, planID)
	delete(f.userEdits, planID)
	return nil
}

var planUserEdits = []review.UserEdit{
	{Path: "a.go", Operation: review.UserEditWrite, UserID: "u1", UserName: "Ada", EditedAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)},
	{Path: "old.go", Operation: review.UserEditRename, EditedAt: time.Date(2026, 10, 4, 12, 1, 0, 0, time.UTC)},
}

func approvalRequest(t *testing.T, f *reviewFixture) event.ReviewImpactEvent {
	t.Helper()
	events := f.hub.snapshot()
	if len(events) != 1 || events[0].EventType != event.EventReviewApprovalRequired {
		t.Fatalf("events = %+v, want one approval request", events)
	}
	return events[0].Data
}

func TestReviewPipeline_ApprovalRequestListsTheUserEdits(t *testing.T) {
	t.Run("high impact", func(t *testing.T) {
		f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 300, "rewritten") })
		f.store.userEdits = map[string][]review.UserEdit{"plan-1": planUserEdits}
		if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
			t.Fatalf("GateStep = %s, want waiting for approval", got)
		}
		ev := approvalRequest(t, f)
		if !slices.Equal(ev.UserEdits, planUserEdits) || ev.UserEditsTotal != 2 || f.store.editsLimit != maxListedUserEdits {
			t.Fatalf("request lists %+v (%d, limit %d), want the plan's user edits", ev.UserEdits, ev.UserEditsTotal, f.store.editsLimit)
		}
	})
	t.Run("ended refactoring", func(t *testing.T) {
		f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 104, "half done") })
		f.store.userEdits = map[string][]review.UserEdit{"plan-1": planUserEdits}
		step.Status = plan.StepStatusFailed
		f.store.plans["plan-1"].Steps = []plan.Step{*step}
		f.store.plans["plan-1"].Status = plan.StatusFailed
		f.svc.PlanEnded(f.ctx, "plan-1", string(plan.StatusFailed))
		if ev := approvalRequest(t, f); !slices.Equal(ev.UserEdits, planUserEdits) || ev.UserEditsTotal != 2 {
			t.Fatalf("request lists %+v (%d), want the plan's user edits", ev.UserEdits, ev.UserEditsTotal)
		}
	})
	t.Run("no user edits", func(t *testing.T) {
		f, _ := waitingStep(t)
		if ev := approvalRequest(t, f); ev.UserEdits != nil || ev.UserEditsTotal != 0 {
			t.Fatalf("request lists %+v (%d), want none", ev.UserEdits, ev.UserEditsTotal)
		}
	})
	// The request is not lost with the edits, and says they are missing
	// (review F2): the dialog loads them again with the pending decisions.
	t.Run("the edits cannot be read", func(t *testing.T) {
		f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 300, "rewritten") })
		f.store.editsErr = errors.New("connection reset")
		if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
			t.Fatalf("GateStep = %s, want waiting for approval", got)
		}
		if ev := approvalRequest(t, f); ev.UserEdits != nil || !ev.UserEditsUnavailable || ev.RunID != "run-4" {
			t.Fatalf("request = %+v, want it announced with the user edits marked unavailable", ev)
		}
	})
	t.Run("read edits are not marked unavailable", func(t *testing.T) {
		f, _ := waitingStep(t)
		if ev := approvalRequest(t, f); ev.UserEditsUnavailable {
			t.Fatalf("request = %+v, want the user edits available", ev)
		}
	})
}

// --- review F1: edits while the baseline is taken ---

func (f *fakeReviewStore) SetReviewBaseline(_ context.Context, planID, baselineSHA string) error {
	if hook := f.beforeBaseline; hook != nil {
		f.beforeBaseline = nil
		hook()
	}
	if f.baselineErr != nil {
		return f.baselineErr
	}
	stored, ok := f.pipelines[planID]
	if !ok {
		return domain.ErrNotFound
	}
	if stored.State != review.PipelineRefactoring || stored.BaselineSHA != "" {
		return domain.ErrConflict
	}
	updated := *stored
	updated.BaselineSHA = baselineSHA
	f.pipelines[planID] = &updated
	return nil
}

// RecordReviewUserEdits records like postgres.Store: for each pipeline of
// the project in state refactoring, one entry per path.
func (f *fakeReviewStore) RecordReviewUserEdits(_ context.Context, projectID, userID string, op review.UserEditOp, paths []string) error {
	for planID, rp := range f.pipelines {
		if rp.ProjectID != projectID || rp.State != review.PipelineRefactoring {
			continue
		}
		if f.userEdits == nil {
			f.userEdits = map[string][]review.UserEdit{}
		}
		for _, p := range paths {
			edits := slices.DeleteFunc(f.userEdits[planID], func(e review.UserEdit) bool { return e.Path == p })
			f.userEdits[planID] = append(edits, review.UserEdit{Path: p, Operation: op, UserID: userID, EditedAt: time.Now()})
		}
	}
	return nil
}

// preparingFixture is a started review pipeline whose refactorer step
// (run-4) is about to be prepared, with the file API on its workspace
// recording user edits.
func preparingFixture(t *testing.T) (f *reviewFixture, step *plan.Step, files *FileService) {
	t.Helper()
	f = newReviewFixture(t)
	if _, err := f.svc.StartReviewPipeline(f.ctx, "proj-1"); err != nil {
		t.Fatalf("StartReviewPipeline: %v", err)
	}
	step = &plan.Step{ID: "step-3", PlanID: "plan-1", ModeID: "refactorer", RunID: "run-4", Status: plan.StepStatusRunning}
	f.store.plans["plan-1"] = &plan.ExecutionPlan{ID: "plan-1", ProjectID: "proj-1", Status: plan.StatusRunning, Steps: []plan.Step{*step}}
	files = NewFileService(&mockStore{projects: []project.Project{{ID: "proj-1", WorkspacePath: f.dir}}})
	files.SetReviewEditRecorder(f.store)
	return f, step, files
}

// A user change made while PrepareStep takes the baseline is in the
// baseline or listed for the decision, never part of the refactoring
// unannounced: the pipeline enters state refactoring - and user edits are
// recorded - before the workspace is snapshot.
func TestReviewPipeline_EditsWhilePreparingAreInTheBaselineOrListed(t *testing.T) {
	f, step, files := preparingFixture(t)
	write := func(name string) {
		if err := files.WriteFile(f.ctx, "proj-1", name, "user\n", "u1"); err != nil {
			t.Errorf("WriteFile(%s): %v", name, err)
		}
	}
	f.store.beforeUpdate = func(from review.PipelineState) {
		if from == review.PipelinePending {
			write("before.go") // the record is still pending
		}
	}
	f.store.beforeBaseline = func() { write("after.go") } // the snapshot is taken, not stored yet

	if err := f.svc.PrepareStep(f.ctx, step); err != nil {
		t.Fatalf("PrepareStep: %v", err)
	}
	writeLines(t, f.dir, "a.go", 300, "rewritten") // the refactoring
	if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s, want waiting for approval", got)
	}
	listed := map[string]bool{}
	for _, e := range approvalRequest(t, f).UserEdits {
		listed[e.Path] = true
	}
	baseline := f.store.pipelines["plan-1"].BaselineSHA
	for _, name := range []string{"before.go", "after.go"} {
		inBaseline := exec.Command("git", "cat-file", "-e", baseline+":"+name) //nolint:gosec // test command on a temp repository
		inBaseline.Dir = f.dir
		if inBaseline.Run() != nil && !listed[name] {
			t.Errorf("%s was changed while the baseline was taken: neither in the baseline nor listed (listed %v)", name, listed)
		}
	}
}

// A baseline that cannot be stored leaves the refactoring without one,
// which fails closed: the gate asks for a decision with the reason, and the
// change can be kept but not undone.
func TestReviewPipeline_UnstoredBaselineFailsClosed(t *testing.T) {
	f, step, _ := preparingFixture(t)
	f.store.baselineErr = errors.New("connection reset")

	if err := f.svc.PrepareStep(f.ctx, step); err == nil {
		t.Fatal("PrepareStep succeeded without storing the baseline")
	}
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineRefactoring || rp.BaselineSHA != "" || rp.StepID != step.ID {
		t.Fatalf("review record = %+v, want refactoring without a baseline", rp)
	}
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("the ref of a baseline that was not stored is kept")
	}

	writeLines(t, f.dir, "a.go", 101, "line") // a small change
	if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
		t.Fatalf("GateStep = %s, want waiting for approval", got)
	}
	if ev := approvalRequest(t, f); ev.Reason == "" {
		t.Fatalf("request = %+v, want the reason", ev)
	}
	step.Status = plan.StepStatusWaitingApproval
	f.store.plans["plan-1"].Steps = []plan.Step{*step}
	if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("Decide(undo) = %v, want a validation error", err)
	}
	if d, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, true); err != nil || d.Status != "approved" {
		t.Fatalf("Decide(keep) = %+v, %v", d, err)
	}
}

// The plan ends while the baseline is taken: the pipeline is done and the
// baseline ref written meanwhile does not stay.
func TestReviewPipeline_PlanEndWhileTheBaselineIsTaken(t *testing.T) {
	f, step, _ := preparingFixture(t)
	f.store.plans["plan-1"] = &plan.ExecutionPlan{ID: "plan-1", ProjectID: "proj-1", Status: plan.StatusCancelled,
		Steps: []plan.Step{{ID: step.ID, PlanID: "plan-1", ModeID: "refactorer", Status: plan.StepStatusCancelled}}}
	f.store.beforeBaseline = func() { f.svc.PlanEnded(f.ctx, "plan-1", string(plan.StatusCancelled)) }

	if err := f.svc.PrepareStep(f.ctx, step); err != nil {
		t.Fatalf("PrepareStep: %v", err)
	}
	if rp := f.store.pipelines["plan-1"]; rp.State != review.PipelineDone || rp.BaselineSHA != "" {
		t.Fatalf("review record = %+v, want the ended pipeline done without a baseline", rp)
	}
	if hasRef(t, f.dir, reviewBaselineRef("plan-1")) {
		t.Fatal("the baseline ref of a pipeline that ended meanwhile dangles")
	}
}

func TestReviewPipeline_PendingDecisionsListTheUserEdits(t *testing.T) {
	f, _ := waitingStep(t)
	many := make([]review.UserEdit, maxListedUserEdits+1)
	for i := range many {
		many[i] = review.UserEdit{Path: fmt.Sprintf("f%03d.go", i), Operation: review.UserEditWrite}
	}
	f.store.userEdits = map[string][]review.UserEdit{"plan-1": many}

	pending, err := f.svc.PendingDecisions(f.ctx, "proj-1")
	if err != nil || len(pending) != 1 {
		t.Fatalf("PendingDecisions = %+v, %v, want one", pending, err)
	}
	if d := pending[0]; len(d.UserEdits) != maxListedUserEdits || d.UserEditsTotal != maxListedUserEdits+1 ||
		d.UserEdits[0].Path != "f000.go" {
		t.Fatalf("pending decision lists %d user edits of %d, want %d of %d",
			len(d.UserEdits), d.UserEditsTotal, maxListedUserEdits, maxListedUserEdits+1)
	}

	// Without its user edits the dialog could offer an undo that sets them
	// back unannounced: the listing fails instead.
	f.store.editsErr = errors.New("connection reset")
	if _, err := f.svc.PendingDecisions(f.ctx, "proj-1"); err == nil {
		t.Fatal("PendingDecisions succeeded without the user edits")
	}
}

func TestReviewPipeline_UserEditsGoWithTheDecision(t *testing.T) {
	for _, tt := range []struct {
		name   string
		decide func(t *testing.T) *reviewFixture
	}{
		{"kept", func(t *testing.T) *reviewFixture {
			f, step := waitingStep(t)
			f.store.userEdits = map[string][]review.UserEdit{"plan-1": planUserEdits}
			if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, true); err != nil {
				t.Fatalf("Decide(keep): %v", err)
			}
			return f
		}},
		{"undone", func(t *testing.T) *reviewFixture {
			f, step := waitingStep(t)
			f.store.userEdits = map[string][]review.UserEdit{"plan-1": planUserEdits}
			if _, err := f.svc.Decide(f.ctx, "run-4", "plan-1", step.ID, false); err != nil {
				t.Fatalf("Decide(undo): %v", err)
			}
			return f
		}},
		{"applied without a decision (low impact)", func(t *testing.T) *reviewFixture {
			f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 102, "line") })
			f.store.userEdits = map[string][]review.UserEdit{"plan-1": planUserEdits}
			if got := gateNow(f, step); got != plan.StepStatusCompleted {
				t.Fatalf("GateStep = %s, want completed", got)
			}
			return f
		}},
		{"plan ended without a change", func(t *testing.T) *reviewFixture {
			f, _ := gateFixture(t, nil, func(string) {})
			f.store.userEdits = map[string][]review.UserEdit{"plan-1": planUserEdits}
			f.svc.PlanEnded(f.ctx, "plan-1", string(plan.StatusCompleted))
			return f
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.decide(t)
			if f.store.pipelines["plan-1"].State != review.PipelineDone {
				t.Fatalf("review state = %s, want done", f.store.pipelines["plan-1"].State)
			}
			if !slices.Equal(f.store.editsDeleted, []string{"plan-1"}) || len(f.store.userEdits["plan-1"]) != 0 {
				t.Fatalf("user edits deleted for %v (left %+v), want plan-1's deleted once", f.store.editsDeleted, f.store.userEdits)
			}
		})
	}

	// A pipeline that moved on elsewhere keeps them: the decision there
	// deletes them.
	t.Run("not finished here", func(t *testing.T) {
		f, _ := waitingStep(t)
		f.store.userEdits = map[string][]review.UserEdit{"plan-1": planUserEdits}
		rp := *f.store.pipelines["plan-1"]
		f.svc.finish(f.ctx, &rp, f.dir, review.PipelineRefactoring) // the record is awaiting_decision
		if len(f.store.editsDeleted) != 0 {
			t.Fatalf("user edits deleted for %v by a finish that lost the race", f.store.editsDeleted)
		}
	})
}
