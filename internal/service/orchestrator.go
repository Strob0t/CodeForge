package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
)

// OrchestratorService manages execution plans — multi-agent DAGs with scheduling protocols.
type OrchestratorService struct {
	store          database.Store
	hub            broadcast.Broadcaster
	events         eventstore.Store
	runtime        *RuntimeService
	orchCfg        *config.Orchestrator
	sharedCtx      *SharedContextService
	onPlanComplete func(ctx context.Context, planID string, status string)
	mu             sync.Mutex // serializes plan advancement
}

// SetOnPlanComplete registers a callback invoked when a plan completes or fails.
func (s *OrchestratorService) SetOnPlanComplete(fn func(ctx context.Context, planID string, status string)) {
	s.onPlanComplete = fn
}

// SetSharedContext sets the shared context service for auto-populating run outputs.
func (s *OrchestratorService) SetSharedContext(sc *SharedContextService) {
	s.sharedCtx = sc
}

// NewOrchestratorService creates an OrchestratorService with all dependencies.
func NewOrchestratorService(
	store database.Store,
	hub broadcast.Broadcaster,
	events eventstore.Store,
	runtime *RuntimeService,
	orchCfg *config.Orchestrator,
) *OrchestratorService {
	return &OrchestratorService{
		store:   store,
		hub:     hub,
		events:  events,
		runtime: runtime,
		orchCfg: orchCfg,
	}
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
func (s *OrchestratorService) advanceConsensus(ctx context.Context, p *plan.ExecutionPlan) {
	if plan.AllTerminal(p.Steps) {
		successCount := 0
		for i := range p.Steps {
			if p.Steps[i].Status == plan.StepStatusCompleted {
				successCount++
			}
		}

		quorum := s.orchCfg.ConsensusQuorum
		if quorum <= 0 {
			quorum = (len(p.Steps) / 2) + 1 // majority
		}

		if successCount >= quorum {
			s.completePlan(ctx, p)
		} else {
			s.failPlan(ctx, p)
		}
		return
	}

	// Launch all pending steps (no dependency constraints in consensus)
	for i := range p.Steps {
		if p.Steps[i].Status == plan.StepStatusPending {
			s.startStep(ctx, p, p.Steps[i].ID)
		}
	}
}

// startStep creates a Run for the step and marks it as running.
func (s *OrchestratorService) startStep(ctx context.Context, p *plan.ExecutionPlan, stepID string) {
	var step *plan.Step
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			step = &p.Steps[i]
			break
		}
	}
	if step == nil {
		slog.Error("step not found in plan", "step_id", stepID, "plan_id", p.ID)
		return
	}

	req := &run.StartRequest{
		TaskID:        step.TaskID,
		AgentID:       step.AgentID,
		ProjectID:     p.ProjectID,
		TeamID:        p.TeamID,
		ModeID:        step.ModeID,
		PolicyProfile: step.PolicyProfile,
		DeliverMode:   run.DeliverMode(step.DeliverMode),
	}

	r, err := s.runtime.StartRun(ctx, req)
	if err != nil {
		slog.Error("start step run", "step_id", stepID, "error", err)
		_ = s.store.UpdatePlanStepStatus(ctx, stepID, plan.StepStatusFailed, "", err.Error())
		s.broadcastStepStatus(ctx, p, step, plan.StepStatusFailed)
		s.hub.BroadcastEvent(ctx, ws.AGUIStepFinished, ws.AGUIStepFinishedEvent{
			RunID:  "",
			StepID: step.ID,
			Status: string(plan.StepStatusFailed),
		})
		return
	}

	_ = s.store.UpdatePlanStepStatus(ctx, stepID, plan.StepStatusRunning, r.ID, "")
	s.broadcastStepStatus(ctx, p, step, plan.StepStatusRunning)
	s.hub.BroadcastEvent(ctx, ws.AGUIStepStarted, ws.AGUIStepStartedEvent{
		RunID:  r.ID,
		StepID: step.ID,
		Name:   step.TaskID,
	})
	slog.Info("plan step started", "plan_id", p.ID, "step_id", stepID, "run_id", r.ID)
}

