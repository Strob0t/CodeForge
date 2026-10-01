package postgres_test

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/review"
)

// KI-17: the review pipeline's record of its plans and commits, the source
// the threshold HITL trusts.

// reviewPlan creates a pending plan of the fixture's project.
func (f *statusFixture) reviewPlan(t *testing.T) *plan.ExecutionPlan {
	t.Helper()
	p := &plan.ExecutionPlan{
		ProjectID: f.project.ID, Name: "review", Protocol: plan.ProtocolSequential, Status: plan.StatusPending, MaxParallel: 1,
		Steps: []plan.Step{{TaskID: f.task.ID, AgentID: f.agent.ID, ModeID: "refactorer", Status: plan.StepStatusPending}},
	}
	if err := f.store.CreatePlan(f.ctx, p); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	return p
}

func TestStore_ReviewPipeline(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	p := a.reviewPlan(t)
	const (
		baseline = "0123456789abcdef0123456789abcdef01234567"
		result   = "89abcdef0123456789abcdef0123456789abcdef"
	)

	rp := &review.Pipeline{PlanID: p.ID, ProjectID: a.project.ID}
	if err := a.store.CreateReviewPipeline(a.ctx, rp); err != nil {
		t.Fatalf("CreateReviewPipeline: %v", err)
	}
	got, err := a.store.GetReviewPipeline(a.ctx, p.ID)
	if err != nil {
		t.Fatalf("GetReviewPipeline: %v", err)
	}
	if got.State != review.PipelinePending || got.BaselineSHA != "" || got.Impact != nil ||
		got.ProjectID != a.project.ID || got.TenantID == "" || got.CreatedAt.IsZero() {
		t.Fatalf("review pipeline = %+v, want a pending record", got)
	}

	// pending -> refactoring -> awaiting_decision, each a compare-and-swap.
	got.State, got.BaselineSHA, got.StepID = review.PipelineRefactoring, baseline, p.Steps[0].ID
	if err := a.store.UpdateReviewPipeline(a.ctx, got, review.PipelinePending); err != nil {
		t.Fatalf("UpdateReviewPipeline to refactoring: %v", err)
	}
	if err := a.store.UpdateReviewPipeline(a.ctx, got, review.PipelinePending); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second update from pending: %v, want a conflict", err)
	}
	got.State, got.ResultSHA, got.RunID = review.PipelineAwaitingDecision, result, "run-1"
	got.Impact = &review.Impact{Level: "high", FilesChanged: 2, LinesAdded: 10, Structural: true, Reason: "why"}
	if err := a.store.UpdateReviewPipeline(a.ctx, got, review.PipelineRefactoring); err != nil {
		t.Fatalf("UpdateReviewPipeline to awaiting_decision: %v", err)
	}
	stored, err := a.store.GetReviewPipeline(a.ctx, p.ID)
	if err != nil {
		t.Fatalf("GetReviewPipeline: %v", err)
	}
	if stored.State != review.PipelineAwaitingDecision || stored.BaselineSHA != baseline || stored.ResultSHA != result ||
		stored.StepID != p.Steps[0].ID || stored.RunID != "run-1" || stored.Impact == nil || *stored.Impact != *got.Impact {
		t.Fatalf("stored = %+v (impact %+v), want the awaited decision", stored, stored.Impact)
	}

	// Tenant isolation.
	if _, err := b.store.GetReviewPipeline(b.ctx, p.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant reads the record: %v, want not found", err)
	}
	stored.State = review.PipelineDone
	if err := b.store.UpdateReviewPipeline(b.ctx, stored, review.PipelineAwaitingDecision); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant updates the record: %v, want not found", err)
	}
	if _, err := a.store.GetReviewPipeline(a.ctx, a.reviewPlan(t).ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("plan without a record: %v, want not found", err)
	}
}
