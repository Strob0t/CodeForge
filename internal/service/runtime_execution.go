package service

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	cfotel "github.com/Strob0t/CodeForge/internal/adapter/otel"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/artifact"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

func (s *RuntimeService) HandleToolCallRequest(ctx context.Context, req *messagequeue.ToolCallRequestPayload) error {
	r, err := s.store.GetRun(ctx, req.RunID)
	if err != nil {
		// The run_id might be a conversation_id (agentic conversation mode
		// reuses the conversation ID as the run ID without creating a run record).
		// Fall back to conversation-based policy evaluation.
		return s.handleConversationToolCall(ctx, req)
	}

	if r.Status != run.StatusRunning {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), "run is not running")
	}

	// Load policy profile for termination checks
	profile, ok := s.policy.GetProfile(r.PolicyProfile)
	if !ok {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), "unknown policy profile")
	}

	// Check termination conditions
	if reason := s.checkTermination(r, &profile); reason != "" {
		// Terminate the run
		_ = s.store.CompleteRun(ctx, r.ID, run.StatusTimeout, "", reason, r.CostUSD, r.StepCount, r.TokensIn, r.TokensOut, r.Model)
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
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), reason)
	}

	// Evaluate policy with reason tracking
	call := policy.ToolCall{
		Tool:    req.Tool,
		Command: req.Command,
		Path:    req.Path,
	}
	result, err := s.policy.EvaluateWithReason(ctx, r.PolicyProfile, call)
	if err != nil {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), err.Error())
	}
	decision := result.Decision

	slog.Debug("policy evaluation",
		"run_id", req.RunID,
		"tool", req.Tool,
		"decision", result.Decision,
		"profile", result.Profile,
		"scope", result.Scope,
		"rule_index", result.RuleIndex,
		"reason", result.Reason,
	)

	// HITL: when policy says "ask", wait for user approval via WebSocket/HTTP.
	if decision == policy.DecisionAsk {
		decision = s.waitForApproval(ctx, r.ID, req.CallID, req.Tool, req.Command, req.Path)
		slog.Info("HITL approval resolved",
			"run_id", r.ID,
			"call_id", req.CallID,
			"tool", req.Tool,
			"decision", decision,
		)
	}

	// Record event
	evType := event.TypeToolCallApproved
	if decision != policy.DecisionAllow {
		evType = event.TypeToolCallDenied
		s.appendAudit(ctx, r, "policy.denied", fmt.Sprintf("Tool %q denied by policy %s (scope: %s, reason: %s)", req.Tool, result.Profile, result.Scope, result.Reason))
	}
	s.appendRunEvent(ctx, evType, r, map[string]string{
		"call_id":  req.CallID,
		"tool":     req.Tool,
		"decision": string(decision),
		"reason":   result.Reason,
	})

	// Broadcast WS
	phase := "approved"
	if decision != policy.DecisionAllow {
		phase = "denied"
	}
	s.hub.BroadcastEvent(ctx, ws.EventToolCallStatus, ws.ToolCallStatusEvent{
		RunID:    r.ID,
		CallID:   req.CallID,
		Tool:     req.Tool,
		Decision: string(decision),
		Phase:    phase,
	})

	// Broadcast AG-UI tool_call alongside native event
	s.hub.BroadcastEvent(ctx, ws.AGUIToolCall, ws.AGUIToolCallEvent{
		RunID:  r.ID,
		CallID: req.CallID,
		Name:   req.Tool,
		Args:   req.Command,
	})

	// OTEL: record tool call span and metric
	_, toolSpan := cfotel.StartToolCallSpan(ctx, req.CallID, req.Tool)
	toolSpan.SetAttributes(attribute.String("decision", string(decision)))
	toolSpan.End()
	if s.metrics != nil {
		s.metrics.ToolCalls.Add(ctx, 1, metric.WithAttributes(
			attribute.String("tool", req.Tool),
			attribute.String("decision", string(decision)),
		))
	}

	// Create checkpoint for file-modifying tools
	if s.checkpoint != nil && decision == policy.DecisionAllow && isFileModifyingTool(req.Tool) {
		proj, projErr := s.store.GetProject(ctx, r.ProjectID)
		if projErr == nil {
			if cpErr := s.checkpoint.CreateCheckpoint(ctx, r.ID, proj.WorkspacePath, req.Tool, req.CallID); cpErr != nil {
				slog.Warn("checkpoint creation failed", "run_id", r.ID, "error", cpErr)
			}
		}
	}

	// Increment step count
	newSteps := r.StepCount + 1
	_ = s.store.UpdateRunStatus(ctx, r.ID, run.StatusRunning, newSteps, r.CostUSD, r.TokensIn, r.TokensOut)

	return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(decision), "")
}

