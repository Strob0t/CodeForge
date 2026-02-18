package roadmap

import "fmt"

// ValidateCreateRoadmap validates a CreateRoadmapRequest.
func ValidateCreateRoadmap(req *CreateRoadmapRequest) error {
	if req.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if req.Title == "" {
		return fmt.Errorf("title is required")
	}
	return nil
}

// ValidateCreateMilestone validates a CreateMilestoneRequest.
func ValidateCreateMilestone(req *CreateMilestoneRequest) error {
	if req.RoadmapID == "" {
		return fmt.Errorf("roadmap_id is required")
	}
	if req.Title == "" {
		return fmt.Errorf("title is required")
	}
	return nil
}

// ValidateCreateFeature validates a CreateFeatureRequest.
func ValidateCreateFeature(req *CreateFeatureRequest) error {
	if req.MilestoneID == "" {
		return fmt.Errorf("milestone_id is required")
	}
	if req.Title == "" {
		return fmt.Errorf("title is required")
	}
	return nil
}

// ValidateRoadmapStatus checks if a roadmap status value is valid.
func ValidateRoadmapStatus(s RoadmapStatus) error {
	switch s {
	case StatusDraft, StatusActive, StatusComplete, StatusArchived:
		return nil
	default:
		return fmt.Errorf("invalid roadmap status: %s", s)
	}
}

// ValidateFeatureStatus checks if a feature status value is valid.
func ValidateFeatureStatus(s FeatureStatus) error {
	switch s {
	case FeatureBacklog, FeaturePlanned, FeatureInProgress, FeatureDone, FeatureCancelled:
		return nil
	default:
		return fmt.Errorf("invalid feature status: %s", s)
	}
}
