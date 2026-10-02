package webhook

// PMWebhookEvent represents a webhook event from a PM tool.
type PMWebhookEvent struct {
	Provider   string `json:"provider"`    // "github", "gitlab", "plane"
	Action     string `json:"action"`      // "opened", "closed", "edited", "labeled"
	ItemID     string `json:"item_id"`     // external item identifier
	ProjectRef string `json:"project_ref"` // "owner/repo" or similar
}
