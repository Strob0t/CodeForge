package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/telemetry"
)

func (s *RuntimeService) HandleToolCallRequest(ctx context.Context, req *messagequeue.ToolCallRequestPayload) error {
	start := time.Now()
	defer func() {
		slog.Info("HandleToolCallRequest completed",
			"run_id", req.RunID,
			"call_id", req.CallID,
			"tool", req.Tool,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}()

	ctx, r, err := s.loadRunScoped(ctx, req.RunID, req.TenantID)
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
		logBestEffort(ctx, s.stopRun(ctx, r, run.StatusTimeout, reason), "stopRun", slog.String("run_id", r.ID))
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), reason)
	}

	// Evaluate policy with reason tracking. Paths are resolved against the
	// project workspace and the run's mode restricts the tools it may use.
	workspace := ""
	proj, projErr := s.store.GetProject(ctx, r.ProjectID)
	if projErr == nil {
		workspace = proj.WorkspacePath
	}
	m, modeErr := s.resolveMode(r.ModeID)
	if modeErr != nil {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), modeErr.Error())
	}
	call := policy.ToolCall{
		Tool:    req.Tool,
		Command: req.Command,
		Path:    req.Path,
	}
	result, err := s.policy.EvaluateWithReason(ctx, r.PolicyProfile, call, policyEvalOptions(workspace, m)...)
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

	// HITL: when policy says "ask", check if the profile mode allows auto-approval.
	if decision == policy.DecisionAsk {
		if profile.Mode == policy.ModeAcceptEdits || profile.Mode == policy.ModeDelegate {
			decision = policy.DecisionAllow
			slog.Info("HITL auto-approved (full-auto profile)",
				"run_id", r.ID,
				"call_id", req.CallID,
				"tool", req.Tool,
				"profile", r.PolicyProfile,
			)
		} else {
			decision = s.waitForApproval(ctx, r.ID, req.CallID, req.Tool, req.Command, req.Path)
			slog.Info("HITL approval resolved",
				"run_id", r.ID,
				"call_id", req.CallID,
				"tool", req.Tool,
				"decision", decision,
			)
		}
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
	s.broadcastToolCallStatus(ctx, r.ID, req.CallID, req.Tool, decisionPhase(decision), string(decision))

	// Broadcast AG-UI tool_call alongside native event
	s.hub.BroadcastEvent(ctx, event.AGUIToolCall, event.AGUIToolCallEvent{
		RunID:  r.ID,
		CallID: req.CallID,
		Name:   req.Tool,
		Args:   req.Command,
	})

	// OTEL: record tool call span and metric
	_, toolSpan := telemetry.StartToolCallSpan(ctx, req.CallID, req.Tool)
	toolSpan.SetAttributes(attribute.String("decision", string(decision)))
	toolSpan.End()
	if s.metrics != nil {
		s.metrics.RecordToolCall(ctx, "tool", req.Tool, "decision", string(decision))
	}

	// Create checkpoint for file-modifying tools
	if s.checkpoint != nil && decision == policy.DecisionAllow && isFileModifyingTool(req.Tool) && projErr == nil {
		if cpErr := s.checkpoint.CreateCheckpoint(ctx, r.ID, proj.WorkspacePath, req.Tool, req.CallID); cpErr != nil {
			slog.Warn("checkpoint creation failed", "run_id", r.ID, "error", cpErr)
		}
	}

	// Increment step count
	newSteps := r.StepCount + 1
	logBestEffort(ctx, s.store.UpdateRunStatus(ctx, r.ID, run.StatusRunning, newSteps, r.CostUSD, r.TokensIn, r.TokensOut), "UpdateRunStatus", slog.String("run_id", r.ID))

	return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(decision), denialReason(decision, result))
}

// handleConversationToolCall handles tool call requests for conversation-based runs
// that don't have a formal run record. It resolves the conversation's project to
// determine the policy profile, evaluates the policy, and supports HITL approval.
func (s *RuntimeService) handleConversationToolCall(ctx context.Context, req *messagequeue.ToolCallRequestPayload) error {
	start := time.Now()
	defer func() {
		slog.Info("handleConversationToolCall completed",
			"conversation_id", req.RunID,
			"call_id", req.CallID,
			"tool", req.Tool,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}()

	// Fast-reject: if this conversation run was cancelled, deny immediately.
	if s.state.IsConversationCancelled(req.RunID) {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), "conversation run cancelled")
	}

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
	ctx = withEntityTenant(ctx, conv.TenantID)

	proj, err := s.store.GetProject(ctx, conv.ProjectID)
	if err != nil {
		slog.Warn("project of conversation not found, denying tool call", "conversation_id", req.RunID, "project_id", conv.ProjectID, "error", err)
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), "conversation project not found")
	}

	// The worker reports the mode it was started with; without it, resolve the
	// mode the same way the dispatch does.
	modeID := req.ModeID
	if modeID == "" {
		modeID = conv.Mode
	}
	if modeID == "" {
		modeID = defaultConversationMode
	}
	m, modeErr := s.resolveMode(modeID)
	if modeErr != nil {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), modeErr.Error())
	}
	modeAutonomy := 0
	if m != nil {
		modeAutonomy = m.Autonomy
	}
	policyProfile := conversationPolicyProfile(proj, modeAutonomy, s.policy.DefaultProfile())

	if _, ok := s.policy.GetProfile(policyProfile); !ok {
		slog.Warn("unknown policy profile for conversation, denying", "profile", policyProfile, "conversation_id", req.RunID)
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), fmt.Sprintf("unknown policy profile %q", policyProfile))
	}

	// Evaluate policy.
	call := policy.ToolCall{
		Tool:    req.Tool,
		Command: req.Command,
		Path:    req.Path,
	}
	result, err := s.policy.EvaluateWithReason(ctx, policyProfile, call, policyEvalOptions(proj.WorkspacePath, m)...)
	if err != nil {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), err.Error())
	}
	decision := result.Decision

	slog.Debug("conversation policy evaluation",
		"conversation_id", req.RunID,
		"tool", req.Tool,
		"mode", modeID,
		"decision", decision,
		"profile", result.Profile,
		"rule_index", result.RuleIndex,
		"reason", result.Reason,
	)

	// HITL: when policy says "ask", check bypass / auto-approval before blocking.
	if decision == policy.DecisionAsk {
		if s.IsConversationBypassed(req.RunID) {
			decision = policy.DecisionAllow
			slog.Info("conversation HITL bypassed (bypass-all)",
				"conversation_id", req.RunID,
				"call_id", req.CallID,
				"tool", req.Tool,
			)
		} else if profile, profileOK := s.policy.GetProfile(policyProfile); profileOK && (profile.Mode == policy.ModeAcceptEdits || profile.Mode == policy.ModeDelegate) {
			decision = policy.DecisionAllow
			slog.Info("conversation HITL auto-approved (full-auto profile)",
				"conversation_id", req.RunID,
				"call_id", req.CallID,
				"tool", req.Tool,
				"profile", policyProfile,
			)
		} else {
			decision = s.waitForApproval(ctx, req.RunID, req.CallID, req.Tool, req.Command, req.Path)
			slog.Info("conversation HITL resolved",
				"conversation_id", req.RunID,
				"call_id", req.CallID,
				"tool", req.Tool,
				"decision", decision,
			)
		}
	}

	// Broadcast WS tool call status.
	s.broadcastToolCallStatus(ctx, req.RunID, req.CallID, req.Tool, decisionPhase(decision), string(decision))

	return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(decision), denialReason(decision, result))
}

