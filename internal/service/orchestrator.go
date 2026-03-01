package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
)

// debateState tracks a debate sub-plan back to the parent plan step that triggered it.
type debateState struct {
	ParentPlanID string
	ParentStepID string
	ProjectID    string
}

// OrchestratorService manages execution plans — multi-agent DAGs with scheduling protocols.
type OrchestratorService struct {
	store                   database.Store
	hub                     broadcast.Broadcaster
	events                  eventstore.Store
	runtime                 *RuntimeService
	orchCfg                 *config.Orchestrator
	sharedCtx               *SharedContextService
	reviewRouter            *ReviewRouterService
	onPlanCompleteCallbacks []func(ctx context.Context, planID string, status string)
	mu                      sync.Mutex // serializes plan advancement

	// Phase 21D: debate tracking — maps debate planID -> parent step info.
	debateMu       sync.Mutex
	debateSteps    map[string]debateState
	debatedStepIDs map[string]bool // steps that already completed a debate (skip re-evaluation)
}

// AddOnPlanComplete appends a callback invoked when a plan completes or fails.
func (s *OrchestratorService) AddOnPlanComplete(fn func(ctx context.Context, planID string, status string)) {
	s.onPlanCompleteCallbacks = append(s.onPlanCompleteCallbacks, fn)
}

// SetOnPlanComplete registers a callback (backward-compatible alias for AddOnPlanComplete).
func (s *OrchestratorService) SetOnPlanComplete(fn func(ctx context.Context, planID string, status string)) {
	s.AddOnPlanComplete(fn)
}

// SetSharedContext sets the shared context service for auto-populating run outputs.
func (s *OrchestratorService) SetSharedContext(sc *SharedContextService) {
	s.sharedCtx = sc
}

// SetReviewRouter sets the review router service for confidence-based step evaluation.
func (s *OrchestratorService) SetReviewRouter(rr *ReviewRouterService) {
	s.reviewRouter = rr
}

// NewOrchestratorService creates an OrchestratorService with all dependencies.
func NewOrchestratorService(
	store database.Store,
	hub broadcast.Broadcaster,
	events eventstore.Store,
	runtime *RuntimeService,
	orchCfg *config.Orchestrator,
) *OrchestratorService {
	svc := &OrchestratorService{
		store:          store,
		hub:            hub,
		events:         events,
		runtime:        runtime,
		orchCfg:        orchCfg,
		debateSteps:    make(map[string]debateState),
		debatedStepIDs: make(map[string]bool),
	}
	// Self-register debate completion handler so debate sub-plans
	// automatically trigger the parent step dispatch.
	svc.AddOnPlanComplete(svc.handleDebateComplete)
	return svc
}

// CreatePlan validates and persists a new execution plan.
func (s *OrchestratorService) CreatePlan(ctx context.Context, req *plan.CreatePlanRequest) (*plan.ExecutionPlan, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate plan: %w", err)
	}

	maxParallel := req.MaxParallel
	if maxParallel == 0 {
		maxParallel = s.orchCfg.MaxParallel
	}

	p := &plan.ExecutionPlan{
		ProjectID:   req.ProjectID,
		TeamID:      req.TeamID,
		Name:        req.Name,
		Description: req.Description,
		Protocol:    req.Protocol,
		Status:      plan.StatusPending,
		MaxParallel: maxParallel,
	}

	// Build steps with correct initial state
	for _, sr := range req.Steps {
		p.Steps = append(p.Steps, plan.Step{
			TaskID:        sr.TaskID,
			AgentID:       sr.AgentID,
			PolicyProfile: sr.PolicyProfile,
			DeliverMode:   sr.DeliverMode,
			DependsOn:     sr.DependsOn, // indices; DB adapter remaps to UUIDs
			Status:        plan.StepStatusPending,
		})
	}

	if err := s.store.CreatePlan(ctx, p); err != nil {
		return nil, fmt.Errorf("store plan: %w", err)
	}

	s.appendPlanEvent(ctx, event.TypePlanCreated, p)
	s.broadcastPlanStatus(ctx, p)

	slog.Info("plan created", "plan_id", p.ID, "protocol", p.Protocol, "steps", len(p.Steps))
	return p, nil
}

