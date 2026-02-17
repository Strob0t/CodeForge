package ws

import (
	"context"
	"encoding/json"
	"log/slog"
)

// Event type constants for WebSocket messages.
const (
	EventTaskStatus  = "task.status"
	EventTaskOutput  = "task.output"
	EventAgentStatus = "agent.status"
)

// TaskStatusEvent is broadcast when a task's status changes.
type TaskStatusEvent struct {
	TaskID    string `json:"task_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
	AgentID   string `json:"agent_id,omitempty"`
}

// TaskOutputEvent is broadcast when a task produces streaming output.
type TaskOutputEvent struct {
	TaskID string `json:"task_id"`
	Line   string `json:"line"`
	Stream string `json:"stream"` // "stdout" or "stderr"
}

// AgentStatusEvent is broadcast when an agent's status changes.
type AgentStatusEvent struct {
	AgentID   string `json:"agent_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
}

// BroadcastEvent is a convenience method that marshals a typed event and broadcasts it.
func (h *Hub) BroadcastEvent(ctx context.Context, eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal ws event payload", "type", eventType, "error", err)
		return
	}

	h.Broadcast(ctx, Message{
		Type:    eventType,
		Payload: json.RawMessage(data),
	})
}
