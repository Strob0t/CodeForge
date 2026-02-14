// Package agentbackend defines the agent backend port (interface) and capabilities.
package agentbackend

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// Capabilities declares which operations an agent backend supports.
type Capabilities struct {
	Edit     bool `json:"edit"`
	Terminal bool `json:"terminal"`
	Browser  bool `json:"browser"`
	Planner  bool `json:"planner"`
	Review   bool `json:"review"`
}

// Backend is the port interface for interacting with a coding agent backend.
type Backend interface {
	// Name returns the unique identifier for this backend (e.g. "aider", "openhands").
	Name() string

	// Capabilities returns what this backend supports.
	Capabilities() Capabilities

	// Execute runs a task on the agent backend and returns the result.
	Execute(ctx context.Context, t *task.Task) (*task.Result, error)

	// Stop cancels a running task.
	Stop(ctx context.Context, taskID string) error
}