// resolveMode loads the agent mode a tool call runs in. It returns nil
// without error when no mode is set or no mode service is configured, and an
// error for an unknown mode, so that the call is denied (fail closed).
func (s *RuntimeService) resolveMode(modeID string) (*mode.Mode, error) {
	if modeID == "" || s.modes == nil {
		return nil, nil
	}
	m, err := s.modes.Get(modeID)
	if err != nil {
		return nil, fmt.Errorf("unknown mode %q", modeID)
	}
	return m, nil
}

// denialReason tells the worker (and through it the agent) why a tool call
// was not allowed.
func denialReason(decision policy.Decision, result *policy.EvaluationResult) string {
	switch {
	case decision == policy.DecisionAllow:
		return ""
	case result.Decision == policy.DecisionAsk:
		return "not approved by a human reviewer"
	default:
		return result.Reason
	}
}

// policyEvalOptions returns the policy evaluation options for a tool call:
// the workspace that paths are resolved against and the mode's tool lists.
func policyEvalOptions(workspace string, m *mode.Mode) []policy.EvalOption {
	opts := []policy.EvalOption{policy.WithWorkspace(workspace)}
	if m != nil {
		opts = append(opts, policy.WithModeTools(m.ID, m.Tools, m.DeniedTools))
	}
	return opts
}

// HandleToolCallResult processes the outcome of an executed tool call.
func (s *RuntimeService) HandleToolCallResult(ctx context.Context, result *messagequeue.ToolCallResultPayload) error {
	ctx, r, err := s.loadRunScoped(ctx, result.RunID, result.TenantID)
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
	logBestEffort(ctx, s.store.UpdateRunStatus(ctx, r.ID, r.Status, r.StepCount, newCost, newTokensIn, newTokensOut), "UpdateRunStatus", slog.String("run_id", r.ID))

	// The run's counters including this tool call, final numbers for the paths
	// below that end the run.
	counted := *r
	counted.CostUSD, counted.TokensIn, counted.TokensOut = newCost, newTokensIn, newTokensOut

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
			s.appendAudit(ctx, r, "budget.exceeded", reason)
			logBestEffort(ctx, s.stopRun(ctx, &counted, run.StatusTimeout, reason), "stopRun", slog.String("run_id", r.ID))
			return nil
		}

		for _, threshold := range []float64{80, 90} {
			if pct >= threshold {
				alertKey := fmt.Sprintf("%s:%d", r.ID, int(threshold))
				if alreadySent := s.state.StoreBudgetAlert(alertKey); !alreadySent {
					s.hub.BroadcastEvent(ctx, event.EventBudgetAlert, event.BudgetAlertEvent{
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
	if st, ok := s.state.GetStallTracker(r.ID); ok {
		if st.RecordStep(result.Tool, result.Success, result.Output) {
			slog.Warn("stall detected, terminating run", "run_id", r.ID, "tool", result.Tool)
			s.appendRunEvent(ctx, event.TypeStallDetected, r, map[string]string{
				"tool":       result.Tool,
				"step_count": fmt.Sprintf("%d", r.StepCount),
			})
			logBestEffort(ctx, s.stopRun(ctx, &counted, run.StatusFailed, "stall detected: agent not making progress"), "stopRun", slog.String("run_id", r.ID))
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
	s.broadcastToolCallStatus(ctx, r.ID, result.CallID, result.Tool, "result", "")

	// Broadcast AG-UI tool_result alongside native event
	toolResultErr := ""
	if !result.Success {
		toolResultErr = result.Output
	}
	s.hub.BroadcastEvent(ctx, event.AGUIToolResult, event.AGUIToolResultEvent{
		RunID:  r.ID,
		CallID: result.CallID,
		Result: result.Output,
		Error:  toolResultErr,
		Diff:   result.Diff,
	})

	return nil
}

// cleanupRunState removes heartbeat, stall tracker, and timeout goroutine for a run.
