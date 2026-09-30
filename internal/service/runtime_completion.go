package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/artifact"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// finishRun finalizes a run on a worker message. A run that already ended on
// another path is skipped instead of failing the handler, which would have
// the message redelivered (KI-31); it keeps the worker's usage totals.
func (s *RuntimeService) finishRun(ctx context.Context, r *run.Run, status run.Status, payload *messagequeue.RunCompletePayload) error {
	err := s.finalizeRun(ctx, r, status, payload)
	if errors.Is(err, domain.ErrConflict) {
		s.keepWorkerTotals(ctx, r.ID, payload)
	}
	return skipEndedRun(ctx, err, "finalizeRun", r.ID)
}

// keepWorkerTotals raises the counters of a run that the control plane
// already ended (cancel, timeout, limits) to the usage totals the worker
// reports at its end: calls finished after the stop still cost money. Nothing
// else of the ended run changes.
func (s *RuntimeService) keepWorkerTotals(ctx context.Context, runID string, payload *messagequeue.RunCompletePayload) {
	totals := &run.Usage{Steps: payload.StepCount, CostUSD: payload.CostUSD, TokensIn: payload.TokensIn, TokensOut: payload.TokensOut}
	logBestEffort(ctx, s.store.RaiseRunUsage(ctx, runID, totals), "RaiseRunUsage", slog.String("run_id", runID))
}

// HandleRunComplete processes a run completion message from a worker.
func (s *RuntimeService) HandleRunComplete(ctx context.Context, payload *messagequeue.RunCompletePayload) error {
	ctx, r, err := s.loadRunScoped(ctx, payload.RunID, payload.TenantID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}
	if r.Status.IsTerminal() {
		slog.Info("completion for a run that already ended, usage kept", "run_id", r.ID, "status", r.Status)
		s.keepWorkerTotals(ctx, r.ID, payload)
		return nil
	}
	// runs.complete is delivered at least once: a run waiting for its quality
	// gate stored its completion when it entered the gate, and only the gate
	// result (or the gate watchdog) ends it.
	if r.Status == run.StatusQualityGate {
		slog.Info("completion for a run waiting for its quality gate, ignored", "run_id", r.ID)
		return nil
	}
	// The control plane is stopping the run and records its end with the
	// stop's status and reason; the worker's completion (its answer to the
	// stop) contributes its usage totals only.
	if s.state.IsStopping(r.ID) {
		slog.Info("completion for a run being stopped, usage kept", "run_id", r.ID, "status", payload.Status)
		s.keepWorkerTotals(ctx, r.ID, payload)
		return nil
	}

	// Determine final status
	status := run.Status(payload.Status)
	if status == "" {
		if payload.Error != "" {
			status = run.StatusFailed
		} else {
			status = run.StatusCompleted
		}
	}

	// Artifact validation gate (Phase 12E)
	if status == run.StatusCompleted && s.modes != nil {
		if m, mErr := s.modes.Get(r.ModeID); mErr == nil && m.RequiredArtifact != "" {
			result := artifact.Validate(m.RequiredArtifact, payload.Output)
			valid := result.Valid
			if err := s.store.UpdateRunArtifact(ctx, r.ID, m.RequiredArtifact, &valid, result.Errors); err != nil {
				slog.Error("failed to persist artifact validation", "run_id", r.ID, "error", err)
			}
			s.hub.BroadcastEvent(ctx, event.EventArtifactValidation, event.ArtifactValidationEvent{
				RunID:        r.ID,
				TaskID:       r.TaskID,
				ProjectID:    r.ProjectID,
				ArtifactType: m.RequiredArtifact,
				Valid:        valid,
				Errors:       result.Errors,
			})
			if valid {
				s.appendRunEvent(ctx, event.TypeArtifactValidated, r, map[string]string{
					"artifact_type": m.RequiredArtifact,
				})
			} else {
				s.appendRunEvent(ctx, event.TypeArtifactFailed, r, map[string]string{
					"artifact_type": m.RequiredArtifact,
					"errors":        fmt.Sprintf("%v", result.Errors),
				})
				s.appendAudit(ctx, r, "artifact.failed", fmt.Sprintf("Artifact validation failed for %s: %v", m.RequiredArtifact, result.Errors))
				status = run.StatusFailed
			}
		}
	}

	// A completed run whose policy has quality gates waits for them; any other
	// run ends now (delivery, if configured, happens when it ends completed).
	// A run whose profile no longer exists cannot tell whether it needs a
	// gate, so it does not complete (fail closed).
	profile, ok := s.policy.GetProfile(r.PolicyProfile)
	if status == run.StatusCompleted && !ok {
		failed := *payload
		failed.Status, failed.Error = string(run.StatusFailed), unknownGateProfile(r.PolicyProfile)
		return s.finishRun(ctx, r, run.StatusFailed, &failed)
	}
	if status == run.StatusCompleted && profile.QualityGate.Enabled() {
		return s.enterQualityGate(ctx, r, &profile.QualityGate, payload)
	}
	return s.finishRun(ctx, r, status, payload)
}

// storedOutcome is a completion that ends a run with the outcome stored on it
// and status and errMsg: for a run that waited for its quality gate, the
// output, model and usage the worker reported (stored when it entered the
// gate); for a run the control plane stops, what it has so far.
func storedOutcome(r *run.Run, status run.Status, errMsg string) *messagequeue.RunCompletePayload {
	return &messagequeue.RunCompletePayload{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    string(status),
		Output:    r.Output,
		Error:     errMsg,
		CostUSD:   r.CostUSD,
		StepCount: r.StepCount,
		TokensIn:  r.TokensIn,
		TokensOut: r.TokensOut,
		Model:     r.Model,
	}
}
