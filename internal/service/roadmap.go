package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// RoadmapService manages roadmaps, milestones, and features.
type RoadmapService struct {
	store database.Store
	hub   broadcast.Broadcaster
}

// NewRoadmapService creates a new RoadmapService.
func NewRoadmapService(store database.Store, hub broadcast.Broadcaster) *RoadmapService {
	return &RoadmapService{store: store, hub: hub}
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
func (s *RoadmapService) GetByProject(ctx context.Context, projectID string) (*roadmap.Roadmap, error) {
	r, err := s.store.GetRoadmapByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	milestones, err := s.store.ListMilestones(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("load milestones: %w", err)
	}

	for i := range milestones {
		features, err := s.store.ListFeatures(ctx, milestones[i].ID)
		if err != nil {
			return nil, fmt.Errorf("load features for milestone %s: %w", milestones[i].ID, err)
		}
		milestones[i].Features = features
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

// --- Auto-Detect ---

// fileMarkers maps spec format names to their file/directory indicators.
var fileMarkers = map[string][]string{
	"roadmap_md": {"ROADMAP.md", "roadmap.md"},
	"openspec":   {"openspec/"},
	"speckit":    {".specify/"},
	"autospec":   {"specs/spec.yaml", "specs/spec.yml"},
}

// AutoDetect scans a workspace for known spec file markers.
func (s *RoadmapService) AutoDetect(ctx context.Context, projectID string) (*roadmap.DetectionResult, error) {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if proj.WorkspacePath == "" {
		return &roadmap.DetectionResult{Found: false}, nil
	}

	result := &roadmap.DetectionResult{}

	for format, markers := range fileMarkers {
		for _, marker := range markers {
			fullPath := filepath.Join(proj.WorkspacePath, marker)
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			// Directory markers end with /
			if strings.HasSuffix(marker, "/") && info.IsDir() {
				result.Found = true
				result.FileMarkers = append(result.FileMarkers, marker)
				result.Format = format
				result.Path = fullPath
			} else if !info.IsDir() {
				result.Found = true
				result.FileMarkers = append(result.FileMarkers, marker)
				result.Format = format
				result.Path = fullPath
			}
		}
	}

	return result, nil
}

// --- AI View ---

// AIView returns an LLM-optimized representation of a project's roadmap.
func (s *RoadmapService) AIView(ctx context.Context, projectID, format string) (*roadmap.AIRoadmapView, error) {
	r, err := s.GetByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	view := &roadmap.AIRoadmapView{
		ProjectID:   projectID,
		Format:      format,
		GeneratedAt: time.Now(),
	}

	switch format {
	case "json":
		data, err := json.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("marshal roadmap: %w", err)
		}
		view.Content = string(data)
		view.RawData = data
	case "yaml":
		view.Content = renderYAML(r)
	default:
		view.Content = renderMarkdown(r)
		view.Format = "markdown"
	}

	return view, nil
}

func renderMarkdown(r *roadmap.Roadmap) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", r.Title))
	if r.Description != "" {
		b.WriteString(r.Description + "\n\n")
	}
	b.WriteString(fmt.Sprintf("Status: %s\n\n", r.Status))

	for i := range r.Milestones {
		m := &r.Milestones[i]
		b.WriteString(fmt.Sprintf("## %s [%s]\n\n", m.Title, m.Status))
		if m.Description != "" {
			b.WriteString(m.Description + "\n\n")
		}
		for j := range m.Features {
			f := &m.Features[j]
			checkbox := "[ ]"
			if f.Status == roadmap.FeatureDone {
				checkbox = "[x]"
			}
			b.WriteString(fmt.Sprintf("- %s %s [%s]", checkbox, f.Title, f.Status))
			if len(f.Labels) > 0 {
				b.WriteString(fmt.Sprintf(" (%s)", strings.Join(f.Labels, ", ")))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

func renderYAML(r *roadmap.Roadmap) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("title: %q\n", r.Title))
	b.WriteString(fmt.Sprintf("status: %s\n", r.Status))
	b.WriteString("milestones:\n")

	for i := range r.Milestones {
		m := &r.Milestones[i]
		b.WriteString(fmt.Sprintf("  - title: %q\n", m.Title))
		b.WriteString(fmt.Sprintf("    status: %s\n", m.Status))
		b.WriteString("    features:\n")
		for j := range m.Features {
			f := &m.Features[j]
			b.WriteString(fmt.Sprintf("      - title: %q\n", f.Title))
			b.WriteString(fmt.Sprintf("        status: %s\n", f.Status))
			if len(f.Labels) > 0 {
				b.WriteString("        labels:\n")
				for _, l := range f.Labels {
					b.WriteString(fmt.Sprintf("          - %q\n", l))
				}
			}
		}
	}

	return b.String()
}

func (s *RoadmapService) broadcastStatus(ctx context.Context, r *roadmap.Roadmap) {
	if s.hub == nil {
		return
	}
	s.hub.BroadcastEvent(ctx, ws.EventRoadmapStatus, ws.RoadmapStatusEvent{
		RoadmapID: r.ID,
		ProjectID: r.ProjectID,
		Status:    string(r.Status),
		Title:     r.Title,
	})
}
