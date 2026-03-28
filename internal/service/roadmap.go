package service

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

// RoadmapService manages roadmaps, milestones, and features.
type RoadmapService struct {
	store     database.Store
	hub       broadcast.Broadcaster
	specProvs []specprovider.Provider
	pmProvs   []pmprovider.Provider
}

// NewRoadmapService creates a new RoadmapService.
func NewRoadmapService(store database.Store, hub broadcast.Broadcaster, specProvs []specprovider.Provider, pmProvs []pmprovider.Provider) *RoadmapService {
	return &RoadmapService{store: store, hub: hub, specProvs: specProvs, pmProvs: pmProvs}
}

// --- Roadmaps ---

// Create creates a new roadmap for a project.
func (s *RoadmapService) Create(ctx context.Context, req roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error) {
	if err := roadmap.ValidateCreateRoadmap(&req); err != nil {
		return nil, err
	}

	r, err := s.store.CreateRoadmap(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create roadmap: %w", err)
	}

	s.broadcastStatus(ctx, r)
	return r, nil
}

// GetByProject returns the roadmap for a project with all milestones and features.
// Uses batch loading to avoid N+1 queries: milestones and features are each
// loaded in a single query, then features are grouped by milestone in Go.
func (s *RoadmapService) GetByProject(ctx context.Context, projectID string) (*roadmap.Roadmap, error) {
	r, err := s.store.GetRoadmapByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	milestones, err := s.store.ListMilestones(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("load milestones: %w", err)
	}

	// Batch-load all features for the roadmap in a single query.
	allFeatures, err := s.store.ListFeaturesByRoadmap(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("load features: %w", err)
	}

	// Group features by milestone ID.
	featuresByMilestone := make(map[string][]roadmap.Feature, len(milestones))
	for _, f := range allFeatures {
		featuresByMilestone[f.MilestoneID] = append(featuresByMilestone[f.MilestoneID], f)
	}

	for i := range milestones {
		milestones[i].Features = featuresByMilestone[milestones[i].ID]
	}

	r.Milestones = milestones
	return r, nil
}

// Update updates a roadmap's title, description, and status.
func (s *RoadmapService) Update(ctx context.Context, r *roadmap.Roadmap) error {
	if err := roadmap.ValidateRoadmapStatus(r.Status); err != nil {
		return err
	}

	if err := s.store.UpdateRoadmap(ctx, r); err != nil {
		return err
	}

	s.broadcastStatus(ctx, r)
	return nil
}

// Delete removes a roadmap and all its milestones/features.
func (s *RoadmapService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteRoadmap(ctx, id)
}

// --- Milestones ---

// CreateMilestone creates a new milestone within a roadmap.
func (s *RoadmapService) CreateMilestone(ctx context.Context, req roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error) {
	if err := roadmap.ValidateCreateMilestone(&req); err != nil {
		return nil, err
	}
	return s.store.CreateMilestone(ctx, req)
}

// UpdateMilestone updates a milestone.
func (s *RoadmapService) UpdateMilestone(ctx context.Context, m *roadmap.Milestone) error {
	if err := roadmap.ValidateRoadmapStatus(m.Status); err != nil {
		return err
	}
	return s.store.UpdateMilestone(ctx, m)
}

// DeleteMilestone removes a milestone and its features.
func (s *RoadmapService) DeleteMilestone(ctx context.Context, id string) error {
	return s.store.DeleteMilestone(ctx, id)
}

// --- Features ---

// CreateFeature creates a new feature within a milestone.
func (s *RoadmapService) CreateFeature(ctx context.Context, req *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	if err := roadmap.ValidateCreateFeature(req); err != nil {
		return nil, err
	}
	return s.store.CreateFeature(ctx, req)
}

// UpdateFeature updates a feature.
func (s *RoadmapService) UpdateFeature(ctx context.Context, f *roadmap.Feature) error {
	if err := roadmap.ValidateFeatureStatus(f.Status); err != nil {
		return err
	}
	return s.store.UpdateFeature(ctx, f)
}

// DeleteFeature removes a feature.
func (s *RoadmapService) DeleteFeature(ctx context.Context, id string) error {
	return s.store.DeleteFeature(ctx, id)
}

// GetMilestone returns a single milestone by ID.
func (s *RoadmapService) GetMilestone(ctx context.Context, id string) (*roadmap.Milestone, error) {
	return s.store.GetMilestone(ctx, id)
}

// GetFeature returns a single feature by ID.
func (s *RoadmapService) GetFeature(ctx context.Context, id string) (*roadmap.Feature, error) {
	return s.store.GetFeature(ctx, id)
}

func (s *RoadmapService) broadcastStatus(ctx context.Context, r *roadmap.Roadmap) {
	if s.hub == nil {
		return
	}
	s.hub.BroadcastEvent(ctx, event.EventRoadmapStatus, event.RoadmapStatusEvent{
		RoadmapID: r.ID,
		ProjectID: r.ProjectID,
		Status:    string(r.Status),
		Title:     r.Title,
	})
}
