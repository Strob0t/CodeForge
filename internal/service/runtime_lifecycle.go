package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/telemetry"
)

func (s *RuntimeService) cleanupRunState(runID string) {
	s.state.CleanupRun(runID)
}

// loadRunScoped loads the run a worker message refers to and returns ctx
// scoped to the run's tenant. Worker messages arrive without a request
// tenant: the tenant the worker echoes selects the tenant for the lookup, and
// the stored run is authoritative for everything that follows (store writes,
// WebSocket events, follow-up runs). The returned ctx carries the payload
// tenant even when the lookup fails.
func (s *RuntimeService) loadRunScoped(ctx context.Context, runID, payloadTenant string) (context.Context, *run.Run, error) {
	ctx = withPayloadTenant(ctx, payloadTenant)
	r, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return ctx, nil, err
	}
	return withEntityTenant(ctx, r.TenantID), r, nil
}

// skipEndedRun drops the domain.ErrConflict with which the store refuses to
// update a run that already ended (KI-31): the path that ended the run did
// the work, so the caller skips it instead of failing. Other errors pass.
func skipEndedRun(ctx context.Context, err error, op, runID string) error {
	if errors.Is(err, domain.ErrConflict) {
		slog.InfoContext(ctx, "run already ended, skipped", "operation", op, "run_id", runID)
		return nil
	}
	return err
}

// logRunUpdate logs the error of a best-effort run update; a run that already
// ended is skipped (skipEndedRun).
func logRunUpdate(ctx context.Context, err error, op, runID string) {
	logBestEffort(ctx, skipEndedRun(ctx, err, op, runID), op, slog.String("run_id", runID))
}

// cancelRunWithReason cancels a run with a specific reason message (used by timeout goroutine).
func (s *RuntimeService) cancelRunWithReason(ctx context.Context, runID, reason string) error {
	r, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}
	if r.Status != run.StatusRunning && r.Status != run.StatusPending {
		return nil // already completed
	}
	return s.stopRun(ctx, r, run.StatusTimeout, reason)
}

// runCancelPayload is the runs.cancel message that tells the worker to stop a run.
type runCancelPayload struct {
	RunID string `json:"run_id"`
}

// workerStopTimeout bounds the runs.cancel publish of a stop.
const workerStopTimeout = 5 * time.Second

// stopRun ends a run that the control plane terminates while the worker still
// executes it: user cancel, context-level timeout, termination limits, the
// post-execution budget and stall detection. The worker is told to stop
// first, then the run goes through the same completion path as a run the
// worker finished (KI-30), with status and reason as its end and the outcome
// it has (output and model of a run waiting for its gate, usage) kept.
//
// While the stop is under way the run is marked stopping: the completion the
// worker sends when it stops only raises the usage totals (HandleRunComplete),
// so the run ends with the stop's status and reason, not the worker's
// "cancelled".
func (s *RuntimeService) stopRun(ctx context.Context, r *run.Run, status run.Status, reason string) error {
	s.state.BeginStop(r.ID)
	defer s.state.EndStop(r.ID)
	s.tellWorkerToStop(ctx, r.ID)
	return s.finalizeRun(ctx, r, status, storedOutcome(r, status, reason))
}

// tellWorkerToStop publishes runs.cancel before the run is completed: the
// worker stops editing the workspace before checkpoints are cleaned up and
// the next plan step starts, and it stops even when the run record cannot be
// completed. The publish does not depend on the caller's context (a
// disconnected HTTP client must not keep the worker running); it keeps the
// context's values (tenant) and has its own timeout.
func (s *RuntimeService) tellWorkerToStop(ctx context.Context, runID string) {
	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workerStopTimeout)
	defer cancel()
	logBestEffort(pubCtx, s.publishJSON(pubCtx, messagequeue.SubjectRunCancel, runCancelPayload{RunID: runID}),
		"publishJSON", slog.String("subject", messagequeue.SubjectRunCancel), slog.String("run_id", runID))
}

// finalizeRun is the one completion path of a run, whichever way it ended:
// run state cleanup, the terminal run record, task and agent reset, events,
// WebSocket broadcasts, checkpoint and sandbox cleanup, the audit entry and
// onRunComplete (execution-plan progress). When the run record cannot be
// completed, nothing after it happens.
func (s *RuntimeService) finalizeRun(ctx context.Context, r *run.Run, status run.Status, payload *messagequeue.RunCompletePayload) error {
	return s.endRun(ctx, r, status, payload, true)
}

