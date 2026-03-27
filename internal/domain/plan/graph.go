package plan

// GraphNode represents a single step in a plan's DAG visualization.
type GraphNode struct {
	ID        string   `json:"id"`
	TaskID    string   `json:"task_id"`
	AgentID   string   `json:"agent_id"`
	ModeID    string   `json:"mode_id,omitempty"`
	Status    string   `json:"status"`
	RunID     string   `json:"run_id,omitempty"`
	Round     int      `json:"round"`
	Error     string   `json:"error,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// GraphEdge represents a dependency between two steps.
type GraphEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Protocol string `json:"protocol"`
}

// Graph is a frontend-friendly DAG representation of an execution plan.
type Graph struct {
	PlanID   string      `json:"plan_id"`
	Name     string      `json:"name"`
	Protocol string      `json:"protocol"`
	Status   string      `json:"status"`
	Nodes    []GraphNode `json:"nodes"`
	Edges    []GraphEdge `json:"edges"`
}

// BuildGraph converts an ExecutionPlan into a Graph for visualization.
func (p *ExecutionPlan) BuildGraph() *Graph {
	g := &Graph{
		PlanID:   p.ID,
		Name:     p.Name,
		Protocol: string(p.Protocol),
		Status:   string(p.Status),
		Nodes:    make([]GraphNode, 0, len(p.Steps)),
		Edges:    make([]GraphEdge, 0),
	}

	for i := range p.Steps {
		step := &p.Steps[i]
		g.Nodes = append(g.Nodes, GraphNode{
			ID:        step.ID,
			TaskID:    step.TaskID,
			AgentID:   step.AgentID,
			ModeID:    step.ModeID,
			Status:    string(step.Status),
			RunID:     step.RunID,
			Round:     step.Round,
			Error:     step.Error,
			DependsOn: step.DependsOn,
		})

		for _, dep := range step.DependsOn {
			g.Edges = append(g.Edges, GraphEdge{
				From:     dep,
				To:       step.ID,
				Protocol: string(p.Protocol),
			})
		}
	}

	return g
}
