package service

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/telemetry"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
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

	// Load policy profile for termination checks: the run's profile, or the
	// project's Allow-Always clone of it.
	profileName := effectivePolicyProfile(ctx, s.policy, r.PolicyProfile, r.ProjectID)
	profile, ok := s.policy.GetProfile(ctx, profileName)
	if !ok {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), "unknown policy profile")
	}

	// Check termination conditions
	if reason := s.checkTermination(r, &profile); reason != "" {
		logRunUpdate(ctx, s.stopRun(ctx, r, run.StatusTimeout, reason), "stopRun", r.ID)
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
	result, err := s.policy.EvaluateWithReason(ctx, profileName, call, s.policyEvalOptions(workspace, m, req.Trust)...)
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
				"profile", profileName,
			)
		} else {
			decision = s.waitForApproval(ctx, permissionRequest(r.ID, req, r.PolicyProfile))
			slog.Info("HITL approval resolved",
				"run_id", r.ID,
				"call_id", req.CallID,
				"tool", req.Tool,
				"decision", decision,
			)
		}
	}

	// Count the step; only a running run counts steps and its usage counters
	// are not touched. A run that ended or moved to its quality gate while the
	// call waited for approval stays where it is, and its call is denied
	// without a policy verdict (KI-31).
	err = s.store.CountRunStep(ctx, r.ID)
	if errors.Is(err, domain.ErrConflict) {
		slog.Info("run no longer running after the tool call was pending, denying", "run_id", r.ID, "call_id", req.CallID)
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), "run is no longer running")
	}
	logBestEffort(ctx, err, "CountRunStep", slog.String("run_id", r.ID))

	// The run's checkpoint comes before a file-modifying call executes; a
	// run that may have to be rolled back or delivered does not change its
	// workspace without one (fail closed).
	if decision == policy.DecisionAllow && isFileModifyingTool(req.Tool) {
		if denial := s.checkpointToolCall(ctx, r, &profile, proj, projErr, req); denial != "" {
			decision = policy.DecisionDeny
			result.Decision, result.Reason = policy.DecisionDeny, denial
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

	// Broadcast AG-UI tool_call alongside native event. The agent loop's
	// "LLM" permission before each completion is no tool call: no card.
	if !isLLMPermission(req.Tool) {
		s.hub.BroadcastEvent(ctx, event.AGUIToolCall, event.AGUIToolCallEvent{
			RunID:  r.ID,
			CallID: req.CallID,
			Name:   req.Tool,
			Args:   req.Command,
		})
	}

	// OTEL: record tool call span and metric
	_, toolSpan := telemetry.StartToolCallSpan(ctx, req.CallID, req.Tool)
	toolSpan.SetAttributes(attribute.String("decision", string(decision)))
	toolSpan.End()
	if s.metrics != nil {
		s.metrics.RecordToolCall(ctx, "tool", req.Tool, "decision", string(decision))
	}

	return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(decision), denialReason(decision, result))
}