// endRun is finalizeRun; agentWorked tells whether the run's end is an
// outcome of the agent's work that its statistics record (a run that could
// not be started is not).
func (s *RuntimeService) endRun(ctx context.Context, r *run.Run, status run.Status, payload *messagequeue.RunCompletePayload, agentWorked bool) error {
	// OTEL: annotate run span before cleanup ends it
	if sp, ok := s.state.GetRunSpan(r.ID); ok {
		sp.SetAttributes(
			attribute.String("status", string(status)),
			attribute.Int64("steps", int64(payload.StepCount)),
			attribute.Float64("cost_usd", payload.CostUSD),
		)
		if status == run.StatusFailed || status == run.StatusTimeout {
			sp.SetStatus(codes.Error, payload.Error)
		}
	}

	// The terminal record comes first, then the run state is released: a HITL
	// waiter woken before the record commits would count a step on a still
	// running run and record a policy denial. A conflict means another path
	// ended the run; the state this process holds for it is released as well.
	if err := s.store.CompleteRun(ctx, &run.CompletionRequest{ID: r.ID, Status: status, Output: payload.Output, Error: payload.Error, CostUSD: payload.CostUSD, StepCount: payload.StepCount, TokensIn: payload.TokensIn, TokensOut: payload.TokensOut, Model: payload.Model}); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			s.cleanupRunState(r.ID)
		}
		return fmt.Errorf("complete run: %w", err)
	}
	s.cleanupRunState(r.ID)

	if s.metrics != nil {
		metricAttrs := []string{"project.id", r.ProjectID, "status", string(status)}
		if status == run.StatusCompleted {
			s.metrics.RecordRunCompleted(ctx, metricAttrs...)
		} else {
			s.metrics.RecordRunFailed(ctx, metricAttrs...)
		}
		s.metrics.RecordRunCost(ctx, payload.CostUSD, metricAttrs...)
	}

	// The task's result and the status the run leaves it in, in one write.
	taskResult := task.Result{
		Output: payload.Output,
		Error:  payload.Error,
	}
	logBestEffort(ctx, s.store.UpdateTaskResult(ctx, r.TaskID, taskStatusForRun(status), taskResult, payload.CostUSD), "UpdateTaskResult", slog.String("task_id", r.TaskID))

	// Set agent back to idle
	logBestEffort(ctx, s.store.UpdateAgentStatus(ctx, r.AgentID, agent.StatusIdle), "UpdateAgentStatus", slog.String("agent_id", r.AgentID))

	// Agent identity stats (Phase 23C) record how the agent's runs turned out.
	// A cancel is the user's (or the plan's) decision and a failed start an
	// infrastructure failure, not outcomes of the agent's work, and are not
	// counted; timeouts and stalls are failures.
	if agentWorked && status != run.StatusCancelled {
		if err := s.store.IncrementAgentStats(ctx, r.AgentID, payload.CostUSD, status == run.StatusCompleted); err != nil {
			slog.Warn("failed to increment agent stats", "agent_id", r.AgentID, "error", err)
		}
	}

	// Record event
	s.appendRunEvent(ctx, event.TypeRunCompleted, r, map[string]string{
		"status":     string(status),
		"step_count": fmt.Sprintf("%d", payload.StepCount),
		"cost":       fmt.Sprintf("%.6f", payload.CostUSD),
		"error":      payload.Error,
	})

	// Broadcast WS
	finalRun := *r
	finalRun.StepCount = payload.StepCount
	finalRun.CostUSD = payload.CostUSD
	finalRun.TokensIn = payload.TokensIn
	finalRun.TokensOut = payload.TokensOut
	finalRun.Model = payload.Model
	s.broadcastRunStatus(ctx, &finalRun, status)
	s.hub.BroadcastEvent(ctx, event.EventAgentStatus, event.AgentStatusEvent{
		AgentID:   r.AgentID,
		ProjectID: r.ProjectID,
		Status:    string(agent.StatusIdle),
	})

	// Broadcast AG-UI run_finished alongside native event
	aguiStatus := "completed"
	switch status {
	case run.StatusFailed, run.StatusTimeout:
		aguiStatus = "failed"
	case run.StatusCancelled:
		aguiStatus = "cancelled"
	}
	s.hub.BroadcastEvent(ctx, event.AGUIRunFinished, event.AGUIRunFinishedEvent{
		RunID:     r.ID,
		Status:    aguiStatus,
		Model:     payload.Model,
		CostUSD:   payload.CostUSD,
		TokensIn:  payload.TokensIn,
		TokensOut: payload.TokensOut,
		Steps:     payload.StepCount,
	})

	// Clean up checkpoints (remove shadow commits, keep working state)
	if s.checkpoint != nil {
		proj, projErr := s.store.GetProject(ctx, r.ProjectID)
		if projErr == nil {
			if cpErr := s.checkpoint.CleanupCheckpoints(ctx, r.ID, proj.WorkspacePath); cpErr != nil {
				slog.Warn("checkpoint cleanup failed", "run_id", r.ID, "error", cpErr)
			}
		}
	}

	// Clean up sandbox
	if s.sandbox != nil {
		if _, ok := s.sandbox.Get(r.ID); ok {
			if err := s.sandbox.Stop(ctx, r.ID); err != nil {
				slog.Warn("sandbox stop failed", "run_id", r.ID, "error", err)
			}
			if err := s.sandbox.Remove(ctx, r.ID); err != nil {
				slog.Warn("sandbox remove failed", "run_id", r.ID, "error", err)
			}
		}
	}

	// Audit trail
	auditAction := "run.completed"
	if status == run.StatusCancelled {
		auditAction = "run.cancelled"
	}
	auditDetails := fmt.Sprintf("Run finalized with status %s, %d steps, cost $%.4f", status, payload.StepCount, payload.CostUSD)
	if payload.Error != "" {
		auditDetails += ": " + payload.Error
	}
	s.appendAudit(ctx, r, auditAction, auditDetails)

	slog.Info("run finalized", "run_id", r.ID, "status", status, "steps", payload.StepCount)

	// Notify orchestrator (if registered) about run completion
	if s.onRunComplete != nil {
		s.onRunComplete(ctx, r.ID, status)
	}

	return nil
}

