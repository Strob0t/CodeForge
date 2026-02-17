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
	store         database.Store
	queue         messagequeue.Queue
	hub           broadcast.Broadcaster
	events        eventstore.Store
	policy        *PolicyService
	deliver       *DeliverService
	onRunComplete func(ctx context.Context, runID string, status run.Status)
	runtimeCfg    *config.Runtime
	stallTrackers sync.Map // map[runID]*run.StallTracker
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

// SetOnRunComplete registers a callback invoked after a run reaches a terminal state.
// Used by the OrchestratorService to advance execution plans.
func (s *RuntimeService) SetOnRunComplete(fn func(context.Context, string, run.Status)) {
	s.onRunComplete = fn
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
		PolicyProfile: profileName,
		ExecMode:      req.ExecMode,
		DeliverMode:   deliverMode,
		Status:        run.StatusPending,
	}
	if err := s.store.CreateRun(ctx, r); err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}

	// Mark run as running
	if err := s.store.UpdateRunStatus(ctx, r.ID, run.StatusRunning, 0, 0); err != nil {
		return nil, fmt.Errorf("update run status: %w", err)
	}
	r.Status = run.StatusRunning

	// Mark agent as running
	_ = s.store.UpdateAgentStatus(ctx, req.AgentID, agent.StatusRunning)

	// Mark task as running
	_ = s.store.UpdateTaskStatus(ctx, req.TaskID, task.StatusRunning)

	// Create stall tracker if policy enables stall detection
	if profile.Termination.StallDetection {
		threshold := profile.Termination.StallThreshold
		if threshold <= 0 {
			threshold = s.runtimeCfg.StallThreshold
		}
		s.stallTrackers.Store(r.ID, run.NewStallTracker(threshold))
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
		Config:        ag.Config,
		Termination: messagequeue.TerminationPayload{
			MaxSteps:       profile.Termination.MaxSteps,
			TimeoutSeconds: profile.Termination.TimeoutSeconds,
			MaxCost:        profile.Termination.MaxCost,
		},
	}
	if err := s.publishJSON(ctx, messagequeue.SubjectRunStart, payload); err != nil {
		return nil, fmt.Errorf("publish run start: %w", err)
	}

	// Record event
	s.appendRunEvent(ctx, event.TypeRunStarted, r, map[string]string{
		"policy_profile": profileName,
		"exec_mode":      string(req.ExecMode),
		"backend":        ag.Backend,
	})

	// Broadcast WS
	s.hub.BroadcastEvent(ctx, ws.EventRunStatus, ws.RunStatusEvent{
		RunID:     r.ID,
		TaskID:    r.TaskID,
		ProjectID: r.ProjectID,
		Status:    string(r.Status),
	})

	slog.Info("run started", "run_id", r.ID, "task_id", r.TaskID, "policy", profileName)
	return r, nil
}

// HandleToolCallRequest processes a tool call permission request from a worker.
// It evaluates termination conditions and policy rules, then publishes a response.
func (s *RuntimeService) HandleToolCallRequest(ctx context.Context, req *messagequeue.ToolCallRequestPayload) error {
	r, err := s.store.GetRun(ctx, req.RunID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
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
		_ = s.store.CompleteRun(ctx, r.ID, run.StatusTimeout, reason, r.CostUSD, r.StepCount)
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
		})
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), reason)
	}

	// Evaluate policy
	call := policy.ToolCall{
		Tool:    req.Tool,
		Command: req.Command,
		Path:    req.Path,
	}
	decision, err := s.policy.Evaluate(ctx, r.PolicyProfile, call)
	if err != nil {
		return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(policy.DecisionDeny), err.Error())
	}

	// Record event
	evType := event.TypeToolCallApproved
	if decision != policy.DecisionAllow {
		evType = event.TypeToolCallDenied
	}
	s.appendRunEvent(ctx, evType, r, map[string]string{
		"call_id":  req.CallID,
		"tool":     req.Tool,
		"decision": string(decision),
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

	// Increment step count
	newSteps := r.StepCount + 1
	_ = s.store.UpdateRunStatus(ctx, r.ID, run.StatusRunning, newSteps, r.CostUSD)

	return s.sendToolCallResponse(ctx, req.RunID, req.CallID, string(decision), "")
}

// HandleToolCallResult processes the outcome of an executed tool call.
func (s *RuntimeService) HandleToolCallResult(ctx context.Context, result *messagequeue.ToolCallResultPayload) error {
	r, err := s.store.GetRun(ctx, result.RunID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}

	// Update run cost
	newCost := r.CostUSD + result.CostUSD
	_ = s.store.UpdateRunStatus(ctx, r.ID, r.Status, r.StepCount, newCost)

	// Check stall detection
	if tracker, ok := s.stallTrackers.Load(r.ID); ok {
		st := tracker.(*run.StallTracker)
		if st.RecordStep(result.Tool, result.Success, result.Output) {
			// Stall detected — terminate run
			slog.Warn("stall detected, terminating run", "run_id", r.ID, "tool", result.Tool)
			_ = s.store.CompleteRun(ctx, r.ID, run.StatusFailed, "stall detected: agent not making progress", newCost, r.StepCount)
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
			})
			// Set agent idle, task failed
			_ = s.store.UpdateAgentStatus(ctx, r.AgentID, agent.StatusIdle)
			_ = s.store.UpdateTaskStatus(ctx, r.TaskID, task.StatusFailed)
			return nil
		}
	}

	// Record event
	s.appendRunEvent(ctx, event.TypeToolCallResultEv, r, map[string]string{
		"call_id": result.CallID,
		"tool":    result.Tool,
		"success": fmt.Sprintf("%t", result.Success),
		"cost":    fmt.Sprintf("%.6f", result.CostUSD),
	})

	// Broadcast WS
	s.hub.BroadcastEvent(ctx, ws.EventToolCallStatus, ws.ToolCallStatusEvent{
		RunID:  r.ID,
		CallID: result.CallID,
		Tool:   result.Tool,
		Phase:  "result",
	})

	return nil
}

