package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/goal"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	feedbackPort "github.com/Strob0t/CodeForge/internal/port/feedback"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	cfmetrics "github.com/Strob0t/CodeForge/internal/port/metrics"
	"github.com/Strob0t/CodeForge/internal/telemetry"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// RuntimeService orchestrates the step-by-step execution protocol between
// Go (control plane) and Python (execution plane).
type RuntimeService struct {
	store         database.Store
	queue         messagequeue.Queue
	hub           broadcast.Broadcaster
	events        eventstore.Store
	policy        runtimePolicyEvaluator
	modes         runtimeModeProvider
	deliver       runtimeDeliverer
	contextOpt    runtimeContextOptimizer
	checkpoint    runtimeCheckpointer
	backlog       runtimeBacklogProbe
	sandbox       runtimeSandboxManager
	mcpSvc        runtimeMCPResolver
	microagentSvc runtimeMicroagentMatcher
	onRunComplete func(ctx context.Context, runID string, status run.Status)
	runtimeCfg    *config.Runtime
	state         *RunStateManager
	quarantine    runtimeQuarantineEvaluator

	feedbackProvidersMu sync.RWMutex
	feedbackProviders   []feedbackPort.Provider
	metrics             cfmetrics.Recorder
	goalSvc             runtimeGoalCreator
}

// NewRuntimeService creates a RuntimeService with all dependencies.
func NewRuntimeService(
	store database.Store,
	queue messagequeue.Queue,
	hub broadcast.Broadcaster,
	events eventstore.Store,
	policySvc runtimePolicyEvaluator,
	runtimeCfg *config.Runtime,
) *RuntimeService {
	return &RuntimeService{
		store:      store,
		queue:      queue,
		hub:        hub,
		events:     events,
		policy:     policySvc,
		runtimeCfg: runtimeCfg,
		state:      NewRunStateManager(),
	}
}

// SetDeliverService sets the delivery service for post-run delivery.
func (s *RuntimeService) SetDeliverService(d runtimeDeliverer) {
	s.deliver = d
}

// SetContextOptimizer sets the context optimizer for building context packs before runs.
func (s *RuntimeService) SetContextOptimizer(co runtimeContextOptimizer) {
	s.contextOpt = co
}

// SetOnRunComplete registers a callback invoked after a run reaches a terminal state.
// Used by the OrchestratorService to advance execution plans.
func (s *RuntimeService) SetOnRunComplete(fn func(context.Context, string, run.Status)) {
	s.onRunComplete = fn
}

// MarkConversationRunCancelled records that a conversation-based run has been
// cancelled so that its remaining tool-call requests are rejected immediately
// without waiting for policy evaluation, until the next run's start of the
// conversation is published or the stopped run reports its end. The
// conversation has no active run afterwards.
func (s *RuntimeService) MarkConversationRunCancelled(conversationID string) {
	s.state.SetCancelledConversation(conversationID)
	s.cleanupRunState(conversationID)
	slog.Info("conversation run marked cancelled", "conversation_id", conversationID)
}

// BeginConversationRun makes the run with turnID the conversation's active
// run before its start is dispatched. A conversation runs one run at a time:
// while another run is active it returns ErrConversationRunInProgress.
// Conversation runs reuse the conversation ID as run ID; the turn tells the
// active run's tool calls from those of a stopped run (KI-24).
func (s *RuntimeService) BeginConversationRun(conversationID, turnID string) error {
	if !s.state.BeginConversationRun(conversationID, turnID) {
		return ErrConversationRunInProgress
	}
	return nil
}

// ConversationRunDispatched records that the run's start was published: the
// mark of an earlier stop is cleared (the stopped run's calls report another
// turn and stay rejected).
func (s *RuntimeService) ConversationRunDispatched(conversationID, turnID string) {
	s.state.ConversationRunDispatched(conversationID, turnID)
}

// AbortConversationRun releases a run whose start was not published.
func (s *RuntimeService) AbortConversationRun(conversationID, turnID string) {
	s.state.AbortConversationRun(conversationID, turnID)
}

