package postgres_test

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/review"
)

// KI-17: the review pipeline's record of its plans and baselines, the source
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
	const sha = "0123456789abcdef0123456789abcdef01234567"

	rp := &review.Pipeline{PlanID: p.ID, ProjectID: a.project.ID, BaselineSHA: sha}
	if err := a.store.CreateReviewPipeline(a.ctx, rp); err != nil {
		t.Fatalf("CreateReviewPipeline: %v", err)
	}
	got, err := a.store.GetReviewPipeline(a.ctx, p.ID)
	if err != nil {
		t.Fatalf("GetReviewPipeline: %v", err)
	}
	if got.BaselineSHA != sha || got.ProjectID != a.project.ID || got.TenantID == "" || got.CreatedAt.IsZero() {
		t.Fatalf("review pipeline = %+v, want the recorded one", got)
	}

	if _, err := b.store.GetReviewPipeline(b.ctx, p.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant reads the record: %v, want not found", err)
	}
	if _, err := a.store.GetReviewPipeline(a.ctx, a.reviewPlan(t).ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("plan without a record: %v, want not found", err)
	}
}
