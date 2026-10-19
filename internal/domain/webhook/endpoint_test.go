package webhook

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// KI-85: a webhook is registered per project for one provider; its kind
// decides what its events do (VCS: push and pull request events, PM: a
// roadmap sync). Only PM webhooks carry an API token for the provider.
func TestCreateRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateRequest
		wantErr string
	}{
		{name: "vcs github", req: CreateRequest{Kind: KindVCS, Provider: "github"}},
		{name: "vcs gitlab", req: CreateRequest{Kind: KindVCS, Provider: "gitlab"}},
		{name: "pm github", req: CreateRequest{Kind: KindPM, Provider: "github"}},
		{name: "pm gitlab with a token", req: CreateRequest{Kind: KindPM, Provider: "gitlab", APIToken: "glpat-abc_123"}},
		// Plane generates its webhooks' signing secret itself: it is given.
		{name: "pm plane with Plane's secret", req: CreateRequest{Kind: KindPM, Provider: "plane", APIToken: "plane_api_x", Secret: "plane_wh_0123456789abcdef"}},
		{name: "pm plane secret of the shortest length", req: CreateRequest{Kind: KindPM, Provider: "plane", Secret: strings.Repeat("s", MinProvidedSecretLength)}},
		{name: "pm plane without its secret", req: CreateRequest{Kind: KindPM, Provider: "plane"}, wantErr: "secret"},
		{name: "pm plane secret too short", req: CreateRequest{Kind: KindPM, Provider: "plane", Secret: strings.Repeat("s", MinProvidedSecretLength-1)}, wantErr: "secret"},
		{name: "pm plane secret too long", req: CreateRequest{Kind: KindPM, Provider: "plane", Secret: strings.Repeat("s", MaxAPITokenLength+1)}, wantErr: "secret"},
		{name: "pm plane secret with a space", req: CreateRequest{Kind: KindPM, Provider: "plane", Secret: "plane secret 0123456789"}, wantErr: "secret"},
		{name: "a secret for a github webhook", req: CreateRequest{Kind: KindVCS, Provider: "github", Secret: "chosen-by-the-caller-0123"}, wantErr: "secret"},
		{name: "a secret for a gitlab webhook", req: CreateRequest{Kind: KindPM, Provider: "gitlab", Secret: "chosen-by-the-caller-0123"}, wantErr: "secret"},
		{name: "token of the longest length", req: CreateRequest{Kind: KindPM, Provider: "gitlab", APIToken: strings.Repeat("t", MaxAPITokenLength)}},
		{name: "vcs plane", req: CreateRequest{Kind: KindVCS, Provider: "plane"}, wantErr: "provider"},
		{name: "unknown kind", req: CreateRequest{Kind: "chat", Provider: "github"}, wantErr: "kind"},
		{name: "no kind", req: CreateRequest{Provider: "github"}, wantErr: "kind"},
		{name: "no provider", req: CreateRequest{Kind: KindPM}, wantErr: "provider"},
		{name: "provider in another case", req: CreateRequest{Kind: KindPM, Provider: "GitHub"}, wantErr: "provider"},
		{name: "token on a vcs webhook", req: CreateRequest{Kind: KindVCS, Provider: "github", APIToken: "t"}, wantErr: "api_token"},
		{name: "token too long", req: CreateRequest{Kind: KindPM, Provider: "gitlab", APIToken: strings.Repeat("t", MaxAPITokenLength+1)}, wantErr: "api_token"},
		{name: "token with a space", req: CreateRequest{Kind: KindPM, Provider: "gitlab", APIToken: "glpat abc"}, wantErr: "api_token"},
		{name: "token with a line break", req: CreateRequest{Kind: KindPM, Provider: "gitlab", APIToken: "glpat\nX-Evil: 1"}, wantErr: "api_token"},
		{name: "token with a non-ASCII character", req: CreateRequest{Kind: KindPM, Provider: "gitlab", APIToken: "glpät"}, wantErr: "api_token"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate = %v, want a validation error naming %s", err, tc.wantErr)
			}
		})
	}
}

func TestEndpoint_PathAndJSON(t *testing.T) {
	e := Endpoint{
		ID: "aaaaaaaa-0000-4000-8000-000000000001", TenantID: "t", ProjectID: "p", Kind: KindPM, Provider: "gitlab",
		EncryptedSecret: []byte("sealed-secret"), EncryptedAPIToken: []byte("sealed-token"),
	}
	if got, want := e.Path(), "/api/v1/webhooks/pm/gitlab/aaaaaaaa-0000-4000-8000-000000000001"; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
	data, err := json.Marshal(&e)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"sealed", "tenant", "secret\""} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("endpoint JSON %s contains %q", data, leaked)
		}
	}
}
