package config

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-84: the operator's approval channels (the Slack channel, the approval
// email recipients) receive the approval requests of the tenants in
// notification.approval_tenants only; by default the default tenant's.
func TestApprovalTenants_DefaultIsTheDefaultTenant(t *testing.T) {
	cfg := Defaults()
	got, err := cfg.Notification.ApprovalTenantIDs()
	if err != nil || !slices.Equal(got, []string{tenantctx.DefaultTenantID}) {
		t.Fatalf("ApprovalTenantIDs = %v, %v; want only the default tenant", got, err)
	}
}

func TestNotification_ApprovalTenantIDs(t *testing.T) {
	const a, b = "aaaaaaaa-0000-4000-8000-000000000001", "bbbbbbbb-0000-4000-8000-000000000002"
	tests := []struct {
		name    string
		tenants []string
		want    []string
		wantErr string
	}{
		{name: "none: no tenant's requests are sent", tenants: nil, want: []string{}},
		{name: "two tenants", tenants: []string{a, b}, want: []string{a, b}},
		{name: "upper case is stored lower case", tenants: []string{strings.ToUpper(a)}, want: []string{a}},
		{name: "spaces around an entry", tenants: []string{" " + a + " "}, want: []string{a}},
		{name: "a tenant listed twice", tenants: []string{a, strings.ToUpper(a)}, want: []string{a}},
		{name: "not a UUID", tenants: []string{"acme"}, wantErr: "notification.approval_tenants"},
		{name: "empty entry", tenants: []string{a, ""}, wantErr: "notification.approval_tenants"},
		{name: "UUID without hyphens", tenants: []string{strings.ReplaceAll(a, "-", "")}, wantErr: "notification.approval_tenants"},
		{name: "wildcard", tenants: []string{"*"}, wantErr: "notification.approval_tenants"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := Notification{ApprovalTenants: tc.tenants}
			got, err := n.ApprovalTenantIDs()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ApprovalTenantIDs = %v, %v; want an error naming %s", got, err, tc.wantErr)
				}
				cfg := Defaults()
				cfg.Auth.JWTSecret = strongTestSecret
				cfg.Notification.ApprovalTenants = tc.tenants
				if err := validate(&cfg); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("validate = %v, want an error naming %s", err, tc.wantErr)
				}
				return
			}
			if err != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("ApprovalTenantIDs = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestLoad_ApprovalTenantsFromEnv(t *testing.T) {
	const a, b = "aaaaaaaa-0000-4000-8000-000000000001", "bbbbbbbb-0000-4000-8000-000000000002"
	t.Setenv("CODEFORGE_NOTIFICATION_APPROVAL_TENANTS", a+","+b)
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got, err := cfg.Notification.ApprovalTenantIDs(); err != nil || !slices.Equal(got, []string{a, b}) {
		t.Fatalf("ApprovalTenantIDs = %v, %v", got, err)
	}
}

// KI-84: Slack approval messages link to the web UI's approval page, so they
// need notification.web_ui_url besides the Slack webhook.
func TestNotification_SlackApprovalsMissing(t *testing.T) {
	n := Notification{}
	if got := n.SlackApprovalsMissing(); !slices.Equal(got, []string{"notification.slack_webhook_url", "notification.web_ui_url"}) {
		t.Fatalf("missing = %v", got)
	}
	n.SlackWebhookURL = "https://hooks.slack.com/services/T/B/x"
	if got := n.SlackApprovalsMissing(); !slices.Equal(got, []string{"notification.web_ui_url"}) {
		t.Fatalf("missing = %v", got)
	}
	n.WebUIURL = "https://cf.example.com"
	if got := n.SlackApprovalsMissing(); len(got) != 0 {
		t.Fatalf("missing = %v, want none", got)
	}
}
