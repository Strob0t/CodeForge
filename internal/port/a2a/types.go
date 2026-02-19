package a2a

// AgentCard describes an agent's capabilities per the A2A protocol.
type AgentCard struct {
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	URL          string  `json:"url"`
	Version      string  `json:"version"`
	Skills       []Skill `json:"skills"`
	Capabilities struct {
		Streaming bool `json:"streaming"`
	} `json:"capabilities"`
}

// Skill describes a single capability of the agent.
type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	InputModes  []string `json:"inputModes"`
	OutputModes []string `json:"outputModes"`
}

// TaskRequest represents an incoming A2A task request.
type TaskRequest struct {
	ID      string         `json:"id"`
	Skill   string         `json:"skill"`
	Input   map[string]any `json:"input"`             //nolint:gosec // A2A protocol requires flexible input
	Context map[string]any `json:"context,omitempty"` //nolint:gosec // A2A protocol requires flexible context
}

// TaskResponse represents an A2A task response.
type TaskResponse struct {
	ID     string         `json:"id"`
	Status string         `json:"status"`           // "queued", "running", "completed", "failed"
	Output map[string]any `json:"output,omitempty"` //nolint:gosec // A2A protocol requires flexible output
	Error  string         `json:"error,omitempty"`
}