// handleConversationToolCall handles tool call requests for conversation-based runs
// that don't have a formal run record. It resolves the conversation's project to
// determine the policy profile, evaluates the policy, and supports HITL approval.
func (s *RuntimeService) handleConversationToolCall(ctx context.Context, req *messagequeue.ToolCallRequestPayload) error {
	conv, err := s.store.GetConversation(ctx, req.RunID)
	if err != nil {
		// Neither a run nor a conversation — likely a stale NATS message.
		// Deny silently to avoid log spam.
		slog.Debug("tool call request for unknown run/conversation", "run_id", req.RunID)
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), "unknown run")
	}
	if conv == nil {
		return fmt.Errorf("conversation not found: %s", req.RunID)
	}

	// Resolve policy profile from the conversation's project.
	policyProfile := ""
	proj, projErr := s.store.GetProject(ctx, conv.ProjectID)
	if projErr == nil {
		policyProfile = proj.PolicyProfile
	}

	// If no policy profile is set, allow by default (conversation mode).
	if policyProfile == "" {
		policyProfile = "default"
	}

	if _, ok := s.policy.GetProfile(policyProfile); !ok {
		// Unknown profile — allow the call to proceed rather than blocking.
		slog.Warn("unknown policy profile for conversation, allowing", "profile", policyProfile, "conversation_id", req.RunID)
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionAllow), "")
	}

	// Evaluate policy.
	call := policy.ToolCall{
		Tool:    req.Tool,
		Command: req.Command,
		Path:    req.Path,
	}
	result, err := s.policy.EvaluateWithReason(ctx, policyProfile, call)
	if err != nil {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), err.Error())
	}
	decision := result.Decision

	slog.Debug("conversation policy evaluation",
		"conversation_id", req.RunID,
		"tool", req.Tool,
		"decision", decision,
		"profile", result.Profile,
	)

	// HITL: when policy says "ask", wait for user approval via WebSocket/HTTP.
	if decision == policy.DecisionAsk {
		decision = s.waitForApproval(ctx, req.RunID, req.CallID, req.Tool, req.Command, req.Path)
		slog.Info("conversation HITL resolved",
			"conversation_id", req.RunID,
			"call_id", req.CallID,
			"tool", req.Tool,
			"decision", decision,
		)
	}

	// Broadcast WS tool call status.
	phase := "approved"
	if decision != policy.DecisionAllow {
		phase = "denied"
	}
	s.hub.BroadcastEvent(ctx, ws.EventToolCallStatus, ws.ToolCallStatusEvent{
		RunID:    req.RunID,
		CallID:   req.CallID,
		Tool:     req.Tool,
		Decision: string(decision),
		Phase:    phase,
	})

	return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(decision), "")
}

