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
		return s.failQualityGate(ctx, gated, gate, gateVerdict{reason: "project unavailable: " + err.Error()}, nil)
	}
	cmds := s.gateCommands(proj)
	if missing := missingGateCommands(gate, cmds); missing != "" {
		return s.failQualityGate(ctx, gated, gate, gateVerdict{reason: fmt.Sprintf("no command for the required checks; set %s in the project config", missing)}, nil)
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
		TimeoutSeconds:   int(math.Ceil(s.runtimeCfg.QualityGateTimeout.Seconds())),
		HeartbeatSeconds: int(gateHeartbeatInterval / time.Second),
	}
	if err := s.publishJSON(ctx, messagequeue.SubjectQualityGateRequest, gateReq); err != nil {
		slog.Error("quality gate request not published, failing the gate", "run_id", r.ID, "error", err)
		return s.failQualityGate(ctx, gated, gate, gateVerdict{reason: "request not published: " + err.Error()}, nil)
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
		return s.failQualityGate(ctx, r, &policy.QualityGate{}, gateVerdict{reason: unknownGateProfile(r.PolicyProfile)}, result)
	}
	if verdict := judgeGate(&profile.QualityGate, result); verdict.reason != "" {
		return s.failQualityGate(ctx, r, &profile.QualityGate, verdict, result)
	}

	// Announced only by the path whose record ends the run (runEnd.ended).
	announce := func(ctx context.Context) {
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
	}
	return s.finishRun(ctx, r, run.StatusCompleted, storedOutcome(r, run.StatusCompleted, ""),
		runEnd{agentWorked: true, ended: announce})
}

// unknownGateProfile is the failure of a gate whose policy profile no longer
// exists: which checks it requires is unknown, so it fails closed.
func unknownGateProfile(name string) string {
	return fmt.Sprintf("unknown policy profile %q, its quality gate cannot be checked", name)
}

// gateVerdict is why a gate fails ("" when it passed) and whether a check ran
// and failed: only then did the agent's work fail the gate. A gate that could
// not run (no command, a command that is not allowed or not found, a timeout,
// a lost request or result, a missing project or profile) fails the run too,
// but it is an infrastructure or configuration failure.
type gateVerdict struct {
	reason      string
	checkFailed bool
}

// judgeGate returns the verdict on a gate result. A required check without a
// result fails the gate (KI-29), as does a failed check or a gate error.
func judgeGate(gate *policy.QualityGate, result *messagequeue.QualityGateResultPayload) gateVerdict {
	var failed, notRun []string
	for _, c := range []struct {
		required        bool
		passed          *bool
		failed, missing string
	}{
		{gate.RequireTestsPass, result.TestsPassed, "tests failed", "no test result"},
		{gate.RequireLintPass, result.LintPassed, "lint failed", "no lint result"},
	} {
		switch {
		case c.passed != nil && !*c.passed:
			failed = append(failed, c.failed)
		case c.passed == nil && c.required && result.Error == "":
			notRun = append(notRun, c.missing)
		}
	}
	reasons := failed
	if result.Error != "" {
		reasons = append(reasons, result.Error)
	}
	reasons = append(reasons, notRun...)
	return gateVerdict{reason: strings.Join(reasons, ", "), checkFailed: len(failed) > 0}
}

// failQualityGate ends a run waiting for its gate as failed (D9), with the
// outcome stored on it and the verdict's reason; it is never delivered. Only
// a check that ran and failed counts against the agent and, if the policy
// says so, rolls the workspace back; both happen only on the path whose
// record ends the run (runEnd), as do the gate's audit entry, event and
// broadcast. result is the worker's gate result, nil when the gate did not
// run.
func (s *RuntimeService) failQualityGate(ctx context.Context, r *run.Run, gate *policy.QualityGate, verdict gateVerdict, result *messagequeue.QualityGateResultPayload) error {
	errMsg := "quality gate failed: " + verdict.reason
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
	announce := func(ctx context.Context) {
		s.appendAudit(ctx, r, "qualitygate.failed", errMsg)
		s.appendRunEvent(ctx, event.TypeQualityGateFailed, r, map[string]string{"error": errMsg})
		s.hub.BroadcastEvent(ctx, event.EventQualityGate, gateEvent)
	}
	if !verdict.checkFailed {
		slog.Warn("quality gate could not run, failing the run without rollback", "run_id", r.ID, "reason", verdict.reason)
	}
	return s.finishRun(ctx, r, run.StatusFailed, storedOutcome(r, run.StatusFailed, errMsg), runEnd{
		agentWorked: verdict.checkFailed,
		rollBack:    verdict.checkFailed && gate.RollbackOnGateFail,
		ended:       announce,
	})
}

