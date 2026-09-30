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
	"github.com/Strob0t/CodeForge/internal/tenantctx"
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

	// Check if quality gates should be triggered
	profile, ok := s.policy.GetProfile(r.PolicyProfile)
	hasGates := ok && status == run.StatusCompleted &&
		(profile.QualityGate.RequireTestsPass || profile.QualityGate.RequireLintPass)

	if hasGates {
		// Transition to quality_gate status — do not finalize yet. The run keeps
		// the worker's outcome for the gate result to finalize it with.
		if err := s.store.EnterQualityGate(ctx, &run.CompletionRequest{
			ID: r.ID, Status: run.StatusQualityGate, Output: payload.Output, Error: payload.Error,
			CostUSD: payload.CostUSD, StepCount: payload.StepCount, TokensIn: payload.TokensIn, TokensOut: payload.TokensOut, Model: payload.Model,
		}); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				s.keepWorkerTotals(ctx, r.ID, payload)
			}
			return skipEndedRun(ctx, fmt.Errorf("enter quality gate: %w", err), "EnterQualityGate", r.ID)
		}

		// Look up project for workspace path
		proj, projErr := s.store.GetProject(ctx, r.ProjectID)
		workspacePath := ""
		if projErr == nil {
			workspacePath = proj.WorkspacePath
		}

		// Determine commands (project-level → config defaults)
		testCmd := s.runtimeCfg.DefaultTestCommand
		lintCmd := s.runtimeCfg.DefaultLintCommand

		// Publish quality gate request
		gateReq := messagequeue.QualityGateRequestPayload{
			RunID:         r.ID,
			ProjectID:     r.ProjectID,
			TenantID:      tenantctx.FromContext(ctx),
			WorkspacePath: workspacePath,
			RunTests:      profile.QualityGate.RequireTestsPass,
			RunLint:       profile.QualityGate.RequireLintPass,
			TestCommand:   testCmd,
			LintCommand:   lintCmd,
		}
		if err := s.publishJSON(ctx, messagequeue.SubjectQualityGateRequest, gateReq); err != nil {
			slog.Error("failed to publish quality gate request, failing run (fail-closed)", "run_id", r.ID, "error", err)
			s.appendAudit(ctx, r, "qualitygate.error", fmt.Sprintf("Failed to publish quality gate request: %s", err.Error()))
			// Fail-closed: if we can't run quality gates, don't silently pass.
			return s.finishRun(ctx, r, run.StatusFailed, &messagequeue.RunCompletePayload{
				RunID:     r.ID,
				TaskID:    r.TaskID,
				ProjectID: r.ProjectID,
				Status:    string(run.StatusFailed),
				Error:     "quality gate unavailable: " + err.Error(),
				CostUSD:   payload.CostUSD,
				StepCount: payload.StepCount,
				TokensIn:  payload.TokensIn,
				TokensOut: payload.TokensOut,
				Model:     payload.Model,
			})
		}

		// Record event and broadcast
		s.appendRunEvent(ctx, event.TypeQualityGateStarted, r, map[string]string{
			"run_tests": fmt.Sprintf("%t", profile.QualityGate.RequireTestsPass),
			"run_lint":  fmt.Sprintf("%t", profile.QualityGate.RequireLintPass),
		})
		s.hub.BroadcastEvent(ctx, event.EventQualityGate, event.QualityGateEvent{
			RunID:     r.ID,
			TaskID:    r.TaskID,
			ProjectID: r.ProjectID,
			Status:    "started",
		})
		// Use a temporary copy with payload values for the broadcast.
		gateRun := *r
		gateRun.StepCount = payload.StepCount
		gateRun.CostUSD = payload.CostUSD
		gateRun.TokensIn = payload.TokensIn
		gateRun.TokensOut = payload.TokensOut
		gateRun.Model = payload.Model
		s.broadcastRunStatus(ctx, &gateRun, run.StatusQualityGate)

		slog.Info("quality gate triggered", "run_id", r.ID)
		return nil // Wait for quality gate result
	}

	// No quality gates configured — finalize immediately
	return s.finishRun(ctx, r, status, payload)
}

// HandleQualityGateResult processes the outcome of a quality gate execution.
func (s *RuntimeService) HandleQualityGateResult(ctx context.Context, result *messagequeue.QualityGateResultPayload) error {
	ctx, r, err := s.loadRunScoped(ctx, result.RunID, result.TenantID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}

	if r.Status != run.StatusQualityGate {
		slog.Warn("received quality gate result for non-gated run", "run_id", r.ID, "status", r.Status)
		return nil
	}
	if s.state.IsStopping(r.ID) {
		slog.Info("quality gate result for a run being stopped, skipped", "run_id", r.ID)
		return nil
	}

	profile, _ := s.policy.GetProfile(r.PolicyProfile)

	// Determine if gates passed
	allPassed := result.Error == "" &&
		(result.TestsPassed == nil || *result.TestsPassed) &&
		(result.LintPassed == nil || *result.LintPassed)

	if allPassed {
		s.appendAudit(ctx, r, "qualitygate.passed", "Quality gate passed")
		s.appendRunEvent(ctx, event.TypeQualityGatePassed, r, map[string]string{})
		s.hub.BroadcastEvent(ctx, event.EventQualityGate, event.QualityGateEvent{
			RunID:       r.ID,
			TaskID:      r.TaskID,
			ProjectID:   r.ProjectID,
			Status:      "passed",
			TestsPassed: result.TestsPassed,
			LintPassed:  result.LintPassed,
		})

		// Trigger delivery if configured, then finalize as completed
		s.triggerDelivery(ctx, r)
		return s.finishRun(ctx, r, run.StatusCompleted, storedOutcome(r, run.StatusCompleted, ""))
	}

	// Gates failed
	finalStatus := run.StatusCompleted // gates failed but don't downgrade unless configured
	errMsg := "quality gate failed"
	if result.Error != "" {
		errMsg = result.Error
	}
	if profile.QualityGate.RollbackOnGateFail {
		finalStatus = run.StatusFailed
		errMsg = "quality gate failed (rollback)"
		if s.checkpoint != nil {
			proj, projErr := s.store.GetProject(ctx, r.ProjectID)
			if projErr == nil {
				if rwErr := s.checkpoint.RewindToFirst(ctx, r.ID, proj.WorkspacePath); rwErr != nil {
					slog.Error("checkpoint rollback failed", "run_id", r.ID, "error", rwErr)
				}
			}
		}
	}

	s.appendAudit(ctx, r, "qualitygate.failed", fmt.Sprintf("Quality gate failed: %s", errMsg))
	s.appendRunEvent(ctx, event.TypeQualityGateFailed, r, map[string]string{
		"error": errMsg,
	})
	s.hub.BroadcastEvent(ctx, event.EventQualityGate, event.QualityGateEvent{
		RunID:       r.ID,
		TaskID:      r.TaskID,
		ProjectID:   r.ProjectID,
		Status:      "failed",
		TestsPassed: result.TestsPassed,
		LintPassed:  result.LintPassed,
		Error:       errMsg,
	})

	return s.finishRun(ctx, r, finalStatus, storedOutcome(r, finalStatus, errMsg))
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
