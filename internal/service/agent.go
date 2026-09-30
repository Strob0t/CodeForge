package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// AgentService handles agent lifecycle and task dispatch.
type AgentService struct {
	store  database.Store
	queue  messagequeue.Queue
	hub    broadcast.Broadcaster
	events eventstore.Store
}

// NewAgentService creates a new AgentService.
func NewAgentService(store database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster) *AgentService {
	return &AgentService{store: store, queue: queue, hub: hub}
}

// SetEventStore attaches an event store for trajectory recording.
func (s *AgentService) SetEventStore(es eventstore.Store) {
	s.events = es
}

// List returns all agents for a project.
func (s *AgentService) List(ctx context.Context, projectID string) ([]agent.Agent, error) {
	return s.store.ListAgents(ctx, projectID)
}

// Get returns an agent by ID.
func (s *AgentService) Get(ctx context.Context, id string) (*agent.Agent, error) {
	return s.store.GetAgent(ctx, id)
}

// Create creates a new agent for a project.
func (s *AgentService) Create(ctx context.Context, projectID, name, backend string, config map[string]string, limits *resource.Limits) (*agent.Agent, error) {
	// Verify the backend exists
	if _, err := agentbackend.New(backend, nil); err != nil {
		return nil, fmt.Errorf("unknown backend %q: %w", backend, err)
	}

	return s.store.CreateAgent(ctx, projectID, name, backend, config, limits)
}

// Delete removes an agent.
func (s *AgentService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteAgent(ctx, id)
}

// Dispatch sends a task to the agent's backend for execution.
func (s *AgentService) Dispatch(ctx context.Context, agentID, taskID string) error {
	ag, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}

	t, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	backend, err := agentbackend.New(ag.Backend, ag.Config)
	if err != nil {
		return fmt.Errorf("create backend: %w", err)
	}

	// The backend edits the agent's project workspace.
	if err := requireProject("task", t.ID, t.ProjectID, ag.ProjectID); err != nil {
		return err
	}
	proj, err := s.store.GetProject(ctx, ag.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if err := requireWorkspace(proj); err != nil {
		return err
	}

	// Mark agent as running
	if err := s.store.UpdateAgentStatus(ctx, agentID, agent.StatusRunning); err != nil {
		return fmt.Errorf("update agent status: %w", err)
	}

	// Update task with agent assignment and status
	t.AgentID = agentID
	if err := s.store.UpdateTaskStatus(ctx, taskID, task.StatusQueued); err != nil {
		return fmt.Errorf("update task status: %w", err)
	}

	// Dispatch to backend (async via NATS). The worker echoes the tenant in
	// its output and result messages, which scopes their WebSocket events.
	t.TenantID = tenantctx.FromContext(ctx)
	if _, err := backend.Execute(ctx, &agentbackend.Execution{Task: t, WorkspacePath: proj.WorkspacePath}); err != nil {
		// Revert agent status on failure
		logBestEffort(ctx, s.store.UpdateAgentStatus(ctx, agentID, agent.StatusIdle), "UpdateAgentStatus", slog.String("agent_id", agentID))
		logBestEffort(ctx, s.store.UpdateTaskStatus(ctx, taskID, task.StatusPending), "UpdateTaskStatus", slog.String("task_id", taskID))
		return fmt.Errorf("dispatch task: %w", err)
	}

	// Record event
	s.appendEvent(ctx, event.TypeAgentStarted, agentID, taskID, ag.ProjectID, map[string]string{
		"backend": ag.Backend,
		"task":    t.Title,
	})

	// Broadcast state changes
	s.hub.BroadcastEvent(ctx, event.EventAgentStatus, event.AgentStatusEvent{
		AgentID:   agentID,
		ProjectID: ag.ProjectID,
		Status:    string(agent.StatusRunning),
	})
	s.hub.BroadcastEvent(ctx, event.EventTaskStatus, event.TaskStatusEvent{
		TaskID:    taskID,
		ProjectID: t.ProjectID,
		Status:    string(task.StatusQueued),
		AgentID:   agentID,
	})

	return nil
}