// EndConversationRun records a conversation run's reported end.
func (s *RuntimeService) EndConversationRun(conversationID, turnID string) {
	s.state.EndConversationRun(conversationID, turnID)
}

// IsActiveConversationRun reports whether the run with turnID is the
// conversation's active run.
func (s *RuntimeService) IsActiveConversationRun(conversationID, turnID string) bool {
	return s.state.IsActiveConversationRun(conversationID, turnID)
}

// ForgetConversation drops the run state of a deleted conversation.
func (s *RuntimeService) ForgetConversation(conversationID string) {
	s.state.ForgetConversation(conversationID)
}

// RegisterFeedbackProvider adds a feedback provider for HITL fan-out.
// BypassConversationApprovals marks a conversation so all future tool-call
// requests are auto-approved without HITL wait.
func (s *RuntimeService) BypassConversationApprovals(conversationID string) {
	s.state.SetBypassedConversation(conversationID)
	slog.Info("conversation approvals bypassed", "conversation_id", conversationID)
}

// IsConversationBypassed returns true if the conversation has "bypass all" enabled.
func (s *RuntimeService) IsConversationBypassed(conversationID string) bool {
	return s.state.IsConversationBypassed(conversationID)
}

func (s *RuntimeService) RegisterFeedbackProvider(p feedbackPort.Provider) {
	s.feedbackProvidersMu.Lock()
	s.feedbackProviders = append(s.feedbackProviders, p)
	s.feedbackProvidersMu.Unlock()
}

// SetCheckpointService sets the checkpoint service for shadow git commits.
func (s *RuntimeService) SetCheckpointService(cp runtimeCheckpointer) {
	s.checkpoint = cp
}

// SetBacklogProbe sets the probe the quality gate watchdog asks whether gate
// requests or results are still queued (FailStuckQualityGates). Without one
// only the watchdog's hard cap applies.
func (s *RuntimeService) SetBacklogProbe(p runtimeBacklogProbe) {
	s.backlog = p
}

// SetSandboxService sets the sandbox service for containerized execution.
func (s *RuntimeService) SetSandboxService(sb runtimeSandboxManager) {
	s.sandbox = sb
}

// SetModeService sets the mode service for resolving agent modes during run start.
func (s *RuntimeService) SetModeService(m runtimeModeProvider) {
	s.modes = m
}

// SetMCPService sets the MCP service for resolving MCP server definitions during run start.
func (s *RuntimeService) SetMCPService(svc runtimeMCPResolver) {
	s.mcpSvc = svc
}

// SetMicroagentService sets the microagent service for matching trigger-based prompts.
func (s *RuntimeService) SetMicroagentService(svc runtimeMicroagentMatcher) {
	s.microagentSvc = svc
}

// SetQuarantineService sets the quarantine service for pre-dispatch message filtering.
func (s *RuntimeService) SetQuarantineService(q runtimeQuarantineEvaluator) {
	s.quarantine = q
}

// SetMetrics sets the OTEL metrics collector.
func (s *RuntimeService) SetMetrics(m cfmetrics.Recorder) {
	s.metrics = m
}

// SetGoalService sets the goal discovery service for auto-persisting agent-proposed goals.
func (s *RuntimeService) SetGoalService(svc runtimeGoalCreator) {
	s.goalSvc = svc
}

// PersistGoalProposal creates a project goal from an agent proposal.
// Returns nil without error if no GoalDiscoveryService is wired.
func (s *RuntimeService) PersistGoalProposal(ctx context.Context, projectID, kind, title, content string, priority int) error {
	if s.goalSvc == nil {
		return nil
	}
	req := &goal.CreateRequest{
		Kind:     goal.GoalKind(kind),
		Title:    title,
		Content:  content,
		Priority: priority,
		Source:   "agent",
	}
	_, err := s.goalSvc.Create(ctx, projectID, req)
	return err
}

// SetHeartbeat sets the last heartbeat timestamp for a run. Intended for testing.
func (s *RuntimeService) SetHeartbeat(runID string, t time.Time) {
	s.state.SetHeartbeat(runID, t)
}

