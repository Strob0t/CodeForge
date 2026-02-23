package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
)

// ReviewService manages review policies and triggers automated reviews.
type ReviewService struct {
	store        database.Store
	pipelineSvc  *PipelineService
	orchestrator *OrchestratorService
	hub          broadcast.Broadcaster
	events       eventstore.Store
	stopCron     chan struct{}
	cronOnce     sync.Once
}

// NewReviewService creates a ReviewService with all dependencies.
func NewReviewService(
	store database.Store,
	pipelineSvc *PipelineService,
	orchestrator *OrchestratorService,
	hub broadcast.Broadcaster,
	events eventstore.Store,
) *ReviewService {
	return &ReviewService{
		store:        store,
		pipelineSvc:  pipelineSvc,
		orchestrator: orchestrator,
		hub:          hub,
		events:       events,
		stopCron:     make(chan struct{}),
	}
}

// --- CRUD ---

// CreatePolicy creates a new review policy for a project.
func (s *ReviewService) CreatePolicy(ctx context.Context, projectID, tenantID string, req *review.CreatePolicyRequest) (*review.ReviewPolicy, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate policy: %w", err)
	}

	now := time.Now().UTC()
	p := &review.ReviewPolicy{
		ID:              generateID(),
		ProjectID:       projectID,
		TenantID:        tenantID,
		Name:            req.Name,
		TriggerType:     req.TriggerType,
		CommitThreshold: req.CommitThreshold,
		CronExpr:        req.CronExpr,
		BranchPattern:   req.BranchPattern,
		TemplateID:      req.TemplateID,
		Enabled:         req.Enabled == nil || *req.Enabled,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if p.TemplateID == "" {
		p.TemplateID = "review-only"
	}

	if err := s.store.CreateReviewPolicy(ctx, p); err != nil {
		return nil, fmt.Errorf("create review policy: %w", err)
	}
	return p, nil
}

// GetPolicy retrieves a review policy by ID.
func (s *ReviewService) GetPolicy(ctx context.Context, id string) (*review.ReviewPolicy, error) {
	return s.store.GetReviewPolicy(ctx, id)
}

// ListPolicies returns all review policies for a project.
func (s *ReviewService) ListPolicies(ctx context.Context, projectID string) ([]review.ReviewPolicy, error) {
	return s.store.ListReviewPoliciesByProject(ctx, projectID)
}

// UpdatePolicy updates an existing review policy.
func (s *ReviewService) UpdatePolicy(ctx context.Context, id string, req review.UpdatePolicyRequest) (*review.ReviewPolicy, error) {
	p, err := s.store.GetReviewPolicy(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get policy for update: %w", err)
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.TriggerType != nil {
		p.TriggerType = *req.TriggerType
	}
	if req.CommitThreshold != nil {
		p.CommitThreshold = *req.CommitThreshold
	}
	if req.CronExpr != nil {
		p.CronExpr = *req.CronExpr
	}
	if req.BranchPattern != nil {
		p.BranchPattern = *req.BranchPattern
	}
	if req.TemplateID != nil {
		p.TemplateID = *req.TemplateID
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}

	// Re-validate after applying updates.
	validate := review.CreatePolicyRequest{
		Name:            p.Name,
		TriggerType:     p.TriggerType,
		CommitThreshold: p.CommitThreshold,
		CronExpr:        p.CronExpr,
		BranchPattern:   p.BranchPattern,
		TemplateID:      p.TemplateID,
		Enabled:         &p.Enabled,
	}
	if err := validate.Validate(); err != nil {
		return nil, fmt.Errorf("validate updated policy: %w", err)
	}

	p.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateReviewPolicy(ctx, p); err != nil {
		return nil, fmt.Errorf("update review policy: %w", err)
	}
	return p, nil
}

// DeletePolicy deletes a review policy.
func (s *ReviewService) DeletePolicy(ctx context.Context, id string) error {
	return s.store.DeleteReviewPolicy(ctx, id)
}

// --- Trigger Logic ---

// HandlePush processes a push event, incrementing commit counters and triggering reviews when thresholds are met.
func (s *ReviewService) HandlePush(ctx context.Context, projectID, branch string, commitCount int) error {
	policies, err := s.store.ListReviewPoliciesByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list policies for push: %w", err)
	}

	for i := range policies {
		p := &policies[i]
		if !p.Enabled || p.TriggerType != review.TriggerCommitCount {
			continue
		}

		newCount, err := s.store.IncrementCommitCounter(ctx, p.ID, commitCount)
		if err != nil {
			slog.Error("increment commit counter", "policy_id", p.ID, "error", err)
			continue
		}

		if newCount >= p.CommitThreshold {
			if err := s.store.ResetCommitCounter(ctx, p.ID); err != nil {
				slog.Error("reset commit counter", "policy_id", p.ID, "error", err)
				continue
			}
			if _, err := s.triggerReview(ctx, p, branch); err != nil {
				slog.Error("trigger review from push", "policy_id", p.ID, "error", err)
			}
		}
	}
	return nil
}

