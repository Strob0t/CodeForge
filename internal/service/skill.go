package service

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/skill"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// SkillService manages reusable skills (workflows and code patterns) for agent prompt injection.
type SkillService struct {
	db database.Store
}

// NewSkillService creates a new SkillService.
func NewSkillService(db database.Store) *SkillService {
	return &SkillService{db: db}
}

// Create creates a new skill.
func (s *SkillService) Create(ctx context.Context, req *skill.CreateRequest) (*skill.Skill, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	skillType := req.Type
	if skillType == "" {
		skillType = skill.TypePattern
	}
	source := req.Source
	if source == "" {
		source = skill.SourceUser
	}
	formatOrigin := req.FormatOrigin
	if formatOrigin == "" {
		formatOrigin = "codeforge"
	}

	sk := &skill.Skill{
		ProjectID:    req.ProjectID,
		Name:         req.Name,
		Type:         skillType,
		Description:  req.Description,
		Language:     req.Language,
		Content:      req.Content,
		Code:         req.Content, // backwards compat: also populate Code
		Tags:         req.Tags,
		Source:       source,
		SourceURL:    req.SourceURL,
		FormatOrigin: formatOrigin,
		Status:       skill.StatusActive,
		Enabled:      true, // backwards compat
	}
	if err := s.db.CreateSkill(ctx, sk); err != nil {
		return nil, fmt.Errorf("create skill: %w", err)
	}
	return sk, nil
}

// Get retrieves a skill by ID.
func (s *SkillService) Get(ctx context.Context, id string) (*skill.Skill, error) {
	return s.db.GetSkill(ctx, id)
}

// List returns all skills for a project (including global ones).
func (s *SkillService) List(ctx context.Context, projectID string) ([]skill.Skill, error) {
	return s.db.ListSkills(ctx, projectID)
}

// ListActive returns only active skills for a project (including global ones).
func (s *SkillService) ListActive(ctx context.Context, projectID string) ([]skill.Skill, error) {
	return s.db.ListActiveSkills(ctx, projectID)
}

// IncrementUsage atomically increments a skill's usage counter.
func (s *SkillService) IncrementUsage(ctx context.Context, id string) error {
	return s.db.IncrementSkillUsage(ctx, id)
}

// Update updates a skill.
func (s *SkillService) Update(ctx context.Context, id string, req *skill.UpdateRequest) (*skill.Skill, error) {
	sk, err := s.db.GetSkill(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		sk.Name = req.Name
	}
	if req.Type != "" {
		sk.Type = req.Type
	}
	if req.Description != "" {
		sk.Description = req.Description
	}
	if req.Language != "" {
		sk.Language = req.Language
	}
	if req.Content != "" {
		sk.Content = req.Content
	}
	if req.Tags != nil {
		sk.Tags = req.Tags
	}
	if req.Status != nil {
		sk.Status = *req.Status
	}
	if err := s.db.UpdateSkill(ctx, sk); err != nil {
		return nil, err
	}
	return sk, nil
}

// Delete removes a skill.
func (s *SkillService) Delete(ctx context.Context, id string) error {
	return s.db.DeleteSkill(ctx, id)
}
