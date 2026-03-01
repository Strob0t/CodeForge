package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

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
// If the review router is enabled, it evaluates the step first and may
// broadcast a review decision event.
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

	// Review router evaluation: assess whether this step needs moderated review.
	// Skip evaluation for steps that already completed a debate.
	s.debateMu.Lock()
	alreadyDebated := s.debatedStepIDs[step.ID]
	s.debateMu.Unlock()

	if s.reviewRouter != nil && s.orchCfg.ReviewRouterEnabled && !alreadyDebated {
		routed := s.evaluateStepReview(ctx, p, step)
		if routed {
			s.startDebate(ctx, p, step)
			return
		}
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

// evaluateStepReview runs the review router against a step and broadcasts the decision.
// Returns true if the step was routed to moderated review.
func (s *OrchestratorService) evaluateStepReview(ctx context.Context, p *plan.ExecutionPlan, step *plan.Step) bool {
	// Fetch task description for context
	taskDesc := ""
	t, err := s.store.GetTask(ctx, step.TaskID)
	if err == nil {
		taskDesc = t.Prompt
		if taskDesc == "" {
			taskDesc = t.Title
		}
	}

	decision, err := s.reviewRouter.Evaluate(ctx, step, taskDesc)
	if err != nil {
		slog.Warn("review router evaluation failed, proceeding without review",
			"step_id", step.ID, "error", err)
		return false
	}

	routed := s.reviewRouter.ShouldRoute(decision)

	// Broadcast the review decision for frontend visibility.
	s.hub.BroadcastEvent(ctx, ws.EventReviewRouterDecision, ws.ReviewRouterDecisionEvent{
		PlanID:             p.ID,
		StepID:             step.ID,
		ProjectID:          p.ProjectID,
		NeedsReview:        decision.NeedsReview,
		Confidence:         decision.Confidence,
		Reason:             decision.Reason,
		SuggestedReviewers: decision.SuggestedReviewers,
		Routed:             routed,
	})

	if routed {
		slog.Info("review router: step routed to review",
			"step_id", step.ID,
			"confidence", decision.Confidence,
			"reason", decision.Reason,
		)
	}

	return routed
}

// startDebate creates a ping_pong sub-plan (proponent + moderator) for a step
// that the review router flagged for moderated review.
func (s *OrchestratorService) startDebate(ctx context.Context, p *plan.ExecutionPlan, step *plan.Step) {
	debateRounds := s.orchCfg.DebateRounds
	if debateRounds <= 0 {
		debateRounds = 1
	}
	if debateRounds > 3 {
		debateRounds = 3
	}

	debateReq := &plan.CreatePlanRequest{
		Name:        fmt.Sprintf("debate:%s:%s", p.ID, step.ID),
		Description: fmt.Sprintf("Multi-agent debate for step %s", step.ID),
		ProjectID:   p.ProjectID,
		TeamID:      p.TeamID,
		Protocol:    plan.ProtocolPingPong,
		MaxParallel: 1,
		Steps: []plan.CreateStepRequest{
			{TaskID: step.TaskID, AgentID: step.AgentID, ModeID: "proponent"},
			{TaskID: step.TaskID, AgentID: step.AgentID, ModeID: "moderator"},
		},
	}

	debatePlan, err := s.CreatePlan(ctx, debateReq)
	if err != nil {
		slog.Error("create debate sub-plan", "step_id", step.ID, "error", err)
		// Fall through to normal execution
		return
	}

	// Track the debate -> parent step mapping.
	s.debateMu.Lock()
	s.debateSteps[debatePlan.ID] = debateState{
		ParentPlanID: p.ID,
		ParentStepID: step.ID,
		ProjectID:    p.ProjectID,
	}
	s.debateMu.Unlock()

	// Mark the parent step as running while the debate executes.
	_ = s.store.UpdatePlanStepStatus(ctx, step.ID, plan.StepStatusRunning, "", "")
	s.broadcastStepStatus(ctx, p, step, plan.StepStatusRunning)

	// Broadcast debate started event.
	s.hub.BroadcastEvent(ctx, ws.EventDebateStatus, ws.DebateStatusEvent{
		PlanID:       p.ID,
		StepID:       step.ID,
		ProjectID:    p.ProjectID,
		DebatePlanID: debatePlan.ID,
		Status:       "started",
	})

	// Override ping_pong max rounds with debate-specific config.
	// We need to start the debate plan with the correct round limit.
	// The debate plan uses the global PingPongMaxRounds, so we temporarily
	// rely on the debate_rounds config being set correctly.
	origRounds := s.orchCfg.PingPongMaxRounds
	s.orchCfg.PingPongMaxRounds = debateRounds
	_, err = s.StartPlan(ctx, debatePlan.ID)
	s.orchCfg.PingPongMaxRounds = origRounds

	if err != nil {
		slog.Error("start debate sub-plan", "debate_plan_id", debatePlan.ID, "error", err)
		// Revert step to pending so it can be retried without debate.
		_ = s.store.UpdatePlanStepStatus(ctx, step.ID, plan.StepStatusPending, "", "")
		s.broadcastStepStatus(ctx, p, step, plan.StepStatusPending)

		s.debateMu.Lock()
		delete(s.debateSteps, debatePlan.ID)
		s.debateMu.Unlock()
	}

	slog.Info("debate started",
		"debate_plan_id", debatePlan.ID,
		"parent_plan_id", p.ID,
		"step_id", step.ID,
		"rounds", debateRounds,
	)
}

// handleDebateComplete is called when a debate sub-plan finishes.
// It extracts the moderator's synthesis, injects it into shared context,
// and dispatches the original step's run.
func (s *OrchestratorService) handleDebateComplete(ctx context.Context, debatePlanID, status string) {
	s.debateMu.Lock()
	ds, ok := s.debateSteps[debatePlanID]
	if ok {
		delete(s.debateSteps, debatePlanID)
	}
	s.debateMu.Unlock()

	if !ok {
		return // not a debate plan
	}

	parentPlan, err := s.store.GetPlan(ctx, ds.ParentPlanID)
	if err != nil {
		slog.Error("get parent plan for debate completion", "plan_id", ds.ParentPlanID, "error", err)
		return
	}

	var parentStep *plan.Step
	for i := range parentPlan.Steps {
		if parentPlan.Steps[i].ID == ds.ParentStepID {
			parentStep = &parentPlan.Steps[i]
			break
		}
	}
	if parentStep == nil {
		slog.Error("parent step not found after debate", "step_id", ds.ParentStepID)
		return
	}

	synthesis := ""
	if status == string(plan.StatusCompleted) {
		// Extract the moderator's output (last step in the debate plan).
		debatePlan, err := s.store.GetPlan(ctx, debatePlanID)
		if err == nil && len(debatePlan.Steps) > 0 {
			lastStep := debatePlan.Steps[len(debatePlan.Steps)-1]
			if lastStep.RunID != "" {
				r, err := s.store.GetRun(ctx, lastStep.RunID)
				if err == nil {
					synthesis = r.Output
				}
			}
		}
	}

	// Broadcast debate completion.
	s.hub.BroadcastEvent(ctx, ws.EventDebateStatus, ws.DebateStatusEvent{
		PlanID:       ds.ParentPlanID,
		StepID:       ds.ParentStepID,
		ProjectID:    ds.ProjectID,
		DebatePlanID: debatePlanID,
		Status:       status,
		Synthesis:    synthesis,
	})

	if status != string(plan.StatusCompleted) {
		slog.Warn("debate failed, proceeding with original step without debate context",
			"debate_plan_id", debatePlanID, "status", status)
	}

	// Inject debate synthesis into shared context for the original step.
	if synthesis != "" && s.sharedCtx != nil && parentPlan.TeamID != "" {
		_, _ = s.sharedCtx.AddItem(ctx, cfcontext.AddSharedItemRequest{
			TeamID: parentPlan.TeamID,
			Key:    "debate_synthesis:" + ds.ParentStepID,
			Value:  synthesis,
			Author: "moderator",
		})
	}

	// Mark this step as debated so the review router does not re-evaluate it.
	s.debateMu.Lock()
	s.debatedStepIDs[ds.ParentStepID] = true
	s.debateMu.Unlock()

	// Reset the parent step to pending so advancePlan can dispatch the actual run.
	_ = s.store.UpdatePlanStepStatus(ctx, ds.ParentStepID, plan.StepStatusPending, "", "")
	s.broadcastStepStatus(ctx, parentPlan, parentStep, plan.StepStatusPending)

	// Re-advance the parent plan to dispatch the original step.
	s.advancePlan(ctx, parentPlan)
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
	for _, fn := range s.onPlanCompleteCallbacks {
		fn(ctx, p.ID, string(p.Status))
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
	for _, fn := range s.onPlanCompleteCallbacks {
		fn(ctx, p.ID, string(p.Status))
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
