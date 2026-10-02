package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/webhook"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// ErrWebhookUnauthorized refuses an inbound delivery: an unknown webhook, a
// webhook addressed under another kind or provider, or a signature that
// does not match. One error for all, so the answer reveals no webhook ID.
var ErrWebhookUnauthorized = errors.New("invalid webhook signature")

// webhookEndpointStore is what WebhookService needs from the store.
type webhookEndpointStore interface {
	database.WebhookStore
	GetProject(ctx context.Context, id string) (*project.Project, error)
}

// maxDeliveryIDLength bounds a stored delivery ID; a longer one (no
// provider sends one) is stored as its SHA-256.
const maxDeliveryIDLength = 128

// WebhookService registers inbound webhooks per project and receives their
// deliveries (KI-85). A webhook's random ID names its tenant and project;
// its own secret, stored encrypted, authenticates the sender. Nothing in a
// delivery - no header, no payload field - picks another tenant or project.
type WebhookService struct {
	store     webhookEndpointStore
	key       []byte // AES-256 key for secrets and API tokens at rest
	vcs       *VCSWebhookService
	pm        *PMWebhookService
	retention time.Duration // how long a delivery ID is remembered
	// decoySecret is checked against the signatures sent to unknown
	// webhooks, so they take about as long as a wrong signature.
	decoySecret string
}

// NewWebhookService creates the webhook service. encryptionKey is the
// AES-256 key the secrets and API tokens are stored with (derived from
// auth.jwt_secret, like VCS account tokens).
func NewWebhookService(store webhookEndpointStore, encryptionKey []byte, vcs *VCSWebhookService, pm *PMWebhookService, deliveryRetention time.Duration) *WebhookService {
	decoy, err := newWebhookSecret()
	if err != nil {
		decoy = "decoy"
	}
	return &WebhookService{store: store, key: encryptionKey, vcs: vcs, pm: pm, retention: deliveryRetention, decoySecret: decoy}
}

// newWebhookSecret returns 32 random bytes in hex.
func newWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// withURL fills the path the provider delivers to.
func withURL(e *webhook.Endpoint) *webhook.Endpoint {
	e.URL = e.Path()
	e.HasAPIToken = len(e.EncryptedAPIToken) > 0
	return e
}

// seal encrypts a secret or token for storage ("" stays nil).
func (s *WebhookService) seal(plain string) ([]byte, error) {
	if plain == "" {
		return nil, nil
	}
	sealed, err := crypto.Encrypt([]byte(plain), s.key)
	if err != nil {
		return nil, fmt.Errorf("encrypt webhook credential: %w", err)
	}
	return sealed, nil
}

// open decrypts a stored secret or token (nil is "").
func (s *WebhookService) open(sealed []byte) (string, error) {
	if len(sealed) == 0 {
		return "", nil
	}
	plain, err := crypto.Decrypt(sealed, s.key)
	if err != nil {
		return "", fmt.Errorf("decrypt webhook credential: %w", err)
	}
	return string(plain), nil
}

