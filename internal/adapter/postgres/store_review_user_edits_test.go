package postgres_test

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// KI-94: editor and file-API changes made while a review pipeline's
// refactoring runs (state refactoring) are recorded per pipeline, one entry
// per path, in the tenant of the request; the approval dialog lists them.

// refactoringPipeline records a review pipeline of a new plan of f's project
// and moves it to state.
func (f *statusFixture) refactoringPipeline(t *testing.T, state review.PipelineState) *review.Pipeline {
	t.Helper()
	p := f.reviewPlan(t)
	rp := &review.Pipeline{PlanID: p.ID, ProjectID: f.project.ID}
	if err := f.store.CreateReviewPipeline(f.ctx, rp); err != nil {
		t.Fatalf("CreateReviewPipeline: %v", err)
	}
	if state != review.PipelinePending {
		rp.State, rp.StepID = state, p.Steps[0].ID
		if err := f.store.UpdateReviewPipeline(f.ctx, rp, review.PipelinePending); err != nil {
			t.Fatalf("UpdateReviewPipeline to %s: %v", state, err)
		}
	}
	return rp
}

func (f *statusFixture) userEdits(t *testing.T, planID string, limit int) (edits []review.UserEdit, total int) {
	t.Helper()
	edits, total, err := f.store.ListReviewUserEdits(f.ctx, planID, limit)
	if err != nil {
		t.Fatalf("ListReviewUserEdits: %v", err)
	}
	return edits, total
}

func TestStore_ReviewUserEdits(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	editor := createChannelTestUser(t, a.store, middleware.TenantIDFromContext(a.ctx))
	record := func(f *statusFixture, userID string, op review.UserEditOp, paths ...string) error {
		return f.store.RecordReviewUserEdits(f.ctx, a.project.ID, userID, op, paths)
	}

	// Nothing refactors yet: nothing is recorded.
	pending := a.refactoringPipeline(t, review.PipelinePending)
	if err := record(a, editor, review.UserEditWrite, "a.go"); err != nil {
		t.Fatalf("RecordReviewUserEdits while pending: %v", err)
	}
	if edits, total := a.userEdits(t, pending.PlanID, 100); len(edits) != 0 || total != 0 {
		t.Fatalf("edits while pending = %+v (%d), want none", edits, total)
	}
	pending.State = review.PipelineDone
	if err := a.store.UpdateReviewPipeline(a.ctx, pending, review.PipelinePending); err != nil {
		t.Fatalf("UpdateReviewPipeline: %v", err)
	}
	if err := a.store.UpdatePlanStatus(a.ctx, pending.PlanID, plan.StatusCancelled); err != nil {
		t.Fatalf("UpdatePlanStatus: %v", err)
	}

	rp := a.refactoringPipeline(t, review.PipelineRefactoring)
	if err := record(a, editor, review.UserEditWrite, "a.go"); err != nil {
		t.Fatalf("RecordReviewUserEdits: %v", err)
	}
	if err := record(a, user.AuthDisabledUserID, review.UserEditRename, "old.go", "new.go"); err != nil {
		t.Fatalf("RecordReviewUserEdits by an identity without an account: %v", err)
	}
	if err := record(a, editor, review.UserEditDelete, "a.go"); err != nil { // the same path again
		t.Fatalf("RecordReviewUserEdits again: %v", err)
	}
	if err := record(a, editor, review.UserEditWrite); err != nil {
		t.Fatalf("RecordReviewUserEdits without paths: %v", err)
	}
	// Another tenant cannot record for the project, nor read or delete what
	// was recorded.
	if err := record(b, editor, review.UserEditWrite, "b.go"); err != nil {
		t.Fatalf("RecordReviewUserEdits from another tenant: %v", err)
	}
	if edits, total, err := b.store.ListReviewUserEdits(b.ctx, rp.PlanID, 100); err != nil || len(edits) != 0 || total != 0 {
		t.Fatalf("other tenant lists %+v (%d), %v, want nothing", edits, total, err)
	}
	if err := b.store.DeleteReviewUserEdits(b.ctx, rp.PlanID); err != nil {
		t.Fatalf("DeleteReviewUserEdits from another tenant: %v", err)
	}

	edits, total := a.userEdits(t, rp.PlanID, 100)
	if total != 3 || len(edits) != 3 {
		t.Fatalf("edits = %+v (%d), want a.go, new.go and old.go once each", edits, total)
	}
	if e := edits[0]; e.Path != "a.go" || e.Operation != review.UserEditDelete || e.UserID != editor ||
		e.UserName != "Channel Test User" || e.EditedAt.IsZero() {
		t.Fatalf("a.go = %+v, want its latest change (delete) by the editor", e)
	}
	for _, e := range edits[1:] {
		if e.Operation != review.UserEditRename || e.UserID != "" || e.UserName != "" {
			t.Fatalf("%s = %+v, want a rename without an account", e.Path, e)
		}
	}
	if edits[1].Path != "new.go" || edits[2].Path != "old.go" {
		t.Fatalf("edits = %+v, want them by path", edits)
	}
	if capped, total := a.userEdits(t, rp.PlanID, 2); len(capped) != 2 || total != 3 || capped[0].Path != "a.go" {
		t.Fatalf("capped edits = %+v (%d), want the first two of three", capped, total)
	}

	// A deleted account leaves the edit without its user; a request whose
	// account is gone records nothing.
	if err := a.store.DeleteUser(a.ctx, editor); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if edits, _ := a.userEdits(t, rp.PlanID, 100); edits[0].UserID != "" || edits[0].UserName != "" {
		t.Fatalf("a.go after the account was deleted = %+v, want no user", edits[0])
	}
	if err := record(a, editor, review.UserEditWrite, "c.go"); !errors.Is(err, user.ErrAccountGone) {
		t.Fatalf("RecordReviewUserEdits by a deleted account = %v, want ErrAccountGone", err)
	}

	// Once the change is measured, later edits are not the refactoring's.
	rp.State = review.PipelineAwaitingDecision
	if err := a.store.UpdateReviewPipeline(a.ctx, rp, review.PipelineRefactoring); err != nil {
		t.Fatalf("UpdateReviewPipeline: %v", err)
	}
	if err := record(a, user.AuthDisabledUserID, review.UserEditWrite, "late.go"); err != nil {
		t.Fatalf("RecordReviewUserEdits after the measurement: %v", err)
	}
	if _, total := a.userEdits(t, rp.PlanID, 100); total != 3 {
		t.Fatalf("edits after the measurement = %d, want still 3", total)
	}

	// The records go with the decision.
	if err := a.store.DeleteReviewUserEdits(a.ctx, rp.PlanID); err != nil {
		t.Fatalf("DeleteReviewUserEdits: %v", err)
	}
	if edits, total := a.userEdits(t, rp.PlanID, 100); len(edits) != 0 || total != 0 {
		t.Fatalf("edits after the delete = %+v (%d), want none", edits, total)
	}
}
