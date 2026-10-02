package webhook

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// Kind is what an inbound webhook feeds: VCS events (push, pull request) or
// a roadmap sync from a PM tool (issues).
type Kind string

const (
	KindVCS Kind = "vcs"
	KindPM  Kind = "pm"
)

// providers lists the providers each kind of webhook accepts.
var providers = map[Kind][]string{
	KindVCS: {"github", "gitlab"},
	KindPM:  {"github", "gitlab", "plane"},
}

// Supports reports whether provider sends webhooks of kind.
func Supports(kind Kind, provider string) bool {
	return slices.Contains(providers[kind], provider)
}

// MaxAPITokenLength bounds a PM integration's API token.
const MaxAPITokenLength = 1024

// ErrRepositoryMismatch is returned for an event about another repository
// (or Plane project) than the webhook's project: the event is ignored.
var ErrRepositoryMismatch = errors.New("the event is for another repository than the webhook's project")

// Endpoint is an inbound webhook registered for a project (KI-85). Its random
// ID, part of its URL, names its tenant and project; its own secret (an HMAC
// key, GitLab's token) authenticates the sender. The secret and a PM
// integration's API token are stored encrypted and never shown after
// registration or rotation.
type Endpoint struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Kind      Kind   `json:"kind"`
	Provider  string `json:"provider"`
	// URL is the path the provider delivers to, on the API's origin.
	URL string `json:"url"`
	// HasAPIToken tells whether a PM webhook's sync has its own token.
	HasAPIToken     bool      `json:"has_api_token"`
	CreatedAt       time.Time `json:"created_at"`
	SecretRotatedAt time.Time `json:"secret_rotated_at"`

	TenantID          string `json:"-"`
	EncryptedSecret   []byte `json:"-"`
	EncryptedAPIToken []byte `json:"-"`
}

// Path is where the provider delivers the webhook's events.
func (e *Endpoint) Path() string {
	return "/api/v1/webhooks/" + string(e.Kind) + "/" + e.Provider + "/" + e.ID
}

// Registered is a webhook with its secret: the answer to a registration or
// a rotation, the only time the secret is shown.
type Registered struct {
	Endpoint
	Secret string `json:"secret"` //nolint:gosec // G117: shown once to the admin who registers or rotates the webhook
}

// CreateRequest registers a webhook for a project.
type CreateRequest struct {
	Kind     Kind   `json:"kind"`
	Provider string `json:"provider"`
	// APIToken is the PM integration's own token for the provider's API
	// (GitLab, GitHub, Plane); PM webhooks only.
	APIToken string `json:"api_token,omitempty"` //nolint:gosec // G117: request field, stored encrypted
}

// Validate checks kind, provider and API token.
func (r *CreateRequest) Validate() error {
	if _, ok := providers[r.Kind]; !ok {
		return fmt.Errorf("kind must be %q or %q: %w", KindVCS, KindPM, domain.ErrValidation)
	}
	if !Supports(r.Kind, r.Provider) {
		return fmt.Errorf("provider %q has no %s webhooks (supported: %v): %w", r.Provider, r.Kind, providers[r.Kind], domain.ErrValidation)
	}
	if r.APIToken != "" && r.Kind != KindPM {
		return fmt.Errorf("api_token is for PM webhooks only: %w", domain.ErrValidation)
	}
	return ValidateAPIToken(r.APIToken)
}

// ValidateAPIToken checks a PM integration's API token: at most
// MaxAPITokenLength printable ASCII characters without spaces (it goes into
// an HTTP header or an environment variable). Empty means no token.
func ValidateAPIToken(token string) error {
	if len(token) > MaxAPITokenLength {
		return fmt.Errorf("api_token is longer than %d characters: %w", MaxAPITokenLength, domain.ErrValidation)
	}
	for i := range len(token) {
		if c := token[i]; c <= ' ' || c > '~' {
			return fmt.Errorf("api_token may contain only printable ASCII characters without spaces: %w", domain.ErrValidation)
		}
	}
	return nil
}

// Delivery is one inbound webhook request: the provider's event type,
// delivery ID and signature (from its headers) and the raw body the
// signature covers.
type Delivery struct {
	Event      string
	DeliveryID string
	Signature  string
	Body       []byte
}

// InboundStatus is what an inbound webhook delivery did.
type InboundStatus string

const (
	// InboundProcessed: a VCS event was handled.
	InboundProcessed InboundStatus = "processed"
	// InboundAccepted: a PM sync was started; its outcome is announced as a
	// pm.sync event.
	InboundAccepted InboundStatus = "accepted"
	// InboundIgnored: an event type the webhook does not handle, or an event
	// for another repository.
	InboundIgnored InboundStatus = "ignored"
	// InboundDuplicate: the delivery ID was handled before.
	InboundDuplicate InboundStatus = "duplicate"
)

// InboundResult is the answer to a webhook delivery.
type InboundResult struct {
	Status InboundStatus `json:"status"`
	Event  string        `json:"event,omitempty"`
	Reason string        `json:"reason,omitempty"`
}
