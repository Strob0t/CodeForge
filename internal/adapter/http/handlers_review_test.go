package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/boundary"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-17: the review endpoints start a pipeline or say why not (never
// "triggered" when nothing started), and approve/reject decide the waiting
// refactoring through the review pipeline.

// reviewTriggerStore knows project proj-1 and records nothing as recent.
type reviewTriggerStore struct{}

func (reviewTriggerStore) FindRecentReviewTrigger(context.Context, string, string, time.Duration) (bool, error) {
	return false, nil
}

func (reviewTriggerStore) CreateReviewTrigger(context.Context, string, string, string) (string, error) {
	return "trigger-1", nil
}

func (reviewTriggerStore) GetProject(_ context.Context, id string) (*project.Project, error) {
	if id != "proj-1" {
		return nil, domain.ErrNotFound
	}
	return &project.Project{ID: id}, nil
}

// reviewStarter starts plan-1, or fails with err.
type reviewStarter struct{ err error }

func (s reviewStarter) StartReviewPipeline(context.Context, string) (*plan.ExecutionPlan, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &plan.ExecutionPlan{ID: "plan-1"}, nil
}

func (s reviewStarter) StartBoundaryAnalysis(ctx context.Context, id string) (*plan.ExecutionPlan, error) {
	return s.StartReviewPipeline(ctx, id)
}

func serveReview(h *cfhttp.Handlers, path, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Post("/projects/{id}/review-refactor", h.TriggerReviewRefactor)
	r.Post("/projects/{id}/boundaries/analyze", h.TriggerBoundaryAnalysis)
	r.Post("/runs/{id}/approve", h.ApproveRun)
	r.Post("/runs/{id}/reject", h.RejectRun)
	r.Post("/runs/{id}/approve-partial", h.ApproveRunPartial)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func TestReviewTriggerEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		trigger  *service.ReviewTriggerService
		path     string
		wantCode int
		wantBody string
	}{
		{"review started", service.NewReviewTriggerService(reviewTriggerStore{}, reviewStarter{}, time.Minute),
			"/projects/proj-1/review-refactor", http.StatusAccepted, `"plan_id":"plan-1"`},
		{"boundary analysis started", service.NewReviewTriggerService(reviewTriggerStore{}, reviewStarter{}, time.Minute),
			"/projects/proj-1/boundaries/analyze", http.StatusAccepted, `"triggered":true`},
		{"no agents", service.NewReviewTriggerService(reviewTriggerStore{}, reviewStarter{err: service.ErrReviewNoAgents}, time.Minute),
			"/projects/proj-1/review-refactor", http.StatusBadRequest, "no agents"},
		{"unknown project", service.NewReviewTriggerService(reviewTriggerStore{}, reviewStarter{}, time.Minute),
			"/projects/other/boundaries/analyze", http.StatusNotFound, "project not found"},
		{"no pipeline wired", service.NewReviewTriggerService(reviewTriggerStore{}, nil, time.Minute),
			"/projects/proj-1/review-refactor", http.StatusServiceUnavailable, "not configured"},
		{"no trigger service", nil, "/projects/proj-1/review-refactor", http.StatusServiceUnavailable, "not configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveReview(&cfhttp.Handlers{ReviewTrigger: tt.trigger}, tt.path, "")
			if rec.Code != tt.wantCode || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("POST %s = %d %s, want %d containing %q", tt.path, rec.Code, rec.Body.String(), tt.wantCode, tt.wantBody)
			}
		})
	}
}

// --- approve / reject ---

// decisionStore serves one plan whose refactoring step (run run-4) waits.
type decisionStore struct{ status plan.StepStatus }

func (decisionStore) GetProject(_ context.Context, id string) (*project.Project, error) {
	return &project.Project{ID: id, WorkspacePath: "/nonexistent"}, nil
}
func (decisionStore) ListAgents(context.Context, string) ([]agent.Agent, error) { return nil, nil }
func (decisionStore) CreateTask(context.Context, task.CreateRequest) (*task.Task, error) {
	return nil, domain.ErrValidation
}
func (decisionStore) GetRun(context.Context, string) (*run.Run, error) {
	return nil, domain.ErrNotFound
}
func (s decisionStore) GetPlan(_ context.Context, id string) (*plan.ExecutionPlan, error) {
	if id != "plan-1" {
		return nil, fmt.Errorf("plan %s: %w", id, domain.ErrNotFound)
	}
	return &plan.ExecutionPlan{ID: id, ProjectID: "proj-1", Steps: []plan.Step{
		{ID: "step-1", PlanID: id, ModeID: "refactorer", RunID: "run-4", Status: s.status},
	}}, nil
}
func (decisionStore) GetProjectBoundaries(context.Context, string) (*boundary.ProjectBoundaryConfig, error) {
	return nil, domain.ErrNotFound
}
func (decisionStore) UpsertProjectBoundaries(context.Context, *boundary.ProjectBoundaryConfig) error {
	return nil
}
func (decisionStore) CreateReviewPipeline(context.Context, *review.Pipeline) error { return nil }
func (decisionStore) GetReviewPipeline(_ context.Context, planID string) (*review.Pipeline, error) {
	if planID != "plan-1" {
		return nil, domain.ErrNotFound
	}
	return &review.Pipeline{PlanID: planID, ProjectID: "proj-1"}, nil // no baseline recorded
}

