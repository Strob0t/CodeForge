package service

import (
	"context"
	"errors"
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
func (s *MicroagentService) Create(ctx context.Context, req *microagent.CreateRequest) (*microagent.Microagent, error) {
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
		if len(req.TriggerPattern) > microagent.MaxTriggerPatternLength {
			return nil, errors.New("trigger_pattern exceeds maximum length")
		}
		if strings.HasPrefix(req.TriggerPattern, "^") || strings.HasPrefix(req.TriggerPattern, "(") {
			if _, compileErr := regexp.Compile(req.TriggerPattern); compileErr != nil {
				return nil, fmt.Errorf("invalid trigger_pattern regex: %w", compileErr)
			}
		}
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
	for i := range all {
		if !all[i].Enabled {
			continue
		}
		if matchesTrigger(all[i].TriggerPattern, text) {
			matched = append(matched, all[i])
		}
	}
	return matched, nil
}

// maxTriggerInputLength is the maximum number of characters of input text
// to match against a regex trigger pattern, bounding regex execution time.
const maxTriggerInputLength = 10_000

// matchesTrigger checks if text matches a trigger pattern.
// Patterns can be simple substrings or regex (prefixed with ^ or ().
// Patterns exceeding MaxTriggerPatternLength are rejected to prevent ReDoS.
// Input text is truncated to maxTriggerInputLength before regex matching.
func matchesTrigger(pattern, text string) bool {
	if len(pattern) > microagent.MaxTriggerPatternLength {
		return false
	}
	if strings.HasPrefix(pattern, "^") || strings.HasPrefix(pattern, "(") {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		input := text
		if len(input) > maxTriggerInputLength {
			input = input[:maxTriggerInputLength]
		}
		return re.MatchString(input)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
}