// LastHeartbeat returns the last heartbeat kept in memory for a run. Intended for testing.
func (s *RuntimeService) LastHeartbeat(runID string) (time.Time, bool) {
	return s.state.GetHeartbeat(runID)
}

// prepareSandbox creates and starts a sandbox or hybrid container for the run.
// Returns nil if exec mode is mount or if no sandbox service is configured.
func (s *RuntimeService) prepareSandbox(ctx context.Context, runID, projectID string, execMode run.ExecMode) error {
	if s.sandbox == nil {
		return nil
	}
	switch execMode {
	case run.ExecModeSandbox:
		proj, projErr := s.store.GetProject(ctx, projectID)
		if projErr != nil {
			return fmt.Errorf("get project for sandbox: %w", projErr)
		}
		if _, sbErr := s.sandbox.Create(ctx, runID, proj.WorkspacePath); sbErr != nil {
			return fmt.Errorf("sandbox create: %w", sbErr)
		}
		if sbErr := s.sandbox.Start(ctx, runID); sbErr != nil {
			_ = s.sandbox.Remove(ctx, runID)
			return fmt.Errorf("sandbox start: %w", sbErr)
		}
		slog.Info("sandbox started", "run_id", runID)

	case run.ExecModeHybrid:
		proj, projErr := s.store.GetProject(ctx, projectID)
		if projErr != nil {
			return fmt.Errorf("get project for hybrid: %w", projErr)
		}
		if _, sbErr := s.sandbox.CreateHybrid(ctx, runID, proj.WorkspacePath); sbErr != nil {
			return fmt.Errorf("hybrid create: %w", sbErr)
		}
		if sbErr := s.sandbox.Start(ctx, runID); sbErr != nil {
			_ = s.sandbox.Remove(ctx, runID)
			return fmt.Errorf("hybrid start: %w", sbErr)
		}
		slog.Info("hybrid container started", "run_id", runID)
	}
	return nil
}

