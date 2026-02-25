package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/artifact"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// RuntimeService orchestrates the step-by-step execution protocol between
// Go (control plane) and Python (execution plane).
type RuntimeService struct {
	store            database.Store
	queue            messagequeue.Queue
	hub              broadcast.Broadcaster
	events           eventstore.Store
	policy           *PolicyService
	modes            *ModeService
	deliver          *DeliverService
	contextOpt       *ContextOptimizerService
	checkpoint       *CheckpointService
	sandbox          *SandboxService
	mcpSvc           *MCPService
	onRunComplete    func(ctx context.Context, runID string, status run.Status)
	runtimeCfg       *config.Runtime
	stallTrackers    sync.Map // map[runID]*run.StallTracker
	heartbeats       sync.Map // map[runID]time.Time — last heartbeat timestamp
	runTimeouts      sync.Map // map[runID]context.CancelFunc — context-level timeout cancel
	budgetAlerts     sync.Map // map["runID:threshold"]bool — dedup budget alerts
	pendingApprovals sync.Map // map["runID:callID"]chan string — HITL approval channels
}

// NewRuntimeService creates a RuntimeService with all dependencies.
func NewRuntimeService(
	store database.Store,
	queue messagequeue.Queue,
	hub broadcast.Broadcaster,
	events eventstore.Store,
	policySvc *PolicyService,
	runtimeCfg *config.Runtime,
) *RuntimeService {
	return &RuntimeService{
		store:      store,
		queue:      queue,
		hub:        hub,
		events:     events,
		policy:     policySvc,
		runtimeCfg: runtimeCfg,
	}
}

// SetDeliverService sets the delivery service for post-run delivery.
func (s *RuntimeService) SetDeliverService(d *DeliverService) {
	s.deliver = d
}

// SetContextOptimizer sets the context optimizer for building context packs before runs.
func (s *RuntimeService) SetContextOptimizer(co *ContextOptimizerService) {
	s.contextOpt = co
}

// SetOnRunComplete registers a callback invoked after a run reaches a terminal state.
// Used by the OrchestratorService to advance execution plans.
func (s *RuntimeService) SetOnRunComplete(fn func(context.Context, string, run.Status)) {
	s.onRunComplete = fn
}

// SetCheckpointService sets the checkpoint service for shadow git commits.
func (s *RuntimeService) SetCheckpointService(cp *CheckpointService) {
	s.checkpoint = cp
}

// SetSandboxService sets the sandbox service for containerized execution.
func (s *RuntimeService) SetSandboxService(sb *SandboxService) {
	s.sandbox = sb
}

// SetModeService sets the mode service for resolving agent modes during run start.
func (s *RuntimeService) SetModeService(m *ModeService) {
	s.modes = m
}

// SetMCPService sets the MCP service for resolving MCP server definitions during run start.
func (s *RuntimeService) SetMCPService(svc *MCPService) {
	s.mcpSvc = svc
}

// SetHeartbeat sets the last heartbeat timestamp for a run. Intended for testing.
func (s *RuntimeService) SetHeartbeat(runID string, t time.Time) {
	s.heartbeats.Store(runID, t)
}

