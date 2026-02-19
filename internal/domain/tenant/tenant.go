// Package tenant defines the tenant domain model for multi-tenancy.
package tenant

import "time"

// Tenant represents an isolated tenant in the system.
type Tenant struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
	Enabled   bool              `json:"enabled"`
	Settings  map[string]string `json:"settings,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// CreateRequest holds the fields required to create a new tenant.
type CreateRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// UpdateRequest holds the fields that can be updated on a tenant.
type UpdateRequest struct {
	Name    string `json:"name,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}
