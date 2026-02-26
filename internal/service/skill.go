package service

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/skill"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// SkillService manages reusable code snippets for agent prompt injection.
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
	sk := &skill.Skill{
		ProjectID:   req.ProjectID,
		Name:        req.Name,
		Description: req.Description,
		Language:    req.Language,
		Code:        req.Code,
		Tags:        req.Tags,
		Enabled:     true,
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

// Update updates a skill.
func (s *SkillService) Update(ctx context.Context, id string, req *skill.UpdateRequest) (*skill.Skill, error) {
	sk, err := s.db.GetSkill(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		sk.Name = req.Name
	}
	if req.Description != "" {
		sk.Description = req.Description
	}
	if req.Language != "" {
		sk.Language = req.Language
	}
	if req.Code != "" {
		sk.Code = req.Code
	}
	if req.Tags != nil {
		sk.Tags = req.Tags
	}
	if req.Enabled != nil {
		sk.Enabled = *req.Enabled
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