// taskStatusForRun maps the terminal status of a run to the status of its task.
func taskStatusForRun(status run.Status) task.Status {
	switch status {
	case run.StatusFailed, run.StatusTimeout:
		return task.StatusFailed
	case run.StatusCancelled:
		return task.StatusCancelled
	default:
		return task.StatusCompleted
	}
}

// triggerDelivery attempts to deliver the run output (patch, commit, branch, PR).
// Delivery is best-effort — failure is logged but does not fail the run.
func (s *RuntimeService) triggerDelivery(ctx context.Context, r *run.Run) {
	if r.DeliverMode == "" || r.DeliverMode == run.DeliverModeNone {
		return
	}
	if s.deliver == nil {
		slog.Warn("deliver service not configured, skipping delivery", "run_id", r.ID)
		return
	}

	// OTEL: delivery span
	_, deliverySpan := telemetry.StartDeliverySpan(ctx, r.ID, string(r.DeliverMode))
	defer deliverySpan.End()

	// Get task title for commit message
	t, err := s.store.GetTask(ctx, r.TaskID)
	taskTitle := r.TaskID
	if err == nil {
		taskTitle = t.Title
	}

	s.appendRunEvent(ctx, event.TypeDeliveryStarted, r, map[string]string{
		"mode": string(r.DeliverMode),
	})
	s.hub.BroadcastEvent(ctx, event.EventDelivery, event.DeliveryEvent{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    "started",
		Mode:      string(r.DeliverMode),
	})

	deliverResult, deliverErr := s.deliver.Deliver(ctx, r, taskTitle)
	if deliverErr != nil {
		deliverySpan.SetStatus(codes.Error, deliverErr.Error())
		s.appendAudit(ctx, r, "delivery.failed", fmt.Sprintf("Delivery mode %s failed: %s", r.DeliverMode, deliverErr.Error()))
		slog.Error("delivery failed", "run_id", r.ID, "mode", r.DeliverMode, "error", deliverErr)
		s.appendRunEvent(ctx, event.TypeDeliveryFailed, r, map[string]string{
			"mode":  string(r.DeliverMode),
			"error": deliverErr.Error(),
		})
		s.hub.BroadcastEvent(ctx, event.EventDelivery, event.DeliveryEvent{
			RunID:     r.ID,
			TaskID:    r.TaskID,
			ProjectID: r.ProjectID,
			Status:    "failed",
			Mode:      string(r.DeliverMode),
			Error:     deliverErr.Error(),
		})
		return
	}

	deliverySpan.SetAttributes(
		attribute.String("delivery.status", "completed"),
		attribute.String("delivery.branch", deliverResult.BranchName),
	)
	s.appendAudit(ctx, r, "delivery.completed", fmt.Sprintf("Delivery mode %s completed (branch: %s, PR: %s)", deliverResult.Mode, deliverResult.BranchName, deliverResult.PRURL))
	s.appendRunEvent(ctx, event.TypeDeliveryCompleted, r, map[string]string{
		"mode":        string(deliverResult.Mode),
		"patch_path":  deliverResult.PatchPath,
		"commit_hash": deliverResult.CommitHash,
		"branch_name": deliverResult.BranchName,
		"pr_url":      deliverResult.PRURL,
	})
	s.hub.BroadcastEvent(ctx, event.EventDelivery, event.DeliveryEvent{
		RunID:      r.ID,
		TaskID:     r.TaskID,
		ProjectID:  r.ProjectID,
		Status:     "completed",
		Mode:       string(deliverResult.Mode),
		PatchPath:  deliverResult.PatchPath,
		CommitHash: deliverResult.CommitHash,
		BranchName: deliverResult.BranchName,
		PRURL:      deliverResult.PRURL,
	})
}

