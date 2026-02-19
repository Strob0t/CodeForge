package webhook

// PMWebhookEvent represents a webhook event from a PM tool.
type PMWebhookEvent struct {
	Provider   string `json:"provider"`    // "github", "gitlab", "plane"
	Action     string `json:"action"`      // "opened", "closed", "edited", "labeled"
	ItemID     string `json:"item_id"`     // external item identifier
	ProjectRef string `json:"project_ref"` // "owner/repo" or similar
}

// PMWebhookConfig holds webhook configuration for a PM provider.
type PMWebhookConfig struct {
	Provider string `json:"provider"`
	Secret   string `json:"secret"` //nolint:gosec // G117: this is a config field name, not a hardcoded secret
	Enabled  bool   `json:"enabled"`
}