// checkpointToolCall records the workspace as the run's next checkpoint
// before an allowed file-modifying call executes and returns why the call
// must be denied, "" when it may run (S3 follow-up 1b). A run whose failed
// quality gate rolls the workspace back, or that delivers its change, needs
// the checkpoint: without it the call is denied. A workspace without a git
// repository has no rollback base at all; its calls run, and the run's audit
// trail says so once. Without a checkpoint service there is nothing to do.
func (s *RuntimeService) checkpointToolCall(ctx context.Context, r *run.Run, profile *policy.PolicyProfile, proj *project.Project, projErr error, req *messagequeue.ToolCallRequestPayload) string {
	if s.checkpoint == nil {
		return ""
	}
	needsBase := profile.QualityGate.RollbackOnGateFail || (r.DeliverMode != "" && r.DeliverMode != run.DeliverModeNone)
	var err error
	if projErr != nil {
		err = fmt.Errorf("project unavailable: %w", projErr)
	} else {
		err = s.checkpoint.CreateCheckpoint(ctx, r.ID, proj.WorkspacePath, req.Tool, req.CallID)
	}
	switch {
	case err == nil:
		return ""
	case errors.Is(err, git.ErrNotRepository):
		if _, recorded := s.noRollbackBase.LoadOrStore(r.ID, struct{}{}); !recorded {
			slog.Warn("workspace without git: the run has no rollback base", "run_id", r.ID)
			s.appendAudit(ctx, r, "checkpoint.unavailable",
				"The workspace is not a git repository: the run has no checkpoint, it cannot be rolled back and its change cannot be delivered")
		}
		return ""
	case !needsBase:
		slog.Warn("checkpoint creation failed", "run_id", r.ID, "call_id", req.CallID, "error", err)
		return ""
	default:
		slog.Error("checkpoint creation failed, denying the call", "run_id", r.ID, "call_id", req.CallID, "error", err)
		return "the workspace could not be checkpointed, and this run may have to be rolled back or delivered: " + err.Error()
	}
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

	// Conversation runs reuse the conversation ID as run ID; the worker reports
	// the run's turn. A call of the conversation's active run is evaluated,
	// also while an earlier stop's mark still holds (the run is recognized
	// before its start is published). A call of another run is rejected: that
	// run was stopped or replaced. Calls without a turn, and calls of a
	// conversation without an active run here (a restart, another replica),
	// are rejected while a stop's mark holds.
	turn, active := s.state.ActiveConversationTurn(req.RunID)
	ofActiveRun := active && req.TurnID != "" && req.TurnID == turn
	if active && req.TurnID != "" && !ofActiveRun {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), "conversation run ended")
	}
	if !ofActiveRun && s.state.IsConversationCancelled(req.RunID) {
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
	// A call of a turn this process does not know as the active run is
	// evaluated only if it is the stored active turn (a restart, another
	// replica). Any other turn ended - stopped, or ended by the stuck-work
	// watchdog while its worker was cut off - and its worker must not go on
	// editing the workspace.
	if !ofActiveRun && req.TurnID != "" && req.TurnID != conv.ActiveTurnID {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), "conversation run ended")
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
	baseProfile := conversationPolicyProfile(proj, modeAutonomy, s.policy.DefaultProfile())
	policyProfile := effectivePolicyProfile(ctx, s.policy, baseProfile, proj.ID)

	if _, ok := s.policy.GetProfile(ctx, policyProfile); !ok {
		slog.Warn("unknown policy profile for conversation, denying", "profile", policyProfile, "conversation_id", req.RunID)
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), fmt.Sprintf("unknown policy profile %q", policyProfile))
	}

	// Evaluate policy.
	call := policy.ToolCall{
		Tool:    req.Tool,
		Command: req.Command,
		Path:    req.Path,
	}
	result, err := s.policy.EvaluateWithReason(ctx, policyProfile, call, s.policyEvalOptions(proj.WorkspacePath, m, req.Trust)...)
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
		} else if profile, profileOK := s.policy.GetProfile(ctx, policyProfile); profileOK && (profile.Mode == policy.ModeAcceptEdits || profile.Mode == policy.ModeDelegate) {
			decision = policy.DecisionAllow
			slog.Info("conversation HITL auto-approved (full-auto profile)",
				"conversation_id", req.RunID,
				"call_id", req.CallID,
				"tool", req.Tool,
				"profile", policyProfile,
			)
		} else {
			decision = s.waitForApproval(ctx, permissionRequest(req.RunID, req, baseProfile))
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

	// The chat shows the call as a live tool card, as for runs; the worker's
	// result completes it (KI-161). A call whose result never follows gets
	// no card: it would stay running.
	if req.ReportsResult && !isLLMPermission(req.Tool) {
		s.hub.BroadcastEvent(ctx, event.AGUIToolCall, event.AGUIToolCallEvent{
			RunID:  req.RunID,
			CallID: req.CallID,
			Name:   req.Tool,
			Args:   cappedArgumentsPreview(req.ArgumentsPreview),
		})
	}

	return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(decision), denialReason(decision, result))
}

