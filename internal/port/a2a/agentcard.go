package a2a

// BuildAgentCard returns a static AgentCard for the CodeForge service.
func BuildAgentCard(baseURL string) AgentCard {
	return AgentCard{
		Name:        "CodeForge",
		Description: "AI coding agent orchestration platform",
		URL:         baseURL,
		Version:     "0.1.0",
		Skills: []Skill{
			{
				ID:          "code-task",
				Name:        "Code Task",
				Description: "Execute a coding task using an AI agent",
				InputModes:  []string{"text"},
				OutputModes: []string{"text"},
			},
			{
				ID:          "decompose",
				Name:        "Feature Decomposition",
				Description: "Decompose a feature into implementation subtasks",
				InputModes:  []string{"text"},
				OutputModes: []string{"text"},
			},
		},
		Capabilities: struct {
			Streaming bool `json:"streaming"`
		}{Streaming: true},
	}
}
