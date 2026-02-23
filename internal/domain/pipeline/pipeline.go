// Package pipeline defines reusable pipeline templates for multi-agent orchestration.
// Templates are plan factories: YAML definitions that, when instantiated with
// project-specific bindings, produce a CreatePlanRequest for the orchestrator.
package pipeline

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

var (
	ErrNameRequired      = errors.New("template name is required")
	ErrIDRequired        = errors.New("template id is required")
	ErrNoSteps           = errors.New("template must have at least one step")
	ErrStepMissingName   = errors.New("step name is required")
	ErrStepMissingMode   = errors.New("step mode_id is required")
	ErrInvalidProtocol   = errors.New("invalid protocol")
	ErrDAGCycle          = errors.New("step dependencies contain a cycle")
	ErrDAGInvalidRef     = errors.New("step dependency references invalid index")
	ErrBindingCount      = errors.New("binding count must match step count")
	ErrBindingMissingIDs = errors.New("binding must have task_id and agent_id")
)

// Template defines a reusable pipeline structure that can be instantiated
// into a CreatePlanRequest. Templates are loaded from YAML files.
type Template struct {
	ID          string        `json:"id" yaml:"id"`
	Name        string        `json:"name" yaml:"name"`
	Description string        `json:"description" yaml:"description"`
	Builtin     bool          `json:"builtin" yaml:"-"`
	Protocol    plan.Protocol `json:"protocol" yaml:"protocol"`
	MaxParallel int           `json:"max_parallel" yaml:"max_parallel"`
	Steps       []Step        `json:"steps" yaml:"steps"`
}

// Step defines one unit of work in a pipeline template.
// Unlike plan.CreateStepRequest, a template step uses mode references
// instead of task/agent IDs (those are provided at instantiation time).
type Step struct {
	Name          string `json:"name" yaml:"name"`
	ModeID        string `json:"mode_id" yaml:"mode_id"`
	PolicyProfile string `json:"policy_profile,omitempty" yaml:"policy_profile,omitempty"`
	DeliverMode   string `json:"deliver_mode,omitempty" yaml:"deliver_mode,omitempty"`
	DependsOn     []int  `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
}

// StepBinding maps a template step to project-specific task and agent.
type StepBinding struct {
	TaskID  string `json:"task_id"`
	AgentID string `json:"agent_id"`
}

// InstantiateRequest holds the parameters for creating a plan from a template.
type InstantiateRequest struct {
	ProjectID string        `json:"project_id"`
	TeamID    string        `json:"team_id,omitempty"`
	PlanName  string        `json:"plan_name,omitempty"`
	Bindings  []StepBinding `json:"bindings"`
}

// Validate checks the template for structural correctness.
func (t *Template) Validate() error {
	if t.ID == "" {
		return ErrIDRequired
	}
	if t.Name == "" {
		return ErrNameRequired
	}

	switch t.Protocol {
	case plan.ProtocolSequential, plan.ProtocolParallel, plan.ProtocolPingPong, plan.ProtocolConsensus:
		// ok
	default:
		return ErrInvalidProtocol
	}

	if len(t.Steps) == 0 {
		return ErrNoSteps
	}

	for i, s := range t.Steps {
		if s.Name == "" {
			return fmt.Errorf("step %d: %w", i, ErrStepMissingName)
		}
		if s.ModeID == "" {
			return fmt.Errorf("step %d: %w", i, ErrStepMissingMode)
		}
	}

	return t.validateDAG()
}

// validateDAG checks that step dependencies form a valid DAG using Kahn's algorithm.
func (t *Template) validateDAG() error {
	n := len(t.Steps)
	inDegree := make([]int, n)
	adj := make([][]int, n)

	for i, s := range t.Steps {
		for _, dep := range s.DependsOn {
			if dep < 0 || dep >= n {
				return fmt.Errorf("step %d depends on %d: %w", i, dep, ErrDAGInvalidRef)
			}
			if dep == i {
				return fmt.Errorf("step %d depends on itself: %w", i, ErrDAGCycle)
			}
			adj[dep] = append(adj[dep], i)
			inDegree[i]++
		}
	}

	queue := make([]int, 0, n)
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, neighbor := range adj[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if visited != n {
		return ErrDAGCycle
	}
	return nil
}

// Instantiate produces a CreatePlanRequest from the template and bindings.
func (t *Template) Instantiate(req InstantiateRequest) (*plan.CreatePlanRequest, error) {
	if len(req.Bindings) != len(t.Steps) {
		return nil, fmt.Errorf(
			"template %q has %d steps but got %d bindings: %w",
			t.ID, len(t.Steps), len(req.Bindings), ErrBindingCount,
		)
	}

	for i, b := range req.Bindings {
		if b.TaskID == "" || b.AgentID == "" {
			return nil, fmt.Errorf("binding %d: %w", i, ErrBindingMissingIDs)
		}
	}

	name := t.Name
	if req.PlanName != "" {
		name = req.PlanName
	}

	steps := make([]plan.CreateStepRequest, len(t.Steps))
	for i, ts := range t.Steps {
		deps := make([]string, len(ts.DependsOn))
		for j, d := range ts.DependsOn {
			deps[j] = strconv.Itoa(d)
		}
		steps[i] = plan.CreateStepRequest{
			TaskID:        req.Bindings[i].TaskID,
			AgentID:       req.Bindings[i].AgentID,
			ModeID:        ts.ModeID,
			PolicyProfile: ts.PolicyProfile,
			DeliverMode:   ts.DeliverMode,
			DependsOn:     deps,
		}
	}

	return &plan.CreatePlanRequest{
		Name:        name,
		Description: t.Description,
		ProjectID:   req.ProjectID,
		TeamID:      req.TeamID,
		Protocol:    t.Protocol,
		MaxParallel: t.MaxParallel,
		Steps:       steps,
	}, nil
}
