package config

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// KI-57: approval emails go to notification.approval_recipients and link to
// the web UI at notification.web_ui_url.
func TestValidate_EmailApprovalSettings(t *testing.T) {
	tests := []struct {
		name       string
		recipients []string
		webUI      string
		wantErr    string
	}{
		{name: "unset"},
		{name: "valid", recipients: []string{"ops@example.com", "lead@example.com"}, webUI: "https://cf.example.com"},
		{name: "display name (SMTP needs the bare address)", recipients: []string{"Lead <lead@example.com>"}, wantErr: "notification.approval_recipients"},
		{name: "web ui with a path", webUI: "https://example.com/codeforge/"},
		{name: "loopback http", webUI: "http://localhost:3000"},
		{name: "not an address", recipients: []string{"ops"}, wantErr: "notification.approval_recipients"},
		{name: "two addresses in one entry", recipients: []string{"a@example.com, b@example.com"}, wantErr: "notification.approval_recipients"},
		{name: "relative web ui", webUI: "/ui", wantErr: "notification.web_ui_url"},
		{name: "web ui with a query", webUI: "https://cf.example.com/?x=1", wantErr: "notification.web_ui_url"},
		{name: "web ui with user info", webUI: "https://u:p@cf.example.com", wantErr: "notification.web_ui_url"},
		{name: "web ui scheme", webUI: "javascript:alert(1)", wantErr: "notification.web_ui_url"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.AppEnv = "development"
			if err := ensureSecrets(&cfg); err != nil {
				t.Fatal(err)
			}
			cfg.Notification.ApprovalRecipients, cfg.Notification.WebUIURL = tc.recipients, tc.webUI
			err := validate(&cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validate = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestNotification_EmailApprovalsMissing(t *testing.T) {
	n := Notification{}
	if got := n.EmailApprovalsMissing(); !slices.Equal(got, []string{"notification.smtp_host", "notification.smtp_from", "notification.approval_recipients", "notification.web_ui_url"}) {
		t.Fatalf("missing = %v", got)
	}
	n = Notification{SMTPHost: "smtp.example.com", SMTPFrom: "cf@example.com", ApprovalRecipients: []string{"ops@example.com"}}
	if got := n.EmailApprovalsMissing(); !slices.Equal(got, []string{"notification.web_ui_url"}) {
		t.Fatalf("missing = %v", got)
	}
	n.WebUIURL = "https://cf.example.com"
	if got := n.EmailApprovalsMissing(); len(got) != 0 {
		t.Fatalf("missing = %v, want none", got)
	}
}

func TestLoad_EmailApprovalSettingsFromEnv(t *testing.T) {
	t.Setenv("CODEFORGE_NOTIFICATION_APPROVAL_RECIPIENTS", "ops@example.com,lead@example.com")
	t.Setenv("CODEFORGE_NOTIFICATION_WEB_UI_URL", "https://cf.example.com")
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if !slices.Equal(cfg.Notification.ApprovalRecipients, []string{"ops@example.com", "lead@example.com"}) || cfg.Notification.WebUIURL != "https://cf.example.com" {
		t.Fatalf("notification = %+v", cfg.Notification)
	}
}
