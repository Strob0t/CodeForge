// Package quarantine defines domain types for the message quarantine system.
// Messages with low trust scores are intercepted before NATS dispatch,
// risk-scored, and held for admin review.
package quarantine

import "time"

// Verdict is the outcome of screening a message.
type Verdict string

const (
	// VerdictPass lets the message through.
	VerdictPass Verdict = "pass"
	// VerdictHeld holds the message for an admin's review; approving it
	// publishes it to its subject.
	VerdictHeld Verdict = "held"
	// VerdictRejected blocks the message for good.
	VerdictRejected Verdict = "rejected"
)

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
	ID          string   `json:"id"`
	TenantID    string   `json:"tenant_id"`
	ProjectID   string   `json:"project_id"`
	Subject     string   `json:"subject"`
	Payload     []byte   `json:"payload"`
	TrustOrigin string   `json:"trust_origin"`
	TrustLevel  string   `json:"trust_level"`
	RiskScore   float64  `json:"risk_score"`
	RiskFactors []string `json:"risk_factors"`
	Status      Status   `json:"status"`
	// ReviewedByID is the user who reviewed the message (empty before the
	// review and after the reviewer's erasure); ReviewedBy is the reviewer's
	// name at the time.
	ReviewedByID string     `json:"reviewed_by_id,omitempty"`
	ReviewedBy   string     `json:"reviewed_by"`
	ReviewNote   string     `json:"review_note"`
	CreatedAt    time.Time  `json:"created_at"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	ExpiresAt    time.Time  `json:"expires_at"`
}

// Review is an admin's decision on a quarantined message: the logged-in
// reviewer (user ID and name at the time) and an optional note.
type Review struct {
	ReviewerID   string
	ReviewerName string
	Note         string
}

// ErasedReviewerName replaces the reviewer name of the reviews of a user
// whose personal data was erased (GDPR Art. 17).
const ErasedReviewerName = "Deleted user"

// Stats holds aggregate counts by quarantine status.
type Stats struct {
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Rejected int `json:"rejected"`
	Expired  int `json:"expired"`
}
