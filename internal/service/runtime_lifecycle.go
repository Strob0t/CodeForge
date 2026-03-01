package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	cfotel "github.com/Strob0t/CodeForge/internal/adapter/otel"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

func (s *RuntimeService) cleanupRunState(runID string) {
	s.heartbeats.Delete(runID)
	s.stallTrackers.Delete(runID)
	if cancel, ok := s.runTimeouts.LoadAndDelete(runID); ok {
		cancel.(context.CancelFunc)()
	}
	if span, ok := s.runSpans.LoadAndDelete(runID); ok {
		span.(trace.Span).End()
	}
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

	// OTEL: annotate run span before cleanup ends it
	if span, ok := s.runSpans.Load(runID); ok {
		sp := span.(trace.Span)
		sp.SetAttributes(attribute.String("cancel.reason", reason))
		sp.SetStatus(codes.Error, reason)
	}
	if s.metrics != nil {
		s.metrics.RunsFailed.Add(ctx, 1, metric.WithAttributes(
			attribute.String("project.id", r.ProjectID),
			attribute.String("status", "timeout"),
		))
	}

	s.cleanupRunState(runID)

	if err := s.store.CompleteRun(ctx, r.ID, run.StatusTimeout, "", reason, r.CostUSD, r.StepCount, r.TokensIn, r.TokensOut, r.Model); err != nil {
		return fmt.Errorf("complete run: %w", err)
	}
	_ = s.store.UpdateAgentStatus(ctx, r.AgentID, agent.StatusIdle)
	_ = s.store.UpdateTaskStatus(ctx, r.TaskID, task.StatusFailed)

	cancelPayload := struct {
		RunID string `json:"run_id"`
	}{RunID: runID}
	_ = s.publishJSON(ctx, messagequeue.SubjectRunCancel, cancelPayload)

	s.appendRunEvent(ctx, event.TypeRunCompleted, r, map[string]string{
		"status": string(run.StatusTimeout),
		"reason": reason,
	})
	s.hub.BroadcastEvent(ctx, ws.EventRunStatus, ws.RunStatusEvent{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    string(run.StatusTimeout),
		StepCount: r.StepCount,
		CostUSD:   r.CostUSD,
		TokensIn:  r.TokensIn,
		TokensOut: r.TokensOut,
		Model:     r.Model,
	})

	if s.onRunComplete != nil {
		s.onRunComplete(ctx, r.ID, run.StatusTimeout)
	}
	return nil
}