// HandlePreMerge checks pre-merge policies and triggers a review for the first matching policy.
func (s *ReviewService) HandlePreMerge(ctx context.Context, projectID, targetBranch string) (*review.Review, error) {
	policies, err := s.store.ListReviewPoliciesByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list policies for pre-merge: %w", err)
	}

	for i := range policies {
		p := &policies[i]
		if !p.Enabled || p.TriggerType != review.TriggerPreMerge {
			continue
		}
		if !p.MatchesBranch(targetBranch) {
			continue
		}
		return s.triggerReview(ctx, p, targetBranch)
	}
	return nil, nil
}

// HandlePlanComplete updates a review's status when its linked plan completes or fails.
func (s *ReviewService) HandlePlanComplete(ctx context.Context, planID, status string) {
	r, err := s.store.GetReviewByPlanID(ctx, planID)
	if err != nil {
		// Not every plan is linked to a review — this is expected.
		return
	}

	var newStatus review.Status
	switch status {
	case "completed":
		newStatus = review.StatusCompleted
	default:
		newStatus = review.StatusFailed
	}

	now := time.Now().UTC()
	if err := s.store.UpdateReviewStatus(ctx, r.ID, newStatus, &now); err != nil {
		slog.Error("update review status from plan", "review_id", r.ID, "error", err)
		return
	}

	s.hub.BroadcastEvent(ctx, ws.EventReviewStatus, ws.ReviewStatusEvent{
		ReviewID:  r.ID,
		PolicyID:  r.PolicyID,
		ProjectID: r.ProjectID,
		Status:    string(newStatus),
		PlanID:    r.PlanID,
	})

	evType := event.TypeReviewCompleted
	if newStatus == review.StatusFailed {
		evType = event.TypeReviewFailed
	}
	s.appendEvent(ctx, evType, r)
}

// ManualTrigger triggers a review manually for a given policy.
func (s *ReviewService) ManualTrigger(ctx context.Context, policyID string) (*review.Review, error) {
	p, err := s.store.GetReviewPolicy(ctx, policyID)
	if err != nil {
		return nil, fmt.Errorf("get policy for manual trigger: %w", err)
	}
	return s.triggerReview(ctx, p, "manual")
}

