package roadmap

import (
	"testing"
)

func TestValidateCreateRoadmap(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateRoadmapRequest
		wantErr bool
	}{
		{
			name:    "valid",
			req:     CreateRoadmapRequest{ProjectID: "p1", Title: "v1.0"},
			wantErr: false,
		},
		{
			name:    "missing project_id",
			req:     CreateRoadmapRequest{Title: "v1.0"},
			wantErr: true,
		},
		{
			name:    "missing title",
			req:     CreateRoadmapRequest{ProjectID: "p1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateRoadmap(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateRoadmap() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCreateMilestone(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateMilestoneRequest
		wantErr bool
	}{
		{
			name:    "valid",
			req:     CreateMilestoneRequest{RoadmapID: "r1", Title: "Phase 1"},
			wantErr: false,
		},
		{
			name:    "missing roadmap_id",
			req:     CreateMilestoneRequest{Title: "Phase 1"},
			wantErr: true,
		},
		{
			name:    "missing title",
			req:     CreateMilestoneRequest{RoadmapID: "r1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateMilestone(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateMilestone() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCreateFeature(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateFeatureRequest
		wantErr bool
	}{
		{
			name:    "valid",
			req:     CreateFeatureRequest{MilestoneID: "m1", Title: "Auth"},
			wantErr: false,
		},
		{
			name:    "missing milestone_id",
			req:     CreateFeatureRequest{Title: "Auth"},
			wantErr: true,
		},
		{
			name:    "missing title",
			req:     CreateFeatureRequest{MilestoneID: "m1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateFeature(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreateFeature() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRoadmapStatus(t *testing.T) {
	valid := []RoadmapStatus{StatusDraft, StatusActive, StatusComplete, StatusArchived}
	for _, s := range valid {
		if err := ValidateRoadmapStatus(s); err != nil {
			t.Errorf("ValidateRoadmapStatus(%s) unexpected error: %v", s, err)
		}
	}

	if err := ValidateRoadmapStatus("invalid"); err == nil {
		t.Error("ValidateRoadmapStatus(invalid) expected error")
	}
}

func TestValidateFeatureStatus(t *testing.T) {
	valid := []FeatureStatus{FeatureBacklog, FeaturePlanned, FeatureInProgress, FeatureDone, FeatureCancelled}
	for _, s := range valid {
		if err := ValidateFeatureStatus(s); err != nil {
			t.Errorf("ValidateFeatureStatus(%s) unexpected error: %v", s, err)
		}
	}

	if err := ValidateFeatureStatus("invalid"); err == nil {
		t.Error("ValidateFeatureStatus(invalid) expected error")
	}
}
