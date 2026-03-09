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
				Content:     "def make_request(url): ...",
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
				Content:     "def query_db(sql): ...",
				Tags:        []string{"database", "sql"},
			},
			wantErr: "",
		},
		{
			name: "missing name",
			req: CreateRequest{
				Description: "some description",
				Content:     "some content",
			},
			wantErr: "name is required",
		},
		{
			name: "missing content",
			req: CreateRequest{
				Name:        "test",
				Description: "some description",
			},
			wantErr: "content is required",
		},
		{
			name: "missing description",
			req: CreateRequest{
				Name:    "test",
				Content: "some content",
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
				Content:     "content",
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
		Type:        TypePattern,
		Description: "Git operations helper",
		Language:    "python",
		Content:     "def git_status(): subprocess.run(['git', 'status'])",
		Tags:        []string{"git", "vcs"},
		Source:      SourceUser,
		Status:      StatusActive,
		CreatedAt:   now,
		// Deprecated fields still accessible for backwards compat.
		Code:    "def git_status(): subprocess.run(['git', 'status'])",
		Enabled: true,
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
	if s.Type != TypePattern {
		t.Errorf("Type = %q, want %q", s.Type, TypePattern)
	}
	if s.Content != "def git_status(): subprocess.run(['git', 'status'])" {
		t.Errorf("Content = %q, want git_status code", s.Content)
	}
	if s.Source != SourceUser {
		t.Errorf("Source = %q, want %q", s.Source, SourceUser)
	}
	if s.Status != StatusActive {
		t.Errorf("Status = %q, want %q", s.Status, StatusActive)
	}
	// Backwards compat: Code and Enabled still work.
	if !s.Enabled {
		t.Error("Enabled = false, want true")
	}
	if s.Code != s.Content {
		t.Errorf("Code = %q, want same as Content", s.Code)
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

	status := StatusActive
	u := UpdateRequest{
		Name:        "updated-name",
		Description: "updated description",
		Language:    "go",
		Content:     "func helper() {}",
		Tags:        []string{"updated"},
		Status:      &status,
	}

	if u.Name != "updated-name" {
		t.Errorf("Name = %q, want %q", u.Name, "updated-name")
	}
	if u.Content != "func helper() {}" {
		t.Errorf("Content = %q, want %q", u.Content, "func helper() {}")
	}
	if u.Status == nil || *u.Status != StatusActive {
		t.Errorf("Status should be %q", StatusActive)
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
	if u.Content != "" {
		t.Errorf("Content = %q, want empty", u.Content)
	}
	if u.Tags != nil {
		t.Errorf("Tags = %v, want nil", u.Tags)
	}
	if u.Status != nil {
		t.Errorf("Status = %v, want nil", u.Status)
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

// --- New tests for extended skill model ---

func TestCreateRequest_Validate_ContentRequired(t *testing.T) {
	t.Parallel()

	req := CreateRequest{Name: "test", Description: "desc", Content: ""}
	err := req.Validate()
	if err == nil || err.Error() != "content is required" {
		t.Errorf("expected content required error, got %v", err)
	}
}

func TestCreateRequest_Validate_InvalidType(t *testing.T) {
	t.Parallel()

	req := CreateRequest{Name: "test", Description: "desc", Content: "x", Type: "invalid"}
	err := req.Validate()
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestCreateRequest_Validate_ValidWorkflow(t *testing.T) {
	t.Parallel()

	req := CreateRequest{Name: "test", Description: "desc", Content: "steps", Type: "workflow"}
	if err := req.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateRequest_Validate_DefaultType(t *testing.T) {
	t.Parallel()

	req := CreateRequest{Name: "test", Description: "desc", Content: "code"}
	if err := req.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSkillStatus_Constants(t *testing.T) {
	t.Parallel()

	if StatusDraft != "draft" || StatusActive != "active" || StatusDisabled != "disabled" {
		t.Error("status constants don't match expected values")
	}
}

func TestSkillSource_Constants(t *testing.T) {
	t.Parallel()

	if SourceBuiltin != "builtin" || SourceImport != "import" || SourceUser != "user" || SourceAgent != "agent" {
		t.Error("source constants don't match expected values")
	}
}
