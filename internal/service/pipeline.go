package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// PipelineService manages pipeline templates (built-in + custom).
type PipelineService struct {
	mu        sync.RWMutex
	templates map[string]pipeline.Template
	modes     *ModeService
}

// NewPipelineService creates a PipelineService pre-loaded with built-in templates.
func NewPipelineService(modes *ModeService) *PipelineService {
	s := &PipelineService{
		templates: make(map[string]pipeline.Template),
		modes:     modes,
	}
	for _, t := range pipeline.BuiltinTemplates() {
		s.templates[t.ID] = t
	}
	return s
}

// List returns all registered pipeline templates.
func (s *PipelineService) List() []pipeline.Template {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]pipeline.Template, 0, len(s.templates))
	for _, t := range s.templates {
		result = append(result, t)
	}
	return result
}

// Get returns a template by ID.
func (s *PipelineService) Get(id string) (*pipeline.Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.templates[id]
	if !ok {
		return nil, fmt.Errorf("pipeline template %q not found", id)
	}
	return &t, nil
}

// Register adds a custom template. Built-in templates cannot be overwritten.
func (s *PipelineService) Register(t *pipeline.Template) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("validate template: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.templates[t.ID]; ok && existing.Builtin {
		return fmt.Errorf("cannot overwrite built-in template %q", t.ID)
	}
	s.templates[t.ID] = *t
	return nil
}

// Instantiate creates a CreatePlanRequest from a template, validating mode references.
func (s *PipelineService) Instantiate(_ context.Context, templateID string, req pipeline.InstantiateRequest) (*plan.CreatePlanRequest, error) {
	t, err := s.Get(templateID)
	if err != nil {
		return nil, err
	}

	// Validate all mode references exist.
	for i, step := range t.Steps {
		if _, mErr := s.modes.Get(step.ModeID); mErr != nil {
			return nil, fmt.Errorf("step %d (%s): %w", i, step.Name, mErr)
		}
	}

	return t.Instantiate(req)
}
