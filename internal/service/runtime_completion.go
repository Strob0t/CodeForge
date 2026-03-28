package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/artifact"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// HandleRunComplete processes a run completion message from a worker.
func (s *RuntimeService) HandleRunComplete(ctx context.Context, payload *messagequeue.RunCompletePayload) error {
	r, err := s.store.GetRun(ctx, payload.RunID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
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
		// Transition to quality_gate status — do not finalize yet
		if err := s.store.UpdateRunStatus(ctx, r.ID, run.StatusQualityGate, payload.StepCount, payload.CostUSD, payload.TokensIn, payload.TokensOut); err != nil {
			return fmt.Errorf("update run to quality_gate: %w", err)
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
			return s.finalizeRun(ctx, r, run.StatusFailed, &messagequeue.RunCompletePayload{
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
	return s.finalizeRun(ctx, r, status, payload)
}

// HandleQualityGateResult processes the outcome of a quality gate execution.
func (s *RuntimeService) HandleQualityGateResult(ctx context.Context, result *messagequeue.QualityGateResultPayload) error {
	r, err := s.store.GetRun(ctx, result.RunID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}

	if r.Status != run.StatusQualityGate {
		slog.Warn("received quality gate result for non-gated run", "run_id", r.ID, "status", r.Status)
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
		return s.finalizeRun(ctx, r, run.StatusCompleted, &messagequeue.RunCompletePayload{
			RunID:     r.ID,
			TaskID:    r.TaskID,
			ProjectID: r.ProjectID,
			Status:    string(run.StatusCompleted),
			CostUSD:   r.CostUSD,
			StepCount: r.StepCount,
		})
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

	return s.finalizeRun(ctx, r, finalStatus, &messagequeue.RunCompletePayload{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    string(finalStatus),
		Error:     errMsg,
		CostUSD:   r.CostUSD,
		StepCount: r.StepCount,
	})
}