// completePlan marks the plan as completed.
func (s *OrchestratorService) completePlan(ctx context.Context, p *plan.ExecutionPlan) {
	if err := s.store.UpdatePlanStatus(ctx, p.ID, plan.StatusCompleted); err != nil {
		slog.Error("complete plan", "plan_id", p.ID, "error", err)
		return
	}
	p.Status = plan.StatusCompleted
	s.appendPlanEvent(ctx, event.TypePlanCompleted, p)
	s.broadcastPlanStatus(ctx, p)
	if s.onPlanComplete != nil {
		s.onPlanComplete(ctx, p.ID, string(p.Status))
	}
	slog.Info("plan completed", "plan_id", p.ID)
}

// failPlan marks the plan as failed and skips remaining pending steps.
func (s *OrchestratorService) failPlan(ctx context.Context, p *plan.ExecutionPlan) {
	for i := range p.Steps {
		if p.Steps[i].Status == plan.StepStatusPending {
			_ = s.store.UpdatePlanStepStatus(ctx, p.Steps[i].ID, plan.StepStatusSkipped, "", "plan failed")
			s.broadcastStepStatus(ctx, p, &p.Steps[i], plan.StepStatusSkipped)
		}
	}

	if err := s.store.UpdatePlanStatus(ctx, p.ID, plan.StatusFailed); err != nil {
		slog.Error("fail plan", "plan_id", p.ID, "error", err)
		return
	}
	p.Status = plan.StatusFailed
	s.appendPlanEvent(ctx, event.TypePlanFailed, p)
	s.broadcastPlanStatus(ctx, p)
	if s.onPlanComplete != nil {
		s.onPlanComplete(ctx, p.ID, string(p.Status))
	}
	slog.Info("plan failed", "plan_id", p.ID)
}

// --- helpers ---

func (s *OrchestratorService) broadcastPlanStatus(ctx context.Context, p *plan.ExecutionPlan) {
	s.hub.BroadcastEvent(ctx, ws.EventPlanStatus, ws.PlanStatusEvent{
		PlanID:    p.ID,
		ProjectID: p.ProjectID,
		Status:    string(p.Status),
	})
}

func (s *OrchestratorService) broadcastStepStatus(ctx context.Context, p *plan.ExecutionPlan, step *plan.Step, status plan.StepStatus) {
	s.hub.BroadcastEvent(ctx, ws.EventPlanStepStatus, ws.PlanStepStatusEvent{
		PlanID:    p.ID,
		StepID:    step.ID,
		ProjectID: p.ProjectID,
		Status:    string(status),
		RunID:     step.RunID,
		Error:     step.Error,
	})

	// Emit AG-UI step_finished for terminal statuses.
	switch status {
	case plan.StepStatusCompleted, plan.StepStatusFailed, plan.StepStatusCancelled, plan.StepStatusSkipped:
		s.hub.BroadcastEvent(ctx, ws.AGUIStepFinished, ws.AGUIStepFinishedEvent{
			RunID:  step.RunID,
			StepID: step.ID,
			Status: string(status),
		})
	}
}

func (s *OrchestratorService) appendPlanEvent(ctx context.Context, evtType event.Type, p *plan.ExecutionPlan) {
	payload, _ := json.Marshal(map[string]string{
		"plan_id":    p.ID,
		"name":       p.Name,
		"protocol":   string(p.Protocol),
		"status":     string(p.Status),
		"project_id": p.ProjectID,
	})

	_ = s.events.Append(ctx, &event.AgentEvent{
		AgentID:   "",
		TaskID:    "",
		ProjectID: p.ProjectID,
		Type:      evtType,
		Payload:   payload,
	})
}

// ReplanStep restarts a stalled run step with a modified prompt that includes
// stall context, enabling the agent to try a different approach (P1-8).
func (s *OrchestratorService) ReplanStep(ctx context.Context, runID string) error {
	r, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run for replan: %w", err)
	}

	// Reset run status to pending so it can be re-dispatched.
	if err := s.store.UpdateRunStatus(ctx, r.ID, run.StatusPending, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("update run for replan: %w", err)
	}

	slog.Info("re-planning stalled run step",
		"run_id", runID,
		"task_id", r.TaskID,
	)

	s.hub.BroadcastEvent(ctx, "run_replan", map[string]string{
		"run_id":  r.ID,
		"task_id": r.TaskID,
	})

	return nil
}
