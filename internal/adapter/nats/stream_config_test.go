package nats

import "testing"

func TestStreamConfig_UsesConfiguredMaxBytes(t *testing.T) {
	for _, maxBytes := range []int64{1 << 20, 3 << 30, 10 << 30} {
		cfg := streamConfig(maxBytes)
		if cfg.MaxBytes != maxBytes {
			t.Errorf("MaxBytes = %d, want %d", cfg.MaxBytes, maxBytes)
		}
		if cfg.Name != streamName {
			t.Errorf("Name = %q, want %q", cfg.Name, streamName)
		}
		if len(cfg.Subjects) == 0 {
			t.Error("stream has no subjects")
		}
	}
}

// KI-71 review: the worker reads notification subjects back with batched
// direct gets (workers/codeforge/notifications.py), which need direct access.
func TestStreamConfig_AllowsDirectGet(t *testing.T) {
	if !streamConfig(1 << 20).AllowDirect {
		t.Fatal("AllowDirect = false, want true: the worker reads notifications back with direct gets")
	}
}