// Register creates a webhook for a project of the caller's tenant and
// returns it with its secret - the only time the secret is shown.
func (s *WebhookService) Register(ctx context.Context, projectID string, req *webhook.CreateRequest) (*webhook.Registered, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if req.Provider != "plane" {
		if _, _, ok := project.RepoHostPath(proj.RepoURL); !ok {
			return nil, fmt.Errorf("project %s has no %s repository URL to match events against: %w", projectID, req.Provider, domain.ErrValidation)
		}
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return nil, err
	}
	sealedSecret, err := s.seal(secret)
	if err != nil {
		return nil, err
	}
	sealedToken, err := s.seal(req.APIToken)
	if err != nil {
		return nil, err
	}
	created, err := s.store.CreateWebhookEndpoint(ctx, &webhook.Endpoint{
		ProjectID: projectID, Kind: req.Kind, Provider: req.Provider,
		EncryptedSecret: sealedSecret, EncryptedAPIToken: sealedToken,
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "webhook registered", "webhook_id", created.ID, "project_id", projectID, "kind", req.Kind, "provider", req.Provider)
	return &webhook.Registered{Endpoint: *withURL(created), Secret: secret}, nil
}

// List returns the webhooks of a project of the caller's tenant, without
// secrets.
func (s *WebhookService) List(ctx context.Context, projectID string) ([]webhook.Endpoint, error) {
	if _, err := s.store.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	endpoints, err := s.store.ListWebhookEndpoints(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range endpoints {
		withURL(&endpoints[i])
	}
	if endpoints == nil {
		endpoints = []webhook.Endpoint{}
	}
	return endpoints, nil
}

// RotateSecret gives a webhook of the caller's tenant a new secret; the old
// one stops working at once. It returns the new secret, the only time it is
// shown.
func (s *WebhookService) RotateSecret(ctx context.Context, projectID, id string) (*webhook.Registered, error) {
	secret, err := newWebhookSecret()
	if err != nil {
		return nil, err
	}
	sealed, err := s.seal(secret)
	if err != nil {
		return nil, err
	}
	rotated, err := s.store.RotateWebhookSecret(ctx, projectID, id, sealed)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "webhook secret rotated", "webhook_id", id, "project_id", projectID)
	return &webhook.Registered{Endpoint: *withURL(rotated), Secret: secret}, nil
}

// SetAPIToken replaces the API token of a PM webhook of the caller's tenant
// ("" removes it).
func (s *WebhookService) SetAPIToken(ctx context.Context, projectID, id, token string) error {
	if err := webhook.ValidateAPIToken(token); err != nil {
		return err
	}
	e, err := s.store.GetWebhookEndpoint(ctx, projectID, id)
	if err != nil {
		return err
	}
	if e.Kind != webhook.KindPM {
		return fmt.Errorf("api_token is for PM webhooks only: %w", domain.ErrValidation)
	}
	sealed, err := s.seal(token)
	if err != nil {
		return err
	}
	return s.store.SetWebhookAPIToken(ctx, projectID, id, sealed)
}

// Delete removes a webhook of the caller's tenant; its URL stops working.
func (s *WebhookService) Delete(ctx context.Context, projectID, id string) error {
	return s.store.DeleteWebhookEndpoint(ctx, projectID, id)
}

// Receive handles a delivery to the webhook id, addressed as kind and
// provider. It authenticates the delivery with the webhook's own secret
// (ErrWebhookUnauthorized otherwise), handles it in the webhook's tenant
// for the webhook's project, and handles a delivery ID once. ctx's tenant,
// whatever set it, is replaced by the webhook's.
func (s *WebhookService) Receive(ctx context.Context, kind webhook.Kind, provider, id string, d *webhook.Delivery) (*webhook.InboundResult, error) {
	e, err := s.authenticate(ctx, kind, provider, id, d)
	if err != nil {
		return nil, err
	}
	ctx = tenantctx.WithTenant(ctx, e.TenantID)
	proj, err := s.store.GetProject(ctx, e.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("webhook %s: project %s: %w", e.ID, e.ProjectID, err)
	}

	deliveryID := storedDeliveryID(d.DeliveryID)
	if deliveryID != "" {
		claimed, err := s.store.ClaimWebhookDelivery(ctx, e.ID, deliveryID, s.retention)
		if err != nil {
			return nil, err
		}
		if !claimed {
			slog.InfoContext(ctx, "webhook delivery handled before", "webhook_id", e.ID, "delivery_id", deliveryID)
			return &webhook.InboundResult{Status: webhook.InboundDuplicate, Event: d.Event}, nil
		}
	}

	res, err := s.dispatch(ctx, e, proj, d)
	if errors.Is(err, webhook.ErrRepositoryMismatch) {
		return &webhook.InboundResult{Status: webhook.InboundIgnored, Event: d.Event, Reason: "the event is for another repository than the webhook's project"}, nil
	}
	if err != nil && deliveryID != "" {
		// A failed delivery is forgotten, so the provider's redelivery
		// (same delivery ID) is handled.
		if relErr := s.store.ReleaseWebhookDelivery(ctx, e.ID, deliveryID); relErr != nil {
			slog.ErrorContext(ctx, "webhook delivery claim not released", "webhook_id", e.ID, "delivery_id", deliveryID, "error", relErr)
		}
	}
	return res, err
}

// storedDeliveryID is the form a delivery ID is stored in: as sent when it
// is at most maxDeliveryIDLength printable ASCII characters (the providers
// send UUIDs), otherwise its SHA-256 - still one key per delivery, and
// storable whatever the sender put in the header.
func storedDeliveryID(id string) string {
	printable := len(id) <= maxDeliveryIDLength
	for i := 0; printable && i < len(id); i++ {
		printable = id[i] > ' ' && id[i] <= '~'
	}
	if printable {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// authenticate finds the webhook and checks the delivery's signature with
// its secret. Every refusal is the same ErrWebhookUnauthorized; an unknown
// webhook is checked against a decoy secret, so it takes about as long as a
// wrong signature.
func (s *WebhookService) authenticate(ctx context.Context, kind webhook.Kind, provider, id string, d *webhook.Delivery) (*webhook.Endpoint, error) {
	var e *webhook.Endpoint
	if isWebhookID(id) && webhook.Supports(kind, provider) {
		found, err := s.store.LookupWebhookEndpoint(ctx, id)
		switch {
		case err == nil && found.Kind == kind && found.Provider == provider:
			e = found
		case err != nil && !errors.Is(err, domain.ErrNotFound):
			return nil, err
		}
	}
	if e == nil {
		verifySignature(provider, s.decoySecret, d)
		return nil, ErrWebhookUnauthorized
	}
	secret, err := s.open(e.EncryptedSecret)
	if err != nil {
		slog.ErrorContext(ctx, "webhook secret cannot be decrypted (auth.jwt_secret changed?) - rotate the webhook's secret",
			"webhook_id", e.ID, "project_id", e.ProjectID, "error", err)
		return nil, ErrWebhookUnauthorized
	}
	if !verifySignature(provider, secret, d) {
		slog.WarnContext(ctx, "webhook delivery with a wrong signature refused", "webhook_id", e.ID, "provider", provider)
		return nil, ErrWebhookUnauthorized
	}
	return e, nil
}

// isWebhookID reports whether id is a webhook ID as its URL shows it: a
// UUID in canonical lower-case form.
func isWebhookID(id string) bool {
	return isCanonicalUUID(id) && id == strings.ToLower(id)
}

// verifySignature checks a delivery's signature with secret, in constant
// time: GitHub sends "sha256=" and the HMAC-SHA256 of the body in hex
// (X-Hub-Signature-256), Plane the HMAC in hex (X-Plane-Signature), GitLab
// the secret itself (X-Gitlab-Token).
func verifySignature(provider, secret string, d *webhook.Delivery) bool {
	switch provider {
	case "gitlab":
		got, want := sha256.Sum256([]byte(d.Signature)), sha256.Sum256([]byte(secret))
		return d.Signature != "" && hmac.Equal(got[:], want[:])
	case "github":
		sig, ok := strings.CutPrefix(d.Signature, "sha256=")
		return ok && hmacMatches(sig, secret, d.Body)
	default: // plane
		return hmacMatches(d.Signature, secret, d.Body)
	}
}

// hmacMatches reports whether sigHex is the HMAC-SHA256 of body with secret.
func hmacMatches(sigHex, secret string, body []byte) bool {
	sig, err := hex.DecodeString(sigHex)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return err == nil && len(sig) == sha256.Size && hmac.Equal(sig, mac.Sum(nil))
}

// dispatch hands an authenticated delivery to the VCS or PM handling of its
// event type; other event types are ignored.
func (s *WebhookService) dispatch(ctx context.Context, e *webhook.Endpoint, proj *project.Project, d *webhook.Delivery) (*webhook.InboundResult, error) {
	processed := &webhook.InboundResult{Status: webhook.InboundProcessed, Event: d.Event}
	ignored := &webhook.InboundResult{Status: webhook.InboundIgnored, Event: d.Event, Reason: "event type not handled"}
	if e.Kind == webhook.KindVCS {
		var err error
		switch {
		case e.Provider == "github" && d.Event == "push":
			_, err = s.vcs.HandleGitHubPush(ctx, proj, d.Body)
		case e.Provider == "github" && d.Event == "pull_request":
			_, err = s.vcs.HandleGitHubPullRequest(ctx, proj, d.Body)
		case e.Provider == "gitlab" && d.Event == "Push Hook":
			_, err = s.vcs.HandleGitLabPush(ctx, proj, d.Body)
		default:
			return ignored, nil
		}
		if err != nil {
			return nil, err
		}
		return processed, nil
	}

	switch {
	case e.Provider == "github" && d.Event != "issues", e.Provider == "gitlab" && d.Event != "Issue Hook":
		return ignored, nil
	}
	apiToken, err := s.open(e.EncryptedAPIToken)
	if err != nil {
		return nil, fmt.Errorf("webhook %s: api token: %w", e.ID, err)
	}
	if _, err := s.pm.HandleEvent(ctx, e.Provider, proj, apiToken, d.Body); err != nil {
		return nil, err
	}
	return &webhook.InboundResult{Status: webhook.InboundAccepted, Event: d.Event}, nil
}