// buildRunPayload assembles the NATS run start payload including context pack,
// MCP servers, and microagent prompts.
func (s *RuntimeService) buildRunPayload(
	ctx context.Context,
	r *run.Run, proj *project.Project, t *task.Task, ag *agent.Agent,
	profileName string, profile *policy.PolicyProfile,
	resolvedMode *messagequeue.ModePayload, modeID string,
	deliverMode run.DeliverMode,
) messagequeue.RunStartPayload {
	payload := messagequeue.RunStartPayload{
		RunID:         r.ID,
		TaskID:        t.ID,
		ProjectID:     t.ProjectID,
		AgentID:       ag.ID,
		TenantID:      tenantctx.FromContext(ctx),
		Prompt:        t.Prompt,
		PolicyProfile: profileName,
		ExecMode:      string(r.ExecMode),
		DeliverMode:   string(deliverMode),
		Mode:          resolvedMode,
		Config:        ag.Config,
		Termination: messagequeue.TerminationPayload{
			MaxSteps:       profile.Termination.MaxSteps,
			TimeoutSeconds: profile.Termination.TimeoutSeconds,
			MaxCost:        profile.Termination.MaxCost,
		},
		Trust:         trust.Internal(ag.ID),
		WorkspacePath: proj.WorkspacePath,
		Backend:       ag.Backend,
	}
	// The worker waits for policy responses longer than Go waits for a HITL
	// approval of one of the run's tool calls (KI-21).
	payload.ApprovalTimeoutSeconds = approvalTimeoutSeconds(s.runtimeCfg)

	// Build context pack if context optimizer is available.
	if s.contextOpt != nil {
		pack, packErr := s.contextOpt.BuildContextPack(ctx, r.TaskID, r.ProjectID, r.TeamID)
		if packErr != nil {
			slog.Warn("context pack build failed", "run_id", r.ID, "error", packErr)
		} else if pack != nil && len(pack.Entries) > 0 {
			payload.Context = toContextEntryPayloads(pack.Entries)
		}
	}

	// Resolve MCP server definitions for this run.
	if s.mcpSvc != nil {
		defs := s.mcpSvc.ResolveForRun(r.ProjectID, modeID)
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

	// Match microagents based on task prompt (trigger patterns).
	if s.microagentSvc != nil {
		matched, maErr := s.microagentSvc.Match(ctx, r.ProjectID, t.Prompt)
		if maErr != nil {
			slog.Warn("microagent match failed", "run_id", r.ID, "error", maErr)
		} else if len(matched) > 0 {
			for i := range matched {
				payload.MicroagentPrompts = append(payload.MicroagentPrompts, matched[i].Prompt)
			}
			slog.Info("microagents matched", "run_id", r.ID, "count", len(matched))
		}
	}

	return payload
}

// resolveRunMode resolves the mode for a run: explicit > agent default > "coder".
func (s *RuntimeService) resolveRunMode(modeID string, ag *agent.Agent) (string, *messagequeue.ModePayload) {
	if modeID == "" {
		modeID = ag.ModeID
	}
	if modeID == "" {
		modeID = "coder"
	}
	if s.modes == nil {
		return modeID, nil
	}
	m, mErr := s.modes.Get(modeID)
	if mErr != nil {
		slog.Warn("mode not found, using default", "mode_id", modeID, "error", mErr)
		return modeID, nil
	}
	_, sections := BuildModePrompt(m)
	sections = PruneToFitBudget(sections, DefaultModePromptBudget)
	assembledPrompt := AssembleSections(sections)
	return modeID, &messagequeue.ModePayload{
		ID:               m.ID,
		PromptPrefix:     assembledPrompt,
		Tools:            m.Tools,
		DeniedTools:      m.DeniedTools,
		DeniedActions:    m.DeniedActions,
		RequiredArtifact: m.RequiredArtifact,
		LLMScenario:      m.LLMScenario,
	}
}

// StartRun creates a new run in the database and publishes a start message to NATS.
func (s *RuntimeService) StartRun(ctx context.Context, req *run.StartRequest) (*run.Run, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate start request: %w", err)
	}

	proj, err := s.store.GetProject(ctx, req.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	// The run's tools edit this workspace.
	if err := requireWorkspace(proj); err != nil {
		return nil, err
	}
	execMode, err := resolveExecMode(req.ExecMode, proj)
	if err != nil {
		return nil, err
	}
	req.ExecMode = execMode

	// Resolve and validate policy profile: the request's, else the one the
	// project selects (as for conversations), else the default (KI-69).
	profileName := req.PolicyProfile
	if profileName == "" {
		profileName = projectPolicyProfile(proj)
	}
	if profileName == "" {
		profileName = s.policy.DefaultProfile()
	}
	profile, ok := s.policy.GetProfile(ctx, profileName)
	if !ok {
		return nil, fmt.Errorf("unknown policy profile %q", profileName)
	}

	ag, err := s.store.GetAgent(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if err := requireProject("agent", ag.ID, ag.ProjectID, req.ProjectID); err != nil {
		return nil, err
	}

	modeID, resolvedMode := s.resolveRunMode(req.ModeID, ag)

	t, err := s.store.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if err := requireProject("task", t.ID, t.ProjectID, req.ProjectID); err != nil {
		return nil, err
	}

	deliverMode := req.DeliverMode
	if deliverMode == "" && s.runtimeCfg.DefaultDeliverMode != "" {
		deliverMode = run.DeliverMode(s.runtimeCfg.DefaultDeliverMode)
	}

	// Create run in DB.
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

	if err := s.store.UpdateRunStatus(ctx, r.ID, run.StatusRunning, 0, 0, 0, 0); err != nil {
		s.endPendingRun(ctx, r, err)
		return nil, fmt.Errorf("update run status: %w", err)
	}
	r.Status = run.StatusRunning

	// OTEL: start run span and record metric.
	_, runSpan := telemetry.StartRunSpan(ctx, r.ID, r.TaskID, r.ProjectID)
	s.state.SetRunSpan(r.ID, runSpan)
	if s.metrics != nil {
		s.metrics.RecordRunStarted(ctx, "project.id", r.ProjectID, "exec_mode", string(req.ExecMode))
	}

	logBestEffort(ctx, s.store.UpdateAgentStatus(ctx, req.AgentID, agent.StatusRunning), "UpdateAgentStatus", slog.String("agent_id", req.AgentID))
	logBestEffort(ctx, s.store.UpdateTaskStatus(ctx, req.TaskID, task.StatusRunning), "UpdateTaskStatus", slog.String("task_id", req.TaskID))

	// Start sandbox/hybrid container if applicable.
	if err := s.prepareSandbox(ctx, r.ID, req.ProjectID, req.ExecMode); err != nil {
		return nil, s.failStartedRun(ctx, r, err)
	}

	// Create stall tracker if policy enables stall detection.
	if profile.Termination.StallDetection {
		threshold := profile.Termination.StallThreshold
		if threshold <= 0 {
			threshold = s.runtimeCfg.StallThreshold
		}
		s.state.SetStallTracker(r.ID, run.NewStallTracker(threshold, s.runtimeCfg.StallMaxRetries))
	}

	// Build and publish NATS payload.
	payload := s.buildRunPayload(ctx, r, proj, t, ag, profileName, &profile, resolvedMode, modeID, deliverMode)

	// Quarantine gate: check if message should be held for review.
	if s.quarantine != nil {
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			slog.Warn("quarantine marshal failed, skipping check", "error", marshalErr)
		} else {
			blocked, qErr := s.quarantine.Evaluate(ctx, payload.Trust, messagequeue.SubjectRunStart, data, payload.ProjectID)
			if qErr != nil {
				slog.Warn("quarantine evaluation failed, allowing message", "run_id", r.ID, "error", qErr)
			}
			if blocked {
				return r, nil
			}
		}
	}

	if err := s.publishJSON(ctx, messagequeue.SubjectRunStart, payload); err != nil {
		return nil, s.failStartedRun(ctx, r, fmt.Errorf("publish run start: %w", err))
	}

	// Record event.
	s.appendRunEvent(ctx, event.TypeRunStarted, r, map[string]string{
		"policy_profile": profileName,
		"exec_mode":      string(req.ExecMode),
		"backend":        ag.Backend,
		"mode_id":        modeID,
	})

	// Broadcast WS.
	s.broadcastRunStatus(ctx, r, r.Status)

	s.hub.BroadcastEvent(ctx, event.AGUIRunStarted, event.AGUIRunStartedEvent{
		RunID:     r.ID,
		AgentName: ag.Name,
	})

	// Start context-level timeout goroutine.
	if profile.Termination.TimeoutSeconds > 0 {
		timeoutDur := time.Duration(profile.Termination.TimeoutSeconds) * time.Second
		// The timer outlives the request: keep only the run's tenant so the
		// lookup, the cancellation and its WebSocket events stay in it.
		runCtx := detachTenant(ctx)
		timeoutCtx, timeoutCancel := context.WithCancel(runCtx)
		s.state.SetRunTimeout(r.ID, timeoutCancel)
		go func(runID string, timeout time.Duration) { //nolint:gosec // G118: timeout goroutine outlives request; cancel stored in s.state
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-timer.C:
				rr, err := s.store.GetRun(runCtx, runID)
				if err != nil || rr.Status != run.StatusRunning {
					return
				}
				slog.Warn("context-level timeout, cancelling run", "run_id", runID, "timeout", timeout)
				logRunUpdate(runCtx, s.cancelRunWithReason(runCtx, runID, "context-level timeout"), "cancelRunWithReason", runID)
			case <-timeoutCtx.Done():
				return
			}
		}(r.ID, timeoutDur)
	}

	s.appendAudit(ctx, r, "run.started", fmt.Sprintf("Run started with policy %s, exec_mode %s, agent %s, mode %s", profileName, req.ExecMode, ag.Name, modeID))

	slog.Info("run started", "run_id", r.ID, "task_id", r.TaskID, "policy", profileName)
	return r, nil
}