// StopTask cancels a running task on an agent.
func (s *AgentService) StopTask(ctx context.Context, agentID, taskID string) error {
	ag, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}

	backend, err := agentbackend.New(ag.Backend, ag.Config)
	if err != nil {
		return fmt.Errorf("create backend: %w", err)
	}

	if err := backend.Stop(ctx, taskID); err != nil {
		return fmt.Errorf("stop task: %w", err)
	}

	logBestEffort(ctx, s.store.UpdateAgentStatus(ctx, agentID, agent.StatusIdle), "UpdateAgentStatus", slog.String("agent_id", agentID))
	logBestEffort(ctx, s.store.UpdateTaskStatus(ctx, taskID, task.StatusCancelled), "UpdateTaskStatus", slog.String("task_id", taskID))

	// Record event
	s.appendEvent(ctx, event.TypeAgentError, agentID, taskID, ag.ProjectID, map[string]string{
		"reason": "stopped by user",
	})

	// Broadcast state changes
	s.hub.BroadcastEvent(ctx, event.EventAgentStatus, event.AgentStatusEvent{
		AgentID:   agentID,
		ProjectID: ag.ProjectID,
		Status:    string(agent.StatusIdle),
	})
	s.hub.BroadcastEvent(ctx, event.EventTaskStatus, event.TaskStatusEvent{
		TaskID:    taskID,
		ProjectID: ag.ProjectID,
		Status:    string(task.StatusCancelled),
		AgentID:   agentID,
	})

	return nil
}

// HandleResult processes a task result received from a worker: an error
// result leaves the task failed, any other completed.
func (s *AgentService) HandleResult(ctx context.Context, result task.Result, taskID, projectID string, costUSD float64) error {
	status := task.StatusCompleted
	if result.Error != "" {
		status = task.StatusFailed
	}
	return s.recordResult(ctx, status, result, taskID, projectID, costUSD)
}

// HandleCancelledResult processes the result of a task the worker stopped on
// tasks.cancel: the task stays cancelled.
func (s *AgentService) HandleCancelledResult(ctx context.Context, result task.Result, taskID, projectID string, costUSD float64) error {
	return s.recordResult(ctx, task.StatusCancelled, result, taskID, projectID, costUSD)
}

// recordResult stores a worker's task result with the final status, records
// the event and broadcasts the status.
func (s *AgentService) recordResult(ctx context.Context, final task.Status, result task.Result, taskID, projectID string, costUSD float64) error {
	if err := s.store.UpdateTaskResult(ctx, taskID, final, result, costUSD); err != nil {
		return fmt.Errorf("update task result: %w", err)
	}
	status := string(final)
	evType := event.TypeAgentFinished
	if final != task.StatusCompleted {
		evType = event.TypeAgentError
	}

	// The result names no agent: record the task's. A task dispatched without
	// an assignment has none, and the event is stored without agent.
	agentID := ""
	t, err := s.store.GetTask(ctx, taskID)
	logBestEffort(ctx, err, "GetTask", slog.String("task_id", taskID))
	if err == nil {
		agentID = t.AgentID
	}

	s.appendEvent(ctx, evType, agentID, taskID, projectID, map[string]string{
		"status": status,
		"cost":   fmt.Sprintf("%.6f", costUSD),
		"output": truncate(result.Output, 200),
		"error":  result.Error,
	})

	s.hub.BroadcastEvent(ctx, event.EventTaskStatus, event.TaskStatusEvent{
		TaskID:    taskID,
		ProjectID: projectID,
		Status:    status,
	})

	slog.Info("task result processed", "task_id", taskID, "status", status)
	return nil
}

// StartResultSubscriber subscribes to task results from NATS and processes them.
func (s *AgentService) StartResultSubscriber(ctx context.Context) (cancel func(), err error) {
	return s.queue.Subscribe(ctx, messagequeue.SubjectTaskResult, func(msgCtx context.Context, _ string, data []byte) error {
		var result messagequeue.TaskResultPayload
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
		msgCtx = withPayloadTenant(msgCtx, result.TenantID)

		taskResult := task.Result{
			Output:    result.Output,
			Files:     result.Files,
			Error:     result.Error,
			TokensIn:  result.TokensIn,
			TokensOut: result.TokensOut,
		}

		if result.Status == string(task.StatusCancelled) {
			return s.HandleCancelledResult(msgCtx, taskResult, result.TaskID, result.ProjectID, result.CostUSD)
		}
		return s.HandleResult(msgCtx, taskResult, result.TaskID, result.ProjectID, result.CostUSD)
	})
}

