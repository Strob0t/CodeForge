package database

import "time"

// A2ATaskFilter defines filters for listing A2A tasks.
type A2ATaskFilter struct {
	State     string
	Direction string
	ProjectID string
	TenantID  string
	Limit     int
	Cursor    string
}

// A2APushConfig represents a push notification configuration for an A2A task.
type A2APushConfig struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditEntry represents a single admin audit log record.
type AuditEntry struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	AdminID    string    `json:"admin_id"`
	AdminEmail string    `json:"admin_email"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	ResourceID string    `json:"resource_id,omitempty"`
	Details    []byte    `json:"details,omitempty"`
	IPAddress  string    `json:"ip_address,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
