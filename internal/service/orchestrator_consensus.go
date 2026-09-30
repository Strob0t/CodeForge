package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/Strob0t/CodeForge/internal/domain"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

func (s *OrchestratorService) advanceConsensus(ctx context.Context, p *plan.ExecutionPlan) (startFailed bool) {
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
		return false
	}

	// Launch all pending steps (no dependency constraints in consensus)
	for i := range p.Steps {
		if p.Steps[i].Status == plan.StepStatusPending && !s.startStep(ctx, p, p.Steps[i].ID) {
			startFailed = true
		}
	}
	return startFailed
}

// startStep creates a Run for the step and marks it as running, and reports
// whether the step started (with a run or a debate). A step whose run cannot
// be started is marked failed. If the review router is enabled, it evaluates
// the step first and may start a debate for it instead. The caller holds s.mu.
func (s *OrchestratorService) startStep(ctx context.Context, p *plan.ExecutionPlan, stepID string) bool {
	var step *plan.Step
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			step = &p.Steps[i]
			break
		}
	}
	if step == nil {
		slog.Error("step not found in plan", "step_id", stepID, "plan_id", p.ID)
		return false
	}

	// Review router evaluation: assess whether this step needs moderated review.
	// Skip evaluation for steps that already completed a debate and for the
	// steps of a debate itself (a debate is never debated again).
	s.debateMu.Lock()
	alreadyDebated := s.debatedStepIDs[step.ID]
	_, isDebateStep := s.debateSteps[p.ID]
	s.debateMu.Unlock()

	if s.reviewRouter != nil && s.orchCfg.ReviewRouterEnabled && !alreadyDebated && !isDebateStep {
		if s.evaluateStepReview(ctx, p, step) && s.startDebate(ctx, p, step) {
			return true
		}
		// Not routed, or the debate could not be started: run the step.
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
		logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, stepID, plan.StepStatusFailed, "", err.Error()), "UpdatePlanStepStatus", slog.String("step_id", stepID))
		s.broadcastStepStatus(ctx, p, step, plan.StepStatusFailed)
		s.hub.BroadcastEvent(ctx, event.AGUIStepFinished, event.AGUIStepFinishedEvent{
			RunID:  "",
			StepID: step.ID,
			Status: string(plan.StepStatusFailed),
		})
		return false
	}

	logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, stepID, plan.StepStatusRunning, r.ID, ""), "UpdatePlanStepStatus", slog.String("step_id", stepID))
	s.broadcastStepStatus(ctx, p, step, plan.StepStatusRunning)
	s.hub.BroadcastEvent(ctx, event.AGUIStepStarted, event.AGUIStepStartedEvent{
		RunID:  r.ID,
		StepID: step.ID,
		Name:   step.TaskID,
	})
	slog.Info("plan step started", "plan_id", p.ID, "step_id", stepID, "run_id", r.ID)
	return true
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
	s.hub.BroadcastEvent(ctx, event.EventReviewRouterDecision, event.ReviewRouterDecisionEvent{
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
// that the review router flagged for moderated review, and reports whether
// the debate started. The caller holds s.mu: the sub-plan is started without
// taking it again.
func (s *OrchestratorService) startDebate(ctx context.Context, p *plan.ExecutionPlan, step *plan.Step) bool {
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
		return false // the step runs without a debate
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
	logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, step.ID, plan.StepStatusRunning, "", ""), "UpdatePlanStepStatus", slog.String("step_id", step.ID))
	s.broadcastStepStatus(ctx, p, step, plan.StepStatusRunning)

	// Broadcast debate started event.
	s.hub.BroadcastEvent(ctx, event.EventDebateStatus, event.DebateStatusEvent{
		PlanID:       p.ID,
		StepID:       step.ID,
		ProjectID:    p.ProjectID,
		DebatePlanID: debatePlan.ID,
		Status:       "started",
	})

	// Store per-plan max rounds override so advancePingPong uses
	// debate-specific rounds instead of the global config value.
	s.debateMu.Lock()
	s.planMaxRounds[debatePlan.ID] = debateRounds
	s.debateMu.Unlock()

	if _, err := s.startPlanLocked(ctx, debatePlan.ID); err != nil {
		slog.Error("start debate sub-plan", "debate_plan_id", debatePlan.ID, "error", err)
		s.debateMu.Lock()
		delete(s.debateSteps, debatePlan.ID)
		delete(s.planMaxRounds, debatePlan.ID)
		s.debateMu.Unlock()
		return false // the step runs without a debate
	}

	slog.Info("debate started",
		"debate_plan_id", debatePlan.ID,
		"parent_plan_id", p.ID,
		"step_id", step.ID,
		"rounds", debateRounds,
	)
	return true
}

