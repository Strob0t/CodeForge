package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// MicroagentService manages microagents — small trigger-based agents.
type MicroagentService struct {
	db database.Store
}

// NewMicroagentService creates a new MicroagentService.
func NewMicroagentService(db database.Store) *MicroagentService {
	return &MicroagentService{db: db}
}

// Create creates a new microagent from a request.
func (s *MicroagentService) Create(ctx context.Context, req microagent.CreateRequest) (*microagent.Microagent, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	m := &microagent.Microagent{
		ProjectID:      req.ProjectID,
		Name:           req.Name,
		Type:           req.Type,
		TriggerPattern: req.TriggerPattern,
		Description:    req.Description,
		Prompt:         req.Prompt,
		Enabled:        true,
	}
	if err := s.db.CreateMicroagent(ctx, m); err != nil {
		return nil, fmt.Errorf("create microagent: %w", err)
	}
	return m, nil
}

// Get retrieves a microagent by ID.
func (s *MicroagentService) Get(ctx context.Context, id string) (*microagent.Microagent, error) {
	return s.db.GetMicroagent(ctx, id)
}

// List returns all microagents for a project (including global ones).
func (s *MicroagentService) List(ctx context.Context, projectID string) ([]microagent.Microagent, error) {
	return s.db.ListMicroagents(ctx, projectID)
}

// Update updates a microagent.
func (s *MicroagentService) Update(ctx context.Context, id string, req microagent.UpdateRequest) (*microagent.Microagent, error) {
	m, err := s.db.GetMicroagent(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		m.Name = req.Name
	}
	if req.TriggerPattern != "" {
		m.TriggerPattern = req.TriggerPattern
	}
	if req.Description != "" {
		m.Description = req.Description
	}
	if req.Prompt != "" {
		m.Prompt = req.Prompt
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	if err := s.db.UpdateMicroagent(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete removes a microagent.
func (s *MicroagentService) Delete(ctx context.Context, id string) error {
	return s.db.DeleteMicroagent(ctx, id)
}

// Match returns all enabled microagents whose trigger pattern matches the given text.
func (s *MicroagentService) Match(ctx context.Context, projectID, text string) ([]microagent.Microagent, error) {
	all, err := s.db.ListMicroagents(ctx, projectID)
	if err != nil {
		return nil, err
	}

	var matched []microagent.Microagent
	for _, m := range all {
		if !m.Enabled {
			continue
		}
		if matchesTrigger(m.TriggerPattern, text) {
			matched = append(matched, m)
		}
	}
	return matched, nil
}

// matchesTrigger checks if text matches a trigger pattern.
// Patterns can be simple substrings or regex (prefixed with ^).
func matchesTrigger(pattern, text string) bool {
	if strings.HasPrefix(pattern, "^") || strings.HasPrefix(pattern, "(") {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(text)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
}