// HandleToolCallResult processes the outcome of an executed tool call.
func (s *RuntimeService) HandleToolCallResult(ctx context.Context, result *messagequeue.ToolCallResultPayload) error {
	r, err := s.store.GetRun(ctx, result.RunID)
	if err != nil {
		// Conversation-based runs don't have a run record.
		// Cost/token tracking for conversations happens via WebSocket events.
		slog.Debug("tool call result for conversation run", "run_id", result.RunID, "cost", result.CostUSD)
		return nil
	}

	// Accumulate cost and tokens
	newCost := r.CostUSD + result.CostUSD
	newTokensIn := r.TokensIn + result.TokensIn
	newTokensOut := r.TokensOut + result.TokensOut
	_ = s.store.UpdateRunStatus(ctx, r.ID, r.Status, r.StepCount, newCost, newTokensIn, newTokensOut)

	// Budget alert checks (80% and 90% thresholds) + post-execution budget enforcement
	profile, profileOK := s.policy.GetProfile(r.PolicyProfile)
	if profileOK && profile.Termination.MaxCost > 0 {
		maxCost := profile.Termination.MaxCost
		pct := (newCost / maxCost) * 100

		// Post-execution budget enforcement: terminate immediately if cost exceeds limit.
		// This catches the case where a single expensive tool call pushes cost over the
		// budget, rather than waiting for the next HandleToolCallRequest check.
		if newCost >= maxCost {
			reason := fmt.Sprintf("budget exceeded after tool execution ($%.2f/$%.2f)", newCost, maxCost)
			slog.Warn("post-execution budget exceeded, terminating run", "run_id", r.ID, "cost", newCost, "max_cost", maxCost)
			_ = s.store.CompleteRun(ctx, r.ID, run.StatusTimeout, "", reason, newCost, r.StepCount, newTokensIn, newTokensOut, r.Model)
			s.cleanupRunState(r.ID)
			s.appendRunEvent(ctx, event.TypeRunCompleted, r, map[string]string{
				"status": string(run.StatusTimeout),
				"reason": reason,
			})
			s.appendAudit(ctx, r, "budget.exceeded", reason)
			s.hub.BroadcastEvent(ctx, ws.EventRunStatus, ws.RunStatusEvent{
				RunID:     r.ID,
				TaskID:    r.TaskID,
				ProjectID: r.ProjectID,
				Status:    string(run.StatusTimeout),
				StepCount: r.StepCount,
				CostUSD:   newCost,
				TokensIn:  newTokensIn,
				TokensOut: newTokensOut,
			})
			_ = s.store.UpdateAgentStatus(ctx, r.AgentID, agent.StatusIdle)
			_ = s.store.UpdateTaskStatus(ctx, r.TaskID, task.StatusFailed)
			if s.onRunComplete != nil {
				s.onRunComplete(ctx, r.ID, run.StatusTimeout)
			}
			return nil
		}

		for _, threshold := range []float64{80, 90} {
			if pct >= threshold {
				alertKey := fmt.Sprintf("%s:%d", r.ID, int(threshold))
				if _, alreadySent := s.budgetAlerts.LoadOrStore(alertKey, true); !alreadySent {
					s.hub.BroadcastEvent(ctx, ws.EventBudgetAlert, ws.BudgetAlertEvent{
						RunID:      r.ID,
						TaskID:     r.TaskID,
						ProjectID:  r.ProjectID,
						CostUSD:    newCost,
						MaxCost:    maxCost,
						Percentage: pct,
					})
					slog.Warn("budget alert", "run_id", r.ID, "cost", newCost, "max_cost", maxCost, "pct", pct)
				}
			}
		}
	}

	// Check stall detection
	if tracker, ok := s.stallTrackers.Load(r.ID); ok {
		st := tracker.(*run.StallTracker)
		if st.RecordStep(result.Tool, result.Success, result.Output) {
			// Stall detected — terminate run
			slog.Warn("stall detected, terminating run", "run_id", r.ID, "tool", result.Tool)
			_ = s.store.CompleteRun(ctx, r.ID, run.StatusFailed, "", "stall detected: agent not making progress", newCost, r.StepCount, newTokensIn, newTokensOut, r.Model)
			s.stallTrackers.Delete(r.ID)
			s.appendRunEvent(ctx, event.TypeStallDetected, r, map[string]string{
				"tool":       result.Tool,
				"step_count": fmt.Sprintf("%d", r.StepCount),
			})
			s.hub.BroadcastEvent(ctx, ws.EventRunStatus, ws.RunStatusEvent{
				RunID:     r.ID,
				TaskID:    r.TaskID,
				ProjectID: r.ProjectID,
				Status:    string(run.StatusFailed),
				StepCount: r.StepCount,
				CostUSD:   newCost,
				TokensIn:  newTokensIn,
				TokensOut: newTokensOut,
			})
			// Set agent idle, task failed
			_ = s.store.UpdateAgentStatus(ctx, r.AgentID, agent.StatusIdle)
			_ = s.store.UpdateTaskStatus(ctx, r.TaskID, task.StatusFailed)
			return nil
		}
	}

	// Record event with per-tool token data
	s.appendRunEventWithTokens(ctx, event.TypeToolCallResultEv, r, map[string]string{
		"call_id": result.CallID,
		"tool":    result.Tool,
		"success": fmt.Sprintf("%t", result.Success),
		"cost":    fmt.Sprintf("%.6f", result.CostUSD),
	}, result.Tool, result.Model, result.TokensIn, result.TokensOut, result.CostUSD)

	// Broadcast WS with token data
	s.hub.BroadcastEvent(ctx, ws.EventToolCallStatus, ws.ToolCallStatusEvent{
		RunID:  r.ID,
		CallID: result.CallID,
		Tool:   result.Tool,
		Phase:  "result",
	})

	// Broadcast AG-UI tool_result alongside native event
	toolResultErr := ""
	if !result.Success {
		toolResultErr = result.Output
	}
	s.hub.BroadcastEvent(ctx, ws.AGUIToolResult, ws.AGUIToolResultEvent{
		RunID:  r.ID,
		CallID: result.CallID,
		Result: result.Output,
		Error:  toolResultErr,
	})

	return nil
}

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
			s.hub.BroadcastEvent(ctx, ws.EventArtifactValidation, ws.ArtifactValidationEvent{
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
		s.hub.BroadcastEvent(ctx, ws.EventQualityGate, ws.QualityGateEvent{
			RunID:     r.ID,
			TaskID:    r.TaskID,
			ProjectID: r.ProjectID,
			Status:    "started",
		})
		s.hub.BroadcastEvent(ctx, ws.EventRunStatus, ws.RunStatusEvent{
			RunID:     r.ID,
			TaskID:    r.TaskID,
			ProjectID: r.ProjectID,
			Status:    string(run.StatusQualityGate),
			StepCount: payload.StepCount,
			CostUSD:   payload.CostUSD,
			TokensIn:  payload.TokensIn,
			TokensOut: payload.TokensOut,
			Model:     payload.Model,
		})

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
		s.hub.BroadcastEvent(ctx, ws.EventQualityGate, ws.QualityGateEvent{
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
	s.hub.BroadcastEvent(ctx, ws.EventQualityGate, ws.QualityGateEvent{
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

// cleanupRunState removes heartbeat, stall tracker, and timeout goroutine for a run.
