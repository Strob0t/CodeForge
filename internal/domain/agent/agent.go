// Package agent defines the Agent domain entity.
package agent

import (
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/resource"
)

// Status represents the current state of an agent.
type Status string

const (
	StatusIdle    Status = "idle"
	StatusRunning Status = "running"
	StatusError   Status = "error"
	StatusStopped Status = "stopped"
)

// Agent represents an AI coding agent instance.
type Agent struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id,omitempty"`
	ProjectID      string            `json:"project_id"`
	Name           string            `json:"name"`
	Backend        string            `json:"backend"`
	Status         Status            `json:"status"`
	Config         map[string]string `json:"config"`
	ResourceLimits *resource.Limits  `json:"resource_limits,omitempty"`
	Version        int               `json:"version"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}
