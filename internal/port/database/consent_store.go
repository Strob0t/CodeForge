package database

import (
	"context"
	"time"
)

// ConsentPurpose describes a data processing purpose for which consent can be given.
type ConsentPurpose struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	LegalBasis  string    `json:"legal_basis"`
	Required    bool      `json:"required"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ConsentRecord is an immutable record of a user granting or withdrawing consent.
type ConsentRecord struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	UserID         string    `json:"user_id"`
	PurposeID      string    `json:"purpose_id"`
	PurposeVersion int       `json:"purpose_version"`
	Granted        bool      `json:"granted"`
	IPAddress      string    `json:"ip_address,omitempty"`
	UserAgent      string    `json:"user_agent,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// ConsentStore manages consent purposes and user consent records.
type ConsentStore interface {
	// HasActiveConsent checks if the user has granted consent for the given purpose.
	// Returns the granted status of the most recent consent record.
	HasActiveConsent(ctx context.Context, userID, purposeID string) (bool, error)

	// RecordConsent appends an immutable consent record (grant or withdrawal).
	RecordConsent(ctx context.Context, record *ConsentRecord) error

	// ListUserConsents returns all consent records for a user, newest first.
	ListUserConsents(ctx context.Context, userID string) ([]ConsentRecord, error)

	// ListConsentPurposes returns all consent purposes for the current tenant.
	ListConsentPurposes(ctx context.Context) ([]ConsentPurpose, error)

	// GetConsentPurpose returns a specific consent purpose.
	GetConsentPurpose(ctx context.Context, purposeID string) (*ConsentPurpose, error)
}