// StartOutputSubscriber subscribes to streaming task output and forwards to WebSocket.
func (s *AgentService) StartOutputSubscriber(ctx context.Context) (cancel func(), err error) {
	return s.queue.Subscribe(ctx, messagequeue.SubjectTaskOutput, func(msgCtx context.Context, _ string, data []byte) error {
		var output struct {
			event.TaskOutputEvent
			TenantID string `json:"tenant_id"`
		}
		if err := json.Unmarshal(data, &output); err != nil {
			return fmt.Errorf("unmarshal output: %w", err)
		}

		s.hub.BroadcastEvent(withPayloadTenant(msgCtx, output.TenantID), event.EventTaskOutput, output.TaskOutputEvent)
		return nil
	})
}

// StartAgentOutputSubscriber subscribes to agent backend output and forwards to WebSocket.
func (s *AgentService) StartAgentOutputSubscriber(ctx context.Context) (cancel func(), err error) {
	return s.queue.Subscribe(ctx, messagequeue.SubjectAgentOutput, func(msgCtx context.Context, _ string, data []byte) error {
		var output struct {
			event.AgentOutputEvent
			TenantID string `json:"tenant_id"`
		}
		if err := json.Unmarshal(data, &output); err != nil {
			slog.Error("malformed agent output message", "error", err)
			return nil // log and skip, don't fail subscription
		}
		s.hub.BroadcastEvent(withPayloadTenant(msgCtx, output.TenantID), event.EventAgentOutput, output.AgentOutputEvent)
		return nil
	})
}

// SendMessage validates and stores an inbox message, then broadcasts a WS event.
func (s *AgentService) SendMessage(ctx context.Context, msg *agent.InboxMessage) error {
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("validate inbox message: %w", err)
	}
	if err := s.store.SendAgentMessage(ctx, msg); err != nil {
		return fmt.Errorf("store inbox message: %w", err)
	}
	s.hub.BroadcastEvent(ctx, event.EventAgentMessage, event.AgentMessageEvent{
		AgentID:   msg.AgentID,
		FromAgent: msg.FromAgent,
		Content:   msg.Content,
	})
	return nil
}

// GetInbox returns inbox messages for an agent.
func (s *AgentService) GetInbox(ctx context.Context, agentID string, unreadOnly bool) ([]agent.InboxMessage, error) {
	return s.store.ListAgentInbox(ctx, agentID, unreadOnly)
}

// MarkRead marks a single inbox message as read.
func (s *AgentService) MarkRead(ctx context.Context, messageID string) error {
	return s.store.MarkInboxRead(ctx, messageID)
}

// IncrementStats updates run count, cost, and success rate for an agent.
func (s *AgentService) IncrementStats(ctx context.Context, agentID string, costDelta float64, success bool) error {
	return s.store.IncrementAgentStats(ctx, agentID, costDelta, success)
}

// UpdateState replaces the agent's key-value state map.
func (s *AgentService) UpdateState(ctx context.Context, agentID string, state map[string]string) error {
	return s.store.UpdateAgentState(ctx, agentID, state)
}

// LoadTaskEvents returns all events for a task from the event store.
func (s *AgentService) LoadTaskEvents(ctx context.Context, taskID string) ([]event.AgentEvent, error) {
	if s.events == nil {
		return nil, nil
	}
	return s.events.LoadByTask(ctx, taskID)
}

// appendEvent records an event to the event store (best-effort, logs errors).
func (s *AgentService) appendEvent(ctx context.Context, evType event.Type, agentID, taskID, projectID string, payload map[string]string) {
	if s.events == nil {
		return
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal event payload", "error", err)
		return
	}
	ev := event.AgentEvent{
		AgentID:   agentID,
		TaskID:    taskID,
		ProjectID: projectID,
		Type:      evType,
		Payload:   payloadJSON,
		RequestID: logger.RequestID(ctx),
		Version:   1,
	}
	logBestEffort(ctx, s.events.Append(ctx, &ev), "AppendEvent", slog.String("type", string(evType)), slog.String("task_id", taskID))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
