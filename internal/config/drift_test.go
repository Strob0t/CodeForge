package config

import (
	"strings"
	"testing"
)

// KI-51: configuration drift between the code, the example file and the docs.

func TestNotificationSMTPPortDefault(t *testing.T) {
	cfg := Defaults()
	if cfg.Notification.SMTPPort != 587 {
		t.Fatalf("smtp_port default = %d, want 587 (submission port, as documented)", cfg.Notification.SMTPPort)
	}

	t.Setenv("CODEFORGE_SMTP_PORT", "2525")
	loadEnv(&cfg)
	if cfg.Notification.SMTPPort != 2525 {
		t.Fatalf("CODEFORGE_SMTP_PORT = %d, want 2525", cfg.Notification.SMTPPort)
	}
}

func TestValidate_SMTPPort(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		port    int
		wantErr bool
	}{
		{"default port", "smtp.example.com", 587, false},
		{"lowest port", "smtp.example.com", 1, false},
		{"highest port", "smtp.example.com", 65535, false},
		{"zero port makes every send fail", "smtp.example.com", 0, true},
		{"negative port", "smtp.example.com", -1, true},
		{"port above range", "smtp.example.com", 65536, true},
		{"port ignored without smtp_host", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.JWTSecret = strongTestSecret
			cfg.Notification.SMTPHost = tt.host
			cfg.Notification.SMTPPort = tt.port
			err := validate(&cfg)
			if tt.wantErr != (err != nil) {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "notification.smtp_port") {
				t.Fatalf("error %q should name notification.smtp_port", err)
			}
		})
	}
}

// TestExampleConfigLoads keeps codeforge.example.yaml loadable and valid:
// copying it to codeforge.yaml must give a working configuration.
func TestExampleConfigLoads(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	for _, key := range []string{"DATABASE_URL", "NATS_URL", "LITELLM_MASTER_KEY", "CODEFORGE_AUTH_JWT_SECRET", "CODEFORGE_AUTH_BCRYPT_COST", "CODEFORGE_ROUTING_ENABLED"} {
		clearSecretEnv(t, key)
	}
	cfg, err := LoadFrom("../../codeforge.example.yaml")
	if err != nil {
		t.Fatalf("codeforge.example.yaml: %v", err)
	}
	if cfg.Auth.BcryptCost < 12 {
		t.Fatalf("example bcrypt_cost = %d, below the enforced minimum 12", cfg.Auth.BcryptCost)
	}
	if !cfg.Routing.Enabled {
		t.Fatal("routing is enabled by default and the example does not disable it")
	}
}
