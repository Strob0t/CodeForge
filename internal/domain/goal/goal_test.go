package goal

import (
	"testing"
)

func TestGoalKindIsValid(t *testing.T) {
	valid := []GoalKind{KindVision, KindRequirement, KindConstraint, KindState, KindContext}
	for _, k := range valid {
		if !k.IsValid() {
			t.Errorf("expected %q to be valid", k)
		}
	}

	invalid := []GoalKind{"", "unknown", "VISION", "Vision"}
	for _, k := range invalid {
		if k.IsValid() {
			t.Errorf("expected %q to be invalid", k)
		}
	}
}

func TestCreateRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateRequest
		wantErr string
	}{
		{
			name:    "valid",
			req:     CreateRequest{Kind: KindVision, Title: "Project Vision", Content: "We build X"},
			wantErr: "",
		},
		{
			name:    "empty title",
			req:     CreateRequest{Kind: KindVision, Content: "content"},
			wantErr: "title is required",
		},
		{
			name:    "empty content",
			req:     CreateRequest{Kind: KindVision, Title: "title"},
			wantErr: "content is required",
		},
		{
			name:    "invalid kind",
			req:     CreateRequest{Kind: "bogus", Title: "title", Content: "content"},
			wantErr: "invalid kind",
		},
		{
			name:    "priority out of range high",
			req:     CreateRequest{Kind: KindVision, Title: "t", Content: "c", Priority: 101},
			wantErr: "priority must be 0-100",
		},
		{
			name:    "priority out of range negative",
			req:     CreateRequest{Kind: KindVision, Title: "t", Content: "c", Priority: -1},
			wantErr: "priority must be 0-100",
		},
		{
			name: "priority at bounds",
			req:  CreateRequest{Kind: KindConstraint, Title: "t", Content: "c", Priority: 100},
		},
		{
			name: "zero priority is valid (default)",
			req:  CreateRequest{Kind: KindState, Title: "t", Content: "c", Priority: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if got := err.Error(); !contains(got, tt.wantErr) {
				t.Errorf("error %q does not contain %q", got, tt.wantErr)
			}
		})
	}
}

func TestUpdateRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     UpdateRequest
		wantErr string
	}{
		{
			name: "all empty is valid (partial update)",
			req:  UpdateRequest{},
		},
		{
			name:    "invalid kind",
			req:     UpdateRequest{Kind: ptr(GoalKind("nope"))},
			wantErr: "invalid kind",
		},
		{
			name:    "priority too high",
			req:     UpdateRequest{Priority: intPtr(200)},
			wantErr: "priority must be 0-100",
		},
		{
			name: "valid partial update",
			req:  UpdateRequest{Title: strPtr("new title"), Priority: intPtr(50)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if got := err.Error(); !contains(got, tt.wantErr) {
				t.Errorf("error %q does not contain %q", got, tt.wantErr)
			}
		})
	}
}

func TestProjectGoalValidate(t *testing.T) {
	tests := []struct {
		name    string
		goal    ProjectGoal
		wantErr string
	}{
		{
			name:    "missing project_id",
			goal:    ProjectGoal{Kind: KindVision, Title: "t", Content: "c"},
			wantErr: "project_id is required",
		},
		{
			name:    "missing title",
			goal:    ProjectGoal{ProjectID: "p1", Kind: KindVision, Content: "c"},
			wantErr: "title is required",
		},
		{
			name:    "missing content",
			goal:    ProjectGoal{ProjectID: "p1", Kind: KindVision, Title: "t"},
			wantErr: "content is required",
		},
		{
			name:    "invalid kind",
			goal:    ProjectGoal{ProjectID: "p1", Kind: "bad", Title: "t", Content: "c"},
			wantErr: "invalid kind",
		},
		{
			name: "valid goal",
			goal: ProjectGoal{ProjectID: "p1", Kind: KindRequirement, Title: "t", Content: "c", Priority: 90},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.goal.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if got := err.Error(); !contains(got, tt.wantErr) {
				t.Errorf("error %q does not contain %q", got, tt.wantErr)
			}
		})
	}
}

func TestValidKinds(t *testing.T) {
	if len(ValidKinds) != 5 {
		t.Fatalf("expected 5 valid kinds, got %d", len(ValidKinds))
	}
	for _, k := range ValidKinds {
		if !k.IsValid() {
			t.Errorf("ValidKinds contains invalid kind %q", k)
		}
	}
}

// helpers

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func ptr(k GoalKind) *GoalKind { return &k }
func strPtr(s string) *string  { return &s }
func intPtr(i int) *int        { return &i }
