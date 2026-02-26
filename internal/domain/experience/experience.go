// Package experience provides the domain model for the experience pool
// (caching successful agent runs for similarity-based reuse).
package experience

import (
	"errors"
	"time"
)

// Entry represents a cached agent run result that can be reused.
type Entry struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	ProjectID       string    `json:"project_id"`
	TaskDescription string    `json:"task_description"`
	TaskEmbedding   []byte    `json:"-"`
	ResultOutput    string    `json:"result_output"`
	ResultCost      float64   `json:"result_cost"`
	ResultStatus    string    `json:"result_status"`
	RunID           string    `json:"run_id"`
	Confidence      float64   `json:"confidence"`
	HitCount        int       `json:"hit_count"`
	CreatedAt       time.Time `json:"created_at"`
	LastUsedAt      time.Time `json:"last_used_at"`
}

// CreateRequest is the input for storing a new experience entry.
type CreateRequest struct {
	ProjectID       string  `json:"project_id"`
	TaskDescription string  `json:"task_description"`
	ResultOutput    string  `json:"result_output"`
	ResultCost      float64 `json:"result_cost"`
	ResultStatus    string  `json:"result_status"`
	RunID           string  `json:"run_id"`
}

// Validate checks required fields on a CreateRequest.
func (r *CreateRequest) Validate() error {
	if r.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if r.TaskDescription == "" {
		return errors.New("task_description is required")
	}
	if r.ResultOutput == "" {
		return errors.New("result_output is required")
	}
	return nil
}
