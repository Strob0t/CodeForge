// Package quarantine defines domain types for the message quarantine system.
// Messages with low trust scores are intercepted before NATS dispatch,
// risk-scored, and held for admin review.
package quarantine

import "time"

// Status represents the review state of a quarantined message.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
)

// Message holds a quarantined NATS message awaiting review.
type Message struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	Subject     string     `json:"subject"`
	Payload     []byte     `json:"payload"`
	TrustOrigin string     `json:"trust_origin"`
	TrustLevel  string     `json:"trust_level"`
	RiskScore   float64    `json:"risk_score"`
	RiskFactors []string   `json:"risk_factors"`
	Status      Status     `json:"status"`
	ReviewedBy  string     `json:"reviewed_by"`
	ReviewNote  string     `json:"review_note"`
	CreatedAt   time.Time  `json:"created_at"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

// Stats holds aggregate counts by quarantine status.
type Stats struct {
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Rejected int `json:"rejected"`
	Expired  int `json:"expired"`
}
