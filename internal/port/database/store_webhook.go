package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/webhook"
)

// WebhookStore keeps the inbound webhooks registered per project and the
// deliveries they handled (KI-85). Everything is scoped to the tenant in ctx
// except LookupWebhookEndpoint, which an inbound delivery needs to find its
// webhook's tenant. Not part of the composite Store (ADR-014): only
// WebhookService uses it.
type WebhookStore interface {
	// CreateWebhookEndpoint stores a webhook for a project of the tenant
	// (domain.ErrNotFound for another tenant's project, domain.ErrConflict
	// when the project has one of that kind and provider already).
	CreateWebhookEndpoint(ctx context.Context, e *webhook.Endpoint) (*webhook.Endpoint, error)
	ListWebhookEndpoints(ctx context.Context, projectID string) ([]webhook.Endpoint, error)
	GetWebhookEndpoint(ctx context.Context, projectID, id string) (*webhook.Endpoint, error)
	// LookupWebhookEndpoint returns a webhook by ID in any tenant, with its
	// tenant.
	LookupWebhookEndpoint(ctx context.Context, id string) (*webhook.Endpoint, error)
	RotateWebhookSecret(ctx context.Context, projectID, id string, encryptedSecret []byte) (*webhook.Endpoint, error)
	// SetWebhookAPIToken replaces a webhook's API token (nil removes it).
	SetWebhookAPIToken(ctx context.Context, projectID, id string, encryptedToken []byte) error
	DeleteWebhookEndpoint(ctx context.Context, projectID, id string) error
	// ClaimWebhookDelivery records that a webhook handles a delivery ID; it
	// reports false when the delivery was claimed before. Claims older than
	// retention are pruned.
	ClaimWebhookDelivery(ctx context.Context, webhookID, deliveryID string, retention time.Duration) (bool, error)
	// ReleaseWebhookDelivery forgets a claim whose handling failed, so the
	// provider's redelivery is handled.
	ReleaseWebhookDelivery(ctx context.Context, webhookID, deliveryID string) error
}