// triggerReview creates a review record, instantiates the pipeline, and starts the plan.
func (s *ReviewService) triggerReview(ctx context.Context, policy *review.ReviewPolicy, triggerRef string) (*review.Review, error) {
	now := time.Now().UTC()
	r := &review.Review{
		ID:         generateID(),
		PolicyID:   policy.ID,
		ProjectID:  policy.ProjectID,
		TenantID:   policy.TenantID,
		Status:     review.StatusPending,
		TriggerRef: triggerRef,
		CreatedAt:  now,
	}

	if err := s.store.CreateReview(ctx, r); err != nil {
		return nil, fmt.Errorf("create review: %w", err)
	}

	// Instantiate the pipeline template to get a plan request.
	// The review-only template has 2 steps (reviewer + security).
	// We create placeholder bindings — the orchestrator will handle task creation.
	tmpl, err := s.pipelineSvc.Get(policy.TemplateID)
	if err != nil {
		return r, fmt.Errorf("get pipeline template %q: %w", policy.TemplateID, err)
	}

	bindings := make([]pipeline.StepBinding, len(tmpl.Steps))
	for i := range tmpl.Steps {
		bindings[i] = pipeline.StepBinding{
			TaskID:  generateID(),
			AgentID: generateID(),
		}
	}

	planReq, err := s.pipelineSvc.Instantiate(ctx, policy.TemplateID, pipeline.InstantiateRequest{
		ProjectID: policy.ProjectID,
		PlanName:  fmt.Sprintf("review-%s-%s", policy.Name, r.ID[:8]),
		Bindings:  bindings,
	})
	if err != nil {
		return r, fmt.Errorf("instantiate review pipeline: %w", err)
	}

	p, err := s.orchestrator.CreatePlan(ctx, planReq)
	if err != nil {
		return r, fmt.Errorf("create review plan: %w", err)
	}

	r.PlanID = p.ID
	r.Status = review.StatusRunning
	if err := s.store.UpdateReviewStatus(ctx, r.ID, review.StatusRunning, nil); err != nil {
		slog.Error("update review status to running", "review_id", r.ID, "error", err)
	}

	if _, err := s.orchestrator.StartPlan(ctx, p.ID); err != nil {
		slog.Error("start review plan", "review_id", r.ID, "plan_id", p.ID, "error", err)
	}

	s.hub.BroadcastEvent(ctx, ws.EventReviewStatus, ws.ReviewStatusEvent{
		ReviewID:  r.ID,
		PolicyID:  policy.ID,
		ProjectID: policy.ProjectID,
		Status:    string(r.Status),
		PlanID:    r.PlanID,
	})

	s.appendEvent(ctx, event.TypeReviewTriggered, r)

	slog.Info("review triggered", "review_id", r.ID, "policy_id", policy.ID, "trigger", triggerRef)
	return r, nil
}

// --- Cron Scheduler ---

// StartCron launches a background ticker that checks cron-based review policies every 60 seconds.
func (s *ReviewService) StartCron(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.runCronCheck(ctx)
			case <-s.stopCron:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	slog.Info("review cron scheduler started")
}

// StopCron stops the cron scheduler.
func (s *ReviewService) StopCron() {
	s.cronOnce.Do(func() {
		close(s.stopCron)
	})
}

func (s *ReviewService) runCronCheck(ctx context.Context) {
	policies, err := s.store.ListEnabledPoliciesByTrigger(ctx, review.TriggerCron)
	if err != nil {
		slog.Error("list cron policies", "error", err)
		return
	}

	now := time.Now().UTC()
	for i := range policies {
		p := &policies[i]
		sched, err := review.ParseCronExpr(p.CronExpr)
		if err != nil {
			slog.Warn("invalid cron expr on policy", "policy_id", p.ID, "expr", p.CronExpr)
			continue
		}

		// Check if the next scheduled time is within the last 60-second window.
		next := sched.NextAfter(p.UpdatedAt)
		if !next.After(now) {
			// Time has arrived — trigger and update the timestamp.
			if _, err := s.triggerReview(ctx, p, "cron"); err != nil {
				slog.Error("trigger cron review", "policy_id", p.ID, "error", err)
				continue
			}
			// Touch updatedAt so we don't re-trigger.
			p.UpdatedAt = now
			_ = s.store.UpdateReviewPolicy(ctx, p)
		}
	}
}

// --- Helpers ---

func (s *ReviewService) appendEvent(ctx context.Context, evType event.Type, r *review.Review) {
	if s.events == nil {
		return
	}
	_ = s.events.Append(ctx, &event.AgentEvent{
		ID:        generateID(),
		TaskID:    r.ID,
		ProjectID: r.ProjectID,
		Type:      evType,
		CreatedAt: time.Now().UTC(),
	})
}

// GetReview retrieves a review by ID.
func (s *ReviewService) GetReview(ctx context.Context, id string) (*review.Review, error) {
	return s.store.GetReview(ctx, id)
}

// ListReviews returns all reviews for a project.
func (s *ReviewService) ListReviews(ctx context.Context, projectID string) ([]review.Review, error) {
	return s.store.ListReviewsByProject(ctx, projectID)
}
