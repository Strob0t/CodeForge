// Package roadmap contains the domain models for Roadmap/Feature-Map (Pillar 2).
package roadmap

import (
	"encoding/json"
	"time"
)

// RoadmapStatus represents the lifecycle state of a roadmap or milestone.
type RoadmapStatus string

const (
	StatusDraft    RoadmapStatus = "draft"
	StatusActive   RoadmapStatus = "active"
	StatusComplete RoadmapStatus = "complete"
	StatusArchived RoadmapStatus = "archived"
)

// FeatureStatus represents the lifecycle state of a feature.
type FeatureStatus string

const (
	FeatureBacklog    FeatureStatus = "backlog"
	FeaturePlanned    FeatureStatus = "planned"
	FeatureInProgress FeatureStatus = "in_progress"
	FeatureDone       FeatureStatus = "done"
	FeatureCancelled  FeatureStatus = "cancelled"
)

// Roadmap is the top-level container linked 1:1 with a project.
type Roadmap struct {
	ID          string        `json:"id"`
	ProjectID   string        `json:"project_id"`
	TenantID    string        `json:"tenant_id,omitempty"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Status      RoadmapStatus `json:"status"`
	Version     int           `json:"version"`
	Milestones  []Milestone   `json:"milestones,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// Milestone groups features within a roadmap.
type Milestone struct {
	ID          string        `json:"id"`
	RoadmapID   string        `json:"roadmap_id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Status      RoadmapStatus `json:"status"`
	SortOrder   int           `json:"sort_order"`
	DueDate     *time.Time    `json:"due_date,omitempty"`
	Version     int           `json:"version"`
	Features    []Feature     `json:"features,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// Feature represents a single deliverable tracked within a milestone.
type Feature struct {
	ID          string            `json:"id"`
	MilestoneID string            `json:"milestone_id"`
	RoadmapID   string            `json:"roadmap_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      FeatureStatus     `json:"status"`
	Labels      []string          `json:"labels"`
	SpecRef     string            `json:"spec_ref,omitempty"`
	ExternalIDs map[string]string `json:"external_ids,omitempty"`
	SortOrder   int               `json:"sort_order"`
	Version     int               `json:"version"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// CreateRoadmapRequest is the input for creating a roadmap.
type CreateRoadmapRequest struct {
	ProjectID   string `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// CreateMilestoneRequest is the input for creating a milestone.
type CreateMilestoneRequest struct {
	RoadmapID   string     `json:"roadmap_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date,omitempty"`
}

// CreateFeatureRequest is the input for creating a feature.
type CreateFeatureRequest struct {
	MilestoneID string            `json:"milestone_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Labels      []string          `json:"labels"`
	SpecRef     string            `json:"spec_ref,omitempty"`
	ExternalIDs map[string]string `json:"external_ids,omitempty"`
}

// DetectionResult describes what was found by auto-detection in a workspace.
type DetectionResult struct {
	Found       bool     `json:"found"`
	FileMarkers []string `json:"file_markers,omitempty"`
	Format      string   `json:"format,omitempty"`
	Path        string   `json:"path,omitempty"`
}

// ImportResult summarizes the outcome of a spec or PM import operation.
type ImportResult struct {
	Source            string   `json:"source"`
	MilestonesCreated int      `json:"milestones_created"`
	FeaturesCreated   int      `json:"features_created"`
	Errors            []string `json:"errors,omitempty"`
}

// AIRoadmapView is an LLM-optimized view of a roadmap.
type AIRoadmapView struct {
	ProjectID   string          `json:"project_id"`
	Format      string          `json:"format"`
	Content     string          `json:"content"`
	RawData     json.RawMessage `json:"raw_data,omitempty"`
	GeneratedAt time.Time       `json:"generated_at"`
}
