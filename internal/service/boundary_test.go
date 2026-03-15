package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
)

type mockBoundaryStore struct {
	cfg *boundary.ProjectBoundaryConfig
}

func (m *mockBoundaryStore) GetProjectBoundaries(_ context.Context, _ string) (*boundary.ProjectBoundaryConfig, error) {
	if m.cfg == nil {
		return nil, fmt.Errorf("not found")
	}
	return m.cfg, nil
}

func (m *mockBoundaryStore) UpsertProjectBoundaries(_ context.Context, cfg *boundary.ProjectBoundaryConfig) error {
	m.cfg = cfg
	return nil
}

func (m *mockBoundaryStore) DeleteProjectBoundaries(_ context.Context, _ string) error {
	m.cfg = nil
	return nil
}

func TestBoundaryService_GetBoundaries(t *testing.T) {
	store := &mockBoundaryStore{cfg: &boundary.ProjectBoundaryConfig{
		ProjectID:  "proj-1",
		Boundaries: []boundary.BoundaryFile{{Path: "a.proto", Type: boundary.BoundaryTypeAPI}},
	}}
	svc := NewBoundaryService(store)
	cfg, err := svc.GetBoundaries(context.Background(), "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Boundaries) != 1 {
		t.Errorf("expected 1 boundary, got %d", len(cfg.Boundaries))
	}
}

func TestBoundaryService_UpdateBoundariesValidates(t *testing.T) {
	store := &mockBoundaryStore{}
	svc := NewBoundaryService(store)
	err := svc.UpdateBoundaries(context.Background(), &boundary.ProjectBoundaryConfig{
		ProjectID: "",
	})
	if err == nil {
		t.Error("expected validation error for empty project ID")
	}
}