// handleDebateComplete is called when a debate sub-plan finishes.
// It extracts the moderator's synthesis, injects it into shared context,
// and dispatches the original step's run. It runs as a plan completion
// callback, which completePlan and failPlan call while the plan is advanced
// under s.mu: the parent plan is advanced without taking the lock again.
func (s *OrchestratorService) handleDebateComplete(ctx context.Context, debatePlanID, status string) {
	s.debateMu.Lock()
	ds, ok := s.debateSteps[debatePlanID]
	if ok {
		delete(s.debateSteps, debatePlanID)
		delete(s.planMaxRounds, debatePlanID)
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
	s.hub.BroadcastEvent(ctx, event.EventDebateStatus, event.DebateStatusEvent{
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
	logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, ds.ParentStepID, plan.StepStatusPending, "", ""), "UpdatePlanStepStatus", slog.String("step_id", ds.ParentStepID))
	s.broadcastStepStatus(ctx, parentPlan, parentStep, plan.StepStatusPending)

	// Re-advance the parent plan to dispatch the original step.
	s.advancePlanLocked(ctx, parentPlan)
}

// completePlan marks the plan as completed. The caller holds s.mu (the plan
// completion callbacks run under it).
func (s *OrchestratorService) completePlan(ctx context.Context, p *plan.ExecutionPlan) {
	if err := s.store.UpdatePlanStatus(ctx, p.ID, plan.StatusCompleted); err != nil {
		logPlanEndFailure(ctx, err, p.ID, plan.StatusCompleted)
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

// failPlan marks the plan as failed and skips remaining pending steps. The
// caller holds s.mu (the plan completion callbacks run under it).
func (s *OrchestratorService) failPlan(ctx context.Context, p *plan.ExecutionPlan) {
	// The plan first: a plan that already ended keeps its steps.
	if err := s.store.UpdatePlanStatus(ctx, p.ID, plan.StatusFailed); err != nil {
		logPlanEndFailure(ctx, err, p.ID, plan.StatusFailed)
		return
	}
	for i := range p.Steps {
		if p.Steps[i].Status == plan.StepStatusPending {
			logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, p.Steps[i].ID, plan.StepStatusSkipped, "", "plan failed"), "UpdatePlanStepStatus", slog.String("step_id", p.Steps[i].ID))
			s.broadcastStepStatus(ctx, p, &p.Steps[i], plan.StepStatusSkipped)
		}
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

// logPlanEndFailure logs why a plan could not be completed or failed. A plan
// that already ended on another path (e.g. cancelled while its last run
// finished) is refused by the store (KI-31) and skipped.
func logPlanEndFailure(ctx context.Context, err error, planID string, status plan.Status) {
	if errors.Is(err, domain.ErrConflict) {
		slog.InfoContext(ctx, "plan already ended, skipped", "plan_id", planID, "status", status)
		return
	}
	slog.ErrorContext(ctx, "end plan", "plan_id", planID, "status", status, "error", err)
}

func (s *OrchestratorService) broadcastPlanStatus(ctx context.Context, p *plan.ExecutionPlan) {
	s.hub.BroadcastEvent(ctx, event.EventPlanStatus, event.PlanStatusEvent{
		PlanID:    p.ID,
		ProjectID: p.ProjectID,
		Status:    string(p.Status),
	})
}

func (s *OrchestratorService) broadcastStepStatus(ctx context.Context, p *plan.ExecutionPlan, step *plan.Step, status plan.StepStatus) {
	s.hub.BroadcastEvent(ctx, event.EventPlanStepStatus, event.PlanStepStatusEvent{
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
		s.hub.BroadcastEvent(ctx, event.AGUIStepFinished, event.AGUIStepFinishedEvent{
			RunID:  step.RunID,
			StepID: step.ID,
			Status: string(status),
		})
	}
}

func (s *OrchestratorService) appendPlanEvent(ctx context.Context, evtType event.Type, p *plan.ExecutionPlan) {
	payload, err := json.Marshal(map[string]string{
		"plan_id":    p.ID,
		"name":       p.Name,
		"protocol":   string(p.Protocol),
		"status":     string(p.Status),
		"project_id": p.ProjectID,
	})
	if err != nil {
		slog.Warn("marshal plan event payload", "plan_id", p.ID, "error", err)
		return
	}

	// A plan has no agent or task of its own; the event store keeps them NULL.
	logBestEffort(ctx, s.events.Append(ctx, &event.AgentEvent{
		ProjectID: p.ProjectID,
		Type:      evtType,
		Payload:   payload,
	}), "AppendEvent", slog.String("type", string(evtType)), slog.String("plan_id", p.ID))
}

// ReplanStep gives the plan step of an ended (e.g. stalled) run another
// attempt (P1-8; not called by stall detection yet, KI-62). The ended run is
// never reopened - a terminal run stays terminal (KI-31): the step becomes
// pending again, the steps skipped because it did not complete become
// pending too, and the plan's protocol starts the step with a new run.
//
// Only a run that ended unsuccessfully (failed, timed out, cancelled) whose
// step still holds it and ended unsuccessfully is re-planned, and only while
// its plan runs. This is decided under the scheduling lock, so concurrent
// re-plans of the same run start one new run, and a step whose completion was
// not processed yet (still running) is refused. The new run gets the step's
// task prompt; stall context is not added to it yet. If the new run cannot be
// started, the step ends failed and the plan is decided again.
func (s *OrchestratorService) ReplanStep(ctx context.Context, runID string) error {
	r, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run for replan: %w", err)
	}
	if !r.Status.IsTerminal() || r.Status == run.StatusCompleted {
		return fmt.Errorf("replan run %s: the run is %s, only a run that ended unsuccessfully is re-planned: %w", runID, r.Status, domain.ErrConflict)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	step, err := s.store.GetPlanStepByRunID(ctx, runID)
	if err != nil {
		return fmt.Errorf("replan run %s: no plan step runs it: %w", runID, err)
	}
	if step.RunID != runID || !step.Status.Unsuccessful() {
		return fmt.Errorf("replan run %s: its step %s is %s: %w", runID, step.ID, step.Status, domain.ErrConflict)
	}
	p, err := s.store.GetPlan(ctx, step.PlanID)
	if err != nil {
		return fmt.Errorf("get plan for replan: %w", err)
	}
	if p.Status != plan.StatusRunning {
		return fmt.Errorf("replan run %s: plan %s is %s: %w", runID, p.ID, p.Status, domain.ErrConflict)
	}

	if err := s.store.UpdatePlanStepStatus(ctx, step.ID, plan.StepStatusPending, "", ""); err != nil {
		return fmt.Errorf("replan run %s: reset step %s: %w", runID, step.ID, err)
	}
	s.unblockDependents(ctx, p, step.ID)

	slog.Info("re-planning plan step with a new run",
		"ended_run_id", runID,
		"step_id", step.ID,
		"task_id", r.TaskID,
	)
	s.hub.BroadcastEvent(ctx, "run_replan", map[string]string{
		"run_id":  r.ID,
		"task_id": r.TaskID,
	})

	s.advancePlanLocked(ctx, p)
	return nil
}

// unblockDependents makes the steps that were skipped because stepID did not
// complete pending again. A dependent that another unsuccessful step still
// blocks is skipped again when the plan advances.
func (s *OrchestratorService) unblockDependents(ctx context.Context, p *plan.ExecutionPlan, stepID string) {
	dependents := plan.DependentSteps(p.Steps, stepID)
	for i := range p.Steps {
		st := &p.Steps[i]
		if st.Status != plan.StepStatusSkipped || st.Error != blockedStepError || !slices.Contains(dependents, st.ID) {
			continue
		}
		logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, st.ID, plan.StepStatusPending, "", ""), "UpdatePlanStepStatus", slog.String("step_id", st.ID))
		st.Status = plan.StepStatusPending
		st.Error = ""
		s.broadcastStepStatus(ctx, p, st, plan.StepStatusPending)
	}
}
