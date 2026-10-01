package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/run"
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

	// The decision is listed as pending for the project, in its tenant only.
	if pending, err := a.store.ListPendingReviewDecisions(a.ctx, a.project.ID); err != nil || len(pending) != 1 ||
		pending[0].PlanID != p.ID || pending[0].Impact == nil {
		t.Fatalf("ListPendingReviewDecisions = %+v, %v, want the plan's decision", pending, err)
	}
	if pending, err := b.store.ListPendingReviewDecisions(b.ctx, a.project.ID); err != nil || len(pending) != 0 {
		t.Fatalf("other tenant lists %+v, %v, want nothing", pending, err)
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

// S6-F 7: one active review pipeline per project, and no agent shared with
// another plan that has not ended.
func TestStore_ReviewPipelineGuard(t *testing.T) {
	f := newStatusFixture(t)
	first := f.reviewPlan(t)
	if err := f.store.CreateReviewPipeline(f.ctx, &review.Pipeline{PlanID: first.ID, ProjectID: f.project.ID}); err != nil {
		t.Fatalf("CreateReviewPipeline: %v", err)
	}
	if active, err := f.store.HasActiveReviewPipeline(f.ctx, f.project.ID); err != nil || !active {
		t.Fatalf("HasActiveReviewPipeline = %v, %v, want true", active, err)
	}

	second := f.reviewPlan(t)
	err := f.store.CreateReviewPipeline(f.ctx, &review.Pipeline{PlanID: second.ID, ProjectID: f.project.ID})
	if !errors.Is(err, domain.ErrConflict) || !errors.Is(err, review.ErrPipelineActive) {
		t.Fatalf("second pipeline: %v, want ErrPipelineActive", err)
	}

	// The first plan ends: its pipeline is no longer active, but the second
	// plan's agent is also the agent of a third plan that has not ended.
	if err := f.store.UpdatePlanStatus(f.ctx, first.ID, plan.StatusCompleted); err != nil {
		t.Fatalf("UpdatePlanStatus: %v", err)
	}
	if active, err := f.store.HasActiveReviewPipeline(f.ctx, f.project.ID); err != nil || active {
		t.Fatalf("HasActiveReviewPipeline after the plan ended = %v, %v, want false", active, err)
	}
	third := f.reviewPlan(t) // a plain plan of the same agent, pending
	err = f.store.CreateReviewPipeline(f.ctx, &review.Pipeline{PlanID: second.ID, ProjectID: f.project.ID})
	if !errors.Is(err, domain.ErrConflict) || !errors.Is(err, review.ErrAgentInUse) {
		t.Fatalf("pipeline sharing an agent: %v, want ErrAgentInUse", err)
	}
	if err := f.store.UpdatePlanStatus(f.ctx, third.ID, plan.StatusCancelled); err != nil {
		t.Fatalf("UpdatePlanStatus: %v", err)
	}
	if err := f.store.CreateReviewPipeline(f.ctx, &review.Pipeline{PlanID: second.ID, ProjectID: f.project.ID}); err != nil {
		t.Fatalf("CreateReviewPipeline once the others ended: %v", err)
	}

	// A refactoring that waits for a decision keeps the pipeline active after
	// its plan ended.
	rp, err := f.store.GetReviewPipeline(f.ctx, second.ID)
	if err != nil {
		t.Fatalf("GetReviewPipeline: %v", err)
	}
	rp.State = review.PipelineAwaitingDecision
	if err := f.store.UpdateReviewPipeline(f.ctx, rp, review.PipelinePending); err != nil {
		t.Fatalf("UpdateReviewPipeline: %v", err)
	}
	if err := f.store.UpdatePlanStatus(f.ctx, second.ID, plan.StatusFailed); err != nil {
		t.Fatalf("UpdatePlanStatus: %v", err)
	}
	if active, err := f.store.HasActiveReviewPipeline(f.ctx, f.project.ID); err != nil || !active {
		t.Fatalf("HasActiveReviewPipeline with a pending decision = %v, %v, want true", active, err)
	}
}

// Review finding 7: retention purges an old refactoring run and clears the
// plan step's reference to it; the review record keeps the run, so the
// pending decision is still listed (and decidable by its record).
func TestRetention_PendingReviewDecisionSurvivesTheRunPurge(t *testing.T) {
	pool := retentionPool(t)
	f := newStatusFixture(t)
	p := f.reviewPlan(t)
	r := f.newRun(t, run.StatusCompleted)
	if err := f.store.UpdatePlanStepStatus(f.ctx, p.Steps[0].ID, plan.StepStatusWaitingApproval, r.ID, ""); err != nil {
		t.Fatalf("UpdatePlanStepStatus: %v", err)
	}
	if err := f.store.CreateReviewPipeline(f.ctx, &review.Pipeline{
		PlanID: p.ID, ProjectID: f.project.ID, State: review.PipelineAwaitingDecision, StepID: p.Steps[0].ID, RunID: r.ID,
		Impact: &review.Impact{Level: "high"},
	}); err != nil {
		t.Fatalf("CreateReviewPipeline: %v", err)
	}
	tag, err := pool.Exec(context.Background(), `UPDATE runs SET updated_at = $2 WHERE id = $1`, r.ID, retentionCutoff.Add(-time.Hour))
	backdated(t, tag, err)

	purgeAll(t, f.store, "runs", purgeRuns)

	got, err := f.store.GetPlan(f.ctx, p.ID)
	if err != nil || got.Steps[0].RunID != "" {
		t.Fatalf("plan step after the purge = %+v, %v, want its run reference cleared", got, err)
	}
	pending, err := f.store.ListPendingReviewDecisions(f.ctx, f.project.ID)
	if err != nil || len(pending) != 1 || pending[0].RunID != r.ID || pending[0].StepID != p.Steps[0].ID {
		t.Fatalf("pending decisions after the purge = %+v, %v, want the decision with its run", pending, err)
	}
}