// StartPlan transitions the plan to running and triggers the first scheduling round.
func (s *OrchestratorService) StartPlan(ctx context.Context, planID string) (*plan.ExecutionPlan, error) {
	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	if p.Status != plan.StatusPending {
		return nil, fmt.Errorf("plan %s is %s, expected pending", planID, p.Status)
	}

	if err := s.store.UpdatePlanStatus(ctx, planID, plan.StatusRunning); err != nil {
		return nil, err
	}
	p.Status = plan.StatusRunning

	s.appendPlanEvent(ctx, event.TypePlanStarted, p)
	s.broadcastPlanStatus(ctx, p)

	slog.Info("plan started", "plan_id", p.ID, "protocol", p.Protocol)

	s.advancePlan(ctx, p)
	return p, nil
}

// GetPlan retrieves a plan with its steps.
func (s *OrchestratorService) GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error) {
	return s.store.GetPlan(ctx, id)
}

// ListPlans returns all plans for a project.
func (s *OrchestratorService) ListPlans(ctx context.Context, projectID string) ([]plan.ExecutionPlan, error) {
	return s.store.ListPlansByProject(ctx, projectID)
}

// CancelPlan cancels a running plan: skips pending steps, cancels running runs.
func (s *OrchestratorService) CancelPlan(ctx context.Context, planID string) error {
	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return err
	}
	if p.Status != plan.StatusRunning && p.Status != plan.StatusPending {
		return fmt.Errorf("plan %s is %s, cannot cancel", planID, p.Status)
	}

	for i := range p.Steps {
		switch p.Steps[i].Status {
		case plan.StepStatusPending:
			_ = s.store.UpdatePlanStepStatus(ctx, p.Steps[i].ID, plan.StepStatusSkipped, "", "plan cancelled")
			s.broadcastStepStatus(ctx, p, &p.Steps[i], plan.StepStatusSkipped)
		case plan.StepStatusRunning:
			if p.Steps[i].RunID != "" {
				_ = s.runtime.CancelRun(ctx, p.Steps[i].RunID)
			}
			_ = s.store.UpdatePlanStepStatus(ctx, p.Steps[i].ID, plan.StepStatusCancelled, "", "plan cancelled")
			s.broadcastStepStatus(ctx, p, &p.Steps[i], plan.StepStatusCancelled)
		}
	}

	if err := s.store.UpdatePlanStatus(ctx, planID, plan.StatusCancelled); err != nil {
		return err
	}
	p.Status = plan.StatusCancelled
	s.appendPlanEvent(ctx, event.TypePlanCancelled, p)
	s.broadcastPlanStatus(ctx, p)

	slog.Info("plan cancelled", "plan_id", planID)
	return nil
}

// HandleRunCompleted is the callback invoked by RuntimeService when a run finishes.
// It finds the corresponding plan step and advances the plan.
func (s *OrchestratorService) HandleRunCompleted(ctx context.Context, runID string, status run.Status) {
	step, err := s.store.GetPlanStepByRunID(ctx, runID)
	if err != nil {
		// Run is not part of a plan — normal, ignore silently
		return
	}

	stepStatus := plan.StepStatusCompleted
	errMsg := ""
	switch status {
	case run.StatusFailed, run.StatusTimeout:
		stepStatus = plan.StepStatusFailed
		r, err := s.store.GetRun(ctx, runID)
		if err == nil {
			errMsg = r.Error
		}
	case run.StatusCancelled:
		stepStatus = plan.StepStatusCancelled
	}

	if err := s.store.UpdatePlanStepStatus(ctx, step.ID, stepStatus, "", errMsg); err != nil {
		slog.Error("update plan step status", "step_id", step.ID, "error", err)
		return
	}

	p, err := s.store.GetPlan(ctx, step.PlanID)
	if err != nil {
		slog.Error("get plan for advancement", "plan_id", step.PlanID, "error", err)
		return
	}

	// Auto-populate SharedContext with run output for downstream agents.
	if s.sharedCtx != nil && stepStatus == plan.StepStatusCompleted {
		r, err := s.store.GetRun(ctx, runID)
		if err == nil && r.TeamID != "" && r.Output != "" {
			_, _ = s.sharedCtx.AddItem(ctx, cfcontext.AddSharedItemRequest{
				TeamID: r.TeamID,
				Key:    "step_output:" + step.ID,
				Value:  r.Output,
				Author: r.AgentID,
			})
		}
	}

	s.broadcastStepStatus(ctx, p, step, stepStatus)
	s.advancePlan(ctx, p)
}