// HandleRunComplete processes a run completion message from a worker.
func (s *RuntimeService) HandleRunComplete(ctx context.Context, payload *messagequeue.RunCompletePayload) error {
	r, err := s.store.GetRun(ctx, payload.RunID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}

	// Clean up stall tracker
	s.stallTrackers.Delete(r.ID)

	// Determine final status
	status := run.Status(payload.Status)
	if status == "" {
		if payload.Error != "" {
			status = run.StatusFailed
		} else {
			status = run.StatusCompleted
		}
	}

	// Check if quality gates should be triggered
	profile, ok := s.policy.GetProfile(r.PolicyProfile)
	hasGates := ok && status == run.StatusCompleted &&
		(profile.QualityGate.RequireTestsPass || profile.QualityGate.RequireLintPass)

	if hasGates {
		// Transition to quality_gate status — do not finalize yet
		if err := s.store.UpdateRunStatus(ctx, r.ID, run.StatusQualityGate, payload.StepCount, payload.CostUSD); err != nil {
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
			slog.Error("failed to publish quality gate request", "run_id", r.ID, "error", err)
			// Fall through to normal completion on publish failure
		} else {
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
			})

			slog.Info("quality gate triggered", "run_id", r.ID)
			return nil // Wait for quality gate result
		}
	}

	// No gates or publish failed — finalize immediately
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
	}

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

// finalizeRun completes the run lifecycle: update DB, task, agent, broadcast events.
func (s *RuntimeService) finalizeRun(ctx context.Context, r *run.Run, status run.Status, payload *messagequeue.RunCompletePayload) error {
	if err := s.store.CompleteRun(ctx, r.ID, status, payload.Error, payload.CostUSD, payload.StepCount); err != nil {
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
	})
	s.hub.BroadcastEvent(ctx, ws.EventAgentStatus, ws.AgentStatusEvent{
		AgentID:   r.AgentID,
		ProjectID: r.ProjectID,
		Status:    string(agent.StatusIdle),
	})

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

	result, deliverErr := s.deliver.Deliver(ctx, r, taskTitle)
	if deliverErr != nil {
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

	s.appendRunEvent(ctx, event.TypeDeliveryCompleted, r, map[string]string{
		"mode":        string(result.Mode),
		"patch_path":  result.PatchPath,
		"commit_hash": result.CommitHash,
		"branch_name": result.BranchName,
		"pr_url":      result.PRURL,
	})
	s.hub.BroadcastEvent(ctx, ws.EventDelivery, ws.DeliveryEvent{
		RunID:      r.ID,
		TaskID:     r.TaskID,
		ProjectID:  r.ProjectID,
		Status:     "completed",
		Mode:       string(result.Mode),
		PatchPath:  result.PatchPath,
		CommitHash: result.CommitHash,
		BranchName: result.BranchName,
		PRURL:      result.PRURL,
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

	// Clean up stall tracker
	s.stallTrackers.Delete(runID)

	// Update DB
	if err := s.store.CompleteRun(ctx, r.ID, run.StatusCancelled, "cancelled by user", r.CostUSD, r.StepCount); err != nil {
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
	})

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
	return ""
}

func (s *RuntimeService) sendToolCallResponse(ctx context.Context, runID, callID, decision, reason string) error {
	resp := messagequeue.ToolCallResponsePayload{
		RunID:    runID,
		CallID:   callID,
		Decision: decision,
		Reason:   reason,
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
		Type:      evType,
		Payload:   payloadJSON,
		RequestID: logger.RequestID(ctx),
		Version:   1,
	}
	if err := s.events.Append(ctx, &ev); err != nil {
		slog.Error("failed to append run event", "type", evType, "run_id", r.ID, "error", err)
	}
}

func cancelAll(fns []func()) {
	for _, fn := range fns {
		fn()
	}
}
