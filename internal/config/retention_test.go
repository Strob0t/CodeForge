package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// KI-52: the retention job runs by default, with the periods of
// docs/data-retention.md, and refuses periods that would purge far more than
// a policy measured in days.

const day = 24 * time.Hour

func TestRetentionDefaultsMatchPolicy(t *testing.T) {
	r := Defaults().Retention
	tests := []struct {
		name      string
		got, want time.Duration
	}{
		{"interval (daily job)", r.Interval, day},
		{"sessions (30 days)", r.Sessions, 30 * day},
		{"conversations (1 year)", r.Conversations, 365 * day},
		{"cost_records (1 year)", r.CostRecords, 365 * day},
		{"audit_entries (7 years)", r.AuditEntries, 7 * 365 * day},
		{"audit_ip_addresses (180 days)", r.AuditIPAddresses, 180 * day},
		{"consent_ip_addresses (180 days)", r.ConsentIPAddresses, 180 * day},
		// KI-90: past the NATS stream's 30-day max age, so a done claim
		// never meets a redelivery of its message.
		{"handoff_claims (30 days)", r.HandoffClaims, 30 * day},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("retention %s default = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestRetentionEnv(t *testing.T) {
	cfg := Defaults()
	t.Setenv("CODEFORGE_RETENTION_INTERVAL", "12h")
	t.Setenv("CODEFORGE_RETENTION_AUDIT_IP_ADDRESSES", "2160h")
	t.Setenv("CODEFORGE_RETENTION_CONSENT_IP_ADDRESSES", "720h")
	t.Setenv("CODEFORGE_RETENTION_SESSIONS", "0s")
	t.Setenv("CODEFORGE_RETENTION_HANDOFF_CLAIMS", "2160h")
	mustLoadEnv(t, &cfg)
	if cfg.Retention.Interval != 12*time.Hour {
		t.Errorf("interval = %v, want 12h", cfg.Retention.Interval)
	}
	if cfg.Retention.AuditIPAddresses != 90*day {
		t.Errorf("audit_ip_addresses = %v, want 2160h", cfg.Retention.AuditIPAddresses)
	}
	if cfg.Retention.ConsentIPAddresses != 30*day {
		t.Errorf("consent_ip_addresses = %v, want 720h", cfg.Retention.ConsentIPAddresses)
	}
	if cfg.Retention.Sessions != 0 {
		t.Errorf("sessions = %v, want 0 (category disabled)", cfg.Retention.Sessions)
	}
	if cfg.Retention.HandoffClaims != 90*day {
		t.Errorf("handoff_claims = %v, want 2160h", cfg.Retention.HandoffClaims)
	}
}

func TestRetentionYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codeforge.yaml")
	yaml := "retention:\n  interval: 6h\n  conversations: 4380h\n  audit_ip_addresses: 0s\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, path); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}
	if cfg.Retention.Interval != 6*time.Hour || cfg.Retention.Conversations != 4380*time.Hour || cfg.Retention.AuditIPAddresses != 0 {
		t.Fatalf("retention = %+v", cfg.Retention)
	}
	if cfg.Retention.Sessions != 30*day {
		t.Fatalf("sessions = %v, want the default kept", cfg.Retention.Sessions)
	}
}

func TestValidate_Retention(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(r *Retention)
		wantErr string // empty: valid
	}{
		{"defaults", func(*Retention) {}, ""},
		{"job disabled", func(r *Retention) { r.Interval = 0 }, ""},
		{"interval of one minute", func(r *Retention) { r.Interval = time.Minute }, ""},
		{"interval below one minute", func(r *Retention) { r.Interval = 59 * time.Second }, "retention.interval"},
		{"negative interval", func(r *Retention) { r.Interval = -time.Hour }, "retention.interval"},
		{"category disabled", func(r *Retention) { r.Sessions = 0 }, ""},
		{"period of one day", func(r *Retention) { r.Sessions = day }, ""},
		{"period below one day", func(r *Retention) { r.Sessions = day - time.Second }, "retention.sessions"},
		{"30m meant as 30 months", func(r *Retention) { r.Conversations = 30 * time.Minute }, "retention.conversations"},
		{"negative period", func(r *Retention) { r.CostRecords = -day }, "retention.cost_records"},
		{"audit entries below one day", func(r *Retention) { r.AuditEntries = time.Hour }, "retention.audit_entries"},
		{"audit IPs below one day", func(r *Retention) { r.AuditIPAddresses = time.Nanosecond }, "retention.audit_ip_addresses"},
		{"consent IPs below one day", func(r *Retention) { r.ConsentIPAddresses = 12 * time.Hour }, "retention.consent_ip_addresses"},
		{"consent IPs kept", func(r *Retention) { r.ConsentIPAddresses = 0 }, ""},
		{"handoff claims below one day", func(r *Retention) { r.HandoffClaims = time.Hour }, "retention.handoff_claims"},
		{"handoff claims kept", func(r *Retention) { r.HandoffClaims = 0 }, ""},
		// KI-90 review: a done claim may go only once no message of its
		// stage can come back, i.e. after the NATS stream's max age.
		{"handoff claims of one day", func(r *Retention) { r.HandoffClaims = day }, "retention.handoff_claims"},
		{"handoff claims just below the stream max age", func(r *Retention) { r.HandoffClaims = messagequeue.StreamMaxAge - time.Second }, "retention.handoff_claims"},
		{"handoff claims at the stream max age", func(r *Retention) { r.HandoffClaims = messagequeue.StreamMaxAge }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.JWTSecret = strongTestSecret
			tt.modify(&cfg.Retention)
			err := validate(&cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validate() = %v, want an error naming %s", err, tt.wantErr)
			}
		})
	}
}
