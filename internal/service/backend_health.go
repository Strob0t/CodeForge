package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// BackendHealthEntry represents the health status of a single backend.
type BackendHealthEntry struct {
	Name         string               `json:"name"`
	DisplayName  string               `json:"display_name"`
	Available    bool                 `json:"available"`
	Error        string               `json:"error,omitempty"`
	Capabilities []string             `json:"capabilities"`
	ConfigFields []BackendConfigField `json:"config_fields,omitempty"`
}

// BackendConfigField describes a single configuration key for a backend.
type BackendConfigField struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Default     any    `json:"default,omitempty"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// backendHealthResult is the NATS reply from the Python worker.
type backendHealthResult struct {
	RequestID string               `json:"request_id"`
	Backends  []BackendHealthEntry `json:"backends"`
}

// BackendHealthService checks availability of Python-side backend executors via NATS.
type BackendHealthService struct {
	queue  messagequeue.Queue
	waiter *syncWaiter[backendHealthResult]
}

// NewBackendHealthService creates a new BackendHealthService.
func NewBackendHealthService(queue messagequeue.Queue) *BackendHealthService {
	return &BackendHealthService{
		queue:  queue,
		waiter: newSyncWaiter[backendHealthResult]("backend-health"),
	}
}

// CheckHealth publishes a health check request and waits for the Python worker response.
func (s *BackendHealthService) CheckHealth(ctx context.Context) ([]BackendHealthEntry, error) {
	requestID, err := generateRequestID()
	if err != nil {
		return nil, err
	}

	ch := s.waiter.register(requestID)
	defer s.waiter.unregister(requestID)

	payload, err := json.Marshal(map[string]string{"request_id": requestID})
	if err != nil {
		return nil, fmt.Errorf("marshal backend health request: %w", err)
	}
	if err := s.queue.Publish(ctx, messagequeue.SubjectBackendHealthRequest, payload); err != nil {
		return nil, fmt.Errorf("publish backend health request: %w", err)
	}

	select {
	case result := <-ch:
		if result == nil {
			return nil, fmt.Errorf("nil backend health result")
		}
		return result.Backends, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("backend health check timed out: %w", ctx.Err())
	}
}

// HandleHealthResult delivers a health check result from the Python worker.
func (s *BackendHealthService) HandleHealthResult(_ context.Context, data []byte) error {
	var result backendHealthResult
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("unmarshal backend health result: %w", err)
	}
	s.waiter.deliver(result.RequestID, &result)
	return nil
}