// StartRun creates a new run in the database and publishes a start message to NATS.
func (s *RuntimeService) StartRun(ctx context.Context, req *run.StartRequest) (*run.Run, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate start request: %w", err)
	}

	// Default exec mode
	if req.ExecMode == "" {
		req.ExecMode = run.ExecModeMount
	}

	// Default policy profile
	profileName := req.PolicyProfile
	if profileName == "" {
		profileName = s.policy.DefaultProfile()
	}

	// Verify policy profile exists
	profile, ok := s.policy.GetProfile(profileName)
	if !ok {
		return nil, fmt.Errorf("unknown policy profile %q", profileName)
	}

	// Verify agent exists
	ag, err := s.store.GetAgent(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}

	// Resolve agent mode: explicit request > agent default > "coder" fallback
	modeID := req.ModeID
	if modeID == "" {
		modeID = ag.ModeID
	}
	if modeID == "" {
		modeID = "coder"
	}
	var resolvedMode *messagequeue.ModePayload
	if s.modes != nil {
		if m, mErr := s.modes.Get(modeID); mErr == nil {
			_, sections := BuildModePrompt(m)
			sections = PruneToFitBudget(sections, DefaultModePromptBudget)
			assembledPrompt := AssembleSections(sections)
			resolvedMode = &messagequeue.ModePayload{
				ID:               m.ID,
				PromptPrefix:     assembledPrompt,
				Tools:            m.Tools,
				DeniedTools:      m.DeniedTools,
				DeniedActions:    m.DeniedActions,
				RequiredArtifact: m.RequiredArtifact,
				LLMScenario:      m.LLMScenario,
			}
		} else {
			slog.Warn("mode not found, using default", "mode_id", modeID, "error", mErr)
		}
	}

	// Verify task exists
	t, err := s.store.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	// Default deliver mode from config
	deliverMode := req.DeliverMode
	if deliverMode == "" && s.runtimeCfg.DefaultDeliverMode != "" {
		deliverMode = run.DeliverMode(s.runtimeCfg.DefaultDeliverMode)
	}

	// Create run in DB
	r := &run.Run{
		TaskID:        req.TaskID,
		AgentID:       req.AgentID,
		ProjectID:     req.ProjectID,
		TeamID:        req.TeamID,
		ModeID:        modeID,
		PolicyProfile: profileName,
		ExecMode:      req.ExecMode,
		DeliverMode:   deliverMode,
		Status:        run.StatusPending,
	}
	if err := s.store.CreateRun(ctx, r); err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}

	// Mark run as running
	if err := s.store.UpdateRunStatus(ctx, r.ID, run.StatusRunning, 0, 0, 0, 0); err != nil {
		return nil, fmt.Errorf("update run status: %w", err)
	}
	r.Status = run.StatusRunning

	// Mark agent as running
	_ = s.store.UpdateAgentStatus(ctx, req.AgentID, agent.StatusRunning)

	// Mark task as running
	_ = s.store.UpdateTaskStatus(ctx, req.TaskID, task.StatusRunning)

	// Start sandbox/hybrid container if applicable
	if s.sandbox != nil {
		switch req.ExecMode {
		case run.ExecModeSandbox:
			proj, projErr := s.store.GetProject(ctx, req.ProjectID)
			if projErr != nil {
				return nil, fmt.Errorf("get project for sandbox: %w", projErr)
			}
			if _, sbErr := s.sandbox.Create(ctx, r.ID, proj.WorkspacePath); sbErr != nil {
				return nil, fmt.Errorf("sandbox create: %w", sbErr)
			}
			if sbErr := s.sandbox.Start(ctx, r.ID); sbErr != nil {
				_ = s.sandbox.Remove(ctx, r.ID)
				return nil, fmt.Errorf("sandbox start: %w", sbErr)
			}
			slog.Info("sandbox started", "run_id", r.ID)

		case run.ExecModeHybrid:
			proj, projErr := s.store.GetProject(ctx, req.ProjectID)
			if projErr != nil {
				return nil, fmt.Errorf("get project for hybrid: %w", projErr)
			}
			if _, sbErr := s.sandbox.CreateHybrid(ctx, r.ID, proj.WorkspacePath); sbErr != nil {
				return nil, fmt.Errorf("hybrid create: %w", sbErr)
			}
			if sbErr := s.sandbox.Start(ctx, r.ID); sbErr != nil {
				_ = s.sandbox.Remove(ctx, r.ID)
				return nil, fmt.Errorf("hybrid start: %w", sbErr)
			}
			slog.Info("hybrid container started", "run_id", r.ID)
		}
	}

	// Create stall tracker if policy enables stall detection
	if profile.Termination.StallDetection {
		threshold := profile.Termination.StallThreshold
		if threshold <= 0 {
			threshold = s.runtimeCfg.StallThreshold
		}
		s.stallTrackers.Store(r.ID, run.NewStallTracker(threshold, s.runtimeCfg.StallMaxRetries))
	}

	// Publish run start to NATS
	payload := messagequeue.RunStartPayload{
		RunID:         r.ID,
		TaskID:        t.ID,
		ProjectID:     t.ProjectID,
		AgentID:       ag.ID,
		Prompt:        t.Prompt,
		PolicyProfile: profileName,
		ExecMode:      string(req.ExecMode),
		DeliverMode:   string(deliverMode),
		Mode:          resolvedMode,
		Config:        ag.Config,
		Termination: messagequeue.TerminationPayload{
			MaxSteps:       profile.Termination.MaxSteps,
			TimeoutSeconds: profile.Termination.TimeoutSeconds,
			MaxCost:        profile.Termination.MaxCost,
		},
	}

	// Build context pack if context optimizer is available.
	if s.contextOpt != nil {
		pack, packErr := s.contextOpt.BuildContextPack(ctx, req.TaskID, req.ProjectID, req.TeamID)
		if packErr != nil {
			slog.Warn("context pack build failed", "run_id", r.ID, "error", packErr)
		} else if pack != nil && len(pack.Entries) > 0 {
			payload.Context = toContextEntryPayloads(pack.Entries)
		}
	}

	// Resolve MCP server definitions for this run.
	if s.mcpSvc != nil {
		defs := s.mcpSvc.ResolveForRun(req.ProjectID, modeID)
		for i := range defs {
			d := &defs[i]
			payload.MCPServers = append(payload.MCPServers, messagequeue.MCPServerDefPayload{
				ID:          d.ID,
				Name:        d.Name,
				Description: d.Description,
				Transport:   string(d.Transport),
				Command:     d.Command,
				Args:        d.Args,
				URL:         d.URL,
				Env:         d.Env,
				Headers:     d.Headers,
				Enabled:     d.Enabled,
			})
		}
	}

	if err := s.publishJSON(ctx, messagequeue.SubjectRunStart, payload); err != nil {
		return nil, fmt.Errorf("publish run start: %w", err)
	}

	// Record event
	s.appendRunEvent(ctx, event.TypeRunStarted, r, map[string]string{
		"policy_profile": profileName,
		"exec_mode":      string(req.ExecMode),
		"backend":        ag.Backend,
		"mode_id":        modeID,
	})

	// Broadcast WS
	s.hub.BroadcastEvent(ctx, ws.EventRunStatus, ws.RunStatusEvent{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    string(r.Status),
	})

	// Broadcast AG-UI run_started alongside native event
	s.hub.BroadcastEvent(ctx, ws.AGUIRunStarted, ws.AGUIRunStartedEvent{
		RunID:     r.ID,
		AgentName: ag.Name,
	})

	// Start context-level timeout goroutine
	if profile.Termination.TimeoutSeconds > 0 {
		timeoutDur := time.Duration(profile.Termination.TimeoutSeconds) * time.Second
		timeoutCtx, timeoutCancel := context.WithCancel(context.Background())
		s.runTimeouts.Store(r.ID, timeoutCancel)
		go func(runID string, timeout time.Duration) {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-timer.C:
				// Check if run is still active before cancelling
				rr, err := s.store.GetRun(context.Background(), runID)
				if err != nil || rr.Status != run.StatusRunning {
					return
				}
				slog.Warn("context-level timeout, cancelling run", "run_id", runID, "timeout", timeout)
				_ = s.cancelRunWithReason(context.Background(), runID, "context-level timeout")
			case <-timeoutCtx.Done():
				return
			}
		}(r.ID, timeoutDur)
	}

	// Audit trail
	s.appendAudit(ctx, r, "run.started", fmt.Sprintf("Run started with policy %s, exec_mode %s, agent %s, mode %s", profileName, req.ExecMode, ag.Name, modeID))

	slog.Info("run started", "run_id", r.ID, "task_id", r.TaskID, "policy", profileName)
	return r, nil
}

