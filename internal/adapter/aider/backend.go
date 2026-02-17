// Package aider implements the agentbackend.Backend interface for the Aider coding agent.
package aider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

const backendName = "aider"

// Backend dispatches tasks to a Python worker running Aider via NATS.
type Backend struct {
	queue messagequeue.Queue
}

// New creates an Aider backend with the given NATS queue.
func New(queue messagequeue.Queue) *Backend {
	return &Backend{queue: queue}
}

// Register registers the Aider backend factory with the given NATS queue.
func Register(queue messagequeue.Queue) {
	agentbackend.Register(backendName, func(_ map[string]string) (agentbackend.Backend, error) {
		return New(queue), nil
	})
}

// Name returns "aider".
func (b *Backend) Name() string { return backendName }

// Capabilities returns what Aider supports.
func (b *Backend) Capabilities() agentbackend.Capabilities {
	return agentbackend.Capabilities{
		Edit:    true,
		Planner: true,
	}
}

// Execute dispatches a task to the Python Aider worker via NATS.
// Returns nil result because execution is asynchronous — the result
// arrives later on the tasks.result subject.
func (b *Backend) Execute(ctx context.Context, t *task.Task) (*task.Result, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("aider: marshal task: %w", err)
	}

	subject := messagequeue.SubjectTaskAgent + "." + backendName
	if err := b.queue.Publish(ctx, subject, data); err != nil {
		return nil, fmt.Errorf("aider: publish task: %w", err)
	}

	return nil, nil
}

// Stop sends a cancel signal for a running task.
func (b *Backend) Stop(ctx context.Context, taskID string) error {
	data, err := json.Marshal(map[string]string{"task_id": taskID, "action": "cancel"})
	if err != nil {
		return fmt.Errorf("aider: marshal cancel: %w", err)
	}

	if err := b.queue.Publish(ctx, messagequeue.SubjectTaskCancel, data); err != nil {
		return fmt.Errorf("aider: publish cancel: %w", err)
	}

	return nil
}
