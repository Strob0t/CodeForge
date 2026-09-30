package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Quality gates (D9): a run the worker completed under a policy with gates
// waits in quality_gate until the worker reports the gate result. A passed
// gate completes the run, which then delivers; a failed gate fails the run,
// rolls the workspace back if the policy says so, and never delivers.

// enterQualityGate moves a completed run to quality_gate with the worker's
// outcome and requests its gate.
func (s *RuntimeService) enterQualityGate(ctx context.Context, r *run.Run, gate *policy.QualityGate, payload *messagequeue.RunCompletePayload) error {
	if err := s.store.EnterQualityGate(ctx, &run.CompletionRequest{
		ID: r.ID, Status: run.StatusQualityGate, Output: payload.Output, Error: payload.Error,
		CostUSD: payload.CostUSD, StepCount: payload.StepCount, TokensIn: payload.TokensIn, TokensOut: payload.TokensOut, Model: payload.Model,
	}); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			s.keepWorkerTotals(ctx, r.ID, payload)
		}
		return skipEndedRun(ctx, fmt.Errorf("enter quality gate: %w", err), "EnterQualityGate", r.ID)
	}
	gated := gatedRun(r, payload)

	proj, err := s.store.GetProject(ctx, r.ProjectID)
	if err != nil {
		return s.failQualityGate(ctx, gated, gate, "project unavailable: "+err.Error(), nil)
	}
	cmds := s.gateCommands(proj)
	if missing := missingGateCommands(gate, cmds); missing != "" {
		return s.failQualityGate(ctx, gated, gate, fmt.Sprintf("no command for the required checks; set %s in the project config", missing), nil)
	}
	gateReq := messagequeue.QualityGateRequestPayload{
		RunID:         r.ID,
		ProjectID:     r.ProjectID,
		TenantID:      tenantctx.FromContext(ctx),
		WorkspacePath: proj.WorkspacePath,
		RunTests:      gate.RequireTestsPass,
		RunLint:       gate.RequireLintPass,
		TestCommand:   cmds.Test,
		LintCommand:   cmds.Lint,
		// Whole seconds, never shorter than configured (KI-28).
		TimeoutSeconds: int(math.Ceil(s.runtimeCfg.QualityGateTimeout.Seconds())),
	}
	if err := s.publishJSON(ctx, messagequeue.SubjectQualityGateRequest, gateReq); err != nil {
		slog.Error("quality gate request not published, failing the gate", "run_id", r.ID, "error", err)
		return s.failQualityGate(ctx, gated, gate, "request not published: "+err.Error(), nil)
	}

	s.appendRunEvent(ctx, event.TypeQualityGateStarted, gated, map[string]string{
		"run_tests": fmt.Sprintf("%t", gate.RequireTestsPass),
		"run_lint":  fmt.Sprintf("%t", gate.RequireLintPass),
	})
	s.hub.BroadcastEvent(ctx, event.EventQualityGate, event.QualityGateEvent{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    "started",
	})
	s.broadcastRunStatus(ctx, gated, run.StatusQualityGate)

	slog.Info("quality gate triggered", "run_id", r.ID)
	return nil
}

// gateCommands returns the commands of the project's gate (KI-29): the
// project config's test_command and lint_command, else the defaults of the
// language detected in the workspace now (the run may have created the
// project), else the configured runtime defaults.
func (s *RuntimeService) gateCommands(proj *project.Project) project.GateCommands {
	cmds := proj.GateCommandOverrides()
	if proj.WorkspacePath != "" {
		stack, err := project.ScanWorkspace(proj.WorkspacePath)
		if err != nil {
			slog.Warn("quality gate: workspace language not detected", "project_id", proj.ID, "error", err)
		} else {
			cmds = cmds.Or(project.DefaultGateCommands(stack.Languages))
		}
	}
	return cmds.Or(project.GateCommands{Test: s.runtimeCfg.DefaultTestCommand, Lint: s.runtimeCfg.DefaultLintCommand})
}

// missingGateCommands names the project config keys of the required checks
// that have no command; "" when every required check has one.
func missingGateCommands(gate *policy.QualityGate, cmds project.GateCommands) string {
	var missing []string
	if gate.RequireTestsPass && cmds.Test == "" {
		missing = append(missing, project.ConfigTestCommand)
	}
	if gate.RequireLintPass && cmds.Lint == "" {
		missing = append(missing, project.ConfigLintCommand)
	}
	return strings.Join(missing, " and ")
}

// gatedRun is r as the store holds it after it entered its quality gate with
// the worker's outcome (usage counters never go down).
func gatedRun(r *run.Run, payload *messagequeue.RunCompletePayload) *run.Run {
	g := *r
	g.Status = run.StatusQualityGate
	g.Output, g.Error, g.Model = payload.Output, payload.Error, payload.Model
	g.CostUSD = max(g.CostUSD, payload.CostUSD)
	g.StepCount = max(g.StepCount, payload.StepCount)
	g.TokensIn = max(g.TokensIn, payload.TokensIn)
	g.TokensOut = max(g.TokensOut, payload.TokensOut)
	return &g
}