// HandleToolCallRequest processes a tool call permission request from a worker.
// It evaluates termination conditions and policy rules, then publishes a response.
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
func (s *RuntimeService) cleanupRunState(runID string) {
	s.heartbeats.Delete(runID)
	s.stallTrackers.Delete(runID)
	if cancel, ok := s.runTimeouts.LoadAndDelete(runID); ok {
		cancel.(context.CancelFunc)()
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
func (s *RuntimeService) CancelRun(ctx context.Context, runID string) error {
	r, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}

	if r.Status != run.StatusRunning && r.Status != run.StatusPending && r.Status != run.StatusQualityGate {
		return fmt.Errorf("run %s is not active (status: %s)", runID, r.Status)
	}

	// Clean up all run-associated state
	s.cleanupRunState(runID)

	// Update DB
	if err := s.store.CompleteRun(ctx, r.ID, run.StatusCancelled, "", "cancelled by user", r.CostUSD, r.StepCount, r.TokensIn, r.TokensOut, r.Model); err != nil {
		return fmt.Errorf("complete run: %w", err)
	}

	// Set agent idle
	_ = s.store.UpdateAgentStatus(ctx, r.AgentID, agent.StatusIdle)
	_ = s.store.UpdateTaskStatus(ctx, r.TaskID, task.StatusCancelled)

	// Notify worker via NATS
	cancelPayload := struct {
		RunID string `json:"run_id"`
	}{RunID: runID}
	_ = s.publishJSON(ctx, messagequeue.SubjectRunCancel, cancelPayload)

	// Record event
	s.appendRunEvent(ctx, event.TypeRunCompleted, r, map[string]string{
		"status": string(run.StatusCancelled),
		"reason": "cancelled by user",
	})

	// Broadcast WS
	s.hub.BroadcastEvent(ctx, ws.EventRunStatus, ws.RunStatusEvent{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    string(run.StatusCancelled),
		StepCount: r.StepCount,
		CostUSD:   r.CostUSD,
		TokensIn:  r.TokensIn,
		TokensOut: r.TokensOut,
		Model:     r.Model,
	})

	// Clean up checkpoints
	if s.checkpoint != nil {
		proj, projErr := s.store.GetProject(ctx, r.ProjectID)
		if projErr == nil {
			if cpErr := s.checkpoint.CleanupCheckpoints(ctx, r.ID, proj.WorkspacePath); cpErr != nil {
				slog.Warn("checkpoint cleanup on cancel failed", "run_id", r.ID, "error", cpErr)
			}
		}
	}

	// Clean up sandbox
	if s.sandbox != nil {
		if _, ok := s.sandbox.Get(r.ID); ok {
			if err := s.sandbox.Stop(ctx, r.ID); err != nil {
				slog.Warn("sandbox stop on cancel failed", "run_id", r.ID, "error", err)
			}
			if err := s.sandbox.Remove(ctx, r.ID); err != nil {
				slog.Warn("sandbox remove on cancel failed", "run_id", r.ID, "error", err)
			}
		}
	}

	// Audit trail
	s.appendAudit(ctx, r, "run.cancelled", fmt.Sprintf("Run cancelled by user, %d steps completed, cost $%.4f", r.StepCount, r.CostUSD))

	slog.Info("run cancelled", "run_id", runID)
	return nil
}

// GetRun returns a run by ID.
func (s *RuntimeService) GetRun(ctx context.Context, id string) (*run.Run, error) {
	return s.store.GetRun(ctx, id)
}

// ListRunsByTask returns all runs for a given task.
func (s *RuntimeService) ListRunsByTask(ctx context.Context, taskID string) ([]run.Run, error) {
	return s.store.ListRunsByTask(ctx, taskID)
}

// StartSubscribers subscribes to all run-related NATS subjects.
// Returns cancel functions for each subscription.
func (s *RuntimeService) StartSubscribers(ctx context.Context) ([]func(), error) {
	var cancels []func()

	// Tool call requests from workers
	cancel, err := s.queue.Subscribe(ctx, messagequeue.SubjectRunToolCallRequest, func(msgCtx context.Context, _ string, data []byte) error {
		var req messagequeue.ToolCallRequestPayload
		if err := json.Unmarshal(data, &req); err != nil {
			return fmt.Errorf("unmarshal tool call request: %w", err)
		}
		return s.HandleToolCallRequest(msgCtx, &req)
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe tool call request: %w", err)
	}
	cancels = append(cancels, cancel)

	// Tool call results from workers
	cancel, err = s.queue.Subscribe(ctx, messagequeue.SubjectRunToolCallResult, func(msgCtx context.Context, _ string, data []byte) error {
		var result messagequeue.ToolCallResultPayload
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("unmarshal tool call result: %w", err)
		}
		return s.HandleToolCallResult(msgCtx, &result)
	})
	if err != nil {
		cancelAll(cancels)
		return nil, fmt.Errorf("subscribe tool call result: %w", err)
	}
	cancels = append(cancels, cancel)

	// Run completion from workers
	cancel, err = s.queue.Subscribe(ctx, messagequeue.SubjectRunComplete, func(msgCtx context.Context, _ string, data []byte) error {
		var payload messagequeue.RunCompletePayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("unmarshal run complete: %w", err)
		}
		return s.HandleRunComplete(msgCtx, &payload)
	})
	if err != nil {
		cancelAll(cancels)
		return nil, fmt.Errorf("subscribe run complete: %w", err)
	}
	cancels = append(cancels, cancel)

	// Quality gate results from workers
	cancel, err = s.queue.Subscribe(ctx, messagequeue.SubjectQualityGateResult, func(msgCtx context.Context, _ string, data []byte) error {
		var result messagequeue.QualityGateResultPayload
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("unmarshal quality gate result: %w", err)
		}
		return s.HandleQualityGateResult(msgCtx, &result)
	})
	if err != nil {
		cancelAll(cancels)
		return nil, fmt.Errorf("subscribe quality gate result: %w", err)
	}
	cancels = append(cancels, cancel)

	// Heartbeat from workers (Phase 3C)
	cancel, err = s.queue.Subscribe(ctx, messagequeue.SubjectRunHeartbeat, func(_ context.Context, _ string, data []byte) error {
		var hb messagequeue.RunHeartbeatPayload
		if err := json.Unmarshal(data, &hb); err != nil {
			return fmt.Errorf("unmarshal heartbeat: %w", err)
		}
		s.heartbeats.Store(hb.RunID, time.Now())
		return nil
	})
	if err != nil {
		cancelAll(cancels)
		return nil, fmt.Errorf("subscribe heartbeat: %w", err)
	}
	cancels = append(cancels, cancel)

	// Streaming output from workers
	cancel, err = s.queue.Subscribe(ctx, messagequeue.SubjectRunOutput, func(msgCtx context.Context, _ string, data []byte) error {
		var output messagequeue.RunOutputPayload
		if err := json.Unmarshal(data, &output); err != nil {
			return fmt.Errorf("unmarshal run output: %w", err)
		}
		s.hub.BroadcastEvent(msgCtx, ws.EventTaskOutput, ws.TaskOutputEvent{
			TaskID: output.TaskID,
			Line:   output.Line,
			Stream: output.Stream,
		})
		return nil
	})
	if err != nil {
		cancelAll(cancels)
		return nil, fmt.Errorf("subscribe run output: %w", err)
	}
	cancels = append(cancels, cancel)

	return cancels, nil
}

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

