package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	cfotel "github.com/Strob0t/CodeForge/internal/adapter/otel"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	feedbackPort "github.com/Strob0t/CodeForge/internal/port/feedback"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// RuntimeService orchestrates the step-by-step execution protocol between
// Go (control plane) and Python (execution plane).
type RuntimeService struct {
	store             database.Store
	queue             messagequeue.Queue
	hub               broadcast.Broadcaster
	events            eventstore.Store
	policy            *PolicyService
	modes             *ModeService
	deliver           *DeliverService
	contextOpt        *ContextOptimizerService
	checkpoint        *CheckpointService
	sandbox           *SandboxService
	mcpSvc            *MCPService
	microagentSvc     *MicroagentService
	onRunComplete     func(ctx context.Context, runID string, status run.Status)
	runtimeCfg        *config.Runtime
	stallTrackers     sync.Map // map[runID]*run.StallTracker
	heartbeats        sync.Map // map[runID]time.Time — last heartbeat timestamp
	runTimeouts       sync.Map // map[runID]context.CancelFunc — context-level timeout cancel
	budgetAlerts      sync.Map // map["runID:threshold"]bool — dedup budget alerts
	pendingApprovals  sync.Map // map["runID:callID"]chan string — HITL approval channels
	feedbackProviders []feedbackPort.Provider
	metrics           *cfotel.Metrics
	runSpans          sync.Map // map[runID]trace.Span
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

// RegisterFeedbackProvider adds a feedback provider for HITL fan-out.
func (s *RuntimeService) RegisterFeedbackProvider(p feedbackPort.Provider) {
	s.feedbackProviders = append(s.feedbackProviders, p)
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

// SetMicroagentService sets the microagent service for matching trigger-based prompts.
func (s *RuntimeService) SetMicroagentService(svc *MicroagentService) {
	s.microagentSvc = svc
}

// SetMetrics sets the OTEL metrics collector.
func (s *RuntimeService) SetMetrics(m *cfotel.Metrics) {
	s.metrics = m
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

	// OTEL: start run span and record metric
	_, runSpan := cfotel.StartRunSpan(ctx, r.ID, r.TaskID, r.ProjectID)
	s.runSpans.Store(r.ID, runSpan)
	if s.metrics != nil {
		s.metrics.RunsStarted.Add(ctx, 1, metric.WithAttributes(
			attribute.String("project.id", r.ProjectID),
			attribute.String("exec_mode", string(req.ExecMode)),
		))
	}

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

	// Match microagents based on task prompt (trigger patterns).
	if s.microagentSvc != nil {
		matched, maErr := s.microagentSvc.Match(ctx, req.ProjectID, t.Prompt)
		if maErr != nil {
			slog.Warn("microagent match failed", "run_id", r.ID, "error", maErr)
		} else if len(matched) > 0 {
			for i := range matched {
				payload.MicroagentPrompts = append(payload.MicroagentPrompts, matched[i].Prompt)
			}
			slog.Info("microagents matched", "run_id", r.ID, "count", len(matched))
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
