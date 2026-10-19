package config

import (
	"strings"
	"testing"
)

// KI-55: the GitHub OAuth web flow is wired when github.client_id,
// client_secret and callback_url are set; the callback URL is the only
// redirect URI the service sends to GitHub and must point at its callback.
func TestValidate_GitHubOAuthWebFlow(t *testing.T) {
	const callback = "https://codeforge.example.com/api/v1/auth/github/callback"
	tests := []struct {
		name     string
		id       string
		secret   string
		callback string
		wantWeb  bool
		wantErr  string
	}{
		{name: "not configured"},
		{name: "device flow only (client id)", id: "Iv1.abc"},
		{name: "web flow", id: "Iv1.abc", secret: "s", callback: callback, wantWeb: true},
		{name: "web flow on loopback over http", id: "Iv1.abc", secret: "s", callback: "http://localhost:3000/api/v1/auth/github/callback", wantWeb: true},
		{name: "web flow on 127.0.0.1 over http", id: "Iv1.abc", secret: "s", callback: "http://127.0.0.1:3000/api/v1/auth/github/callback", wantWeb: true},
		{name: "secret without callback", id: "Iv1.abc", secret: "s", wantErr: "github.callback_url"},
		{name: "callback without secret", id: "Iv1.abc", callback: callback, wantErr: "github.client_secret"},
		{name: "secret without client id", secret: "s", callback: callback, wantErr: "github.client_id"},
		{name: "plain http off loopback", id: "i", secret: "s", callback: "http://codeforge.example.com/api/v1/auth/github/callback", wantErr: "https"},
		{name: "other scheme", id: "i", secret: "s", callback: "ftp://codeforge.example.com/api/v1/auth/github/callback", wantErr: "https"},
		{name: "relative", id: "i", secret: "s", callback: "/api/v1/auth/github/callback", wantErr: "absolute"},
		{name: "user info", id: "i", secret: "s", callback: "https://u:p@codeforge.example.com/api/v1/auth/github/callback", wantErr: "user info"},
		{name: "query", id: "i", secret: "s", callback: callback + "?next=/x", wantErr: "query"},
		{name: "fragment", id: "i", secret: "s", callback: callback + "#x", wantErr: "query"},
		{name: "other path", id: "i", secret: "s", callback: "https://codeforge.example.com/login", wantErr: "/api/v1/auth/github/callback"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.AppEnv = "development"
			if err := ensureSecrets(&cfg); err != nil {
				t.Fatal(err)
			}
			cfg.GitHub = GitHub{ClientID: tc.id, ClientSecret: tc.secret, CallbackURL: tc.callback}
			err := validate(&cfg)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("validate = %v, want an error containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if got := cfg.GitHub.WebFlowConfigured(); got != tc.wantWeb {
				t.Fatalf("WebFlowConfigured = %v, want %v", got, tc.wantWeb)
			}
		})
	}
}