// advancePlan is the core scheduling loop. It checks the current state of all steps
// and dispatches to the appropriate protocol handler.
func (s *OrchestratorService) advancePlan(ctx context.Context, p *plan.ExecutionPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reload steps for fresh state
	steps, err := s.store.ListPlanSteps(ctx, p.ID)
	if err != nil {
		slog.Error("reload steps", "plan_id", p.ID, "error", err)
		return
	}
	p.Steps = steps

	// Check if plan is already terminal
	if p.Status != plan.StatusRunning {
		return
	}

	switch p.Protocol {
	case plan.ProtocolSequential:
		s.advanceSequential(ctx, p)
	case plan.ProtocolParallel:
		s.advanceParallel(ctx, p)
	case plan.ProtocolPingPong:
		s.advancePingPong(ctx, p)
	case plan.ProtocolConsensus:
		s.advanceConsensus(ctx, p)
	}
}

// advanceSequential: one step at a time. Failure stops the plan.
func (s *OrchestratorService) advanceSequential(ctx context.Context, p *plan.ExecutionPlan) {
	if plan.AnyFailed(p.Steps) {
		s.failPlan(ctx, p)
		return
	}
	if plan.AllTerminal(p.Steps) {
		s.completePlan(ctx, p)
		return
	}
	if plan.RunningCount(p.Steps) > 0 {
		return // wait for current step
	}

	ready := plan.ReadySteps(p.Steps)
	if len(ready) > 0 {
		s.startStep(ctx, p, ready[0])
	}
}

// advanceParallel: start all ready steps up to MaxParallel.
func (s *OrchestratorService) advanceParallel(ctx context.Context, p *plan.ExecutionPlan) {
	if plan.AllTerminal(p.Steps) {
		if plan.AnyFailed(p.Steps) {
			s.failPlan(ctx, p)
		} else {
			s.completePlan(ctx, p)
		}
		return
	}

	running := plan.RunningCount(p.Steps)
	maxP := p.MaxParallel
	if maxP == 0 {
		maxP = s.orchCfg.MaxParallel
	}

	ready := plan.ReadySteps(p.Steps)
	for _, stepID := range ready {
		if running >= maxP {
			break
		}
		s.startStep(ctx, p, stepID)
		running++
	}
}

// advancePingPong: alternate between 2 steps for PingPongMaxRounds each.
func (s *OrchestratorService) advancePingPong(ctx context.Context, p *plan.ExecutionPlan) {
	if len(p.Steps) != 2 {
		slog.Error("ping_pong requires exactly 2 steps", "plan_id", p.ID)
		s.failPlan(ctx, p)
		return
	}

	maxRounds := s.orchCfg.PingPongMaxRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}

	s0 := &p.Steps[0]
	s1 := &p.Steps[1]

	if s0.Status == plan.StepStatusFailed || s1.Status == plan.StepStatusFailed {
		s.failPlan(ctx, p)
		return
	}

	// Check if both have completed their rounds
	if s0.Round >= maxRounds && s1.Round >= maxRounds &&
		s0.Status.IsTerminal() && s1.Status.IsTerminal() {
		s.completePlan(ctx, p)
		return
	}

	if plan.RunningCount(p.Steps) > 0 {
		return // wait for current step
	}

	// Determine which step goes next: alternate, starting with step 0
	// Step 0 goes on rounds: 1, 3, 5, ... ; Step 1 goes on rounds: 2, 4, 6, ...
	totalCompleted := s0.Round + s1.Round
	var next *plan.Step
	if totalCompleted%2 == 0 {
		next = s0
	} else {
		next = s1
	}

	if next.Round >= maxRounds {
		// This step is done, check the other
		if next == s0 {
			next = s1
		} else {
			next = s0
		}
	}

	if next.Round >= maxRounds {
		// Both at max rounds
		s.completePlan(ctx, p)
		return
	}

	// Reset step to pending for next round
	newRound := next.Round + 1
	if err := s.store.UpdatePlanStepRound(ctx, next.ID, newRound); err != nil {
		slog.Error("update step round", "step_id", next.ID, "error", err)
		return
	}
	if err := s.store.UpdatePlanStepStatus(ctx, next.ID, plan.StepStatusPending, "", ""); err != nil {
		slog.Error("reset step to pending", "step_id", next.ID, "error", err)
		return
	}

	s.startStep(ctx, p, next.ID)
}

// advanceConsensus: launch all steps in parallel, evaluate quorum when all done.
