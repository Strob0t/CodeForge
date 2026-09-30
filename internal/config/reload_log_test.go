package config

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigHolderReload_NATSURLChangeLogsNoCredentials: the restart-required
// warning for a changed nats.url must not print the userinfo of either URL.
func TestConfigHolderReload_NATSURLChangeLogsNoCredentials(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	clearSecretEnv(t, "NATS_URL")
	t.Setenv("NATS_URL", "nats://olduser:oldpass-123@nats-a:4222")
	yamlPath := filepath.Join(t.TempDir(), "absent.yaml")
	cfg, err := LoadFrom(yamlPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	holder := NewHolder(cfg, yamlPath)

	t.Setenv("NATS_URL", "nats://newuser:newpass-456@nats-b:4222")
	if err := holder.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	out := buf.String()
	for _, secret := range []string{"oldpass-123", "newpass-456", "olduser", "newuser"} {
		if strings.Contains(out, secret) {
			t.Fatalf("reload log leaks %q: %s", secret, out)
		}
	}
	for _, host := range []string{"nats-a:4222", "nats-b:4222"} {
		if !strings.Contains(out, host) {
			t.Fatalf("reload log should still name the server %q: %s", host, out)
		}
	}
}