// isLLMPermission tells the permission the agent loop requests before each
// LLM completion from a tool call: it is no tool the chat shows as a card.
func isLLMPermission(tool string) bool {
	return policy.CanonicalTool(tool) == policy.ToolLLM
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

// maxArgumentsPreviewBytes caps the arguments preview a worker sends (1000
// characters, up to 4 bytes each), so a misbehaving worker cannot flood the
// WebSocket clients.
const maxArgumentsPreviewBytes = 4096

// permissionRequest builds the HITL permission request for a tool call that
// the policy profile asks about. profile is the profile the call resolved to
// before the project's Allow-Always clone was applied: Allow-Always extends
// the project's clone of it.
func permissionRequest(runID string, req *messagequeue.ToolCallRequestPayload, profile string) *event.AGUIPermissionRequestEvent {
	return &event.AGUIPermissionRequestEvent{
		RunID:            runID,
		CallID:           req.CallID,
		Tool:             req.Tool,
		Command:          req.Command,
		Path:             req.Path,
		Profile:          profile,
		ArgumentsPreview: cappedArgumentsPreview(req.ArgumentsPreview),
	}
}

// cappedArgumentsPreview caps a worker's arguments preview at
// maxArgumentsPreviewBytes without splitting a UTF-8 character.
func cappedArgumentsPreview(preview string) string {
	if len(preview) <= maxArgumentsPreviewBytes {
		return preview
	}
	return truncateUTF8(preview, maxArgumentsPreviewBytes-len("...")) + "..."
}

// policyEvalOptions returns the policy evaluation options for a tool call:
// the workspace that paths are resolved against (and its real path, for the
// files a command redirects to), the trust annotation of the request (allow
// rules with a trust minimum need one) and the mode's tool lists.
func (s *RuntimeService) policyEvalOptions(workspace string, m *mode.Mode, ann *trust.Annotation) []policy.EvalOption {
	opts := []policy.EvalOption{
		policy.WithWorkspace(workspace),
		policy.WithWorkspaceRealPath(s.workspaceRealPath(workspace)),
		policy.WithTrust(ann),
	}
	if m != nil {
		opts = append(opts, policy.WithModeTools(m.ID, m.Tools, m.DeniedTools))
	}
	return opts
}

// workspaceRealPath returns the workspace path with its symlinks resolved;
// "" for a workspace that is not an absolute path or cannot be resolved (it
// does not exist yet), which is then only checked in its given form.
//
// A resolution is cached per workspace path together with the directory it
// led to, and reused only while the path still leads to that directory and
// the resolved path still names it (two stat calls per tool call, instead of
// a walk over every path component): a retargeted symlink or a moved
// directory is resolved again. A failed resolution of an existing directory
// is cached the same way; a missing one costs a single stat.
func (s *RuntimeService) workspaceRealPath(workspace string) string {
	if !filepath.IsAbs(workspace) {
		return ""
	}
	dir, err := os.Stat(workspace)
	if err != nil {
		return ""
	}
	if cached, ok := s.workspaceRealPaths.Load(workspace); ok {
		if entry, isEntry := cached.(realPathEntry); isEntry && entry.stillValid(dir) {
			return entry.resolved
		}
	}
	resolved, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		resolved = ""
	}
	s.workspaceRealPaths.Store(workspace, realPathEntry{dir: dir, resolved: resolved})
	return resolved
}

// realPathEntry is a cached workspace resolution: the directory the
// workspace path led to and its real path ("" when it could not be resolved).
type realPathEntry struct {
	dir      os.FileInfo
	resolved string
}

