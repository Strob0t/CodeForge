package project

import (
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
)

func TestValidateCreateRequest(t *testing.T) {
	// Simulate available providers for testing.
	providers := []string{"github", "gitlab", "gitea", "local"}

	tests := []struct {
		name    string
		req     CreateRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid request with name only",
			req:     CreateRequest{Name: "my-project"},
			wantErr: false,
		},
		{
			name: "valid request with all fields",
			req: CreateRequest{
				Name:        "my-project",
				Description: "A test project",
				RepoURL:     "https://github.com/user/repo.git",
				Provider:    "github",
			},
			wantErr: false,
		},
		{
			name:    "valid request with SSH URL",
			req:     CreateRequest{Name: "my-project", RepoURL: "git@github.com:user/repo.git"},
			wantErr: false,
		},
		{
			name:    "valid request with repo URL and no name",
			req:     CreateRequest{RepoURL: "https://github.com/user/repo.git"},
			wantErr: false,
		},
		{
			name:    "empty name and empty URL",
			req:     CreateRequest{},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name:    "name too long",
			req:     CreateRequest{Name: strings.Repeat("a", 256)},
			wantErr: true,
			errMsg:  "name exceeds 255 characters",
		},
		{
			name:    "name at max length is valid",
			req:     CreateRequest{Name: strings.Repeat("a", 255)},
			wantErr: false,
		},
		{
			name:    "name with control characters",
			req:     CreateRequest{Name: "my-project\x00"},
			wantErr: true,
			errMsg:  "control characters",
		},
		{
			name:    "name with tab control character",
			req:     CreateRequest{Name: "my\tproject"},
			wantErr: true,
			errMsg:  "control characters",
		},
		{
			name:    "invalid provider",
			req:     CreateRequest{Name: "my-project", Provider: "nonexistent-provider"},
			wantErr: true,
			errMsg:  "unknown provider",
		},
		{
			name:    "valid provider",
			req:     CreateRequest{Name: "my-project", Provider: "github"},
			wantErr: false,
		},
		{
			name:    "invalid repo URL - plain string",
			req:     CreateRequest{Name: "my-project", RepoURL: "not-a-url"},
			wantErr: true,
			errMsg:  "repo_url must start with https://",
		},
		{
			name:    "invalid repo URL - http",
			req:     CreateRequest{Name: "my-project", RepoURL: "http://example.com/repo.git"},
			wantErr: true,
			errMsg:  "repo_url must start with https://",
		},
		{
			name:    "description too long",
			req:     CreateRequest{Name: "my-project", Description: strings.Repeat("x", 2001)},
			wantErr: true,
			errMsg:  "description exceeds 2000 characters",
		},
		{
			name:    "description at max length is valid",
			req:     CreateRequest{Name: "my-project", Description: strings.Repeat("x", 2000)},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateRequest(tt.req, providers)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, domain.ErrValidation) {
					t.Errorf("expected ErrValidation, got: %v", err)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error to contain %q, got: %v", tt.errMsg, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateUpdateRequest(t *testing.T) {
	name := "updated"
	longName := strings.Repeat("a", 256)
	ctrlName := "test\x00"
	desc := strings.Repeat("x", 2001)
	badURL := "not-a-url"
	goodURL := "https://github.com/user/repo"

	tests := []struct {
		name    string
		req     UpdateRequest
		wantErr bool
	}{
		{name: "empty update is valid", req: UpdateRequest{}, wantErr: false},
		{name: "valid name update", req: UpdateRequest{Name: &name}, wantErr: false},
		{name: "too long name", req: UpdateRequest{Name: &longName}, wantErr: true},
		{name: "control char name", req: UpdateRequest{Name: &ctrlName}, wantErr: true},
		{name: "too long description", req: UpdateRequest{Description: &desc}, wantErr: true},
		{name: "invalid URL", req: UpdateRequest{RepoURL: &badURL}, wantErr: true},
		{name: "valid URL", req: UpdateRequest{RepoURL: &goodURL}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateRequest(tt.req)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
