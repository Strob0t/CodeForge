package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
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

	// Mark agent as running
	if err := s.store.UpdateAgentStatus(ctx, agentID, agent.StatusRunning); err != nil {
		return fmt.Errorf("update agent status: %w", err)
	}

	// Update task with agent assignment and status
	t.AgentID = agentID
	if err := s.store.UpdateTaskStatus(ctx, taskID, task.StatusQueued); err != nil {
		return fmt.Errorf("update task status: %w", err)
	}

	// Dispatch to backend (async via NATS)
	if _, err := backend.Execute(ctx, t); err != nil {
		// Revert agent status on failure
		_ = s.store.UpdateAgentStatus(ctx, agentID, agent.StatusIdle)
		_ = s.store.UpdateTaskStatus(ctx, taskID, task.StatusPending)
		return fmt.Errorf("dispatch task: %w", err)
	}

	// Record event
	s.appendEvent(ctx, event.TypeAgentStarted, agentID, taskID, ag.ProjectID, map[string]string{
		"backend": ag.Backend,
		"task":    t.Title,
	})

	// Broadcast state changes
	s.hub.BroadcastEvent(ctx, ws.EventAgentStatus, ws.AgentStatusEvent{
		AgentID:   agentID,
		ProjectID: ag.ProjectID,
		Status:    string(agent.StatusRunning),
	})
	s.hub.BroadcastEvent(ctx, ws.EventTaskStatus, ws.TaskStatusEvent{
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

	_ = s.store.UpdateAgentStatus(ctx, agentID, agent.StatusIdle)
	_ = s.store.UpdateTaskStatus(ctx, taskID, task.StatusCancelled)

	// Record event
	s.appendEvent(ctx, event.TypeAgentError, agentID, taskID, ag.ProjectID, map[string]string{
		"reason": "stopped by user",
	})

	// Broadcast state changes
	s.hub.BroadcastEvent(ctx, ws.EventAgentStatus, ws.AgentStatusEvent{
		AgentID:   agentID,
		ProjectID: ag.ProjectID,
		Status:    string(agent.StatusIdle),
	})
	s.hub.BroadcastEvent(ctx, ws.EventTaskStatus, ws.TaskStatusEvent{
		TaskID:    taskID,
		ProjectID: ag.ProjectID,
		Status:    string(task.StatusCancelled),
		AgentID:   agentID,
	})

	return nil
}

// HandleResult processes a task result received from a worker.
func (s *AgentService) HandleResult(ctx context.Context, result task.Result, taskID, projectID string, costUSD float64) error {
	if err := s.store.UpdateTaskResult(ctx, taskID, result, costUSD); err != nil {
		return fmt.Errorf("update task result: %w", err)
	}

	status := string(task.StatusCompleted)
	evType := event.TypeAgentFinished
	if result.Error != "" {
		status = string(task.StatusFailed)
		evType = event.TypeAgentError
	}

	// Record event (agentID not available here, use empty string)
	s.appendEvent(ctx, evType, "", taskID, projectID, map[string]string{
		"status": status,
		"cost":   fmt.Sprintf("%.6f", costUSD),
		"output": truncate(result.Output, 200),
		"error":  result.Error,
	})

	s.hub.BroadcastEvent(ctx, ws.EventTaskStatus, ws.TaskStatusEvent{
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
		var result struct {
			TaskID    string   `json:"task_id"`
			ProjectID string   `json:"project_id"`
			Status    string   `json:"status"`
			Output    string   `json:"output"`
			Files     []string `json:"files"`
			Error     string   `json:"error"`
			TokensIn  int      `json:"tokens_in"`
			TokensOut int      `json:"tokens_out"`
			CostUSD   float64  `json:"cost_usd"`
		}

		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}

		taskResult := task.Result{
			Output:    result.Output,
			Files:     result.Files,
			Error:     result.Error,
			TokensIn:  result.TokensIn,
			TokensOut: result.TokensOut,
		}

		return s.HandleResult(msgCtx, taskResult, result.TaskID, result.ProjectID, result.CostUSD)
	})
}

// StartOutputSubscriber subscribes to streaming task output and forwards to WebSocket.
func (s *AgentService) StartOutputSubscriber(ctx context.Context) (cancel func(), err error) {
	return s.queue.Subscribe(ctx, messagequeue.SubjectTaskOutput, func(msgCtx context.Context, _ string, data []byte) error {
		var output ws.TaskOutputEvent
		if err := json.Unmarshal(data, &output); err != nil {
			return fmt.Errorf("unmarshal output: %w", err)
		}

		s.hub.BroadcastEvent(msgCtx, ws.EventTaskOutput, output)
		return nil
	})
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
	if err := s.events.Append(ctx, &ev); err != nil {
		slog.Error("failed to append event", "type", evType, "task_id", taskID, "error", err)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
