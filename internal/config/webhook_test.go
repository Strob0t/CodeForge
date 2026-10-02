package config

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// KI-85: a delivery ID is remembered for webhook.delivery_retention, so a
// redelivered or replayed event is handled once.
func TestWebhook_DeliveryRetention(t *testing.T) {
	if got := Defaults().Webhook.DeliveryRetention; got != 7*24*time.Hour {
		t.Fatalf("default delivery_retention = %s, want 168h", got)
	}
	t.Setenv("CODEFORGE_WEBHOOK_DELIVERY_RETENTION", "24h")
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Webhook.DeliveryRetention != 24*time.Hour {
		t.Fatalf("delivery_retention = %s, want 24h", cfg.Webhook.DeliveryRetention)
	}

	for _, bad := range []time.Duration{0, -time.Hour} {
		cfg := Defaults()
		cfg.Auth.JWTSecret = strongTestSecret
		cfg.Webhook.DeliveryRetention = bad
		if err := validate(&cfg); err == nil || !strings.Contains(err.Error(), "webhook.delivery_retention") {
			t.Fatalf("delivery_retention %s: validate = %v, want an error naming it", bad, err)
		}
	}
}

// KI-85: the global webhook secrets verified the removed global webhook
// routes. They still load (old files and environments keep working) and are
// reported so the operator registers per-project webhooks.
func TestWebhook_RemovedGlobalSecrets(t *testing.T) {
	if got := (&Webhook{}).RemovedGlobalSecrets(); len(got) != 0 {
		t.Fatalf("unset: %v", got)
	}
	w := Webhook{GitHubSecret: "a", PlaneSecret: "c"}
	if got := w.RemovedGlobalSecrets(); !slices.Equal(got, []string{"webhook.github_secret", "webhook.plane_secret"}) {
		t.Fatalf("RemovedGlobalSecrets = %v", got)
	}
	t.Setenv("CODEFORGE_WEBHOOK_GITLAB_TOKEN", "b")
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("LoadFrom with a removed key: %v", err)
	}
	if got := cfg.Webhook.RemovedGlobalSecrets(); !slices.Equal(got, []string{"webhook.gitlab_token"}) {
		t.Fatalf("RemovedGlobalSecrets = %v", got)
	}
}
