package plan

// ReadySteps returns the IDs of steps that are pending and have all dependencies completed.
func ReadySteps(steps []Step) []string {
	completed := make(map[string]bool, len(steps))
	for i := range steps {
		if steps[i].Status == StepStatusCompleted {
			completed[steps[i].ID] = true
		}
	}

	var ready []string
	for i := range steps {
		if steps[i].Status != StepStatusPending {
			continue
		}
		allDepsComplete := true
		for _, dep := range steps[i].DependsOn {
			if !completed[dep] {
				allDepsComplete = false
				break
			}
		}
		if allDepsComplete {
			ready = append(ready, steps[i].ID)
		}
	}
	return ready
}

// RunningCount returns the number of steps currently running.
func RunningCount(steps []Step) int {
	count := 0
	for i := range steps {
		if steps[i].Status == StepStatusRunning {
			count++
		}
	}
	return count
}

// AllTerminal returns true if every step is in a terminal state.
func AllTerminal(steps []Step) bool {
	for i := range steps {
		if !steps[i].Status.IsTerminal() {
			return false
		}
	}
	return true
}

// AnyFailed returns true if at least one step has failed.
func AnyFailed(steps []Step) bool {
	for i := range steps {
		if steps[i].Status == StepStatusFailed {
			return true
		}
	}
	return false
}
