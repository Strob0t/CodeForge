package config

import "testing"

// KI-62: runtime.stall_max_retries bounds the new runs a plan step gets
// after stalled runs (default 2); 0 disables re-planning, a negative value is
// rejected (it used to mean the default silently).
func TestStallMaxRetries(t *testing.T) {
	cfg := Defaults()
	if cfg.Runtime.StallMaxRetries != 2 {
		t.Fatalf("stall_max_retries default = %d, want 2", cfg.Runtime.StallMaxRetries)
	}

	t.Setenv("CODEFORGE_STALL_MAX_RETRIES", "0")
	loadEnv(&cfg)
	if cfg.Runtime.StallMaxRetries != 0 {
		t.Fatalf("CODEFORGE_STALL_MAX_RETRIES = %d, want 0", cfg.Runtime.StallMaxRetries)
	}
	if err := validateStallMaxRetries(cfg.Runtime.StallMaxRetries); err != nil {
		t.Fatalf("stall_max_retries 0 (no re-planning): %v", err)
	}
	if err := validateStallMaxRetries(-1); err == nil {
		t.Fatal("negative stall_max_retries accepted")
	}
}
