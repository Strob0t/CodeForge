package plan

import (
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrNameRequired        = errors.New("name is required")
	ErrInvalidProtocol     = errors.New("invalid protocol: must be sequential, parallel, ping_pong, or consensus")
	ErrNoSteps             = errors.New("at least one step is required")
	ErrPingPongStepCount   = errors.New("ping_pong protocol requires exactly 2 steps")
	ErrConsensusStepCount  = errors.New("consensus protocol requires at least 2 steps")
	ErrConsensusSameTask   = errors.New("consensus protocol requires all steps to use the same task")
	ErrDAGCycle            = errors.New("step dependencies contain a cycle")
	ErrDAGInvalidRef       = errors.New("step dependency references invalid index")
	ErrStepMissingTask     = errors.New("step task_id is required")
	ErrStepMissingAgent    = errors.New("step agent_id is required")
	ErrMaxParallelNegative = errors.New("max_parallel must be >= 0")
)

// Validate checks the CreatePlanRequest for structural correctness.
func (r *CreatePlanRequest) Validate() error {
	if r.Name == "" {
		return ErrNameRequired
	}
	if r.MaxParallel < 0 {
		return ErrMaxParallelNegative
	}

	switch r.Protocol {
	case ProtocolSequential, ProtocolParallel, ProtocolPingPong, ProtocolConsensus:
		// ok
	default:
		return ErrInvalidProtocol
	}

	if len(r.Steps) == 0 {
		return ErrNoSteps
	}

	for i, s := range r.Steps {
		if s.TaskID == "" {
			return fmt.Errorf("step %d: %w", i, ErrStepMissingTask)
		}
		if s.AgentID == "" {
			return fmt.Errorf("step %d: %w", i, ErrStepMissingAgent)
		}
	}

	// Protocol-specific checks
	switch r.Protocol {
	case ProtocolPingPong:
		if len(r.Steps) != 2 {
			return ErrPingPongStepCount
		}
	case ProtocolConsensus:
		if len(r.Steps) < 2 {
			return ErrConsensusStepCount
		}
		firstTask := r.Steps[0].TaskID
		for _, s := range r.Steps[1:] {
			if s.TaskID != firstTask {
				return ErrConsensusSameTask
			}
		}
	}

	return validateDAG(r.Steps)
}

// validateDAG checks that step dependencies form a valid DAG using Kahn's algorithm.
func validateDAG(steps []CreateStepRequest) error {
	n := len(steps)
	inDegree := make([]int, n)
	adj := make([][]int, n)

	for i, s := range steps {
		for _, dep := range s.DependsOn {
			idx, err := strconv.Atoi(dep)
			if err != nil || idx < 0 || idx >= n {
				return fmt.Errorf("step %d depends on %q: %w", i, dep, ErrDAGInvalidRef)
			}
			if idx == i {
				return fmt.Errorf("step %d depends on itself: %w", i, ErrDAGCycle)
			}
			adj[idx] = append(adj[idx], i)
			inDegree[i]++
		}
	}

	// Kahn's algorithm: topological sort
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
