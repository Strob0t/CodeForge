package service

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// boundaryTriggerRecorder records the boundary analyses AutoIndex starts,
// with the tenant of their context.
type boundaryTriggerRecorder struct {
	tenants chan string
}

func (r *boundaryTriggerRecorder) TriggerBoundaryAnalysis(ctx context.Context, projectID string) (*plan.ExecutionPlan, error) {
	r.tenants <- tenantctx.FromContext(ctx) + "/" + projectID
	return &plan.ExecutionPlan{ID: "plan-1"}, nil
}

// KI-17: indexing a new project starts its boundary analysis (spec step 2),
// not the whole review-refactor pipeline, in the project's tenant.
func TestAutoIndex_StartsTheBoundaryAnalysis(t *testing.T) {
	rec := &boundaryTriggerRecorder{tenants: make(chan string, 1)}
	svc := NewProjectService(&mockStore{}, t.TempDir())
	svc.SetReviewTriggerer(rec)

	const tenant = "aaaaaaaa-0000-0000-0000-00000000000b"
	svc.AutoIndex(tenant, "proj-1", t.TempDir())
	select {
	case got := <-rec.tenants:
		if got != tenant+"/proj-1" {
			t.Fatalf("boundary analysis for %s, want %s/proj-1", got, tenant)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AutoIndex did not start the boundary analysis")
	}
}
