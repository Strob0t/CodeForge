package skill

import (
	"testing"
	"time"
)

func TestCreateRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     CreateRequest
		wantErr string
	}{
		{
			name: "valid request",
			req: CreateRequest{
				Name:        "http-client",
				Description: "HTTP client helper for making API calls",
				Code:        "def make_request(url): ...",
			},
			wantErr: "",
		},
		{
			name: "valid with all fields",
			req: CreateRequest{
				ProjectID:   "proj-1",
				Name:        "db-query",
				Description: "Database query helper",
				Language:    "python",
				Code:        "def query_db(sql): ...",
				Tags:        []string{"database", "sql"},
			},
			wantErr: "",
		},
		{
			name: "missing name",
			req: CreateRequest{
				Description: "some description",
				Code:        "some code",
			},
			wantErr: "name is required",
		},
		{
			name: "missing code",
			req: CreateRequest{
				Name:        "test",
				Description: "some description",
			},
			wantErr: "code is required",
		},
		{
			name: "missing description",
			req: CreateRequest{
				Name: "test",
				Code: "some code",
			},
			wantErr: "description is required",
		},
		{
			name:    "all empty",
			req:     CreateRequest{},
			wantErr: "name is required",
		},
		{
			name: "empty name",
			req: CreateRequest{
				Name:        "",
				Description: "desc",
				Code:        "code",
			},
			wantErr: "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() = nil, want error %q", tt.wantErr)
				} else if err.Error() != tt.wantErr {
					t.Errorf("Validate() = %q, want %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestSkillFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	s := Skill{
		ID:          "skill-1",
		TenantID:    "tenant-1",
		ProjectID:   "proj-1",
		Name:        "git-helper",
		Description: "Git operations helper",
		Language:    "python",
		Code:        "def git_status(): subprocess.run(['git', 'status'])",
		Tags:        []string{"git", "vcs"},
		Enabled:     true,
		CreatedAt:   now,
	}

	if s.ID != "skill-1" {
		t.Errorf("ID = %q, want %q", s.ID, "skill-1")
	}
	if s.Name != "git-helper" {
		t.Errorf("Name = %q, want %q", s.Name, "git-helper")
	}
	if s.Language != "python" {
		t.Errorf("Language = %q, want %q", s.Language, "python")
	}
	if len(s.Tags) != 2 {
		t.Errorf("len(Tags) = %d, want 2", len(s.Tags))
	}
	if !s.Enabled {
		t.Error("Enabled = false, want true")
	}
}

func TestSkillGlobalScope(t *testing.T) {
	t.Parallel()

	s := Skill{
		ID:       "skill-global",
		TenantID: "tenant-1",
		Name:     "global-skill",
	}

	// ProjectID empty means global scope.
	if s.ProjectID != "" {
		t.Errorf("ProjectID = %q, want empty for global scope", s.ProjectID)
	}
}

func TestUpdateRequestFields(t *testing.T) {
	t.Parallel()

	enabled := true
	u := UpdateRequest{
		Name:        "updated-name",
		Description: "updated description",
		Language:    "go",
		Code:        "func helper() {}",
		Tags:        []string{"updated"},
		Enabled:     &enabled,
	}

	if u.Name != "updated-name" {
		t.Errorf("Name = %q, want %q", u.Name, "updated-name")
	}
	if u.Enabled == nil || !*u.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestUpdateRequestPartial(t *testing.T) {
	t.Parallel()

	// Only update name, leave everything else as zero values.
	u := UpdateRequest{
		Name: "just-name",
	}

	if u.Name != "just-name" {
		t.Errorf("Name = %q, want %q", u.Name, "just-name")
	}
	if u.Description != "" {
		t.Errorf("Description = %q, want empty", u.Description)
	}
	if u.Code != "" {
		t.Errorf("Code = %q, want empty", u.Code)
	}
	if u.Tags != nil {
		t.Errorf("Tags = %v, want nil", u.Tags)
	}
	if u.Enabled != nil {
		t.Errorf("Enabled = %v, want nil", u.Enabled)
	}
}

func TestCreateRequestValidateOrder(t *testing.T) {
	t.Parallel()

	// When all fields are missing, name should be reported first.
	req := CreateRequest{}
	err := req.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error")
	}
	if err.Error() != "name is required" {
		t.Errorf("Validate() = %q, want first validation to be name", err.Error())
	}
}