// startCleanupTimeout bounds the writes that end a run which could not be
// started.
const startCleanupTimeout = 30 * time.Second

// startCleanupContext is the context that ends a run which could not be
// started: the start may have failed because the request's context was
// cancelled (client gone), and the run must not stay running because of it.
// It keeps the context's values (tenant).
func startCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), startCleanupTimeout)
}

// failStartedRun ends a run that was created and marked running but could not
// be started (sandbox, dispatch): it goes through the completion path as
// failed, so that run, task and agent do not stay running. No worker executes
// it, so none is told to stop, and no agent work happened, so the agent's
// statistics do not count it. It returns startErr for StartRun to return.
func (s *RuntimeService) failStartedRun(ctx context.Context, r *run.Run, startErr error) error {
	ctx, cancel := startCleanupContext(ctx)
	defer cancel()
	logRunUpdate(ctx, s.endRun(ctx, r, run.StatusFailed, &messagequeue.RunCompletePayload{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    string(run.StatusFailed),
		Error:     "run could not be started: " + startErr.Error(),
	}, runEnd{}), "endRun", r.ID)
	return startErr
}

// endPendingRun ends a run that was created but could not be marked running:
// only its record is ended as failed, since its task and agent were not
// touched yet and nothing was announced for it.
func (s *RuntimeService) endPendingRun(ctx context.Context, r *run.Run, startErr error) {
	ctx, cancel := startCleanupContext(ctx)
	defer cancel()
	logRunUpdate(ctx, s.store.CompleteRun(ctx, &run.CompletionRequest{
		ID: r.ID, Status: run.StatusFailed, Error: "run could not be started: " + startErr.Error(),
	}), "CompleteRun", r.ID)
}

