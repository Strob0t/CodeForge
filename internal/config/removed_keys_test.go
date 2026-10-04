package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadFrom_RemovedKeysStillLoad: keys the configuration no longer has are
// ignored, so configuration files written for an older version still load.
// runtime.stale_work_threshold was removed with KI-65 (lost work is found by
// runtime.heartbeat_timeout).
func TestLoadFrom_RemovedKeysStillLoad(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(yamlPath, []byte(`
runtime:
  stale_work_threshold: 30m
  heartbeat_timeout: 90s
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "development")

	cfg, err := LoadFrom(yamlPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Runtime.HeartbeatTimeout.Seconds() != 90 {
		t.Fatalf("heartbeat_timeout = %s, want 90s", cfg.Runtime.HeartbeatTimeout)
	}
}