// stillValid reports whether the workspace path still leads to the cached
// directory (dir is its current stat) and the real path still names it.
func (e realPathEntry) stillValid(dir os.FileInfo) bool {
	if !os.SameFile(e.dir, dir) {
		return false
	}
	if e.resolved == "" {
		return true
	}
	current, err := os.Stat(e.resolved)
	return err == nil && os.SameFile(current, dir)
}

// HandleToolCallResult processes the outcome of an executed tool call.
func (s *RuntimeService) HandleToolCallResult(ctx context.Context, result *messagequeue.ToolCallResultPayload) error {
	ctx, r, err := s.loadRunScoped(ctx, result.RunID, result.TenantID)
	if err != nil {
		// Conversation-based runs don't have a run record.
		// Cost/token tracking for conversations happens via WebSocket events.
		slog.Debug("tool call result for conversation run", "run_id", result.RunID, "cost", result.CostUSD)
		if errors.Is(err, domain.ErrNotFound) {
			s.broadcastConversationToolResult(ctx, result)
		}
		return nil
	}

	// A result delivered again (at-least-once) was handled already.
	if r.Status == run.StatusRunning && !s.state.FirstToolResult(r.ID, result.CallID) {
		slog.Info("tool call result already handled, skipped", "run_id", r.ID, "call_id", result.CallID)
		return nil
	}

	counted, running := s.countToolUsage(ctx, r, result)
	newCost := counted.CostUSD

	// The per-tool usage record (cost by tool), kept for every executed call.
	s.appendRunEventWithTokens(ctx, event.TypeToolCallResultEv, r, map[string]string{
		"call_id": result.CallID,
		"tool":    result.Tool,
		"success": fmt.Sprintf("%t", result.Success),
		"cost":    fmt.Sprintf("%.6f", result.CostUSD),
	}, result.Tool, result.Model, result.TokensIn, result.TokensOut, result.CostUSD)

	// A run that ended (or waits for its quality gate) gets no budget or stall
	// decision and no live events after its run_finished.
	if !running {
		slog.Info("tool call result for a run that is not running, usage in the worker's totals", "run_id", r.ID)
		return nil
	}

	// Budget alert checks (80% and 90% thresholds) + post-execution budget enforcement
	profile, profileOK := s.policy.GetProfile(ctx, r.PolicyProfile)
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
			logRunUpdate(ctx, s.stopRun(ctx, counted, run.StatusTimeout, reason), "stopRun", r.ID)
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
			logRunUpdate(ctx, s.stopRun(ctx, counted, run.StatusFailed, run.StallDetectedError), "stopRun", r.ID)
			return nil
		}
	}

	// Broadcast WS with token data
	s.broadcastToolCallStatus(ctx, r.ID, result.CallID, result.Tool, "result", "")

	// Broadcast AG-UI tool_result alongside native event (none for the "LLM"
	// permission's usage report, which has no card).
	if !isLLMPermission(result.Tool) {
		s.hub.BroadcastEvent(ctx, event.AGUIToolResult, aguiToolResult(r.ID, result))
	}

	return nil
}

// broadcastConversationToolResult completes the conversation's live tool
// card (KI-161); delivered again, it completes the same card again. The
// tenant comes from the worker, so the card is shown only when the
// conversation is found in that tenant.
func (s *RuntimeService) broadcastConversationToolResult(ctx context.Context, result *messagequeue.ToolCallResultPayload) {
	if isLLMPermission(result.Tool) {
		return
	}
	if _, hasTenant := tenantctx.Lookup(ctx); !hasTenant {
		return
	}
	if _, err := s.store.GetConversation(ctx, result.RunID); err != nil {
		slog.DebugContext(ctx, "tool call result of no conversation in its tenant, no card", "run_id", result.RunID, "error", err)
		return
	}
	s.hub.BroadcastEvent(ctx, event.AGUIToolResult, aguiToolResult(result.RunID, result))
}