type decisionPlanner struct{ approved, rejected []string }

func (*decisionPlanner) CreatePlan(context.Context, *plan.CreatePlanRequest) (*plan.ExecutionPlan, error) {
	return nil, domain.ErrValidation
}
func (*decisionPlanner) StartPlan(context.Context, string) (*plan.ExecutionPlan, error) {
	return nil, domain.ErrValidation
}
func (*decisionPlanner) CancelPlan(context.Context, string) error { return nil }
func (p *decisionPlanner) ApproveStep(_ context.Context, _, stepID string) error {
	p.approved = append(p.approved, stepID)
	return nil
}
func (p *decisionPlanner) RejectStep(_ context.Context, _, stepID string) error {
	p.rejected = append(p.rejected, stepID)
	return nil
}

type noTeams struct{}

func (noTeams) CreateTeam(context.Context, *agent.CreateTeamRequest) (*agent.Team, error) {
	return nil, domain.ErrValidation
}

type noEvents struct{}

func (noEvents) BroadcastEvent(context.Context, string, any) {}

func TestReviewDecisionEndpoints(t *testing.T) {
	newHandlers := func(status plan.StepStatus) (*cfhttp.Handlers, *decisionPlanner) {
		planner := &decisionPlanner{}
		svc := service.NewReviewPipelineService(decisionStore{status: status}, service.NewPipelineService(service.NewModeService()),
			planner, noTeams{}, git.NewPool(1), noEvents{}, service.DefaultDiffImpactConfig())
		return &cfhttp.Handlers{ReviewPipeline: svc}, planner
	}
	step := `{"plan_id":"plan-1","step_id":"step-1"}`

	t.Run("approve", func(t *testing.T) {
		h, planner := newHandlers(plan.StepStatusWaitingApproval)
		rec := serveReview(h, "/runs/run-4/approve", step)
		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusOK || body["status"] != "approved" || len(planner.approved) != 1 {
			t.Fatalf("approve = %d %s (approved %v), want 200 approved", rec.Code, rec.Body.String(), planner.approved)
		}
	})
	t.Run("reject that cannot undo the refactoring", func(t *testing.T) {
		h, planner := newHandlers(plan.StepStatusWaitingApproval)
		rec := serveReview(h, "/runs/run-4/reject", step)
		if rec.Code == http.StatusOK || len(planner.rejected) != 0 {
			t.Fatalf("reject = %d %s (rejected %v), want an error and the step kept waiting", rec.Code, rec.Body.String(), planner.rejected)
		}
	})
	for _, tt := range []struct {
		name, path, body string
		status           plan.StepStatus
		wantCode         int
	}{
		{"another run", "/runs/run-9/approve", step, plan.StepStatusWaitingApproval, http.StatusNotFound},
		{"unknown plan", "/runs/run-4/approve", `{"plan_id":"plan-9","step_id":"step-1"}`, plan.StepStatusWaitingApproval, http.StatusNotFound},
		{"step not waiting", "/runs/run-4/reject", step, plan.StepStatusCompleted, http.StatusBadRequest},
		{"missing step", "/runs/run-4/approve", `{"plan_id":"plan-1"}`, plan.StepStatusWaitingApproval, http.StatusBadRequest},
		{"partial approval", "/runs/run-4/approve-partial", step, plan.StepStatusWaitingApproval, http.StatusNotImplemented},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, planner := newHandlers(tt.status)
			rec := serveReview(h, tt.path, tt.body)
			if rec.Code != tt.wantCode || len(planner.approved)+len(planner.rejected) != 0 {
				t.Fatalf("POST %s = %d %s (decided %v %v), want %d and no decision",
					tt.path, rec.Code, rec.Body.String(), planner.approved, planner.rejected, tt.wantCode)
			}
		})
	}
	t.Run("no pipeline wired", func(t *testing.T) {
		if rec := serveReview(&cfhttp.Handlers{}, "/runs/run-4/approve", step); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("approve = %d, want 503", rec.Code)
		}
	})
}
