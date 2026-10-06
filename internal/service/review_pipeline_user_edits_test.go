package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
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
	// The request is not lost with the edits: the dialog loads them again
	// with the pending decisions.
	t.Run("the edits cannot be read", func(t *testing.T) {
		f, step := gateFixture(t, nil, func(dir string) { writeLines(t, dir, "a.go", 300, "rewritten") })
		f.store.editsErr = errors.New("connection reset")
		if got := gateNow(f, step); got != plan.StepStatusWaitingApproval {
			t.Fatalf("GateStep = %s, want waiting for approval", got)
		}
		if ev := approvalRequest(t, f); ev.UserEdits != nil || ev.RunID != "run-4" {
			t.Fatalf("request = %+v, want it announced without user edits", ev)
		}
	})
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