// aguiToolResult is the AG-UI result of a reported tool call. A failed call
// always carries an error, so the chat marks its card failed: the worker's
// error, else its output (a denied call has only an error).
func aguiToolResult(runID string, result *messagequeue.ToolCallResultPayload) event.AGUIToolResultEvent {
	ev := event.AGUIToolResultEvent{
		RunID:  runID,
		CallID: result.CallID,
		Result: result.Output,
		Diff:   cappedDiff(result.Diff),
	}
	if !result.Success {
		ev.Error = cmp.Or(result.Error, result.Output, "tool call failed")
	}
	return ev
}

// countToolUsage adds a tool call's usage to the counters of a running run
// and returns the run as stored afterwards and whether it still runs. Once
// the worker reported its totals (quality gate, completion) or the run ended,
// the totals include the call - RaiseRunUsage keeps them after a stop - and
// the store refuses the addition, so the call is not counted twice.
func (s *RuntimeService) countToolUsage(ctx context.Context, r *run.Run, result *messagequeue.ToolCallResultPayload) (*run.Run, bool) {
	if r.Status != run.StatusRunning {
		return r, false
	}
	stored, err := s.store.AddRunUsage(ctx, r.ID, &run.Usage{CostUSD: result.CostUSD, TokensIn: result.TokensIn, TokensOut: result.TokensOut})
	switch {
	case errors.Is(err, domain.ErrConflict):
		s.state.ForgetToolResult(r.ID, result.CallID)
		return r, false
	case err != nil:
		// Not stored: decide on the run's counters with the call added.
		logBestEffort(ctx, err, "AddRunUsage", slog.String("run_id", r.ID))
		estimate := *r
		estimate.CostUSD += result.CostUSD
		estimate.TokensIn += result.TokensIn
		estimate.TokensOut += result.TokensOut
		return &estimate, true
	}
	return stored, stored.Status == run.StatusRunning
}

// cleanupRunState removes heartbeat, stall tracker, and timeout goroutine for a run.

// maxBroadcastDiffContentBytes caps the file content a tool result's diff
// carries to the browser: a write_file diff holds the whole old and new
// file (up to the workspace file limit, an overwritten .env included).
const maxBroadcastDiffContentBytes = 8 << 10

// broadcastDiff is the diff a worker reports with a file tool's result
// (workers/codeforge/tools: write_file, edit_file).
type broadcastDiff struct {
	Path      string              `json:"path"`
	Hunks     []broadcastDiffHunk `json:"hunks"`
	Truncated bool                `json:"truncated,omitempty"`
}

type broadcastDiffHunk struct {
	OldStart   int    `json:"old_start"`
	OldLines   int    `json:"old_lines"`
	NewStart   int    `json:"new_start"`
	NewLines   int    `json:"new_lines"`
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content"`
}

// cappedDiff returns a tool result's diff as broadcast: unchanged when
// small, else with its hunks' content cut to maxBroadcastDiffContentBytes in
// all and marked truncated. A large diff that is no diff is dropped.
func cappedDiff(raw json.RawMessage) json.RawMessage {
	if len(raw) <= maxBroadcastDiffContentBytes {
		return raw
	}
	var d broadcastDiff
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil
	}
	budget := maxBroadcastDiffContentBytes
	take := func(content string) string {
		cut := truncateUTF8(content, budget)
		budget -= len(cut)
		if len(cut) < len(content) {
			d.Truncated = true
		}
		return cut
	}
	hunks := make([]broadcastDiffHunk, 0, len(d.Hunks))
	for _, h := range d.Hunks {
		if budget == 0 {
			d.Truncated = true
			break
		}
		h.OldContent = take(h.OldContent)
		h.NewContent = take(h.NewContent)
		hunks = append(hunks, h)
	}
	d.Hunks = hunks
	out, err := json.Marshal(d)
	if err != nil {
		return nil
	}
	return out
}
