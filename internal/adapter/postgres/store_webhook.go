package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
)

// --- Inbound webhooks (KI-85) ---

const webhookColumns = `id, tenant_id, project_id, kind, provider, encrypted_secret, encrypted_api_token, created_at, secret_rotated_at`

func scanWebhookEndpoint(row pgx.Row) (webhook.Endpoint, error) {
	var e webhook.Endpoint
	err := row.Scan(&e.ID, &e.TenantID, &e.ProjectID, &e.Kind, &e.Provider, &e.EncryptedSecret, &e.EncryptedAPIToken,
		&e.CreatedAt, &e.SecretRotatedAt)
	return e, err
}

// CreateWebhookEndpoint stores a webhook for a project of the caller's
// tenant: domain.ErrNotFound for a project of another tenant,
// domain.ErrConflict when the project has a webhook of that kind and
// provider already.
func (s *Store) CreateWebhookEndpoint(ctx context.Context, e *webhook.Endpoint) (*webhook.Endpoint, error) {
	tid := tenantFromCtx(ctx)
	row := s.pool.QueryRow(ctx,
		`INSERT INTO webhook_endpoints (tenant_id, project_id, kind, provider, encrypted_secret, encrypted_api_token)
		 SELECT $1, p.id, $3, $4, $5, $6 FROM projects p WHERE p.id = $2 AND p.tenant_id = $1
		 ON CONFLICT (project_id, kind, provider) DO NOTHING
		 RETURNING `+webhookColumns,
		tid, e.ProjectID, e.Kind, e.Provider, e.EncryptedSecret, e.EncryptedAPIToken)
	created, err := scanWebhookEndpoint(row)
	if err == nil {
		return &created, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("create webhook for project %s: %w", e.ProjectID, err)
	}
	var projectExists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1 AND tenant_id = $2)`, e.ProjectID, tid,
	).Scan(&projectExists); err != nil {
		return nil, fmt.Errorf("create webhook for project %s: %w", e.ProjectID, err)
	}
	if projectExists {
		return nil, fmt.Errorf("project %s has a %s webhook for %s already: %w", e.ProjectID, e.Kind, e.Provider, domain.ErrConflict)
	}
	return nil, fmt.Errorf("create webhook for project %s: %w", e.ProjectID, domain.ErrNotFound)
}

// ListWebhookEndpoints returns the webhooks of a project of the caller's
// tenant.
func (s *Store) ListWebhookEndpoints(ctx context.Context, projectID string) ([]webhook.Endpoint, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+webhookColumns+` FROM webhook_endpoints
		 WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at, id LIMIT $3`,
		projectID, tenantFromCtx(ctx), DefaultListLimit)
	if err != nil {
		return nil, fmt.Errorf("list webhooks of project %s: %w", projectID, err)
	}
	return scanRows(rows, func(r pgx.Rows) (webhook.Endpoint, error) { return scanWebhookEndpoint(r) })
}

// GetWebhookEndpoint returns a webhook of a project of the caller's tenant.
func (s *Store) GetWebhookEndpoint(ctx context.Context, projectID, id string) (*webhook.Endpoint, error) {
	e, err := scanWebhookEndpoint(s.pool.QueryRow(ctx,
		`SELECT `+webhookColumns+` FROM webhook_endpoints WHERE id = $1 AND project_id = $2 AND tenant_id = $3`,
		id, projectID, tenantFromCtx(ctx)))
	if err != nil {
		return nil, notFoundWrap(err, "get webhook %s", id)
	}
	return &e, nil
}

// LookupWebhookEndpoint returns a webhook by its ID, with its tenant.
//
// INTENTIONALLY CROSS-TENANT: an inbound delivery carries no user and no
// tenant; its signature is checked with this webhook's secret and the
// delivery is then handled in the returned tenant.
func (s *Store) LookupWebhookEndpoint(ctx context.Context, id string) (*webhook.Endpoint, error) {
	e, err := scanWebhookEndpoint(s.pool.QueryRow(ctx,
		`SELECT `+webhookColumns+` FROM webhook_endpoints WHERE id = $1`, id))
	if err != nil {
		return nil, notFoundWrap(err, "look up webhook %s", id)
	}
	return &e, nil
}

// RotateWebhookSecret replaces the secret of a webhook of the caller's
// tenant.
func (s *Store) RotateWebhookSecret(ctx context.Context, projectID, id string, encryptedSecret []byte) (*webhook.Endpoint, error) {
	e, err := scanWebhookEndpoint(s.pool.QueryRow(ctx,
		`UPDATE webhook_endpoints SET encrypted_secret = $4, secret_rotated_at = now()
		 WHERE id = $1 AND project_id = $2 AND tenant_id = $3
		 RETURNING `+webhookColumns,
		id, projectID, tenantFromCtx(ctx), encryptedSecret))
	if err != nil {
		return nil, notFoundWrap(err, "rotate secret of webhook %s", id)
	}
	return &e, nil
}

// SetWebhookAPIToken replaces the API token of a webhook of the caller's
// tenant (nil removes it).
func (s *Store) SetWebhookAPIToken(ctx context.Context, projectID, id string, encryptedToken []byte) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE webhook_endpoints SET encrypted_api_token = $4 WHERE id = $1 AND project_id = $2 AND tenant_id = $3`,
		id, projectID, tenantFromCtx(ctx), encryptedToken)
	return execExpectOne(tag, err, "set api token of webhook %s", id)
}

// DeleteWebhookEndpoint removes a webhook of the caller's tenant with its
// deliveries.
func (s *Store) DeleteWebhookEndpoint(ctx context.Context, projectID, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM webhook_endpoints WHERE id = $1 AND project_id = $2 AND tenant_id = $3`,
		id, projectID, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete webhook %s", id)
}

// ClaimWebhookDelivery records a delivery ID for a webhook of the caller's
// tenant and reports whether it is new. The webhook's claims older than
// retention are pruned first, which bounds the table by what the webhook
// receives within the retention.
func (s *Store) ClaimWebhookDelivery(ctx context.Context, webhookID, deliveryID string, retention time.Duration) (bool, error) {
	tid := tenantFromCtx(ctx)
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM webhook_deliveries WHERE webhook_id = $1 AND tenant_id = $2 AND received_at < now() - $3::interval`,
		webhookID, tid, retention); err != nil {
		return false, fmt.Errorf("prune deliveries of webhook %s: %w", webhookID, err)
	}
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (webhook_id, delivery_id, tenant_id) VALUES ($1, $2, $3)
		 ON CONFLICT (webhook_id, delivery_id) DO NOTHING`,
		webhookID, deliveryID, tid)
	if err != nil {
		return false, fmt.Errorf("claim delivery %s of webhook %s: %w", deliveryID, webhookID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// ReleaseWebhookDelivery forgets a delivery claim of the caller's tenant.
func (s *Store) ReleaseWebhookDelivery(ctx context.Context, webhookID, deliveryID string) error {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM webhook_deliveries WHERE webhook_id = $1 AND delivery_id = $2 AND tenant_id = $3`,
		webhookID, deliveryID, tenantFromCtx(ctx)); err != nil {
		return fmt.Errorf("release delivery %s of webhook %s: %w", deliveryID, webhookID, err)
	}
	return nil
}
