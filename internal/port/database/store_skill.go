package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/skill"
)

// SkillStore defines database operations for skill management.
type SkillStore interface {
	CreateSkill(ctx context.Context, s *skill.Skill) error
	GetSkill(ctx context.Context, id string) (*skill.Skill, error)
	ListSkills(ctx context.Context, projectID string) ([]skill.Skill, error)
	UpdateSkill(ctx context.Context, s *skill.Skill) error
	DeleteSkill(ctx context.Context, id string) error
	IncrementSkillUsage(ctx context.Context, id string) error
	ListActiveSkills(ctx context.Context, projectID string) ([]skill.Skill, error)
}