// --- Internal helpers ---

// AbsoluteMaxExecutionTimeout is the hard upper bound for any run, regardless of
// policy configuration. It acts as a safety net when stall detection is disabled
// and no TimeoutSeconds is configured. Even if all other termination conditions
// are set to 0 (unlimited), this ensures no run can exceed 1 hour of wall time.
//
// Defense-in-depth relationship with other safety layers:
//   - MaxSteps (policy): checked per tool call, capped at 10,000 (MaxStepsLimit)
//   - TimeoutSeconds (policy): per-policy configurable timeout
//   - MaxCost (policy): budget-based termination
//   - HeartbeatTimeout (config): kills unresponsive workers (default: 120s)
//   - StallDetection (policy): detects non-progress loops
//   - AbsoluteMaxExecutionTimeout: final fallback, always enforced
const AbsoluteMaxExecutionTimeout = 3600 * time.Second

func (s *RuntimeService) checkTermination(r *run.Run, profile *policy.PolicyProfile) string {
	tc := profile.Termination

	if tc.MaxSteps > 0 && r.StepCount >= tc.MaxSteps {
		return fmt.Sprintf("max steps reached (%d/%d)", r.StepCount, tc.MaxSteps)
	}
	if tc.MaxCost > 0 && r.CostUSD >= tc.MaxCost {
		return fmt.Sprintf("max cost reached ($%.2f/$%.2f)", r.CostUSD, tc.MaxCost)
	}

	elapsed := time.Since(r.StartedAt)

	if tc.TimeoutSeconds > 0 {
		if elapsed >= time.Duration(tc.TimeoutSeconds)*time.Second {
			return fmt.Sprintf("timeout reached (%s/%ds)", elapsed.Truncate(time.Second), tc.TimeoutSeconds)
		}
	}

	// Absolute safety net: enforce hard timeout regardless of policy configuration.
	if elapsed >= AbsoluteMaxExecutionTimeout {
		return fmt.Sprintf("absolute execution timeout reached (%s)", elapsed.Truncate(time.Second))
	}

	// Check heartbeat timeout
	if s.runtimeCfg.HeartbeatTimeout > 0 {
		if lastHB, ok := s.state.GetHeartbeat(r.ID); ok {
			if time.Since(lastHB) > s.runtimeCfg.HeartbeatTimeout {
				return "heartbeat timeout (worker unresponsive)"
			}
		}
	}

	return ""
}

func (s *RuntimeService) sendToolCallResponse(ctx context.Context, runID, callID, decision, reason string) error {
	resp := messagequeue.ToolCallResponsePayload{
		RunID:    runID,
		CallID:   callID,
		Decision: decision,
		Reason:   reason,
	}

	// For hybrid runs, include exec_mode and container_id so the worker
	// can route file operations to the host and commands to the container.
	if s.sandbox != nil {
		if sb, ok := s.sandbox.Get(runID); ok {
			r, err := s.store.GetRun(ctx, runID)
			if err == nil && r.ExecMode == run.ExecModeHybrid {
				resp.ExecMode = string(run.ExecModeHybrid)
				resp.ContainerID = sb.ContainerID
			}
		}
	}

	return s.publishJSON(ctx, messagequeue.SubjectRunToolCallResponse, resp)
}

