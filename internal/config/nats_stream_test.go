package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tenGiB = int64(10) << 30

func TestDefaults_NATSStreamMaxBytes(t *testing.T) {
	if got := Defaults().NATS.StreamMaxBytes; got != tenGiB {
		t.Fatalf("default nats.stream_max_bytes = %d, want %d (10 GiB)", got, tenGiB)
	}
}

func TestLoadEnv_NATSStreamMaxBytes(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want int64
	}{
		{"unset keeps default", "", tenGiB},
		{"2 GiB", "2147483648", 2 << 30},
		{"above int32", "21474836480", 20 << 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CODEFORGE_NATS_STREAM_MAX_BYTES", tt.env)
			cfg := Defaults()
			mustLoadEnv(t, &cfg)
			if cfg.NATS.StreamMaxBytes != tt.want {
				t.Fatalf("got %d, want %d", cfg.NATS.StreamMaxBytes, tt.want)
			}
		})
	}
	t.Run("invalid is an error", func(t *testing.T) {
		t.Setenv("CODEFORGE_NATS_STREAM_MAX_BYTES", "10GB")
		cfg := Defaults()
		if err := loadEnv(&cfg); err == nil || !strings.Contains(err.Error(), "CODEFORGE_NATS_STREAM_MAX_BYTES") {
			t.Fatalf("loadEnv = %v, want an error naming CODEFORGE_NATS_STREAM_MAX_BYTES", err)
		}
	})
}

func TestLoadYAML_NATSStreamMaxBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codeforge.yaml")
	if err := os.WriteFile(path, []byte("nats:\n  stream_max_bytes: 1073741824\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, path); err != nil {
		t.Fatal(err)
	}
	if cfg.NATS.StreamMaxBytes != 1<<30 {
		t.Fatalf("got %d, want %d", cfg.NATS.StreamMaxBytes, int64(1<<30))
	}
	if cfg.NATS.URL != Defaults().NATS.URL {
		t.Fatalf("nats.url must keep its default, got %q", cfg.NATS.URL)
	}
}

func TestValidate_NATSStreamMaxBytes(t *testing.T) {
	tests := []struct {
		value   int64
		wantErr bool
	}{
		{-1, true},
		{0, true},
		{1, false},
		{tenGiB, false},
	}
	for _, tt := range tests {
		cfg := Defaults()
		cfg.Auth.JWTSecret = strongTestSecret
		cfg.NATS.StreamMaxBytes = tt.value
		err := validate(&cfg)
		if tt.wantErr && (err == nil || !strings.Contains(err.Error(), "nats.stream_max_bytes")) {
			t.Errorf("value %d: expected nats.stream_max_bytes error, got %v", tt.value, err)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("value %d: unexpected error %v", tt.value, err)
		}
	}
}