// HandleQualityGateResult ends a run waiting for its quality gate with the
// gate's outcome. The result subject is delivered at least once: a result for
// a run that no longer waits for its gate changes nothing.
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

	profile, ok := s.policy.GetProfile(r.PolicyProfile)
	if !ok {
		return s.failQualityGate(ctx, r, &policy.QualityGate{}, unknownGateProfile(r.PolicyProfile), result)
	}
	if reason := gateFailure(&profile.QualityGate, result); reason != "" {
		return s.failQualityGate(ctx, r, &profile.QualityGate, reason, result)
	}

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
	return s.finishRun(ctx, r, run.StatusCompleted, storedOutcome(r, run.StatusCompleted, ""), agentEnd)
}

// unknownGateProfile is the failure of a gate whose policy profile no longer
// exists: which checks it requires is unknown, so it fails closed.
func unknownGateProfile(name string) string {
	return fmt.Sprintf("unknown policy profile %q, its quality gate cannot be checked", name)
}

// gateFailure tells why a gate result fails the gate; "" means it passed. A
// required check without a result fails (KI-29), as does any failed check.
func gateFailure(gate *policy.QualityGate, result *messagequeue.QualityGateResultPayload) string {
	if result.Error != "" {
		return result.Error
	}
	var failed []string
	if reason := checkFailure(gate.RequireTestsPass, result.TestsPassed, "no test result", "tests failed"); reason != "" {
		failed = append(failed, reason)
	}
	if reason := checkFailure(gate.RequireLintPass, result.LintPassed, "no lint result", "lint failed"); reason != "" {
		failed = append(failed, reason)
	}
	return strings.Join(failed, ", ")
}

// checkFailure is the failure of one check of a gate result, "" if none.
func checkFailure(required bool, passed *bool, missing, failed string) string {
	switch {
	case passed == nil && required:
		return missing
	case passed != nil && !*passed:
		return failed
	default:
		return ""
	}
}

// failQualityGate ends a run waiting for its gate as failed (D9), with the
// outcome stored on it and reason; it is never delivered. If the policy says
// so, the workspace is rolled back once the failed record is written, so only
// the path that ends the run rolls back (runEnd.rollBack). result is the
// worker's gate result, nil when the gate did not run.
func (s *RuntimeService) failQualityGate(ctx context.Context, r *run.Run, gate *policy.QualityGate, reason string, result *messagequeue.QualityGateResultPayload) error {
	errMsg := "quality gate failed: " + reason
	gateEvent := event.QualityGateEvent{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    "failed",
		Error:     errMsg,
	}
	if result != nil {
		gateEvent.TestsPassed, gateEvent.LintPassed = result.TestsPassed, result.LintPassed
	}
	s.appendAudit(ctx, r, "qualitygate.failed", errMsg)
	s.appendRunEvent(ctx, event.TypeQualityGateFailed, r, map[string]string{"error": errMsg})
	s.hub.BroadcastEvent(ctx, event.EventQualityGate, gateEvent)

	return s.finishRun(ctx, r, run.StatusFailed, storedOutcome(r, run.StatusFailed, errMsg),
		runEnd{agentWorked: true, rollBack: gate.RollbackOnGateFail})
}

// qualityGateMargin is how long past the longest possible gate (both
// commands running into the timeout) a run may wait for its gate result:
// queueing, worker restarts and the result's delivery.
const qualityGateMargin = time.Minute

// staleRunBatch bounds the runs one watchdog sweep ends.
const staleRunBatch = 100

// qualityGateDeadline is how long a run may wait in quality_gate before the
// watchdog fails it: a gate runs at most the test and the lint command, each
// bounded by runtime.quality_gate_timeout on the worker.
func (s *RuntimeService) qualityGateDeadline() time.Duration {
	return 2*s.runtimeCfg.QualityGateTimeout + qualityGateMargin
}

// FailStuckQualityGates fails the runs that have waited in quality_gate
// longer than their gate can take (KI-28): the gate request or result was
// lost, or the worker died. Each is ended as a failed gate through the gate
// result path (fresh status check, rollback if configured, never delivered),
// in its own tenant; a run another replica or a late result ended meanwhile is
// skipped by the store's status predicate. It returns how many stuck runs it
// handled. Runs are found in the store, so the sweep also covers runs that
// entered their gate before a Go Core restart.
func (s *RuntimeService) FailStuckQualityGates(ctx context.Context) (int, error) {
	deadline := s.qualityGateDeadline()
	stale, err := s.store.ListStaleRuns(ctx, run.StatusQualityGate, deadline, staleRunBatch)
	if err != nil {
		return 0, fmt.Errorf("list runs stuck in quality_gate: %w", err)
	}
	handled := 0
	var errs []error
	for i := range stale {
		r := &stale[i]
		slog.Warn("quality gate result missing, failing the run", "run_id", r.ID, "deadline", deadline)
		if err := s.HandleQualityGateResult(ctx, &messagequeue.QualityGateResultPayload{
			RunID:    r.ID,
			TenantID: r.TenantID,
			Error:    fmt.Sprintf("no quality gate result within %s", deadline),
		}); err != nil {
			errs = append(errs, fmt.Errorf("run %s: %w", r.ID, err))
			continue
		}
		handled++
	}
	return handled, errors.Join(errs...)
}
