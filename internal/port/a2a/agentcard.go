package a2a

import "github.com/Strob0t/CodeForge/internal/domain/mode"

// BuildAgentCard returns an AgentCard for the CodeForge service.
// Skills are built dynamically from registered modes. If no modes are
// provided, a single fallback skill is returned for backward compatibility.
func BuildAgentCard(baseURL string, modes []mode.Mode) AgentCard {
	return AgentCard{
		Name:        "CodeForge",
		Description: "AI coding agent orchestration platform",
		URL:         baseURL,
		Version:     "0.1.0",
		Skills:      buildSkillsFromModes(modes),
		Capabilities: struct {
			Streaming bool `json:"streaming"`
		}{Streaming: true},
	}
}

// buildSkillsFromModes converts registered modes into A2A skills.
func buildSkillsFromModes(modes []mode.Mode) []Skill {
	if len(modes) == 0 {
		return []Skill{
			{
				ID:          "code-task",
				Name:        "Code Task",
				Description: "Execute a coding task using an AI agent",
				InputModes:  []string{"text"},
				OutputModes: []string{"text"},
			},
		}
	}
	skills := make([]Skill, len(modes))
	for i := range modes {
		skills[i] = Skill{
			ID:          modes[i].ID,
			Name:        modes[i].Name,
			Description: modes[i].Description,
			InputModes:  []string{"text"},
			OutputModes: []string{"text"},
		}
	}
	return skills
}