// Liveness of a quality gate (KI-28, S3 review finding 4). A run waiting in
// quality_gate is alive while its updated_at is recent: EnterQualityGate sets
// it, and the worker refreshes it with a heartbeat every
// gateHeartbeatInterval while it runs the gate (HandleHeartbeat). A gate
// request that waits in the queue behind other gates, or for its redelivery
// after a worker died, sends no heartbeats; a lost one neither. The watchdog
// tells them apart with the backlog of the gate subjects (requests the
// workers have not settled, results the Go Core has not settled):
//   - no backlog: nothing is queued or being handled, so a gate silent for
//     gateSilenceLimit is lost (request or result lost, dead-lettered);
//   - a backlog, or no way to know: the gate may be queued; it is failed
//     only after qualityGateMaxWait.
//
// Failing a healthy queued gate is worse than holding a lost one longer, and
// the cap is what bounds a lost gate while other gates are queued.
const (
	gateHeartbeatInterval = 30 * time.Second
	// qualityGateMargin covers publishing, the Go Core's handling and the
	// store write around a heartbeat or a result.
	qualityGateMargin = time.Minute
	gateSilenceLimit  = 2*gateHeartbeatInterval + qualityGateMargin
	// gateDeliveries and gateAckWait are the delivery limits of the gate
	// request (ADR-016: MaxDeliver 4, AckWait 90 s).
	gateDeliveries     = 4
	gateAckWait        = 90 * time.Second
	qualityGateWaitCap = time.Hour
	// qualityGateEndTimeout bounds ending one stuck run: the watchdog's stop
	// does not abort an end under way (store writes, events, the plan's next
	// step), and waits at most this long for it.
	qualityGateEndTimeout = 2 * time.Minute
)

// staleRunBatch bounds the runs one watchdog sweep ends.
const staleRunBatch = 100

// qualityGateMaxWait is how long a gate may stay silent while gates are
// queued: at least qualityGateWaitCap, and never shorter than a gate that is
// delivered gateDeliveries times, each time running the test and the lint
// command into runtime.quality_gate_timeout and waiting gateAckWait for its
// redelivery.
func (s *RuntimeService) qualityGateMaxWait() time.Duration {
	perDelivery := 2*s.runtimeCfg.QualityGateTimeout + gateAckWait
	return max(qualityGateWaitCap, gateDeliveries*perDelivery+qualityGateMargin)
}

// gateBacklog returns how many gate requests and results are not settled
// yet; ok is false when that is unknown.
func (s *RuntimeService) gateBacklog(ctx context.Context) (queued int, ok bool) {
	if s.backlog == nil {
		return 0, false
	}
	for _, subject := range []string{messagequeue.SubjectQualityGateRequest, messagequeue.SubjectQualityGateResult} {
		n, err := s.backlog.Backlog(ctx, subject)
		if err != nil {
			slog.Warn("quality gate watchdog: backlog unknown, only the hard cap applies", "subject", subject, "error", err)
			return 0, false
		}
		queued += n
	}
	return queued, true
}

// FailStuckQualityGates fails the runs whose quality gate is lost (KI-28):
// see the liveness rules above. Each is ended as a gate that could not run
// through the gate result path (fresh status check, never delivered, no
// rollback, no agent failure), in its own tenant and with a context the
// watchdog's stop does not cancel; a run another replica or a late result
// ended meanwhile is skipped by the store's status predicate. A stop ends
// the sweep before the next run. It returns how many stuck runs it handled.
// Runs are found in the store, so the sweep also covers runs that entered
// their gate before a Go Core restart.
func (s *RuntimeService) FailStuckQualityGates(ctx context.Context) (int, error) {
	silentFor := s.qualityGateMaxWait()
	if queued, ok := s.gateBacklog(ctx); ok && queued == 0 {
		silentFor = gateSilenceLimit
	}
	stale, err := s.store.ListStaleRuns(ctx, run.StatusQualityGate, silentFor, staleRunBatch)
	if err != nil {
		return 0, fmt.Errorf("list runs stuck in quality_gate: %w", err)
	}
	handled := 0
	var errs []error
	for i := range stale {
		if ctx.Err() != nil {
			break // stopped: the remaining runs are left for the next sweep
		}
		r := &stale[i]
		slog.Warn("quality gate lost, failing the run", "run_id", r.ID, "silent_for", silentFor)
		if err := s.endStuckQualityGate(ctx, r, silentFor); err != nil {
			errs = append(errs, fmt.Errorf("run %s: %w", r.ID, err))
			continue
		}
		handled++
	}
	return handled, errors.Join(errs...)
}

func (s *RuntimeService) endStuckQualityGate(ctx context.Context, r *run.Run, silentFor time.Duration) error {
	endCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), qualityGateEndTimeout)
	defer cancel()
	return s.HandleQualityGateResult(endCtx, &messagequeue.QualityGateResultPayload{
		RunID:    r.ID,
		TenantID: r.TenantID,
		Error:    fmt.Sprintf("no quality gate result: the gate was not heard of for %s", silentFor),
	})
}