// CancelRun cancels an active run on the user's request and tells the worker
// to stop it.
func (s *RuntimeService) CancelRun(ctx context.Context, runID string) error {
	r, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}

	if r.Status != run.StatusRunning && r.Status != run.StatusPending && r.Status != run.StatusQualityGate {
		return fmt.Errorf("run %s is not active (status: %s)", runID, r.Status)
	}

	if err := s.stopRun(ctx, r, run.StatusCancelled, "cancelled by user"); err != nil {
		// Another cancel of the run (a double click, a plan cancel) ended it
		// first: the run is cancelled as requested.
		if errors.Is(err, domain.ErrConflict) && s.endedCancelled(ctx, runID) {
			slog.Info("run already cancelled", "run_id", runID)
			return nil
		}
		return err
	}
	slog.Info("run cancelled", "run_id", runID)
	return nil
}

// endedCancelled reports whether the run is stored as cancelled.
func (s *RuntimeService) endedCancelled(ctx context.Context, runID string) bool {
	r, err := s.store.GetRun(ctx, runID)
	return err == nil && r.Status == run.StatusCancelled
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
// Each handler is a named method on RuntimeService (see runtime_subscribers.go).
// Returns cancel functions for each subscription.
func (s *RuntimeService) StartSubscribers(ctx context.Context) ([]func(), error) {
	type sub struct {
		subject string
		handler func(context.Context, []byte) error
		label   string
	}

	subs := []sub{
		{messagequeue.SubjectRunToolCallRequest, s.handleToolCallRequest, "tool call request"},
		{messagequeue.SubjectRunToolCallResult, s.handleToolCallResult, "tool call result"},
		{messagequeue.SubjectRunComplete, s.handleRunComplete, "run complete"},
		{messagequeue.SubjectQualityGateResult, s.handleQualityGateResult, "quality gate result"},
		{messagequeue.SubjectRunHeartbeat, s.handleHeartbeat, "heartbeat"},
		{messagequeue.SubjectRunOutput, s.handleRunOutput, "run output"},
		{messagequeue.SubjectTrajectoryEvent, s.handleTrajectoryEvent, "trajectory events"},
		{messagequeue.SubjectRunStart + deadLetterSuffix, s.handleDeadLetteredRunStart, "dead-lettered run starts"},
	}

	var cancels []func()
	for _, entry := range subs {
		handler := entry.handler // capture for closure
		cancel, err := s.queue.Subscribe(ctx, entry.subject, func(msgCtx context.Context, _ string, data []byte) error {
			return handler(msgCtx, data)
		})
		if err != nil {
			cancelAll(cancels)
			return nil, fmt.Errorf("subscribe %s: %w", entry.label, err)
		}
		cancels = append(cancels, cancel)
	}

	return cancels, nil
}
