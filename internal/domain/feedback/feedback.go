// Package feedback provides the domain model for the human feedback provider
// protocol — multi-channel HITL approval with audit trail.
package feedback

import (
	"slices"
	"time"
)

// Provider identifies the channel through which feedback was collected.
type Provider string

const (
	ProviderWeb   Provider = "web"
	ProviderSlack Provider = "slack"
	ProviderEmail Provider = "email"
)

// Decision represents the approval outcome.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

// AuditEntry records a single HITL feedback decision for audit purposes.
type AuditEntry struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	RunID          string    `json:"run_id"`
	CallID         string    `json:"call_id"`
	Tool           string    `json:"tool"`
	Provider       Provider  `json:"provider"`
	Decision       Decision  `json:"decision"`
	Responder      string    `json:"responder"`
	ResponseTimeMs int       `json:"response_time_ms"`
	CreatedAt      time.Time `json:"created_at"`
}

// FeedbackRequest describes a tool call requiring human approval: what the
// web approval card shows, including the deciding policy profile and a
// truncated preview of the call's arguments (display only).
type FeedbackRequest struct {
	// TenantID is the tenant of the run asking ("" when unknown): providers
	// that send to operator-wide recipients answer only the tenants the
	// operator configured (SendsTo).
	TenantID         string `json:"tenant_id"`
	RunID            string `json:"run_id"`
	CallID           string `json:"call_id"`
	Tool             string `json:"tool"`
	Command          string `json:"command"`
	Path             string `json:"path"`
	Profile          string `json:"profile,omitempty"`
	ArgumentsPreview string `json:"arguments_preview,omitempty"`
}

// SendsTo reports whether an operator-configured approval channel (the
// Slack channel, the approval email recipients) receives the approval
// requests of tenantID: only the tenants listed in tenants
// (notification.approval_tenants, KI-84). A request without a tenant is
// never sent.
func SendsTo(tenants []string, tenantID string) bool {
	return tenantID != "" && slices.Contains(tenants, tenantID)
}

// FeedbackResult is the outcome of a feedback request.
type FeedbackResult struct {
	Decision  Decision `json:"decision"`
	Responder string   `json:"responder"`
	Provider  Provider `json:"provider"`
}