// --- HITL (Human-in-the-Loop) approval ---

// approvalKey builds a unique key for pending approval channels.
func approvalKey(runID, callID string) string {
	return runID + ":" + callID
}

// waitForApproval broadcasts a permission request to the frontend and blocks until
// the user responds (via ResolveApproval) or the timeout expires. Returns the final
// decision (allow or deny).
func (s *RuntimeService) waitForApproval(ctx context.Context, runID, callID, tool, command, path string) policy.Decision {
	// Default timeout: 60 seconds.
	timeout := 60 * time.Second
	if s.runtimeCfg != nil && s.runtimeCfg.ApprovalTimeoutSeconds > 0 {
		timeout = time.Duration(s.runtimeCfg.ApprovalTimeoutSeconds) * time.Second
	}

	ch := make(chan string, 1)
	key := approvalKey(runID, callID)
	s.pendingApprovals.Store(key, ch)
	defer s.pendingApprovals.Delete(key)

	// Broadcast permission request to connected clients.
	s.hub.BroadcastEvent(ctx, ws.AGUIPermissionRequest, ws.AGUIPermissionRequestEvent{
		RunID:   runID,
		CallID:  callID,
		Tool:    tool,
		Command: command,
		Path:    path,
	})

	slog.Info("HITL approval requested",
		"run_id", runID,
		"call_id", callID,
		"tool", tool,
		"timeout", timeout,
	)

	select {
	case decision := <-ch:
		if decision == "allow" {
			return policy.DecisionAllow
		}
		return policy.DecisionDeny
	case <-time.After(timeout):
		slog.Warn("HITL approval timed out, denying",
			"run_id", runID,
			"call_id", callID,
			"tool", tool,
		)
		return policy.DecisionDeny
	case <-ctx.Done():
		return policy.DecisionDeny
	}
}

// ResolveApproval is called from the HTTP handler when a user approves or denies
// a pending tool call. Returns true if a pending approval was found and resolved.
func (s *RuntimeService) ResolveApproval(runID, callID, decision string) bool {
	key := approvalKey(runID, callID)
	val, ok := s.pendingApprovals.LoadAndDelete(key)
	if !ok {
		return false
	}
	ch, _ := val.(chan string)
	if ch == nil {
		return false
	}
	select {
	case ch <- decision:
		return true
	default:
		return false
	}
}
