package service

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/port/database"
)

// ConsentService wraps the consent store with validation.
type ConsentService struct {
	db database.ConsentStore
}

// NewConsentService creates a new consent service backed by the given store.
func NewConsentService(db database.ConsentStore) *ConsentService {
	return &ConsentService{db: db}
}

// ConsentStatus pairs a purpose with the user's current granted state.
// ConsentStatus represents the current consent state for a single purpose.
type ConsentStatus struct {
	PurposeID string `json:"purpose_id"`
	Granted   bool   `json:"granted"`
}

// ListPurposes returns all consent purposes for the current tenant.
func (s *ConsentService) ListPurposes(ctx context.Context) ([]database.ConsentPurpose, error) {
	return s.db.ListConsentPurposes(ctx)
}

// GetStatus returns the current consent state for a user, keyed by purpose ID.
func (s *ConsentService) GetStatus(ctx context.Context, userID string) ([]ConsentStatus, error) {
	purposes, err := s.db.ListConsentPurposes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list purposes: %w", err)
	}

	result := make([]ConsentStatus, 0, len(purposes))
	for i := range purposes {
		granted, gErr := s.db.HasActiveConsent(ctx, userID, purposes[i].ID)
		if gErr != nil {
			return nil, fmt.Errorf("check consent for %s: %w", purposes[i].ID, gErr)
		}
		result = append(result, ConsentStatus{
			PurposeID: purposes[i].ID,
			Granted:   granted,
		})
	}
	return result, nil
}

// SetConsent records a user granting or withdrawing consent for a purpose.
// It validates that the purpose exists and rejects withdrawal of required purposes.
func (s *ConsentService) SetConsent(ctx context.Context, record *database.ConsentRecord) error {
	purpose, err := s.db.GetConsentPurpose(ctx, record.PurposeID)
	if err != nil {
		return fmt.Errorf("unknown purpose: %w", err)
	}

	// Required purposes cannot be withdrawn.
	if purpose.Required && !record.Granted {
		return fmt.Errorf("consent for required purpose %q cannot be withdrawn", purpose.Label)
	}

	record.PurposeVersion = purpose.Version
	return s.db.RecordConsent(ctx, record)
}
