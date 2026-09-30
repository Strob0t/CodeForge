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

// Unsuccessful reports whether a step ended without completing its work: it
// failed or was cancelled. A skipped step never ran.
func (s StepStatus) Unsuccessful() bool {
	return s == StepStatusFailed || s == StepStatusCancelled
}

// AnyUnsuccessful returns true if at least one step failed or was cancelled.
// Such a plan cannot complete as planned.
func AnyUnsuccessful(steps []Step) bool {
	for i := range steps {
		if steps[i].Status.Unsuccessful() {
			return true
		}
	}
	return false
}

// BlockedSteps returns the IDs of pending steps that can never run: one of
// their dependencies failed, was cancelled or was skipped, directly or through
// other blocked steps.
func BlockedSteps(steps []Step) []string {
	dead := make(map[string]bool, len(steps))
	for i := range steps {
		if steps[i].Status.Unsuccessful() || steps[i].Status == StepStatusSkipped {
			dead[steps[i].ID] = true
		}
	}
	var blocked []string
	for changed := true; changed; {
		changed = false
		for i := range steps {
			st := &steps[i]
			if st.Status != StepStatusPending || dead[st.ID] {
				continue
			}
			for _, dep := range st.DependsOn {
				if dead[dep] {
					dead[st.ID] = true
					blocked = append(blocked, st.ID)
					changed = true
					break
				}
			}
		}
	}
	return blocked
}

// DependentSteps returns the IDs of the steps that depend on stepID, directly
// or through other steps, each once and in plan order.
func DependentSteps(steps []Step, stepID string) []string {
	dependent := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for i := range steps {
			st := &steps[i]
			if dependent[st.ID] {
				continue
			}
			for _, dep := range st.DependsOn {
				if dep == stepID || dependent[dep] {
					dependent[st.ID] = true
					changed = true
					break
				}
			}
		}
	}
	var ids []string
	for i := range steps {
		if dependent[steps[i].ID] {
			ids = append(ids, steps[i].ID)
		}
	}
	return ids
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