func (s *RuntimeService) publishJSON(ctx context.Context, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return s.queue.Publish(ctx, subject, data)
}

func (s *RuntimeService) appendRunEvent(ctx context.Context, evType event.Type, r *run.Run, payload map[string]string) {
	s.appendRunEventWithTokens(ctx, evType, r, payload, "", "", 0, 0, 0)
}

func (s *RuntimeService) appendRunEventWithTokens(ctx context.Context, evType event.Type, r *run.Run, payload map[string]string, toolName, model string, tokensIn, tokensOut int64, costUSD float64) {
	if s.events == nil {
		return
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal run event payload", "error", err)
		return
	}
	ev := event.AgentEvent{
		AgentID:   r.AgentID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		RunID:     r.ID,
		Type:      evType,
		Payload:   payloadJSON,
		RequestID: logger.RequestID(ctx),
		Version:   1,
		ToolName:  toolName,
		Model:     model,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
		CostUSD:   costUSD,
	}
	if err := s.events.Append(ctx, &ev); err != nil {
		slog.Error("failed to append run event", "type", evType, "run_id", r.ID, "error", err)
	}
}

// broadcastToolCallStatus broadcasts a ToolCallStatusEvent to all connected WebSocket clients.
// The decision parameter is optional (empty for "result" phase).
func (s *RuntimeService) broadcastToolCallStatus(ctx context.Context, runID, callID, tool, phase, decision string) {
	s.hub.BroadcastEvent(ctx, event.EventToolCallStatus, event.ToolCallStatusEvent{
		RunID:    runID,
		CallID:   callID,
		Tool:     tool,
		Decision: decision,
		Phase:    phase,
	})
}

// decisionPhase maps a policy decision to the corresponding broadcast phase string.
func decisionPhase(d policy.Decision) string {
	if d == policy.DecisionAllow {
		return "approved"
	}
	return "denied"
}

// broadcastRunStatus broadcasts a RunStatusEvent to all connected clients.
// Centralizes the 8+ occurrences of this pattern across runtime_execution.go,
// runtime.go, and runtime_lifecycle.go.
func (s *RuntimeService) broadcastRunStatus(ctx context.Context, r *run.Run, status run.Status) {
	s.hub.BroadcastEvent(ctx, event.EventRunStatus, event.RunStatusEvent{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		AgentID:   r.AgentID,
		Status:    string(status),
		StepCount: r.StepCount,
		CostUSD:   r.CostUSD,
		TokensIn:  r.TokensIn,
		TokensOut: r.TokensOut,
		Model:     r.Model,
	})
}

// appendAudit records an entry in the audit trail table for compliance and debugging.
// This is separate from agent_events — the audit trail captures high-level lifecycle
// actions (run start/complete, policy denials, quality gate outcomes, delivery, cancel).
func (s *RuntimeService) appendAudit(ctx context.Context, r *run.Run, action, details string) {
	if s.events == nil {
		return
	}
	entry := &event.AuditEntry{
		ProjectID: r.ProjectID,
		RunID:     r.ID,
		AgentID:   r.AgentID,
		Action:    action,
		Details:   details,
	}
	if err := s.events.AppendAudit(ctx, entry); err != nil {
		slog.Error("failed to append audit entry", "action", action, "run_id", r.ID, "error", err)
	}
}

// toContextEntryPayloads converts domain context entries to NATS payload entries.
func toContextEntryPayloads(entries []cfcontext.ContextEntry) []messagequeue.ContextEntryPayload {
	out := make([]messagequeue.ContextEntryPayload, len(entries))
	for i, e := range entries {
		out[i] = messagequeue.ContextEntryPayload{
			Kind:     string(e.Kind),
			Path:     e.Path,
			Content:  e.Content,
			Tokens:   e.Tokens,
			Priority: e.Priority,
		}
	}
	return out
}

// isFileModifyingTool returns true for tools that change files on disk:
// Edit, Write and Bash under any worker or Claude Code name.
func isFileModifyingTool(tool string) bool {
	switch policy.CanonicalTool(tool) {
	case policy.ToolEdit, policy.ToolWrite, policy.ToolBash, "execute":
		return true
	}
	return false
}

func cancelAll(fns []func()) {
	for _, fn := range fns {
		fn()
	}
}
