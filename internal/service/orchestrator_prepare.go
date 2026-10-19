package service

import (
	"context"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// StepPreparer prepares plan steps before their run starts (the review
// pipeline records the workspace baseline of its refactoring step, S6-F 2).
// NeedsPreparation answers under the scheduling lock and must not do I/O;
// PrepareStep runs in the background without the lock and may do git and
// database work. A step whose preparation fails ends failed.
type StepPreparer interface {
	NeedsPreparation(step *plan.Step) bool
	PrepareStep(ctx context.Context, step *plan.Step) error
}

// SetStepPreparer installs the step preparer.
func (s *OrchestratorService) SetStepPreparer(p StepPreparer) {
	s.mu.Lock()
	s.preparer = p
	s.unlock()
}

// preparation is the outcome of a step's preparation, kept until the step
// starts.
type preparation struct{ err error }

// takePreparation returns the preparation outcome of a step that is about to
// start and forgets it. Without an outcome it starts preparing the step in
// the background, once per step, and the step stays pending; when the
// preparation is done the plan is advanced again and the step starts with
// it. The caller holds s.mu.
func (s *OrchestratorService) takePreparation(ctx context.Context, p *plan.ExecutionPlan, step *plan.Step) (done bool, err error) {
	s.prepMu.Lock()
	defer s.prepMu.Unlock()
	if prep, ok := s.prepared[step.ID]; ok {
		delete(s.prepared, step.ID)
		return true, prep.err
	}
	if !s.preparing[step.ID] {
		s.preparing[step.ID] = true
		prepared := *step // the goroutine prepares its own copy
		go s.prepareStep(detachTenant(ctx), p.ID, &prepared)
	}
	return false, nil
}

// prepareStep prepares a step without the scheduling lock, then advances the
// step's plan under it. A plan that is no longer running starts nothing, and
// the outcome is dropped.
func (s *OrchestratorService) prepareStep(ctx context.Context, planID string, step *plan.Step) {
	err := s.preparer.PrepareStep(ctx, step)
	if err != nil {
		slog.Error("prepare plan step", "plan_id", planID, "step_id", step.ID, "error", err)
	}

	s.mu.Lock()
	defer s.unlock()
	s.prepMu.Lock()
	delete(s.preparing, step.ID)
	s.prepared[step.ID] = preparation{err: err}
	s.prepMu.Unlock()

	p, getErr := s.store.GetPlan(ctx, planID)
	if getErr == nil && p.Status == plan.StatusRunning {
		s.advancePlanLocked(ctx, p)
		return
	}
	if getErr != nil {
		slog.Error("get plan after its step's preparation", "plan_id", planID, "step_id", step.ID, "error", getErr)
	}
	s.prepMu.Lock()
	delete(s.prepared, step.ID)
	s.prepMu.Unlock()
}

// failStepStart ends a step whose run could not be started (or prepared) as
// failed. The caller holds s.mu.
func (s *OrchestratorService) failStepStart(ctx context.Context, p *plan.ExecutionPlan, step *plan.Step, reason string) {
	logBestEffort(ctx, s.store.UpdatePlanStepStatus(ctx, step.ID, plan.StepStatusFailed, "", reason), "UpdatePlanStepStatus", slog.String("step_id", step.ID))
	s.broadcastStepStatus(ctx, p, step, plan.StepStatusFailed)
	s.hub.BroadcastEvent(ctx, event.AGUIStepFinished, event.AGUIStepFinishedEvent{
		RunID:  "",
		StepID: step.ID,
		Status: string(plan.StepStatusFailed),
	})
}
