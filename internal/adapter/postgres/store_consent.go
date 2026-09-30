package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/port/database"
)

// HasActiveConsent returns whether the user has active (granted) consent for
// the given purpose. It checks the most recent consent record.
func (s *Store) HasActiveConsent(ctx context.Context, userID, purposeID string) (bool, error) {
	tid := tenantFromCtx(ctx)

	var granted bool
	err := s.pool.QueryRow(ctx,
		`SELECT granted FROM user_consents
		 WHERE tenant_id = $1 AND user_id = $2 AND purpose_id = $3
		 ORDER BY created_at DESC LIMIT 1`,
		tid, userID, purposeID).Scan(&granted)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check consent: %w", err)
	}
	return granted, nil
}

// RecordConsent appends an immutable consent record.
func (s *Store) RecordConsent(ctx context.Context, record *database.ConsentRecord) error {
	tid := tenantFromCtx(ctx)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_consents (tenant_id, user_id, purpose_id, purpose_version, granted, ip_address, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tid, record.UserID, record.PurposeID, record.PurposeVersion, record.Granted,
		nullIfEmpty(record.IPAddress), nullIfEmpty(record.UserAgent))
	if err != nil {
		return fmt.Errorf("record consent: %w", err)
	}
	return nil
}

// AnonymizeConsentsForUser clears the IP address and user agent of the user's
// consent records in the current tenant. Called before the user is deleted
// (GDPR Art. 17); the foreign key then sets user_id to NULL (migration 093), so
// the records remain as anonymized proof of consent (Art. 7(1)).
func (s *Store) AnonymizeConsentsForUser(ctx context.Context, userID string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE user_consents SET ip_address = NULL, user_agent = NULL
		 WHERE user_id = $1 AND tenant_id = $2`,
		userID, tenantFromCtx(ctx))
	if err != nil {
		return 0, fmt.Errorf("anonymize consents for user: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AnonymizeExpiredConsentIPAddresses clears the IP address and user agent of
// up to batchSize consent records created before the cutoff and returns how
// many it changed; the records (user, purpose, decision) are kept as proof of
// consent.
//
// INTENTIONALLY CROSS-TENANT: part of the instance-wide retention job (see
// store_retention.go): one policy for all tenants, and the only predicate is
// the record's age against the cutoff the caller computed from it.
func (s *Store) AnonymizeExpiredConsentIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE user_consents SET ip_address = NULL, user_agent = NULL WHERE id IN (
		   SELECT id FROM user_consents
		   WHERE (ip_address IS NOT NULL OR user_agent IS NOT NULL) AND created_at < $1 LIMIT $2
		 )`,
		before, batchSize)
	if err != nil {
		return 0, fmt.Errorf("anonymize expired consent ip addresses: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListUserConsents returns all consent records for a user in the current tenant.
func (s *Store) ListUserConsents(ctx context.Context, userID string) ([]database.ConsentRecord, error) {
	tid := tenantFromCtx(ctx)

	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, user_id, purpose_id, purpose_version, granted,
		        COALESCE(host(ip_address), ''), COALESCE(user_agent, ''), created_at
		 FROM user_consents WHERE tenant_id = $1 AND user_id = $2
		 ORDER BY created_at DESC LIMIT 200`,
		tid, userID)
	if err != nil {
		return nil, fmt.Errorf("list user consents: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (database.ConsentRecord, error) {
		var c database.ConsentRecord
		err := r.Scan(&c.ID, &c.TenantID, &c.UserID, &c.PurposeID, &c.PurposeVersion,
			&c.Granted, &c.IPAddress, &c.UserAgent, &c.CreatedAt)
		return c, err
	})
}

// ListConsentPurposes returns all consent purposes for the current tenant.
func (s *Store) ListConsentPurposes(ctx context.Context) ([]database.ConsentPurpose, error) {
	tid := tenantFromCtx(ctx)

	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, label, description, legal_basis, required, version, created_at, updated_at
		 FROM consent_purposes WHERE tenant_id = $1 ORDER BY created_at`,
		tid)
	if err != nil {
		return nil, fmt.Errorf("list consent purposes: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (database.ConsentPurpose, error) {
		var p database.ConsentPurpose
		err := r.Scan(&p.ID, &p.TenantID, &p.Label, &p.Description, &p.LegalBasis,
			&p.Required, &p.Version, &p.CreatedAt, &p.UpdatedAt)
		return p, err
	})
}

// GetConsentPurpose returns a specific consent purpose by ID.
func (s *Store) GetConsentPurpose(ctx context.Context, purposeID string) (*database.ConsentPurpose, error) {
	tid := tenantFromCtx(ctx)

	var p database.ConsentPurpose
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, label, description, legal_basis, required, version, created_at, updated_at
		 FROM consent_purposes WHERE tenant_id = $1 AND id = $2`,
		tid, purposeID).Scan(&p.ID, &p.TenantID, &p.Label, &p.Description, &p.LegalBasis,
		&p.Required, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("consent purpose not found: %s", purposeID)
	}
	if err != nil {
		return nil, fmt.Errorf("get consent purpose: %w", err)
	}
	return &p, nil
}
