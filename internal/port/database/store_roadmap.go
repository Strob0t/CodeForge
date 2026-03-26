package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

// RoadmapStore defines database operations for roadmaps, milestones, and features.
type RoadmapStore interface {
	// Roadmaps
	CreateRoadmap(ctx context.Context, req roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error)
	GetRoadmap(ctx context.Context, id string) (*roadmap.Roadmap, error)
	GetRoadmapByProject(ctx context.Context, projectID string) (*roadmap.Roadmap, error)
	UpdateRoadmap(ctx context.Context, r *roadmap.Roadmap) error
	DeleteRoadmap(ctx context.Context, id string) error

	// Milestones
	CreateMilestone(ctx context.Context, req roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error)
	GetMilestone(ctx context.Context, id string) (*roadmap.Milestone, error)
	ListMilestones(ctx context.Context, roadmapID string) ([]roadmap.Milestone, error)
	UpdateMilestone(ctx context.Context, m *roadmap.Milestone) error
	DeleteMilestone(ctx context.Context, id string) error
	FindMilestoneByTitle(ctx context.Context, roadmapID, title string) (*roadmap.Milestone, error)

	// Features
	CreateFeature(ctx context.Context, req *roadmap.CreateFeatureRequest) (*roadmap.Feature, error)
	GetFeature(ctx context.Context, id string) (*roadmap.Feature, error)
	FindFeatureBySpecRef(ctx context.Context, milestoneID, specRef string) (*roadmap.Feature, error)
	ListFeatures(ctx context.Context, milestoneID string) ([]roadmap.Feature, error)
	ListFeaturesByRoadmap(ctx context.Context, roadmapID string) ([]roadmap.Feature, error)
	UpdateFeature(ctx context.Context, f *roadmap.Feature) error
	DeleteFeature(ctx context.Context, id string) error
}
