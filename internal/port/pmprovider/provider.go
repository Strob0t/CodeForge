// Package pmprovider defines the port interface for project management providers
// (Plane, OpenProject, GitHub Issues, GitLab Issues, etc.).
package pmprovider

import (
	"context"
	"errors"
)

// ErrNotSupported is returned when a provider does not support the requested operation.
var ErrNotSupported = errors.New("operation not supported by this provider")

// Item represents a work item from a PM platform.
type Item struct {
	ID          string            `json:"id"`
	ExternalID  string            `json:"external_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Labels      []string          `json:"labels"`
	Priority    string            `json:"priority,omitempty"`
	Assignee    string            `json:"assignee,omitempty"`
	Meta        map[string]string `json:"meta,omitempty"`
}

// Capabilities declares what a PM provider supports.
type Capabilities struct {
	ListItems  bool `json:"list_items"`
	GetItem    bool `json:"get_item"`
	CreateItem bool `json:"create_item"`
	UpdateItem bool `json:"update_item"`
	Webhooks   bool `json:"webhooks"`
}

// Provider is the port interface for project management platforms.
type Provider interface {
	// Name returns the provider identifier (e.g., "plane", "openproject", "github-issues").
	Name() string

	// Capabilities returns what this provider supports.
	Capabilities() Capabilities

	// ListItems returns work items from the PM platform.
	ListItems(ctx context.Context, projectRef string) ([]Item, error)

	// GetItem returns a single work item by its external ID.
	GetItem(ctx context.Context, projectRef, itemID string) (*Item, error)

	// CreateItem creates a new work item in the PM platform.
	// Returns ErrNotSupported if the provider does not support creation.
	CreateItem(ctx context.Context, projectRef string, item *Item) (*Item, error)

	// UpdateItem updates an existing work item in the PM platform.
	// Returns ErrNotSupported if the provider does not support updates.
	UpdateItem(ctx context.Context, projectRef string, item *Item) (*Item, error)
}