// finalizeRun completes the run lifecycle: update DB, task, agent, broadcast events.
func (s *RuntimeService) finalizeRun(ctx context.Context, r *run.Run, status run.Status, payload *messagequeue.RunCompletePayload) error {
	// OTEL: annotate run span before cleanup ends it
	if span, ok := s.runSpans.Load(r.ID); ok {
		sp := span.(trace.Span)
		sp.SetAttributes(
			attribute.String("status", string(status)),
			attribute.Int64("steps", int64(payload.StepCount)),
			attribute.Float64("cost_usd", payload.CostUSD),
		)
		if status == run.StatusFailed || status == run.StatusTimeout {
			sp.SetStatus(codes.Error, payload.Error)
		}
	}
	if s.metrics != nil {
		attrs := metric.WithAttributes(
			attribute.String("project.id", r.ProjectID),
			attribute.String("status", string(status)),
		)
		if status == run.StatusCompleted {
			s.metrics.RunsCompleted.Add(ctx, 1, attrs)
		} else {
			s.metrics.RunsFailed.Add(ctx, 1, attrs)
		}
		s.metrics.RunCost.Record(ctx, payload.CostUSD, attrs)
	}

	s.cleanupRunState(r.ID)

	if err := s.store.CompleteRun(ctx, r.ID, status, payload.Output, payload.Error, payload.CostUSD, payload.StepCount, payload.TokensIn, payload.TokensOut, payload.Model); err != nil {
		return fmt.Errorf("complete run: %w", err)
	}

	// Update task result
	taskResult := task.Result{
		Output: payload.Output,
		Error:  payload.Error,
	}
	taskStatus := task.StatusCompleted
	if status == run.StatusFailed || status == run.StatusTimeout {
		taskStatus = task.StatusFailed
	}
	_ = s.store.UpdateTaskStatus(ctx, r.TaskID, taskStatus)
	_ = s.store.UpdateTaskResult(ctx, r.TaskID, taskResult, payload.CostUSD)

	// Set agent back to idle
	_ = s.store.UpdateAgentStatus(ctx, r.AgentID, agent.StatusIdle)

	// Clean up budget alerts for this run
	s.budgetAlerts.Delete(fmt.Sprintf("%s:80", r.ID))
	s.budgetAlerts.Delete(fmt.Sprintf("%s:90", r.ID))

	// Record event
	s.appendRunEvent(ctx, event.TypeRunCompleted, r, map[string]string{
		"status":     string(status),
		"step_count": fmt.Sprintf("%d", payload.StepCount),
		"cost":       fmt.Sprintf("%.6f", payload.CostUSD),
		"error":      payload.Error,
	})

	// Broadcast WS
	s.hub.BroadcastEvent(ctx, ws.EventRunStatus, ws.RunStatusEvent{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    string(status),
		StepCount: payload.StepCount,
		CostUSD:   payload.CostUSD,
		TokensIn:  payload.TokensIn,
		TokensOut: payload.TokensOut,
		Model:     payload.Model,
	})
	s.hub.BroadcastEvent(ctx, ws.EventAgentStatus, ws.AgentStatusEvent{
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
	s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
		RunID:  r.ID,
		Status: aguiStatus,
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
	s.appendAudit(ctx, r, "run.completed", fmt.Sprintf("Run finalized with status %s, %d steps, cost $%.4f", status, payload.StepCount, payload.CostUSD))

	slog.Info("run finalized", "run_id", r.ID, "status", status, "steps", payload.StepCount)

	// Notify orchestrator (if registered) about run completion
	if s.onRunComplete != nil {
		s.onRunComplete(ctx, r.ID, status)
	}

	return nil
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
	_, deliverySpan := cfotel.StartDeliverySpan(ctx, r.ID, string(r.DeliverMode))
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
	s.hub.BroadcastEvent(ctx, ws.EventDelivery, ws.DeliveryEvent{
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
		s.hub.BroadcastEvent(ctx, ws.EventDelivery, ws.DeliveryEvent{
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
	s.hub.BroadcastEvent(ctx, ws.EventDelivery, ws.DeliveryEvent{
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

// CancelRun cancels a running run and notifies the worker.

// --- Internal helpers ---

func (s *RuntimeService) checkTermination(r *run.Run, profile *policy.PolicyProfile) string {
	tc := profile.Termination

	if tc.MaxSteps > 0 && r.StepCount >= tc.MaxSteps {
		return fmt.Sprintf("max steps reached (%d/%d)", r.StepCount, tc.MaxSteps)
	}
	if tc.MaxCost > 0 && r.CostUSD >= tc.MaxCost {
		return fmt.Sprintf("max cost reached ($%.2f/$%.2f)", r.CostUSD, tc.MaxCost)
	}
	if tc.TimeoutSeconds > 0 {
		elapsed := time.Since(r.StartedAt)
		if elapsed >= time.Duration(tc.TimeoutSeconds)*time.Second {
			return fmt.Sprintf("timeout reached (%s/%ds)", elapsed.Truncate(time.Second), tc.TimeoutSeconds)
		}
	}

	// Check heartbeat timeout
	if s.runtimeCfg.HeartbeatTimeout > 0 {
		if lastHB, ok := s.heartbeats.Load(r.ID); ok {
			if time.Since(lastHB.(time.Time)) > s.runtimeCfg.HeartbeatTimeout {
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

// isFileModifyingTool returns true for tools that change files on disk.
func isFileModifyingTool(tool string) bool {
	switch tool {
	case "Edit", "Write", "Bash", "execute", "write_file", "edit_file":
		return true
	}
	return false
}

func cancelAll(fns []func()) {
	for _, fn := range fns {
		fn()
	}
}
